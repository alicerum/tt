-- What the sources report as awaiting me (D20, D50): the full set from the
-- last successful fetch. first_seen_at is kept across fetches.
CREATE TABLE source_items (
    kind          TEXT    NOT NULL,
    external_id   TEXT    NOT NULL,
    number        INTEGER NOT NULL DEFAULT 0,
    title         TEXT    NOT NULL,
    url           TEXT    NOT NULL DEFAULT '',
    author        TEXT    NOT NULL DEFAULT '',
    repo          TEXT    NOT NULL DEFAULT '',
    requested_at  TEXT,
    first_seen_at TEXT    NOT NULL,
    PRIMARY KEY (kind, external_id)
);
