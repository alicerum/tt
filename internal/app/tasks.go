package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"tt/internal/domain"
	"tt/internal/dsl"
	"tt/internal/store"
)

var (
	ErrNotFound      = store.ErrNotFound
	ErrInvalidParent = errors.New("invalid parent")
	ErrInvalidValue  = errors.New("invalid value")
)

// Add captures a task from a quick-add line (D25).
func (s *Service) Add(ctx context.Context, line string) (domain.Task, Undo, error) {
	d, err := dsl.ParseAdd(line, s.Today())
	if err != nil {
		return domain.Task{}, nil, err
	}
	now := s.clock.Now()
	t := domain.Task{
		Title: d.Title, Note: d.Note, Link: d.Link, Requester: d.Requester, Tags: d.Tags,
		Impact: d.Impact, Due: d.Due, Size: d.Size, Status: domain.StatusOpen, CreatedAt: now, UpdatedAt: now,
	}
	err = s.store.InTx(ctx, func(x *store.Tx) (err error) {
		t.ID, err = x.InsertTask(ctx, t)
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

// EditLine renders a task as a token line for prefilling the edit input.
// The note is left out; Edit keeps it unless the new line has " -- ".
// A child's impact is its parent's (D68), so it is left out (D32, D73); the
// due is the task's own, as an absolute date, never an inherited one.
func EditLine(t domain.Task) string {
	d := dsl.Draft{Title: t.Title, Requester: t.Requester, Tags: t.Tags, Impact: t.Impact, Due: t.Due, Size: t.Size, Link: t.Link}
	if t.ParentID != 0 {
		d.Impact = 0
	}
	return dsl.FormatAdd(d)
}

// ParseEdit parses an edit line for t: !n is refused on a child (D32, D73).
// The TUI calls it to validate before submitting; Edit applies the same rule.
func (s *Service) ParseEdit(t domain.Task, line string) (dsl.Draft, error) {
	if t.ParentID != 0 {
		return s.ParseChild(t.ParentID, line)
	}
	return dsl.ParseAdd(line, s.Today())
}

// Edit replaces title, requester, tags, impact, due, size and link from
// line; a token missing from line clears that field. The note changes only
// if line contains " -- ".
func (s *Service) Edit(ctx context.Context, id int64, line string) (Undo, error) {
	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
		d, err := s.ParseEdit(*t, line)
		if err != nil {
			return err
		}
		t.Title, t.Requester, t.Tags, t.Impact, t.Due, t.Size, t.Link = d.Title, d.Requester, d.Tags, d.Impact, d.Due, d.Size, d.Link
		if d.HasNote {
			t.Note = d.Note
		}
		return nil
	})
}

// SetNote replaces the note (used after editing it in $EDITOR, D29).
func (s *Service) SetNote(ctx context.Context, id int64, note string) (Undo, error) {
	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
		t.Note = strings.TrimRight(note, "\n")
		return nil
	})
}

// SetNoteFrom replaces the note, but only if it is still from: the note as
// it was when the user started editing it. Otherwise ErrConflict (D29a).
func (s *Service) SetNoteFrom(ctx context.Context, id int64, from, note string) (Undo, error) {
	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
		if t.Note != from {
			return fmt.Errorf("note changed since you opened it: %w", ErrConflict)
		}
		t.Note = strings.TrimRight(note, "\n")
		return nil
	})
}

// SetImpact sets impact n (1 to 3, D68); 0 clears it. A child's impact is
// its parent's, so setting one is refused (D73).
func (s *Service) SetImpact(ctx context.Context, id int64, n int) (Undo, error) {
	if n < 0 || n > domain.MaxImpact {
		return nil, fmt.Errorf("impact %d: %w (use 1 to %d, or 0 to clear)", n, ErrInvalidValue, domain.MaxImpact)
	}
	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
		if t.ParentID != 0 && n != 0 {
			return inheritedImpact(t.ParentID, n)
		}
		t.Impact = n
		return nil
	})
}

// SetDue sets the due date (D69); "" clears it.
func (s *Service) SetDue(ctx context.Context, id int64, due domain.Day) (Undo, error) {
	if due != "" {
		if _, err := domain.ParseDay(string(due)); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidValue, err)
		}
	}
	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
		t.Due = due
		return nil
	})
}

// SetSize sets the size; domain.SizeNone clears it.
func (s *Service) SetSize(ctx context.Context, id int64, size domain.Size) (Undo, error) {
	if size != domain.SizeNone && !slices.Contains(domain.Sizes, size) {
		return nil, fmt.Errorf("size %q: %w", size, ErrInvalidValue)
	}
	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
		t.Size = size
		return nil
	})
}

// SetParent makes id a child of parentID (appended last); 0 detaches it.
// Only one level of nesting is allowed (D6). Under a pinned parent the
// child is pinned too (D34).
func (s *Service) SetParent(ctx context.Context, id, parentID int64) (Undo, error) {
	return s.modifyAnd(ctx, id, func(x *store.Tx, t *domain.Task) (undoStep, error) {
		if parentID == 0 {
			t.ParentID, t.Position = 0, 0
			return nil, nil
		}
		if parentID == id {
			return nil, fmt.Errorf("%w: a task can't be its own parent", ErrInvalidParent)
		}
		parent, err := x.GetTask(ctx, parentID)
		if err != nil {
			return nil, err
		}
		if parent.ParentID != 0 {
			return nil, fmt.Errorf("%w: #%d is already a child; only one level of nesting", ErrInvalidParent, parentID)
		}
		if has, err := x.HasChildren(ctx, id); err != nil {
			return nil, err
		} else if has {
			return nil, fmt.Errorf("%w: #%d has children; only one level of nesting", ErrInvalidParent, id)
		}
		pos, err := x.NextChildPosition(ctx, parentID)
		if err != nil {
			return nil, err
		}
		t.ParentID, t.Position = parentID, pos
		return s.pinnedUnderParent(ctx, x, *t)
	})
}

// pinnedUnderParent pins t if it joins a pinned parent and returns the undo
// step that removes that pin again (nil when nothing was pinned).
func (s *Service) pinnedUnderParent(ctx context.Context, x *store.Tx, t domain.Task) (undoStep, error) {
	added, err := s.pinUnderParent(ctx, x, t)
	if err != nil || !added {
		return nil, err
	}
	return removeAddedPin(t.ID), nil
}

// closedAt keeps the original close time when t is already closed.
func (s *Service) closedAt(t domain.Task) time.Time {
	if !t.ClosedAt.IsZero() {
		return t.ClosedAt
	}
	return s.clock.Now()
}

// Done marks a task done.
func (s *Service) Done(ctx context.Context, id int64) (Undo, error) {
	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
		t.Status, t.ClosedAt, t.ClosedBy, t.DropReason = domain.StatusDone, s.closedAt(*t), domain.ClosedByUser, ""
		return nil
	})
}

// Drop marks a task dropped with an optional reason (D23).
func (s *Service) Drop(ctx context.Context, id int64, reason string) (Undo, error) {
	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
		t.Status, t.ClosedAt, t.ClosedBy, t.DropReason = domain.StatusDropped, s.closedAt(*t), domain.ClosedByUser, strings.TrimSpace(reason)
		return nil
	})
}

// Reopen returns a done or dropped task to open. A child of a pinned parent
// is pinned again (D34).
func (s *Service) Reopen(ctx context.Context, id int64) (Undo, error) {
	return s.modifyAnd(ctx, id, func(x *store.Tx, t *domain.Task) (undoStep, error) {
		t.Status, t.ClosedAt, t.ClosedBy, t.DropReason = domain.StatusOpen, time.Time{}, "", ""
		return s.pinnedUnderParent(ctx, x, *t)
	})
}

// Delete hard-deletes a task (D23). It is not undoable; the TUI confirms first.
// Children of a deleted parent become top-level.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.store.InTx(ctx, func(x *store.Tx) error {
		// Children are detached by the delete; their pending undos must not resurrect the link.
		if err := x.TouchChildren(ctx, id, s.clock.Now()); err != nil {
			return err
		}
		return x.DeleteTask(ctx, id)
	})
}
