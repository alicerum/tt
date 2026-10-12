// Package store persists tt's data in SQLite (D13).
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

//go:embed migrations/*.sql
var migrationFS embed.FS

type Store struct {
	db    *sql.DB
	watch *sql.Conn // dedicated connection: PRAGMA data_version is per-connection
}

// Open opens (creating if needed) the database at path and applies migrations.
// path is a plain filesystem path; spaces are fine (e.g. "Application Support").
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	watch, err := db.Conn(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, watch: watch}, nil
}

func (s *Store) Close() error {
	return errors.Join(s.watch.Close(), s.db.Close())
}

// DataVersion changes whenever another connection or process commits a write.
// Poll it to notice external changes such as `tt add` from another shell.
func (s *Store) DataVersion(ctx context.Context) (int64, error) {
	var v int64
	err := s.watch.QueryRowContext(ctx, "PRAGMA data_version").Scan(&v)
	return v, err
}

func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	names, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	slices.Sort(names)
	for _, name := range names {
		base := strings.TrimPrefix(name, "migrations/")
		version, err := strconv.Atoi(base[:strings.IndexByte(base, '_')])
		if err != nil {
			return fmt.Errorf("migration %s: name must start with a number", name)
		}
		if err := applyMigration(ctx, db, name, version); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, name string, version int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var applied int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version = ?", version).Scan(&applied); err != nil {
		return err
	}
	if applied > 0 {
		return nil
	}
	body, err := migrationFS.ReadFile(name)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)",
		version, formatTime(time.Now())); err != nil {
		return err
	}
	return tx.Commit()
}

// InTx runs fn in a write transaction (BEGIN IMMEDIATE); fn's error rolls back.
func (s *Store) InTx(ctx context.Context, fn func(*Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(&Tx{tx: tx}); err != nil {
		return err
	}
	return tx.Commit()
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339Nano, s) }
