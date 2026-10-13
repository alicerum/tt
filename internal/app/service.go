// Package app is the only place that changes tt's state (spec: Architecture).
// The TUI and the CLI both call Service.
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/store"
)

type Clock interface{ Now() time.Time }

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

// Store is what Service needs from persistence; *store.Store implements it.
type Store interface {
	InTx(ctx context.Context, fn func(*store.Tx) error) error
	LoadSnapshot(ctx context.Context) (domain.Snapshot, error)
	LoadPlan(ctx context.Context, day domain.Day) (domain.Snapshot, store.PlanState, error)
	DataVersion(ctx context.Context) (int64, error)
	UIState(ctx context.Context, key string) (string, bool, error)
	Integration(ctx context.Context, name string) (store.Integration, bool, error)
}

// Undo reverts the command that returned it. The TUI keeps these on a
// session-only undo stack (spec: Open items P2).
type Undo func(ctx context.Context) error

type Service struct {
	store Store
	clock Clock
	cfg   config.Config
	cal   CalendarSource // nil: no calendar set up (D40)
}

func New(st Store, clock Clock, cfg config.Config) *Service {
	return &Service{store: st, clock: clock, cfg: cfg}
}

func (s *Service) Config() config.Config { return s.cfg }

// Now is the service clock's current time (for "captured 2d ago" and similar).
func (s *Service) Now() time.Time { return s.clock.Now() }

func (s *Service) Snapshot(ctx context.Context) (domain.Snapshot, error) {
	return s.store.LoadSnapshot(ctx)
}

func (s *Service) DataVersion(ctx context.Context) (int64, error) {
	return s.store.DataVersion(ctx)
}

// ErrConflict means the task changed after the command being undone, e.g.
// a rollover or a GitHub auto-close; undoing would silently revert that.
var ErrConflict = errors.New("task changed since; can't undo")

// undoStep is extra work an undo does in its transaction.
type undoStep func(ctx context.Context, x *store.Tx) error

// modify loads task id, applies fn, saves it, and returns an Undo that
// restores the previous row, but only if nothing has changed the row since.
func (s *Service) modify(ctx context.Context, id int64, fn func(x *store.Tx, t *domain.Task) error) (Undo, error) {
	return s.modifyAnd(ctx, id, func(x *store.Tx, t *domain.Task) (undoStep, error) { return nil, fn(x, t) })
}

// modifyAnd is modify for commands whose undo has more to do: the step fn
// returns (if any) runs in the undo's transaction after the row is restored.
func (s *Service) modifyAnd(ctx context.Context, id int64, fn func(x *store.Tx, t *domain.Task) (undoStep, error)) (Undo, error) {
	var (
		before, after domain.Task
		step          undoStep
	)
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		var err error
		if before, err = x.GetTask(ctx, id); err != nil {
			return err
		}
		after = before
		after.Tags = append([]string(nil), before.Tags...)
		if step, err = fn(x, &after); err != nil {
			return err
		}
		after.UpdatedAt = s.clock.Now()
		return x.UpdateTask(ctx, after)
	})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) error {
		return s.store.InTx(ctx, func(x *store.Tx) error {
			if err := unchangedSince(ctx, x, after); err != nil {
				return err
			}
			if err := x.UpdateTask(ctx, before); err != nil || step == nil {
				return err
			}
			return step(ctx, x)
		})
	}, nil
}

// unchangedSince fails with ErrConflict unless task want.ID still has the
// UpdatedAt that want carries.
func unchangedSince(ctx context.Context, x *store.Tx, want domain.Task) error {
	cur, err := x.GetTask(ctx, want.ID)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("task #%d is gone: %w", want.ID, ErrConflict)
	}
	if err != nil {
		return err
	}
	if !cur.UpdatedAt.Equal(want.UpdatedAt) {
		return fmt.Errorf("task #%d: %w", want.ID, ErrConflict)
	}
	return nil
}
