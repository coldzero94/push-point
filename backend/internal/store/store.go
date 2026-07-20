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
}

// Tag는 태그 사전 항목. Aliases는 aliases JSON 컬럼을 디코드한 값.
type Tag struct {
	ID        int64
	Name      string
	Aliases   []string
	LinkCount int64 // 부착된 (미삭제) 링크 수 — ListTags/CreateTag 응답용
	CreatedAt int64
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
	// url_hash(SHA-256 hex) 중복이면 기존 id와 duplicate=true를 반환 (멱등, 잡 없음).
	// 소프트 삭제된 행과 중복이면 같은 트랜잭션에서 undelete(pending 복귀, note 교체,
	// scrape 재-enqueue, FTS 재색인)하고 duplicate=false로 반환한다 — 신규 저장과 동일 취급.
	// createdAt은 DB가 실제로 기록한 값 (INSERT ... RETURNING / 기존 행 조회).
	SaveLink(ctx context.Context, url, note string) (id, createdAt int64, duplicate bool, err error)

	// GetLink는 상세 조회. 소프트 삭제됐거나 없으면 ErrNotFound.
	GetLink(ctx context.Context, id int64) (*LinkDetail, error)

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
	CreateTag(ctx context.Context, name string, aliases []string) (*Tag, error)

	// UpdateTag는 이름/별칭을 수정한다. name/aliases 각각 nil이면 유지.
	UpdateTag(ctx context.Context, id int64, name *string, aliases []string) (*Tag, error)

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
