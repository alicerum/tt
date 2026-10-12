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
