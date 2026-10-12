# tt Plan 2 — Planning Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The planning core behind the morning ritual: pins (candidates), time blocks, planning days with rollover and pinned-days counters, plan commit, and a `Plan` query for Today — plus the Plan 1 carry-forward fixes (safe undo, consistent reads, newer-schema guard).

**Architecture:** Pure planning types and DST-safe day mapping live in `domain`. `store` gets migration 002 (`pins`, `blocks`, `days`) and planning queries, all reads in one read-only transaction. `app.Service` gains planning commands (each returning an `Undo`), `StartDay` (idempotent, one transaction, guarded by the `days` primary key), `CommitPlan`, and `Plan`. No UI in this plan; Plan 3 builds the TUI on these calls.

**Tech Stack:** Go 1.27, `modernc.org/sqlite` v1.60.1, `github.com/BurntSushi/toml` v1.6.0, `github.com/google/go-cmp` v0.7.0 (tests). No new dependencies.

**Spec:** `docs/superpowers/specs/2026-10-12-task-tracking-design.md` (decisions D9–D10d, D12a–D12d, D19, D22, D24; section "Carry-forward from Plan 1 review").

## Global Constraints

- No new dependencies.
- Only `internal/app` mutates state.
- Planning day (D19): wall-clock times before `day_start` (default 04:00) belong to the previous date. Days are `"YYYY-MM-DD"` strings.
- Block times are wall-clock offsets from midnight of the planning day, in whole minutes; a block must lie within `[day_start, day_start + 24h)`. Task blocks may not overlap each other; meetings are not checked (D18).
- Rollover (D10): +1 for each **open** task that had a block on the previous planning day; candidates never increment it; computed per planning day (D10b), not calendar day.
- Pinned days (D10d): planning days from `pinned_on` through today, inclusive; blocking a task resets `pinned_on` to today; a rollover re-pins with `pinned_on` = the new day.
- Closed tasks stay pinned (visible as wins) until the next `StartDay` unpins them.
- Undo of a task edit is refused with `ErrConflict` if the task row changed since (carry-forward).
- Defaults: `planning.unsized_block = "30m"`, `planning.rollover_alert = 3`.
- Every task ends green on: `gofmt -l .` (no output), `go vet ./...`, `go test ./...`.
- Every commit message ends with a blank line and `Co-Authored-By: Claude <noreply@anthropic.com>`.

## Review Focus

1. **DST in Amsterdam**: 04:30 on 2026-03-29 (spring forward) is that day; 03:59 on 2026-10-25 (fall back) is the day before. Absolute-duration arithmetic gets both wrong. Pinned in Task 3 `TestPlanningDay`.
2. **TUI and `tt` racing `StartDay` at the boundary**: rollover must count once. Pinned in Task 6 `TestStartDayRaceCountsOnce`.
3. **App not opened for days** (weekend, holiday): one rollover, pinned days +1, recap since the last planning day. Pinned in Task 6 `TestStartDayCountsPlanningDaysNotCalendarDays`.
4. **ctrl+z after a system write**: undoing an edit made before a rollover must not erase the rollover. Pinned in Task 2 `TestUndoRefusesWhenTaskChangedSince` and Task 6 `TestUndoAfterRolloverConflicts`.
5. **Late-night work**: a block at 01:00 after midnight belongs to the current planning day. Pinned in Task 3 `TestDayAt` and Task 6 `TestBlock`.

## File Structure

```
internal/store/store.go             modify: ErrNewerSchema, migrationVersion, readTx
internal/store/tasks.go             modify: LoadSnapshot in a read tx (loadTasks)
internal/store/hardening_test.go    new
internal/app/service.go             modify: ErrConflict + compare-and-set undo; LoadPlan in Store interface
internal/app/tasks.go               modify: Add's undo uses compare-and-set
internal/app/undo_test.go           new
internal/domain/plan.go             new: Day, PlanningDay, Day.At, Pin, Block, DefaultBlockLength, Candidate, Plan
internal/domain/plan_test.go        new
internal/config/config.go           modify: UnsizedBlock, RolloverAlert, [planning] section
internal/config/planning_test.go    new
internal/store/migrations/002_planning.sql  new
internal/store/planning.go          new: pins, blocks, days, LoadPlan
internal/store/planning_test.go     new
internal/app/planning.go            new: Today, DefaultBlockLength, Pin, Unpin, Block, MoveBlock, ResizeBlock, Unblock
internal/app/day.go                 new: StartDay, CommitPlan, Plan
internal/app/planning_test.go       new
internal/app/day_test.go            new
```

Changes to existing files are given as patches; apply each with `git apply` from the repo root (it fails loudly if the file doesn't match, which means an earlier step went wrong).

---

### Task 1: Store hardening

**Files:**
- Modify: `internal/store/store.go`, `internal/store/tasks.go`
- Test: `internal/store/hardening_test.go`

**Interfaces:**
- Consumes: Plan 1 store (`Open`, `InTx`, `LoadSnapshot`, `Tx.InsertTask`; test helpers `openTemp`, `insert`, `sampleTask` in `store_test.go`).
- Produces:
  - `var ErrNewerSchema error`: `Open` returns it (wrapped) when `schema_migrations` holds a version newer than any embedded migration.
  - `func migrationVersion(name string) (int, error)`: parses `migrations/NNN_name.sql`; errors (no panic) on other names.
  - `func (s *Store) readTx(ctx, fn func(*sql.Tx) error) error`: read-only transaction (`sql.TxOptions{ReadOnly: true}`); modernc issues a plain `BEGIN` for it, so reads never take the write lock.
  - `func loadTasks(ctx, *sql.Tx) ([]domain.Task, error)`; `LoadSnapshot` now reads tasks and tags inside one `readTx`.

- [ ] **Step 1: Write the failing tests**

`internal/store/hardening_test.go`:

```go
package store

import (
	"context"
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
	s.Close()
	if _, err := Open(context.Background(), path); !errors.Is(err, ErrNewerSchema) {
		t.Errorf("Open error = %v; want ErrNewerSchema", err)
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/store/`
Expected: FAIL to build: `undefined: ErrNewerSchema`, `undefined: migrationVersion`

- [ ] **Step 3: Apply the store changes**

```bash
git apply <<'PATCH'
diff --git a/internal/store/store.go b/internal/store/store.go
index d7c2f40..ea35e1d 100644
--- a/internal/store/store.go
+++ b/internal/store/store.go
@@ -16,7 +16,10 @@ import (
 	_ "modernc.org/sqlite"
 )
 
-var ErrNotFound = errors.New("not found")
+var (
+	ErrNotFound    = errors.New("not found")
+	ErrNewerSchema = errors.New("database was created by a newer tt; upgrade tt")
+)
 
 //go:embed migrations/*.sql
 var migrationFS embed.FS
@@ -68,19 +71,38 @@ func migrate(ctx context.Context, db *sql.DB) error {
 		return err
 	}
 	slices.Sort(names)
+	latest := 0
 	for _, name := range names {
-		base := strings.TrimPrefix(name, "migrations/")
-		version, err := strconv.Atoi(base[:strings.IndexByte(base, '_')])
+		version, err := migrationVersion(name)
 		if err != nil {
-			return fmt.Errorf("migration %s: name must start with a number", name)
+			return err
 		}
 		if err := applyMigration(ctx, db, name, version); err != nil {
 			return fmt.Errorf("migration %s: %w", name, err)
 		}
+		latest = max(latest, version)
+	}
+	var applied int
+	if err := db.QueryRowContext(ctx, "SELECT coalesce(max(version), 0) FROM schema_migrations").Scan(&applied); err != nil {
+		return err
+	}
+	if applied > latest {
+		return fmt.Errorf("schema version %d, this tt knows up to %d: %w", applied, latest, ErrNewerSchema)
 	}
 	return nil
 }
 
+// migrationVersion parses "migrations/NNN_name.sql" into NNN.
+func migrationVersion(name string) (int, error) {
+	base := strings.TrimPrefix(name, "migrations/")
+	prefix, _, ok := strings.Cut(base, "_")
+	version, err := strconv.Atoi(prefix)
+	if !ok || err != nil {
+		return 0, fmt.Errorf("migration %s: name must look like NNN_name.sql", name)
+	}
+	return version, nil
+}
+
 func applyMigration(ctx context.Context, db *sql.DB, name string, version int) error {
 	tx, err := db.BeginTx(ctx, nil)
 	if err != nil {
@@ -108,6 +130,17 @@ func applyMigration(ctx context.Context, db *sql.DB, name string, version int) e
 	return tx.Commit()
 }
 
+// readTx runs fn in a read-only transaction: one consistent view, and it
+// never takes the write lock (modernc issues a plain BEGIN for ReadOnly).
+func (s *Store) readTx(ctx context.Context, fn func(*sql.Tx) error) error {
+	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
+	if err != nil {
+		return err
+	}
+	defer tx.Rollback()
+	return fn(tx)
+}
+
 // InTx runs fn in a write transaction (BEGIN IMMEDIATE); fn's error rolls back.
 func (s *Store) InTx(ctx context.Context, fn func(*Tx) error) error {
 	tx, err := s.db.BeginTx(ctx, nil)
diff --git a/internal/store/tasks.go b/internal/store/tasks.go
index 95fbe5e..0ca3e6f 100644
--- a/internal/store/tasks.go
+++ b/internal/store/tasks.go
@@ -102,11 +102,24 @@ func (x *Tx) setTags(ctx context.Context, id int64, tags []string) error {
 	return nil
 }
 
-// LoadSnapshot reads every task with its tags.
+// LoadSnapshot reads every task with its tags in one consistent read.
 func (s *Store) LoadSnapshot(ctx context.Context) (domain.Snapshot, error) {
-	rows, err := s.db.QueryContext(ctx, "SELECT "+taskColumns+" FROM tasks ORDER BY id")
+	var snap domain.Snapshot
+	err := s.readTx(ctx, func(tx *sql.Tx) error {
+		tasks, err := loadTasks(ctx, tx)
+		if err != nil {
+			return err
+		}
+		snap = domain.NewSnapshot(tasks)
+		return nil
+	})
+	return snap, err
+}
+
+func loadTasks(ctx context.Context, tx *sql.Tx) ([]domain.Task, error) {
+	rows, err := tx.QueryContext(ctx, "SELECT "+taskColumns+" FROM tasks ORDER BY id")
 	if err != nil {
-		return domain.Snapshot{}, err
+		return nil, err
 	}
 	defer rows.Close()
 	var tasks []domain.Task
@@ -114,34 +127,31 @@ func (s *Store) LoadSnapshot(ctx context.Context) (domain.Snapshot, error) {
 	for rows.Next() {
 		t, err := scanTask(rows)
 		if err != nil {
-			return domain.Snapshot{}, err
+			return nil, err
 		}
 		index[t.ID] = len(tasks)
 		tasks = append(tasks, t)
 	}
 	if err := rows.Err(); err != nil {
-		return domain.Snapshot{}, err
+		return nil, err
 	}
 
-	tagRows, err := s.db.QueryContext(ctx, "SELECT task_id, tag FROM task_tags ORDER BY task_id, tag")
+	tagRows, err := tx.QueryContext(ctx, "SELECT task_id, tag FROM task_tags ORDER BY task_id, tag")
 	if err != nil {
-		return domain.Snapshot{}, err
+		return nil, err
 	}
 	defer tagRows.Close()
 	for tagRows.Next() {
 		var id int64
 		var tag string
 		if err := tagRows.Scan(&id, &tag); err != nil {
-			return domain.Snapshot{}, err
+			return nil, err
 		}
 		if i, ok := index[id]; ok {
 			tasks[i].Tags = append(tasks[i].Tags, tag)
 		}
 	}
-	if err := tagRows.Err(); err != nil {
-		return domain.Snapshot{}, err
-	}
-	return domain.NewSnapshot(tasks), nil
+	return tasks, tagRows.Err()
 }
 
 func taskArgs(t domain.Task) []any {
PATCH
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test ./internal/store/`
Expected: no gofmt output; `ok  tt/internal/store`. `TestSnapshotDoesNotWaitForWriters` takes ~1s: it holds a write lock for a second.

- [ ] **Step 5: Commit**

```bash
git add internal/store
git commit -m "fix(store): consistent snapshot reads and newer-schema guard" -m "Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 2: Compare-and-set undo

**Files:**
- Modify: `internal/app/service.go`, `internal/app/tasks.go`
- Test: `internal/app/undo_test.go`

**Interfaces:**
- Consumes: Plan 1 app (`modify`, `Add`, `Edit`, `SetNote`, `SetPriority`; test helpers `newService`, `mustAdd`, `mustGet`, `mustUndo` in `service_test.go`).
- Produces:
  - `var ErrConflict error` in package `app`.
  - `func unchangedSince(ctx, x *store.Tx, want domain.Task) error`: `ErrConflict` (wrapped) unless the stored row's `UpdatedAt` equals `want.UpdatedAt`.
  - Every `Undo` returned by `modify` and by `Add` now checks `unchangedSince` against the row the command wrote before restoring or deleting. A refused undo changes nothing.

Why (spec carry-forward): from Task 6 on, `StartDay` writes tasks (rollover). Whole-row restore would silently erase those writes.

- [ ] **Step 1: Write the failing tests**

`internal/app/undo_test.go`:

```go
package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUndoRefusesWhenTaskChangedSince(t *testing.T) {
	s, clock := newService(t)
	must := mustUndo(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Original !1 ~s")

	clock.Advance(time.Minute)
	editUndo := must(s.Edit(ctx, task.ID, "Edited !1 ~s"))
	clock.Advance(time.Minute)
	noteUndo := must(s.SetNote(ctx, task.ID, "later change")) // stands in for a rollover or auto-close

	if err := editUndo(ctx); !errors.Is(err, ErrConflict) {
		t.Fatalf("undo of an older command: error = %v; want ErrConflict", err)
	}
	if got := mustGet(t, s, task.ID); got.Title != "Edited" || got.Note != "later change" {
		t.Errorf("a refused undo must change nothing: %+v", got)
	}

	// Undoing newest-first still works.
	if err := noteUndo(ctx); err != nil {
		t.Fatal(err)
	}
	if err := editUndo(ctx); err != nil {
		t.Fatalf("undo in order: %v", err)
	}
	if got := mustGet(t, s, task.ID); got.Title != "Original" || got.Note != "" {
		t.Errorf("after undoing both: %+v", got)
	}
}

func TestUndoAddRefusesWhenTaskChangedSince(t *testing.T) {
	s, clock := newService(t)
	ctx := context.Background()
	task, addUndo, err := s.Add(ctx, "Captured")
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Minute)
	if _, err := s.SetPriority(ctx, task.ID, 2); err != nil {
		t.Fatal(err)
	}
	if err := addUndo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo Add after a later change: error = %v; want ErrConflict", err)
	}
	if got := mustGet(t, s, task.ID); got.Priority != 2 {
		t.Errorf("task must survive a refused undo: %+v", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/`
Expected: FAIL to build: `undefined: ErrConflict`

- [ ] **Step 3: Apply the service changes**

```bash
git apply <<'PATCH'
diff --git a/internal/app/service.go b/internal/app/service.go
index d41e72a..026157e 100644
--- a/internal/app/service.go
+++ b/internal/app/service.go
@@ -4,6 +4,8 @@ package app
 
 import (
 	"context"
+	"errors"
+	"fmt"
 	"time"
 
 	"tt/internal/config"
@@ -48,16 +50,20 @@ func (s *Service) DataVersion(ctx context.Context) (int64, error) {
 	return s.store.DataVersion(ctx)
 }
 
+// ErrConflict means the task changed after the command being undone, e.g.
+// a rollover or a GitHub auto-close; undoing would silently revert that.
+var ErrConflict = errors.New("task changed since; can't undo")
+
 // modify loads task id, applies fn, saves it, and returns an Undo that
-// restores the previous row.
+// restores the previous row, but only if nothing has changed the row since.
 func (s *Service) modify(ctx context.Context, id int64, fn func(x *store.Tx, t *domain.Task) error) (Undo, error) {
-	var before domain.Task
+	var before, after domain.Task
 	err := s.store.InTx(ctx, func(x *store.Tx) error {
 		var err error
 		if before, err = x.GetTask(ctx, id); err != nil {
 			return err
 		}
-		after := before
+		after = before
 		after.Tags = append([]string(nil), before.Tags...)
 		if err := fn(x, &after); err != nil {
 			return err
@@ -69,6 +75,24 @@ func (s *Service) modify(ctx context.Context, id int64, fn func(x *store.Tx, t *
 		return nil, err
 	}
 	return func(ctx context.Context) error {
-		return s.store.InTx(ctx, func(x *store.Tx) error { return x.UpdateTask(ctx, before) })
+		return s.store.InTx(ctx, func(x *store.Tx) error {
+			if err := unchangedSince(ctx, x, after); err != nil {
+				return err
+			}
+			return x.UpdateTask(ctx, before)
+		})
 	}, nil
 }
+
+// unchangedSince fails with ErrConflict unless task want.ID still has the
+// UpdatedAt that want carries.
+func unchangedSince(ctx context.Context, x *store.Tx, want domain.Task) error {
+	cur, err := x.GetTask(ctx, want.ID)
+	if err != nil {
+		return err
+	}
+	if !cur.UpdatedAt.Equal(want.UpdatedAt) {
+		return fmt.Errorf("task #%d: %w", want.ID, ErrConflict)
+	}
+	return nil
+}
diff --git a/internal/app/tasks.go b/internal/app/tasks.go
index 402ed40..fb635cc 100644
--- a/internal/app/tasks.go
+++ b/internal/app/tasks.go
@@ -37,9 +37,14 @@ func (s *Service) Add(ctx context.Context, line string) (domain.Task, Undo, erro
 	if err != nil {
 		return domain.Task{}, nil, err
 	}
-	id := t.ID
+	added := t
 	undo := func(ctx context.Context) error {
-		return s.store.InTx(ctx, func(x *store.Tx) error { return x.DeleteTask(ctx, id) })
+		return s.store.InTx(ctx, func(x *store.Tx) error {
+			if err := unchangedSince(ctx, x, added); err != nil {
+				return err
+			}
+			return x.DeleteTask(ctx, added.ID)
+		})
 	}
 	return t, undo, nil
 }
PATCH
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test ./internal/app/`
Expected: no gofmt output; `ok  tt/internal/app` (Plan 1's undo tests still pass: they undo newest-first).

- [ ] **Step 5: Commit**

```bash
git add internal/app
git commit -m "fix(app): undo refuses when the task changed since" -m "Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 3: Planning domain types

**Files:**
- Create: `internal/domain/plan.go`
- Test: `internal/domain/plan_test.go`

**Interfaces:**
- Consumes: `domain.Task`, `domain.Size`, `domain.SizeNone` (Plan 1).
- Produces:
  - `type Day string`; `func PlanningDay(t time.Time, dayStart time.Duration) Day` (uses wall-clock fields of `t` in its own location); `func ParseDay(string) (Day, error)`; `func (d Day) At(off time.Duration, loc *time.Location) time.Time` (offsets ≥ 24h land on the next date).
  - `type Pin struct{ TaskID int64; PinnedOn Day }`.
  - `type Block struct{ ID, TaskID int64; Day Day; Start, Duration time.Duration }`; `(Block) End() time.Duration`; `(Block) Overlaps(Block) bool` (same day, half-open intervals: touching is not overlapping).
  - `func DefaultBlockLength(size Size, gap time.Duration, sizes map[Size]time.Duration, unsized time.Duration) time.Duration` (D24; `gap` 0 = no cap).
  - `type Candidate struct{ Task Task; PinnedDays int; Blocked, Alert bool }`; `type Plan struct{ Day Day; Committed bool; Candidates []Candidate; Blocks []Block }`.

- [ ] **Step 1: Write the failing tests**

`internal/domain/plan_test.go`:

```go
package domain

import (
	"testing"
	"time"
)

func TestPlanningDay(t *testing.T) {
	ams, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	const four = 4 * time.Hour
	cases := []struct {
		name     string
		t        time.Time
		dayStart time.Duration
		want     Day
	}{
		{"morning", time.Date(2026, 10, 12, 9, 0, 0, 0, ams), four, "2026-10-12"},
		{"exactly day start", time.Date(2026, 10, 12, 4, 0, 0, 0, ams), four, "2026-10-12"},
		{"just before day start", time.Date(2026, 10, 12, 3, 59, 59, 0, ams), four, "2026-10-11"},
		{"late night", time.Date(2026, 10, 13, 1, 30, 0, 0, ams), four, "2026-10-12"},
		{"midnight start", time.Date(2026, 10, 13, 0, 0, 0, 0, ams), 0, "2026-10-13"},
		{"month boundary", time.Date(2026, 11, 1, 2, 0, 0, 0, ams), four, "2026-10-31"},
		{"year boundary", time.Date(2027, 1, 1, 3, 0, 0, 0, ams), four, "2026-12-31"},
		// Review Focus: DST. Absolute-duration arithmetic gets these wrong.
		{"spring forward, after day start", time.Date(2026, 3, 29, 4, 30, 0, 0, ams), four, "2026-03-29"},
		{"spring forward, before day start", time.Date(2026, 3, 29, 3, 30, 0, 0, ams), four, "2026-03-28"},
		{"fall back, after day start", time.Date(2026, 10, 25, 4, 0, 0, 0, ams), four, "2026-10-25"},
		{"fall back, before day start", time.Date(2026, 10, 25, 3, 59, 0, 0, ams), four, "2026-10-24"},
	}
	for _, c := range cases {
		if got := PlanningDay(c.t, c.dayStart); got != c.want {
			t.Errorf("%s: PlanningDay(%v) = %s; want %s", c.name, c.t, got, c.want)
		}
	}
}

func TestDayAt(t *testing.T) {
	ams, _ := time.LoadLocation("Europe/Amsterdam")
	d := Day("2026-03-29") // DST starts at 02:00
	if got, want := d.At(9*time.Hour+30*time.Minute, ams), time.Date(2026, 3, 29, 9, 30, 0, 0, ams); !got.Equal(want) {
		t.Errorf("At(9:30) = %v; want %v", got, want)
	}
	if got, want := d.At(25*time.Hour, ams), time.Date(2026, 3, 30, 1, 0, 0, 0, ams); !got.Equal(want) {
		t.Errorf("At(25h) = %v; want next day 01:00 (%v)", got, want)
	}
	if PlanningDay(d.At(4*time.Hour, ams), 4*time.Hour) != d {
		t.Error("PlanningDay(d.At(dayStart)) should be d")
	}
}

func TestParseDay(t *testing.T) {
	if d, err := ParseDay("2026-10-12"); err != nil || d != "2026-10-12" {
		t.Errorf("ParseDay = %q, %v", d, err)
	}
	for _, bad := range []string{"", "2026-13-01", "12-10-2026", "2026-10-12T00:00"} {
		if _, err := ParseDay(bad); err == nil {
			t.Errorf("ParseDay(%q) succeeded; want error", bad)
		}
	}
}

func TestBlockOverlaps(t *testing.T) {
	b := Block{Day: "2026-10-12", Start: 10 * time.Hour, Duration: time.Hour}
	cases := []struct {
		name string
		o    Block
		want bool
	}{
		{"same slot", b, true},
		{"inside", Block{Day: b.Day, Start: 10*time.Hour + 15*time.Minute, Duration: 15 * time.Minute}, true},
		{"overlapping end", Block{Day: b.Day, Start: 10*time.Hour + 30*time.Minute, Duration: time.Hour}, true},
		{"touching after", Block{Day: b.Day, Start: 11 * time.Hour, Duration: time.Hour}, false},
		{"touching before", Block{Day: b.Day, Start: 9 * time.Hour, Duration: time.Hour}, false},
		{"other day", Block{Day: "2026-10-13", Start: 10 * time.Hour, Duration: time.Hour}, false},
	}
	for _, c := range cases {
		if got := b.Overlaps(c.o); got != c.want {
			t.Errorf("%s: Overlaps = %v; want %v", c.name, got, c.want)
		}
	}
}

func TestDefaultBlockLength(t *testing.T) {
	sizes := map[Size]time.Duration{SizeS: time.Hour, SizeM: 4 * time.Hour, SizeL: 8 * time.Hour, SizeXL: 40 * time.Hour}
	unsized := 30 * time.Minute
	cases := []struct {
		size Size
		gap  time.Duration
		want time.Duration
	}{
		{SizeS, 3 * time.Hour, time.Hour},
		{SizeXL, 2 * time.Hour, 2 * time.Hour},
		{SizeM, 0, 4 * time.Hour},
		{SizeNone, 3 * time.Hour, 30 * time.Minute},
		{SizeNone, 20 * time.Minute, 20 * time.Minute},
	}
	for _, c := range cases {
		if got := DefaultBlockLength(c.size, c.gap, sizes, unsized); got != c.want {
			t.Errorf("DefaultBlockLength(%q, %v) = %v; want %v", c.size, c.gap, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/domain/`
Expected: FAIL to build: `undefined: Day`, `undefined: PlanningDay`, …

- [ ] **Step 3: Implement the planning types**

`internal/domain/plan.go`:

```go
package domain

import (
	"fmt"
	"time"
)

// Day is a planning day as "YYYY-MM-DD" (D19). Days compare correctly as strings.
type Day string

// PlanningDay returns the planning day that t (in its own location) falls in.
// Wall-clock times before dayStart belong to the previous date, so late-night
// work stays "today". Uses wall-clock fields, so DST changes don't shift it.
func PlanningDay(t time.Time, dayStart time.Duration) Day {
	y, m, d := t.Date()
	sinceMidnight := time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute +
		time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())
	if sinceMidnight < dayStart {
		d--
	}
	return Day(time.Date(y, m, d, 12, 0, 0, 0, time.UTC).Format(time.DateOnly))
}

// ParseDay validates a "YYYY-MM-DD" string.
func ParseDay(s string) (Day, error) {
	if _, err := time.Parse(time.DateOnly, s); err != nil {
		return "", fmt.Errorf("day %q: want YYYY-MM-DD", s)
	}
	return Day(s), nil
}

// At returns the instant at wall-clock offset off (e.g. 9h30m) on day d in loc.
// Offsets of 24h or more land on the following date (after-midnight work).
func (d Day) At(off time.Duration, loc *time.Location) time.Time {
	t, err := time.Parse(time.DateOnly, string(d))
	if err != nil {
		panic(fmt.Sprintf("invalid Day %q", d))
	}
	minutes := int(off / time.Minute)
	return time.Date(t.Year(), t.Month(), t.Day(), minutes/60, minutes%60, 0, 0, loc)
}

// Pin marks a task as one of today's candidates since PinnedOn (D12a, D10d).
type Pin struct {
	TaskID   int64
	PinnedOn Day
}

// Block is a task placed on a planning day's timeline (D12a). Start is the
// wall-clock offset from midnight of Day; values past 24h mean after midnight.
type Block struct {
	ID       int64
	TaskID   int64
	Day      Day
	Start    time.Duration
	Duration time.Duration
}

func (b Block) End() time.Duration { return b.Start + b.Duration }

// Overlaps reports whether b and o share any time on the same day.
func (b Block) Overlaps(o Block) bool {
	return b.Day == o.Day && b.Start < o.End() && o.Start < b.End()
}

// DefaultBlockLength (D24): a task's size duration, or unsized for unsized
// tasks, capped at gap. A gap of 0 means "no cap".
func DefaultBlockLength(size Size, gap time.Duration, sizes map[Size]time.Duration, unsized time.Duration) time.Duration {
	length, ok := sizes[size]
	if !ok {
		length = unsized
	}
	if gap > 0 && gap < length {
		return gap
	}
	return length
}

// Candidate is a pinned task as shown in Today (D12a).
type Candidate struct {
	Task       Task
	PinnedDays int  // planning days pinned, counting today (D10d)
	Blocked    bool // has at least one block today
	Alert      bool // rollover count reached the configured threshold (D10a)
}

// Plan is one planning day: its candidates and its timeline blocks.
type Plan struct {
	Day        Day
	Committed  bool
	Candidates []Candidate // effective priority first (unset last), then task id
	Blocks     []Block     // by start time
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test ./internal/domain/`
Expected: no gofmt output; `ok  tt/internal/domain`. (The test loads `Europe/Amsterdam` from the system zoneinfo, which macOS ships.)

- [ ] **Step 5: Commit**

```bash
git add internal/domain
git commit -m "feat(domain): planning days, pins, blocks and default block length" -m "Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 4: Planning config

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/planning_test.go`

**Interfaces:**
- Consumes: Plan 1 config (`Config`, `Default`, `Parse`, the `file` struct).
- Produces: `Config.UnsizedBlock time.Duration` (default 30m) and `Config.RolloverAlert int` (default 3), set from an optional `[planning]` TOML table with keys `unsized_block` (positive duration) and `rollover_alert` (≥ 1). Unknown keys stay errors.

```toml
[planning]
unsized_block = "45m"
rollover_alert = 5
```

- [ ] **Step 1: Write the failing tests**

`internal/config/planning_test.go`:

```go
package config

import (
	"strings"
	"testing"
	"time"
)

func TestPlanningDefaults(t *testing.T) {
	cfg := Default()
	if cfg.UnsizedBlock != 30*time.Minute || cfg.RolloverAlert != 3 {
		t.Errorf("planning defaults: unsized %v, alert %d", cfg.UnsizedBlock, cfg.RolloverAlert)
	}
}

func TestParsePlanning(t *testing.T) {
	cfg, err := Parse([]byte("[planning]\nunsized_block = \"45m\"\nrollover_alert = 5\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UnsizedBlock != 45*time.Minute || cfg.RolloverAlert != 5 {
		t.Errorf("planning: unsized %v, alert %d", cfg.UnsizedBlock, cfg.RolloverAlert)
	}
	cases := map[string]string{
		"[planning]\nunsized_block = \"0m\"":   "planning.unsized_block",
		"[planning]\nunsized_block = \"soon\"": "planning.unsized_block",
		"[planning]\nrollover_alert = 0":       "planning.rollover_alert",
		"[planning]\nresize = \"30m\"":         "unknown config key",
	}
	for input, wantErr := range cases {
		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/`
Expected: FAIL to build: `cfg.UnsizedBlock undefined`

- [ ] **Step 3: Apply the config changes**

```bash
git apply <<'PATCH'
diff --git a/internal/config/config.go b/internal/config/config.go
index 74b5050..ffd9c5c 100644
--- a/internal/config/config.go
+++ b/internal/config/config.go
@@ -23,6 +23,10 @@ type Config struct {
 	DayStart   time.Duration // offset from midnight when a new planning day begins (D19)
 	Priorities []PriorityLevel
 	Sizes      map[domain.Size]time.Duration
+	// UnsizedBlock is the default block length for unsized tasks such as PR reviews (D24).
+	UnsizedBlock time.Duration
+	// RolloverAlert is the rollover count at which planning flags a task (D10a).
+	RolloverAlert int
 }
 
 func Default() Config {
@@ -39,6 +43,8 @@ func Default() Config {
 			domain.SizeL:  8 * time.Hour,
 			domain.SizeXL: 40 * time.Hour,
 		},
+		UnsizedBlock:  30 * time.Minute,
+		RolloverAlert: 3,
 	}
 }
 
@@ -47,6 +53,10 @@ type file struct {
 	DayStart   *string           `toml:"day_start"`
 	Priorities []PriorityLevel   `toml:"priorities"`
 	Sizes      map[string]string `toml:"sizes"`
+	Planning   struct {
+		UnsizedBlock  *string `toml:"unsized_block"`
+		RolloverAlert *int    `toml:"rollover_alert"`
+	} `toml:"planning"`
 }
 
 // Load reads path. A missing file yields Default().
@@ -104,6 +114,19 @@ func Parse(data []byte) (Config, error) {
 		}
 		cfg.Sizes[size] = d
 	}
+	if p := f.Planning.UnsizedBlock; p != nil {
+		d, err := time.ParseDuration(*p)
+		if err != nil || d <= 0 {
+			return Config{}, fmt.Errorf("planning.unsized_block: want a positive duration like \"30m\", got %q", *p)
+		}
+		cfg.UnsizedBlock = d
+	}
+	if p := f.Planning.RolloverAlert; p != nil {
+		if *p < 1 {
+			return Config{}, fmt.Errorf("planning.rollover_alert: want 1 or more, got %d", *p)
+		}
+		cfg.RolloverAlert = *p
+	}
 	return cfg, nil
 }
 
PATCH
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test ./internal/config/`
Expected: no gofmt output; `ok  tt/internal/config`

- [ ] **Step 5: Commit**

```bash
git add internal/config
git commit -m "feat(config): planning settings (unsized block, rollover alert)" -m "Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 5: Planning storage

**Files:**
- Create: `internal/store/migrations/002_planning.sql`, `internal/store/planning.go`
- Test: `internal/store/planning_test.go`

**Interfaces:**
- Consumes: Task 1 (`readTx`, `loadTasks`), Task 3 (`domain.Day`, `domain.Pin`, `domain.Block`), Plan 1 store (`Tx`, `formatTime`, `scanner`, `ErrNotFound`; test helpers `openTemp`, `insert`, `sampleTask`).
- Produces (all on `*Tx` unless noted):
  - Pins: `GetPin(ctx, taskID) (domain.Pin, bool, error)`, `PutPin(ctx, domain.Pin) error` (upsert), `DeletePin(ctx, taskID) error` (missing is fine), `DeletePinsOfClosedTasks(ctx) error`, `CountOpenPins(ctx) (int, error)`.
  - Blocks: `InsertBlock(ctx, domain.Block) (int64, error)` (a non-zero `ID` is kept, for undo), `GetBlock(ctx, id) (domain.Block, error)` (`ErrNotFound`), `UpdateBlock(ctx, domain.Block) error`, `DeleteBlock(ctx, id) error` (`ErrNotFound`), `BlocksOn(ctx, day) ([]domain.Block, error)` and `TaskBlocksOn(ctx, taskID, day)` (by start), `OpenTasksBlockedOn(ctx, day) ([]int64, error)`.
  - Days: `HasDay(ctx, day) (bool, error)`, `LatestDayBefore(ctx, day) (domain.Day, bool, error)`, `InsertDay(ctx, day, startedAt time.Time) error` (fails if the day exists: the PK guards `StartDay`), `CommitDay(ctx, day, at) error` (`ErrNotFound` if no such day), `CountDone(ctx, from, to time.Time) (int, error)` (status done, `closed_at` in `[from, to)`).
  - `type PinState struct{ domain.Pin; PinnedDays int }`, `type PlanState struct{ Pins []PinState; Blocks []domain.Block; Committed bool }`, and `func (s *Store) LoadPlan(ctx, day) (domain.Snapshot, PlanState, error)` (one read transaction).
  - Deleting a task cascades to its pin and blocks.

- [ ] **Step 1: Write the failing tests**

`internal/store/planning_test.go`:

```go
package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"tt/internal/domain"
)

func inTx(t *testing.T, s *Store, fn func(ctx context.Context, x *Tx) error) {
	t.Helper()
	ctx := context.Background()
	if err := s.InTx(ctx, func(x *Tx) error { return fn(ctx, x) }); err != nil {
		t.Fatal(err)
	}
}

func TestPins(t *testing.T) {
	s, _ := openTemp(t)
	id := insert(t, s, sampleTask())
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		if _, ok, _ := x.GetPin(ctx, id); ok {
			t.Error("new task is pinned")
		}
		if err := x.PutPin(ctx, domain.Pin{TaskID: id, PinnedOn: "2026-10-12"}); err != nil {
			return err
		}
		if err := x.PutPin(ctx, domain.Pin{TaskID: id, PinnedOn: "2026-10-13"}); err != nil { // upsert
			return err
		}
		p, ok, err := x.GetPin(ctx, id)
		if err != nil || !ok || p.PinnedOn != "2026-10-13" {
			t.Errorf("GetPin = %+v, %v, %v; want pinned on 2026-10-13", p, ok, err)
		}
		if err := x.DeletePin(ctx, id); err != nil {
			return err
		}
		if _, ok, _ := x.GetPin(ctx, id); ok {
			t.Error("pin survived DeletePin")
		}
		return x.DeletePin(ctx, id) // deleting a missing pin is fine
	})
}

func TestBlocks(t *testing.T) {
	s, _ := openTemp(t)
	id := insert(t, s, sampleTask())
	b := domain.Block{TaskID: id, Day: "2026-10-12", Start: 11 * time.Hour, Duration: 90 * time.Minute}
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		var err error
		if b.ID, err = x.InsertBlock(ctx, b); err != nil {
			return err
		}
		early := domain.Block{TaskID: id, Day: b.Day, Start: 9 * time.Hour, Duration: time.Hour}
		if early.ID, err = x.InsertBlock(ctx, early); err != nil {
			return err
		}
		if _, err := x.InsertBlock(ctx, domain.Block{TaskID: id, Day: "2026-10-13", Start: 9 * time.Hour, Duration: time.Hour}); err != nil {
			return err
		}

		got, err := x.BlocksOn(ctx, b.Day)
		if err != nil {
			return err
		}
		if diff := cmp.Diff([]domain.Block{early, b}, got); diff != "" {
			t.Errorf("BlocksOn (-want +got):\n%s", diff)
		}

		b.Start, b.Duration = 25*time.Hour, 30*time.Minute // after midnight
		if err := x.UpdateBlock(ctx, b); err != nil {
			return err
		}
		if got, _ := x.GetBlock(ctx, b.ID); got != b {
			t.Errorf("after update: %+v; want %+v", got, b)
		}

		if err := x.DeleteBlock(ctx, b.ID); err != nil {
			return err
		}
		if _, err := x.GetBlock(ctx, b.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("GetBlock after delete: %v; want ErrNotFound", err)
		}
		if err := x.DeleteBlock(ctx, b.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("second DeleteBlock: %v; want ErrNotFound", err)
		}

		// Undo restores a block under its old id.
		if restored, err := x.InsertBlock(ctx, b); err != nil || restored != b.ID {
			t.Errorf("InsertBlock with id = %d, %v; want %d", restored, err, b.ID)
		}
		return nil
	})
}

func TestDeletingTaskRemovesItsPlanning(t *testing.T) {
	s, _ := openTemp(t)
	id := insert(t, s, sampleTask())
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		if err := x.PutPin(ctx, domain.Pin{TaskID: id, PinnedOn: "2026-10-12"}); err != nil {
			return err
		}
		if _, err := x.InsertBlock(ctx, domain.Block{TaskID: id, Day: "2026-10-12", Start: 9 * time.Hour, Duration: time.Hour}); err != nil {
			return err
		}
		return x.DeleteTask(ctx, id)
	})
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		if _, ok, _ := x.GetPin(ctx, id); ok {
			t.Error("pin survived task delete")
		}
		if blocks, _ := x.BlocksOn(ctx, "2026-10-12"); len(blocks) != 0 {
			t.Errorf("blocks survived task delete: %v", blocks)
		}
		return nil
	})
}

func TestDays(t *testing.T) {
	s, _ := openTemp(t)
	at := time.Date(2026, 10, 12, 4, 0, 0, 0, time.UTC)
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		if _, ok, _ := x.LatestDayBefore(ctx, "2026-10-12"); ok {
			t.Error("LatestDayBefore on empty days found something")
		}
		for _, d := range []domain.Day{"2026-10-09", "2026-10-12"} {
			if err := x.InsertDay(ctx, d, at); err != nil {
				return err
			}
		}
		if err := x.InsertDay(ctx, "2026-10-12", at); err == nil {
			t.Error("InsertDay twice for one day succeeded; the PK must guard StartDay races")
		}
		if ok, _ := x.HasDay(ctx, "2026-10-12"); !ok {
			t.Error("HasDay(2026-10-12) = false")
		}
		if d, ok, _ := x.LatestDayBefore(ctx, "2026-10-12"); !ok || d != "2026-10-09" {
			t.Errorf("LatestDayBefore = %q, %v; want 2026-10-09 (skips the weekend)", d, ok)
		}
		if err := x.CommitDay(ctx, "2026-10-12", at); err != nil {
			return err
		}
		if err := x.CommitDay(ctx, "2026-10-13", at); !errors.Is(err, ErrNotFound) {
			t.Errorf("CommitDay on a missing day: %v; want ErrNotFound", err)
		}
		return nil
	})
}

func TestDayQueries(t *testing.T) {
	s, _ := openTemp(t)
	closedAt := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	open := insert(t, s, sampleTask())
	done := sampleTask()
	done.Status, done.ClosedAt, done.ClosedBy = domain.StatusDone, closedAt, domain.ClosedByUser
	doneID := insert(t, s, done)
	dropped := sampleTask()
	dropped.Status, dropped.ClosedAt, dropped.ClosedBy = domain.StatusDropped, closedAt, domain.ClosedByUser
	droppedID := insert(t, s, dropped)

	inTx(t, s, func(ctx context.Context, x *Tx) error {
		for _, id := range []int64{open, doneID, droppedID} {
			if err := x.PutPin(ctx, domain.Pin{TaskID: id, PinnedOn: "2026-10-12"}); err != nil {
				return err
			}
			if _, err := x.InsertBlock(ctx, domain.Block{TaskID: id, Day: "2026-10-12", Start: 9 * time.Hour, Duration: time.Hour}); err != nil {
				return err
			}
		}
		if ids, _ := x.OpenTasksBlockedOn(ctx, "2026-10-12"); !cmp.Equal(ids, []int64{open}) {
			t.Errorf("OpenTasksBlockedOn = %v; want [%d]", ids, open)
		}
		from, to := closedAt.Add(-time.Hour), closedAt.Add(time.Hour)
		if n, _ := x.CountDone(ctx, from, to); n != 1 {
			t.Errorf("CountDone in window = %d; want 1 (dropped doesn't count)", n)
		}
		if n, _ := x.CountDone(ctx, to, to.Add(time.Hour)); n != 0 {
			t.Errorf("CountDone outside window = %d; want 0", n)
		}
		if n, _ := x.CountDone(ctx, closedAt, to); n != 1 {
			t.Errorf("CountDone with from == closed_at = %d; want 1 (from is inclusive)", n)
		}
		if err := x.DeletePinsOfClosedTasks(ctx); err != nil {
			return err
		}
		if n, _ := x.CountOpenPins(ctx); n != 1 {
			t.Errorf("CountOpenPins = %d; want 1", n)
		}
		if _, ok, _ := x.GetPin(ctx, doneID); ok {
			t.Error("done task still pinned")
		}
		return nil
	})
}

func TestLoadPlan(t *testing.T) {
	s, _ := openTemp(t)
	a := insert(t, s, sampleTask())
	b := insert(t, s, sampleTask())
	at := time.Date(2026, 10, 12, 4, 0, 0, 0, time.UTC)
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		for _, d := range []domain.Day{"2026-10-08", "2026-10-09", "2026-10-12"} {
			if err := x.InsertDay(ctx, d, at); err != nil {
				return err
			}
		}
		if err := x.PutPin(ctx, domain.Pin{TaskID: a, PinnedOn: "2026-10-09"}); err != nil { // Fri, Mon → 2
			return err
		}
		if err := x.PutPin(ctx, domain.Pin{TaskID: b, PinnedOn: "2026-10-12"}); err != nil { // today → 1
			return err
		}
		if _, err := x.InsertBlock(ctx, domain.Block{TaskID: b, Day: "2026-10-12", Start: 10 * time.Hour, Duration: time.Hour}); err != nil {
			return err
		}
		return x.CommitDay(ctx, "2026-10-12", at)
	})

	snap, state, err := s.LoadPlan(context.Background(), "2026-10-12")
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Tasks()) != 2 {
		t.Errorf("snapshot has %d tasks; want 2", len(snap.Tasks()))
	}
	want := []PinState{
		{Pin: domain.Pin{TaskID: a, PinnedOn: "2026-10-09"}, PinnedDays: 2},
		{Pin: domain.Pin{TaskID: b, PinnedOn: "2026-10-12"}, PinnedDays: 1},
	}
	if diff := cmp.Diff(want, state.Pins); diff != "" {
		t.Errorf("pins (-want +got):\n%s", diff)
	}
	if len(state.Blocks) != 1 || state.Blocks[0].TaskID != b || !state.Committed {
		t.Errorf("state = %+v", state)
	}
	if _, state, _ := s.LoadPlan(context.Background(), "2026-10-13"); state.Committed || len(state.Blocks) != 0 {
		t.Errorf("unplanned day state = %+v", state)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/store/`
Expected: FAIL to build: `x.GetPin undefined`, `undefined: PinState`, …

- [ ] **Step 3: Write the migration**

`internal/store/migrations/002_planning.sql`:

```sql
CREATE TABLE pins (
    task_id   INTEGER PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
    pinned_on TEXT    NOT NULL
);

CREATE TABLE blocks (
    id           INTEGER PRIMARY KEY,
    task_id      INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    day          TEXT    NOT NULL,
    start_min    INTEGER NOT NULL CHECK (start_min >= 0),
    duration_min INTEGER NOT NULL CHECK (duration_min > 0)
);

CREATE INDEX blocks_day ON blocks (day, start_min);
CREATE INDEX blocks_task ON blocks (task_id);

CREATE TABLE days (
    day          TEXT PRIMARY KEY,
    started_at   TEXT NOT NULL,
    committed_at TEXT
);
```

- [ ] **Step 4: Implement the planning queries**

`internal/store/planning.go`:

```go
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"tt/internal/domain"
)

// PinState is a pin plus its derived pinned-days counter (D10d).
type PinState struct {
	domain.Pin
	PinnedDays int // planning days in `days` from PinnedOn through the plan's day
}

// PlanState is the planning data for one day.
type PlanState struct {
	Pins      []PinState
	Blocks    []domain.Block // by start
	Committed bool
}

func (x *Tx) GetPin(ctx context.Context, taskID int64) (domain.Pin, bool, error) {
	var on string
	err := x.tx.QueryRowContext(ctx, "SELECT pinned_on FROM pins WHERE task_id = ?", taskID).Scan(&on)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Pin{}, false, nil
	}
	if err != nil {
		return domain.Pin{}, false, err
	}
	return domain.Pin{TaskID: taskID, PinnedOn: domain.Day(on)}, true, nil
}

// PutPin creates or replaces the pin for p.TaskID.
func (x *Tx) PutPin(ctx context.Context, p domain.Pin) error {
	_, err := x.tx.ExecContext(ctx, `INSERT INTO pins (task_id, pinned_on) VALUES (?, ?)
		ON CONFLICT (task_id) DO UPDATE SET pinned_on = excluded.pinned_on`, p.TaskID, string(p.PinnedOn))
	return err
}

// DeletePin removes the pin for taskID, if any.
func (x *Tx) DeletePin(ctx context.Context, taskID int64) error {
	_, err := x.tx.ExecContext(ctx, "DELETE FROM pins WHERE task_id = ?", taskID)
	return err
}

// DeletePinsOfClosedTasks unpins done and dropped tasks (their "done today"
// visibility ends when a new planning day starts).
func (x *Tx) DeletePinsOfClosedTasks(ctx context.Context) error {
	_, err := x.tx.ExecContext(ctx, "DELETE FROM pins WHERE task_id IN (SELECT id FROM tasks WHERE status != 'open')")
	return err
}

func (x *Tx) CountOpenPins(ctx context.Context) (int, error) {
	var n int
	err := x.tx.QueryRowContext(ctx, `SELECT count(*) FROM pins p JOIN tasks t ON t.id = p.task_id
		WHERE t.status = 'open'`).Scan(&n)
	return n, err
}

// InsertBlock inserts b and returns its id. A non-zero b.ID is kept (used by undo).
func (x *Tx) InsertBlock(ctx context.Context, b domain.Block) (int64, error) {
	var id any
	if b.ID != 0 {
		id = b.ID
	}
	res, err := x.tx.ExecContext(ctx, "INSERT INTO blocks (id, task_id, day, start_min, duration_min) VALUES (?, ?, ?, ?, ?)",
		id, b.TaskID, string(b.Day), minutes(b.Start), minutes(b.Duration))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (x *Tx) GetBlock(ctx context.Context, id int64) (domain.Block, error) {
	b, err := scanBlock(x.tx.QueryRowContext(ctx, "SELECT "+blockColumns+" FROM blocks WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Block{}, fmt.Errorf("block #%d: %w", id, ErrNotFound)
	}
	return b, err
}

// UpdateBlock overwrites day, start and duration of block b.ID.
func (x *Tx) UpdateBlock(ctx context.Context, b domain.Block) error {
	res, err := x.tx.ExecContext(ctx, "UPDATE blocks SET day = ?, start_min = ?, duration_min = ? WHERE id = ?",
		string(b.Day), minutes(b.Start), minutes(b.Duration), b.ID)
	if err != nil {
		return err
	}
	return mustAffectBlock(res, b.ID)
}

func (x *Tx) DeleteBlock(ctx context.Context, id int64) error {
	res, err := x.tx.ExecContext(ctx, "DELETE FROM blocks WHERE id = ?", id)
	if err != nil {
		return err
	}
	return mustAffectBlock(res, id)
}

// BlocksOn returns day's blocks ordered by start.
func (x *Tx) BlocksOn(ctx context.Context, day domain.Day) ([]domain.Block, error) {
	return queryBlocks(ctx, x.tx, "WHERE day = ?", string(day))
}

// TaskBlocksOn returns taskID's blocks on day.
func (x *Tx) TaskBlocksOn(ctx context.Context, taskID int64, day domain.Day) ([]domain.Block, error) {
	return queryBlocks(ctx, x.tx, "WHERE task_id = ? AND day = ?", taskID, string(day))
}

// OpenTasksBlockedOn returns ids of open tasks with at least one block on day.
func (x *Tx) OpenTasksBlockedOn(ctx context.Context, day domain.Day) ([]int64, error) {
	rows, err := x.tx.QueryContext(ctx, `SELECT DISTINCT b.task_id FROM blocks b JOIN tasks t ON t.id = b.task_id
		WHERE b.day = ? AND t.status = 'open' ORDER BY b.task_id`, string(day))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (x *Tx) HasDay(ctx context.Context, day domain.Day) (bool, error) {
	var n int
	err := x.tx.QueryRowContext(ctx, "SELECT count(*) FROM days WHERE day = ?", string(day)).Scan(&n)
	return n > 0, err
}

// LatestDayBefore returns the most recent planning day before day, if any.
func (x *Tx) LatestDayBefore(ctx context.Context, day domain.Day) (domain.Day, bool, error) {
	var d sql.NullString
	err := x.tx.QueryRowContext(ctx, "SELECT max(day) FROM days WHERE day < ?", string(day)).Scan(&d)
	if err != nil || !d.Valid {
		return "", false, err
	}
	return domain.Day(d.String), true, nil
}

func (x *Tx) InsertDay(ctx context.Context, day domain.Day, startedAt time.Time) error {
	_, err := x.tx.ExecContext(ctx, "INSERT INTO days (day, started_at) VALUES (?, ?)", string(day), formatTime(startedAt))
	return err
}

// CommitDay records that day's plan was committed at at.
func (x *Tx) CommitDay(ctx context.Context, day domain.Day, at time.Time) error {
	res, err := x.tx.ExecContext(ctx, "UPDATE days SET committed_at = ? WHERE day = ?", formatTime(at), string(day))
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("day %s: %w", day, ErrNotFound)
	}
	return nil
}

// CountDone counts tasks marked done with closed_at in [from, to).
func (x *Tx) CountDone(ctx context.Context, from, to time.Time) (int, error) {
	var n int
	err := x.tx.QueryRowContext(ctx, "SELECT count(*) FROM tasks WHERE status = 'done' AND closed_at >= ? AND closed_at < ?",
		formatTime(from), formatTime(to)).Scan(&n)
	return n, err
}

// LoadPlan reads all tasks and day's planning state in one consistent read.
func (s *Store) LoadPlan(ctx context.Context, day domain.Day) (domain.Snapshot, PlanState, error) {
	var (
		snap  domain.Snapshot
		state PlanState
	)
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		tasks, err := loadTasks(ctx, tx)
		if err != nil {
			return err
		}
		snap = domain.NewSnapshot(tasks)

		rows, err := tx.QueryContext(ctx, `SELECT p.task_id, p.pinned_on,
			(SELECT count(*) FROM days d WHERE d.day >= p.pinned_on AND d.day <= ?)
			FROM pins p ORDER BY p.task_id`, string(day))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p PinState
			var on string
			if err := rows.Scan(&p.TaskID, &on, &p.PinnedDays); err != nil {
				return err
			}
			p.PinnedOn = domain.Day(on)
			state.Pins = append(state.Pins, p)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		if state.Blocks, err = queryBlocks(ctx, tx, "WHERE day = ?", string(day)); err != nil {
			return err
		}
		var committed sql.NullString
		err = tx.QueryRowContext(ctx, "SELECT committed_at FROM days WHERE day = ?", string(day)).Scan(&committed)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		state.Committed = committed.Valid
		return nil
	})
	return snap, state, err
}

const blockColumns = "id, task_id, day, start_min, duration_min"

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func queryBlocks(ctx context.Context, q querier, where string, args ...any) ([]domain.Block, error) {
	rows, err := q.QueryContext(ctx, "SELECT "+blockColumns+" FROM blocks "+where+" ORDER BY start_min, id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var blocks []domain.Block
	for rows.Next() {
		b, err := scanBlock(rows)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, b)
	}
	return blocks, rows.Err()
}

func scanBlock(row scanner) (domain.Block, error) {
	var (
		b                domain.Block
		day              string
		startMin, durMin int64
	)
	if err := row.Scan(&b.ID, &b.TaskID, &day, &startMin, &durMin); err != nil {
		return domain.Block{}, err
	}
	b.Day = domain.Day(day)
	b.Start = time.Duration(startMin) * time.Minute
	b.Duration = time.Duration(durMin) * time.Minute
	return b, nil
}

func minutes(d time.Duration) int64 { return int64(d / time.Minute) }

func mustAffectBlock(res sql.Result, id int64) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("block #%d: %w", id, ErrNotFound)
	}
	return nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test ./internal/store/`
Expected: no gofmt output; `ok  tt/internal/store`

- [ ] **Step 6: Commit**

```bash
git add internal/store
git commit -m "feat(store): pins, blocks and planning days" -m "Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 6: App planning engine

**Files:**
- Modify: `internal/app/service.go` (one line in the `Store` interface)
- Create: `internal/app/planning.go`, `internal/app/day.go`
- Test: `internal/app/planning_test.go`, `internal/app/day_test.go`

**Interfaces:**
- Consumes: Task 2 (`ErrConflict`, compare-and-set undo), Task 3 (all planning types), Task 4 (`cfg.UnsizedBlock`, `cfg.RolloverAlert`, `cfg.DayStart`, `cfg.Sizes`), Task 5 (all `Tx` planning methods, `store.PlanState`, `Store.LoadPlan`); Plan 1 (`ErrInvalidValue`, `ErrNotFound`, `mustAdd`, `mustGet`, `mustUndo`, `newService`, `fakeClock`).
- Produces:
  - `app.Store` interface gains `LoadPlan(ctx, domain.Day) (domain.Snapshot, store.PlanState, error)`.
  - `var ErrOverlap error`.
  - `(*Service) Today() domain.Day`; `DefaultBlockLength(domain.Task, gap time.Duration) time.Duration`.
  - `Pin(ctx, id) (Undo, error)` (open tasks only; already pinned → no change); `Unpin(ctx, id) (Undo, error)` (also removes the task's blocks today; unpinned/missing → no-op).
  - `Block(ctx, id, start, dur time.Duration) (domain.Block, Undo, error)` (today only; pins the task with `pinned_on` = today; undo restores the previous pin state); `MoveBlock(ctx, blockID, start) (Undo, error)`; `ResizeBlock(ctx, blockID, dur) (Undo, error)`; `Unblock(ctx, blockID) (Undo, error)` (task stays pinned). Move/resize/unblock refuse blocks from other days with `ErrInvalidValue`.
  - `type Recap struct{ Since domain.Day; Done, RolledOver, Carried int }`, `type DayResult struct{ Day domain.Day; New bool; Recap *Recap }`, `StartDay(ctx) (DayResult, error)`.
  - `type CommitSummary struct{ Blocks int; Blocked time.Duration; Candidates int }`, `CommitPlan(ctx) (CommitSummary, error)` (starts the day if needed).
  - `Plan(ctx) (domain.Plan, error)`: today's candidates (closed tasks included until the next day), sorted by effective priority with unset last, then id. `Alert` = rollover ≥ `cfg.RolloverAlert`.

`StartDay` algorithm, all in one write transaction:
1. `today = PlanningDay(now, day_start)`. If `days` has today → return `{Day: today, New: false}`.
2. `prev` = latest planning day before today, if any. For each open task blocked on `prev`: `RolloverCount++`, `UpdatedAt = now` (so older undos conflict), pin with `pinned_on = today`. `Done` = tasks done in `[prev.At(day_start), today.At(day_start))` (local time).
3. Unpin closed tasks; `Carried` = open pins left.
4. Insert `days(today)`.

`BEGIN IMMEDIATE` serializes concurrent callers, so a second `StartDay` (TUI and `tt` at the boundary) waits, then sees today's row in step 1 and changes nothing. The `days` primary key is the backstop.

- [ ] **Step 1: Write the failing tests**

`internal/app/planning_test.go`:

```go
package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"tt/internal/domain"
)

const (
	h = time.Hour
	m = time.Minute
)

// startedService is newService with today's planning day already started.
func startedService(t *testing.T) (*Service, *fakeClock) {
	t.Helper()
	s, clock := newService(t)
	if _, err := s.StartDay(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s, clock
}

func mustPlan(t *testing.T, s *Service) domain.Plan {
	t.Helper()
	plan, err := s.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func candidate(t *testing.T, plan domain.Plan, id int64) (domain.Candidate, bool) {
	t.Helper()
	for _, c := range plan.Candidates {
		if c.Task.ID == id {
			return c, true
		}
	}
	return domain.Candidate{}, false
}

func mustBlock(t *testing.T, s *Service, id int64, start, dur time.Duration) (domain.Block, Undo) {
	t.Helper()
	b, undo, err := s.Block(context.Background(), id, start, dur)
	if err != nil {
		t.Fatalf("Block(#%d, %v, %v): %v", id, start, dur, err)
	}
	return b, undo
}

func TestTodayUsesDayStart(t *testing.T) {
	s, clock := newService(t)
	if got := s.Today(); got != "2026-10-12" {
		t.Errorf("Today at 09:00 = %s", got)
	}
	clock.now = time.Date(2026, 10, 13, 3, 0, 0, 0, time.UTC)
	if got := s.Today(); got != "2026-10-12" {
		t.Errorf("Today at 03:00 next day = %s; want still 2026-10-12", got)
	}
}

func TestPinAndUnpin(t *testing.T) {
	s, _ := startedService(t)
	must := mustUndo(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Pin me !1 ~s")

	pinUndo := must(s.Pin(ctx, task.ID))
	c, ok := candidate(t, mustPlan(t, s), task.ID)
	if !ok || c.PinnedDays != 1 || c.Blocked {
		t.Fatalf("after Pin: %+v, %v", c, ok)
	}
	again := must(s.Pin(ctx, task.ID)) // already pinned: no change
	if err := again(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := candidate(t, mustPlan(t, s), task.ID); !ok {
		t.Error("undoing a no-op Pin must not unpin")
	}
	if err := pinUndo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := candidate(t, mustPlan(t, s), task.ID); ok {
		t.Error("undo of Pin should unpin")
	}

	must(s.Pin(ctx, task.ID))
	b, _ := mustBlock(t, s, task.ID, 10*h, h)
	unpinUndo := must(s.Unpin(ctx, task.ID))
	plan := mustPlan(t, s)
	if _, ok := candidate(t, plan, task.ID); ok || len(plan.Blocks) != 0 {
		t.Errorf("Unpin should remove the task and today's blocks: %+v", plan)
	}
	if err := unpinUndo(ctx); err != nil {
		t.Fatal(err)
	}
	plan = mustPlan(t, s)
	if _, ok := candidate(t, plan, task.ID); !ok || len(plan.Blocks) != 1 || plan.Blocks[0] != b {
		t.Errorf("undo of Unpin should restore pin and block %+v: %+v", b, plan)
	}

	must(s.Unpin(ctx, 999)) // unpinning an unpinned or missing task is a no-op

	if _, err := s.Done(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	other := mustAdd(t, s, "Closed")
	if _, err := s.Drop(ctx, other.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pin(ctx, other.ID); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("Pin on a dropped task: %v; want ErrInvalidValue", err)
	}
	if _, err := s.Pin(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("Pin on a missing task: %v; want ErrNotFound", err)
	}
}

func TestBlock(t *testing.T) {
	s, _ := startedService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Block me !1 ~m")

	b, undo := mustBlock(t, s, task.ID, 11*h, 90*m)
	if b.ID == 0 || b.Day != "2026-10-12" || b.Start != 11*h || b.Duration != 90*m {
		t.Errorf("block = %+v", b)
	}
	c, ok := candidate(t, mustPlan(t, s), task.ID)
	if !ok || !c.Blocked {
		t.Errorf("blocking pins the task and marks it blocked: %+v, %v", c, ok)
	}
	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if plan := mustPlan(t, s); len(plan.Blocks) != 0 || len(plan.Candidates) != 0 {
		t.Errorf("undo of Block on an unpinned task should leave it unpinned: %+v", plan)
	}

	mustBlock(t, s, task.ID, 11*h, h)
	mustBlock(t, s, task.ID, 14*h, h) // a task may have several blocks
	mustBlock(t, s, task.ID, 25*h, h) // 01:00 after midnight is still today (day start 04:00)

	other := mustAdd(t, s, "Other")
	bad := []struct {
		name       string
		start, dur time.Duration
		want       error
	}{
		{"overlap", 11*h + 30*m, h, ErrOverlap},
		{"overlap containing", 10 * h, 6 * h, ErrOverlap},
		{"before day start", 3 * h, h, ErrInvalidValue},
		{"past next day start", 27*h + 30*m, h, ErrInvalidValue},
		{"zero length", 9 * h, 0, ErrInvalidValue},
		{"seconds", 9*h + 30*time.Second, h, ErrInvalidValue},
	}
	for _, c := range bad {
		if _, _, err := s.Block(ctx, other.ID, c.start, c.dur); !errors.Is(err, c.want) {
			t.Errorf("%s: error = %v; want %v", c.name, err, c.want)
		}
	}
	if _, _, err := s.Block(ctx, other.ID, 12*h, h); err != nil {
		t.Errorf("touching blocks are fine: %v", err)
	}
	if _, _, err := s.Block(ctx, other.ID, 4*h, 24*h-5*h); !errors.Is(err, ErrOverlap) {
		t.Errorf("long block over existing ones: %v; want ErrOverlap", err)
	}

	if _, err := s.Done(ctx, other.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Block(ctx, other.ID, 16*h, h); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("Block on a done task: %v; want ErrInvalidValue", err)
	}
}

func TestBlockResetsPinnedDaysAndUndoRestoresPin(t *testing.T) {
	s, clock := startedService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Long-pinned !2 ~s")
	if _, err := s.Pin(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	clock.Advance(24 * h)
	if _, err := s.StartDay(ctx); err != nil {
		t.Fatal(err)
	}
	if c, _ := candidate(t, mustPlan(t, s), task.ID); c.PinnedDays != 2 {
		t.Fatalf("pinned days after two planning days = %d; want 2", c.PinnedDays)
	}

	_, undo := mustBlock(t, s, task.ID, 10*h, h)
	if c, _ := candidate(t, mustPlan(t, s), task.ID); c.PinnedDays != 1 {
		t.Errorf("pinned days after Block = %d; want 1 (reset, D10d)", c.PinnedDays)
	}
	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if c, ok := candidate(t, mustPlan(t, s), task.ID); !ok || c.PinnedDays != 2 {
		t.Errorf("undo of Block should restore the old pin: %+v, %v", c, ok)
	}
}

func TestMoveResizeUnblock(t *testing.T) {
	s, clock := startedService(t)
	must := mustUndo(t)
	ctx := context.Background()
	a := mustAdd(t, s, "A !1 ~s")
	bTask := mustAdd(t, s, "B !1 ~s")
	ba, _ := mustBlock(t, s, a.ID, 10*h, h)
	bb, _ := mustBlock(t, s, bTask.ID, 12*h, h)

	if _, err := s.MoveBlock(ctx, ba.ID, 11*h+30*m); !errors.Is(err, ErrOverlap) {
		t.Errorf("move into another block: %v; want ErrOverlap", err)
	}
	if _, err := s.MoveBlock(ctx, ba.ID, 10*h+30*m); err != nil {
		t.Errorf("move overlapping only itself: %v", err)
	}
	moveUndo := must(s.MoveBlock(ctx, ba.ID, 15*h))
	resizeUndo := must(s.ResizeBlock(ctx, ba.ID, 2*h))
	if got := mustPlan(t, s).Blocks; got[1].Start != 15*h || got[1].Duration != 2*h {
		t.Errorf("after move+resize: %+v", got)
	}
	if _, err := s.ResizeBlock(ctx, bb.ID, 4*h); !errors.Is(err, ErrOverlap) {
		t.Errorf("resize into another block: %v; want ErrOverlap", err)
	}
	if _, err := s.ResizeBlock(ctx, bb.ID, 0); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("resize to zero: %v; want ErrInvalidValue", err)
	}
	if err := resizeUndo(ctx); err != nil {
		t.Fatal(err)
	}
	if err := moveUndo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mustPlan(t, s).Blocks[0]; got.Start != 10*h+30*m || got.Duration != h {
		t.Errorf("after undoing resize and move: %+v", got)
	}

	unblockUndo := must(s.Unblock(ctx, bb.ID))
	plan := mustPlan(t, s)
	if c, ok := candidate(t, plan, bTask.ID); !ok || c.Blocked || len(plan.Blocks) != 1 {
		t.Errorf("Unblock keeps the task as an unblocked candidate: %+v", plan)
	}
	if err := unblockUndo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mustPlan(t, s).Blocks; len(got) != 2 || got[1] != bb {
		t.Errorf("undo of Unblock should restore block %+v: %+v", bb, got)
	}

	if _, err := s.MoveBlock(ctx, 999, 9*h); !errors.Is(err, ErrNotFound) {
		t.Errorf("move missing block: %v; want ErrNotFound", err)
	}
	clock.Advance(24 * h)
	if _, err := s.MoveBlock(ctx, bb.ID, 9*h); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("move yesterday's block: %v; want ErrInvalidValue", err)
	}
	if _, err := s.Unblock(ctx, bb.ID); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("unblock yesterday's block: %v; want ErrInvalidValue", err)
	}
}

func TestServiceDefaultBlockLength(t *testing.T) {
	s, _ := newService(t)
	if got := s.DefaultBlockLength(domain.Task{Size: domain.SizeS}, 3*h); got != h {
		t.Errorf("S in 3h gap = %v; want 1h", got)
	}
	if got := s.DefaultBlockLength(domain.Task{}, 0); got != 30*m {
		t.Errorf("unsized, no cap = %v; want 30m", got)
	}
}
```

`internal/app/day_test.go`:

```go
package app

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"tt/internal/config"
	"tt/internal/store"
)

func mustStartDay(t *testing.T, s *Service) DayResult {
	t.Helper()
	res, err := s.StartDay(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestStartDayFirstRunAndIdempotence(t *testing.T) {
	s, clock := newService(t)
	if res := mustStartDay(t, s); !res.New || res.Day != "2026-10-12" || res.Recap != nil {
		t.Errorf("first StartDay = %+v; want new day, no recap", res)
	}
	if res := mustStartDay(t, s); res.New {
		t.Errorf("second StartDay = %+v; want New false", res)
	}
	clock.now = time.Date(2026, 10, 13, 3, 30, 0, 0, time.UTC) // before day start
	if res := mustStartDay(t, s); res.New {
		t.Errorf("StartDay at 03:30 next date = %+v; still the same planning day", res)
	}
}

func TestStartDayRollsOver(t *testing.T) {
	s, clock := startedService(t)
	ctx := context.Background()
	blocked := mustAdd(t, s, "Blocked, unfinished !1 ~l")
	candidateOnly := mustAdd(t, s, "Candidate only !2 ~s")
	finished := mustAdd(t, s, "Blocked, finished !1 ~s")
	dropped := mustAdd(t, s, "Pinned, dropped !3 ~s")
	mustBlock(t, s, blocked.ID, 10*h, 2*h)
	mustBlock(t, s, finished.ID, 13*h, h)
	for _, id := range []int64{candidateOnly.ID, dropped.ID} {
		if _, err := s.Pin(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	clock.Advance(6 * h) // 15:00
	if _, err := s.Done(ctx, finished.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Drop(ctx, dropped.ID, "not needed"); err != nil {
		t.Fatal(err)
	}
	if c, ok := candidate(t, mustPlan(t, s), finished.ID); !ok || c.Task.Status != "done" {
		t.Errorf("done task stays visible in today's plan until the next day: %+v, %v", c, ok)
	}

	clock.Advance(18 * h) // next day 09:00
	res := mustStartDay(t, s)
	want := &Recap{Since: "2026-10-12", Done: 1, RolledOver: 1, Carried: 2}
	if !res.New || res.Day != "2026-10-13" || !cmp.Equal(res.Recap, want) {
		t.Errorf("StartDay = %+v, recap %+v; want recap %+v", res, res.Recap, want)
	}

	if got := mustGet(t, s, blocked.ID).RolloverCount; got != 1 {
		t.Errorf("blocked task rollover = %d; want 1", got)
	}
	if got := mustGet(t, s, candidateOnly.ID).RolloverCount; got != 0 {
		t.Errorf("candidate rollover = %d; want 0 (candidates never count, D10)", got)
	}
	plan := mustPlan(t, s)
	if c, ok := candidate(t, plan, blocked.ID); !ok || c.PinnedDays != 1 || c.Blocked {
		t.Errorf("rolled-over task is carried as an unblocked candidate, pinned-days reset: %+v, %v", c, ok)
	}
	if c, ok := candidate(t, plan, candidateOnly.ID); !ok || c.PinnedDays != 2 {
		t.Errorf("candidate is carried with pinned days 2: %+v, %v", c, ok)
	}
	for _, id := range []int64{finished.ID, dropped.ID} {
		if _, ok := candidate(t, plan, id); ok {
			t.Errorf("closed task #%d should be unpinned on the new day", id)
		}
	}
	if len(plan.Blocks) != 0 {
		t.Errorf("a new day starts with an empty timeline: %+v", plan.Blocks)
	}
}

func TestStartDayCountsPlanningDaysNotCalendarDays(t *testing.T) {
	s, clock := startedService(t) // Monday 2026-10-12
	ctx := context.Background()
	blocked := mustAdd(t, s, "Blocked !1 ~s")
	pinned := mustAdd(t, s, "Pinned !1 ~s")
	mustBlock(t, s, blocked.ID, 10*h, h)
	if _, err := s.Pin(ctx, pinned.ID); err != nil {
		t.Fatal(err)
	}

	clock.Advance(4 * 24 * h) // Friday; the app wasn't used Tue–Thu
	res := mustStartDay(t, s)
	if res.Recap == nil || res.Recap.Since != "2026-10-12" {
		t.Errorf("recap = %+v; want since Monday", res.Recap)
	}
	if got := mustGet(t, s, blocked.ID).RolloverCount; got != 1 {
		t.Errorf("rollover after a 4-day gap = %d; want 1 (D10b)", got)
	}
	if c, _ := candidate(t, mustPlan(t, s), pinned.ID); c.PinnedDays != 2 {
		t.Errorf("pinned days after a 4-day gap = %d; want 2", c.PinnedDays)
	}
}

func TestStartDayRaceCountsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tt.db")
	clock := &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
	open := func() *Service {
		st, err := store.Open(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { st.Close() })
		return New(st, clock, config.Default())
	}
	tui, cli := open(), open()
	mustStartDay(t, tui)
	task := mustAdd(t, tui, "Blocked !1 ~s")
	mustBlock(t, tui, task.ID, 10*h, h)
	clock.Advance(24 * h)

	var wg sync.WaitGroup
	results := make([]DayResult, 2)
	for i, s := range []*Service{tui, cli} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var err error
			if results[i], err = s.StartDay(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if results[0].New == results[1].New {
		t.Errorf("exactly one racer should start the day: %+v", results)
	}
	if got := mustGet(t, tui, task.ID).RolloverCount; got != 1 {
		t.Errorf("rollover after racing StartDay = %d; want 1", got)
	}
}

func TestUndoAfterRolloverConflicts(t *testing.T) {
	s, clock := startedService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Edit me !1 ~s")
	mustBlock(t, s, task.ID, 10*h, h)
	clock.Advance(time.Hour)
	editUndo, err := s.Edit(ctx, task.ID, "Edited !1 ~s")
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(24 * h)
	mustStartDay(t, s) // rollover writes the task

	if err := editUndo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo across a rollover: %v; want ErrConflict", err)
	}
	if got := mustGet(t, s, task.ID); got.RolloverCount != 1 || got.Title != "Edited" {
		t.Errorf("the rollover must survive: %+v", got)
	}
}

func TestCommitPlan(t *testing.T) {
	s, _ := newService(t) // day not started yet: CommitPlan starts it
	ctx := context.Background()
	a := mustAdd(t, s, "A !1 ~s")
	b := mustAdd(t, s, "B !2 ~s")
	c := mustAdd(t, s, "C !3 ~s")
	mustBlock(t, s, a.ID, 10*h, 90*m)
	mustBlock(t, s, a.ID, 14*h, 30*m)
	for _, id := range []int64{b.ID, c.ID} {
		if _, err := s.Pin(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Done(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if mustPlan(t, s).Committed {
		t.Error("plan committed before CommitPlan")
	}
	sum, err := s.CommitPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := CommitSummary{Blocks: 2, Blocked: 2 * h, Candidates: 1}
	if sum != want {
		t.Errorf("summary = %+v; want %+v", sum, want)
	}
	if !mustPlan(t, s).Committed {
		t.Error("plan not committed after CommitPlan")
	}
	if _, err := s.CommitPlan(ctx); err != nil {
		t.Errorf("committing again: %v", err)
	}
}

func TestPlanOrderAndAlert(t *testing.T) {
	s, clock := startedService(t)
	s.cfg.RolloverAlert = 1
	ctx := context.Background()
	unset := mustAdd(t, s, "No priority")
	p2 := mustAdd(t, s, "Second !2 ~s")
	p1 := mustAdd(t, s, "First !1 ~s")
	parent := mustAdd(t, s, "Parent !1 ~xl")
	child := mustAdd(t, s, "Child ~s")
	if _, err := s.SetParent(ctx, child.ID, parent.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{unset.ID, p2.ID, child.ID} {
		if _, err := s.Pin(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	mustBlock(t, s, p1.ID, 10*h, h)
	clock.Advance(24 * h)
	mustStartDay(t, s) // p1 rolls over once → alert at threshold 1

	var ids []int64
	var alerts []bool
	for _, c := range mustPlan(t, s).Candidates {
		ids = append(ids, c.Task.ID)
		alerts = append(alerts, c.Alert)
	}
	// child inherits P1 from its parent, so it sorts with p1 (by id).
	if diff := cmp.Diff([]int64{p1.ID, child.ID, p2.ID, unset.ID}, ids); diff != "" {
		t.Errorf("candidate order (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]bool{true, false, false, false}, alerts); diff != "" {
		t.Errorf("alerts (-want +got):\n%s", diff)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/`
Expected: FAIL to build: `s.StartDay undefined`, `s.Block undefined`, …

- [ ] **Step 3: Extend the Store interface**

```bash
git apply <<'PATCH'
diff --git a/internal/app/service.go b/internal/app/service.go
index 026157e..db423bb 100644
--- a/internal/app/service.go
+++ b/internal/app/service.go
@@ -23,6 +23,7 @@ func (SystemClock) Now() time.Time { return time.Now() }
 type Store interface {
 	InTx(ctx context.Context, fn func(*store.Tx) error) error
 	LoadSnapshot(ctx context.Context) (domain.Snapshot, error)
+	LoadPlan(ctx context.Context, day domain.Day) (domain.Snapshot, store.PlanState, error)
 	DataVersion(ctx context.Context) (int64, error)
 }
 
PATCH
```

- [ ] **Step 4: Implement the planning commands**

`internal/app/planning.go`:

```go
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"tt/internal/domain"
	"tt/internal/store"
)

var ErrOverlap = errors.New("overlaps another block")

// Today is the current planning day (D19).
func (s *Service) Today() domain.Day {
	return domain.PlanningDay(s.clock.Now(), s.cfg.DayStart)
}

// DefaultBlockLength is the block length for placing t into a gap (D24);
// gap 0 means no cap.
func (s *Service) DefaultBlockLength(t domain.Task, gap time.Duration) time.Duration {
	return domain.DefaultBlockLength(t.Size, gap, s.cfg.Sizes, s.cfg.UnsizedBlock)
}

// Pin makes an open task one of today's candidates. Pinning a pinned task
// changes nothing.
func (s *Service) Pin(ctx context.Context, id int64) (Undo, error) {
	today := s.Today()
	var had bool
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		if err := mustBeOpen(ctx, x, id); err != nil {
			return err
		}
		var err error
		if _, had, err = x.GetPin(ctx, id); err != nil || had {
			return err
		}
		return x.PutPin(ctx, domain.Pin{TaskID: id, PinnedOn: today})
	})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) error {
		if had {
			return nil
		}
		return s.store.InTx(ctx, func(x *store.Tx) error { return x.DeletePin(ctx, id) })
	}, nil
}

// Unpin removes a task from Today, together with its blocks on today's timeline.
func (s *Service) Unpin(ctx context.Context, id int64) (Undo, error) {
	today := s.Today()
	var (
		pin    domain.Pin
		had    bool
		blocks []domain.Block
	)
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		var err error
		if pin, had, err = x.GetPin(ctx, id); err != nil || !had {
			return err
		}
		if blocks, err = x.TaskBlocksOn(ctx, id, today); err != nil {
			return err
		}
		for _, b := range blocks {
			if err := x.DeleteBlock(ctx, b.ID); err != nil {
				return err
			}
		}
		return x.DeletePin(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) error {
		if !had {
			return nil
		}
		return s.store.InTx(ctx, func(x *store.Tx) error {
			if err := x.PutPin(ctx, pin); err != nil {
				return err
			}
			for _, b := range blocks {
				if _, err := x.InsertBlock(ctx, b); err != nil {
					return err
				}
			}
			return nil
		})
	}, nil
}

// Block places an open task on today's timeline at wall-clock offset start
// (from midnight) for dur. It pins the task and resets its pinned-days
// counter (D10d). Blocks may not overlap each other; meetings are not checked
// here (D18 conflicts are shown, not prevented).
func (s *Service) Block(ctx context.Context, id int64, start, dur time.Duration) (domain.Block, Undo, error) {
	b := domain.Block{TaskID: id, Day: s.Today(), Start: start, Duration: dur}
	if err := s.validateSpan(b); err != nil {
		return domain.Block{}, nil, err
	}
	var (
		pin domain.Pin
		had bool
	)
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		if err := mustBeOpen(ctx, x, id); err != nil {
			return err
		}
		if err := checkFree(ctx, x, b); err != nil {
			return err
		}
		var err error
		if pin, had, err = x.GetPin(ctx, id); err != nil {
			return err
		}
		if err := x.PutPin(ctx, domain.Pin{TaskID: id, PinnedOn: b.Day}); err != nil {
			return err
		}
		b.ID, err = x.InsertBlock(ctx, b)
		return err
	})
	if err != nil {
		return domain.Block{}, nil, err
	}
	return b, func(ctx context.Context) error {
		return s.store.InTx(ctx, func(x *store.Tx) error {
			if err := x.DeleteBlock(ctx, b.ID); err != nil {
				return err
			}
			if had {
				return x.PutPin(ctx, pin)
			}
			return x.DeletePin(ctx, id)
		})
	}, nil
}

// MoveBlock moves one of today's blocks to a new start.
func (s *Service) MoveBlock(ctx context.Context, blockID int64, start time.Duration) (Undo, error) {
	return s.reshape(ctx, blockID, func(b *domain.Block) { b.Start = start })
}

// ResizeBlock changes the length of one of today's blocks.
func (s *Service) ResizeBlock(ctx context.Context, blockID int64, dur time.Duration) (Undo, error) {
	return s.reshape(ctx, blockID, func(b *domain.Block) { b.Duration = dur })
}

// Unblock removes one of today's blocks; the task stays pinned as a candidate.
func (s *Service) Unblock(ctx context.Context, blockID int64) (Undo, error) {
	var old domain.Block
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		var err error
		if old, err = s.todaysBlock(ctx, x, blockID); err != nil {
			return err
		}
		return x.DeleteBlock(ctx, blockID)
	})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) error {
		return s.store.InTx(ctx, func(x *store.Tx) error {
			_, err := x.InsertBlock(ctx, old)
			return err
		})
	}, nil
}

func (s *Service) reshape(ctx context.Context, blockID int64, fn func(*domain.Block)) (Undo, error) {
	var old domain.Block
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		var err error
		if old, err = s.todaysBlock(ctx, x, blockID); err != nil {
			return err
		}
		b := old
		fn(&b)
		if err := s.validateSpan(b); err != nil {
			return err
		}
		if err := checkFree(ctx, x, b); err != nil {
			return err
		}
		return x.UpdateBlock(ctx, b)
	})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) error {
		return s.store.InTx(ctx, func(x *store.Tx) error { return x.UpdateBlock(ctx, old) })
	}, nil
}

// todaysBlock loads a block and refuses blocks from other days: past days are
// rollover history (D10).
func (s *Service) todaysBlock(ctx context.Context, x *store.Tx, id int64) (domain.Block, error) {
	b, err := x.GetBlock(ctx, id)
	if err != nil {
		return domain.Block{}, err
	}
	if today := s.Today(); b.Day != today {
		return domain.Block{}, fmt.Errorf("block #%d is on %s, not today (%s): %w", id, b.Day, today, ErrInvalidValue)
	}
	return b, nil
}

// validateSpan checks that b is whole minutes, at least a minute long, and
// inside the planning day: from day start until the next day start.
func (s *Service) validateSpan(b domain.Block) error {
	switch {
	case b.Start%time.Minute != 0 || b.Duration%time.Minute != 0:
		return fmt.Errorf("block times must be whole minutes: %w", ErrInvalidValue)
	case b.Duration < time.Minute:
		return fmt.Errorf("block length %v: %w", b.Duration, ErrInvalidValue)
	case b.Start < s.cfg.DayStart || b.End() > s.cfg.DayStart+24*time.Hour:
		return fmt.Errorf("block %v–%v is outside the planning day: %w", b.Start, b.End(), ErrInvalidValue)
	}
	return nil
}

// checkFree fails with ErrOverlap if b overlaps another block on its day.
func checkFree(ctx context.Context, x *store.Tx, b domain.Block) error {
	blocks, err := x.BlocksOn(ctx, b.Day)
	if err != nil {
		return err
	}
	for _, o := range blocks {
		if o.ID != b.ID && o.Overlaps(b) {
			return fmt.Errorf("block %v–%v and block #%d: %w", b.Start, b.End(), o.ID, ErrOverlap)
		}
	}
	return nil
}

func mustBeOpen(ctx context.Context, x *store.Tx, id int64) error {
	t, err := x.GetTask(ctx, id)
	if err != nil {
		return err
	}
	if t.Status != domain.StatusOpen {
		return fmt.Errorf("task #%d is %s: %w", id, t.Status, ErrInvalidValue)
	}
	return nil
}
```

- [ ] **Step 5: Implement the day lifecycle and plan query**

`internal/app/day.go`:

```go
package app

import (
	"cmp"
	"context"
	"math"
	"slices"
	"time"

	"tt/internal/domain"
	"tt/internal/store"
)

// Recap summarizes the previous planning day for the new-day banner.
type Recap struct {
	Since      domain.Day // the previous planning day
	Done       int        // tasks marked done since Since began
	RolledOver int        // time-blocked tasks left unfinished (rollover +1 each)
	Carried    int        // open candidates carried into the new day
}

// DayResult is what StartDay did.
type DayResult struct {
	Day   domain.Day
	New   bool   // false: the day had already started; nothing changed
	Recap *Recap // nil on the very first planning day
}

// StartDay begins the current planning day, once (D9, D10, D10b, D19). For the
// previous planning day it increments the rollover counter of every open task
// that had a block (re-pinning it, pinned-days reset) and unpins closed tasks.
// Candidates stay pinned. It runs in one transaction guarded by the days
// primary key, so a TUI and a CLI racing at the boundary count once.
func (s *Service) StartDay(ctx context.Context) (DayResult, error) {
	now := s.clock.Now()
	today := domain.PlanningDay(now, s.cfg.DayStart)
	res := DayResult{Day: today}
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		if started, err := x.HasDay(ctx, today); err != nil || started {
			return err
		}
		res.New = true
		prev, ok, err := x.LatestDayBefore(ctx, today)
		if err != nil {
			return err
		}
		if ok {
			recap := Recap{Since: prev}
			ids, err := x.OpenTasksBlockedOn(ctx, prev)
			if err != nil {
				return err
			}
			for _, id := range ids {
				t, err := x.GetTask(ctx, id)
				if err != nil {
					return err
				}
				t.RolloverCount++
				t.UpdatedAt = now
				if err := x.UpdateTask(ctx, t); err != nil {
					return err
				}
				if err := x.PutPin(ctx, domain.Pin{TaskID: id, PinnedOn: today}); err != nil {
					return err
				}
			}
			recap.RolledOver = len(ids)
			loc := now.Location()
			if recap.Done, err = x.CountDone(ctx, prev.At(s.cfg.DayStart, loc), today.At(s.cfg.DayStart, loc)); err != nil {
				return err
			}
			res.Recap = &recap
		}
		if err := x.DeletePinsOfClosedTasks(ctx); err != nil {
			return err
		}
		if res.Recap != nil {
			if res.Recap.Carried, err = x.CountOpenPins(ctx); err != nil {
				return err
			}
		}
		return x.InsertDay(ctx, today, now)
	})
	if err != nil {
		return DayResult{}, err
	}
	return res, nil
}

// CommitSummary is the finish-line summary of a committed plan (D22).
type CommitSummary struct {
	Blocks     int
	Blocked    time.Duration // total time in blocks
	Candidates int           // open candidates without a block
}

// CommitPlan marks today's plan as committed and summarizes it. It starts the
// day first if needed. Committing again just refreshes the time.
func (s *Service) CommitPlan(ctx context.Context) (CommitSummary, error) {
	if _, err := s.StartDay(ctx); err != nil {
		return CommitSummary{}, err
	}
	today := s.Today()
	err := s.store.InTx(ctx, func(x *store.Tx) error { return x.CommitDay(ctx, today, s.clock.Now()) })
	if err != nil {
		return CommitSummary{}, err
	}
	plan, err := s.Plan(ctx)
	if err != nil {
		return CommitSummary{}, err
	}
	sum := CommitSummary{Blocks: len(plan.Blocks)}
	for _, b := range plan.Blocks {
		sum.Blocked += b.Duration
	}
	for _, c := range plan.Candidates {
		if c.Task.Status == domain.StatusOpen && !c.Blocked {
			sum.Candidates++
		}
	}
	return sum, nil
}

// Plan returns today's candidates and blocks. Closed tasks stay among the
// candidates until the next planning day, so the day's wins remain visible.
func (s *Service) Plan(ctx context.Context) (domain.Plan, error) {
	today := s.Today()
	snap, state, err := s.store.LoadPlan(ctx, today)
	if err != nil {
		return domain.Plan{}, err
	}
	blocked := map[int64]bool{}
	for _, b := range state.Blocks {
		blocked[b.TaskID] = true
	}
	plan := domain.Plan{Day: today, Committed: state.Committed, Blocks: state.Blocks}
	for _, p := range state.Pins {
		t, ok := snap.Get(p.TaskID)
		if !ok {
			continue
		}
		plan.Candidates = append(plan.Candidates, domain.Candidate{
			Task:       t,
			PinnedDays: p.PinnedDays,
			Blocked:    blocked[t.ID],
			Alert:      t.RolloverCount >= s.cfg.RolloverAlert,
		})
	}
	slices.SortFunc(plan.Candidates, func(a, b domain.Candidate) int {
		pa := domain.EffectivePriority(a.Task, snap.ParentOf(a.Task))
		pb := domain.EffectivePriority(b.Task, snap.ParentOf(b.Task))
		return cmp.Or(cmp.Compare(unsetLast(pa), unsetLast(pb)), cmp.Compare(a.Task.ID, b.Task.ID))
	})
	return plan, nil
}

// unsetLast sorts priority 0 (unset) after every real level.
func unsetLast(p int) int {
	if p == 0 {
		return math.MaxInt
	}
	return p
}
```

- [ ] **Step 6: Run the tests to verify they pass**

```bash
gofmt -l . && go vet ./... && go test ./...
go test -race -count=3 -run TestStartDayRaceCountsOnce ./internal/app/
```

Expected: no gofmt output; every package `ok`; the race run `ok` with no race reports.

- [ ] **Step 7: Commit**

```bash
git add internal/app
git commit -m "feat(app): pins, blocks, StartDay rollover, CommitPlan and Plan" -m "Co-Authored-By: Claude <noreply@anthropic.com>"
```
