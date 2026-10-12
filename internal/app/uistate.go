package app

import (
	"context"

	"tt/internal/store"
)

// UIState returns a remembered UI choice (e.g. the backlog grouping, D26);
// "" when none is stored. UI state is not undoable.
func (s *Service) UIState(ctx context.Context, key string) (string, error) {
	v, _, err := s.store.UIState(ctx, key)
	return v, err
}

// SetUIState remembers a UI choice across restarts.
func (s *Service) SetUIState(ctx context.Context, key, value string) error {
	return s.store.InTx(ctx, func(x *store.Tx) error { return x.PutUIState(ctx, key, value) })
}
