# tt Plan 4b — Edit & Detail Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `⏎` on any task opens a full-screen Task detail (facts, link, note preview, children with progress) where `e` edits the line, `E` edits the note in `$EDITOR`, `o` opens the link, `a` adds children one after another and `J/K` reorders them; `tt add` with no line reads a prompt or a pipe, and multi-line pastes keep their body as the note.

**Architecture:** New service commands (`AddChild`, `Reorder`, `SetNoteFrom`, `ParseChild`/`ParseEdit`) keep every rule in `app`. The TUI gets a third screen (`screenDetail`, a stack of task ids with a cached row list), shares the per-task keys between Pick and detail (`taskKey`), and runs outside programs through injectable `Options.Exec` / `Options.Open` so tests stay in-process. `dsl.WithNoteLines` is the one rule for "lines 2+ are the note", used by the CLI and the TUI.

**Tech Stack:** Go 1.27; Bubble Tea v2 (`tea.ExecProcess`), bubbles v2 textinput, Lip Gloss v2; pure-Go SQLite. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-10-12-task-tracking-design.md` — View 5, D6, D21, D25, D29, D29a, D31–D33, open item P1, "Plan 4 split" → Plan 4b, "Carry-forward from Plan 4a review".

## Global Constraints

- No new dependencies: `go.mod` and `go.sum` stay unchanged.
- Only `app` mutates state; `internal/tui` and `cmd/tt` call `app.Service`. Every user command returns an undo; undo refuses with `ErrConflict` rather than damaging state.
- D32: children never take `!n` — refused (not ignored) in the edit line and add-child, with "children inherit priority from #<parent>".
- D29/D29a: unchanged file → no write; empty file → note cleared; non-zero editor exit → discarded; a note that changed elsewhere is never overwritten, and the typed text stays in the temp file whose path the status shows.
- D33: lines 2+ of a pipe or a multi-line paste become the note; `tt add <line>` is unchanged.
- Overlays that parse tokens validate synchronously and keep the text on error.
- Lines never wrap: every rendered line is truncated to the terminal width.
- Every task ends green on: `gofmt -l .` (no output), `go vet ./...`, `go test ./...`.
- Every commit message ends with a blank line and `Co-Authored-By: Claude <noreply@anthropic.com>`.
- Sandbox: save each patch to a file and `git apply <file>`; run plain, separate git commands (no `git -C`, no `cd … &&`). If `git apply` fails, stop: an earlier step went wrong.

## Review Focus

1. **The note changed in another shell while `$EDITOR` was open**: nothing is overwritten and the typed text survives in the file named on the status line. Pinned in Task 4 `TestNoteChangedElsewhereKeepsYourText` (and Task 1 `TestSetNoteFrom`).
2. **The task (or its parent) is deleted from another shell while its detail is open**: detail falls back a level, then to Pick, saying "#n is gone". Pinned in Task 3 `TestDetailFallsBackWhenTheTaskIsGone`.
3. **Pasting a Slack message (multi-line, CRLF) into quick-add or piping it to `tt add`**: line 1 is the token line, the rest is the note, tokens inside the body stay text. Pinned in Task 5 `TestMultiLinePasteBecomesTheNote`, Task 2 `TestAddFromAPipe` and `TestWithNoteLines`.
4. **The editor exits non-zero (`:cq`) or the file is left unchanged**: nothing is saved, nothing is written. Pinned in Task 4 `TestNoteInTheEditor`.
5. **`!n` typed on a child** (edit line or add-child): refused with the reason, the text stays. Pinned in Task 1 `TestChildPriorityIsRefusedInEdits`/`TestAddChildRules`, Task 4 `TestEditLineOfAChildRefusesPriority`, Task 5 `TestAddChildrenOneAfterAnother`.

## File Structure

```
internal/store/tasks.go                     modify: Tx.Children
internal/app/children.go, children_test.go  new: ParseChild, AddChild, Reorder, ErrNoMove
internal/app/tasks.go                       modify: EditLine (child), ParseEdit, Edit, SetNoteFrom
internal/dsl/add.go, add_test.go            modify: WithNoteLines
cmd/tt/main.go, main_test.go                modify: input, tt add prompt/pipe, error-path tests
internal/tui/detail.go, detail_test.go      new: Task detail screen
internal/tui/edit.go, edit_test.go          new: edit line, editor, open link, add-child, reorder
internal/tui/children_test.go               new: add-child, J/K, multi-line paste
internal/tui/browse_test.go                 new: 4a carry-forwards
internal/tui/pick.go                        modify: taskKey, ⏎, ✎, placement parent flag, l/h when filtered, saveGroup
internal/tui/model.go                       modify: screenDetail, Options.Exec/Open, messages, help full screen, a in detail
internal/tui/common.go, overlays.go         modify: header/hints/help, filterBar selection, quick-add paste
internal/tui/tokeninput.go                  modify: pasted note lines, plain (filter) mode
internal/tui/harness_test.go                modify: fake editor and opener
```

Work happens on branch `plan-4b` in a worktree; fast-forward merge to `main` when the final review is green.

---

### Task 1: Service: add child, reorder, compare-and-set notes

View 5 / D6 / D29a / D32: the commands Task detail needs. `!n` on a child is refused in one place for edits and add-child.

**Files:**
- Create: `internal/app/children.go`
- Create: `internal/app/children_test.go`
- Modify: `internal/app/tasks.go`
- Modify: `internal/store/tasks.go`

**Interfaces:**
- Consumes: `Service.modify`, `unchangedSince`, `store.Tx` (`GetTask`, `InsertTask`, `UpdateTask`, `NextChildPosition`); test helpers `newService`, `mustAdd`, `mustGet`, `mustUndo`.
- Produces:
  - `func (*store.Tx) Children(ctx, parentID int64) ([]domain.Task, error)`: sibling order (position, then id), with tags.
  - `var app.ErrNoMove`; `func (*Service) ParseChild(parentID int64, line string) (dsl.Draft, error)` (`!n` → `ErrInvalidValue` "children inherit priority from #<parent>; drop !<n>").
  - `func (*Service) AddChild(ctx, parentID int64, line string) (domain.Task, Undo, error)`: appended last; parent must be top-level (`ErrInvalidParent`).
  - `func (*Service) Reorder(ctx, id int64, delta int) (Undo, error)`: moves past the nearest sibling of the same kind (open among open, closed among closed), renumbers 0..n-1; `ErrNoMove` at an end; undo refuses with `ErrConflict` if a moved row changed.
  - `func (*Service) ParseEdit(t domain.Task, line string) (dsl.Draft, error)`; `Edit` uses it (D32); `EditLine` leaves a child's priority out.
  - `func (*Service) SetNoteFrom(ctx, id int64, from, note string) (Undo, error)`: `ErrConflict` ("note changed since you opened it") unless the note is still `from`.

- [ ] **Step 1: Write the failing tests**

Create `internal/app/children_test.go`:

```go
package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"tt/internal/domain"
)

// childOrder returns parentID's children ids in sibling order.
func childOrder(t *testing.T, s *Service, parentID int64) []int64 {
	t.Helper()
	snap, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var kids []domain.Task
	for _, k := range snap.Tasks() {
		if k.ParentID == parentID {
			kids = append(kids, k)
		}
	}
	slices.SortFunc(kids, func(a, b domain.Task) int {
		if a.Position != b.Position {
			return a.Position - b.Position
		}
		return int(a.ID - b.ID)
	})
	var ids []int64
	for _, k := range kids {
		ids = append(ids, k.ID)
	}
	return ids
}

func mustAddChild(t *testing.T, s *Service, parentID int64, line string) domain.Task {
	t.Helper()
	c, _, err := s.AddChild(context.Background(), parentID, line)
	if err != nil {
		t.Fatalf("AddChild(%q): %v", line, err)
	}
	return c
}

func TestAddChild(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	parent := mustAdd(t, s, "Auth migration !1 ~xl #auth")
	a := mustAddChild(t, s, parent.ID, "Token spike ~s #ops -- try the new lib")
	b, undo, err := s.AddChild(ctx, parent.ID, "Rollback plan ~m @ana")
	if err != nil {
		t.Fatal(err)
	}
	got := mustGet(t, s, a.ID)
	if got.ParentID != parent.ID || got.Size != domain.SizeS || got.Note != "try the new lib" || got.Priority != 0 ||
		!slices.Equal(got.Tags, []string{"ops"}) {
		t.Errorf("child: %+v", got)
	}
	if domain.InInbox(got, &parent) {
		t.Error("a sized child inherits priority and skips the Inbox")
	}
	if order := childOrder(t, s, parent.ID); !slices.Equal(order, []int64{a.ID, b.ID}) {
		t.Errorf("children appended last: %v", order)
	}
	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if order := childOrder(t, s, parent.ID); !slices.Equal(order, []int64{a.ID}) {
		t.Errorf("undo removes the added child: %v", order)
	}
}

func TestAddChildRules(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	parent := mustAdd(t, s, "Parent !1 ~l")
	child := mustAddChild(t, s, parent.ID, "Child ~s")

	_, _, err := s.AddChild(ctx, parent.ID, "Urgent bit !1 ~s")
	if !errors.Is(err, ErrInvalidValue) || !strings.Contains(err.Error(), "children inherit priority from #1; drop !1") {
		t.Errorf("!n on a child: %v", err)
	}
	if _, _, err := s.AddChild(ctx, child.ID, "Grandchild ~s"); !errors.Is(err, ErrInvalidParent) {
		t.Errorf("child of a child: %v; want ErrInvalidParent (D6)", err)
	}
	if _, _, err := s.AddChild(ctx, 999, "Orphan ~s"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing parent: %v", err)
	}
	if _, _, err := s.AddChild(ctx, parent.ID, "#only-tokens"); err == nil {
		t.Error("a child needs a title")
	}
	if got := childOrder(t, s, parent.ID); len(got) != 1 {
		t.Errorf("refused adds must not add: %v", got)
	}
}

func TestReorder(t *testing.T) {
	must := mustUndo(t)
	s, clock := newService(t)
	ctx := context.Background()
	parent := mustAdd(t, s, "Parent !1 ~l")
	a := mustAddChild(t, s, parent.ID, "A ~s")
	b := mustAddChild(t, s, parent.ID, "B ~s")
	c := mustAddChild(t, s, parent.ID, "C ~s")
	d := mustAddChild(t, s, parent.ID, "D ~s")
	clock.Advance(time.Minute)
	must(s.Done(ctx, b.ID))

	// Open children move past open ones: C up skips the closed B.
	clock.Advance(time.Minute)
	undo := must(s.Reorder(ctx, c.ID, -1))
	if got := childOrder(t, s, parent.ID); !slices.Equal(got, []int64{c.ID, b.ID, a.ID, d.ID}) {
		t.Errorf("after C up: %v", got)
	}
	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := childOrder(t, s, parent.ID); !slices.Equal(got, []int64{a.ID, b.ID, c.ID, d.ID}) {
		t.Errorf("undo restores the order: %v", got)
	}

	if _, err := s.Reorder(ctx, a.ID, -1); !errors.Is(err, ErrNoMove) {
		t.Errorf("first open child up: %v; want ErrNoMove", err)
	}
	if _, err := s.Reorder(ctx, d.ID, +1); !errors.Is(err, ErrNoMove) {
		t.Errorf("last child down: %v; want ErrNoMove", err)
	}
	if _, err := s.Reorder(ctx, b.ID, +1); !errors.Is(err, ErrNoMove) {
		t.Errorf("the only closed child: %v; want ErrNoMove", err)
	}
	if _, err := s.Reorder(ctx, parent.ID, +1); !errors.Is(err, ErrInvalidParent) {
		t.Errorf("a top-level task: %v; want ErrInvalidParent", err)
	}

	// Undo refuses once a moved row changed since (clean state beats undo).
	clock.Advance(time.Minute)
	undo = must(s.Reorder(ctx, a.ID, +1))
	clock.Advance(time.Minute)
	must(s.SetNote(ctx, c.ID, "changed elsewhere"))
	if err := undo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo after a change: %v; want ErrConflict", err)
	}
	if got := childOrder(t, s, parent.ID); !slices.Equal(got, []int64{c.ID, b.ID, a.ID, d.ID}) {
		t.Errorf("a refused undo changes nothing: %v", got)
	}
}

func TestChildPriorityIsRefusedInEdits(t *testing.T) {
	must := mustUndo(t)
	s, clock := newService(t)
	ctx := context.Background()
	parent := mustAdd(t, s, "Parent !1 ~l")
	child := mustAdd(t, s, "Child !3 ~s") // own priority from before it became a child
	clock.Advance(time.Minute)
	must(s.SetParent(ctx, child.ID, parent.ID))
	got := mustGet(t, s, child.ID)

	if line := EditLine(got); line != "Child ~s" {
		t.Errorf("EditLine of a child = %q; want no priority (D32)", line)
	}
	if line := EditLine(mustGet(t, s, parent.ID)); line != "Parent !1 ~l" {
		t.Errorf("EditLine of a parent = %q", line)
	}
	if _, err := s.ParseEdit(got, "Child !2 ~s"); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("ParseEdit with !n on a child: %v", err)
	}
	if _, err := s.Edit(ctx, child.ID, "Child !2 ~s"); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("Edit with !n on a child: %v", err)
	}
	clock.Advance(time.Minute)
	must(s.Edit(ctx, child.ID, "Child renamed ~m"))
	if got := mustGet(t, s, child.ID); got.Title != "Child renamed" || got.Size != domain.SizeM {
		t.Errorf("edited child: %+v", got)
	}
}

func TestSetNoteFrom(t *testing.T) {
	must := mustUndo(t)
	s, clock := newService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Task -- first")

	clock.Advance(time.Minute)
	undo := must(s.SetNoteFrom(ctx, task.ID, "first", "second\n\n"))
	if got := mustGet(t, s, task.ID).Note; got != "second" {
		t.Errorf("note = %q", got)
	}
	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, s, task.ID).Note; got != "first" {
		t.Errorf("after undo: %q", got)
	}

	// D29a: the note changed while the editor was open.
	clock.Advance(time.Minute)
	must(s.SetNote(ctx, task.ID, "from another shell"))
	if _, err := s.SetNoteFrom(ctx, task.ID, "first", "mine"); !errors.Is(err, ErrConflict) ||
		!strings.Contains(err.Error(), "note changed since you opened it") {
		t.Errorf("stale note edit: %v; want ErrConflict", err)
	}
	if got := mustGet(t, s, task.ID).Note; got != "from another shell" {
		t.Errorf("a refused note edit changes nothing: %q", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/`
Expected: FAIL: `s.AddChild undefined`, `undefined: ErrNoMove`, `s.ParseEdit undefined`, `s.SetNoteFrom undefined`.

- [ ] **Step 3: Implement**

Create `internal/app/children.go`:

```go
package app

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"tt/internal/domain"
	"tt/internal/dsl"
	"tt/internal/store"
)

// ErrNoMove means a reorder had no sibling to move past.
var ErrNoMove = errors.New("no sibling in that direction")

// ParseChild parses an add-child line for parentID. Children inherit their
// parent's priority (D6), so !n is refused (D32).
func (s *Service) ParseChild(parentID int64, line string) (dsl.Draft, error) {
	d, err := dsl.ParseAdd(line, len(s.cfg.Priorities))
	if err != nil {
		return dsl.Draft{}, err
	}
	if d.Priority != 0 {
		return dsl.Draft{}, inheritedPriority(parentID, d.Priority)
	}
	return d, nil
}

func inheritedPriority(parentID int64, p int) error {
	return fmt.Errorf("%w: children inherit priority from #%d; drop !%d", ErrInvalidValue, parentID, p)
}

// AddChild captures a task from line as the last child of parentID.
func (s *Service) AddChild(ctx context.Context, parentID int64, line string) (domain.Task, Undo, error) {
	d, err := s.ParseChild(parentID, line)
	if err != nil {
		return domain.Task{}, nil, err
	}
	now := s.clock.Now()
	t := domain.Task{
		Title: d.Title, Note: d.Note, Link: d.Link, Requester: d.Requester, Tags: d.Tags, Size: d.Size,
		ParentID: parentID, Status: domain.StatusOpen, CreatedAt: now, UpdatedAt: now,
	}
	err = s.store.InTx(ctx, func(x *store.Tx) error {
		parent, err := x.GetTask(ctx, parentID)
		if err != nil {
			return err
		}
		if parent.ParentID != 0 {
			return fmt.Errorf("%w: #%d is already a child; only one level of nesting", ErrInvalidParent, parentID)
		}
		if t.Position, err = x.NextChildPosition(ctx, parentID); err != nil {
			return err
		}
		t.ID, err = x.InsertTask(ctx, t)
		return err
	})
	if err != nil {
		return domain.Task{}, nil, err
	}
	added := t
	undo := func(ctx context.Context) error {
		return s.store.InTx(ctx, func(x *store.Tx) error {
			if err := unchangedSince(ctx, x, added); err != nil {
				return err
			}
			return x.DeleteTask(ctx, added.ID)
		})
	}
	return t, undo, nil
}

// Reorder moves child id one place up (delta < 0) or down (delta > 0) past
// its nearest sibling of the same kind: open children move among open ones,
// closed among closed, matching how Task detail lists them. Siblings are then
// renumbered 0..n-1. ErrNoMove when there is no such sibling.
func (s *Service) Reorder(ctx context.Context, id int64, delta int) (Undo, error) {
	var before, after []domain.Task
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		t, err := x.GetTask(ctx, id)
		if err != nil {
			return err
		}
		if t.ParentID == 0 {
			return fmt.Errorf("%w: #%d has no parent to reorder within", ErrInvalidParent, id)
		}
		sibs, err := x.Children(ctx, t.ParentID)
		if err != nil {
			return err
		}
		open := t.Status == domain.StatusOpen
		i := slices.IndexFunc(sibs, func(c domain.Task) bool { return c.ID == id })
		j := i + sign(delta)
		for j >= 0 && j < len(sibs) && (sibs[j].Status == domain.StatusOpen) != open {
			j += sign(delta)
		}
		if delta == 0 || j < 0 || j >= len(sibs) {
			return ErrNoMove
		}
		order := slices.Clone(sibs)
		order[i], order[j] = order[j], order[i]
		now := s.clock.Now()
		for pos, c := range order {
			if c.Position == pos {
				continue
			}
			before = append(before, c)
			c.Position, c.UpdatedAt = pos, now
			if err := x.UpdateTask(ctx, c); err != nil {
				return err
			}
			after = append(after, c)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) error {
		return s.store.InTx(ctx, func(x *store.Tx) error {
			for _, c := range after {
				if err := unchangedSince(ctx, x, c); err != nil {
					return err
				}
			}
			for _, c := range before {
				if err := x.UpdateTask(ctx, c); err != nil {
					return err
				}
			}
			return nil
		})
	}, nil
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
```

Patch `internal/app/tasks.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/tasks.go b/internal/app/tasks.go
index 3316a1a..2d111a1 100644
--- a/internal/app/tasks.go
+++ b/internal/app/tasks.go
@@ -51,21 +51,33 @@ func (s *Service) Add(ctx context.Context, line string) (domain.Task, Undo, erro
 
 // EditLine renders a task as a token line for prefilling the edit input.
 // The note is left out; Edit keeps it unless the new line has " -- ".
+// A child's priority is inherited (D6), so it is left out too (D32).
 func EditLine(t domain.Task) string {
-	return dsl.FormatAdd(dsl.Draft{
-		Title: t.Title, Requester: t.Requester, Tags: t.Tags, Priority: t.Priority, Size: t.Size, Link: t.Link,
-	})
+	d := dsl.Draft{Title: t.Title, Requester: t.Requester, Tags: t.Tags, Priority: t.Priority, Size: t.Size, Link: t.Link}
+	if t.ParentID != 0 {
+		d.Priority = 0
+	}
+	return dsl.FormatAdd(d)
+}
+
+// ParseEdit parses an edit line for t: !n is refused on a child (D32).
+// The TUI calls it to validate before submitting; Edit applies the same rule.
+func (s *Service) ParseEdit(t domain.Task, line string) (dsl.Draft, error) {
+	if t.ParentID != 0 {
+		return s.ParseChild(t.ParentID, line)
+	}
+	return dsl.ParseAdd(line, len(s.cfg.Priorities))
 }
 
 // Edit replaces title, requester, tags, priority, size and link from line;
 // a token missing from line clears that field. The note changes only if
 // line contains " -- ".
 func (s *Service) Edit(ctx context.Context, id int64, line string) (Undo, error) {
-	d, err := dsl.ParseAdd(line, len(s.cfg.Priorities))
-	if err != nil {
-		return nil, err
-	}
 	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
+		d, err := s.ParseEdit(*t, line)
+		if err != nil {
+			return err
+		}
 		t.Title, t.Requester, t.Tags, t.Priority, t.Size, t.Link = d.Title, d.Requester, d.Tags, d.Priority, d.Size, d.Link
 		if d.HasNote {
 			t.Note = d.Note
@@ -82,6 +94,18 @@ func (s *Service) SetNote(ctx context.Context, id int64, note string) (Undo, err
 	})
 }
 
+// SetNoteFrom replaces the note, but only if it is still from: the note as
+// it was when the user started editing it. Otherwise ErrConflict (D29a).
+func (s *Service) SetNoteFrom(ctx context.Context, id int64, from, note string) (Undo, error) {
+	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
+		if t.Note != from {
+			return fmt.Errorf("note changed since you opened it: %w", ErrConflict)
+		}
+		t.Note = strings.TrimRight(note, "\n")
+		return nil
+	})
+}
+
 // SetPriority sets priority p (1..levels); 0 clears it.
 func (s *Service) SetPriority(ctx context.Context, id int64, p int) (Undo, error) {
 	if p < 0 || p > len(s.cfg.Priorities) {
```

Patch `internal/store/tasks.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/store/tasks.go b/internal/store/tasks.go
index 585b01b..16f51b8 100644
--- a/internal/store/tasks.go
+++ b/internal/store/tasks.go
@@ -91,6 +91,35 @@ func (x *Tx) HasChildren(ctx context.Context, id int64) (bool, error) {
 	return n > 0, err
 }
 
+// Children returns parentID's children in sibling order (position, then id).
+func (x *Tx) Children(ctx context.Context, parentID int64) ([]domain.Task, error) {
+	rows, err := x.tx.QueryContext(ctx, "SELECT id FROM tasks WHERE parent_id = ? ORDER BY position, id", parentID)
+	if err != nil {
+		return nil, err
+	}
+	var ids []int64
+	for rows.Next() {
+		var id int64
+		if err := rows.Scan(&id); err != nil {
+			rows.Close()
+			return nil, err
+		}
+		ids = append(ids, id)
+	}
+	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
+		return nil, err
+	}
+	kids := make([]domain.Task, 0, len(ids))
+	for _, id := range ids {
+		t, err := x.GetTask(ctx, id)
+		if err != nil {
+			return nil, err
+		}
+		kids = append(kids, t)
+	}
+	return kids, nil
+}
+
 // NextChildPosition returns the position after parentID's last child.
 func (x *Tx) NextChildPosition(ctx context.Context, parentID int64) (int, error) {
 	var n int
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/app/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/app/children.go internal/app/children_test.go internal/app/tasks.go internal/store/tasks.go
git commit -m 'feat(app): add child, reorder children, compare-and-set notes (D29a, D32)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 2: `tt add` from a prompt or a pipe

D33 and the Plan 3 carry-forwards "`tt add` without shell quoting" and "`tt` open/ui error paths untested".

**Files:**
- Modify: `cmd/tt/main.go`
- Modify: `cmd/tt/main_test.go`
- Modify: `internal/dsl/add.go`
- Modify: `internal/dsl/add_test.go`

**Interfaces:**
- Consumes: `dsl.ParseAdd`'s note rule (` -- `), `run`'s existing flow.
- Produces:
  - `func dsl.WithNoteLines(line string, rest []string) string`: lines 2+ become the note (after the line's own ` -- ` note on a new line; an empty trailing ` --` is filled); trailing blank lines dropped. Task 5 uses it for multi-line pastes.
  - `cmd/tt`: `type input struct{ r io.Reader; terminal bool }`; `run(ctx, args, in input, stdout, stderr, getenv, ui)` (new `in` parameter); `tt add` with no line prompts `add › ` on stderr from a terminal, or reads the pipe (≤ 1 MiB, CRLF tolerated). Usage text updated.

- [ ] **Step 1: Write the failing tests**

Patch `cmd/tt/main_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/cmd/tt/main_test.go b/cmd/tt/main_test.go
index 3dc22ab..c559142 100644
--- a/cmd/tt/main_test.go
+++ b/cmd/tt/main_test.go
@@ -3,12 +3,14 @@ package main
 import (
 	"bytes"
 	"context"
+	"errors"
 	"os"
 	"path/filepath"
 	"strings"
 	"testing"
 
 	"tt/internal/app"
+	"tt/internal/domain"
 	"tt/internal/store"
 )
 
@@ -18,12 +20,35 @@ type result struct {
 }
 
 func runTT(t *testing.T, env map[string]string, args ...string) result {
+	t.Helper()
+	return runWithInput(t, env, input{r: strings.NewReader("")}, args...)
+}
+
+func runWithInput(t *testing.T, env map[string]string, in input, args ...string) result {
 	t.Helper()
 	var out, errOut bytes.Buffer
-	code := run(context.Background(), args, &out, &errOut, func(k string) string { return env[k] }, noUI)
+	code := run(context.Background(), args, in, &out, &errOut, func(k string) string { return env[k] }, noUI)
 	return result{code, out.String(), errOut.String()}
 }
 
+func storedTask(t *testing.T, env map[string]string, id int64) domain.Task {
+	t.Helper()
+	st, err := store.Open(context.Background(), env["TT_DB"])
+	if err != nil {
+		t.Fatal(err)
+	}
+	defer st.Close()
+	snap, err := st.LoadSnapshot(context.Background())
+	if err != nil {
+		t.Fatal(err)
+	}
+	task, ok := snap.Get(id)
+	if !ok {
+		t.Fatalf("task #%d not stored", id)
+	}
+	return task
+}
+
 func testEnv(t *testing.T) map[string]string {
 	dir := t.TempDir()
 	return map[string]string{
@@ -109,13 +134,73 @@ func TestNoArgsStartsTheTUI(t *testing.T) {
 	env := testEnv(t)
 	var got *app.Service
 	var out, errOut bytes.Buffer
-	code := run(context.Background(), nil, &out, &errOut, func(k string) string { return env[k] },
+	code := run(context.Background(), nil, input{}, &out, &errOut, func(k string) string { return env[k] },
 		func(_ context.Context, svc *app.Service) error { got = svc; return nil })
 	if code != 0 || got == nil {
 		t.Errorf("tt with no args: exit %d, ui started: %v, stderr %q", code, got != nil, errOut.String())
 	}
 }
 
+func TestTUIErrorsExitNonZero(t *testing.T) {
+	env := testEnv(t)
+	var out, errOut bytes.Buffer
+	code := run(context.Background(), nil, input{}, &out, &errOut, func(k string) string { return env[k] },
+		func(context.Context, *app.Service) error { return errors.New("terminal went away") })
+	if code != 1 || !strings.Contains(errOut.String(), "tt: terminal went away") {
+		t.Errorf("ui error: exit %d, stderr %q", code, errOut.String())
+	}
+
+	// The database can't be opened: its directory is a file.
+	blocker := filepath.Join(t.TempDir(), "not-a-dir")
+	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
+		t.Fatal(err)
+	}
+	env["TT_DB"] = filepath.Join(blocker, "tt.db")
+	errOut.Reset()
+	code = run(context.Background(), nil, input{}, &out, &errOut, func(k string) string { return env[k] }, noUI)
+	if code != 1 || !strings.HasPrefix(errOut.String(), "tt: ") {
+		t.Errorf("open error: exit %d, stderr %q", code, errOut.String())
+	}
+}
+
+func TestAddFromAPrompt(t *testing.T) {
+	env := testEnv(t)
+	in := input{r: strings.NewReader("Reply to Ana! @ana ~s #photos?\nignored second line\n"), terminal: true}
+	r := runWithInput(t, env, in, "add")
+	if r.code != 0 || r.stdout != "added #1 to inbox\n" || r.stderr != "add › " {
+		t.Fatalf("prompted add: %+v", r)
+	}
+	if got := storedTask(t, env, 1); got.Title != "Reply to Ana!" || got.Requester != "ana" || got.Note != "" ||
+		len(got.Tags) != 1 || got.Tags[0] != "photos" {
+		t.Errorf("stored: %+v", got)
+	}
+	// Nothing typed (or ctrl+d): usage, nothing stored.
+	if r := runWithInput(t, env, input{r: strings.NewReader("   \n"), terminal: true}, "add"); r.code != 2 || !strings.Contains(r.stderr, "usage:") {
+		t.Errorf("empty prompt: %+v", r)
+	}
+}
+
+func TestAddFromAPipe(t *testing.T) {
+	env := testEnv(t)
+	in := input{r: strings.NewReader("Look at the schema !1 ~m -- from Slack\r\nAna: can you check\r\n\r\nthe migration?\r\n")}
+	if r := runWithInput(t, env, in, "add"); r.code != 0 || r.stdout != "added #1 to backlog\n" || r.stderr != "" {
+		t.Fatalf("piped add: %+v", r)
+	}
+	if got := storedTask(t, env, 1); got.Title != "Look at the schema" || got.Note != "from Slack\nAna: can you check\n\nthe migration?" {
+		t.Errorf("stored: %+v", got)
+	}
+	if r := runWithInput(t, env, input{r: strings.NewReader("\nbody without a title\n")}, "add"); r.code != 2 || !strings.Contains(r.stderr, "usage:") {
+		t.Errorf("empty first line: %+v", r)
+	}
+	// A line on the command line wins; stdin isn't read.
+	if r := runWithInput(t, env, input{r: strings.NewReader("from stdin")}, "add", "from args"); r.code != 0 {
+		t.Fatalf("args add: %+v", r)
+	}
+	if got := storedTask(t, env, 2); got.Title != "from args" {
+		t.Errorf("stored: %+v", got)
+	}
+}
+
 func TestHelp(t *testing.T) {
 	env := testEnv(t)
 	for _, arg := range []string{"-h", "--help", "help"} {
```

Patch `internal/dsl/add_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/dsl/add_test.go b/internal/dsl/add_test.go
index 0a88118..70d18cf 100644
--- a/internal/dsl/add_test.go
+++ b/internal/dsl/add_test.go
@@ -97,3 +97,31 @@ func TestFormatAddRoundTrip(t *testing.T) {
 		}
 	}
 }
+
+func TestWithNoteLines(t *testing.T) {
+	cases := []struct {
+		line string
+		rest []string
+		want Draft
+	}{
+		{"Reply to Ana @ana", nil, Draft{Title: "Reply to Ana", Requester: "ana"}},
+		{"Reply to Ana @ana", []string{"", "  "}, Draft{Title: "Reply to Ana", Requester: "ana"}},
+		{"Reply to Ana @ana", []string{"Hey, can you look at", "", "the schema?"},
+			Draft{Title: "Reply to Ana", Requester: "ana", Note: "Hey, can you look at\n\nthe schema?", HasNote: true}},
+		{"Reply -- context first", []string{"pasted body", ""},
+			Draft{Title: "Reply", Note: "context first\npasted body", HasNote: true}},
+		{"Reply --", []string{"body"}, Draft{Title: "Reply", Note: "body", HasNote: true}},
+		// Tokens in the extra lines stay in the note.
+		{"Ship it", []string{"#not-a-tag !1 @nobody"}, Draft{Title: "Ship it", Note: "#not-a-tag !1 @nobody", HasNote: true}},
+	}
+	for _, c := range cases {
+		got, err := ParseAdd(WithNoteLines(c.line, c.rest), 3)
+		if err != nil {
+			t.Errorf("WithNoteLines(%q, %q): %v", c.line, c.rest, err)
+			continue
+		}
+		if diff := cmp.Diff(c.want, got, cmp.Comparer(tagsEqual)); diff != "" {
+			t.Errorf("WithNoteLines(%q, %q) (-want +got):\n%s", c.line, c.rest, diff)
+		}
+	}
+}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/dsl/ ./cmd/tt/`
Expected: FAIL: `undefined: WithNoteLines`; `undefined: input` / too many arguments to `run`.

- [ ] **Step 3: Implement**

Patch `cmd/tt/main.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/cmd/tt/main.go b/cmd/tt/main.go
index 18728fd..f443c91 100644
--- a/cmd/tt/main.go
+++ b/cmd/tt/main.go
@@ -2,6 +2,7 @@
 package main
 
 import (
+	"bufio"
 	"context"
 	"fmt"
 	"io"
@@ -14,6 +15,7 @@ import (
 	"tt/internal/app"
 	"tt/internal/config"
 	"tt/internal/domain"
+	"tt/internal/dsl"
 	"tt/internal/store"
 	"tt/internal/tui"
 )
@@ -22,6 +24,8 @@ const usage = `usage:
   tt               open the planner (TUI)
   tt add <line>    capture a task, e.g. tt add 'Reply to Ana @ana #photos !1 ~s -- before Friday'
                    quote the line with single quotes: ! and ~ are special in zsh/bash
+  tt add           no line: type it at the add › prompt (no quoting needed), or pipe it in;
+                   piped lines after the first become the note, e.g. pbpaste | tt add
 
 environment:
   TT_CONFIG   config file (default ~/.config/tt/config.toml)
@@ -29,7 +33,43 @@ environment:
 `
 
 func main() {
-	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, os.Getenv, runTUI))
+	in := input{r: os.Stdin}
+	if fi, err := os.Stdin.Stat(); err == nil {
+		in.terminal = fi.Mode()&os.ModeCharDevice != 0
+	}
+	os.Exit(run(context.Background(), os.Args[1:], in, os.Stdout, os.Stderr, os.Getenv, runTUI))
+}
+
+// input is where `tt add` with no line reads from (D33).
+type input struct {
+	r        io.Reader
+	terminal bool // prompt for one line; otherwise read a pipe to the end
+}
+
+// maxPiped bounds what `tt add` reads from a pipe.
+const maxPiped = 1 << 20
+
+// readLine reads the capture line: from a terminal, one line after an
+// "add › " prompt on stderr; from a pipe, line 1 plus lines 2+ as the note.
+func (in input) readLine(stderr io.Writer) (string, error) {
+	if in.terminal {
+		fmt.Fprint(stderr, "add › ")
+		line, err := bufio.NewReader(in.r).ReadString('\n')
+		if err != nil && err != io.EOF {
+			return "", err
+		}
+		return strings.TrimSpace(line), nil
+	}
+	data, err := io.ReadAll(io.LimitReader(in.r, maxPiped))
+	if err != nil {
+		return "", err
+	}
+	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
+	first := strings.TrimSpace(lines[0])
+	if first == "" {
+		return "", nil
+	}
+	return dsl.WithNoteLines(first, lines[1:]), nil
 }
 
 // runTUI runs the full-screen planner until the user quits.
@@ -39,7 +79,7 @@ func runTUI(ctx context.Context, svc *app.Service) error {
 }
 
 // run is main without process globals, so tests can call it directly.
-func run(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string,
+func run(ctx context.Context, args []string, in input, stdout, stderr io.Writer, getenv func(string) string,
 	ui func(context.Context, *app.Service) error) int {
 	switch {
 	case len(args) == 0:
@@ -63,6 +103,13 @@ func run(ctx context.Context, args []string, stdout, stderr io.Writer, getenv fu
 	}
 	// Join the rest so both `tt add "a b"` and `tt add a b` work.
 	line := strings.TrimSpace(strings.Join(args[1:], " "))
+	if len(args) == 1 {
+		var err error
+		if line, err = in.readLine(stderr); err != nil {
+			fmt.Fprintf(stderr, "tt: %v\n", err)
+			return 1
+		}
+	}
 	if line == "" {
 		fmt.Fprint(stderr, usage)
 		return 2
```

Patch `internal/dsl/add.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/dsl/add.go b/internal/dsl/add.go
index d69be20..6890c57 100644
--- a/internal/dsl/add.go
+++ b/internal/dsl/add.go
@@ -139,3 +139,24 @@ func isSize(s string) bool {
 func isURL(s string) bool {
 	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
 }
+
+// WithNoteLines appends rest (extra lines from a multi-line paste or piped
+// stdin, D33) to line as its note: after the line's own " -- " note on a new
+// line, or as a new " -- " note. Trailing blank lines are dropped.
+func WithNoteLines(line string, rest []string) string {
+	for len(rest) > 0 && strings.TrimSpace(rest[len(rest)-1]) == "" {
+		rest = rest[:len(rest)-1]
+	}
+	if len(rest) == 0 {
+		return line
+	}
+	note := strings.Join(rest, "\n")
+	switch head := strings.TrimRight(line, " "); {
+	case strings.HasSuffix(" "+head, " --"): // an empty " --" note: fill it
+		return head + " " + note
+	case strings.Contains(" "+line+" ", noteSep):
+		return line + "\n" + note
+	default:
+		return line + noteSep + note
+	}
+}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/dsl/ ./cmd/tt/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add cmd/tt/main.go cmd/tt/main_test.go internal/dsl/add.go internal/dsl/add_test.go
git commit -m 'feat(cli): tt add reads a prompt or piped stdin; lines 2+ become the note (D33)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 3: Task detail screen

View 5 and D31: `⏎` opens a full-screen detail (header, facts, link, note preview, children with progress), `esc` goes back a level; the keys that act on a task are shared with Pick.

**Files:**
- Modify: `internal/tui/common.go`
- Create: `internal/tui/detail.go`
- Create: `internal/tui/detail_test.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/pick.go`

**Interfaces:**
- Consumes: Task 1 `Service.AddChild` (tests only, via the harness helper `family`); `m.idx.children` (sibling order), `m.idx.cand`, `m.badge`, `ago`, `dayLabel`, `pad`, `styleSelected/Dim/Title/Closed`.
- Produces (used by Tasks 4–6):
  - `screenDetail`; `Model.detail detailState{stack []int64; rows []domain.Task; cursor int; selected int64; offset int}` with `top()`, `refresh(m)`, `move(delta)`, `follow(m)`; `(*Model) openDetail(id)`, `detailKey(key)`, `detailHead()`, `detailView()`, `childLine`, `progress(t)`, `childRows()`.
  - `func (*Model) taskKey(key string, t domain.Task) (tea.Cmd, bool)` in pick.go: `p u x d D` for any task (pinned from `m.idx.cand`); Pick's `⏎` calls `openDetail`.
  - `refreshRows` also refreshes an open detail; `Update` calls `m.detail.follow(m)`; the help overlay now takes the whole screen.
  - Lists show `✎` after a title that has a note.
  - Harness helper `(*harness) family(parentLine string, kids ...string) (domain.Task, []domain.Task)`.

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/detail_test.go`:

```go
package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/config"
	"tt/internal/domain"
)

// family adds a parent with children (each line a child) and reloads.
func (h *harness) family(parentLine string, kids ...string) (domain.Task, []domain.Task) {
	h.t.Helper()
	ctx := context.Background()
	parent := h.add(parentLine)
	var out []domain.Task
	for _, line := range kids {
		k, _, err := h.svc.AddChild(ctx, parent.ID, line)
		if err != nil {
			h.t.Fatal(err)
		}
		out = append(out, k)
	}
	h.keys("R")
	return parent, out
}

func TestDetailShowsTheTaskAndEscGoesBack(t *testing.T) {
	h := newHarness(t, config.Default())
	ctx := context.Background()
	note := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8"
	parent, kids := h.family("Auth migration !1 ~xl @ana #auth https://x.io/pr/482 -- "+note,
		"Token spike ~s", "Rollback plan ~m", "Dry run ~s -- notes here")
	if _, err := h.svc.Done(ctx, kids[0].ID); err != nil {
		t.Fatal(err)
	}
	h.add("Other !2 ~s")
	h.mustSee("[P1 XL] Auth migration ✎ @ana")

	h.keys("enter") // the cursor starts on Auth migration (P1 sorts first)
	h.mustSee("tt · Detail",
		fmt.Sprintf("#%d Auth migration  [P1 XL]", parent.ID),
		"@ana · #auth · captured just now",
		"🔗 https://x.io/pr/482",
		"NOTE", "  line 1", "  line 6", "… +2 lines",
		"CHILDREN ██░░░░░░ 1/3",
		"  [P1 M] Rollback plan", // open children first, in sibling order
		"  [P1 S] Dry run ✎",
		"  [P1 S] Token spike ✓ Mon 12 Oct") // then closed ones with their close date
	h.mustNotSee("line 7", "Other")
	if s := h.screen(); strings.Index(s, "Rollback plan") > strings.Index(s, "Dry run") ||
		strings.Index(s, "Dry run") > strings.Index(s, "Token spike") {
		t.Errorf("children order:\n%s", s)
	}

	// ⏎ on a child opens its detail; esc goes back one level, then to Pick.
	h.keys("j", "j", "enter")
	h.mustSee(fmt.Sprintf("#%d Dry run  [P1 S]", kids[2].ID), fmt.Sprintf("child of #%d Auth migration", parent.ID), "  notes here")
	h.mustNotSee("CHILDREN") // one level only (D6)
	h.keys("esc")
	h.mustSee("CHILDREN")
	if got := h.m.detail.selected; got != kids[2].ID {
		t.Errorf("back on the parent, the cursor is on #%d; want the child we came from (#%d)", got, kids[2].ID)
	}
	h.keys("esc")
	h.mustSee("tt · Pick", "BACKLOG")
	if r, _ := h.m.selectedRow(); r.task.ID != parent.ID {
		t.Errorf("Pick's cursor moved to #%d", r.task.ID)
	}
}

func TestDetailKeysActOnTheRowUnderTheCursor(t *testing.T) {
	h := newHarness(t, config.Default())
	parent, kids := h.family("Parent !1 ~l", "First ~s", "Second ~s")
	h.keys("enter", "j")
	h.keys("x")
	h.mustSee(`✓ "First"`, "  [P1 S] First ✓ Mon 12 Oct", "1/2")
	if got := h.task(kids[0].ID); got.Status != domain.StatusDone {
		t.Fatalf("x on a child: %s", got.Status)
	}
	if h.m.detail.selected != kids[0].ID {
		t.Error("the cursor follows the task into the closed section")
	}
	h.keys("ctrl+z")
	h.mustSee(`undid done "First"`, "0/2")

	h.keys("k") // back on the task itself
	h.keys("p")
	h.mustSee(`pinned "Parent"`, fmt.Sprintf("#%d Parent  [P1 L] 📌", parent.ID))
	h.keys("d")
	h.typeText("handed over")
	h.keys("enter")
	h.mustSee(`dropped "Parent"`, "dropped: handed over")
	h.keys("d")
	h.mustSee("already dropped — x reopens it")
}

func TestDetailFallsBackWhenTheTaskIsGone(t *testing.T) {
	h := newHarness(t, config.Default())
	ctx := context.Background()
	parent, kids := h.family("Parent !1 ~l", "Child ~s")
	h.keys("enter", "j", "enter")
	h.mustSee("child of #1 Parent")

	if err := h.svc.Delete(ctx, kids[0].ID); err != nil { // e.g. from another shell
		t.Fatal(err)
	}
	h.keys("R")
	h.mustSee(fmt.Sprintf("#%d is gone", kids[0].ID), fmt.Sprintf("#%d Parent", parent.ID), "no children")

	if err := h.svc.Delete(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	h.mustSee(fmt.Sprintf("#%d is gone", parent.ID), "tt · Pick")
}

func TestDetailFromTodayAndChildScrolling(t *testing.T) {
	h := newHarness(t, config.Default())
	var kids []string
	for i := 1; i <= 12; i++ {
		kids = append(kids, fmt.Sprintf("Step %02d ~s", i))
	}
	parent, _ := h.family("Big effort !1 ~xl", kids...)
	if _, err := h.svc.Pin(context.Background(), parent.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "tab", "enter") // ⏎ in Today's candidates opens detail too
	h.mustSee("#1 Big effort")

	h.send(tea.WindowSizeMsg{Width: 120, Height: 16})
	for range 12 {
		h.keys("j")
	}
	h.mustSee("Step 12")
	h.mustNotSee("Step 01")
	h.keys("k")
	h.mustSee("Step 12") // the window stays while the cursor moves inside it
	for range 11 {
		h.keys("k")
	}
	h.mustSee("Step 01", "… ")
	h.mustNotSee("Step 12")
}

func TestDetailOfAClosedTaskAndHelp(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.add("Shipped !1 ~s")
	h.clock.now = h.clock.now.Add(time.Hour)
	if _, err := h.svc.Done(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	if err := h.m.applyFilter("is:done"); err != nil {
		t.Fatal(err)
	}
	h.keys("enter")
	h.mustSee("#1 Shipped  [P1 S] ✓ Mon 12 Oct", "no note", "no children")
	h.keys("?")
	h.mustSee("open the child under the cursor")
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL: `h.m.detail undefined`, `undefined: screenDetail`.

- [ ] **Step 3: Implement**

Patch `internal/tui/common.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/common.go b/internal/tui/common.go
index b2c7a4a..af3135d 100644
--- a/internal/tui/common.go
+++ b/internal/tui/common.go
@@ -94,8 +94,11 @@ func (m *Model) closedToday(t domain.Task) bool {
 
 func (m *Model) headerView() string {
 	name := "Pick"
-	if m.screen == screenInbox {
+	switch m.screen {
+	case screenInbox:
 		name = "Inbox"
+	case screenDetail:
+		name = "Detail"
 	}
 	h := styleTitle.Render("tt · "+name) + " · " + dayLabel(m.state.Plan.Day)
 	if n := len(m.idx.inInbox); n > 0 {
@@ -133,7 +136,10 @@ func (m *Model) hintsView() string {
 		}
 		return styleDim.Render(fmt.Sprintf("1-%d priority · s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help", len(m.svc.Config().Priorities)))
 	}
-	hints := "a add · / filter · g group · p pin · u unpin · x done · d drop · tab pane · →/← expand · i inbox · ctrl+z undo · ? help · q quit"
+	if m.screen == screenDetail {
+		return styleDim.Render("j/k move · ⏎ open child · x done · d drop · p pin · u unpin · esc back · a add · ctrl+z undo · ? help")
+	}
+	hints := "⏎ open · a add · / filter · g group · p pin · u unpin · x done · d drop · tab pane · →/← expand · i inbox · ctrl+z undo · ? help · q quit"
 	if m.pick.filtered() && m.overlay == nil { // in the filter line, esc restores the previous filter
 		hints = "esc clears filter · " + hints
 	}
```

Create `internal/tui/detail.go`:

```go
package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"tt/internal/domain"
)

// notePreviewLines is how much of a note Task detail shows (view 5).
const notePreviewLines = 6

// detailState is Task detail (view 5): a stack of open tasks (a child on top
// of its parent) and a cursor over the top task (row 0) and its children.
type detailState struct {
	stack    []int64
	rows     []domain.Task // the task, then its children: open ones in sibling order, then closed ones
	cursor   int
	selected int64 // task id under the cursor; survives reloads
	offset   int   // first child shown (scroll window)
}

func (d *detailState) top() int64 {
	if len(d.stack) == 0 {
		return 0
	}
	return d.stack[len(d.stack)-1]
}

// openDetail shows task id's detail, starting a new stack (from Pick).
func (m *Model) openDetail(id int64) {
	m.detail = detailState{stack: []int64{id}, selected: id}
	m.screen = screenDetail
	m.detail.refresh(m)
}

// refresh rebuilds the rows. A task that disappeared falls back one level
// (to Pick when none is left) and says so.
func (d *detailState) refresh(m *Model) {
	snap := m.state.Snapshot
	for len(d.stack) > 0 {
		if _, ok := snap.Get(d.top()); ok {
			break
		}
		m.setStatus(fmt.Sprintf("#%d is gone", d.top()))
		d.selected = d.top()
		d.stack = d.stack[:len(d.stack)-1]
	}
	if len(d.stack) == 0 {
		d.rows = nil
		if m.screen == screenDetail {
			m.screen = screenPick
		}
		return
	}
	t, _ := snap.Get(d.top())
	var open, closed []domain.Task
	for _, c := range m.idx.children[t.ID] { // sibling order
		if c.Status == domain.StatusOpen {
			open = append(open, c)
		} else {
			closed = append(closed, c)
		}
	}
	d.rows = append(append([]domain.Task{t}, open...), closed...)
	if c := slices.IndexFunc(d.rows, func(r domain.Task) bool { return r.ID == d.selected }); c >= 0 {
		d.cursor = c
	} else {
		d.cursor = min(d.cursor, len(d.rows)-1) // the selected child is gone: stay put
	}
	d.selected = d.rows[d.cursor].ID
}

func (d *detailState) move(delta int) {
	d.cursor = max(0, min(d.cursor+delta, len(d.rows)-1))
	d.selected = d.rows[d.cursor].ID
}

// childRows is how many children fit below the task's own lines.
func (m *Model) childRows() int {
	reserved := 4 + len(m.detailHead())
	if m.banner != nil {
		reserved++
	}
	if m.overlay != nil {
		reserved += strings.Count(m.overlay.view(m), "\n") + 1
	}
	return max(m.height-reserved, 3)
}

// follow keeps the cursor's child inside the scroll window (see pickState.follow).
func (d *detailState) follow(m *Model) {
	if len(d.rows) == 0 {
		return
	}
	visible, kids := m.childRows(), len(d.rows)-1
	c := d.cursor - 1 // the cursor's child index; -1 on the task itself
	off := min(d.offset, max(c, 0))
	if c >= off+visible {
		off = c - visible + 1
	}
	d.offset = max(0, min(off, kids-visible))
}

func (m *Model) detailKey(key string) tea.Cmd {
	d := &m.detail
	switch key {
	case "j", "down":
		d.move(+1)
		return nil
	case "k", "up":
		d.move(-1)
		return nil
	case "esc":
		from := d.top()
		d.stack = d.stack[:len(d.stack)-1]
		if len(d.stack) == 0 {
			m.screen = screenPick
			return nil
		}
		d.selected, d.offset = from, 0 // back on the child we came from
		d.refresh(m)
		return nil
	}
	if len(d.rows) == 0 {
		return nil
	}
	t := d.rows[d.cursor]
	if key == "enter" {
		if d.cursor == 0 {
			return nil
		}
		d.stack = append(d.stack, t.ID)
		d.selected, d.offset = t.ID, 0
		d.refresh(m)
		return nil
	}
	cmd, _ := m.taskKey(key, t)
	return cmd
}

// detailHead renders the task's own lines: header, facts, link, note preview
// and the children heading. The first line is the cursor's row 0.
func (m *Model) detailHead() []string {
	if len(m.detail.rows) == 0 {
		return nil
	}
	t := m.detail.rows[0]
	snap := m.state.Snapshot
	head := fmt.Sprintf("#%d %s  %s", t.ID, t.Title, m.badge(t))
	if t.RolloverCount > 0 {
		head += fmt.Sprintf(" ↻%d", t.RolloverCount)
	}
	if c, ok := m.idx.cand[t.ID]; ok {
		head += " 📌"
		if c.PinnedDays > 1 {
			head += fmt.Sprint(c.PinnedDays)
		}
	}
	switch t.Status {
	case domain.StatusDone:
		head += " ✓ " + dayLabel(m.svc.DayOf(t.ClosedAt))
	case domain.StatusDropped:
		head += " ✗ " + dayLabel(m.svc.DayOf(t.ClosedAt))
	}
	lines := []string{head}

	var facts []string
	if p := snap.ParentOf(t); p != nil {
		facts = append(facts, fmt.Sprintf("child of #%d %s", p.ID, p.Title))
	}
	if t.Requester != "" {
		facts = append(facts, "@"+t.Requester)
	}
	if tags := domain.EffectiveTags(t, snap.ParentOf(t)); len(tags) > 0 {
		facts = append(facts, "#"+strings.Join(tags, " #"))
	}
	facts = append(facts, "captured "+ago(m.svc.Now(), t.CreatedAt))
	if t.Status == domain.StatusDropped && t.DropReason != "" {
		facts = append(facts, "dropped: "+t.DropReason)
	}
	lines = append(lines, styleDim.Render(strings.Join(facts, " · ")))
	if t.Link != "" {
		lines = append(lines, "🔗 "+t.Link)
	}

	lines = append(lines, "", styleTitle.Render("NOTE"))
	if t.Note == "" {
		lines = append(lines, styleDim.Render("  no note"))
	} else {
		note := strings.Split(t.Note, "\n")
		for _, l := range note[:min(len(note), notePreviewLines)] {
			lines = append(lines, "  "+l)
		}
		if extra := len(note) - notePreviewLines; extra > 0 {
			lines = append(lines, styleDim.Render(fmt.Sprintf("  … +%d lines", extra)))
		}
	}

	if t.ParentID == 0 {
		lines = append(lines, "", styleTitle.Render("CHILDREN")+m.progress(t))
	}
	return lines
}

// progress renders " ████░░░░ 2/4" for a parent; "" when nothing counts.
func (m *Model) progress(t domain.Task) string {
	done, total := 0, 0
	for _, k := range m.idx.children[t.ID] {
		if k.Status == domain.StatusDropped {
			continue
		}
		total++
		if k.Status == domain.StatusDone {
			done++
		}
	}
	if total == 0 {
		return ""
	}
	const cells = 8
	filled := done * cells / total
	return fmt.Sprintf(" %s%s %d/%d", strings.Repeat("█", filled), strings.Repeat("░", cells-filled), done, total)
}

func (m *Model) detailView() string {
	d := &m.detail
	head := m.detailHead()
	if len(head) == 0 {
		return ""
	}
	if d.cursor == 0 {
		head[0] = styleSelected.Render(pad(head[0], m.width))
	}
	lines := head
	if d.rows[0].ParentID == 0 {
		kids := d.rows[1:]
		if len(kids) == 0 {
			lines = append(lines, styleDim.Render("  no children"))
		}
		visible := m.childRows()
		for i := d.offset; i < len(kids) && i < d.offset+visible; i++ {
			lines = append(lines, m.childLine(kids[i], d.cursor == i+1))
		}
		if more := len(kids) - d.offset - visible; more > 0 {
			lines = append(lines, styleDim.Render(fmt.Sprintf("  … %d more", more)))
		}
	}
	return strings.Join(lines, "\n")
}

func (m *Model) childLine(c domain.Task, selected bool) string {
	line := "  " + m.badge(c) + " " + c.Title
	if c.Note != "" {
		line += " ✎"
	}
	if _, pinned := m.idx.cand[c.ID]; pinned {
		line += " 📌"
	}
	switch c.Status {
	case domain.StatusDone:
		line += " ✓ " + dayLabel(m.svc.DayOf(c.ClosedAt))
	case domain.StatusDropped:
		line += " ✗ " + dayLabel(m.svc.DayOf(c.ClosedAt))
	}
	line = pad(line, m.width)
	if c.Status != domain.StatusOpen {
		line = styleClosed.Render(line)
	}
	if selected {
		line = styleSelected.Render(line)
	}
	return line
}
```

Patch `internal/tui/model.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/model.go b/internal/tui/model.go
index 01b4b59..02c31d0 100644
--- a/internal/tui/model.go
+++ b/internal/tui/model.go
@@ -39,6 +39,7 @@ type screen int
 const (
 	screenPick screen = iota
 	screenInbox
+	screenDetail
 )
 
 const maxUndo = 50
@@ -75,6 +76,7 @@ type Model struct {
 	screen screen
 	pick   pickState
 	inbox  inboxState
+	detail detailState
 
 	overlay     overlay
 	banner      *app.DayResult
@@ -135,6 +137,7 @@ func (m *Model) Init() tea.Cmd {
 func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
 	cmd := m.update(msg)
 	m.pick.follow(m)
+	m.detail.follow(m)
 	return m, cmd
 }
 
@@ -272,8 +275,11 @@ func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
 		m.setStatus("new day — refreshing")
 		return m.startDay()
 	}
-	if m.screen == screenInbox {
+	switch m.screen {
+	case screenInbox:
 		return m.inboxKey(key)
+	case screenDetail:
+		return m.detailKey(key)
 	}
 	return m.pickKey(key)
 }
@@ -344,13 +350,19 @@ func (m *Model) render() string {
 	}
 	var parts []string
 	parts = append(parts, m.headerView())
+	if h, ok := m.overlay.(helpOverlay); ok { // help takes the whole screen
+		return fitWidth(strings.Join(append(parts, h.view(m)), "\n"), m.width)
+	}
 	if m.banner != nil {
 		parts = append(parts, m.bannerView())
 	}
 	fb, filtering := m.overlay.(*filterBar)
-	if m.screen == screenInbox {
+	switch m.screen {
+	case screenInbox:
 		parts = append(parts, m.inboxView())
-	} else {
+	case screenDetail:
+		parts = append(parts, m.detailView())
+	default:
 		if filtering {
 			parts = append(parts, fb.view(m)) // the filter line sits above the backlog
 		}
```

Patch `internal/tui/overlays.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/overlays.go b/internal/tui/overlays.go
index bfa920e..57e63a7 100644
--- a/internal/tui/overlays.go
+++ b/internal/tui/overlays.go
@@ -221,7 +221,16 @@ func (helpOverlay) view(m *Model) string {
 		"R        reload · q quit · ? this help",
 	}
 	var local []string
-	if m.screen == screenInbox {
+	switch m.screen {
+	case screenDetail:
+		local = []string{
+			"j k      move between the task and its children",
+			"⏎        open the child under the cursor · esc back (child → parent → Pick)",
+			"p / u    pin to today / unpin",
+			"x        done (again: reopen)",
+			"d / D    drop with optional reason / delete",
+		}
+	case screenInbox:
 		local = []string{
 			fmt.Sprintf("1-%d      set priority", len(m.svc.Config().Priorities)),
 			"s m l x  set size (x = XL); a card with priority and size leaves the inbox",
@@ -229,8 +238,9 @@ func (helpOverlay) view(m *Model) string {
 			"d / D    drop with optional reason / delete",
 			"esc      back to Pick",
 		}
-	} else {
+	default:
 		local = []string{
+			"⏎        task detail",
 			"tab      switch backlog / today",
 			"j k      move · → ← expand / collapse a parent",
 			"p / u    pin to today / unpin",
```

Patch `internal/tui/pick.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/pick.go b/internal/tui/pick.go
index 382f2d4..c771350 100644
--- a/internal/tui/pick.go
+++ b/internal/tui/pick.go
@@ -64,6 +64,9 @@ func (m *Model) refreshRows() {
 	m.pick.rows[paneBacklog] = m.backlogRows()
 	m.pick.rows[paneToday] = m.todayRows()
 	m.pick.clamp(m)
+	if len(m.detail.stack) > 0 {
+		m.detail.refresh(m)
+	}
 }
 
 // applyFilter makes text the backlog filter (D26a). On a parse error the
@@ -311,8 +314,9 @@ func (m *Model) pickKey(key string) tea.Cmd {
 		return nil
 	}
 	t := r.task
-	label := fmt.Sprintf("%q", t.Title)
 	switch key {
+	case "enter":
+		m.openDetail(t.ID) // D31
 	case "right", "l":
 		if p.focus == paneBacklog && r.parent {
 			p.expanded[t.ID] = true
@@ -327,37 +331,51 @@ func (m *Model) pickKey(key string) tea.Cmd {
 			p.expanded[t.ID] = false
 			m.refreshRows()
 		}
+	default:
+		cmd, _ := m.taskKey(key, t)
+		return cmd
+	}
+	return nil
+}
+
+// taskKey runs the keys that act on one task the same way wherever it is
+// shown (Pick panes, Task detail). ok is false for any other key.
+func (m *Model) taskKey(key string, t domain.Task) (cmd tea.Cmd, ok bool) {
+	svc := m.svc
+	label := fmt.Sprintf("%q", t.Title)
+	_, pinned := m.idx.cand[t.ID]
+	switch key {
 	case "p":
 		if t.Status != domain.StatusOpen {
 			m.setStatus("can't pin a closed task — x reopens it")
-			return nil
+			return nil, true
 		}
-		if r.cand != nil {
+		if pinned {
 			m.setStatus("already pinned")
-			return nil
+			return nil, true
 		}
-		return m.act("pin "+label, "pinned "+label, func(ctx context.Context) (app.Undo, error) { return svc.Pin(ctx, t.ID) })
+		return m.act("pin "+label, "pinned "+label, func(ctx context.Context) (app.Undo, error) { return svc.Pin(ctx, t.ID) }), true
 	case "u":
-		if r.cand == nil {
+		if !pinned {
 			m.setStatus("not pinned")
-			return nil
+			return nil, true
 		}
-		return m.act("unpin "+label, "unpinned "+label, func(ctx context.Context) (app.Undo, error) { return svc.Unpin(ctx, t.ID) })
+		return m.act("unpin "+label, "unpinned "+label, func(ctx context.Context) (app.Undo, error) { return svc.Unpin(ctx, t.ID) }), true
 	case "x":
 		if t.Status != domain.StatusOpen {
-			return m.act("reopen "+label, "reopened "+label, func(ctx context.Context) (app.Undo, error) { return svc.Reopen(ctx, t.ID) })
+			return m.act("reopen "+label, "reopened "+label, func(ctx context.Context) (app.Undo, error) { return svc.Reopen(ctx, t.ID) }), true
 		}
-		return m.act("done "+label, "✓ "+label, func(ctx context.Context) (app.Undo, error) { return svc.Done(ctx, t.ID) })
+		return m.act("done "+label, "✓ "+label, func(ctx context.Context) (app.Undo, error) { return svc.Done(ctx, t.ID) }), true
 	case "d":
 		if t.Status == domain.StatusDropped {
 			m.setStatus("already dropped — x reopens it") // Drop would overwrite the reason
-			return nil
+			return nil, true
 		}
-		return m.openDropPrompt(t)
+		return m.openDropPrompt(t), true
 	case "D":
-		return m.openDeleteConfirm(t)
+		return m.openDeleteConfirm(t), true
 	}
-	return nil
+	return nil, false
 }
 
 func (m *Model) pickView() string {
@@ -429,6 +447,9 @@ func (m *Model) rowLine(r row, width int, selected bool) string {
 		b.WriteString("  ")
 	}
 	b.WriteString(m.badge(t) + " " + t.Title)
+	if t.Note != "" {
+		b.WriteString(" ✎")
+	}
 	if t.Requester != "" {
 		b.WriteString(" @" + t.Requester)
 	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/common.go internal/tui/detail.go internal/tui/detail_test.go internal/tui/model.go internal/tui/overlays.go internal/tui/pick.go
git commit -m 'feat(tui): Task detail screen with children, note preview and shared task keys (D31)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 4: Edit line, note in `$EDITOR`, open link

P1, D29, D29a, D21: `e`, `E`, `o` on the task under the cursor in Pick or Task detail.

**Files:**
- Modify: `internal/tui/common.go`
- Modify: `internal/tui/detail.go`
- Create: `internal/tui/edit.go`
- Create: `internal/tui/edit_test.go`
- Modify: `internal/tui/harness_test.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/pick.go`

**Interfaces:**
- Consumes: Task 1 `Service.ParseEdit`, `Service.Edit`, `Service.SetNoteFrom`; Task 3 `taskKey`, `detailHead`; `tokenInput`, `actionMsg`, `m.act`.
- Produces:
  - `Options.Exec func(c *exec.Cmd, done tea.ExecCallback) tea.Cmd` (nil → `tea.ExecProcess`) and `Options.Open func(ctx, url string) error` (nil → macOS `open`).
  - edit.go: `editLine` overlay + `openEdit(t)`; `editorCommand(getenv, path) *exec.Cmd` ($VISUAL → $EDITOR → vi, arguments allowed); `noteEditedMsg`, `editNote(t)`, `noteEdited(msg)`; `openLink(t)`; `m.exec`, `m.opener`, `openWithSystem`.
  - `taskKey` gains `e`, `E`, `o`.
  - Harness: `h.editor func(path string) error` stands in for the editor; `h.opened []string`, `h.openErr error` for links.

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/edit_test.go`:

```go
package tui

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"tt/internal/config"
)

func TestEditLine(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.add("Old title @ana #a !1 ~s https://x.io -- keep me")

	h.keys("e")
	h.mustSee("edit #1 › Old title @ana #a !1 ~s https://x.io")
	h.typeText(" #b @bob")
	h.keys("enter")
	h.mustSee(`edited "Old title"`)
	if got := h.task(a.ID); got.Requester != "bob" || !slices.Equal(got.Tags, []string{"a", "b"}) || got.Note != "keep me" {
		t.Errorf("edited: %+v", got)
	}
	h.keys("ctrl+z")
	h.mustSee(`undid edit "Old title"`)
	if got := h.task(a.ID); got.Requester != "ana" {
		t.Errorf("after undo: %+v", got)
	}

	// A bad line keeps the overlay and the text.
	h.keys("e")
	h.typeText(" !9")
	h.keys("enter")
	if _, ok := h.m.overlay.(*editLine); !ok {
		t.Fatal("a bad edit must keep the overlay open")
	}
	h.mustSee("edit #1 › Old title @ana #a !1 ~s https://x.io !9", "✗ priority !9 out of range")
	h.keys("esc")
	if got := h.task(a.ID); got.Priority != 1 {
		t.Errorf("a refused edit changed the task: %+v", got)
	}
}

func TestEditLineOfAChildRefusesPriority(t *testing.T) {
	h := newHarness(t, config.Default())
	parent, kids := h.family("Parent !1 ~l", "Child ~s")
	h.keys("enter", "j", "e")
	h.mustSee("edit #2 › Child ~s")
	h.typeText(" !2")
	h.keys("enter")
	h.mustSee("✗ invalid value: children inherit priority from #1; drop !2")
	if _, ok := h.m.overlay.(*editLine); !ok {
		t.Fatal("the overlay stays open")
	}
	h.keys("esc")
	if got := h.task(kids[0].ID); got.Priority != 0 || got.ParentID != parent.ID {
		t.Errorf("child: %+v", got)
	}
}

func TestNoteInTheEditor(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.add("Task !1 ~s -- first line")
	var seen string
	h.editor = func(path string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		seen = string(data)
		return os.WriteFile(path, []byte("rewritten\nwith two lines\n\n"), 0o600)
	}
	h.clock.now = h.clock.now.Add(time.Minute)
	h.keys("E")
	if seen != "first line\n" {
		t.Errorf("the editor got %q", seen)
	}
	h.mustSee(`note saved`)
	if got := h.task(a.ID).Note; got != "rewritten\nwith two lines" {
		t.Errorf("note = %q", got)
	}
	h.keys("ctrl+z")
	h.mustSee(`undid note of "Task"`)
	if got := h.task(a.ID).Note; got != "first line" {
		t.Errorf("after undo: %q", got)
	}

	// Unchanged file → no write.
	before := h.task(a.ID).UpdatedAt
	h.editor = func(string) error { return nil }
	h.keys("E")
	h.mustSee("note unchanged")
	if !h.task(a.ID).UpdatedAt.Equal(before) {
		t.Error("an unchanged note must not be written")
	}

	// Empty file → note cleared.
	h.clock.now = h.clock.now.Add(time.Minute)
	h.editor = func(path string) error { return os.WriteFile(path, nil, 0o600) }
	h.keys("E")
	h.mustSee("note cleared")
	if got := h.task(a.ID).Note; got != "" {
		t.Errorf("cleared note = %q", got)
	}

	// The editor fails → discarded.
	h.editor = func(path string) error {
		os.WriteFile(path, []byte("half-typed"), 0o600)
		return errors.New("exit status 1")
	}
	h.keys("E")
	h.mustSee("✗ editor failed (exit status 1); note unchanged")
	if got := h.task(a.ID).Note; got != "" {
		t.Errorf("a failed editor changed the note: %q", got)
	}
}

func TestNoteChangedElsewhereKeepsYourText(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.add("Task !1 ~s -- original")
	var path string
	h.editor = func(p string) error {
		path = p
		h.clock.now = h.clock.now.Add(time.Minute)
		if _, err := h.svc.SetNote(context.Background(), a.ID, "from another shell"); err != nil {
			return err
		}
		return os.WriteFile(p, []byte("my careful edit\n"), 0o600)
	}
	h.keys("E")
	h.mustSee("✗ note changed since you opened it — your text is in ")
	if got := h.task(a.ID).Note; got != "from another shell" {
		t.Errorf("note overwritten: %q", got)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "my careful edit\n" {
		t.Errorf("the edit must survive in %s: %q, %v", path, data, err)
	}
	os.Remove(path)
	if !strings.Contains(h.screen(), path) {
		t.Errorf("the status names the file:\n%s", h.screen())
	}
}

func TestEditorTempFilesAreRemoved(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("Task !1 ~s -- x")
	var paths []string
	h.editor = func(p string) error {
		paths = append(paths, p)
		return os.WriteFile(p, []byte("y\n"), 0o600)
	}
	h.keys("E")
	h.editor = func(p string) error { paths = append(paths, p); return nil }
	h.keys("E")
	for _, p := range paths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s left behind: %v", p, err)
		}
	}
	if len(paths) != 2 || !strings.Contains(paths[0], "tt-note-1-") || !strings.HasSuffix(paths[0], ".md") {
		t.Errorf("note files: %v", paths)
	}
}

func TestOpenLink(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("With link !1 ~s https://x.io/pr/482")
	h.add("Without !2 ~s")
	h.keys("o")
	h.mustSee("opened https://x.io/pr/482")
	h.keys("j", "o")
	h.mustSee("no link")
	h.openErr = errors.New("no browser")
	h.keys("k", "o")
	h.mustSee("✗ can't open https://x.io/pr/482: no browser")
	if !slices.Equal(h.opened, []string{"https://x.io/pr/482", "https://x.io/pr/482"}) {
		t.Errorf("opened: %v", h.opened)
	}
}

func TestEditorCommand(t *testing.T) {
	env := func(kv ...string) func(string) string {
		m := map[string]string{}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return func(k string) string { return m[k] }
	}
	cases := []struct {
		getenv func(string) string
		want   []string
	}{
		{env("VISUAL", "code --wait", "EDITOR", "nano"), []string{"code", "--wait", "/tmp/n.md"}},
		{env("VISUAL", "  ", "EDITOR", "nvim"), []string{"nvim", "/tmp/n.md"}},
		{env(), []string{"vi", "/tmp/n.md"}},
	}
	for _, c := range cases {
		if got := editorCommand(c.getenv, "/tmp/n.md").Args; !slices.Equal(got, c.want) {
			t.Errorf("editorCommand = %v; want %v", got, c.want)
		}
	}
}

// Task detail points at E for writing or reading the whole note.
func TestDetailNoteHints(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("No note !1 ~s")
	h.add("Long note !2 ~s -- " + strings.Repeat("line\n", 7) + "last")
	h.keys("enter")
	h.mustSee("no note · E to write one")
	h.keys("esc", "j", "enter")
	h.mustSee("… +2 lines · E edit in $EDITOR")
}
```

Patch `internal/tui/harness_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/harness_test.go b/internal/tui/harness_test.go
index 5eba493..1fc1c8e 100644
--- a/internal/tui/harness_test.go
+++ b/internal/tui/harness_test.go
@@ -2,6 +2,7 @@ package tui
 
 import (
 	"context"
+	"os/exec"
 	"path/filepath"
 	"strings"
 	"testing"
@@ -30,6 +31,12 @@ type harness struct {
 	clock *fakeClock
 	path  string
 	quit  bool
+
+	// editor stands in for $EDITOR: it gets the note file's path and may
+	// rewrite it; its error is the editor's exit status.
+	editor  func(path string) error
+	opened  []string // links passed to Open
+	openErr error
 }
 
 func newHarness(t *testing.T, cfg config.Config) *harness {
@@ -42,9 +49,23 @@ func newHarness(t *testing.T, cfg config.Config) *harness {
 	t.Cleanup(func() { st.Close() })
 	clock := &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
 	svc := app.New(st, clock, cfg)
+	h := &harness{t: t, svc: svc, clock: clock, path: path}
 	opts := Options{WatchEvery: time.Second, ClockEvery: 30 * time.Second,
-		Schedule: func(time.Duration, tea.Msg) tea.Cmd { return nil }}
-	h := &harness{t: t, m: New(context.Background(), svc, opts), svc: svc, clock: clock, path: path}
+		Schedule: func(time.Duration, tea.Msg) tea.Cmd { return nil },
+		Exec: func(c *exec.Cmd, done tea.ExecCallback) tea.Cmd {
+			return func() tea.Msg {
+				if h.editor == nil {
+					h.t.Fatal("the editor ran but the test set no h.editor")
+				}
+				return done(h.editor(c.Args[len(c.Args)-1]))
+			}
+		},
+		Open: func(_ context.Context, url string) error {
+			h.opened = append(h.opened, url)
+			return h.openErr
+		},
+	}
+	h.m = New(context.Background(), svc, opts)
 	h.run(h.m.Init())
 	h.send(tea.WindowSizeMsg{Width: 140, Height: 40})
 	if h.m.banner == nil {
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL: `undefined: editLine`, `undefined: editorCommand`, unknown field `Exec` in `Options`.

- [ ] **Step 3: Implement**

Patch `internal/tui/common.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/common.go b/internal/tui/common.go
index af3135d..f3c26c9 100644
--- a/internal/tui/common.go
+++ b/internal/tui/common.go
@@ -137,9 +137,9 @@ func (m *Model) hintsView() string {
 		return styleDim.Render(fmt.Sprintf("1-%d priority · s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help", len(m.svc.Config().Priorities)))
 	}
 	if m.screen == screenDetail {
-		return styleDim.Render("j/k move · ⏎ open child · x done · d drop · p pin · u unpin · esc back · a add · ctrl+z undo · ? help")
+		return styleDim.Render("j/k move · ⏎ open child · e edit · E note · o link · x done · d drop · p pin · esc back · ctrl+z undo · ? help")
 	}
-	hints := "⏎ open · a add · / filter · g group · p pin · u unpin · x done · d drop · tab pane · →/← expand · i inbox · ctrl+z undo · ? help · q quit"
+	hints := "⏎ open · e edit · a add · / filter · g group · p pin · u unpin · x done · d drop · tab pane · →/← expand · i inbox · ctrl+z undo · ? help · q quit"
 	if m.pick.filtered() && m.overlay == nil { // in the filter line, esc restores the previous filter
 		hints = "esc clears filter · " + hints
 	}
```

Patch `internal/tui/detail.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/detail.go b/internal/tui/detail.go
index e7e50de..cc910b8 100644
--- a/internal/tui/detail.go
+++ b/internal/tui/detail.go
@@ -189,14 +189,14 @@ func (m *Model) detailHead() []string {
 
 	lines = append(lines, "", styleTitle.Render("NOTE"))
 	if t.Note == "" {
-		lines = append(lines, styleDim.Render("  no note"))
+		lines = append(lines, styleDim.Render("  no note · E to write one"))
 	} else {
 		note := strings.Split(t.Note, "\n")
 		for _, l := range note[:min(len(note), notePreviewLines)] {
 			lines = append(lines, "  "+l)
 		}
 		if extra := len(note) - notePreviewLines; extra > 0 {
-			lines = append(lines, styleDim.Render(fmt.Sprintf("  … +%d lines", extra)))
+			lines = append(lines, styleDim.Render(fmt.Sprintf("  … +%d lines · E edit in $EDITOR", extra)))
 		}
 	}
 
```

Create `internal/tui/edit.go`:

```go
package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"tt/internal/app"
	"tt/internal/domain"
)

// editLine is the `e` overlay (P1): the task's token line, prefilled.
type editLine struct {
	in   tokenInput
	task domain.Task
	err  error // why the last enter was refused; shown under the input
}

func (m *Model) openEdit(t domain.Task) tea.Cmd {
	in := newTokenInput(fmt.Sprintf("edit #%d › ", t.ID), "title @who #tag !1 ~s link -- note (replaces it)", m.width)
	in.input.SetValue(app.EditLine(t))
	in.input.CursorEnd()
	m.overlay = &editLine{in: in, task: t}
	return nil
}

func (e *editLine) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.overlay = nil
		return nil
	case "tab":
		if s := e.in.suggestions(m); len(s) > 0 {
			e.in.complete(s[0])
		}
		return nil
	case "enter":
		line := e.in.value()
		if _, err := m.svc.ParseEdit(e.task, line); err != nil {
			e.err = err
			return nil
		}
		m.overlay = nil
		svc, id, label := m.svc, e.task.ID, fmt.Sprintf("%q", e.task.Title)
		return m.act("edit "+label, "edited "+label, func(ctx context.Context) (app.Undo, error) { return svc.Edit(ctx, id, line) })
	}
	e.err = nil
	return e.in.update(k)
}

func (e *editLine) paste(_ *Model, msg tea.PasteMsg) tea.Cmd { return e.in.update(msg) }

func (e *editLine) resize(width int) { e.in.resize(width) }

func (e *editLine) view(m *Model) string { return e.in.view(m, e.err) }

// editorCommand is $VISUAL, else $EDITOR, else vi, on path (D29). The
// variable may carry arguments, e.g. "code --wait".
func editorCommand(getenv func(string) string, path string) *exec.Cmd {
	editor := getenv("VISUAL")
	if strings.TrimSpace(editor) == "" {
		editor = getenv("EDITOR")
	}
	if strings.TrimSpace(editor) == "" {
		editor = "vi"
	}
	f := strings.Fields(editor)
	return exec.Command(f[0], append(f[1:], path)...)
}

// noteEditedMsg reports that the editor exited.
type noteEditedMsg struct {
	task domain.Task
	from string // the note the file started with
	path string
	err  error
}

// editNote opens t's note in the user's editor (D29): the TUI is suspended
// until the editor exits.
func (m *Model) editNote(t domain.Task) tea.Cmd {
	f, err := os.CreateTemp("", fmt.Sprintf("tt-note-%d-*.md", t.ID))
	if err != nil {
		m.setErr(fmt.Errorf("can't create the note file: %w", err))
		return nil
	}
	body := t.Note
	if body != "" {
		body += "\n"
	}
	_, werr := f.WriteString(body)
	if err := errors.Join(werr, f.Close()); err != nil {
		os.Remove(f.Name())
		m.setErr(fmt.Errorf("can't write the note file: %w", err))
		return nil
	}
	path, from := f.Name(), t.Note
	return m.exec(editorCommand(os.Getenv, path), func(err error) tea.Msg {
		return noteEditedMsg{task: t, from: from, path: path, err: err}
	})
}

// noteEdited saves what the editor left: unchanged → no write; empty →
// cleared; a failed editor → discarded. A note that changed elsewhere in the
// meantime is not overwritten; the text stays in the file (D29a).
func (m *Model) noteEdited(msg noteEditedMsg) tea.Cmd {
	if msg.err != nil {
		os.Remove(msg.path)
		m.setErr(fmt.Errorf("editor failed (%v); note unchanged", msg.err))
		return nil
	}
	data, err := os.ReadFile(msg.path)
	if err != nil {
		m.setErr(fmt.Errorf("can't read the note file %s: %w", msg.path, err))
		return nil
	}
	note := strings.TrimRight(string(data), "\n")
	if note == strings.TrimRight(msg.from, "\n") {
		os.Remove(msg.path)
		m.setStatus("note unchanged")
		return nil
	}
	svc, ctx, t, path := m.svc, m.ctx, msg.task, msg.path
	label, status := fmt.Sprintf("note of %q", t.Title), "note saved"
	if note == "" {
		status = "note cleared"
	}
	return func() tea.Msg {
		undo, err := svc.SetNoteFrom(ctx, t.ID, msg.from, note)
		if errors.Is(err, app.ErrConflict) {
			err = errors.New("note changed since you opened it")
		}
		if err != nil {
			return actionMsg{err: fmt.Errorf("%w — your text is in %s", err, path)}
		}
		os.Remove(path)
		return actionMsg{label: label, status: status, undo: undo}
	}
}

// openLink opens t's link in the browser (D21).
func (m *Model) openLink(t domain.Task) tea.Cmd {
	if t.Link == "" {
		m.setStatus("no link")
		return nil
	}
	open, ctx, link := m.opener(), m.ctx, t.Link
	return func() tea.Msg {
		if err := open(ctx, link); err != nil {
			return actionMsg{err: fmt.Errorf("can't open %s: %w", link, err)}
		}
		return actionMsg{status: "opened " + link}
	}
}

func (m *Model) exec(c *exec.Cmd, done tea.ExecCallback) tea.Cmd {
	if m.opts.Exec != nil {
		return m.opts.Exec(c, done)
	}
	return tea.ExecProcess(c, done)
}

func (m *Model) opener() func(context.Context, string) error {
	if m.opts.Open != nil {
		return m.opts.Open
	}
	return openWithSystem
}

// openWithSystem hands url to macOS `open`.
func openWithSystem(ctx context.Context, url string) error {
	out, err := exec.CommandContext(ctx, "open", url).CombinedOutput()
	if err != nil && len(out) > 0 {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return err
}
```

Patch `internal/tui/model.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/model.go b/internal/tui/model.go
index 02c31d0..511fb8f 100644
--- a/internal/tui/model.go
+++ b/internal/tui/model.go
@@ -6,6 +6,7 @@ import (
 	"context"
 	"errors"
 	"fmt"
+	"os/exec"
 	"strings"
 	"time"
 
@@ -15,13 +16,18 @@ import (
 	"tt/internal/search"
 )
 
-// Options tunes the background loops.
+// Options tunes the background loops and the outside programs the TUI runs.
 type Options struct {
 	WatchEvery time.Duration // how often to check for writes from other processes (D13)
 	ClockEvery time.Duration // how often to check for a new planning day (D19)
 	// Schedule returns a command that delivers msg after d. Tests replace it
 	// so Update round-trips never wait on timers.
 	Schedule func(d time.Duration, msg tea.Msg) tea.Cmd
+	// Exec hands the terminal to c (the note editor, D29) and reports its
+	// exit through done. nil means tea.ExecProcess; tests edit in-process.
+	Exec func(c *exec.Cmd, done tea.ExecCallback) tea.Cmd
+	// Open opens a link (D21). nil means macOS `open`.
+	Open func(ctx context.Context, url string) error
 }
 
 func DefaultOptions() Options {
@@ -219,6 +225,8 @@ func (m *Model) update(msg tea.Msg) tea.Cmd {
 		if msg.err != nil {
 			m.setErr(fmt.Errorf("can't remember the grouping: %w", msg.err))
 		}
+	case noteEditedMsg:
+		return m.noteEdited(msg)
 	case tea.KeyPressMsg:
 		return m.handleKey(msg)
 	case tea.PasteMsg:
```

Patch `internal/tui/overlays.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/overlays.go b/internal/tui/overlays.go
index 57e63a7..09a639d 100644
--- a/internal/tui/overlays.go
+++ b/internal/tui/overlays.go
@@ -226,6 +226,8 @@ func (helpOverlay) view(m *Model) string {
 		local = []string{
 			"j k      move between the task and its children",
 			"⏎        open the child under the cursor · esc back (child → parent → Pick)",
+			"e / E    edit the line (title @who #tag !1 ~s link) / the note in $EDITOR",
+			"o        open the link",
 			"p / u    pin to today / unpin",
 			"x        done (again: reopen)",
 			"d / D    drop with optional reason / delete",
@@ -241,6 +243,7 @@ func (helpOverlay) view(m *Model) string {
 	default:
 		local = []string{
 			"⏎        task detail",
+			"e / E    edit the line / the note in $EDITOR · o open the link",
 			"tab      switch backlog / today",
 			"j k      move · → ← expand / collapse a parent",
 			"p / u    pin to today / unpin",
```

Patch `internal/tui/pick.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/pick.go b/internal/tui/pick.go
index c771350..7d8f14b 100644
--- a/internal/tui/pick.go
+++ b/internal/tui/pick.go
@@ -374,6 +374,12 @@ func (m *Model) taskKey(key string, t domain.Task) (cmd tea.Cmd, ok bool) {
 		return m.openDropPrompt(t), true
 	case "D":
 		return m.openDeleteConfirm(t), true
+	case "e":
+		return m.openEdit(t), true
+	case "E":
+		return m.editNote(t), true
+	case "o":
+		return m.openLink(t), true
 	}
 	return nil, false
 }
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/common.go internal/tui/detail.go internal/tui/edit.go internal/tui/edit_test.go internal/tui/harness_test.go internal/tui/model.go internal/tui/overlays.go internal/tui/pick.go
git commit -m 'feat(tui): e edit line, E note in the editor, o open link (D21, D29, D29a, P1)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 5: Add children, reorder, multi-line paste

View 5 and D32/D33: `a` in Task detail is a repeating add-child input, `J/K` reorder, and a multi-line paste keeps lines 2+ as the note.

**Files:**
- Create: `internal/tui/children_test.go`
- Modify: `internal/tui/common.go`
- Modify: `internal/tui/detail.go`
- Modify: `internal/tui/edit.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/tokeninput.go`

**Interfaces:**
- Consumes: Task 1 `Service.ParseChild`, `AddChild`, `Reorder`, `ErrNoMove`; Task 2 `dsl.WithNoteLines`; Task 3 `detailKey`, `detail.rows`; Task 4 edit.go.
- Produces:
  - `tokenInput.extra []string`, `pasteLines(msg)`, `line()`, `reset()`, `noteLines()`; `view` shows `+ N lines → note`. Quick-add and add-child paste through `pasteLines`; the filter bar doesn't.
  - edit.go: `addChild` overlay + `openAddChild()`, `moveChild(t, delta)` ("already first/last").
  - Global `a` opens add-child while Task detail is shown (quick-add elsewhere); `J/K` in `detailKey`.

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/children_test.go`:

```go
package tui

import (
	"context"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"tt/internal/config"
	"tt/internal/domain"
)

// childIDs lists parentID's children in the order Task detail shows them.
func (h *harness) childIDs(parentID int64) []int64 {
	var ids []int64
	for _, c := range h.m.idx.children[parentID] {
		ids = append(ids, c.ID)
	}
	return ids
}

func TestAddChildrenOneAfterAnother(t *testing.T) {
	h := newHarness(t, config.Default())
	parent := h.add("Auth migration !1 ~xl #auth")
	h.keys("enter")
	h.mustSee("no children · a adds one")

	h.keys("a")
	h.mustSee("child of #1 › ")
	h.typeText("Token spike ~s")
	h.keys("enter")
	h.mustSee("added #2 under #1", "CHILDREN ░░░░░░░░ 0/1", "  [P1 S] Token spike")
	if _, ok := h.m.overlay.(*addChild); !ok {
		t.Fatal("the add-child input stays open for the next child")
	}
	h.mustSee("child of #1 › title ~s #tag") // emptied: the placeholder shows
	h.typeText("Rollback plan ~m #ops -- ask Ana first")
	h.keys("enter")
	h.mustSee("added #3 under #1", "0/2")

	h.typeText("Urgent bit !1 ~s") // priority is the parent's (D32)
	h.keys("enter")
	h.mustSee("✗ invalid value: children inherit priority from #1; drop !1", "child of #1 › Urgent bit !1 ~s")
	h.keys("esc")
	if h.m.overlay != nil {
		t.Fatal("esc closes the input")
	}

	ids := h.childIDs(parent.ID)
	if !slices.Equal(ids, []int64{2, 3}) {
		t.Fatalf("children: %v", ids)
	}
	got := h.task(3)
	if got.Size != domain.SizeM || !slices.Equal(got.Tags, []string{"ops"}) || got.Note != "ask Ana first" {
		t.Errorf("second child: %+v", got)
	}
	if domain.InInbox(got, &parent) {
		t.Error("a sized child skips the Inbox")
	}

	h.keys("ctrl+z") // undo the last added child
	h.mustSee(`undid add "Rollback plan"`)
	if ids := h.childIDs(parent.ID); !slices.Equal(ids, []int64{2}) {
		t.Errorf("after undo: %v", ids)
	}

	// A child's detail can't take children (D6); a outside detail is quick-add.
	h.keys("j", "enter", "a")
	h.mustSee("one level only — esc to the parent")
	h.keys("esc", "esc", "a")
	h.mustSee("add › ")
}

func TestReorderChildren(t *testing.T) {
	h := newHarness(t, config.Default())
	parent, kids := h.family("Parent !1 ~l", "A ~s", "B ~s", "C ~s")
	a, b, c := kids[0].ID, kids[1].ID, kids[2].ID
	h.keys("enter", "J")
	h.mustSee("J/K move a child · j selects one")

	h.keys("j", "J") // A down
	h.mustSee(`moved "A"`)
	if ids := h.childIDs(parent.ID); !slices.Equal(ids, []int64{b, a, c}) {
		t.Fatalf("after A down: %v", ids)
	}
	if h.m.detail.selected != a {
		t.Error("the cursor stays on the moved child")
	}
	h.keys("K", "K")
	h.mustSee("already first")
	if ids := h.childIDs(parent.ID); !slices.Equal(ids, []int64{a, b, c}) {
		t.Fatalf("after A up: %v", ids)
	}
	h.keys("ctrl+z") // undoes the last move (A up)
	if ids := h.childIDs(parent.ID); !slices.Equal(ids, []int64{b, a, c}) {
		t.Errorf("after undo: %v", ids)
	}

	// Closed children stay below the open ones; moves skip them.
	if _, err := h.svc.Done(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "K")
	h.mustSee("already first") // B before A is closed: A is the first open child
	h.keys("J")
	h.mustSee(`moved "A"`)
	if ids := h.childIDs(parent.ID); !slices.Equal(ids, []int64{b, c, a}) {
		t.Errorf("A down past C: %v", ids)
	}
	if s := h.screen(); !(strings.Index(s, "] C") < strings.Index(s, "] A") && strings.Index(s, "] A") < strings.Index(s, "] B ✓")) {
		t.Errorf("open children in order, then closed ones:\n%s", s)
	}
}

func TestMultiLinePasteBecomesTheNote(t *testing.T) {
	h := newHarness(t, config.Default())
	h.keys("a")
	h.send(tea.PasteMsg{Content: "Look at the schema @ana !1 ~s\r\nAna: can you check\r\n\r\nthe migration?\r\n"})
	h.mustSee("add › Look at the schema @ana !1 ~s", "+ 3 lines → note")
	h.keys("enter")
	h.mustSee("added #1 to backlog")
	if got := h.task(1); got.Title != "Look at the schema" || got.Note != "Ana: can you check\n\nthe migration?" {
		t.Errorf("pasted task: %+v", got)
	}

	// esc drops the held lines with the rest of the input.
	h.keys("a")
	h.send(tea.PasteMsg{Content: "Draft\nbody"})
	h.keys("esc", "a")
	h.mustNotSee("→ note")
	h.keys("esc")

	// Add-child takes pastes the same way.
	h.keys("enter", "a")
	h.send(tea.PasteMsg{Content: "Child ~s\nwith details"})
	h.mustSee("+ 1 line → note")
	h.keys("enter")
	if got := h.task(2); got.ParentID != 1 || got.Note != "with details" {
		t.Errorf("pasted child: %+v", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL: `undefined: addChild`.

- [ ] **Step 3: Implement**

Patch `internal/tui/common.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/common.go b/internal/tui/common.go
index f3c26c9..208e69a 100644
--- a/internal/tui/common.go
+++ b/internal/tui/common.go
@@ -137,7 +137,7 @@ func (m *Model) hintsView() string {
 		return styleDim.Render(fmt.Sprintf("1-%d priority · s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help", len(m.svc.Config().Priorities)))
 	}
 	if m.screen == screenDetail {
-		return styleDim.Render("j/k move · ⏎ open child · e edit · E note · o link · x done · d drop · p pin · esc back · ctrl+z undo · ? help")
+		return styleDim.Render("j/k move · ⏎ open child · a add child · J/K reorder · e edit · E note · o link · x done · d drop · p pin · esc back · ? help")
 	}
 	hints := "⏎ open · e edit · a add · / filter · g group · p pin · u unpin · x done · d drop · tab pane · →/← expand · i inbox · ctrl+z undo · ? help · q quit"
 	if m.pick.filtered() && m.overlay == nil { // in the filter line, esc restores the previous filter
```

Patch `internal/tui/detail.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/detail.go b/internal/tui/detail.go
index cc910b8..1048112 100644
--- a/internal/tui/detail.go
+++ b/internal/tui/detail.go
@@ -129,6 +129,17 @@ func (m *Model) detailKey(key string) tea.Cmd {
 		return nil
 	}
 	t := d.rows[d.cursor]
+	if key == "J" || key == "K" {
+		if d.cursor == 0 {
+			m.setStatus("J/K move a child · j selects one")
+			return nil
+		}
+		delta := +1
+		if key == "K" {
+			delta = -1
+		}
+		return m.moveChild(t, delta)
+	}
 	if key == "enter" {
 		if d.cursor == 0 {
 			return nil
@@ -239,7 +250,7 @@ func (m *Model) detailView() string {
 	if d.rows[0].ParentID == 0 {
 		kids := d.rows[1:]
 		if len(kids) == 0 {
-			lines = append(lines, styleDim.Render("  no children"))
+			lines = append(lines, styleDim.Render("  no children · a adds one"))
 		}
 		visible := m.childRows()
 		for i := d.offset; i < len(kids) && i < d.offset+visible; i++ {
```

Patch `internal/tui/edit.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/edit.go b/internal/tui/edit.go
index 7fdc110..32caf73 100644
--- a/internal/tui/edit.go
+++ b/internal/tui/edit.go
@@ -59,6 +59,73 @@ func (e *editLine) resize(width int) { e.in.resize(width) }
 
 func (e *editLine) view(m *Model) string { return e.in.view(m, e.err) }
 
+// addChild is the repeating add-child input of Task detail (view 5): each
+// enter adds a child and the input stays open for the next one.
+type addChild struct {
+	in     tokenInput
+	parent domain.Task
+	err    error
+}
+
+func (m *Model) openAddChild() tea.Cmd {
+	t := m.detail.rows[0]
+	if t.ParentID != 0 {
+		m.setStatus("one level only — esc to the parent")
+		return nil
+	}
+	m.overlay = &addChild{in: newTokenInput(fmt.Sprintf("child of #%d › ", t.ID), "title ~s #tag @who -- note", m.width), parent: t}
+	return nil
+}
+
+func (a *addChild) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
+	switch k.String() {
+	case "esc":
+		m.overlay = nil
+		return nil
+	case "tab":
+		if s := a.in.suggestions(m); len(s) > 0 {
+			a.in.complete(s[0])
+		}
+		return nil
+	case "enter":
+		line := a.in.line()
+		if _, err := m.svc.ParseChild(a.parent.ID, line); err != nil {
+			a.err = err
+			return nil
+		}
+		a.in.reset() // ready for the next child
+		svc, ctx, parentID := m.svc, m.ctx, a.parent.ID
+		return func() tea.Msg {
+			c, undo, err := svc.AddChild(ctx, parentID, line)
+			return actionMsg{label: fmt.Sprintf("add %q", c.Title), status: fmt.Sprintf("added #%d under #%d", c.ID, parentID), undo: undo, err: err}
+		}
+	}
+	a.err = nil
+	return a.in.update(k)
+}
+
+func (a *addChild) paste(_ *Model, msg tea.PasteMsg) tea.Cmd { return a.in.pasteLines(msg) }
+
+func (a *addChild) resize(width int) { a.in.resize(width) }
+
+func (a *addChild) view(m *Model) string { return a.in.view(m, a.err) }
+
+// moveChild moves the selected child one place among its siblings.
+func (m *Model) moveChild(t domain.Task, delta int) tea.Cmd {
+	svc, ctx, label := m.svc, m.ctx, fmt.Sprintf("%q", t.Title)
+	return func() tea.Msg {
+		undo, err := svc.Reorder(ctx, t.ID, delta)
+		if errors.Is(err, app.ErrNoMove) {
+			edge := "already last"
+			if delta < 0 {
+				edge = "already first"
+			}
+			return actionMsg{status: edge}
+		}
+		return actionMsg{label: "move " + label, status: "moved " + label, undo: undo, err: err}
+	}
+}
+
 // editorCommand is $VISUAL, else $EDITOR, else vi, on path (D29). The
 // variable may carry arguments, e.g. "code --wait".
 func editorCommand(getenv func(string) string, path string) *exec.Cmd {
```

Patch `internal/tui/model.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/model.go b/internal/tui/model.go
index 511fb8f..b9aeba0 100644
--- a/internal/tui/model.go
+++ b/internal/tui/model.go
@@ -273,6 +273,9 @@ func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
 		m.overlay = helpOverlay{}
 		return nil
 	case "a":
+		if m.screen == screenDetail && len(m.detail.rows) > 0 {
+			return m.openAddChild() // in Task detail, a adds a child (view 5)
+		}
 		return m.openQuickAdd()
 	case "ctrl+z":
 		return m.undoLast()
```

Patch `internal/tui/overlays.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/overlays.go b/internal/tui/overlays.go
index 09a639d..d576a46 100644
--- a/internal/tui/overlays.go
+++ b/internal/tui/overlays.go
@@ -54,7 +54,7 @@ func (q *quickAdd) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
 		}
 		return nil
 	case "enter":
-		line := q.in.value()
+		line := q.in.line()
 		if _, err := dsl.ParseAdd(line, len(m.svc.Config().Priorities)); err != nil {
 			q.err = err
 			return nil
@@ -74,7 +74,7 @@ func (q *quickAdd) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
 	return q.in.update(k)
 }
 
-func (q *quickAdd) paste(_ *Model, msg tea.PasteMsg) tea.Cmd { return q.in.update(msg) }
+func (q *quickAdd) paste(_ *Model, msg tea.PasteMsg) tea.Cmd { return q.in.pasteLines(msg) }
 
 func (q *quickAdd) resize(width int) { q.in.resize(width) }
 
@@ -228,6 +228,8 @@ func (helpOverlay) view(m *Model) string {
 			"⏎        open the child under the cursor · esc back (child → parent → Pick)",
 			"e / E    edit the line (title @who #tag !1 ~s link) / the note in $EDITOR",
 			"o        open the link",
+			"a        add children, one per line (title ~s #tag -- note; priority is the parent's)",
+			"J / K    move the selected child down / up",
 			"p / u    pin to today / unpin",
 			"x        done (again: reopen)",
 			"d / D    drop with optional reason / delete",
```

Patch `internal/tui/tokeninput.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/tokeninput.go b/internal/tui/tokeninput.go
index f27397d..dcbd064 100644
--- a/internal/tui/tokeninput.go
+++ b/internal/tui/tokeninput.go
@@ -1,16 +1,22 @@
 package tui
 
 import (
+	"fmt"
 	"strings"
 
 	"charm.land/bubbles/v2/textinput"
 	tea "charm.land/bubbletea/v2"
+
+	"tt/internal/dsl"
 )
 
 // tokenInput is a one-line input in token syntax (D25, D26a) that completes
-// the @requester or #tag at the cursor from known values. Quick-add and the
-// filter bar share it.
-type tokenInput struct{ input textinput.Model }
+// the @requester or #tag at the cursor from known values. Quick-add, add-child,
+// the edit line and the filter bar share it.
+type tokenInput struct {
+	input textinput.Model
+	extra []string // lines 2+ of a multi-line paste: the note (D33)
+}
 
 func newTokenInput(prompt, placeholder string, width int) tokenInput {
 	return tokenInput{input: newInput(prompt, placeholder, width)}
@@ -26,6 +32,33 @@ func (ti *tokenInput) update(msg tea.Msg) tea.Cmd {
 
 func (ti *tokenInput) resize(width int) { fitInput(&ti.input, width) }
 
+// pasteLines pastes the first line into the input and keeps lines 2+ as
+// the note (D33); line() adds them on submit.
+func (ti *tokenInput) pasteLines(msg tea.PasteMsg) tea.Cmd {
+	lines := strings.Split(strings.ReplaceAll(msg.Content, "\r\n", "\n"), "\n")
+	cmd := ti.update(tea.PasteMsg{Content: lines[0]})
+	ti.extra = append(ti.extra, lines[1:]...)
+	return cmd
+}
+
+// line is what to submit: the typed line plus any pasted note lines.
+func (ti *tokenInput) line() string { return dsl.WithNoteLines(ti.value(), ti.extra) }
+
+// reset empties the input for the next entry.
+func (ti *tokenInput) reset() {
+	ti.input.SetValue("")
+	ti.extra = nil
+}
+
+// noteLines counts the pasted note lines, ignoring trailing blank ones.
+func (ti *tokenInput) noteLines() int {
+	n := len(ti.extra)
+	for n > 0 && strings.TrimSpace(ti.extra[n-1]) == "" {
+		n--
+	}
+	return n
+}
+
 // word returns the value as runes and the bounds [start, end) of the word
 // the cursor is in (or right after).
 func (ti *tokenInput) word() (r []rune, start, end int) {
@@ -81,6 +114,9 @@ func (ti *tokenInput) complete(s string) {
 // suggestions, when there are any.
 func (ti *tokenInput) view(m *Model, err error) string {
 	out := ti.input.View()
+	if n := ti.noteLines(); n > 0 {
+		out += "\n" + styleDim.Render(fmt.Sprintf("  + %s → note", plural(n, "line", "lines")))
+	}
 	if err != nil {
 		out += "\n" + styleError.Render("✗ "+err.Error())
 	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/children_test.go internal/tui/common.go internal/tui/detail.go internal/tui/edit.go internal/tui/model.go internal/tui/overlays.go internal/tui/tokeninput.go
git commit -m 'feat(tui): add children, reorder with J/K, multi-line paste into the note (D32, D33)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 6: Plan 4a carry-forwards

Spec "Carry-forward from Plan 4a review": esc in the filter line restores the cursors; the last grouping choice wins; filtered expand/collapse leaves the unfiltered expansion alone and a parent with no listed children shows no `▸`; completion works after `--` in the filter bar.

**Files:**
- Create: `internal/tui/browse_test.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/pick.go`
- Modify: `internal/tui/tokeninput.go`

**Interfaces:**
- Consumes: Task 5 `tokenInput`; 4a `filterBar`, `pickState`, `uiStateMsg`, `uiSavedMsg`, `groupKey`.
- Produces:
  - `filterBar.prevSel [2]int64`; `tokenInput.plain bool` (set by the filter bar).
  - `pickState.groupChosen`, `saving`, `saveAgain`; `(*Model) saveGroup() tea.Cmd` (one save at a time, latest wins); `uiStateMsg` ignored once `g` was pressed.
  - Backlog rows: `parent` counts listed children only; `backlogRow` helper removed (rows built inline).

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/browse_test.go`:

```go
package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"tt/internal/config"
)

func TestFilterEscRestoresTheSelection(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("Alpha !1 ~s")
	h.add("Beta !1 ~s")
	gamma := h.add("Gamma !1 ~s")
	h.keys("j", "j")
	h.keys("/")
	h.typeText("alp") // live narrowing moves the cursor to Alpha
	h.mustSee("1 match · alp")
	h.keys("esc")
	if r, _ := h.m.selectedRow(); r.task.ID != gamma.ID {
		t.Errorf("after esc the cursor is on %q; want Gamma, where it was before /", r.task.Title)
	}
}

func TestGroupingSavesTheLatestChoice(t *testing.T) {
	h := newHarness(t, config.Default())
	g := tea.KeyPressMsg{Code: 'g', Text: "g"}

	// Two quick g presses: the second save waits for the first, then saves the latest.
	_, first := h.m.Update(g)
	_, second := h.m.Update(g)
	if second != nil {
		t.Fatal("a second save must not run while the first is in flight")
	}
	h.run(first)
	h.mustSee("BACKLOG · by created")
	if v, _ := h.svc.UIState(context.Background(), groupKey); v != "created" {
		t.Errorf("remembered %q; want the latest choice, created", v)
	}

	// The startup read landing after a g press doesn't override it.
	h.send(uiStateMsg{group: "requester"})
	h.mustSee("BACKLOG · by created")
}

func TestFilteredExpandCollapseLeavesTheBacklogAlone(t *testing.T) {
	h := newHarness(t, config.Default())
	parent, _ := h.family("Auth migration !1 ~xl #auth", "Rollback plan ~s", "Token spike ~m")
	h.keys("right") // expanded before filtering
	h.mustSee("▾ [P1 XL] Auth migration", "    [P1 S] Rollback plan")

	if err := h.m.applyFilter("#auth"); err != nil {
		t.Fatal(err)
	}
	h.keys("j", "left") // on a child: the parent takes the cursor
	if r, _ := h.m.selectedRow(); r.task.ID != parent.ID {
		t.Errorf("cursor on %q; want the parent", r.task.Title)
	}
	h.keys("left") // on the parent: filtered parents stay expanded
	h.mustSee("    [P1 S] Rollback plan")
	// Clearing the filter shows the parent still expanded, as before the filter.
	h.keys("esc")
	h.mustSee("▾ [P1 XL] Auth migration", "    [P1 S] Rollback plan")
}

func TestParentWithoutListedChildrenHasNoMarker(t *testing.T) {
	h := newHarness(t, config.Default())
	h.family("Lonely parent !2 ~m", "Untriaged idea") // the child is in the Inbox
	h.mustSee("  [P2 M] Lonely parent")
	h.mustNotSee("▸ [P2 M] Lonely parent")
	h.keys("right")
	h.mustNotSee("Untriaged idea")
}

func TestFilterBarCompletesAfterDashes(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("One @ana !1 ~s")
	h.keys("/")
	h.typeText("-- @an") // in a filter, -- is just a word
	h.mustSee("tab → @ana")
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL: `TestFilterEscRestoresTheSelection`, `TestGroupingSavesTheLatestChoice`, `TestFilteredExpandCollapseLeavesTheBacklogAlone`, `TestParentWithoutListedChildrenHasNoMarker`, `TestFilterBarCompletesAfterDashes` (assertion failures, not build errors).

- [ ] **Step 3: Implement**

Patch `internal/tui/model.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/model.go b/internal/tui/model.go
index b9aeba0..c512f99 100644
--- a/internal/tui/model.go
+++ b/internal/tui/model.go
@@ -217,14 +217,22 @@ func (m *Model) update(msg tea.Msg) tea.Cmd {
 			m.setErr(fmt.Errorf("can't read the remembered grouping: %w", msg.err))
 			return nil
 		}
+		if m.pick.groupChosen {
+			return nil // a g pressed before the read landed wins
+		}
 		m.pick.group = search.ParseGroupBy(msg.group)
 		if m.loaded {
 			m.refreshRows()
 		}
 	case uiSavedMsg:
+		m.pick.saving = false
 		if msg.err != nil {
 			m.setErr(fmt.Errorf("can't remember the grouping: %w", msg.err))
 		}
+		if m.pick.saveAgain {
+			m.pick.saveAgain = false
+			return m.saveGroup()
+		}
 	case noteEditedMsg:
 		return m.noteEdited(msg)
 	case tea.KeyPressMsg:
```

Patch `internal/tui/overlays.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/overlays.go b/internal/tui/overlays.go
index d576a46..b3b77b9 100644
--- a/internal/tui/overlays.go
+++ b/internal/tui/overlays.go
@@ -82,19 +82,21 @@ func (q *quickAdd) view(m *Model) string { return q.in.view(m, q.err) }
 
 // filterBar edits the backlog filter and applies it as you type (D26e).
 type filterBar struct {
-	in   tokenInput
-	prev string // the filter before `/`; esc restores it
-	err  error  // the line doesn't parse; the last valid filter stays applied
+	in      tokenInput
+	prev    string   // the filter before `/`; esc restores it
+	prevSel [2]int64 // and the tasks the cursors were on
+	err     error    // the line doesn't parse; the last valid filter stays applied
 }
 
 func (m *Model) openFilter() tea.Cmd {
 	in := newTokenInput("filter › ", "@who #tag !1 ~s <7d >30d is:done words", m.width)
+	in.plain = true // no " -- " note in a filter: complete everywhere
 	if text := m.pick.filterText; text != "" {
 		in.input.SetValue(text + " ")
 		in.input.CursorEnd()
 	}
 	m.pick.focus = paneBacklog
-	m.overlay = &filterBar{in: in, prev: m.pick.filterText}
+	m.overlay = &filterBar{in: in, prev: m.pick.filterText, prevSel: m.pick.selected}
 	return nil
 }
 
@@ -102,7 +104,8 @@ func (f *filterBar) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
 	switch k.String() {
 	case "esc":
 		m.overlay = nil
-		_ = m.applyFilter(f.prev) // it parsed when it was applied
+		m.pick.selected = f.prevSel // live narrowing moved the cursors
+		_ = m.applyFilter(f.prev)   // it parsed when it was applied
 		return nil
 	case "enter":
 		if f.err == nil {
```

Patch `internal/tui/pick.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/pick.go b/internal/tui/pick.go
index 7d8f14b..939ee24 100644
--- a/internal/tui/pick.go
+++ b/internal/tui/pick.go
@@ -45,15 +45,18 @@ type row struct {
 const groupKey = "pick.group"
 
 type pickState struct {
-	focus      pane
-	cursor     [2]int
-	offset     [2]int   // first row shown in each pane (scroll window)
-	selected   [2]int64 // task id under each pane's cursor (0 = none); survives reloads
-	expanded   map[int64]bool
-	rows       [2][]row // built by refreshRows, read by keys and render
-	group      search.GroupBy
-	filterText string     // the filter as typed (trimmed); "" = none
-	filter     dsl.Filter // parsed filterText
+	focus       pane
+	cursor      [2]int
+	offset      [2]int   // first row shown in each pane (scroll window)
+	selected    [2]int64 // task id under each pane's cursor (0 = none); survives reloads
+	expanded    map[int64]bool
+	rows        [2][]row // built by refreshRows, read by keys and render
+	group       search.GroupBy
+	groupChosen bool       // g was pressed: the remembered grouping read at startup no longer applies
+	saving      bool       // a grouping save is in flight
+	saveAgain   bool       // g was pressed again meanwhile: save the latest when it lands
+	filterText  string     // the filter as typed (trimmed); "" = none
+	filter      dsl.Filter // parsed filterText
 }
 
 func (p *pickState) filtered() bool { return !p.filter.Empty() }
@@ -138,23 +141,20 @@ func (m *Model) backlogRows() []row {
 					kids = append(kids, c)
 				}
 			}
+			// parent: only listed children count, so ▸ always has something to show.
 			open := len(kids) > 0 && (filtered || m.pick.expanded[t.ID])
-			rows = append(rows, m.backlogRow(t, false, open, orphanOf[t.ID]))
+			rows = append(rows, row{kind: rowTask, task: t, parent: len(kids) > 0, expanded: open, orphan: orphanOf[t.ID]})
 			if !open {
 				continue
 			}
 			for _, c := range kids {
-				rows = append(rows, m.backlogRow(c, true, false, 0))
+				rows = append(rows, row{kind: rowTask, task: c, child: true})
 			}
 		}
 	}
 	return rows
 }
 
-func (m *Model) backlogRow(t domain.Task, child, expanded bool, orphan int64) row {
-	return row{kind: rowTask, task: t, child: child, parent: len(m.idx.children[t.ID]) > 0, expanded: expanded, orphan: orphan}
-}
-
 // todayRows: today's candidates in plan order. A pinned parent lists its open
 // children and children closed today beneath it; a pinned child of a pinned
 // parent appears only there.
@@ -235,6 +235,19 @@ func (p *pickState) move(delta int) {
 	}
 }
 
+// saveGroup remembers the grouping (D26). One save runs at a time; a g
+// pressed meanwhile is saved when it lands, so the last choice wins.
+func (m *Model) saveGroup() tea.Cmd {
+	p := &m.pick
+	if p.saving {
+		p.saveAgain = true
+		return nil
+	}
+	p.saving = true
+	svc, ctx, g := m.svc, m.ctx, p.group
+	return func() tea.Msg { return uiSavedMsg{err: svc.SetUIState(ctx, groupKey, string(g))} }
+}
+
 // follow keeps each pane's cursor inside its scroll window, moving the window
 // only when the cursor leaves it; a group heading right above the cursor
 // comes along. Called after every update.
@@ -278,7 +291,6 @@ func (m *Model) selectedRow() (row, bool) {
 
 func (m *Model) pickKey(key string) tea.Cmd {
 	p := &m.pick
-	svc := m.svc
 	switch key {
 	case "tab":
 		p.focus = 1 - p.focus
@@ -297,11 +309,10 @@ func (m *Model) pickKey(key string) tea.Cmd {
 	case "/":
 		return m.openFilter()
 	case "g":
-		p.group = p.group.Next()
+		p.group, p.groupChosen = p.group.Next(), true
 		m.refreshRows()
 		m.setStatus("grouped by " + string(p.group))
-		ctx, g := m.ctx, p.group
-		return func() tea.Msg { return uiSavedMsg{err: svc.SetUIState(ctx, groupKey, string(g))} }
+		return m.saveGroup()
 	case "esc":
 		if p.filtered() {
 			_ = m.applyFilter("") // an empty line always parses
@@ -318,16 +329,20 @@ func (m *Model) pickKey(key string) tea.Cmd {
 	case "enter":
 		m.openDetail(t.ID) // D31
 	case "right", "l":
-		if p.focus == paneBacklog && r.parent {
+		if p.focus == paneBacklog && r.parent && !p.filtered() { // filtered parents are always expanded
 			p.expanded[t.ID] = true
 			m.refreshRows()
 		}
 	case "left", "h":
-		if p.focus == paneBacklog && r.child {
-			p.expanded[t.ParentID] = false
+		switch {
+		case p.focus != paneBacklog:
+		case r.child:
+			if !p.filtered() { // the filter mustn't change the unfiltered expansion
+				p.expanded[t.ParentID] = false
+			}
 			p.selected[paneBacklog] = t.ParentID // the parent takes the cursor
 			m.refreshRows()
-		} else if p.focus == paneBacklog {
+		case !p.filtered():
 			p.expanded[t.ID] = false
 			m.refreshRows()
 		}
```

Patch `internal/tui/tokeninput.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/tokeninput.go b/internal/tui/tokeninput.go
index dcbd064..66b7001 100644
--- a/internal/tui/tokeninput.go
+++ b/internal/tui/tokeninput.go
@@ -16,6 +16,7 @@ import (
 type tokenInput struct {
 	input textinput.Model
 	extra []string // lines 2+ of a multi-line paste: the note (D33)
+	plain bool     // the line has no " -- " note (the filter bar)
 }
 
 func newTokenInput(prompt, placeholder string, width int) tokenInput {
@@ -79,7 +80,7 @@ func (ti *tokenInput) word() (r []rune, start, end int) {
 func (ti *tokenInput) suggestions(m *Model) []string {
 	r, start, _ := ti.word()
 	tok := string(r[start:min(ti.input.Position(), len(r))])
-	if tok == "" || (tok[0] != '@' && tok[0] != '#') || strings.Contains(" "+string(r[:start]), " -- ") {
+	if tok == "" || (tok[0] != '@' && tok[0] != '#') || !ti.plain && strings.Contains(" "+string(r[:start]), " -- ") {
 		return nil
 	}
 	pool := m.idx.tags
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/browse_test.go internal/tui/model.go internal/tui/overlays.go internal/tui/pick.go internal/tui/tokeninput.go
git commit -m 'fix(tui): 4a carry-forwards: filter esc restores the cursor, last grouping wins, filtered expand leaves the backlog alone

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

## After the last task

- Build into the scratchpad and drive the real binary through a Python `pty` script with `EDITOR` set to a small script that appends a line: `tt add` at the prompt (no quoting), `tt add` from a pipe, `⏎` detail, `E` round-trip, `a` two children, `J/K`, `e`, `esc` back to Pick, `q`; check the stripped output and the database. Never run `o` against a real browser.
- `go test -race ./internal/tui/ ./internal/app/`.
- Final whole-branch review, one fix wave, park residuals as spec carry-forwards.
