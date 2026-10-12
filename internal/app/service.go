// Package app is the only place that changes tt's state (spec: Architecture).
// The TUI and the CLI both call Service.
package app

import (
	"context"
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
	DataVersion(ctx context.Context) (int64, error)
}

// Undo reverts the command that returned it. The TUI keeps these on a
// session-only undo stack (spec: Open items P2).
type Undo func(ctx context.Context) error

type Service struct {
	store Store
	clock Clock
	cfg   config.Config
}

func New(st Store, clock Clock, cfg config.Config) *Service {
	return &Service{store: st, clock: clock, cfg: cfg}
}

func (s *Service) Config() config.Config { return s.cfg }

func (s *Service) Snapshot(ctx context.Context) (domain.Snapshot, error) {
	return s.store.LoadSnapshot(ctx)
}

func (s *Service) DataVersion(ctx context.Context) (int64, error) {
	return s.store.DataVersion(ctx)
}

// modify loads task id, applies fn, saves it, and returns an Undo that
// restores the previous row.
func (s *Service) modify(ctx context.Context, id int64, fn func(x *store.Tx, t *domain.Task) error) (Undo, error) {
	var before domain.Task
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		var err error
		if before, err = x.GetTask(ctx, id); err != nil {
			return err
		}
		after := before
		after.Tags = append([]string(nil), before.Tags...)
		if err := fn(x, &after); err != nil {
			return err
		}
		after.UpdatedAt = s.clock.Now()
		return x.UpdateTask(ctx, after)
	})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) error {
		return s.store.InTx(ctx, func(x *store.Tx) error { return x.UpdateTask(ctx, before) })
	}, nil
}
