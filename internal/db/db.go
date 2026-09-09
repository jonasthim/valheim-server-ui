// Package db opens the SQLite database and applies embedded goose migrations.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Open opens (creating if needed) the database at path with WAL, foreign keys
// and a 5 s busy timeout, then migrates to the latest version.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)", path)
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite handles one writer; keep a small pool to avoid lock churn.
	sqldb.SetMaxOpenConns(4)
	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := Migrate(ctx, sqldb); err != nil {
		_ = sqldb.Close()
		return nil, err
	}
	// The database holds password hashes, session ids and the plaintext
	// secrets the manager needs at runtime: owner-only, whatever the umask.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Chmod(path+suffix, 0o600); err != nil && !os.IsNotExist(err) {
			_ = sqldb.Close()
			return nil, fmt.Errorf("restrict permissions on %s: %w", path+suffix, err)
		}
	}
	return sqldb, nil
}

// Migrate applies all pending migrations.
func Migrate(ctx context.Context, sqldb *sql.DB) error {
	goose.SetBaseFS(migrations)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, sqldb, "migrations"); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// OpenMemory opens an in-memory database for tests.
func OpenMemory(ctx context.Context) (*sql.DB, error) {
	sqldb, err := sql.Open("sqlite", "file::memory:?cache=shared&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, err
	}
	sqldb.SetMaxOpenConns(1)
	if err := Migrate(ctx, sqldb); err != nil {
		_ = sqldb.Close()
		return nil, err
	}
	return sqldb, nil
}
