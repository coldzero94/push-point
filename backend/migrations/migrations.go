// Package migrations는 SQLite 마이그레이션 SQL을 바이너리에 embed한다.
// golang-migrate + iofs 소스로 시작 시 자동 적용된다 (internal/store/db.go).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
