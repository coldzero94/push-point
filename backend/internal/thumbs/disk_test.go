package thumbs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// newTestStore는 SSRF 가드 없는 http.Client를 주입한 diskStore를 만든다 — 테스트는
// httptest(127.0.0.1)로 다운로드하므로 프로덕션 기본(safedial 가드)이 루프백을 막지
// 않도록 client를 주입한다. 기존 테스트가 client 주입으로 통과하는 계약을 실현한다.
func newTestStore(dataDir string) Store {
	return NewDiskStore(dataDir, WithHTTPClient(&http.Client{Timeout: fetchTimeout}))
}

// smallWebPBase64는 cwebp로 만든 48×32 무손실 WebP(VP8L)다. YouTube 썸네일이 webp라
// x/image/webp 블랭크 임포트가 유지돼야 디코드된다 — 이 픽스처로 회귀를 잡는다.
const smallWebPBase64 = "UklGRjQAAABXRUJQVlA4TCgAAAAvL8AHALkyRPQ/dhHR/zAQaduU3b/t4SMFaRuwsN0ZwJgAdPUB6v8M"

// makePNG는 w×h 크기의 테스트용 PNG 바이트를 만든다.
func makePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("PNG 인코딩 실패: %v", err)
	}
	return buf.Bytes()
}

// makeJPEG는 w×h 크기의 테스트용 JPEG 바이트를 만든다.
func makeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: 64, B: uint8(y), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("JPEG 인코딩 실패: %v", err)
	}
	return buf.Bytes()
}

// makeHeaderOnlyPNG는 IHDR만 담아 임의의 폭·높이를 "선언"하는 아주 작은 PNG를 만든다.
// 실제 픽셀(IDAT)은 없으므로 몇십 바이트에 불과하지만 image.DecodeConfig는 헤더에서
// 선언 해상도를 그대로 읽는다 — 압축폭탄(작은 입력 → 거대 비트맵 할당)을 재현한다.
func makeHeaderOnlyPNG(t *testing.T, w, h uint32) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("\x89PNG\r\n\x1a\n") // PNG 시그니처
	ihdr := make([]byte, 0, 17)
	ihdr = append(ihdr, 'I', 'H', 'D', 'R')
	var dims [8]byte
	binary.BigEndian.PutUint32(dims[0:4], w)
	binary.BigEndian.PutUint32(dims[4:8], h)
	ihdr = append(ihdr, dims[:]...)
	ihdr = append(ihdr, 8, 6, 0, 0, 0) // bit depth 8, color type RGBA, 압축/필터/인터레이스 0
	var lenb [4]byte
	binary.BigEndian.PutUint32(lenb[:], 13) // IHDR 데이터 길이
	buf.Write(lenb[:])
	buf.Write(ihdr)
	var crcb [4]byte
	binary.BigEndian.PutUint32(crcb[:], crc32.ChecksumIEEE(ihdr))
	buf.Write(crcb[:])
	return buf.Bytes()
}

// serveImage는 지정한 content-type과 body를 반환하고 요청 수를 세는 fixture 서버를 연다.
func serveImage(t *testing.T, contentType string, body []byte, hits *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits != nil {
			atomic.AddInt32(hits, 1)
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// 64자 hex 해시 (테스트 고정값).
const testHash = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

// decodeSaved는 저장된 썸네일 JPEG를 디코드해 이미지 설정(config)을 돌려준다.
func decodeSaved(t *testing.T, dataDir, relPath string) image.Config {
	t.Helper()
	abs := filepath.Join(dataDir, "thumbs", filepath.FromSlash(relPath))
	f, err := os.Open(abs)
	if err != nil {
		t.Fatalf("저장 파일 열기 실패 %s: %v", abs, err)
	}
	defer f.Close()
	cfg, format, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatalf("저장 파일 디코드 실패: %v", err)
	}
	if format != "jpeg" {
		t.Fatalf("저장 포맷이 jpeg가 아님: %q", format)
	}
	return cfg
}

func TestSave_Success(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        func(t *testing.T) []byte
		srcW, srcH  int
		wantW       int // 저장된 JPEG의 기대 폭
		wantH       int // 저장된 JPEG의 기대 높이
	}{
		{
			name:        "작은 PNG는 리사이즈 없이 재인코딩",
			contentType: "image/png",
			body:        func(t *testing.T) []byte { return makePNG(t, 100, 50) },
			wantW:       100, wantH: 50,
		},
		{
			name:        "JPEG 입력 폭 640 정확히는 그대로",
			contentType: "image/jpeg",
			body:        func(t *testing.T) []byte { return makeJPEG(t, 640, 480) },
			wantW:       640, wantH: 480,
		},
		{
			name:        "큰 이미지는 폭 640으로 비율 유지 축소",
			contentType: "image/png",
			body:        func(t *testing.T) []byte { return makePNG(t, 1200, 600) },
			wantW:       640, wantH: 320,
		},
		{
			name:        "content-type에 charset이 붙어도 허용",
			contentType: "image/png; charset=binary",
			body:        func(t *testing.T) []byte { return makePNG(t, 200, 200) },
			wantW:       200, wantH: 200,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			srv := serveImage(t, tc.contentType, tc.body(t), nil)
			store := newTestStore(dataDir)

			relPath, err := store.Save(context.Background(), testHash, srv.URL)
			if err != nil {
				t.Fatalf("Save 실패: %v", err)
			}
			wantRel := testHash[:2] + "/" + testHash + ".jpg"
			if relPath != wantRel {
				t.Fatalf("relPath = %q, 기대 %q", relPath, wantRel)
			}
			cfg := decodeSaved(t, dataDir, relPath)
			if cfg.Width != tc.wantW || cfg.Height != tc.wantH {
				t.Fatalf("저장 크기 = %dx%d, 기대 %dx%d", cfg.Width, cfg.Height, tc.wantW, tc.wantH)
			}
			if cfg.Width > MaxWidth {
				t.Fatalf("저장 폭 %d가 MaxWidth %d 초과", cfg.Width, MaxWidth)
			}
		})
	}
}

func TestSave_Idempotent(t *testing.T) {
	dataDir := t.TempDir()
	var hits int32
	srv := serveImage(t, "image/png", makePNG(t, 300, 150), &hits)
	store := newTestStore(dataDir)

	rel1, err := store.Save(context.Background(), testHash, srv.URL)
	if err != nil {
		t.Fatalf("첫 Save 실패: %v", err)
	}
	rel2, err := store.Save(context.Background(), testHash, srv.URL)
	if err != nil {
		t.Fatalf("둘째 Save 실패: %v", err)
	}
	if rel1 != rel2 {
		t.Fatalf("멱등 위반: %q != %q", rel1, rel2)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("재다운로드 발생: 요청 수 %d, 기대 1", got)
	}
}

func TestSave_Errors(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        []byte
		status      int
		wantSubstr  string
	}{
		{
			name:        "이미지 아닌 content-type 거부",
			contentType: "text/html",
			body:        []byte("<html>not an image</html>"),
			status:      http.StatusOK,
			wantSubstr:  "content-type",
		},
		{
			name:        "손상 이미지 디코드 에러",
			contentType: "image/png",
			body:        []byte("\x89PNG\r\n\x1a\n corrupted garbage bytes"),
			status:      http.StatusOK,
			wantSubstr:  "디코드",
		},
		{
			name:        "200 아닌 상태 코드 거부",
			contentType: "image/png",
			body:        nil,
			status:      http.StatusNotFound,
			wantSubstr:  "상태 코드",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				_, _ = w.Write(tc.body)
			}))
			t.Cleanup(srv.Close)
			store := newTestStore(dataDir)

			_, err := store.Save(context.Background(), testHash, srv.URL)
			if err == nil {
				t.Fatalf("에러를 기대했으나 nil")
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("에러 %q 에 %q 미포함", err.Error(), tc.wantSubstr)
			}
			// 실패 시 파일이 남지 않아야 한다.
			abs := filepath.Join(dataDir, "thumbs", testHash[:2], testHash+".jpg")
			if _, statErr := os.Stat(abs); !os.IsNotExist(statErr) {
				t.Fatalf("실패했는데 파일이 남음: %s", abs)
			}
		})
	}
}

func TestSave_RejectsDecompressionBomb(t *testing.T) {
	// F1: 작게 압축됐지만 헤더에 거대한 해상도를 선언한 이미지는 image.Decode(원본
	// 비트맵 전체 할당) 전에 거부돼야 한다 — 단일 프로세스 OOM 방어. 헤더만 담은
	// 수십 바이트 PNG로 재현하므로 테스트가 실제 거대 비트맵을 할당하지 않는다.
	tests := []struct {
		name string
		w, h uint32
	}{
		{"양변 모두 과대 (25000x25000)", 25000, 25000}, // 변·픽셀 상한 동시 초과
		{"픽셀 곱 초과 (8000x8000=64MP)", 8000, 8000}, // 각 변은 상한 이하, 픽셀 곱만 초과
		{"한 변만 비정상 (30000x10)", 30000, 10},       // 픽셀 곱은 작지만 한 변이 상한 초과
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			body := makeHeaderOnlyPNG(t, tc.w, tc.h)
			// 압축폭탄 전제 검증: 입력 자체는 아주 작다.
			if len(body) > 1024 {
				t.Fatalf("헤더 온리 PNG가 너무 큼: %d bytes", len(body))
			}
			srv := serveImage(t, "image/png", body, nil)
			store := newTestStore(dataDir)

			_, err := store.Save(context.Background(), testHash, srv.URL)
			if err == nil {
				t.Fatalf("%dx%d 선언 이미지에 에러를 기대했으나 nil", tc.w, tc.h)
			}
			if !strings.Contains(err.Error(), "해상도") {
				t.Fatalf("에러 %q 에 해상도 미포함", err.Error())
			}
			// 거부 시 썸네일 파일이 남지 않아야 한다.
			abs := filepath.Join(dataDir, "thumbs", testHash[:2], testHash+".jpg")
			if _, statErr := os.Stat(abs); !os.IsNotExist(statErr) {
				t.Fatalf("거부됐는데 파일이 남음: %s", abs)
			}
		})
	}
}

func TestSave_AllowsLargeButBoundedImage(t *testing.T) {
	// 상한 이하의 큰(그러나 폭탄 아닌) 이미지는 통과해 640px로 축소 저장돼야 한다.
	// 실제 픽셀을 가진 이미지를 쓰되 할당이 과하지 않은 3000x2000(6MP)로 검증한다.
	dataDir := t.TempDir()
	srv := serveImage(t, "image/png", makePNG(t, 3000, 2000), nil)
	store := newTestStore(dataDir)

	relPath, err := store.Save(context.Background(), testHash, srv.URL)
	if err != nil {
		t.Fatalf("정상(경계 이하) 이미지 Save 실패: %v", err)
	}
	cfg := decodeSaved(t, dataDir, relPath)
	if cfg.Width != MaxWidth {
		t.Fatalf("저장 폭 = %d, 기대 %d", cfg.Width, MaxWidth)
	}
}

func TestSave_DecodesWebP(t *testing.T) {
	// YouTube 썸네일은 webp — x/image/webp 블랭크 임포트가 제거되면 디코드가 깨진다.
	// 실제 48×32 무손실 WebP를 내려받아 디코드→리사이즈 판정→JPEG 재인코딩까지 성공해야 한다.
	data, err := base64.StdEncoding.DecodeString(smallWebPBase64)
	if err != nil {
		t.Fatalf("webp 픽스처 디코드 실패: %v", err)
	}
	dataDir := t.TempDir()
	srv := serveImage(t, "image/webp", data, nil)
	store := newTestStore(dataDir)

	relPath, err := store.Save(context.Background(), testHash, srv.URL)
	if err != nil {
		t.Fatalf("webp Save 실패 (webp 디코더 미등록 회귀?): %v", err)
	}
	// 저장물은 항상 JPEG. 48×32는 폭 640 이하라 리사이즈 없이 그대로 재인코딩된다.
	cfg := decodeSaved(t, dataDir, relPath)
	if cfg.Width != 48 || cfg.Height != 32 {
		t.Fatalf("저장 크기 = %dx%d, 기대 48x32", cfg.Width, cfg.Height)
	}
}

func TestSave_RejectsOversizedBody(t *testing.T) {
	// >10MB 본문은 디코드 이전에 크기로 거부돼야 한다 (OOM·대역폭 방어). 크기 검사가
	// 디코드보다 앞서므로 본문이 유효 이미지일 필요는 없다 — 정확히 상한+1 바이트를 보낸다.
	dataDir := t.TempDir()
	oversized := bytes.Repeat([]byte{0}, maxImageBytes+1)
	srv := serveImage(t, "image/png", oversized, nil)
	store := newTestStore(dataDir)

	_, err := store.Save(context.Background(), testHash, srv.URL)
	if err == nil {
		t.Fatalf(">%d bytes(10MB 초과) 본문인데 에러가 nil", maxImageBytes)
	}
	if !strings.Contains(err.Error(), "너무 큼") {
		t.Fatalf("에러 %q 에 '너무 큼' 미포함", err.Error())
	}
	// 거부 시 썸네일 파일이 남지 않아야 한다.
	abs := filepath.Join(dataDir, "thumbs", testHash[:2], testHash+".jpg")
	if _, statErr := os.Stat(abs); !os.IsNotExist(statErr) {
		t.Fatalf("거부됐는데 파일이 남음: %s", abs)
	}
}

func TestSave_InvalidHash(t *testing.T) {
	tests := []struct {
		name string
		hash string
	}{
		{"경로 탈출 시도", "../../etc/passwd"},
		{"빈 문자열", ""},
		{"한 글자", "a"},
		{"슬래시 포함", "ab/cd"},
		{"hex 아닌 문자", "abcdefgh"},
	}
	dataDir := t.TempDir()
	srv := serveImage(t, "image/png", makePNG(t, 10, 10), nil)
	store := newTestStore(dataDir)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.Save(context.Background(), tc.hash, srv.URL)
			if err == nil {
				t.Fatalf("유효하지 않은 해시 %q 에 에러를 기대했으나 nil", tc.hash)
			}
			if !strings.Contains(err.Error(), "url_hash") {
				t.Fatalf("에러 %q 에 url_hash 미포함", err.Error())
			}
		})
	}
}

func TestSave_AtomicNoTempLeftover(t *testing.T) {
	dataDir := t.TempDir()
	srv := serveImage(t, "image/png", makePNG(t, 50, 50), nil)
	store := newTestStore(dataDir)

	if _, err := store.Save(context.Background(), testHash, srv.URL); err != nil {
		t.Fatalf("Save 실패: %v", err)
	}
	// 썸네일 디렉터리에 .tmp 임시 파일이 남지 않아야 한다.
	dir := filepath.Join(dataDir, "thumbs", testHash[:2])
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("디렉터리 읽기 실패: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") || strings.HasPrefix(e.Name(), ".thumb-") {
			t.Fatalf("임시 파일 잔류: %s", e.Name())
		}
	}
	if len(entries) != 1 {
		t.Fatalf("파일 수 %d, 기대 1", len(entries))
	}
}
