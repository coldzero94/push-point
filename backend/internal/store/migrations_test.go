package store

import (
	"testing"

	"github.com/golang-migrate/migrate/v4"
	sqlitemigrate "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/coby/push-point/backend/migrations"
)

// newMigrator는 열려 있는 DB에 embed 마이그레이션 소스를 붙인다.
// m.Close()는 전달한 *sql.DB까지 닫으므로 호출하지 않는다 (db.go migrateUp과 동일).
func newMigrator(t *testing.T, db *DB) *migrate.Migrate {
	t.Helper()
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		t.Fatalf("마이그레이션 소스 로드 실패: %v", err)
	}
	drv, err := sqlitemigrate.WithInstance(db.Writer, &sqlitemigrate.Config{})
	if err != nil {
		t.Fatalf("마이그레이션 드라이버 초기화 실패: %v", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
	if err != nil {
		t.Fatalf("마이그레이션 초기화 실패: %v", err)
	}
	return m
}

// TestMigration0003_TagFacet_Reversible은 down.sql이 실제로 되감기는지 본다.
// down은 운영 경로가 아니라 롤백 경로라 아무 테스트도 태우지 않으면 깨진 채로 커밋된다
// (SQLite의 DROP COLUMN은 인덱스·뷰·트리거 참조가 하나라도 있으면 실패한다).
func TestMigration0003_TagFacet_Reversible(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open 실패: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	m := newMigrator(t, db)

	hasFacet := func() int {
		t.Helper()
		var n int
		if err := db.Writer.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('tags') WHERE name = 'facet'`).Scan(&n); err != nil {
			t.Fatalf("pragma_table_info 실패: %v", err)
		}
		return n
	}

	if hasFacet() != 1 {
		t.Fatal("Open 직후 tags.facet이 없다 — 0003이 적용되지 않았다")
	}
	// 상위 마이그레이션(0004+)을 먼저 내려 0003을 top으로 만든다 — 그래야 Steps(-1)이
	// 0003 down을 정확히 태운다(새 마이그레이션이 붙어도 이 테스트가 0003만 검증).
	if err := m.Migrate(3); err != nil {
		t.Fatalf("버전 3으로 이동 실패: %v", err)
	}
	if err := m.Steps(-1); err != nil {
		t.Fatalf("0003 down 실패: %v", err)
	}
	if hasFacet() != 0 {
		t.Fatal("down 후에도 tags.facet이 남아 있다")
	}
	if err := m.Steps(1); err != nil {
		t.Fatalf("0003 재-up 실패: %v", err)
	}
	// 재-up은 시드 배정 UPDATE까지 다시 돌아야 한다 (멱등).
	var facet string
	if err := db.Writer.QueryRow(`SELECT facet FROM tags WHERE name = 'golang'`).Scan(&facet); err != nil {
		t.Fatalf("facet 조회 실패: %v", err)
	}
	if facet != FacetCraft {
		t.Fatalf("재-up 후 golang facet = %q, want %q", facet, FacetCraft)
	}
}

// TestMigration0004_BodyText_Reversible은 0004 down.sql(DROP COLUMN body_text)이
// 되감기는지 본다.
func TestMigration0004_BodyText_Reversible(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open 실패: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	m := newMigrator(t, db)

	hasBody := func() int {
		t.Helper()
		var n int
		if err := db.Writer.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('links') WHERE name = 'body_text'`).Scan(&n); err != nil {
			t.Fatalf("pragma_table_info 실패: %v", err)
		}
		return n
	}

	if hasBody() != 1 {
		t.Fatal("Open 직후 links.body_text가 없다 — 0004가 적용되지 않았다")
	}
	// 상위 마이그레이션(0005+)을 먼저 내려 0004를 top으로 만든다 — 0003 테스트와 같은 이유.
	if err := m.Migrate(4); err != nil {
		t.Fatalf("버전 4로 이동 실패: %v", err)
	}
	if err := m.Steps(-1); err != nil {
		t.Fatalf("0004 down 실패: %v", err)
	}
	if hasBody() != 0 {
		t.Fatal("down 후에도 links.body_text가 남아 있다")
	}
	if err := m.Steps(1); err != nil {
		t.Fatalf("0004 재-up 실패: %v", err)
	}
	if hasBody() != 1 {
		t.Fatal("재-up 후 links.body_text가 없다")
	}
}

// TestMigration0005_Summary_Reversible은 0005 down.sql(DROP COLUMN summary)이
// 되감기는지 본다 — 0005가 top이라 Steps(-1)이 곧 0005 down이다.
func TestMigration0005_Summary_Reversible(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open 실패: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	m := newMigrator(t, db)

	hasSummary := func() int {
		t.Helper()
		var n int
		if err := db.Writer.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('links') WHERE name = 'summary'`).Scan(&n); err != nil {
			t.Fatalf("pragma_table_info 실패: %v", err)
		}
		return n
	}

	if hasSummary() != 1 {
		t.Fatal("Open 직후 links.summary가 없다 — 0005가 적용되지 않았다")
	}
	if err := m.Steps(-1); err != nil {
		t.Fatalf("0005 down 실패: %v", err)
	}
	if hasSummary() != 0 {
		t.Fatal("down 후에도 links.summary가 남아 있다")
	}
	if err := m.Steps(1); err != nil {
		t.Fatalf("0005 재-up 실패: %v", err)
	}
	if hasSummary() != 1 {
		t.Fatal("재-up 후 links.summary가 없다")
	}
}
