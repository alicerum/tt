-- Impact and due replace priority (D68, D69, D76). No task gets an impact:
-- open top-level tasks go back to the Inbox for one re-triage. Pinned
-- reviews never reach the Inbox, so open source-linked tasks get impact 1
-- and are due 3 days after the request (D75's defaults; the request's UTC
-- date, a one-off). priority stays, unused.
ALTER TABLE tasks ADD COLUMN impact INTEGER;
ALTER TABLE tasks ADD COLUMN due TEXT;

UPDATE tasks SET
    impact = 1,
    due = (SELECT date(coalesce(i.requested_at, i.first_seen_at), '+3 days')
           FROM source_items i
           WHERE i.kind = tasks.source_kind AND i.external_id = tasks.source_id)
WHERE status = 'open' AND source_kind IS NOT NULL;
