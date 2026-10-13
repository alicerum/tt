package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestSnapshotDoesNotWaitForWriters(t *testing.T) {
	writer, path := openTemp(t)
	reader, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	insert(t, writer, sampleTask())

	ctx := context.Background()
	holding := make(chan struct{})
	done := make(chan error)
	go func() {
		done <- writer.InTx(ctx, func(x *Tx) error {
			if _, err := x.InsertTask(ctx, sampleTask()); err != nil {
				return err
			}
			close(holding)
			time.Sleep(time.Second) // hold the write lock
			return nil
		})
	}()
	<-holding
	start := time.Now()
	snap, err := reader.LoadSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 500*time.Millisecond {
		t.Errorf("LoadSnapshot waited %v for a writer; reads must not take the write lock", took)
	}
	if n := len(snap.Tasks()); n != 1 {
		t.Errorf("snapshot sees %d tasks; want 1 (the uncommitted insert is invisible)", n)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestNewerSchemaIsRefused(t *testing.T) {
	s, path := openTemp(t)
	if _, err := s.db.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES (999, 'x')"); err != nil {
		t.Fatal(err)
	}
	// Drop the newest real migration's row: a refused Open must not re-apply it.
	if _, err := s.db.Exec("DELETE FROM schema_migrations WHERE version = (SELECT max(version) FROM schema_migrations WHERE version < 999)"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	count := func() int {
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		var n int
		if err := db.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := count()
	if _, err := Open(context.Background(), path); !errors.Is(err, ErrNewerSchema) {
		t.Errorf("Open error = %v; want ErrNewerSchema", err)
	}
	if after := count(); after != before {
		t.Errorf("refused Open changed schema_migrations rows: %d -> %d", before, after)
	}
}

func TestMigrationVersion(t *testing.T) {
	if v, err := migrationVersion("migrations/002_planning.sql"); err != nil || v != 2 {
		t.Errorf("migrationVersion = %d, %v; want 2", v, err)
	}
	for _, bad := range []string{"migrations/002.sql", "migrations/x_y.sql", "migrations/_y.sql"} {
		if _, err := migrationVersion(bad); err == nil {
			t.Errorf("migrationVersion(%q) succeeded; want error", bad)
		}
	}
}
