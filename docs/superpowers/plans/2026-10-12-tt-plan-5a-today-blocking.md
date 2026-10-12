# tt Plan 5a — Today & Blocking Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** tt opens on Today: a timeline of today's blocks beside the candidates, where `b` picks a candidate up, `j/k` move a ghost block and `⏎` drops it, blocks can be moved, resized, fine-tuned and unblocked, and `c` commits the plan; Pick becomes the Log (`L`, `esc`, `B`), and a pinned parent pins its children (D34).

**Architecture:** D34 and `SetBlockSpan` stay in `app` (one transaction each; undo steps through a new `modifyAnd`). The TUI gets a `screenToday` home screen whose timeline is a pure row model (`internal/tui/timeline.go`, rebuilt on every load like the Log's rows) rendered beside the Log's candidates pane, which it shares. Placement is a small "carry" state on Today (a ghost block that follows the cursor until dropped). All screens share one height budget (`bodyHeight`). No calendar code: Plan 5b adds meetings to the same timeline.

**Tech Stack:** Go 1.27; Bubble Tea v2, bubbles v2 textinput, Lip Gloss v2; pure-Go SQLite. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-10-12-task-tracking-design.md` — D12a, D18 (5b), D22, D24, D31, D34 (amended), D35–D38, open items P3/P4, "Plan 5 split" → Plan 5a, "Carry-forward from Plan 2 review" (DST rows), "Carry-forward from Plan 3 review" (shared height budget), "Carry-forward from Plan 4b review" (D34).

## Global Constraints

- No new dependencies: `go.mod` and `go.sum` stay unchanged. No migration in 5a.
- Only `app` mutates state; `internal/tui` and `cmd/tt` call `app.Service`. Every user command returns an undo, except pin/unpin of a task with children (D34, the status says `· no undo`), commit, and hard delete. Undo refuses with `ErrConflict` rather than damaging state.
- D34: unpinning a parent never deletes a child's block: children with a block today stay pinned on their own.
- D36: outside a pick-up, `⏎` opens Task detail (D31). A drop onto another block is refused and the block stays picked up; placing in the past is allowed.
- D37 wording, exactly: `big task · consider splitting` (L and XL only) and `parent task; blocking its children is recommended · ⏎ block anyway · esc`.
- D38: rows are 30 minutes and wall-clock offsets (no DST special case); the window is `[timeline] start`–`end`, stretched to every block and now.
- Lines never wrap: every rendered line is truncated to the terminal width, and a screen's body never exceeds `bodyHeight()`.
- Every task ends green on: `gofmt -l .` (no output), `go vet ./...`, `go test ./...`; Tasks 3–4 also `go test -race ./internal/tui/ ./internal/app/`.
- Every commit message ends with a blank line and `Co-Authored-By: Claude <noreply@anthropic.com>`. Commit messages use single quotes.
- Sandbox: save each patch to a file and `git apply <file>`; run plain, separate git commands (no `git -C`, no `cd … &&`). If `git apply` fails, stop: an earlier step went wrong.

## Review Focus

1. **Late-night work or an early start** (a block at 21:00, now at 06:40): the timeline stretches to show them; nothing is cut off or unplaceable. Pinned in Task 3 `TestTimelineStretchesToBlocksAndNow` (and Task 2 `TestTimelineWindow`).
2. **Dropping, moving, resizing or fine-tuning onto another block**: refused with `overlaps "X" 10:00–11:00`, the pick-up or the typed line stays, nothing is written. Pinned in Task 4 `TestDropOntoABlockIsRefused` and `TestMoveResizeFineTuneUnblock`.
3. **The picked-up block changes elsewhere** (undo, another `tt`, a reload) while it follows the cursor: the move stops and says why, nothing acts on a stale block. Pinned in Task 4 `TestAMovedBlockThatVanishesStopsTheMove`.
4. **Unpinning a parent whose child is blocked today**: the child's block and pin survive; the status says so and that there's no undo. Pinned in Task 1 `TestUnpinningAParentKeepsBlockedChildren` and Task 3 `TestPinningAParentInTheTUIWithChildren`.
5. **A short terminal (tmux split) or a DST night**: no screen grows past the terminal, the timeline scrolls with its cursor, and rows stay wall-clock labels. Pinned in Task 3 `TestScreensShareTheHeightBudget` and `TestTimelineRowsAcrossADSTGap` (and Task 2 `TestDayOffset`).

## File Structure

```
internal/app/planning.go, pins_test.go      modify/new: D34 in Pin/Unpin/Block, ErrParentPinned, SetBlockSpan, pin helpers
internal/app/service.go                     modify: undoStep, modifyAnd (modify wraps it)
internal/app/tasks.go, children.go          modify: SetParent/Reopen/AddChild pin under a pinned parent
internal/config/config.go, timeline_test.go modify/new: [timeline] start/end
internal/domain/plan.go, plan_test.go       modify: Day.Offset
internal/tui/timeline.go, timeline_test.go  new: the row model (pure)
internal/tui/today.go, today_test.go        new: Today screen, carry/drop, block keys, fine-tune, commit
internal/tui/place_test.go                  new: placement flows
internal/tui/model.go                       modify: screenToday first, screenLog, today state, routing, clipped bodies
internal/tui/pick.go                        modify: L/B/esc, moveIn, selectedIn, columns, D34 statuses, visibleRows
internal/tui/inbox.go, detail.go            modify: return to the screen they came from; childRows on the budget
internal/tui/common.go, overlays.go         modify: header/hints, bodyHeight, clipLines, help, prompt.check, nudge
internal/tui/*_test.go                      modify: newLogHarness, "tt · Log"
```

Work happens on branch `plan-5a` in a worktree; fast-forward merge to `main` when the final review is green.

---
### Task 1: Service: a pinned parent pins its children (D34); `SetBlockSpan`

D34 as amended in the spec: pin and unpin cascade to children in one transaction, a child under a pinned parent can't be unpinned alone, children joining a pinned parent are pinned, and unpinning a parent keeps children that have blocks today. Plus one fine-tune command for D36.

**Files:**
- Modify: `internal/app/children.go`
- Modify: `internal/app/planning.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/tasks.go`
- Create: `internal/app/pins_test.go`

**Interfaces:**
- Consumes: `store.Tx` (`GetTask`, `Children`, `GetPin`, `PutPin`, `DeletePin`, `TaskBlocksOn`), `Service.reshape`, `mustBeOpen`; test helpers `startedService`, `mustAdd`, `mustAddChild`, `mustGet`, `mustUndo`, `mustBlock`, `mustPlan`, `candidate`.
- Produces:
  - `var app.ErrParentPinned` ("unpin the parent first"): `Unpin` of a pinned child whose parent is pinned.
  - `Pin`/`Unpin` of a task with children return a **nil** `Undo` (D34); childless pin/unpin undo unchanged.
  - `func (*Service) SetBlockSpan(ctx, blockID int64, start, dur time.Duration) (Undo, error)`: start and length in one undoable step; same errors as `MoveBlock`/`ResizeBlock` (`ErrOverlap`, `ErrInvalidValue`).
  - `type undoStep func(ctx, *store.Tx) error`; `func (*Service) modifyAnd(ctx, id, fn func(*store.Tx, *domain.Task) (undoStep, error)) (Undo, error)`; `modify` is now a wrapper over it.
  - `AddChild`, `SetParent`, `Reopen` pin the child under a pinned parent; the undo of `SetParent`/`Reopen` removes a pin it added (unless the task got a block meanwhile); undoing `AddChild` deletes the child and its pin.

- [ ] **Step 1: Write the failing tests**

Create `internal/app/pins_test.go`:

```go
package app

import (
	"context"
	"errors"
	"testing"

	"tt/internal/domain"
	"tt/internal/store"
)

func pinned(t *testing.T, s *Service, id int64) bool {
	t.Helper()
	_, ok := candidate(t, mustPlan(t, s), id)
	return ok
}

func TestPinningAParentPinsItsOpenChildren(t *testing.T) {
	s, _ := startedService(t)
	ctx := context.Background()
	must := mustUndo(t)
	parent := mustAdd(t, s, "Parent !1 ~xl")
	a := mustAddChild(t, s, parent.ID, "A ~s")
	b := mustAddChild(t, s, parent.ID, "B ~s")
	done := mustAddChild(t, s, parent.ID, "Done ~s")
	must(s.Done(ctx, done.ID))

	undo, err := s.Pin(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undo != nil {
		t.Error("pinning a task with children has no undo (D34)")
	}
	for _, id := range []int64{parent.ID, a.ID, b.ID} {
		if !pinned(t, s, id) {
			t.Errorf("#%d not pinned", id)
		}
	}
	if pinned(t, s, done.ID) {
		t.Error("a closed child must not be pinned")
	}

	// A childless pin keeps its undo.
	solo := mustAdd(t, s, "Solo !2 ~s")
	if undo := must(s.Pin(ctx, solo.ID)); undo == nil {
		t.Error("pinning a childless task is undoable")
	}
}

func TestUnpinningAParentKeepsBlockedChildren(t *testing.T) {
	s, _ := startedService(t)
	ctx := context.Background()
	must := mustUndo(t)
	parent := mustAdd(t, s, "Parent !1 ~xl")
	a := mustAddChild(t, s, parent.ID, "A ~s")
	b := mustAddChild(t, s, parent.ID, "B ~s")
	if _, err := s.Pin(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}
	mustBlock(t, s, b.ID, 10*h, h)

	undo, err := s.Unpin(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undo != nil {
		t.Error("unpinning a task with children has no undo (D34)")
	}
	if pinned(t, s, parent.ID) || pinned(t, s, a.ID) {
		t.Error("parent and unblocked child must be unpinned")
	}
	if !pinned(t, s, b.ID) {
		t.Fatal("a child blocked today stays pinned on its own")
	}
	if blocks := mustPlan(t, s).Blocks; len(blocks) != 1 || blocks[0].TaskID != b.ID {
		t.Errorf("the child's block must survive: %+v", blocks)
	}
	// With the parent unpinned, the child can be unpinned on its own.
	must(s.Unpin(ctx, b.ID))
	if pinned(t, s, b.ID) {
		t.Error("child still pinned")
	}
}

func TestUnpinningAChildOfAPinnedParentIsRefused(t *testing.T) {
	s, _ := startedService(t)
	ctx := context.Background()
	parent := mustAdd(t, s, "Parent !1 ~xl")
	a := mustAddChild(t, s, parent.ID, "A ~s")
	if _, err := s.Pin(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Unpin(ctx, a.ID); !errors.Is(err, ErrParentPinned) {
		t.Fatalf("Unpin(child) = %v; want ErrParentPinned", err)
	}
	if !pinned(t, s, a.ID) {
		t.Error("the refused unpin changed something")
	}
}

func TestChildrenJoiningAPinnedParentArePinned(t *testing.T) {
	s, _ := startedService(t)
	ctx := context.Background()
	must := mustUndo(t)
	parent := mustAdd(t, s, "Parent !1 ~xl")
	if _, err := s.Pin(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}

	added, undoAdd, err := s.AddChild(ctx, parent.ID, "Added ~s")
	if err != nil {
		t.Fatal(err)
	}
	if !pinned(t, s, added.ID) {
		t.Error("AddChild under a pinned parent pins the child")
	}
	if err := undoAdd(ctx); err != nil {
		t.Fatal(err)
	}
	if pinned(t, s, added.ID) {
		t.Error("undoing the add removes the child and its pin")
	}

	moved := mustAdd(t, s, "Moved !2 ~s")
	undoMove := must(s.SetParent(ctx, moved.ID, parent.ID))
	if !pinned(t, s, moved.ID) {
		t.Error("SetParent under a pinned parent pins the child")
	}
	if err := undoMove(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, s, moved.ID); got.ParentID != 0 || pinned(t, s, moved.ID) {
		t.Errorf("undo restores the row and removes the pin it added: parent %d, pinned %v", got.ParentID, pinned(t, s, moved.ID))
	}

	// A task that was pinned before keeps its pin when the move is undone.
	own := mustAdd(t, s, "Own !2 ~s")
	must(s.Pin(ctx, own.ID))
	undoOwn := must(s.SetParent(ctx, own.ID, parent.ID))
	if err := undoOwn(ctx); err != nil {
		t.Fatal(err)
	}
	if !pinned(t, s, own.ID) {
		t.Error("undo removed a pin the command didn't add")
	}

	// Reopening a child of a pinned parent pins it again (its pin was cleared by a new day).
	kid := mustAddChild(t, s, parent.ID, "Kid ~s")
	must(s.Done(ctx, kid.ID))
	if err := s.store.InTx(ctx, func(x *store.Tx) error { return x.DeletePin(ctx, kid.ID) }); err != nil {
		t.Fatal(err)
	}
	undoReopen := must(s.Reopen(ctx, kid.ID))
	if !pinned(t, s, kid.ID) {
		t.Error("Reopen under a pinned parent pins the child")
	}
	if err := undoReopen(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, s, kid.ID); got.Status != domain.StatusDone || pinned(t, s, kid.ID) {
		t.Errorf("undo of reopen: status %s, pinned %v", got.Status, pinned(t, s, kid.ID))
	}
}

func TestBlockingAParentPinsItsChildren(t *testing.T) {
	s, _ := startedService(t)
	parent := mustAdd(t, s, "Parent !1 ~xl")
	a := mustAddChild(t, s, parent.ID, "A ~s")
	mustBlock(t, s, parent.ID, 9*h, h)
	if !pinned(t, s, a.ID) {
		t.Error("Block pins a parent, and with it its children (D34)")
	}
}

func TestSetBlockSpan(t *testing.T) {
	s, _ := startedService(t)
	ctx := context.Background()
	must := mustUndo(t)
	a := mustAdd(t, s, "A !1 ~s")
	b := mustAdd(t, s, "B !1 ~s")
	blk, _ := mustBlock(t, s, a.ID, 9*h, h)
	mustBlock(t, s, b.ID, 11*h, h)

	undo := must(s.SetBlockSpan(ctx, blk.ID, 9*h+10*m, 45*m))
	got := mustPlan(t, s).Blocks[0]
	if got.Start != 9*h+10*m || got.Duration != 45*m {
		t.Errorf("block = %v +%v; want 9h10m +45m", got.Start, got.Duration)
	}
	if _, err := s.SetBlockSpan(ctx, blk.ID, 10*h+30*m, h); !errors.Is(err, ErrOverlap) {
		t.Errorf("overlapping span: %v; want ErrOverlap", err)
	}
	if _, err := s.SetBlockSpan(ctx, blk.ID, 9*h, 0); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("zero length: %v; want ErrInvalidValue", err)
	}
	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mustPlan(t, s).Blocks[0]; got.Start != 9*h || got.Duration != h {
		t.Errorf("after undo: %v +%v; want 9h +1h", got.Start, got.Duration)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/`
Expected: FAIL (build): `undefined: ErrParentPinned`, `s.SetBlockSpan undefined`.

- [ ] **Step 3: Implement**

Patch `internal/app/children.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/children.go b/internal/app/children.go
index e1f98ac..121868c 100644
--- a/internal/app/children.go
+++ b/internal/app/children.go
@@ -31,7 +31,9 @@ func inheritedPriority(parentID int64, p int) error {
 	return fmt.Errorf("%w: children inherit priority from #%d; drop !%d", ErrInvalidValue, parentID, p)
 }
 
-// AddChild captures a task from line as the last child of parentID.
+// AddChild captures a task from line as the last child of parentID. Under a
+// pinned parent the child is pinned too (D34); undoing the add deletes the
+// child, and its pin with it.
 func (s *Service) AddChild(ctx context.Context, parentID int64, line string) (domain.Task, Undo, error) {
 	d, err := s.ParseChild(parentID, line)
 	if err != nil {
@@ -53,7 +55,10 @@ func (s *Service) AddChild(ctx context.Context, parentID int64, line string) (do
 		if t.Position, err = x.NextChildPosition(ctx, parentID); err != nil {
 			return err
 		}
-		t.ID, err = x.InsertTask(ctx, t)
+		if t.ID, err = x.InsertTask(ctx, t); err != nil {
+			return err
+		}
+		_, err = s.pinUnderParent(ctx, x, t)
 		return err
 	})
 	if err != nil {
```

Patch `internal/app/planning.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/planning.go b/internal/app/planning.go
index 475dd03..501b222 100644
--- a/internal/app/planning.go
+++ b/internal/app/planning.go
@@ -12,6 +12,10 @@ import (
 
 var ErrOverlap = errors.New("overlaps another block")
 
+// ErrParentPinned: a child of a pinned parent is pinned with it, so it can't
+// be unpinned on its own (D34).
+var ErrParentPinned = errors.New("unpin the parent first")
+
 // Today is the current planning day (D19).
 func (s *Service) Today() domain.Day {
 	return domain.PlanningDay(s.clock.Now(), s.cfg.DayStart)
@@ -30,21 +34,25 @@ func (s *Service) DefaultBlockLength(t domain.Task, gap time.Duration) time.Dura
 }
 
 // Pin makes an open task one of today's candidates. Pinning a pinned task
-// changes nothing.
+// changes nothing. Pinning a parent pins its open children too (D34); a task
+// with children returns no undo (unpinning it by hand reverses the pin).
 func (s *Service) Pin(ctx context.Context, id int64) (Undo, error) {
 	today := s.Today()
-	var had bool
+	var had, parent bool
 	err := s.store.InTx(ctx, func(x *store.Tx) error {
 		if err := mustBeOpen(ctx, x, id); err != nil {
 			return err
 		}
 		var err error
+		if parent, err = pinChildren(ctx, x, id, today); err != nil {
+			return err
+		}
 		if _, had, err = x.GetPin(ctx, id); err != nil || had {
 			return err
 		}
 		return x.PutPin(ctx, domain.Pin{TaskID: id, PinnedOn: today})
 	})
-	if err != nil {
+	if err != nil || parent {
 		return nil, err
 	}
 	return func(ctx context.Context) error {
@@ -68,11 +76,16 @@ func (s *Service) Pin(ctx context.Context, id int64) (Undo, error) {
 }
 
 // Unpin removes a task from Today, together with its blocks on today's timeline.
+// Unpinning a parent unpins its children too, except children with a block
+// today: they stay pinned on their own, so no block is lost (D34). A task with
+// children returns no undo. A child of a pinned parent can't be unpinned on
+// its own (ErrParentPinned).
 func (s *Service) Unpin(ctx context.Context, id int64) (Undo, error) {
 	today := s.Today()
 	var (
 		pin    domain.Pin
 		had    bool
+		parent bool
 		blocks []domain.Block
 	)
 	err := s.store.InTx(ctx, func(x *store.Tx) error {
@@ -80,6 +93,20 @@ func (s *Service) Unpin(ctx context.Context, id int64) (Undo, error) {
 		if pin, had, err = x.GetPin(ctx, id); err != nil || !had {
 			return err
 		}
+		t, err := x.GetTask(ctx, id)
+		if err != nil {
+			return err
+		}
+		if t.ParentID != 0 {
+			if _, parentPinned, err := x.GetPin(ctx, t.ParentID); err != nil {
+				return err
+			} else if parentPinned {
+				return fmt.Errorf("#%d is pinned with its parent #%d: %w", id, t.ParentID, ErrParentPinned)
+			}
+		}
+		if parent, err = unpinChildren(ctx, x, id, today); err != nil {
+			return err
+		}
 		if blocks, err = x.TaskBlocksOn(ctx, id, today); err != nil {
 			return err
 		}
@@ -90,7 +117,7 @@ func (s *Service) Unpin(ctx context.Context, id int64) (Undo, error) {
 		}
 		return x.DeletePin(ctx, id)
 	})
-	if err != nil {
+	if err != nil || parent {
 		return nil, err
 	}
 	return func(ctx context.Context) error {
@@ -122,8 +149,9 @@ func (s *Service) Unpin(ctx context.Context, id int64) (Undo, error) {
 
 // Block places an open task on today's timeline at wall-clock offset start
 // (from midnight) for dur. It pins the task and resets its pinned-days
-// counter (D10d). Blocks may not overlap each other; meetings are not checked
-// here (D18 conflicts are shown, not prevented).
+// counter (D10d); a parent it pins brings its open children along (D34; the
+// undo leaves those pinned). Blocks may not overlap each other; meetings are
+// not checked here (D18 conflicts are shown, not prevented).
 func (s *Service) Block(ctx context.Context, id int64, start, dur time.Duration) (domain.Block, Undo, error) {
 	b := domain.Block{TaskID: id, Day: s.Today(), Start: start, Duration: dur}
 	if err := s.validateSpan(b); err != nil {
@@ -144,6 +172,11 @@ func (s *Service) Block(ctx context.Context, id int64, start, dur time.Duration)
 		if pin, had, err = x.GetPin(ctx, id); err != nil {
 			return err
 		}
+		if !had {
+			if _, err := pinChildren(ctx, x, id, b.Day); err != nil {
+				return err
+			}
+		}
 		if err := x.PutPin(ctx, domain.Pin{TaskID: id, PinnedOn: b.Day}); err != nil {
 			return err
 		}
@@ -181,6 +214,12 @@ func (s *Service) Block(ctx context.Context, id int64, start, dur time.Duration)
 	}, nil
 }
 
+// SetBlockSpan sets the start and the length of one of today's blocks in one
+// step (fine-tune, D36).
+func (s *Service) SetBlockSpan(ctx context.Context, blockID int64, start, dur time.Duration) (Undo, error) {
+	return s.reshape(ctx, blockID, func(b *domain.Block) { b.Start, b.Duration = start, dur })
+}
+
 // MoveBlock moves one of today's blocks to a new start.
 func (s *Service) MoveBlock(ctx context.Context, blockID int64, start time.Duration) (Undo, error) {
 	return s.reshape(ctx, blockID, func(b *domain.Block) { b.Start = start })
@@ -260,6 +299,83 @@ func (s *Service) reshape(ctx context.Context, blockID int64, fn func(*domain.Bl
 	}, nil
 }
 
+// pinChildren pins id's open children that aren't pinned yet (D34) and
+// reports whether id has any children.
+func pinChildren(ctx context.Context, x *store.Tx, id int64, day domain.Day) (bool, error) {
+	kids, err := x.Children(ctx, id)
+	if err != nil {
+		return false, err
+	}
+	for _, k := range kids {
+		if k.Status != domain.StatusOpen {
+			continue
+		}
+		if _, had, err := x.GetPin(ctx, k.ID); err != nil {
+			return false, err
+		} else if had {
+			continue
+		}
+		if err := x.PutPin(ctx, domain.Pin{TaskID: k.ID, PinnedOn: day}); err != nil {
+			return false, err
+		}
+	}
+	return len(kids) > 0, nil
+}
+
+// unpinChildren unpins id's children that have no block on day (D34) and
+// reports whether id has any children.
+func unpinChildren(ctx context.Context, x *store.Tx, id int64, day domain.Day) (bool, error) {
+	kids, err := x.Children(ctx, id)
+	if err != nil {
+		return false, err
+	}
+	for _, k := range kids {
+		blocks, err := x.TaskBlocksOn(ctx, k.ID, day)
+		if err != nil {
+			return false, err
+		}
+		if len(blocks) > 0 {
+			continue // a blocked child stays pinned on its own
+		}
+		if err := x.DeletePin(ctx, k.ID); err != nil {
+			return false, err
+		}
+	}
+	return len(kids) > 0, nil
+}
+
+// pinUnderParent pins t when it is an open child of a pinned parent and not
+// pinned yet (D34). added reports whether it put a pin; the undo of the
+// command then removes it again with removeAddedPin.
+func (s *Service) pinUnderParent(ctx context.Context, x *store.Tx, t domain.Task) (added bool, err error) {
+	if t.ParentID == 0 || t.Status != domain.StatusOpen {
+		return false, nil
+	}
+	if _, parentPinned, err := x.GetPin(ctx, t.ParentID); err != nil || !parentPinned {
+		return false, err
+	}
+	if _, had, err := x.GetPin(ctx, t.ID); err != nil || had {
+		return false, err
+	}
+	return true, x.PutPin(ctx, domain.Pin{TaskID: t.ID, PinnedOn: s.Today()})
+}
+
+// removeAddedPin is the undo step for a pin pinUnderParent added: the pin
+// goes unless the task got a block meanwhile.
+func removeAddedPin(id int64) undoStep {
+	return func(ctx context.Context, x *store.Tx) error {
+		pin, had, err := x.GetPin(ctx, id)
+		if err != nil || !had {
+			return err
+		}
+		blocks, err := x.TaskBlocksOn(ctx, id, pin.PinnedOn)
+		if err != nil || len(blocks) > 0 {
+			return err
+		}
+		return x.DeletePin(ctx, id)
+	}
+}
+
 // taskStillExists fails with ErrConflict if the task was deleted since.
 func taskStillExists(ctx context.Context, x *store.Tx, id int64) error {
 	if _, err := x.GetTask(ctx, id); errors.Is(err, store.ErrNotFound) {
```

Patch `internal/app/service.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/service.go b/internal/app/service.go
index d39e627..139b832 100644
--- a/internal/app/service.go
+++ b/internal/app/service.go
@@ -59,10 +59,22 @@ func (s *Service) DataVersion(ctx context.Context) (int64, error) {
 // a rollover or a GitHub auto-close; undoing would silently revert that.
 var ErrConflict = errors.New("task changed since; can't undo")
 
+// undoStep is extra work an undo does in its transaction.
+type undoStep func(ctx context.Context, x *store.Tx) error
+
 // modify loads task id, applies fn, saves it, and returns an Undo that
 // restores the previous row, but only if nothing has changed the row since.
 func (s *Service) modify(ctx context.Context, id int64, fn func(x *store.Tx, t *domain.Task) error) (Undo, error) {
-	var before, after domain.Task
+	return s.modifyAnd(ctx, id, func(x *store.Tx, t *domain.Task) (undoStep, error) { return nil, fn(x, t) })
+}
+
+// modifyAnd is modify for commands whose undo has more to do: the step fn
+// returns (if any) runs in the undo's transaction after the row is restored.
+func (s *Service) modifyAnd(ctx context.Context, id int64, fn func(x *store.Tx, t *domain.Task) (undoStep, error)) (Undo, error) {
+	var (
+		before, after domain.Task
+		step          undoStep
+	)
 	err := s.store.InTx(ctx, func(x *store.Tx) error {
 		var err error
 		if before, err = x.GetTask(ctx, id); err != nil {
@@ -70,7 +82,7 @@ func (s *Service) modify(ctx context.Context, id int64, fn func(x *store.Tx, t *
 		}
 		after = before
 		after.Tags = append([]string(nil), before.Tags...)
-		if err := fn(x, &after); err != nil {
+		if step, err = fn(x, &after); err != nil {
 			return err
 		}
 		after.UpdatedAt = s.clock.Now()
@@ -84,7 +96,10 @@ func (s *Service) modify(ctx context.Context, id int64, fn func(x *store.Tx, t *
 			if err := unchangedSince(ctx, x, after); err != nil {
 				return err
 			}
-			return x.UpdateTask(ctx, before)
+			if err := x.UpdateTask(ctx, before); err != nil || step == nil {
+				return err
+			}
+			return step(ctx, x)
 		})
 	}, nil
 }
```

Patch `internal/app/tasks.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/tasks.go b/internal/app/tasks.go
index 2d111a1..69b8a48 100644
--- a/internal/app/tasks.go
+++ b/internal/app/tasks.go
@@ -129,37 +129,48 @@ func (s *Service) SetSize(ctx context.Context, id int64, size domain.Size) (Undo
 }
 
 // SetParent makes id a child of parentID (appended last); 0 detaches it.
-// Only one level of nesting is allowed (D6).
+// Only one level of nesting is allowed (D6). Under a pinned parent the
+// child is pinned too (D34).
 func (s *Service) SetParent(ctx context.Context, id, parentID int64) (Undo, error) {
-	return s.modify(ctx, id, func(x *store.Tx, t *domain.Task) error {
+	return s.modifyAnd(ctx, id, func(x *store.Tx, t *domain.Task) (undoStep, error) {
 		if parentID == 0 {
 			t.ParentID, t.Position = 0, 0
-			return nil
+			return nil, nil
 		}
 		if parentID == id {
-			return fmt.Errorf("%w: a task can't be its own parent", ErrInvalidParent)
+			return nil, fmt.Errorf("%w: a task can't be its own parent", ErrInvalidParent)
 		}
 		parent, err := x.GetTask(ctx, parentID)
 		if err != nil {
-			return err
+			return nil, err
 		}
 		if parent.ParentID != 0 {
-			return fmt.Errorf("%w: #%d is already a child; only one level of nesting", ErrInvalidParent, parentID)
+			return nil, fmt.Errorf("%w: #%d is already a child; only one level of nesting", ErrInvalidParent, parentID)
 		}
 		if has, err := x.HasChildren(ctx, id); err != nil {
-			return err
+			return nil, err
 		} else if has {
-			return fmt.Errorf("%w: #%d has children; only one level of nesting", ErrInvalidParent, id)
+			return nil, fmt.Errorf("%w: #%d has children; only one level of nesting", ErrInvalidParent, id)
 		}
 		pos, err := x.NextChildPosition(ctx, parentID)
 		if err != nil {
-			return err
+			return nil, err
 		}
 		t.ParentID, t.Position = parentID, pos
-		return nil
+		return s.pinnedUnderParent(ctx, x, *t)
 	})
 }
 
+// pinnedUnderParent pins t if it joins a pinned parent and returns the undo
+// step that removes that pin again (nil when nothing was pinned).
+func (s *Service) pinnedUnderParent(ctx context.Context, x *store.Tx, t domain.Task) (undoStep, error) {
+	added, err := s.pinUnderParent(ctx, x, t)
+	if err != nil || !added {
+		return nil, err
+	}
+	return removeAddedPin(t.ID), nil
+}
+
 // closedAt keeps the original close time when t is already closed.
 func (s *Service) closedAt(t domain.Task) time.Time {
 	if !t.ClosedAt.IsZero() {
@@ -184,11 +195,12 @@ func (s *Service) Drop(ctx context.Context, id int64, reason string) (Undo, erro
 	})
 }
 
-// Reopen returns a done or dropped task to open.
+// Reopen returns a done or dropped task to open. A child of a pinned parent
+// is pinned again (D34).
 func (s *Service) Reopen(ctx context.Context, id int64) (Undo, error) {
-	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
+	return s.modifyAnd(ctx, id, func(x *store.Tx, t *domain.Task) (undoStep, error) {
 		t.Status, t.ClosedAt, t.ClosedBy, t.DropReason = domain.StatusOpen, time.Time{}, "", ""
-		return nil
+		return s.pinnedUnderParent(ctx, x, *t)
 	})
 }
 
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/app/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS (the existing pin/unpin/undo tests still pass unchanged), no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/app/children.go internal/app/pins_test.go internal/app/planning.go internal/app/service.go internal/app/tasks.go
git commit -m 'feat(app): a pinned parent pins its children (D34); SetBlockSpan for fine-tune

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 2: Timeline window config, `Day.Offset`, the timeline grid

D38's pure parts: the configured window, the inverse of `Day.At` for "now", and the row model the Today screen renders (no rendering yet).

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/domain/plan.go`
- Create: `internal/tui/timeline.go`
- Create: `internal/config/timeline_test.go`
- Modify: `internal/domain/plan_test.go`
- Create: `internal/tui/timeline_test.go`

**Interfaces:**
- Consumes: `config.parseClock`, `domain.Block`.
- Produces:
  - `config.Config.TimelineStart`, `TimelineEnd time.Duration` (defaults 8h, 19h; TOML `[timeline] start`, `end` as `HH:MM`). Errors: `timeline.start: …`, `timeline.end: …`, `timeline.end must be after timeline.start`, `timeline.start must not be before day_start …`. Unset keys follow a late `day_start` instead of failing.
  - `func (d domain.Day) Offset(t time.Time) time.Duration`: wall-clock offset from midnight of `d` (25h for 01:00 the next date), DST-safe.
  - `internal/tui/timeline.go`: `const rowLen = 30 * time.Minute`; `type tlRow struct{ start time.Duration; blocks []domain.Block }`; `type timeline struct{ rows []tlRow; blocks []domain.Block; nowRow int; dayEnd time.Duration }` (Task 3 adds `now`); `func buildTimeline(blocks []domain.Block, from, to, now, dayStart time.Duration) timeline` (`now < 0` = no now); methods `rowOf(at) int`, `blockAt(i) (domain.Block, bool)`, `gapFrom(at) time.Duration`, `firstFree(i) int`, `clash(b) (domain.Block, bool)`.

- [ ] **Step 1: Write the failing tests**

Create `internal/config/timeline_test.go`:

```go
package config

import (
	"strings"
	"testing"
	"time"
)

func TestTimelineDefaults(t *testing.T) {
	cfg := Default()
	if cfg.TimelineStart != 8*time.Hour || cfg.TimelineEnd != 19*time.Hour {
		t.Errorf("timeline defaults: %v–%v; want 8h–19h", cfg.TimelineStart, cfg.TimelineEnd)
	}
}

func TestParseTimeline(t *testing.T) {
	cfg, err := Parse([]byte("[timeline]\nstart = \"07:30\"\nend = \"18:00\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TimelineStart != 7*time.Hour+30*time.Minute || cfg.TimelineEnd != 18*time.Hour {
		t.Errorf("timeline: %v–%v", cfg.TimelineStart, cfg.TimelineEnd)
	}
	// Only the keys you set are checked: a late day_start moves the default window along.
	late, err := Parse([]byte("day_start = \"21:00\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if late.TimelineStart != 21*time.Hour || late.TimelineEnd != 22*time.Hour {
		t.Errorf("late day_start: timeline %v–%v; want 21h–22h", late.TimelineStart, late.TimelineEnd)
	}
	cases := map[string]string{
		"[timeline]\nstart = \"8am\"":                          "timeline.start",
		"[timeline]\nend = \"25:00\"":                          "timeline.end",
		"[timeline]\nstart = \"19:00\"\nend = \"18:00\"":       "timeline.end must be after timeline.start",
		"[timeline]\nstart = \"10:00\"\nend = \"10:00\"":       "timeline.end must be after timeline.start",
		"day_start = \"06:00\"\n[timeline]\nstart = \"05:00\"": "timeline.start must not be before day_start",
		"[timeline]\nrows = \"30m\"":                           "unknown config key",
	}
	for input, wantErr := range cases {
		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}
```

Patch `internal/domain/plan_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/domain/plan_test.go b/internal/domain/plan_test.go
index 5d0aa74..a274085 100644
--- a/internal/domain/plan_test.go
+++ b/internal/domain/plan_test.go
@@ -104,3 +104,29 @@ func TestDefaultBlockLength(t *testing.T) {
 		}
 	}
 }
+
+func TestDayOffset(t *testing.T) {
+	ams, err := time.LoadLocation("Europe/Amsterdam")
+	if err != nil {
+		t.Fatal(err)
+	}
+	const day Day = "2026-03-28"
+	cases := []struct {
+		name string
+		t    time.Time
+		want time.Duration
+	}{
+		{"morning", time.Date(2026, 3, 28, 9, 30, 15, 0, ams), 9*time.Hour + 30*time.Minute + 15*time.Second},
+		{"after midnight", time.Date(2026, 3, 29, 1, 0, 0, 0, ams), 25 * time.Hour},
+		// Spring forward: 02:00 → 03:00. Wall clock, not elapsed time.
+		{"after the DST gap", time.Date(2026, 3, 29, 3, 30, 0, 0, ams), 27*time.Hour + 30*time.Minute},
+	}
+	for _, c := range cases {
+		if got := day.Offset(c.t); got != c.want {
+			t.Errorf("%s: Offset(%v) = %v; want %v", c.name, c.t, got, c.want)
+		}
+	}
+	if got := Day("2026-10-12").Offset(Day("2026-10-12").At(14*time.Hour+5*time.Minute, ams)); got != 14*time.Hour+5*time.Minute {
+		t.Errorf("Offset(At(14:05)) = %v", got)
+	}
+}
```

Create `internal/tui/timeline_test.go`:

```go
package tui

import (
	"testing"
	"time"

	"tt/internal/domain"
)

const (
	hr = time.Hour
	mn = time.Minute
)

func starts(tl timeline) []time.Duration {
	var out []time.Duration
	for _, r := range tl.rows {
		out = append(out, r.start)
	}
	return out
}

func TestTimelineWindow(t *testing.T) {
	const dayStart = 4 * hr
	tl := buildTimeline(nil, 8*hr, 19*hr, 10*hr+12*mn, dayStart)
	if got := starts(tl); len(got) != 22 || got[0] != 8*hr || got[21] != 18*hr+30*mn {
		t.Fatalf("default window: %d rows %v…; want 22 rows 08:00…18:30", len(got), got[:1])
	}
	if tl.nowRow != 4 { // 10:00
		t.Errorf("nowRow = %d; want 4 (the 10:00 row)", tl.nowRow)
	}

	// The window stretches to an early now and a late, off-grid block.
	late := domain.Block{ID: 1, TaskID: 1, Start: 21*hr + 10*mn, Duration: 40 * mn}
	tl = buildTimeline([]domain.Block{late}, 8*hr, 19*hr, 6*hr+45*mn, dayStart)
	got := starts(tl)
	if got[0] != 6*hr+30*mn || got[len(got)-1] != 21*hr+30*mn {
		t.Errorf("stretched window %v–%v; want 06:30–21:30 rows", got[0], got[len(got)-1])
	}
	if tl.nowRow != 0 {
		t.Errorf("nowRow = %d; want 0", tl.nowRow)
	}

	// No now (a stale day): no stretching for it, no now row.
	tl = buildTimeline(nil, 8*hr, 19*hr, -1, dayStart)
	if len(tl.rows) != 22 || tl.nowRow != -1 {
		t.Errorf("without now: %d rows, nowRow %d", len(tl.rows), tl.nowRow)
	}
}

func TestTimelineRowsAndGaps(t *testing.T) {
	a := domain.Block{ID: 1, TaskID: 10, Start: 9 * hr, Duration: 90 * mn}        // 09:00–10:30
	b := domain.Block{ID: 2, TaskID: 11, Start: 11*hr + 10*mn, Duration: 15 * mn} // 11:10–11:25
	c := domain.Block{ID: 3, TaskID: 12, Start: 11*hr + 25*mn, Duration: 5 * mn}  // 11:25–11:30
	tl := buildTimeline([]domain.Block{a, b, c}, 8*hr, 19*hr, 8*hr, 4*hr)

	at := func(off time.Duration) int { return tl.rowOf(off) }
	if blk, ok := tl.blockAt(at(9*hr + 30*mn)); !ok || blk.ID != a.ID {
		t.Errorf("09:30 row: %+v %v; want block 1", blk, ok)
	}
	if blk, ok := tl.blockAt(at(11 * hr)); !ok || blk.ID != b.ID || len(tl.rows[at(11*hr)].blocks) != 2 {
		t.Errorf("11:00 row holds two short blocks, keys act on the earlier: %+v", tl.rows[at(11*hr)].blocks)
	}
	if _, ok := tl.blockAt(at(10*hr + 30*mn)); ok {
		t.Error("10:30 row is free")
	}
	if tl.rowOf(7*hr) != -1 {
		t.Error("07:00 is outside the rows")
	}

	cases := []struct {
		at, want time.Duration
	}{
		{8 * hr, hr},                   // until a starts
		{9*hr + 30*mn, 0},              // inside a
		{10*hr + 30*mn, 40 * mn},       // until b
		{11*hr + 30*mn, 16*hr + 30*mn}, // until the planning day ends (04:00 next date)
	}
	for _, c := range cases {
		if got := tl.gapFrom(c.at); got != c.want {
			t.Errorf("gapFrom(%v) = %v; want %v", c.at, got, c.want)
		}
	}

	if got := tl.firstFree(at(9 * hr)); got != at(10*hr+30*mn) {
		t.Errorf("firstFree(09:00 row) = row %d; want the 10:30 row %d", got, at(10*hr+30*mn))
	}
	if blk, ok := tl.clash(domain.Block{ID: 1, Start: 10 * hr, Duration: 90 * mn}); !ok || blk.ID != b.ID {
		t.Errorf("a moved to 10:00–11:30 clashes with b, not itself: %+v %v", blk, ok)
	}
	if _, ok := tl.clash(domain.Block{Start: 12 * hr, Duration: hr}); ok {
		t.Error("12:00–13:00 is free")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/ ./internal/domain/ ./internal/tui/`
Expected: FAIL (build): `cfg.TimelineStart undefined`, `day.Offset undefined`, `undefined: buildTimeline`.

- [ ] **Step 3: Implement**

Patch `internal/config/config.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/config/config.go b/internal/config/config.go
index ffd9c5c..dbf544e 100644
--- a/internal/config/config.go
+++ b/internal/config/config.go
@@ -27,6 +27,9 @@ type Config struct {
 	UnsizedBlock time.Duration
 	// RolloverAlert is the rollover count at which planning flags a task (D10a).
 	RolloverAlert int
+	// TimelineStart and TimelineEnd are the hours Today's timeline shows (D38),
+	// as offsets from midnight; it stretches to show blocks and "now" outside them.
+	TimelineStart, TimelineEnd time.Duration
 }
 
 func Default() Config {
@@ -45,6 +48,8 @@ func Default() Config {
 		},
 		UnsizedBlock:  30 * time.Minute,
 		RolloverAlert: 3,
+		TimelineStart: 8 * time.Hour,
+		TimelineEnd:   19 * time.Hour,
 	}
 }
 
@@ -57,6 +62,10 @@ type file struct {
 		UnsizedBlock  *string `toml:"unsized_block"`
 		RolloverAlert *int    `toml:"rollover_alert"`
 	} `toml:"planning"`
+	Timeline struct {
+		Start *string `toml:"start"`
+		End   *string `toml:"end"`
+	} `toml:"timeline"`
 }
 
 // Load reads path. A missing file yields Default().
@@ -127,6 +136,26 @@ func Parse(data []byte) (Config, error) {
 		}
 		cfg.RolloverAlert = *p
 	}
+	if p := f.Timeline.Start; p != nil {
+		if cfg.TimelineStart, err = parseClock(*p); err != nil {
+			return Config{}, fmt.Errorf("timeline.start: %w", err)
+		}
+	} else {
+		cfg.TimelineStart = max(cfg.TimelineStart, cfg.DayStart) // a late day_start moves the default along
+	}
+	if p := f.Timeline.End; p != nil {
+		if cfg.TimelineEnd, err = parseClock(*p); err != nil {
+			return Config{}, fmt.Errorf("timeline.end: %w", err)
+		}
+	} else if cfg.TimelineEnd <= cfg.TimelineStart {
+		cfg.TimelineEnd = min(cfg.TimelineStart+time.Hour, 24*time.Hour-time.Minute)
+	}
+	switch {
+	case cfg.TimelineEnd <= cfg.TimelineStart:
+		return Config{}, fmt.Errorf("timeline.end must be after timeline.start")
+	case cfg.TimelineStart < cfg.DayStart:
+		return Config{}, fmt.Errorf("timeline.start must not be before day_start (rows before it belong to the previous day)")
+	}
 	return cfg, nil
 }
 
```

Patch `internal/domain/plan.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/domain/plan.go b/internal/domain/plan.go
index 8b2e2f3..e1f8609 100644
--- a/internal/domain/plan.go
+++ b/internal/domain/plan.go
@@ -40,6 +40,20 @@ func (d Day) At(off time.Duration, loc *time.Location) time.Time {
 	return time.Date(t.Year(), t.Month(), t.Day(), minutes/60, minutes%60, 0, 0, loc)
 }
 
+// Offset is t's wall-clock offset from midnight of d, the inverse of At:
+// 9h30m for 09:30 on d, 25h for 01:00 on the following date. It reads t's
+// wall clock in t's own location, so a DST change doesn't shift it.
+func (d Day) Offset(t time.Time) time.Duration {
+	day, err := time.Parse(time.DateOnly, string(d))
+	if err != nil {
+		panic(fmt.Sprintf("invalid Day %q", d))
+	}
+	y, m, dd := t.Date()
+	days := time.Date(y, m, dd, 0, 0, 0, 0, time.UTC).Sub(day) / (24 * time.Hour)
+	return days*24*time.Hour + time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute +
+		time.Duration(t.Second())*time.Second
+}
+
 // Pin marks a task as one of today's candidates since PinnedOn (D12a, D10d).
 type Pin struct {
 	TaskID   int64
```

Create `internal/tui/timeline.go`:

```go
package tui

import (
	"time"

	"tt/internal/domain"
)

// rowLen is the length of one timeline row (D38).
const rowLen = 30 * time.Minute

// tlRow is one row of Today's timeline.
type tlRow struct {
	start  time.Duration  // wall-clock offset from midnight of the plan's day
	blocks []domain.Block // the blocks that overlap the row, by start
}

// timeline is Today's grid (D38), rebuilt on every load. Rows are
// wall-clock offsets like stored blocks, so a DST night needs no special
// case: a row that doesn't exist on the clock that night is still a row.
type timeline struct {
	rows   []tlRow
	blocks []domain.Block // the day's blocks, by start
	nowRow int            // the row holding now; -1 when there is no now
	dayEnd time.Duration  // the planning day's end: day start + 24h
}

// buildTimeline lays out the hours from–to in rows, stretched to include
// every block and now. now < 0 means no now (the shown day isn't today).
func buildTimeline(blocks []domain.Block, from, to, now, dayStart time.Duration) timeline {
	if now >= 0 {
		from, to = min(from, now), max(to, now+time.Minute)
	}
	for _, b := range blocks {
		from, to = min(from, b.Start), max(to, b.End())
	}
	from = from.Truncate(rowLen)
	if to%rowLen != 0 {
		to = to.Truncate(rowLen) + rowLen
	}
	tl := timeline{blocks: blocks, nowRow: -1, dayEnd: dayStart + 24*time.Hour}
	for s := from; s < to; s += rowLen {
		r := tlRow{start: s}
		for _, b := range blocks {
			if b.Start < s+rowLen && s < b.End() {
				r.blocks = append(r.blocks, b)
			}
		}
		if now >= s && now < s+rowLen {
			tl.nowRow = len(tl.rows)
		}
		tl.rows = append(tl.rows, r)
	}
	return tl
}

// rowOf is the index of the row holding offset at, or -1.
func (tl *timeline) rowOf(at time.Duration) int {
	for i, r := range tl.rows {
		if at >= r.start && at < r.start+rowLen {
			return i
		}
	}
	return -1
}

// blockAt is the earliest block in row i; keys on a row act on it.
func (tl *timeline) blockAt(i int) (domain.Block, bool) {
	if i < 0 || i >= len(tl.rows) || len(tl.rows[i].blocks) == 0 {
		return domain.Block{}, false
	}
	return tl.rows[i].blocks[0], true
}

// gapFrom is the free time from offset at until the next block starts or
// the planning day ends; 0 when a block covers at.
func (tl *timeline) gapFrom(at time.Duration) time.Duration {
	end := tl.dayEnd
	for _, b := range tl.blocks {
		if b.Start <= at && at < b.End() {
			return 0
		}
		if b.Start > at {
			end = min(end, b.Start)
		}
	}
	return max(end-at, 0)
}

// firstFree is the first row from i on whose start no block covers; i when
// there is none.
func (tl *timeline) firstFree(i int) int {
	for j := max(i, 0); j < len(tl.rows); j++ {
		if tl.gapFrom(tl.rows[j].start) > 0 {
			return j
		}
	}
	return i
}

// clash is the first block other than b itself (same ID) that b overlaps.
func (tl *timeline) clash(b domain.Block) (domain.Block, bool) {
	for _, o := range tl.blocks {
		if o.ID != b.ID && b.Start < o.End() && o.Start < b.End() {
			return o, true
		}
	}
	return domain.Block{}, false
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/config/ ./internal/domain/ ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/timeline_test.go internal/domain/plan.go internal/domain/plan_test.go internal/tui/timeline.go internal/tui/timeline_test.go
git commit -m 'feat: timeline window config, Day.Offset and the timeline grid (D38)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 3: Today is the home screen; Pick becomes the Log; shared height budget

D35 and D38's rendering: tt opens on Today (timeline beside the candidates), `L`/`esc`/`B` move between Today and the Log, Inbox and Task detail return to where they were opened, D34's statuses, and one height budget for every screen (Plan 3 carry-forward). Placement comes in Task 4. The test harness now checks that tt opens on Today; most existing tests switch to `newLogHarness`, and "tt · Pick" becomes "tt · Log".

**Files:**
- Modify: `internal/tui/common.go`
- Modify: `internal/tui/detail.go`
- Modify: `internal/tui/inbox.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/pick.go`
- Modify: `internal/tui/timeline.go`
- Create: `internal/tui/today.go`
- Modify: `internal/tui/browse_test.go`
- Modify: `internal/tui/children_test.go`
- Modify: `internal/tui/detail_test.go`
- Modify: `internal/tui/edit_test.go`
- Modify: `internal/tui/filter_test.go`
- Modify: `internal/tui/harness_test.go`
- Modify: `internal/tui/pick_test.go`
- Create: `internal/tui/today_test.go`
- Modify: `internal/tui/tui_test.go`

**Interfaces:**
- Consumes: Task 1 (`Pin`/`Unpin` nil undo for parents), Task 2 (`buildTimeline`, `Day.Offset`, `Config.TimelineStart/End`).
- Produces:
  - `screenToday` (the zero value, first in the `screen` consts); `screenPick` renamed `screenLog`.
  - `todayState{focus todayFocus; tl timeline; at time.Duration; placed bool; cursor, offset int}`; `focusTimeline`, `focusCandidates`; `(*todayState).refresh(m)`, `move(delta)`, `follow(m)`.
  - `(*Model).todayKey(key) tea.Cmd`, `todayView() string`, `timelineLines(width) []string`, `timelineRow(i, width) string`, `blockLabel(b) string`, `todayHints() string`; `var todayHelp []string`; `func clock(off time.Duration) string` ("09:30", wraps past 24h).
  - `(*Model).openInbox()`; `inboxState.from`, `detailState.from` (esc returns there).
  - `(*Model).bodyHeight() int` (header, status, hints, banner, non-help overlay subtracted; floor 2), `visibleRows()` = `bodyHeight()-1` (floor 1), `clipLines(s, n)`, `plusChildren(n)`.
  - `(*pickState).moveIn(pn, delta)`, `(*Model).selectedIn(pn) (row, bool)`, `columns(left, right []string, lw, rw int) string`; `paneLines` takes a `focused bool`.
  - `timeline.now` field (now's offset, -1 for none).
  - Test helpers: `newLogHarness(t, cfg)`, `(*harness).block(id, start, dur) domain.Block`.

- [ ] **Step 1: Write the failing tests**

Patch `internal/tui/browse_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/browse_test.go b/internal/tui/browse_test.go
index 2521fc4..ee44f77 100644
--- a/internal/tui/browse_test.go
+++ b/internal/tui/browse_test.go
@@ -10,7 +10,7 @@ import (
 )
 
 func TestFilterEscRestoresTheSelection(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.add("Alpha !1 ~s")
 	h.add("Beta !1 ~s")
 	gamma := h.add("Gamma !1 ~s")
@@ -25,7 +25,7 @@ func TestFilterEscRestoresTheSelection(t *testing.T) {
 }
 
 func TestGroupingSavesTheLatestChoice(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	g := tea.KeyPressMsg{Code: 'g', Text: "g"}
 
 	// Two quick g presses: the second save waits for the first, then saves the latest.
@@ -46,7 +46,7 @@ func TestGroupingSavesTheLatestChoice(t *testing.T) {
 }
 
 func TestFilteredExpandCollapseLeavesTheBacklogAlone(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	parent, _ := h.family("Auth migration !1 ~xl #auth", "Rollback plan ~s", "Token spike ~m")
 	h.keys("right") // expanded before filtering
 	h.mustSee("▾ [P1 XL] Auth migration", "    [P1 S] Rollback plan")
@@ -66,7 +66,7 @@ func TestFilteredExpandCollapseLeavesTheBacklogAlone(t *testing.T) {
 }
 
 func TestParentWithoutListedChildrenHasNoMarker(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.family("Lonely parent !2 ~m", "Untriaged idea") // the child is in the Inbox
 	h.mustSee("  [P2 M] Lonely parent")
 	h.mustNotSee("▸ [P2 M] Lonely parent")
@@ -75,7 +75,7 @@ func TestParentWithoutListedChildrenHasNoMarker(t *testing.T) {
 }
 
 func TestFilterBarCompletesAfterDashes(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.add("One @ana !1 ~s")
 	h.keys("/")
 	h.typeText("-- @an") // in a filter, -- is just a word
```

Patch `internal/tui/children_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/children_test.go b/internal/tui/children_test.go
index 2a67dd9..46d6269 100644
--- a/internal/tui/children_test.go
+++ b/internal/tui/children_test.go
@@ -22,7 +22,7 @@ func (h *harness) childIDs(parentID int64) []int64 {
 }
 
 func TestAddChildrenOneAfterAnother(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	parent := h.add("Auth migration !1 ~xl #auth")
 	h.keys("enter")
 	h.mustSee("no children · a adds one")
@@ -74,7 +74,7 @@ func TestAddChildrenOneAfterAnother(t *testing.T) {
 }
 
 func TestAddChildKeepsTheLineWhenItFails(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	parent := h.add("Auth migration !1 ~xl")
 	h.keys("enter", "a")
 	if err := h.svc.Delete(context.Background(), parent.ID); err != nil { // behind the TUI's back
@@ -89,7 +89,7 @@ func TestAddChildKeepsTheLineWhenItFails(t *testing.T) {
 }
 
 func TestReorderChildren(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	parent, kids := h.family("Parent !1 ~l", "A ~s", "B ~s", "C ~s")
 	a, b, c := kids[0].ID, kids[1].ID, kids[2].ID
 	h.keys("enter", "J")
@@ -130,7 +130,7 @@ func TestReorderChildren(t *testing.T) {
 }
 
 func TestMultiLinePasteBecomesTheNote(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.keys("a")
 	h.send(tea.PasteMsg{Content: "Look at the schema @ana !1 ~s\r\nAna: can you check\r\n\r\nthe migration?\r\n"})
 	h.mustSee("add › Look at the schema @ana !1 ~s", "+ 3 lines → note")
```

Patch `internal/tui/detail_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/detail_test.go b/internal/tui/detail_test.go
index a141182..c3da968 100644
--- a/internal/tui/detail_test.go
+++ b/internal/tui/detail_test.go
@@ -31,7 +31,7 @@ func (h *harness) family(parentLine string, kids ...string) (domain.Task, []doma
 }
 
 func TestDetailShowsTheTaskAndEscGoesBack(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
 	note := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8"
 	parent, kids := h.family("Auth migration !1 ~xl @ana #auth https://x.io/pr/482 -- "+note,
@@ -68,14 +68,14 @@ func TestDetailShowsTheTaskAndEscGoesBack(t *testing.T) {
 		t.Errorf("back on the parent, the cursor is on #%d; want the child we came from (#%d)", got, kids[2].ID)
 	}
 	h.keys("esc")
-	h.mustSee("tt · Pick", "BACKLOG")
+	h.mustSee("tt · Log", "BACKLOG")
 	if r, _ := h.m.selectedRow(); r.task.ID != parent.ID {
 		t.Errorf("Pick's cursor moved to #%d", r.task.ID)
 	}
 }
 
 func TestDetailKeysActOnTheRowUnderTheCursor(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	parent, kids := h.family("Parent !1 ~l", "First ~s", "Second ~s")
 	h.keys("enter", "j")
 	h.keys("x")
@@ -101,7 +101,7 @@ func TestDetailKeysActOnTheRowUnderTheCursor(t *testing.T) {
 }
 
 func TestDetailFallsBackWhenTheTaskIsGone(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
 	parent, kids := h.family("Parent !1 ~l", "Child ~s")
 	h.keys("enter", "j", "enter")
@@ -117,11 +117,11 @@ func TestDetailFallsBackWhenTheTaskIsGone(t *testing.T) {
 		t.Fatal(err)
 	}
 	h.keys("R")
-	h.mustSee(fmt.Sprintf("#%d is gone", parent.ID), "tt · Pick")
+	h.mustSee(fmt.Sprintf("#%d is gone", parent.ID), "tt · Log")
 }
 
 func TestFallingBackClosesTheOverlay(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	parent := h.add("Parent !1 ~l")
 	h.keys("enter", "a")
 	h.mustSee("child of #1 › ")
@@ -136,7 +136,7 @@ func TestFallingBackClosesTheOverlay(t *testing.T) {
 }
 
 func TestDetailFromTodayAndChildScrolling(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	var kids []string
 	for i := 1; i <= 12; i++ {
 		kids = append(kids, fmt.Sprintf("Step %02d ~s", i))
@@ -164,7 +164,7 @@ func TestDetailFromTodayAndChildScrolling(t *testing.T) {
 }
 
 func TestDetailOfAClosedTaskAndHelp(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	a := h.add("Shipped !1 ~s")
 	h.clock.now = h.clock.now.Add(time.Hour)
 	if _, err := h.svc.Done(context.Background(), a.ID); err != nil {
@@ -181,7 +181,7 @@ func TestDetailOfAClosedTaskAndHelp(t *testing.T) {
 }
 
 func TestNewDayBannerLeavesDetailBehind(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	a := h.add("Gone !1 ~s")
 	h.keys("enter") // detail is open when the day rolls over
 	h.mustSee("#1 Gone")
@@ -190,7 +190,7 @@ func TestNewDayBannerLeavesDetailBehind(t *testing.T) {
 	h.send(clockTickMsg{})
 	h.mustSee("tt · Detail")
 	h.keys("enter") // ⏎ on the new-day banner goes to planning
-	h.mustSee("tt · Pick")
+	h.mustSee("tt · Log")
 	if len(h.m.detail.stack) != 0 {
 		t.Fatalf("detail stack survived the banner: %v", h.m.detail.stack)
 	}
```

Patch `internal/tui/edit_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/edit_test.go b/internal/tui/edit_test.go
index 51ab08c..4c47171 100644
--- a/internal/tui/edit_test.go
+++ b/internal/tui/edit_test.go
@@ -15,7 +15,7 @@ import (
 )
 
 func TestEditLine(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	a := h.add("Old title @ana #a !1 ~s https://x.io -- keep me")
 
 	h.keys("e")
@@ -47,7 +47,7 @@ func TestEditLine(t *testing.T) {
 }
 
 func TestEditLineOfAChildRefusesPriority(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	parent, kids := h.family("Parent !1 ~l", "Child ~s")
 	h.keys("enter", "j", "e")
 	h.mustSee("edit #2 › Child ~s")
@@ -64,7 +64,7 @@ func TestEditLineOfAChildRefusesPriority(t *testing.T) {
 }
 
 func TestNoteInTheEditor(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	a := h.add("Task !1 ~s -- first line")
 	var seen string
 	h.editor = func(path string) error {
@@ -138,7 +138,7 @@ func TestNoteInTheEditor(t *testing.T) {
 }
 
 func TestNoteChangedElsewhereKeepsYourText(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	a := h.add("Task !1 ~s -- original")
 	var path string
 	h.editor = func(p string) error {
@@ -167,7 +167,7 @@ func TestNoteChangedElsewhereKeepsYourText(t *testing.T) {
 // On a narrow pane the status is cut at the right edge; the path comes first
 // so the part the user needs survives the cut (D29a).
 func TestNoteConflictPathFitsANarrowPane(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	a := h.add("Task !1 ~s -- original")
 	h.send(tea.WindowSizeMsg{Width: 100, Height: 40})
 	var path string
@@ -185,7 +185,7 @@ func TestNoteConflictPathFitsANarrowPane(t *testing.T) {
 }
 
 func TestEditorTempFilesAreRemoved(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.add("Task !1 ~s -- x")
 	var paths []string
 	h.editor = func(p string) error {
@@ -206,7 +206,7 @@ func TestEditorTempFilesAreRemoved(t *testing.T) {
 }
 
 func TestOpenLink(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.add("With link !1 ~s https://x.io/pr/482")
 	h.add("Without !2 ~s")
 	h.keys("o")
@@ -246,7 +246,7 @@ func TestEditorCommand(t *testing.T) {
 
 // Task detail points at E for writing or reading the whole note.
 func TestDetailNoteHints(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.add("No note !1 ~s")
 	h.add("Long note !2 ~s -- " + strings.Repeat("line\n", 7) + "last")
 	h.keys("enter")
```

Patch `internal/tui/filter_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/filter_test.go b/internal/tui/filter_test.go
index ad96c9d..e603184 100644
--- a/internal/tui/filter_test.go
+++ b/internal/tui/filter_test.go
@@ -14,7 +14,7 @@ import (
 )
 
 func TestFilterBarNarrowsLiveAndKeepsTypingOnError(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.add("Schema review !1 ~s @ana")
 	h.add("Deploy fix !2 ~m")
 
@@ -61,7 +61,7 @@ func TestFilterBarNarrowsLiveAndKeepsTypingOnError(t *testing.T) {
 }
 
 func TestReloadWhileFilteringKeepsTheFilter(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.add("Schema review !1 ~s")
 	h.keys("/")
 	h.typeText("schema")
@@ -78,7 +78,7 @@ func TestReloadWhileFilteringKeepsTheFilter(t *testing.T) {
 }
 
 func TestTokenInputCompletesAtTheCursor(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.add("one @ana #photos !1 ~s")
 	h.add("two @andrei !1 ~s")
 
@@ -103,7 +103,7 @@ func TestTokenInputCompletesAtTheCursor(t *testing.T) {
 }
 
 func TestOverlayInputFollowsResize(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.keys("a")
 	h.send(tea.WindowSizeMsg{Width: 60, Height: 40})
 	if w, want := h.m.overlay.(*quickAdd).in.input.Width(), 60-ansi.StringWidth("add › ")-2; w != want {
@@ -117,7 +117,7 @@ func TestOverlayInputFollowsResize(t *testing.T) {
 }
 
 func TestDropKeyKeepsTheReasonOfADroppedTask(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	a := h.add("Alpha !1 ~s")
 	h.keys("d")
 	h.typeText("handed to @marko")
@@ -137,7 +137,7 @@ func TestDropKeyKeepsTheReasonOfADroppedTask(t *testing.T) {
 }
 
 func TestMatchCountSurvivesNarrowLeftPane(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.send(tea.WindowSizeMsg{Width: 100, Height: 30})
 	h.add("Schema migration !1 ~s @ana #impl")
 	h.add("Deploy fix !2 ~m")
@@ -147,7 +147,7 @@ func TestMatchCountSurvivesNarrowLeftPane(t *testing.T) {
 }
 
 func TestEscHintHiddenWhileFilterLineOpen(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.add("Schema review !1 ~s")
 	h.keys("/")
 	h.typeText("sch")
```

Patch `internal/tui/harness_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/harness_test.go b/internal/tui/harness_test.go
index 1fc1c8e..1bc65b8 100644
--- a/internal/tui/harness_test.go
+++ b/internal/tui/harness_test.go
@@ -72,6 +72,18 @@ func newHarness(t *testing.T, cfg config.Config) *harness {
 		t.Fatal("the first launch of a day shows the new-day banner")
 	}
 	h.keys("esc") // tests start with the banner dismissed
+	if h.m.screen != screenToday {
+		t.Fatalf("tt opens on Today (D35), got screen %d", h.m.screen)
+	}
+	return h
+}
+
+// newLogHarness is newHarness moved on to the Log, where most browsing
+// and editing tests start.
+func newLogHarness(t *testing.T, cfg config.Config) *harness {
+	t.Helper()
+	h := newHarness(t, cfg)
+	h.keys("L")
 	return h
 }
 
```

Patch `internal/tui/pick_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/pick_test.go b/internal/tui/pick_test.go
index 69b7715..622b923 100644
--- a/internal/tui/pick_test.go
+++ b/internal/tui/pick_test.go
@@ -13,7 +13,7 @@ import (
 )
 
 func TestGroupingCyclesAndIsRemembered(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.add("Small one !1 ~s @ana")
 	h.add("Medium one !2 ~m")
 	h.mustSee("BACKLOG · by priority", "P1 · someone is blocked", "P2 ·")
@@ -34,6 +34,7 @@ func TestGroupingCyclesAndIsRemembered(t *testing.T) {
 		m: New(context.Background(), h.svc, h.m.opts)}
 	h2.run(h2.m.Init())
 	h2.send(tea.WindowSizeMsg{Width: 140, Height: 40})
+	h2.keys("L") // a new session opens on Today
 	h2.mustSee("BACKLOG · by requester")
 
 	h.keys("g")
@@ -41,18 +42,19 @@ func TestGroupingCyclesAndIsRemembered(t *testing.T) {
 }
 
 func TestUnknownRememberedGroupingFallsBackToPriority(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	if err := h.svc.SetUIState(context.Background(), groupKey, "colour"); err != nil {
 		t.Fatal(err)
 	}
 	h2 := &harness{t: t, svc: h.svc, clock: h.clock, path: h.path,
 		m: New(context.Background(), h.svc, h.m.opts)}
 	h2.run(h2.m.Init())
+	h2.keys("L")
 	h2.mustSee("BACKLOG · by priority")
 }
 
 func TestFilteredBacklogPlacement(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
 	parent := h.add("Auth migration !1 ~xl #auth")
 	for _, line := range []string{"Rollback plan ~s", "Token spike ~m"} {
@@ -130,7 +132,7 @@ func TestFilteredBacklogPlacement(t *testing.T) {
 }
 
 func TestScrollWindowMovesOnlyWhenTheCursorLeavesIt(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	for i := 1; i <= 10; i++ {
 		h.add(fmt.Sprintf("Task %02d !1 ~s", i))
 	}
@@ -155,7 +157,7 @@ func TestScrollWindowMovesOnlyWhenTheCursorLeavesIt(t *testing.T) {
 }
 
 func TestAllDroppedChildrenHideProgress(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
 	parent := h.add("Parent !1 ~l")
 	kid := h.add("Kid ~s")
```

Create `internal/tui/today_test.go`:

```go
package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/config"
	"tt/internal/domain"
)

// block places task id on today's timeline through the service and reloads.
func (h *harness) block(id int64, start, dur time.Duration) domain.Block {
	h.t.Helper()
	b, _, err := h.svc.Block(context.Background(), id, start, dur)
	if err != nil {
		h.t.Fatal(err)
	}
	h.keys("R")
	return b
}

func TestTodayIsTheHomeScreen(t *testing.T) {
	h := newHarness(t, config.Default())
	h.mustSee("tt · Today", "TIMELINE", "CANDIDATES", " 08:00 ", " 18:30 ", "━━ now 09:00", "nothing pinned · L opens the log")
	h.mustNotSee(" 19:00 ")

	h.keys("L")
	h.mustSee("tt · Log", "BACKLOG")
	h.keys("esc")
	h.mustSee("tt · Today")

	// esc in the Log clears a filter first, then goes back to Today.
	h.add("Alpha !1 ~s")
	h.keys("L", "/")
	h.typeText("zzz")
	h.keys("enter", "esc")
	h.mustSee("tt · Log", "filter cleared")
	h.keys("esc")
	h.mustSee("tt · Today")

	// B ends Pick: Today with the candidates focused.
	h.keys("L", "p", "B")
	h.mustSee("tt · Today", "Alpha")
	if h.m.today.focus != focusCandidates {
		t.Error("B focuses the candidates")
	}

	// The Inbox returns to where it was opened from.
	h.add("Untriaged")
	h.keys("i")
	h.mustSee("tt · Inbox")
	h.keys("esc")
	h.mustSee("tt · Today")
	h.keys("L", "i", "esc")
	h.mustSee("tt · Log")
}

func TestTimelineShowsBlocks(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.add("Alpha !1 ~s")
	b := h.add("Beta !2 ~m")
	h.block(a.ID, 10*time.Hour, time.Hour)
	h.block(b.ID, 11*time.Hour+10*time.Minute, 45*time.Minute)
	h.mustSee("TIMELINE · 0/2 done", " 10:00 ▓ Alpha [P1 S]", " 10:30 ▓ ", " 11:00 ▓ 11:10–11:55 Beta [P2 M]", " 11:30 ▓ ")
	h.mustSee("CANDIDATES", "[P1 S] Alpha ▓") // blocking pins (D12a)

	if _, err := h.svc.Done(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	h.mustSee("TIMELINE · 1/2 done", " 10:00 ▓ Alpha [P1 S] ✓") // P3: done blocks stay
}

func TestTimelineStretchesToBlocksAndNow(t *testing.T) {
	h := newHarness(t, config.Default())
	late := h.add("Late one !1 ~s")
	h.block(late.ID, 21*time.Hour, time.Hour)
	h.mustSee(" 21:00 ▓ Late one", " 21:30 ▓ ")
	h.mustNotSee(" 22:00 ")

	h.clock.now = time.Date(2026, 10, 12, 6, 40, 0, 0, time.UTC)
	h.keys("R")
	h.mustSee(" 06:30 ", "━━ now 06:40")
	h.mustNotSee(" 06:00 ")
}

func TestTimelineCursorAndBlockKeys(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.add("Alpha !1 ~s")
	h.block(a.ID, 10*time.Hour, time.Hour)
	if got := h.m.today.at; got != 9*time.Hour {
		t.Fatalf("the cursor starts on the now row: %v", got)
	}
	h.keys("x")
	h.mustSee("nothing blocked here")

	h.keys("j", "j", "enter") // 10:00
	h.mustSee("tt · Detail", "#1 Alpha")
	h.keys("esc")
	h.mustSee("tt · Today")
	if got := h.m.today.at; got != 10*time.Hour {
		t.Errorf("the cursor stays on its row: %v", got)
	}
	h.keys("j", "x") // 10:30 is still Alpha's block
	h.mustSee(`✓ "Alpha"`, "1/1 done")

	// The candidates pane: its own cursor and keys.
	h.keys("tab", "enter")
	h.mustSee("tt · Detail", "#1 Alpha")
	h.keys("esc", "x")
	h.mustSee(`reopened "Alpha"`)
}

func TestPinningAParentInTheTUIWithChildren(t *testing.T) {
	h := newLogHarness(t, config.Default())
	ctx := context.Background()
	parent, _, err := h.svc.Add(ctx, "Parent !1 ~xl")
	if err != nil {
		t.Fatal(err)
	}
	var kids []domain.Task
	for _, line := range []string{"A ~s", "B ~s"} {
		k, _, err := h.svc.AddChild(ctx, parent.ID, line)
		if err != nil {
			t.Fatal(err)
		}
		kids = append(kids, k)
	}
	h.keys("R", "p")
	h.mustSee(`pinned "Parent" + 2 children · no undo, u unpins`)

	h.keys("tab", "j") // on child A in today's candidates
	h.keys("u")
	h.mustSee("pinned with its parent #1 · unpin the parent first")

	h.block(kids[1].ID, 10*time.Hour, time.Hour)
	h.keys("k", "u")
	h.mustSee(`unpinned "Parent" + 1 child · 1 blocked child kept · no undo, p pins`)
	if _, ok := h.m.idx.cand[kids[1].ID]; !ok {
		t.Error("the blocked child stays pinned")
	}
}

func TestScreensShareTheHeightBudget(t *testing.T) {
	h := newHarness(t, config.Default())
	for i := range 8 {
		h.add("Task " + string(rune('A'+i)) + " !1 ~s")
	}
	h.add("Untriaged @ana #x https://example.com -- a note")
	for _, height := range []int{6, 10, 16} {
		h.send(tea.WindowSizeMsg{Width: 100, Height: height})
		for _, keys := range [][]string{{}, {"L"}, {"i"}} {
			h.keys(keys...)
			if n := strings.Count(h.screen(), "\n") + 1; n > height {
				t.Errorf("height %d, after %v: %d lines\n%s", height, keys, n, h.screen())
			}
			h.keys("esc")
		}
	}

	// The timeline scrolls to keep its cursor visible.
	h.send(tea.WindowSizeMsg{Width: 100, Height: 10})
	for range 20 {
		h.keys("j")
	}
	h.mustSee(" 18:30 ")
	h.mustNotSee(" 09:00 ")
	for range 30 {
		h.keys("k")
	}
	h.mustSee(" 08:00 ")
}

func TestTimelineRowsAcrossADSTGap(t *testing.T) {
	ams, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.DayStart, cfg.TimelineStart, cfg.TimelineEnd = 0, 0, 6*time.Hour
	h := newHarness(t, cfg)
	h.clock.now = time.Date(2026, 3, 29, 3, 40, 0, 0, ams) // 02:00–03:00 doesn't exist this night
	h.keys("R")
	h.mustSee("Sun 29 Mar", " 02:00 ", " 02:30 ", " 03:30 ", "━━ now 03:40")
	if got := h.m.today.tl.rows[h.m.today.tl.nowRow].start; got != 3*time.Hour+30*time.Minute {
		t.Errorf("now row = %v; want 03:30 by the wall clock", got)
	}
}
```

Patch `internal/tui/tui_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/tui_test.go b/internal/tui/tui_test.go
index 8cb9be9..a263464 100644
--- a/internal/tui/tui_test.go
+++ b/internal/tui/tui_test.go
@@ -18,8 +18,8 @@ import (
 )
 
 func TestQuickAddThenTriageInTwoKeystrokes(t *testing.T) {
-	h := newHarness(t, config.Default())
-	h.mustSee("tt · Pick", "Mon 12 Oct", "nothing triaged yet")
+	h := newLogHarness(t, config.Default())
+	h.mustSee("tt · Log", "Mon 12 Oct", "nothing triaged yet")
 
 	h.keys("a")
 	h.typeText("Reply to Ana @ana #photos")
@@ -38,7 +38,7 @@ func TestQuickAddThenTriageInTwoKeystrokes(t *testing.T) {
 }
 
 func TestQuickAddDirectlyToBacklogAndEscCancels(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.keys("a")
 	h.typeText("Ready !2 ~m")
 	h.keys("enter")
@@ -78,7 +78,7 @@ func TestQuickAddDirectlyToBacklogAndEscCancels(t *testing.T) {
 }
 
 func TestQuickAddAutocomplete(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.add("one @ana #photos")
 	h.add("two @ana")
 	h.add("three @andrei")
@@ -97,7 +97,7 @@ func TestQuickAddAutocomplete(t *testing.T) {
 }
 
 func TestPickPinDoneUndo(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	a := h.add("Alpha !1 ~s")
 	h.add("Beta !2 ~m")
 	h.mustSee("[P1 S] Alpha", "[P2 M] Beta", "nothing pinned")
@@ -123,7 +123,7 @@ func TestPickPinDoneUndo(t *testing.T) {
 }
 
 func TestUndoThatConflictsSaysSo(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	a := h.add("Alpha !1 ~s")
 	h.keys("p", "tab", "x")
 	h.clock.now = h.clock.now.Add(time.Minute)
@@ -138,7 +138,7 @@ func TestUndoThatConflictsSaysSo(t *testing.T) {
 }
 
 func TestDropWithReasonAndDeleteConfirm(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	a := h.add("Alpha !1 ~s")
 	b := h.add("Beta !1 ~s")
 
@@ -165,7 +165,7 @@ func TestDropWithReasonAndDeleteConfirm(t *testing.T) {
 func TestBacklogParentExpandsAndTodayShowsChildren(t *testing.T) {
 	cfg := config.Default()
 	cfg.RolloverAlert = 1
-	h := newHarness(t, cfg)
+	h := newLogHarness(t, cfg)
 	ctx := context.Background()
 	parent := h.add("Auth migration !1 ~xl")
 	kid1 := h.add("Rollback plan ~s")
@@ -197,7 +197,7 @@ func TestBacklogParentExpandsAndTodayShowsChildren(t *testing.T) {
 }
 
 func TestExternalWriteReloads(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	other, err := store.Open(context.Background(), h.path) // e.g. `tt add` from another shell
 	if err != nil {
 		t.Fatal(err)
@@ -213,7 +213,7 @@ func TestExternalWriteReloads(t *testing.T) {
 }
 
 func TestNewDayBannerStartsPlanning(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	a := h.add("Blocked !1 ~s")
 	if _, _, err := h.svc.Block(context.Background(), a.ID, 10*time.Hour, time.Hour); err != nil {
 		t.Fatal(err)
@@ -224,7 +224,7 @@ func TestNewDayBannerStartsPlanning(t *testing.T) {
 	h.send(clockTickMsg{}) // same day again: no new banner, nothing changes
 	h.keys("enter")
 	h.mustNotSee("New day")
-	h.mustSee("tt · Pick") // empty inbox: straight to Pick
+	h.mustSee("tt · Log") // empty inbox: straight to Pick
 
 	h.add("Untriaged")
 	h.clock.now = h.clock.now.Add(24 * time.Hour)
@@ -234,7 +234,7 @@ func TestNewDayBannerStartsPlanning(t *testing.T) {
 }
 
 func TestInboxSkipAndChildPriority(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	parent := h.add("Parent !1 ~xl")
 	child := h.add("Child")
 	h.add("Other")
@@ -256,7 +256,7 @@ func TestInboxSkipAndChildPriority(t *testing.T) {
 }
 
 func TestHelpAndQuit(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.keys("?")
 	h.mustSee("KEYS", "p / u    pin to today / unpin", "any key closes")
 	h.keys("x")
@@ -268,7 +268,7 @@ func TestHelpAndQuit(t *testing.T) {
 }
 
 func TestNarrowTerminalTruncatesInsteadOfWrapping(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.add("A rather long task title that will not fit in a narrow terminal @someone !1 ~s")
 	h.keys("p")
 	h.send(tea.WindowSizeMsg{Width: 40, Height: 12})
@@ -281,7 +281,7 @@ func TestNarrowTerminalTruncatesInsteadOfWrapping(t *testing.T) {
 }
 
 func TestPasteIntoQuickAdd(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.keys("a")
 	h.send(tea.PasteMsg{Content: "Pasted title https://x.io/a"})
 	h.keys("enter")
@@ -291,7 +291,7 @@ func TestPasteIntoQuickAdd(t *testing.T) {
 }
 
 func TestCursorFollowsTaskAcrossReload(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.add("Low !2 ~s")
 	if _, _, err := h.svc.Add(context.Background(), "High !1 ~s"); err != nil {
 		t.Fatal(err)
@@ -302,7 +302,7 @@ func TestCursorFollowsTaskAcrossReload(t *testing.T) {
 }
 
 func TestCollapseFromChildSelectsParent(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
 	parent := h.add("Auth migration !1 ~xl")
 	kid1 := h.add("Rollback plan ~s")
@@ -318,7 +318,7 @@ func TestCollapseFromChildSelectsParent(t *testing.T) {
 }
 
 func TestHelpFitsWithFullBacklog(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.send(tea.WindowSizeMsg{Width: 120, Height: 20})
 	for n := 1; n <= 30; n++ {
 		h.add(fmt.Sprintf("T%d !1 ~s", n))
@@ -331,7 +331,7 @@ func TestHelpFitsWithFullBacklog(t *testing.T) {
 }
 
 func TestErrorBeforeFirstLoadIsShown(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	opts := Options{Schedule: func(time.Duration, tea.Msg) tea.Cmd { return nil }}
 	m := New(context.Background(), h.svc, opts)
 	m.Update(dayStartedMsg{err: errors.New("boom")})
@@ -348,7 +348,7 @@ func TestErrorBeforeFirstLoadIsShown(t *testing.T) {
 }
 
 func TestChildProgressExcludesDropped(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
 	parent := h.add("Parent !1 ~xl")
 	done := h.add("Finished ~s")
@@ -370,7 +370,7 @@ func TestChildProgressExcludesDropped(t *testing.T) {
 }
 
 func TestOrphanedChildrenStayInBacklog(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
 	parent := h.add("P !1 ~xl")
 	kid := h.add("Kid ~s")
@@ -394,7 +394,7 @@ func TestOrphanedChildrenStayInBacklog(t *testing.T) {
 }
 
 func TestChildOfPinnedParentNotListedInBacklog(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
 	parent := h.add("Pinned parent !1 ~xl")
 	kid := h.add("Under pin ~s")
@@ -412,14 +412,14 @@ func TestChildOfPinnedParentNotListedInBacklog(t *testing.T) {
 }
 
 func TestNewDayShownWithoutWaitingForTheClockTick(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.clock.now = h.clock.now.Add(24 * time.Hour)
 	h.send(watchTickMsg{})
 	h.mustSee("☀ New day Tue 13 Oct")
 }
 
 func TestKeyOnStaleDayRefreshesInstead(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	a := h.add("Alpha !1 ~s")
 	h.keys("p")
 	h.clock.now = h.clock.now.Add(24 * time.Hour)
@@ -431,7 +431,7 @@ func TestKeyOnStaleDayRefreshesInstead(t *testing.T) {
 }
 
 func TestDeleteConfirmKeepsHintOnNarrowScreens(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	h.add("An extremely long task title that goes on and on and on forever !1 ~s")
 	h.send(tea.WindowSizeMsg{Width: 50, Height: 20})
 	h.keys("D")
@@ -448,7 +448,7 @@ func TestDeleteConfirmKeepsHintOnNarrowScreens(t *testing.T) {
 }
 
 func TestRolloverMarkerShownInBacklog(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	a := h.add("Slipper !1 ~s")
 	h.keys("p")
 	if _, _, err := h.svc.Block(context.Background(), a.ID, 10*time.Hour, time.Hour); err != nil {
@@ -466,7 +466,7 @@ func TestRolloverMarkerShownInBacklog(t *testing.T) {
 func TestPinnedChildGetsAlertLine(t *testing.T) {
 	cfg := config.Default()
 	cfg.RolloverAlert = 1
-	h := newHarness(t, cfg)
+	h := newLogHarness(t, cfg)
 	ctx := context.Background()
 	parent := h.add("Par !1 ~xl")
 	kid := h.add("Kid ~s")
@@ -488,7 +488,7 @@ func TestPinnedChildGetsAlertLine(t *testing.T) {
 }
 
 func TestPinningClosedTaskExplainsItself(t *testing.T) {
-	h := newHarness(t, config.Default())
+	h := newLogHarness(t, config.Default())
 	ctx := context.Background()
 	parent := h.add("Par !1 ~xl")
 	kid := h.add("Kid ~s")
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL (build): `undefined: screenToday`, `h.m.today undefined`, `undefined: focusCandidates`.

- [ ] **Step 3: Implement**

Patch `internal/tui/common.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/common.go b/internal/tui/common.go
index 208e69a..e244999 100644
--- a/internal/tui/common.go
+++ b/internal/tui/common.go
@@ -93,8 +93,10 @@ func (m *Model) closedToday(t domain.Task) bool {
 }
 
 func (m *Model) headerView() string {
-	name := "Pick"
+	name := "Log"
 	switch m.screen {
+	case screenToday:
+		name = "Today"
 	case screenInbox:
 		name = "Inbox"
 	case screenDetail:
@@ -130,6 +132,9 @@ func (m *Model) statusView() string {
 }
 
 func (m *Model) hintsView() string {
+	if m.screen == screenToday {
+		return styleDim.Render(m.todayHints())
+	}
 	if m.screen == screenInbox {
 		if t, ok := m.state.Snapshot.Get(m.inbox.current); ok && t.ParentID != 0 {
 			return styleDim.Render("s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help")
@@ -139,9 +144,9 @@ func (m *Model) hintsView() string {
 	if m.screen == screenDetail {
 		return styleDim.Render("j/k move · ⏎ open child · a add child · J/K reorder · e edit · E note · o link · x done · d drop · p pin · esc back · ? help")
 	}
-	hints := "⏎ open · e edit · a add · / filter · g group · p pin · u unpin · x done · d drop · tab pane · →/← expand · i inbox · ctrl+z undo · ? help · q quit"
+	hints := "⏎ open · e edit · a add · / filter · g group · p pin · u unpin · x done · d drop · tab pane · →/← expand · B block today · esc today · i inbox · ctrl+z undo · ? help · q quit"
 	if m.pick.filtered() && m.overlay == nil { // in the filter line, esc restores the previous filter
-		hints = "esc clears filter · " + hints
+		hints = "esc clears filter · " + strings.Replace(hints, " · esc today", "", 1)
 	}
 	return styleDim.Render(hints)
 }
@@ -169,6 +174,34 @@ func ago(now, t time.Time) string {
 	}
 }
 
+// bodyHeight is how many lines a screen's body gets: the terminal minus the
+// header, status and hints lines, the banner and an open overlay (help takes
+// the whole screen and doesn't count). Every screen shares this budget.
+func (m *Model) bodyHeight() int {
+	used := 3
+	if m.banner != nil {
+		used++
+	}
+	if _, help := m.overlay.(helpOverlay); m.overlay != nil && !help {
+		used += strings.Count(m.overlay.view(m), "\n") + 1
+	}
+	return max(m.height-used, 2)
+}
+
+// clipLines keeps the first n lines of s.
+func clipLines(s string, n int) string {
+	lines := strings.Split(s, "\n")
+	return strings.Join(lines[:min(len(lines), n)], "\n")
+}
+
+// plusChildren renders " + 3 children" (D34 statuses); "" for none.
+func plusChildren(n int) string {
+	if n == 0 {
+		return ""
+	}
+	return " + " + plural(n, "child", "children")
+}
+
 // plural renders "1 match" / "3 matches".
 func plural(n int, one, many string) string {
 	if n == 1 {
```

Patch `internal/tui/detail.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/detail.go b/internal/tui/detail.go
index 07dec8f..1f596e1 100644
--- a/internal/tui/detail.go
+++ b/internal/tui/detail.go
@@ -19,8 +19,9 @@ type detailState struct {
 	stack    []int64
 	rows     []domain.Task // the task, then its children: open ones in sibling order, then closed ones
 	cursor   int
-	selected int64 // task id under the cursor; survives reloads
-	offset   int   // first child shown (scroll window)
+	selected int64  // task id under the cursor; survives reloads
+	offset   int    // first child shown (scroll window)
+	from     screen // the screen the detail was opened from; esc ends there
 }
 
 func (d *detailState) top() int64 {
@@ -30,15 +31,15 @@ func (d *detailState) top() int64 {
 	return d.stack[len(d.stack)-1]
 }
 
-// openDetail shows task id's detail, starting a new stack (from Pick).
+// openDetail shows task id's detail, starting a new stack (from Today or the Log).
 func (m *Model) openDetail(id int64) {
-	m.detail = detailState{stack: []int64{id}, selected: id}
+	m.detail = detailState{stack: []int64{id}, selected: id, from: m.screen}
 	m.screen = screenDetail
 	m.detail.refresh(m)
 }
 
 // refresh rebuilds the rows. A task that disappeared falls back one level
-// (to Pick when none is left) and says so.
+// (to the screen it was opened from when none is left) and says so.
 func (d *detailState) refresh(m *Model) {
 	snap := m.state.Snapshot
 	for len(d.stack) > 0 {
@@ -55,7 +56,7 @@ func (d *detailState) refresh(m *Model) {
 	if len(d.stack) == 0 {
 		d.rows = nil
 		if m.screen == screenDetail {
-			m.screen = screenPick
+			m.screen = d.from
 		}
 		return
 	}
@@ -82,17 +83,9 @@ func (d *detailState) move(delta int) {
 	d.selected = d.rows[d.cursor].ID
 }
 
-// childRows is how many children fit below the task's own lines.
-func (m *Model) childRows() int {
-	reserved := 4 + len(m.detailHead())
-	if m.banner != nil {
-		reserved++
-	}
-	if m.overlay != nil {
-		reserved += strings.Count(m.overlay.view(m), "\n") + 1
-	}
-	return max(m.height-reserved, 3)
-}
+// childRows is how many children fit below the task's own lines and a
+// "… N more" line.
+func (m *Model) childRows() int { return max(m.bodyHeight()-1-len(m.detailHead()), 1) }
 
 // follow keeps the cursor's child inside the scroll window (see pickState.follow).
 func (d *detailState) follow(m *Model) {
@@ -121,8 +114,8 @@ func (m *Model) detailKey(key string) tea.Cmd {
 		from := d.top()
 		d.stack = d.stack[:len(d.stack)-1]
 		if len(d.stack) == 0 {
+			m.screen = d.from
 			m.detail = detailState{}
-			m.screen = screenPick
 			return nil
 		}
 		d.selected, d.offset = from, 0 // back on the child we came from
```

Patch `internal/tui/inbox.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/inbox.go b/internal/tui/inbox.go
index d792e02..2167008 100644
--- a/internal/tui/inbox.go
+++ b/internal/tui/inbox.go
@@ -17,6 +17,13 @@ import (
 type inboxState struct {
 	current int64
 	skipped map[int64]bool
+	from    screen // esc returns here
+}
+
+// openInbox shows the Inbox; esc returns to the current screen.
+func (m *Model) openInbox() {
+	m.inbox.from, m.screen = m.screen, screenInbox
+	m.inbox.sync(m)
 }
 
 func (in *inboxState) order(m *Model) []domain.Task {
@@ -52,7 +59,7 @@ func (in *inboxState) sync(m *Model) {
 func (m *Model) inboxKey(key string) tea.Cmd {
 	in := &m.inbox
 	if key == "esc" {
-		m.screen = screenPick
+		m.screen = in.from
 		return nil
 	}
 	t, ok := m.state.Snapshot.Get(in.current)
```

Patch `internal/tui/model.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/model.go b/internal/tui/model.go
index 0e97d46..d245c73 100644
--- a/internal/tui/model.go
+++ b/internal/tui/model.go
@@ -43,7 +43,8 @@ func DefaultOptions() Options {
 type screen int
 
 const (
-	screenPick screen = iota
+	screenToday screen = iota // the home screen (D35)
+	screenLog                 // the backlog; the planning step Pick
 	screenInbox
 	screenDetail
 )
@@ -80,6 +81,7 @@ type Model struct {
 	width, height int
 
 	screen screen
+	today  todayState
 	pick   pickState
 	inbox  inboxState
 	detail detailState
@@ -145,6 +147,7 @@ func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
 	cmd := m.update(msg)
 	m.pick.follow(m)
 	m.detail.follow(m)
+	m.today.follow(m)
 	return m, cmd
 }
 
@@ -188,6 +191,7 @@ func (m *Model) update(msg tea.Msg) tea.Cmd {
 		if msg.res.New {
 			res := msg.res
 			m.banner = &res
+			m.today.placed = false // a new day's cursor starts on its now row
 		}
 		return m.load()
 	case actionMsg:
@@ -268,9 +272,9 @@ func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
 		case "enter":
 			m.banner = nil
 			m.detail = detailState{} // a detail left open overnight must not survive the banner
-			m.screen = screenPick
+			m.screen = screenLog     // planning: Inbox → Pick (in the Log) → B → Today (D35)
 			if len(m.state.Snapshot.Inbox()) > 0 {
-				m.screen = screenInbox // planning starts with triage (D12d)
+				m.openInbox() // planning starts with triage (D12d)
 			}
 			return nil
 		case "esc":
@@ -300,6 +304,8 @@ func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
 		return m.startDay()
 	}
 	switch m.screen {
+	case screenToday:
+		return m.todayKey(key)
 	case screenInbox:
 		return m.inboxKey(key)
 	case screenDetail:
@@ -382,10 +388,12 @@ func (m *Model) render() string {
 	}
 	fb, filtering := m.overlay.(*filterBar)
 	switch m.screen {
+	case screenToday:
+		parts = append(parts, m.todayView())
 	case screenInbox:
-		parts = append(parts, m.inboxView())
+		parts = append(parts, clipLines(m.inboxView(), m.bodyHeight()))
 	case screenDetail:
-		parts = append(parts, m.detailView())
+		parts = append(parts, clipLines(m.detailView(), m.bodyHeight()))
 	default:
 		if filtering {
 			parts = append(parts, fb.view(m)) // the filter line sits above the backlog
```

Patch `internal/tui/overlays.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/overlays.go b/internal/tui/overlays.go
index b3b77b9..ec00763 100644
--- a/internal/tui/overlays.go
+++ b/internal/tui/overlays.go
@@ -228,7 +228,7 @@ func (helpOverlay) view(m *Model) string {
 	case screenDetail:
 		local = []string{
 			"j k      move between the task and its children",
-			"⏎        open the child under the cursor · esc back (child → parent → Pick)",
+			"⏎        open the child under the cursor · esc back (child → parent → where you came from)",
 			"e / E    edit the line (title @who #tag !1 ~s link) / the note in $EDITOR",
 			"o        open the link",
 			"a        add children, one per line (title ~s #tag -- note; priority is the parent's)",
@@ -243,8 +243,10 @@ func (helpOverlay) view(m *Model) string {
 			"s m l x  set size (x = XL); a card with priority and size leaves the inbox",
 			"n        skip (stays in the inbox)",
 			"d / D    drop with optional reason / delete",
-			"esc      back to Pick",
+			"esc      back",
 		}
+	case screenToday:
+		local = todayHelp
 	default:
 		local = []string{
 			"⏎        task detail",
@@ -256,7 +258,8 @@ func (helpOverlay) view(m *Model) string {
 			"d / D    drop with optional reason / delete",
 			"/        filter: @who #tag !1 ~s <7d >30d is:done|dropped|all words (live; esc on the list clears)",
 			"g        group by priority → size → created → requester (remembered)",
-			"i        inbox",
+			"B        done picking: block today's candidates in Today",
+			"esc      back to Today (when no filter is set) · i inbox",
 		}
 	}
 	return styleTitle.Render("KEYS") + "\n" + strings.Join(append(local, global...), "\n") + "\n" + styleDim.Render("any key closes")
```

Patch `internal/tui/pick.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/pick.go b/internal/tui/pick.go
index 939ee24..291fa6d 100644
--- a/internal/tui/pick.go
+++ b/internal/tui/pick.go
@@ -67,6 +67,7 @@ func (m *Model) refreshRows() {
 	m.pick.rows[paneBacklog] = m.backlogRows()
 	m.pick.rows[paneToday] = m.todayRows()
 	m.pick.clamp(m)
+	m.today.refresh(m)
 	if len(m.detail.stack) > 0 {
 		m.detail.refresh(m)
 	}
@@ -224,12 +225,15 @@ func (p *pickState) clamp(m *Model) {
 	}
 }
 
-func (p *pickState) move(delta int) {
-	rows := p.rows[p.focus]
-	for c := p.cursor[p.focus] + delta; c >= 0 && c < len(rows); c += delta {
+func (p *pickState) move(delta int) { p.moveIn(p.focus, delta) }
+
+// moveIn moves pane pn's cursor to the next task row in direction delta.
+func (p *pickState) moveIn(pn pane, delta int) {
+	rows := p.rows[pn]
+	for c := p.cursor[pn] + delta; c >= 0 && c < len(rows); c += delta {
 		if rows[c].kind == rowTask {
-			p.cursor[p.focus] = c
-			p.selected[p.focus] = rows[c].task.ID
+			p.cursor[pn] = c
+			p.selected[pn] = rows[c].task.ID
 			return
 		}
 	}
@@ -267,22 +271,16 @@ func (p *pickState) follow(m *Model) {
 	}
 }
 
-// visibleRows is how many task rows fit beside the header, pane titles,
-// status and hints, plus the banner and an open overlay.
-func (m *Model) visibleRows() int {
-	reserved := 4
-	if m.banner != nil {
-		reserved++
-	}
-	if m.overlay != nil {
-		reserved += strings.Count(m.overlay.view(m), "\n") + 1
-	}
-	return max(m.height-reserved, 3)
-}
+// visibleRows is how many rows fit in a pane below its title (the Log's
+// panes, Today's candidates and timeline share this budget).
+func (m *Model) visibleRows() int { return max(m.bodyHeight()-1, 1) }
 
-func (m *Model) selectedRow() (row, bool) {
-	rows := m.rows(m.pick.focus)
-	c := m.pick.cursor[m.pick.focus]
+func (m *Model) selectedRow() (row, bool) { return m.selectedIn(m.pick.focus) }
+
+// selectedIn is the task row under pane pn's cursor.
+func (m *Model) selectedIn(pn pane) (row, bool) {
+	rows := m.rows(pn)
+	c := m.pick.cursor[pn]
 	if c < 0 || c >= len(rows) || rows[c].kind != rowTask {
 		return row{}, false
 	}
@@ -303,8 +301,10 @@ func (m *Model) pickKey(key string) tea.Cmd {
 		p.move(-1)
 		return nil
 	case "i":
-		m.screen = screenInbox
-		m.inbox.sync(m)
+		m.openInbox()
+		return nil
+	case "B":
+		m.screen, m.today.focus = screenToday, focusCandidates // Pick done: block in Today (D35)
 		return nil
 	case "/":
 		return m.openFilter()
@@ -317,7 +317,9 @@ func (m *Model) pickKey(key string) tea.Cmd {
 		if p.filtered() {
 			_ = m.applyFilter("") // an empty line always parses
 			m.setStatus("filter cleared")
+			return nil
 		}
+		m.screen = screenToday
 		return nil
 	}
 	r, ok := m.selectedRow()
@@ -369,13 +371,43 @@ func (m *Model) taskKey(key string, t domain.Task) (cmd tea.Cmd, ok bool) {
 			m.setStatus("already pinned")
 			return nil, true
 		}
-		return m.act("pin "+label, "pinned "+label, func(ctx context.Context) (app.Undo, error) { return svc.Pin(ctx, t.ID) }), true
+		status := "pinned " + label
+		if kids := m.idx.children[t.ID]; len(kids) > 0 { // D34: no undo
+			n := 0
+			for _, k := range kids {
+				if _, ok := m.idx.cand[k.ID]; !ok && k.Status == domain.StatusOpen {
+					n++
+				}
+			}
+			status += plusChildren(n) + " · no undo, u unpins"
+		}
+		return m.act("pin "+label, status, func(ctx context.Context) (app.Undo, error) { return svc.Pin(ctx, t.ID) }), true
 	case "u":
 		if !pinned {
 			m.setStatus("not pinned")
 			return nil, true
 		}
-		return m.act("unpin "+label, "unpinned "+label, func(ctx context.Context) (app.Undo, error) { return svc.Unpin(ctx, t.ID) }), true
+		if _, ok := m.idx.cand[t.ParentID]; t.ParentID != 0 && ok {
+			m.setStatus(fmt.Sprintf("pinned with its parent #%d · unpin the parent first", t.ParentID)) // D34
+			return nil, true
+		}
+		status := "unpinned " + label
+		if kids := m.idx.children[t.ID]; len(kids) > 0 { // D34: no undo
+			n, kept := 0, 0
+			for _, k := range kids {
+				if c, ok := m.idx.cand[k.ID]; ok && c.Blocked {
+					kept++
+				} else if ok {
+					n++
+				}
+			}
+			status += plusChildren(n)
+			if kept > 0 {
+				status += " · " + plural(kept, "blocked child", "blocked children") + " kept"
+			}
+			status += " · no undo, p pins"
+		}
+		return m.act("unpin "+label, status, func(ctx context.Context) (app.Undo, error) { return svc.Unpin(ctx, t.ID) }), true
 	case "x":
 		if t.Status != domain.StatusOpen {
 			return m.act("reopen "+label, "reopened "+label, func(ctx context.Context) (app.Undo, error) { return svc.Reopen(ctx, t.ID) }), true
@@ -414,8 +446,13 @@ func (m *Model) pickView() string {
 		title += " · " + plural(n, "match", "matches") + " · " + m.pick.filterText // the count first: truncation eats the filter text
 		empty = "no matches · esc clears"
 	}
-	left := m.paneLines(paneBacklog, title, empty, lw)
-	right := m.paneLines(paneToday, "TODAY'S CANDIDATES", "nothing pinned · p on a backlog task", rw)
+	left := m.paneLines(paneBacklog, title, empty, lw, m.pick.focus == paneBacklog)
+	right := m.paneLines(paneToday, "TODAY'S CANDIDATES", "nothing pinned · p on a backlog task", rw, m.pick.focus == paneToday)
+	return columns(left, right, lw, rw)
+}
+
+// columns renders two panes side by side, lw and rw cells wide.
+func columns(left, right []string, lw, rw int) string {
 	var b strings.Builder
 	for i := range max(len(left), len(right)) {
 		l, r := "", ""
@@ -433,7 +470,8 @@ func (m *Model) pickView() string {
 	return b.String()
 }
 
-func (m *Model) paneLines(pn pane, title, empty string, width int) []string {
+// paneLines renders pane pn's title and visible rows; focused shows its cursor.
+func (m *Model) paneLines(pn pane, title, empty string, width int, focused bool) []string {
 	lines := []string{styleTitle.Render(title)}
 	rows := m.rows(pn)
 	if len(rows) == 0 {
@@ -443,7 +481,7 @@ func (m *Model) paneLines(pn pane, title, empty string, width int) []string {
 	cursor := m.pick.cursor[pn]
 	start := m.pick.offset[pn]
 	for i := start; i < len(rows) && i < start+visible; i++ {
-		lines = append(lines, m.rowLine(rows[i], width, pn == m.pick.focus && i == cursor))
+		lines = append(lines, m.rowLine(rows[i], width, focused && i == cursor))
 	}
 	return lines
 }
```

Patch `internal/tui/timeline.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/timeline.go b/internal/tui/timeline.go
index ee3508d..07fbeda 100644
--- a/internal/tui/timeline.go
+++ b/internal/tui/timeline.go
@@ -21,6 +21,7 @@ type tlRow struct {
 type timeline struct {
 	rows   []tlRow
 	blocks []domain.Block // the day's blocks, by start
+	now    time.Duration  // now's wall-clock offset; -1 when there is no now
 	nowRow int            // the row holding now; -1 when there is no now
 	dayEnd time.Duration  // the planning day's end: day start + 24h
 }
@@ -38,7 +39,7 @@ func buildTimeline(blocks []domain.Block, from, to, now, dayStart time.Duration)
 	if to%rowLen != 0 {
 		to = to.Truncate(rowLen) + rowLen
 	}
-	tl := timeline{blocks: blocks, nowRow: -1, dayEnd: dayStart + 24*time.Hour}
+	tl := timeline{blocks: blocks, now: max(now, -1), nowRow: -1, dayEnd: dayStart + 24*time.Hour}
 	for s := from; s < to; s += rowLen {
 		r := tlRow{start: s}
 		for _, b := range blocks {
```

Create `internal/tui/today.go`:

```go
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/domain"
)

type todayFocus int

const (
	focusTimeline todayFocus = iota
	focusCandidates
)

// todayState is the Today screen (D35): the timeline (D38) beside today's
// candidates. The candidates pane shares the Log's candidates rows and cursor.
type todayState struct {
	focus  todayFocus
	tl     timeline
	at     time.Duration // start of the row under the timeline cursor; survives reloads
	placed bool          // at has been set; until then the cursor goes to the now row
	cursor int           // the row index of at
	offset int           // first timeline line shown (the now line counts as one)
}

// refresh rebuilds the timeline from the loaded plan and puts the cursor
// back on its row (the window may have grown or shrunk).
func (td *todayState) refresh(m *Model) {
	cfg, plan := m.svc.Config(), m.state.Plan
	now := time.Duration(-1)
	if m.svc.Today() == plan.Day {
		now = plan.Day.Offset(m.svc.Now())
	}
	td.tl = buildTimeline(plan.Blocks, cfg.TimelineStart, cfg.TimelineEnd, now, cfg.DayStart)
	rows := td.tl.rows
	if !td.placed {
		td.at, td.placed = rows[0].start, true
		if td.tl.nowRow >= 0 {
			td.at = rows[td.tl.nowRow].start
		}
	}
	td.cursor = td.tl.rowOf(td.at)
	if td.cursor < 0 { // the window shrank past the cursor
		td.cursor = 0
		if td.at > rows[0].start {
			td.cursor = len(rows) - 1
		}
	}
	td.at = rows[td.cursor].start
}

func (td *todayState) move(delta int) {
	td.cursor = max(0, min(td.cursor+delta, len(td.tl.rows)-1))
	td.at = td.tl.rows[td.cursor].start
}

// line is the timeline line row i is drawn on: the now line comes right
// after the row holding now.
func (td *todayState) line(i int) int {
	if n := td.tl.nowRow; n >= 0 && i > n {
		return i + 1
	}
	return i
}

func (td *todayState) lines() int {
	if td.tl.nowRow >= 0 {
		return len(td.tl.rows) + 1
	}
	return len(td.tl.rows)
}

// follow keeps the cursor's row inside the timeline's scroll window.
func (td *todayState) follow(m *Model) {
	if len(td.tl.rows) == 0 {
		return
	}
	visible, l := m.visibleRows(), td.line(td.cursor)
	off := min(td.offset, l)
	if l >= off+visible {
		off = l - visible + 1
	}
	td.offset = max(0, min(off, td.lines()-visible))
}

// blockKeys act on the block under the timeline cursor.
var blockKeys = map[string]bool{"enter": true, "x": true, "d": true, "D": true, "e": true, "E": true, "o": true}

func (m *Model) todayKey(key string) tea.Cmd {
	td := &m.today
	switch key {
	case "tab":
		td.focus = 1 - td.focus
		return nil
	case "L":
		m.screen = screenLog
		return nil
	case "i":
		m.openInbox()
		return nil
	}
	if td.focus == focusCandidates {
		switch key {
		case "j", "down":
			m.pick.moveIn(paneToday, +1)
			return nil
		case "k", "up":
			m.pick.moveIn(paneToday, -1)
			return nil
		}
		r, ok := m.selectedIn(paneToday)
		if !ok {
			return nil
		}
		if key == "enter" {
			m.openDetail(r.task.ID) // D31
			return nil
		}
		cmd, _ := m.taskKey(key, r.task)
		return cmd
	}
	switch key {
	case "j", "down":
		td.move(+1)
		return nil
	case "k", "up":
		td.move(-1)
		return nil
	}
	if !blockKeys[key] {
		return nil
	}
	b, ok := td.tl.blockAt(td.cursor)
	if !ok {
		m.setStatus("nothing blocked here · tab to the candidates")
		return nil
	}
	t, ok := m.state.Snapshot.Get(b.TaskID)
	if !ok {
		return nil
	}
	if key == "enter" {
		m.openDetail(t.ID)
		return nil
	}
	cmd, _ := m.taskKey(key, t)
	return cmd
}

func (m *Model) todayView() string {
	total := max(m.width-3, 20)
	lw := total / 2
	rw := total - lw
	left := m.timelineLines(lw)
	right := m.paneLines(paneToday, "CANDIDATES", "nothing pinned · L opens the log to pick", rw, m.today.focus == focusCandidates)
	return columns(left, right, lw, rw)
}

// timelineLines renders the timeline's title and the lines in its window.
func (m *Model) timelineLines(width int) []string {
	td := &m.today
	title := "TIMELINE"
	if n := len(td.tl.blocks); n > 0 {
		done := 0
		for _, b := range td.tl.blocks {
			if t, ok := m.state.Snapshot.Get(b.TaskID); ok && t.Status == domain.StatusDone {
				done++
			}
		}
		title += fmt.Sprintf(" · %d/%d done", done, n)
	}
	var all []string
	for i := range td.tl.rows {
		all = append(all, m.timelineRow(i, width))
		if i == td.tl.nowRow {
			now := "━━ now " + m.svc.Now().Format("15:04") + " "
			all = append(all, styleAlert.Render(now+strings.Repeat("━", max(width-ansi.StringWidth(now), 0))))
		}
	}
	end := min(len(all), td.offset+m.visibleRows())
	return append([]string{styleTitle.Render(title)}, all[min(td.offset, end):end]...)
}

// timelineRow renders row i: its time, then the blocks starting in it
// (▓ title [P S]), or ▓ where a block carries on from an earlier row.
func (m *Model) timelineRow(i, width int) string {
	td := &m.today
	r := td.tl.rows[i]
	var labels []string
	carried := false
	for _, b := range r.blocks {
		if b.Start >= r.start {
			labels = append(labels, m.blockLabel(b))
		} else {
			carried = true
		}
	}
	text := " " + clock(r.start) + " "
	switch {
	case len(labels) > 0:
		text += "▓ " + strings.Join(labels, " · ")
	case carried:
		text += "▓"
	}
	line := pad(text, width)
	if now := td.tl.now; now >= 0 && r.start+rowLen <= now {
		line = styleDim.Render(line) // the past
	}
	if td.focus == focusTimeline && i == td.cursor {
		line = styleSelected.Render(line)
	}
	return line
}

// blockLabel is a block's text on its first row: exact times when it is off
// the half-hour grid, the task's title and badge, struck through when closed.
func (m *Model) blockLabel(b domain.Block) string {
	t, ok := m.state.Snapshot.Get(b.TaskID)
	if !ok {
		return fmt.Sprintf("#%d", b.TaskID)
	}
	s := t.Title + " " + m.badge(t)
	if b.Start%rowLen != 0 || b.Duration%rowLen != 0 {
		s = clock(b.Start) + "–" + clock(b.End()) + " " + s
	}
	switch t.Status {
	case domain.StatusDone:
		return styleClosed.Render(s + " ✓")
	case domain.StatusDropped:
		return styleClosed.Render(s + " ✗")
	}
	return s
}

// clock renders a wall-clock offset as "09:30"; offsets past 24h wrap.
func clock(off time.Duration) string {
	minutes := int(off / time.Minute)
	return fmt.Sprintf("%02d:%02d", minutes/60%24, minutes%60)
}

func (m *Model) todayHints() string {
	if m.today.focus == focusCandidates {
		return "⏎ open · u unpin · x done · d drop · e edit · tab timeline · L log · i inbox · a add · ctrl+z undo · ? help · q quit"
	}
	return "j/k move · ⏎ open · x done · e edit · tab candidates · L log · i inbox · a add · ctrl+z undo · ? help · q quit"
}

var todayHelp = []string{
	"tab      switch timeline / candidates",
	"j k      move",
	"⏎        task detail (the block's task on the timeline)",
	"x d D    done (again: reopen) / drop / delete",
	"e / E    edit the line / the note in $EDITOR · o open the link",
	"u        on a candidate: unpin",
	"L        the log (backlog; pick today's work there, B comes back) · i inbox",
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/browse_test.go internal/tui/children_test.go internal/tui/common.go internal/tui/detail.go internal/tui/detail_test.go internal/tui/edit_test.go internal/tui/filter_test.go internal/tui/harness_test.go internal/tui/inbox.go internal/tui/model.go internal/tui/overlays.go internal/tui/pick.go internal/tui/pick_test.go internal/tui/timeline.go internal/tui/today.go internal/tui/today_test.go internal/tui/tui_test.go
git commit -m 'feat(tui): Today is the home screen with the timeline; Pick becomes the Log (D35, D38)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 4: Pick up and drop: place, move, resize, fine-tune, unblock, nudges, commit

D36, D37 and commit on Today: `b` picks a candidate up as a ghost of D24 length, `m` picks a block up, `⏎` drops (refused onto another block, the pick-up stays), `esc` cancels; `+/-`, `f` (one-line fine-tune with validation that keeps the text), `u` unblock; the L/XL and parent nudges; `c` commits with the status-line summary.

**Files:**
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/today.go`
- Create: `internal/tui/place_test.go`

**Interfaces:**
- Consumes: Task 1 (`SetBlockSpan`), Tasks 2–3 (`timeline` methods, `todayState`, `clock`, `blockKeys`), `Service.Block`/`MoveBlock`/`ResizeBlock`/`Unblock`/`CommitPlan`/`DefaultBlockLength`.
- Produces:
  - `todayState.carry *carry`; `type carry struct{ task domain.Task; block domain.Block }` (block ID 0 = placing a candidate).
  - `(*Model).pickUp(t)`, `ghost() domain.Block`, `fits(b) error` (`overlaps "Title" 10:00–11:00` / `runs past the end of the planning day`), `blockName(b)`, `drop() tea.Cmd`, `carryKey(key)`, `reshape(old, t, b) tea.Cmd`, `openFineTune(b, t) tea.Cmd`, `commitPlan() tea.Cmd`, `hasOpenChildren(t) bool`; `func span(b) string`; `func parseSpan(s string, dayStart time.Duration) (start, dur time.Duration, err error)`.
  - `prompt.check func(string) error` (enter refused while it fails; the error shows under the input); `type nudge struct{ question string; onEnter func() tea.Cmd }` (⏎ goes ahead, any other key cancels).
  - Statuses: `blocked "X" 09:30–10:30[ · big task · consider splitting]`, `moved "X" to 11:00–12:00`, `"X" now 10:00–11:30`, `unblocked "X" · still a candidate`, `plan committed · 1 block · 1h30m · 1 candidate left` (`· no blocks ·` when none), `stopped moving "X": its block is gone`.
  - Test helpers: `(*harness).pinned(line) domain.Task`, `(*harness).blocks() []domain.Block`.

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/place_test.go`:

```go
package tui

import (
	"context"
	"testing"
	"time"

	"tt/internal/config"
	"tt/internal/domain"
)

// pinned adds a task from line and pins it.
func (h *harness) pinned(line string) domain.Task {
	h.t.Helper()
	t := h.add(line)
	if _, err := h.svc.Pin(context.Background(), t.ID); err != nil {
		h.t.Fatal(err)
	}
	h.keys("R")
	return t
}

func (h *harness) blocks() []domain.Block {
	h.t.Helper()
	plan, err := h.svc.Plan(context.Background())
	if err != nil {
		h.t.Fatal(err)
	}
	return plan.Blocks
}

func TestPlaceACandidate(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.pinned("Alpha !1 ~s")
	h.keys("tab", "b")
	if h.m.today.focus != focusTimeline || h.m.today.at != 9*time.Hour {
		t.Fatalf("b moves to the timeline's first free row from now: focus %d at %v", h.m.today.focus, h.m.today.at)
	}
	h.mustSee(`placing "Alpha" · j/k move · ⏎ drop · esc cancel`, " 09:00 ░ Alpha 1h", " 09:30 ░")
	h.keys("j")
	h.mustSee(" 09:30 ░ Alpha 1h", " 10:00 ░")
	h.keys("enter")
	h.mustSee(`blocked "Alpha" 09:30–10:30`, " 09:30 ▓ Alpha [P1 S]")
	if b := h.blocks(); len(b) != 1 || b[0].TaskID != a.ID || b[0].Start != 9*time.Hour+30*time.Minute || b[0].Duration != time.Hour {
		t.Fatalf("blocks = %+v", b)
	}
	h.keys("ctrl+z")
	h.mustSee(`undid block "Alpha"`)
	if b := h.blocks(); len(b) != 0 {
		t.Errorf("after undo: %+v", b)
	}
}

func TestPlacementLengthAndFreeRows(t *testing.T) {
	h := newHarness(t, config.Default())
	busy := h.pinned("Busy !1 ~s")
	h.block(busy.ID, 9*time.Hour, time.Hour)
	later := h.pinned("Later !1 ~s")
	h.block(later.ID, 11*time.Hour, time.Hour)
	h.pinned("Medium !2 ~m")
	h.pinned("Unsized !3")

	h.keys("tab", "j", "j", "b") // Medium (candidates in plan order: Busy, Later, Medium, Unsized)
	if h.m.today.at != 10*time.Hour {
		t.Fatalf("the first free row from now is 10:00, got %v", h.m.today.at)
	}
	h.mustSee(" 10:00 ░ Medium 1h") // D24: min(4h, the hour before Later)
	h.keys("enter")
	h.mustSee(`blocked "Medium" 10:00–11:00`)

	h.keys("tab", "j", "j", "j", "b")
	h.mustSee(" 12:00 ░ Unsized 30m") // unsized tasks default to 30m
	h.keys("esc")
	h.mustSee("cancelled")
	if h.m.today.carry != nil {
		t.Error("esc cancels the pick-up")
	}
}

func TestDropOntoABlockIsRefused(t *testing.T) {
	h := newHarness(t, config.Default())
	busy := h.pinned("Busy !1 ~s")
	h.block(busy.ID, 10*time.Hour, time.Hour)
	h.pinned("Alpha !1 ~s")
	h.keys("tab", "j", "b") // at 09:00: Alpha would need 09:00–10:00, which fits
	h.keys("j", "j")        // 10:00 is inside Busy
	h.keys("enter")
	h.mustSee(`✗ overlaps "Busy" 10:00–11:00`)
	if h.m.today.carry == nil {
		t.Fatal("a refused drop keeps the block picked up")
	}
	h.keys("j", "j", "enter") // 11:00
	h.mustSee(`blocked "Alpha" 11:00–12:00`)
}

func TestPlanningNudges(t *testing.T) {
	h := newHarness(t, config.Default())
	h.pinned("Large !1 ~l")
	h.keys("tab", "b", "enter")
	h.mustSee(`blocked "Large" 09:00–17:00 · big task · consider splitting`) // D37

	ctx := context.Background()
	parent := h.pinned("Parent !1 ~xl")
	if _, _, err := h.svc.AddChild(ctx, parent.ID, "Child ~s"); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "tab", "j", "b")
	h.mustSee("parent task; blocking its children is recommended · ⏎ block anyway · esc")
	h.keys("esc")
	h.mustSee("cancelled")
	if h.m.today.carry != nil {
		t.Fatal("esc on the nudge picks nothing up")
	}
	h.keys("b", "enter")
	if c := h.m.today.carry; c == nil || c.task.ID != parent.ID {
		t.Fatalf("⏎ on the nudge picks the parent up: %+v", c)
	}
	h.mustSee(" 17:00 ░ Parent") // the first free row after Large
	h.keys("enter")
	h.mustSee(`blocked "Parent" 17:00–04:00 · big task · consider splitting`) // XL: the rest of the planning day

	done := h.pinned("Done already !2 ~s")
	if _, err := h.svc.Done(ctx, done.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "tab")
	for range 5 {
		h.keys("j")
	}
	h.keys("b")
	h.mustSee("can't block a closed task — x reopens it")
}

func TestMoveResizeFineTuneUnblock(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.pinned("Alpha !1 ~s")
	h.block(a.ID, 10*time.Hour, time.Hour)
	other := h.pinned("Other !1 ~s")
	h.block(other.ID, 13*time.Hour, time.Hour)

	h.keys("j", "j", "m") // on Alpha at 10:00
	h.mustSee(`moving "Alpha" · j/k move · ⏎ drop · esc cancel`)
	h.keys("j", "j", "enter")
	h.mustSee(`moved "Alpha" to 11:00–12:00`)
	if b := h.blocks()[0]; b.Start != 11*time.Hour || b.Duration != time.Hour {
		t.Fatalf("moved block: %+v", b)
	}
	h.keys("ctrl+z")
	h.mustSee(`undid move "Alpha"`)

	h.keys("k", "k", "+") // back on 10:00, where the undo put Alpha
	h.mustSee(`"Alpha" now 10:00–11:30`)
	h.keys("j", "j", "+") // 11:00, the last row of Alpha's block, acts on it too
	h.mustSee(`"Alpha" now 10:00–12:00`)
	h.keys("k", "k", "-", "-", "-")
	h.mustSee(`"Alpha" now 10:00–10:30`)
	h.keys("-")
	h.mustSee("can't be shorter")

	h.keys("f")
	h.mustSee(`fine-tune "Alpha" › 10:00 30m`)
	for range 10 {
		h.keys("backspace")
	}
	h.typeText("12:30 45m")
	h.keys("enter")
	h.mustSee(`✗ overlaps "Other" 13:00–14:00`)
	if h.m.overlay == nil {
		t.Fatal("a refused fine-tune keeps the line")
	}
	for range 10 {
		h.keys("backspace")
	}
	h.typeText("10:10 45m")
	h.keys("enter")
	h.mustSee(`"Alpha" now 10:10–10:55`, " 10:00 ▓ 10:10–10:55 Alpha [P1 S]")
	h.keys("ctrl+z")
	if b := h.blocks()[0]; b.Start != 10*time.Hour || b.Duration != 30*time.Minute {
		t.Errorf("fine-tune undoes in one step: %+v", b)
	}

	h.keys("u")
	h.mustSee(`unblocked "Alpha" · still a candidate`)
	if len(h.blocks()) != 1 {
		t.Error("u removes Alpha's block only")
	}
	if _, ok := h.m.idx.cand[a.ID]; !ok {
		t.Error("an unblocked task stays a candidate")
	}
}

func TestAMovedBlockThatVanishesStopsTheMove(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.pinned("Alpha !1 ~s")
	b := h.block(a.ID, 10*time.Hour, time.Hour)
	h.keys("j", "j", "m")
	if _, err := h.svc.Unblock(context.Background(), b.ID); err != nil { // e.g. from another tt
		t.Fatal(err)
	}
	h.keys("R")
	h.mustSee(`stopped moving "Alpha": its block is gone`)
	if h.m.today.carry != nil {
		t.Error("the move stops")
	}
}

func TestCommitPlan(t *testing.T) {
	h := newHarness(t, config.Default())
	h.pinned("Waiting !2 ~s")
	h.keys("c")
	h.mustSee("plan committed · no blocks · 1 candidate left", "· plan committed")

	a := h.pinned("Alpha !1 ~s")
	h.block(a.ID, 10*time.Hour, 90*time.Minute)
	h.keys("tab", "c")
	h.mustSee("plan committed · 1 block · 1h30m · 1 candidate left")
}

func TestParseSpan(t *testing.T) {
	const dayStart = 4 * time.Hour
	good := []struct {
		in         string
		start, dur time.Duration
	}{
		{"09:30 45m", 9*time.Hour + 30*time.Minute, 45 * time.Minute},
		{"9:05 1h30m", 9*time.Hour + 5*time.Minute, 90 * time.Minute},
		{"00:30 1h", 24*time.Hour + 30*time.Minute, time.Hour}, // after midnight, same planning day
	}
	for _, c := range good {
		start, dur, err := parseSpan(c.in, dayStart)
		if err != nil || start != c.start || dur != c.dur {
			t.Errorf("parseSpan(%q) = %v %v %v; want %v %v", c.in, start, dur, err, c.start, c.dur)
		}
	}
	for _, in := range []string{"", "09:30", "09:30 45m extra", "9.30 45m", "09:30 0m", "09:30 90s", "09:30 soon", "25:00 1h"} {
		if _, _, err := parseSpan(in, dayStart); err == nil {
			t.Errorf("parseSpan(%q) accepted", in)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL (build): `h.m.today.carry undefined`, `undefined: parseSpan`.

- [ ] **Step 3: Implement**

Patch `internal/tui/overlays.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/overlays.go b/internal/tui/overlays.go
index ec00763..2c74374 100644
--- a/internal/tui/overlays.go
+++ b/internal/tui/overlays.go
@@ -137,8 +137,11 @@ func (f *filterBar) resize(width int) { f.in.resize(width) }
 func (f *filterBar) view(m *Model) string { return f.in.view(m, f.err) }
 
 // prompt is a one-line text question, e.g. the optional drop reason (D23).
+// With check set, enter is refused while check fails; the text stays.
 type prompt struct {
 	input   textinput.Model
+	check   func(value string) error
+	err     error
 	onEnter func(value string) tea.Cmd
 }
 
@@ -148,9 +151,15 @@ func (p *prompt) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
 		m.overlay = nil
 		return nil
 	case "enter":
+		if p.check != nil {
+			if p.err = p.check(p.input.Value()); p.err != nil {
+				return nil
+			}
+		}
 		m.overlay = nil
 		return p.onEnter(p.input.Value())
 	}
+	p.err = nil
 	var cmd tea.Cmd
 	p.input, cmd = p.input.Update(k)
 	return cmd
@@ -164,7 +173,12 @@ func (p *prompt) paste(_ *Model, msg tea.PasteMsg) tea.Cmd {
 
 func (p *prompt) resize(width int) { fitInput(&p.input, width) }
 
-func (p *prompt) view(*Model) string { return p.input.View() }
+func (p *prompt) view(*Model) string {
+	if p.err != nil {
+		return p.input.View() + "\n" + styleError.Render("✗ "+p.err.Error())
+	}
+	return p.input.View()
+}
 
 func (m *Model) openDropPrompt(t domain.Task) tea.Cmd {
 	label := fmt.Sprintf("%q", t.Title)
@@ -209,6 +223,24 @@ func (m *Model) openDeleteConfirm(t domain.Task) tea.Cmd {
 	return nil
 }
 
+// nudge asks before something discouraged (D37): ⏎ goes ahead, any other
+// key cancels.
+type nudge struct {
+	question string
+	onEnter  func() tea.Cmd
+}
+
+func (n *nudge) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
+	m.overlay = nil
+	if k.String() == "enter" {
+		return n.onEnter()
+	}
+	m.setStatus("cancelled")
+	return nil
+}
+
+func (n *nudge) view(*Model) string { return styleAlert.Render(n.question) }
+
 // helpOverlay lists the keys; any key closes it.
 type helpOverlay struct{}
 
```

Patch `internal/tui/today.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/today.go b/internal/tui/today.go
index a821b8f..b9a2fdb 100644
--- a/internal/tui/today.go
+++ b/internal/tui/today.go
@@ -1,13 +1,17 @@
 package tui
 
 import (
+	"context"
+	"errors"
 	"fmt"
+	"slices"
 	"strings"
 	"time"
 
 	tea "charm.land/bubbletea/v2"
 	"github.com/charmbracelet/x/ansi"
 
+	"tt/internal/app"
 	"tt/internal/domain"
 )
 
@@ -27,6 +31,14 @@ type todayState struct {
 	placed bool          // at has been set; until then the cursor goes to the now row
 	cursor int           // the row index of at
 	offset int           // first timeline line shown (the now line counts as one)
+	carry  *carry        // a block being placed or moved (D36); nil otherwise
+}
+
+// carry is a picked-up block: a candidate being placed, or a block being
+// moved. It follows the timeline cursor as a ghost until ⏎ drops it.
+type carry struct {
+	task  domain.Task
+	block domain.Block // the block being moved; ID 0 when placing a candidate
 }
 
 // refresh rebuilds the timeline from the loaded plan and puts the cursor
@@ -53,6 +65,199 @@ func (td *todayState) refresh(m *Model) {
 		}
 	}
 	td.at = rows[td.cursor].start
+	td.refreshCarry(m)
+}
+
+// refreshCarry drops a pick-up whose task or block went away meanwhile (an
+// undo, another shell) and picks up the latest copy of a moved block.
+func (td *todayState) refreshCarry(m *Model) {
+	c := td.carry
+	if c == nil {
+		return
+	}
+	t, ok := m.state.Snapshot.Get(c.task.ID)
+	if !ok || t.Status != domain.StatusOpen && c.block.ID == 0 {
+		td.carry = nil
+		m.setStatus(fmt.Sprintf("stopped placing %q: it changed meanwhile", c.task.Title))
+		return
+	}
+	c.task = t
+	if c.block.ID == 0 {
+		return
+	}
+	i := slices.IndexFunc(td.tl.blocks, func(b domain.Block) bool { return b.ID == c.block.ID })
+	if i < 0 {
+		td.carry = nil
+		m.setStatus(fmt.Sprintf("stopped moving %q: its block is gone", t.Title))
+		return
+	}
+	c.block = td.tl.blocks[i]
+}
+
+// pickUp starts placing candidate t (D36): the cursor goes to the first free
+// row from now.
+func (m *Model) pickUp(t domain.Task) {
+	td := &m.today
+	td.carry = &carry{task: t}
+	td.focus = focusTimeline
+	td.cursor = td.tl.firstFree(max(td.tl.nowRow, 0))
+	td.at = td.tl.rows[td.cursor].start
+}
+
+// ghost is the carried block as it would land at the cursor: a moved block
+// keeps its length; a candidate gets its D24 length, min(size, the free time
+// from the cursor to the next block or the planning day's end).
+func (m *Model) ghost() domain.Block {
+	td := &m.today
+	c := td.carry
+	if c.block.ID != 0 {
+		b := c.block
+		b.Start = td.at
+		return b
+	}
+	// gapFrom is 0 inside a block, and a gap of 0 means "no cap": the ghost
+	// keeps its full length there and shows the clash.
+	length := m.svc.DefaultBlockLength(c.task, td.tl.gapFrom(td.at))
+	return domain.Block{TaskID: c.task.ID, Day: m.state.Plan.Day, Start: td.at, Duration: length}
+}
+
+// fits explains why b can't go where it is: another block (D36), or the end
+// of the planning day. nil when it fits.
+func (m *Model) fits(b domain.Block) error {
+	if o, clash := m.today.tl.clash(b); clash {
+		return fmt.Errorf("overlaps %s", m.blockName(o))
+	}
+	if b.End() > m.today.tl.dayEnd {
+		return errors.New("runs past the end of the planning day")
+	}
+	return nil
+}
+
+// blockName renders `"Title" 10:00–11:00`.
+func (m *Model) blockName(b domain.Block) string {
+	title := fmt.Sprintf("#%d", b.TaskID)
+	if t, ok := m.state.Snapshot.Get(b.TaskID); ok {
+		title = fmt.Sprintf("%q", t.Title)
+	}
+	return title + " " + span(b)
+}
+
+func span(b domain.Block) string { return clock(b.Start) + "–" + clock(b.End()) }
+
+// drop places or moves the carried block at the cursor. A clash keeps it
+// picked up, so another row can be tried.
+func (m *Model) drop() tea.Cmd {
+	td := &m.today
+	c, g := td.carry, m.ghost()
+	if err := m.fits(g); err != nil {
+		m.setErr(err)
+		return nil
+	}
+	td.carry = nil
+	svc, label := m.svc, fmt.Sprintf("%q", c.task.Title)
+	if c.block.ID != 0 {
+		return m.act("move "+label, "moved "+label+" to "+span(g), func(ctx context.Context) (app.Undo, error) {
+			return svc.MoveBlock(ctx, c.block.ID, g.Start)
+		})
+	}
+	status := "blocked " + label + " " + span(g)
+	if c.task.Size == domain.SizeL || c.task.Size == domain.SizeXL {
+		status += " · big task · consider splitting" // D37
+	}
+	return m.act("block "+label, status, func(ctx context.Context) (app.Undo, error) {
+		_, undo, err := svc.Block(ctx, c.task.ID, g.Start, g.Duration)
+		return undo, err
+	})
+}
+
+// carryKey handles keys while a block is picked up.
+func (m *Model) carryKey(key string) tea.Cmd {
+	td := &m.today
+	switch key {
+	case "j", "down":
+		td.move(+1)
+	case "k", "up":
+		td.move(-1)
+	case "enter":
+		return m.drop()
+	case "esc":
+		td.carry = nil
+		m.setStatus("cancelled")
+	}
+	return nil
+}
+
+// reshape resizes or re-times block b after checking it still fits.
+func (m *Model) reshape(old domain.Block, t domain.Task, b domain.Block) tea.Cmd {
+	if err := m.fits(b); err != nil {
+		m.setErr(err)
+		return nil
+	}
+	svc, label := m.svc, fmt.Sprintf("%q", t.Title)
+	return m.act("change "+label, label+" now "+span(b), func(ctx context.Context) (app.Undo, error) {
+		if b.Start == old.Start {
+			return svc.ResizeBlock(ctx, b.ID, b.Duration)
+		}
+		return svc.SetBlockSpan(ctx, b.ID, b.Start, b.Duration)
+	})
+}
+
+// openFineTune asks for a block's exact start and length (D36).
+func (m *Model) openFineTune(b domain.Block, t domain.Task) tea.Cmd {
+	dayStart := m.svc.Config().DayStart
+	in := newInput(fmt.Sprintf("fine-tune %q › ", ansi.Truncate(t.Title, 30, "…")), "09:30 45m", m.width)
+	in.SetValue(clock(b.Start) + " " + shortDur(b.Duration))
+	in.CursorEnd()
+	nb := b
+	m.overlay = &prompt{
+		input: in,
+		check: func(v string) error {
+			start, dur, err := parseSpan(v, dayStart)
+			if err != nil {
+				return err
+			}
+			nb.Start, nb.Duration = start, dur
+			return m.fits(nb)
+		},
+		onEnter: func(string) tea.Cmd { return m.reshape(b, t, nb) },
+	}
+	return nil
+}
+
+// parseSpan reads a fine-tune line "HH:MM <length>", e.g. "09:30 45m". A
+// time before the day start is after midnight, in the same planning day.
+func parseSpan(s string, dayStart time.Duration) (start, dur time.Duration, err error) {
+	f := strings.Fields(s)
+	if len(f) != 2 {
+		return 0, 0, errors.New("want a start and a length, like 09:30 45m")
+	}
+	at, err := time.Parse("15:04", f[0])
+	if err != nil {
+		return 0, 0, fmt.Errorf("start %q: want HH:MM", f[0])
+	}
+	start = time.Duration(at.Hour())*time.Hour + time.Duration(at.Minute())*time.Minute
+	if start < dayStart {
+		start += 24 * time.Hour
+	}
+	dur, err = time.ParseDuration(f[1])
+	if err != nil || dur < time.Minute || dur%time.Minute != 0 {
+		return 0, 0, fmt.Errorf("length %q: want whole minutes, like 45m or 1h30m", f[1])
+	}
+	return start, dur, nil
+}
+
+// commitPlan commits today's plan and shows the summary (D22, D35).
+func (m *Model) commitPlan() tea.Cmd {
+	svc, ctx := m.svc, m.ctx
+	return func() tea.Msg {
+		sum, err := svc.CommitPlan(ctx)
+		status := "plan committed · no blocks"
+		if sum.Blocks > 0 {
+			status = "plan committed · " + plural(sum.Blocks, "block", "blocks") + " · " + shortDur(sum.Blocked)
+		}
+		status += " · " + plural(sum.Candidates, "candidate", "candidates") + " left"
+		return actionMsg{status: status, err: err}
+	}
 }
 
 func (td *todayState) move(delta int) {
@@ -90,11 +295,19 @@ func (td *todayState) follow(m *Model) {
 }
 
 // blockKeys act on the block under the timeline cursor.
-var blockKeys = map[string]bool{"enter": true, "x": true, "d": true, "D": true, "e": true, "E": true, "o": true}
+var blockKeys = map[string]bool{
+	"enter": true, "x": true, "d": true, "D": true, "e": true, "E": true, "o": true,
+	"m": true, "+": true, "=": true, "-": true, "f": true, "u": true,
+}
 
 func (m *Model) todayKey(key string) tea.Cmd {
 	td := &m.today
+	if td.carry != nil {
+		return m.carryKey(key)
+	}
 	switch key {
+	case "c":
+		return m.commitPlan()
 	case "tab":
 		td.focus = 1 - td.focus
 		return nil
@@ -118,11 +331,27 @@ func (m *Model) todayKey(key string) tea.Cmd {
 		if !ok {
 			return nil
 		}
-		if key == "enter" {
-			m.openDetail(r.task.ID) // D31
+		t := r.task
+		switch key {
+		case "enter":
+			m.openDetail(t.ID) // D31
+			return nil
+		case "b":
+			if t.Status != domain.StatusOpen {
+				m.setStatus("can't block a closed task — x reopens it")
+				return nil
+			}
+			if m.hasOpenChildren(t) { // D37
+				m.overlay = &nudge{
+					question: "parent task; blocking its children is recommended · ⏎ block anyway · esc",
+					onEnter:  func() tea.Cmd { m.pickUp(t); return nil },
+				}
+				return nil
+			}
+			m.pickUp(t)
 			return nil
 		}
-		cmd, _ := m.taskKey(key, r.task)
+		cmd, _ := m.taskKey(key, t)
 		return cmd
 	}
 	switch key {
@@ -145,14 +374,41 @@ func (m *Model) todayKey(key string) tea.Cmd {
 	if !ok {
 		return nil
 	}
-	if key == "enter" {
+	switch key {
+	case "enter":
 		m.openDetail(t.ID)
 		return nil
+	case "m":
+		td.carry = &carry{task: t, block: b}
+		return nil
+	case "+", "=":
+		nb := b
+		nb.Duration += rowLen
+		return m.reshape(b, t, nb)
+	case "-":
+		if b.Duration <= rowLen {
+			m.setStatus("can't be shorter · f sets an exact length · u unblocks")
+			return nil
+		}
+		nb := b
+		nb.Duration -= rowLen
+		return m.reshape(b, t, nb)
+	case "f":
+		return m.openFineTune(b, t)
+	case "u":
+		label := fmt.Sprintf("%q", t.Title)
+		return m.act("unblock "+label, "unblocked "+label+" · still a candidate", func(ctx context.Context) (app.Undo, error) {
+			return m.svc.Unblock(ctx, b.ID)
+		})
 	}
 	cmd, _ := m.taskKey(key, t)
 	return cmd
 }
 
+func (m *Model) hasOpenChildren(t domain.Task) bool {
+	return slices.ContainsFunc(m.idx.children[t.ID], func(k domain.Task) bool { return k.Status == domain.StatusOpen })
+}
+
 func (m *Model) todayView() string {
 	total := max(m.width-3, 20)
 	lw := total / 2
@@ -188,10 +444,16 @@ func (m *Model) timelineLines(width int) []string {
 }
 
 // timelineRow renders row i: its time, then the blocks starting in it
-// (▓ title [P S]), or ▓ where a block carries on from an earlier row.
+// (▓ title [P S]), or ▓ where a block carries on from an earlier row. A
+// picked-up block shows as a ░ ghost over the rows it would take.
 func (m *Model) timelineRow(i, width int) string {
 	td := &m.today
 	r := td.tl.rows[i]
+	if td.carry != nil {
+		if g := m.ghost(); r.start < g.End() && g.Start < r.start+rowLen {
+			return m.ghostRow(r, g, width)
+		}
+	}
 	var labels []string
 	carried := false
 	for _, b := range r.blocks {
@@ -218,6 +480,25 @@ func (m *Model) timelineRow(i, width int) string {
 	return line
 }
 
+// ghostRow renders a row the carried block would take; it turns the alert
+// colour when the block doesn't fit there.
+func (m *Model) ghostRow(r tlRow, g domain.Block, width int) string {
+	text := " " + clock(r.start) + " ░"
+	if r.start <= g.Start {
+		text += fmt.Sprintf(" %s %s", m.today.carry.task.Title, shortDur(g.Duration))
+	}
+	for _, b := range r.blocks {
+		if b.ID != g.ID {
+			text += " · " + m.blockLabel(b)
+		}
+	}
+	line := pad(text, width)
+	if m.fits(g) != nil {
+		return styleError.Render(line)
+	}
+	return styleSelected.Render(line)
+}
+
 // blockLabel is a block's text on its first row: exact times when it is off
 // the half-hour grid, the task's title and badge, struck through when closed.
 func (m *Model) blockLabel(b domain.Block) string {
@@ -245,18 +526,30 @@ func clock(off time.Duration) string {
 }
 
 func (m *Model) todayHints() string {
+	if c := m.today.carry; c != nil {
+		verb := "placing"
+		if c.block.ID != 0 {
+			verb = "moving"
+		}
+		return fmt.Sprintf("%s %q · j/k move · ⏎ drop · esc cancel", verb, c.task.Title)
+	}
 	if m.today.focus == focusCandidates {
-		return "⏎ open · u unpin · x done · d drop · e edit · tab timeline · L log · i inbox · a add · ctrl+z undo · ? help · q quit"
+		return "b block · ⏎ open · u unpin · x done · d drop · e edit · c commit · tab timeline · L log · i inbox · a add · ctrl+z undo · ? help · q quit"
 	}
-	return "j/k move · ⏎ open · x done · e edit · tab candidates · L log · i inbox · a add · ctrl+z undo · ? help · q quit"
+	return "j/k move · m move · +/- size · f fine-tune · u unblock · ⏎ open · x done · c commit · tab candidates · L log · i inbox · ctrl+z undo · ? help"
 }
 
 var todayHelp = []string{
 	"tab      switch timeline / candidates",
 	"j k      move",
+	"b        on a candidate: pick it up; j/k choose the row, ⏎ drops it, esc cancels",
+	"m        on a block: pick it up to move it (keeps its length)",
+	"+ / -    on a block: 30m longer / shorter",
+	"f        on a block: fine-tune start and length (09:30 45m)",
+	"u        on a block: unblock (stays a candidate) · on a candidate: unpin",
 	"⏎        task detail (the block's task on the timeline)",
 	"x d D    done (again: reopen) / drop / delete",
 	"e / E    edit the line / the note in $EDITOR · o open the link",
-	"u        on a candidate: unpin",
+	"c        commit the plan (again: refresh)",
 	"L        the log (backlog; pick today's work there, B comes back) · i inbox",
 }
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/overlays.go internal/tui/place_test.go internal/tui/today.go
git commit -m 'feat(tui): pick up and drop blocks, move/resize/fine-tune/unblock, nudges, commit (D36, D37)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---


## After the last task

- Build into the scratchpad and drive the real binary through a Python `pty` script on a throwaway database (`TT_DB=… TT_CONFIG=/nonexistent`): it opens on `tt · Today` with the timeline and the now line; `esc` (banner), `L` → Log, `a` + `Alpha !1 ~s` + `⏎`, `p`, `B` → Today, `b` → `placing`, `⏎` → `blocked`, `c` → `plan committed`, `q`; then the database has one block for today and `days.committed_at` set.
- `go test -race ./internal/tui/ ./internal/app/`.
- Final whole-branch review, one fix wave, park residuals as spec carry-forwards.
