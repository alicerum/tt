package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"tt/internal/domain"
)

func requesters(t *testing.T, s *Service, ids ...int64) []string {
	t.Helper()
	var out []string
	for _, id := range ids {
		out = append(out, mustGet(t, s, id).Requester)
	}
	return out
}

func TestRenameRequester(t *testing.T) {
	s, clock := newService(t)
	ctx := context.Background()
	open := mustAdd(t, s, "Open ask @raf !1 ~s")
	done := mustAdd(t, s, "Done ask @raf !1 ~s")
	parent := mustAdd(t, s, "Parent !1 ~l")
	child := mustAddChild(t, s, parent.ID, "Child @raf ~s")
	other := mustAdd(t, s, "Other @ana !1 ~s")
	nobody := mustAdd(t, s, "Nobody's !1 ~s")
	if _, err := s.Done(ctx, done.ID); err != nil {
		t.Fatal(err)
	}
	untouched := []domain.Task{mustGet(t, s, parent.ID), mustGet(t, s, other.ID), mustGet(t, s, nobody.ID)}

	clock.Advance(time.Minute)
	n, undo, err := s.RenameRequester(ctx, "raf", "  @Raff ")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("renamed %d tasks; want 3 (open, closed and a child)", n)
	}
	if got := requesters(t, s, open.ID, done.ID, child.ID); !cmp.Equal(got, []string{"raff", "raff", "raff"}) {
		t.Errorf("requesters = %v; want raff (normalized like @who)", got)
	}
	for _, id := range []int64{open.ID, done.ID, child.ID} {
		if got := mustGet(t, s, id); !got.UpdatedAt.Equal(clock.now) {
			t.Errorf("#%d updated_at = %v; want now, so older undos see the change", id, got.UpdatedAt)
		}
	}
	if got := mustGet(t, s, done.ID); got.Status != domain.StatusDone {
		t.Errorf("a renamed task keeps its status: %s", got.Status)
	}
	for _, want := range untouched {
		if got := mustGet(t, s, want.ID); !cmp.Equal(got, want) {
			t.Errorf("#%d isn't raf's and must not change:\n%s", want.ID, cmp.Diff(want, got))
		}
	}

	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := requesters(t, s, open.ID, done.ID, child.ID); !cmp.Equal(got, []string{"raf", "raf", "raf"}) {
		t.Errorf("after undo: %v; want raf back", got)
	}
}

func TestRenameRequesterMerges(t *testing.T) {
	s, clock := newService(t)
	ctx := context.Background()
	raf1 := mustAdd(t, s, "One @raf !1 ~s")
	ana := mustAdd(t, s, "Ana's own @ana !1 ~s")
	raf2 := mustAdd(t, s, "Two @raf !2 ~s")

	clock.Advance(time.Minute)
	n, undo, err := s.RenameRequester(ctx, "raf", "ana")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("merged %d tasks; want 2", n)
	}
	if got := requesters(t, s, raf1.ID, ana.ID, raf2.ID); !cmp.Equal(got, []string{"ana", "ana", "ana"}) {
		t.Errorf("after the merge: %v", got)
	}
	ownBefore := mustGet(t, s, ana.ID)

	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := requesters(t, s, raf1.ID, ana.ID, raf2.ID); !cmp.Equal(got, []string{"raf", "ana", "raf"}) {
		t.Errorf("the merge's undo gives raf back exactly the tasks it moved: %v", got)
	}
	if got := mustGet(t, s, ana.ID); !cmp.Equal(got, ownBefore) {
		t.Errorf("the merge's undo touched ana's own task:\n%s", cmp.Diff(ownBefore, got))
	}
}

func TestRenameRequesterUndoIsAllOrNothing(t *testing.T) {
	s, clock := newService(t)
	ctx := context.Background()
	a := mustAdd(t, s, "A @raf !1 ~s")
	b := mustAdd(t, s, "B @raf !1 ~s")
	clock.Advance(time.Minute)
	_, undo, err := s.RenameRequester(ctx, "raf", "raff")
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Minute)
	if _, err := s.Done(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := undo(ctx); !errors.Is(err, ErrConflict) {
		t.Fatalf("undo after one task changed: %v; want ErrConflict", err)
	}
	if got := requesters(t, s, a.ID, b.ID); !cmp.Equal(got, []string{"raff", "raff"}) {
		t.Errorf("a refused undo changes nothing: %v", got)
	}

	clock.Advance(time.Minute)
	_, undo, err = s.RenameRequester(ctx, "raff", "rafa")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := undo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo after one task was deleted: %v; want ErrConflict", err)
	}
	if got := requesters(t, s, b.ID); !cmp.Equal(got, []string{"rafa"}) {
		t.Errorf("a refused undo changes nothing: %v", got)
	}
}

func TestRenameRequesterRefusals(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	raf := mustAdd(t, s, "A @raf !1 ~s")
	nobody := mustAdd(t, s, "No requester !1 ~s")

	for _, to := range []string{"", "  ", "@", "two words"} {
		if _, _, err := s.RenameRequester(ctx, "raf", to); !errors.Is(err, ErrInvalidValue) {
			t.Errorf("rename to %q: %v; want ErrInvalidValue", to, err)
		}
	}
	if _, _, err := s.RenameRequester(ctx, "", "ana"); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("rename of no name: %v; want ErrInvalidValue", err)
	}
	if got := mustGet(t, s, nobody.ID).Requester; got != "" {
		t.Errorf("a task without a requester got %q", got)
	}
	if _, _, err := s.RenameRequester(ctx, "zed", "ana"); !errors.Is(err, ErrNotFound) {
		t.Errorf("rename of a name no task has: %v; want ErrNotFound", err)
	}
	n, undo, err := s.RenameRequester(ctx, "raf", "@RAF")
	if err != nil || n != 0 || undo != nil {
		t.Errorf("rename to the same name = %d, undo %v, %v; want nothing done", n, undo != nil, err)
	}
	if got := mustGet(t, s, raf.ID).Requester; got != "raf" {
		t.Errorf("raf's task: %q", got)
	}
}
