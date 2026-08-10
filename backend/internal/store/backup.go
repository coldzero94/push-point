package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// restoreName은 복원 대기 파일. Open이 DB를 열기 **전에** 이 이름을 찾아 갈아끼운다.
//
// **왜 파일을 놓고 다음 기동에 바꾸는가.** 복원은 돌고 있는 서버가 열어 둔 바로 그 파일을
// 통째로 바꾸는 일이다. 열린 핸들 밑에서 파일을 갈면 WAL과 본체가 서로 다른 DB를 가리키게
// 되고, 그 상태는 즉시 안 터지고 **나중에 조용히 틀린 답을 준다.** 그래서 교체는 아무도
// 열고 있지 않은 순간 — 기동 직전 — 에만 한다.
const restoreName = "pushpoint.db.restore"

// Backup은 아카이브 전체를 path에 파일 하나로 쓴다.
//
// **`VACUUM INTO`를 쓴다.** 링크만 골라 내보내면 백업이 아니라 발췌다 — FTS 색인,
// corpus_df, tag_feedback, 잡 이력까지 들어가야 복원된 것이 원본과 같은 앱이 된다.
// 07-DEPLOYMENT §7이 데스크톱에서 `cp`/`VACUUM INTO`를 백업으로 지목한 것과 같은 판단이고,
// 여기서는 폰에 `cp`가 없어서 앱이 대신 한다.
//
// WAL 모드에서도 안전하다 — VACUUM INTO는 하나의 읽기 트랜잭션 안에서 일관된 스냅샷을
// 쓴다. 그래서 저장이 동시에 일어나도 반쪽짜리 파일이 나오지 않는다.
func (d *DB) Backup(ctx context.Context, path string) error {
	if path == "" {
		return fmt.Errorf("store: 백업 경로가 비었습니다")
	}
	// VACUUM INTO는 **대상이 이미 있으면 실패한다.** 덮어쓰기를 원하는 호출자가 매번
	// 지우는 것을 잊지 않게 여기서 지운다.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("store: 백업 대상 정리 실패: %w", err)
	}
	if _, err := d.Writer.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return fmt.Errorf("store: 백업 실패: %w", err)
	}
	return nil
}

// StageRestore는 복원할 DB 파일을 dataDir에 대기시킨다. 실제 교체는 다음 Open이 한다.
//
// **여기서 열어 본다.** 대기시키기만 하고 다음 기동에 검증하면, 잘못된 파일을 받았을 때
// 사용자가 그 사실을 **앱이 안 열리는 것으로** 알게 된다. 받는 자리에서 거절해야 원본이
// 그대로 남는다.
func StageRestore(dataDir string, src string) error {
	if err := validateArchive(src); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("store: 복원 파일 읽기 실패: %w", err)
	}
	dst := filepath.Join(dataDir, restoreName)
	// 0600 — 개인 아카이브 전체가 든 파일이다.
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return fmt.Errorf("store: 복원 파일 대기 실패: %w", err)
	}
	return nil
}

// applyStagedRestore는 대기 중인 복원 파일이 있으면 본체와 바꾼다. Open이 DB를 열기 전에
// 부른다.
//
// **-wal과 -shm을 반드시 지운다.** 남겨 두면 새 본체 위에 옛 DB의 저널이 얹혀
// SQLITE_NOTADB나 — 더 나쁘게는 — 옛 데이터가 되살아난 것처럼 보이는 상태가 된다.
// ios.md가 "복사할 때 셋을 함께 옮기라"고 적은 것의 뒷면이다.
func applyStagedRestore(dataDir string) error {
	staged := filepath.Join(dataDir, restoreName)
	if _, err := os.Stat(staged); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("store: 복원 파일 확인 실패: %w", err)
	}
	// 교체 직전에 한 번 더 본다 — 대기와 기동 사이에 파일이 망가졌을 수 있고, 그 사이에
	// 원본을 지우고 나면 되돌릴 자리가 없다.
	if err := validateArchive(staged); err != nil {
		// 못 쓰는 파일 때문에 앱이 영영 안 열리면 안 된다. 치우고 원본으로 뜬다.
		_ = os.Remove(staged)
		return fmt.Errorf("store: 대기 중이던 복원 파일이 유효하지 않아 무시했습니다: %w", err)
	}
	main := filepath.Join(dataDir, "pushpoint.db")
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(main + suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("store: 옛 저널 정리 실패: %w", err)
		}
	}
	if err := os.Rename(staged, main); err != nil {
		return fmt.Errorf("store: 복원 교체 실패: %w", err)
	}
	return nil
}

// validateArchive는 이 파일이 **우리 아카이브인지** 본다.
//
// SQLite 파일인지만 보면 모자란다 — 사용자가 사진 라이브러리나 남의 앱 DB를 고를 수 있고,
// 그것도 SQLite다. 그걸 받아들이면 아카이브가 조용히 빈 앱으로 바뀐다. 그래서 우리 테이블이
// 있는지까지 확인한다.
func validateArchive(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("store: 복원 파일을 열 수 없습니다: %w", err)
	}
	if st.Size() == 0 {
		return fmt.Errorf("store: 복원 파일이 비어 있습니다")
	}
	// 읽기 전용으로 연다 — 검사가 대상 파일을 바꾸면 안 된다.
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return fmt.Errorf("store: 복원 파일을 열 수 없습니다: %w", err)
	}
	defer db.Close()

	for _, table := range []string{"links", "tags", "link_tags"} {
		var n int
		if err := db.QueryRow(
			`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&n); err != nil {
			return fmt.Errorf("store: 복원 파일이 SQLite가 아니거나 손상됐습니다: %w", err)
		}
		if n != 1 {
			return fmt.Errorf("store: Push-Point 아카이브가 아닙니다 (%s 테이블이 없습니다)", table)
		}
	}
	return nil
}

// Backup은 Store 인터페이스 구현 — 실제 일은 *DB가 한다.
func (s *sqliteStore) Backup(ctx context.Context, path string) error {
	return s.db.Backup(ctx, path)
}
