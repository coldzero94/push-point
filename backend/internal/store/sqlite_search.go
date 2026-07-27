// Store 인터페이스의 SQLite 구현 — 검색(FTS5/LIKE 폴백)과 통계.
package store

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// Search는 q가 3자(rune) 이상이면 FTS5 MATCH + bm25 (mode="fts"),
// 미만이면 title/note/description LIKE 폴백 (mode="like", Rank=nil, created_at DESC).
// 공백 분리 토큰이 전부 3자 미만이라 trigram 색인을 탈 수 없는 경우도 LIKE로 폴백한다.
func (s *sqliteStore) Search(ctx context.Context, q, tag string, from, to *int64, cursor string, limit int) ([]SearchResult, string, SearchMode, error) {
	q = strings.TrimSpace(q)
	if utf8.RuneCountInString(q) >= 3 {
		if match := ftsMatchQuery(q); match != "" {
			items, next, err := s.searchFTS(ctx, match, tag, from, to, cursor, limit)
			if err != nil {
				return nil, "", SearchModeFTS, err
			}
			// **모든 낱말을 요구했다가 하나도 못 찾으면 일부라도 맞는 것을 보여준다.**
			//
			// FTS5에서 공백은 암묵적 AND라, 색인에 없는 낱말이 **하나만** 섞여도 결과가
			// 통째로 0이 된다. 실측(2026-07-27, `just eval-search` 25질의): 미발견 12건의
			// 대부분이 이것이었다 — `웹 취약점 top 10`은 `top`·`10`이 OWASP 제목에 있는데
			// `취약점`이 없어서 전멸했고, `판다스 10분 입문`은 제목이 `10 minutes to pandas`라
			// `판다스` 하나에 막혔다.
			//
			// **AND를 OR로 바꾸지 않고, AND가 빈손일 때만 OR로 다시 묻는다.** 전면 OR은
			// 지금 잘 찾는 질의의 순위까지 흔들지만, 이 방식은 결과가 0인 경우에만 개입하므로
			// **되던 것이 나빠질 수가 없다.** 0건이 1건 이상이 되는 방향으로만 움직인다.
			//
			// 재시도 판정은 질의만 보고 결정되므로 페이지를 넘어가도 같은 모드가 유지된다.
			if len(items) == 0 && cursor == "" {
				if orMatch := ftsMatchQueryAny(q); orMatch != match {
					items, next, err = s.searchFTS(ctx, orMatch, tag, from, to, cursor, limit)
					return items, next, SearchModeFTS, err
				}
			}
			return items, next, SearchModeFTS, err
		}
	}
	items, next, err := s.searchLike(ctx, q, tag, from, to, cursor, limit, false)
	if err != nil {
		return nil, "", SearchModeLike, err
	}
	// **FTS와 같은 규약**: 모든 낱말을 요구했다가 빈손이면 일부라도 맞는 것을 보여준다.
	//
	// 여기서도 같은 함정이 있었다. `직방 다방 차이`의 대상 제목은
	// `직방, 다방, 네이버 부동산, 집토스 뭐가 다를까?`라 **`차이`가 아예 없다** —
	// 사람은 뜻으로 기억하고 문서는 다른 낱말을 쓴다. 낱말 AND면 그 한 낱말이 전부를 죽인다.
	//
	// 개입은 결과 0인 경우로 한정되므로 되던 질의는 순위까지 그대로다.
	if len(items) == 0 && cursor == "" && len(strings.Fields(q)) > 1 {
		return s.searchLikeAnyRetry(ctx, q, tag, from, to, limit)
	}
	return items, next, SearchModeLike, err
}

// searchLikeAnyRetry는 낱말 OR로 다시 묻는다. 호출부를 짧게 유지하기 위한 얇은 껍데기다.
func (s *sqliteStore) searchLikeAnyRetry(ctx context.Context, q, tag string, from, to *int64, limit int) ([]SearchResult, string, SearchMode, error) {
	items, next, err := s.searchLike(ctx, q, tag, from, to, "", limit, true)
	return items, next, SearchModeLike, err
}

// ftsMatchQuery는 사용자 입력을 FTS5 MATCH 문자열로 만든다.
// 토큰별로 큰따옴표로 감싸 FTS 문법(AND/OR/NEAR/컬럼 필터 등) 주입을 차단하고,
// trigram 최소 길이(3자) 미만 토큰은 제외한다. 남는 토큰이 없으면 "" (LIKE 폴백 신호).
func ftsMatchQuery(q string) string {
	var quoted []string
	for _, tok := range strings.Fields(q) {
		if utf8.RuneCountInString(tok) < 3 {
			continue
		}
		quoted = append(quoted, `"`+strings.ReplaceAll(tok, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " ") // 공백 = 암묵적 AND
}

// ftsMatchQueryAny는 같은 토큰을 **OR**로 잇는다 — `ftsMatchQuery`가 빈손일 때의 재시도용.
//
// 토큰이 하나뿐이면 AND와 글자까지 같은 문자열이 나오고, 호출부는 그걸 보고 재시도를
// 건너뛴다(같은 질의를 두 번 돌릴 이유가 없다).
func ftsMatchQueryAny(q string) string {
	var quoted []string
	for _, tok := range strings.Fields(q) {
		if utf8.RuneCountInString(tok) < 3 {
			continue
		}
		quoted = append(quoted, `"`+strings.ReplaceAll(tok, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " OR ")
}

// searchFTS는 links_fts MATCH + bm25 랭킹 검색.
// 커서는 (bm25 rank의 float64 비트, id)를 EncodeFTSCursor에 실은 keyset —
// ORDER BY rank, id ASC이므로 WHERE (rank, id) > (?, ?)로 이어 읽는다.
// bm25는 코퍼스 전역 통계에 의존하므로 페이지 사이에 쓰기가 발생하면 페이지 경계가
// 이동할 수 있다 (단일 사용자 규모에서 허용 — 06 §6 참조).
func (s *sqliteStore) searchFTS(ctx context.Context, match, tag string, from, to *int64, cursor string, limit int) ([]SearchResult, string, error) {
	var (
		sb   strings.Builder
		args []any
	)
	// 바깥 프로젝션은 `*`다. 컬럼을 손으로 다시 적으면 linkCols에 컬럼이 추가돼도
	// 여기만 옛 목록으로 남는데, **두 사본의 개수가 서로 맞아떨어져서 Scan이 통과한다** —
	// 목록은 새 값을 내고 검색만 조용히 빈 값을 낸다. 실측으로 그 상태를 재현했고
	// 이 한 줄이면 같은 실수가 기존 테스트 4개에서 arity 불일치로 시끄럽게 터진다.
	sb.WriteString(`SELECT * FROM (
		SELECT ` + linkCols + `, bm25(links_fts) AS rank
		FROM links_fts JOIN links l ON l.id = links_fts.rowid`)
	if tag != "" {
		sb.WriteString(` JOIN link_tags lt ON lt.link_id = l.id JOIN tags t ON t.id = lt.tag_id`)
	}
	sb.WriteString(` WHERE links_fts MATCH ? AND l.deleted_at IS NULL`)
	args = append(args, match)
	if tag != "" {
		sb.WriteString(` AND t.name = ?`)
		args = append(args, tag)
	}
	if from != nil {
		sb.WriteString(` AND l.created_at >= ?`)
		args = append(args, *from)
	}
	if to != nil {
		sb.WriteString(` AND l.created_at <= ?`)
		args = append(args, *to)
	}
	sb.WriteString(`)`)
	if cursor != "" {
		rankBits, lastID, err := DecodeFTSCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		sb.WriteString(` WHERE (rank, id) > (?, ?)`)
		args = append(args, math.Float64frombits(uint64(rankBits)), lastID)
	}
	sb.WriteString(` ORDER BY rank, id LIMIT ?`) // bm25는 낮을수록 관련도 높음
	args = append(args, limit+1)

	rows, err := s.db.Reader.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, "", fmt.Errorf("store: FTS 검색 실패: %w", err)
	}
	defer rows.Close()
	var items []SearchResult
	for rows.Next() {
		var (
			r     SearchResult
			thumb *string
			rank  float64
		)
		if err := rows.Scan(&r.ID, &r.URL, &r.Domain, &r.Title, &r.Description, &r.ContentType,
			&thumb, &r.Status, &r.Note, &r.CreatedAt, &rank); err != nil {
			return nil, "", fmt.Errorf("store: FTS 결과 스캔 실패: %w", err)
		}
		r.ThumbPath = thumb
		r.Rank = &rank
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("store: FTS 결과 순회 실패: %w", err)
	}

	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = EncodeFTSCursor(int64(math.Float64bits(*last.Rank)), last.ID)
	}
	if err := s.attachSearchTags(ctx, items); err != nil {
		return nil, "", err
	}
	return items, next, nil
}

// escapeLike는 LIKE 패턴 메타문자 %·_와 이스케이프 문자 \를 \로 이스케이프한다.
// 쿼리에서 ESCAPE '\'와 함께 쓴다.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// searchLike는 links 테이블 LIKE 폴백 — 표준 (created_at, id) keyset.
func (s *sqliteStore) searchLike(ctx context.Context, q, tag string, from, to *int64, cursor string, limit int, anyWord bool) ([]SearchResult, string, error) {
	// **낱말별로 나눠 찾는다.** 예전에는 질의 전체가 한 덩어리 패턴이라
	// `%직방 다방 차이%`처럼 **그 문자열이 통째로** 들어 있어야 했다. 사람은 기억나는
	// 낱말을 순서대로 치지 문서의 어구를 그대로 치지 않으므로, 그 형태로는 거의 안 맞는다.
	//
	// 규칙: **낱말끼리는 AND, 낱말 하나는 세 필드 중 아무 데나.** 제목에 `직방`,
	// 설명에 `차이`처럼 흩어져 있어도 잡힌다. 낱말끼리 OR로 하면 흔한 한 낱말이
	// 전부를 끌어오므로 AND가 맞다 — FTS 쪽은 AND가 빈손일 때 OR로 재시도하지만,
	// 여기는 애초에 3자 미만이라 FTS를 못 탄 짧은 질의라 후보가 적고 정밀도가 더 중요하다.
	words := strings.Fields(q)
	if len(words) == 0 {
		words = []string{q}
	}
	var (
		sb   strings.Builder
		args []any
	)
	sb.WriteString(`SELECT ` + linkCols + ` FROM links l`)
	if tag != "" {
		sb.WriteString(` JOIN link_tags lt ON lt.link_id = l.id JOIN tags t ON t.id = lt.tag_id`)
	}
	sb.WriteString(` WHERE l.deleted_at IS NULL`)
	// anyWord=false면 낱말끼리 AND, true면 OR. 호출부가 AND로 먼저 묻고 빈손이면 OR로
	// 다시 묻는다 — FTS 쪽과 같은 규약이고 같은 이유다(searchLikeAnyRetry 주석).
	joiner := " AND "
	if anyWord {
		joiner = " OR "
	}
	sb.WriteString(` AND (`)
	for i, w := range words {
		if i > 0 {
			sb.WriteString(joiner)
		}
		pattern := "%" + escapeLike(w) + "%"
		sb.WriteString(`(l.title LIKE ? ESCAPE '\' OR l.note LIKE ? ESCAPE '\' OR l.description LIKE ? ESCAPE '\')`)
		args = append(args, pattern, pattern, pattern)
	}
	sb.WriteString(`)`)
	if tag != "" {
		sb.WriteString(` AND t.name = ?`)
		args = append(args, tag)
	}
	if from != nil {
		sb.WriteString(` AND l.created_at >= ?`)
		args = append(args, *from)
	}
	if to != nil {
		sb.WriteString(` AND l.created_at <= ?`)
		args = append(args, *to)
	}
	if cursor != "" {
		ca, cid, err := DecodeCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		sb.WriteString(` AND (l.created_at, l.id) < (?, ?)`)
		args = append(args, ca, cid)
	}
	sb.WriteString(` ORDER BY l.created_at DESC, l.id DESC LIMIT ?`)
	args = append(args, limit+1)

	rows, err := s.db.Reader.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, "", fmt.Errorf("store: LIKE 검색 실패: %w", err)
	}
	defer rows.Close()
	var items []SearchResult
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, "", err
		}
		items = append(items, SearchResult{Link: l}) // Rank는 nil
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("store: LIKE 결과 순회 실패: %w", err)
	}

	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = EncodeCursor(last.CreatedAt, last.ID)
	}
	if err := s.attachSearchTags(ctx, items); err != nil {
		return nil, "", err
	}
	return items, next, nil
}

// attachSearchTags는 검색 결과 페이지의 태그를 IN 쿼리 한 번으로 채운다.
func (s *sqliteStore) attachSearchTags(ctx context.Context, items []SearchResult) error {
	ptrs := make([]*Link, len(items))
	for i := range items {
		ptrs[i] = &items[i].Link
	}
	return attachTags(ctx, s.db.Reader, ptrs)
}

// Stats는 위젯용 통계 — 06 §7. this_week은 최근 7일, by_day는 최근 30일(localtime).
func (s *sqliteStore) Stats(ctx context.Context) (*Stats, error) {
	st := &Stats{ByTag: []TagCount{}, ByDay: []DayCount{}}
	if err := s.db.Reader.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM links WHERE deleted_at IS NULL`).Scan(&st.TotalLinks); err != nil {
		return nil, fmt.Errorf("store: total_links 조회 실패: %w", err)
	}
	if err := s.db.Reader.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM links WHERE deleted_at IS NULL AND created_at >= unixepoch() - 7*86400`,
	).Scan(&st.LinksThisWeek); err != nil {
		return nil, fmt.Errorf("store: links_this_week 조회 실패: %w", err)
	}

	rows, err := s.db.Reader.QueryContext(ctx, `
		SELECT t.name, COUNT(*) AS cnt
		FROM link_tags lt
		JOIN tags t  ON t.id = lt.tag_id
		JOIN links l ON l.id = lt.link_id
		WHERE l.deleted_at IS NULL
		GROUP BY t.id
		ORDER BY cnt DESC, t.name`)
	if err != nil {
		return nil, fmt.Errorf("store: by_tag 조회 실패: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var tc TagCount
		if err := rows.Scan(&tc.Name, &tc.Count); err != nil {
			return nil, fmt.Errorf("store: by_tag 스캔 실패: %w", err)
		}
		st.ByTag = append(st.ByTag, tc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: by_tag 순회 실패: %w", err)
	}

	drows, err := s.db.Reader.QueryContext(ctx, `
		SELECT date(created_at, 'unixepoch', 'localtime') AS d, COUNT(*)
		FROM links
		WHERE deleted_at IS NULL AND created_at >= unixepoch() - 30*86400
		GROUP BY d
		ORDER BY d`)
	if err != nil {
		return nil, fmt.Errorf("store: by_day 조회 실패: %w", err)
	}
	defer drows.Close()
	for drows.Next() {
		var dc DayCount
		if err := drows.Scan(&dc.Date, &dc.Count); err != nil {
			return nil, fmt.Errorf("store: by_day 스캔 실패: %w", err)
		}
		st.ByDay = append(st.ByDay, dc)
	}
	if err := drows.Err(); err != nil {
		return nil, fmt.Errorf("store: by_day 순회 실패: %w", err)
	}
	return st, nil
}
