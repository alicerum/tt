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

var (
	ErrNotFound    = errors.New("not found")
	ErrNewerSchema = errors.New("database was created by a newer tt; upgrade tt")
)

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
	latest := 0
	for _, name := range names {
		version, err := migrationVersion(name)
		if err != nil {
			return err
		}
		if err := applyMigration(ctx, db, name, version); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		latest = max(latest, version)
	}
	var applied int
	if err := db.QueryRowContext(ctx, "SELECT coalesce(max(version), 0) FROM schema_migrations").Scan(&applied); err != nil {
		return err
	}
	if applied > latest {
		return fmt.Errorf("schema version %d, this tt knows up to %d: %w", applied, latest, ErrNewerSchema)
	}
	return nil
}

// migrationVersion parses "migrations/NNN_name.sql" into NNN.
func migrationVersion(name string) (int, error) {
	base := strings.TrimPrefix(name, "migrations/")
	prefix, _, ok := strings.Cut(base, "_")
	version, err := strconv.Atoi(prefix)
	if !ok || err != nil {
		return 0, fmt.Errorf("migration %s: name must look like NNN_name.sql", name)
	}
	return version, nil
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

// readTx runs fn in a read-only transaction: one consistent view, and it
// never takes the write lock (modernc issues a plain BEGIN for ReadOnly).
func (s *Store) readTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	return fn(tx)
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

// timeLayout is fixed-width (always 9 fractional digits, always Z) so stored
// times sort correctly as text.
const timeLayout = "2006-01-02T15:04:05.000000000Z07:00"

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

func parseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339Nano, s) }
