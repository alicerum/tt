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

// Today is the current planning day (D19).
func (s *Service) Today() domain.Day {
	return domain.PlanningDay(s.clock.Now(), s.cfg.DayStart)
}

// DefaultBlockLength is the block length for placing t into a gap (D24);
// gap 0 means no cap.
func (s *Service) DefaultBlockLength(t domain.Task, gap time.Duration) time.Duration {
	return domain.DefaultBlockLength(t.Size, gap, s.cfg.Sizes, s.cfg.UnsizedBlock)
}

// Pin makes an open task one of today's candidates. Pinning a pinned task
// changes nothing.
func (s *Service) Pin(ctx context.Context, id int64) (Undo, error) {
	today := s.Today()
	var had bool
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		if err := mustBeOpen(ctx, x, id); err != nil {
			return err
		}
		var err error
		if _, had, err = x.GetPin(ctx, id); err != nil || had {
			return err
		}
		return x.PutPin(ctx, domain.Pin{TaskID: id, PinnedOn: today})
	})
	if err != nil {
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
func (s *Service) Unpin(ctx context.Context, id int64) (Undo, error) {
	today := s.Today()
	var (
		pin    domain.Pin
		had    bool
		blocks []domain.Block
	)
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		var err error
		if pin, had, err = x.GetPin(ctx, id); err != nil || !had {
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
	if err != nil {
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
// counter (D10d). Blocks may not overlap each other; meetings are not checked
// here (D18 conflicts are shown, not prevented).
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
			if err := x.DeleteBlock(ctx, b.ID); err != nil {
				return err
			}
			if had {
				return x.PutPin(ctx, pin)
			}
			return x.DeletePin(ctx, id)
		})
	}, nil
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
			return x.UpdateBlock(ctx, old)
		})
	}, nil
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
