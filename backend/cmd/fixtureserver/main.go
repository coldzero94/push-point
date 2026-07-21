// fixtureserver — scripts/test_crash.sh가 기동하는 결정적 fixture HTTP 서버.
//
// og 메타(title/description/image)를 담은 HTML을 -delay 만큼 지연 응답해
// pushpoint 서버에 kill -9를 날릴 때 scrape 잡이 running 상태로 남도록
// 타이밍을 벌어 준다. 썸네일용 작은 JPEG도 서빙한다.
// 외부 네트워크 의존 없이 크래시 복구 테스트를 결정적으로 만드는 것이 목적이다.
//
//	fixtureserver -addr 127.0.0.1:19090 -delay 500ms
//
// stdlib만 사용한다 (독립 바이너리 — 새 의존 없음).
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"net/http"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:19090", "listen 주소")
	delay := flag.Duration("delay", 500*time.Millisecond, "page 응답 지연 (크래시 타이밍 확보용)")
	flag.Parse()

	// 썸네일 원본 JPEG은 한 번만 인코드해 재사용한다.
	img := smallJPEG()

	mux := http.NewServeMux()

	// 준비 확인용 — 지연 없음 (test_crash.sh가 이 엔드포인트로 기동을 기다린다).
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// og 메타를 담은 HTML — -delay 만큼 지연 응답한다.
	// og:image는 자기 자신의 /img 엔드포인트를 절대 URL로 가리킨다 (Host 헤더 기준).
	mux.HandleFunc("/page/", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(*delay):
		case <-r.Context().Done(): // 스크래퍼 측 context timeout/취소 — 조용히 중단
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/page/")
		imgURL := fmt.Sprintf("http://%s/img/%s.jpg", r.Host, id)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, pageHTML, id, id, id, imgURL, id, id)
	})

	// 썸네일 원본 — 지연 없음 (thumb 잡은 best-effort).
	mux.HandleFunc("/img/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(img)
	})

	log.Printf("fixtureserver listen=%s delay=%s", *addr, *delay)
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("fixtureserver: %v", err)
	}
}

// pageHTML의 %s 6개: <title> / og:title / og:description / og:image / meta description / <h1>.
const pageHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Fixture Page %s</title>
<meta property="og:title" content="Fixture Title %s">
<meta property="og:description" content="크래시 복구 테스트용 fixture 설명 %s">
<meta property="og:image" content="%s">
<meta property="og:site_name" content="Fixture">
<meta name="description" content="fixture meta description %s">
<meta name="author" content="fixture-author">
</head>
<body><h1>Fixture %s</h1></body>
</html>
`

// smallJPEG는 32x32 단색 JPEG 바이트를 만든다 (썸네일 640px 리사이즈 경로 입력용).
func smallJPEG() []byte {
	im := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			im.Set(x, y, color.RGBA{R: 0x33, G: 0x66, B: 0x99, A: 0xff})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, im, &jpeg.Options{Quality: 90}); err != nil {
		log.Fatalf("fixtureserver: jpeg 인코드 실패: %v", err)
	}
	return buf.Bytes()
}
