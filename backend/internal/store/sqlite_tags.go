// Store 인터페이스의 SQLite 구현 — 태그 사전 CRUD.
// 태그 이름 변경/삭제는 부착 링크들의 FTS tags 텍스트도 같은 트랜잭션에서 재색인한다.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// encodeAliases는 aliases를 JSON 배열 문자열로 만든다. nil/빈 배열은 "[]".
func encodeAliases(aliases []string) (string, error) {
	if aliases == nil {
		aliases = []string{}
	}
	b, err := json.Marshal(aliases)
	if err != nil {
		return "", fmt.Errorf("store: aliases 인코딩 실패: %w", err)
	}
	return string(b), nil
}

// decodeAliases는 aliases JSON 컬럼을 디코드한다. 비정상 값이면 에러.
func decodeAliases(raw string) ([]string, error) {
	if raw == "" {
		return []string{}, nil
	}
	var a []string
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		return nil, fmt.Errorf("store: aliases 디코드 실패 (%q): %w", raw, err)
	}
	if a == nil {
		a = []string{}
	}
	return a, nil
}

// ListTags는 태그 사전 전체 + 부착된 미삭제 링크 수 (link_count 내림차순).
func (s *sqliteStore) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := s.db.Reader.QueryContext(ctx, `
		SELECT t.id, t.name, t.aliases, t.created_at, COUNT(l.id) AS link_count
		FROM tags t
		LEFT JOIN link_tags lt ON lt.tag_id = t.id
		LEFT JOIN links l      ON l.id = lt.link_id AND l.deleted_at IS NULL
		GROUP BY t.id
		ORDER BY link_count DESC, t.name`)
	if err != nil {
		return nil, fmt.Errorf("store: 태그 목록 조회 실패: %w", err)
	}
	defer rows.Close()
	var tags []Tag
	for rows.Next() {
		var (
			t   Tag
			raw string
		)
		if err := rows.Scan(&t.ID, &t.Name, &raw, &t.CreatedAt, &t.LinkCount); err != nil {
			return nil, fmt.Errorf("store: 태그 스캔 실패: %w", err)
		}
		if t.Aliases, err = decodeAliases(raw); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: 태그 목록 순회 실패: %w", err)
	}
	return tags, nil
}

// CreateTag는 태그를 추가한다. 이름 중복(NOCASE)이면 ErrDuplicateTag.
func (s *sqliteStore) CreateTag(ctx context.Context, name string, aliases []string) (*Tag, error) {
	raw, err := encodeAliases(aliases)
	if err != nil {
		return nil, err
	}
	t := Tag{Name: name}
	if t.Aliases, err = decodeAliases(raw); err != nil {
		return nil, err
	}
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		// writer 단일 커넥션이라 check-then-insert가 경합 없이 안전
		var dup int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM tags WHERE name = ?`, name).Scan(&dup)
		if err == nil {
			return fmt.Errorf("%w: %q", ErrDuplicateTag, name)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("store: 태그 중복 확인 실패: %w", err)
		}
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO tags (name, aliases) VALUES (?, ?) RETURNING id, created_at`,
			name, raw).Scan(&t.ID, &t.CreatedAt); err != nil {
			return fmt.Errorf("store: tags INSERT 실패: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &t, nil // 방금 만든 태그 — LinkCount는 0
}

// UpdateTag는 이름/별칭을 수정한다. name/aliases 각각 nil이면 유지.
// 이름이 바뀌면 부착 링크들의 FTS tags 텍스트를 같은 트랜잭션에서 재색인.
func (s *sqliteStore) UpdateTag(ctx context.Context, id int64, name *string, aliases []string) (*Tag, error) {
	var t Tag
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		var (
			curName, curAliases string
			createdAt           int64
		)
		err := tx.QueryRowContext(ctx,
			`SELECT name, aliases, created_at FROM tags WHERE id = ?`, id,
		).Scan(&curName, &curAliases, &createdAt)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("store: 태그 조회 실패: %w", err)
		}

		newName, renamed := curName, false
		if name != nil && *name != curName {
			// 다른 태그와의 중복 검사 (자기 자신 제외, NOCASE)
			var other int64
			err := tx.QueryRowContext(ctx,
				`SELECT id FROM tags WHERE name = ? AND id <> ?`, *name, id).Scan(&other)
			if err == nil {
				return fmt.Errorf("%w: %q", ErrDuplicateTag, *name)
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("store: 태그 중복 확인 실패: %w", err)
			}
			newName, renamed = *name, true
		}
		newAliases := curAliases
		if aliases != nil {
			if newAliases, err = encodeAliases(aliases); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE tags SET name = ?, aliases = ? WHERE id = ?`, newName, newAliases, id); err != nil {
			return fmt.Errorf("store: tags UPDATE 실패: %w", err)
		}
		if renamed {
			if err := reindexLinksOfTag(ctx, tx, id); err != nil {
				return err
			}
		}
		t = Tag{ID: id, Name: newName, CreatedAt: createdAt}
		if t.Aliases, err = decodeAliases(newAliases); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// 응답용 link_count (읽기 — 트랜잭션 밖)
	if err := s.db.Reader.QueryRowContext(ctx, `
		SELECT COUNT(l.id) FROM link_tags lt
		JOIN links l ON l.id = lt.link_id AND l.deleted_at IS NULL
		WHERE lt.tag_id = ?`, id).Scan(&t.LinkCount); err != nil {
		return nil, fmt.Errorf("store: link_count 조회 실패: %w", err)
	}
	return &t, nil
}

// DeleteTag는 사전에서 제거한다. link_tags는 FK CASCADE로 함께 삭제되므로
// 부착됐던 링크들의 FTS를 같은 트랜잭션에서 재색인한다.
func (s *sqliteStore) DeleteTag(ctx context.Context, id int64) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		// CASCADE로 지워지기 전에 재색인 대상 링크를 수집
		linkIDs, err := linkIDsOfTag(ctx, tx, id)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("store: tags DELETE 실패: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: RowsAffected 실패: %w", err)
		}
		if n == 0 {
			return ErrNotFound
		}
		for _, lid := range linkIDs {
			if err := reindexFTS(ctx, tx, lid); err != nil {
				return err
			}
		}
		return nil
	})
}

// linkIDsOfTag는 태그가 부착된 링크 id들을 반환한다.
func linkIDsOfTag(ctx context.Context, tx *sql.Tx, tagID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT link_id FROM link_tags WHERE tag_id = ?`, tagID)
	if err != nil {
		return nil, fmt.Errorf("store: 태그 부착 링크 조회 실패: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: 링크 id 스캔 실패: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: 태그 부착 링크 순회 실패: %w", err)
	}
	return ids, nil
}

// reindexLinksOfTag는 태그가 부착된 모든 링크의 FTS를 재색인한다 (태그 개명 시).
func reindexLinksOfTag(ctx context.Context, tx *sql.Tx, tagID int64) error {
	ids, err := linkIDsOfTag(ctx, tx, tagID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := reindexFTS(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}
