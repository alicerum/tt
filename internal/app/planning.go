package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"tt/internal/domain"
	"tt/internal/store"
)

var ErrOverlap = errors.New("overlaps another block")

// ErrParentPinned: a child of a pinned parent is pinned with it, so it can't
// be unpinned on its own (D34).
var ErrParentPinned = errors.New("unpin the parent first")

// Today is the current planning day (D19).
func (s *Service) Today() domain.Day {
	return domain.PlanningDay(s.clock.Now(), s.cfg.DayStart)
}

// DayOf is the planning day containing t, in the clock's time zone (stored
// times come back as UTC).
func (s *Service) DayOf(t time.Time) domain.Day {
	return domain.PlanningDay(t.In(s.clock.Now().Location()), s.cfg.DayStart)
}

// DefaultBlockLength is the block length for placing t into a gap (D24);
// gap 0 means no cap.
func (s *Service) DefaultBlockLength(t domain.Task, gap time.Duration) time.Duration {
	return domain.DefaultBlockLength(t.Size, gap, s.cfg.Sizes, s.cfg.UnsizedBlock)
}

// Pin makes an open task one of today's candidates. Pinning a pinned task
// changes nothing. Pinning a parent pins its open children too (D34); a task
// with children returns no undo (unpinning it by hand reverses the pin).
func (s *Service) Pin(ctx context.Context, id int64) (Undo, error) {
	today := s.Today()
	var had, parent bool
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		if err := mustBeOpen(ctx, x, id); err != nil {
			return err
		}
		var err error
		if parent, err = pinChildren(ctx, x, id, today); err != nil {
			return err
		}
		if _, had, err = x.GetPin(ctx, id); err != nil || had {
			return err
		}
		return x.PutPin(ctx, domain.Pin{TaskID: id, PinnedOn: today})
	})
	if err != nil || parent {
		return nil, err
	}
	return func(ctx context.Context) error {
		if had {
			return nil
		}
		if err := s.checkDay(today); err != nil {
			return err
		}
		return s.store.InTx(ctx, func(x *store.Tx) error {
			blocks, err := x.TaskBlocksOn(ctx, id, today)
			if err != nil {
				return err
			}
			if len(blocks) > 0 {
				return fmt.Errorf("task #%d is blocked today: %w", id, ErrConflict)
			}
			return x.DeletePin(ctx, id)
		})
	}, nil
}

// Unpin removes a task from Today, together with its blocks on today's timeline.
// Unpinning a parent unpins its children too, except children with a block
// today: they stay pinned on their own, so no block is lost (D34). A task with
// children returns no undo. A child of a pinned parent can't be unpinned on
// its own (ErrParentPinned).
func (s *Service) Unpin(ctx context.Context, id int64) (Undo, error) {
	today := s.Today()
	var (
		pin    domain.Pin
		had    bool
		parent bool
		blocks []domain.Block
	)
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		var err error
		if pin, had, err = x.GetPin(ctx, id); err != nil || !had {
			return err
		}
		t, err := x.GetTask(ctx, id)
		if err != nil {
			return err
		}
		if t.ParentID != 0 {
			if _, parentPinned, err := x.GetPin(ctx, t.ParentID); err != nil {
				return err
			} else if parentPinned {
				return fmt.Errorf("#%d is pinned with its parent #%d: %w", id, t.ParentID, ErrParentPinned)
			}
		}
		if parent, err = unpinChildren(ctx, x, id, today); err != nil {
			return err
		}
		if blocks, err = x.TaskBlocksOn(ctx, id, today); err != nil {
			return err
		}
		for _, b := range blocks {
			if err := x.DeleteBlock(ctx, b.ID); err != nil {
				return err
			}
		}
		return x.DeletePin(ctx, id)
	})
	if err != nil || parent {
		return nil, err
	}
	return func(ctx context.Context) error {
		if !had {
			return nil
		}
		if err := s.checkDay(today); err != nil {
			return err
		}
		return s.store.InTx(ctx, func(x *store.Tx) error {
			if err := taskStillExists(ctx, x, pin.TaskID); err != nil {
				return err
			}
			if err := x.PutPin(ctx, pin); err != nil {
				return err
			}
			for _, b := range blocks {
				if err := checkFreeInsert(ctx, x, b); err != nil {
					return err
				}
				if _, err := x.InsertBlock(ctx, b); err != nil {
					return err
				}
			}
			return nil
		})
	}, nil
}

// Block places an open task on today's timeline at wall-clock offset start
// (from midnight) for dur. It pins the task and resets its pinned-days
// counter (D10d); a parent it pins brings its open children along (D34; the
// undo leaves those pinned). Blocks may not overlap each other; meetings are
// not checked here (D18 conflicts are shown, not prevented).
func (s *Service) Block(ctx context.Context, id int64, start, dur time.Duration) (domain.Block, Undo, error) {
	b := domain.Block{TaskID: id, Day: s.Today(), Start: start, Duration: dur}
	if err := s.validateSpan(b); err != nil {
		return domain.Block{}, nil, err
	}
	var (
		pin domain.Pin
		had bool
	)
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		if err := mustBeOpen(ctx, x, id); err != nil {
			return err
		}
		if err := checkFree(ctx, x, b); err != nil {
			return err
		}
		var err error
		if pin, had, err = x.GetPin(ctx, id); err != nil {
			return err
		}
		if !had {
			if _, err := pinChildren(ctx, x, id, b.Day); err != nil {
				return err
			}
		}
		if err := x.PutPin(ctx, domain.Pin{TaskID: id, PinnedOn: b.Day}); err != nil {
			return err
		}
		b.ID, err = x.InsertBlock(ctx, b)
		return err
	})
	if err != nil {
		return domain.Block{}, nil, err
	}
	return b, func(ctx context.Context) error {
		if err := s.checkDay(b.Day); err != nil {
			return err
		}
		return s.store.InTx(ctx, func(x *store.Tx) error {
			if err := x.DeleteBlock(ctx, b.ID); errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("block #%d is gone: %w", b.ID, ErrConflict)
			} else if err != nil {
				return err
			}
			if had {
				if err := taskStillExists(ctx, x, id); err != nil {
					return err
				}
				return x.PutPin(ctx, pin)
			}
			left, err := x.TaskBlocksOn(ctx, id, b.Day)
			if err != nil {
				return err
			}
			if len(left) > 0 {
				return nil
			}
			return x.DeletePin(ctx, id)
		})
	}, nil
}

// SetBlockSpan sets the start and the length of one of today's blocks in one
// step (fine-tune, D36).
func (s *Service) SetBlockSpan(ctx context.Context, blockID int64, start, dur time.Duration) (Undo, error) {
	return s.reshape(ctx, blockID, func(b *domain.Block) { b.Start, b.Duration = start, dur })
}

// MoveBlock moves one of today's blocks to a new start.
func (s *Service) MoveBlock(ctx context.Context, blockID int64, start time.Duration) (Undo, error) {
	return s.reshape(ctx, blockID, func(b *domain.Block) { b.Start = start })
}

// ResizeBlock changes the length of one of today's blocks.
func (s *Service) ResizeBlock(ctx context.Context, blockID int64, dur time.Duration) (Undo, error) {
	return s.reshape(ctx, blockID, func(b *domain.Block) { b.Duration = dur })
}

// Unblock removes one of today's blocks; the task stays pinned as a candidate.
func (s *Service) Unblock(ctx context.Context, blockID int64) (Undo, error) {
	day := s.Today()
	var old domain.Block
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		var err error
		if old, err = s.todaysBlock(ctx, x, blockID); err != nil {
			return err
		}
		return x.DeleteBlock(ctx, blockID)
	})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) error {
		if err := s.checkDay(day); err != nil {
			return err
		}
		return s.store.InTx(ctx, func(x *store.Tx) error {
			if err := taskStillExists(ctx, x, old.TaskID); err != nil {
				return err
			}
			if err := checkFreeInsert(ctx, x, old); err != nil {
				return err
			}
			_, err := x.InsertBlock(ctx, old)
			return err
		})
	}, nil
}

func (s *Service) reshape(ctx context.Context, blockID int64, fn func(*domain.Block)) (Undo, error) {
	day := s.Today()
	var old domain.Block
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		var err error
		if old, err = s.todaysBlock(ctx, x, blockID); err != nil {
			return err
		}
		b := old
		fn(&b)
		if err := s.validateSpan(b); err != nil {
			return err
		}
		if err := checkFree(ctx, x, b); err != nil {
			return err
		}
		return x.UpdateBlock(ctx, b)
	})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) error {
		if err := s.checkDay(day); err != nil {
			return err
		}
		return s.store.InTx(ctx, func(x *store.Tx) error {
			if err := checkFreeForUndo(ctx, x, old); err != nil {
				return err
			}
			if err := x.UpdateBlock(ctx, old); errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("block #%d is gone: %w", old.ID, ErrConflict)
			} else {
				return err
			}
		})
	}, nil
}

// pinChildren pins id's open children that aren't pinned yet (D34) and
// reports whether id has any children.
func pinChildren(ctx context.Context, x *store.Tx, id int64, day domain.Day) (bool, error) {
	kids, err := x.Children(ctx, id)
	if err != nil {
		return false, err
	}
	for _, k := range kids {
		if k.Status != domain.StatusOpen {
			continue
		}
		if _, had, err := x.GetPin(ctx, k.ID); err != nil {
			return false, err
		} else if had {
			continue
		}
		if err := x.PutPin(ctx, domain.Pin{TaskID: k.ID, PinnedOn: day}); err != nil {
			return false, err
		}
	}
	return len(kids) > 0, nil
}

// unpinChildren unpins id's children that have no block on day (D34) and
// reports whether id has any children.
func unpinChildren(ctx context.Context, x *store.Tx, id int64, day domain.Day) (bool, error) {
	kids, err := x.Children(ctx, id)
	if err != nil {
		return false, err
	}
	for _, k := range kids {
		blocks, err := x.TaskBlocksOn(ctx, k.ID, day)
		if err != nil {
			return false, err
		}
		if len(blocks) > 0 {
			continue // a blocked child stays pinned on its own
		}
		if err := x.DeletePin(ctx, k.ID); err != nil {
			return false, err
		}
	}
	return len(kids) > 0, nil
}

// pinUnderParent pins t when it is an open child of a pinned parent and not
// pinned yet (D34). added reports whether it put a pin; the undo of the
// command then removes it again with removeAddedPin.
func (s *Service) pinUnderParent(ctx context.Context, x *store.Tx, t domain.Task) (added bool, err error) {
	if t.ParentID == 0 || t.Status != domain.StatusOpen {
		return false, nil
	}
	if _, parentPinned, err := x.GetPin(ctx, t.ParentID); err != nil || !parentPinned {
		return false, err
	}
	if _, had, err := x.GetPin(ctx, t.ID); err != nil || had {
		return false, err
	}
	return true, x.PutPin(ctx, domain.Pin{TaskID: t.ID, PinnedOn: s.Today()})
}

// removeAddedPin is the undo step for a pin pinUnderParent added: the pin
// goes unless the task got a block meanwhile.
func removeAddedPin(id int64) undoStep {
	return func(ctx context.Context, x *store.Tx) error {
		pin, had, err := x.GetPin(ctx, id)
		if err != nil || !had {
			return err
		}
		blocks, err := x.TaskBlocksOn(ctx, id, pin.PinnedOn)
		if err != nil || len(blocks) > 0 {
			return err
		}
		return x.DeletePin(ctx, id)
	}
}

// taskStillExists fails with ErrConflict if the task was deleted since.
func taskStillExists(ctx context.Context, x *store.Tx, id int64) error {
	if _, err := x.GetTask(ctx, id); errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("task #%d is gone: %w", id, ErrConflict)
	} else {
		return err
	}
}

// checkDay refuses an undo once the planning day has moved on (ErrConflict):
// the day's rollover has already rewritten the state the undo would restore.
func (s *Service) checkDay(day domain.Day) error {
	if s.Today() != day {
		return fmt.Errorf("planning day changed since: %w", ErrConflict)
	}
	return nil
}

// checkFreeForUndo is checkFree for undo writes: an overlap means the slot was
// taken after the original command, so the undo is a conflict.
func checkFreeForUndo(ctx context.Context, x *store.Tx, b domain.Block) error {
	err := checkFree(ctx, x, b)
	if errors.Is(err, ErrOverlap) {
		return fmt.Errorf("undo would overlap another block: %w", ErrConflict)
	}
	return err
}

// checkFreeInsert is checkFreeForUndo for a block being re-inserted. Its row
// does not exist, and SQLite may have reused its ID for another block, so no
// block may be excluded by ID here.
func checkFreeInsert(ctx context.Context, x *store.Tx, b domain.Block) error {
	b.ID = 0
	return checkFreeForUndo(ctx, x, b)
}

// todaysBlock loads a block and refuses blocks from other days: past days are
// rollover history (D10).
func (s *Service) todaysBlock(ctx context.Context, x *store.Tx, id int64) (domain.Block, error) {
	b, err := x.GetBlock(ctx, id)
	if err != nil {
		return domain.Block{}, err
	}
	if today := s.Today(); b.Day != today {
		return domain.Block{}, fmt.Errorf("block #%d is on %s, not today (%s): %w", id, b.Day, today, ErrInvalidValue)
	}
	return b, nil
}

// validateSpan checks that b is whole minutes, at least a minute long, and
// inside the planning day: from day start until the next day start.
func (s *Service) validateSpan(b domain.Block) error {
	switch {
	case b.Start%time.Minute != 0 || b.Duration%time.Minute != 0:
		return fmt.Errorf("block times must be whole minutes: %w", ErrInvalidValue)
	case b.Duration < time.Minute:
		return fmt.Errorf("block length %v: %w", b.Duration, ErrInvalidValue)
	case b.Duration > 24*time.Hour:
		return fmt.Errorf("block length %v is longer than a day: %w", b.Duration, ErrInvalidValue)
	case b.Start > s.cfg.DayStart+24*time.Hour:
		return fmt.Errorf("block start %v is past the planning day: %w", b.Start, ErrInvalidValue)
	case b.Start < s.cfg.DayStart || b.End() > s.cfg.DayStart+24*time.Hour:
		return fmt.Errorf("block %v–%v is outside the planning day: %w", b.Start, b.End(), ErrInvalidValue)
	}
	return nil
}

// checkFree fails with ErrOverlap if b overlaps another block on its day.
func checkFree(ctx context.Context, x *store.Tx, b domain.Block) error {
	blocks, err := x.BlocksOn(ctx, b.Day)
	if err != nil {
		return err
	}
	for _, o := range blocks {
		if o.ID != b.ID && o.Overlaps(b) {
			return fmt.Errorf("block %v–%v and block #%d: %w", b.Start, b.End(), o.ID, ErrOverlap)
		}
	}
	return nil
}

func mustBeOpen(ctx context.Context, x *store.Tx, id int64) error {
	t, err := x.GetTask(ctx, id)
	if err != nil {
		return err
	}
	if t.Status != domain.StatusOpen {
		return fmt.Errorf("task #%d is %s: %w", id, t.Status, ErrInvalidValue)
	}
	return nil
}
