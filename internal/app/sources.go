package app

import (
	"cmp"
	"context"
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
	// Reviews is the Reviews lane: the requests with no open task, longest
	// waiting first.
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

// githubState reads the last fetch and the lane: the stored items without an
// open task in snap (a pinned review is a candidate instead; a closed one
// whose PR still requests me is back, D49).
func (s *Service) githubState(ctx context.Context, snap domain.Snapshot) (GitHubState, error) {
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
	items, err := s.store.SourceItems(ctx, kind)
	if err != nil {
		return GitHubState{}, err
	}
	planned := map[string]bool{}
	for _, t := range snap.Tasks() {
		if t.Source != nil && t.Source.Kind == kind && t.Status == domain.StatusOpen {
			planned[t.Source.ID] = true
		}
	}
	for _, i := range items {
		if !planned[i.ExternalID] {
			gs.Reviews = append(gs.Reviews, i)
		}
	}
	slices.SortFunc(gs.Reviews, func(a, b domain.SourceItem) int {
		return cmp.Or(a.WaitingSince().Compare(b.WaitingSince()), cmp.Compare(a.ExternalID, b.ExternalID))
	})
	return gs, nil
}
