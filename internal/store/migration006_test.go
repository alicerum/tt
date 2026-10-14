package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"tt/internal/domain"
)

// TestMigration006 runs 006 on a database as Plan 9 left it (D76): no task
// gets an impact except open pinned reviews (impact 1, due 3 days after the
// request, or after first seen without one); priority stays as it was,
// unread.
func TestMigration006(t *testing.T) {
	s, path := openTemp(t)
	for _, q := range []string{
		"DELETE FROM schema_migrations WHERE version = 6",
		"ALTER TABLE tasks DROP COLUMN impact",
		"ALTER TABLE tasks DROP COLUMN due",
	} {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	at := func(tm time.Time) string { return formatTime(tm) }
	created := at(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))
	requested := time.Date(2026, 10, 12, 23, 30, 0, 123456789, time.UTC)
	seen := time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)
	tasks := []struct {
		id                   int64
		priority             any
		parent               any
		status               string
		sourceKind, sourceID any
	}{
		{1, 1, nil, "open", nil, nil},                 // a triaged top-level task
		{2, nil, 1, "open", nil, nil},                 // its child
		{3, 2, nil, "done", nil, nil},                 // closed
		{4, 2, nil, "open", "github", "acme/api#901"}, // an open review with a request time
		{5, 2, nil, "open", "github", "acme/web#3"},   // an open review seen, never requested
		{6, 2, nil, "done", "github", "acme/web#4"},   // a closed review
		{7, 2, nil, "open", "github", "acme/gone#1"},  // its source item is gone
	}
	for _, tk := range tasks {
		_, err := s.db.Exec(`INSERT INTO tasks (id, title, priority, size, parent_id, status, source_kind, source_id,
			created_at, updated_at) VALUES (?, ?, ?, 's', ?, ?, ?, ?, ?, ?)`,
			tk.id, "task", tk.priority, tk.parent, tk.status, tk.sourceKind, tk.sourceID, created, created)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, it := range []struct {
		id        string
		requested any
	}{{"acme/api#901", at(requested)}, {"acme/web#3", nil}, {"acme/web#4", at(requested)}} {
		_, err := s.db.Exec(`INSERT INTO source_items (kind, external_id, title, requested_at, first_seen_at)
			VALUES ('github', ?, 'PR', ?, ?)`, it.id, it.requested, at(seen))
		if err != nil {
			t.Fatal(err)
		}
	}
	s.Close()

	again, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	snap, err := again.LoadSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]struct {
		impact   int
		due      domain.Day
		priority int
	}{
		1: {0, "", 1}, 2: {0, "", 0}, 3: {0, "", 2},
		4: {1, "2026-10-15", 2}, 5: {1, "2026-10-16", 2}, 6: {0, "", 2}, 7: {1, "", 2},
	}
	for id, w := range want {
		got, _ := snap.Get(id)
		var priority sql.NullInt64
		if err := again.db.QueryRow("SELECT priority FROM tasks WHERE id = ?", id).Scan(&priority); err != nil {
			t.Fatal(err)
		}
		if got.Impact != w.impact || got.Due != w.due || int(priority.Int64) != w.priority {
			t.Errorf("task #%d: impact %d, due %q, priority %d; want %d, %q, %d",
				id, got.Impact, got.Due, priority.Int64, w.impact, w.due, w.priority)
		}
	}
}
