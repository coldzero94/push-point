// Package tagjob은 링크 한 건의 태깅+요약 파이프라인을 담는다.
//
// 이 파이프라인에는 소비자가 둘이다: 서버 워커의 tag 잡 핸들러와, 서버 없이 도는
// iOS Share Extension(mobile/ppshare). 둘이 같은 순서·같은 실패 관용을 지켜야 하므로
// 로직을 여기 한 곳에 둔다 — 한쪽만 고쳐 규칙이 갈라지는 것을 막는다.
//
// scraper를 import하지 않는 것이 이 패키지의 불변식이다. scraper는 호출하지 않고
// 링크만 해도 패키지 init()이 정규식·셀렉터 테이블을 만들어 RSS를 +51MB 올리는데,
// iOS Share Extension 예산(~120MB)에서는 감당할 수 없다(실측 근거는 docs/v2/08).
package tagjob

import (
	"context"

	"github.com/coby/push-point/backend/internal/store"
	"github.com/coby/push-point/backend/internal/summarizer"
	"github.com/coby/push-point/backend/internal/tagger"
)

// Result는 Run이 실제로 한 일. 호출자가 로그·응답에 쓴다.
type Result struct {
	// Tags는 Classify가 뽑은 태그 수(≤ topK). ApplyTags가 실제로 INSERT한 행 수와는
	// 다를 수 있다 — 같은 태그에 manual 행이 이미 있으면 ON CONFLICT DO NOTHING으로
	// 건너뛴다(수동 태그 보존이 의도다).
	Tags int
	// Names는 붙은 태그 이름(Tags와 같은 순서). iOS 확장이 저장 직후 화면에 태그를
	// 보여주려면 개수만으로는 부족하다 — 서버 왕복 없이 그 자리에서 필요한 값이다.
	Names []string
	// SummaryLen은 **DB에 기록된** 요약의 길이다. SummaryErr가 nil이 아니면 0이다 —
	// 기록되지 않은 요약의 길이를 돌려주면 호출자(특히 확장 UI)가 "요약됨"으로 읽는다.
	SummaryLen int
	// SummaryErr는 nil이 아니면 요약 기록만 실패했다는 뜻 — 태그는 이미 커밋됐다.
	SummaryErr error
}

// Run은 링크 하나를 태깅하고 요약한다: 콘텐츠 조회 → 사전 로드 → Classify →
// ApplyTags(source='rules') → Summarize → SetSummary.
//
// 요약은 태그와 **같은 본문 읽기를 재사용**하므로 추가 I/O가 없다. 순서가 중요하다:
// 태그가 코어이고 요약은 부가물이라 ApplyTags 뒤에 오며, 요약 기록이 실패해도
// error를 반환하지 않는다 — thumb 잡과 같은 best-effort 관용이다. 재시도 자체는 안전하다
// (ApplyTags는 source='rules'를 먼저 지워 멱등하다 — TestRun_isIdempotent가 고정한다).
// 다만 이미 커밋된 태그를 위해 잡 전체를 재시도시키는 것은 낭비이고, 요약이 없다고
// 링크가 못 쓰게 되지도 않는다. 실패는 Result.SummaryErr로 올려 호출자가 로그만 남긴다.
func Run(ctx context.Context, st store.Store, linkID int64) (Result, error) {
	content, err := st.GetLinkContent(ctx, linkID)
	if err != nil {
		return Result{}, err
	}
	entries, err := st.LoadTagDict(ctx)
	if err != nil {
		return Result{}, err
	}
	dict := tagger.BuildDictionary(toTagEntries(entries))
	scored := tagger.Classify(toTaggerContent(content), dict)
	if err := st.ApplyTags(ctx, linkID, toStoreScored(scored)); err != nil {
		return Result{}, err
	}

	// 빈 문자열도 정상 값이다(가드 불통과 = 요약 없음).
	sum := summarizer.Summarize(content.Body, content.Description)
	byID := make(map[int64]string, len(entries))
	for _, e := range entries {
		byID[e.ID] = e.Name
	}
	names := make([]string, 0, len(scored))
	for _, sc := range scored {
		if n, ok := byID[sc.TagID]; ok {
			names = append(names, n)
		}
	}
	res := Result{Tags: len(scored), Names: names}
	// SummaryLen은 기록에 **성공한 뒤에만** 채운다. 미리 채우면 실패했을 때
	// "길이가 있는데 DB에는 없는" 상태가 되고, 확장 UI가 그걸 요약 있음으로 읽는다.
	if err := st.SetSummary(ctx, linkID, sum); err != nil {
		res.SummaryErr = err
	} else {
		res.SummaryLen = len(sum)
	}
	return res, nil
}

// store ↔ tagger 타입 변환 — store가 tagger를 import하지 않도록(층 분리) 여기가 다리를 놓는다.
func toTagEntries(es []store.TagDictEntry) []tagger.TagEntry {
	out := make([]tagger.TagEntry, len(es))
	for i, e := range es {
		out[i] = tagger.TagEntry{ID: e.ID, Name: e.Name, Aliases: e.Aliases, Facet: e.Facet}
	}
	return out
}

func toTaggerContent(c store.LinkContent) tagger.Content {
	return tagger.Content{
		Domain: c.Domain, Title: c.Title, Description: c.Description,
		Note: c.Note, Body: c.Body, Keywords: c.Keywords,
	}
}

func toStoreScored(ss []tagger.ScoredTag) []store.ScoredTag {
	out := make([]store.ScoredTag, len(ss))
	for i, s := range ss {
		out[i] = store.ScoredTag{TagID: s.TagID, Confidence: s.Confidence}
	}
	return out
}
