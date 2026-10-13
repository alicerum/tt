package app

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"tt/internal/domain"
	"tt/internal/store"
)

// Source is where review requests come from (D20, D50); the gh adapter in
// internal/source implements it.
type Source interface {
	Kind() string                                           // its integration_state row and the tasks' source kind
	Fetch(ctx context.Context) ([]domain.SourceItem, error) // everything awaiting me now
}

// WithSource sets the source SyncSources reads. Without one there is no
// Reviews lane (D50). Call it before the service is used.
func (s *Service) WithSource(src Source) *Service {
	s.src = src
	return s
}

// SourceSetUp reports whether GitHub is set up (D50).
func (s *Service) SourceSetUp() bool { return s.src != nil }

// GitHubState is what the views show of GitHub (D49, D50).
type GitHubState struct {
	SetUp bool
	// Reviews is the Reviews lane: every request awaiting me, with or
	// without a task (D51), longest waiting first.
	Reviews     []domain.SourceItem
	LastSuccess time.Time // zero: never fetched
	Err         string    // why the last fetch failed; "" once one succeeds
}

// SyncSources fetches what is awaiting me. Only on success it reconciles, in
// one transaction (D50): the fetched set replaces the stored one (keeping
// when each item was first seen), and every open task whose item is gone is
// closed by the source. It returns those tasks. A failed fetch isn't
// returned: it is recorded, and the views show it from the state; only a
// failure to record the fetch is.
func (s *Service) SyncSources(ctx context.Context) ([]domain.Task, error) {
	if s.src == nil {
		return nil, nil
	}
	items, fetchErr := s.src.Fetch(ctx)
	if ctx.Err() != nil {
		return nil, ctx.Err() // tt is quitting; that isn't GitHub's fault
	}
	kind, now := s.src.Kind(), s.clock.Now()
	var closed []domain.Task
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		in, _, err := x.GetIntegration(ctx, kind)
		if err != nil {
			return err
		}
		if fetchErr != nil {
			in.LastError, in.LastErrorAt = fetchErr.Error(), now
			return x.PutIntegration(ctx, in)
		}
		old, err := x.SourceItems(ctx, kind)
		if err != nil {
			return err
		}
		firstSeen := map[string]time.Time{}
		for _, i := range old {
			firstSeen[i.ExternalID] = i.FirstSeenAt
		}
		live := map[string]bool{}
		var set []domain.SourceItem
		for _, i := range items {
			// Text from outside is stored as one clean line, so no view or
			// task title built from it can carry a control character.
			i.ExternalID, i.Title, i.URL = domain.OneLine(i.ExternalID), domain.OneLine(i.Title), domain.OneLine(i.URL)
			i.Author, i.Repo = domain.OneLine(i.Author), domain.OneLine(i.Repo)
			if live[i.ExternalID] {
				continue
			}
			live[i.ExternalID] = true
			i.Kind, i.FirstSeenAt = kind, now
			if t, ok := firstSeen[i.ExternalID]; ok {
				i.FirstSeenAt = t
			}
			set = append(set, i)
		}
		if err := x.ReplaceSourceItems(ctx, kind, set); err != nil {
			return err
		}
		open, err := x.OpenSourceTasks(ctx, kind)
		if err != nil {
			return err
		}
		for _, t := range open {
			if live[t.Source.ID] {
				continue
			}
			// D20a: done by the source. UpdatedAt moves, so an older undo of
			// this task refuses instead of reopening it.
			t.Status, t.ClosedAt, t.ClosedBy, t.DropReason, t.UpdatedAt = domain.StatusDone, now, domain.ClosedBySource, "", now
			if err := x.UpdateTask(ctx, t); err != nil {
				return err
			}
			closed = append(closed, t)
		}
		in.LastSuccessAt, in.LastError, in.LastErrorAt = now, "", time.Time{}
		return x.PutIntegration(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return closed, nil
}

// PinSource pins the review request kind/extID to today (D20, D49). It
// creates the request's task (D20b, D48: "Review #n: <title>", the PR link,
// the configured priority and size, no requester) or, when that task exists
// but is closed, reopens it, keeping its history. The undo deletes the task
// it created, or closes the reopened one again as it was; it refuses
// (ErrConflict) once the task changed, was blocked today, or the day moved on.
func (s *Service) PinSource(ctx context.Context, kind, extID string) (domain.Task, Undo, error) {
	today, now := s.Today(), s.clock.Now()
	var (
		before, after domain.Task // before is set when a closed task was reopened
		created, had  bool
	)
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		items, err := x.SourceItems(ctx, kind)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(items, func(i domain.SourceItem) bool { return i.ExternalID == extID })
		if i < 0 {
			return fmt.Errorf("review request %s isn't waiting any more: %w", extID, ErrNotFound)
		}
		item := items[i]
		t, ok, err := x.TaskBySource(ctx, item.Ref())
		switch {
		case err != nil:
			return err
		case !ok:
			ref := item.Ref()
			after = domain.Task{Title: domain.ReviewTitle(item), Link: item.URL, Source: &ref,
				Priority: min(s.cfg.ReviewPriority, len(s.cfg.Priorities)), Size: s.cfg.ReviewSize,
				Status: domain.StatusOpen, CreatedAt: now, UpdatedAt: now}
			if after.ID, err = x.InsertTask(ctx, after); err != nil {
				return err
			}
			created = true
		case t.Status != domain.StatusOpen:
			before, after = t, t
			after.Status, after.ClosedAt, after.ClosedBy, after.DropReason, after.UpdatedAt = domain.StatusOpen, time.Time{}, "", "", now
			if err := x.UpdateTask(ctx, after); err != nil {
				return err
			}
		default:
			after = t
		}
		if _, had, err = x.GetPin(ctx, after.ID); err != nil || had {
			return err
		}
		return x.PutPin(ctx, domain.Pin{TaskID: after.ID, PinnedOn: today})
	})
	if err != nil {
		return domain.Task{}, nil, err
	}
	if !created && before.ID == 0 && had {
		return after, nil, nil // open and pinned already: nothing changed
	}
	return after, func(ctx context.Context) error {
		if err := s.checkDay(today); err != nil {
			return err
		}
		return s.store.InTx(ctx, func(x *store.Tx) error {
			if err := unchangedSince(ctx, x, after); err != nil {
				return err
			}
			if !had { // removing the pin (or the task) would take a block placed since
				blocks, err := x.TaskBlocksOn(ctx, after.ID, today)
				if err != nil {
					return err
				}
				if len(blocks) > 0 {
					return fmt.Errorf("task #%d is blocked today: %w", after.ID, ErrConflict)
				}
			}
			if created {
				return x.DeleteTask(ctx, after.ID) // its pin goes with it
			}
			if before.ID != 0 {
				if err := x.UpdateTask(ctx, before); err != nil {
					return err
				}
			}
			if had {
				return nil
			}
			return x.DeletePin(ctx, after.ID)
		})
	}, nil
}

// githubState reads the last fetch and the lane: every stored item, whether
// it has a task or not (D51; the TUI marks the ones that have one).
func (s *Service) githubState(ctx context.Context) (GitHubState, error) {
	if s.src == nil {
		return GitHubState{}, nil
	}
	kind := s.src.Kind()
	gs := GitHubState{SetUp: true}
	in, ok, err := s.store.Integration(ctx, kind)
	if err != nil {
		return GitHubState{}, err
	}
	if ok {
		gs.LastSuccess, gs.Err = in.LastSuccessAt, in.LastError
	}
	if gs.Reviews, err = s.store.SourceItems(ctx, kind); err != nil {
		return GitHubState{}, err
	}
	slices.SortFunc(gs.Reviews, func(a, b domain.SourceItem) int {
		return cmp.Or(a.WaitingSince().Compare(b.WaitingSince()), cmp.Compare(a.ExternalID, b.ExternalID))
	})
	return gs, nil
}
