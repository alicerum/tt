package app

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"tt/internal/domain"
)

const (
	h = time.Hour
	m = time.Minute
)

// startedService is newService with today's planning day already started.
func startedService(t *testing.T) (*Service, *fakeClock) {
	t.Helper()
	s, clock := newService(t)
	if _, err := s.StartDay(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s, clock
}

func mustPlan(t *testing.T, s *Service) domain.Plan {
	t.Helper()
	plan, err := s.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func candidate(t *testing.T, plan domain.Plan, id int64) (domain.Candidate, bool) {
	t.Helper()
	for _, c := range plan.Candidates {
		if c.Task.ID == id {
			return c, true
		}
	}
	return domain.Candidate{}, false
}

func mustBlock(t *testing.T, s *Service, id int64, start, dur time.Duration) (domain.Block, Undo) {
	t.Helper()
	b, undo, err := s.Block(context.Background(), id, start, dur)
	if err != nil {
		t.Fatalf("Block(#%d, %v, %v): %v", id, start, dur, err)
	}
	return b, undo
}

func TestTodayUsesDayStart(t *testing.T) {
	s, clock := newService(t)
	if got := s.Today(); got != "2026-10-12" {
		t.Errorf("Today at 09:00 = %s", got)
	}
	clock.now = time.Date(2026, 10, 13, 3, 0, 0, 0, time.UTC)
	if got := s.Today(); got != "2026-10-12" {
		t.Errorf("Today at 03:00 next day = %s; want still 2026-10-12", got)
	}
}

func TestPinAndUnpin(t *testing.T) {
	s, _ := startedService(t)
	must := mustUndo(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Pin me !1 ~s")

	pinUndo := must(s.Pin(ctx, task.ID))
	c, ok := candidate(t, mustPlan(t, s), task.ID)
	if !ok || c.PinnedDays != 1 || c.Blocked {
		t.Fatalf("after Pin: %+v, %v", c, ok)
	}
	again := must(s.Pin(ctx, task.ID)) // already pinned: no change
	if err := again(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := candidate(t, mustPlan(t, s), task.ID); !ok {
		t.Error("undoing a no-op Pin must not unpin")
	}
	if err := pinUndo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := candidate(t, mustPlan(t, s), task.ID); ok {
		t.Error("undo of Pin should unpin")
	}

	must(s.Pin(ctx, task.ID))
	b, _ := mustBlock(t, s, task.ID, 10*h, h)
	unpinUndo := must(s.Unpin(ctx, task.ID))
	plan := mustPlan(t, s)
	if _, ok := candidate(t, plan, task.ID); ok || len(plan.Blocks) != 0 {
		t.Errorf("Unpin should remove the task and today's blocks: %+v", plan)
	}
	if err := unpinUndo(ctx); err != nil {
		t.Fatal(err)
	}
	plan = mustPlan(t, s)
	if _, ok := candidate(t, plan, task.ID); !ok || len(plan.Blocks) != 1 || plan.Blocks[0] != b {
		t.Errorf("undo of Unpin should restore pin and block %+v: %+v", b, plan)
	}

	must(s.Unpin(ctx, 999)) // unpinning an unpinned or missing task is a no-op

	if _, err := s.Done(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	other := mustAdd(t, s, "Closed")
	if _, err := s.Drop(ctx, other.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pin(ctx, other.ID); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("Pin on a dropped task: %v; want ErrInvalidValue", err)
	}
	if _, err := s.Pin(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("Pin on a missing task: %v; want ErrNotFound", err)
	}
}

func TestBlock(t *testing.T) {
	s, _ := startedService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Block me !1 ~m")

	b, undo := mustBlock(t, s, task.ID, 11*h, 90*m)
	if b.ID == 0 || b.Day != "2026-10-12" || b.Start != 11*h || b.Duration != 90*m {
		t.Errorf("block = %+v", b)
	}
	c, ok := candidate(t, mustPlan(t, s), task.ID)
	if !ok || !c.Blocked {
		t.Errorf("blocking pins the task and marks it blocked: %+v, %v", c, ok)
	}
	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if plan := mustPlan(t, s); len(plan.Blocks) != 0 || len(plan.Candidates) != 0 {
		t.Errorf("undo of Block on an unpinned task should leave it unpinned: %+v", plan)
	}

	mustBlock(t, s, task.ID, 11*h, h)
	mustBlock(t, s, task.ID, 14*h, h) // a task may have several blocks
	mustBlock(t, s, task.ID, 25*h, h) // 01:00 after midnight is still today (day start 04:00)

	other := mustAdd(t, s, "Other")
	bad := []struct {
		name       string
		start, dur time.Duration
		want       error
	}{
		{"overlap", 11*h + 30*m, h, ErrOverlap},
		{"overlap containing", 10 * h, 6 * h, ErrOverlap},
		{"before day start", 3 * h, h, ErrInvalidValue},
		{"past next day start", 27*h + 30*m, h, ErrInvalidValue},
		{"zero length", 9 * h, 0, ErrInvalidValue},
		{"seconds", 9*h + 30*time.Second, h, ErrInvalidValue},
		{"longer than a day", 5 * h, 25 * h, ErrInvalidValue},
		{"overflowing length", 5 * h, time.Duration(math.MaxInt64) / time.Minute * time.Minute, ErrInvalidValue},
		{"start far past the day", time.Duration(math.MaxInt64)/time.Minute*time.Minute - h, h, ErrInvalidValue},
	}
	for _, c := range bad {
		if _, _, err := s.Block(ctx, other.ID, c.start, c.dur); !errors.Is(err, c.want) {
			t.Errorf("%s: error = %v; want %v", c.name, err, c.want)
		}
	}
	if _, _, err := s.Block(ctx, other.ID, 12*h, h); err != nil {
		t.Errorf("touching blocks are fine: %v", err)
	}
	if _, _, err := s.Block(ctx, other.ID, 4*h, 24*h-5*h); !errors.Is(err, ErrOverlap) {
		t.Errorf("long block over existing ones: %v; want ErrOverlap", err)
	}

	if _, err := s.Done(ctx, other.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Block(ctx, other.ID, 16*h, h); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("Block on a done task: %v; want ErrInvalidValue", err)
	}
}

func TestBlockResetsPinnedDaysAndUndoRestoresPin(t *testing.T) {
	s, clock := startedService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Long-pinned !2 ~s")
	if _, err := s.Pin(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	clock.Advance(24 * h)
	if _, err := s.StartDay(ctx); err != nil {
		t.Fatal(err)
	}
	if c, _ := candidate(t, mustPlan(t, s), task.ID); c.PinnedDays != 2 {
		t.Fatalf("pinned days after two planning days = %d; want 2", c.PinnedDays)
	}

	_, undo := mustBlock(t, s, task.ID, 10*h, h)
	if c, _ := candidate(t, mustPlan(t, s), task.ID); c.PinnedDays != 1 {
		t.Errorf("pinned days after Block = %d; want 1 (reset, D10d)", c.PinnedDays)
	}
	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if c, ok := candidate(t, mustPlan(t, s), task.ID); !ok || c.PinnedDays != 2 {
		t.Errorf("undo of Block should restore the old pin: %+v, %v", c, ok)
	}
}

func TestMoveResizeUnblock(t *testing.T) {
	s, clock := startedService(t)
	must := mustUndo(t)
	ctx := context.Background()
	a := mustAdd(t, s, "A !1 ~s")
	bTask := mustAdd(t, s, "B !1 ~s")
	ba, _ := mustBlock(t, s, a.ID, 10*h, h)
	bb, _ := mustBlock(t, s, bTask.ID, 12*h, h)

	if _, err := s.MoveBlock(ctx, ba.ID, 11*h+30*m); !errors.Is(err, ErrOverlap) {
		t.Errorf("move into another block: %v; want ErrOverlap", err)
	}
	if _, err := s.MoveBlock(ctx, ba.ID, 10*h+30*m); err != nil {
		t.Errorf("move overlapping only itself: %v", err)
	}
	moveUndo := must(s.MoveBlock(ctx, ba.ID, 15*h))
	resizeUndo := must(s.ResizeBlock(ctx, ba.ID, 2*h))
	if got := mustPlan(t, s).Blocks; got[1].Start != 15*h || got[1].Duration != 2*h {
		t.Errorf("after move+resize: %+v", got)
	}
	if _, err := s.ResizeBlock(ctx, bb.ID, 4*h); !errors.Is(err, ErrOverlap) {
		t.Errorf("resize into another block: %v; want ErrOverlap", err)
	}
	if _, err := s.ResizeBlock(ctx, bb.ID, 0); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("resize to zero: %v; want ErrInvalidValue", err)
	}
	if err := resizeUndo(ctx); err != nil {
		t.Fatal(err)
	}
	if err := moveUndo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mustPlan(t, s).Blocks[0]; got.Start != 10*h+30*m || got.Duration != h {
		t.Errorf("after undoing resize and move: %+v", got)
	}

	unblockUndo := must(s.Unblock(ctx, bb.ID))
	plan := mustPlan(t, s)
	if c, ok := candidate(t, plan, bTask.ID); !ok || c.Blocked || len(plan.Blocks) != 1 {
		t.Errorf("Unblock keeps the task as an unblocked candidate: %+v", plan)
	}
	if err := unblockUndo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mustPlan(t, s).Blocks; len(got) != 2 || got[1] != bb {
		t.Errorf("undo of Unblock should restore block %+v: %+v", bb, got)
	}

	if _, err := s.MoveBlock(ctx, 999, 9*h); !errors.Is(err, ErrNotFound) {
		t.Errorf("move missing block: %v; want ErrNotFound", err)
	}
	clock.Advance(24 * h)
	if _, err := s.MoveBlock(ctx, bb.ID, 9*h); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("move yesterday's block: %v; want ErrInvalidValue", err)
	}
	if _, err := s.Unblock(ctx, bb.ID); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("unblock yesterday's block: %v; want ErrInvalidValue", err)
	}
}

func TestServiceDefaultBlockLength(t *testing.T) {
	s, _ := newService(t)
	if got := s.DefaultBlockLength(domain.Task{Size: domain.SizeS}, 3*h); got != h {
		t.Errorf("S in 3h gap = %v; want 1h", got)
	}
	if got := s.DefaultBlockLength(domain.Task{}, 0); got != 30*m {
		t.Errorf("unsized, no cap = %v; want 30m", got)
	}
}

func TestPlanningUndoRefusesAcrossDayBoundary(t *testing.T) {
	s, clock := startedService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "X !1 ~s")
	_, undo := mustBlock(t, s, task.ID, 10*h, h)
	clock.Advance(24 * h)
	if _, err := s.StartDay(ctx); err != nil {
		t.Fatal(err)
	}

	if err := undo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo across the day boundary: %v; want ErrConflict", err)
	}
	if _, ok := candidate(t, mustPlan(t, s), task.ID); !ok {
		t.Error("the rollover's pin must survive a refused undo")
	}
	if got := mustGet(t, s, task.ID).RolloverCount; got != 1 {
		t.Errorf("rollover = %d; want 1", got)
	}
}

func TestUndoUnblockRefusesOverlap(t *testing.T) {
	s, _ := startedService(t)
	ctx := context.Background()
	a := mustAdd(t, s, "A !1 ~s")
	b := mustAdd(t, s, "B !1 ~s")
	ba, _ := mustBlock(t, s, a.ID, 12*h, h)
	undo, err := s.Unblock(ctx, ba.ID)
	if err != nil {
		t.Fatal(err)
	}
	mustBlock(t, s, b.ID, 12*h, h)

	if err := undo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo of Unblock into a taken slot: %v; want ErrConflict", err)
	}
	plan := mustPlan(t, s)
	if len(plan.Blocks) != 1 || plan.Blocks[0].TaskID != b.ID {
		t.Errorf("blocks after refused undo = %+v; want only B's block", plan.Blocks)
	}
}

func TestPinUndoRefusesWhenBlockedSince(t *testing.T) {
	s, _ := startedService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Pinned then blocked !1 ~s")
	pinUndo, err := s.Pin(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	mustBlock(t, s, task.ID, 10*h, h)

	if err := pinUndo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo of Pin with a block today: %v; want ErrConflict", err)
	}
	if c, ok := candidate(t, mustPlan(t, s), task.ID); !ok || !c.Blocked {
		t.Errorf("task should stay a blocked candidate: %+v, %v", c, ok)
	}
}

func TestBlockUndoKeepsPinWhileOtherBlocksRemain(t *testing.T) {
	s, _ := startedService(t)
	ctx := context.Background()
	a := mustAdd(t, s, "A !1 ~s")
	_, first := mustBlock(t, s, a.ID, 10*h, h)
	second, _ := mustBlock(t, s, a.ID, 14*h, h)

	if err := first(ctx); err != nil {
		t.Fatal(err)
	}
	plan := mustPlan(t, s)
	if len(plan.Blocks) != 1 || plan.Blocks[0] != second {
		t.Errorf("blocks after undoing the first = %+v; want only %+v", plan.Blocks, second)
	}
	if c, ok := candidate(t, plan, a.ID); !ok || !c.Blocked {
		t.Errorf("task must stay a blocked candidate while another block remains: %+v, %v", c, ok)
	}
}

func TestRestoredBlockKeepsItsIDWithoutCollision(t *testing.T) {
	s, _ := startedService(t)
	ctx := context.Background()
	a := mustAdd(t, s, "A !1 ~s")
	b := mustAdd(t, s, "B !1 ~s")
	ba, _ := mustBlock(t, s, a.ID, 12*h, h)
	undo, err := s.Unblock(ctx, ba.ID)
	if err != nil {
		t.Fatal(err)
	}
	bb, _ := mustBlock(t, s, b.ID, 15*h, h)
	if bb.ID == ba.ID {
		t.Fatalf("new block reused the deleted id %d; test needs distinct ids", ba.ID)
	}

	if err := undo(ctx); err != nil {
		t.Fatalf("undo of Unblock with a reused-id neighbour: %v", err)
	}
	plan := mustPlan(t, s)
	if len(plan.Blocks) != 2 {
		t.Fatalf("blocks = %+v; want 2", plan.Blocks)
	}
	for _, blk := range plan.Blocks {
		if blk.Start == 12*h && blk.ID != ba.ID {
			t.Errorf("restored 12:00 block id = %d; want %d", blk.ID, ba.ID)
		}
	}
}
