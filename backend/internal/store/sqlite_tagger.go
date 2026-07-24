// tag 잡 핸들러가 호출하는 Store 메서드의 SQLite 구현 — 태거 입력 조회·사전 로드·결과 반영.
// 런타임 사전은 DB tags 테이블(마이그레이션 시드 + CRUD)이고, 태거 결과는 source='rules'로 기록한다.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GetLinkContent는 태거 입력(도메인·제목·설명·메모)을 읽는다. 삭제/부재면 ErrNotFound.
func (s *sqliteStore) GetLinkContent(ctx context.Context, linkID int64) (LinkContent, error) {
	var c LinkContent
	err := s.db.Reader.QueryRowContext(ctx,
		`SELECT domain, title, description, note FROM links WHERE id = ? AND deleted_at IS NULL`, linkID,
	).Scan(&c.Domain, &c.Title, &c.Description, &c.Note)
	if errors.Is(err, sql.ErrNoRows) {
		return LinkContent{}, ErrNotFound
	}
	if err != nil {
		return LinkContent{}, fmt.Errorf("store: 링크 콘텐츠 조회 실패: %w", err)
	}
	return c, nil
}

// LoadTagDict는 태그 사전 전체를 읽는다(id/name/aliases/facet). aliases는 decodeAliases 재사용.
func (s *sqliteStore) LoadTagDict(ctx context.Context) ([]TagDictEntry, error) {
	rows, err := s.db.Reader.QueryContext(ctx, `SELECT id, name, aliases, facet FROM tags`)
	if err != nil {
		return nil, fmt.Errorf("store: 태그 사전 조회 실패: %w", err)
	}
	defer rows.Close()

	var out []TagDictEntry
	for rows.Next() {
		var e TagDictEntry
		var aliasesRaw string
		if err := rows.Scan(&e.ID, &e.Name, &aliasesRaw, &e.Facet); err != nil {
			return nil, fmt.Errorf("store: 태그 사전 스캔 실패: %w", err)
		}
		if e.Aliases, err = decodeAliases(aliasesRaw); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: 태그 사전 순회 실패: %w", err)
	}
	return out, nil
}

// ApplyTags는 규칙 태거 결과를 한 writer 트랜잭션으로 반영한다. 재태깅 멱등을 위해 이 링크의
// source='rules' 행을 먼저 지우고(manual/embed는 보존), scored 태그를 INSERT한다 — 같은 태그의
// 사용자(manual) 행이 이미 있으면 ON CONFLICT DO NOTHING으로 사용자 선택을 우선 보존한다.
// link_tags가 바뀌었으므로 같은 트랜잭션에서 FTS 'tags' 컬럼을 재색인한다.
func (s *sqliteStore) ApplyTags(ctx context.Context, linkID int64, scored []ScoredTag) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM link_tags WHERE link_id = ? AND source = 'rules'`, linkID); err != nil {
			return fmt.Errorf("store: 기존 rules 태그 삭제 실패: %w", err)
		}
		for _, t := range scored {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO link_tags (link_id, tag_id, source, confidence) VALUES (?, ?, 'rules', ?)
				 ON CONFLICT (link_id, tag_id) DO NOTHING`,
				linkID, t.TagID, t.Confidence); err != nil {
				return fmt.Errorf("store: rules 태그 INSERT 실패 (tag=%d): %w", t.TagID, err)
			}
		}
		// link_tags 변경 → FTS의 tags 컬럼 재색인(링크/태그 쓰기와 같은 트랜잭션 규약).
		return reindexFTS(ctx, tx, linkID)
	})
}
