// scrape/thumb 잡 핸들러가 호출하는 Store 메서드의 SQLite 구현.
// 모든 쓰기는 db.Writer 트랜잭션, FTS5 동기화는 같은 트랜잭션에서 재색인.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/coby/push-point/backend/internal/queue"
)

// GetLinkURL은 link_id로부터 원본 URL과 url_hash를 읽는다. 삭제/부재면 ErrNotFound.
func (s *sqliteStore) GetLinkURL(ctx context.Context, linkID int64) (string, string, error) {
	var url, hash string
	err := s.db.Reader.QueryRowContext(ctx,
		`SELECT url, url_hash FROM links WHERE id = ? AND deleted_at IS NULL`, linkID,
	).Scan(&url, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("store: 링크 URL 조회 실패: %w", err)
	}
	return url, hash, nil
}

// ApplyScrape는 스크랩 결과를 반영한다: 메타데이터 UPDATE + status='done' + FTS 재색인
// + (m.HasImage면) thumb 잡 enqueue. 전부 한 트랜잭션, 커밋 후 필요 시 Wake.
// content_type은 CHECK 제약이 있으므로 호출자가 유효 값('video'|'article'|'post'|'other')을
// 넘겨야 한다 — 빈 값이면 UPDATE가 CHECK 위반으로 실패하며, 이는 숨기지 않고 그대로 반환한다.
func (s *sqliteStore) ApplyScrape(ctx context.Context, linkID int64, m ScrapeResult) error {
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		// 클라이언트 캡처 값은 스크랩이 덮지 않는다(body_source='client'): 서버가 못 가져오는
		// 페이지라서 클라이언트가 준 것이므로 서버 재시도 결과는 항상 더 나쁘다.
		// 그리고 body_source와 **무관하게** 빈 추출 결과로 기존 본문을 지우지 않는다 —
		// scrapeResult가 추출 실패를 ""로 넘기는데, 그대로 덮으면 멀쩡한 본문이 사라진다
		// (클라이언트 캡처와 무관한 순수 서버 경로의 버그였다).
		res, err := tx.ExecContext(ctx, `
			UPDATE links SET
				title       = CASE WHEN body_source = 'client' THEN title       ELSE ? END,
				description = CASE WHEN body_source = 'client' THEN description ELSE ? END,
				body_text   = CASE WHEN body_source = 'client' OR ? = '' THEN body_text ELSE ? END,
				body_source = CASE WHEN body_source = 'client' THEN body_source
				                   WHEN ? <> '' THEN 'server' ELSE body_source END,
				author = ?, content_type = ?, lang = ?,
				published_at = ?, duration_sec = ?, word_count = ?,
				status = 'done', error = '', updated_at = unixepoch()
			WHERE id = ? AND deleted_at IS NULL`,
			m.Title, m.Description,
			m.BodyText, m.BodyText,
			m.BodyText,
			m.Author, m.ContentType, m.Lang,
			m.PublishedAt, m.DurationSec, m.WordCount, linkID)
		if err != nil {
			return fmt.Errorf("store: 스크랩 결과 UPDATE 실패: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: RowsAffected 실패: %w", err)
		}
		if n == 0 {
			return ErrNotFound
		}
		// FTS 재색인 — title/description이 바뀌었으므로 같은 트랜잭션에서 동기화.
		if err := reindexFTS(ctx, tx, linkID); err != nil {
			return err
		}
		// tag 잡 enqueue — 콘텐츠가 준비됐으므로 무조건 (best-effort: 실패해도 링크 status 불변).
		if err := s.q.EnqueueTx(tx, queue.KindTag, linkID); err != nil {
			return fmt.Errorf("store: tag 잡 enqueue 실패: %w", err)
		}
		// og:image 있으면 thumb 잡도 enqueue (best-effort).
		if m.HasImage {
			if err := s.q.EnqueueTx(tx, queue.KindThumb, linkID); err != nil {
				return fmt.Errorf("store: thumb 잡 enqueue 실패: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.q.Wake() // 커밋 성공 후에만 dispatcher를 깨운다 (tag 잡이 항상 있으므로 무조건)
	return nil
}

// SetThumbPath는 thumb 잡 성공 시 links.thumb_path를 기록한다 (FTS 무관 — 재색인 없음).
func (s *sqliteStore) SetThumbPath(ctx context.Context, linkID int64, relPath string) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE links SET thumb_path = ?, updated_at = unixepoch()
			 WHERE id = ? AND deleted_at IS NULL`, relPath, linkID)
		if err != nil {
			return fmt.Errorf("store: thumb_path UPDATE 실패: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: RowsAffected 실패: %w", err)
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}
