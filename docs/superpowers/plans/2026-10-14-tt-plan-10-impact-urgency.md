# tt Plan 10 — Impact and Urgency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Priority levels (P1/P2/P3) give way to impact (1–3, who notices if it slips) and an optional due date (`^fri`). tt derives Taskwarrior-style urgency from them, shows it as `[ 73 S]` where the P-level was, sorts the backlog by urgency or impact, and moves existing tasks off priority with one re-triage.

**Architecture:** Impact and due are stored on the task (migration 006); urgency is a pure function in `domain` of the effective impact, the effective due and the planning day, weighted by `[urgency]`, and never stored. The switch happens in one task across `dsl`, `config`, `app`, `search` and `tui`, between two additive tasks before it (the model, the parsers and config) and one after it (the due marker, Task detail and the Inbox's due-first flow).

**Tech Stack:** Go 1.27; Bubble Tea v2.1, Lip Gloss; pure-Go SQLite (modernc), BurntSushi TOML. No new Go dependencies.

**Spec:** `docs/superpowers/specs/2026-10-12-task-tracking-design.md`: D68–D76 and the section "Plan 10 — Impact and urgency" (with D11, D14, D25, D26, D26d, D32, D48 and D52 as amended).

## Global Constraints

- No new Go dependencies: `go.mod` and `go.sum` stay unchanged.
- Impact is 1–3: **1** a person, **2** one team, **3** more than one team (who notices if it slips). Children take their parent's impact, never their own.
- Urgency, exactly: `(w_i·impact/3 + w_d·f) / (w_i + w_d)`; `f` = 0 with no due, 0.2 more than 14 days ahead, `0.2 + (d + 14)·0.8/21` from 14 days ahead to 7 overdue (`d` = today − due in whole planning days), 1.0 after. Default weights 1 and 1. Shown as `UrgencyScore` = round(u·100), padded to three cells: `[ 73 S]`, untriaged `[  — S]`.
- `^` forms: `today`, `tom`, `tomorrow`, `mon`…`sun`, `Nd`, `Nw`, `30oct`, `2026-10-30`; against the planning day (D19), not the calendar date.
- Texts, exactly: `impact !4 out of range: use !1 to !3`; `children inherit impact from #N; drop !n` (the service error, after `invalid value: `); `[[priorities]] was replaced by impact (D68): remove it, see config.example.toml`; `[github] priority was replaced by impact and due (D75): remove it, see config.example.toml`; `due "x": want today, tom, a weekday like fri, 3d, 2w, 30oct or 2026-10-30`; `due ^31feb: no such date`; the Inbox's `set impact and size first · n skips` and `triaged · ⏎ next card`.
- Migration 006 never touches `priority` and never deletes anything; it runs once (schema_migrations version 6).
- Every task ends green on: `gofmt -l .` (no output), `go vet ./...`, `go test ./...`; Tasks 3 and 4 also `go test -race ./internal/tui/ ./internal/app/`.
- Every commit message ends with a blank line and `Co-Authored-By: Claude <noreply@anthropic.com>`. Commit messages use single quotes.
- Sandbox: save each patch to a file and `git apply <file>`; run plain, separate git commands (no `git -C`, no `cd … &&`). If `git apply` fails, stop: an earlier step went wrong.

## Rulings made while drafting (in the code; each a one-line reversal)

- The steps' wording lives in `domain` (`ImpactNames`, `ImpactExamples`); the Inbox card reads `[1] a person · me, one reviewer, one asker`, `[2] one team · my team, or one other team`, `[3] more than one team · several teams, a stream` under `who notices if it slips?`.
- The TUI computes urgency against `svc.Today()` (always a valid day), not the loaded plan's day.
- The due marker goes on list rows and Task detail's children, not on timeline labels (kept short); closed tasks show none.
- `SetImpact` refuses a child itself (not only the parsers), so no path stores a child's own impact.
- Weekdays are their three-letter names only: `^friday` stays in the title, like any other `^word`. `^Nd`/`^Nw` go up to 9999.
- `[github] due` takes whole days `Nd` (0d–365d) or `""`; `Config.ReviewDue` is -1 for no due. The review's due counts from the planning day of the request (`SourceItem.WaitingSince`), in the clock's zone; migration 006 uses the request's UTC date, a one-off.
- `g` says `sorted by urgency` / `sorted by impact` for the flat sorts and `grouped by …` for the rest; the help reads `sort by urgency → impact, group by size → created → requester`. The footer hint stays `g group`.
- The Inbox's `^` prompt is prefilled with the task's own due (`2026-10-16`) and takes the forms with or without the `^`.
- A held Inbox card (one a key acted on) is listed first while it is open, so the title reads `INBOX · 1 of N` until ⏎.

## Review Focus

1. **Opening the real database** (the first run after the upgrade, a database full of P-levels): it migrates once, priority stays untouched, open top-level tasks wait in the Inbox, pinned reviews keep working with impact 1 and a due. Pinned in Task 1 `TestMigration006` (open, closed, child, source-linked with and without a request time, a gone source item) and the pty run after the last task.
2. **An old `config.toml`** (made by `make config` with `[[priorities]]` and `[github] priority`): tt refuses to start with a message that says what to remove, not `unknown config key`. Pinned in Task 3 `TestPriorityKeysAreGone` and `cmd/tt` `TestConfigIsHonoured` (`tt add` exits 1 with the message).
3. **Working past midnight** (02:00 with `day_start = "04:00"`): `^tom` and `^today` count from the planning day, and urgency stays the same until the day starts. Pinned in Task 3 `TestDueDatesFollowThePlanningDay`; `DueFactor` works in whole planning days (Task 1 `TestDueFactor`).
4. **Titles that look like dates, and dates that don't exist** (`x^2`, `^_^`, a lone `^`, `^someday`, `^friday`, `^31feb`, `29feb` outside a leap year, `2jan` typed on 30 Dec): look-alikes stay in the title, impossible dates are errors, and the year rolls over. Pinned in Task 2 `TestDueWordRolloverAndErrors` and Task 3 `TestParseAdd` / `TestParseAddErrors`.
5. **An Inbox card that changes under the cursor** (dropped, closed elsewhere, or a triage step undone with ctrl+z): it never sticks on screen once it is closed, and an undone step shows it untriaged again rather than skipping it. Pinned in Task 4 `TestInboxHeldCardLeavesWhenDropped` and `TestInboxShowsTheDueFirstAndWaitsForEnter` (ctrl+z).

## File Structure

```
internal/domain/urgency.go             new: impact steps, EffectiveImpact/Due, DueFactor, Urgency, Day.AddDays (Task 1)
internal/domain/task.go, rules.go      modify: Impact, Due (1); Priority gone, InInbox per D74 (3)
internal/store/tasks.go                modify: impact and due columns (1); priority unread (3)
internal/store/migrations/006_impact_due.sql   new (1)
internal/dsl/due.go                    new: dueWord, ParseDue (2)
internal/config/config.go, config.example.toml  modify: [urgency], [github] impact/due (2); priorities gone, D76 errors (3)
internal/dsl/add.go, filter.go         modify: !n impact, ^ due (3)
internal/app/tasks.go, children.go, sources.go, day.go   modify: impact, due, SetImpact, SetDue, D75, candidate order (3)
internal/search/search.go              modify: ByUrgency, ByImpact, Sorter (3)
internal/tui/*.go                      modify: badge, Inbox impact, People order, help (3); due marker, detail line, Inbox due and ⏎ (4)
cmd/tt/main.go, README.md              modify: usage and syntax (3)
tests: internal/domain/urgency_test.go, internal/store/migration006_test.go, internal/dsl/due_test.go, internal/config/urgency_test.go, internal/tui/due_test.go new; the rest modified
```

Work happens on branch `plan-10` in a worktree.

---
### Task 1: Impact, due date and urgency in the model; migration 006

D68–D70, D76, additive: nothing reads the new fields yet, and priority still works everywhere. `domain.Task` gains `Impact` (0 = unset, 1–3) and `Due` (a `domain.Day`, "" = none). A new `internal/domain/urgency.go` holds the impact steps' names and examples (shown on the Inbox card later), `EffectiveImpact` (always the parent's for a child) and `EffectiveDue` (own, else the parent's), Taskwarrior's due ramp `DueFactor` in whole planning days, `Urgency` (the weighted mean of impact/3 and the due factor; `ok` false while untriaged), `UrgencyScore` (0–100 as shown), `CompareUrgency` (more urgent first, untriaged last) and day arithmetic (`Day.AddDays`, `DaysBetween`). The store reads and writes `impact` and `due`. Migration `006` adds both columns (`ALTER TABLE … ADD COLUMN`, no table rebuild), gives no task an impact, and gives open source-linked tasks impact 1 and a due 3 days after their request's UTC date (first seen without one; none when the source item is gone). `priority` stays in the schema.

**Files:**
- Modify: `internal/domain/task.go`
- Create: `internal/domain/urgency.go`
- Create: `internal/store/migrations/006_impact_due.sql`
- Modify: `internal/store/tasks.go`
- Create: `internal/domain/urgency_test.go`
- Create: `internal/store/migration006_test.go`
- Modify: `internal/store/store_test.go`

**Interfaces:**
- Consumes: existing `domain.Day`, `domain.Task`, store `taskColumns`/`taskArgs`/`scanTask`, `source_items`.
- Produces (internal/domain/urgency.go):
  - `const MaxImpact = 3`; `var ImpactNames, ImpactExamples []string` (index i-1 for step i).
  - `func EffectiveImpact(t Task, parent *Task) int`; `func EffectiveDue(t Task, parent *Task) Day`.
  - `type Weights struct{ Impact, Due float64 }`; `var DefaultWeights = Weights{Impact: 1, Due: 1}`.
  - `func DueFactor(today, due Day) float64`; `func Urgency(t Task, parent *Task, today Day, w Weights) (u float64, ok bool)`; `func UrgencyScore(u float64) int`; `func CompareUrgency(a float64, aok bool, b float64, bok bool) int`.
  - `func (d Day) AddDays(n int) Day`; `func DaysBetween(from, to Day) int` (positive when `to` is later).
- Produces: `domain.Task.Impact int`, `domain.Task.Due Day`; store migration `006_impact_due.sql`.

- [ ] **Step 1: Write the failing tests**

Create `internal/domain/urgency_test.go`:

```go
package domain

import (
	"math"
	"testing"
)

const today = Day("2026-10-14")

func near(a, b float64) bool { return math.Abs(a-b) < 0.005 }

func TestDueFactor(t *testing.T) {
	cases := []struct {
		name string
		due  Day
		want float64
	}{
		{"no due", "", 0},
		{"15 days ahead", today.AddDays(15), 0.2},
		{"14 days ahead", today.AddDays(14), 0.2},
		{"7 days ahead", today.AddDays(7), 0.2 + 7*0.8/21},
		{"tomorrow", today.AddDays(1), 0.2 + 13*0.8/21},
		{"today", today, 0.2 + 14*0.8/21},
		{"yesterday", today.AddDays(-1), 0.2 + 15*0.8/21},
		{"7 days overdue", today.AddDays(-7), 1},
		{"8 days overdue", today.AddDays(-8), 1},
		{"a year overdue", today.AddDays(-365), 1},
	}
	for _, c := range cases {
		if got := DueFactor(today, c.due); !near(got, c.want) {
			t.Errorf("%s: DueFactor = %.3f; want %.3f", c.name, got, c.want)
		}
	}
	if got := DueFactor(today, today); !near(got, 0.733) {
		t.Errorf("due today = %.3f; want about 0.73 (D70)", got)
	}
}

// The worked examples of D70, with the default weights.
func TestUrgencyExamples(t *testing.T) {
	cases := []struct {
		name string
		task Task
		want float64
	}{
		{"step 3, no due", Task{Impact: 3}, 0.50},
		{"step 1, due in a month", Task{Impact: 1, Due: today.AddDays(30)}, 0.27},
		{"step 1, due today", Task{Impact: 1, Due: today}, 0.53},
		{"step 1, 7 days overdue", Task{Impact: 1, Due: today.AddDays(-7)}, 0.67},
		{"step 3, due in 3 weeks", Task{Impact: 3, Due: today.AddDays(21)}, 0.60},
		{"step 2, due in 10 days", Task{Impact: 2, Due: today.AddDays(10)}, 0.51},
		{"step 1, no due", Task{Impact: 1}, 0.17},
		{"step 3, a week overdue", Task{Impact: 3, Due: today.AddDays(-7)}, 1},
	}
	for _, c := range cases {
		u, ok := Urgency(c.task, nil, today, DefaultWeights)
		if !ok || !near(u, c.want) {
			t.Errorf("%s: Urgency = %.3f, %v; want %.2f, true", c.name, u, ok, c.want)
		}
	}
	if u, ok := Urgency(Task{Due: today}, nil, today, DefaultWeights); ok || u != 0 {
		t.Errorf("untriaged: %.2f, %v; want no urgency", u, ok)
	}
}

func TestUrgencyWeights(t *testing.T) {
	task := Task{Impact: 1, Due: today}
	if u, _ := Urgency(task, nil, today, Weights{Impact: 1, Due: 0}); !near(u, 1.0/3) {
		t.Errorf("impact only: %.3f; want 0.333", u)
	}
	if u, _ := Urgency(task, nil, today, Weights{Impact: 0, Due: 2}); !near(u, 0.733) {
		t.Errorf("due only: %.3f; want 0.733", u)
	}
	if u, _ := Urgency(task, nil, today, Weights{Impact: 1, Due: 3}); !near(u, (1.0/3+3*0.7333)/4) {
		t.Errorf("1:3: %.3f", u)
	}
}

func TestEffectiveImpactAndDue(t *testing.T) {
	parent := Task{ID: 1, Impact: 3, Due: "2026-10-20"}
	child := Task{ID: 2, ParentID: 1, Impact: 1}
	if got := EffectiveImpact(child, &parent); got != 3 {
		t.Errorf("child impact = %d; want the parent's 3 (D68)", got)
	}
	if got := EffectiveImpact(child, nil); got != 1 {
		t.Errorf("top-level impact = %d; want its own 1", got)
	}
	if got := EffectiveDue(child, &parent); got != "2026-10-20" {
		t.Errorf("child without a due = %q; want the parent's (D69)", got)
	}
	child.Due = "2026-10-16"
	if got := EffectiveDue(child, &parent); got != "2026-10-16" {
		t.Errorf("child with its own due = %q; want its own", got)
	}
	if u, ok := Urgency(Task{ParentID: 1}, &Task{ID: 1}, today, DefaultWeights); ok {
		t.Errorf("child of an untriaged parent: %.2f, true; want no urgency", u)
	}
}

func TestUrgencyScoreAndOrder(t *testing.T) {
	for u, want := range map[float64]int{0: 0, 1.0 / 6: 17, 0.5: 50, 0.7333: 73, 1: 100} {
		if got := UrgencyScore(u); got != want {
			t.Errorf("UrgencyScore(%.4f) = %d; want %d", u, got, want)
		}
	}
	if CompareUrgency(0.6, true, 0.5, true) >= 0 || CompareUrgency(0.1, true, 0.9, false) >= 0 ||
		CompareUrgency(0.9, false, 0.1, true) <= 0 || CompareUrgency(0.5, true, 0.5, true) != 0 {
		t.Error("CompareUrgency: want the more urgent first, untriaged last, 0 on a tie")
	}
}

func TestDayArithmetic(t *testing.T) {
	if got := Day("2026-12-30").AddDays(3); got != "2027-01-02" {
		t.Errorf("AddDays over the year end = %q", got)
	}
	if got := Day("2026-03-01").AddDays(-1); got != "2026-02-28" {
		t.Errorf("AddDays(-1) = %q", got)
	}
	if got := DaysBetween("2026-10-14", "2026-10-21"); got != 7 {
		t.Errorf("DaysBetween forward = %d; want 7", got)
	}
	if got := DaysBetween("2026-10-21", "2026-10-14"); got != -7 {
		t.Errorf("DaysBetween backward = %d; want -7", got)
	}
	if got := DaysBetween("2026-03-28", "2026-03-30"); got != 2 {
		t.Errorf("DaysBetween over a DST change = %d; want 2", got)
	}
}
```

Create `internal/store/migration006_test.go`:

```go
package store

import (
	"context"
	"testing"
	"time"

	"tt/internal/domain"
)

// TestMigration006 runs 006 on a database as Plan 9 left it (D76): no task
// gets an impact except open pinned reviews (impact 1, due 3 days after the
// request, or after first seen without one); priority stays as it was.
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
		if got.Impact != w.impact || got.Due != w.due || got.Priority != w.priority {
			t.Errorf("task #%d: impact %d, due %q, priority %d; want %d, %q, %d",
				id, got.Impact, got.Due, got.Priority, w.impact, w.due, w.priority)
		}
	}
}
```

Patch `internal/store/store_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/store/store_test.go b/internal/store/store_test.go
index f294100..f9cb207 100644
--- a/internal/store/store_test.go
+++ b/internal/store/store_test.go
@@ -33,7 +33,7 @@ func sampleTask() domain.Task {
 	now := time.Date(2026, 10, 12, 9, 30, 0, 123, time.UTC)
 	return domain.Task{
 		Title: "Review PR #482", Note: "line 1\nline 2", Link: "https://github.com/a/b/pull/482",
-		Requester: "ivan", Tags: []string{"impl", "photos"}, Priority: 2, Size: domain.SizeM,
+		Requester: "ivan", Tags: []string{"impl", "photos"}, Priority: 2, Impact: 2, Due: "2026-10-16", Size: domain.SizeM,
 		Status: domain.StatusOpen, CreatedAt: now, UpdatedAt: now,
 	}
 }
@@ -92,7 +92,7 @@ func TestTaskRoundTrip(t *testing.T) {
 	want.Status, want.ClosedBy, want.DropReason = domain.StatusDropped, domain.ClosedByUser, "handed to @marko"
 	want.ClosedAt = want.CreatedAt.Add(time.Hour)
 	want.Tags = []string{"org"}
-	want.Priority, want.Size = 0, domain.SizeNone
+	want.Priority, want.Impact, want.Due, want.Size = 0, 0, "", domain.SizeNone
 	want.Source = &domain.SourceRef{Kind: "github", ID: "a/b#482"}
 	err = s.InTx(context.Background(), func(x *Tx) error { return x.UpdateTask(context.Background(), want) })
 	if err != nil {
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/domain/ ./internal/store/`
Expected: FAIL (build): `today.AddDays undefined (type Day has no field or method AddDays)`, `undefined: DueFactor`, and in store `unknown field Impact in struct literal of type domain.Task`.

- [ ] **Step 3: Implement**

Patch `internal/domain/task.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/domain/task.go b/internal/domain/task.go
index 599a24c..78870a1 100644
--- a/internal/domain/task.go
+++ b/internal/domain/task.go
@@ -62,6 +62,8 @@ type Task struct {
 	Requester     string   // lowercase; "" = none
 	Tags          []string // normalized: lowercase, unique, sorted
 	Priority      int      // 0 = unset; 1 = highest
+	Impact        int      // 0 = unset; 1–3, who notices if it slips (D68)
+	Due           Day      // "" = none (D69)
 	Size          Size
 	ParentID      int64 // 0 = no parent
 	Position      int   // order among siblings
```

Create `internal/domain/urgency.go`:

```go
package domain

import (
	"cmp"
	"fmt"
	"math"
	"time"
)

// MaxImpact is the biggest impact step (D68).
const MaxImpact = 3

// Impact steps (D68): who notices if a task slips, counted in teams.
// ImpactNames[i-1] names step i; ImpactExamples[i-1] goes with it on the
// Inbox card.
var (
	ImpactNames    = []string{"a person", "one team", "more than one team"}
	ImpactExamples = []string{"me, one reviewer, one asker", "my team, or one other team", "several teams, a stream"}
)

// EffectiveImpact is the impact that applies to t: a child always takes its
// parent's, with no override (D68).
func EffectiveImpact(t Task, parent *Task) int {
	if parent != nil {
		return parent.Impact
	}
	return t.Impact
}

// EffectiveDue is the due date that applies to t: its own, else its
// parent's (D69). "" means none.
func EffectiveDue(t Task, parent *Task) Day {
	if t.Due == "" && parent != nil {
		return parent.Due
	}
	return t.Due
}

// Weights weigh impact against the due date in urgency (D70).
type Weights struct{ Impact, Due float64 }

// DefaultWeights count both the same.
var DefaultWeights = Weights{Impact: 1, Due: 1}

// DueFactor is Taskwarrior's due ramp in whole planning days (D70): 0 with
// no due; 0.2 more than 14 days ahead; rising in a straight line to 1.0 at 7
// days overdue, and 1.0 after that. Due today is about 0.73.
func DueFactor(today, due Day) float64 {
	if due == "" {
		return 0
	}
	switch d := float64(DaysBetween(due, today)); { // days overdue
	case d >= 7:
		return 1
	case d >= -14:
		return 0.2 + (d+14)*0.8/21
	}
	return 0.2
}

// Urgency is t's urgency on today, 0 to 1 (D70): the weighted mean of its
// effective impact (scaled to 0–1) and its due factor. ok is false while t
// has no effective impact (untriaged): it has no urgency then.
func Urgency(t Task, parent *Task, today Day, w Weights) (u float64, ok bool) {
	impact := EffectiveImpact(t, parent)
	if impact == 0 {
		return 0, false
	}
	f := DueFactor(today, EffectiveDue(t, parent))
	return (w.Impact*float64(impact)/MaxImpact + w.Due*f) / (w.Impact + w.Due), true
}

// UrgencyScore is urgency as shown, 0 to 100 (D71).
func UrgencyScore(u float64) int { return int(math.Round(u * 100)) }

// CompareUrgency orders the more urgent first and an untriaged task (ok
// false) after every triaged one; 0 on a tie (D72).
func CompareUrgency(a float64, aok bool, b float64, bok bool) int {
	if aok != bok {
		if aok {
			return -1
		}
		return 1
	}
	return cmp.Compare(b, a)
}

// AddDays is the day n days after d (before it for n < 0).
func (d Day) AddDays(n int) Day {
	return Day(d.date().AddDate(0, 0, n).Format(time.DateOnly))
}

// DaysBetween is how many days from comes before to: positive when to is
// later.
func DaysBetween(from, to Day) int {
	return int(math.Round(to.date().Sub(from.date()).Hours() / 24))
}

func (d Day) date() time.Time {
	t, err := time.Parse(time.DateOnly, string(d))
	if err != nil {
		panic(fmt.Sprintf("invalid Day %q", d))
	}
	return t
}
```

Create `internal/store/migrations/006_impact_due.sql`:

```sql
-- Impact and due replace priority (D68, D69, D76). No task gets an impact:
-- open top-level tasks go back to the Inbox for one re-triage. Pinned
-- reviews never reach the Inbox, so open source-linked tasks get impact 1
-- and are due 3 days after the request (D75's defaults; the request's UTC
-- date, a one-off). priority stays, unused.
ALTER TABLE tasks ADD COLUMN impact INTEGER;
ALTER TABLE tasks ADD COLUMN due TEXT;

UPDATE tasks SET
    impact = 1,
    due = (SELECT date(coalesce(i.requested_at, i.first_seen_at), '+3 days')
           FROM source_items i
           WHERE i.kind = tasks.source_kind AND i.external_id = tasks.source_id)
WHERE status = 'open' AND source_kind IS NOT NULL;
```

Patch `internal/store/tasks.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/store/tasks.go b/internal/store/tasks.go
index 12d50ff..9e40dc6 100644
--- a/internal/store/tasks.go
+++ b/internal/store/tasks.go
@@ -14,13 +14,13 @@ import (
 // Tx is a write transaction. Get one from Store.InTx.
 type Tx struct{ tx *sql.Tx }
 
-const taskColumns = `id, title, note, link, requester, priority, size, parent_id, position, status,
-	drop_reason, source_kind, source_id, rollover_count, created_at, updated_at, closed_at, closed_by`
+const taskColumns = `id, title, note, link, requester, priority, impact, due, size, parent_id, position,
+	status, drop_reason, source_kind, source_id, rollover_count, created_at, updated_at, closed_at, closed_by`
 
 // InsertTask inserts t (ignoring t.ID) with its tags and returns the new id.
 func (x *Tx) InsertTask(ctx context.Context, t domain.Task) (int64, error) {
 	res, err := x.tx.ExecContext(ctx, `INSERT INTO tasks (`+strings.TrimPrefix(taskColumns, "id, ")+`)
-		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, taskArgs(t)...)
+		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, taskArgs(t)...)
 	if err != nil {
 		return 0, err
 	}
@@ -34,7 +34,7 @@ func (x *Tx) InsertTask(ctx context.Context, t domain.Task) (int64, error) {
 // UpdateTask overwrites every column and the tag set of task t.ID.
 func (x *Tx) UpdateTask(ctx context.Context, t domain.Task) error {
 	res, err := x.tx.ExecContext(ctx, `UPDATE tasks SET title = ?, note = ?, link = ?, requester = ?,
-		priority = ?, size = ?, parent_id = ?, position = ?, status = ?, drop_reason = ?, source_kind = ?,
+		priority = ?, impact = ?, due = ?, size = ?, parent_id = ?, position = ?, status = ?, drop_reason = ?, source_kind = ?,
 		source_id = ?, rollover_count = ?, created_at = ?, updated_at = ?, closed_at = ?, closed_by = ?
 		WHERE id = ?`, append(taskArgs(t), t.ID)...)
 	if err != nil {
@@ -222,7 +222,8 @@ func taskArgs(t domain.Task) []any {
 	if !t.ClosedAt.IsZero() {
 		closedAt = formatTime(t.ClosedAt)
 	}
-	return []any{t.Title, t.Note, t.Link, t.Requester, nullIfZero(t.Priority), nullIfZero(string(t.Size)),
+	return []any{t.Title, t.Note, t.Link, t.Requester, nullIfZero(t.Priority), nullIfZero(t.Impact),
+		nullIfZero(string(t.Due)), nullIfZero(string(t.Size)),
 		nullIfZero(t.ParentID), t.Position, string(t.Status), t.DropReason, sourceKind, sourceID,
 		t.RolloverCount, formatTime(t.CreatedAt), formatTime(t.UpdatedAt), closedAt, nullIfZero(t.ClosedBy)}
 }
@@ -231,19 +232,21 @@ type scanner interface{ Scan(dest ...any) error }
 
 func scanTask(row scanner) (domain.Task, error) {
 	var (
-		t                                    domain.Task
-		priority, parentID                   sql.NullInt64
-		size, sourceKind, sourceID, closedBy sql.NullString
-		status, createdAt, updatedAt         string
-		closedAt                             sql.NullString
+		t                                         domain.Task
+		priority, impact, parentID                sql.NullInt64
+		due, size, sourceKind, sourceID, closedBy sql.NullString
+		status, createdAt, updatedAt              string
+		closedAt                                  sql.NullString
 	)
-	err := row.Scan(&t.ID, &t.Title, &t.Note, &t.Link, &t.Requester, &priority, &size, &parentID,
+	err := row.Scan(&t.ID, &t.Title, &t.Note, &t.Link, &t.Requester, &priority, &impact, &due, &size, &parentID,
 		&t.Position, &status, &t.DropReason, &sourceKind, &sourceID, &t.RolloverCount,
 		&createdAt, &updatedAt, &closedAt, &closedBy)
 	if err != nil {
 		return domain.Task{}, err
 	}
 	t.Priority = int(priority.Int64)
+	t.Impact = int(impact.Int64)
+	t.Due = domain.Day(due.String)
 	t.Size = domain.Size(size.String)
 	t.ParentID = parentID.Int64
 	t.Status = domain.Status(status)
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/domain/ ./internal/store/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/task.go internal/domain/urgency.go internal/domain/urgency_test.go internal/store/migration006_test.go internal/store/migrations/006_impact_due.sql internal/store/store_test.go internal/store/tasks.go
git commit -m 'feat(domain,store): impact, due date and derived urgency; migration 006 (D68-D70, D76)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 2: `^` due dates and the new config keys

D70, D73, D75, still additive. A new `internal/dsl/due.go` reads the `^` forms against a given today (a planning day): `today`, `tom`/`tomorrow`, `mon`…`sun` (the next one, today counts), `Nd`/`Nw`, `30oct` (the next 30 Oct, today counts; a date that doesn't exist this year or next is an error) and `2026-10-30`, case-insensitive. `dueWord` tells "not a date" (stays in the title, like `~`) from "a date's shape but no real date" (an error). `ParseDue` is the Inbox input's reader: with or without the `^`, empty means no due. In config, `[urgency] impact` and `due` are weights (numbers ≥ 0, not both 0; TOML integers work), and `[github] impact` (1–3) and `due` (`Nd`, 0d–365d, or `""` for no due) are a pinned review's defaults (`ReviewImpact`, `ReviewDue`, -1 = no due). `config.example.toml` lists them with their defaults. `[[priorities]]` and `[github] priority` still work until Task 3.

**Files:**
- Modify: `config.example.toml`
- Modify: `internal/config/config.go`
- Create: `internal/dsl/due.go`
- Create: `internal/config/urgency_test.go`
- Create: `internal/dsl/due_test.go`

**Interfaces:**
- Consumes: Task 1 (`domain.Day.AddDays`, `domain.MaxImpact`, `domain.Weights`, `domain.DefaultWeights`); existing `isDigits` in internal/dsl.
- Produces:
  - internal/dsl/due.go: `func ParseDue(text string, today domain.Day) (domain.Day, error)`; `func dueWord(w string, today domain.Day) (due domain.Day, ok bool, err error)` (w without its `^`); test constant `wednesday = domain.Day("2026-10-14")` in due_test.go.
  - internal/config/config.go: `Config.ReviewImpact int` (default 1), `Config.ReviewDue int` (days, default 3, -1 = none), `Config.Urgency domain.Weights` (default `domain.DefaultWeights`); `parseUrgency`.

- [ ] **Step 1: Write the failing tests**

Create `internal/config/urgency_test.go`:

```go
package config

import (
	"strings"
	"testing"

	"tt/internal/domain"
)

func TestParseUrgency(t *testing.T) {
	if w := Default().Urgency; w != (domain.Weights{Impact: 1, Due: 1}) {
		t.Errorf("default weights = %+v; want 1 and 1 (D70)", w)
	}
	cfg, err := Parse([]byte("[urgency]\nimpact = 1\ndue = 2.5\n"))
	if err != nil || cfg.Urgency != (domain.Weights{Impact: 1, Due: 2.5}) {
		t.Errorf("weights 1, 2.5: %+v, %v", cfg.Urgency, err)
	}
	if cfg, err := Parse([]byte("[urgency]\ndue = 0\n")); err != nil || cfg.Urgency != (domain.Weights{Impact: 1, Due: 0}) {
		t.Errorf("due 0 (impact only): %+v, %v", cfg.Urgency, err)
	}
	cases := map[string]string{
		"[urgency]\nimpact = -1":         "urgency.impact: want a weight of 0 or more, got -1",
		"[urgency]\ndue = -0.5":          "urgency.due: want a weight of 0 or more, got -0.5",
		"[urgency]\nimpact = 0\ndue = 0": "urgency: impact and due can't both be 0",
		"[urgency]\nimpact = \"high\"":   "urgency.impact",
		"[urgency]\nsize = 1":            "unknown config key",
	}
	for input, wantErr := range cases {
		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}

func TestParseReviewImpactAndDue(t *testing.T) {
	if cfg := Default(); cfg.ReviewImpact != 1 || cfg.ReviewDue != 3 {
		t.Errorf("defaults: impact %d, due %d; want 1 and 3 days (D75)", cfg.ReviewImpact, cfg.ReviewDue)
	}
	for in, want := range map[string]int{`"0d"`: 0, `"5d"`: 5, `" 2d "`: 2, `""`: -1, `"365d"`: 365} {
		if cfg, err := Parse([]byte("[github]\ndue = " + in + "\n")); err != nil || cfg.ReviewDue != want {
			t.Errorf("due = %s: %d, %v; want %d", in, cfg.ReviewDue, err, want)
		}
	}
	if cfg, err := Parse([]byte("[github]\nimpact = 2\n")); err != nil || cfg.ReviewImpact != 2 {
		t.Errorf("impact = 2: %d, %v", cfg.ReviewImpact, err)
	}
	cases := map[string]string{
		"[github]\nimpact = 0":     "github.impact: want 1 to 3, got 0",
		"[github]\nimpact = 4":     "github.impact: want 1 to 3, got 4",
		"[github]\ndue = \"3\"":    `github.due: want whole days like "3d" (0d to 365d), or "" for no due, got "3"`,
		"[github]\ndue = \"1w\"":   "github.due: want whole days",
		"[github]\ndue = \"-1d\"":  "github.due: want whole days",
		"[github]\ndue = \"366d\"": "github.due: want whole days",
		"[github]\ndue = 3":        "github.due",
	}
	for input, wantErr := range cases {
		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}
```

Create `internal/dsl/due_test.go`:

```go
package dsl

import (
	"strings"
	"testing"

	"tt/internal/domain"
)

// 2026-10-14 is a Wednesday.
const wednesday = domain.Day("2026-10-14")

func TestDueWord(t *testing.T) {
	cases := map[string]domain.Day{
		"today":      "2026-10-14",
		"TOM":        "2026-10-15",
		"tomorrow":   "2026-10-15",
		"wed":        "2026-10-14", // today counts
		"thu":        "2026-10-15",
		"Fri":        "2026-10-16",
		"tue":        "2026-10-20",
		"0d":         "2026-10-14",
		"3d":         "2026-10-17",
		"2w":         "2026-10-28",
		"14oct":      "2026-10-14", // today counts
		"30oct":      "2026-10-30",
		"1OCT":       "2027-10-01", // passed this year: next year's
		"3jan":       "2027-01-03",
		"2026-10-30": "2026-10-30",
		"2026-01-02": "2026-01-02", // a past date is allowed: overdue (nudge, never block)
	}
	for in, want := range cases {
		got, ok, err := dueWord(in, wednesday)
		if !ok || err != nil || got != want {
			t.Errorf("dueWord(%q) = %q, %v, %v; want %q", in, got, ok, err, want)
		}
	}
}

func TestDueWordRolloverAndErrors(t *testing.T) {
	if got, _, err := dueWord("2jan", "2026-12-30"); err != nil || got != "2027-01-02" {
		t.Errorf("2jan at the year end = %q, %v; want next year's", got, err)
	}
	if got, _, err := dueWord("sat", "2026-12-30"); err != nil || got != "2027-01-02" {
		t.Errorf("sat at the year end = %q, %v", got, err)
	}
	if got, _, err := dueWord("29feb", "2027-03-01"); err != nil || got != "2028-02-29" {
		t.Errorf("29feb before a leap year = %q, %v; want 2028-02-29", got, err)
	}
	for _, in := range []string{"31feb", "0oct", "32oct", "2026-13-01", "2026-02-30", "29feb", "10000d"} {
		if _, ok, err := dueWord(in, wednesday); !ok || err == nil {
			t.Errorf("dueWord(%q) = ok %v, err %v; want a date-shaped error", in, ok, err)
		}
	}
	for _, in := range []string{"", "_^", "foo", "friday", "fri.", "3", "d", "3x", "oct", "123oct", "2026-10", "10/30"} {
		if _, ok, err := dueWord(in, wednesday); ok || err != nil {
			t.Errorf("dueWord(%q) = ok %v, err %v; want not a date (stays in the title)", in, ok, err)
		}
	}
}

func TestParseDue(t *testing.T) {
	for in, want := range map[string]domain.Day{"": "", "  ": "", "fri": "2026-10-16", "^fri": "2026-10-16", " ^3d ": "2026-10-17"} {
		if got, err := ParseDue(in, wednesday); err != nil || got != want {
			t.Errorf("ParseDue(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ParseDue("someday", wednesday); err == nil || !strings.Contains(err.Error(), "want today, tom, a weekday like fri, 3d, 2w, 30oct or 2026-10-30") {
		t.Errorf("ParseDue(someday) error = %v", err)
	}
	if _, err := ParseDue("31feb", wednesday); err == nil || !strings.Contains(err.Error(), "no such date") {
		t.Errorf("ParseDue(31feb) error = %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/dsl/ ./internal/config/`
Expected: FAIL (build): `undefined: dueWord`, `undefined: ParseDue`, and `Default().Urgency undefined (type Config has no field or method Urgency)`.

- [ ] **Step 3: Implement**

Patch `config.example.toml` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/config.example.toml b/config.example.toml
index a95739b..186d308 100644
--- a/config.example.toml
+++ b/config.example.toml
@@ -21,6 +21,13 @@ definition = "important this month"
 name = "P3"
 definition = "worth doing eventually"
 
+# Urgency (D70), the number tasks are sorted by: impact and the due date,
+# weighted. Raise due if deadlines should count more, impact if what matters
+# should; each 0 or more, not both 0.
+[urgency]
+impact = 1
+due = 1
+
 # How long each size takes: a block's default length (D24).
 [sizes]
 s = "1h"
@@ -52,6 +59,8 @@ host = ""       # a GitHub Enterprise host; empty: gh's default (github.com)
 poll = "5m"     # how often GitHub is fetched; 30s or more
 priority = "P2" # a pinned review's priority: one of the [[priorities]] names
 size = "s"      # a pinned review's size: s, m, l, xl, or "" for unsized (D48)
+impact = 1      # a pinned review's impact, 1 to 3 (D75)
+due = "3d"      # a pinned review is due this many days after the request; "" = no due
 
 # macOS notifications from the running tt (D63), each kind off until you turn
 # it on. The first one makes macOS ask to allow notifications for Script
```

Patch `internal/config/config.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/config/config.go b/internal/config/config.go
index 7acaf70..afaf9e3 100644
--- a/internal/config/config.go
+++ b/internal/config/config.go
@@ -7,6 +7,7 @@ import (
 	"io/fs"
 	"os"
 	"slices"
+	"strconv"
 	"strings"
 	"time"
 
@@ -46,6 +47,12 @@ type Config struct {
 	// pinned review request starts with (D48).
 	ReviewPriority int
 	ReviewSize     domain.Size
+	// ReviewImpact and ReviewDue are the impact and the days after the
+	// request a pinned review's task is due (D75); ReviewDue -1 is no due.
+	ReviewImpact int
+	ReviewDue    int
+	// Urgency weighs impact against the due date (D70).
+	Urgency domain.Weights
 	// Notify is what the running TUI notifies about (D63) and when (D64).
 	Notify Notify
 }
@@ -86,6 +93,9 @@ func Default() Config {
 		GitHubPoll:     5 * time.Minute,
 		ReviewPriority: 2,
 		ReviewSize:     domain.SizeS,
+		ReviewImpact:   1,
+		ReviewDue:      3,
+		Urgency:        domain.DefaultWeights,
 		Notify:         Notify{Alerts: []int{5, 0}, Sound: "Glass"},
 	}
 }
@@ -113,7 +123,13 @@ type file struct {
 		Poll     *string `toml:"poll"`
 		Priority *string `toml:"priority"`
 		Size     *string `toml:"size"`
+		Impact   *int    `toml:"impact"`
+		Due      *string `toml:"due"`
 	} `toml:"github"`
+	Urgency struct {
+		Impact *float64 `toml:"impact"`
+		Due    *float64 `toml:"due"`
+	} `toml:"urgency"`
 	Notify struct {
 		Meetings *bool   `toml:"meetings"`
 		Blocks   *bool   `toml:"blocks"`
@@ -224,6 +240,9 @@ func Parse(data []byte) (Config, error) {
 	if err := parseNotify(f, &cfg); err != nil {
 		return Config{}, err
 	}
+	if err := parseUrgency(f, &cfg); err != nil {
+		return Config{}, err
+	}
 	switch {
 	case cfg.WorkEnd <= cfg.WorkStart:
 		return Config{}, fmt.Errorf("work_hours.end must be after work_hours.start")
@@ -270,6 +289,47 @@ func parseGitHub(f file, cfg *Config) error {
 		}
 		cfg.ReviewSize = size
 	}
+	if p := g.Impact; p != nil {
+		if *p < 1 || *p > domain.MaxImpact {
+			return fmt.Errorf("github.impact: want 1 to %d, got %d", domain.MaxImpact, *p)
+		}
+		cfg.ReviewImpact = *p
+	}
+	if p := g.Due; p != nil {
+		v := strings.TrimSpace(*p)
+		n, err := strconv.Atoi(strings.TrimSuffix(v, "d"))
+		switch {
+		case v == "":
+			cfg.ReviewDue = -1
+		case !strings.HasSuffix(v, "d") || err != nil || n < 0 || n > 365:
+			return fmt.Errorf("github.due: want whole days like \"3d\" (0d to 365d), or \"\" for no due, got %q", *p)
+		default:
+			cfg.ReviewDue = n
+		}
+	}
+	return nil
+}
+
+// parseUrgency applies [urgency] (D70): two weights, each 0 or more, not
+// both 0.
+func parseUrgency(f file, cfg *Config) error {
+	u := f.Urgency
+	for _, w := range []struct {
+		name string
+		from *float64
+		to   *float64
+	}{{"impact", u.Impact, &cfg.Urgency.Impact}, {"due", u.Due, &cfg.Urgency.Due}} {
+		if w.from == nil {
+			continue
+		}
+		if *w.from < 0 {
+			return fmt.Errorf("urgency.%s: want a weight of 0 or more, got %v", w.name, *w.from)
+		}
+		*w.to = *w.from
+	}
+	if cfg.Urgency.Impact+cfg.Urgency.Due == 0 {
+		return fmt.Errorf("urgency: impact and due can't both be 0")
+	}
 	return nil
 }
 
```

Create `internal/dsl/due.go`:

```go
package dsl

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"tt/internal/domain"
)

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

var months = map[string]time.Month{
	"jan": time.January, "feb": time.February, "mar": time.March, "apr": time.April,
	"may": time.May, "jun": time.June, "jul": time.July, "aug": time.August,
	"sep": time.September, "oct": time.October, "nov": time.November, "dec": time.December,
}

// dueForms lists the forms for error messages.
const dueForms = "today, tom, a weekday like fri, 3d, 2w, 30oct or 2026-10-30"

// ParseDue reads a due date typed on its own, with or without its ^ (the
// Inbox's due input, D73): "" is no due.
func ParseDue(text string, today domain.Day) (domain.Day, error) {
	s := strings.TrimPrefix(strings.TrimSpace(text), "^")
	if s == "" {
		return "", nil
	}
	due, ok, err := dueWord(s, today)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("due %q: want %s", s, dueForms)
	}
	return due, nil
}

// dueWord reads w, a ^ token without its ^ (D73), against today (a
// planning day): today, tom or tomorrow, mon…sun (the next one; today
// counts), Nd or Nw from today, 30oct (the next 30 Oct; today counts) and
// 2026-10-30, case-insensitive. ok is false when w has none of these shapes
// (it stays in the title); err is set when it has a date's shape but names
// no real date (31feb, 2026-13-01).
func dueWord(w string, today domain.Day) (due domain.Day, ok bool, err error) {
	w = strings.ToLower(w)
	t, perr := time.Parse(time.DateOnly, string(today))
	if perr != nil {
		return "", false, fmt.Errorf("today %q: %w", today, perr)
	}
	switch {
	case w == "today":
		return today, true, nil
	case w == "tom" || w == "tomorrow":
		return today.AddDays(1), true, nil
	}
	if wd, isDay := weekdays[w]; isDay {
		return today.AddDays((int(wd) - int(t.Weekday()) + 7) % 7), true, nil
	}
	if n := len(w); n >= 2 && (w[n-1] == 'd' || w[n-1] == 'w') && isDigits(w[:n-1]) {
		k, err := strconv.Atoi(w[:n-1])
		if err != nil || k > 9999 {
			return "", true, fmt.Errorf("due ^%s: want at most 9999 days or weeks", w)
		}
		if w[n-1] == 'w' {
			k *= 7
		}
		return today.AddDays(k), true, nil
	}
	if len(w) == 10 && w[4] == '-' && w[7] == '-' && isDigits(w[:4]+w[5:7]+w[8:]) {
		if _, err := time.Parse(time.DateOnly, w); err != nil {
			return "", true, fmt.Errorf("due ^%s: no such date", w)
		}
		return domain.Day(w), true, nil
	}
	if n := len(w); n >= 4 && n <= 5 && isDigits(w[:n-3]) {
		if month, isMonth := months[w[n-3:]]; isMonth {
			day, _ := strconv.Atoi(w[:n-3])
			for year := t.Year(); year <= t.Year()+1; year++ {
				d := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
				if d.Month() == month && d.Day() == day && !d.Before(t) {
					return domain.Day(d.Format(time.DateOnly)), true, nil
				}
			}
			return "", true, fmt.Errorf("due ^%s: no such date", w)
		}
	}
	return "", false, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/dsl/ ./internal/config/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add config.example.toml internal/config/config.go internal/config/urgency_test.go internal/dsl/due.go internal/dsl/due_test.go
git commit -m 'feat(dsl,config): ^ due dates, [urgency] weights and the review impact and due (D70, D73, D75)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 3: Impact and urgency replace priority

D68, D71–D76: the switch, in every package at once (removing `Task.Priority` breaks every user of it). `domain`: `Priority` and `EffectivePriority` go; `InInbox` follows D74 (a top-level task without impact or size; a child without its own size). `store`: `priority` is no longer read or written. `config`: `[[priorities]]`, `PriorityLevel`, `Priorities` and `ReviewPriority` go; the old keys get their own errors (D76). `dsl`: `ParseAdd(line, today)` reads `!1`–`!3` as impact (`impact !4 out of range: use !1 to !3`) and `^` as due (last one wins); `FormatAdd` writes `!n` and an absolute `^YYYY-MM-DD`; `ParseFilter(line)` filters `!n` by effective impact. `app`: `Add`, `Edit`, `EditLine`, `ParseEdit`, `ParseChild`, `AddChild` use impact and due against `s.Today()`; `SetImpact` (refused on a child) replaces `SetPriority`; `SetDue` is new; `PinSource` uses D75's defaults; candidates order by urgency. `search`: `ByUrgency` (the default) and `ByImpact` are flat sorts (`Flat`, `Sorter`), `ByPriority` is gone, a remembered `priority` falls back to urgency. `tui`: the badge is `[ 73 S]` (`badgeOf(u, ok, size)`), the Inbox's `1`–`3` set impact with the steps on the card, People lists by urgency, the help and placeholders show `!1 ^fri`. `cmd/tt` usage and the README show the new syntax. The existing TUI tests swap `!1` and `!3` so each keeps its meaning (old P1, the top, is impact 3, the biggest): with no due, `[P1 S]` reads `[ 50 S]`, `[P2 S]` `[ 33 S]`, `[P3 S]` `[ 17 S]`.

**Files:**
- Modify: `README.md`
- Modify: `cmd/tt/main.go`
- Modify: `config.example.toml`
- Modify: `internal/app/children.go`
- Modify: `internal/app/day.go`
- Modify: `internal/app/sources.go`
- Modify: `internal/app/tasks.go`
- Modify: `internal/config/config.go`
- Modify: `internal/domain/rules.go`
- Modify: `internal/domain/task.go`
- Modify: `internal/dsl/add.go`
- Modify: `internal/dsl/filter.go`
- Modify: `internal/search/search.go`
- Modify: `internal/store/tasks.go`
- Modify: `internal/tui/common.go`
- Modify: `internal/tui/edit.go`
- Modify: `internal/tui/inbox.go`
- Modify: `internal/tui/lane.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/people.go`
- Modify: `internal/tui/pick.go`
- Modify: `cmd/tt/main_test.go`
- Modify: `internal/app/children_test.go`
- Modify: `internal/app/day_test.go`
- Modify: `internal/app/pinsource_test.go`
- Modify: `internal/app/service_test.go`
- Modify: `internal/app/undo_test.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/config/github_test.go`
- Modify: `internal/domain/domain_test.go`
- Modify: `internal/dsl/add_test.go`
- Modify: `internal/dsl/filter_test.go`
- Modify: `internal/dsl/requester_test.go`
- Modify: `internal/search/search_test.go`
- Modify: `internal/store/migration006_test.go`
- Modify: `internal/store/store_test.go`
- Modify: `internal/tui/browse_test.go`
- Modify: `internal/tui/children_test.go`
- Modify: `internal/tui/colour_test.go`
- Modify: `internal/tui/detail_test.go`
- Modify: `internal/tui/edit_test.go`
- Modify: `internal/tui/filter_test.go`
- Modify: `internal/tui/filterline_test.go`
- Modify: `internal/tui/halfpage_test.go`
- Modify: `internal/tui/lane_test.go`
- Modify: `internal/tui/layout_test.go`
- Modify: `internal/tui/meetings_test.go`
- Modify: `internal/tui/notifications_test.go`
- Modify: `internal/tui/people_test.go`
- Modify: `internal/tui/pick_test.go`
- Modify: `internal/tui/place_test.go`
- Modify: `internal/tui/program_test.go`
- Modify: `internal/tui/rename_test.go`
- Modify: `internal/tui/reslot_test.go`
- Modify: `internal/tui/tab_test.go`
- Modify: `internal/tui/today_test.go`
- Modify: `internal/tui/tui_test.go`

**Interfaces:**
- Consumes: Task 1 (urgency.go, `Task.Impact`, `Task.Due`), Task 2 (`dueWord`, `ParseDue`, `Config.Urgency`, `ReviewImpact`, `ReviewDue`).
- Produces:
  - internal/dsl: `Draft.Impact int`, `Draft.Due domain.Day` (no `Priority`); `func ParseAdd(line string, today domain.Day) (Draft, error)`; `func ParseFilter(line string) (Filter, error)`; `Filter.Impacts []int` (no `Priorities`); `func parseImpact(tok string) (int, error)`.
  - internal/app: `func (s *Service) SetImpact(ctx context.Context, id int64, n int) (Undo, error)`; `func (s *Service) SetDue(ctx context.Context, id int64, due domain.Day) (Undo, error)`; `inheritedImpact(parentID int64, n int) error`.
  - internal/search: `ByUrgency`, `ByImpact`; `func (g GroupBy) Flat() bool`; `func Sorter(g GroupBy, parentOf func(domain.Task) *domain.Task, today domain.Day, w domain.Weights) func(a, b domain.Task) int`; `func GroupOf(by GroupBy, t domain.Task, now time.Time) Group` (`Group{}` for a flat sort).
  - internal/config: `func unknownKey(key string) error`.
  - internal/tui: `(*Model).urgency(t domain.Task, parent *domain.Task) (float64, bool)`; `func badgeOf(u float64, ok bool, size domain.Size) string`; `func sizeLabel(size domain.Size) string`.

- [ ] **Step 1: Write the failing tests**

Patch `cmd/tt/main_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/cmd/tt/main_test.go b/cmd/tt/main_test.go
index 35bd5db..b0ba853 100644
--- a/cmd/tt/main_test.go
+++ b/cmd/tt/main_test.go
@@ -59,7 +59,7 @@ func testEnv(t *testing.T) map[string]string {
 
 func TestAddReportsWhereTaskLanded(t *testing.T) {
 	env := testEnv(t)
-	if r := runTT(t, env, "add", "Reply to Ana @ana !1 ~s"); r.code != 0 || r.stdout != "added #1 to backlog\n" {
+	if r := runTT(t, env, "add", "Reply to Ana @ana !3 ~s"); r.code != 0 || r.stdout != "added #1 to backlog\n" {
 		t.Errorf("triaged add: %+v", r)
 	}
 	// Unquoted words are joined, as a shell would split them.
@@ -91,7 +91,7 @@ func TestUsageErrors(t *testing.T) {
 func TestAddErrorsExitNonZero(t *testing.T) {
 	env := testEnv(t)
 	if r := runTT(t, env, "add", "x !9"); r.code != 1 || !strings.Contains(r.stderr, "use !1 to !3") {
-		t.Errorf("bad priority: %+v", r)
+		t.Errorf("bad impact: %+v", r)
 	}
 	if r := runTT(t, env, "add", "@ana #only-tokens"); r.code != 1 || !strings.Contains(r.stderr, "title is required") {
 		t.Errorf("no title: %+v", r)
@@ -100,11 +100,18 @@ func TestAddErrorsExitNonZero(t *testing.T) {
 
 func TestConfigIsHonoured(t *testing.T) {
 	env := testEnv(t)
+	// A config from before D68 says what replaced the P-levels (D76).
 	if err := os.WriteFile(env["TT_CONFIG"], []byte("[[priorities]]\nname = \"Now\"\n"), 0o644); err != nil {
 		t.Fatal(err)
 	}
-	if r := runTT(t, env, "add", "x !2"); r.code != 1 || !strings.Contains(r.stderr, "use !1 to !1") {
-		t.Errorf("one configured level: %+v", r)
+	if r := runTT(t, env, "add", "x !2"); r.code != 1 || !strings.Contains(r.stderr, "[[priorities]] was replaced by impact (D68): remove it, see config.example.toml") {
+		t.Errorf("an old config: %+v", r)
+	}
+	if err := os.WriteFile(env["TT_CONFIG"], []byte("[urgency]\ndue = 3\n"), 0o644); err != nil {
+		t.Fatal(err)
+	}
+	if r := runTT(t, env, "add", "x !2 ^tom"); r.code != 0 {
+		t.Errorf("weights from the config: %+v", r)
 	}
 	if err := os.WriteFile(env["TT_CONFIG"], []byte("bogus = 1\n"), 0o644); err != nil {
 		t.Fatal(err)
@@ -280,7 +287,7 @@ func TestAddFromAPipeOverTheLimit(t *testing.T) {
 
 func TestAddFromAPipe(t *testing.T) {
 	env := testEnv(t)
-	in := input{r: strings.NewReader("Look at the schema !1 ~m -- from Slack\r\nAna: can you check\r\n\r\nthe migration?\r\n")}
+	in := input{r: strings.NewReader("Look at the schema !3 ~m -- from Slack\r\nAna: can you check\r\n\r\nthe migration?\r\n")}
 	if r := runWithInput(t, env, in, "add"); r.code != 0 || r.stdout != "added #1 to backlog\n" || r.stderr != "" {
 		t.Fatalf("piped add: %+v", r)
 	}
```

Patch `internal/app/children_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/children_test.go b/internal/app/children_test.go
index 8b52a18..fdc598e 100644
--- a/internal/app/children_test.go
+++ b/internal/app/children_test.go
@@ -3,6 +3,7 @@ package app
 import (
 	"context"
 	"errors"
+	"fmt"
 	"slices"
 	"strings"
 	"testing"
@@ -56,12 +57,12 @@ func TestAddChild(t *testing.T) {
 		t.Fatal(err)
 	}
 	got := mustGet(t, s, a.ID)
-	if got.ParentID != parent.ID || got.Size != domain.SizeS || got.Note != "try the new lib" || got.Priority != 0 ||
+	if got.ParentID != parent.ID || got.Size != domain.SizeS || got.Note != "try the new lib" || got.Impact != 0 ||
 		!slices.Equal(got.Tags, []string{"ops"}) {
 		t.Errorf("child: %+v", got)
 	}
 	if domain.InInbox(got, &parent) {
-		t.Error("a sized child inherits priority and skips the Inbox")
+		t.Error("a sized child inherits impact and skips the Inbox")
 	}
 	if order := childOrder(t, s, parent.ID); !slices.Equal(order, []int64{a.ID, b.ID}) {
 		t.Errorf("children appended last: %v", order)
@@ -81,7 +82,7 @@ func TestAddChildRules(t *testing.T) {
 	child := mustAddChild(t, s, parent.ID, "Child ~s")
 
 	_, _, err := s.AddChild(ctx, parent.ID, "Urgent bit !1 ~s")
-	if !errors.Is(err, ErrInvalidValue) || !strings.Contains(err.Error(), "children inherit priority from #1; drop !1") {
+	if !errors.Is(err, ErrInvalidValue) || !strings.Contains(err.Error(), "children inherit impact from #1; drop !1") {
 		t.Errorf("!n on a child: %v", err)
 	}
 	if _, _, err := s.AddChild(ctx, child.ID, "Grandchild ~s"); !errors.Is(err, ErrInvalidParent) {
@@ -149,32 +150,46 @@ func TestReorder(t *testing.T) {
 	}
 }
 
-func TestChildPriorityIsRefusedInEdits(t *testing.T) {
+func TestChildImpactIsRefusedInEdits(t *testing.T) {
 	must := mustUndo(t)
 	s, clock := newService(t)
 	ctx := context.Background()
-	parent := mustAdd(t, s, "Parent !1 ~l")
-	child := mustAdd(t, s, "Child !3 ~s") // own priority from before it became a child
+	parent := mustAdd(t, s, "Parent !3 ^fri ~l")
+	child := mustAdd(t, s, "Child !1 ~s") // own impact from before it became a child
 	clock.Advance(time.Minute)
 	must(s.SetParent(ctx, child.ID, parent.ID))
 	got := mustGet(t, s, child.ID)
 
 	if line := EditLine(got); line != "Child ~s" {
-		t.Errorf("EditLine of a child = %q; want no priority (D32)", line)
+		t.Errorf("EditLine of a child = %q; want no impact, and no inherited due (D32, D73)", line)
 	}
-	if line := EditLine(mustGet(t, s, parent.ID)); line != "Parent !1 ~l" {
+	if line := EditLine(mustGet(t, s, parent.ID)); line != "Parent !3 ^2026-10-16 ~l" {
 		t.Errorf("EditLine of a parent = %q", line)
 	}
-	if _, err := s.ParseEdit(got, "Child !2 ~s"); !errors.Is(err, ErrInvalidValue) {
+	if _, err := s.ParseEdit(got, "Child !2 ~s"); !errors.Is(err, ErrInvalidValue) ||
+		!strings.Contains(err.Error(), fmt.Sprintf("children inherit impact from #%d", parent.ID)) {
 		t.Errorf("ParseEdit with !n on a child: %v", err)
 	}
 	if _, err := s.Edit(ctx, child.ID, "Child !2 ~s"); !errors.Is(err, ErrInvalidValue) {
 		t.Errorf("Edit with !n on a child: %v", err)
 	}
+	if _, err := s.SetImpact(ctx, child.ID, 2); !errors.Is(err, ErrInvalidValue) {
+		t.Errorf("SetImpact on a child: %v; want ErrInvalidValue", err)
+	}
 	clock.Advance(time.Minute)
-	must(s.Edit(ctx, child.ID, "Child renamed ~m"))
-	if got := mustGet(t, s, child.ID); got.Title != "Child renamed" || got.Size != domain.SizeM {
-		t.Errorf("edited child: %+v", got)
+	must(s.Edit(ctx, child.ID, "Child renamed ^tom ~m"))
+	if got := mustGet(t, s, child.ID); got.Title != "Child renamed" || got.Size != domain.SizeM || got.Due != "2026-10-13" {
+		t.Errorf("edited child: %+v; want its own due", got)
+	}
+	if line := EditLine(mustGet(t, s, child.ID)); line != "Child renamed ^2026-10-13 ~m" {
+		t.Errorf("EditLine of a child with its own due = %q", line)
+	}
+	kid := mustAddChild(t, s, parent.ID, "Kid ^3d ~s")
+	if got := mustGet(t, s, kid.ID); got.Due != "2026-10-15" || got.Impact != 0 {
+		t.Errorf("AddChild with ^3d: %+v", got)
+	}
+	if _, _, err := s.AddChild(ctx, parent.ID, "Kid !2 ~s"); !errors.Is(err, ErrInvalidValue) {
+		t.Errorf("AddChild with !n: %v", err)
 	}
 }
 
```

Patch `internal/app/day_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/day_test.go b/internal/app/day_test.go
index 43ed5be..8979b27 100644
--- a/internal/app/day_test.go
+++ b/internal/app/day_test.go
@@ -180,8 +180,8 @@ func TestPlanOrderAndAlert(t *testing.T) {
 	ctx := context.Background()
 	unset := mustAdd(t, s, "No priority")
 	p2 := mustAdd(t, s, "Second !2 ~s")
-	p1 := mustAdd(t, s, "First !1 ~s")
-	parent := mustAdd(t, s, "Parent !1 ~xl")
+	p1 := mustAdd(t, s, "First !3 ~s")
+	parent := mustAdd(t, s, "Parent !3 ~xl")
 	child := mustAdd(t, s, "Child ~s")
 	if _, err := s.SetParent(ctx, child.ID, parent.ID); err != nil {
 		t.Fatal(err)
@@ -201,7 +201,8 @@ func TestPlanOrderAndAlert(t *testing.T) {
 		ids = append(ids, c.Task.ID)
 		alerts = append(alerts, c.Alert)
 	}
-	// child inherits P1 from its parent, so it sorts with p1 (by id).
+	// Most urgent first (D72): child inherits impact 3 from its parent, so it
+	// sorts with p1 (by id); the untriaged one last.
 	if diff := cmp.Diff([]int64{p1.ID, child.ID, p2.ID, unset.ID}, ids); diff != "" {
 		t.Errorf("candidate order (-want +got):\n%s", diff)
 	}
```

Patch `internal/app/pinsource_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/pinsource_test.go b/internal/app/pinsource_test.go
index 486a50b..ae4731d 100644
--- a/internal/app/pinsource_test.go
+++ b/internal/app/pinsource_test.go
@@ -32,8 +32,8 @@ func TestPinSourceCreatesTheTask(t *testing.T) {
 	got := mustGet(t, s, task.ID)
 	if got.Title != "Review #901: Add idempotency key" || got.Link != "https://github.com/acme/api/pull/901" ||
 		got.Source == nil || *got.Source != (domain.SourceRef{Kind: "github", ID: "acme/api#901"}) ||
-		got.Priority != 2 || got.Size != domain.SizeS || got.Requester != "" || got.Status != domain.StatusOpen {
-		t.Errorf("task = %+v; want D20b's wrapper with D48's P2 S and no requester", got)
+		got.Impact != 1 || got.Due != "2026-10-15" || got.Size != domain.SizeS || got.Requester != "" || got.Status != domain.StatusOpen {
+		t.Errorf("task = %+v; want D20b's wrapper with D75's impact 1, due 3 days after the request, S and no requester", got)
 	}
 	if !pinned(t, s, task.ID) {
 		t.Error("the review is a candidate now")
@@ -59,7 +59,7 @@ func TestPinSourceUsesTheConfiguredDefaults(t *testing.T) {
 		t.Fatal(err)
 	}
 	t.Cleanup(func() { st.Close() })
-	cfg, err := config.Parse([]byte("[github]\npriority = \"P1\"\nsize = \"\"\n"))
+	cfg, err := config.Parse([]byte("[github]\nimpact = 2\ndue = \"0d\"\nsize = \"\"\n"))
 	if err != nil {
 		t.Fatal(err)
 	}
@@ -68,8 +68,17 @@ func TestPinSourceUsesTheConfiguredDefaults(t *testing.T) {
 	src.Set([]domain.SourceItem{review(901, "Add idempotency key", time.Time{})}, nil)
 	mustSyncSources(t, s)
 	task, _ := mustPinSource(t, s, "acme/api#901")
-	if got := mustGet(t, s, task.ID); got.Priority != 1 || got.Size != domain.SizeNone {
-		t.Errorf("task = P%d %q; want P1 unsized", got.Priority, got.Size)
+	if got := mustGet(t, s, task.ID); got.Impact != 2 || got.Due != "2026-10-12" || got.Size != domain.SizeNone {
+		t.Errorf("task = impact %d, due %q, %q; want 2, due the day it was first seen (no request time), unsized", got.Impact, got.Due, got.Size)
+	}
+
+	cfg, _ = config.Parse([]byte("[github]\ndue = \"\"\n"))
+	s = New(st, &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}, cfg).WithSource(src)
+	src.Set([]domain.SourceItem{review(877, "Bump go", time.Time{})}, nil)
+	mustSyncSources(t, s)
+	task, _ = mustPinSource(t, s, "acme/api#877")
+	if got := mustGet(t, s, task.ID); got.Due != "" {
+		t.Errorf("due = \"\": task due %q; want none", got.Due)
 	}
 }
 
```

Patch `internal/app/service_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/service_test.go b/internal/app/service_test.go
index 10533d2..cc9abd1 100644
--- a/internal/app/service_test.go
+++ b/internal/app/service_test.go
@@ -68,14 +68,14 @@ func TestAddAndUndo(t *testing.T) {
 	s, _ := newService(t)
 	ctx := context.Background()
 
-	task, undo, err := s.Add(ctx, "Reply to Ana @ana #photos !1 ~s https://x.io -- before Friday")
+	task, undo, err := s.Add(ctx, "Reply to Ana @ana #photos !1 ^fri ~s https://x.io -- before Friday")
 	if err != nil {
 		t.Fatal(err)
 	}
 	got := mustGet(t, s, task.ID)
 	want := domain.Task{
 		ID: task.ID, Title: "Reply to Ana", Note: "before Friday", Link: "https://x.io", Requester: "ana",
-		Tags: []string{"photos"}, Priority: 1, Size: domain.SizeS, Status: domain.StatusOpen,
+		Tags: []string{"photos"}, Impact: 1, Due: "2026-10-16", Size: domain.SizeS, Status: domain.StatusOpen,
 		CreatedAt: got.CreatedAt, UpdatedAt: got.UpdatedAt,
 	}
 	if diff := cmp.Diff(want, got); diff != "" {
@@ -107,16 +107,16 @@ func TestEditReplacesFieldsAndKeepsNote(t *testing.T) {
 	must := mustUndo(t)
 	s, clock := newService(t)
 	ctx := context.Background()
-	task := mustAdd(t, s, "Old title @ana #a !1 ~s -- keep me")
+	task := mustAdd(t, s, "Old title @ana #a !1 ^tom ~s -- keep me")
 
-	if line := EditLine(mustGet(t, s, task.ID)); line != "Old title @ana #a !1 ~s" {
+	if line := EditLine(mustGet(t, s, task.ID)); line != "Old title @ana #a !1 ^2026-10-13 ~s" {
 		t.Errorf("EditLine = %q", line)
 	}
 
 	clock.Advance(time.Minute)
 	undo := must(s.Edit(ctx, task.ID, "New title #b ~l"))
 	got := mustGet(t, s, task.ID)
-	if got.Title != "New title" || got.Requester != "" || got.Priority != 0 || got.Size != domain.SizeL ||
+	if got.Title != "New title" || got.Requester != "" || got.Impact != 0 || got.Due != "" || got.Size != domain.SizeL ||
 		!cmp.Equal(got.Tags, []string{"b"}) || got.Note != "keep me" {
 		t.Errorf("after Edit: %+v", got)
 	}
@@ -154,22 +154,39 @@ func TestTriageSetters(t *testing.T) {
 	ctx := context.Background()
 	task := mustAdd(t, s, "Triage me")
 
-	must(s.SetPriority(ctx, task.ID, 2))
+	must(s.SetImpact(ctx, task.ID, 2))
 	undo := must(s.SetSize(ctx, task.ID, domain.SizeXL))
 	got := mustGet(t, s, task.ID)
-	if got.Priority != 2 || got.Size != domain.SizeXL || domain.InInbox(got, nil) {
+	if got.Impact != 2 || got.Size != domain.SizeXL || domain.InInbox(got, nil) {
 		t.Errorf("after triage: %+v", got)
 	}
 	if err := undo(ctx); err != nil {
 		t.Fatal(err)
 	}
-	if got := mustGet(t, s, task.ID); got.Size != domain.SizeNone || got.Priority != 2 {
+	if got := mustGet(t, s, task.ID); got.Size != domain.SizeNone || got.Impact != 2 {
 		t.Errorf("undo of SetSize: %+v", got)
 	}
+	undo = must(s.SetDue(ctx, task.ID, "2026-10-30"))
+	if got := mustGet(t, s, task.ID); got.Due != "2026-10-30" {
+		t.Errorf("after SetDue: %+v", got)
+	}
+	if err := undo(ctx); err != nil {
+		t.Fatal(err)
+	}
+	must(s.SetDue(ctx, task.ID, "2026-10-30"))
+	must(s.SetDue(ctx, task.ID, ""))
+	if got := mustGet(t, s, task.ID); got.Due != "" {
+		t.Errorf("SetDue(\"\") clears it: %+v", got)
+	}
 
-	for _, p := range []int{-1, 4} {
-		if _, err := s.SetPriority(ctx, task.ID, p); !errors.Is(err, ErrInvalidValue) {
-			t.Errorf("SetPriority(%d) error = %v; want ErrInvalidValue", p, err)
+	for _, n := range []int{-1, 4} {
+		if _, err := s.SetImpact(ctx, task.ID, n); !errors.Is(err, ErrInvalidValue) {
+			t.Errorf("SetImpact(%d) error = %v; want ErrInvalidValue", n, err)
+		}
+	}
+	for _, due := range []domain.Day{"fri", "2026-02-30"} {
+		if _, err := s.SetDue(ctx, task.ID, due); !errors.Is(err, ErrInvalidValue) {
+			t.Errorf("SetDue(%q) error = %v; want ErrInvalidValue", due, err)
 		}
 	}
 	for _, size := range []domain.Size{"x", "xs"} {
@@ -177,8 +194,8 @@ func TestTriageSetters(t *testing.T) {
 			t.Errorf("SetSize(%q) error = %v; want ErrInvalidValue", size, err)
 		}
 	}
-	if _, err := s.SetPriority(ctx, 999, 1); !errors.Is(err, ErrNotFound) {
-		t.Errorf("SetPriority on missing task error = %v; want ErrNotFound", err)
+	if _, err := s.SetImpact(ctx, 999, 1); !errors.Is(err, ErrNotFound) {
+		t.Errorf("SetImpact on missing task error = %v; want ErrNotFound", err)
 	}
 }
 
@@ -290,3 +307,18 @@ func TestCloseKeepsExistingClosedAt(t *testing.T) {
 		t.Errorf("Done then Drop: %+v; want dropped with ClosedAt %v", got, first)
 	}
 }
+
+// Review Focus: ^dates count from the planning day (D19, D73), so at 02:00
+// with day_start 04:00 "today" is still yesterday's date.
+func TestDueDatesFollowThePlanningDay(t *testing.T) {
+	s, clock := newService(t)
+	clock.now = time.Date(2026, 10, 13, 2, 0, 0, 0, time.UTC) // Tue 02:00: still Monday's planning day
+	task := mustAdd(t, s, "Late night ^tom !1 ~s")
+	if got := mustGet(t, s, task.ID).Due; got != "2026-10-13" {
+		t.Errorf("^tom at 02:00 = %q; want the day after Monday's planning day", got)
+	}
+	task = mustAdd(t, s, "Same night ^today !1 ~s")
+	if got := mustGet(t, s, task.ID).Due; got != "2026-10-12" {
+		t.Errorf("^today at 02:00 = %q; want Monday", got)
+	}
+}
```

Patch `internal/app/undo_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/undo_test.go b/internal/app/undo_test.go
index 1ed820e..be55c83 100644
--- a/internal/app/undo_test.go
+++ b/internal/app/undo_test.go
@@ -45,13 +45,13 @@ func TestUndoAddRefusesWhenTaskChangedSince(t *testing.T) {
 		t.Fatal(err)
 	}
 	clock.Advance(time.Minute)
-	if _, err := s.SetPriority(ctx, task.ID, 2); err != nil {
+	if _, err := s.SetImpact(ctx, task.ID, 2); err != nil {
 		t.Fatal(err)
 	}
 	if err := addUndo(ctx); !errors.Is(err, ErrConflict) {
 		t.Errorf("undo Add after a later change: error = %v; want ErrConflict", err)
 	}
-	if got := mustGet(t, s, task.ID); got.Priority != 2 {
+	if got := mustGet(t, s, task.ID); got.Impact != 2 {
 		t.Errorf("task must survive a refused undo: %+v", got)
 	}
 }
```

Patch `internal/config/config_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/config/config_test.go b/internal/config/config_test.go
index 0652aa4..64fedbc 100644
--- a/internal/config/config_test.go
+++ b/internal/config/config_test.go
@@ -14,7 +14,7 @@ func TestLoadMissingFileGivesDefaults(t *testing.T) {
 	if err != nil {
 		t.Fatal(err)
 	}
-	if cfg.DayStart != 4*time.Hour || len(cfg.Priorities) != 3 || cfg.Sizes[domain.SizeXL] != 40*time.Hour {
+	if cfg.DayStart != 4*time.Hour || cfg.Sizes[domain.SizeXL] != 40*time.Hour {
 		t.Errorf("unexpected defaults: %+v", cfg)
 	}
 }
@@ -23,13 +23,6 @@ func TestParseOverrides(t *testing.T) {
 	cfg, err := Parse([]byte(`
 day_start = "00:00"
 
-[[priorities]]
-name = "Now"
-definition = "today or tomorrow"
-
-[[priorities]]
-name = "Later"
-
 [sizes]
 m = "3h"
 `))
@@ -39,24 +32,34 @@ m = "3h"
 	if cfg.DayStart != 0 {
 		t.Errorf("DayStart = %v; want 0", cfg.DayStart)
 	}
-	if len(cfg.Priorities) != 2 || cfg.Priorities[0].Name != "Now" {
-		t.Errorf("Priorities = %+v", cfg.Priorities)
-	}
 	if cfg.Sizes[domain.SizeM] != 3*time.Hour || cfg.Sizes[domain.SizeS] != time.Hour {
 		t.Errorf("Sizes = %v; want m overridden, s default", cfg.Sizes)
 	}
 }
 
+// The keys of the P-levels say what replaced them instead of "unknown
+// config key", so an old config.toml tells its owner what to do (D76).
+func TestPriorityKeysAreGone(t *testing.T) {
+	cases := map[string]string{
+		"[[priorities]]\nname = \"P1\"\ndefinition = \"x\"": "[[priorities]] was replaced by impact (D68): remove it, see config.example.toml",
+		"priorities = []":             "[[priorities]] was replaced by impact (D68)",
+		"[github]\npriority = \"P2\"": "[github] priority was replaced by impact and due (D75): remove it, see config.example.toml",
+	}
+	for input, want := range cases {
+		if _, err := Parse([]byte(input)); err == nil || !strings.HasPrefix(err.Error(), want) {
+			t.Errorf("Parse(%q) error = %v; want %q", input, err, want)
+		}
+	}
+}
+
 func TestParseErrors(t *testing.T) {
 	cases := map[string]string{
-		`day_start = "4am"`:                  "day_start",
-		`day_start = "24:00"`:                "day_start",
-		`priorities = []`:                    "1 to 9 levels",
-		"[[priorities]]\ndefinition = \"x\"": "name is required",
-		"[sizes]\nxs = \"1h\"":               "unknown size",
-		"[sizes]\nx = \"1h\"":                "unknown size",
-		"[sizes]\ns = \"soon\"":              "positive duration",
-		`daystart = "04:00"`:                 "unknown config key",
+		`day_start = "4am"`:     "day_start",
+		`day_start = "24:00"`:   "day_start",
+		"[sizes]\nxs = \"1h\"":  "unknown size",
+		"[sizes]\nx = \"1h\"":   "unknown size",
+		"[sizes]\ns = \"soon\"": "positive duration",
+		`daystart = "04:00"`:    "unknown config key",
 	}
 	for input, wantErr := range cases {
 		_, err := Parse([]byte(input))
```

Patch `internal/config/github_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/config/github_test.go b/internal/config/github_test.go
index 9d5374e..dcd5862 100644
--- a/internal/config/github_test.go
+++ b/internal/config/github_test.go
@@ -10,16 +10,16 @@ import (
 
 func TestParseGitHub(t *testing.T) {
 	cfg := Default()
-	if !cfg.GitHub || cfg.GitHubHost != "" || cfg.GitHubPoll != 5*time.Minute || cfg.ReviewPriority != 2 || cfg.ReviewSize != domain.SizeS {
-		t.Errorf("defaults: %v %q %v P%d %q; want on, gh's host, 5m, P2, S (D48, D50)",
-			cfg.GitHub, cfg.GitHubHost, cfg.GitHubPoll, cfg.ReviewPriority, cfg.ReviewSize)
+	if !cfg.GitHub || cfg.GitHubHost != "" || cfg.GitHubPoll != 5*time.Minute || cfg.ReviewSize != domain.SizeS {
+		t.Errorf("defaults: %v %q %v %q; want on, gh's host, 5m, S (D48, D50)",
+			cfg.GitHub, cfg.GitHubHost, cfg.GitHubPoll, cfg.ReviewSize)
 	}
-	cfg, err := Parse([]byte("[github]\nenabled = false\nhost = \" github.acme.com \"\npoll = \"30s\"\npriority = \"p1\"\nsize = \"\"\n"))
+	cfg, err := Parse([]byte("[github]\nenabled = false\nhost = \" github.acme.com \"\npoll = \"30s\"\nsize = \"\"\n"))
 	if err != nil {
 		t.Fatal(err)
 	}
-	if cfg.GitHub || cfg.GitHubHost != "github.acme.com" || cfg.GitHubPoll != 30*time.Second || cfg.ReviewPriority != 1 || cfg.ReviewSize != domain.SizeNone {
-		t.Errorf("github: %v %q %v P%d %q", cfg.GitHub, cfg.GitHubHost, cfg.GitHubPoll, cfg.ReviewPriority, cfg.ReviewSize)
+	if cfg.GitHub || cfg.GitHubHost != "github.acme.com" || cfg.GitHubPoll != 30*time.Second || cfg.ReviewSize != domain.SizeNone {
+		t.Errorf("github: %v %q %v %q", cfg.GitHub, cfg.GitHubHost, cfg.GitHubPoll, cfg.ReviewSize)
 	}
 	if cfg, err := Parse([]byte("[github]\nsize = \"xl\"\n")); err != nil || cfg.ReviewSize != domain.SizeXL {
 		t.Errorf("size xl: %q, %v", cfg.ReviewSize, err)
@@ -28,28 +28,13 @@ func TestParseGitHub(t *testing.T) {
 		t.Errorf("size \" m \": %q, %v", cfg.ReviewSize, err)
 	}
 
-	// Other priority levels: the default follows the level named P2, else the last.
-	two := "[[priorities]]\nname = \"Now\"\n[[priorities]]\nname = \"P2\"\n[[priorities]]\nname = \"Later\"\n[[priorities]]\nname = \"Never\"\n"
-	if cfg, err := Parse([]byte(two)); err != nil || cfg.ReviewPriority != 2 {
-		t.Errorf("a level named P2: P%d, %v", cfg.ReviewPriority, err)
-	}
-	none := "[[priorities]]\nname = \"Now\"\n[[priorities]]\nname = \"Soon\"\n[[priorities]]\nname = \"Later\"\n"
-	if cfg, err := Parse([]byte(none)); err != nil || cfg.ReviewPriority != 3 {
-		t.Errorf("no level named P2: P%d, %v; want the last level", cfg.ReviewPriority, err)
-	}
-	if cfg, err := Parse([]byte(none + "[github]\npriority = \"Soon\"\n")); err != nil || cfg.ReviewPriority != 2 {
-		t.Errorf("priority Soon: P%d, %v", cfg.ReviewPriority, err)
-	}
-
 	cases := map[string]string{
-		"[github]\npoll = \"10s\"":           "github.poll: want a duration of 30s or more",
-		"[github]\npoll = \"5\"":             "github.poll: want a duration of 30s or more",
-		"[github]\npriority = \"P7\"":        `github.priority: no priority level named "P7"`,
-		none + "[github]\npriority = \"P2\"": `github.priority: no priority level named "P2"`,
-		"[github]\nsize = \"huge\"":          "github.size: want s, m, l, xl, or \"\" for unsized",
-		"[github]\nsize = \"x\"":             "github.size: want s, m, l, xl",
-		"[github]\nenabled = \"yes\"":        "enabled",
-		"[github]\ntoken = \"x\"":            "unknown config key",
+		"[github]\npoll = \"10s\"":    "github.poll: want a duration of 30s or more",
+		"[github]\npoll = \"5\"":      "github.poll: want a duration of 30s or more",
+		"[github]\nsize = \"huge\"":   "github.size: want s, m, l, xl, or \"\" for unsized",
+		"[github]\nsize = \"x\"":      "github.size: want s, m, l, xl",
+		"[github]\nenabled = \"yes\"": "enabled",
+		"[github]\ntoken = \"x\"":     "unknown config key",
 	}
 	for input, wantErr := range cases {
 		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), wantErr) {
```

Patch `internal/domain/domain_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/domain/domain_test.go b/internal/domain/domain_test.go
index 4520f73..07fe992 100644
--- a/internal/domain/domain_test.go
+++ b/internal/domain/domain_test.go
@@ -33,22 +33,16 @@ func TestNormalizeTags(t *testing.T) {
 }
 
 func TestEffectiveFields(t *testing.T) {
-	parent := Task{ID: 1, Priority: 1, Tags: []string{"photos"}}
-	child := Task{ID: 2, ParentID: 1, Priority: 3, Tags: []string{"impl"}}
+	parent := Task{ID: 1, Impact: 1, Tags: []string{"photos"}}
+	child := Task{ID: 2, ParentID: 1, Tags: []string{"impl"}}
 
-	if got := EffectivePriority(child, &parent); got != 1 {
-		t.Errorf("child priority = %d; want parent's 1", got)
-	}
-	if got := EffectivePriority(child, nil); got != 3 {
-		t.Errorf("top-level priority = %d; want own 3", got)
-	}
 	if diff := cmp.Diff([]string{"impl", "photos"}, EffectiveTags(child, &parent)); diff != "" {
 		t.Errorf("child tags (-want +got):\n%s", diff)
 	}
 }
 
 func TestInInbox(t *testing.T) {
-	triagedParent := &Task{Priority: 2}
+	triagedParent := &Task{ID: 1, Impact: 2}
 	cases := []struct {
 		name   string
 		task   Task
@@ -56,11 +50,14 @@ func TestInInbox(t *testing.T) {
 		want   bool
 	}{
 		{"title only", Task{Status: StatusOpen}, nil, true},
-		{"priority only", Task{Status: StatusOpen, Priority: 1}, nil, true},
+		{"impact only", Task{Status: StatusOpen, Impact: 1}, nil, true},
 		{"size only", Task{Status: StatusOpen, Size: SizeS}, nil, true},
-		{"priority and size", Task{Status: StatusOpen, Priority: 1, Size: SizeS}, nil, false},
-		{"child with size inherits priority", Task{Status: StatusOpen, Size: SizeM}, triagedParent, false},
-		{"child of untriaged parent", Task{Status: StatusOpen, Size: SizeM}, &Task{}, true},
+		{"impact and size", Task{Status: StatusOpen, Impact: 1, Size: SizeS}, nil, false},
+		{"a due doesn't count", Task{Status: StatusOpen, Due: "2026-10-16", Size: SizeS}, nil, true},
+		{"child with size inherits impact", Task{Status: StatusOpen, ParentID: 1, Size: SizeM}, triagedParent, false},
+		{"child of untriaged parent: the parent is triaged instead (D74)", Task{Status: StatusOpen, ParentID: 1, Size: SizeM}, &Task{ID: 1}, false},
+		{"child without a size", Task{Status: StatusOpen, ParentID: 1}, triagedParent, true},
+		{"child without a size, untriaged parent", Task{Status: StatusOpen, ParentID: 1}, &Task{ID: 1}, true},
 		{"done", Task{Status: StatusDone}, nil, false},
 		{"source-linked, unsized", Task{Status: StatusOpen, Source: &SourceRef{Kind: "github", ID: "a/b#1"}}, nil, false},
 	}
@@ -75,7 +72,7 @@ func TestSnapshotInboxOrderAndParent(t *testing.T) {
 	t0 := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
 	snap := NewSnapshot([]Task{
 		{ID: 3, Status: StatusOpen, CreatedAt: t0.Add(2 * time.Hour)},
-		{ID: 1, Status: StatusOpen, Priority: 1, Size: SizeXL, CreatedAt: t0},
+		{ID: 1, Status: StatusOpen, Impact: 1, Size: SizeXL, CreatedAt: t0},
 		{ID: 2, Status: StatusOpen, CreatedAt: t0.Add(time.Hour)},
 		{ID: 4, Status: StatusOpen, ParentID: 1, Size: SizeS, CreatedAt: t0},
 	})
```

Patch `internal/dsl/add_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/dsl/add_test.go b/internal/dsl/add_test.go
index 4d482d9..5a8ba4f 100644
--- a/internal/dsl/add_test.go
+++ b/internal/dsl/add_test.go
@@ -19,7 +19,7 @@ func TestParseAdd(t *testing.T) {
 		{
 			"Reply to Ana re schema @Ana #photos #Impl !1 ~s https://x.slack.com/archives/C1/p2",
 			Draft{Title: "Reply to Ana re schema", Requester: "ana", Tags: []string{"impl", "photos"},
-				Priority: 1, Size: domain.SizeS, Link: "https://x.slack.com/archives/C1/p2"},
+				Impact: 1, Size: domain.SizeS, Link: "https://x.slack.com/archives/C1/p2"},
 		},
 		{"tokens #a anywhere ~xl in @bob the line", Draft{Title: "tokens anywhere in the line", Requester: "bob", Tags: []string{"a"}, Size: domain.SizeXL}},
 		{"~X alias", Draft{Title: "alias", Size: domain.SizeXL}},
@@ -40,7 +40,12 @@ func TestParseAdd(t *testing.T) {
 		{"Ping @ana, about schema", Draft{Title: "Ping about schema", Requester: "ana"}},
 		{"Ask @ana.", Draft{Title: "Ask", Requester: "ana"}},
 		{"Tag it #photos.", Draft{Title: "Tag it", Tags: []string{"photos"}}},
-		{"x !1.", Draft{Title: "x", Priority: 1}},
+		{"x !1.", Draft{Title: "x", Impact: 1}},
+		// Due dates (D73).
+		{"Ship it ^fri !3", Draft{Title: "Ship it", Due: "2026-10-16", Impact: 3}},
+		{"last ^tom date ^2026-11-02 wins", Draft{Title: "last date wins", Due: "2026-11-02"}},
+		{"Send it ^fri, then rest", Draft{Title: "Send it then rest", Due: "2026-10-16"}},
+		{"x^2 and ^_^ and ^ and ^someday stay", Draft{Title: "x^2 and ^_^ and ^ and ^someday stay"}},
 		// Notes.
 		{"Call Ivan -- he needs it before Friday's sync", Draft{Title: "Call Ivan", Note: "he needs it before Friday's sync", HasNote: true}},
 		{"Ping #infra -- see https://x.io #not-a-tag !1 -- twice", Draft{Title: "Ping", Tags: []string{"infra"}, Note: "see https://x.io #not-a-tag !1 -- twice", HasNote: true}},
@@ -49,7 +54,7 @@ func TestParseAdd(t *testing.T) {
 		{"en--dash stays", Draft{Title: "en--dash stays"}},
 	}
 	for _, c := range cases {
-		got, err := ParseAdd(c.line, 3)
+		got, err := ParseAdd(c.line, wednesday)
 		if err != nil {
 			t.Errorf("ParseAdd(%q) error: %v", c.line, err)
 			continue
@@ -65,14 +70,19 @@ func tagsEqual(a, b []string) bool { return strings.Join(a, ",") == strings.Join
 
 func TestParseAddErrors(t *testing.T) {
 	for _, line := range []string{"", "   ", "@ana #x !1 ~s", "https://only.a.link", "-- just a note"} {
-		if _, err := ParseAdd(line, 3); !errors.Is(err, ErrEmptyTitle) {
+		if _, err := ParseAdd(line, wednesday); !errors.Is(err, ErrEmptyTitle) {
 			t.Errorf("ParseAdd(%q) error = %v; want ErrEmptyTitle", line, err)
 		}
 	}
 	for _, line := range []string{"x !0", "x !4", "x !99999999999999999999"} {
-		_, err := ParseAdd(line, 3)
-		if err == nil || !strings.Contains(err.Error(), "use !1 to !3") {
-			t.Errorf("ParseAdd(%q) error = %v; want priority range error", line, err)
+		_, err := ParseAdd(line, wednesday)
+		if err == nil || !strings.Contains(err.Error(), "out of range: use !1 to !3") || !strings.HasPrefix(err.Error(), "impact !") {
+			t.Errorf("ParseAdd(%q) error = %v; want the impact range error", line, err)
+		}
+	}
+	for _, line := range []string{"x ^31feb", "x ^2026-13-01"} {
+		if _, err := ParseAdd(line, wednesday); err == nil || !strings.Contains(err.Error(), "no such date") {
+			t.Errorf("ParseAdd(%q) error = %v; want no such date", line, err)
 		}
 	}
 }
@@ -80,14 +90,15 @@ func TestParseAddErrors(t *testing.T) {
 func TestFormatAddRoundTrip(t *testing.T) {
 	drafts := []Draft{
 		{Title: "Just a title"},
-		{Title: "Review PR #482", Requester: "ana", Tags: []string{"impl", "photos"}, Priority: 2, Size: domain.SizeXL, Link: "https://x.io/a?b=c#d"},
+		{Title: "Review PR #482", Requester: "ana", Tags: []string{"impl", "photos"}, Impact: 2, Due: "2026-10-30", Size: domain.SizeXL, Link: "https://x.io/a?b=c#d"},
+		{Title: "Overdue", Due: "2026-01-02"},
 		{Title: "With note", Note: "multi -- separators", HasNote: true},
 		{Title: "Empty note", HasNote: true},
 		{Title: "two links https://b.io", Link: "https://a.io"},
 	}
 	for _, d := range drafts {
 		line := FormatAdd(d)
-		got, err := ParseAdd(line, 3)
+		got, err := ParseAdd(line, wednesday)
 		if err != nil {
 			t.Errorf("ParseAdd(FormatAdd(%+v)) = %q: %v", d, line, err)
 			continue
@@ -115,7 +126,7 @@ func TestWithNoteLines(t *testing.T) {
 		{"Ship it", []string{"#not-a-tag !1 @nobody"}, Draft{Title: "Ship it", Note: "#not-a-tag !1 @nobody", HasNote: true}},
 	}
 	for _, c := range cases {
-		got, err := ParseAdd(WithNoteLines(c.line, c.rest), 3)
+		got, err := ParseAdd(WithNoteLines(c.line, c.rest), wednesday)
 		if err != nil {
 			t.Errorf("WithNoteLines(%q, %q): %v", c.line, c.rest, err)
 			continue
```

Patch `internal/dsl/filter_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/dsl/filter_test.go b/internal/dsl/filter_test.go
index 4735340..f7c1b2b 100644
--- a/internal/dsl/filter_test.go
+++ b/internal/dsl/filter_test.go
@@ -17,11 +17,11 @@ func TestParseFilter(t *testing.T) {
 	}{
 		{"", Filter{}},
 		{"   ", Filter{}},
-		{"@ana #impl !1 ~l <7d schema", Filter{Requesters: []string{"ana"}, Tags: []string{"impl"}, Priorities: []int{1},
+		{"@ana #impl !1 ~l <7d schema", Filter{Requesters: []string{"ana"}, Tags: []string{"impl"}, Impacts: []int{1},
 			Sizes: []domain.Size{domain.SizeL}, NewerThan: 7 * day, Words: []string{"schema"}}},
 		{"@Ana @bob @ana", Filter{Requesters: []string{"ana", "bob"}}},
 		{"#B #a #b", Filter{Tags: []string{"a", "b"}}},
-		{"!1 !2 !1 ~s ~x ~XL", Filter{Priorities: []int{1, 2}, Sizes: []domain.Size{domain.SizeS, domain.SizeXL}}},
+		{"!1 !2 !1 ~s ~x ~XL", Filter{Impacts: []int{1, 2}, Sizes: []domain.Size{domain.SizeS, domain.SizeXL}}},
 		{">30d", Filter{OlderThan: 30 * day}},
 		{"<2w >1d", Filter{NewerThan: 14 * day, OlderThan: day}},
 		{"<7D", Filter{NewerThan: 7 * day}},
@@ -38,7 +38,7 @@ func TestParseFilter(t *testing.T) {
 		{"https://x.io -- note", Filter{Words: []string{"https://x.io", "--", "note"}}},
 	}
 	for _, c := range cases {
-		got, err := ParseFilter(c.line, 3)
+		got, err := ParseFilter(c.line)
 		if err != nil {
 			t.Errorf("ParseFilter(%q): %v", c.line, err)
 			continue
@@ -51,15 +51,15 @@ func TestParseFilter(t *testing.T) {
 
 func TestParseFilterErrors(t *testing.T) {
 	cases := map[string]string{
-		"x !4":     "priority !4 out of range: use !1 to !3",
-		"x !0":     "priority !0 out of range",
+		"x !4":     "impact !4 out of range: use !1 to !3",
+		"x !0":     "impact !0 out of range",
 		"<0d":      "<0d: age must be 1 to 10000 days or weeks",
 		">99999w":  ">99999w: age must be",
 		"is:":      "unknown status is:",
 		"is:later": "unknown status is:later: use is:open, is:done, is:dropped or is:all",
 	}
 	for line, want := range cases {
-		_, err := ParseFilter(line, 3)
+		_, err := ParseFilter(line)
 		if err == nil || !strings.Contains(err.Error(), want) {
 			t.Errorf("ParseFilter(%q) error = %v; want it to contain %q", line, err, want)
 		}
@@ -68,7 +68,7 @@ func TestParseFilterErrors(t *testing.T) {
 
 func TestFilterEmpty(t *testing.T) {
 	for line, want := range map[string]bool{"": true, "  ": true, "x": false, "is:open": false, "<1d": false} {
-		f, err := ParseFilter(line, 3)
+		f, err := ParseFilter(line)
 		if err != nil {
 			t.Fatal(err)
 		}
```

Patch `internal/dsl/requester_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/dsl/requester_test.go b/internal/dsl/requester_test.go
index 4b0d1bb..129625d 100644
--- a/internal/dsl/requester_test.go
+++ b/internal/dsl/requester_test.go
@@ -35,7 +35,7 @@ func TestParseRequester(t *testing.T) {
 // typed as @who in a quick-add line (D53).
 func TestParseRequesterAgreesWithTheToken(t *testing.T) {
 	for _, name := range []string{"ana", "Ana", "ana,", "ANA.", "@ana", "ana-maria", "bob@x.com", "Ünïcode"} {
-		d, err := ParseAdd("Ask @"+name, 3)
+		d, err := ParseAdd("Ask @"+name, wednesday)
 		if err != nil {
 			t.Fatal(err)
 		}
```

Patch `internal/search/search_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/search/search_test.go b/internal/search/search_test.go
index bab5ab5..df66ba5 100644
--- a/internal/search/search_test.go
+++ b/internal/search/search_test.go
@@ -1,6 +1,7 @@
 package search
 
 import (
+	"slices"
 	"testing"
 	"time"
 
@@ -12,7 +13,7 @@ var now = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
 
 func filter(t *testing.T, line string) dsl.Filter {
 	t.Helper()
-	f, err := dsl.ParseFilter(line, 3)
+	f, err := dsl.ParseFilter(line)
 	if err != nil {
 		t.Fatal(err)
 	}
@@ -22,7 +23,7 @@ func filter(t *testing.T, line string) dsl.Filter {
 func TestMatch(t *testing.T) {
 	task := domain.Task{
 		ID: 1, Title: "Review schema-migration for Photos", Requester: "ana", Tags: []string{"impl"},
-		Priority: 2, Size: domain.SizeM, Status: domain.StatusOpen, CreatedAt: now.Add(-3 * 24 * time.Hour),
+		Impact: 2, Size: domain.SizeM, Status: domain.StatusOpen, CreatedAt: now.Add(-3 * 24 * time.Hour),
 	}
 	cases := map[string]bool{
 		"":                  true,
@@ -63,8 +64,8 @@ func TestMatch(t *testing.T) {
 }
 
 func TestMatchChildUsesEffectiveFields(t *testing.T) {
-	parent := domain.Task{ID: 1, Title: "Auth", Tags: []string{"auth"}, Priority: 1, Status: domain.StatusOpen, CreatedAt: now}
-	child := domain.Task{ID: 2, Title: "Rollback plan", Tags: []string{"ops"}, Priority: 3, ParentID: 1, Status: domain.StatusOpen, CreatedAt: now}
+	parent := domain.Task{ID: 1, Title: "Auth", Tags: []string{"auth"}, Impact: 1, Status: domain.StatusOpen, CreatedAt: now}
+	child := domain.Task{ID: 2, Title: "Rollback plan", Tags: []string{"ops"}, Impact: 3, ParentID: 1, Status: domain.StatusOpen, CreatedAt: now}
 	for line, want := range map[string]bool{"!1": true, "!3": false, "#auth #ops": true, "auth": true} {
 		if got := Match(filter(t, line), child, &parent, now); got != want {
 			t.Errorf("child Match(%q) = %v; want %v", line, got, want)
@@ -87,32 +88,27 @@ func TestMatchClosedTasks(t *testing.T) {
 }
 
 func TestGroupOf(t *testing.T) {
-	levels := []string{"P1 · blocked", "P2 · soon", "P3 · later"}
 	days := func(n int) time.Time { return now.Add(-time.Duration(n) * 24 * time.Hour) }
-	parent := domain.Task{ID: 1, Priority: 2}
 	cases := []struct {
-		by     GroupBy
-		task   domain.Task
-		parent *domain.Task
-		want   Group
+		by   GroupBy
+		task domain.Task
+		want Group
 	}{
-		{ByPriority, domain.Task{Priority: 1}, nil, Group{1, "P1 · blocked"}},
-		{ByPriority, domain.Task{Priority: 3, ParentID: 1}, &parent, Group{2, "P2 · soon"}},
-		{ByPriority, domain.Task{Priority: 5}, nil, Group{4, "P? · above the configured levels"}},
-		{ByPriority, domain.Task{}, nil, Group{5, "unset"}},
-		{BySize, domain.Task{Size: domain.SizeS}, nil, Group{0, "S"}},
-		{BySize, domain.Task{Size: domain.SizeXL}, nil, Group{3, "XL"}},
-		{BySize, domain.Task{}, nil, Group{4, "unsized"}},
-		{ByCreated, domain.Task{CreatedAt: now}, nil, Group{0, "this week"}},
-		{ByCreated, domain.Task{CreatedAt: days(7)}, nil, Group{1, "this month"}},
-		{ByCreated, domain.Task{CreatedAt: days(29)}, nil, Group{1, "this month"}},
-		{ByCreated, domain.Task{CreatedAt: days(30)}, nil, Group{2, "older"}},
-		{ByCreated, domain.Task{CreatedAt: days(90)}, nil, Group{3, "ancient"}},
-		{ByRequester, domain.Task{Requester: "ana"}, nil, Group{0, "@ana"}},
-		{ByRequester, domain.Task{}, nil, Group{1, "no requester"}},
+		{ByUrgency, domain.Task{Impact: 3}, Group{}},
+		{ByImpact, domain.Task{Impact: 1}, Group{}},
+		{BySize, domain.Task{Size: domain.SizeS}, Group{0, "S"}},
+		{BySize, domain.Task{Size: domain.SizeXL}, Group{3, "XL"}},
+		{BySize, domain.Task{}, Group{4, "unsized"}},
+		{ByCreated, domain.Task{CreatedAt: now}, Group{0, "this week"}},
+		{ByCreated, domain.Task{CreatedAt: days(7)}, Group{1, "this month"}},
+		{ByCreated, domain.Task{CreatedAt: days(29)}, Group{1, "this month"}},
+		{ByCreated, domain.Task{CreatedAt: days(30)}, Group{2, "older"}},
+		{ByCreated, domain.Task{CreatedAt: days(90)}, Group{3, "ancient"}},
+		{ByRequester, domain.Task{Requester: "ana"}, Group{0, "@ana"}},
+		{ByRequester, domain.Task{}, Group{1, "no requester"}},
 	}
 	for _, c := range cases {
-		if got := GroupOf(c.by, c.task, c.parent, now, levels); got != c.want {
+		if got := GroupOf(c.by, c.task, now); got != c.want {
 			t.Errorf("GroupOf(%s, %+v) = %+v; want %+v", c.by, c.task, got, c.want)
 		}
 	}
@@ -122,19 +118,19 @@ func TestGroupOf(t *testing.T) {
 }
 
 func TestGroupByCycle(t *testing.T) {
-	g := ByPriority
+	g := ByUrgency
 	var seen []GroupBy
 	for range GroupBys {
 		g = g.Next()
 		seen = append(seen, g)
 	}
-	want := []GroupBy{BySize, ByCreated, ByRequester, ByPriority}
+	want := []GroupBy{ByImpact, BySize, ByCreated, ByRequester, ByUrgency}
 	for i := range want {
 		if seen[i] != want[i] {
 			t.Fatalf("cycle = %v; want %v", seen, want)
 		}
 	}
-	for s, want := range map[string]GroupBy{"size": BySize, "requester": ByRequester, "": ByPriority, "bogus": ByPriority} {
+	for s, want := range map[string]GroupBy{"size": BySize, "impact": ByImpact, "": ByUrgency, "priority": ByUrgency, "bogus": ByUrgency} {
 		if got := ParseGroupBy(s); got != want {
 			t.Errorf("ParseGroupBy(%q) = %s; want %s", s, got, want)
 		}
@@ -164,3 +160,53 @@ func TestMatchName(t *testing.T) {
 		}
 	}
 }
+
+func TestSorter(t *testing.T) {
+	const today = domain.Day("2026-10-14")
+	day := func(n int) time.Time { return now.Add(time.Duration(n) * time.Hour) }
+	tasks := []domain.Task{
+		{ID: 1, Title: "undated stream task", Impact: 3, CreatedAt: day(1)},                         // 0.50
+		{ID: 2, Title: "ask due today", Impact: 1, Due: today, CreatedAt: day(2)},                   // 0.53
+		{ID: 3, Title: "team task due in 10 days", Impact: 2, Due: "2026-10-24", CreatedAt: day(3)}, // 0.51
+		{ID: 4, Title: "untriaged", CreatedAt: day(0)},
+		{ID: 5, Title: "older undated stream task", Impact: 3, CreatedAt: day(0)}, // 0.50, older
+		{ID: 6, Title: "child of #3", ParentID: 3, Due: today, CreatedAt: day(4)}, // impact 2, due today: 0.70
+	}
+	byID := map[int64]domain.Task{}
+	for _, tk := range tasks {
+		byID[tk.ID] = tk
+	}
+	parentOf := func(tk domain.Task) *domain.Task {
+		if p, ok := byID[tk.ParentID]; ok {
+			return &p
+		}
+		return nil
+	}
+	order := func(g GroupBy, w domain.Weights) []int64 {
+		got := slices.Clone(tasks)
+		slices.SortFunc(got, Sorter(g, parentOf, today, w))
+		var ids []int64
+		for _, tk := range got {
+			ids = append(ids, tk.ID)
+		}
+		return ids
+	}
+	cases := []struct {
+		g    GroupBy
+		w    domain.Weights
+		want []int64
+	}{
+		{ByUrgency, domain.DefaultWeights, []int64{6, 2, 3, 5, 1, 4}},
+		{ByImpact, domain.DefaultWeights, []int64{5, 1, 6, 3, 2, 4}},
+		{ByUrgency, domain.Weights{Impact: 1, Due: 0}, []int64{5, 1, 3, 6, 2, 4}}, // impact only: ties oldest first
+		{BySize, domain.DefaultWeights, []int64{4, 5, 1, 2, 3, 6}},                // a grouping: oldest first
+	}
+	for _, c := range cases {
+		if got := order(c.g, c.w); !slices.Equal(got, c.want) {
+			t.Errorf("%s with %+v: %v; want %v", c.g, c.w, got, c.want)
+		}
+	}
+	if !ByUrgency.Flat() || !ByImpact.Flat() || BySize.Flat() || ByRequester.Flat() {
+		t.Error("urgency and impact are flat sorts; the rest are groupings (D72)")
+	}
+}
```

Patch `internal/store/migration006_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/store/migration006_test.go b/internal/store/migration006_test.go
index cda28ae..cefbe71 100644
--- a/internal/store/migration006_test.go
+++ b/internal/store/migration006_test.go
@@ -2,6 +2,7 @@ package store
 
 import (
 	"context"
+	"database/sql"
 	"testing"
 	"time"
 
@@ -10,7 +11,8 @@ import (
 
 // TestMigration006 runs 006 on a database as Plan 9 left it (D76): no task
 // gets an impact except open pinned reviews (impact 1, due 3 days after the
-// request, or after first seen without one); priority stays as it was.
+// request, or after first seen without one); priority stays as it was,
+// unread.
 func TestMigration006(t *testing.T) {
 	s, path := openTemp(t)
 	for _, q := range []string{
@@ -80,9 +82,13 @@ func TestMigration006(t *testing.T) {
 	}
 	for id, w := range want {
 		got, _ := snap.Get(id)
-		if got.Impact != w.impact || got.Due != w.due || got.Priority != w.priority {
+		var priority sql.NullInt64
+		if err := again.db.QueryRow("SELECT priority FROM tasks WHERE id = ?", id).Scan(&priority); err != nil {
+			t.Fatal(err)
+		}
+		if got.Impact != w.impact || got.Due != w.due || int(priority.Int64) != w.priority {
 			t.Errorf("task #%d: impact %d, due %q, priority %d; want %d, %q, %d",
-				id, got.Impact, got.Due, got.Priority, w.impact, w.due, w.priority)
+				id, got.Impact, got.Due, priority.Int64, w.impact, w.due, w.priority)
 		}
 	}
 }
```

Patch `internal/store/store_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/store/store_test.go b/internal/store/store_test.go
index f9cb207..07d2f97 100644
--- a/internal/store/store_test.go
+++ b/internal/store/store_test.go
@@ -33,7 +33,7 @@ func sampleTask() domain.Task {
 	now := time.Date(2026, 10, 12, 9, 30, 0, 123, time.UTC)
 	return domain.Task{
 		Title: "Review PR #482", Note: "line 1\nline 2", Link: "https://github.com/a/b/pull/482",
-		Requester: "ivan", Tags: []string{"impl", "photos"}, Priority: 2, Impact: 2, Due: "2026-10-16", Size: domain.SizeM,
+		Requester: "ivan", Tags: []string{"impl", "photos"}, Impact: 2, Due: "2026-10-16", Size: domain.SizeM,
 		Status: domain.StatusOpen, CreatedAt: now, UpdatedAt: now,
 	}
 }
@@ -92,7 +92,7 @@ func TestTaskRoundTrip(t *testing.T) {
 	want.Status, want.ClosedBy, want.DropReason = domain.StatusDropped, domain.ClosedByUser, "handed to @marko"
 	want.ClosedAt = want.CreatedAt.Add(time.Hour)
 	want.Tags = []string{"org"}
-	want.Priority, want.Impact, want.Due, want.Size = 0, 0, "", domain.SizeNone
+	want.Impact, want.Due, want.Size = 0, "", domain.SizeNone
 	want.Source = &domain.SourceRef{Kind: "github", ID: "a/b#482"}
 	err = s.InTx(context.Background(), func(x *Tx) error { return x.UpdateTask(context.Background(), want) })
 	if err != nil {
```

Patch `internal/tui/browse_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/browse_test.go b/internal/tui/browse_test.go
index fd6f756..37e8f9f 100644
--- a/internal/tui/browse_test.go
+++ b/internal/tui/browse_test.go
@@ -11,9 +11,9 @@ import (
 
 func TestFilterEscRestoresTheSelection(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	h.add("Alpha !1 ~s")
-	h.add("Beta !1 ~s")
-	gamma := h.add("Gamma !1 ~s")
+	h.add("Alpha !3 ~s")
+	h.add("Beta !3 ~s")
+	gamma := h.add("Gamma !3 ~s")
 	h.keys("j", "j")
 	h.keys("/")
 	h.typeText("alp") // live narrowing moves the cursor to Alpha
@@ -35,21 +35,21 @@ func TestGroupingSavesTheLatestChoice(t *testing.T) {
 		t.Fatal("a second save must not run while the first is in flight")
 	}
 	h.run(first)
-	h.mustSee("BACKLOG · by created")
-	if v, _ := h.svc.UIState(context.Background(), groupKey); v != "created" {
-		t.Errorf("remembered %q; want the latest choice, created", v)
+	h.mustSee("BACKLOG · by size")
+	if v, _ := h.svc.UIState(context.Background(), groupKey); v != "size" {
+		t.Errorf("remembered %q; want the latest choice, size", v)
 	}
 
 	// The startup read landing after a g press doesn't override it.
 	h.send(uiStateMsg{group: "requester"})
-	h.mustSee("BACKLOG · by created")
+	h.mustSee("BACKLOG · by size")
 }
 
 func TestFilteredExpandCollapseLeavesTheBacklogAlone(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	parent, _ := h.family("Auth migration !1 ~xl #auth", "Rollback plan ~s", "Token spike ~m")
+	parent, _ := h.family("Auth migration !3 ~xl #auth", "Rollback plan ~s", "Token spike ~m")
 	h.keys("right") // expanded before filtering
-	h.mustSee("▾ [P1 XL] Auth migration", "    [P1 S] Rollback plan")
+	h.mustSee("▾ [ 50 XL] Auth migration", "    [ 50 S] Rollback plan")
 
 	if err := h.m.applyFilter("#auth"); err != nil {
 		t.Fatal(err)
@@ -59,24 +59,24 @@ func TestFilteredExpandCollapseLeavesTheBacklogAlone(t *testing.T) {
 		t.Errorf("cursor on %q; want the parent", r.task.Title)
 	}
 	h.keys("left") // on the parent: filtered parents stay expanded
-	h.mustSee("    [P1 S] Rollback plan")
+	h.mustSee("    [ 50 S] Rollback plan")
 	// Clearing the filter shows the parent still expanded, as before the filter.
 	h.keys("esc")
-	h.mustSee("▾ [P1 XL] Auth migration", "    [P1 S] Rollback plan")
+	h.mustSee("▾ [ 50 XL] Auth migration", "    [ 50 S] Rollback plan")
 }
 
 func TestParentWithoutListedChildrenHasNoMarker(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	h.family("Lonely parent !2 ~m", "Untriaged idea") // the child is in the Inbox
-	h.mustSee("  [P2 M] Lonely parent")
-	h.mustNotSee("▸ [P2 M] Lonely parent")
+	h.mustSee("  [ 33 M] Lonely parent")
+	h.mustNotSee("▸ [ 33 M] Lonely parent")
 	h.keys("right")
 	h.mustNotSee("Untriaged idea")
 }
 
 func TestFilterBarCompletesAfterDashes(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	h.add("One @ana !1 ~s")
+	h.add("One @ana !3 ~s")
 	h.keys("/")
 	h.typeText("-- @an") // in a filter, -- is just a word
 	h.mustSee("tab → @ana")
```

Patch `internal/tui/children_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/children_test.go b/internal/tui/children_test.go
index 46d6269..ed31698 100644
--- a/internal/tui/children_test.go
+++ b/internal/tui/children_test.go
@@ -23,7 +23,7 @@ func (h *harness) childIDs(parentID int64) []int64 {
 
 func TestAddChildrenOneAfterAnother(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	parent := h.add("Auth migration !1 ~xl #auth")
+	parent := h.add("Auth migration !3 ~xl #auth")
 	h.keys("enter")
 	h.mustSee("no children · a adds one")
 
@@ -31,7 +31,7 @@ func TestAddChildrenOneAfterAnother(t *testing.T) {
 	h.mustSee("child of #1 › ")
 	h.typeText("Token spike ~s")
 	h.keys("enter")
-	h.mustSee("added #2 under #1", "CHILDREN ░░░░░░░░ 0/1", "  [P1 S] Token spike")
+	h.mustSee("added #2 under #1", "CHILDREN ░░░░░░░░ 0/1", "  [ 50 S] Token spike")
 	if _, ok := h.m.overlay.(*addChild); !ok {
 		t.Fatal("the add-child input stays open for the next child")
 	}
@@ -40,9 +40,9 @@ func TestAddChildrenOneAfterAnother(t *testing.T) {
 	h.keys("enter")
 	h.mustSee("added #3 under #1", "0/2")
 
-	h.typeText("Urgent bit !1 ~s") // priority is the parent's (D32)
+	h.typeText("Urgent bit !3 ~s") // impact is the parent's (D32, D73)
 	h.keys("enter")
-	h.mustSee("✗ invalid value: children inherit priority from #1; drop !1", "child of #1 › Urgent bit !1 ~s")
+	h.mustSee("✗ invalid value: children inherit impact from #1; drop !3", "child of #1 › Urgent bit !3 ~s")
 	h.keys("esc")
 	if h.m.overlay != nil {
 		t.Fatal("esc closes the input")
@@ -75,7 +75,7 @@ func TestAddChildrenOneAfterAnother(t *testing.T) {
 
 func TestAddChildKeepsTheLineWhenItFails(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	parent := h.add("Auth migration !1 ~xl")
+	parent := h.add("Auth migration !3 ~xl")
 	h.keys("enter", "a")
 	if err := h.svc.Delete(context.Background(), parent.ID); err != nil { // behind the TUI's back
 		t.Fatal(err)
@@ -90,7 +90,7 @@ func TestAddChildKeepsTheLineWhenItFails(t *testing.T) {
 
 func TestReorderChildren(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	parent, kids := h.family("Parent !1 ~l", "A ~s", "B ~s", "C ~s")
+	parent, kids := h.family("Parent !3 ~l", "A ~s", "B ~s", "C ~s")
 	a, b, c := kids[0].ID, kids[1].ID, kids[2].ID
 	h.keys("enter", "J")
 	h.mustSee("J/K move a child · j selects one")
@@ -132,8 +132,8 @@ func TestReorderChildren(t *testing.T) {
 func TestMultiLinePasteBecomesTheNote(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	h.keys("a")
-	h.send(tea.PasteMsg{Content: "Look at the schema @ana !1 ~s\r\nAna: can you check\r\n\r\nthe migration?\r\n"})
-	h.mustSee("add › Look at the schema @ana !1 ~s", "+ 3 lines → note")
+	h.send(tea.PasteMsg{Content: "Look at the schema @ana !3 ~s\r\nAna: can you check\r\n\r\nthe migration?\r\n"})
+	h.mustSee("add › Look at the schema @ana !3 ~s", "+ 3 lines → note")
 	h.keys("enter")
 	h.mustSee("added #1 to backlog")
 	if got := h.task(1); got.Title != "Look at the schema" || got.Note != "Ana: can you check\n\nthe migration?" {
```

Patch `internal/tui/colour_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/colour_test.go b/internal/tui/colour_test.go
index 24a8c9a..05f45e9 100644
--- a/internal/tui/colour_test.go
+++ b/internal/tui/colour_test.go
@@ -36,7 +36,7 @@ func colour(i int) string { return on(lipgloss.NewStyle().Foreground(lipgloss.Co
 func TestBlocksAreColouredPerTask(t *testing.T) {
 	h := newHarness(t, config.Default())
 	ctx := context.Background()
-	fix := h.pinned("Fix CI !1 ~s")    // #1
+	fix := h.pinned("Fix CI !3 ~s")    // #1
 	rfc := h.pinned("Write RFC !2 ~s") // #2
 	h.block(fix.ID, 10*time.Hour, time.Hour)
 	h.block(rfc.ID, 12*time.Hour, 30*time.Minute)
@@ -45,7 +45,7 @@ func TestBlocksAreColouredPerTask(t *testing.T) {
 	if c1 == c2 {
 		t.Fatal("tasks #1 and #2 get different colours")
 	}
-	if l := h.rawLine("10:00 ▓ Fix CI"); !strings.Contains(l, c1+"▓") || !strings.Contains(l, c1+"Fix CI [P1 S]") {
+	if l := h.rawLine("10:00 ▓ Fix CI"); !strings.Contains(l, c1+"▓") || !strings.Contains(l, c1+"Fix CI [ 50 S]") {
 		t.Errorf("a block's ▓ and label take its task's colour: %q", l)
 	}
 	if l := h.rawLine("10:30 ▓"); !strings.Contains(l, c1+"▓") {
@@ -74,7 +74,7 @@ func TestBlocksAreColouredPerTask(t *testing.T) {
 
 func TestConflictsAndMeetingsKeepTheirOwnStyles(t *testing.T) {
 	h := newCalHarness(t, config.Default())
-	fix := h.pinned("Fix CI !1 ~s")
+	fix := h.pinned("Fix CI !3 ~s")
 	h.block(fix.ID, 10*time.Hour, time.Hour)
 	h.meetings(ev("Standup", 10*time.Hour, 10*time.Hour+30*time.Minute), ev("Lunch", 12*time.Hour, 13*time.Hour))
 	l := h.rawLine("10:00 ▓ Fix CI")
@@ -106,7 +106,7 @@ func reversedToTheEnd(t *testing.T, what, line string) {
 func TestTheHighlightRunsToTheEndOfTheRow(t *testing.T) {
 	h := newHarness(t, config.Default())
 	ctx := context.Background()
-	fix := h.pinned("Fix CI !1 ~s")
+	fix := h.pinned("Fix CI !3 ~s")
 	h.block(fix.ID, 9*time.Hour, time.Hour) // the cursor rests on 09:00 (now)
 	reversedToTheEnd(t, "the cursor's coloured block row", h.m.timelineRow(h.m.today.cursor, 60))
 
```

Patch `internal/tui/detail_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/detail_test.go b/internal/tui/detail_test.go
index c3da968..6a1a453 100644
--- a/internal/tui/detail_test.go
+++ b/internal/tui/detail_test.go
@@ -34,24 +34,24 @@ func TestDetailShowsTheTaskAndEscGoesBack(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
 	note := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8"
-	parent, kids := h.family("Auth migration !1 ~xl @ana #auth https://x.io/pr/482 -- "+note,
+	parent, kids := h.family("Auth migration !3 ~xl @ana #auth https://x.io/pr/482 -- "+note,
 		"Token spike ~s", "Rollback plan ~m", "Dry run ~s -- notes here")
 	if _, err := h.svc.Done(ctx, kids[0].ID); err != nil {
 		t.Fatal(err)
 	}
 	h.add("Other !2 ~s")
-	h.mustSee("[P1 XL] Auth migration ✎ @ana")
+	h.mustSee("[ 50 XL] Auth migration ✎ @ana")
 
 	h.keys("enter") // the cursor starts on Auth migration (P1 sorts first)
 	h.mustSee("tt · Detail",
-		fmt.Sprintf("#%d Auth migration  [P1 XL]", parent.ID),
+		fmt.Sprintf("#%d Auth migration  [ 50 XL]", parent.ID),
 		"@ana · #auth · captured just now",
 		"🔗 https://x.io/pr/482",
 		"NOTE", "  line 1", "  line 6", "… +2 lines",
 		"CHILDREN ██░░░░░░ 1/3",
-		"  [P1 M] Rollback plan", // open children first, in sibling order
-		"  [P1 S] Dry run ✎",
-		"  [P1 S] Token spike ✓ Mon 12 Oct") // then closed ones with their close date
+		"  [ 50 M] Rollback plan", // open children first, in sibling order
+		"  [ 50 S] Dry run ✎",
+		"  [ 50 S] Token spike ✓ Mon 12 Oct") // then closed ones with their close date
 	h.mustNotSee("line 7", "Other")
 	if s := h.screen(); strings.Index(s, "Rollback plan") > strings.Index(s, "Dry run") ||
 		strings.Index(s, "Dry run") > strings.Index(s, "Token spike") {
@@ -60,7 +60,7 @@ func TestDetailShowsTheTaskAndEscGoesBack(t *testing.T) {
 
 	// ⏎ on a child opens its detail; esc goes back one level, then to Pick.
 	h.keys("j", "j", "enter")
-	h.mustSee(fmt.Sprintf("#%d Dry run  [P1 S]", kids[2].ID), fmt.Sprintf("child of #%d Auth migration", parent.ID), "  notes here")
+	h.mustSee(fmt.Sprintf("#%d Dry run  [ 50 S]", kids[2].ID), fmt.Sprintf("child of #%d Auth migration", parent.ID), "  notes here")
 	h.mustNotSee("CHILDREN") // one level only (D6)
 	h.keys("esc")
 	h.mustSee("CHILDREN")
@@ -76,10 +76,10 @@ func TestDetailShowsTheTaskAndEscGoesBack(t *testing.T) {
 
 func TestDetailKeysActOnTheRowUnderTheCursor(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	parent, kids := h.family("Parent !1 ~l", "First ~s", "Second ~s")
+	parent, kids := h.family("Parent !3 ~l", "First ~s", "Second ~s")
 	h.keys("enter", "j")
 	h.keys("x")
-	h.mustSee(`✓ "First"`, "  [P1 S] First ✓ Mon 12 Oct", "1/2")
+	h.mustSee(`✓ "First"`, "  [ 50 S] First ✓ Mon 12 Oct", "1/2")
 	if got := h.task(kids[0].ID); got.Status != domain.StatusDone {
 		t.Fatalf("x on a child: %s", got.Status)
 	}
@@ -91,7 +91,7 @@ func TestDetailKeysActOnTheRowUnderTheCursor(t *testing.T) {
 
 	h.keys("k") // back on the task itself
 	h.keys("p")
-	h.mustSee(`pinned "Parent"`, fmt.Sprintf("#%d Parent  [P1 L] 📌", parent.ID))
+	h.mustSee(`pinned "Parent"`, fmt.Sprintf("#%d Parent  [ 50 L] 📌", parent.ID))
 	h.keys("d")
 	h.typeText("handed over")
 	h.keys("enter")
@@ -103,7 +103,7 @@ func TestDetailKeysActOnTheRowUnderTheCursor(t *testing.T) {
 func TestDetailFallsBackWhenTheTaskIsGone(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
-	parent, kids := h.family("Parent !1 ~l", "Child ~s")
+	parent, kids := h.family("Parent !3 ~l", "Child ~s")
 	h.keys("enter", "j", "enter")
 	h.mustSee("child of #1 Parent")
 
@@ -122,7 +122,7 @@ func TestDetailFallsBackWhenTheTaskIsGone(t *testing.T) {
 
 func TestFallingBackClosesTheOverlay(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	parent := h.add("Parent !1 ~l")
+	parent := h.add("Parent !3 ~l")
 	h.keys("enter", "a")
 	h.mustSee("child of #1 › ")
 	if err := h.svc.Delete(context.Background(), parent.ID); err != nil {
@@ -141,7 +141,7 @@ func TestDetailFromTodayAndChildScrolling(t *testing.T) {
 	for i := 1; i <= 12; i++ {
 		kids = append(kids, fmt.Sprintf("Step %02d ~s", i))
 	}
-	parent, _ := h.family("Big effort !1 ~xl", kids...)
+	parent, _ := h.family("Big effort !3 ~xl", kids...)
 	if _, err := h.svc.Pin(context.Background(), parent.ID); err != nil {
 		t.Fatal(err)
 	}
@@ -165,7 +165,7 @@ func TestDetailFromTodayAndChildScrolling(t *testing.T) {
 
 func TestDetailOfAClosedTaskAndHelp(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	a := h.add("Shipped !1 ~s")
+	a := h.add("Shipped !3 ~s")
 	h.clock.now = h.clock.now.Add(time.Hour)
 	if _, err := h.svc.Done(context.Background(), a.ID); err != nil {
 		t.Fatal(err)
@@ -175,14 +175,14 @@ func TestDetailOfAClosedTaskAndHelp(t *testing.T) {
 		t.Fatal(err)
 	}
 	h.keys("enter")
-	h.mustSee("#1 Shipped  [P1 S] ✓ Mon 12 Oct", "no note", "no children")
+	h.mustSee("#1 Shipped  [ 50 S] ✓ Mon 12 Oct", "no note", "no children")
 	h.keys("?")
 	h.mustSee("open the child under the cursor")
 }
 
 func TestNewDayBannerLeavesDetailBehind(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	a := h.add("Gone !1 ~s")
+	a := h.add("Gone !3 ~s")
 	h.keys("enter") // detail is open when the day rolls over
 	h.mustSee("#1 Gone")
 
```

Patch `internal/tui/edit_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/edit_test.go b/internal/tui/edit_test.go
index 4c47171..40f35b8 100644
--- a/internal/tui/edit_test.go
+++ b/internal/tui/edit_test.go
@@ -16,10 +16,10 @@ import (
 
 func TestEditLine(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	a := h.add("Old title @ana #a !1 ~s https://x.io -- keep me")
+	a := h.add("Old title @ana #a !3 ~s https://x.io -- keep me")
 
 	h.keys("e")
-	h.mustSee("edit #1 › Old title @ana #a !1 ~s https://x.io")
+	h.mustSee("edit #1 › Old title @ana #a !3 ~s https://x.io")
 	h.typeText(" #b @bob")
 	h.keys("enter")
 	h.mustSee(`edited "Old title"`)
@@ -39,33 +39,33 @@ func TestEditLine(t *testing.T) {
 	if _, ok := h.m.overlay.(*editLine); !ok {
 		t.Fatal("a bad edit must keep the overlay open")
 	}
-	h.mustSee("edit #1 › Old title @ana #a !1 ~s https://x.io !9", "✗ priority !9 out of range")
+	h.mustSee("edit #1 › Old title @ana #a !3 ~s https://x.io !9", "✗ impact !9 out of range")
 	h.keys("esc")
-	if got := h.task(a.ID); got.Priority != 1 {
+	if got := h.task(a.ID); got.Impact != 3 {
 		t.Errorf("a refused edit changed the task: %+v", got)
 	}
 }
 
-func TestEditLineOfAChildRefusesPriority(t *testing.T) {
+func TestEditLineOfAChildRefusesImpact(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	parent, kids := h.family("Parent !1 ~l", "Child ~s")
+	parent, kids := h.family("Parent !3 ~l", "Child ~s")
 	h.keys("enter", "j", "e")
 	h.mustSee("edit #2 › Child ~s")
 	h.typeText(" !2")
 	h.keys("enter")
-	h.mustSee("✗ invalid value: children inherit priority from #1; drop !2")
+	h.mustSee("✗ invalid value: children inherit impact from #1; drop !2")
 	if _, ok := h.m.overlay.(*editLine); !ok {
 		t.Fatal("the overlay stays open")
 	}
 	h.keys("esc")
-	if got := h.task(kids[0].ID); got.Priority != 0 || got.ParentID != parent.ID {
+	if got := h.task(kids[0].ID); got.Impact != 0 || got.ParentID != parent.ID {
 		t.Errorf("child: %+v", got)
 	}
 }
 
 func TestNoteInTheEditor(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	a := h.add("Task !1 ~s -- first line")
+	a := h.add("Task !3 ~s -- first line")
 	var seen string
 	h.editor = func(path string) error {
 		data, err := os.ReadFile(path)
@@ -139,7 +139,7 @@ func TestNoteInTheEditor(t *testing.T) {
 
 func TestNoteChangedElsewhereKeepsYourText(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	a := h.add("Task !1 ~s -- original")
+	a := h.add("Task !3 ~s -- original")
 	var path string
 	h.editor = func(p string) error {
 		path = p
@@ -168,7 +168,7 @@ func TestNoteChangedElsewhereKeepsYourText(t *testing.T) {
 // so the part the user needs survives the cut (D29a).
 func TestNoteConflictPathFitsANarrowPane(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	a := h.add("Task !1 ~s -- original")
+	a := h.add("Task !3 ~s -- original")
 	h.send(tea.WindowSizeMsg{Width: 100, Height: 40})
 	var path string
 	h.editor = func(p string) error {
@@ -186,7 +186,7 @@ func TestNoteConflictPathFitsANarrowPane(t *testing.T) {
 
 func TestEditorTempFilesAreRemoved(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	h.add("Task !1 ~s -- x")
+	h.add("Task !3 ~s -- x")
 	var paths []string
 	h.editor = func(p string) error {
 		paths = append(paths, p)
@@ -207,7 +207,7 @@ func TestEditorTempFilesAreRemoved(t *testing.T) {
 
 func TestOpenLink(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	h.add("With link !1 ~s https://x.io/pr/482")
+	h.add("With link !3 ~s https://x.io/pr/482")
 	h.add("Without !2 ~s")
 	h.keys("o")
 	h.mustSee("opened https://x.io/pr/482")
@@ -247,7 +247,7 @@ func TestEditorCommand(t *testing.T) {
 // Task detail points at E for writing or reading the whole note.
 func TestDetailNoteHints(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	h.add("No note !1 ~s")
+	h.add("No note !3 ~s")
 	h.add("Long note !2 ~s -- " + strings.Repeat("line\n", 7) + "last")
 	h.keys("enter")
 	h.mustSee("no note · E to write one")
```

Patch `internal/tui/filter_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/filter_test.go b/internal/tui/filter_test.go
index 1c087e1..b0937b3 100644
--- a/internal/tui/filter_test.go
+++ b/internal/tui/filter_test.go
@@ -15,7 +15,7 @@ import (
 
 func TestFilterBarNarrowsLiveAndKeepsTypingOnError(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	h.add("Schema review !1 ~s @ana")
+	h.add("Schema review !3 ~s @ana")
 	h.add("Deploy fix !2 ~m")
 
 	h.keys("/")
@@ -23,11 +23,11 @@ func TestFilterBarNarrowsLiveAndKeepsTypingOnError(t *testing.T) {
 		t.Errorf("the filter line is the first row inside the backlog's frame (D56):\n%s", h.screen())
 	}
 	h.typeText("sch")
-	h.mustSee("BACKLOG · by priority · 1 match ─", "filter › sch", "Schema review")
+	h.mustSee("BACKLOG · by urgency · 1 match ─", "filter › sch", "Schema review")
 	h.mustNotSee("Deploy fix")
 
 	h.typeText(" !9")
-	h.mustSee("✗ priority !9 out of range", "BACKLOG · by priority · 1 match", "Schema review") // the last valid filter ("sch !") stays
+	h.mustSee("✗ impact !9 out of range", "BACKLOG · by urgency · 1 match", "Schema review") // the last valid filter ("sch !") stays
 	h.keys("enter")
 	if _, ok := h.m.overlay.(*filterBar); !ok {
 		t.Fatal("enter on a bad filter must keep the line open")
@@ -62,7 +62,7 @@ func TestFilterBarNarrowsLiveAndKeepsTypingOnError(t *testing.T) {
 
 func TestReloadWhileFilteringKeepsTheFilter(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	h.add("Schema review !1 ~s")
+	h.add("Schema review !3 ~s")
 	h.keys("/")
 	h.typeText("schema")
 	other, err := store.Open(context.Background(), h.path) // `tt add` from another shell
@@ -70,17 +70,17 @@ func TestReloadWhileFilteringKeepsTheFilter(t *testing.T) {
 		t.Fatal(err)
 	}
 	defer other.Close()
-	if _, _, err := app.New(other, h.clock, config.Default()).Add(context.Background(), "Schema follow-up !1 ~s"); err != nil {
+	if _, _, err := app.New(other, h.clock, config.Default()).Add(context.Background(), "Schema follow-up !3 ~s"); err != nil {
 		t.Fatal(err)
 	}
 	h.send(watchTickMsg{})
-	h.mustSee("filter › schema", "2 matches", "[P1 S] Schema follow-up")
+	h.mustSee("filter › schema", "2 matches", "[ 50 S] Schema follow-up")
 }
 
 func TestTokenInputCompletesAtTheCursor(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	h.add("one @ana #photos !1 ~s")
-	h.add("two @andrei !1 ~s")
+	h.add("one @ana #photos !3 ~s")
+	h.add("two @andrei !3 ~s")
 
 	h.keys("a")
 	h.typeText("x @an #ph")
@@ -118,7 +118,7 @@ func TestOverlayInputFollowsResize(t *testing.T) {
 
 func TestDropKeyKeepsTheReasonOfADroppedTask(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	a := h.add("Alpha !1 ~s")
+	a := h.add("Alpha !3 ~s")
 	h.keys("d")
 	h.typeText("handed to @marko")
 	h.keys("enter")
@@ -139,7 +139,7 @@ func TestDropKeyKeepsTheReasonOfADroppedTask(t *testing.T) {
 func TestMatchCountSurvivesNarrowLeftPane(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	h.send(tea.WindowSizeMsg{Width: 100, Height: 30})
-	h.add("Schema migration !1 ~s @ana #impl")
+	h.add("Schema migration !3 ~s @ana #impl")
 	h.add("Deploy fix !2 ~m")
 	h.keys("/")
 	h.typeText("@ana #impl schema migration")
@@ -148,7 +148,7 @@ func TestMatchCountSurvivesNarrowLeftPane(t *testing.T) {
 
 func TestEscHintHiddenWhileFilterLineOpen(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	h.add("Schema review !1 ~s")
+	h.add("Schema review !3 ~s")
 	h.keys("/")
 	h.typeText("sch")
 	h.keys("enter")
```

Patch `internal/tui/filterline_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/filterline_test.go b/internal/tui/filterline_test.go
index 2a24d4f..eab1b13 100644
--- a/internal/tui/filterline_test.go
+++ b/internal/tui/filterline_test.go
@@ -15,7 +15,7 @@ func (h *harness) lineOf(s string) int {
 
 func TestTheAppliedFilterStaysVisibleInTheLog(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	h.add("Schema review !1 ~s")
+	h.add("Schema review !3 ~s")
 	h.add("Deploy fix !2 ~m")
 	h.keys("/")
 	h.typeText("schema")
@@ -24,7 +24,7 @@ func TestTheAppliedFilterStaysVisibleInTheLog(t *testing.T) {
 		t.Errorf("the input is the first row inside the backlog's frame: line %d", got)
 	}
 	h.keys("enter")
-	h.mustSee("BACKLOG · by priority · 1 match ─", "filter › schema · esc clears")
+	h.mustSee("BACKLOG · by urgency · 1 match ─", "filter › schema · esc clears")
 	h.mustNotSee("1 match · schema") // the title keeps only the count
 	if got := h.lineOf("filter › schema · esc clears"); got != 2 {
 		t.Errorf("the applied filter keeps the input's row: line %d", got)
@@ -48,8 +48,8 @@ func TestTheAppliedFilterStaysVisibleInTheLog(t *testing.T) {
 
 func TestTheAppliedFilterStaysVisibleInPeople(t *testing.T) {
 	h := newHarness(t, config.Default())
-	h.add("Ask @ana !1 ~s")
-	h.add("Ask @bob !1 ~s")
+	h.add("Ask @ana !3 ~s")
+	h.add("Ask @bob !3 ~s")
 	h.keys("P", "/")
 	h.typeText("an")
 	if got := h.lineOf("filter › an"); got != 2 {
```

Patch `internal/tui/halfpage_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/halfpage_test.go b/internal/tui/halfpage_test.go
index ddf0659..765ee6a 100644
--- a/internal/tui/halfpage_test.go
+++ b/internal/tui/halfpage_test.go
@@ -17,7 +17,7 @@ const shortHeight = 17
 func TestHalfPageInTheLog(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	for i := 1; i <= 20; i++ {
-		h.add(fmt.Sprintf("Task %02d !1 ~s", i))
+		h.add(fmt.Sprintf("Task %02d !3 ~s", i))
 	}
 	h.send(tea.WindowSizeMsg{Width: 140, Height: shortHeight})
 	title := func() string {
@@ -54,7 +54,7 @@ func TestHalfPageOnToday(t *testing.T) {
 		t.Errorf("ctrl+u past the top: %v; want the first row", at)
 	}
 
-	h.pinned("Alpha !1 ~s")
+	h.pinned("Alpha !3 ~s")
 	h.keys("tab", "b", "ctrl+d") // a picked-up ghost moves too
 	if at := h.m.today.at; at != 12*time.Hour {
 		t.Errorf("ctrl+d while placing: %v; want 12:00", at)
@@ -67,7 +67,7 @@ func TestHalfPageInTaskDetail(t *testing.T) {
 	for i := 1; i <= 10; i++ {
 		lines = append(lines, fmt.Sprintf("Kid %02d ~s", i))
 	}
-	_, kids := h.family("Parent !1 ~xl", lines...)
+	_, kids := h.family("Parent !3 ~xl", lines...)
 	h.send(tea.WindowSizeMsg{Width: 140, Height: shortHeight})
 	h.keys("enter", "ctrl+d")
 	if h.m.screen != screenDetail || h.m.detail.selected != kids[5].ID {
```

Patch `internal/tui/lane_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/lane_test.go b/internal/tui/lane_test.go
index 08cdd56..e9f2924 100644
--- a/internal/tui/lane_test.go
+++ b/internal/tui/lane_test.go
@@ -50,10 +50,11 @@ func TestReviewsLaneOnToday(t *testing.T) {
 	h.keys("w")
 	h.mustSee("j/k move · p pin · o open PR · w back")
 	h.keys("j", "p") // #877
-	h.mustSee(`pinned "Review #877: Bump go" · P2 S`, "REVIEWS · 3 waiting", "Review #877: Bump go")
+	// Impact 1, due 3 days after the request a day ago (D75).
+	h.mustSee(`pinned "Review #877: Bump go" · 50 S`, "REVIEWS · 3 waiting", "Review #877: Bump go")
 	h.keys("k") // up to #915, the newest
-	if l := h.lineWith("Review #877"); !strings.Contains(l, "[P2 S]") {
-		t.Errorf("the review is a candidate with D48's defaults: %q", l)
+	if l := h.lineWith("Review #877"); !strings.Contains(l, "[ 50 S]") {
+		t.Errorf("the review is a candidate with D48's and D75's defaults: %q", l)
 	}
 	h.keys("ctrl+z")
 	h.mustSee(`undid pin "Review #877: Bump go"`, "REVIEWS · 3 waiting")
@@ -74,7 +75,7 @@ func TestReviewsLaneOnToday(t *testing.T) {
 	if h.m.lane.focused || h.m.today.focus != focusTimeline {
 		t.Errorf("tab leaves the lane for the timeline: lane %v, focus %v", h.m.lane.focused, h.m.today.focus)
 	}
-	h.pinned("A candidate !1 ~s") // tab skips an empty candidates pane (D57)
+	h.pinned("A candidate !3 ~s") // tab skips an empty candidates pane (D57)
 	h.keys("tab")                 // tab stays between timeline and candidates
 	if h.m.lane.focused || h.m.today.focus != focusCandidates {
 		t.Errorf("tab: lane %v, focus %v", h.m.lane.focused, h.m.today.focus)
@@ -98,7 +99,8 @@ func TestReviewsLaneInTheLog(t *testing.T) {
 	}
 	h.mustSee("w reviews · ⏎ open")
 	h.keys("w", "j", "j", "p") // #901, the longest waiting, is last (D60)
-	h.mustSee(`pinned "Review #901: Add idempotency key" · P2 S`)
+	// Requested 3 days ago: due today (D75).
+	h.mustSee(`pinned "Review #901: Add idempotency key" · 53 S`)
 	if l := h.lineWith("Review #901"); !strings.Contains(l, "│") || strings.Index(l, "Review #901") < strings.Index(l, "│") {
 		t.Errorf("the pinned review is a candidate (right), not in the backlog: %q", l)
 	}
@@ -158,7 +160,7 @@ func TestReviewsLaneHeight(t *testing.T) {
 	h.setReviews(items...)
 	h.send(tea.WindowSizeMsg{Width: 140, Height: 24})
 	for n := range 6 {
-		h.add("Candidate " + string(rune('A'+n)) + " !1 ~s")
+		h.add("Candidate " + string(rune('A'+n)) + " !3 ~s")
 	}
 	for range 6 {
 		h.keys("L", "p", "B", "esc") // pin the first backlog task, back to Today
@@ -234,7 +236,7 @@ func TestReviewsWithATaskAreMarked(t *testing.T) {
 	h := newGHHarness(t, config.Default())
 	threeReviews(h)
 	h.keys("w", "j", "p")                                                    // #877
-	h.mustSee(`pinned "Review #877: Bump go" · P2 S`, "REVIEWS · 3 waiting") // it stays in the lane (D51)
+	h.mustSee(`pinned "Review #877: Bump go" · 50 S`, "REVIEWS · 3 waiting") // it stays in the lane (D51)
 	if l := h.lineWith("#877 photos"); !strings.HasSuffix(strings.TrimRight(l, " │"), "@tom  1d · #1") {
 		t.Errorf("a review with a task names it: %q", l)
 	}
```

Patch `internal/tui/layout_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/layout_test.go b/internal/tui/layout_test.go
index 15d6599..338a719 100644
--- a/internal/tui/layout_test.go
+++ b/internal/tui/layout_test.go
@@ -22,10 +22,10 @@ func busyHarness(t *testing.T) *harness {
 	t.Helper()
 	h := newGHHarness(t, config.Default())
 	ctx := context.Background()
-	a := h.pinned("Fix CI on the release branch @ana !1 ~s")
+	a := h.pinned("Fix CI on the release branch @ana !3 ~s")
 	h.pinned("Write the schema RFC !2 ~m")
 	h.block(a.ID, 10*time.Hour, time.Hour)
-	parent := h.add("Profiles migration #photos !1 ~l -- a note\nline two")
+	parent := h.add("Profiles migration #photos !3 ~l -- a note\nline two")
 	for _, kid := range []string{"Import flow ~s", "Report export ~m"} {
 		if _, _, err := h.svc.AddChild(ctx, parent.ID, kid); err != nil {
 			t.Fatal(err)
```

Patch `internal/tui/meetings_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/meetings_test.go b/internal/tui/meetings_test.go
index c3e94fc..e56b62e 100644
--- a/internal/tui/meetings_test.go
+++ b/internal/tui/meetings_test.go
@@ -25,7 +25,7 @@ func ev(title string, from, to time.Duration) domain.Event {
 
 func TestMeetingsOnToday(t *testing.T) {
 	h := newCalHarness(t, config.Default())
-	fix := h.pinned("Fix CI !1 ~s")
+	fix := h.pinned("Fix CI !3 ~s")
 	h.block(fix.ID, 12*time.Hour, time.Hour)
 	h.meetings(
 		ev("Early", 6*time.Hour, 7*time.Hour),                                   // outside the working hours: not shown
@@ -39,7 +39,7 @@ func TestMeetingsOnToday(t *testing.T) {
 		" 08:00 ■ Breakfast",           // named on its first row inside the hours
 		" 09:30 ■ 09:30–09:45 Standup", // off the grid: exact times (D41)
 		" 10:00 ■ 10:00–11:15 Review +1", " 10:30 ■", " 11:00 ■",
-		" 12:00 ▓ Fix CI [P1 S] ⚠ · ■ Lunch", " 12:30 ▓ · ■", // the block first, then the meeting
+		" 12:00 ▓ Fix CI [ 50 S] ⚠ · ■ Lunch", " 12:30 ▓ · ■", // the block first, then the meeting
 		"TIMELINE · 0/1 done · ⚠ 1",
 	)
 	h.mustNotSee("Early", " 11:30 ■")
@@ -73,25 +73,25 @@ func TestPlacementAroundMeetings(t *testing.T) {
 		ev("Standup", 9*time.Hour, 9*time.Hour+30*time.Minute),
 		ev("Review", 10*time.Hour, 11*time.Hour),
 	)
-	h.pinned("Alpha !1 ~s")
+	h.pinned("Alpha !3 ~s")
 	h.keys("tab", "b")
 	h.mustSee(" 09:30 ░ Alpha 30m") // D42: the first row free of meetings; the length stops at Review
 	h.keys("j")
 	h.mustSee(" 10:00 ░ Alpha 1h ⚠ · ■ Review") // over a meeting: the whole length, with ⚠
 	h.keys("enter")
 	h.mustSee(`blocked "Alpha" 10:00–11:00 · ⚠ overlaps meeting "Review" 10:00–11:00`,
-		" 10:00 ▓ Alpha [P1 S] ⚠ · ■ Review", "⚠ 1")
+		" 10:00 ▓ Alpha [ 50 S] ⚠ · ■ Review", "⚠ 1")
 
 	beta := h.pinned("Beta !2 ~s")
 	h.block(beta.ID, 14*time.Hour, time.Hour)
-	h.mustSee(" 14:00 ▓ Beta [P2 S]")
-	h.mustNotSee("Beta [P2 S] ⚠")
+	h.mustSee(" 14:00 ▓ Beta [ 33 S]")
+	h.mustNotSee("Beta [ 33 S] ⚠")
 	h.meetings( // a meeting that lands on a block later (D18)
 		ev("Standup", 9*time.Hour, 9*time.Hour+30*time.Minute),
 		ev("Review", 10*time.Hour, 11*time.Hour),
 		ev("Escalation", 14*time.Hour+30*time.Minute, 15*time.Hour),
 	)
-	h.mustSee(" 14:00 ▓ Beta [P2 S] ⚠", " 14:30 ▓ · ■ Escalation", "⚠ 2")
+	h.mustSee(" 14:00 ▓ Beta [ 33 S] ⚠", " 14:30 ▓ · ■ Escalation", "⚠ 2")
 	if b := h.blocks(); len(b) != 2 || b[1].Start != 14*time.Hour {
 		t.Fatalf("the block keeps its time (D18): %+v", b)
 	}
```

Patch `internal/tui/notifications_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/notifications_test.go b/internal/tui/notifications_test.go
index 46e4a96..caf5761 100644
--- a/internal/tui/notifications_test.go
+++ b/internal/tui/notifications_test.go
@@ -49,7 +49,7 @@ func (h *harness) wantNotes(want ...note) {
 
 func TestMeetingAndBlockAlerts(t *testing.T) {
 	h := newCalHarness(t, notifyCfg(config.Notify{Meetings: true, Blocks: true}))
-	rfc := h.pinned("Write RFC !1 ~s")
+	rfc := h.pinned("Write RFC !3 ~s")
 	h.block(rfc.ID, 10*time.Hour, time.Hour)
 	h.meetings(ev("Standup", 10*time.Hour, 10*time.Hour+15*time.Minute))
 	if !h.m.View().ReportFocus {
@@ -72,20 +72,20 @@ func TestMeetingAndBlockAlerts(t *testing.T) {
 
 func TestAlertsOnlyForMomentsPassedWhileRunning(t *testing.T) {
 	h := newHarness(t, notifyCfg(config.Notify{Blocks: true}))
-	early := h.pinned("Early !1 ~s")
+	early := h.pinned("Early !3 ~s")
 	h.block(early.ID, 8*time.Hour+30*time.Minute, 30*time.Minute) // started before launch
 	h.tickAt(9*time.Hour + 20*time.Second)
 	h.wantNotes()
 
 	h.clock.now = dayAt(9*time.Hour + 27*time.Minute)
-	late := h.pinned("Late !1 ~s")
+	late := h.pinned("Late !3 ~s")
 	h.block(late.ID, 9*time.Hour+30*time.Minute, 30*time.Minute) // its 5-minute moment passed already
-	moved := h.pinned("Moved !1 ~s")
+	moved := h.pinned("Moved !3 ~s")
 	b := h.block(moved.ID, 10*time.Hour, 30*time.Minute)
 	if _, err := h.svc.MoveBlock(context.Background(), b.ID, 11*time.Hour); err != nil {
 		t.Fatal(err)
 	}
-	done := h.pinned("Done !1 ~s")
+	done := h.pinned("Done !3 ~s")
 	h.block(done.ID, 12*time.Hour, 30*time.Minute)
 	if _, err := h.svc.Done(context.Background(), done.ID); err != nil {
 		t.Fatal(err)
@@ -105,13 +105,13 @@ func TestAlertsOnlyForMomentsPassedWhileRunning(t *testing.T) {
 	h.wantNotes(note{"tt · now", "Late"}, note{"tt · in 5 min", "Moved"})
 
 	// Asleep from 12:00 to 13:58: the moments it slept through stay quiet.
-	again := h.pinned("Again !1 ~s")
+	again := h.pinned("Again !3 ~s")
 	h.block(again.ID, 13*time.Hour, 30*time.Minute)
 	h.tickAt(13*time.Hour + 58*time.Minute)
 	h.wantNotes(note{"tt · now", "Late"}, note{"tt · in 5 min", "Moved"})
 
 	// Quit and start again at 15:55:30: the 15:55 moment was before launch.
-	last := h.pinned("Last !1 ~s")
+	last := h.pinned("Last !3 ~s")
 	h.block(last.ID, 16*time.Hour, 30*time.Minute)
 	h.clock.now = dayAt(15*time.Hour + 55*time.Minute + 30*time.Second)
 	h.relaunch()
@@ -122,7 +122,7 @@ func TestAlertsOnlyForMomentsPassedWhileRunning(t *testing.T) {
 
 func TestNoAlertsWhileFocused(t *testing.T) {
 	h := newHarness(t, notifyCfg(config.Notify{Blocks: true}))
-	rfc := h.pinned("Write RFC !1 ~s")
+	rfc := h.pinned("Write RFC !3 ~s")
 	h.block(rfc.ID, 10*time.Hour, time.Hour)
 	h.send(tea.FocusMsg{})
 	h.tickAt(9*time.Hour + 55*time.Minute)
@@ -136,7 +136,7 @@ func TestNoAlertsWhileFocused(t *testing.T) {
 
 func TestNotificationsAreOffByDefault(t *testing.T) {
 	h := newCalHarness(t, config.Default())
-	rfc := h.pinned("Write RFC !1 ~s")
+	rfc := h.pinned("Write RFC !3 ~s")
 	h.block(rfc.ID, 10*time.Hour, time.Hour)
 	h.meetings(ev("Standup", 10*time.Hour, 10*time.Hour+15*time.Minute))
 	h.tickAt(9*time.Hour + 55*time.Minute)
@@ -193,7 +193,7 @@ func TestNewReviewRequestsNotify(t *testing.T) {
 func TestAFailedSendSaysSoOnce(t *testing.T) {
 	h := newHarness(t, notifyCfg(config.Notify{Blocks: true}))
 	h.notifyErr = errors.New("exec: \"osascript\": executable file not found in $PATH")
-	rfc := h.pinned("Write RFC !1 ~s")
+	rfc := h.pinned("Write RFC !3 ~s")
 	h.block(rfc.ID, 10*time.Hour, time.Hour)
 	h.tickAt(9*time.Hour + 55*time.Minute)
 	h.mustSee(`can't notify: exec: "osascript": executable file not found`)
@@ -232,9 +232,9 @@ func TestAStaleLoadIsNotANewFetch(t *testing.T) {
 func TestABatchTriesEveryNote(t *testing.T) {
 	h := newHarness(t, notifyCfg(config.Notify{Blocks: true}))
 	h.notifyErr = errors.New("no osascript")
-	x := h.pinned("X !1 ~s")
+	x := h.pinned("X !3 ~s")
 	h.block(x.ID, 10*time.Hour, 5*time.Minute)
-	y := h.pinned("Y !1 ~s")
+	y := h.pinned("Y !3 ~s")
 	h.block(y.ID, 10*time.Hour+5*time.Minute, 30*time.Minute)
 	h.tickAt(9*time.Hour + 59*time.Minute)
 	h.tickAt(10 * time.Hour) // "now" X and "in 5 min" Y in one tick
```

Patch `internal/tui/people_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/people_test.go b/internal/tui/people_test.go
index d986ddf..f8bd117 100644
--- a/internal/tui/people_test.go
+++ b/internal/tui/people_test.go
@@ -28,9 +28,9 @@ func (h *harness) before(parts ...string) {
 
 func TestPeopleOpensFromTodayAndTheLog(t *testing.T) {
 	h := newHarness(t, config.Default())
-	h.add("Fix login @ana !1 ~s")
+	h.add("Fix login @ana !3 ~s")
 	h.keys("P")
-	h.mustSee("tt · People", "PEOPLE · 1", "ana · 1 open", "@ana", "[P1 S] Fix login")
+	h.mustSee("tt · People", "PEOPLE · 1", "ana · 1 open", "@ana", "[ 50 S] Fix login")
 	h.keys("esc")
 	if h.m.screen != screenToday {
 		t.Fatalf("esc from People opened from Today: screen %d", h.m.screen)
@@ -54,7 +54,7 @@ func TestPeopleOpensFromTodayAndTheLog(t *testing.T) {
 
 func TestPeopleFromTheReviewsLane(t *testing.T) {
 	h := newGHHarness(t, config.Default())
-	h.add("Fix login @ana !1 ~s")
+	h.add("Fix login @ana !3 ~s")
 	threeReviews(h)
 	h.keys("w", "P") // a screen key leaves the lane (D49)
 	h.mustSee("tt · People")
@@ -67,11 +67,11 @@ func TestPeopleFromTheReviewsLane(t *testing.T) {
 func TestPeopleListsEveryRequester(t *testing.T) {
 	h := newHarness(t, config.Default())
 	ctx := context.Background()
-	h.add("Schema review @ana !1 ~s")
+	h.add("Schema review @ana !3 ~s")
 	h.add("Later ask @ana !2 ~m")
-	old := h.add("Old ask @ana !1 ~s")
-	idea := h.add("Raff's idea @raff !3 ~s")
-	h.add("Nobody asked !1 ~s")
+	old := h.add("Old ask @ana !3 ~s")
+	idea := h.add("Raff's idea @raff !1 ~s")
+	h.add("Nobody asked !3 ~s")
 	if _, err := h.svc.Done(ctx, old.ID); err != nil {
 		t.Fatal(err)
 	}
@@ -93,15 +93,15 @@ func TestPeopleListsEveryRequester(t *testing.T) {
 func TestPeopleShowsAPersonsTasks(t *testing.T) {
 	h := newHarness(t, config.Default())
 	ctx := context.Background()
-	login := h.add("Fix login @ana !1 ~s")
+	login := h.add("Fix login @ana !3 ~s")
 	h.add("Write RFC @ana !2 ~m")
-	parent := h.add("Profiles !1 ~l")
+	parent := h.add("Profiles !3 ~l")
 	if _, _, err := h.svc.AddChild(ctx, parent.ID, "Import flow @ana ~s"); err != nil {
 		t.Fatal(err)
 	}
-	bug := h.add("Old bug @ana !1 ~s")
-	idea := h.add("Idea @ana !3 ~s")
-	h.add("Bob's ask @bob !1 ~s")
+	bug := h.add("Old bug @ana !3 ~s")
+	idea := h.add("Idea @ana !1 ~s")
+	h.add("Bob's ask @bob !3 ~s")
 	if _, err := h.svc.Pin(ctx, login.ID); err != nil {
 		t.Fatal(err)
 	}
@@ -113,18 +113,17 @@ func TestPeopleShowsAPersonsTasks(t *testing.T) {
 		t.Fatal(err)
 	}
 	h.keys("R", "P")
-	h.before("@ana",
-		"P1 · someone is blocked", "[P1 S] Fix login 📌", fmt.Sprintf("[P1 S] Import flow ↳ #%d", parent.ID),
-		"P2 · important this month", "[P2 M] Write RFC",
+	h.before("@ana", // the urgency order, flat (D72)
+		"[ 50 S] Fix login 📌", fmt.Sprintf("[ 50 S] Import flow ↳ #%d", parent.ID), "[ 33 M] Write RFC",
 		"closed", "Idea ✗ Mon 12 Oct", "Old bug ✓ Mon 12 Oct") // newest first
 	h.mustNotSee("Fix login @ana", "Bob's ask", "Profiles")
 }
 
 func TestPeopleTaskKeys(t *testing.T) {
 	h := newHarness(t, config.Default())
-	login := h.add("Fix login @ana !1 ~s")
+	login := h.add("Fix login @ana !3 ~s")
 	h.add("Write RFC @ana !2 ~m")
-	h.add("Bob's ask @bob !1 ~s")
+	h.add("Bob's ask @bob !3 ~s")
 	h.keys("P", "tab")
 	h.keys("x")
 	h.mustSee(`✓ "Fix login"`)
@@ -132,10 +131,10 @@ func TestPeopleTaskKeys(t *testing.T) {
 	h.keys("ctrl+z")
 	h.mustSee(`undid done "Fix login"`, "ana · 2 open")
 	h.keys("p")
-	h.mustSee(`pinned "Fix login"`, "[P1 S] Fix login 📌")
+	h.mustSee(`pinned "Fix login"`, "[ 50 S] Fix login 📌")
 
 	h.keys("e", "ctrl+u") // the edit line, cleared
-	h.typeText("Fix login @bob !1 ~s")
+	h.typeText("Fix login @bob !3 ~s")
 	h.keys("enter")
 	if got := h.task(login.ID).Requester; got != "bob" {
 		t.Fatalf("requester after e = %q", got)
@@ -144,7 +143,7 @@ func TestPeopleTaskKeys(t *testing.T) {
 		t.Errorf("the cursor stays on the person: %q", h.m.people.selected)
 	}
 	h.mustSee("ana · 1 open", "bob · 2 open", "@ana", "Write RFC", `edited "Fix login"`)
-	h.mustNotSee("[P1 S] Fix login")
+	h.mustNotSee("[ 50 S] Fix login")
 	h.keys("enter")
 	h.mustSee("tt · Detail", "Write RFC")
 }
@@ -152,7 +151,7 @@ func TestPeopleTaskKeys(t *testing.T) {
 func TestPeopleFilter(t *testing.T) {
 	h := newHarness(t, config.Default())
 	for _, who := range []string{"ana", "anabel", "raff", "bob"} {
-		h.add(fmt.Sprintf("Ask from %s @%s !1 ~s", who, who))
+		h.add(fmt.Sprintf("Ask from %s @%s !3 ~s", who, who))
 	}
 	h.keys("P", "/")
 	h.typeText("an")
@@ -186,15 +185,15 @@ func TestPeopleFilter(t *testing.T) {
 func TestPeopleCursorAcrossReloads(t *testing.T) {
 	h := newHarness(t, config.Default())
 	ctx := context.Background()
-	h.add("Ana's @ana !1 ~s")
-	h.add("Bob's @bob !1 ~s")
+	h.add("Ana's @ana !3 ~s")
+	h.add("Bob's @bob !3 ~s")
 	h.add("Cat's first @cat !2 ~s")
 	second := h.add("Cat's second @cat !2 ~s")
 	h.keys("P", "j", "j", "tab", "j")
 	if r, _ := h.m.people.tasks.selectedRow(); r.task.ID != second.ID {
 		t.Fatalf("on cat's second task, got #%d", r.task.ID)
 	}
-	h.add("Cat's urgent @cat !1 ~s") // sorts above both
+	h.add("Cat's urgent @cat !3 ~s") // sorts above both
 	if r, _ := h.m.people.tasks.selectedRow(); r.task.ID != second.ID {
 		t.Errorf("a reload keeps the task under the cursor, got #%d", r.task.ID)
 	}
@@ -213,8 +212,8 @@ func TestPeopleCursorAcrossReloads(t *testing.T) {
 func TestPeopleFitsAShortTerminal(t *testing.T) {
 	h := newHarness(t, config.Default())
 	for i := 1; i <= 12; i++ {
-		h.add(fmt.Sprintf("Ask %02d @p%02d !1 ~s", i, i))
-		h.add(fmt.Sprintf("Task %02d @p01 !1 ~s", i))
+		h.add(fmt.Sprintf("Ask %02d @p%02d !3 ~s", i, i))
+		h.add(fmt.Sprintf("Task %02d @p01 !3 ~s", i))
 	}
 	h.send(tea.WindowSizeMsg{Width: 100, Height: 8})
 	h.keys("P")
@@ -245,7 +244,7 @@ func TestPeopleFitsAShortTerminal(t *testing.T) {
 
 func TestPeopleWithNobody(t *testing.T) {
 	h := newHarness(t, config.Default())
-	h.add("Nobody asked !1 ~s")
+	h.add("Nobody asked !3 ~s")
 	h.keys("P")
 	h.mustSee("PEOPLE · 0", "nobody yet · @who names who asked")
 	h.keys("tab", "x", "enter", "j") // nothing to act on
```

Patch `internal/tui/pick_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/pick_test.go b/internal/tui/pick_test.go
index 64a9656..dc2f413 100644
--- a/internal/tui/pick_test.go
+++ b/internal/tui/pick_test.go
@@ -14,13 +14,18 @@ import (
 
 func TestGroupingCyclesAndIsRemembered(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	h.add("Small one !1 ~s @ana")
 	h.add("Medium one !2 ~m")
-	h.mustSee("BACKLOG · by priority", "P1 · someone is blocked", "P2 ·")
+	h.add("Small one !3 ~s @ana")
+	// D72: urgency is a flat sort, the most urgent first, with no headings.
+	h.mustSee("BACKLOG · by urgency", "│   [ 50 S] Small one @ana", "│   [ 33 M] Medium one")
+	if s := h.screen(); strings.Index(s, "Small one") > strings.Index(s, "Medium one") {
+		t.Errorf("the more urgent task comes first:\n%s", s)
+	}
 
 	h.keys("g")
-	h.mustSee("grouped by size", "BACKLOG · by size", "\n│ S ", "[P1 S] Small one", "\n│ M ")
-	h.mustNotSee("P1 · someone is blocked")
+	h.mustSee("sorted by impact", "BACKLOG · by impact", "│   [ 50 S] Small one @ana")
+	h.keys("g")
+	h.mustSee("grouped by size", "BACKLOG · by size", "\n│ S ", "[ 50 S] Small one", "\n│ M ")
 	h.keys("g")
 	h.mustSee("BACKLOG · by created", "this week")
 	h.keys("g")
@@ -38,25 +43,29 @@ func TestGroupingCyclesAndIsRemembered(t *testing.T) {
 	h2.mustSee("BACKLOG · by requester")
 
 	h.keys("g")
-	h.mustSee("BACKLOG · by priority")
+	h.mustSee("BACKLOG · by urgency")
 }
 
-func TestUnknownRememberedGroupingFallsBackToPriority(t *testing.T) {
-	h := newLogHarness(t, config.Default())
-	if err := h.svc.SetUIState(context.Background(), groupKey, "colour"); err != nil {
-		t.Fatal(err)
+// A remembered grouping tt no longer knows, "priority" from before D72
+// among them, falls back to urgency.
+func TestUnknownRememberedGroupingFallsBackToUrgency(t *testing.T) {
+	for _, remembered := range []string{"colour", "priority"} {
+		h := newLogHarness(t, config.Default())
+		if err := h.svc.SetUIState(context.Background(), groupKey, remembered); err != nil {
+			t.Fatal(err)
+		}
+		h2 := &harness{t: t, svc: h.svc, clock: h.clock, path: h.path,
+			m: New(context.Background(), h.svc, h.m.opts)}
+		h2.run(h2.m.Init())
+		h2.keys("L")
+		h2.mustSee("BACKLOG · by urgency")
 	}
-	h2 := &harness{t: t, svc: h.svc, clock: h.clock, path: h.path,
-		m: New(context.Background(), h.svc, h.m.opts)}
-	h2.run(h2.m.Init())
-	h2.keys("L")
-	h2.mustSee("BACKLOG · by priority")
 }
 
 func TestFilteredBacklogPlacement(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
-	parent := h.add("Auth migration !1 ~xl #auth")
+	parent := h.add("Auth migration !3 ~xl #auth")
 	for _, line := range []string{"Rollback plan ~s", "Token spike ~m"} {
 		kid := h.add(line)
 		if _, err := h.svc.SetParent(ctx, kid.ID, parent.ID); err != nil {
@@ -65,8 +74,8 @@ func TestFilteredBacklogPlacement(t *testing.T) {
 	}
 	h.add("Schema idea")                   // Inbox
 	pinned := h.add("Schema review !2 ~s") // pinned below
-	h.add("Schema backlog !3 ~m")
-	done := h.add("Schema shipped !1 ~s")
+	h.add("Schema backlog !1 ~m")
+	done := h.add("Schema shipped !3 ~s")
 	if _, err := h.svc.Pin(ctx, pinned.ID); err != nil {
 		t.Fatal(err)
 	}
@@ -80,7 +89,7 @@ func TestFilteredBacklogPlacement(t *testing.T) {
 	if err := h.m.applyFilter("schema"); err != nil {
 		t.Fatal(err)
 	}
-	h.mustSee("BACKLOG · by priority · 1 match", "filter › schema · esc clears", "[P3 M] Schema backlog")
+	h.mustSee("BACKLOG · by urgency · 1 match", "filter › schema · esc clears", "[ 17 M] Schema backlog")
 	h.mustNotSee("Schema idea", "Schema shipped", "unset")
 
 	// is: widens it to closed backlog tasks (D26b).
@@ -94,17 +103,17 @@ func TestFilteredBacklogPlacement(t *testing.T) {
 	if err := h.m.applyFilter("rollback"); err != nil {
 		t.Fatal(err)
 	}
-	h.mustSee("1 match", fmt.Sprintf("[P1 S] Rollback plan ↳ #%d", parent.ID))
+	h.mustSee("1 match", fmt.Sprintf("[ 50 S] Rollback plan ↳ #%d", parent.ID))
 
 	// A matching parent lists its matching children beneath it, expanded.
 	if err := h.m.applyFilter("#auth"); err != nil {
 		t.Fatal(err)
 	}
-	h.mustSee("3 matches", "▾ [P1 XL] Auth migration 0/2", "    [P1 S] Rollback plan", "    [P1 M] Token spike")
+	h.mustSee("3 matches", "▾ [ 50 XL] Auth migration 0/2", "    [ 50 S] Rollback plan", "    [ 50 M] Token spike")
 	if err := h.m.applyFilter("#auth spike"); err != nil {
 		t.Fatal(err)
 	}
-	h.mustSee("1 match", fmt.Sprintf("[P1 M] Token spike ↳ #%d", parent.ID))
+	h.mustSee("1 match", fmt.Sprintf("[ 50 M] Token spike ↳ #%d", parent.ID))
 
 	// A bad filter keeps the current one.
 	if err := h.m.applyFilter("!9"); err == nil {
@@ -117,7 +126,7 @@ func TestFilteredBacklogPlacement(t *testing.T) {
 	}
 	h.mustSee("0 matches", "filter › zzz · esc clears", "no matches")
 	h.keys("esc")
-	h.mustSee("filter cleared", "▸ [P1 XL] Auth migration 0/2")
+	h.mustSee("filter cleared", "▸ [ 50 XL] Auth migration 0/2")
 	h.mustNotSee("matches", "Schema idea")
 
 	// Children of a pinned parent live under it in Today, filtered or not.
@@ -128,20 +137,22 @@ func TestFilteredBacklogPlacement(t *testing.T) {
 	if err := h.m.applyFilter("rollback"); err != nil {
 		t.Fatal(err)
 	}
-	h.mustSee("0 matches", "filter › rollback · esc clears", "    [P1 S] Rollback plan")
+	h.mustSee("0 matches", "filter › rollback · esc clears", "    [ 50 S] Rollback plan")
 }
 
 func TestScrollWindowMovesOnlyWhenTheCursorLeavesIt(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	for i := 1; i <= 10; i++ {
-		h.add(fmt.Sprintf("Task %02d !1 ~s", i))
+		h.add(fmt.Sprintf("Task %02d !3 ~s", i))
 	}
 	h.send(tea.WindowSizeMsg{Width: 140, Height: 11}) // 6 rows per framed pane
+	// Grouped by size: one heading, S, above the tasks.
+	h.keys("g", "g")
 	for range 9 {
 		h.keys("j")
 	}
 	h.mustSee("Task 05", "Task 10")
-	h.mustNotSee("Task 04", "P1 · someone")
+	h.mustNotSee("Task 04", "│ S ")
 	h.keys("k") // the cursor moves up inside the window; the window stays
 	h.mustSee("Task 05", "Task 10")
 	h.mustNotSee("Task 04")
@@ -153,13 +164,13 @@ func TestScrollWindowMovesOnlyWhenTheCursorLeavesIt(t *testing.T) {
 	for range 3 {
 		h.keys("k")
 	}
-	h.mustSee("P1 · someone is blocked", "Task 01") // the heading above the first task comes along
+	h.mustSee("│ S ", "Task 01") // the heading above the first task comes along
 }
 
 func TestAllDroppedChildrenHideProgress(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
-	parent := h.add("Parent !1 ~l")
+	parent := h.add("Parent !3 ~l")
 	kid := h.add("Kid ~s")
 	if _, err := h.svc.SetParent(ctx, kid.ID, parent.ID); err != nil {
 		t.Fatal(err)
@@ -169,7 +180,7 @@ func TestAllDroppedChildrenHideProgress(t *testing.T) {
 		t.Fatal(err)
 	}
 	h.keys("R")
-	h.mustSee("[P1 L] Parent")
+	h.mustSee("[ 50 L] Parent")
 	h.mustNotSee("0/0")
 }
 
@@ -177,20 +188,20 @@ func TestAllDroppedChildrenHideProgress(t *testing.T) {
 // so pinned yesterday reads 📌1, like ↻1 for a block left unfinished yesterday.
 func TestPinCountShowsDaysCarriedOver(t *testing.T) {
 	h := newHarness(t, config.Default())
-	task := h.add("Carried !1 ~s")
+	task := h.add("Carried !3 ~s")
 	if _, err := h.svc.Pin(context.Background(), task.ID); err != nil {
 		t.Fatal(err)
 	}
 	h.keys("R")
-	h.mustSee("[P1 S] Carried")
+	h.mustSee("[ 50 S] Carried")
 	h.mustNotSee("📌")
 
 	for _, want := range []string{"📌1", "📌2"} {
 		h.clock.now = h.clock.now.Add(24 * time.Hour)
 		h.send(clockTickMsg{})
 		h.keys("enter") // past the new-day banner
-		h.mustSee("[P1 S] Carried " + want)
+		h.mustSee("[ 50 S] Carried " + want)
 	}
 	h.keys("tab", "enter") // the backlog is empty: open the candidate
-	h.mustSee(fmt.Sprintf("#%d Carried  [P1 S] 📌2", task.ID))
+	h.mustSee(fmt.Sprintf("#%d Carried  [ 50 S] 📌2", task.ID))
 }
```

Patch `internal/tui/place_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/place_test.go b/internal/tui/place_test.go
index 56563b1..a77e544 100644
--- a/internal/tui/place_test.go
+++ b/internal/tui/place_test.go
@@ -32,7 +32,7 @@ func (h *harness) blocks() []domain.Block {
 
 func TestPlaceACandidate(t *testing.T) {
 	h := newHarness(t, config.Default())
-	a := h.pinned("Alpha !1 ~s")
+	a := h.pinned("Alpha !3 ~s")
 	h.keys("tab", "b")
 	if h.m.today.focus != focusTimeline || h.m.today.at != 9*time.Hour {
 		t.Fatalf("b moves to the timeline's first free row from now: focus %d at %v", h.m.today.focus, h.m.today.at)
@@ -41,7 +41,7 @@ func TestPlaceACandidate(t *testing.T) {
 	h.keys("j")
 	h.mustSee(" 09:30 ░ Alpha 1h", " 10:00 ░")
 	h.keys("enter")
-	h.mustSee(`blocked "Alpha" 09:30–10:30`, " 09:30 ▓ Alpha [P1 S]")
+	h.mustSee(`blocked "Alpha" 09:30–10:30`, " 09:30 ▓ Alpha [ 50 S]")
 	if b := h.blocks(); len(b) != 1 || b[0].TaskID != a.ID || b[0].Start != 9*time.Hour+30*time.Minute || b[0].Duration != time.Hour {
 		t.Fatalf("blocks = %+v", b)
 	}
@@ -54,12 +54,12 @@ func TestPlaceACandidate(t *testing.T) {
 
 func TestPlacementLengthAndFreeRows(t *testing.T) {
 	h := newHarness(t, config.Default())
-	busy := h.pinned("Busy !1 ~s")
+	busy := h.pinned("Busy !3 ~s")
 	h.block(busy.ID, 9*time.Hour, time.Hour)
-	later := h.pinned("Later !1 ~s")
+	later := h.pinned("Later !3 ~s")
 	h.block(later.ID, 11*time.Hour, time.Hour)
 	h.pinned("Medium !2 ~m")
-	h.pinned("Unsized !3")
+	h.pinned("Unsized !1")
 
 	h.keys("tab", "j", "j", "b") // Medium (candidates in plan order: Busy, Later, Medium, Unsized)
 	if h.m.today.at != 10*time.Hour {
@@ -80,9 +80,9 @@ func TestPlacementLengthAndFreeRows(t *testing.T) {
 
 func TestDropOntoABlockIsRefused(t *testing.T) {
 	h := newHarness(t, config.Default())
-	busy := h.pinned("Busy !1 ~s")
+	busy := h.pinned("Busy !3 ~s")
 	h.block(busy.ID, 10*time.Hour, time.Hour)
-	h.pinned("Alpha !1 ~s")
+	h.pinned("Alpha !3 ~s")
 	h.keys("tab", "j", "b") // at 09:00: Alpha would need 09:00–10:00, which fits
 	h.keys("j", "j")        // 10:00 is inside Busy
 	h.keys("enter")
@@ -96,12 +96,12 @@ func TestDropOntoABlockIsRefused(t *testing.T) {
 
 func TestPlanningNudges(t *testing.T) {
 	h := newHarness(t, config.Default())
-	h.pinned("Large !1 ~l")
+	h.pinned("Large !3 ~l")
 	h.keys("tab", "b", "enter")
 	h.mustSee(`blocked "Large" 09:00–17:00 · big task · consider splitting`) // D37
 
 	ctx := context.Background()
-	parent := h.pinned("Parent !1 ~xl")
+	parent := h.pinned("Parent !3 ~xl")
 	if _, _, err := h.svc.AddChild(ctx, parent.ID, "Child ~s"); err != nil {
 		t.Fatal(err)
 	}
@@ -134,9 +134,9 @@ func TestPlanningNudges(t *testing.T) {
 
 func TestMoveResizeFineTuneUnblock(t *testing.T) {
 	h := newHarness(t, config.Default())
-	a := h.pinned("Alpha !1 ~s")
+	a := h.pinned("Alpha !3 ~s")
 	h.block(a.ID, 10*time.Hour, time.Hour)
-	other := h.pinned("Other !1 ~s")
+	other := h.pinned("Other !3 ~s")
 	h.block(other.ID, 13*time.Hour, time.Hour)
 
 	h.keys("j", "j", "m") // on Alpha at 10:00
@@ -174,7 +174,7 @@ func TestMoveResizeFineTuneUnblock(t *testing.T) {
 	}
 	h.typeText("10:10 45m")
 	h.keys("enter")
-	h.mustSee(`"Alpha" now 10:10–10:55`, " 10:00 ▓ 10:10–10:55 Alpha [P1 S]")
+	h.mustSee(`"Alpha" now 10:10–10:55`, " 10:00 ▓ 10:10–10:55 Alpha [ 50 S]")
 	h.keys("ctrl+z")
 	if b := h.blocks()[0]; b.Start != 10*time.Hour || b.Duration != 30*time.Minute {
 		t.Errorf("fine-tune undoes in one step: %+v", b)
@@ -192,7 +192,7 @@ func TestMoveResizeFineTuneUnblock(t *testing.T) {
 
 func TestAMovedBlockThatVanishesStopsTheMove(t *testing.T) {
 	h := newHarness(t, config.Default())
-	a := h.pinned("Alpha !1 ~s")
+	a := h.pinned("Alpha !3 ~s")
 	b := h.block(a.ID, 10*time.Hour, time.Hour)
 	h.keys("j", "j", "m")
 	if _, err := h.svc.Unblock(context.Background(), b.ID); err != nil { // e.g. from another tt
@@ -209,7 +209,7 @@ func TestAMovedBlockThatVanishesStopsTheMove(t *testing.T) {
 // no hint or help line offers it.
 func TestNoCommitStep(t *testing.T) {
 	h := newHarness(t, config.Default())
-	a := h.pinned("Alpha !1 ~s")
+	a := h.pinned("Alpha !3 ~s")
 	h.block(a.ID, 10*time.Hour, 90*time.Minute)
 	for _, focus := range []string{"timeline", "candidates"} {
 		h.keys("c")
@@ -226,7 +226,7 @@ func TestNoCommitStep(t *testing.T) {
 func TestUnpinningABlockedParentIsRefusedInTheTUI(t *testing.T) {
 	h := newHarness(t, config.Default())
 	ctx := context.Background()
-	parent := h.pinned("Parent !1 ~xl")
+	parent := h.pinned("Parent !3 ~xl")
 	if _, _, err := h.svc.AddChild(ctx, parent.ID, "Child ~s"); err != nil {
 		t.Fatal(err)
 	}
@@ -263,7 +263,7 @@ func (h *harness) unpinDirectly(id int64) {
 func TestPinRepairsAParentPinnedBeforeD34(t *testing.T) {
 	h := newHarness(t, config.Default())
 	ctx := context.Background()
-	parent := h.pinned("Parent !1 ~xl")
+	parent := h.pinned("Parent !3 ~xl")
 	kid, _, err := h.svc.AddChild(ctx, parent.ID, "Child ~s")
 	if err != nil {
 		t.Fatal(err)
@@ -288,7 +288,7 @@ func TestPinRepairsAParentPinnedBeforeD34(t *testing.T) {
 
 func TestBlockingOutsideWorkingHours(t *testing.T) {
 	h := newHarness(t, config.Default()) // working hours 08:00–19:00 (D39)
-	late := h.pinned("Late !1 ~s")
+	late := h.pinned("Late !3 ~s")
 	h.clock.now = time.Date(2026, 10, 13, 0, 20, 0, 0, time.UTC) // past midnight: still the 12th's planning day
 	h.m.today.placed = false                                     // as when tt starts now
 	h.keys("R")
@@ -329,7 +329,7 @@ func TestBlockingOutsideWorkingHours(t *testing.T) {
 
 func TestMovingAnOffGridBlockKeepsItsStart(t *testing.T) {
 	h := newHarness(t, config.Default())
-	a := h.pinned("Alpha !1 ~s")
+	a := h.pinned("Alpha !3 ~s")
 	h.block(a.ID, 10*time.Hour+10*time.Minute, 45*time.Minute)
 	undos := len(h.m.undo)
 	h.keys("j", "j", "m", "enter") // m ⏎ leaves it where it was
```

Patch `internal/tui/program_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/program_test.go b/internal/tui/program_test.go
index bb0d6a6..155d907 100644
--- a/internal/tui/program_test.go
+++ b/internal/tui/program_test.go
@@ -67,7 +67,7 @@ func TestRealProgram(t *testing.T) {
 		t.Fatal(err)
 	}
 	defer other.Close()
-	if _, _, err := app.New(other, clock, cfg).Add(ctx, "From another shell !1 ~s"); err != nil {
+	if _, _, err := app.New(other, clock, cfg).Add(ctx, "From another shell !3 ~s"); err != nil {
 		t.Fatal(err)
 	}
 	for !sawTask.Load() { // only the watch loop can bring it in
```

Patch `internal/tui/rename_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/rename_test.go b/internal/tui/rename_test.go
index 85ee4c3..02adfa3 100644
--- a/internal/tui/rename_test.go
+++ b/internal/tui/rename_test.go
@@ -11,9 +11,9 @@ import (
 
 func TestRenameAPerson(t *testing.T) {
 	h := newHarness(t, config.Default())
-	one := h.add("One @raf !1 ~s")
+	one := h.add("One @raf !3 ~s")
 	two := h.add("Two @raf !2 ~s")
-	h.add("Ana's @ana !1 ~s")
+	h.add("Ana's @ana !3 ~s")
 	h.keys("P", "j")
 	h.keys("r")
 	h.mustSee("rename @raf › raf")
@@ -48,9 +48,9 @@ func TestRenameAPerson(t *testing.T) {
 func TestMergeIntoAnotherPerson(t *testing.T) {
 	h := newHarness(t, config.Default())
 	ctx := context.Background()
-	one := h.add("One @raf !1 ~s")
+	one := h.add("One @raf !3 ~s")
 	two := h.add("Two @raf !2 ~s")
-	own := h.add("Ana's @ana !1 ~s")
+	own := h.add("Ana's @ana !3 ~s")
 	merge := func() {
 		t.Helper()
 		h.keys("r", "ctrl+u")
@@ -96,8 +96,8 @@ func TestMergeIntoAnotherPerson(t *testing.T) {
 
 func TestRenameWhileFiltered(t *testing.T) {
 	h := newHarness(t, config.Default())
-	h.add("One @raf !1 ~s")
-	h.add("Ana's @ana !1 ~s")
+	h.add("One @raf !3 ~s")
+	h.add("Ana's @ana !3 ~s")
 	h.keys("P", "/")
 	h.typeText("ra")
 	h.keys("enter")
@@ -119,8 +119,8 @@ func TestRenameWhileFiltered(t *testing.T) {
 
 func TestRenameOfANameGoneMeanwhile(t *testing.T) {
 	h := newHarness(t, config.Default())
-	h.add("One @raf !1 ~s")
-	h.add("Ana's @ana !1 ~s")
+	h.add("One @raf !3 ~s")
+	h.add("Ana's @ana !3 ~s")
 	h.keys("P", "j")
 	if _, _, err := h.svc.RenameRequester(context.Background(), "raf", "zed"); err != nil {
 		t.Fatal(err) // another shell; the TUI hasn't reloaded yet
```

Patch `internal/tui/reslot_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/reslot_test.go b/internal/tui/reslot_test.go
index 15a4187..395954f 100644
--- a/internal/tui/reslot_test.go
+++ b/internal/tui/reslot_test.go
@@ -15,9 +15,9 @@ import (
 func TestReslotAConflictedBlock(t *testing.T) {
 	h := newCalHarness(t, config.Default())
 	h.meetings(ev("Review", 10*time.Hour, 11*time.Hour), ev("Lunch", 12*time.Hour, 13*time.Hour))
-	a := h.pinned("Alpha !1 ~s")
+	a := h.pinned("Alpha !3 ~s")
 	h.block(a.ID, 10*time.Hour, time.Hour)
-	b := h.pinned("Beta !1 ~s")
+	b := h.pinned("Beta !3 ~s")
 	h.block(b.ID, 11*time.Hour, time.Hour)
 
 	h.keys("j", "j", "j", "j") // 11:00: Beta, no conflict
@@ -39,7 +39,7 @@ func TestReslotAConflictedBlock(t *testing.T) {
 	h.mustSee(`moved "Alpha" to 13:00–14:00`)
 	h.mustNotSee("⚠")
 	h.keys("ctrl+z")
-	h.mustSee(`undid move "Alpha"`, " 10:00 ▓ Alpha [P1 S] ⚠")
+	h.mustSee(`undid move "Alpha"`, " 10:00 ▓ Alpha [ 50 S] ⚠")
 
 	h.keys("r", "esc") // esc leaves it where it is
 	if bs := h.blocks(); bs[0].Start != 10*time.Hour || h.m.today.carry != nil {
@@ -50,7 +50,7 @@ func TestReslotAConflictedBlock(t *testing.T) {
 func TestReslotStartsFromNow(t *testing.T) {
 	h := newCalHarness(t, config.Default())
 	h.meetings(ev("Review", 10*time.Hour, 11*time.Hour))
-	a := h.pinned("Alpha !1 ~s")
+	a := h.pinned("Alpha !3 ~s")
 	h.block(a.ID, 10*time.Hour, time.Hour)
 	h.clock.now = h.clock.now.Add(6 * time.Hour) // 15:00
 	h.keys("R")
@@ -63,7 +63,7 @@ func TestReslotStartsFromNow(t *testing.T) {
 func TestReslotWithNoFreeSlot(t *testing.T) {
 	h := newCalHarness(t, config.Default())
 	h.meetings(ev("All hands", 14*time.Hour, 14*time.Hour+30*time.Minute))
-	long := h.pinned("Long !1 ~m")
+	long := h.pinned("Long !3 ~m")
 	h.block(long.ID, 14*time.Hour, 5*time.Hour)              // 14:00–19:00: nothing later fits 5h before 19:00
 	h.keys("j", "j", "j", "j", "j", "j", "j", "j", "j", "j") // 14:00
 	h.keys("r")
@@ -78,8 +78,8 @@ func TestReslotWithNoFreeSlot(t *testing.T) {
 
 func TestARefusedDropKeepsThePickUp(t *testing.T) {
 	h := newHarness(t, config.Default())
-	other := h.pinned("Other !1 ~s")
-	h.pinned("Alpha !1 ~s")
+	other := h.pinned("Other !3 ~s")
+	h.pinned("Alpha !3 ~s")
 	h.keys("tab", "j", "b") // Alpha's ghost at 09:00
 	// Meanwhile another tt blocks Other at 09:00; this one hasn't reloaded yet.
 	if _, _, err := h.svc.Block(context.Background(), other.ID, 9*time.Hour, time.Hour); err != nil {
@@ -90,7 +90,7 @@ func TestARefusedDropKeepsThePickUp(t *testing.T) {
 	if c := h.m.today.carry; c == nil || c.task.Title != "Alpha" || h.m.today.at != 9*time.Hour {
 		t.Fatalf("a refused drop gives the pick-up back where it was: %+v at %v", c, h.m.today.at)
 	}
-	h.mustSee(" 09:00 ░ Alpha 1h · Other [P1 S]") // the reload shows why
+	h.mustSee(" 09:00 ░ Alpha 1h · Other [ 50 S]") // the reload shows why
 	h.keys("j", "j", "enter")
 	h.mustSee(`blocked "Alpha" 10:00–11:00`)
 }
@@ -98,7 +98,7 @@ func TestARefusedDropKeepsThePickUp(t *testing.T) {
 func TestFineTuneRereadsTheBlock(t *testing.T) {
 	ctx := context.Background()
 	h := newHarness(t, config.Default())
-	a := h.pinned("Alpha !1 ~s")
+	a := h.pinned("Alpha !3 ~s")
 	b := h.block(a.ID, 10*time.Hour, time.Hour)
 	h.keys("j", "j", "f")
 	undos := len(h.m.undo)
@@ -133,7 +133,7 @@ func TestFineTuneRereadsTheBlock(t *testing.T) {
 
 func TestScrollKeepsTheCursorWhenRowsAppearAbove(t *testing.T) {
 	h := newHarness(t, config.Default())
-	early := h.pinned("Early !1 ~s")
+	early := h.pinned("Early !3 ~s")
 	h.send(tea.WindowSizeMsg{Width: 100, Height: 12})
 	for range 10 {
 		h.keys("j") // 14:00, scrolled
```

Patch `internal/tui/tab_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/tab_test.go b/internal/tui/tab_test.go
index 95da843..306e335 100644
--- a/internal/tui/tab_test.go
+++ b/internal/tui/tab_test.go
@@ -10,7 +10,7 @@ import (
 func TestTabSkipsAnEmptyPaneInTheLog(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
-	a := h.add("Alpha !1 ~s")
+	a := h.add("Alpha !3 ~s")
 	h.keys("tab")
 	h.mustSee("nothing pinned yet · p pins a task")
 	if h.m.pick.focus != paneBacklog {
@@ -53,7 +53,7 @@ func TestTabSkipsAnEmptyPaneOnToday(t *testing.T) {
 	if h.m.today.focus != focusTimeline {
 		t.Fatal("tab into empty candidates keeps the focus on the timeline")
 	}
-	a := h.pinned("Alpha !1 ~s")
+	a := h.pinned("Alpha !3 ~s")
 	h.keys("tab")
 	if h.m.today.focus != focusCandidates {
 		t.Fatal("tab moves to the candidates once there are some")
@@ -74,7 +74,7 @@ func TestTabSkipsAnEmptyPaneInPeople(t *testing.T) {
 	if h.m.people.onTasks {
 		t.Fatal("tab with nobody listed keeps the focus on the names")
 	}
-	a := h.add("Ask @ana !1 ~s")
+	a := h.add("Ask @ana !3 ~s")
 	h.keys("tab")
 	if !h.m.people.onTasks {
 		t.Fatal("tab moves to ana's tasks")
@@ -91,7 +91,7 @@ func TestTabSkipsAnEmptyPaneInPeople(t *testing.T) {
 
 func TestARefocusLeavesScreensNotOnShowAlone(t *testing.T) {
 	h := newHarness(t, config.Default())
-	a := h.add("Alpha !1 ~s")
+	a := h.add("Alpha !3 ~s")
 	h.keys("L") // the Log's focus is on the backlog
 	h.keys("esc")
 	if _, err := h.svc.Pin(context.Background(), a.ID); err != nil {
@@ -105,7 +105,7 @@ func TestARefocusLeavesScreensNotOnShowAlone(t *testing.T) {
 
 func TestTypingAFilterKeepsTheFocusThroughAReload(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	a := h.add("Alpha !1 ~s")
+	a := h.add("Alpha !3 ~s")
 	h.pinned("Beta !2 ~s")
 	if h.m.pick.focus != paneBacklog {
 		t.Fatal("the focus starts on the backlog")
```

Patch `internal/tui/today_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/today_test.go b/internal/tui/today_test.go
index 6fd616b..fcb5613 100644
--- a/internal/tui/today_test.go
+++ b/internal/tui/today_test.go
@@ -36,7 +36,7 @@ func TestTodayIsTheHomeScreen(t *testing.T) {
 	h.mustSee("tt · Today")
 
 	// esc in the Log clears a filter first, then goes back to Today.
-	h.add("Alpha !1 ~s")
+	h.add("Alpha !3 ~s")
 	h.keys("L", "/")
 	h.typeText("zzz")
 	h.keys("enter", "esc")
@@ -63,18 +63,18 @@ func TestTodayIsTheHomeScreen(t *testing.T) {
 
 func TestTimelineShowsBlocks(t *testing.T) {
 	h := newHarness(t, config.Default())
-	a := h.add("Alpha !1 ~s")
+	a := h.add("Alpha !3 ~s")
 	b := h.add("Beta !2 ~m")
 	h.block(a.ID, 10*time.Hour, time.Hour)
 	h.block(b.ID, 11*time.Hour+10*time.Minute, 45*time.Minute)
-	h.mustSee("TIMELINE · 0/2 done", " 10:00 ▓ Alpha [P1 S]", " 10:30 ▓ ", " 11:00 ▓ 11:10–11:55 Beta [P2 M]", " 11:30 ▓ ")
-	h.mustSee("CANDIDATES", "[P1 S] Alpha ▓") // blocking pins (D12a)
+	h.mustSee("TIMELINE · 0/2 done", " 10:00 ▓ Alpha [ 50 S]", " 10:30 ▓ ", " 11:00 ▓ 11:10–11:55 Beta [ 33 M]", " 11:30 ▓ ")
+	h.mustSee("CANDIDATES", "[ 50 S] Alpha ▓") // blocking pins (D12a)
 
 	if _, err := h.svc.Done(context.Background(), a.ID); err != nil {
 		t.Fatal(err)
 	}
 	h.keys("R")
-	h.mustSee("TIMELINE · 1/2 done", " 10:00 ▓ Alpha [P1 S] ✓") // P3: done blocks stay
+	h.mustSee("TIMELINE · 1/2 done", " 10:00 ▓ Alpha [ 50 S] ✓") // P3: done blocks stay
 }
 
 // blockDirectly puts a block on today's timeline behind the service's back:
@@ -112,9 +112,9 @@ func TestTimelineShowsOnlyWorkingHours(t *testing.T) {
 	}
 
 	// A block outside the hours (placed before D39) keeps a row of its own.
-	late := h.add("Late one !1 ~s")
+	late := h.add("Late one !3 ~s")
 	h.blockDirectly(late.ID, 21*time.Hour, time.Hour)
-	h.mustSee(" 21:00 ▓ 21:00–22:00 Late one [P1 S] · outside of working hours", "TIMELINE · 0/1 done")
+	h.mustSee(" 21:00 ▓ 21:00–22:00 Late one [ 50 S] · outside of working hours", "TIMELINE · 0/1 done")
 	h.mustNotSee(" 21:30 ")
 	for range 30 {
 		h.keys("j")
@@ -133,19 +133,19 @@ func TestTimelineShowsOnlyWorkingHours(t *testing.T) {
 // A block whose task has a note shows ✎ after the title, like the rows (D62).
 func TestTimelineMarksBlocksWithANote(t *testing.T) {
 	h := newHarness(t, config.Default())
-	a := h.add("Alpha !1 ~s")
+	a := h.add("Alpha !3 ~s")
 	b := h.add("Beta !2 ~m")
 	if _, err := h.svc.SetNote(context.Background(), a.ID, "context"); err != nil {
 		t.Fatal(err)
 	}
 	h.block(a.ID, 10*time.Hour, time.Hour)
 	h.block(b.ID, 11*time.Hour+10*time.Minute, 45*time.Minute)
-	h.mustSee(" 10:00 ▓ Alpha ✎ [P1 S]", " 11:00 ▓ 11:10–11:55 Beta [P2 M]")
+	h.mustSee(" 10:00 ▓ Alpha ✎ [ 50 S]", " 11:00 ▓ 11:10–11:55 Beta [ 33 M]")
 }
 
 func TestTimelineCursorAndBlockKeys(t *testing.T) {
 	h := newHarness(t, config.Default())
-	a := h.add("Alpha !1 ~s")
+	a := h.add("Alpha !3 ~s")
 	h.block(a.ID, 10*time.Hour, time.Hour)
 	if got := h.m.today.at; got != 9*time.Hour {
 		t.Fatalf("the cursor starts on the now row: %v", got)
@@ -173,7 +173,7 @@ func TestTimelineCursorAndBlockKeys(t *testing.T) {
 func TestPinningAParentInTheTUIWithChildren(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
-	parent, _, err := h.svc.Add(ctx, "Parent !1 ~xl")
+	parent, _, err := h.svc.Add(ctx, "Parent !3 ~xl")
 	if err != nil {
 		t.Fatal(err)
 	}
@@ -203,7 +203,7 @@ func TestPinningAParentInTheTUIWithChildren(t *testing.T) {
 func TestScreensShareTheHeightBudget(t *testing.T) {
 	h := newHarness(t, config.Default())
 	for i := range 8 {
-		h.add("Task " + string(rune('A'+i)) + " !1 ~s")
+		h.add("Task " + string(rune('A'+i)) + " !3 ~s")
 	}
 	h.add("Untriaged @ana #x https://example.com -- a note")
 	for _, height := range []int{6, 10, 16} {
```

Patch `internal/tui/tui_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/tui_test.go b/internal/tui/tui_test.go
index acbc3a2..b14860c 100644
--- a/internal/tui/tui_test.go
+++ b/internal/tui/tui_test.go
@@ -27,12 +27,12 @@ func TestQuickAddThenTriageInTwoKeystrokes(t *testing.T) {
 	h.mustSee("added #1 to inbox", "inbox 1")
 
 	h.keys("i")
-	h.mustSee("INBOX · 1 of 1", "Reply to Ana", "@ana · #photos", "[1] P1 · someone is blocked")
-	h.keys("1", "s") // D11a: priority + size → leaves the inbox
+	h.mustSee("INBOX · 1 of 1", "Reply to Ana", "@ana · #photos", "[1] a person · me, one reviewer, one asker")
+	h.keys("1", "s") // D11a: impact + size → leaves the inbox
 	h.mustSee("Inbox zero ✓")
 	h.keys("esc")
-	h.mustSee("P1 · someone is blocked", "[P1 S] Reply to Ana @ana")
-	if got := h.task(1); got.Priority != 1 || got.Size != domain.SizeS {
+	h.mustSee("BACKLOG · by urgency", "[ 17 S] Reply to Ana @ana") // impact 1, no due: 17 (D71)
+	if got := h.task(1); got.Impact != 1 || got.Size != domain.SizeS {
 		t.Errorf("task after triage: %+v", got)
 	}
 }
@@ -42,7 +42,7 @@ func TestQuickAddDirectlyToBacklogAndEscCancels(t *testing.T) {
 	h.keys("a")
 	h.typeText("Ready !2 ~m")
 	h.keys("enter")
-	h.mustSee("added #1 to backlog", "[P2 M] Ready")
+	h.mustSee("added #1 to backlog", "[ 33 M] Ready")
 
 	h.keys("a")
 	h.typeText("never mind")
@@ -68,9 +68,9 @@ func TestQuickAddDirectlyToBacklogAndEscCancels(t *testing.T) {
 	h.typeText("Long capture @ana !7")
 	h.keys("enter")
 	if h.m.overlay == nil {
-		t.Fatal("an out-of-range priority must keep the overlay open")
+		t.Fatal("an out-of-range impact must keep the overlay open")
 	}
-	h.mustSee("add › Long capture @ana !7", "✗ priority !7 out of range")
+	h.mustSee("add › Long capture @ana !7", "✗ impact !7 out of range")
 	h.keys("esc")
 	if snap, _ := h.svc.Snapshot(context.Background()); len(snap.Tasks()) != 1 {
 		t.Errorf("failed adds must not add: %d tasks", len(snap.Tasks()))
@@ -98,9 +98,9 @@ func TestQuickAddAutocomplete(t *testing.T) {
 
 func TestPickPinDoneUndo(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	a := h.add("Alpha !1 ~s")
+	a := h.add("Alpha !3 ~s")
 	h.add("Beta !2 ~m")
-	h.mustSee("[P1 S] Alpha", "[P2 M] Beta", "nothing pinned")
+	h.mustSee("[ 50 S] Alpha", "[ 33 M] Beta", "nothing pinned")
 
 	h.keys("p") // cursor starts on the first backlog task
 	h.mustSee(`pinned "Alpha"`)
@@ -124,7 +124,7 @@ func TestPickPinDoneUndo(t *testing.T) {
 
 func TestUndoThatConflictsSaysSo(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	a := h.add("Alpha !1 ~s")
+	a := h.add("Alpha !3 ~s")
 	h.keys("p", "tab", "x")
 	h.clock.now = h.clock.now.Add(time.Minute)
 	if _, err := h.svc.SetNote(context.Background(), a.ID, "changed elsewhere"); err != nil {
@@ -139,8 +139,8 @@ func TestUndoThatConflictsSaysSo(t *testing.T) {
 
 func TestDropWithReasonAndDeleteConfirm(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	a := h.add("Alpha !1 ~s")
-	b := h.add("Beta !1 ~s")
+	a := h.add("Alpha !3 ~s")
+	b := h.add("Beta !3 ~s")
 
 	h.keys("d")
 	h.typeText("handed to @marko")
@@ -167,7 +167,7 @@ func TestBacklogParentExpandsAndTodayShowsChildren(t *testing.T) {
 	cfg.RolloverAlert = 1
 	h := newLogHarness(t, cfg)
 	ctx := context.Background()
-	parent := h.add("Auth migration !1 ~xl")
+	parent := h.add("Auth migration !3 ~xl")
 	kid1 := h.add("Rollback plan ~s")
 	kid2 := h.add("Token spike ~m")
 	for _, k := range []int64{kid1.ID, kid2.ID} {
@@ -176,10 +176,10 @@ func TestBacklogParentExpandsAndTodayShowsChildren(t *testing.T) {
 		}
 	}
 	h.keys("R")
-	h.mustSee("▸ [P1 XL] Auth migration 0/2")
+	h.mustSee("▸ [ 50 XL] Auth migration 0/2")
 	h.mustNotSee("Rollback plan")
 	h.keys("right")
-	h.mustSee("▾ [P1 XL] Auth migration", "    [P1 S] Rollback plan", "    [P1 M] Token spike")
+	h.mustSee("▾ [ 50 XL] Auth migration", "    [ 50 S] Rollback plan", "    [ 50 M] Token spike")
 	h.keys("down", "left") // collapsing from a child collapses its parent
 	h.mustNotSee("Rollback plan")
 
@@ -192,7 +192,7 @@ func TestBacklogParentExpandsAndTodayShowsChildren(t *testing.T) {
 	h.clock.now = h.clock.now.Add(24 * time.Hour) // parent rolls over once → alert at threshold 1
 	h.send(clockTickMsg{})
 	h.keys("esc") // dismiss the banner
-	h.mustSee("▾ [P1 XL] Auth migration 1/2 ↻1", "⚠ rolled over 1×", "    [P1 M] Token spike")
+	h.mustSee("▾ [ 50 XL] Auth migration 1/2 ↻1", "⚠ rolled over 1×", "    [ 50 M] Token spike")
 	h.mustNotSee("Rollback plan") // closed yesterday, not today
 }
 
@@ -214,7 +214,7 @@ func TestExternalWriteReloads(t *testing.T) {
 
 func TestNewDayBannerStartsPlanning(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	a := h.add("Blocked !1 ~s")
+	a := h.add("Blocked !3 ~s")
 	if _, _, err := h.svc.Block(context.Background(), a.ID, 10*time.Hour, time.Hour); err != nil {
 		t.Fatal(err)
 	}
@@ -233,26 +233,26 @@ func TestNewDayBannerStartsPlanning(t *testing.T) {
 	h.mustSee("tt · Inbox", "INBOX · 1 of 1") // D12d: planning starts with triage
 }
 
-func TestInboxSkipAndChildPriority(t *testing.T) {
+func TestInboxSkipAndChildImpact(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	parent := h.add("Parent !1 ~xl")
+	parent := h.add("Parent !3 ~xl")
 	child := h.add("Child")
 	h.add("Other")
 	if _, err := h.svc.SetParent(context.Background(), child.ID, parent.ID); err != nil {
 		t.Fatal(err)
 	}
 	h.keys("R", "i")
-	h.mustSee("INBOX · 1 of 2", "Child", "from parent: [P1 XL]")
+	h.mustSee("INBOX · 1 of 2", "Child", "from parent #1: more than one team")
 	h.keys("2")
-	h.mustSee(`priority comes from parent #1 "Parent"`)
+	h.mustSee("children inherit impact from #1")
 	h.keys("n")
 	h.mustSee("INBOX · 1 of 2", "Other") // the skipped card moved to the end
 	h.keys("n")
 	h.mustSee("Child") // everything skipped: start over
-	h.keys("m")        // size alone triages a child of a prioritized parent
+	h.keys("m")        // size alone triages a child (D74)
 	h.mustSee("INBOX · 1 of 1", "Other")
 	h.keys("9")
-	h.mustSee("there are 3 priority levels")
+	h.mustSee("impact is 1 to 3")
 }
 
 func TestHelpAndQuit(t *testing.T) {
@@ -269,7 +269,7 @@ func TestHelpAndQuit(t *testing.T) {
 
 func TestNarrowTerminalTruncatesInsteadOfWrapping(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	h.add("A rather long task title that will not fit in a narrow terminal @someone !1 ~s")
+	h.add("A rather long task title that will not fit in a narrow terminal @someone !3 ~s")
 	h.keys("p")
 	h.send(tea.WindowSizeMsg{Width: 40, Height: 12})
 	for _, line := range strings.Split(h.m.render(), "\n") {
@@ -293,7 +293,7 @@ func TestPasteIntoQuickAdd(t *testing.T) {
 func TestCursorFollowsTaskAcrossReload(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	h.add("Low !2 ~s")
-	if _, _, err := h.svc.Add(context.Background(), "High !1 ~s"); err != nil {
+	if _, _, err := h.svc.Add(context.Background(), "High !3 ~s"); err != nil {
 		t.Fatal(err)
 	}
 	h.keys("R") // High now sorts above Low; the cursor must stay on Low
@@ -304,10 +304,10 @@ func TestCursorFollowsTaskAcrossReload(t *testing.T) {
 func TestCollapseFromChildSelectsParent(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
-	parent := h.add("Auth migration !1 ~xl")
+	parent := h.add("Auth migration !3 ~xl")
 	kid1 := h.add("Rollback plan ~s")
 	kid2 := h.add("Token spike ~m")
-	h.add("Later !1 ~s")
+	h.add("Later !3 ~s")
 	for _, k := range []int64{kid1.ID, kid2.ID} {
 		if _, err := h.svc.SetParent(ctx, k, parent.ID); err != nil {
 			t.Fatal(err)
@@ -321,7 +321,7 @@ func TestHelpFitsWithFullBacklog(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	h.send(tea.WindowSizeMsg{Width: 120, Height: 20})
 	for n := 1; n <= 30; n++ {
-		h.add(fmt.Sprintf("T%d !1 ~s", n))
+		h.add(fmt.Sprintf("T%d !3 ~s", n))
 	}
 	h.keys("?")
 	if lines := strings.Count(h.m.render(), "\n") + 1; lines > 20 {
@@ -350,7 +350,7 @@ func TestErrorBeforeFirstLoadIsShown(t *testing.T) {
 func TestChildProgressExcludesDropped(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
-	parent := h.add("Parent !1 ~xl")
+	parent := h.add("Parent !3 ~xl")
 	done := h.add("Finished ~s")
 	dropped := h.add("Abandoned ~s")
 	open := h.add("Pending ~s")
@@ -366,13 +366,13 @@ func TestChildProgressExcludesDropped(t *testing.T) {
 		t.Fatal(err)
 	}
 	h.keys("R")
-	h.mustSee("▸ [P1 XL] Parent 1/2")
+	h.mustSee("▸ [ 50 XL] Parent 1/2")
 }
 
 func TestOrphanedChildrenStayInBacklog(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
-	parent := h.add("P !1 ~xl")
+	parent := h.add("P !3 ~xl")
 	kid := h.add("Kid ~s")
 	if _, err := h.svc.SetParent(ctx, kid.ID, parent.ID); err != nil {
 		t.Fatal(err)
@@ -381,10 +381,10 @@ func TestOrphanedChildrenStayInBacklog(t *testing.T) {
 		t.Fatal(err)
 	}
 	h.keys("R")
-	h.mustSee("[P1 S] Kid", "↳ #1")
+	h.mustSee("[ 50 S] Kid", "↳ #1")
 
 	// A parent still in the Inbox (no size) doesn't hide its children either.
-	p2 := h.add("P2 !1")
+	p2 := h.add("P2 !3")
 	kid2 := h.add("Kid2 ~s")
 	if _, err := h.svc.SetParent(ctx, kid2.ID, p2.ID); err != nil {
 		t.Fatal(err)
@@ -396,7 +396,7 @@ func TestOrphanedChildrenStayInBacklog(t *testing.T) {
 func TestChildOfPinnedParentNotListedInBacklog(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
-	parent := h.add("Pinned parent !1 ~xl")
+	parent := h.add("Pinned parent !3 ~xl")
 	kid := h.add("Under pin ~s")
 	if _, err := h.svc.SetParent(ctx, kid.ID, parent.ID); err != nil {
 		t.Fatal(err)
@@ -420,7 +420,7 @@ func TestNewDayShownWithoutWaitingForTheClockTick(t *testing.T) {
 
 func TestKeyOnStaleDayRefreshesInstead(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	a := h.add("Alpha !1 ~s")
+	a := h.add("Alpha !3 ~s")
 	h.keys("p")
 	h.clock.now = h.clock.now.Add(24 * time.Hour)
 	h.keys("u")
@@ -432,7 +432,7 @@ func TestKeyOnStaleDayRefreshesInstead(t *testing.T) {
 
 func TestDeleteConfirmKeepsHintOnNarrowScreens(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	h.add("An extremely long task title that goes on and on and on forever !1 ~s")
+	h.add("An extremely long task title that goes on and on and on forever !3 ~s")
 	h.send(tea.WindowSizeMsg{Width: 50, Height: 20})
 	h.keys("D")
 	h.mustSee("(y/n)")
@@ -449,7 +449,7 @@ func TestDeleteConfirmKeepsHintOnNarrowScreens(t *testing.T) {
 
 func TestRolloverMarkerShownInBacklog(t *testing.T) {
 	h := newLogHarness(t, config.Default())
-	a := h.add("Slipper !1 ~s")
+	a := h.add("Slipper !3 ~s")
 	h.keys("p")
 	if _, _, err := h.svc.Block(context.Background(), a.ID, 10*time.Hour, time.Hour); err != nil {
 		t.Fatal(err)
@@ -457,7 +457,7 @@ func TestRolloverMarkerShownInBacklog(t *testing.T) {
 	h.clock.now = h.clock.now.Add(24 * time.Hour)
 	h.send(clockTickMsg{})
 	h.keys("esc", "tab", "u")
-	h.mustSee("[P1 S] Slipper", "↻1")
+	h.mustSee("[ 50 S] Slipper", "↻1")
 	if _, ok := h.m.idx.cand[a.ID]; ok {
 		t.Fatal("Slipper should be unpinned")
 	}
@@ -468,7 +468,7 @@ func TestPinnedChildGetsAlertLine(t *testing.T) {
 	cfg.RolloverAlert = 1
 	h := newLogHarness(t, cfg)
 	ctx := context.Background()
-	parent := h.add("Par !1 ~xl")
+	parent := h.add("Par !3 ~xl")
 	kid := h.add("Kid ~s")
 	if _, err := h.svc.SetParent(ctx, kid.ID, parent.ID); err != nil {
 		t.Fatal(err)
@@ -490,7 +490,7 @@ func TestPinnedChildGetsAlertLine(t *testing.T) {
 func TestPinningClosedTaskExplainsItself(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
-	parent := h.add("Par !1 ~xl")
+	parent := h.add("Par !3 ~xl")
 	kid := h.add("Kid ~s")
 	if _, err := h.svc.SetParent(ctx, kid.ID, parent.ID); err != nil {
 		t.Fatal(err)
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./...`
Expected: FAIL (build): `internal/dsl/add_test.go: unknown field Impact in struct literal of type Draft` and `cannot use wednesday (constant "2026-10-14" of string type domain.Day) as int value in argument to ParseAdd`, and the like in every package.

- [ ] **Step 3: Implement**

Patch `README.md` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/README.md b/README.md
index d306661..826eb12 100644
--- a/README.md
+++ b/README.md
@@ -19,10 +19,13 @@ Make sure `~/.local/bin` is on your `PATH`.
 ## Use
 
 ```sh
-tt                                # open the planner (press ? for keys)
-tt add 'Reply to Ana @ana !1 ~s'  # capture a task
+tt                                    # open the planner (press ? for keys)
+tt add 'Reply to Ana @ana !2 ~s ^fri' # capture a task
 ```
 
+In a task line, `!1`–`!3` is impact (who notices if it slips: a person, one
+team, more than one team), `^fri` the due date, `~s` the size.
+
 ## Config
 
 Edit `~/.config/tt/config.toml`. Every option is listed there with its default.
```

Patch `cmd/tt/main.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/cmd/tt/main.go b/cmd/tt/main.go
index 18caff0..63c9215 100644
--- a/cmd/tt/main.go
+++ b/cmd/tt/main.go
@@ -25,7 +25,7 @@ import (
 
 const usage = `usage:
   tt               open the planner (TUI)
-  tt add <line>    capture a task, e.g. tt add 'Reply to Ana @ana #photos !1 ~s -- before Friday'
+  tt add <line>    capture a task, e.g. tt add 'Reply to Ana @ana #photos !1 ^fri ~s -- before Friday'
                    quote the line with single quotes: ! and ~ are special in zsh/bash
   tt add           no line: type it at the add › prompt (no quoting needed), or pipe it in;
                    piped lines after the first become the note, e.g. pbpaste | tt add
```

Patch `config.example.toml` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/config.example.toml b/config.example.toml
index 186d308..dd867e2 100644
--- a/config.example.toml
+++ b/config.example.toml
@@ -7,22 +7,9 @@
 # previous day.
 day_start = "04:00"
 
-# Priority levels, most urgent first (1 to 9 of them). Quick-add's !1, !2, …
-# pick them by position.
-[[priorities]]
-name = "P1"
-definition = "someone is blocked / committed this week"
-
-[[priorities]]
-name = "P2"
-definition = "important this month"
-
-[[priorities]]
-name = "P3"
-definition = "worth doing eventually"
-
-# Urgency (D70), the number tasks are sorted by: impact and the due date,
-# weighted. Raise due if deadlines should count more, impact if what matters
+# Urgency (D70), the number tasks are sorted by: impact (!1 to !3, who
+# notices if it slips: a person, one team, more than one team) and the due
+# date (^fri), weighted. Raise due if deadlines should count more, impact if what matters
 # should; each 0 or more, not both 0.
 [urgency]
 impact = 1
@@ -57,7 +44,6 @@ poll = "2m" # how often the calendar is fetched; 10s or more
 enabled = true  # false: no fetch, no Reviews lane
 host = ""       # a GitHub Enterprise host; empty: gh's default (github.com)
 poll = "5m"     # how often GitHub is fetched; 30s or more
-priority = "P2" # a pinned review's priority: one of the [[priorities]] names
 size = "s"      # a pinned review's size: s, m, l, xl, or "" for unsized (D48)
 impact = 1      # a pinned review's impact, 1 to 3 (D75)
 due = "3d"      # a pinned review is due this many days after the request; "" = no due
```

Patch `internal/app/children.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/children.go b/internal/app/children.go
index 121868c..40436a4 100644
--- a/internal/app/children.go
+++ b/internal/app/children.go
@@ -15,20 +15,20 @@ import (
 var ErrNoMove = errors.New("no sibling in that direction")
 
 // ParseChild parses an add-child line for parentID. Children inherit their
-// parent's priority (D6), so !n is refused (D32).
+// parent's impact (D68), so !n is refused (D32, D73); ^date is their own.
 func (s *Service) ParseChild(parentID int64, line string) (dsl.Draft, error) {
-	d, err := dsl.ParseAdd(line, len(s.cfg.Priorities))
+	d, err := dsl.ParseAdd(line, s.Today())
 	if err != nil {
 		return dsl.Draft{}, err
 	}
-	if d.Priority != 0 {
-		return dsl.Draft{}, inheritedPriority(parentID, d.Priority)
+	if d.Impact != 0 {
+		return dsl.Draft{}, inheritedImpact(parentID, d.Impact)
 	}
 	return d, nil
 }
 
-func inheritedPriority(parentID int64, p int) error {
-	return fmt.Errorf("%w: children inherit priority from #%d; drop !%d", ErrInvalidValue, parentID, p)
+func inheritedImpact(parentID int64, n int) error {
+	return fmt.Errorf("%w: children inherit impact from #%d; drop !%d", ErrInvalidValue, parentID, n)
 }
 
 // AddChild captures a task from line as the last child of parentID. Under a
@@ -41,7 +41,7 @@ func (s *Service) AddChild(ctx context.Context, parentID int64, line string) (do
 	}
 	now := s.clock.Now()
 	t := domain.Task{
-		Title: d.Title, Note: d.Note, Link: d.Link, Requester: d.Requester, Tags: d.Tags, Size: d.Size,
+		Title: d.Title, Note: d.Note, Link: d.Link, Requester: d.Requester, Tags: d.Tags, Due: d.Due, Size: d.Size,
 		ParentID: parentID, Status: domain.StatusOpen, CreatedAt: now, UpdatedAt: now,
 	}
 	err = s.store.InTx(ctx, func(x *store.Tx) error {
```

Patch `internal/app/day.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/day.go b/internal/app/day.go
index 20a170d..6ac2653 100644
--- a/internal/app/day.go
+++ b/internal/app/day.go
@@ -3,7 +3,6 @@ package app
 import (
 	"cmp"
 	"context"
-	"math"
 	"slices"
 
 	"tt/internal/domain"
@@ -148,18 +147,11 @@ func (s *Service) buildPlan(today domain.Day, snap domain.Snapshot, state store.
 			Alert:      t.Status == domain.StatusOpen && t.RolloverCount >= s.cfg.RolloverAlert,
 		})
 	}
+	// Most urgent first, untriaged last, then by id (D72).
 	slices.SortFunc(plan.Candidates, func(a, b domain.Candidate) int {
-		pa := domain.EffectivePriority(a.Task, snap.ParentOf(a.Task))
-		pb := domain.EffectivePriority(b.Task, snap.ParentOf(b.Task))
-		return cmp.Or(cmp.Compare(unsetLast(pa), unsetLast(pb)), cmp.Compare(a.Task.ID, b.Task.ID))
+		ua, oka := domain.Urgency(a.Task, snap.ParentOf(a.Task), today, s.cfg.Urgency)
+		ub, okb := domain.Urgency(b.Task, snap.ParentOf(b.Task), today, s.cfg.Urgency)
+		return cmp.Or(domain.CompareUrgency(ua, oka, ub, okb), cmp.Compare(a.Task.ID, b.Task.ID))
 	})
 	return plan
 }
-
-// unsetLast sorts priority 0 (unset) after every real level.
-func unsetLast(p int) int {
-	if p == 0 {
-		return math.MaxInt
-	}
-	return p
-}
```

Patch `internal/app/sources.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/sources.go b/internal/app/sources.go
index e14ceb7..6183e8b 100644
--- a/internal/app/sources.go
+++ b/internal/app/sources.go
@@ -117,8 +117,9 @@ func (s *Service) SyncSources(ctx context.Context) ([]domain.Task, error) {
 }
 
 // PinSource pins the review request kind/extID to today (D20, D49). It
-// creates the request's task (D20b, D48: "Review #n: <title>", the PR link,
-// the configured priority and size, no requester) or, when that task exists
+// creates the request's task (D20b, D48, D75: "Review #n: <title>", the PR
+// link, the configured size and impact, due the configured days after the
+// request, no requester) or, when that task exists
 // but is closed, reopens it, keeping its history. The undo deletes the task
 // it created, or closes the reopened one again as it was; it refuses
 // (ErrConflict) once the task changed, was blocked today, or the day moved on.
@@ -145,8 +146,11 @@ func (s *Service) PinSource(ctx context.Context, kind, extID string) (domain.Tas
 		case !ok:
 			ref := item.Ref()
 			after = domain.Task{Title: domain.ReviewTitle(item), Link: item.URL, Source: &ref,
-				Priority: min(s.cfg.ReviewPriority, len(s.cfg.Priorities)), Size: s.cfg.ReviewSize,
+				Impact: s.cfg.ReviewImpact, Size: s.cfg.ReviewSize,
 				Status: domain.StatusOpen, CreatedAt: now, UpdatedAt: now}
+			if s.cfg.ReviewDue >= 0 {
+				after.Due = s.DayOf(item.WaitingSince()).AddDays(s.cfg.ReviewDue)
+			}
 			if after.ID, err = x.InsertTask(ctx, after); err != nil {
 				return err
 			}
```

Patch `internal/app/tasks.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/tasks.go b/internal/app/tasks.go
index 69b8a48..2417e05 100644
--- a/internal/app/tasks.go
+++ b/internal/app/tasks.go
@@ -21,14 +21,14 @@ var (
 
 // Add captures a task from a quick-add line (D25).
 func (s *Service) Add(ctx context.Context, line string) (domain.Task, Undo, error) {
-	d, err := dsl.ParseAdd(line, len(s.cfg.Priorities))
+	d, err := dsl.ParseAdd(line, s.Today())
 	if err != nil {
 		return domain.Task{}, nil, err
 	}
 	now := s.clock.Now()
 	t := domain.Task{
 		Title: d.Title, Note: d.Note, Link: d.Link, Requester: d.Requester, Tags: d.Tags,
-		Priority: d.Priority, Size: d.Size, Status: domain.StatusOpen, CreatedAt: now, UpdatedAt: now,
+		Impact: d.Impact, Due: d.Due, Size: d.Size, Status: domain.StatusOpen, CreatedAt: now, UpdatedAt: now,
 	}
 	err = s.store.InTx(ctx, func(x *store.Tx) (err error) {
 		t.ID, err = x.InsertTask(ctx, t)
@@ -51,34 +51,35 @@ func (s *Service) Add(ctx context.Context, line string) (domain.Task, Undo, erro
 
 // EditLine renders a task as a token line for prefilling the edit input.
 // The note is left out; Edit keeps it unless the new line has " -- ".
-// A child's priority is inherited (D6), so it is left out too (D32).
+// A child's impact is its parent's (D68), so it is left out (D32, D73); the
+// due is the task's own, as an absolute date, never an inherited one.
 func EditLine(t domain.Task) string {
-	d := dsl.Draft{Title: t.Title, Requester: t.Requester, Tags: t.Tags, Priority: t.Priority, Size: t.Size, Link: t.Link}
+	d := dsl.Draft{Title: t.Title, Requester: t.Requester, Tags: t.Tags, Impact: t.Impact, Due: t.Due, Size: t.Size, Link: t.Link}
 	if t.ParentID != 0 {
-		d.Priority = 0
+		d.Impact = 0
 	}
 	return dsl.FormatAdd(d)
 }
 
-// ParseEdit parses an edit line for t: !n is refused on a child (D32).
+// ParseEdit parses an edit line for t: !n is refused on a child (D32, D73).
 // The TUI calls it to validate before submitting; Edit applies the same rule.
 func (s *Service) ParseEdit(t domain.Task, line string) (dsl.Draft, error) {
 	if t.ParentID != 0 {
 		return s.ParseChild(t.ParentID, line)
 	}
-	return dsl.ParseAdd(line, len(s.cfg.Priorities))
+	return dsl.ParseAdd(line, s.Today())
 }
 
-// Edit replaces title, requester, tags, priority, size and link from line;
-// a token missing from line clears that field. The note changes only if
-// line contains " -- ".
+// Edit replaces title, requester, tags, impact, due, size and link from
+// line; a token missing from line clears that field. The note changes only
+// if line contains " -- ".
 func (s *Service) Edit(ctx context.Context, id int64, line string) (Undo, error) {
 	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
 		d, err := s.ParseEdit(*t, line)
 		if err != nil {
 			return err
 		}
-		t.Title, t.Requester, t.Tags, t.Priority, t.Size, t.Link = d.Title, d.Requester, d.Tags, d.Priority, d.Size, d.Link
+		t.Title, t.Requester, t.Tags, t.Impact, t.Due, t.Size, t.Link = d.Title, d.Requester, d.Tags, d.Impact, d.Due, d.Size, d.Link
 		if d.HasNote {
 			t.Note = d.Note
 		}
@@ -106,13 +107,30 @@ func (s *Service) SetNoteFrom(ctx context.Context, id int64, from, note string)
 	})
 }
 
-// SetPriority sets priority p (1..levels); 0 clears it.
-func (s *Service) SetPriority(ctx context.Context, id int64, p int) (Undo, error) {
-	if p < 0 || p > len(s.cfg.Priorities) {
-		return nil, fmt.Errorf("priority %d: %w (use 1 to %d, or 0 to clear)", p, ErrInvalidValue, len(s.cfg.Priorities))
+// SetImpact sets impact n (1 to 3, D68); 0 clears it. A child's impact is
+// its parent's, so setting one is refused (D73).
+func (s *Service) SetImpact(ctx context.Context, id int64, n int) (Undo, error) {
+	if n < 0 || n > domain.MaxImpact {
+		return nil, fmt.Errorf("impact %d: %w (use 1 to %d, or 0 to clear)", n, ErrInvalidValue, domain.MaxImpact)
 	}
 	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
-		t.Priority = p
+		if t.ParentID != 0 && n != 0 {
+			return inheritedImpact(t.ParentID, n)
+		}
+		t.Impact = n
+		return nil
+	})
+}
+
+// SetDue sets the due date (D69); "" clears it.
+func (s *Service) SetDue(ctx context.Context, id int64, due domain.Day) (Undo, error) {
+	if due != "" {
+		if _, err := domain.ParseDay(string(due)); err != nil {
+			return nil, fmt.Errorf("%w: %w", ErrInvalidValue, err)
+		}
+	}
+	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
+		t.Due = due
 		return nil
 	})
 }
```

Patch `internal/config/config.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/config/config.go b/internal/config/config.go
index afaf9e3..7096be4 100644
--- a/internal/config/config.go
+++ b/internal/config/config.go
@@ -16,15 +16,9 @@ import (
 	"tt/internal/domain"
 )
 
-type PriorityLevel struct {
-	Name       string `toml:"name"`
-	Definition string `toml:"definition"`
-}
-
 type Config struct {
-	DayStart   time.Duration // offset from midnight when a new planning day begins (D19)
-	Priorities []PriorityLevel
-	Sizes      map[domain.Size]time.Duration
+	DayStart time.Duration // offset from midnight when a new planning day begins (D19)
+	Sizes    map[domain.Size]time.Duration
 	// UnsizedBlock is the default block length for unsized tasks such as PR reviews (D24).
 	UnsizedBlock time.Duration
 	// RolloverAlert is the rollover count at which planning flags a task (D10a).
@@ -43,12 +37,10 @@ type Config struct {
 	GitHubHost string
 	// GitHubPoll is how often GitHub is fetched (D3).
 	GitHubPoll time.Duration
-	// ReviewPriority (a level index) and ReviewSize are what the task of a
-	// pinned review request starts with (D48).
-	ReviewPriority int
-	ReviewSize     domain.Size
-	// ReviewImpact and ReviewDue are the impact and the days after the
-	// request a pinned review's task is due (D75); ReviewDue -1 is no due.
+	// ReviewSize, ReviewImpact and ReviewDue are what the task of a pinned
+	// review request starts with (D48, D75): its size, its impact, and how
+	// many days after the request it is due; ReviewDue -1 is no due.
+	ReviewSize   domain.Size
 	ReviewImpact int
 	ReviewDue    int
 	// Urgency weighs impact against the due date (D70).
@@ -73,39 +65,32 @@ func (n Notify) On() bool { return n.Meetings || n.Blocks || n.Reviews }
 func Default() Config {
 	return Config{
 		DayStart: 4 * time.Hour,
-		Priorities: []PriorityLevel{
-			{Name: "P1", Definition: "someone is blocked / committed this week"},
-			{Name: "P2", Definition: "important this month"},
-			{Name: "P3", Definition: "worth doing eventually"},
-		},
 		Sizes: map[domain.Size]time.Duration{
 			domain.SizeS:  time.Hour,
 			domain.SizeM:  4 * time.Hour,
 			domain.SizeL:  8 * time.Hour,
 			domain.SizeXL: 40 * time.Hour,
 		},
-		UnsizedBlock:   30 * time.Minute,
-		RolloverAlert:  3,
-		WorkStart:      8 * time.Hour,
-		WorkEnd:        19 * time.Hour,
-		CalendarPoll:   2 * time.Minute,
-		GitHub:         true,
-		GitHubPoll:     5 * time.Minute,
-		ReviewPriority: 2,
-		ReviewSize:     domain.SizeS,
-		ReviewImpact:   1,
-		ReviewDue:      3,
-		Urgency:        domain.DefaultWeights,
-		Notify:         Notify{Alerts: []int{5, 0}, Sound: "Glass"},
+		UnsizedBlock:  30 * time.Minute,
+		RolloverAlert: 3,
+		WorkStart:     8 * time.Hour,
+		WorkEnd:       19 * time.Hour,
+		CalendarPoll:  2 * time.Minute,
+		GitHub:        true,
+		GitHubPoll:    5 * time.Minute,
+		ReviewSize:    domain.SizeS,
+		ReviewImpact:  1,
+		ReviewDue:     3,
+		Urgency:       domain.DefaultWeights,
+		Notify:        Notify{Alerts: []int{5, 0}, Sound: "Glass"},
 	}
 }
 
 // file mirrors the TOML layout; every field is optional.
 type file struct {
-	DayStart   *string           `toml:"day_start"`
-	Priorities []PriorityLevel   `toml:"priorities"`
-	Sizes      map[string]string `toml:"sizes"`
-	Planning   struct {
+	DayStart *string           `toml:"day_start"`
+	Sizes    map[string]string `toml:"sizes"`
+	Planning struct {
 		UnsizedBlock  *string `toml:"unsized_block"`
 		RolloverAlert *int    `toml:"rollover_alert"`
 	} `toml:"planning"`
@@ -118,13 +103,12 @@ type file struct {
 		Poll      *string  `toml:"poll"`
 	} `toml:"calendar"`
 	GitHub struct {
-		Enabled  *bool   `toml:"enabled"`
-		Host     *string `toml:"host"`
-		Poll     *string `toml:"poll"`
-		Priority *string `toml:"priority"`
-		Size     *string `toml:"size"`
-		Impact   *int    `toml:"impact"`
-		Due      *string `toml:"due"`
+		Enabled *bool   `toml:"enabled"`
+		Host    *string `toml:"host"`
+		Poll    *string `toml:"poll"`
+		Size    *string `toml:"size"`
+		Impact  *int    `toml:"impact"`
+		Due     *string `toml:"due"`
 	} `toml:"github"`
 	Urgency struct {
 		Impact *float64 `toml:"impact"`
@@ -163,7 +147,7 @@ func Parse(data []byte) (Config, error) {
 		return Config{}, err
 	}
 	if und := md.Undecoded(); len(und) > 0 {
-		return Config{}, fmt.Errorf("unknown config key %q", und[0].String())
+		return Config{}, unknownKey(und[0].String())
 	}
 
 	cfg := Default()
@@ -172,17 +156,6 @@ func Parse(data []byte) (Config, error) {
 			return Config{}, fmt.Errorf("day_start: %w", err)
 		}
 	}
-	if f.Priorities != nil {
-		if n := len(f.Priorities); n < 1 || n > 9 {
-			return Config{}, fmt.Errorf("priorities: need 1 to 9 levels, got %d", n)
-		}
-		for i, p := range f.Priorities {
-			if strings.TrimSpace(p.Name) == "" {
-				return Config{}, fmt.Errorf("priorities[%d]: name is required", i)
-			}
-		}
-		cfg.Priorities = f.Priorities
-	}
 	for key, val := range f.Sizes {
 		size, ok := domain.ParseSize(key)
 		if !ok || string(size) != strings.ToLower(key) {
@@ -252,9 +225,19 @@ func Parse(data []byte) (Config, error) {
 	return cfg, nil
 }
 
-// parseGitHub applies [github] (D48, D50). Without a priority the reviews
-// get the level named P2, else the last level, so configuring other
-// priorities never fails over a key that was never set.
+// unknownKey is the error for a key tt doesn't know (typos surface). The
+// keys priority had before D68 say what replaced them (D76).
+func unknownKey(key string) error {
+	switch {
+	case key == "priorities" || strings.HasPrefix(key, "priorities."):
+		return fmt.Errorf("[[priorities]] was replaced by impact (D68): remove it, see config.example.toml")
+	case key == "github.priority":
+		return fmt.Errorf("[github] priority was replaced by impact and due (D75): remove it, see config.example.toml")
+	}
+	return fmt.Errorf("unknown config key %q", key)
+}
+
+// parseGitHub applies [github] (D48, D50, D75).
 func parseGitHub(f file, cfg *Config) error {
 	g := f.GitHub
 	if g.Enabled != nil {
@@ -270,17 +253,6 @@ func parseGitHub(f file, cfg *Config) error {
 		}
 		cfg.GitHubPoll = d
 	}
-	name := "P2"
-	if g.Priority != nil {
-		name = strings.TrimSpace(*g.Priority)
-	}
-	cfg.ReviewPriority = slices.IndexFunc(cfg.Priorities, func(l PriorityLevel) bool { return strings.EqualFold(l.Name, name) }) + 1
-	if cfg.ReviewPriority == 0 {
-		if g.Priority != nil {
-			return fmt.Errorf("github.priority: no priority level named %q", name)
-		}
-		cfg.ReviewPriority = len(cfg.Priorities)
-	}
 	if g.Size != nil {
 		p := strings.TrimSpace(*g.Size)
 		size, ok := domain.ParseSize(p)
```

Patch `internal/domain/rules.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/domain/rules.go b/internal/domain/rules.go
index 4e4c9ef..c37161b 100644
--- a/internal/domain/rules.go
+++ b/internal/domain/rules.go
@@ -2,15 +2,6 @@ package domain
 
 import "slices"
 
-// EffectivePriority returns the priority that applies to t. Children always
-// inherit their parent's priority; their own value is ignored (no override in v1).
-func EffectivePriority(t Task, parent *Task) int {
-	if parent != nil {
-		return parent.Priority
-	}
-	return t.Priority
-}
-
 // EffectiveTags returns t's own tags merged with its parent's.
 func EffectiveTags(t Task, parent *Task) []string {
 	if parent == nil {
@@ -19,11 +10,16 @@ func EffectiveTags(t Task, parent *Task) []string {
 	return NormalizeTags(append(slices.Clone(t.Tags), parent.Tags...))
 }
 
-// InInbox reports whether t still needs triage (D11): it is open, not
-// source-linked, and lacks an effective priority or a size.
+// InInbox reports whether t still needs triage (D11, D74): it is open and
+// not source-linked, and it is top-level without an impact or a size, or a
+// child without its own size. A child takes its parent's impact, so an
+// untriaged parent is triaged in the Inbox for all of its children.
 func InInbox(t Task, parent *Task) bool {
 	if t.Status != StatusOpen || t.Source != nil {
 		return false
 	}
-	return EffectivePriority(t, parent) == 0 || t.Size == SizeNone
+	if t.ParentID != 0 {
+		return t.Size == SizeNone
+	}
+	return t.Impact == 0 || t.Size == SizeNone
 }
```

Patch `internal/domain/task.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/domain/task.go b/internal/domain/task.go
index 78870a1..ffbea0c 100644
--- a/internal/domain/task.go
+++ b/internal/domain/task.go
@@ -61,7 +61,6 @@ type Task struct {
 	Link          string
 	Requester     string   // lowercase; "" = none
 	Tags          []string // normalized: lowercase, unique, sorted
-	Priority      int      // 0 = unset; 1 = highest
 	Impact        int      // 0 = unset; 1–3, who notices if it slips (D68)
 	Due           Day      // "" = none (D69)
 	Size          Size
```

Patch `internal/dsl/add.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/dsl/add.go b/internal/dsl/add.go
index 6890c57..72377b0 100644
--- a/internal/dsl/add.go
+++ b/internal/dsl/add.go
@@ -18,7 +18,8 @@ type Draft struct {
 	Title     string
 	Requester string // lowercase
 	Tags      []string
-	Priority  int // 0 = not given
+	Impact    int        // 0 = not given; 1–3 (D68)
+	Due       domain.Day // "" = not given (D69)
 	Size      domain.Size
 	Link      string
 	Note      string
@@ -30,16 +31,18 @@ const noteSep = " -- "
 // trailingPunct is stripped from the end of sigil tokens ("@ana," is @ana).
 const trailingPunct = ".,;:!?)]}'\""
 
-// ParseAdd parses a quick-add line. levels is the number of configured
-// priority levels; !n outside 1..levels is an error.
+// ParseAdd parses a quick-add line; today (a planning day) resolves ^dates.
 //
 //	@name   requester (last one wins)
 //	#tag    tag, repeatable (#123 stays in the title: it's an issue ref)
-//	!n      priority
+//	!n      impact, 1 to 3 (D68); out of range is an error
+//	^date   due (D73): today, tom, fri, 3d, 2w, 30oct, 2026-10-30 (last one
+//	        wins; a date that doesn't exist is an error; ^anything-else
+//	        stays in the title)
 //	~size   s, m, l, xl or x (~anything-else stays in the title)
 //	URL     first http(s) URL becomes the link; later URLs stay in the title
 //	 -- …   everything after the first " -- " is the note, verbatim
-func ParseAdd(line string, levels int) (Draft, error) {
+func ParseAdd(line string, today domain.Day) (Draft, error) {
 	var d Draft
 	// Padding lets " -- " also match a leading "-- " and a trailing " --".
 	head := line
@@ -51,7 +54,7 @@ func ParseAdd(line string, levels int) (Draft, error) {
 	var title, tags []string
 	for _, tok := range strings.Fields(head) {
 		orig := tok
-		if strings.ContainsRune("@#!~", rune(tok[0])) {
+		if strings.ContainsRune("@#!~^", rune(tok[0])) {
 			stripped := strings.TrimRight(tok, trailingPunct)
 			// "#482." stays whole (issue ref); "@." has nothing left to classify.
 			if len(stripped) <= 1 || (stripped[0] == '#' && isDigits(stripped[1:])) {
@@ -66,11 +69,21 @@ func ParseAdd(line string, levels int) (Draft, error) {
 		case sigil == '#' && rest != "" && !isDigits(rest):
 			tags = append(tags, rest)
 		case sigil == '!' && isDigits(rest):
-			n, err := strconv.Atoi(rest)
-			if err != nil || n < 1 || n > levels {
-				return Draft{}, fmt.Errorf("priority %s out of range: use !1 to !%d", tok, levels)
+			n, err := parseImpact(tok)
+			if err != nil {
+				return Draft{}, err
 			}
-			d.Priority = n
+			d.Impact = n
+		case sigil == '^':
+			due, ok, err := dueWord(rest, today)
+			if err != nil {
+				return Draft{}, err
+			}
+			if !ok {
+				title = append(title, orig)
+				continue
+			}
+			d.Due = due
 		case sigil == '~' && isSize(rest):
 			d.Size, _ = domain.ParseSize(rest)
 		case d.Link == "" && isURL(tok):
@@ -103,8 +116,11 @@ func FormatAdd(d Draft) string {
 	for _, t := range d.Tags {
 		parts = append(parts, "#"+t)
 	}
-	if d.Priority > 0 {
-		parts = append(parts, "!"+strconv.Itoa(d.Priority))
+	if d.Impact > 0 {
+		parts = append(parts, "!"+strconv.Itoa(d.Impact))
+	}
+	if d.Due != "" {
+		parts = append(parts, "^"+string(d.Due)) // absolute: an edit doesn't drift (D73)
 	}
 	if d.Size != domain.SizeNone {
 		parts = append(parts, "~"+string(d.Size))
@@ -119,6 +135,15 @@ func FormatAdd(d Draft) string {
 	return s
 }
 
+// parseImpact reads !n (tok, all digits after the !) as impact 1–3 (D68).
+func parseImpact(tok string) (int, error) {
+	n, err := strconv.Atoi(tok[1:])
+	if err != nil || n < 1 || n > domain.MaxImpact {
+		return 0, fmt.Errorf("impact %s out of range: use !1 to !%d", tok, domain.MaxImpact)
+	}
+	return n, nil
+}
+
 func isDigits(s string) bool {
 	if s == "" {
 		return false
```

Patch `internal/dsl/filter.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/dsl/filter.go b/internal/dsl/filter.go
index ae6a468..1d63e46 100644
--- a/internal/dsl/filter.go
+++ b/internal/dsl/filter.go
@@ -14,7 +14,7 @@ import (
 type Filter struct {
 	Requesters []string        // any of; lowercase
 	Tags       []string        // all of; normalized
-	Priorities []int           // any of (effective priority)
+	Impacts    []int           // any of (effective impact)
 	Sizes      []domain.Size   // any of
 	NewerThan  time.Duration   // created less than this long ago; 0 = no bound
 	OlderThan  time.Duration   // created more than this long ago; 0 = no bound
@@ -24,23 +24,23 @@ type Filter struct {
 
 // Empty reports whether the line had no tokens at all.
 func (f Filter) Empty() bool {
-	return len(f.Requesters) == 0 && len(f.Tags) == 0 && len(f.Priorities) == 0 && len(f.Sizes) == 0 &&
+	return len(f.Requesters) == 0 && len(f.Tags) == 0 && len(f.Impacts) == 0 && len(f.Sizes) == 0 &&
 		f.NewerThan == 0 && f.OlderThan == 0 && len(f.Statuses) == 0 && len(f.Words) == 0
 }
 
 const day = 24 * time.Hour
 
-// ParseFilter parses a filter-bar line. levels is the number of configured
-// priority levels; !n outside 1..levels is an error, like in ParseAdd.
+// ParseFilter parses a filter-bar line. !n outside 1..3 is an error, like
+// in ParseAdd.
 //
 //	@name        requester (repeats: any of)
 //	#tag         tag, own or inherited (repeats: all of)
-//	!n  ~size    effective priority / size (repeats: any of)
+//	!n  ~size    effective impact / size (repeats: any of)
 //	<7d  >30d    created within / more than N days (or w weeks) ago
 //	is:open|done|dropped|all
 //	a lone sigil (@ # ! ~ < >) is ignored: it's a token still being typed
 //	anything else is a fuzzy word
-func ParseFilter(line string, levels int) (Filter, error) {
+func ParseFilter(line string) (Filter, error) {
 	var f Filter
 	var tags []string
 	for _, tok := range strings.Fields(line) {
@@ -62,11 +62,11 @@ func ParseFilter(line string, levels int) (Filter, error) {
 		case sigil == '#' && !isDigits(rest):
 			tags = append(tags, rest)
 		case sigil == '!' && isDigits(rest):
-			n, err := strconv.Atoi(rest)
-			if err != nil || n < 1 || n > levels {
-				return Filter{}, fmt.Errorf("priority %s out of range: use !1 to !%d", tok, levels)
+			n, err := parseImpact(tok)
+			if err != nil {
+				return Filter{}, err
 			}
-			f.Priorities = appendNew(f.Priorities, n)
+			f.Impacts = appendNew(f.Impacts, n)
 		case sigil == '~' && isSize(rest):
 			s, _ := domain.ParseSize(rest)
 			f.Sizes = appendNew(f.Sizes, s)
```

Patch `internal/search/search.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/search/search.go b/internal/search/search.go
index 361c45b..2a02df4 100644
--- a/internal/search/search.go
+++ b/internal/search/search.go
@@ -1,5 +1,5 @@
-// Package search evaluates filters (D26a–D26d) and groups tasks for the
-// backlog (D26) over an in-memory snapshot. It does no I/O.
+// Package search evaluates filters (D26a–D26d) and sorts or groups tasks
+// for the backlog (D26, D72) over an in-memory snapshot. It does no I/O.
 package search
 
 import (
@@ -14,7 +14,7 @@ import (
 )
 
 // Match reports whether t passes f at now. parent is t's parent (nil if none):
-// children are matched on their effective priority and tags.
+// children are matched on their effective impact and tags.
 func Match(f dsl.Filter, t domain.Task, parent *domain.Task, now time.Time) bool {
 	statuses := f.Statuses
 	if statuses == nil {
@@ -32,7 +32,7 @@ func Match(f dsl.Filter, t domain.Task, parent *domain.Task, now time.Time) bool
 			return false
 		}
 	}
-	if len(f.Priorities) > 0 && !slices.Contains(f.Priorities, domain.EffectivePriority(t, parent)) {
+	if len(f.Impacts) > 0 && !slices.Contains(f.Impacts, domain.EffectiveImpact(t, parent)) {
 		return false
 	}
 	if len(f.Sizes) > 0 && !slices.Contains(f.Sizes, t.Size) {
@@ -97,25 +97,52 @@ func anchoredSubsequence(w, word []rune) bool {
 	return i == len(w)
 }
 
-// GroupBy is a backlog grouping (D26).
+// GroupBy is a backlog grouping (D26) or, for urgency and impact, a flat
+// sort (D72).
 type GroupBy string
 
 const (
-	ByPriority  GroupBy = "priority"
+	ByUrgency   GroupBy = "urgency"
+	ByImpact    GroupBy = "impact"
 	BySize      GroupBy = "size"
 	ByCreated   GroupBy = "created"
 	ByRequester GroupBy = "requester"
 )
 
 // GroupBys lists the groupings in the order `g` cycles through them.
-var GroupBys = []GroupBy{ByPriority, BySize, ByCreated, ByRequester}
+var GroupBys = []GroupBy{ByUrgency, ByImpact, BySize, ByCreated, ByRequester}
 
-// ParseGroupBy returns the grouping named s; anything unknown is ByPriority.
+// ParseGroupBy returns the grouping named s; anything unknown (a remembered
+// "priority" among them) is ByUrgency.
 func ParseGroupBy(s string) GroupBy {
 	if g := GroupBy(s); slices.Contains(GroupBys, g) {
 		return g
 	}
-	return ByPriority
+	return ByUrgency
+}
+
+// Flat reports whether g is a sort with no headings (D72).
+func (g GroupBy) Flat() bool { return g == ByUrgency || g == ByImpact }
+
+// Sorter is the order of tasks within g's list (flat) or within one of its
+// groups, on today with weights w (D72): ByUrgency most urgent first,
+// ByImpact biggest effective impact first, then most urgent; an untriaged
+// task after the triaged ones; then oldest first. The groupings sort oldest
+// first (D26). parentOf finds a task's parent (nil for none).
+func Sorter(g GroupBy, parentOf func(domain.Task) *domain.Task, today domain.Day, w domain.Weights) func(a, b domain.Task) int {
+	if !g.Flat() {
+		return Oldest
+	}
+	return func(a, b domain.Task) int {
+		pa, pb := parentOf(a), parentOf(b)
+		ua, oka := domain.Urgency(a, pa, today, w)
+		ub, okb := domain.Urgency(b, pb, today, w)
+		byImpact := 0
+		if g == ByImpact {
+			byImpact = cmp.Compare(domain.EffectiveImpact(b, pb), domain.EffectiveImpact(a, pa))
+		}
+		return cmp.Or(byImpact, domain.CompareUrgency(ua, oka, ub, okb), Oldest(a, b))
+	}
 }
 
 // Next is the grouping after g in the `g` cycle.
@@ -150,9 +177,9 @@ var ageBuckets = []struct {
 	{90 * 24 * time.Hour, "older"},
 }
 
-// GroupOf returns the group t lands in. priorities holds one heading per
-// configured level (e.g. "P1 · someone is blocked").
-func GroupOf(by GroupBy, t domain.Task, parent *domain.Task, now time.Time, priorities []string) Group {
+// GroupOf returns the group t lands in at now. A flat sort (D72) has one
+// group, Group{}, shown without a heading.
+func GroupOf(by GroupBy, t domain.Task, now time.Time) Group {
 	switch by {
 	case BySize:
 		if i := slices.Index(domain.Sizes, t.Size); i >= 0 {
@@ -173,12 +200,5 @@ func GroupOf(by GroupBy, t domain.Task, parent *domain.Task, now time.Time, prio
 		}
 		return Group{Label: "@" + t.Requester}
 	}
-	switch p := domain.EffectivePriority(t, parent); {
-	case p == 0:
-		return Group{Rank: len(priorities) + 2, Label: "unset"}
-	case p > len(priorities) || p < 0:
-		return Group{Rank: len(priorities) + 1, Label: "P? · above the configured levels"}
-	default:
-		return Group{Rank: p, Label: priorities[p-1]}
-	}
+	return Group{}
 }
```

Patch `internal/store/tasks.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/store/tasks.go b/internal/store/tasks.go
index 9e40dc6..330002c 100644
--- a/internal/store/tasks.go
+++ b/internal/store/tasks.go
@@ -14,13 +14,14 @@ import (
 // Tx is a write transaction. Get one from Store.InTx.
 type Tx struct{ tx *sql.Tx }
 
-const taskColumns = `id, title, note, link, requester, priority, impact, due, size, parent_id, position,
-	status, drop_reason, source_kind, source_id, rollover_count, created_at, updated_at, closed_at, closed_by`
+// taskColumns leaves out priority: unused since D76, kept in the schema.
+const taskColumns = `id, title, note, link, requester, impact, due, size, parent_id, position, status,
+	drop_reason, source_kind, source_id, rollover_count, created_at, updated_at, closed_at, closed_by`
 
 // InsertTask inserts t (ignoring t.ID) with its tags and returns the new id.
 func (x *Tx) InsertTask(ctx context.Context, t domain.Task) (int64, error) {
 	res, err := x.tx.ExecContext(ctx, `INSERT INTO tasks (`+strings.TrimPrefix(taskColumns, "id, ")+`)
-		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, taskArgs(t)...)
+		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, taskArgs(t)...)
 	if err != nil {
 		return 0, err
 	}
@@ -34,7 +35,7 @@ func (x *Tx) InsertTask(ctx context.Context, t domain.Task) (int64, error) {
 // UpdateTask overwrites every column and the tag set of task t.ID.
 func (x *Tx) UpdateTask(ctx context.Context, t domain.Task) error {
 	res, err := x.tx.ExecContext(ctx, `UPDATE tasks SET title = ?, note = ?, link = ?, requester = ?,
-		priority = ?, impact = ?, due = ?, size = ?, parent_id = ?, position = ?, status = ?, drop_reason = ?, source_kind = ?,
+		impact = ?, due = ?, size = ?, parent_id = ?, position = ?, status = ?, drop_reason = ?, source_kind = ?,
 		source_id = ?, rollover_count = ?, created_at = ?, updated_at = ?, closed_at = ?, closed_by = ?
 		WHERE id = ?`, append(taskArgs(t), t.ID)...)
 	if err != nil {
@@ -222,8 +223,8 @@ func taskArgs(t domain.Task) []any {
 	if !t.ClosedAt.IsZero() {
 		closedAt = formatTime(t.ClosedAt)
 	}
-	return []any{t.Title, t.Note, t.Link, t.Requester, nullIfZero(t.Priority), nullIfZero(t.Impact),
-		nullIfZero(string(t.Due)), nullIfZero(string(t.Size)),
+	return []any{t.Title, t.Note, t.Link, t.Requester, nullIfZero(t.Impact), nullIfZero(string(t.Due)),
+		nullIfZero(string(t.Size)),
 		nullIfZero(t.ParentID), t.Position, string(t.Status), t.DropReason, sourceKind, sourceID,
 		t.RolloverCount, formatTime(t.CreatedAt), formatTime(t.UpdatedAt), closedAt, nullIfZero(t.ClosedBy)}
 }
@@ -233,18 +234,17 @@ type scanner interface{ Scan(dest ...any) error }
 func scanTask(row scanner) (domain.Task, error) {
 	var (
 		t                                         domain.Task
-		priority, impact, parentID                sql.NullInt64
+		impact, parentID                          sql.NullInt64
 		due, size, sourceKind, sourceID, closedBy sql.NullString
 		status, createdAt, updatedAt              string
 		closedAt                                  sql.NullString
 	)
-	err := row.Scan(&t.ID, &t.Title, &t.Note, &t.Link, &t.Requester, &priority, &impact, &due, &size, &parentID,
+	err := row.Scan(&t.ID, &t.Title, &t.Note, &t.Link, &t.Requester, &impact, &due, &size, &parentID,
 		&t.Position, &status, &t.DropReason, &sourceKind, &sourceID, &t.RolloverCount,
 		&createdAt, &updatedAt, &closedAt, &closedBy)
 	if err != nil {
 		return domain.Task{}, err
 	}
-	t.Priority = int(priority.Int64)
 	t.Impact = int(impact.Int64)
 	t.Due = domain.Day(due.String)
 	t.Size = domain.Size(size.String)
```

Patch `internal/tui/common.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/common.go b/internal/tui/common.go
index ab60fc5..ffabda4 100644
--- a/internal/tui/common.go
+++ b/internal/tui/common.go
@@ -12,7 +12,6 @@ import (
 	"github.com/charmbracelet/x/ansi"
 
 	"tt/internal/app"
-	"tt/internal/config"
 	"tt/internal/domain"
 )
 
@@ -91,24 +90,34 @@ func byUse(counts map[string]int) []string {
 	return out
 }
 
-// badge renders "[P1 M]": the effective priority's configured name and the size.
+// badge renders "[ 73 M]": the task's urgency today and its size (D71).
 func (m *Model) badge(t domain.Task) string {
-	return badgeOf(m.svc.Config().Priorities, domain.EffectivePriority(t, m.state.Snapshot.ParentOf(t)), t.Size)
+	u, ok := m.urgency(t, m.state.Snapshot.ParentOf(t))
+	return badgeOf(u, ok, t.Size)
 }
 
-// badgeOf renders "[P1 M]" for priority level p of levels and size.
-func badgeOf(levels []config.PriorityLevel, p int, size domain.Size) string {
-	name := "—"
-	if p > 0 && p <= len(levels) {
-		name = levels[p-1].Name
-	} else if p > 0 {
-		name = "P?"
-	}
+// urgency is t's urgency today (D70); ok is false while t is untriaged.
+func (m *Model) urgency(t domain.Task, parent *domain.Task) (float64, bool) {
+	return domain.Urgency(t, parent, m.svc.Today(), m.svc.Config().Urgency)
+}
+
+// badgeOf renders "[ 73 M]" for urgency u (0 to 1; ok false: "  —") and a
+// size: the score padded to three cells, so 100 lines up with the rest.
+func badgeOf(u float64, ok bool, size domain.Size) string {
+	score := "  —"
+	if ok {
+		score = fmt.Sprintf("%3d", domain.UrgencyScore(u))
+	}
+	return "[" + score + " " + sizeLabel(size) + "]"
+}
+
+// sizeLabel is "M" for a size, "—" for none.
+func sizeLabel(size domain.Size) string {
 	s := "—"
 	if size != domain.SizeNone {
 		s = strings.ToUpper(string(size))
 	}
-	return "[" + name + " " + s + "]"
+	return s
 }
 
 // closedToday reports whether t was closed on the plan's day (D29 wins stay visible).
@@ -205,7 +214,7 @@ func (m *Model) hintsText() string {
 		if t, ok := m.state.Snapshot.Get(m.inbox.current); ok && t.ParentID != 0 {
 			return "s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help"
 		}
-		return fmt.Sprintf("1-%d priority · s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help", len(m.svc.Config().Priorities))
+		return "1-3 impact · s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help"
 	}
 	if m.screen == screenPeople {
 		return m.peopleHints()
```

Patch `internal/tui/edit.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/edit.go b/internal/tui/edit.go
index a6e2020..8498624 100644
--- a/internal/tui/edit.go
+++ b/internal/tui/edit.go
@@ -22,7 +22,7 @@ type editLine struct {
 }
 
 func (m *Model) openEdit(t domain.Task) tea.Cmd {
-	in := newTokenInput(fmt.Sprintf("edit #%d › ", t.ID), "title @who #tag !1 ~s link -- note (replaces it)", m.width)
+	in := newTokenInput(fmt.Sprintf("edit #%d › ", t.ID), "title @who #tag !1 ^fri ~s link -- note (replaces it)", m.width)
 	in.input.SetValue(app.EditLine(t))
 	in.input.CursorEnd()
 	m.overlay = &editLine{in: in, task: t}
```

Patch `internal/tui/inbox.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/inbox.go b/internal/tui/inbox.go
index 9fc6f0b..c07bc1b 100644
--- a/internal/tui/inbox.go
+++ b/internal/tui/inbox.go
@@ -67,19 +67,18 @@ func (m *Model) inboxKey(key string) tea.Cmd {
 		return nil
 	}
 	label := fmt.Sprintf("%q", t.Title)
-	levels := len(m.svc.Config().Priorities)
 	switch {
 	case len(key) == 1 && key[0] >= '1' && key[0] <= '9':
 		n := int(key[0] - '0')
-		if n > levels {
-			m.setStatus(fmt.Sprintf("there are %d priority levels", levels))
+		if n > domain.MaxImpact {
+			m.setStatus(fmt.Sprintf("impact is 1 to %d", domain.MaxImpact))
 			return nil
 		}
-		if parent := m.state.Snapshot.ParentOf(t); parent != nil {
-			m.setStatus(fmt.Sprintf("priority comes from parent #%d %q", parent.ID, parent.Title))
+		if t.ParentID != 0 {
+			m.setStatus(fmt.Sprintf("children inherit impact from #%d", t.ParentID)) // D73
 			return nil
 		}
-		return m.act("priority "+label, "", func(ctx context.Context) (app.Undo, error) { return m.svc.SetPriority(ctx, t.ID, n) })
+		return m.act("impact "+label, "", func(ctx context.Context) (app.Undo, error) { return m.svc.SetImpact(ctx, t.ID, n) })
 	case key == "s" || key == "m" || key == "l" || key == "x":
 		size, _ := domain.ParseSize(key)
 		return m.act("size "+label, "", func(ctx context.Context) (app.Undo, error) { return m.svc.SetSize(ctx, t.ID, size) })
@@ -134,18 +133,19 @@ func (m *Model) inboxView() (string, []string) {
 	}
 	b.WriteString("\n")
 	if parent != nil {
-		fmt.Fprintf(&b, "  priority  %s\n", styleDim.Render(fmt.Sprintf("from parent: %s", m.badge(*parent))))
+		impact := "not set"
+		if parent.Impact > 0 {
+			impact = domain.ImpactNames[parent.Impact-1]
+		}
+		fmt.Fprintf(&b, "  impact    %s\n", styleDim.Render(fmt.Sprintf("from parent #%d: %s", parent.ID, impact)))
 	} else {
-		for i, lvl := range cfg.Priorities {
+		fmt.Fprintf(&b, "  impact    %s\n", styleDim.Render("who notices if it slips?"))
+		for i, name := range domain.ImpactNames {
 			mark := " "
-			if t.Priority == i+1 {
+			if t.Impact == i+1 {
 				mark = "●"
 			}
-			lead := "          "
-			if i == 0 {
-				lead = "  priority"
-			}
-			fmt.Fprintf(&b, "%s %s [%d] %s · %s\n", lead, mark, i+1, lvl.Name, lvl.Definition)
+			fmt.Fprintf(&b, "           %s [%d] %s · %s\n", mark, i+1, name, domain.ImpactExamples[i])
 		}
 	}
 	var sizes []string
```

Patch `internal/tui/lane.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/lane.go b/internal/tui/lane.go
index f6367e6..be0e6d3 100644
--- a/internal/tui/lane.go
+++ b/internal/tui/lane.go
@@ -207,10 +207,11 @@ func (m *Model) pinReview() tea.Cmd {
 		}
 	}
 	label := fmt.Sprintf("%q", domain.ReviewTitle(i))
-	svc, ctx, levels := m.svc, m.ctx, m.svc.Config().Priorities
+	svc, ctx := m.svc, m.ctx
 	return func() tea.Msg {
 		t, undo, err := svc.PinSource(ctx, i.Kind, i.ExternalID)
-		badge := strings.Trim(badgeOf(levels, t.Priority, t.Size), "[]")
+		u, ok := domain.Urgency(t, nil, svc.Today(), svc.Config().Urgency) // a review's task is top-level
+		badge := strings.TrimSpace(strings.Trim(badgeOf(u, ok, t.Size), "[]"))
 		return actionMsg{label: "pin " + label, status: "pinned " + label + " · " + badge, undo: undo, err: err}
 	}
 }
```

Patch `internal/tui/model.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/model.go b/internal/tui/model.go
index 8b0b38d..54cf72a 100644
--- a/internal/tui/model.go
+++ b/internal/tui/model.go
@@ -107,7 +107,7 @@ type Model struct {
 
 func New(ctx context.Context, svc *app.Service, opts Options) *Model {
 	now := svc.Now()
-	return &Model{svc: svc, opts: opts, ctx: ctx, width: 100, height: 30, pick: pickState{group: search.ByPriority},
+	return &Model{svc: svc, opts: opts, ctx: ctx, width: 100, height: 30, pick: pickState{group: search.ByUrgency},
 		notify: notifyState{launched: now, checked: now}}
 }
 
```

Patch `internal/tui/overlays.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/overlays.go b/internal/tui/overlays.go
index b07f4ba..edbb9c2 100644
--- a/internal/tui/overlays.go
+++ b/internal/tui/overlays.go
@@ -39,7 +39,7 @@ type quickAdd struct {
 }
 
 func (m *Model) openQuickAdd() tea.Cmd {
-	m.overlay = &quickAdd{in: newTokenInput("add › ", "title @who #tag !1 ~s link -- note", m.width)}
+	m.overlay = &quickAdd{in: newTokenInput("add › ", "title @who #tag !1 ^fri ~s link -- note", m.width)}
 	return nil
 }
 
@@ -55,7 +55,7 @@ func (q *quickAdd) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
 		return nil
 	case "enter":
 		line := q.in.line()
-		if _, err := dsl.ParseAdd(line, len(m.svc.Config().Priorities)); err != nil {
+		if _, err := dsl.ParseAdd(line, m.svc.Today()); err != nil {
 			q.err = err
 			return nil
 		}
@@ -257,7 +257,7 @@ func (helpOverlay) update(m *Model, _ tea.KeyPressMsg) tea.Cmd {
 
 func (helpOverlay) view(m *Model) string {
 	global := []string{
-		"a        quick-add: title @who #tag !1 ~s link -- note (tab completes @/#)",
+		"a        quick-add: title @who #tag !1 ^fri ~s link -- note (!1-!3 impact, ^ due; tab completes @/#)",
 		"ctrl+z   undo (refuses if the task changed since)",
 		"ctrl+d / ctrl+u   half a page down / up",
 		"R        reload, and fetch the calendar and GitHub now · q quit · ? this help",
@@ -268,9 +268,9 @@ func (helpOverlay) view(m *Model) string {
 		local = []string{
 			"j k      move between the task and its children",
 			"⏎        open the child under the cursor · esc back (child → parent → where you came from)",
-			"e / E    edit the line (title @who #tag !1 ~s link) / the note in $EDITOR",
+			"e / E    edit the line (title @who #tag !1 ^fri ~s link) / the note in $EDITOR",
 			"o        open the link",
-			"a        add children, one per line (title ~s #tag -- note; priority is the parent's)",
+			"a        add children, one per line (title ~s ^fri #tag -- note; impact is the parent's)",
 			"J / K    move the selected child down / up",
 			"p / u    pin to today / unpin",
 			"x        done (again: reopen)",
@@ -278,8 +278,8 @@ func (helpOverlay) view(m *Model) string {
 		}
 	case screenInbox:
 		local = []string{
-			fmt.Sprintf("1-%d      set priority", len(m.svc.Config().Priorities)),
-			"s m l x  set size (x = XL); a card with priority and size leaves the inbox",
+			"1-3      set impact: who notices if it slips (a person, one team, more than one team)",
+			"s m l x  set size (x = XL); a card with impact and size leaves the inbox",
 			"n        skip (stays in the inbox)",
 			"d / D    drop with optional reason / delete",
 			"esc      back",
@@ -298,7 +298,7 @@ func (helpOverlay) view(m *Model) string {
 			"x        done (again: reopen)",
 			"d / D    drop with optional reason / delete",
 			"/        filter: @who #tag !1 ~s <7d >30d is:done|dropped|all words (live; esc on the list clears)",
-			"g        group by priority → size → created → requester (remembered)",
+			"g        sort by urgency → impact, group by size → created → requester (remembered)",
 			"B        done picking: block today's candidates in Today",
 			"w        the Reviews lane: p pins a review (a task Review #n), o opens the PR · w or tab back",
 			"esc      back to Today (when no filter is set) · i inbox · P people (who asked for what)",
```

Patch `internal/tui/people.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/people.go b/internal/tui/people.go
index bb275e3..2dcc718 100644
--- a/internal/tui/people.go
+++ b/internal/tui/people.go
@@ -3,7 +3,6 @@ package tui
 import (
 	"cmp"
 	"fmt"
-	"maps"
 	"slices"
 	"strings"
 
@@ -108,25 +107,23 @@ func (m *Model) peopleFilterLines() []string {
 // namesWidth is People's left column for a terminal width cells wide.
 func namesWidth(width int) int { return width / 3 }
 
-// personRows lists name's tasks (D52): the open ones grouped by priority
-// like the backlog, pinned ones too, oldest first within a group; then a
-// closed heading over the done and dropped ones, newest first. Every task
+// personRows lists name's tasks (D52): the open ones, pinned ones too, in
+// the urgency order, flat (D72); then a closed heading over the done and
+// dropped ones, newest first. Every task
 // is a row of its own: a child shows ↳ #parent, a parent its progress.
 func (m *Model) personRows(name string) []row {
 	if name == "" {
 		return nil
 	}
-	snap, now, headings := m.state.Snapshot, m.svc.Now(), m.priorityHeadings()
-	groups := map[search.Group][]domain.Task{}
-	var closed []domain.Task
+	snap := m.state.Snapshot
+	var open, closed []domain.Task
 	for _, t := range snap.Tasks() {
 		switch {
 		case t.Requester != name:
 		case t.Status != domain.StatusOpen:
 			closed = append(closed, t)
 		default:
-			g := search.GroupOf(search.ByPriority, t, snap.ParentOf(t), now, headings)
-			groups[g] = append(groups[g], t)
+			open = append(open, t)
 		}
 	}
 	var rows []row
@@ -137,12 +134,9 @@ func (m *Model) personRows(name string) []row {
 		}
 		rows = append(rows, r)
 	}
-	for _, g := range slices.SortedFunc(maps.Keys(groups), search.Compare) {
-		slices.SortFunc(groups[g], search.Oldest)
-		rows = append(rows, row{kind: rowHeader, text: g.Label})
-		for _, t := range groups[g] {
-			add(t)
-		}
+	slices.SortFunc(open, search.Sorter(search.ByUrgency, snap.ParentOf, m.svc.Today(), m.svc.Config().Urgency))
+	for _, t := range open {
+		add(t)
 	}
 	if len(closed) > 0 {
 		slices.SortFunc(closed, func(a, b domain.Task) int {
```

Patch `internal/tui/pick.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/pick.go b/internal/tui/pick.go
index b39b90b..660b30e 100644
--- a/internal/tui/pick.go
+++ b/internal/tui/pick.go
@@ -79,7 +79,7 @@ func (m *Model) refreshRows() {
 // applyFilter makes text the backlog filter (D26a). On a parse error the
 // current filter stays and the error is returned.
 func (m *Model) applyFilter(text string) error {
-	f, err := dsl.ParseFilter(text, len(m.svc.Config().Priorities))
+	f, err := dsl.ParseFilter(text)
 	if err != nil {
 		return err
 	}
@@ -95,8 +95,8 @@ func (m *Model) applyFilter(text string) error {
 // the parent is expanded; filtered: always). A child whose parent isn't listed
 // (closed, in the Inbox, missing, or not matching) is listed top-level with
 // ↳ #parent so it never vanishes; the children of a pinned parent live under
-// it in Today instead. Top-level tasks are grouped (D26), oldest first within
-// a group.
+// it in Today instead. Top-level tasks are sorted by urgency or impact, with
+// no headings (D72), or grouped, oldest first within a group (D26).
 func (m *Model) backlogRows() []row {
 	snap := m.state.Snapshot
 	now := m.svc.Now()
@@ -110,7 +110,6 @@ func (m *Model) backlogRows() []row {
 			listed[t.ID] = true
 		}
 	}
-	headings := m.priorityHeadings()
 	groups := map[search.Group][]domain.Task{}
 	orphanOf := map[int64]int64{}
 	for _, t := range snap.Tasks() {
@@ -126,14 +125,17 @@ func (m *Model) backlogRows() []row {
 			}
 			orphanOf[t.ID] = t.ParentID
 		}
-		g := search.GroupOf(m.pick.group, t, snap.ParentOf(t), now, headings)
+		g := search.GroupOf(m.pick.group, t, now)
 		groups[g] = append(groups[g], t)
 	}
+	order := search.Sorter(m.pick.group, snap.ParentOf, m.svc.Today(), m.svc.Config().Urgency)
 	var rows []row
 	for _, g := range slices.SortedFunc(maps.Keys(groups), search.Compare) {
 		items := groups[g]
-		slices.SortFunc(items, search.Oldest)
-		rows = append(rows, row{kind: rowHeader, text: g.Label})
+		slices.SortFunc(items, order)
+		if !m.pick.group.Flat() {
+			rows = append(rows, row{kind: rowHeader, text: g.Label})
+		}
 		for _, t := range items {
 			var kids []domain.Task
 			for _, c := range m.idx.children[t.ID] {
@@ -155,17 +157,6 @@ func (m *Model) backlogRows() []row {
 	return rows
 }
 
-// priorityHeadings are the priority groups' headings, one per configured
-// level: "P1 · someone is blocked" (D26).
-func (m *Model) priorityHeadings() []string {
-	levels := m.svc.Config().Priorities
-	headings := make([]string, len(levels))
-	for i, l := range levels {
-		headings[i] = l.Name + " · " + l.Definition
-	}
-	return headings
-}
-
 // todayRows: today's candidates in plan order. A pinned parent lists its open
 // children and children closed today beneath it; a pinned child of a pinned
 // parent appears only there.
@@ -453,7 +444,11 @@ func (m *Model) pickKey(key string) tea.Cmd {
 	case "g":
 		p.group, p.groupChosen = p.group.Next(), true
 		m.refreshRows()
-		m.setStatus("grouped by " + string(p.group))
+		verb := "grouped by "
+		if p.group.Flat() {
+			verb = "sorted by "
+		}
+		m.setStatus(verb + string(p.group))
 		return m.saveGroup()
 	case "esc":
 		if p.filtered() {
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add README.md cmd/tt/main.go cmd/tt/main_test.go config.example.toml internal/app/children.go internal/app/children_test.go internal/app/day.go internal/app/day_test.go internal/app/pinsource_test.go internal/app/service_test.go internal/app/sources.go internal/app/tasks.go internal/app/undo_test.go internal/config/config.go internal/config/config_test.go internal/config/github_test.go internal/domain/domain_test.go internal/domain/rules.go internal/domain/task.go internal/dsl/add.go internal/dsl/add_test.go internal/dsl/filter.go internal/dsl/filter_test.go internal/dsl/requester_test.go internal/search/search.go internal/search/search_test.go internal/store/migration006_test.go internal/store/store_test.go internal/store/tasks.go internal/tui/browse_test.go internal/tui/children_test.go internal/tui/colour_test.go internal/tui/common.go internal/tui/detail_test.go internal/tui/edit.go internal/tui/edit_test.go internal/tui/filter_test.go internal/tui/filterline_test.go internal/tui/halfpage_test.go internal/tui/inbox.go internal/tui/lane.go internal/tui/lane_test.go internal/tui/layout_test.go internal/tui/meetings_test.go internal/tui/model.go internal/tui/notifications_test.go internal/tui/overlays.go internal/tui/people.go internal/tui/people_test.go internal/tui/pick.go internal/tui/pick_test.go internal/tui/place_test.go internal/tui/program_test.go internal/tui/rename_test.go internal/tui/reslot_test.go internal/tui/tab_test.go internal/tui/today_test.go internal/tui/tui_test.go
git commit -m 'feat: impact and urgency replace priority across tt (D68, D71-D76)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 4: Due on screen: the marker, Task detail, the Inbox's due first and ⏎

D71, D73. `dueMarker` words a due against today: `→today`, `→fri` within 6 days, `→30 oct` later (`→15 nov 2027` over a year away), `2d late` in the alert colour; `(*Model).due` uses the effective due of open tasks. It follows the title (and its ✎) on every row (the Log, Today's candidates, People) and on Task detail's children. Task detail gets a line under its header: `impact: one team · due Fri 16 Oct · urgency 73`, with `(from #12)` after an inherited value and `impact: not set` when untriaged. The Inbox card shows the due first (`due       — · ^ to set`; a child's inherited one `from parent #1 · ^ to set its own`). `^` opens a `due › ` prompt (prefilled with the task's own due, any `^` form, empty clears, a bad form keeps the prompt with its error). The card no longer leaves on its own: `inboxState.held` keeps it on screen once a key acted on it, `triaged · ⏎ next card` shows when it is, and `⏎` moves on (`set impact and size first · n skips`, a child `set the size first · n skips`, otherwise). A held card that is dropped or closed leaves at once; ctrl+z of its last triage step leaves it untriaged on screen.

**Files:**
- Modify: `internal/tui/common.go`
- Modify: `internal/tui/detail.go`
- Modify: `internal/tui/inbox.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/pick.go`
- Create: `internal/tui/due_test.go`
- Modify: `internal/tui/tui_test.go`

**Interfaces:**
- Consumes: Task 1 (`EffectiveDue`, `EffectiveImpact`, `ImpactNames`, `UrgencyScore`, `DaysBetween`), Task 2 (`dsl.ParseDue`), Task 3 (`SetDue`, `(*Model).urgency`, `badgeOf`); existing `prompt`, `newInput`, `dayLabel`, `styleAlert`, harness helpers `before`, `family`.
- Produces:
  - internal/tui/common.go: `func dueMarker(today, due domain.Day) string`; `func (m *Model) due(t domain.Task) string`.
  - internal/tui/detail.go: `func (m *Model) urgencyLine(t domain.Task) string`.
  - internal/tui/inbox.go: `inboxState.held int64`; `func (m *Model) openDuePrompt(t domain.Task) tea.Cmd`.

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/due_test.go`:

```go
package tui

import (
	"context"
	"fmt"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"tt/internal/config"
	"tt/internal/domain"
)

// The harness's today is Monday 12 Oct 2026.
func TestDueMarker(t *testing.T) {
	const today = domain.Day("2026-10-12")
	cases := map[domain.Day]string{
		"":           "",
		"2026-10-12": "→today",
		"2026-10-13": "→tue",
		"2026-10-16": "→fri",
		"2026-10-18": "→sun",
		"2026-10-19": "→19 oct",
		"2027-01-03": "→3 jan",
		"2027-10-12": "→12 oct",
		"2027-11-15": "→15 nov 2027",
		"2026-10-11": "1d late",
		"2026-10-02": "10d late",
	}
	for due, want := range cases {
		if got := ansi.Strip(dueMarker(today, due)); got != want {
			t.Errorf("dueMarker(%s) = %q; want %q", due, got, want)
		}
	}
}

func TestRowsShowTheDueAndUrgency(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.add("Undated stream task !3 ~xl")
	h.add("Ask due today @ana !1 ~s ^today")
	parent := h.add("Migration !2 ~l ^2026-10-30")
	if _, _, err := h.svc.AddChild(context.Background(), parent.ID, "Dry run ~s"); err != nil {
		t.Fatal(err)
	}
	h.add("Overdue ask !1 ~s ^2026-10-09")
	h.keys("R", "j", "j", "j", "right")
	// On the urgency sort (D72): impact 1 three days overdue 0.59, impact 1
	// due today 0.53, impact 3 undated 0.50, impact 2 due in 18 days 0.43.
	h.before("BACKLOG · by urgency",
		"[ 59 S] Overdue ask 3d late",
		"[ 53 S] Ask due today →today @ana",
		"[ 50 XL] Undated stream task",
		"[ 43 L] Migration →30 oct 0/1", "[ 43 S] Dry run →30 oct") // the child takes its parent's due (D69)
	h.mustNotSee("Undated stream task →")
}

func TestDetailShowsImpactDueAndUrgency(t *testing.T) {
	h := newLogHarness(t, config.Default())
	parent, kids := h.family("Migration !2 ~l ^fri", "Dry run ~s", "Cutover ~s")
	if _, err := h.svc.SetDue(context.Background(), kids[1].ID, "2026-10-13"); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "enter")
	// Impact 2 due in 4 days: 0.62; due tomorrow: 0.68.
	h.mustSee("impact: one team · due Fri 16 Oct · urgency 62", "[ 62 S] Dry run →fri", "[ 68 S] Cutover →tue")
	h.keys("j", "enter")
	h.mustSee("#2 Dry run", fmt.Sprintf("impact: one team (from #%d) · due Fri 16 Oct (from #%[1]d) · urgency 62", parent.ID))
	h.keys("esc", "j", "j", "enter")
	h.mustSee("#3 Cutover", fmt.Sprintf("impact: one team (from #%d) · due Tue 13 Oct · urgency 68", parent.ID))

	h.add("Untriaged thing ~s")
	h.keys("esc", "esc", "i", "enter")
	h.mustSee("set impact and size first · n skips")
}

func TestInboxShowsTheDueFirstAndWaitsForEnter(t *testing.T) {
	h := newLogHarness(t, config.Default())
	first := h.add("Ship it")
	h.add("Second")
	h.keys("R", "i")
	h.before("INBOX · 1 of 2", "Ship it", "due       — · ^ to set", "impact", "size") // D73
	h.keys("enter")
	h.mustSee("set impact and size first · n skips", "INBOX · 1 of 2", "Ship it")

	h.keys("^")
	h.mustSee("due › ")
	h.typeText("someday")
	h.keys("enter")
	h.mustSee(`✗ due "someday": want today, tom, a weekday like fri, 3d, 2w, 30oct or 2026-10-30`)
	h.keys("esc", "^")
	h.typeText("^fri")
	h.keys("enter")
	h.mustSee("due       Fri 16 Oct →fri · ^ to change")
	if got := h.task(first.ID); got.Due != "2026-10-16" {
		t.Errorf("due = %q; want Fri 16 Oct", got.Due)
	}

	h.keys("3", "s")
	h.mustSee("INBOX · 1 of 2", "Ship it", "triaged · ⏎ next card")
	h.keys("^") // still the same card: the due can change after triage
	h.mustSee("due › 2026-10-16")
	h.keys("esc", "enter")
	h.mustSee("INBOX · 1 of 1", "Second")
	h.keys("esc")
	h.mustSee("[ 79 S] Ship it →fri") // impact 3 due in 4 days

	// ctrl+z undoes the last triage step; the card is back in the Inbox.
	h.keys("i", "1", "m")
	h.mustSee("triaged · ⏎ next card")
	h.keys("ctrl+z")
	h.mustSee("INBOX · 1 of 1", "Second")
	h.mustNotSee("triaged · ⏎ next card")
}

func TestInboxChildCardShowsTheInheritedDue(t *testing.T) {
	h := newLogHarness(t, config.Default())
	parent := h.add("Migration !2 ~l ^fri")
	if _, _, err := h.svc.AddChild(context.Background(), parent.ID, "Dry run"); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "i")
	h.mustSee("INBOX · 1 of 1", "Dry run", "due       Fri 16 Oct from parent #1 · ^ to set its own", "from parent #1: one team")
	h.keys("enter")
	h.mustSee("set the size first · n skips")
	h.keys("^")
	h.typeText("tom")
	h.keys("enter", "s")
	h.mustSee("due       Tue 13 Oct →tue · ^ to change", "triaged · ⏎ next card")
	h.keys("enter")
	h.mustSee("Inbox zero ✓")
}

// Review Focus: a held card that is dropped (or closed elsewhere) leaves the
// screen at once; the next card shows.
func TestInboxHeldCardLeavesWhenDropped(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.add("Doomed")
	h.add("Next one")
	h.keys("R", "i", "2", "s")
	h.mustSee("INBOX · 1 of 2", "Doomed", "triaged · ⏎ next card")
	h.keys("d")
	h.mustSee("reason (enter to skip)")
	h.keys("enter")
	h.mustSee("INBOX · 1 of 1", "Next one", `dropped "Doomed"`)
	h.mustNotSee("triaged · ⏎ next card")
}
```

Patch `internal/tui/tui_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/tui_test.go b/internal/tui/tui_test.go
index b14860c..17c9cc3 100644
--- a/internal/tui/tui_test.go
+++ b/internal/tui/tui_test.go
@@ -17,7 +17,7 @@ import (
 	"tt/internal/store"
 )
 
-func TestQuickAddThenTriageInTwoKeystrokes(t *testing.T) {
+func TestQuickAddThenTriageInThreeKeys(t *testing.T) {
 	h := newLogHarness(t, config.Default())
 	h.mustSee("tt · Log", "Mon 12 Oct", "nothing triaged yet")
 
@@ -29,6 +29,9 @@ func TestQuickAddThenTriageInTwoKeystrokes(t *testing.T) {
 	h.keys("i")
 	h.mustSee("INBOX · 1 of 1", "Reply to Ana", "@ana · #photos", "[1] a person · me, one reviewer, one asker")
 	h.keys("1", "s") // D11a: impact + size → leaves the inbox
+	// The card waits for ⏎ (D73).
+	h.mustSee("INBOX · 1 of 1", "triaged · ⏎ next card")
+	h.keys("enter")
 	h.mustSee("Inbox zero ✓")
 	h.keys("esc")
 	h.mustSee("BACKLOG · by urgency", "[ 17 S] Reply to Ana @ana") // impact 1, no due: 17 (D71)
@@ -250,6 +253,8 @@ func TestInboxSkipAndChildImpact(t *testing.T) {
 	h.keys("n")
 	h.mustSee("Child") // everything skipped: start over
 	h.keys("m")        // size alone triages a child (D74)
+	h.mustSee("INBOX · 1 of 2", "Child", "triaged · ⏎ next card")
+	h.keys("enter")
 	h.mustSee("INBOX · 1 of 1", "Other")
 	h.keys("9")
 	h.mustSee("impact is 1 to 3")
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL (build): `internal/tui/due_test.go:31:24: undefined: dueMarker`.

- [ ] **Step 3: Implement**

Patch `internal/tui/common.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/common.go b/internal/tui/common.go
index ffabda4..9858d0a 100644
--- a/internal/tui/common.go
+++ b/internal/tui/common.go
@@ -111,6 +111,39 @@ func badgeOf(u float64, ok bool, size domain.Size) string {
 	return "[" + score + " " + sizeLabel(size) + "]"
 }
 
+// dueMarker is the marker after an open task's title when it has a due
+// (D71), worded against today: "→today"; "→fri" within the next 6 days;
+// "→30 oct" later, with the year when it is more than a year away; "2d late"
+// once overdue, in the alert colour. "" without a due.
+func dueMarker(today, due domain.Day) string {
+	if due == "" {
+		return ""
+	}
+	d := domain.DaysBetween(today, due)
+	date, err := time.Parse(time.DateOnly, string(due))
+	switch {
+	case err != nil:
+		return ""
+	case d < 0:
+		return styleAlert.Render(fmt.Sprintf("%dd late", -d))
+	case d == 0:
+		return "→today"
+	case d < 7:
+		return "→" + strings.ToLower(date.Format("Mon"))
+	case d > 365:
+		return "→" + strings.ToLower(date.Format("2 Jan 2006"))
+	}
+	return "→" + strings.ToLower(date.Format("2 Jan"))
+}
+
+// due is t's due marker today (D71): its effective due, open tasks only.
+func (m *Model) due(t domain.Task) string {
+	if t.Status != domain.StatusOpen {
+		return ""
+	}
+	return dueMarker(m.svc.Today(), domain.EffectiveDue(t, m.state.Snapshot.ParentOf(t)))
+}
+
 // sizeLabel is "M" for a size, "—" for none.
 func sizeLabel(size domain.Size) string {
 	s := "—"
@@ -212,9 +245,9 @@ func (m *Model) hintsText() string {
 	}
 	if m.screen == screenInbox {
 		if t, ok := m.state.Snapshot.Get(m.inbox.current); ok && t.ParentID != 0 {
-			return "s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help"
+			return "^ due · s/m/l/x size · ⏎ next · n skip · d drop · esc back · a add · ctrl+z undo · ? help"
 		}
-		return "1-3 impact · s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help"
+		return "^ due · 1-3 impact · s/m/l/x size · ⏎ next · n skip · d drop · esc back · a add · ctrl+z undo · ? help"
 	}
 	if m.screen == screenPeople {
 		return m.peopleHints()
```

Patch `internal/tui/detail.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/detail.go b/internal/tui/detail.go
index 090db5b..ea92b2c 100644
--- a/internal/tui/detail.go
+++ b/internal/tui/detail.go
@@ -174,7 +174,7 @@ func (m *Model) detailHead() []string {
 	case t.Status == domain.StatusDropped:
 		head += " ✗ " + dayLabel(m.svc.DayOf(t.ClosedAt))
 	}
-	lines := []string{head}
+	lines := []string{head, styleDim.Render(m.urgencyLine(t))}
 
 	var facts []string
 	if p := snap.ParentOf(t); p != nil {
@@ -214,6 +214,34 @@ func (m *Model) detailHead() []string {
 	return lines
 }
 
+// urgencyLine is Task detail's "impact: one team · due Fri 16 Oct ·
+// urgency 73" (D71): an inherited value says "(from #12)", an untriaged
+// task "impact: not set".
+func (m *Model) urgencyLine(t domain.Task) string {
+	parent := m.state.Snapshot.ParentOf(t)
+	from := ""
+	if parent != nil {
+		from = fmt.Sprintf(" (from #%d)", parent.ID)
+	}
+	impact := "impact: not set"
+	if n := domain.EffectiveImpact(t, parent); n > 0 {
+		impact = "impact: " + domain.ImpactNames[n-1] + from
+	}
+	parts := []string{impact}
+	switch due := domain.EffectiveDue(t, parent); {
+	case due == "":
+		parts = append(parts, "no due")
+	case t.Due == "":
+		parts = append(parts, "due "+dayLabel(due)+from)
+	default:
+		parts = append(parts, "due "+dayLabel(due))
+	}
+	if u, ok := m.urgency(t, parent); ok {
+		parts = append(parts, fmt.Sprintf("urgency %d", domain.UrgencyScore(u)))
+	}
+	return strings.Join(parts, " · ")
+}
+
 // progress renders " ████░░░░ 2/4" for a parent; "" when nothing counts.
 func (m *Model) progress(t domain.Task) string {
 	done, total := 0, 0
@@ -266,6 +294,9 @@ func (m *Model) childLine(c domain.Task, selected bool) string {
 	if c.Note != "" {
 		line += " ✎"
 	}
+	if due := m.due(c); due != "" {
+		line += " " + due // D71
+	}
 	if _, pinned := m.idx.cand[c.ID]; pinned {
 		line += " 📌"
 	}
```

Patch `internal/tui/inbox.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/inbox.go b/internal/tui/inbox.go
index c07bc1b..c180cdc 100644
--- a/internal/tui/inbox.go
+++ b/internal/tui/inbox.go
@@ -10,24 +10,33 @@ import (
 
 	"tt/internal/app"
 	"tt/internal/domain"
+	"tt/internal/dsl"
 )
 
 // inboxState shows one card at a time (D11a). Skipped cards move to the end
-// for this session; they stay in the Inbox.
+// for this session; they stay in the Inbox. A card stays on screen once
+// triaged, until ⏎ moves on (D73): held is that card.
 type inboxState struct {
 	current int64
+	held    int64 // the card being triaged: shown until ⏎ even once it left the Inbox
 	skipped map[int64]bool
 	from    screen // esc returns here
 }
 
 // openInbox shows the Inbox; esc returns to the current screen.
 func (m *Model) openInbox() {
-	m.inbox.from, m.screen = m.screen, screenInbox
+	m.inbox.from, m.screen, m.inbox.held = m.screen, screenInbox, 0
 	m.inbox.sync(m)
 }
 
+// order is the cards: the held one first while it is triaged but still
+// open, then the Inbox, skipped ones last.
 func (in *inboxState) order(m *Model) []domain.Task {
 	tasks := m.state.Snapshot.Inbox()
+	if t, ok := m.state.Snapshot.Get(in.held); ok && t.Status == domain.StatusOpen &&
+		!slices.ContainsFunc(tasks, func(x domain.Task) bool { return x.ID == t.ID }) {
+		tasks = append([]domain.Task{t}, tasks...)
+	}
 	slices.SortStableFunc(tasks, func(a, b domain.Task) int {
 		switch sa, sb := in.skipped[a.ID], in.skipped[b.ID]; {
 		case sa == sb:
@@ -63,11 +72,25 @@ func (m *Model) inboxKey(key string) tea.Cmd {
 		return nil
 	}
 	t, ok := m.state.Snapshot.Get(in.current)
-	if !ok || !m.idx.inInbox[t.ID] {
+	if !ok || !m.idx.inInbox[t.ID] && t.ID != in.held {
 		return nil
 	}
 	label := fmt.Sprintf("%q", t.Title)
 	switch {
+	case key == "enter":
+		if domain.InInbox(t, m.state.Snapshot.ParentOf(t)) {
+			if t.ParentID != 0 {
+				m.setStatus("set the size first · n skips")
+			} else {
+				m.setStatus("set impact and size first · n skips")
+			}
+			return nil
+		}
+		in.held = 0
+		in.sync(m)
+	case key == "^":
+		in.held = t.ID
+		return m.openDuePrompt(t)
 	case len(key) == 1 && key[0] >= '1' && key[0] <= '9':
 		n := int(key[0] - '0')
 		if n > domain.MaxImpact {
@@ -78,11 +101,14 @@ func (m *Model) inboxKey(key string) tea.Cmd {
 			m.setStatus(fmt.Sprintf("children inherit impact from #%d", t.ParentID)) // D73
 			return nil
 		}
+		in.held = t.ID
 		return m.act("impact "+label, "", func(ctx context.Context) (app.Undo, error) { return m.svc.SetImpact(ctx, t.ID, n) })
 	case key == "s" || key == "m" || key == "l" || key == "x":
 		size, _ := domain.ParseSize(key)
+		in.held = t.ID
 		return m.act("size "+label, "", func(ctx context.Context) (app.Undo, error) { return m.svc.SetSize(ctx, t.ID, size) })
 	case key == "n":
+		in.held = 0
 		in.skipped[t.ID] = true
 		order := in.order(m)
 		if in.skipped[order[0].ID] { // everything skipped: start over
@@ -132,6 +158,15 @@ func (m *Model) inboxView() (string, []string) {
 		fmt.Fprintf(&b, "  %s\n", styleDim.Render("note: "+note))
 	}
 	b.WriteString("\n")
+	// The optional due comes first (D73).
+	switch due := domain.EffectiveDue(t, parent); {
+	case due == "":
+		fmt.Fprintf(&b, "  due       %s\n", styleDim.Render("— · ^ to set"))
+	case t.Due == "":
+		fmt.Fprintf(&b, "  due       %s %s\n", dayLabel(due), styleDim.Render(fmt.Sprintf("from parent #%d · ^ to set its own", parent.ID)))
+	default:
+		fmt.Fprintf(&b, "  due       %s %s\n", dayLabel(due), styleDim.Render(dueMarker(m.svc.Today(), due)+" · ^ to change"))
+	}
 	if parent != nil {
 		impact := "not set"
 		if parent.Impact > 0 {
@@ -161,5 +196,27 @@ func (m *Model) inboxView() (string, []string) {
 		sizes = append(sizes, item)
 	}
 	fmt.Fprintf(&b, "  size       %s", strings.Join(sizes, "  "))
+	if !domain.InInbox(t, parent) {
+		fmt.Fprintf(&b, "\n\n  %s", styleDim.Render("triaged · ⏎ next card"))
+	}
 	return title, strings.Split(b.String(), "\n")
 }
+
+// openDuePrompt asks for t's due date (D73): the ^ forms, with or without
+// the ^; empty clears it.
+func (m *Model) openDuePrompt(t domain.Task) tea.Cmd {
+	label := fmt.Sprintf("%q", t.Title)
+	today := m.svc.Today()
+	in := newInput("due › ", "fri · tom · 3d · 30oct · 2026-10-30 · empty clears", m.width)
+	in.SetValue(string(t.Due))
+	in.CursorEnd()
+	m.overlay = &prompt{
+		input: in,
+		check: func(v string) error { _, err := dsl.ParseDue(v, today); return err },
+		onEnter: func(v string) tea.Cmd {
+			due, _ := dsl.ParseDue(v, today) // check passed
+			return m.act("due "+label, "", func(ctx context.Context) (app.Undo, error) { return m.svc.SetDue(ctx, t.ID, due) })
+		},
+	}
+	return nil
+}
```

Patch `internal/tui/overlays.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/overlays.go b/internal/tui/overlays.go
index edbb9c2..da9093e 100644
--- a/internal/tui/overlays.go
+++ b/internal/tui/overlays.go
@@ -278,8 +278,10 @@ func (helpOverlay) view(m *Model) string {
 		}
 	case screenInbox:
 		local = []string{
+			"^        set the due date (optional): fri, tom, 3d, 2w, 30oct, 2026-10-30; empty clears",
 			"1-3      set impact: who notices if it slips (a person, one team, more than one team)",
-			"s m l x  set size (x = XL); a card with impact and size leaves the inbox",
+			"s m l x  set size (x = XL)",
+			"⏎        next card, once impact and size are set (a child needs only its size)",
 			"n        skip (stays in the inbox)",
 			"d / D    drop with optional reason / delete",
 			"esc      back",
```

Patch `internal/tui/pick.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/pick.go b/internal/tui/pick.go
index 660b30e..8a54271 100644
--- a/internal/tui/pick.go
+++ b/internal/tui/pick.go
@@ -643,6 +643,9 @@ func (m *Model) rowLine(r row, width int, selected bool) string {
 	if t.Note != "" {
 		b.WriteString(" ✎")
 	}
+	if due := m.due(t); due != "" {
+		b.WriteString(" " + due) // D71
+	}
 	if t.Requester != "" && !r.person {
 		b.WriteString(" @" + t.Requester)
 	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/common.go internal/tui/detail.go internal/tui/due_test.go internal/tui/inbox.go internal/tui/overlays.go internal/tui/pick.go internal/tui/tui_test.go
git commit -m 'feat(tui): due marker on rows, impact and due in Task detail, the Inbox shows the due first and waits for enter (D71, D73)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---


## After the last task

- Build both binaries into the scratchpad: the current `main` as `tt-old` and the branch as `tt-new`. Run `docs/superpowers/tooling/pty_impact.py <tt-old> <tt-new> <db>` (copy it into the scratchpad first; `<db>` a new path there). It adds three tasks and a pinned review with `tt-old`, then opens the database with `tt-new` under a pty. That checks the Inbox shows `INBOX · 1 of 3` with `— · ^ to set` first, and that `⏎` is refused before triage. It sets `^` to 2026-10-30 and impact 3 with size s, waits for `next card`, then checks `⏎` moves on and the Log shows `BACKLOG · by urgency` and `Old top →30 oct`. Finally it prints the tasks: priority unchanged, impact 3 and due 2026-10-30 on the triaged one, impact 1 and due 2026-10-15 on the review. Every line prints `OK`.
- Run the new binary with an old `config.example.toml` (from `main`) as `TT_CONFIG`: `tt add 'x !2'` exits 1 with the `[[priorities]]` message.
- `go test -race ./internal/tui/ ./internal/app/`.
- Final whole-branch review, one fix wave, park residuals as spec carry-forwards.
- **The user tries the branch, on a copy of the database:** migration 006 makes the database version 6, which `main`'s tt refuses to open ("created by a newer tt"). So: `cp ~/Library/Application\ Support/tt/tt.db /tmp/tt-try.db`, remove `[[priorities]]` and `[github] priority` from a copy of the config, then `make build` in the worktree and `TT_DB=/tmp/tt-try.db TT_CONFIG=<copy> <worktree>/bin/tt`. Merge (fast-forward to `main`) only after the user says so; after the merge, the user edits the real config the same way, and the real database migrates on the next start.
