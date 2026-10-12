package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/store"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newService(t *testing.T) (*Service, *fakeClock) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "tt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	clock := &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
	return New(st, clock, config.Default()), clock
}

func mustAdd(t *testing.T, s *Service, line string) domain.Task {
	t.Helper()
	task, _, err := s.Add(context.Background(), line)
	if err != nil {
		t.Fatalf("Add(%q): %v", line, err)
	}
	return task
}

func mustGet(t *testing.T, s *Service, id int64) domain.Task {
	t.Helper()
	snap, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	task, ok := snap.Get(id)
	if !ok {
		t.Fatalf("task #%d not in snapshot", id)
	}
	return task
}

// mustUndo returns a helper that fails the test on error: must(s.Done(ctx, id)).
func mustUndo(t *testing.T) func(Undo, error) Undo {
	return func(undo Undo, err error) Undo {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return undo
	}
}

func TestAddAndUndo(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()

	task, undo, err := s.Add(ctx, "Reply to Ana @ana #photos !1 ~s https://x.io -- before Friday")
	if err != nil {
		t.Fatal(err)
	}
	got := mustGet(t, s, task.ID)
	want := domain.Task{
		ID: task.ID, Title: "Reply to Ana", Note: "before Friday", Link: "https://x.io", Requester: "ana",
		Tags: []string{"photos"}, Priority: 1, Size: domain.SizeS, Status: domain.StatusOpen,
		CreatedAt: got.CreatedAt, UpdatedAt: got.UpdatedAt,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("added task (-want +got):\n%s", diff)
	}
	if domain.InInbox(got, nil) {
		t.Error("task with priority and size should skip the Inbox")
	}

	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Snapshot(ctx)
	if len(snap.Tasks()) != 0 {
		t.Error("undo of Add should delete the task")
	}
}

func TestAddRejectsBadLines(t *testing.T) {
	s, _ := newService(t)
	for _, line := range []string{"", "@ana #x", "x !4"} {
		if _, _, err := s.Add(context.Background(), line); err == nil {
			t.Errorf("Add(%q) succeeded; want error", line)
		}
	}
}

func TestEditReplacesFieldsAndKeepsNote(t *testing.T) {
	must := mustUndo(t)
	s, clock := newService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Old title @ana #a !1 ~s -- keep me")

	if line := EditLine(mustGet(t, s, task.ID)); line != "Old title @ana #a !1 ~s" {
		t.Errorf("EditLine = %q", line)
	}

	clock.Advance(time.Minute)
	undo := must(s.Edit(ctx, task.ID, "New title #b ~l"))
	got := mustGet(t, s, task.ID)
	if got.Title != "New title" || got.Requester != "" || got.Priority != 0 || got.Size != domain.SizeL ||
		!cmp.Equal(got.Tags, []string{"b"}) || got.Note != "keep me" {
		t.Errorf("after Edit: %+v", got)
	}
	if !got.UpdatedAt.Equal(clock.now) {
		t.Errorf("UpdatedAt = %v; want %v", got.UpdatedAt, clock.now)
	}

	must(s.Edit(ctx, task.ID, "New title -- "))
	if got := mustGet(t, s, task.ID); got.Note != "" {
		t.Errorf("note = %q; an explicit empty note should clear it", got.Note)
	}

	// Undo restores the state before the first edit, note included.
	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, s, task.ID); got.Title != "Old title" || got.Note != "keep me" || got.Requester != "ana" {
		t.Errorf("after undo: %+v", got)
	}
}

func TestSetNote(t *testing.T) {
	must := mustUndo(t)
	s, _ := newService(t)
	task := mustAdd(t, s, "Task")
	must(s.SetNote(context.Background(), task.ID, "# Heading\n\nbody\n\n"))
	if got := mustGet(t, s, task.ID).Note; got != "# Heading\n\nbody" {
		t.Errorf("note = %q; want trailing newlines trimmed", got)
	}
}

func TestTriageSetters(t *testing.T) {
	must := mustUndo(t)
	s, _ := newService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Triage me")

	must(s.SetPriority(ctx, task.ID, 2))
	undo := must(s.SetSize(ctx, task.ID, domain.SizeXL))
	got := mustGet(t, s, task.ID)
	if got.Priority != 2 || got.Size != domain.SizeXL || domain.InInbox(got, nil) {
		t.Errorf("after triage: %+v", got)
	}
	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, s, task.ID); got.Size != domain.SizeNone || got.Priority != 2 {
		t.Errorf("undo of SetSize: %+v", got)
	}

	for _, p := range []int{-1, 4} {
		if _, err := s.SetPriority(ctx, task.ID, p); !errors.Is(err, ErrInvalidValue) {
			t.Errorf("SetPriority(%d) error = %v; want ErrInvalidValue", p, err)
		}
	}
	for _, size := range []domain.Size{"x", "xs"} {
		if _, err := s.SetSize(ctx, task.ID, size); !errors.Is(err, ErrInvalidValue) {
			t.Errorf("SetSize(%q) error = %v; want ErrInvalidValue", size, err)
		}
	}
	if _, err := s.SetPriority(ctx, 999, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetPriority on missing task error = %v; want ErrNotFound", err)
	}
}

func TestSetParentRules(t *testing.T) {
	must := mustUndo(t)
	s, _ := newService(t)
	ctx := context.Background()
	parent := mustAdd(t, s, "Auth migration !1 #photos ~xl")
	a := mustAdd(t, s, "Rollback plan ~s")
	b := mustAdd(t, s, "Token refresh spike ~m")
	other := mustAdd(t, s, "Other")

	must(s.SetParent(ctx, a.ID, parent.ID))
	undo := must(s.SetParent(ctx, b.ID, parent.ID))

	snap, _ := s.Snapshot(ctx)
	gotA, _ := snap.Get(a.ID)
	gotB, _ := snap.Get(b.ID)
	if gotA.Position != 0 || gotB.Position != 1 {
		t.Errorf("positions = %d, %d; want 0, 1", gotA.Position, gotB.Position)
	}
	if domain.InInbox(gotB, snap.ParentOf(gotB)) {
		t.Error("sized child of a prioritized parent should not be in the Inbox")
	}

	cases := []struct {
		name          string
		child, parent int64
	}{
		{"self", other.ID, other.ID},
		{"grandchild", other.ID, a.ID},
		{"parent becoming a child", parent.ID, other.ID},
	}
	for _, c := range cases {
		if _, err := s.SetParent(ctx, c.child, c.parent); !errors.Is(err, ErrInvalidParent) {
			t.Errorf("%s: error = %v; want ErrInvalidParent", c.name, err)
		}
	}
	if _, err := s.SetParent(ctx, other.ID, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing parent: error = %v; want ErrNotFound", err)
	}

	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, s, b.ID); got.ParentID != 0 {
		t.Errorf("undo of SetParent: ParentID = %d; want 0", got.ParentID)
	}
	must(s.SetParent(ctx, a.ID, 0))
	if got := mustGet(t, s, a.ID); got.ParentID != 0 {
		t.Errorf("detach: ParentID = %d; want 0", got.ParentID)
	}
}

func TestLifecycle(t *testing.T) {
	must := mustUndo(t)
	s, clock := newService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Lifecycle !1 ~s")

	clock.Advance(time.Hour)
	undo := must(s.Done(ctx, task.ID))
	got := mustGet(t, s, task.ID)
	if got.Status != domain.StatusDone || got.ClosedBy != domain.ClosedByUser || !got.ClosedAt.Equal(clock.now) {
		t.Errorf("after Done: %+v", got)
	}
	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, s, task.ID); got.Status != domain.StatusOpen || !got.ClosedAt.IsZero() {
		t.Errorf("after undo Done: %+v", got)
	}

	must(s.Drop(ctx, task.ID, "  handed to @marko  "))
	if got := mustGet(t, s, task.ID); got.Status != domain.StatusDropped || got.DropReason != "handed to @marko" {
		t.Errorf("after Drop: %+v", got)
	}
	must(s.Reopen(ctx, task.ID))
	if got := mustGet(t, s, task.ID); got.Status != domain.StatusOpen || got.DropReason != "" || got.ClosedBy != "" {
		t.Errorf("after Reopen: %+v", got)
	}

	if err := s.Delete(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, task.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete error = %v; want ErrNotFound", err)
	}
}
