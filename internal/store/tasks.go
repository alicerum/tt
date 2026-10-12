package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"tt/internal/domain"
)

// Tx is a write transaction. Get one from Store.InTx.
type Tx struct{ tx *sql.Tx }

const taskColumns = `id, title, note, link, requester, priority, size, parent_id, position, status,
	drop_reason, source_kind, source_id, rollover_count, created_at, updated_at, closed_at, closed_by`

// InsertTask inserts t (ignoring t.ID) with its tags and returns the new id.
func (x *Tx) InsertTask(ctx context.Context, t domain.Task) (int64, error) {
	res, err := x.tx.ExecContext(ctx, `INSERT INTO tasks (`+strings.TrimPrefix(taskColumns, "id, ")+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, taskArgs(t)...)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, x.setTags(ctx, id, t.Tags)
}

// UpdateTask overwrites every column and the tag set of task t.ID.
func (x *Tx) UpdateTask(ctx context.Context, t domain.Task) error {
	res, err := x.tx.ExecContext(ctx, `UPDATE tasks SET title = ?, note = ?, link = ?, requester = ?,
		priority = ?, size = ?, parent_id = ?, position = ?, status = ?, drop_reason = ?, source_kind = ?,
		source_id = ?, rollover_count = ?, created_at = ?, updated_at = ?, closed_at = ?, closed_by = ?
		WHERE id = ?`, append(taskArgs(t), t.ID)...)
	if err != nil {
		return err
	}
	if err := mustAffect(res, t.ID); err != nil {
		return err
	}
	return x.setTags(ctx, t.ID, t.Tags)
}

func (x *Tx) GetTask(ctx context.Context, id int64) (domain.Task, error) {
	t, err := scanTask(x.tx.QueryRowContext(ctx, "SELECT "+taskColumns+" FROM tasks WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Task{}, fmt.Errorf("task #%d: %w", id, ErrNotFound)
	}
	if err != nil {
		return domain.Task{}, err
	}
	rows, err := x.tx.QueryContext(ctx, "SELECT tag FROM task_tags WHERE task_id = ? ORDER BY tag", id)
	if err != nil {
		return domain.Task{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return domain.Task{}, err
		}
		t.Tags = append(t.Tags, tag)
	}
	return t, rows.Err()
}

// DeleteTask hard-deletes a task. Its tags go with it; its children become top-level.
func (x *Tx) DeleteTask(ctx context.Context, id int64) error {
	res, err := x.tx.ExecContext(ctx, "DELETE FROM tasks WHERE id = ?", id)
	if err != nil {
		return err
	}
	return mustAffect(res, id)
}

// TouchChildren sets updated_at on parentID's children, so undos recorded
// against them are refused once the parent changes or goes away.
func (x *Tx) TouchChildren(ctx context.Context, parentID int64, now time.Time) error {
	_, err := x.tx.ExecContext(ctx, "UPDATE tasks SET updated_at = ? WHERE parent_id = ?", formatTime(now), parentID)
	return err
}

func (x *Tx) HasChildren(ctx context.Context, id int64) (bool, error) {
	var n int
	err := x.tx.QueryRowContext(ctx, "SELECT count(*) FROM tasks WHERE parent_id = ?", id).Scan(&n)
	return n > 0, err
}

// Children returns parentID's children in sibling order (position, then id).
func (x *Tx) Children(ctx context.Context, parentID int64) ([]domain.Task, error) {
	rows, err := x.tx.QueryContext(ctx, "SELECT id FROM tasks WHERE parent_id = ? ORDER BY position, id", parentID)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	kids := make([]domain.Task, 0, len(ids))
	for _, id := range ids {
		t, err := x.GetTask(ctx, id)
		if err != nil {
			return nil, err
		}
		kids = append(kids, t)
	}
	return kids, nil
}

// NextChildPosition returns the position after parentID's last child.
func (x *Tx) NextChildPosition(ctx context.Context, parentID int64) (int, error) {
	var n int
	err := x.tx.QueryRowContext(ctx, "SELECT coalesce(max(position) + 1, 0) FROM tasks WHERE parent_id = ?", parentID).Scan(&n)
	return n, err
}

func (x *Tx) setTags(ctx context.Context, id int64, tags []string) error {
	if _, err := x.tx.ExecContext(ctx, "DELETE FROM task_tags WHERE task_id = ?", id); err != nil {
		return err
	}
	for _, tag := range tags {
		if _, err := x.tx.ExecContext(ctx, "INSERT INTO task_tags (task_id, tag) VALUES (?, ?)", id, tag); err != nil {
			return err
		}
	}
	return nil
}

// LoadSnapshot reads every task with its tags in one consistent read.
func (s *Store) LoadSnapshot(ctx context.Context) (domain.Snapshot, error) {
	var snap domain.Snapshot
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		tasks, err := loadTasks(ctx, tx)
		if err != nil {
			return err
		}
		snap = domain.NewSnapshot(tasks)
		return nil
	})
	return snap, err
}

func loadTasks(ctx context.Context, tx *sql.Tx) ([]domain.Task, error) {
	rows, err := tx.QueryContext(ctx, "SELECT "+taskColumns+" FROM tasks ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []domain.Task
	index := map[int64]int{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		index[t.ID] = len(tasks)
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	tagRows, err := tx.QueryContext(ctx, "SELECT task_id, tag FROM task_tags ORDER BY task_id, tag")
	if err != nil {
		return nil, err
	}
	defer tagRows.Close()
	for tagRows.Next() {
		var id int64
		var tag string
		if err := tagRows.Scan(&id, &tag); err != nil {
			return nil, err
		}
		if i, ok := index[id]; ok {
			tasks[i].Tags = append(tasks[i].Tags, tag)
		}
	}
	return tasks, tagRows.Err()
}

func taskArgs(t domain.Task) []any {
	var sourceKind, sourceID any
	if t.Source != nil {
		sourceKind, sourceID = t.Source.Kind, t.Source.ID
	}
	var closedAt any
	if !t.ClosedAt.IsZero() {
		closedAt = formatTime(t.ClosedAt)
	}
	return []any{t.Title, t.Note, t.Link, t.Requester, nullIfZero(t.Priority), nullIfZero(string(t.Size)),
		nullIfZero(t.ParentID), t.Position, string(t.Status), t.DropReason, sourceKind, sourceID,
		t.RolloverCount, formatTime(t.CreatedAt), formatTime(t.UpdatedAt), closedAt, nullIfZero(t.ClosedBy)}
}

type scanner interface{ Scan(dest ...any) error }

func scanTask(row scanner) (domain.Task, error) {
	var (
		t                                    domain.Task
		priority, parentID                   sql.NullInt64
		size, sourceKind, sourceID, closedBy sql.NullString
		status, createdAt, updatedAt         string
		closedAt                             sql.NullString
	)
	err := row.Scan(&t.ID, &t.Title, &t.Note, &t.Link, &t.Requester, &priority, &size, &parentID,
		&t.Position, &status, &t.DropReason, &sourceKind, &sourceID, &t.RolloverCount,
		&createdAt, &updatedAt, &closedAt, &closedBy)
	if err != nil {
		return domain.Task{}, err
	}
	t.Priority = int(priority.Int64)
	t.Size = domain.Size(size.String)
	t.ParentID = parentID.Int64
	t.Status = domain.Status(status)
	t.ClosedBy = closedBy.String
	if sourceKind.Valid {
		t.Source = &domain.SourceRef{Kind: sourceKind.String, ID: sourceID.String}
	}
	if t.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Task{}, err
	}
	if t.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.Task{}, err
	}
	if closedAt.Valid {
		if t.ClosedAt, err = parseTime(closedAt.String); err != nil {
			return domain.Task{}, err
		}
	}
	return t, nil
}

func nullIfZero[T comparable](v T) any {
	var zero T
	if v == zero {
		return nil
	}
	return v
}

func mustAffect(res sql.Result, id int64) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("task #%d: %w", id, ErrNotFound)
	}
	return nil
}
