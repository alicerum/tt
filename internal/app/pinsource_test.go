package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/source"
	"tt/internal/store"
)

func mustPinSource(t *testing.T, s *Service, extID string) (domain.Task, Undo) {
	t.Helper()
	task, undo, err := s.PinSource(context.Background(), "github", extID)
	if err != nil {
		t.Fatalf("PinSource(%s): %v", extID, err)
	}
	return task, undo
}

func TestPinSourceCreatesTheTask(t *testing.T) {
	s, clock, src, _ := srcService(t)
	ctx := context.Background()
	src.Set([]domain.SourceItem{review(901, "Add idempotency key", clock.now.Add(-time.Hour))}, nil)
	mustSyncSources(t, s)

	task, undo := mustPinSource(t, s, "acme/api#901")
	got := mustGet(t, s, task.ID)
	if got.Title != "Review #901: Add idempotency key" || got.Link != "https://github.com/acme/api/pull/901" ||
		got.Source == nil || *got.Source != (domain.SourceRef{Kind: "github", ID: "acme/api#901"}) ||
		got.Impact != 1 || got.Due != "2026-10-15" || got.Size != domain.SizeS || got.Requester != "" || got.Status != domain.StatusOpen {
		t.Errorf("task = %+v; want D20b's wrapper with D75's impact 1, due 3 days after the request, S and no requester", got)
	}
	if !pinned(t, s, task.ID) {
		t.Error("the review is a candidate now")
	}
	if snap := mustLoad(t, s); len(snap.GitHub.Reviews) != 1 || len(snap.Snapshot.Inbox()) != 0 {
		t.Errorf("a pinned review stays in the lane (D51) and skips the Inbox: lane %v", ids(snap.GitHub.Reviews))
	}
	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if snap := mustLoad(t, s); len(snap.Snapshot.Tasks()) != 0 || len(snap.GitHub.Reviews) != 1 || len(snap.Plan.Candidates) != 0 {
		t.Errorf("undo deletes the task and the review is back in the lane: %d tasks, lane %v", len(snap.Snapshot.Tasks()), ids(snap.GitHub.Reviews))
	}

	if _, _, err := s.PinSource(ctx, "github", "acme/api#999"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a request no longer waiting: %v; want ErrNotFound", err)
	}
}

func TestPinSourceUsesTheConfiguredDefaults(t *testing.T) {
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "tt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg, err := config.Parse([]byte("[github]\nimpact = 2\ndue = \"0d\"\nsize = \"\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	src := &source.Fake{}
	s := New(st, &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}, cfg).WithSource(src)
	src.Set([]domain.SourceItem{review(901, "Add idempotency key", time.Time{})}, nil)
	mustSyncSources(t, s)
	task, _ := mustPinSource(t, s, "acme/api#901")
	if got := mustGet(t, s, task.ID); got.Impact != 2 || got.Due != "2026-10-12" || got.Size != domain.SizeNone {
		t.Errorf("task = impact %d, due %q, %q; want 2, due the day it was first seen (no request time), unsized", got.Impact, got.Due, got.Size)
	}

	cfg, _ = config.Parse([]byte("[github]\ndue = \"\"\n"))
	s = New(st, &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}, cfg).WithSource(src)
	src.Set([]domain.SourceItem{review(877, "Bump go", time.Time{})}, nil)
	mustSyncSources(t, s)
	task, _ = mustPinSource(t, s, "acme/api#877")
	if got := mustGet(t, s, task.ID); got.Due != "" {
		t.Errorf("due = \"\": task due %q; want none", got.Due)
	}
}

func TestPinSourceUndoRefusesAfterChanges(t *testing.T) {
	s, clock, src, _ := srcService(t)
	ctx := context.Background()
	src.Set([]domain.SourceItem{review(901, "Add idempotency key", clock.now), review(877, "Bump go", clock.now)}, nil)
	mustSyncSources(t, s)

	blocked, undo := mustPinSource(t, s, "acme/api#901")
	if _, _, err := s.Block(ctx, blocked.ID, 10*time.Hour, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := undo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo after a block: %v; want ErrConflict (the block must not go silently)", err)
	}
	if !pinned(t, s, blocked.ID) || len(mustPlan(t, s).Blocks) != 1 {
		t.Error("the refused undo left the task and its block alone")
	}

	edited, undo := mustPinSource(t, s, "acme/api#877")
	clock.Advance(time.Minute)
	if _, err := s.SetSize(ctx, edited.ID, domain.SizeM); err != nil {
		t.Fatal(err)
	}
	if err := undo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo after an edit: %v; want ErrConflict", err)
	}

	_, undo = mustPinSource(t, s, "acme/api#877") // already open and pinned: nothing to undo
	if undo != nil {
		t.Error("pinning a pinned review returns no undo")
	}
}

func TestPinSourceReopensItsTask(t *testing.T) {
	s, clock, src, _ := srcService(t)
	ctx := context.Background()
	a := review(901, "Add idempotency key", clock.now)
	src.Set([]domain.SourceItem{a}, nil)
	mustSyncSources(t, s)
	first, _ := mustPinSource(t, s, "acme/api#901")
	if _, err := s.SetSize(ctx, first.ID, domain.SizeM); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Block(ctx, first.ID, 10*time.Hour, time.Hour); err != nil {
		t.Fatal(err)
	}

	// Removed as a reviewer: auto-closed, still pinned and blocked today.
	clock.Advance(5 * time.Minute)
	src.Set(nil, nil)
	mustSyncSources(t, s)
	closedAt := clock.now
	// Re-requested: back in the lane (D20a), its task stays closed.
	clock.Advance(5 * time.Minute)
	a.RequestedAt = clock.now
	src.Set([]domain.SourceItem{a}, nil)
	mustSyncSources(t, s)
	if got := ids(mustLoad(t, s).GitHub.Reviews); len(got) != 1 {
		t.Fatalf("lane = %v; want the re-requested review", got)
	}
	if got := mustGet(t, s, first.ID); got.Status != domain.StatusDone {
		t.Fatalf("no auto-reopen: %v", got.Status)
	}

	again, undo := mustPinSource(t, s, "acme/api#901")
	got := mustGet(t, s, again.ID)
	if again.ID != first.ID || got.Status != domain.StatusOpen || got.ClosedBy != "" || !got.ClosedAt.IsZero() || got.Size != domain.SizeM {
		t.Errorf("reopened = %+v; want the same task, open, keeping its own size", got)
	}
	if !pinned(t, s, first.ID) || len(mustLoad(t, s).GitHub.Reviews) != 1 {
		t.Error("the reopened review is a candidate, and still in the lane (D51)")
	}
	if err := undo(ctx); err != nil {
		t.Fatalf("undo of the reopen (the block was there before): %v", err)
	}
	got = mustGet(t, s, first.ID)
	if got.Status != domain.StatusDone || got.ClosedBy != domain.ClosedBySource || !got.ClosedAt.Equal(closedAt) {
		t.Errorf("after the undo: %+v; want closed by the source again, as it was", got)
	}
	if !pinned(t, s, first.ID) || len(mustPlan(t, s).Blocks) != 1 {
		t.Error("the undo keeps the pin and block it found")
	}

	// Closed by me and still requested: p reopens it too.
	if _, err := s.Reopen(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Drop(ctx, first.ID, "not mine"); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustLoad(t, s).GitHub.Reviews); len(got) != 1 {
		t.Fatalf("a dropped review that still requests me is in the lane: %v", got)
	}
	if again, _ := mustPinSource(t, s, "acme/api#901"); again.ID != first.ID || mustGet(t, s, first.ID).DropReason != "" {
		t.Errorf("reopened after a drop: %+v", mustGet(t, s, first.ID))
	}
}

// A reopened review gets its due counted from the new request, and an impact
// when it has none (a review closed before migration 006).
func TestPinSourceReopenRecountsDueAndImpact(t *testing.T) {
	s, clock, src, _ := srcService(t)
	ctx := context.Background()
	a := review(901, "Add idempotency key", clock.now)
	src.Set([]domain.SourceItem{a}, nil)
	mustSyncSources(t, s)
	first, _ := mustPinSource(t, s, "acme/api#901")
	oldDue := mustGet(t, s, first.ID).Due
	if _, err := s.SetImpact(ctx, first.ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Done(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	before := mustGet(t, s, first.ID)

	clock.Advance(48 * time.Hour)
	a.RequestedAt = clock.now
	src.Set([]domain.SourceItem{a}, nil)
	mustSyncSources(t, s)
	again, undo := mustPinSource(t, s, "acme/api#901")
	got := mustGet(t, s, again.ID)
	wantDue := s.DayOf(clock.now).AddDays(3)
	if got.Status != domain.StatusOpen || got.Due != wantDue || got.Due == oldDue || got.Impact != 1 {
		t.Errorf("reopened = status %v due %q impact %d; want open, due %q (new request + 3d), impact 1", got.Status, got.Due, got.Impact, wantDue)
	}
	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	got = mustGet(t, s, first.ID)
	if got.Status != domain.StatusDone || got.Due != before.Due || got.Impact != before.Impact {
		t.Errorf("after the undo: %+v; want the closed task with due %q and impact %d", got, before.Due, before.Impact)
	}
}
