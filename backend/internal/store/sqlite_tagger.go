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
		`SELECT domain, title, description, note, body_text, keywords FROM links WHERE id = ? AND deleted_at IS NULL`, linkID,
	).Scan(&c.Domain, &c.Title, &c.Description, &c.Note, &c.Body, &c.Keywords)
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
func (s *sqliteStore) ApplyTags(ctx context.Context, linkID int64, scored []ScoredTag, terms []string) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		// 삭제된 링크에는 쓰지 않는다.
		//
		// tagjob은 콘텐츠 조회·분류를 마친 **뒤에** writer를 잡는데, 그 사이에 삭제가
		// 커밋될 수 있다(DeleteLink는 running 잡을 일부러 남긴다). 그러면 방금 회수한
		// 기여가 곧바로 다시 쌓이고, 두 번째 DeleteLink는 ErrNotFound로 빠져 자가 치유
		// 경로가 없다 — 지운 주제가 계속 "흔한 낱말"로 남아 앞으로의 태깅을 끌어내린다.
		// 그건 DeleteLink가 기여를 회수하는 이유와 정확히 반대다.
		//
		// 가드가 헬퍼가 아니라 여기 있는 이유: 헬퍼에 두면 DeleteLink가 deleted_at을
		// 찍은 뒤 기여를 회수하는 **정상 경로**까지 막힌다.
		var alive int
		err := tx.QueryRowContext(ctx,
			`SELECT 1 FROM links WHERE id = ? AND deleted_at IS NULL`, linkID).Scan(&alive)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("store: 링크 생존 확인 실패: %w", err)
		}
		if err := applyCorpusTerms(ctx, tx, linkID, terms); err != nil {
			return err
		}
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

// SetSummary는 추출식 요약을 기록한다. 빈 문자열도 정상 값이라 그대로 저장한다 —
// 가드에 걸려 요약이 없다는 사실 자체가 상태이고, UI는 그때 섹션을 그리지 않는다.
//
// **같은 트랜잭션에서 FTS를 재색인한다.** 요약이 색인 대상이 된 뒤로(reindexFTS 주석 참고)
// 이걸 빼먹으면 요약이 붙어도 검색에는 영원히 안 잡힌다 — tag 잡이 ApplyTags로 재색인한
// **뒤에** SetSummary가 오기 때문이다. 링크/태그 쓰기와 같은 트랜잭션 규약이기도 하다.
func (s *sqliteStore) SetSummary(ctx context.Context, linkID int64, summary string) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE links SET summary = ?, updated_at = unixepoch()
			 WHERE id = ? AND deleted_at IS NULL`, summary, linkID)
		if err != nil {
			return fmt.Errorf("store: summary UPDATE 실패: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: RowsAffected 실패: %w", err)
		}
		if n == 0 {
			return ErrNotFound
		}
		return reindexFTS(ctx, tx, linkID)
	})
}

// applyCorpusTerms는 이 링크가 corpus_df에 기여한 몫을 새 terms로 교체한다.
//
// 되돌리기가 먼저인 이유: 태깅은 재시도·본문 보충·undelete로 여러 번 돈다. 올리기만 하면
// df가 "그 낱말을 가진 문서 수"가 아니라 "태깅이 돈 횟수"가 되어, 오래 쓸수록 통계가
// 실제와 멀어진다 — 그것도 **조용히**. link_terms에 지난번 기여를 적어 두는 이유가 이것이다.
func applyCorpusTerms(ctx context.Context, tx *sql.Tx, linkID int64, terms []string) error {
	// 지난 기여만큼 df를 되돌린다. link_terms가 원장이라 정확히 상쇄된다.
	if _, err := tx.ExecContext(ctx, `
		UPDATE corpus_df SET df = df - 1
		WHERE term IN (SELECT term FROM link_terms WHERE link_id = ?)`, linkID); err != nil {
		return fmt.Errorf("store: corpus_df 되돌리기 실패: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM link_terms WHERE link_id = ?`, linkID); err != nil {
		return fmt.Errorf("store: link_terms 정리 실패: %w", err)
	}
	for _, t := range terms {
		if t == "" {
			continue
		}
		// INSERT OR IGNORE — terms에 중복이 와도 한 문서는 df에 1만 기여해야 한다.
		res, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO link_terms (link_id, term) VALUES (?, ?)`, linkID, t)
		if err != nil {
			return fmt.Errorf("store: link_terms INSERT 실패 (%q): %w", t, err)
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			continue // 이미 이 문서에 기록된 표면 — df를 두 번 올리지 않는다
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO corpus_df (term, df) VALUES (?, 1)
			ON CONFLICT (term) DO UPDATE SET df = df + 1`, t); err != nil {
			return fmt.Errorf("store: corpus_df 갱신 실패 (%q): %w", t, err)
		}
	}
	// df가 0이 된 낱말은 지운다 — 안 지우면 코퍼스에서 사라진 낱말이 테이블에 영원히 쌓인다.
	if _, err := tx.ExecContext(ctx, `DELETE FROM corpus_df WHERE df <= 0`); err != nil {
		return fmt.Errorf("store: corpus_df 정리 실패: %w", err)
	}
	return nil
}

// CorpusDF는 문서 빈도 스냅샷을 돌려준다.
//
// 문서 수는 link_terms에 기여한 **서로 다른 링크 수**다 — links 전체가 아니다. 아직 태깅이
// 돌지 않은 링크는 df에 아무것도 보태지 않았으므로 분모에 넣으면 모든 낱말이 실제보다
// 희귀해 보인다.
//
// 같은 이유로 **태깅은 돌았지만 사전 표면이 하나도 안 나온 링크**(외국어·사전 밖 주제·
// 본문 추출 실패)도 빠진다. 그건 의도된 것이고, `just eval`의 goldenCorpus도 같은 규칙을
// 쓴다 — 둘이 다른 분모를 쓰면 IDF를 켤 때 판정 도구와 실제 동작이 어긋난다.
func (s *sqliteStore) CorpusDF(ctx context.Context) (int64, map[string]int64, error) {
	var docs int64
	if err := s.db.Reader.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM (SELECT 1 FROM link_terms GROUP BY link_id)`).Scan(&docs); err != nil {
		return 0, nil, fmt.Errorf("store: 코퍼스 문서 수 조회 실패: %w", err)
	}
	rows, err := s.db.Reader.QueryContext(ctx, `SELECT term, df FROM corpus_df WHERE df > 0`)
	if err != nil {
		return 0, nil, fmt.Errorf("store: corpus_df 조회 실패: %w", err)
	}
	defer rows.Close()
	df := map[string]int64{}
	for rows.Next() {
		var term string
		var n int64
		if err := rows.Scan(&term, &n); err != nil {
			return 0, nil, fmt.Errorf("store: corpus_df 스캔 실패: %w", err)
		}
		df[term] = n
	}
	if err := rows.Err(); err != nil {
		return 0, nil, fmt.Errorf("store: corpus_df 순회 실패: %w", err)
	}
	return docs, df, nil
}
