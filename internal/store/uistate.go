package store

import (
	"context"
	"database/sql"
	"errors"
)

// UIState returns the remembered value for key; ok is false if none is stored.
func (s *Store) UIState(ctx context.Context, key string) (value string, ok bool, err error) {
	err = s.readTx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT value FROM ui_state WHERE key = ?", key).Scan(&value)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		ok = err == nil
		return err
	})
	return value, ok, err
}

// PutUIState stores value under key, replacing any previous value.
func (x *Tx) PutUIState(ctx context.Context, key, value string) error {
	_, err := x.tx.ExecContext(ctx, `INSERT INTO ui_state (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
