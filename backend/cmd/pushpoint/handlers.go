package main

// scrape/thumb 잡 핸들러 배선 — dispatcher가 kind별로 이 핸들러들을 호출한다.
// (M2: scrape가 메타+status='done' 전이, thumb는 best-effort. tag 핸들러는 M3.)

import (
	"context"
	"log/slog"

	"golang.org/x/sync/semaphore"

	"github.com/coby/push-point/backend/internal/queue"
	"github.com/coby/push-point/backend/internal/scraper"
	"github.com/coby/push-point/backend/internal/store"
	"github.com/coby/push-point/backend/internal/thumbs"
)

// thumbConcurrency는 thumb 워커 동시성 상한 (스펙 §6: thumb 워커 2 goroutine).
const thumbConcurrency = 2

// newScrapeHandler는 scrape 잡 핸들러를 만든다. dispatcher가 잡마다 goroutine을
// 띄우므로(동시성 상한은 핸들러 책임) 여기서 semaphore(concurrency, 기본 8)로 상한을 건다 (스펙 §6).
// 처리: link_id → 원본 URL 조회 → scraper.Fetch(어댑터 라우팅) → store.ApplyScrape로
// 메타+status='done' 반영. og:image가 있으면 ApplyScrape가 같은 트랜잭션에서 thumb 잡을 enqueue한다.
// 네트워크·파싱 실패는 감추지 않고 그대로 반환 — 재시도/확정 실패 판정은 큐 몫이다.
func newScrapeHandler(sc scraper.Scraper, st store.Store, concurrency int, log *slog.Logger) queue.Handler {
	if concurrency < 1 {
		concurrency = 1
	}
	sem := semaphore.NewWeighted(int64(concurrency))
	return func(ctx context.Context, job *queue.Job) error {
		if err := sem.Acquire(ctx, 1); err != nil {
			return err // ctx 취소 — dispatcher가 셧다운 복귀로 처리
		}
		defer sem.Release(1)

		rawURL, _, err := st.GetLinkURL(ctx, job.LinkID)
		if err != nil {
			return err
		}
		m, err := sc.Fetch(ctx, rawURL)
		if err != nil {
			return err
		}
		return st.ApplyScrape(ctx, job.LinkID, scrapeResult(m))
	}
}

// scrapeResult는 scraper.Metadata를 store.ScrapeResult(links 컬럼 매핑)로 변환한다.
// content_type은 CHECK 제약('video'|'article'|'post'|'other')을 만족해야 하므로
// 어댑터 계약상 도달 불가한 빈/미지 값은 'other'로 보정한다 (CHECK 위반을 사전 차단).
// thumb 잡 enqueue 여부는 HasImage로만 판단 — og:image URL은 links에 저장하지 않는다.
func scrapeResult(m scraper.Metadata) store.ScrapeResult {
	ct := m.ContentType
	switch ct {
	case "video", "article", "post", "other":
	default:
		ct = "other"
	}
	return store.ScrapeResult{
		Title:       m.Title,
		Description: m.Description,
		Author:      m.Author,
		ContentType: ct,
		Lang:        m.Lang,
		PublishedAt: m.PublishedAt,
		DurationSec: m.DurationSec,
		WordCount:   m.WordCount,
		HasImage:    m.ImageURL != "",
	}
}

// newThumbHandler는 thumb 잡 핸들러를 만든다 (best-effort, 동시성 상한 thumbConcurrency=2).
// 처리: link_id → URL·url_hash 조회 → scraper.Fetch로 og:image URL 재추출
// → thumbs.Save(다운로드·640px 리사이즈·JPEG q80 저장) → store.SetThumbPath.
// og:image URL은 links에 저장하지 않으므로(스펙: HasImage만 기록) 여기서 재-Fetch로 얻는다.
// 실패는 그대로 반환해 큐가 재시도하지만, max_attempts 소진 후 thumb 잡이 failed여도
// 링크 status는 불변이다 (queue.Fail이 kind='thumb'을 링크와 분리해 처리 — best-effort 의미론).
func newThumbHandler(sc scraper.Scraper, ts thumbs.Store, st store.Store, log *slog.Logger) queue.Handler {
	sem := semaphore.NewWeighted(thumbConcurrency)
	return func(ctx context.Context, job *queue.Job) error {
		if err := sem.Acquire(ctx, 1); err != nil {
			return err
		}
		defer sem.Release(1)

		rawURL, urlHash, err := st.GetLinkURL(ctx, job.LinkID)
		if err != nil {
			return err
		}
		m, err := sc.Fetch(ctx, rawURL)
		if err != nil {
			return err
		}
		if m.ImageURL == "" {
			// 재-Fetch 시점에 og:image가 사라짐 — thumb 없이 성공 완료 (best-effort, 에러 아님).
			log.Info("thumb: og:image 없음 — 스킵", "link", job.LinkID)
			return nil
		}
		relPath, err := ts.Save(ctx, urlHash, m.ImageURL)
		if err != nil {
			return err
		}
		return st.SetThumbPath(ctx, job.LinkID, relPath)
	}
}
