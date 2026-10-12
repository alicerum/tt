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
