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
	d, err := dsl.ParseAdd(line, len(s.cfg.Priorities))
	if err != nil {
		return domain.Task{}, nil, err
	}
	now := s.clock.Now()
	t := domain.Task{
		Title: d.Title, Note: d.Note, Link: d.Link, Requester: d.Requester, Tags: d.Tags,
		Priority: d.Priority, Size: d.Size, Status: domain.StatusOpen, CreatedAt: now, UpdatedAt: now,
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
func EditLine(t domain.Task) string {
	return dsl.FormatAdd(dsl.Draft{
		Title: t.Title, Requester: t.Requester, Tags: t.Tags, Priority: t.Priority, Size: t.Size, Link: t.Link,
	})
}

// Edit replaces title, requester, tags, priority, size and link from line;
// a token missing from line clears that field. The note changes only if
// line contains " -- ".
func (s *Service) Edit(ctx context.Context, id int64, line string) (Undo, error) {
	d, err := dsl.ParseAdd(line, len(s.cfg.Priorities))
	if err != nil {
		return nil, err
	}
	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
		t.Title, t.Requester, t.Tags, t.Priority, t.Size, t.Link = d.Title, d.Requester, d.Tags, d.Priority, d.Size, d.Link
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

// SetPriority sets priority p (1..levels); 0 clears it.
func (s *Service) SetPriority(ctx context.Context, id int64, p int) (Undo, error) {
	if p < 0 || p > len(s.cfg.Priorities) {
		return nil, fmt.Errorf("priority %d: %w (use 1 to %d, or 0 to clear)", p, ErrInvalidValue, len(s.cfg.Priorities))
	}
	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
		t.Priority = p
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
// Only one level of nesting is allowed (D6).
func (s *Service) SetParent(ctx context.Context, id, parentID int64) (Undo, error) {
	return s.modify(ctx, id, func(x *store.Tx, t *domain.Task) error {
		if parentID == 0 {
			t.ParentID, t.Position = 0, 0
			return nil
		}
		if parentID == id {
			return fmt.Errorf("%w: a task can't be its own parent", ErrInvalidParent)
		}
		parent, err := x.GetTask(ctx, parentID)
		if err != nil {
			return err
		}
		if parent.ParentID != 0 {
			return fmt.Errorf("%w: #%d is already a child; only one level of nesting", ErrInvalidParent, parentID)
		}
		if has, err := x.HasChildren(ctx, id); err != nil {
			return err
		} else if has {
			return fmt.Errorf("%w: #%d has children; only one level of nesting", ErrInvalidParent, id)
		}
		pos, err := x.NextChildPosition(ctx, parentID)
		if err != nil {
			return err
		}
		t.ParentID, t.Position = parentID, pos
		return nil
	})
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

// Reopen returns a done or dropped task to open.
func (s *Service) Reopen(ctx context.Context, id int64) (Undo, error) {
	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
		t.Status, t.ClosedAt, t.ClosedBy, t.DropReason = domain.StatusOpen, time.Time{}, "", ""
		return nil
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
