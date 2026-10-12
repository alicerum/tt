CREATE TABLE tasks (
    id             INTEGER PRIMARY KEY,
    title          TEXT    NOT NULL,
    note           TEXT    NOT NULL DEFAULT '',
    link           TEXT    NOT NULL DEFAULT '',
    requester      TEXT    NOT NULL DEFAULT '',
    priority       INTEGER,
    size           TEXT,
    parent_id      INTEGER REFERENCES tasks(id) ON DELETE SET NULL,
    position       INTEGER NOT NULL DEFAULT 0,
    status         TEXT    NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'done', 'dropped')),
    drop_reason    TEXT    NOT NULL DEFAULT '',
    source_kind    TEXT,
    source_id      TEXT,
    rollover_count INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT    NOT NULL,
    updated_at     TEXT    NOT NULL,
    closed_at      TEXT,
    closed_by      TEXT
);

CREATE UNIQUE INDEX tasks_source ON tasks (source_kind, source_id) WHERE source_kind IS NOT NULL;
CREATE INDEX tasks_parent ON tasks (parent_id);

CREATE TABLE task_tags (
    task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    tag     TEXT    NOT NULL,
    PRIMARY KEY (task_id, tag)
);
