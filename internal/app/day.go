package app

import (
	"cmp"
	"context"
	"math"
	"slices"
	"time"

	"tt/internal/domain"
	"tt/internal/store"
)

// Recap summarizes the previous planning day for the new-day banner.
type Recap struct {
	Since      domain.Day // the previous planning day
	Done       int        // tasks marked done since Since began
	RolledOver int        // time-blocked tasks left unfinished (rollover +1 each)
	Carried    int        // open candidates carried into the new day
}

// DayResult is what StartDay did.
type DayResult struct {
	Day   domain.Day
	New   bool   // false: the day had already started; nothing changed
	Recap *Recap // nil on the very first planning day
}

// StartDay begins the current planning day, once (D9, D10, D10b, D19). For the
// previous planning day it increments the rollover counter of every open task
// that had a block (re-pinning it, pinned-days reset) and unpins closed tasks.
// Candidates stay pinned. It runs in one transaction guarded by the days
// primary key, so a TUI and a CLI racing at the boundary count once.
func (s *Service) StartDay(ctx context.Context) (DayResult, error) {
	now := s.clock.Now()
	today := domain.PlanningDay(now, s.cfg.DayStart)
	res := DayResult{Day: today}
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		if started, err := x.HasDay(ctx, today); err != nil || started {
			return err
		}
		res.New = true
		prev, ok, err := x.LatestDayBefore(ctx, today)
		if err != nil {
			return err
		}
		if ok {
			recap := Recap{Since: prev}
			ids, err := x.OpenTasksBlockedOn(ctx, prev)
			if err != nil {
				return err
			}
			for _, id := range ids {
				t, err := x.GetTask(ctx, id)
				if err != nil {
					return err
				}
				t.RolloverCount++
				t.UpdatedAt = now
				if err := x.UpdateTask(ctx, t); err != nil {
					return err
				}
				if err := x.PutPin(ctx, domain.Pin{TaskID: id, PinnedOn: today}); err != nil {
					return err
				}
			}
			recap.RolledOver = len(ids)
			loc := now.Location()
			if recap.Done, err = x.CountDone(ctx, prev.At(s.cfg.DayStart, loc), today.At(s.cfg.DayStart, loc)); err != nil {
				return err
			}
			res.Recap = &recap
		}
		if err := x.TouchClosedPinned(ctx, now); err != nil {
			return err
		}
		if err := x.DeletePinsOfClosedTasks(ctx, today); err != nil {
			return err
		}
		if res.Recap != nil {
			if res.Recap.Carried, err = x.CountOpenPins(ctx); err != nil {
				return err
			}
		}
		return x.InsertDay(ctx, today, now)
	})
	if err != nil {
		return DayResult{}, err
	}
	return res, nil
}

// CommitSummary is the finish-line summary of a committed plan (D22).
type CommitSummary struct {
	Blocks     int
	Blocked    time.Duration // total time in blocks
	Candidates int           // open candidates without a block
}

// CommitPlan marks today's plan as committed and summarizes it. It starts the
// day first if needed. Committing again just refreshes the time.
func (s *Service) CommitPlan(ctx context.Context) (CommitSummary, error) {
	res, err := s.StartDay(ctx)
	if err != nil {
		return CommitSummary{}, err
	}
	err = s.store.InTx(ctx, func(x *store.Tx) error { return x.CommitDay(ctx, res.Day, s.clock.Now()) })
	if err != nil {
		return CommitSummary{}, err
	}
	plan, err := s.Plan(ctx)
	if err != nil {
		return CommitSummary{}, err
	}
	sum := CommitSummary{Blocks: len(plan.Blocks)}
	for _, b := range plan.Blocks {
		sum.Blocked += b.Duration
	}
	for _, c := range plan.Candidates {
		if c.Task.Status == domain.StatusOpen && !c.Blocked {
			sum.Candidates++
		}
	}
	return sum, nil
}

// Plan returns today's candidates and blocks. Closed tasks stay among the
// candidates until the next planning day, so the day's wins remain visible.
func (s *Service) Plan(ctx context.Context) (domain.Plan, error) {
	st, err := s.Load(ctx)
	return st.Plan, err
}

// State is everything a view needs, read consistently.
type State struct {
	Version  int64 // DataVersion read before loading; compare to detect later writes
	Snapshot domain.Snapshot
	Plan     domain.Plan
	Calendar CalendarState
	GitHub   GitHubState
}

// Load reads the data version first, then all tasks and today's plan in one
// read, then the calendar's cache and GitHub's review requests. A write that
// lands during the load changes the version, so the next version check
// reloads again.
func (s *Service) Load(ctx context.Context) (State, error) {
	version, err := s.store.DataVersion(ctx)
	if err != nil {
		return State{}, err
	}
	today := s.Today()
	snap, state, err := s.store.LoadPlan(ctx, today)
	if err != nil {
		return State{}, err
	}
	cal, err := s.calendarState(ctx, today)
	if err != nil {
		return State{}, err
	}
	gh, err := s.githubState(ctx)
	if err != nil {
		return State{}, err
	}
	return State{Version: version, Snapshot: snap, Plan: s.buildPlan(today, snap, state), Calendar: cal, GitHub: gh}, nil
}

func (s *Service) buildPlan(today domain.Day, snap domain.Snapshot, state store.PlanState) domain.Plan {
	blocked := map[int64]bool{}
	for _, b := range state.Blocks {
		blocked[b.TaskID] = true
	}
	plan := domain.Plan{Day: today, Committed: state.Committed, Blocks: state.Blocks}
	for _, p := range state.Pins {
		t, ok := snap.Get(p.TaskID)
		if !ok {
			continue
		}
		plan.Candidates = append(plan.Candidates, domain.Candidate{
			Task:       t,
			PinnedDays: p.PinnedDays,
			Blocked:    blocked[t.ID],
			Alert:      t.Status == domain.StatusOpen && t.RolloverCount >= s.cfg.RolloverAlert,
		})
	}
	slices.SortFunc(plan.Candidates, func(a, b domain.Candidate) int {
		pa := domain.EffectivePriority(a.Task, snap.ParentOf(a.Task))
		pb := domain.EffectivePriority(b.Task, snap.ParentOf(b.Task))
		return cmp.Or(cmp.Compare(unsetLast(pa), unsetLast(pb)), cmp.Compare(a.Task.ID, b.Task.ID))
	})
	return plan
}

// unsetLast sorts priority 0 (unset) after every real level.
func unsetLast(p int) int {
	if p == 0 {
		return math.MaxInt
	}
	return p
}
