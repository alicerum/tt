package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Integration is the last fetch of one integration (D3a): when it last
// worked, the last error ("" after a success), and its cached data, which a
// failed fetch leaves alone.
type Integration struct {
	Name          string
	LastSuccessAt time.Time // zero: never
	LastError     string
	LastErrorAt   time.Time
	Payload       string
}

type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Integration returns name's stored state; ok is false before its first fetch.
func (s *Store) Integration(ctx context.Context, name string) (in Integration, ok bool, err error) {
	err = s.readTx(ctx, func(tx *sql.Tx) error {
		in, ok, err = getIntegration(ctx, tx, name)
		return err
	})
	return in, ok, err
}

func (x *Tx) GetIntegration(ctx context.Context, name string) (Integration, bool, error) {
	return getIntegration(ctx, x.tx, name)
}

// PutIntegration stores in, replacing name's previous state.
func (x *Tx) PutIntegration(ctx context.Context, in Integration) error {
	_, err := x.tx.ExecContext(ctx, `INSERT INTO integration_state
		(name, last_success_at, last_error, last_error_at, payload) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (name) DO UPDATE SET last_success_at = excluded.last_success_at,
			last_error = excluded.last_error, last_error_at = excluded.last_error_at, payload = excluded.payload`,
		in.Name, nullTime(in.LastSuccessAt), in.LastError, nullTime(in.LastErrorAt), in.Payload)
	return err
}

func getIntegration(ctx context.Context, q rowQuerier, name string) (Integration, bool, error) {
	in := Integration{Name: name}
	var success, errorAt sql.NullString
	err := q.QueryRowContext(ctx, `SELECT last_success_at, last_error, last_error_at, payload
		FROM integration_state WHERE name = ?`, name).Scan(&success, &in.LastError, &errorAt, &in.Payload)
	if errors.Is(err, sql.ErrNoRows) {
		return in, false, nil
	}
	if err != nil {
		return Integration{}, false, err
	}
	if in.LastSuccessAt, err = parseNullTime(success); err != nil {
		return Integration{}, false, err
	}
	if in.LastErrorAt, err = parseNullTime(errorAt); err != nil {
		return Integration{}, false, err
	}
	return in, true, nil
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return formatTime(t)
}

func parseNullTime(s sql.NullString) (time.Time, error) {
	if !s.Valid {
		return time.Time{}, nil
	}
	return parseTime(s.String)
}
