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
	if _, err := s.SetImpact(ctx, task.ID, 2); err != nil {
		t.Fatal(err)
	}
	if err := addUndo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo Add after a later change: error = %v; want ErrConflict", err)
	}
	if got := mustGet(t, s, task.ID); got.Impact != 2 {
		t.Errorf("task must survive a refused undo: %+v", got)
	}
}

func TestUndoDoneAfterDeleteConflicts(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Doomed")
	undo := mustUndo(t)(s.Done(ctx, task.ID))
	if err := s.Delete(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := undo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo Done after Delete: %v; want ErrConflict", err)
	}
}

func TestUndoUnblockAfterTaskDeleteConflicts(t *testing.T) {
	s, _ := startedService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Doomed ~s")
	b, _ := mustBlock(t, s, task.ID, 10*h, h)
	undo := mustUndo(t)(s.Unblock(ctx, b.ID))
	if err := s.Delete(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := undo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo Unblock after task Delete: %v; want ErrConflict", err)
	}
}

func TestUndoMoveAfterUnblockConflicts(t *testing.T) {
	s, _ := startedService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Moved ~s")
	b, _ := mustBlock(t, s, task.ID, 10*h, h)
	undo := mustUndo(t)(s.MoveBlock(ctx, b.ID, 12*h))
	if _, err := s.Unblock(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := undo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo Move after Unblock: %v; want ErrConflict", err)
	}
}

func TestUndoRefusesAfterParentDeleted(t *testing.T) {
	s, clock := newService(t)
	ctx := context.Background()
	parent := mustAdd(t, s, "Parent !1 ~xl")
	kid := mustAdd(t, s, "Kid")
	if _, err := s.SetParent(ctx, kid.ID, parent.ID); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Minute)
	sizeUndo, err := s.SetSize(ctx, kid.ID, "s")
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Minute)
	if err := s.Delete(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}
	if err := sizeUndo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo after the parent was deleted: %v; want ErrConflict", err)
	}
}
