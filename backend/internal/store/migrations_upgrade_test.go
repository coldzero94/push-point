package store

import (
	"errors"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
)

// 오래된 아카이브가 최신 스키마까지 **데이터를 실은 채** 올라오는가.
//
// **왜 이게 따로 필요한가.** 이 패키지의 다른 마이그레이션 테스트는 전부 한 칸을 내렸다
// 올리는 가역성을 본다. 그건 마이그레이션 하나하나가 자기 일을 하는지의 검사이지, **12개를
// 연달아 통과한 실제 아카이브가 멀쩡한지**의 검사가 아니다. 그리고 나머지 테스트는 전부
// 빈 DB에서 시작한다 — 그래서 "행이 있으면 실패하는 마이그레이션"은 어느 테스트도 못 본다.
//
// iOS에서는 이것이 특히 무겁다. 앱이 곧 DB 주인이라 기동 시 마이그레이션이 실패하면
// 아카이브째 못 연다. 되돌릴 수단은 방금 만든 백업뿐이고, 백업이 없던 시절에는 그마저
// 없었다.
func TestMigrations_UpgradeCarriesData(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	m := newMigrator(t, db)

	// **초기 스키마까지 내린다.** 0002는 시드(태그 사전)가 들어오는 지점이라 여기가
	// "옛 아카이브"의 가장 이른 현실적인 모습이다.
	if err := m.Migrate(2); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("0002로 내리기: %v", err)
	}

	// 옛 스키마에만 기대어 행을 넣는다. 나중에 생긴 열을 쓰면 그 시절 아카이브가 아니다.
	if _, err := db.Writer.Exec(`
		INSERT INTO links (url, url_hash, domain, title, description, status, created_at, updated_at)
		VALUES ('https://example.com/old-archive', 'oldhash0001', 'example.com',
		        '12번을 통과해야 하는 옛 링크', '설명', 'done', 1700000000, 1700000000)`); err != nil {
		t.Fatalf("옛 행 삽입: %v", err)
	}
	var linkID int64
	if err := db.Writer.QueryRow(`SELECT id FROM links WHERE url_hash='oldhash0001'`).Scan(&linkID); err != nil {
		t.Fatalf("옛 행 id: %v", err)
	}
	// 태그 연결까지 만든다 — 링크만 있으면 조인이 없는 마이그레이션만 검증된다.
	var tagID int64
	if err := db.Writer.QueryRow(`SELECT id FROM tags LIMIT 1`).Scan(&tagID); err != nil {
		t.Fatalf("시드 태그: %v", err)
	}
	if _, err := db.Writer.Exec(
		`INSERT INTO link_tags (link_id, tag_id, source) VALUES (?, ?, 'rules')`, linkID, tagID); err != nil {
		t.Fatalf("옛 태그 연결: %v", err)
	}

	// 최신까지 올린다.
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("최신까지 올리기: %v", err)
	}

	// 행이 살아 있는가.
	var title string
	if err := db.Reader.QueryRow(
		`SELECT title FROM links WHERE url_hash='oldhash0001'`).Scan(&title); err != nil {
		t.Fatalf("업그레이드 후 링크 조회: %v", err)
	}
	if title != "12번을 통과해야 하는 옛 링크" {
		t.Errorf("제목이 변했다: %q", title)
	}
	var n int
	if err := db.Reader.QueryRow(
		`SELECT count(*) FROM link_tags WHERE link_id=?`, linkID).Scan(&n); err != nil {
		t.Fatalf("업그레이드 후 태그 조회: %v", err)
	}
	if n != 1 {
		t.Errorf("태그 연결이 사라졌다: %d", n)
	}

	// **새 열이 실제로 붙었는가.** 행이 남았다는 것과 스키마가 최신이라는 것은 다른 말이다.
	for _, col := range []string{"body_text", "summary", "opened_at"} {
		var one int
		if err := db.Reader.QueryRow(
			`SELECT count(*) FROM pragma_table_info('links') WHERE name=?`, col).Scan(&one); err != nil {
			t.Fatalf("열 확인 %s: %v", col, err)
		}
		if one != 1 {
			t.Errorf("업그레이드 후에도 links.%s 가 없다", col)
		}
	}

	// 0012가 넣은 별칭이 왔는가 — 마이그레이션이 **시드도** 갱신한다는 사실을 여기서 묶는다.
	var aliases string
	if err := db.Reader.QueryRow(`SELECT aliases FROM tags WHERE name='devops'`).Scan(&aliases); err != nil {
		t.Fatalf("devops 별칭: %v", err)
	}
	if !strings.Contains(aliases, "deployment") {
		t.Errorf("0012 별칭이 업그레이드 경로로 안 왔다: %s", aliases)
	}
}
