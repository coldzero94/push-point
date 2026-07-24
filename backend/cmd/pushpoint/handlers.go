package main

// scrape/thumb/tag 잡 핸들러 배선 — dispatcher가 kind별로 이 핸들러들을 호출한다.
// (M2: scrape가 메타+status='done' 전이, thumb는 best-effort. M3: tag가 규칙 태거로 부착.)

import (
	"context"
	"log/slog"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/coby/push-point/backend/internal/queue"
	"github.com/coby/push-point/backend/internal/scraper"
	"github.com/coby/push-point/backend/internal/store"
	"github.com/coby/push-point/backend/internal/tagger"
	"github.com/coby/push-point/backend/internal/thumbs"
)

// thumbConcurrency는 thumb 워커 동시성 상한 (스펙 §6: thumb 워커 2 goroutine).
const thumbConcurrency = 2

// tagConcurrency는 tag 워커 동시성 상한. 태깅은 네트워크 0인 순수 CPU 작업이라 여유 있게 둔다.
const tagConcurrency = 4

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

		start := time.Now()
		rawURL, _, err := st.GetLinkURL(ctx, job.LinkID)
		if err != nil {
			return err
		}
		m, err := sc.Fetch(ctx, rawURL)
		if err != nil {
			return err
		}
		res := scrapeResult(m)
		if err := st.ApplyScrape(ctx, job.LinkID, res); err != nil {
			return err
		}
		// 잡 성공은 Debug — dev(레벨 debug)에서 잡이 도는지 보이고, 운영(info)에선 조용.
		log.Debug("scrape 완료", "link", job.LinkID, "content_type", res.ContentType, "dur_ms", time.Since(start).Milliseconds())
		return nil
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
		BodyText:    m.BodyText,
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

		start := time.Now()
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
			// 잡별 best-effort 스킵은 Debug — 운영(info) 로그를 조용하게 유지한다.
			log.Debug("thumb: og:image 없음 — 스킵", "link", job.LinkID)
			return nil
		}
		relPath, err := ts.Save(ctx, urlHash, m.ImageURL)
		if err != nil {
			return err
		}
		if err := st.SetThumbPath(ctx, job.LinkID, relPath); err != nil {
			return err
		}
		log.Debug("thumb 저장 완료", "link", job.LinkID, "path", relPath, "dur_ms", time.Since(start).Milliseconds())
		return nil
	}
}

// newTagHandler는 tag 잡 핸들러를 만든다 (best-effort, 순수 CPU). 처리: link_id → 콘텐츠
// 조회 → 태그 사전 로드 → tagger.BuildDictionary + Classify → store.ApplyTags(source='rules').
// 실패는 그대로 반환해 큐가 재시도하지만, max_attempts 소진 후에도 링크 status는 불변이다
// (queue.Fail이 KindTag를 thumb과 함께 best-effort로 처리 — 태깅 실패가 링크를 죽이지 않는다).
func newTagHandler(st store.Store, log *slog.Logger) queue.Handler {
	sem := semaphore.NewWeighted(tagConcurrency)
	return func(ctx context.Context, job *queue.Job) error {
		if err := sem.Acquire(ctx, 1); err != nil {
			return err
		}
		defer sem.Release(1)

		start := time.Now()
		content, err := st.GetLinkContent(ctx, job.LinkID)
		if err != nil {
			return err
		}
		entries, err := st.LoadTagDict(ctx)
		if err != nil {
			return err
		}
		dict := tagger.BuildDictionary(toTagEntries(entries))
		scored := tagger.Classify(toTaggerContent(content), dict)
		if err := st.ApplyTags(ctx, job.LinkID, toStoreScored(scored)); err != nil {
			return err
		}
		log.Debug("tag 완료", "link", job.LinkID, "tags", len(scored), "dur_ms", time.Since(start).Milliseconds())
		return nil
	}
}

// store ↔ tagger 타입 변환 — store가 tagger를 import하지 않도록(층 분리) 핸들러가 다리를 놓는다.
func toTagEntries(es []store.TagDictEntry) []tagger.TagEntry {
	out := make([]tagger.TagEntry, len(es))
	for i, e := range es {
		out[i] = tagger.TagEntry{ID: e.ID, Name: e.Name, Aliases: e.Aliases, Facet: e.Facet}
	}
	return out
}

func toTaggerContent(c store.LinkContent) tagger.Content {
	return tagger.Content{Domain: c.Domain, Title: c.Title, Description: c.Description, Note: c.Note, Body: c.Body}
}

func toStoreScored(ss []tagger.ScoredTag) []store.ScoredTag {
	out := make([]store.ScoredTag, len(ss))
	for i, s := range ss {
		out[i] = store.ScoredTag{TagID: s.TagID, Confidence: s.Confidence}
	}
	return out
}
