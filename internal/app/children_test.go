package app

import (
	"context"
	"errors"
	"fmt"
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
	if got.ParentID != parent.ID || got.Size != domain.SizeS || got.Note != "try the new lib" || got.Impact != 0 ||
		!slices.Equal(got.Tags, []string{"ops"}) {
		t.Errorf("child: %+v", got)
	}
	if domain.InInbox(got, &parent) {
		t.Error("a sized child inherits impact and skips the Inbox")
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
	if !errors.Is(err, ErrInvalidValue) || !strings.Contains(err.Error(), "children inherit impact from #1; drop !1") {
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

func TestChildImpactIsRefusedInEdits(t *testing.T) {
	must := mustUndo(t)
	s, clock := newService(t)
	ctx := context.Background()
	parent := mustAdd(t, s, "Parent !3 ^fri ~l")
	child := mustAdd(t, s, "Child !1 ~s") // own impact from before it became a child
	clock.Advance(time.Minute)
	must(s.SetParent(ctx, child.ID, parent.ID))
	got := mustGet(t, s, child.ID)

	if line := EditLine(got); line != "Child ~s" {
		t.Errorf("EditLine of a child = %q; want no impact, and no inherited due (D32, D73)", line)
	}
	if line := EditLine(mustGet(t, s, parent.ID)); line != "Parent !3 ^2026-10-16 ~l" {
		t.Errorf("EditLine of a parent = %q", line)
	}
	if _, err := s.ParseEdit(got, "Child !2 ~s"); !errors.Is(err, ErrInvalidValue) ||
		!strings.Contains(err.Error(), fmt.Sprintf("children inherit impact from #%d", parent.ID)) {
		t.Errorf("ParseEdit with !n on a child: %v", err)
	}
	if _, err := s.Edit(ctx, child.ID, "Child !2 ~s"); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("Edit with !n on a child: %v", err)
	}
	if _, err := s.SetImpact(ctx, child.ID, 2); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("SetImpact on a child: %v; want ErrInvalidValue", err)
	}
	clock.Advance(time.Minute)
	must(s.Edit(ctx, child.ID, "Child renamed ^tom ~m"))
	if got := mustGet(t, s, child.ID); got.Title != "Child renamed" || got.Size != domain.SizeM || got.Due != "2026-10-13" {
		t.Errorf("edited child: %+v; want its own due", got)
	}
	if line := EditLine(mustGet(t, s, child.ID)); line != "Child renamed ^2026-10-13 ~m" {
		t.Errorf("EditLine of a child with its own due = %q", line)
	}
	kid := mustAddChild(t, s, parent.ID, "Kid ^3d ~s")
	if got := mustGet(t, s, kid.ID); got.Due != "2026-10-15" || got.Impact != 0 {
		t.Errorf("AddChild with ^3d: %+v", got)
	}
	if _, _, err := s.AddChild(ctx, parent.ID, "Kid !2 ~s"); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("AddChild with !n: %v", err)
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
