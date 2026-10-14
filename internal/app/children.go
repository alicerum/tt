package app

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"tt/internal/domain"
	"tt/internal/dsl"
	"tt/internal/store"
)

// ErrNoMove means a reorder had no sibling to move past.
var ErrNoMove = errors.New("no sibling in that direction")

// ParseChild parses an add-child line for parentID. Children inherit their
// parent's impact (D68), so !n is refused (D32, D73); ^date is their own.
func (s *Service) ParseChild(parentID int64, line string) (dsl.Draft, error) {
	d, err := dsl.ParseAdd(line, s.Today())
	if err != nil {
		return dsl.Draft{}, err
	}
	if d.Impact != 0 {
		return dsl.Draft{}, inheritedImpact(parentID, d.Impact)
	}
	return d, nil
}

func inheritedImpact(parentID int64, n int) error {
	return fmt.Errorf("%w: children inherit impact from #%d; drop !%d", ErrInvalidValue, parentID, n)
}

// AddChild captures a task from line as the last child of parentID. Under a
// pinned parent the child is pinned too (D34); undoing the add deletes the
// child, and its pin with it.
func (s *Service) AddChild(ctx context.Context, parentID int64, line string) (domain.Task, Undo, error) {
	d, err := s.ParseChild(parentID, line)
	if err != nil {
		return domain.Task{}, nil, err
	}
	now := s.clock.Now()
	t := domain.Task{
		Title: d.Title, Note: d.Note, Link: d.Link, Requester: d.Requester, Tags: d.Tags, Due: d.Due, Size: d.Size,
		ParentID: parentID, Status: domain.StatusOpen, CreatedAt: now, UpdatedAt: now,
	}
	err = s.store.InTx(ctx, func(x *store.Tx) error {
		parent, err := x.GetTask(ctx, parentID)
		if err != nil {
			return err
		}
		if parent.ParentID != 0 {
			return fmt.Errorf("%w: #%d is already a child; only one level of nesting", ErrInvalidParent, parentID)
		}
		if t.Position, err = x.NextChildPosition(ctx, parentID); err != nil {
			return err
		}
		if t.ID, err = x.InsertTask(ctx, t); err != nil {
			return err
		}
		_, err = s.pinUnderParent(ctx, x, t)
		return err
	})
	if err != nil {
		return domain.Task{}, nil, err
	}
	added := t
	undo := func(ctx context.Context) error {
		return s.store.InTx(ctx, func(x *store.Tx) error {
			if err := unchangedSince(ctx, x, added); err != nil {
				return err
			}
			return x.DeleteTask(ctx, added.ID)
		})
	}
	return t, undo, nil
}

// Reorder moves child id one place up (delta < 0) or down (delta > 0) past
// its nearest sibling of the same kind: open children move among open ones,
// closed among closed, matching how Task detail lists them. Siblings are then
// renumbered 0..n-1. ErrNoMove when there is no such sibling.
func (s *Service) Reorder(ctx context.Context, id int64, delta int) (Undo, error) {
	var before, after []domain.Task
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		t, err := x.GetTask(ctx, id)
		if err != nil {
			return err
		}
		if t.ParentID == 0 {
			return fmt.Errorf("%w: #%d has no parent to reorder within", ErrInvalidParent, id)
		}
		sibs, err := x.Children(ctx, t.ParentID)
		if err != nil {
			return err
		}
		open := t.Status == domain.StatusOpen
		i := slices.IndexFunc(sibs, func(c domain.Task) bool { return c.ID == id })
		j := i + sign(delta)
		for j >= 0 && j < len(sibs) && (sibs[j].Status == domain.StatusOpen) != open {
			j += sign(delta)
		}
		if delta == 0 || j < 0 || j >= len(sibs) {
			return ErrNoMove
		}
		order := slices.Clone(sibs)
		order[i], order[j] = order[j], order[i]
		now := s.clock.Now()
		for pos, c := range order {
			if c.Position == pos {
				continue
			}
			before = append(before, c)
			c.Position, c.UpdatedAt = pos, now
			if err := x.UpdateTask(ctx, c); err != nil {
				return err
			}
			after = append(after, c)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) error {
		return s.store.InTx(ctx, func(x *store.Tx) error {
			for _, c := range after {
				if err := unchangedSince(ctx, x, c); err != nil {
					return err
				}
			}
			for _, c := range before {
				if err := x.UpdateTask(ctx, c); err != nil {
					return err
				}
			}
			return nil
		})
	}, nil
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
