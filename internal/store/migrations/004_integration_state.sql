-- The last fetch of each integration (D3a): the calendar's busy events live
-- in payload, kept when a later fetch fails.
CREATE TABLE integration_state (
    name            TEXT PRIMARY KEY,
    last_success_at TEXT,
    last_error      TEXT NOT NULL DEFAULT '',
    last_error_at   TEXT,
    payload         TEXT NOT NULL DEFAULT ''
);
