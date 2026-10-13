package store

import (
	"context"
	"database/sql"
	"errors"

	"tt/internal/domain"
)

const sourceItemColumns = "kind, external_id, number, title, url, author, repo, requested_at, first_seen_at"

// SourceItems returns kind's items from the last successful fetch, by
// external id.
func (s *Store) SourceItems(ctx context.Context, kind string) (items []domain.SourceItem, err error) {
	err = s.readTx(ctx, func(tx *sql.Tx) error {
		items, err = querySourceItems(ctx, tx, kind)
		return err
	})
	return items, err
}

func (x *Tx) SourceItems(ctx context.Context, kind string) ([]domain.SourceItem, error) {
	return querySourceItems(ctx, x.tx, kind)
}

// ReplaceSourceItems makes items the whole set of kind: items missing from
// it are deleted (D50).
func (x *Tx) ReplaceSourceItems(ctx context.Context, kind string, items []domain.SourceItem) error {
	if _, err := x.tx.ExecContext(ctx, "DELETE FROM source_items WHERE kind = ?", kind); err != nil {
		return err
	}
	for _, i := range items {
		_, err := x.tx.ExecContext(ctx, "INSERT INTO source_items ("+sourceItemColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			kind, i.ExternalID, i.Number, i.Title, i.URL, i.Author, i.Repo, nullTime(i.RequestedAt), formatTime(i.FirstSeenAt))
		if err != nil {
			return err
		}
	}
	return nil
}

// TaskBySource returns the task materialized from ref; ok is false when
// there is none (the unique index allows one per ref).
func (x *Tx) TaskBySource(ctx context.Context, ref domain.SourceRef) (t domain.Task, ok bool, err error) {
	var id int64
	err = x.tx.QueryRowContext(ctx, "SELECT id FROM tasks WHERE source_kind = ? AND source_id = ?", ref.Kind, ref.ID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Task{}, false, nil
	}
	if err != nil {
		return domain.Task{}, false, err
	}
	t, err = x.GetTask(ctx, id)
	return t, err == nil, err
}

// OpenSourceTasks returns the open tasks materialized from kind's items, by id.
func (x *Tx) OpenSourceTasks(ctx context.Context, kind string) ([]domain.Task, error) {
	rows, err := x.tx.QueryContext(ctx, "SELECT id FROM tasks WHERE source_kind = ? AND status = 'open' ORDER BY id", kind)
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
	tasks := make([]domain.Task, 0, len(ids))
	for _, id := range ids {
		t, err := x.GetTask(ctx, id)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func querySourceItems(ctx context.Context, q querier, kind string) ([]domain.SourceItem, error) {
	rows, err := q.QueryContext(ctx, "SELECT "+sourceItemColumns+" FROM source_items WHERE kind = ? ORDER BY external_id", kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.SourceItem
	for rows.Next() {
		var (
			i           domain.SourceItem
			requestedAt sql.NullString
			firstSeen   string
		)
		if err := rows.Scan(&i.Kind, &i.ExternalID, &i.Number, &i.Title, &i.URL, &i.Author, &i.Repo, &requestedAt, &firstSeen); err != nil {
			return nil, err
		}
		if i.RequestedAt, err = parseNullTime(requestedAt); err != nil {
			return nil, err
		}
		if i.FirstSeenAt, err = parseTime(firstSeen); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}
