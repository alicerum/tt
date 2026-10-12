package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"tt/internal/domain"
)

// PinState is a pin plus its derived pinned-days counter (D10d).
type PinState struct {
	domain.Pin
	PinnedDays int // planning days in `days` from PinnedOn through the plan's day
}

// PlanState is the planning data for one day.
type PlanState struct {
	Pins      []PinState
	Blocks    []domain.Block // by start
	Committed bool
}

func (x *Tx) GetPin(ctx context.Context, taskID int64) (domain.Pin, bool, error) {
	var on string
	err := x.tx.QueryRowContext(ctx, "SELECT pinned_on FROM pins WHERE task_id = ?", taskID).Scan(&on)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Pin{}, false, nil
	}
	if err != nil {
		return domain.Pin{}, false, err
	}
	return domain.Pin{TaskID: taskID, PinnedOn: domain.Day(on)}, true, nil
}

// PutPin creates or replaces the pin for p.TaskID.
func (x *Tx) PutPin(ctx context.Context, p domain.Pin) error {
	_, err := x.tx.ExecContext(ctx, `INSERT INTO pins (task_id, pinned_on) VALUES (?, ?)
		ON CONFLICT (task_id) DO UPDATE SET pinned_on = excluded.pinned_on`, p.TaskID, string(p.PinnedOn))
	return err
}

// DeletePin removes the pin for taskID, if any.
func (x *Tx) DeletePin(ctx context.Context, taskID int64) error {
	_, err := x.tx.ExecContext(ctx, "DELETE FROM pins WHERE task_id = ?", taskID)
	return err
}

// DeletePinsOfClosedTasks unpins done and dropped tasks (their "done today"
// visibility ends when a new planning day starts).
func (x *Tx) DeletePinsOfClosedTasks(ctx context.Context) error {
	_, err := x.tx.ExecContext(ctx, "DELETE FROM pins WHERE task_id IN (SELECT id FROM tasks WHERE status != 'open')")
	return err
}

func (x *Tx) CountOpenPins(ctx context.Context) (int, error) {
	var n int
	err := x.tx.QueryRowContext(ctx, `SELECT count(*) FROM pins p JOIN tasks t ON t.id = p.task_id
		WHERE t.status = 'open'`).Scan(&n)
	return n, err
}

// InsertBlock inserts b and returns its id. A non-zero b.ID is kept (used by undo).
func (x *Tx) InsertBlock(ctx context.Context, b domain.Block) (int64, error) {
	var id any
	if b.ID != 0 {
		id = b.ID
	}
	res, err := x.tx.ExecContext(ctx, "INSERT INTO blocks (id, task_id, day, start_min, duration_min) VALUES (?, ?, ?, ?, ?)",
		id, b.TaskID, string(b.Day), minutes(b.Start), minutes(b.Duration))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (x *Tx) GetBlock(ctx context.Context, id int64) (domain.Block, error) {
	b, err := scanBlock(x.tx.QueryRowContext(ctx, "SELECT "+blockColumns+" FROM blocks WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Block{}, fmt.Errorf("block #%d: %w", id, ErrNotFound)
	}
	return b, err
}

// UpdateBlock overwrites day, start and duration of block b.ID.
func (x *Tx) UpdateBlock(ctx context.Context, b domain.Block) error {
	res, err := x.tx.ExecContext(ctx, "UPDATE blocks SET day = ?, start_min = ?, duration_min = ? WHERE id = ?",
		string(b.Day), minutes(b.Start), minutes(b.Duration), b.ID)
	if err != nil {
		return err
	}
	return mustAffectBlock(res, b.ID)
}

func (x *Tx) DeleteBlock(ctx context.Context, id int64) error {
	res, err := x.tx.ExecContext(ctx, "DELETE FROM blocks WHERE id = ?", id)
	if err != nil {
		return err
	}
	return mustAffectBlock(res, id)
}

// BlocksOn returns day's blocks ordered by start.
func (x *Tx) BlocksOn(ctx context.Context, day domain.Day) ([]domain.Block, error) {
	return queryBlocks(ctx, x.tx, "WHERE day = ?", string(day))
}

// TaskBlocksOn returns taskID's blocks on day.
func (x *Tx) TaskBlocksOn(ctx context.Context, taskID int64, day domain.Day) ([]domain.Block, error) {
	return queryBlocks(ctx, x.tx, "WHERE task_id = ? AND day = ?", taskID, string(day))
}

// OpenTasksBlockedOn returns ids of open tasks with at least one block on day.
func (x *Tx) OpenTasksBlockedOn(ctx context.Context, day domain.Day) ([]int64, error) {
	rows, err := x.tx.QueryContext(ctx, `SELECT DISTINCT b.task_id FROM blocks b JOIN tasks t ON t.id = b.task_id
		WHERE b.day = ? AND t.status = 'open' ORDER BY b.task_id`, string(day))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (x *Tx) HasDay(ctx context.Context, day domain.Day) (bool, error) {
	var n int
	err := x.tx.QueryRowContext(ctx, "SELECT count(*) FROM days WHERE day = ?", string(day)).Scan(&n)
	return n > 0, err
}

// LatestDayBefore returns the most recent planning day before day, if any.
func (x *Tx) LatestDayBefore(ctx context.Context, day domain.Day) (domain.Day, bool, error) {
	var d sql.NullString
	err := x.tx.QueryRowContext(ctx, "SELECT max(day) FROM days WHERE day < ?", string(day)).Scan(&d)
	if err != nil || !d.Valid {
		return "", false, err
	}
	return domain.Day(d.String), true, nil
}

func (x *Tx) InsertDay(ctx context.Context, day domain.Day, startedAt time.Time) error {
	_, err := x.tx.ExecContext(ctx, "INSERT INTO days (day, started_at) VALUES (?, ?)", string(day), formatTime(startedAt))
	return err
}

// CommitDay records that day's plan was committed at at.
func (x *Tx) CommitDay(ctx context.Context, day domain.Day, at time.Time) error {
	res, err := x.tx.ExecContext(ctx, "UPDATE days SET committed_at = ? WHERE day = ?", formatTime(at), string(day))
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("day %s: %w", day, ErrNotFound)
	}
	return nil
}

// CountDone counts tasks marked done with closed_at in [from, to).
func (x *Tx) CountDone(ctx context.Context, from, to time.Time) (int, error) {
	var n int
	err := x.tx.QueryRowContext(ctx, "SELECT count(*) FROM tasks WHERE status = 'done' AND closed_at >= ? AND closed_at < ?",
		formatTime(from), formatTime(to)).Scan(&n)
	return n, err
}

// LoadPlan reads all tasks and day's planning state in one consistent read.
func (s *Store) LoadPlan(ctx context.Context, day domain.Day) (domain.Snapshot, PlanState, error) {
	var (
		snap  domain.Snapshot
		state PlanState
	)
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		tasks, err := loadTasks(ctx, tx)
		if err != nil {
			return err
		}
		snap = domain.NewSnapshot(tasks)

		rows, err := tx.QueryContext(ctx, `SELECT p.task_id, p.pinned_on,
			(SELECT count(*) FROM days d WHERE d.day >= p.pinned_on AND d.day <= ?)
			FROM pins p ORDER BY p.task_id`, string(day))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p PinState
			var on string
			if err := rows.Scan(&p.TaskID, &on, &p.PinnedDays); err != nil {
				return err
			}
			p.PinnedOn = domain.Day(on)
			state.Pins = append(state.Pins, p)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		if state.Blocks, err = queryBlocks(ctx, tx, "WHERE day = ?", string(day)); err != nil {
			return err
		}
		var committed sql.NullString
		err = tx.QueryRowContext(ctx, "SELECT committed_at FROM days WHERE day = ?", string(day)).Scan(&committed)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		state.Committed = committed.Valid
		return nil
	})
	return snap, state, err
}

const blockColumns = "id, task_id, day, start_min, duration_min"

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func queryBlocks(ctx context.Context, q querier, where string, args ...any) ([]domain.Block, error) {
	rows, err := q.QueryContext(ctx, "SELECT "+blockColumns+" FROM blocks "+where+" ORDER BY start_min, id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var blocks []domain.Block
	for rows.Next() {
		b, err := scanBlock(rows)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, b)
	}
	return blocks, rows.Err()
}

func scanBlock(row scanner) (domain.Block, error) {
	var (
		b                domain.Block
		day              string
		startMin, durMin int64
	)
	if err := row.Scan(&b.ID, &b.TaskID, &day, &startMin, &durMin); err != nil {
		return domain.Block{}, err
	}
	b.Day = domain.Day(day)
	b.Start = time.Duration(startMin) * time.Minute
	b.Duration = time.Duration(durMin) * time.Minute
	return b, nil
}

func minutes(d time.Duration) int64 { return int64(d / time.Minute) }

func mustAffectBlock(res sql.Result, id int64) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("block #%d: %w", id, ErrNotFound)
	}
	return nil
}
