// Store 인터페이스의 SQLite 구현 — 링크 CRUD.
// 모든 쓰기는 db.Writer 트랜잭션, FTS5 동기화는 같은 트랜잭션에서 DELETE 후 INSERT.
// 태그/검색/통계 구현은 sqlite_tags.go / sqlite_search.go 참조.
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	neturl "net/url"
	"strings"

	"github.com/coby/push-point/backend/internal/queue"
)

// linkCols는 목록/검색이 공유하는 links 컬럼 목록 (scanLink와 순서 일치).
const linkCols = `l.id, l.url, l.domain, l.title, l.description, l.content_type, l.thumb_path, l.status, l.note, l.created_at`

type sqliteStore struct {
	db *DB
	q  queue.Queue
}

// 컴파일 타임 인터페이스 검증 (.claude/rules/backend.md 인터페이스 계약).
var _ Store = (*sqliteStore)(nil)

// New는 sqlite Store 구현체를 만든다. 쓰기는 db.Writer, 읽기는 db.Reader를 쓴다.
func New(db *DB, q queue.Queue) Store {
	return &sqliteStore{db: db, q: q}
}

func (s *sqliteStore) Close() error { return s.db.Close() }

// withWriteTx는 writer 커넥션에서 트랜잭션을 열고 fn 성공 시 커밋한다.
func (s *sqliteStore) withWriteTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.Writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: 트랜잭션 시작 실패: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 커밋 성공 후에는 no-op
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: 커밋 실패: %w", err)
	}
	return nil
}

// urlHash는 SHA-256(url) hex — links.url_hash 규약.
func urlHash(rawURL string) string {
	sum := sha256.Sum256([]byte(rawURL))
	return hex.EncodeToString(sum[:])
}

// hostOf는 URL의 호스트(포트 제외, 소문자)를 뽑는다. 파싱 실패면 빈 문자열.
func hostOf(rawURL string) string {
	u, err := neturl.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// SaveLink는 INSERT links + scrape 잡 enqueue + FTS 색인을 한 트랜잭션으로 커밋한다.
// url_hash 중복이면 기존 id와 duplicate=true 반환. 이때 저장된 본문이 서버 출처인데 요청이
// 클라이언트 본문을 실어 오면 제목·설명·본문을 1회 보충하고 tag 잡을 재-enqueue한다
// (이미 클라이언트 본문이면 무동작 — 반복 호출은 같은 상태로 수렴한다).
// 소프트 삭제된 행과 중복이면 같은 트랜잭션에서 undelete(pending 복귀, note 교체,
// 부착 태그 전부 제거, scrape 재-enqueue, FTS 재색인)하고 신규 저장처럼 duplicate=false로
// 반환한다 — url_hash UNIQUE 때문에 재-INSERT가 불가능하므로 이 경로가 "삭제한 URL 재저장"이다.
func (s *sqliteStore) SaveLink(ctx context.Context, in SaveInput) (int64, int64, bool, error) {
	// 진입점(HTTP 핸들러 / 임베드 모드의 로컬 큐 드레인)에 무관하게 같은 검증·정제를 받는다.
	in, err0 := in.Normalize()
	if err0 != nil {
		return 0, 0, false, err0
	}
	url, note := in.URL, in.Note
	hasClientBody := in.BodyText != ""
	// 클라이언트 본문이 있으면 body_source='client'로 굳힌다 — 이후 스크랩이 3필드를 덮지 않는다.
	bodySource := ""
	if hasClientBody {
		bodySource = "client"
	}
	hash := urlHash(url)
	var (
		id        int64
		createdAt int64
		duplicate bool
		// backfilled: 중복 링크에 클라이언트 본문을 보충했는가 (커밋 후 Wake 여부를 정한다)
		backfilled bool
	)
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		// 중복 확인 — writer 단일 커넥션이라 check-then-insert가 경합 없이 안전
		var deletedAt sql.NullInt64
		var curBodySource string
		err := tx.QueryRowContext(ctx,
			`SELECT id, created_at, deleted_at, body_source FROM links WHERE url_hash = ?`, hash,
		).Scan(&id, &createdAt, &deletedAt, &curBodySource)
		switch {
		case err == nil && !deletedAt.Valid:
			duplicate = true
			// 단방향 보충: 저장된 본문이 서버 출처인데 이번엔 클라이언트가 본문을 줬다.
			// 이 경로가 없으면 **이미 실패해 저장돼 있는 링크**(이 기능의 실제 동기)에
			// 본문을 넣을 방법이 없다. 반대 방향(클라이언트 → 서버 덮어쓰기)은 하지 않는다.
			if !hasClientBody || curBodySource == "client" {
				return nil
			}
			// 제목·설명은 요청이 준 경우에만 덮는다 — 셋은 각각 optional이라 본문만 실어
			// 오는 요청(Share Extension의 일반형)이 서버가 얻어둔 제목을 지우면 안 된다.
			// status: 스크랩이 실패해 'failed'인 링크가 이 기능의 실제 동기다. 제목·본문·태그가
			// 다 생겼는데 UI에 "수집 실패"로 남으면 거짓이므로 done으로 올린다(queue.Fail의
			// 같은 규약). error는 그대로 둬 진단 정보를 남긴다.
			if _, err := tx.ExecContext(ctx, `
				UPDATE links SET
				       title       = CASE WHEN ? <> '' THEN ? ELSE title END,
				       description = CASE WHEN ? <> '' THEN ? ELSE description END,
				       body_text   = ?,
				       body_source = 'client',
				       status      = CASE WHEN status = 'failed' THEN 'done' ELSE status END,
				       updated_at  = unixepoch()
				WHERE id = ?`,
				in.Title, in.Title,
				in.Description, in.Description,
				in.BodyText, id); err != nil {
				return fmt.Errorf("store: 클라이언트 본문 보충 실패: %w", err)
			}
			// 본문이 새로 생겼으니 태깅·요약을 다시 돌린다(tag 핸들러가 둘 다 한다).
			if err := s.q.EnqueueTx(tx, queue.KindTag, id); err != nil {
				return fmt.Errorf("store: 보충 후 tag 잡 enqueue 실패: %w", err)
			}
			backfilled = true
			return reindexFTS(ctx, tx, id)
		case err == nil: // 소프트 삭제된 행 — undelete 후 신규 저장과 동일하게 처리
			// 부착 태그 전부 제거 — 신규 저장은 태그 0으로 시작한다. 시스템 동작이므로
			// tag_feedback(사용자 수정 이력)은 남기지 않는다. 이어지는 reindexFTS가
			// 태그 빈 상태로 재색인해 옛 태그 텍스트가 검색에서 사라진다.
			if _, err := tx.ExecContext(ctx, `DELETE FROM link_tags WHERE link_id = ?`, id); err != nil {
				return fmt.Errorf("store: undelete 태그 정리 실패: %w", err)
			}
			// body_source는 **무조건** 새 값으로 리셋한다 — 안 하면 옛 'client' 플래그가
			// 살아남아 새 스크랩이 영원히 3필드를 못 쓴다. 3필드는 이번 요청이 준 경우에만
			// 덮어쓴다(빈 요청이 멀쩡한 제목을 지우지 않게).
			if _, err := tx.ExecContext(ctx, `
				UPDATE links SET deleted_at = NULL, status = 'pending', error = '',
				       note = ?, body_source = ?,
				       title       = CASE WHEN ? <> '' THEN ? ELSE title END,
				       description = CASE WHEN ? <> '' THEN ? ELSE description END,
				       body_text   = CASE WHEN ? <> '' THEN ? ELSE body_text END,
				       updated_at = unixepoch()
				WHERE id = ?`,
				note, bodySource,
				in.Title, in.Title,
				in.Description, in.Description,
				in.BodyText, in.BodyText,
				id); err != nil {
				return fmt.Errorf("store: 링크 undelete 실패: %w", err)
			}
			if err := s.q.EnqueueTx(tx, queue.KindScrape, id); err != nil {
				return fmt.Errorf("store: scrape 잡 enqueue 실패: %w", err)
			}
			if hasClientBody {
				if err := s.q.EnqueueTx(tx, queue.KindTag, id); err != nil {
					return fmt.Errorf("store: tag 잡 enqueue 실패: %w", err)
				}
			}
			return reindexFTS(ctx, tx, id)
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("store: url_hash 중복 확인 실패: %w", err)
		}
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO links (url, url_hash, domain, note, title, description, body_text, body_source)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id, created_at`,
			url, hash, hostOf(url), note, in.Title, in.Description, in.BodyText, bodySource,
		).Scan(&id, &createdAt); err != nil {
			return fmt.Errorf("store: links INSERT 실패: %w", err)
		}
		if err := s.q.EnqueueTx(tx, queue.KindScrape, id); err != nil {
			return fmt.Errorf("store: scrape 잡 enqueue 실패: %w", err)
		}
		// 클라이언트 본문이 이미 있으면 스크랩을 기다리지 않고 태깅·요약을 시작한다 —
		// 스크랩이 실패할 페이지라서 클라이언트가 준 것이므로 여기 의존하면 안 된다.
		if hasClientBody {
			if err := s.q.EnqueueTx(tx, queue.KindTag, id); err != nil {
				return fmt.Errorf("store: tag 잡 enqueue 실패: %w", err)
			}
		}
		return reindexFTS(ctx, tx, id)
	})
	if err != nil {
		return 0, 0, false, err
	}
	if !duplicate || backfilled {
		s.q.Wake() // 커밋 성공 후에만 dispatcher를 깨운다 (보충한 경우도 깨워야 태깅이 돈다)
	}
	return id, createdAt, duplicate, nil
}

// reindexFTS는 링크의 FTS 색인을 같은 트랜잭션에서 재작성한다 (DELETE 후 INSERT).
// tags 컬럼은 태그 이름을 공백으로 연결. 소프트 삭제된 링크면 색인 제거만 한다.
func reindexFTS(ctx context.Context, tx *sql.Tx, linkID int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM links_fts WHERE rowid = ?`, linkID); err != nil {
		return fmt.Errorf("store: links_fts DELETE 실패: %w", err)
	}
	var title, desc, note string
	err := tx.QueryRowContext(ctx,
		`SELECT title, description, note FROM links WHERE id = ? AND deleted_at IS NULL`, linkID,
	).Scan(&title, &desc, &note)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // 삭제된 링크 — 색인에서 빠진 상태 유지
	}
	if err != nil {
		return fmt.Errorf("store: FTS 재색인용 링크 조회 실패: %w", err)
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT t.name FROM link_tags lt JOIN tags t ON t.id = lt.tag_id WHERE lt.link_id = ? ORDER BY t.name`, linkID)
	if err != nil {
		return fmt.Errorf("store: FTS 재색인용 태그 조회 실패: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return fmt.Errorf("store: 태그 이름 스캔 실패: %w", err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: 태그 이름 순회 실패: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO links_fts (rowid, title, description, note, tags) VALUES (?, ?, ?, ?, ?)`,
		linkID, title, desc, note, strings.Join(names, " ")); err != nil {
		return fmt.Errorf("store: links_fts INSERT 실패: %w", err)
	}
	return nil
}

// GetLink는 상세 조회 — 소프트 삭제됐거나 없으면 ErrNotFound.
func (s *sqliteStore) GetLink(ctx context.Context, id int64) (*LinkDetail, error) {
	var (
		d           LinkDetail
		thumb       sql.NullString
		publishedAt sql.NullInt64
		durationSec sql.NullInt64
		wordCount   sql.NullInt64
	)
	err := s.db.Reader.QueryRowContext(ctx, `
		SELECT id, url, domain, title, description, author, content_type, lang,
		       published_at, duration_sec, word_count, thumb_path, note, status, error,
		       created_at, updated_at, summary
		FROM links WHERE id = ? AND deleted_at IS NULL`, id,
	).Scan(&d.ID, &d.URL, &d.Domain, &d.Title, &d.Description, &d.Author, &d.ContentType, &d.Lang,
		&publishedAt, &durationSec, &wordCount, &thumb, &d.Note, &d.Status, &d.Error,
		&d.CreatedAt, &d.UpdatedAt, &d.Summary)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: 링크 상세 조회 실패: %w", err)
	}
	if thumb.Valid {
		d.ThumbPath = &thumb.String
	}
	if publishedAt.Valid {
		d.PublishedAt = &publishedAt.Int64
	}
	if durationSec.Valid {
		d.DurationSec = &durationSec.Int64
	}
	if wordCount.Valid {
		d.WordCount = &wordCount.Int64
	}

	// 부착 태그
	rows, err := s.db.Reader.QueryContext(ctx, `
		SELECT t.id, t.name, lt.source, lt.confidence
		FROM link_tags lt JOIN tags t ON t.id = lt.tag_id
		WHERE lt.link_id = ? ORDER BY t.name`, id)
	if err != nil {
		return nil, fmt.Errorf("store: 링크 태그 조회 실패: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			lt   LinkTag
			conf sql.NullFloat64
		)
		if err := rows.Scan(&lt.ID, &lt.Name, &lt.Source, &conf); err != nil {
			return nil, fmt.Errorf("store: 링크 태그 스캔 실패: %w", err)
		}
		if conf.Valid {
			lt.Confidence = &conf.Float64
		}
		d.Tags = append(d.Tags, lt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: 링크 태그 순회 실패: %w", err)
	}

	// 잡 요약 — kind별 최신(id 최대) 잡의 상태. id 순 순회로 마지막 값이 남는다.
	jrows, err := s.db.Reader.QueryContext(ctx,
		`SELECT kind, status FROM jobs WHERE link_id = ? ORDER BY id`, id)
	if err != nil {
		return nil, fmt.Errorf("store: 잡 요약 조회 실패: %w", err)
	}
	defer jrows.Close()
	for jrows.Next() {
		var kind, status string
		if err := jrows.Scan(&kind, &status); err != nil {
			return nil, fmt.Errorf("store: 잡 요약 스캔 실패: %w", err)
		}
		switch kind {
		case string(queue.KindScrape):
			d.Jobs.Scrape = status
		case string(queue.KindTag):
			d.Jobs.Tag = status
		case string(queue.KindThumb):
			d.Jobs.Thumb = status
		}
	}
	if err := jrows.Err(); err != nil {
		return nil, fmt.Errorf("store: 잡 요약 순회 실패: %w", err)
	}
	return &d, nil
}

// ListLinks는 keyset 커서 목록 — (created_at, id) < (?, ?), OFFSET 금지.
func (s *sqliteStore) ListLinks(ctx context.Context, cursor string, limit int, tag, status string) ([]Link, string, error) {
	var (
		sb   strings.Builder
		args []any
	)
	sb.WriteString(`SELECT ` + linkCols + ` FROM links l`)
	if tag != "" {
		// link_tags PK(link_id, tag_id) + name 단일 매칭이라 중복 행이 생기지 않는다
		sb.WriteString(` JOIN link_tags lt ON lt.link_id = l.id JOIN tags t ON t.id = lt.tag_id`)
	}
	sb.WriteString(` WHERE l.deleted_at IS NULL`)
	if tag != "" {
		sb.WriteString(` AND t.name = ?`) // 컬럼이 COLLATE NOCASE
		args = append(args, tag)
	}
	if status != "" {
		sb.WriteString(` AND l.status = ?`)
		args = append(args, status)
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
	args = append(args, limit+1) // 한 건 더 읽어 next_cursor 유무 판단

	rows, err := s.db.Reader.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, "", fmt.Errorf("store: 목록 조회 실패: %w", err)
	}
	defer rows.Close()
	var items []Link
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, "", err
		}
		items = append(items, l)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("store: 목록 순회 실패: %w", err)
	}

	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = EncodeCursor(last.CreatedAt, last.ID)
	}
	ptrs := make([]*Link, len(items))
	for i := range items {
		ptrs[i] = &items[i]
	}
	if err := attachTags(ctx, s.db.Reader, ptrs); err != nil {
		return nil, "", err
	}
	return items, next, nil
}

// scanLink는 linkCols 순서의 행을 Link로 스캔한다.
func scanLink(rows *sql.Rows) (Link, error) {
	var (
		l     Link
		thumb sql.NullString
	)
	if err := rows.Scan(&l.ID, &l.URL, &l.Domain, &l.Title, &l.Description, &l.ContentType,
		&thumb, &l.Status, &l.Note, &l.CreatedAt); err != nil {
		return Link{}, fmt.Errorf("store: 링크 행 스캔 실패: %w", err)
	}
	if thumb.Valid {
		l.ThumbPath = &thumb.String
	}
	return l, nil
}

// attachTags는 페이지 항목들의 태그를 IN 쿼리 한 번으로 채운다 (N+1 금지).
func attachTags(ctx context.Context, db *sql.DB, links []*Link) error {
	if len(links) == 0 {
		return nil
	}
	ph := make([]string, len(links))
	args := make([]any, len(links))
	byID := make(map[int64]*Link, len(links))
	for i, l := range links {
		ph[i] = "?"
		args[i] = l.ID
		byID[l.ID] = l
	}
	rows, err := db.QueryContext(ctx, `
		SELECT lt.link_id, t.id, t.name, lt.source, lt.confidence
		FROM link_tags lt JOIN tags t ON t.id = lt.tag_id
		WHERE lt.link_id IN (`+strings.Join(ph, ",")+`)
		ORDER BY lt.link_id, t.name`, args...)
	if err != nil {
		return fmt.Errorf("store: 태그 일괄 조회 실패: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			linkID int64
			lt     LinkTag
			conf   sql.NullFloat64
		)
		if err := rows.Scan(&linkID, &lt.ID, &lt.Name, &lt.Source, &conf); err != nil {
			return fmt.Errorf("store: 태그 일괄 스캔 실패: %w", err)
		}
		if conf.Valid {
			lt.Confidence = &conf.Float64
		}
		if l, ok := byID[linkID]; ok {
			l.Tags = append(l.Tags, lt)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: 태그 일괄 순회 실패: %w", err)
	}
	return nil
}

// UpdateLink는 메모/태그를 수정한다 (06 §4.4). tags는 전체 교체 —
// 추가분 link_tags(manual)+tag_feedback(added), 제거분 tag_feedback(removed).
func (s *sqliteStore) UpdateLink(ctx context.Context, id int64, note *string, tags []string) (*LinkDetail, error) {
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		var exists int64
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM links WHERE id = ? AND deleted_at IS NULL`, id).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("store: 링크 존재 확인 실패: %w", err)
		}
		if note != nil {
			if _, err := tx.ExecContext(ctx,
				`UPDATE links SET note = ?, updated_at = unixepoch() WHERE id = ?`, *note, id); err != nil {
				return fmt.Errorf("store: note 갱신 실패: %w", err)
			}
		}
		if tags != nil {
			if err := replaceTags(ctx, tx, id, tags); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE links SET updated_at = unixepoch() WHERE id = ?`, id); err != nil {
				return fmt.Errorf("store: updated_at 갱신 실패: %w", err)
			}
		}
		return reindexFTS(ctx, tx, id)
	})
	if err != nil {
		return nil, err
	}
	return s.GetLink(ctx, id)
}

// replaceTags는 링크의 태그를 이름 배열로 전체 교체한다.
// 사전에 없는 이름이면 ErrUnknownTag (invalid_input).
func replaceTags(ctx context.Context, tx *sql.Tx, linkID int64, names []string) error {
	// 이름 → id 해석 (tags.name이 COLLATE NOCASE라 대소문자 무시 매칭)
	want := make(map[int64]bool, len(names))
	for _, name := range names {
		var tagID int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM tags WHERE name = ?`, name).Scan(&tagID)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %q", ErrUnknownTag, name)
		}
		if err != nil {
			return fmt.Errorf("store: 태그 이름 해석 실패: %w", err)
		}
		want[tagID] = true
	}

	// 현재 부착 집합
	cur := make(map[int64]bool)
	rows, err := tx.QueryContext(ctx, `SELECT tag_id FROM link_tags WHERE link_id = ?`, linkID)
	if err != nil {
		return fmt.Errorf("store: 현재 태그 조회 실패: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var tagID int64
		if err := rows.Scan(&tagID); err != nil {
			return fmt.Errorf("store: 현재 태그 스캔 실패: %w", err)
		}
		cur[tagID] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: 현재 태그 순회 실패: %w", err)
	}

	// 추가분: link_tags(source='manual', confidence=NULL) + tag_feedback('added')
	for tagID := range want {
		if cur[tagID] {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO link_tags (link_id, tag_id, source, confidence) VALUES (?, ?, 'manual', NULL)`,
			linkID, tagID); err != nil {
			return fmt.Errorf("store: link_tags INSERT 실패: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tag_feedback (link_id, tag_id, action) VALUES (?, ?, 'added')`,
			linkID, tagID); err != nil {
			return fmt.Errorf("store: tag_feedback(added) 실패: %w", err)
		}
	}
	// 제거분: link_tags 삭제 + tag_feedback('removed')
	for tagID := range cur {
		if want[tagID] {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM link_tags WHERE link_id = ? AND tag_id = ?`, linkID, tagID); err != nil {
			return fmt.Errorf("store: link_tags DELETE 실패: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tag_feedback (link_id, tag_id, action) VALUES (?, ?, 'removed')`,
			linkID, tagID); err != nil {
			return fmt.Errorf("store: tag_feedback(removed) 실패: %w", err)
		}
	}
	return nil
}

// DeleteLink는 소프트 삭제 — deleted_at 기록 + FTS 색인 제거 + 대기·실패 잡 정리 (한 트랜잭션).
// 소프트 삭제라 links 행이 남으므로 jobs FK CASCADE가 발동하지 않는다 — 처리 예정이 없어진
// pending/failed 잡을 명시적으로 지워 큐·잡 요약에 유령 잡이 남지 않게 한다. running 잡은
// dispatcher가 처리 중이므로 유지한다 (완료 시 Complete/Fail이 링크 부재를 감지).
func (s *sqliteStore) DeleteLink(ctx context.Context, id int64) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE links SET deleted_at = unixepoch(), updated_at = unixepoch()
			 WHERE id = ? AND deleted_at IS NULL`, id)
		if err != nil {
			return fmt.Errorf("store: 소프트 삭제 실패: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: RowsAffected 실패: %w", err)
		}
		if n == 0 {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM links_fts WHERE rowid = ?`, id); err != nil {
			return fmt.Errorf("store: links_fts 제거 실패: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM jobs WHERE link_id = ? AND status IN ('pending','failed')`, id); err != nil {
			return fmt.Errorf("store: 삭제 링크 잡 정리 실패: %w", err)
		}
		return nil
	})
}

// RetryLink는 failed 링크를 pending으로 되돌리고 scrape 잡을 재-enqueue한다.
// 같은 트랜잭션에서 해당 링크의 failed 잡을 삭제한다 — 잡 요약(GetLink)이
// 옛 failed 잡을 kind별 최신으로 계속 보여주는 것을 막는다.
func (s *sqliteStore) RetryLink(ctx context.Context, id int64) error {
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		var status string
		err := tx.QueryRowContext(ctx,
			`SELECT status FROM links WHERE id = ? AND deleted_at IS NULL`, id).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("store: 링크 상태 조회 실패: %w", err)
		}
		if status != "failed" {
			return ErrNotFailed
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE links SET status = 'pending', error = '', updated_at = unixepoch() WHERE id = ?`, id); err != nil {
			return fmt.Errorf("store: 링크 상태 되돌리기 실패: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM jobs WHERE link_id = ? AND status = 'failed'`, id); err != nil {
			return fmt.Errorf("store: failed 잡 정리 실패: %w", err)
		}
		if err := s.q.EnqueueTx(tx, queue.KindScrape, id); err != nil {
			return fmt.Errorf("store: 재시도 잡 enqueue 실패: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.q.Wake()
	return nil
}
