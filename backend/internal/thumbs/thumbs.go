// Package thumbs는 og:image를 내려받아 리사이즈·저장하는 계약을 정의한다.
// (스펙 docs/v2/ko/05 §5 — 최대 폭 640px, JPEG q80, 단일 사이즈)
//
// [경계]
//   - Save는 best-effort다: 호출자(thumb 잡 핸들러)는 실패해도 링크 상태를 바꾸지 않고
//     thumb_path만 NULL로 남긴다. 성공 시 반환된 relPath를 store.SetThumbPath로 기록한다.
//   - 경로 규칙: data/thumbs/{hash[:2]}/{hash}.jpg. 반환하는 relPath는 data/thumbs/ 이하
//     상대 경로 "{hash[:2]}/{hash}.jpg" — links.thumb_path에 그대로 저장되고,
//     API 계층이 "/thumbs/" 접두를 붙여 서빙 URL로 변환한다.
package thumbs

import (
	"context"
	"fmt"
	"path"

	"golang.org/x/image/draw"
)

// 썸네일 규격 (스펙 docs/v2/ko/05 §5): 최대 폭 640px, JPEG q80, 단일 사이즈.
const (
	MaxWidth    = 640 // 리사이즈 최대 폭(px). 원본이 더 작으면 확대하지 않는다.
	JPEGQuality = 80  // JPEG 인코딩 품질.
)

// Resampler는 리사이즈 보간 커널 — 다음 단계 diskStore 구현이 이 커널로 640px 축소한다.
var Resampler draw.Interpolator = draw.CatmullRom

// Store는 이미지 URL을 받아 리사이즈된 썸네일을 디스크에 저장하고 상대 경로를 반환한다.
type Store interface {
	// Save는 imageURL을 내려받아 최대 폭 640px JPEG q80으로 리사이즈하고
	// data/thumbs/{urlHash[:2]}/{urlHash}.jpg에 저장한다. 반환 relPath는
	// data/thumbs/ 이하 상대 경로 "{urlHash[:2]}/{urlHash}.jpg".
	Save(ctx context.Context, urlHash, imageURL string) (relPath string, err error)
}

// RelPath는 urlHash에 대한 썸네일 상대 경로 "{hash[:2]}/{hash}.jpg"를 만든다
// (data/thumbs/ 기준). urlHash는 SHA-256 hex(64자)라 항상 2자 이상이다.
func RelPath(urlHash string) string {
	if len(urlHash) < 2 {
		// 방어: 규약상 도달 불가 — 잘못된 해시를 조용히 삼키지 않는다.
		panic(fmt.Sprintf("thumbs: url_hash가 너무 짧음: %q", urlHash))
	}
	return path.Join(urlHash[:2], urlHash+".jpg")
}
