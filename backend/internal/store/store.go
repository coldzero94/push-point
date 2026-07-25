// Package store는 SQLite 저장 계층의 도메인 타입과 Store 인터페이스를 정의한다.
// 타입은 DB 표현 기준이다 — 시각은 int64 unix epoch 초, NULL 가능 컬럼은 포인터.
// API 표현(gen 패키지 타입, thumb_url 변환, description 200자 절단 등)으로의
// 매핑은 internal/api 핸들러 계층의 책임이다.
//
// [큐 결합 방식 — 확정]
// sqlite Store 구현체는 생성 시 queue.Queue를 주입받는다:
//
//	func New(db *DB, q queue.Queue) Store
//
// SaveLink는 writer 트랜잭션 하나에서 INSERT links → q.EnqueueTx(tx, queue.KindScrape, id)
// → links_fts INSERT까지 수행하고 커밋한 뒤 q.Wake()를 호출한다. RetryLink도 동일 패턴.
// Store 인터페이스 자체는 큐를 노출하지 않는다 — API 계층은 Store만 본다.
// (별도 트랜잭션 헬퍼 노출·enqueue 클로저 주입 방식은 채택하지 않음.)
package store

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// 센티널 에러 — API 계층이 errors.Is로 HTTP 상태에 매핑한다.
var (
	ErrNotFound      = errors.New("store: 리소스 없음")            // → 404 not_found
	ErrDuplicateTag  = errors.New("store: 태그 이름 중복")          // → 400 invalid_input
	ErrUnknownTag    = errors.New("store: 사전에 없는 태그 이름")      // → 400 invalid_input
	ErrNotFailed     = errors.New("store: failed 상태의 링크가 아님") // → 400 invalid_input
	ErrInvalidCursor = errors.New("store: 커서 파싱 실패")          // → 400 invalid_input
)

// LinkTag는 링크에 부착된 태그 (link_tags JOIN tags).
type LinkTag struct {
	ID         int64
	Name       string
	Source     string   // 'rules' | 'embed' | 'manual'
	Confidence *float64 // manual이면 nil
}

// Link는 목록 항목 (links 한 행 + 부착 태그). description은 절단하지 않은 원문.
type Link struct {
	ID          int64
	URL         string
	Domain      string
	Title       string
	Description string
	ContentType string  // 'video' | 'article' | 'post' | 'other'
	ThumbPath   *string // data/thumbs/ 이하 상대 경로. 없으면 nil
	Status      string  // 'pending' | 'scraping' | 'tagging' | 'done' | 'failed'
	Note        string
	Tags        []LinkTag
	CreatedAt   int64
}

// JobSummary는 링크별 잡 상태 요약. 해당 kind의 잡이 없으면 빈 문자열.
type JobSummary struct {
	Scrape string
	Tag    string
	Thumb  string
}

// LinkDetail은 상세 조회 — Link 전체 필드 + 메타·에러·잡 요약.
type LinkDetail struct {
	Link
	Author      string
	PublishedAt *int64
	DurationSec *int64
	WordCount   *int64
	Lang        string
	Error       string
	UpdatedAt   int64
	Jobs        JobSummary
	// Summary는 추출식 요약(M5). 목록(Link)이 아니라 **상세에만** 있다 — 목록·검색 경로가
	// summary를 모르게 두는 것이 linkCols·scanLink·sqlite_search.go 무변경의 근거다.
	Summary string
}

// facet 값 — tags.facet CHECK 제약(migrations/0003_tag_facet.up.sql)과
// api/openapi.yaml의 TagFacet enum이 같은 집합이어야 한다 (scripts/lint_enums.sh가 검사).
// facet은 색이 아니라 분류 축이다 — 어떤 색으로 그릴지는 각 클라이언트가 정한다.
const (
	FacetCraft   = "craft"
	FacetMedia   = "media"
	FacetLife    = "life"
	FacetNeutral = "neutral" // 계약상 default — 사전에 없는 새 태그는 여기서 태어난다
)

// Tag는 태그 사전 항목. Aliases는 aliases JSON 컬럼을 디코드한 값.
type Tag struct {
	ID        int64
	Name      string
	Aliases   []string
	Facet     string // FacetCraft | FacetMedia | FacetLife | FacetNeutral
	LinkCount int64  // 부착된 (미삭제) 링크 수 — ListTags/CreateTag 응답용
	CreatedAt int64
}

// ScrapeResult는 scrape 잡 핸들러가 links에 반영하는 스크랩 결과다.
// scraper.Metadata를 store가 아는 필드로 매핑한 값 — 순환 import를 피하려 store가 자체 정의한다.
// (site_name 등 links 컬럼이 없는 필드는 여기 포함하지 않는다. thumb 잡 enqueue 여부는 HasImage로만 판단.)
type ScrapeResult struct {
	Title       string // links.title
	Description string // links.description
	Author      string // links.author
	ContentType string // links.content_type — 'video'|'article'|'post'|'other' 중 하나여야 함 (CHECK 제약)
	Lang        string // links.lang
	PublishedAt *int64 // links.published_at (nil이면 NULL)
	DurationSec *int64 // links.duration_sec (nil이면 NULL)
	WordCount   *int64 // links.word_count (nil이면 NULL)
	HasImage    bool   // og:image 존재 여부 — true면 ApplyScrape가 같은 트랜잭션에서 thumb 잡 enqueue
	BodyText    string // links.body_text — 추출 본문(태거·요약 입력 전용, FTS·API 미노출)
}

// SaveInput은 저장 요청. url 외는 전부 optional이며, Title/Description/BodyText는
// **클라이언트 캡처** 값이다 — 서버가 fetch할 수 없는 페이지(SPA·봇 차단·로그인 벽)에서
// 이미 렌더된 콘텐츠를 가진 클라이언트가 함께 보낸다. 길이 절단·정제는 호출자(API 계층)가
// 이미 끝낸 상태로 넘긴다. 위치 인자 대신 구조체인 이유: 필드가 더 붙을 자리(M4 iOS)이고,
// 인자 순서 실수가 조용한 데이터 오염이 되기 때문이다.
type SaveInput struct {
	URL         string
	Note        string
	Title       string
	Description string
	BodyText    string // 비어 있지 않으면 body_source='client'로 표시된다
}

// LinkContent는 tag 잡 핸들러가 태거에 넘길 링크 콘텐츠.
type LinkContent struct {
	Domain      string
	Title       string
	Description string
	Note        string
	Body        string // 추출 본문(body_text) — 있으면 태거의 강한 신호
}

// TagDictEntry는 태그 사전 한 항목(DB tags 행). 핸들러가 tagger.TagEntry로 변환한다.
// store가 tagger를 import하지 않도록(역방향 결합 회피) store 쪽에서 자체 정의한다.
type TagDictEntry struct {
	ID      int64
	Name    string
	Aliases []string
	Facet   string
}

// ScoredTag는 태거가 낸 태그 부착 지시(tagger.ScoredTag의 store 측 대응).
type ScoredTag struct {
	TagID      int64
	Confidence float64
}

// SearchMode는 검색 경로. q 3자 이상 → FTS5, 미만 → LIKE 폴백.
type SearchMode string

const (
	SearchModeFTS  SearchMode = "fts"
	SearchModeLike SearchMode = "like"
)

// SearchResult는 검색 결과 항목. Rank는 bm25 점수(낮을수록 관련도 높음),
// LIKE 폴백이면 nil.
type SearchResult struct {
	Link
	Rank *float64
}

// TagCount / DayCount / Stats — GET /api/v1/stats 응답 재료.
type TagCount struct {
	Name  string
	Count int64
}

type DayCount struct {
	Date  string // "2026-07-20" (localtime)
	Count int64
}

type Stats struct {
	TotalLinks    int64
	LinksThisWeek int64
	ByTag         []TagCount
	ByDay         []DayCount // 최근 30일
}

// Store는 저장 계층 인터페이스. 모든 쓰기는 writer 커넥션의 트랜잭션에서 수행하고,
// links_fts 동기화(DELETE 후 INSERT)를 같은 트랜잭션에 포함한다.
// 목록·검색 페이지네이션은 keyset 커서 — OFFSET 금지.
type Store interface {
	// SaveLink는 INSERT links + scrape 잡 enqueue를 한 트랜잭션으로 커밋한다.
	// url_hash(SHA-256 hex) 중복이면 기존 id와 duplicate=true를 반환한다.
	// 소프트 삭제된 행과 중복이면 같은 트랜잭션에서 undelete(pending 복귀, note 교체,
	// scrape 재-enqueue, FTS 재색인)하고 duplicate=false로 반환한다 — 신규 저장과 동일 취급.
	// createdAt은 DB가 실제로 기록한 값 (INSERT ... RETURNING / 기존 행 조회).
	//
	// in.BodyText가 있으면(클라이언트 캡처): 3필드를 함께 저장하고 body_source='client'로
	// 표시하며, **tag 잡도 같은 트랜잭션에서 enqueue**한다 — 그러지 않으면 스크랩이 실패하는
	// 바로 그 페이지에서 태그·요약이 영원히 안 생긴다(tag 잡의 유일한 생성 지점이 ApplyScrape다).
	// 중복(미삭제) 링크라도 저장된 본문이 서버 출처면 **1회 보충**한다(이미 클라이언트 본문이면
	// 무동작) — 반복 호출은 같은 상태로 수렴하므로 재시도 안전성은 유지된다.
	SaveLink(ctx context.Context, in SaveInput) (id, createdAt int64, duplicate bool, err error)

	// GetLink는 상세 조회. 소프트 삭제됐거나 없으면 ErrNotFound.
	GetLink(ctx context.Context, id int64) (*LinkDetail, error)

	// GetLinkURL은 scrape/thumb 잡 핸들러가 link_id로부터 원본 URL과 url_hash를 얻는다.
	// urlHash는 썸네일 경로 규칙(data/thumbs/{hash[:2]}/{hash}.jpg)에 쓰인다.
	// 소프트 삭제됐거나 없으면 ErrNotFound.
	GetLinkURL(ctx context.Context, linkID int64) (url, urlHash string, err error)

	// ApplyScrape는 scrape 결과를 한 writer 트랜잭션으로 반영한다:
	// links 메타데이터 UPDATE + status='done' + FTS 재색인 + tag 잡 EnqueueTx(무조건 —
	// 콘텐츠가 준비됐으므로), 그리고 m.HasImage면 thumb 잡도 EnqueueTx. 커밋 성공 후 Wake.
	// 링크가 없거나 소프트 삭제됐으면 ErrNotFound.
	ApplyScrape(ctx context.Context, linkID int64, m ScrapeResult) error

	// SetThumbPath는 thumb 잡 핸들러가 성공 시 호출 — links.thumb_path에 상대 경로를 기록한다.
	// best-effort 경로라 실패해도 링크 상태는 불변(호출자가 판단). 링크 부재/삭제면 ErrNotFound.
	SetThumbPath(ctx context.Context, linkID int64, relPath string) error

	// GetLinkContent는 tag 잡 핸들러가 태거 입력(도메인·제목·설명·메모)을 읽는다.
	// 소프트 삭제됐거나 없으면 ErrNotFound.
	GetLinkContent(ctx context.Context, linkID int64) (LinkContent, error)

	// LoadTagDict는 태그 사전 전체(id/name/aliases/facet)를 읽어 태거에 넘길 형태로 반환한다.
	// 런타임 사전 = DB tags 테이블(마이그레이션 시드 + 사용자 CRUD 확장).
	LoadTagDict(ctx context.Context) ([]TagDictEntry, error)

	// SetSummary는 tag 잡이 추출식 요약을 기록한다(best-effort — 실패해도 태그는 이미 커밋됨).
	// 빈 문자열도 정상 값이다(가드 불통과 = 요약 없음). FTS 재색인은 하지 않는다.
	SetSummary(ctx context.Context, linkID int64, summary string) error

	// ApplyTags는 tag 잡 결과를 한 writer 트랜잭션으로 반영한다: source='rules' 행을 먼저
	// 삭제(재태깅 멱등)한 뒤 scored 태그를 INSERT(같은 태그의 manual 행은 ON CONFLICT DO
	// NOTHING으로 보존), FTS 'tags' 컬럼 재색인. 링크 부재/삭제여도 FK로 무해(멱등).
	ApplyTags(ctx context.Context, linkID int64, scored []ScoredTag) error

	// ListLinks는 keyset 커서 목록. tag는 태그 이름 필터, status는 links.status 필터
	// (각각 빈 문자열이면 미적용). cursor는 이전 응답의 nextCursor (첫 페이지는 "").
	// limit는 호출 전에 보정된 값(1~100)을 전달한다. 다음 페이지가 없으면 nextCursor="".
	ListLinks(ctx context.Context, cursor string, limit int, tag, status string) (items []Link, nextCursor string, err error)

	// UpdateLink는 메모/태그를 수정하고 수정된 상세를 반환한다.
	//   - note: nil이면 유지, 아니면 교체.
	//   - tags: nil이면 유지, 아니면 태그 이름 배열로 전체 교체 —
	//     추가분은 link_tags(source='manual', confidence=NULL) + tag_feedback('added'),
	//     제거분은 link_tags 삭제 + tag_feedback('removed'). 사전에 없는 이름이면 ErrUnknownTag.
	// links_fts 재색인 포함, 전부 한 트랜잭션.
	UpdateLink(ctx context.Context, id int64, note *string, tags []string) (*LinkDetail, error)

	// DeleteLink는 소프트 삭제 — deleted_at 기록 + links_fts에서 제거.
	DeleteLink(ctx context.Context, id int64) error

	// RetryLink는 status='failed'인 링크를 pending으로 되돌리고 scrape 잡을
	// 재-enqueue한다 (한 트랜잭션, 커밋 후 Wake). failed가 아니면 ErrNotFailed.
	RetryLink(ctx context.Context, id int64) error

	// ListTags는 태그 사전 전체를 link_count 내림차순으로 반환한다.
	ListTags(ctx context.Context) ([]Tag, error)

	// CreateTag는 태그를 추가한다. 이름 중복(NOCASE)이면 ErrDuplicateTag.
	// facet이 빈 문자열이면 FacetNeutral로 저장한다 (계약의 default).
	CreateTag(ctx context.Context, name string, aliases []string, facet string) (*Tag, error)

	// UpdateTag는 이름/별칭/facet을 수정한다. name/aliases/facet 각각 nil이면 유지.
	UpdateTag(ctx context.Context, id int64, name *string, aliases []string, facet *string) (*Tag, error)

	// DeleteTag는 사전에서 제거한다 (link_tags 등은 FK CASCADE).
	DeleteTag(ctx context.Context, id int64) error

	// Search는 q 3자 이상이면 FTS5 MATCH + bm25(mode="fts"), 미만이면
	// title/note/description LIKE 폴백(mode="like", Rank=nil, created_at DESC).
	// LIKE 바인딩 전 %/_/\ 이스케이프 필수. from/to는 created_at 범위(nil이면 미적용).
	Search(ctx context.Context, q, tag string, from, to *int64, cursor string, limit int) (items []SearchResult, nextCursor string, mode SearchMode, err error)

	// Stats는 위젯용 통계를 반환한다.
	Stats(ctx context.Context) (*Stats, error)

	// Close는 writer/reader 풀을 닫는다.
	Close() error
}

// ---- keyset 커서 인코딩 (공용 헬퍼 — 목록/검색 구현이 공유) ----
//
// 커서는 모드 판별자 + keyset 값을 "mode:a:b"로 적고 base64url로 감싼 불투명
// 토큰이다. 클라이언트는 해석하지 않는다. 모드가 다른 커서(목록 커서를 FTS 검색에
// 전달하는 등)나 형식 오류는 ErrInvalidCursor(→ 400 invalid_input)다.
const (
	cursorModeCreated = "c" // 목록/LIKE 폴백 — (created_at, id) keyset
	cursorModeFTS     = "f" // FTS — (bm25 rank의 float64 비트, id) keyset
)

func encodeCursor(mode string, a, b int64) string {
	raw := mode + ":" + strconv.FormatInt(a, 10) + ":" + strconv.FormatInt(b, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(wantMode, cursor string) (a, b int64, err error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 3 || parts[0] != wantMode {
		return 0, 0, ErrInvalidCursor
	}
	a, err1 := strconv.ParseInt(parts[1], 10, 64)
	b, err2 := strconv.ParseInt(parts[2], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, ErrInvalidCursor
	}
	return a, b, nil
}

// EncodeCursor는 목록/LIKE 폴백의 (createdAt, id)를 불투명 커서로 인코딩한다.
func EncodeCursor(createdAt, id int64) string {
	return encodeCursor(cursorModeCreated, createdAt, id)
}

// DecodeCursor는 목록/LIKE 커서를 (createdAt, id)로 복원한다.
// 형식 오류·모드 불일치(FTS 커서 등)면 ErrInvalidCursor.
func DecodeCursor(cursor string) (createdAt, id int64, err error) {
	return decodeCursor(cursorModeCreated, cursor)
}

// EncodeFTSCursor는 FTS 검색의 (bm25 rank float64 비트, id)를 불투명 커서로 인코딩한다.
func EncodeFTSCursor(rankBits, id int64) string {
	return encodeCursor(cursorModeFTS, rankBits, id)
}

// DecodeFTSCursor는 FTS 커서를 (rankBits, id)로 복원한다.
// 형식 오류·모드 불일치(목록/LIKE 커서 등)면 ErrInvalidCursor.
func DecodeFTSCursor(cursor string) (rankBits, id int64, err error) {
	return decodeCursor(cursorModeFTS, cursor)
}
