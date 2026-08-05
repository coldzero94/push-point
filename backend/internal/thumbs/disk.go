package thumbs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"  // Encode + JPEG 디코더 등록
	_ "image/png" // PNG 디코더 등록
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // WebP 디코더 등록

	"github.com/coby/push-point/backend/internal/safedial"
)

// 다운로드 제약 (스펙 docs/v2/ko/05 §5): timeout 10s, 최대 10MB, image/* 만 허용.
const (
	fetchTimeout  = 10 * time.Second
	maxImageBytes = 10 << 20 // 10MB
	// maxImagePixels는 디코드 허용 최대 픽셀 수(Width*Height). 압축폭탄 방어:
	// 작게 압축된 입력이 헤더에 거대한 해상도를 선언하면 image.Decode가 그만큼의
	// 원본 비트맵을 통째로 할당해(폭*높이*4B RGBA) 단일 프로세스 전체를 OOM으로
	// 죽인다. 디코드 전에 DecodeConfig로 선언 크기를 검사해 초과분을 거부한다.
	// 40메가픽셀 ≈ 리사이즈 대상 og:image로는 충분히 크고, RGBA로도 ~160MB 상한.
	maxImagePixels = 40_000_000
	// maxImageDim은 각 변의 상한. 극단적 종횡비(예: 1×10^9)로 픽셀 곱은 작아 보이나
	// 한 변이 비정상인 입력도 거부한다.
	maxImageDim = 20000
)

// diskStore는 og:image를 내려받아 리사이즈·JPEG 인코딩 후 로컬 디스크에 저장하는 Store 구현이다.
// 파일은 {dataDir}/thumbs/{hash[:2]}/{hash}.jpg 에 원자적으로 쓴다.
type diskStore struct {
	dataDir string
	client  *http.Client
}

// 컴파일 타임 인터페이스 검증.
var _ Store = (*diskStore)(nil)

// Option은 NewDiskStore의 가변 설정 함수.
type Option func(*diskStore)

// WithHTTPClient는 이미지 다운로드에 쓸 HTTP 클라이언트를 주입한다.
// 테스트가 SSRF 가드 없는 client(httptest 127.0.0.1 접속용)를 넣는 데 쓴다.
func WithHTTPClient(c *http.Client) Option {
	return func(s *diskStore) { s.client = c }
}

// NewDiskStore는 dataDir(예: ./data)를 주입받아 diskStore를 만든다.
// 썸네일은 {dataDir}/thumbs/ 이하에 저장된다. 기본 클라이언트는 SSRF 가드 dial(사설/
// 루프백/링크로컬 거부) + 10s 타임아웃 — og:image URL은 사용자 링크에서 파생되므로 내부
// 주소로 못 나가게 막는다. 테스트는 WithHTTPClient로 가드 없는 client를 주입한다.
func NewDiskStore(dataDir string, opts ...Option) Store {
	s := &diskStore{
		dataDir: dataDir,
		client:  safedial.Client(fetchTimeout),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Save는 imageURL을 내려받아 최대 폭 640px JPEG q80으로 리사이즈하고
// {dataDir}/thumbs/{urlHash[:2]}/{urlHash}.jpg 에 저장한 뒤 상대 경로를 반환한다.
// 이미 파일이 있으면 재다운로드 없이 멱등하게 상대 경로만 반환한다.
func (s *diskStore) Save(ctx context.Context, urlHash, imageURL string) (string, error) {
	// 경로 탈출 방지: urlHash는 SHA-256 hex여야 한다 (../ 등 주입 차단).
	if !validHexHash(urlHash) {
		return "", fmt.Errorf("thumbs: 유효하지 않은 url_hash: %q", urlHash)
	}
	relPath := RelPath(urlHash)
	absPath := filepath.Join(s.dataDir, "thumbs", filepath.FromSlash(relPath))

	// 멱등: 이미 존재하면 재다운로드 스킵.
	if _, err := os.Stat(absPath); err == nil {
		return relPath, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("thumbs: 파일 확인 실패 %s: %w", absPath, err)
	}

	img, err := s.fetchImage(ctx, imageURL)
	if err != nil {
		return "", err
	}
	resized := resize(img)

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return "", fmt.Errorf("thumbs: 디렉터리 생성 실패 %s: %w", filepath.Dir(absPath), err)
	}
	if err := writeJPEGAtomic(absPath, resized); err != nil {
		return "", err
	}
	return relPath, nil
}

// fetchImage는 imageURL을 GET해 image.Image로 디코드한다.
// content-type이 image/* 가 아니거나 10MB를 넘거나 디코드에 실패하면 에러.
func (s *diskStore) fetchImage(ctx context.Context, imageURL string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("thumbs: 요청 생성 실패 %q: %w", imageURL, err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("thumbs: 다운로드 실패 %q: %w", imageURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("thumbs: 다운로드 상태 코드 %d: %s", resp.StatusCode, imageURL)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/") {
		return nil, fmt.Errorf("thumbs: 이미지가 아닌 content-type %q: %s", ct, imageURL)
	}

	// 최대 10MB — 한도+1까지 읽어 초과 여부를 판별한다.
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("thumbs: 본문 읽기 실패 %q: %w", imageURL, err)
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("thumbs: 이미지가 너무 큼 (>%d bytes): %s", maxImageBytes, imageURL)
	}

	// 압축폭탄 방어: 디코드 전에 헤더만 읽어 선언된 픽셀 수를 검사한다.
	// DecodeConfig는 이미 버퍼된 data의 헤더만 소비하므로 재다운로드가 없다.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("thumbs: 이미지 헤더 디코드 실패 %q: %w", imageURL, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxImageDim || cfg.Height > maxImageDim ||
		int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return nil, fmt.Errorf("thumbs: 이미지 해상도 과다 %dx%d (상한 %d px, 변 %d): %s",
			cfg.Width, cfg.Height, maxImagePixels, maxImageDim, imageURL)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("thumbs: 이미지 디코드 실패 %q: %w", imageURL, err)
	}
	return img, nil
}

// resize는 폭이 MaxWidth를 넘으면 비율을 유지해 축소하고, 이하면 원본 그대로 반환한다
// (확대하지 않는다 — 폭 640 이하면 호출자가 재인코딩만 한다).
func resize(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= MaxWidth {
		return src
	}
	newW := MaxWidth
	newH := int(float64(h) * float64(newW) / float64(w))
	if newH < 1 {
		newH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	Resampler.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

// writeJPEGAtomic은 같은 디렉터리에 임시 파일로 JPEG를 쓴 뒤 rename으로 원자적으로 교체한다.
func writeJPEGAtomic(absPath string, img image.Image) error {
	tmp, err := os.CreateTemp(filepath.Dir(absPath), ".thumb-*.tmp")
	if err != nil {
		return fmt.Errorf("thumbs: 임시 파일 생성 실패: %w", err)
	}
	tmpName := tmp.Name()
	// 실패 경로에서 임시 파일을 남기지 않는다 (성공 시 rename 후라 Remove는 no-op).
	defer os.Remove(tmpName)

	if err := jpeg.Encode(tmp, img, &jpeg.Options{Quality: JPEGQuality}); err != nil {
		tmp.Close()
		return fmt.Errorf("thumbs: JPEG 인코딩 실패: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("thumbs: 임시 파일 닫기 실패: %w", err)
	}
	if err := os.Rename(tmpName, absPath); err != nil {
		return fmt.Errorf("thumbs: 원자적 rename 실패 %s: %w", absPath, err)
	}
	return nil
}

// validHexHash는 s가 2자 이상 hex 문자열인지 검증한다 (경로 탈출 방지).
func validHexHash(s string) bool {
	if len(s) < 2 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
