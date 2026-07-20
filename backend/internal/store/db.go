package store

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	sqlitemigrate "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite" // driver name "sqlite" (CGO-free)

	"github.com/coby/push-point/backend/migrations"
)

// DB는 커넥션 전략(writer 1 + reader 4)을 담는 핸들.
//   - Writer: 모든 쓰기 트랜잭션 전용. MaxOpenConns(1)로 락 경합 자체를 제거.
//   - Reader: 목록·검색·상세 등 읽기 전용. WAL 덕에 쓰기와 동시 진행.
type DB struct {
	Writer *sql.DB
	Reader *sql.DB
	Path   string // pushpoint.db 절대/상대 경로
}

// PRAGMA 5종 (docs/v2/05 §3). DSN _pragma 파라미터로 지정해
// 풀의 모든 커넥션에 연결 시점마다 적용되게 한다.
var pragmas = []string{
	"journal_mode(WAL)",
	"synchronous(NORMAL)",
	"busy_timeout(5000)",
	"foreign_keys(1)",
	"cache_size(-64000)", // 64MB
}

// Open은 dataDir/pushpoint.db를 열고 마이그레이션을 적용한 뒤
// writer/reader 분리 핸들을 반환한다. dataDir는 없으면 생성한다.
func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("store: 데이터 디렉터리 생성 실패: %w", err)
	}
	path := filepath.Join(dataDir, "pushpoint.db")
	dsn := dsnFor(path)

	writer, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: writer 오픈 실패: %w", err)
	}
	writer.SetMaxOpenConns(1) // 쓰기 직렬화 — SQLite 쓰기는 어차피 DB 단위 직렬
	writer.SetMaxIdleConns(1)

	// 마이그레이션은 writer로 적용 (embed.FS, 시작 시 자동)
	if err := migrateUp(writer); err != nil {
		writer.Close()
		return nil, err
	}

	reader, err := sql.Open("sqlite", dsn)
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("store: reader 오픈 실패: %w", err)
	}
	reader.SetMaxOpenConns(4)
	reader.SetMaxIdleConns(4)

	if err := reader.Ping(); err != nil {
		writer.Close()
		reader.Close()
		return nil, fmt.Errorf("store: reader ping 실패: %w", err)
	}
	return &DB{Writer: writer, Reader: reader, Path: path}, nil
}

// Close는 writer/reader 풀을 모두 닫는다.
func (d *DB) Close() error {
	return errors.Join(d.Writer.Close(), d.Reader.Close())
}

// dsnFor는 modernc.org/sqlite DSN을 만든다. _pragma 파라미터는
// 커넥션마다 PRAGMA로 실행된다.
func dsnFor(path string) string {
	q := url.Values{}
	for _, p := range pragmas {
		q.Add("_pragma", p)
	}
	return "file:" + path + "?" + q.Encode()
}

// migrateUp은 embed된 backend/migrations/*.sql을 순서대로 적용한다.
// 이미 최신이면(ErrNoChange) 정상 종료.
func migrateUp(db *sql.DB) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("store: 마이그레이션 소스 로드 실패: %w", err)
	}
	drv, err := sqlitemigrate.WithInstance(db, &sqlitemigrate.Config{})
	if err != nil {
		return fmt.Errorf("store: 마이그레이션 드라이버 초기화 실패: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
	if err != nil {
		return fmt.Errorf("store: 마이그레이션 초기화 실패: %w", err)
	}
	// m.Close()는 전달한 *sql.DB까지 닫으므로 호출하지 않는다.
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("store: 마이그레이션 적용 실패: %w", err)
	}
	return nil
}
