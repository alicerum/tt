CREATE TABLE pins (
    task_id   INTEGER PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
    pinned_on TEXT    NOT NULL
);

CREATE TABLE blocks (
    id           INTEGER PRIMARY KEY,
    task_id      INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    day          TEXT    NOT NULL,
    start_min    INTEGER NOT NULL CHECK (start_min >= 0),
    duration_min INTEGER NOT NULL CHECK (duration_min > 0)
);

CREATE INDEX blocks_day ON blocks (day, start_min);
CREATE INDEX blocks_task ON blocks (task_id);

CREATE TABLE days (
    day          TEXT PRIMARY KEY,
    started_at   TEXT NOT NULL,
    committed_at TEXT
);
