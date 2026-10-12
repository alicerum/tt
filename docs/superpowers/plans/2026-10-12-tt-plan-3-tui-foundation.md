# tt Plan 3 — TUI Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `tt` with no arguments opens a full-screen planner: Pick (backlog + today's candidates), Inbox triage cards, quick-add with `@`/`#` autocomplete, drop/delete prompts, undo, help — live-reloading when `tt add` writes from another shell and showing a new-day banner at the day boundary.

**Architecture:** `internal/tui` is a Bubble Tea v2 program that never changes state itself: every key that changes something calls one `app.Service` command in a background `tea.Cmd`, then reloads with `Service.Load` (data version + snapshot + today's plan in one read). Two timer loops run: a 1s data-version check (D13) and a 30s clock tick that calls `StartDay` (D19). Overlays (quick-add, prompts, confirm, help) take all keys while open.

**Tech Stack:** Go 1.27; `charm.land/bubbletea/v2` v2.1.0, `charm.land/bubbles/v2` v2.2.1 (textinput), `charm.land/lipgloss/v2` v2.0.6; `github.com/charmbracelet/x/ansi` (already a Bubble Tea dependency; used for width-aware truncation); existing SQLite/TOML/go-cmp.

**Spec:** `docs/superpowers/specs/2026-10-12-task-tracking-design.md` (Views: Pick, Inbox, Quick-add; D11a, D12d, D19, D23, D25, D26 priority grouping, D27; carry-forward sections from Plans 1 and 2).

## Roadmap (revised)

The original Plan 3 ("TUI: capture & grooming") is split in two so each lands as usable software:

| Plan | Delivers |
|------|----------|
| 1 ✓ | Core & capture (`tt add`) |
| 2 ✓ | Planning engine |
| **3 (this)** | TUI foundation: shell, live reload, new-day banner, quick-add + autocomplete, Inbox cards, Pick (priority-grouped backlog, candidates, expand parents, pin/unpin/done/drop/delete), undo, help |
| 4 | Pick power tools: filter DSL + fuzzy search (D8a, D26a/b), switchable grouping (D26), Task detail with add-child/reorder, `e` edit line, `$EDITOR` notes (D29), open link (D21), `tt add` from stdin |
| 5 | Calendar & Today/Block: `tt-cal` EventKit helper, `CalendarSource`, timeline, Block phase, commit plan, conflicts |
| 6 | GitHub reviews: source adapter, Reviews lane, pin-to-wrap, auto-close |

## Global Constraints

- New dependencies: exactly `charm.land/bubbletea/v2@v2.1.0`, `charm.land/bubbles/v2@v2.2.1`, `charm.land/lipgloss/v2@v2.0.6` (plus their transitive modules; `github.com/charmbracelet/x/ansi` may be imported directly). Nothing else.
- `internal/tui` never writes to the store; it only calls `app.Service`.
- Undo (spec rule): an undo that returns `ErrConflict` or `ErrNotFound` is dropped with the message `can't undo <label>: it changed since`; nothing changes. Hard delete is confirmed (`y/n`) and not undoable.
- Planning starts with triage (D12d): from the new-day banner, Enter opens the Inbox when it has cards, otherwise Pick.
- Children inherit priority (D6): the Inbox card of a child never sets priority; it explains where priority comes from.
- Lines never wrap: every rendered line is truncated to the terminal width.
- Every task ends green on: `gofmt -l .` (no output), `go vet ./...`, `go test ./...`.
- Every commit message ends with a blank line and `Co-Authored-By: Claude <noreply@anthropic.com>`.

## Review Focus

1. **`tt add` from another shell while the TUI is open** shows up within a second. Pinned in Task 2 `TestExternalWriteReloads`.
2. **TUI left open across 04:00**: one banner with the recap, rollover once, Enter goes to triage. Pinned in Task 2 `TestNewDayBannerStartsPlanning`.
3. **Undo after the task changed elsewhere** says so and changes nothing. Pinned in Task 2 `TestUndoThatConflictsSaysSo`.
4. **Narrow terminal**: lines are truncated with `…`, never wider than the terminal. Pinned in Task 2 `TestNarrowTerminalTruncatesInsteadOfWrapping`.
5. **Child task in the Inbox**: priority keys explain inheritance; size alone triages it. Pinned in Task 2 `TestInboxSkipAndChildPriority`.

## File Structure

```
internal/dsl/add.go, add_test.go      modify: "@." / "#," stay in the title
internal/app/planning.go              modify: Block's undo maps a missing block to ErrConflict
internal/app/day.go                   modify: State, Load (version + snapshot + plan in one read), buildPlan
internal/app/service.go               modify: Service.Now
internal/app/state_test.go            new
internal/tui/model.go                 new: Model, Options, messages, Update/View, loops, undo
internal/tui/common.go                new: styles, index, badge, header/banner/status/hints, helpers
internal/tui/pick.go                  new: Pick rows, keys, two-pane view
internal/tui/inbox.go                 new: Inbox cards, keys, view
internal/tui/overlays.go              new: quick-add (+autocomplete), prompt, confirm, help
internal/tui/harness_test.go          new: synchronous test harness
internal/tui/tui_test.go              new
cmd/tt/main.go, main_test.go          modify: `tt` opens the TUI; -h/--help
```

Changes to existing files are patches; apply each with `git apply` from the repo root. If the sandbox refuses a heredoc, save the patch text verbatim to a file and `git apply <file>`. If `git apply` fails, stop: an earlier step went wrong.

---

### Task 1: Core support for the TUI

**Files:**
- Modify: `internal/dsl/add.go`, `internal/dsl/add_test.go`, `internal/app/planning.go`, `internal/app/day.go`, `internal/app/service.go`
- Test: `internal/app/state_test.go`

**Interfaces:**
- Consumes: Plans 1–2 (`dsl.ParseAdd`, `Service.Block`, `Service.Plan`, `store.LoadPlan`, `store.PlanState`, `DataVersion`; test helpers `startedService`, `mustAdd`, `mustBlock`, `mustPlan`).
- Produces:
  - `type app.State struct{ Version int64; Snapshot domain.Snapshot; Plan domain.Plan }` and `(*Service) Load(ctx) (State, error)`: reads `DataVersion` **first**, then tasks and today's plan in one `LoadPlan` read (carry-forward "watch order" and "one read for Pick"). `Plan()` now delegates to `Load`.
  - `(*Service) Now() time.Time`.
  - `Block`'s undo returns `ErrConflict` (not `ErrNotFound`) when the block is already gone.
  - `dsl.ParseAdd`: a sigil token that is only punctuation after stripping (`@.`, `#,`, `~?`, `!)`) stays in the title.

- [ ] **Step 1: Write the failing tests**

`internal/app/state_test.go`:

```go
package app

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestLoadReadsVersionSnapshotAndPlan(t *testing.T) {
	s, _ := startedService(t)
	ctx := context.Background()
	a := mustAdd(t, s, "A !1 ~s")
	mustBlock(t, s, a.ID, 10*h, h)

	st, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Snapshot.Get(a.ID); !ok {
		t.Error("snapshot misses the task")
	}
	if diff := cmp.Diff(mustPlan(t, s), st.Plan); diff != "" {
		t.Errorf("Load's plan differs from Plan() (-Plan +Load):\n%s", diff)
	}

	mustAdd(t, s, "B")
	again, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again.Version == st.Version {
		t.Error("Version did not change after a write")
	}
	if len(again.Snapshot.Tasks()) != 2 {
		t.Errorf("snapshot has %d tasks; want 2", len(again.Snapshot.Tasks()))
	}
}

func TestBlockUndoAfterUnblockConflicts(t *testing.T) {
	s, _ := startedService(t)
	ctx := context.Background()
	a := mustAdd(t, s, "A !1 ~s")
	b, blockUndo := mustBlock(t, s, a.ID, 10*h, h)
	if _, err := s.Unblock(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := blockUndo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo Block after the block is gone: %v; want ErrConflict", err)
	}
}
```

and add the parser case:

```bash
git apply <<'PATCH'
diff --git a/internal/dsl/add_test.go b/internal/dsl/add_test.go
index 0f58c77..0a88118 100644
--- a/internal/dsl/add_test.go
+++ b/internal/dsl/add_test.go
@@ -30,6 +30,7 @@ func TestParseAdd(t *testing.T) {
 		{"clean ~/tmp and ~zz", Draft{Title: "clean ~/tmp and ~zz"}},
 		{"Fix this! now !!", Draft{Title: "Fix this! now !!"}},
 		{"lone @ # ! ~ sigils", Draft{Title: "lone @ # ! ~ sigils"}},
+		{"sigil plus punctuation @. #, ~? !)", Draft{Title: "sigil plus punctuation @. #, ~? !)"}},
 		{"mail bob@x.com about #2", Draft{Title: "mail bob@x.com about #2"}},
 		{"two https://a.io links https://b.io", Draft{Title: "two links https://b.io", Link: "https://a.io"}},
 		{"🚀 launch #Ünïcode", Draft{Title: "🚀 launch", Tags: []string{"ünïcode"}}},
PATCH
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/ ./internal/dsl/`
Expected: FAIL: `s.Load undefined` (build), and the new `TestParseAdd` case fails with requester `"."`.

- [ ] **Step 3: Apply the changes**

```bash
git apply <<'PATCH'
diff --git a/internal/app/day.go b/internal/app/day.go
index d5546ce..28da0b8 100644
--- a/internal/app/day.go
+++ b/internal/app/day.go
@@ -127,11 +127,34 @@ func (s *Service) CommitPlan(ctx context.Context) (CommitSummary, error) {
 // Plan returns today's candidates and blocks. Closed tasks stay among the
 // candidates until the next planning day, so the day's wins remain visible.
 func (s *Service) Plan(ctx context.Context) (domain.Plan, error) {
+	st, err := s.Load(ctx)
+	return st.Plan, err
+}
+
+// State is everything a view needs, read consistently.
+type State struct {
+	Version  int64 // DataVersion read before loading; compare to detect later writes
+	Snapshot domain.Snapshot
+	Plan     domain.Plan
+}
+
+// Load reads the data version first, then all tasks and today's plan in one
+// read. A write that lands during the load changes the version, so the next
+// version check reloads again.
+func (s *Service) Load(ctx context.Context) (State, error) {
+	version, err := s.store.DataVersion(ctx)
+	if err != nil {
+		return State{}, err
+	}
 	today := s.Today()
 	snap, state, err := s.store.LoadPlan(ctx, today)
 	if err != nil {
-		return domain.Plan{}, err
+		return State{}, err
 	}
+	return State{Version: version, Snapshot: snap, Plan: s.buildPlan(today, snap, state)}, nil
+}
+
+func (s *Service) buildPlan(today domain.Day, snap domain.Snapshot, state store.PlanState) domain.Plan {
 	blocked := map[int64]bool{}
 	for _, b := range state.Blocks {
 		blocked[b.TaskID] = true
@@ -154,7 +177,7 @@ func (s *Service) Plan(ctx context.Context) (domain.Plan, error) {
 		pb := domain.EffectivePriority(b.Task, snap.ParentOf(b.Task))
 		return cmp.Or(cmp.Compare(unsetLast(pa), unsetLast(pb)), cmp.Compare(a.Task.ID, b.Task.ID))
 	})
-	return plan, nil
+	return plan
 }
 
 // unsetLast sorts priority 0 (unset) after every real level.
diff --git a/internal/app/planning.go b/internal/app/planning.go
index e094eac..475dd03 100644
--- a/internal/app/planning.go
+++ b/internal/app/planning.go
@@ -158,7 +158,9 @@ func (s *Service) Block(ctx context.Context, id int64, start, dur time.Duration)
 			return err
 		}
 		return s.store.InTx(ctx, func(x *store.Tx) error {
-			if err := x.DeleteBlock(ctx, b.ID); err != nil {
+			if err := x.DeleteBlock(ctx, b.ID); errors.Is(err, store.ErrNotFound) {
+				return fmt.Errorf("block #%d is gone: %w", b.ID, ErrConflict)
+			} else if err != nil {
 				return err
 			}
 			if had {
diff --git a/internal/app/service.go b/internal/app/service.go
index a474843..1ed190e 100644
--- a/internal/app/service.go
+++ b/internal/app/service.go
@@ -43,6 +43,9 @@ func New(st Store, clock Clock, cfg config.Config) *Service {
 
 func (s *Service) Config() config.Config { return s.cfg }
 
+// Now is the service clock's current time (for "captured 2d ago" and similar).
+func (s *Service) Now() time.Time { return s.clock.Now() }
+
 func (s *Service) Snapshot(ctx context.Context) (domain.Snapshot, error) {
 	return s.store.LoadSnapshot(ctx)
 }
diff --git a/internal/dsl/add.go b/internal/dsl/add.go
index 7f723f8..d69be20 100644
--- a/internal/dsl/add.go
+++ b/internal/dsl/add.go
@@ -54,13 +54,11 @@ func ParseAdd(line string, levels int) (Draft, error) {
 		if strings.ContainsRune("@#!~", rune(tok[0])) {
 			stripped := strings.TrimRight(tok, trailingPunct)
 			// "#482." stays whole (issue ref); "@." has nothing left to classify.
-			if len(stripped) > 1 {
-				if stripped[0] == '#' && isDigits(stripped[1:]) {
-					title = append(title, orig)
-					continue
-				}
-				tok = stripped
+			if len(stripped) <= 1 || (stripped[0] == '#' && isDigits(stripped[1:])) {
+				title = append(title, orig)
+				continue
 			}
+			tok = stripped
 		}
 		switch sigil, rest := tok[0], tok[1:]; {
 		case sigil == '@' && rest != "":
PATCH
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: no gofmt output; every package `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/dsl internal/app
git commit -m "feat(app): Load state for views; parser and undo edge fixes" -m "Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 2: The TUI package

**Files:**
- Create: `internal/tui/model.go`, `internal/tui/common.go`, `internal/tui/pick.go`, `internal/tui/inbox.go`, `internal/tui/overlays.go`
- Test: `internal/tui/harness_test.go`, `internal/tui/tui_test.go`

**Interfaces:**
- Consumes: Task 1 (`Service.Load`, `app.State`, `Service.Now`), Plans 1–2 (`Service.Add`, `Pin`, `Unpin`, `Done`, `Reopen`, `Drop`, `Delete`, `SetPriority`, `SetSize`, `StartDay`, `DayOf`, `DataVersion`, `Config`; `app.ErrConflict`, `app.ErrNotFound`, `app.DayResult`, `app.Undo`; `domain` types; `config.Default`; `store.Open`).
- Produces:
  - `type tui.Options struct{ WatchEvery, ClockEvery time.Duration; Schedule func(time.Duration, tea.Msg) tea.Cmd }`, `func tui.DefaultOptions() Options` (1s, 30s, `tea.Tick`).
  - `func tui.New(ctx, *app.Service, Options) *Model`; `*Model` implements `tea.Model` (`Init`, `Update`, `View` with `AltScreen`).

Behavior (see the spec's Views section; this plan implements Pick without filter/grouping switch and the Inbox without tag/requester/parent editing — those are Plan 4):

| Where | Keys |
|-------|------|
| everywhere | `a` quick-add (tab completes a trailing `@`/`#` token, most-used first) · `ctrl+z` undo · `R` reload · `?` help · `q`/`ctrl+c` quit |
| new-day banner | `enter` plan now (Inbox if it has cards, else Pick) · `esc` later |
| Pick | `tab` switch pane · `j`/`k`/arrows move · `→`/`←` expand/collapse a backlog parent · `p` pin · `u` unpin · `x` done / reopen · `d` drop (reason prompt) · `D` delete (y/n) · `i` Inbox |
| Inbox | `1`–`N` priority · `s`/`m`/`l`/`x` size · `n` skip (moves to the end, stays in the Inbox) · `d` drop · `D` delete · `esc` back |

- Backlog: open, triaged, unpinned top-level tasks grouped by priority (header `P1 · <definition>`), oldest first; expanded parents list open, triaged, unpinned children indented. Priority above the configured levels groups under `P?`.
- Today's candidates: plan order; a pinned parent lists its open children and children closed today (via `DayOf`); a pinned child of a pinned parent appears only under the parent. Markers: `▓` blocked, `↻N` rollover, `📌N` pinned days > 1, `✓`/`✗` closed (struck through), and a `⚠ rolled over N× → refine · split · reprioritize · hand off · drop` line when `Alert`.
- The first `StartDay` of a day (at launch or on the clock tick) shows the banner `☀ New day <day> · since <prev>: N done · N rolled over · N carried`.

- [ ] **Step 1: Add the dependencies**

```bash
go get charm.land/bubbletea/v2@v2.1.0 charm.land/bubbles/v2@v2.2.1 charm.land/lipgloss/v2@v2.0.6
```

- [ ] **Step 2: Write the failing tests**

`internal/tui/harness_test.go`:

```go
package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/app"
	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/store"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

// harness drives a Model synchronously: every command a key or message
// returns is executed and its message fed back, until nothing is left.
// Timers are disabled (Schedule returns nil).
type harness struct {
	t     *testing.T
	m     *Model
	svc   *app.Service
	clock *fakeClock
	path  string
	quit  bool
}

func newHarness(t *testing.T, cfg config.Config) *harness {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tt.db")
	st, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	clock := &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
	svc := app.New(st, clock, cfg)
	opts := Options{WatchEvery: time.Second, ClockEvery: 30 * time.Second,
		Schedule: func(time.Duration, tea.Msg) tea.Cmd { return nil }}
	h := &harness{t: t, m: New(context.Background(), svc, opts), svc: svc, clock: clock, path: path}
	h.run(h.m.Init())
	h.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	if h.m.banner == nil {
		t.Fatal("the first launch of a day shows the new-day banner")
	}
	h.keys("esc") // tests start with the banner dismissed
	return h
}

func (h *harness) send(msg tea.Msg) {
	h.t.Helper()
	_, cmd := h.m.Update(msg)
	h.run(cmd)
}

func (h *harness) run(cmd tea.Cmd) {
	h.t.Helper()
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
	case tea.QuitMsg:
		h.quit = true
	default:
		h.send(msg)
	}
}

var named = map[string]tea.KeyPressMsg{
	"enter": {Code: tea.KeyEnter}, "tab": {Code: tea.KeyTab}, "esc": {Code: tea.KeyEscape},
	"up": {Code: tea.KeyUp}, "down": {Code: tea.KeyDown}, "left": {Code: tea.KeyLeft}, "right": {Code: tea.KeyRight},
	"ctrl+z": {Code: 'z', Mod: tea.ModCtrl}, "ctrl+c": {Code: 'c', Mod: tea.ModCtrl},
}

// keys sends named keys ("enter", "ctrl+z") or single characters.
func (h *harness) keys(keys ...string) {
	h.t.Helper()
	for _, k := range keys {
		msg, ok := named[k]
		if !ok {
			r := []rune(k)
			if len(r) != 1 {
				h.t.Fatalf("unknown key %q", k)
			}
			msg = tea.KeyPressMsg{Code: r[0], Text: k}
		}
		h.send(msg)
	}
}

// typeText types s one character at a time.
func (h *harness) typeText(s string) {
	h.t.Helper()
	for _, r := range s {
		h.send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func (h *harness) screen() string { return ansi.Strip(h.m.render()) }

func (h *harness) mustSee(parts ...string) {
	h.t.Helper()
	s := h.screen()
	for _, p := range parts {
		if !strings.Contains(s, p) {
			h.t.Fatalf("screen lacks %q:\n%s", p, s)
		}
	}
}

func (h *harness) mustNotSee(parts ...string) {
	h.t.Helper()
	s := h.screen()
	for _, p := range parts {
		if strings.Contains(s, p) {
			h.t.Fatalf("screen unexpectedly shows %q:\n%s", p, s)
		}
	}
}

func (h *harness) add(line string) domain.Task {
	h.t.Helper()
	t, _, err := h.svc.Add(context.Background(), line)
	if err != nil {
		h.t.Fatal(err)
	}
	h.keys("R")
	return t
}

func (h *harness) task(id int64) domain.Task {
	h.t.Helper()
	snap, err := h.svc.Snapshot(context.Background())
	if err != nil {
		h.t.Fatal(err)
	}
	t, ok := snap.Get(id)
	if !ok {
		h.t.Fatalf("task #%d missing", id)
	}
	return t
}
```

`internal/tui/tui_test.go`:

```go
package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/app"
	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/store"
)

func TestQuickAddThenTriageInTwoKeystrokes(t *testing.T) {
	h := newHarness(t, config.Default())
	h.mustSee("tt · Pick", "Mon 12 Oct", "nothing triaged yet")

	h.keys("a")
	h.typeText("Reply to Ana @ana #photos")
	h.keys("enter")
	h.mustSee("added #1 to inbox", "inbox 1")

	h.keys("i")
	h.mustSee("INBOX · 1 of 1", "Reply to Ana", "@ana · #photos", "[1] P1 · someone is blocked")
	h.keys("1", "s") // D11a: priority + size → leaves the inbox
	h.mustSee("Inbox zero ✓")
	h.keys("esc")
	h.mustSee("P1 · someone is blocked", "[P1 S] Reply to Ana @ana")
	if got := h.task(1); got.Priority != 1 || got.Size != domain.SizeS {
		t.Errorf("task after triage: %+v", got)
	}
}

func TestQuickAddDirectlyToBacklogAndEscCancels(t *testing.T) {
	h := newHarness(t, config.Default())
	h.keys("a")
	h.typeText("Ready !2 ~m")
	h.keys("enter")
	h.mustSee("added #1 to backlog", "[P2 M] Ready")

	h.keys("a")
	h.typeText("never mind")
	h.keys("esc")
	h.mustNotSee("never mind")
	if snap, _ := h.svc.Snapshot(context.Background()); len(snap.Tasks()) != 1 {
		t.Errorf("esc must not add: %d tasks", len(snap.Tasks()))
	}

	h.keys("a")
	h.typeText("@only-tokens")
	h.keys("enter")
	h.mustSee("✗ title is required")
}

func TestQuickAddAutocomplete(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("one @ana #photos")
	h.add("two @ana")
	h.add("three @andrei")

	h.keys("a")
	h.typeText("x @an")
	h.mustSee("tab → @ana  @andrei") // most used first
	h.keys("tab")
	h.typeText("#ph")
	h.keys("tab")
	h.mustSee("add › x @ana #photos")
	h.keys("enter")
	if got := h.task(4); got.Requester != "ana" || len(got.Tags) != 1 || got.Tags[0] != "photos" {
		t.Errorf("autocompleted task: %+v", got)
	}
}

func TestPickPinDoneUndo(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.add("Alpha !1 ~s")
	h.add("Beta !2 ~m")
	h.mustSee("[P1 S] Alpha", "[P2 M] Beta", "nothing pinned")

	h.keys("p") // cursor starts on the first backlog task
	h.mustSee(`pinned "Alpha"`)
	if _, ok := h.m.idx.cand[a.ID]; !ok {
		t.Fatal("Alpha not pinned")
	}
	h.keys("tab", "x")
	h.mustSee(`✓ "Alpha"`, "Alpha ✓")
	h.keys("ctrl+z")
	h.mustSee(`undid done "Alpha"`)
	if got := h.task(a.ID); got.Status != domain.StatusOpen {
		t.Errorf("after undo: %s", got.Status)
	}
	h.keys("u")
	h.mustSee(`unpinned "Alpha"`, "nothing pinned")
	h.keys("ctrl+z", "ctrl+z")
	h.mustSee(`undid pin "Alpha"`)
	h.keys("ctrl+z")
	h.mustSee("nothing to undo")
}

func TestUndoThatConflictsSaysSo(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.add("Alpha !1 ~s")
	h.keys("p", "tab", "x")
	h.clock.now = h.clock.now.Add(time.Minute)
	if _, err := h.svc.SetNote(context.Background(), a.ID, "changed elsewhere"); err != nil {
		t.Fatal(err)
	}
	h.keys("ctrl+z")
	h.mustSee(`✗ can't undo done "Alpha": it changed since`)
	if got := h.task(a.ID); got.Status != domain.StatusDone {
		t.Errorf("refused undo changed the task: %s", got.Status)
	}
}

func TestDropWithReasonAndDeleteConfirm(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.add("Alpha !1 ~s")
	b := h.add("Beta !1 ~s")

	h.keys("d")
	h.typeText("handed to @marko")
	h.keys("enter")
	h.mustSee(`dropped "Alpha"`)
	if got := h.task(a.ID); got.Status != domain.StatusDropped || got.DropReason != "handed to @marko" {
		t.Errorf("dropped task: %+v", got)
	}

	h.keys("D")
	h.mustSee(`Delete #2 "Beta"? This can't be undone. (y/n)`)
	h.keys("n")
	h.mustSee("cancelled")
	h.task(b.ID) // still there
	h.keys("D", "y")
	h.mustSee("deleted #2")
	if snap, _ := h.svc.Snapshot(context.Background()); len(snap.Tasks()) != 1 {
		t.Errorf("after delete: %d tasks", len(snap.Tasks()))
	}
}

func TestBacklogParentExpandsAndTodayShowsChildren(t *testing.T) {
	cfg := config.Default()
	cfg.RolloverAlert = 1
	h := newHarness(t, cfg)
	ctx := context.Background()
	parent := h.add("Auth migration !1 ~xl")
	kid1 := h.add("Rollback plan ~s")
	kid2 := h.add("Token spike ~m")
	for _, k := range []int64{kid1.ID, kid2.ID} {
		if _, err := h.svc.SetParent(ctx, k, parent.ID); err != nil {
			t.Fatal(err)
		}
	}
	h.keys("R")
	h.mustSee("▸ [P1 XL] Auth migration 0/2")
	h.mustNotSee("Rollback plan")
	h.keys("right")
	h.mustSee("▾ [P1 XL] Auth migration", "    [P1 S] Rollback plan", "    [P1 M] Token spike")
	h.keys("down", "left") // collapsing from a child collapses its parent
	h.mustNotSee("Rollback plan")

	if _, _, err := h.svc.Block(ctx, parent.ID, 10*time.Hour, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Done(ctx, kid1.ID); err != nil {
		t.Fatal(err)
	}
	h.clock.now = h.clock.now.Add(24 * time.Hour) // parent rolls over once → alert at threshold 1
	h.send(clockTickMsg{})
	h.keys("esc") // dismiss the banner
	h.mustSee("▾ [P1 XL] Auth migration 1/2 ↻1", "⚠ rolled over 1×", "    [P1 M] Token spike")
	h.mustNotSee("Rollback plan") // closed yesterday, not today
}

func TestExternalWriteReloads(t *testing.T) {
	h := newHarness(t, config.Default())
	other, err := store.Open(context.Background(), h.path) // e.g. `tt add` from another shell
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	cli := app.New(other, h.clock, config.Default())
	if _, _, err := cli.Add(context.Background(), "From the CLI"); err != nil {
		t.Fatal(err)
	}
	h.mustNotSee("inbox 1")
	h.send(watchTickMsg{})
	h.mustSee("inbox 1")
}

func TestNewDayBannerStartsPlanning(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.add("Blocked !1 ~s")
	if _, _, err := h.svc.Block(context.Background(), a.ID, 10*time.Hour, time.Hour); err != nil {
		t.Fatal(err)
	}
	h.clock.now = h.clock.now.Add(24 * time.Hour)
	h.send(clockTickMsg{})
	h.mustSee("☀ New day Tue 13 Oct · since Mon 12 Oct: 0 done · 1 rolled over · 1 carried", "⏎ plan now")
	h.send(clockTickMsg{}) // same day again: no new banner, nothing changes
	h.keys("enter")
	h.mustNotSee("New day")
	h.mustSee("tt · Pick") // empty inbox: straight to Pick

	h.add("Untriaged")
	h.clock.now = h.clock.now.Add(24 * time.Hour)
	h.send(clockTickMsg{})
	h.keys("enter")
	h.mustSee("tt · Inbox", "INBOX · 1 of 1") // D12d: planning starts with triage
}

func TestInboxSkipAndChildPriority(t *testing.T) {
	h := newHarness(t, config.Default())
	parent := h.add("Parent !1 ~xl")
	child := h.add("Child")
	h.add("Other")
	if _, err := h.svc.SetParent(context.Background(), child.ID, parent.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "i")
	h.mustSee("INBOX · 1 of 2", "Child", "from parent: [P1 XL]")
	h.keys("2")
	h.mustSee(`priority comes from parent #1 "Parent"`)
	h.keys("n")
	h.mustSee("INBOX · 1 of 2", "Other") // the skipped card moved to the end
	h.keys("n")
	h.mustSee("Child") // everything skipped: start over
	h.keys("m")        // size alone triages a child of a prioritized parent
	h.mustSee("INBOX · 1 of 1", "Other")
	h.keys("9")
	h.mustSee("there are 3 priority levels")
}

func TestHelpAndQuit(t *testing.T) {
	h := newHarness(t, config.Default())
	h.keys("?")
	h.mustSee("KEYS", "p / u    pin to today / unpin", "any key closes")
	h.keys("x")
	h.mustNotSee("KEYS")
	h.keys("q")
	if !h.quit {
		t.Error("q should quit")
	}
}

func TestNarrowTerminalTruncatesInsteadOfWrapping(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("A rather long task title that will not fit in a narrow terminal @someone !1 ~s")
	h.keys("p")
	h.send(tea.WindowSizeMsg{Width: 40, Height: 12})
	for _, line := range strings.Split(h.m.render(), "\n") {
		if w := ansi.StringWidth(line); w > 40 {
			t.Errorf("line is %d cells wide (> 40): %q", w, ansi.Strip(line))
		}
	}
	h.mustSee("…")
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL to build: `undefined: Options`, `undefined: New`, …

- [ ] **Step 4: Implement the model**

`internal/tui/model.go`:

```go
// Package tui is tt's terminal UI (Bubble Tea). It never changes state itself:
// every change goes through app.Service, and the view reloads afterwards.
package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/app"
)

// Options tunes the background loops.
type Options struct {
	WatchEvery time.Duration // how often to check for writes from other processes (D13)
	ClockEvery time.Duration // how often to check for a new planning day (D19)
	// Schedule returns a command that delivers msg after d. Tests replace it
	// so Update round-trips never wait on timers.
	Schedule func(d time.Duration, msg tea.Msg) tea.Cmd
}

func DefaultOptions() Options {
	return Options{
		WatchEvery: time.Second,
		ClockEvery: 30 * time.Second,
		Schedule: func(d time.Duration, msg tea.Msg) tea.Cmd {
			return tea.Tick(d, func(time.Time) tea.Msg { return msg })
		},
	}
}

type screen int

const (
	screenPick screen = iota
	screenInbox
)

const maxUndo = 50

type undoEntry struct {
	label string
	fn    app.Undo
}

// overlay is a modal input (quick-add, prompts, help) that takes all keys.
type overlay interface {
	update(m *Model, k tea.KeyPressMsg) tea.Cmd
	view(m *Model) string
}

type Model struct {
	svc  *app.Service
	opts Options
	ctx  context.Context

	state         app.State
	idx           index
	loaded        bool
	width, height int

	screen screen
	pick   pickState
	inbox  inboxState

	overlay     overlay
	banner      *app.DayResult
	undo        []undoEntry
	status      string
	statusIsErr bool
}

func New(ctx context.Context, svc *app.Service, opts Options) *Model {
	return &Model{svc: svc, opts: opts, ctx: ctx, width: 100, height: 30}
}

// Messages.
type (
	loadedMsg struct {
		state app.State
		err   error
	}
	watchTickMsg struct{}
	versionMsg   struct {
		version int64
		err     error
	}
	clockTickMsg  struct{}
	dayStartedMsg struct {
		res app.DayResult
		err error
	}
	actionMsg struct {
		label, status string
		undo          app.Undo
		err           error
	}
	undoneMsg struct {
		label string
		err   error
	}
)

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.startDay(),
		m.opts.Schedule(m.opts.WatchEvery, watchTickMsg{}),
		m.opts.Schedule(m.opts.ClockEvery, clockTickMsg{}),
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case loadedMsg:
		if msg.err != nil {
			m.setErr(msg.err)
			return m, nil
		}
		m.state, m.loaded = msg.state, true
		m.idx = buildIndex(msg.state)
		m.pick.clamp(m)
		m.inbox.sync(m)
	case watchTickMsg:
		return m, tea.Batch(m.checkVersion(), m.opts.Schedule(m.opts.WatchEvery, watchTickMsg{}))
	case versionMsg:
		if msg.err == nil && msg.version != m.state.Version {
			return m, m.load()
		}
	case clockTickMsg:
		return m, tea.Batch(m.startDay(), m.opts.Schedule(m.opts.ClockEvery, clockTickMsg{}))
	case dayStartedMsg:
		if msg.err != nil {
			m.setErr(msg.err)
			return m, m.load()
		}
		if msg.res.New {
			res := msg.res
			m.banner = &res
		}
		return m, m.load()
	case actionMsg:
		if msg.err != nil {
			m.setErr(msg.err)
			return m, m.load()
		}
		if msg.undo != nil {
			m.undo = append(m.undo, undoEntry{label: msg.label, fn: msg.undo})
			if len(m.undo) > maxUndo {
				m.undo = m.undo[1:]
			}
		}
		m.setStatus(msg.status)
		return m, m.load()
	case undoneMsg:
		switch {
		case errors.Is(msg.err, app.ErrConflict) || errors.Is(msg.err, app.ErrNotFound):
			m.setErr(fmt.Errorf("can't undo %s: it changed since", msg.label))
		case msg.err != nil:
			m.setErr(msg.err)
		default:
			m.setStatus("undid " + msg.label)
		}
		return m, m.load()
	case tea.KeyPressMsg:
		return m, m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if key == "ctrl+c" {
		return tea.Quit
	}
	if m.overlay != nil {
		return m.overlay.update(m, k)
	}
	if !m.loaded {
		return nil
	}
	if m.banner != nil {
		switch key {
		case "enter":
			m.banner = nil
			m.screen = screenPick
			if len(m.state.Snapshot.Inbox()) > 0 {
				m.screen = screenInbox // planning starts with triage (D12d)
			}
			return nil
		case "esc":
			m.banner = nil
			return nil
		}
	}
	m.status, m.statusIsErr = "", false
	switch key {
	case "q":
		return tea.Quit
	case "?":
		m.overlay = helpOverlay{}
		return nil
	case "a":
		return m.openQuickAdd()
	case "ctrl+z":
		return m.undoLast()
	case "R":
		return m.load()
	}
	if m.screen == screenInbox {
		return m.inboxKey(key)
	}
	return m.pickKey(key)
}

func (m *Model) load() tea.Cmd {
	svc, ctx := m.svc, m.ctx
	return func() tea.Msg {
		st, err := svc.Load(ctx)
		return loadedMsg{state: st, err: err}
	}
}

func (m *Model) checkVersion() tea.Cmd {
	svc, ctx := m.svc, m.ctx
	return func() tea.Msg {
		v, err := svc.DataVersion(ctx)
		return versionMsg{version: v, err: err}
	}
}

func (m *Model) startDay() tea.Cmd {
	svc, ctx := m.svc, m.ctx
	return func() tea.Msg {
		res, err := svc.StartDay(ctx)
		return dayStartedMsg{res: res, err: err}
	}
}

// act runs a service command in the background and reports it as actionMsg.
func (m *Model) act(label, status string, fn func(context.Context) (app.Undo, error)) tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		undo, err := fn(ctx)
		return actionMsg{label: label, status: status, undo: undo, err: err}
	}
}

func (m *Model) undoLast() tea.Cmd {
	if len(m.undo) == 0 {
		m.setStatus("nothing to undo")
		return nil
	}
	e := m.undo[len(m.undo)-1]
	m.undo = m.undo[:len(m.undo)-1]
	ctx := m.ctx
	return func() tea.Msg { return undoneMsg{label: e.label, err: e.fn(ctx)} }
}

func (m *Model) setStatus(s string) { m.status, m.statusIsErr = s, false }
func (m *Model) setErr(err error)   { m.status, m.statusIsErr = err.Error(), true }

func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "tt"
	return v
}

func (m *Model) render() string {
	if !m.loaded {
		return "loading…"
	}
	var parts []string
	parts = append(parts, m.headerView())
	if m.banner != nil {
		parts = append(parts, m.bannerView())
	}
	if m.screen == screenInbox {
		parts = append(parts, m.inboxView())
	} else {
		parts = append(parts, m.pickView())
	}
	if m.overlay != nil {
		parts = append(parts, m.overlay.view(m))
	}
	parts = append(parts, m.statusView(), m.hintsView())
	return fitWidth(strings.Join(parts, "\n"), m.width)
}
```

`internal/tui/common.go`:

```go
package tui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/app"
	"tt/internal/domain"
)

var (
	styleTitle    = lipgloss.NewStyle().Bold(true)
	styleDim      = lipgloss.NewStyle().Faint(true)
	styleSelected = lipgloss.NewStyle().Reverse(true)
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleAlert    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleClosed   = lipgloss.NewStyle().Strikethrough(true).Faint(true)
	styleBanner   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
)

// index holds lookups derived from one loaded state.
type index struct {
	children   map[int64][]domain.Task // by parent id, in Position order
	cand       map[int64]domain.Candidate
	inInbox    map[int64]bool
	requesters []string // most used first
	tags       []string // most used first
}

func buildIndex(st app.State) index {
	idx := index{children: map[int64][]domain.Task{}, cand: map[int64]domain.Candidate{}, inInbox: map[int64]bool{}}
	reqCount, tagCount := map[string]int{}, map[string]int{}
	for _, t := range st.Snapshot.Tasks() {
		if t.ParentID != 0 {
			idx.children[t.ParentID] = append(idx.children[t.ParentID], t)
		}
		if t.Requester != "" {
			reqCount[t.Requester]++
		}
		for _, tag := range t.Tags {
			tagCount[tag]++
		}
	}
	for _, kids := range idx.children {
		slices.SortFunc(kids, func(a, b domain.Task) int {
			return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID))
		})
	}
	for _, c := range st.Plan.Candidates {
		idx.cand[c.Task.ID] = c
	}
	for _, t := range st.Snapshot.Inbox() {
		idx.inInbox[t.ID] = true
	}
	idx.requesters, idx.tags = byUse(reqCount), byUse(tagCount)
	return idx
}

func byUse(counts map[string]int) []string {
	out := make([]string, 0, len(counts))
	for k := range counts {
		out = append(out, k)
	}
	slices.SortFunc(out, func(a, b string) int { return cmp.Or(cmp.Compare(counts[b], counts[a]), cmp.Compare(a, b)) })
	return out
}

// badge renders "[P1 M]": the effective priority's configured name and the size.
func (m *Model) badge(t domain.Task) string {
	p := domain.EffectivePriority(t, m.state.Snapshot.ParentOf(t))
	name := "—"
	if levels := m.svc.Config().Priorities; p > 0 && p <= len(levels) {
		name = levels[p-1].Name
	} else if p > 0 {
		name = "P?"
	}
	size := "—"
	if t.Size != domain.SizeNone {
		size = strings.ToUpper(string(t.Size))
	}
	return "[" + name + " " + size + "]"
}

// closedToday reports whether t was closed on the plan's day (D29 wins stay visible).
func (m *Model) closedToday(t domain.Task) bool {
	return t.Status != domain.StatusOpen && !t.ClosedAt.IsZero() && m.svc.DayOf(t.ClosedAt) == m.state.Plan.Day
}

func (m *Model) headerView() string {
	name := "Pick"
	if m.screen == screenInbox {
		name = "Inbox"
	}
	h := styleTitle.Render("tt · "+name) + " · " + dayLabel(m.state.Plan.Day)
	if n := len(m.idx.inInbox); n > 0 {
		h += styleDim.Render(fmt.Sprintf(" · inbox %d", n))
	}
	if m.state.Plan.Committed {
		h += styleDim.Render(" · plan committed")
	}
	return h
}

func (m *Model) bannerView() string {
	b := m.banner
	text := "☀ New day " + dayLabel(b.Day)
	if r := b.Recap; r != nil {
		text += fmt.Sprintf(" · since %s: %d done · %d rolled over · %d carried", dayLabel(r.Since), r.Done, r.RolledOver, r.Carried)
	}
	return styleBanner.Render(text) + styleDim.Render(" · ⏎ plan now · esc later")
}

func (m *Model) statusView() string {
	if m.status == "" {
		return ""
	}
	if m.statusIsErr {
		return styleError.Render("✗ " + m.status)
	}
	return styleDim.Render(m.status)
}

func (m *Model) hintsView() string {
	if m.screen == screenInbox {
		return styleDim.Render(fmt.Sprintf("1-%d priority · s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help", len(m.svc.Config().Priorities)))
	}
	return styleDim.Render("a add · p pin · u unpin · x done · d drop · tab pane · →/← expand · i inbox · ctrl+z undo · ? help · q quit")
}

func dayLabel(d domain.Day) string {
	t, err := time.Parse(time.DateOnly, string(d))
	if err != nil {
		return string(d)
	}
	return t.Format("Mon 2 Jan")
}

// ago renders a coarse relative time: "just now", "5m ago", "3h ago", "2d ago".
func ago(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	}
}

// shortDur renders 1h, 4h, 30m, 1h30m.
func shortDur(d time.Duration) string {
	s := d.String()
	s = strings.TrimSuffix(s, "0s")
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// pad truncates or right-pads s (which may contain ANSI styles) to width cells.
func pad(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if w := ansi.StringWidth(s); w > width {
		return ansi.Truncate(s, width, "…")
	} else {
		return s + strings.Repeat(" ", width-w)
	}
}

// fitWidth truncates every line of s to width cells.
func fitWidth(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if ansi.StringWidth(l) > width {
			lines[i] = ansi.Truncate(l, width, "…")
		}
	}
	return strings.Join(lines, "\n")
}
```

- [ ] **Step 5: Implement Pick**

`internal/tui/pick.go`:

```go
package tui

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"tt/internal/app"
	"tt/internal/domain"
)

type pane int

const (
	paneBacklog pane = iota
	paneToday
)

type rowKind int

const (
	rowHeader rowKind = iota
	rowTask
	rowHint
)

type row struct {
	kind   rowKind
	text   string // header or hint text
	task   domain.Task
	child  bool              // indented under its parent
	cand   *domain.Candidate // set when the task is pinned
	parent bool              // has children
}

type pickState struct {
	focus    pane
	cursor   [2]int
	expanded map[int64]bool
}

// backlogRows: triaged open top-level tasks that aren't pinned, grouped by
// priority (oldest first within a group, D26). Expanded parents list their
// open, triaged, unpinned children.
func (m *Model) backlogRows() []row {
	snap := m.state.Snapshot
	levels := m.svc.Config().Priorities
	groups := map[int][]domain.Task{}
	for _, t := range snap.Tasks() {
		if t.Status != domain.StatusOpen || t.ParentID != 0 || m.idx.inInbox[t.ID] {
			continue
		}
		if _, pinned := m.idx.cand[t.ID]; pinned {
			continue
		}
		p := t.Priority
		if p > len(levels) {
			p = len(levels) + 1 // shrunk config: "P?" group at the end
		}
		groups[p] = append(groups[p], t)
	}
	var rows []row
	for p := 1; p <= len(levels)+1; p++ {
		items := groups[p]
		if len(items) == 0 {
			continue
		}
		slices.SortFunc(items, func(a, b domain.Task) int { return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID)) })
		header := "P? · above the configured levels"
		if p <= len(levels) {
			header = levels[p-1].Name + " · " + levels[p-1].Definition
		}
		rows = append(rows, row{kind: rowHeader, text: header})
		for _, t := range items {
			kids := m.idx.children[t.ID]
			rows = append(rows, row{kind: rowTask, task: t, parent: len(kids) > 0})
			if !m.pick.expanded[t.ID] {
				continue
			}
			for _, c := range kids {
				if _, pinned := m.idx.cand[c.ID]; c.Status != domain.StatusOpen || m.idx.inInbox[c.ID] || pinned {
					continue
				}
				rows = append(rows, row{kind: rowTask, task: c, child: true})
			}
		}
	}
	return rows
}

// todayRows: today's candidates in plan order. A pinned parent lists its open
// children and children closed today beneath it; a pinned child of a pinned
// parent appears only there.
func (m *Model) todayRows() []row {
	var rows []row
	for _, c := range m.state.Plan.Candidates {
		if _, parentPinned := m.idx.cand[c.Task.ParentID]; c.Task.ParentID != 0 && parentPinned {
			continue
		}
		cc := c
		kids := m.idx.children[c.Task.ID]
		rows = append(rows, row{kind: rowTask, task: c.Task, cand: &cc, parent: len(kids) > 0})
		if c.Alert {
			rows = append(rows, row{kind: rowHint, text: fmt.Sprintf("⚠ rolled over %d× → refine · split · reprioritize · hand off · drop", c.Task.RolloverCount)})
		}
		for _, k := range kids {
			if k.Status != domain.StatusOpen && !m.closedToday(k) {
				continue
			}
			var kc *domain.Candidate
			if x, ok := m.idx.cand[k.ID]; ok {
				kc = &x
			}
			rows = append(rows, row{kind: rowTask, task: k, child: true, cand: kc})
		}
	}
	return rows
}

func (m *Model) rows(p pane) []row {
	if p == paneToday {
		return m.todayRows()
	}
	return m.backlogRows()
}

// clamp keeps each pane's cursor on a selectable row.
func (p *pickState) clamp(m *Model) {
	if p.expanded == nil {
		p.expanded = map[int64]bool{}
	}
	for _, pn := range []pane{paneBacklog, paneToday} {
		rows := m.rows(pn)
		c := min(p.cursor[pn], len(rows)-1)
		for c >= 0 && rows[c].kind != rowTask {
			c--
		}
		if c < 0 {
			c = slices.IndexFunc(rows, func(r row) bool { return r.kind == rowTask })
		}
		p.cursor[pn] = max(c, 0)
	}
}

func (p *pickState) move(m *Model, delta int) {
	rows := m.rows(p.focus)
	for c := p.cursor[p.focus] + delta; c >= 0 && c < len(rows); c += delta {
		if rows[c].kind == rowTask {
			p.cursor[p.focus] = c
			return
		}
	}
}

func (m *Model) selectedRow() (row, bool) {
	rows := m.rows(m.pick.focus)
	c := m.pick.cursor[m.pick.focus]
	if c < 0 || c >= len(rows) || rows[c].kind != rowTask {
		return row{}, false
	}
	return rows[c], true
}

func (m *Model) pickKey(key string) tea.Cmd {
	p := &m.pick
	switch key {
	case "tab":
		p.focus = 1 - p.focus
		p.clamp(m)
		return nil
	case "j", "down":
		p.move(m, +1)
		return nil
	case "k", "up":
		p.move(m, -1)
		return nil
	case "i":
		m.screen = screenInbox
		m.inbox.sync(m)
		return nil
	}
	r, ok := m.selectedRow()
	if !ok {
		return nil
	}
	t := r.task
	label := fmt.Sprintf("%q", t.Title)
	switch key {
	case "right", "l":
		if p.focus == paneBacklog && r.parent {
			p.expanded[t.ID] = true
		}
	case "left", "h":
		if p.focus == paneBacklog && r.child {
			p.expanded[t.ParentID] = false
			p.clamp(m)
		} else if p.focus == paneBacklog {
			p.expanded[t.ID] = false
		}
	case "p":
		if r.cand != nil {
			m.setStatus("already pinned")
			return nil
		}
		return m.act("pin "+label, "pinned "+label, func(ctx context.Context) (app.Undo, error) { return m.svc.Pin(ctx, t.ID) })
	case "u":
		if r.cand == nil {
			m.setStatus("not pinned")
			return nil
		}
		return m.act("unpin "+label, "unpinned "+label, func(ctx context.Context) (app.Undo, error) { return m.svc.Unpin(ctx, t.ID) })
	case "x":
		if t.Status != domain.StatusOpen {
			return m.act("reopen "+label, "reopened "+label, func(ctx context.Context) (app.Undo, error) { return m.svc.Reopen(ctx, t.ID) })
		}
		return m.act("done "+label, "✓ "+label, func(ctx context.Context) (app.Undo, error) { return m.svc.Done(ctx, t.ID) })
	case "d":
		return m.openDropPrompt(t)
	case "D":
		return m.openDeleteConfirm(t)
	}
	return nil
}

func (m *Model) pickView() string {
	total := max(m.width-3, 20)
	lw := total * 55 / 100
	rw := total - lw
	left := m.paneLines(paneBacklog, "BACKLOG", "nothing triaged yet · a to add · i to triage", lw)
	right := m.paneLines(paneToday, "TODAY'S CANDIDATES", "nothing pinned · p on a backlog task", rw)
	var b strings.Builder
	for i := range max(len(left), len(right)) {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(pad(l, lw) + " │ " + pad(r, rw))
	}
	return b.String()
}

func (m *Model) paneLines(pn pane, title, empty string, width int) []string {
	lines := []string{styleTitle.Render(title)}
	rows := m.rows(pn)
	if len(rows) == 0 {
		return append(lines, styleDim.Render("  "+empty))
	}
	visible := max(m.height-6, 3)
	cursor := m.pick.cursor[pn]
	start := max(0, cursor-visible+1)
	for i := start; i < len(rows) && i < start+visible; i++ {
		lines = append(lines, m.rowLine(rows[i], width, pn == m.pick.focus && i == cursor))
	}
	return lines
}

func (m *Model) rowLine(r row, width int, selected bool) string {
	switch r.kind {
	case rowHeader:
		return styleDim.Render(r.text)
	case rowHint:
		return styleAlert.Render("    " + r.text)
	}
	t := r.task
	var b strings.Builder
	switch {
	case r.child:
		b.WriteString("    ")
	case r.parent && r.cand == nil && !m.pick.expanded[t.ID]:
		b.WriteString("▸ ")
	case r.parent:
		b.WriteString("▾ ")
	default:
		b.WriteString("  ")
	}
	b.WriteString(m.badge(t) + " " + t.Title)
	if t.Requester != "" {
		b.WriteString(" @" + t.Requester)
	}
	if kids := m.idx.children[t.ID]; len(kids) > 0 {
		done := 0
		for _, k := range kids {
			if k.Status == domain.StatusDone {
				done++
			}
		}
		fmt.Fprintf(&b, " %d/%d", done, len(kids))
	}
	if c := r.cand; c != nil {
		if c.Blocked {
			b.WriteString(" ▓")
		}
		if t.RolloverCount > 0 {
			fmt.Fprintf(&b, " ↻%d", t.RolloverCount)
		}
		if c.PinnedDays > 1 {
			fmt.Fprintf(&b, " 📌%d", c.PinnedDays)
		}
	}
	switch t.Status {
	case domain.StatusDone:
		b.WriteString(" ✓")
	case domain.StatusDropped:
		b.WriteString(" ✗")
	}
	line := pad(b.String(), width)
	if t.Status != domain.StatusOpen {
		line = styleClosed.Render(line)
	}
	if selected {
		line = styleSelected.Render(line)
	}
	return line
}
```

- [ ] **Step 6: Implement the Inbox**

`internal/tui/inbox.go`:

```go
package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"tt/internal/app"
	"tt/internal/domain"
)

// inboxState shows one card at a time (D11a). Skipped cards move to the end
// for this session; they stay in the Inbox.
type inboxState struct {
	current int64
	skipped map[int64]bool
}

func (in *inboxState) order(m *Model) []domain.Task {
	tasks := m.state.Snapshot.Inbox()
	slices.SortStableFunc(tasks, func(a, b domain.Task) int {
		switch sa, sb := in.skipped[a.ID], in.skipped[b.ID]; {
		case sa == sb:
			return 0
		case sa:
			return 1
		default:
			return -1
		}
	})
	return tasks
}

// sync keeps the current card if it's still in the Inbox, else shows the first.
func (in *inboxState) sync(m *Model) {
	if in.skipped == nil {
		in.skipped = map[int64]bool{}
	}
	order := in.order(m)
	if slices.ContainsFunc(order, func(t domain.Task) bool { return t.ID == in.current }) {
		return
	}
	in.current = 0
	if len(order) > 0 {
		in.current = order[0].ID
	}
}

func (m *Model) inboxKey(key string) tea.Cmd {
	in := &m.inbox
	if key == "esc" {
		m.screen = screenPick
		return nil
	}
	t, ok := m.state.Snapshot.Get(in.current)
	if !ok || !m.idx.inInbox[t.ID] {
		return nil
	}
	label := fmt.Sprintf("%q", t.Title)
	levels := len(m.svc.Config().Priorities)
	switch {
	case len(key) == 1 && key[0] >= '1' && key[0] <= '9':
		n := int(key[0] - '0')
		if n > levels {
			m.setStatus(fmt.Sprintf("there are %d priority levels", levels))
			return nil
		}
		if parent := m.state.Snapshot.ParentOf(t); parent != nil {
			m.setStatus(fmt.Sprintf("priority comes from parent #%d %q", parent.ID, parent.Title))
			return nil
		}
		return m.act("priority "+label, "", func(ctx context.Context) (app.Undo, error) { return m.svc.SetPriority(ctx, t.ID, n) })
	case key == "s" || key == "m" || key == "l" || key == "x":
		size, _ := domain.ParseSize(key)
		return m.act("size "+label, "", func(ctx context.Context) (app.Undo, error) { return m.svc.SetSize(ctx, t.ID, size) })
	case key == "n":
		in.skipped[t.ID] = true
		order := in.order(m)
		if in.skipped[order[0].ID] { // everything skipped: start over
			clear(in.skipped)
		}
		in.current = in.order(m)[0].ID
	case key == "d":
		return m.openDropPrompt(t)
	case key == "D":
		return m.openDeleteConfirm(t)
	}
	return nil
}

func (m *Model) inboxView() string {
	order := m.inbox.order(m)
	if len(order) == 0 {
		return styleTitle.Render("INBOX") + "\n\n  Inbox zero ✓  " + styleDim.Render("esc to go back")
	}
	pos := slices.IndexFunc(order, func(t domain.Task) bool { return t.ID == m.inbox.current })
	t := order[max(pos, 0)]
	cfg := m.svc.Config()
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", styleTitle.Render(fmt.Sprintf("INBOX · %d of %d", pos+1, len(order))))
	fmt.Fprintf(&b, "  %s\n", styleTitle.Render(t.Title))
	if t.Link != "" {
		fmt.Fprintf(&b, "  %s\n", styleDim.Render(t.Link))
	}
	meta := []string{"captured " + ago(m.svc.Now(), t.CreatedAt)}
	if t.Requester != "" {
		meta = append(meta, "@"+t.Requester)
	}
	for _, tag := range t.Tags {
		meta = append(meta, "#"+tag)
	}
	parent := m.state.Snapshot.ParentOf(t)
	if parent != nil {
		meta = append(meta, fmt.Sprintf("parent #%d %s", parent.ID, parent.Title))
	}
	fmt.Fprintf(&b, "  %s\n", styleDim.Render(strings.Join(meta, " · ")))
	if t.Note != "" {
		note, _, _ := strings.Cut(t.Note, "\n")
		fmt.Fprintf(&b, "  %s\n", styleDim.Render("note: "+note))
	}
	b.WriteString("\n")
	if parent != nil {
		fmt.Fprintf(&b, "  priority  %s\n", styleDim.Render(fmt.Sprintf("from parent: %s", m.badge(*parent))))
	} else {
		for i, lvl := range cfg.Priorities {
			mark := " "
			if t.Priority == i+1 {
				mark = "●"
			}
			lead := "          "
			if i == 0 {
				lead = "  priority"
			}
			fmt.Fprintf(&b, "%s %s [%d] %s · %s\n", lead, mark, i+1, lvl.Name, lvl.Definition)
		}
	}
	var sizes []string
	for _, sz := range domain.Sizes {
		key := string(sz)
		if sz == domain.SizeXL {
			key = "x"
		}
		item := fmt.Sprintf("[%s] %s", key, shortDur(cfg.Sizes[sz]))
		if t.Size == sz {
			item = styleSelected.Render(item)
		}
		sizes = append(sizes, item)
	}
	fmt.Fprintf(&b, "  size       %s", strings.Join(sizes, "  "))
	return b.String()
}
```

- [ ] **Step 7: Implement the overlays**

`internal/tui/overlays.go`:

```go
package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"tt/internal/app"
	"tt/internal/domain"
)

func newInput(prompt, placeholder string, width int) textinput.Model {
	ti := textinput.New()
	ti.Prompt = prompt
	ti.Placeholder = placeholder
	ti.CharLimit = 1000
	ti.SetWidth(max(width-len(prompt)-2, 10))
	st := ti.Styles()
	st.Cursor.Blink = false // a static cursor: no blink timers
	ti.SetStyles(st)
	ti.Focus()
	return ti
}

// quickAdd is the one-line capture overlay (D25) with @/# autocomplete.
type quickAdd struct{ input textinput.Model }

func (m *Model) openQuickAdd() tea.Cmd {
	m.overlay = &quickAdd{input: newInput("add › ", "title @who #tag !1 ~s link -- note", m.width)}
	return nil
}

func (q *quickAdd) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.overlay = nil
		return nil
	case "tab":
		if s := q.suggestions(m); len(s) > 0 {
			q.complete(s[0])
		}
		return nil
	case "enter":
		line := q.input.Value()
		m.overlay = nil
		svc := m.svc
		return func() tea.Msg {
			t, undo, err := svc.Add(m.ctx, line)
			where := "backlog"
			if domain.InInbox(t, nil) {
				where = "inbox"
			}
			return actionMsg{label: fmt.Sprintf("add %q", t.Title), status: fmt.Sprintf("added #%d to %s", t.ID, where), undo: undo, err: err}
		}
	}
	var cmd tea.Cmd
	q.input, cmd = q.input.Update(k)
	return cmd
}

// currentToken is the word being typed (the input's last word).
func (q *quickAdd) currentToken() string {
	v := q.input.Value()
	if strings.HasSuffix(v, " ") {
		return ""
	}
	fields := strings.Fields(v)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// suggestions completes a trailing @requester or #tag from known values, most used first.
func (q *quickAdd) suggestions(m *Model) []string {
	tok := q.currentToken()
	if len(tok) < 1 || (tok[0] != '@' && tok[0] != '#') {
		return nil
	}
	pool := m.idx.tags
	if tok[0] == '@' {
		pool = m.idx.requesters
	}
	prefix := strings.ToLower(tok[1:])
	var out []string
	for _, v := range pool {
		if strings.HasPrefix(v, prefix) && v != prefix {
			out = append(out, string(tok[0])+v)
			if len(out) == 5 {
				break
			}
		}
	}
	return out
}

func (q *quickAdd) complete(s string) {
	v := q.input.Value()
	q.input.SetValue(v[:len(v)-len(q.currentToken())] + s + " ")
	q.input.CursorEnd()
}

func (q *quickAdd) view(m *Model) string {
	out := q.input.View()
	if s := q.suggestions(m); len(s) > 0 {
		out += "\n" + styleDim.Render("  tab → "+strings.Join(s, "  "))
	}
	return out
}

// prompt is a one-line text question, e.g. the optional drop reason (D23).
type prompt struct {
	input   textinput.Model
	onEnter func(value string) tea.Cmd
}

func (p *prompt) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.overlay = nil
		return nil
	case "enter":
		m.overlay = nil
		return p.onEnter(p.input.Value())
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(k)
	return cmd
}

func (p *prompt) view(*Model) string { return p.input.View() }

func (m *Model) openDropPrompt(t domain.Task) tea.Cmd {
	label := fmt.Sprintf("%q", t.Title)
	m.overlay = &prompt{
		input: newInput("drop "+label+" · reason (enter to skip) › ", "", m.width),
		onEnter: func(reason string) tea.Cmd {
			return m.act("drop "+label, "dropped "+label, func(ctx context.Context) (app.Undo, error) { return m.svc.Drop(ctx, t.ID, reason) })
		},
	}
	return nil
}

// confirm asks y/n; anything but y cancels.
type confirm struct {
	question string
	onYes    func() tea.Cmd
}

func (c *confirm) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
	m.overlay = nil
	if k.String() == "y" {
		return c.onYes()
	}
	m.setStatus("cancelled")
	return nil
}

func (c *confirm) view(*Model) string { return styleAlert.Render(c.question + " (y/n)") }

func (m *Model) openDeleteConfirm(t domain.Task) tea.Cmd {
	m.overlay = &confirm{
		question: fmt.Sprintf("Delete #%d %q? This can't be undone.", t.ID, t.Title),
		onYes: func() tea.Cmd {
			ctx, svc := m.ctx, m.svc
			return func() tea.Msg {
				err := svc.Delete(ctx, t.ID)
				return actionMsg{status: fmt.Sprintf("deleted #%d", t.ID), err: err}
			}
		},
	}
	return nil
}

// helpOverlay lists the keys; any key closes it.
type helpOverlay struct{}

func (helpOverlay) update(m *Model, _ tea.KeyPressMsg) tea.Cmd {
	m.overlay = nil
	return nil
}

func (helpOverlay) view(m *Model) string {
	global := []string{
		"a        quick-add: title @who #tag !1 ~s link -- note (tab completes @/#)",
		"ctrl+z   undo (refuses if the task changed since)",
		"R        reload · q quit · ? this help",
	}
	var local []string
	if m.screen == screenInbox {
		local = []string{
			fmt.Sprintf("1-%d      set priority", len(m.svc.Config().Priorities)),
			"s m l x  set size (x = XL); a card with priority and size leaves the inbox",
			"n        skip (stays in the inbox)",
			"d / D    drop with optional reason / delete",
			"esc      back to Pick",
		}
	} else {
		local = []string{
			"tab      switch backlog / today",
			"j k      move · → ← expand / collapse a parent",
			"p / u    pin to today / unpin",
			"x        done (again: reopen)",
			"d / D    drop with optional reason / delete",
			"i        inbox",
		}
	}
	return styleTitle.Render("KEYS") + "\n" + strings.Join(append(local, global...), "\n") + "\n" + styleDim.Render("any key closes")
}
```

- [ ] **Step 8: Run the tests to verify they pass**

```bash
go mod tidy
gofmt -l . && go vet ./... && go test ./...
go test -race ./internal/tui/
```

Expected: no gofmt output; every package `ok`; the race run `ok`.

- [ ] **Step 9: Commit**

```bash
git add go.mod go.sum internal/tui
git commit -m "feat(tui): Pick, Inbox, quick-add, undo, live reload and new-day banner" -m "Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 3: `tt` opens the planner

**Files:**
- Modify: `cmd/tt/main.go`, `cmd/tt/main_test.go`

**Interfaces:**
- Consumes: Task 2 (`tui.New`, `tui.DefaultOptions`), Plan 1 (`run`, `open`, `paths`, `usage`).
- Produces: `run(ctx, args, stdout, stderr, getenv, ui func(context.Context, *app.Service) error) int`. No arguments → opens the service and calls `ui` (production: `runTUI`, a `tea.NewProgram(...).Run()`), exit 0 on a clean quit. `-h`, `--help`, `help` → usage on stdout, exit 0 (carry-forward). Unknown arguments → usage on stderr, exit 2. `tt add` is unchanged.

- [ ] **Step 1: Apply the test changes and see them fail**

```bash
git apply <<'PATCH'
diff --git a/cmd/tt/main_test.go b/cmd/tt/main_test.go
index 752a372..3dc22ab 100644
--- a/cmd/tt/main_test.go
+++ b/cmd/tt/main_test.go
@@ -8,6 +8,7 @@ import (
 	"strings"
 	"testing"
 
+	"tt/internal/app"
 	"tt/internal/store"
 )
 
@@ -19,7 +20,7 @@ type result struct {
 func runTT(t *testing.T, env map[string]string, args ...string) result {
 	t.Helper()
 	var out, errOut bytes.Buffer
-	code := run(context.Background(), args, &out, &errOut, func(k string) string { return env[k] })
+	code := run(context.Background(), args, &out, &errOut, func(k string) string { return env[k] }, noUI)
 	return result{code, out.String(), errOut.String()}
 }
 
@@ -55,7 +56,7 @@ func TestAddReportsWhereTaskLanded(t *testing.T) {
 
 func TestUsageErrors(t *testing.T) {
 	env := testEnv(t)
-	for _, args := range [][]string{nil, {"bogus"}, {"add"}, {"add", "  "}} {
+	for _, args := range [][]string{{"bogus"}, {"add"}, {"add", "  "}} {
 		if r := runTT(t, env, args...); r.code != 2 || !strings.Contains(r.stderr, "usage:") {
 			t.Errorf("tt %v: %+v; want exit 2 with usage", args, r)
 		}
@@ -100,3 +101,26 @@ func TestDefaultPaths(t *testing.T) {
 		t.Error("paths without HOME should fail")
 	}
 }
+
+// noUI fails the test if the TUI would start.
+func noUI(context.Context, *app.Service) error { panic("TUI started") }
+
+func TestNoArgsStartsTheTUI(t *testing.T) {
+	env := testEnv(t)
+	var got *app.Service
+	var out, errOut bytes.Buffer
+	code := run(context.Background(), nil, &out, &errOut, func(k string) string { return env[k] },
+		func(_ context.Context, svc *app.Service) error { got = svc; return nil })
+	if code != 0 || got == nil {
+		t.Errorf("tt with no args: exit %d, ui started: %v, stderr %q", code, got != nil, errOut.String())
+	}
+}
+
+func TestHelp(t *testing.T) {
+	env := testEnv(t)
+	for _, arg := range []string{"-h", "--help", "help"} {
+		if r := runTT(t, env, arg); r.code != 0 || !strings.Contains(r.stdout, "usage:") {
+			t.Errorf("tt %s: %+v; want exit 0 with usage on stdout", arg, r)
+		}
+	}
+}
PATCH
go test ./cmd/tt/
```

Expected: FAIL to build: `too many arguments in call to run`.

- [ ] **Step 2: Apply the command changes**

```bash
git apply <<'PATCH'
diff --git a/cmd/tt/main.go b/cmd/tt/main.go
index fdcee3f..18728fd 100644
--- a/cmd/tt/main.go
+++ b/cmd/tt/main.go
@@ -9,13 +9,17 @@ import (
 	"path/filepath"
 	"strings"
 
+	tea "charm.land/bubbletea/v2"
+
 	"tt/internal/app"
 	"tt/internal/config"
 	"tt/internal/domain"
 	"tt/internal/store"
+	"tt/internal/tui"
 )
 
 const usage = `usage:
+  tt               open the planner (TUI)
   tt add <line>    capture a task, e.g. tt add 'Reply to Ana @ana #photos !1 ~s -- before Friday'
                    quote the line with single quotes: ! and ~ are special in zsh/bash
 
@@ -25,12 +29,35 @@ environment:
 `
 
 func main() {
-	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
+	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, os.Getenv, runTUI))
+}
+
+// runTUI runs the full-screen planner until the user quits.
+func runTUI(ctx context.Context, svc *app.Service) error {
+	_, err := tea.NewProgram(tui.New(ctx, svc, tui.DefaultOptions()), tea.WithContext(ctx)).Run()
+	return err
 }
 
 // run is main without process globals, so tests can call it directly.
-func run(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string) int {
-	if len(args) == 0 || args[0] != "add" {
+func run(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string,
+	ui func(context.Context, *app.Service) error) int {
+	switch {
+	case len(args) == 0:
+		svc, closeFn, err := open(ctx, getenv)
+		if err != nil {
+			fmt.Fprintf(stderr, "tt: %v\n", err)
+			return 1
+		}
+		defer closeFn()
+		if err := ui(ctx, svc); err != nil {
+			fmt.Fprintf(stderr, "tt: %v\n", err)
+			return 1
+		}
+		return 0
+	case args[0] == "-h" || args[0] == "--help" || args[0] == "help":
+		fmt.Fprint(stdout, usage)
+		return 0
+	case args[0] != "add":
 		fmt.Fprint(stderr, usage)
 		return 2
 	}
PATCH
```

- [ ] **Step 3: Run the tests and a real-terminal smoke test**

```bash
gofmt -l . && go vet ./... && go test ./...
go build -o tt ./cmd/tt
TT_DB="$(mktemp -d)/tt.db" TT_CONFIG=/nonexistent ./tt
```

Expected: all packages `ok`. In the TUI: the new-day banner shows; `esc`, then `a`, type `Smoke !1 ~s`, `enter` shows `added #1 to backlog` and the row `[P1 S] Smoke`; `q` exits. Then `rm tt`. (An automated agent without a terminal may skip the interactive part and say so.)

- [ ] **Step 4: Commit**

```bash
git add cmd/tt
git commit -m "feat(cli): tt opens the planner; -h/--help" -m "Co-Authored-By: Claude <noreply@anthropic.com>"
```
