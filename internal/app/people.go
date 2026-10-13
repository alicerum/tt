package app

import (
	"context"
	"fmt"

	"tt/internal/domain"
	"tt/internal/dsl"
	"tt/internal/store"
)

// RenameRequester gives every task whose requester is from (open or closed,
// children included) the requester to, normalized like the @who token
// (D53). When to already names someone, that merges the two. n is how many
// tasks changed; renaming to the same name changes nothing. The undo gives
// exactly those tasks their old rows back, so a merge's undo never touches
// the target's own tasks, and refuses as a whole (ErrConflict) once any of
// them changed since.
func (s *Service) RenameRequester(ctx context.Context, from, to string) (n int, undo Undo, err error) {
	if from == "" {
		return 0, nil, fmt.Errorf("no name to rename: %w", ErrInvalidValue)
	}
	name, err := dsl.ParseRequester(to)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %w", err, ErrInvalidValue)
	}
	if name == from {
		return 0, nil, nil
	}
	var before, after []domain.Task
	err = s.store.InTx(ctx, func(x *store.Tx) error {
		ids, err := x.TasksWithRequester(ctx, from)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return fmt.Errorf("no task has @%s any more: %w", from, ErrNotFound)
		}
		now := s.clock.Now()
		for _, id := range ids {
			t, err := x.GetTask(ctx, id)
			if err != nil {
				return err
			}
			before = append(before, t)
			t.Tags = append([]string(nil), t.Tags...)
			t.Requester, t.UpdatedAt = name, now
			if err := x.UpdateTask(ctx, t); err != nil {
				return err
			}
			after = append(after, t)
		}
		return nil
	})
	if err != nil {
		return 0, nil, err
	}
	return len(after), func(ctx context.Context) error {
		return s.store.InTx(ctx, func(x *store.Tx) error {
			for _, t := range after { // all or nothing: check every task before restoring any
				if err := unchangedSince(ctx, x, t); err != nil {
					return err
				}
			}
			for _, t := range before {
				if err := x.UpdateTask(ctx, t); err != nil {
					return err
				}
			}
			return nil
		})
	}, nil
}
