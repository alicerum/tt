package app

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"tt/internal/config"
	"tt/internal/store"
)

func mustStartDay(t *testing.T, s *Service) DayResult {
	t.Helper()
	res, err := s.StartDay(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestStartDayFirstRunAndIdempotence(t *testing.T) {
	s, clock := newService(t)
	if res := mustStartDay(t, s); !res.New || res.Day != "2026-10-12" || res.Recap != nil {
		t.Errorf("first StartDay = %+v; want new day, no recap", res)
	}
	if res := mustStartDay(t, s); res.New {
		t.Errorf("second StartDay = %+v; want New false", res)
	}
	clock.now = time.Date(2026, 10, 13, 3, 30, 0, 0, time.UTC) // before day start
	if res := mustStartDay(t, s); res.New {
		t.Errorf("StartDay at 03:30 next date = %+v; still the same planning day", res)
	}
}

func TestStartDayRollsOver(t *testing.T) {
	s, clock := startedService(t)
	ctx := context.Background()
	blocked := mustAdd(t, s, "Blocked, unfinished !1 ~l")
	candidateOnly := mustAdd(t, s, "Candidate only !2 ~s")
	finished := mustAdd(t, s, "Blocked, finished !1 ~s")
	dropped := mustAdd(t, s, "Pinned, dropped !3 ~s")
	mustBlock(t, s, blocked.ID, 10*h, 2*h)
	mustBlock(t, s, finished.ID, 13*h, h)
	for _, id := range []int64{candidateOnly.ID, dropped.ID} {
		if _, err := s.Pin(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	clock.Advance(6 * h) // 15:00
	if _, err := s.Done(ctx, finished.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Drop(ctx, dropped.ID, "not needed"); err != nil {
		t.Fatal(err)
	}
	if c, ok := candidate(t, mustPlan(t, s), finished.ID); !ok || c.Task.Status != "done" {
		t.Errorf("done task stays visible in today's plan until the next day: %+v, %v", c, ok)
	}

	clock.Advance(18 * h) // next day 09:00
	res := mustStartDay(t, s)
	want := &Recap{Since: "2026-10-12", Done: 1, RolledOver: 1, Carried: 2}
	if !res.New || res.Day != "2026-10-13" || !cmp.Equal(res.Recap, want) {
		t.Errorf("StartDay = %+v, recap %+v; want recap %+v", res, res.Recap, want)
	}

	if got := mustGet(t, s, blocked.ID).RolloverCount; got != 1 {
		t.Errorf("blocked task rollover = %d; want 1", got)
	}
	if got := mustGet(t, s, candidateOnly.ID).RolloverCount; got != 0 {
		t.Errorf("candidate rollover = %d; want 0 (candidates never count, D10)", got)
	}
	plan := mustPlan(t, s)
	if c, ok := candidate(t, plan, blocked.ID); !ok || c.PinnedDays != 1 || c.Blocked {
		t.Errorf("rolled-over task is carried as an unblocked candidate, pinned-days reset: %+v, %v", c, ok)
	}
	if c, ok := candidate(t, plan, candidateOnly.ID); !ok || c.PinnedDays != 2 {
		t.Errorf("candidate is carried with pinned days 2: %+v, %v", c, ok)
	}
	for _, id := range []int64{finished.ID, dropped.ID} {
		if _, ok := candidate(t, plan, id); ok {
			t.Errorf("closed task #%d should be unpinned on the new day", id)
		}
	}
	if len(plan.Blocks) != 0 {
		t.Errorf("a new day starts with an empty timeline: %+v", plan.Blocks)
	}
}

func TestStartDayCountsPlanningDaysNotCalendarDays(t *testing.T) {
	s, clock := startedService(t) // Monday 2026-10-12
	ctx := context.Background()
	blocked := mustAdd(t, s, "Blocked !1 ~s")
	pinned := mustAdd(t, s, "Pinned !1 ~s")
	mustBlock(t, s, blocked.ID, 10*h, h)
	if _, err := s.Pin(ctx, pinned.ID); err != nil {
		t.Fatal(err)
	}

	clock.Advance(4 * 24 * h) // Friday; the app wasn't used Tue–Thu
	res := mustStartDay(t, s)
	if res.Recap == nil || res.Recap.Since != "2026-10-12" {
		t.Errorf("recap = %+v; want since Monday", res.Recap)
	}
	if got := mustGet(t, s, blocked.ID).RolloverCount; got != 1 {
		t.Errorf("rollover after a 4-day gap = %d; want 1 (D10b)", got)
	}
	if c, _ := candidate(t, mustPlan(t, s), pinned.ID); c.PinnedDays != 2 {
		t.Errorf("pinned days after a 4-day gap = %d; want 2", c.PinnedDays)
	}
}

func TestStartDayRaceCountsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tt.db")
	clock := &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
	open := func() *Service {
		st, err := store.Open(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { st.Close() })
		return New(st, clock, config.Default())
	}
	tui, cli := open(), open()
	mustStartDay(t, tui)
	task := mustAdd(t, tui, "Blocked !1 ~s")
	mustBlock(t, tui, task.ID, 10*h, h)
	clock.Advance(24 * h)

	var wg sync.WaitGroup
	results := make([]DayResult, 2)
	for i, s := range []*Service{tui, cli} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var err error
			if results[i], err = s.StartDay(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if results[0].New == results[1].New {
		t.Errorf("exactly one racer should start the day: %+v", results)
	}
	if got := mustGet(t, tui, task.ID).RolloverCount; got != 1 {
		t.Errorf("rollover after racing StartDay = %d; want 1", got)
	}
}

func TestUndoAfterRolloverConflicts(t *testing.T) {
	s, clock := startedService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Edit me !1 ~s")
	mustBlock(t, s, task.ID, 10*h, h)
	clock.Advance(time.Hour)
	editUndo, err := s.Edit(ctx, task.ID, "Edited !1 ~s")
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(24 * h)
	mustStartDay(t, s) // rollover writes the task

	if err := editUndo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo across a rollover: %v; want ErrConflict", err)
	}
	if got := mustGet(t, s, task.ID); got.RolloverCount != 1 || got.Title != "Edited" {
		t.Errorf("the rollover must survive: %+v", got)
	}
}

func TestCommitPlan(t *testing.T) {
	s, _ := newService(t) // day not started yet: CommitPlan starts it
	ctx := context.Background()
	a := mustAdd(t, s, "A !1 ~s")
	b := mustAdd(t, s, "B !2 ~s")
	c := mustAdd(t, s, "C !3 ~s")
	mustBlock(t, s, a.ID, 10*h, 90*m)
	mustBlock(t, s, a.ID, 14*h, 30*m)
	for _, id := range []int64{b.ID, c.ID} {
		if _, err := s.Pin(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Done(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if mustPlan(t, s).Committed {
		t.Error("plan committed before CommitPlan")
	}
	sum, err := s.CommitPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := CommitSummary{Blocks: 2, Blocked: 2 * h, Candidates: 1}
	if sum != want {
		t.Errorf("summary = %+v; want %+v", sum, want)
	}
	if !mustPlan(t, s).Committed {
		t.Error("plan not committed after CommitPlan")
	}
	if _, err := s.CommitPlan(ctx); err != nil {
		t.Errorf("committing again: %v", err)
	}
}

func TestPlanOrderAndAlert(t *testing.T) {
	s, clock := startedService(t)
	s.cfg.RolloverAlert = 1
	ctx := context.Background()
	unset := mustAdd(t, s, "No priority")
	p2 := mustAdd(t, s, "Second !2 ~s")
	p1 := mustAdd(t, s, "First !1 ~s")
	parent := mustAdd(t, s, "Parent !1 ~xl")
	child := mustAdd(t, s, "Child ~s")
	if _, err := s.SetParent(ctx, child.ID, parent.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{unset.ID, p2.ID, child.ID} {
		if _, err := s.Pin(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	mustBlock(t, s, p1.ID, 10*h, h)
	clock.Advance(24 * h)
	mustStartDay(t, s) // p1 rolls over once → alert at threshold 1

	var ids []int64
	var alerts []bool
	for _, c := range mustPlan(t, s).Candidates {
		ids = append(ids, c.Task.ID)
		alerts = append(alerts, c.Alert)
	}
	// child inherits P1 from its parent, so it sorts with p1 (by id).
	if diff := cmp.Diff([]int64{p1.ID, child.ID, p2.ID, unset.ID}, ids); diff != "" {
		t.Errorf("candidate order (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]bool{true, false, false, false}, alerts); diff != "" {
		t.Errorf("alerts (-want +got):\n%s", diff)
	}
}

func TestUndoDoneAfterNewDayConflicts(t *testing.T) {
	s, clock := startedService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Finish me !1 ~s")
	mustBlock(t, s, task.ID, 10*h, h)
	undo := mustUndo(t)(s.Done(ctx, task.ID))
	clock.Advance(24 * h)
	mustStartDay(t, s)
	if err := undo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo Done after a new day: %v; want ErrConflict", err)
	}
	if got := mustGet(t, s, task.ID); got.Status != "done" {
		t.Errorf("task must stay done: %+v", got)
	}
}

func TestStartDayKeepsBlockPlacedBeforeItRan(t *testing.T) {
	s, clock := startedService(t)
	s.cfg.WorkStart = s.cfg.DayStart // the 05:00 block below is inside working hours (D39)
	ctx := context.Background()
	task := mustAdd(t, s, "Early bird !1 ~s")
	mustBlock(t, s, task.ID, 10*h, h)
	clock.now = time.Date(2026, 10, 13, 4, 10, 0, 0, time.UTC) // new planning day, StartDay not run
	mustBlock(t, s, task.ID, 5*h, h)
	if _, err := s.Done(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	mustStartDay(t, s)
	plan := mustPlan(t, s)
	if len(plan.Blocks) != 1 {
		t.Errorf("blocks = %+v; want the 05:00 block kept", plan.Blocks)
	}
	if _, ok := candidate(t, plan, task.ID); !ok {
		t.Error("task with a block today must stay a candidate")
	}
}

func amsterdamService(t *testing.T, now func(*time.Location) time.Time) (*Service, *fakeClock) {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	s, clock := newService(t)
	clock.now = now(loc)
	return s, clock
}

func TestDayOfUsesClockLocation(t *testing.T) {
	s, _ := amsterdamService(t, func(l *time.Location) time.Time { return time.Date(2026, 10, 12, 9, 0, 0, 0, l) })
	if got := s.DayOf(time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC)); got != "2026-10-12" {
		t.Errorf("DayOf(05:00 local) = %q; want 2026-10-12", got)
	}
	if got := s.DayOf(time.Date(2026, 10, 12, 1, 30, 0, 0, time.UTC)); got != "2026-10-11" {
		t.Errorf("DayOf(03:30 local) = %q; want 2026-10-11", got)
	}
}

func TestStartDayAcrossDSTChange(t *testing.T) {
	s, clock := amsterdamService(t, func(l *time.Location) time.Time { return time.Date(2026, 10, 24, 9, 0, 0, 0, l) })
	ctx := context.Background()
	mustStartDay(t, s)
	blocked := mustAdd(t, s, "Blocked !1 ~s")
	finished := mustAdd(t, s, "Finished !1 ~s")
	mustBlock(t, s, blocked.ID, 10*h, h)
	clock.now = time.Date(2026, 10, 24, 15, 0, 0, 0, clock.now.Location())
	if _, err := s.Done(ctx, finished.ID); err != nil {
		t.Fatal(err)
	}
	clock.now = time.Date(2026, 10, 25, 9, 0, 0, 0, clock.now.Location()) // fall-back day
	res := mustStartDay(t, s)
	want := &Recap{Since: "2026-10-24", Done: 1, RolledOver: 1, Carried: 1}
	if !res.New || !cmp.Equal(res.Recap, want) {
		t.Errorf("StartDay = %+v, recap %+v; want recap %+v", res, res.Recap, want)
	}
}

func TestTwoBlocksRollOverOnce(t *testing.T) {
	s, clock := startedService(t)
	task := mustAdd(t, s, "Twice blocked !1 ~s")
	mustBlock(t, s, task.ID, 10*h, h)
	mustBlock(t, s, task.ID, 14*h, h)
	clock.Advance(24 * h)
	mustStartDay(t, s)
	if got := mustGet(t, s, task.ID).RolloverCount; got != 1 {
		t.Errorf("rollover with two blocks = %d; want 1", got)
	}
}

func TestPlanAlertOnlyForOpenTasks(t *testing.T) {
	s, clock := startedService(t)
	s.cfg.RolloverAlert = 1
	task := mustAdd(t, s, "Rolled then done !1 ~s")
	mustBlock(t, s, task.ID, 10*h, h)
	clock.Advance(24 * h)
	mustStartDay(t, s)
	if c, ok := candidate(t, mustPlan(t, s), task.ID); !ok || !c.Alert {
		t.Fatalf("open rolled-over task should alert: %+v, %v", c, ok)
	}
	if _, err := s.Done(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}
	if c, ok := candidate(t, mustPlan(t, s), task.ID); !ok || c.Alert {
		t.Errorf("done task must not alert: %+v, %v", c, ok)
	}
}
