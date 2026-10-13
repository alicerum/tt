# tt Plan 6 — GitHub Reviews Lane Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** PRs that request my review directly show in a Reviews lane on Today and in the Log; `w` focuses it, `p` turns a request into a P2 S candidate task (`Review #n: <title>`), `o` opens the PR; tt polls GitHub through `gh` every 5 minutes and closes a review's task by itself once I'm no longer requested ("✓ via GitHub"), and the header says when GitHub is stale or broken and what to do.

**Architecture:** `internal/source` runs one `gh api graphql` search per poll and maps failures to short reasons (D50). `app.SyncSources` records a failed fetch, and only after a successful one replaces `source_items` (migration 005) and auto-closes linked tasks whose PR is gone, in one transaction. `app.PinSource` creates or reopens a review's task. `Load` returns the lane (requests with no open task) and the fetch status. The TUI copies the calendar's sequenced poll loop and adds the lane as a small `laneState` beside the existing panes, sharing their height budget.

**Tech Stack:** Go 1.27; Bubble Tea v2, Lip Gloss v2; pure-Go SQLite; the `gh` CLI (2.x) at run time. No new Go dependencies.

**Spec:** `docs/superpowers/specs/2026-10-12-task-tracking-design.md` — D3, D3a, D4, D20, D20a, D20b, D44 (wording style), D45 (consumer-side interface), D48, D49, D50, Data (`source_items`), Runtime (GitHub loop), Adapters (GitHub), Error handling, "Plan 6 — GitHub reviews lane", "Carry-forward from Plan 2 review" (auto-close sets `UpdatedAt = now`).

## Global Constraints

- No new Go dependencies: `go.mod` and `go.sum` stay unchanged.
- Migration 005 only adds `source_items`; earlier migrations are untouched.
- Only `app` mutates state. `app` defines `Source` and never imports `internal/source` outside tests; adapters never import `app`.
- Reconcile only after a successful fetch (spec: Error handling); a fetch with more than 100 results is a failed fetch. Auto-close sets `closed_at = updated_at = now` and `closed_by = source`, never reopens anything, and leaves pins and blocks alone.
- D50 wording, exactly: `gh not found · brew install gh`, `not logged in · gh auth login`, `over 100 review requests`, `gh timed out after 20s`, `⚠ gh: <reason>`, dim `gh 12m ago` once the last success is older than two polls; nothing in the header while fresh or with `enabled = false`.
- D49 wording, exactly: `REVIEWS · 4 waiting`, `REVIEWS · none waiting`, `REVIEWS · not fetched yet`, `pinned "Review #901: …" · P2 S`, `a review request · p pins it, o opens it`, `no reviews waiting`, `✓ via GitHub: "Review #877: …"` / `✓ via GitHub: 2 reviews`, `w reviews` in the hints.
- D20b/D48: a created review task is `Review #<n>: <PR title>`, link = PR URL, priority and size from `[github]`, requester empty (the PR author is never copied).
- Tests never reach GitHub: the adapter tests run fake `gh` scripts, app/TUI tests use `source.Fake`, and `cmd/tt`'s test empties `PATH`. Never run tt or `gh` against real GitHub from a test.
- Lines never wrap: every rendered line is truncated to the terminal width, and a screen's body never exceeds `bodyHeight()`. Text from GitHub goes through `oneLine` before rendering.
- Every task ends green on: `gofmt -l .` (no output), `go vet ./...`, `go test ./...`; Tasks 3–6 also `go test -race` on `./internal/app/` (and `./internal/tui/` from Task 5).
- Every commit message ends with a blank line and `Co-Authored-By: Claude <noreply@anthropic.com>`. Commit messages use single quotes.
- Sandbox: save each patch to a file and `git apply <file>`; run plain, separate git commands (no `git -C`, no `cd … &&`). If `git apply` fails, stop: an earlier step went wrong.

## Review Focus

1. **GitHub breaks while tt is open** (SSO session expired, token revoked, offline laptop, gh uninstalled, a 20s hang): the lane keeps the last good requests, no review task closes, and the header says what to do. Pinned in Task 2 `TestFetchFailures`, `TestFetchTimesOut`, `TestFetchFindsGH`, Task 3 `TestNoReconcileOnAFailedFetch`, `TestSyncSourcesStopsWhenQuitting`, Task 5 `TestGitHubHeaderStatus`.
2. **A truncated result** (more than 100 direct requests, or a duplicate PR in one answer): never mass-closes tasks; duplicates collapse to one row. Pinned in Task 2 `TestFetchFailures` ("too many", "exactly 100 is fine"), Task 3 `TestNoReconcileOnAFailedFetch`, `TestSyncSourcesFillsTheLane`.
3. **Undo after GitHub or I changed the task** (ctrl+z of an edit made before an auto-close; of a `p` after the review was blocked or resized): the undo refuses with "it changed since" and nothing is damaged. Pinned in Task 3 `TestSyncSourcesAutoCloses`, Task 4 `TestPinSourceUndoRefusesAfterChanges`.
4. **The same PR coming back** (removed then re-requested; marked done or dropped by hand while GitHub still asks; already blocked when it auto-closed): it is back in the lane, and `p` reopens the same task with its history, keeping its own priority/size; the reopen's undo works with the old block in place. Pinned in Task 4 `TestPinSourceReopensItsTask`, Task 6 `TestReviewsLaneInTheLog`.
5. **Real PR data on a real terminal** (titles of 200 characters with tabs, newlines or escape codes; a deleted author; 7+ requests; a 12-line tmux pane): rows stay one line with author and wait visible, the lane scrolls with its cursor, gives up rows before the candidates do and disappears when there's no room. Pinned in Task 2 `TestFetchRunsGH` (ghost author), Task 6 `TestReviewRowsFitTheirColumn`, `TestReviewsLaneHeight`.

## File Structure

```
internal/domain/source.go                     new: SourceItem, Ref, WaitingSince, ReviewTitle
internal/store/migrations/005_source_items.sql new
internal/store/source.go                      new: SourceItems, ReplaceSourceItems, TaskBySource, OpenSourceTasks
internal/source/github.go, fake.go            new: the gh adapter (D50 errors), a fake
internal/config/config.go, config.example.toml modify: [github] enabled, host, poll, priority, size
internal/app/sources.go                       new: Source, WithSource, SyncSources, PinSource, GitHubState
internal/app/service.go, day.go               modify: Store.SourceItems, Service.src, State.GitHub in Load
cmd/tt/main.go                                modify: source.GitHub unless [github] enabled = false
internal/tui/model.go                         modify: pollLoop, gh loop + messages, syncGitHub, R, lane field
internal/tui/common.go, overlays.go, detail.go modify: githubStatus, badgeOf, hints, help, ✓ via GitHub
internal/tui/lane.go                          new: the Reviews lane (state, height, view, keys)
internal/tui/pick.go, today.go, edit.go       modify: lane in both screens, paneVisible, w, openURL
internal/tui/*_test.go                        new/modify: harness with a fake GitHub, poll loop, header, lane
```

Work happens on branch `plan-6` in a worktree; fast-forward merge to `main` when the final review is green.

---
### Task 1: Source items: `domain.SourceItem`, migration 005, store lookups

The data side of D20/D50: `domain.SourceItem` (a review request; `WaitingSince` is the request time, else when tt first saw it; `ReviewTitle` is D20b's `Review #n: <title>`), migration 005 `source_items` (the spec's Data row, trimmed to what is read), and the store calls the app needs: read and replace a kind's set, find the task materialized from a ref, list a kind's open linked tasks.

**Files:**
- Create: `internal/domain/source.go`
- Create: `internal/store/migrations/005_source_items.sql`
- Create: `internal/store/source.go`
- Create: `internal/domain/source_test.go`
- Create: `internal/store/source_test.go`

**Interfaces:**
- Consumes: `store.Tx`, `querier`, `nullTime`/`parseNullTime`/`formatTime`/`parseTime` (existing).
- Produces:
  - `domain.SourceGitHub = "github"`; `domain.SourceItem{Kind, ExternalID string; Number int; Title, URL, Author, Repo string; RequestedAt, FirstSeenAt time.Time}` (`ExternalID` is `owner/repo#123`; `RequestedAt` zero = unknown).
  - `func (SourceItem) Ref() domain.SourceRef`, `func (SourceItem) WaitingSince() time.Time`, `func domain.ReviewTitle(SourceItem) string`.
  - `func (*store.Store) SourceItems(ctx, kind string) ([]domain.SourceItem, error)` and `func (*store.Tx) SourceItems(ctx, kind string)` (by external id); `func (*store.Tx) ReplaceSourceItems(ctx, kind string, items []domain.SourceItem) error` (the whole set; stores `kind`, not `items[i].Kind`).
  - `func (*store.Tx) TaskBySource(ctx, ref domain.SourceRef) (domain.Task, bool, error)` (open or closed); `func (*store.Tx) OpenSourceTasks(ctx, kind string) ([]domain.Task, error)` (by id).

- [ ] **Step 1: Write the failing tests**

Create `internal/domain/source_test.go`:

```go
package domain

import (
	"testing"
	"time"
)

func TestSourceItem(t *testing.T) {
	seen := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	i := SourceItem{Kind: SourceGitHub, ExternalID: "acme/api#901", Number: 901, Title: "Add idempotency key", FirstSeenAt: seen}
	if got := i.WaitingSince(); !got.Equal(seen) {
		t.Errorf("without a request time, waiting since first seen: %v", got)
	}
	i.RequestedAt = seen.Add(-48 * time.Hour)
	if got := i.WaitingSince(); !got.Equal(i.RequestedAt) {
		t.Errorf("waiting since the request: %v", got)
	}
	if got := ReviewTitle(i); got != "Review #901: Add idempotency key" {
		t.Errorf("ReviewTitle = %q", got)
	}
	if got := i.Ref(); got != (SourceRef{Kind: "github", ID: "acme/api#901"}) {
		t.Errorf("Ref = %+v", got)
	}
}
```

Create `internal/store/source_test.go`:

```go
package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"tt/internal/domain"
)

func TestSourceItems(t *testing.T) {
	ctx := context.Background()
	s, path := openTemp(t)
	if items, err := s.SourceItems(ctx, "github"); err != nil || len(items) != 0 {
		t.Fatalf("before the first fetch: %v, %v", items, err)
	}
	seen := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	first := []domain.SourceItem{
		{Kind: "github", ExternalID: "acme/photos#877", Number: 877, Title: "Bump go", URL: "https://github.com/acme/photos/pull/877",
			Author: "tom", Repo: "acme/photos", FirstSeenAt: seen}, // no request time
		{Kind: "github", ExternalID: "acme/api#901", Number: 901, Title: "Add idempotency key", URL: "https://github.com/acme/api/pull/901",
			Author: "ana", Repo: "acme/api", RequestedAt: seen.Add(-72 * time.Hour), FirstSeenAt: seen},
	}
	inTx(t, s, func(ctx context.Context, x *Tx) error { return x.ReplaceSourceItems(ctx, "github", first) })
	inTx(t, s, func(ctx context.Context, x *Tx) error { // another kind's set is its own
		return x.ReplaceSourceItems(ctx, "other", []domain.SourceItem{{ExternalID: "x", Title: "x", FirstSeenAt: seen}})
	})
	s.Close()

	s, err := Open(ctx, path) // kept across restarts
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	items, err := s.SourceItems(ctx, "github")
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.SourceItem{first[1], first[0]} // by external id
	if diff := cmp.Diff(want, items); diff != "" {
		t.Errorf("stored items (-want +got):\n%s", diff)
	}

	inTx(t, s, func(ctx context.Context, x *Tx) error { // the next fetch replaces the set
		return x.ReplaceSourceItems(ctx, "github", first[:1])
	})
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		items, err := x.SourceItems(ctx, "github")
		if err != nil {
			return err
		}
		if len(items) != 1 || items[0].ExternalID != "acme/photos#877" || !items[0].RequestedAt.IsZero() {
			t.Errorf("after the replace: %+v", items)
		}
		other, err := x.SourceItems(ctx, "other")
		if err != nil {
			return err
		}
		if len(other) != 1 {
			t.Errorf("the other kind's items: %+v", other)
		}
		return nil
	})
}

func TestTasksBySource(t *testing.T) {
	s, _ := openTemp(t)
	plain := insert(t, s, sampleTask())
	review := sampleTask()
	review.Source = &domain.SourceRef{Kind: "github", ID: "acme/api#901"}
	reviewID := insert(t, s, review)
	closed := sampleTask()
	closed.Source = &domain.SourceRef{Kind: "github", ID: "acme/api#3"}
	closed.Status, closed.ClosedAt, closed.ClosedBy = domain.StatusDone, closed.CreatedAt, domain.ClosedBySource
	closedID := insert(t, s, closed)
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		got, ok, err := x.TaskBySource(ctx, domain.SourceRef{Kind: "github", ID: "acme/api#901"})
		if err != nil || !ok || got.ID != reviewID || len(got.Tags) != 2 {
			t.Errorf("TaskBySource = %+v, %v, %v; want #%d with its tags", got, ok, err, reviewID)
		}
		if got, ok, err := x.TaskBySource(ctx, domain.SourceRef{Kind: "github", ID: "acme/api#3"}); err != nil || !ok || got.ID != closedID {
			t.Errorf("a closed task is found too: %+v, %v, %v", got, ok, err)
		}
		if _, ok, err := x.TaskBySource(ctx, domain.SourceRef{Kind: "github", ID: "acme/api#902"}); err != nil || ok {
			t.Errorf("no task for #902: %v, %v", ok, err)
		}
		open, err := x.OpenSourceTasks(ctx, "github")
		if err != nil {
			return err
		}
		if len(open) != 1 || open[0].ID != reviewID {
			t.Errorf("OpenSourceTasks = %+v; want only #%d (not #%d plain, not #%d closed)", open, reviewID, plain, closedID)
		}
		return nil
	})
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/domain/ ./internal/store/`
Expected: FAIL (build): `undefined: SourceItem`, `undefined: SourceGitHub`, `undefined: ReviewTitle`, `s.SourceItems undefined`, `x.ReplaceSourceItems undefined`.

- [ ] **Step 3: Implement**

Create `internal/domain/source.go`:

```go
package domain

import (
	"fmt"
	"time"
)

// SourceGitHub is the source kind of GitHub review requests (D20, D50).
const SourceGitHub = "github"

// SourceItem is a read-only item a source reports as awaiting me (D20): for
// GitHub, an open pull request that requests my review. Pinning one
// materializes a task that links back to it.
type SourceItem struct {
	Kind        string // e.g. SourceGitHub
	ExternalID  string // unique per kind: "owner/repo#123"
	Number      int
	Title       string
	URL         string
	Author      string    // shown in the Reviews lane, never copied into a requester (D20b)
	Repo        string    // "owner/repo"
	RequestedAt time.Time // when I was (last) requested; zero when the source doesn't say
	FirstSeenAt time.Time // when tt first saw it (kept across fetches)
}

// Ref is the source ref of a task materialized from i.
func (i SourceItem) Ref() SourceRef { return SourceRef{Kind: i.Kind, ID: i.ExternalID} }

// WaitingSince is how long i has waited on me: since RequestedAt, else
// since tt first saw it.
func (i SourceItem) WaitingSince() time.Time {
	if !i.RequestedAt.IsZero() {
		return i.RequestedAt
	}
	return i.FirstSeenAt
}

// ReviewTitle is the title of the task materialized from a review request (D20b).
func ReviewTitle(i SourceItem) string { return fmt.Sprintf("Review #%d: %s", i.Number, i.Title) }
```

Create `internal/store/migrations/005_source_items.sql`:

```sql
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
```

Create `internal/store/source.go`:

```go
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/domain/ ./internal/store/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS (`TestNewerSchemaIsRefused` already finds the newest migration itself), no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/source.go internal/domain/source_test.go internal/store/migrations/005_source_items.sql internal/store/source.go internal/store/source_test.go
git commit -m 'feat(store): source items (migration 005) and source-linked task lookups

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 2: The `gh` adapter (`internal/source`) and a fake

D50's adapter: one `gh api graphql` run (20s timeout; `--hostname` for Enterprise) with the search `is:open is:pr user-review-requested:@me archived:false`, first 100. `RequestedAt` is the newest `ReviewRequestedEvent` that names the viewer. More than 100 results fail the fetch, so a truncated set can never auto-close anything. Failures map to short reasons that say what to do. Tests drive fake `gh` scripts; nothing reaches GitHub.

**Files:**
- Create: `internal/source/fake.go`
- Create: `internal/source/github.go`
- Create: `internal/source/github_test.go`

**Interfaces:**
- Consumes: Task 1 (`domain.SourceItem`, `domain.SourceGitHub`).
- Produces:
  - `source.GitHub{Host, Path string; Timeout time.Duration}` (`Path` "" = `gh` on `PATH`; `Timeout` 0 = `source.DefaultTimeout`, 20s); `func (*GitHub) Kind() string` (`"github"`); `func (*GitHub) Fetch(ctx) ([]domain.SourceItem, error)` (`FirstSeenAt` left zero: the app sets it; a deleted author is `ghost`).
  - Errors: `source.ErrGHNotFound` (`gh not found · brew install gh`), `source.ErrNotLoggedIn` (`not logged in · gh auth login`, gh exit 4), `source.ErrTooMany` (`over 100 review requests`), `gh timed out after 20s`, `gh printed bad JSON: …`, a GraphQL error's message, else the first line of gh's stderr without its `gh: ` prefix, else `gh failed: exit status N`; a cancelled ctx returns `ctx.Err()`. `source.MaxItems = 100`.
  - `source.Fake` (mutex-guarded): `Kind()`, `Set(items []domain.SourceItem, err error)`, `Fetch(ctx)`, `Calls() int`.

- [ ] **Step 1: Write the failing tests**

Create `internal/source/github_test.go`:

```go
package source

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"tt/internal/domain"
)

// fakeGH writes a fake gh script into dir and returns its path.
func fakeGH(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "gh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

const answer = `{"data":{"viewer":{"login":"Me"},"search":{"issueCount":3,"nodes":[
 {"number":901,"title":"Add idempotency key","url":"https://github.com/acme/api/pull/901","author":{"login":"ana"},
  "repository":{"nameWithOwner":"acme/api"},"timelineItems":{"nodes":[
   {"createdAt":"2026-10-09T08:00:00Z","requestedReviewer":{"login":"me"}},
   {"createdAt":"2026-10-10T08:00:00Z","requestedReviewer":{"login":"me"}},
   {"createdAt":"2026-10-11T08:00:00Z","requestedReviewer":{"login":"tom"}},
   {"createdAt":"2026-10-11T09:00:00Z","requestedReviewer":null}]}},
 {},
 {"number":877,"title":"Bump go","url":"https://github.com/acme/photos/pull/877","author":null,
  "repository":{"nameWithOwner":"acme/photos"},"timelineItems":{"nodes":[]}}]}}}`

func TestFetchRunsGH(t *testing.T) {
	dir := t.TempDir()
	path := fakeGH(t, dir, `for a in "$@"; do echo "$a"; done > "$(dirname "$0")/args"
cat <<'JSON'
`+answer+`
JSON`)
	items, err := (&GitHub{Path: path}).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.SourceItem{
		{Kind: "github", ExternalID: "acme/api#901", Number: 901, Title: "Add idempotency key", URL: "https://github.com/acme/api/pull/901",
			Author: "ana", Repo: "acme/api", RequestedAt: time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)}, // my latest request, not tom's
		{Kind: "github", ExternalID: "acme/photos#877", Number: 877, Title: "Bump go", URL: "https://github.com/acme/photos/pull/877",
			Author: "ghost", Repo: "acme/photos"}, // a deleted author; no request event
	}
	if diff := cmp.Diff(want, items); diff != "" {
		t.Errorf("items (-want +got):\n%s", diff)
	}
	args, err := os.ReadFile(filepath.Join(dir, "args"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitN(string(args), "\n", 4)
	if lines[0] != "api" || lines[1] != "graphql" || lines[2] != "-f" || !strings.HasPrefix(lines[3], "query=query {") ||
		!strings.Contains(lines[3], `user-review-requested:@me archived:false`) {
		t.Errorf("args:\n%s", args)
	}

	if _, err := (&GitHub{Path: path, Host: "github.acme.com"}).Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	args, _ = os.ReadFile(filepath.Join(dir, "args"))
	if !strings.HasPrefix(string(args), "api\ngraphql\n--hostname\ngithub.acme.com\n-f\nquery=") {
		t.Errorf("an Enterprise host goes to gh:\n%s", args)
	}
	if k := (&GitHub{}).Kind(); k != domain.SourceGitHub {
		t.Errorf("Kind = %q", k)
	}
}

func TestFetchFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       string
	}{
		{"not logged in", "echo 'To get started with GitHub CLI, please run:  gh auth login' >&2; exit 4", ErrNotLoggedIn.Error()},
		{"bad token", "echo 'gh: Bad credentials (HTTP 401)' >&2; exit 1", "Bad credentials (HTTP 401)"},
		{"silent", "exit 3", "gh failed: exit status 3"},
		{"bad JSON", "echo '{not json'", "gh printed bad JSON: "},
		{"GraphQL error", `echo '{"errors":[{"message":"Resource protected by organization SAML enforcement."}]}'`,
			"Resource protected by organization SAML enforcement."},
		{"too many", `echo '{"data":{"viewer":{"login":"me"},"search":{"issueCount":101,"nodes":[]}}}'`, "over 100 review requests"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items, err := (&GitHub{Path: fakeGH(t, t.TempDir(), tc.body)}).Fetch(context.Background())
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) || items != nil {
				t.Errorf("items %v, err = %v; want %q", items, err, tc.want)
			}
		})
	}
	ok := `echo '{"data":{"viewer":{"login":"me"},"search":{"issueCount":100,"nodes":[]}}}'`
	if items, err := (&GitHub{Path: fakeGH(t, t.TempDir(), ok)}).Fetch(context.Background()); err != nil || len(items) != 0 {
		t.Errorf("exactly 100 is fine: %v, %v", items, err)
	}
}

func TestFetchTimesOut(t *testing.T) {
	g := &GitHub{Path: fakeGH(t, t.TempDir(), "sleep 10"), Timeout: 100 * time.Millisecond}
	began := time.Now()
	_, err := g.Fetch(context.Background())
	if err == nil || err.Error() != "gh timed out after 100ms" {
		t.Errorf("err = %v; want the timeout", err)
	}
	if took := time.Since(began); took > 3*time.Second {
		t.Errorf("Fetch took %v after a timeout", took)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.Fetch(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled context: err = %v; want context.Canceled", err)
	}
}

func TestFetchFindsGH(t *testing.T) {
	if _, err := (&GitHub{Path: filepath.Join(t.TempDir(), "gh")}).Fetch(context.Background()); !errors.Is(err, ErrGHNotFound) {
		t.Errorf("missing gh: err = %v; want ErrGHNotFound", err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := (&GitHub{}).Fetch(context.Background()); !errors.Is(err, ErrGHNotFound) {
		t.Errorf("not on PATH: err = %v; want ErrGHNotFound", err)
	}
	onPath := t.TempDir()
	fakeGH(t, onPath, `echo '{"data":{"viewer":{"login":"me"},"search":{"issueCount":0,"nodes":[]}}}'`)
	t.Setenv("PATH", onPath+string(os.PathListSeparator)+"/bin:/usr/bin")
	if items, err := (&GitHub{}).Fetch(context.Background()); err != nil || len(items) != 0 {
		t.Errorf("on PATH: %v, %v; want no items and no error", items, err)
	}
}

func TestFake(t *testing.T) {
	var f Fake
	f.Set([]domain.SourceItem{{ExternalID: "a#1"}}, nil)
	items, err := f.Fetch(context.Background())
	if err != nil || len(items) != 1 || f.Calls() != 1 || f.Kind() != domain.SourceGitHub {
		t.Errorf("Fetch = %v, %v; calls %d", items, err, f.Calls())
	}
	f.Set(nil, errors.New("down"))
	if _, err := f.Fetch(context.Background()); err == nil || f.Calls() != 2 {
		t.Errorf("Set's error comes back: %v, calls %d", err, f.Calls())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/source/`
Expected: FAIL (build): `undefined: GitHub`, `undefined: ErrNotLoggedIn`, `undefined: ErrGHNotFound` (no non-test files yet).

- [ ] **Step 3: Implement**

Create `internal/source/fake.go`:

```go
package source

import (
	"context"
	"sync"

	"tt/internal/domain"
)

// Fake is a GitHub for tests: it returns what Set gave it. Safe for
// concurrent use (the TUI fetches in the background).
type Fake struct {
	mu    sync.Mutex
	items []domain.SourceItem
	err   error
	calls int
}

func (f *Fake) Kind() string { return domain.SourceGitHub }

// Set makes the next fetches return items, or err when it isn't nil.
func (f *Fake) Set(items []domain.SourceItem, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items, f.err = append([]domain.SourceItem(nil), items...), err
}

// Fetch returns a copy of what Set gave it (or its error) and counts the call.
func (f *Fake) Fetch(context.Context) ([]domain.SourceItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return append([]domain.SourceItem(nil), f.items...), nil
}

// Calls is how many fetches there were.
func (f *Fake) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}
```

Create `internal/source/github.go`:

```go
// Package source fetches what is awaiting me from outside tt (D20): GitHub
// review requests, through the gh CLI (spec: Adapters, D50). It has no rules
// of its own: app reconciles the result.
package source

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"
	"time"

	"tt/internal/domain"
)

// The failures with a fix of their own (D50); their text is what the header shows.
var (
	ErrGHNotFound  = errors.New("gh not found · brew install gh")
	ErrNotLoggedIn = errors.New("not logged in · gh auth login")
	ErrTooMany     = fmt.Errorf("over %d review requests", MaxItems)
)

// DefaultTimeout bounds one gh run (D50).
const DefaultTimeout = 20 * time.Second

// MaxItems is how many review requests one fetch can return. A larger set
// fails the fetch (ErrTooMany): reconciling a truncated set would
// auto-close the requests left out.
const MaxItems = 100

// query is the one search a fetch runs: open PRs that request my review
// directly (team requests aren't included), with the latest review-request
// events for the time I was asked.
const query = `query {
  viewer { login }
  search(type: ISSUE, first: 100, query: "is:open is:pr user-review-requested:@me archived:false") {
    issueCount
    nodes {
      ... on PullRequest {
        number title url
        author { login }
        repository { nameWithOwner }
        timelineItems(itemTypes: [REVIEW_REQUESTED_EVENT], last: 10) {
          nodes { ... on ReviewRequestedEvent { createdAt requestedReviewer { ... on User { login } } } }
        }
      }
    }
  }
}`

// GitHub runs `gh api graphql` for the review requests awaiting me (D50).
type GitHub struct {
	Host    string        // GitHub Enterprise host; "" is gh's default (github.com)
	Path    string        // the gh binary; "" finds gh on PATH
	Timeout time.Duration // 0 means DefaultTimeout
}

func (g *GitHub) Kind() string { return domain.SourceGitHub }

// Fetch returns every open PR that requests my review. Errors are short and
// say what to do: ErrGHNotFound, ErrNotLoggedIn (gh exit 4), ErrTooMany, a
// timeout, bad JSON, else the first line of gh's stderr.
func (g *GitHub) Fetch(ctx context.Context) ([]domain.SourceItem, error) {
	timeout := cmp.Or(g.Timeout, DefaultTimeout)
	run, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{"api", "graphql"}
	if g.Host != "" {
		args = append(args, "--hostname", g.Host)
	}
	args = append(args, "-f", "query="+query)
	cmd := exec.CommandContext(run, cmp.Or(g.Path, "gh"), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = time.Second // a child left holding the pipes can't keep us waiting
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return parse(stdout.Bytes())
	case errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist):
		return nil, ErrGHNotFound
	case ctx.Err() != nil:
		return nil, ctx.Err() // tt is quitting
	case run.Err() != nil:
		return nil, fmt.Errorf("gh timed out after %s", timeout)
	case errors.As(err, &exit) && exit.ExitCode() == 4:
		return nil, ErrNotLoggedIn
	}
	if line := firstLine(stderr.String()); line != "" {
		return nil, errors.New(line)
	}
	return nil, fmt.Errorf("gh failed: %v", err)
}

// response is the part of the GraphQL answer Fetch reads.
type response struct {
	Data struct {
		Viewer struct {
			Login string `json:"login"`
		} `json:"viewer"`
		Search struct {
			IssueCount int `json:"issueCount"`
			Nodes      []struct {
				Number int    `json:"number"`
				Title  string `json:"title"`
				URL    string `json:"url"`
				Author *struct {
					Login string `json:"login"`
				} `json:"author"` // null for a deleted account
				Repository struct {
					NameWithOwner string `json:"nameWithOwner"`
				} `json:"repository"`
				TimelineItems struct {
					Nodes []struct {
						CreatedAt         time.Time `json:"createdAt"`
						RequestedReviewer *struct {
							Login string `json:"login"`
						} `json:"requestedReviewer"` // null for a team
					} `json:"nodes"`
				} `json:"timelineItems"`
			} `json:"nodes"`
		} `json:"search"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func parse(data []byte) ([]domain.SourceItem, error) {
	var r response
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("gh printed bad JSON: %v", err)
	}
	if len(r.Errors) > 0 {
		return nil, errors.New(firstLine(r.Errors[0].Message))
	}
	if r.Data.Search.IssueCount > MaxItems {
		return nil, ErrTooMany
	}
	me := r.Data.Viewer.Login
	items := make([]domain.SourceItem, 0, len(r.Data.Search.Nodes))
	for _, n := range r.Data.Search.Nodes {
		if n.Number == 0 {
			continue // not a pull request
		}
		i := domain.SourceItem{
			Kind:       domain.SourceGitHub,
			ExternalID: fmt.Sprintf("%s#%d", n.Repository.NameWithOwner, n.Number),
			Number:     n.Number, Title: n.Title, URL: n.URL, Author: "ghost", Repo: n.Repository.NameWithOwner,
		}
		if n.Author != nil {
			i.Author = n.Author.Login
		}
		for _, e := range n.TimelineItems.Nodes {
			if e.RequestedReviewer != nil && strings.EqualFold(e.RequestedReviewer.Login, me) && e.CreatedAt.After(i.RequestedAt) {
				i.RequestedAt = e.CreatedAt
			}
		}
		items = append(items, i)
	}
	return items, nil
}

// firstLine is the first non-empty line of s, without gh's "gh: " prefix.
func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "gh: "))
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/source/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS (~2s: the timeout test waits out `WaitDelay`), no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/source/fake.go internal/source/github.go internal/source/github_test.go
git commit -m 'feat(source): the gh adapter for review requests (D50) and a fake

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 3: `[github]` config, `SyncSources` (reconcile, auto-close), the lane state; `cmd/tt` wiring

D48's review defaults and D50's `[github]` keys; `app.Source` (consumer side, as D45 did for the calendar), `WithSource`/`SourceSetUp`; `SyncSources` records a failed fetch and, only on success, replaces the stored set (keeping `first_seen_at`) and auto-closes every open linked task whose PR is gone (done, `closed_by = source`, `closed_at = updated_at = now`: the Plan 2 carry-forward), returning those tasks. `Load` returns `State.GitHub`: the lane (items with no open task, longest waiting first) and the fetch status. `cmd/tt` sets GitHub up unless `enabled = false`.

**Files:**
- Modify: `cmd/tt/main.go`
- Modify: `config.example.toml`
- Modify: `internal/app/day.go`
- Modify: `internal/app/service.go`
- Create: `internal/app/sources.go`
- Modify: `internal/config/config.go`
- Modify: `cmd/tt/main_test.go`
- Create: `internal/app/sources_test.go`
- Create: `internal/config/github_test.go`

**Interfaces:**
- Consumes: Task 1 (`SourceItems`, `ReplaceSourceItems`, `OpenSourceTasks`, `domain.SourceItem`), Task 2 (`source.Fake`, `source.ErrNotLoggedIn`, `source.ErrTooMany`, `source.ErrGHNotFound` in tests; `source.GitHub` in `cmd/tt`).
- Produces:
  - `config.Config.GitHub bool` (default true), `GitHubHost string`, `GitHubPoll time.Duration` (5m; at least 30s), `ReviewPriority int` (a level index; default the level named P2, else the last), `ReviewSize domain.Size` (default S; `""` = unsized). Errors: `github.poll: want a duration of 30s or more, like "5m", got "…"`, `github.priority: no priority level named "X"`, `github.size: want s, m, l, xl, or "" for unsized, got "…"`.
  - `app.Source interface { Kind() string; Fetch(ctx) ([]domain.SourceItem, error) }`; `func (*Service) WithSource(Source) *Service`; `func (*Service) SourceSetUp() bool`.
  - `func (*Service) SyncSources(ctx) ([]domain.Task, error)`: the tasks it closed; a failed fetch is recorded, not returned; a cancelled ctx returns `ctx.Err()` and records nothing.
  - `app.GitHubState{SetUp bool; Reviews []domain.SourceItem; LastSuccess time.Time; Err string}`; `app.State.GitHub`. `app.Store` gains `SourceItems(ctx, kind)`.

- [ ] **Step 1: Write the failing tests**

Patch `cmd/tt/main_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/cmd/tt/main_test.go b/cmd/tt/main_test.go
index a2d1e46..fdef5c5 100644
--- a/cmd/tt/main_test.go
+++ b/cmd/tt/main_test.go
@@ -174,6 +174,39 @@ func TestCalendarComesFromTheConfig(t *testing.T) {
 	}
 }
 
+func TestGitHubComesFromTheConfig(t *testing.T) {
+	ctx := context.Background()
+	env := testEnv(t)
+	t.Setenv("PATH", t.TempDir()) // no gh anywhere: nothing reaches GitHub
+	githubOf := func() app.GitHubState {
+		t.Helper()
+		var gs app.GitHubState
+		var out, errOut bytes.Buffer
+		code := run(ctx, nil, input{}, &out, &errOut, func(k string) string { return env[k] },
+			func(ctx context.Context, svc *app.Service) error {
+				if _, err := svc.SyncSources(ctx); err != nil {
+					return err
+				}
+				st, err := svc.Load(ctx)
+				gs = st.GitHub
+				return err
+			})
+		if code != 0 {
+			t.Fatalf("exit %d, stderr %q", code, errOut.String())
+		}
+		return gs
+	}
+	if gs := githubOf(); !gs.SetUp || gs.Err != "gh not found · brew install gh" {
+		t.Errorf("GitHub is on by default (D50): %+v; want set up, with the hint", gs)
+	}
+	if err := os.WriteFile(env["TT_CONFIG"], []byte("[github]\nenabled = false\n"), 0o644); err != nil {
+		t.Fatal(err)
+	}
+	if gs := githubOf(); gs.SetUp {
+		t.Errorf("enabled = false: %+v; want not set up", gs)
+	}
+}
+
 func TestTUIErrorsExitNonZero(t *testing.T) {
 	env := testEnv(t)
 	var out, errOut bytes.Buffer
```

Create `internal/app/sources_test.go`:

```go
package app

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/source"
	"tt/internal/store"
)

// srcService is a started service with a fake GitHub; it also returns the
// store, for inspecting rows by hand.
func srcService(t *testing.T) (*Service, *fakeClock, *source.Fake, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "tt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	clock := &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
	src := &source.Fake{}
	s := New(st, clock, config.Default()).WithSource(src)
	if _, err := s.StartDay(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s, clock, src, st
}

// review is a review request for PR n, requested at requested (zero: unknown).
func review(n int, title string, requested time.Time) domain.SourceItem {
	return domain.SourceItem{Kind: "github", ExternalID: "acme/api#" + strconv.Itoa(n), Number: n, Title: title,
		URL: "https://github.com/acme/api/pull/" + strconv.Itoa(n), Author: "ana", Repo: "acme/api", RequestedAt: requested}
}

func mustSyncSources(t *testing.T, s *Service) []domain.Task {
	t.Helper()
	closed, err := s.SyncSources(context.Background())
	if err != nil {
		t.Fatalf("SyncSources: %v", err)
	}
	return closed
}

func ids(items []domain.SourceItem) []string {
	var out []string
	for _, i := range items {
		out = append(out, i.ExternalID)
	}
	return out
}

// linked inserts an open task materialized from item (what PinSource does).
func linked(t *testing.T, st *store.Store, item domain.SourceItem, now time.Time) int64 {
	t.Helper()
	var id int64
	err := st.InTx(context.Background(), func(x *store.Tx) (err error) {
		ref := item.Ref()
		id, err = x.InsertTask(context.Background(), domain.Task{Title: domain.ReviewTitle(item), Link: item.URL, Source: &ref,
			Status: domain.StatusOpen, CreatedAt: now, UpdatedAt: now})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestGitHubNotSetUp(t *testing.T) {
	s, _ := startedService(t)
	if closed, err := s.SyncSources(context.Background()); err != nil || closed != nil {
		t.Fatal(closed, err)
	}
	if s.SourceSetUp() {
		t.Error("SourceSetUp without WithSource")
	}
	if gs := mustLoad(t, s).GitHub; gs.SetUp || gs.Reviews != nil || gs.Err != "" || !gs.LastSuccess.IsZero() {
		t.Errorf("GitHub off (D50): %+v", gs)
	}
}

func TestSyncSourcesFillsTheLane(t *testing.T) {
	s, clock, src, _ := srcService(t)
	if !s.SourceSetUp() {
		t.Error("SourceSetUp after WithSource")
	}
	if gs := mustLoad(t, s).GitHub; !gs.SetUp || gs.Reviews != nil || !gs.LastSuccess.IsZero() || gs.Err != "" {
		t.Fatalf("before the first fetch: %+v", gs)
	}
	start := clock.now
	src.Set([]domain.SourceItem{
		review(877, "Bump go", start.Add(-24*time.Hour)),
		review(901, "Add idempotency key", start.Add(-72*time.Hour)),
		review(920, "Fix nil deref", time.Time{}), // no request event: waits since first seen
		review(877, "Bump go", start.Add(-24*time.Hour)),
	}, nil)
	if closed := mustSyncSources(t, s); closed != nil {
		t.Errorf("nothing to close: %v", closed)
	}
	gs := mustLoad(t, s).GitHub
	if got := ids(gs.Reviews); len(got) != 3 || got[0] != "acme/api#901" || got[1] != "acme/api#877" || got[2] != "acme/api#920" {
		t.Fatalf("lane = %v; want longest waiting first, one row per PR", got)
	}
	if r := gs.Reviews[2]; !r.FirstSeenAt.Equal(start) || !r.WaitingSince().Equal(start) || r.Author != "ana" || r.Kind != "github" {
		t.Errorf("#920 = %+v", r)
	}
	if !gs.LastSuccess.Equal(start) || gs.Err != "" {
		t.Errorf("after a success: %+v", gs)
	}

	// The next fetch keeps first_seen_at; a PR gone from the set leaves the lane.
	clock.Advance(5 * time.Minute)
	src.Set([]domain.SourceItem{review(920, "Fix nil deref (v2)", time.Time{}), review(901, "Add idempotency key", start.Add(-72*time.Hour))}, nil)
	mustSyncSources(t, s)
	gs = mustLoad(t, s).GitHub
	if got := ids(gs.Reviews); len(got) != 2 || got[1] != "acme/api#920" {
		t.Fatalf("lane = %v", got)
	}
	if r := gs.Reviews[1]; !r.FirstSeenAt.Equal(start) || r.Title != "Fix nil deref (v2)" {
		t.Errorf("#920 after a refetch = %+v; want first seen kept, title refreshed", r)
	}

	// D3a: a failed fetch keeps the last good lane and says why.
	clock.Advance(5 * time.Minute)
	src.Set(nil, source.ErrNotLoggedIn)
	mustSyncSources(t, s)
	gs = mustLoad(t, s).GitHub
	if len(gs.Reviews) != 2 || gs.Err != source.ErrNotLoggedIn.Error() || !gs.LastSuccess.Equal(start.Add(5*time.Minute)) {
		t.Errorf("after a failure: %v, err %q, last success %v", ids(gs.Reviews), gs.Err, gs.LastSuccess)
	}

	// A success clears the error; an empty set is an empty lane, not a failure.
	clock.Advance(5 * time.Minute)
	src.Set(nil, nil)
	mustSyncSources(t, s)
	if gs = mustLoad(t, s).GitHub; gs.Reviews != nil || gs.Err != "" || !gs.LastSuccess.Equal(clock.now) {
		t.Errorf("after an empty success: %+v", gs)
	}
}

func TestSyncSourcesAutoCloses(t *testing.T) {
	s, clock, src, st := srcService(t)
	ctx := context.Background()
	a, b, c := review(901, "Add idempotency key", clock.now), review(877, "Bump go", clock.now), review(915, "Lint config", clock.now)
	src.Set([]domain.SourceItem{a, b, c}, nil)
	mustSyncSources(t, s)
	aID, bID := linked(t, st, a, clock.now), linked(t, st, b, clock.now)
	cID := linked(t, st, c, clock.now)
	if _, err := s.Done(ctx, cID); err != nil { // done by me while GitHub still asks
		t.Fatal(err)
	}
	if _, _, err := s.Block(ctx, aID, 10*time.Hour, time.Hour); err != nil { // blocked today, pinned
		t.Fatal(err)
	}
	edit := mustUndo(t)(s.Edit(ctx, aID, "Review #901 with notes"))
	if got := ids(mustLoad(t, s).GitHub.Reviews); len(got) != 1 || got[0] != "acme/api#915" {
		t.Fatalf("lane = %v; want only #915: pinned reviews leave it, a closed one comes back (D49)", got)
	}

	// D20a: #901 is no longer requested; #877 still is.
	clock.Advance(5 * time.Minute)
	src.Set([]domain.SourceItem{b, c}, nil)
	closed := mustSyncSources(t, s)
	if len(closed) != 1 || closed[0].ID != aID || closed[0].Title != "Review #901 with notes" {
		t.Fatalf("closed = %+v; want #%d", closed, aID)
	}
	got := mustGet(t, s, aID)
	if got.Status != domain.StatusDone || got.ClosedBy != domain.ClosedBySource || !got.ClosedAt.Equal(clock.now) || !got.UpdatedAt.Equal(clock.now) {
		t.Errorf("auto-closed task = %+v; want done by the source now, UpdatedAt now", got)
	}
	if got := mustGet(t, s, bID); got.Status != domain.StatusOpen {
		t.Errorf("#877 is still requested: %v", got.Status)
	}
	plan := mustPlan(t, s)
	if len(plan.Candidates) != 1 || plan.Candidates[0].Task.ID != aID || len(plan.Blocks) != 1 {
		t.Errorf("the auto-closed review stays on Today, done, with its block: %+v", plan)
	}
	if err := edit(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo of an edit made before the auto-close: %v; want ErrConflict (integrity over undo)", err)
	}
	if got := mustGet(t, s, aID); got.Status != domain.StatusDone {
		t.Errorf("the refused undo left %v", got.Status)
	}

	// A sync that changes nothing closes nothing; a closed task stays closed.
	clock.Advance(5 * time.Minute)
	if closed := mustSyncSources(t, s); closed != nil {
		t.Errorf("second sync closed %v", closed)
	}
	if got := mustGet(t, s, cID); got.ClosedBy != domain.ClosedByUser {
		t.Errorf("a task I closed keeps closed_by user: %+v", got)
	}

	// Auto-closes never count as rollover (D20a): the next day, #901's block
	// isn't an unfinished one.
	clock.now = time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	res, err := s.StartDay(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Recap == nil || res.Recap.RolledOver != 0 || mustGet(t, s, aID).RolloverCount != 0 {
		t.Errorf("rollover after an auto-close: %+v", res.Recap)
	}
}

func TestNoReconcileOnAFailedFetch(t *testing.T) {
	s, clock, src, st := srcService(t)
	a := review(901, "Add idempotency key", clock.now)
	src.Set([]domain.SourceItem{a}, nil)
	mustSyncSources(t, s)
	id := linked(t, st, a, clock.now)
	for _, err := range []error{source.ErrTooMany, errors.New("could not resolve host api.github.com"), source.ErrGHNotFound} {
		clock.Advance(5 * time.Minute)
		src.Set(nil, err)
		if closed := mustSyncSources(t, s); closed != nil {
			t.Errorf("%v: closed %v", err, closed)
		}
		if got := mustGet(t, s, id); got.Status != domain.StatusOpen {
			t.Errorf("%v: an outage closed the review (spec: Error handling)", err)
		}
		if gs := mustLoad(t, s).GitHub; gs.Err != err.Error() {
			t.Errorf("Err = %q; want %q", gs.Err, err)
		}
	}
}

func TestSyncSourcesStopsWhenQuitting(t *testing.T) {
	s, _, src, _ := srcService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	src.Set(nil, context.Canceled)
	if _, err := s.SyncSources(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want context.Canceled", err)
	}
	if gs := mustLoad(t, s).GitHub; gs.Err != "" {
		t.Errorf("quitting isn't GitHub's failure: %q", gs.Err)
	}
}
```

Create `internal/config/github_test.go`:

```go
package config

import (
	"strings"
	"testing"
	"time"

	"tt/internal/domain"
)

func TestParseGitHub(t *testing.T) {
	cfg := Default()
	if !cfg.GitHub || cfg.GitHubHost != "" || cfg.GitHubPoll != 5*time.Minute || cfg.ReviewPriority != 2 || cfg.ReviewSize != domain.SizeS {
		t.Errorf("defaults: %v %q %v P%d %q; want on, gh's host, 5m, P2, S (D48, D50)",
			cfg.GitHub, cfg.GitHubHost, cfg.GitHubPoll, cfg.ReviewPriority, cfg.ReviewSize)
	}
	cfg, err := Parse([]byte("[github]\nenabled = false\nhost = \" github.acme.com \"\npoll = \"30s\"\npriority = \"p1\"\nsize = \"\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitHub || cfg.GitHubHost != "github.acme.com" || cfg.GitHubPoll != 30*time.Second || cfg.ReviewPriority != 1 || cfg.ReviewSize != domain.SizeNone {
		t.Errorf("github: %v %q %v P%d %q", cfg.GitHub, cfg.GitHubHost, cfg.GitHubPoll, cfg.ReviewPriority, cfg.ReviewSize)
	}
	if cfg, err := Parse([]byte("[github]\nsize = \"xl\"\n")); err != nil || cfg.ReviewSize != domain.SizeXL {
		t.Errorf("size xl: %q, %v", cfg.ReviewSize, err)
	}

	// Other priority levels: the default follows the level named P2, else the last.
	two := "[[priorities]]\nname = \"Now\"\n[[priorities]]\nname = \"P2\"\n[[priorities]]\nname = \"Later\"\n[[priorities]]\nname = \"Never\"\n"
	if cfg, err := Parse([]byte(two)); err != nil || cfg.ReviewPriority != 2 {
		t.Errorf("a level named P2: P%d, %v", cfg.ReviewPriority, err)
	}
	none := "[[priorities]]\nname = \"Now\"\n[[priorities]]\nname = \"Soon\"\n[[priorities]]\nname = \"Later\"\n"
	if cfg, err := Parse([]byte(none)); err != nil || cfg.ReviewPriority != 3 {
		t.Errorf("no level named P2: P%d, %v; want the last level", cfg.ReviewPriority, err)
	}
	if cfg, err := Parse([]byte(none + "[github]\npriority = \"Soon\"\n")); err != nil || cfg.ReviewPriority != 2 {
		t.Errorf("priority Soon: P%d, %v", cfg.ReviewPriority, err)
	}

	cases := map[string]string{
		"[github]\npoll = \"10s\"":           "github.poll: want a duration of 30s or more",
		"[github]\npoll = \"5\"":             "github.poll: want a duration of 30s or more",
		"[github]\npriority = \"P7\"":        `github.priority: no priority level named "P7"`,
		none + "[github]\npriority = \"P2\"": `github.priority: no priority level named "P2"`,
		"[github]\nsize = \"huge\"":          "github.size: want s, m, l, xl, or \"\" for unsized",
		"[github]\nsize = \"x\"":             "github.size: want s, m, l, xl",
		"[github]\nenabled = \"yes\"":        "enabled",
		"[github]\ntoken = \"x\"":            "unknown config key",
	}
	for input, wantErr := range cases {
		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/ ./internal/app/ ./cmd/tt/`
Expected: FAIL (build) in all three: `cfg.GitHub undefined (type Config has no field or method GitHub)`, `cfg.ReviewPriority undefined`; in app and cmd/tt `WithSource`, `SyncSources` and `GitHub` (the State field and `app.GitHubState`) undefined.

- [ ] **Step 3: Implement**

Patch `cmd/tt/main.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/cmd/tt/main.go b/cmd/tt/main.go
index 9a0a46b..80e6576 100644
--- a/cmd/tt/main.go
+++ b/cmd/tt/main.go
@@ -18,6 +18,7 @@ import (
 	"tt/internal/config"
 	"tt/internal/domain"
 	"tt/internal/dsl"
+	"tt/internal/source"
 	"tt/internal/store"
 	"tt/internal/tui"
 )
@@ -182,6 +183,9 @@ func open(ctx context.Context, getenv func(string) string) (*app.Service, func()
 	if len(cfg.Calendars) > 0 { // D40: no calendars, no calendar
 		svc.WithCalendar(&calendar.EventKit{Calendars: cfg.Calendars})
 	}
+	if cfg.GitHub { // D50
+		svc.WithSource(&source.GitHub{Host: cfg.GitHubHost})
+	}
 	return svc, func() { st.Close() }, nil
 }
 
```

Patch `config.example.toml` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/config.example.toml b/config.example.toml
index cb4a59c..06a7805 100644
--- a/config.example.toml
+++ b/config.example.toml
@@ -43,3 +43,12 @@ end = "19:00"
 calendars = []
 # calendars = ["Work"]
 poll = "2m" # how often the calendar is fetched; 10s or more
+
+# PR reviews waiting on me, through the gh CLI (D50): open PRs that request
+# my review directly. gh must be logged in (`gh auth login`).
+[github]
+enabled = true  # false: no fetch, no Reviews lane
+host = ""       # a GitHub Enterprise host; empty: gh's default (github.com)
+poll = "5m"     # how often GitHub is fetched; 30s or more
+priority = "P2" # a pinned review's priority: one of the [[priorities]] names
+size = "s"      # a pinned review's size: s, m, l, xl, or "" for unsized (D48)
```

Patch `internal/app/day.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/day.go b/internal/app/day.go
index 13bab78..9316c79 100644
--- a/internal/app/day.go
+++ b/internal/app/day.go
@@ -137,10 +137,11 @@ type State struct {
 	Snapshot domain.Snapshot
 	Plan     domain.Plan
 	Calendar CalendarState
+	GitHub   GitHubState
 }
 
 // Load reads the data version first, then all tasks and today's plan in one
-// read, then the calendar's cache. A write that lands during the load changes
+// read, then the calendar's cache and GitHub's review requests. A write that lands during the load changes
 // the version, so the next version check reloads again.
 func (s *Service) Load(ctx context.Context) (State, error) {
 	version, err := s.store.DataVersion(ctx)
@@ -156,7 +157,11 @@ func (s *Service) Load(ctx context.Context) (State, error) {
 	if err != nil {
 		return State{}, err
 	}
-	return State{Version: version, Snapshot: snap, Plan: s.buildPlan(today, snap, state), Calendar: cal}, nil
+	gh, err := s.githubState(ctx, snap)
+	if err != nil {
+		return State{}, err
+	}
+	return State{Version: version, Snapshot: snap, Plan: s.buildPlan(today, snap, state), Calendar: cal, GitHub: gh}, nil
 }
 
 func (s *Service) buildPlan(today domain.Day, snap domain.Snapshot, state store.PlanState) domain.Plan {
```

Patch `internal/app/service.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/service.go b/internal/app/service.go
index 3c54fd1..ed2d3d4 100644
--- a/internal/app/service.go
+++ b/internal/app/service.go
@@ -27,6 +27,7 @@ type Store interface {
 	DataVersion(ctx context.Context) (int64, error)
 	UIState(ctx context.Context, key string) (string, bool, error)
 	Integration(ctx context.Context, name string) (store.Integration, bool, error)
+	SourceItems(ctx context.Context, kind string) ([]domain.SourceItem, error)
 }
 
 // Undo reverts the command that returned it. The TUI keeps these on a
@@ -38,6 +39,7 @@ type Service struct {
 	clock Clock
 	cfg   config.Config
 	cal   CalendarSource // nil: no calendar set up (D40)
+	src   Source         // nil: GitHub isn't set up (D50)
 }
 
 func New(st Store, clock Clock, cfg config.Config) *Service {
```

Create `internal/app/sources.go`:

```go
package app

import (
	"cmp"
	"context"
	"slices"
	"time"

	"tt/internal/domain"
	"tt/internal/store"
)

// Source is where review requests come from (D20, D50); the gh adapter in
// internal/source implements it.
type Source interface {
	Kind() string                                           // its integration_state row and the tasks' source kind
	Fetch(ctx context.Context) ([]domain.SourceItem, error) // everything awaiting me now
}

// WithSource sets the source SyncSources reads. Without one there is no
// Reviews lane (D50). Call it before the service is used.
func (s *Service) WithSource(src Source) *Service {
	s.src = src
	return s
}

// SourceSetUp reports whether GitHub is set up (D50).
func (s *Service) SourceSetUp() bool { return s.src != nil }

// GitHubState is what the views show of GitHub (D49, D50).
type GitHubState struct {
	SetUp bool
	// Reviews is the Reviews lane: the requests with no open task, longest
	// waiting first.
	Reviews     []domain.SourceItem
	LastSuccess time.Time // zero: never fetched
	Err         string    // why the last fetch failed; "" once one succeeds
}

// SyncSources fetches what is awaiting me. Only on success it reconciles, in
// one transaction (D50): the fetched set replaces the stored one (keeping
// when each item was first seen), and every open task whose item is gone is
// closed by the source. It returns those tasks. A failed fetch isn't
// returned: it is recorded, and the views show it from the state; only a
// failure to record the fetch is.
func (s *Service) SyncSources(ctx context.Context) ([]domain.Task, error) {
	if s.src == nil {
		return nil, nil
	}
	items, fetchErr := s.src.Fetch(ctx)
	if ctx.Err() != nil {
		return nil, ctx.Err() // tt is quitting; that isn't GitHub's fault
	}
	kind, now := s.src.Kind(), s.clock.Now()
	var closed []domain.Task
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		in, _, err := x.GetIntegration(ctx, kind)
		if err != nil {
			return err
		}
		if fetchErr != nil {
			in.LastError, in.LastErrorAt = fetchErr.Error(), now
			return x.PutIntegration(ctx, in)
		}
		old, err := x.SourceItems(ctx, kind)
		if err != nil {
			return err
		}
		firstSeen := map[string]time.Time{}
		for _, i := range old {
			firstSeen[i.ExternalID] = i.FirstSeenAt
		}
		live := map[string]bool{}
		var set []domain.SourceItem
		for _, i := range items {
			if live[i.ExternalID] {
				continue
			}
			live[i.ExternalID] = true
			i.Kind, i.FirstSeenAt = kind, now
			if t, ok := firstSeen[i.ExternalID]; ok {
				i.FirstSeenAt = t
			}
			set = append(set, i)
		}
		if err := x.ReplaceSourceItems(ctx, kind, set); err != nil {
			return err
		}
		open, err := x.OpenSourceTasks(ctx, kind)
		if err != nil {
			return err
		}
		for _, t := range open {
			if live[t.Source.ID] {
				continue
			}
			// D20a: done by the source. UpdatedAt moves, so an older undo of
			// this task refuses instead of reopening it.
			t.Status, t.ClosedAt, t.ClosedBy, t.DropReason, t.UpdatedAt = domain.StatusDone, now, domain.ClosedBySource, "", now
			if err := x.UpdateTask(ctx, t); err != nil {
				return err
			}
			closed = append(closed, t)
		}
		in.LastSuccessAt, in.LastError, in.LastErrorAt = now, "", time.Time{}
		return x.PutIntegration(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return closed, nil
}

// githubState reads the last fetch and the lane: the stored items without an
// open task in snap (a pinned review is a candidate instead; a closed one
// whose PR still requests me is back, D49).
func (s *Service) githubState(ctx context.Context, snap domain.Snapshot) (GitHubState, error) {
	if s.src == nil {
		return GitHubState{}, nil
	}
	kind := s.src.Kind()
	gs := GitHubState{SetUp: true}
	in, ok, err := s.store.Integration(ctx, kind)
	if err != nil {
		return GitHubState{}, err
	}
	if ok {
		gs.LastSuccess, gs.Err = in.LastSuccessAt, in.LastError
	}
	items, err := s.store.SourceItems(ctx, kind)
	if err != nil {
		return GitHubState{}, err
	}
	planned := map[string]bool{}
	for _, t := range snap.Tasks() {
		if t.Source != nil && t.Source.Kind == kind && t.Status == domain.StatusOpen {
			planned[t.Source.ID] = true
		}
	}
	for _, i := range items {
		if !planned[i.ExternalID] {
			gs.Reviews = append(gs.Reviews, i)
		}
	}
	slices.SortFunc(gs.Reviews, func(a, b domain.SourceItem) int {
		return cmp.Or(a.WaitingSince().Compare(b.WaitingSince()), cmp.Compare(a.ExternalID, b.ExternalID))
	})
	return gs, nil
}
```

Patch `internal/config/config.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/config/config.go b/internal/config/config.go
index 8e31416..1428d45 100644
--- a/internal/config/config.go
+++ b/internal/config/config.go
@@ -6,6 +6,7 @@ import (
 	"fmt"
 	"io/fs"
 	"os"
+	"slices"
 	"strings"
 	"time"
 
@@ -35,6 +36,16 @@ type Config struct {
 	Calendars []string
 	// CalendarPoll is how often the calendar is fetched (D3).
 	CalendarPoll time.Duration
+	// GitHub turns on the Reviews lane: review requests are fetched through
+	// gh (D50). GitHubHost is a GitHub Enterprise host; "" is gh's default.
+	GitHub     bool
+	GitHubHost string
+	// GitHubPoll is how often GitHub is fetched (D3).
+	GitHubPoll time.Duration
+	// ReviewPriority (a level index) and ReviewSize are what the task of a
+	// pinned review request starts with (D48).
+	ReviewPriority int
+	ReviewSize     domain.Size
 }
 
 func Default() Config {
@@ -51,11 +62,15 @@ func Default() Config {
 			domain.SizeL:  8 * time.Hour,
 			domain.SizeXL: 40 * time.Hour,
 		},
-		UnsizedBlock:  30 * time.Minute,
-		RolloverAlert: 3,
-		WorkStart:     8 * time.Hour,
-		WorkEnd:       19 * time.Hour,
-		CalendarPoll:  2 * time.Minute,
+		UnsizedBlock:   30 * time.Minute,
+		RolloverAlert:  3,
+		WorkStart:      8 * time.Hour,
+		WorkEnd:        19 * time.Hour,
+		CalendarPoll:   2 * time.Minute,
+		GitHub:         true,
+		GitHubPoll:     5 * time.Minute,
+		ReviewPriority: 2,
+		ReviewSize:     domain.SizeS,
 	}
 }
 
@@ -76,6 +91,13 @@ type file struct {
 		Calendars []string `toml:"calendars"`
 		Poll      *string  `toml:"poll"`
 	} `toml:"calendar"`
+	GitHub struct {
+		Enabled  *bool   `toml:"enabled"`
+		Host     *string `toml:"host"`
+		Poll     *string `toml:"poll"`
+		Priority *string `toml:"priority"`
+		Size     *string `toml:"size"`
+	} `toml:"github"`
 }
 
 // Load reads path. A missing file yields Default().
@@ -173,6 +195,9 @@ func Parse(data []byte) (Config, error) {
 		}
 		cfg.CalendarPoll = d
 	}
+	if err := parseGitHub(f, &cfg); err != nil {
+		return Config{}, err
+	}
 	switch {
 	case cfg.WorkEnd <= cfg.WorkStart:
 		return Config{}, fmt.Errorf("work_hours.end must be after work_hours.start")
@@ -182,6 +207,45 @@ func Parse(data []byte) (Config, error) {
 	return cfg, nil
 }
 
+// parseGitHub applies [github] (D48, D50). Without a priority the reviews
+// get the level named P2, else the last level, so configuring other
+// priorities never fails over a key that was never set.
+func parseGitHub(f file, cfg *Config) error {
+	g := f.GitHub
+	if g.Enabled != nil {
+		cfg.GitHub = *g.Enabled
+	}
+	if g.Host != nil {
+		cfg.GitHubHost = strings.TrimSpace(*g.Host)
+	}
+	if p := g.Poll; p != nil {
+		d, err := time.ParseDuration(*p)
+		if err != nil || d < 30*time.Second {
+			return fmt.Errorf("github.poll: want a duration of 30s or more, like \"5m\", got %q", *p)
+		}
+		cfg.GitHubPoll = d
+	}
+	name := "P2"
+	if g.Priority != nil {
+		name = strings.TrimSpace(*g.Priority)
+	}
+	cfg.ReviewPriority = slices.IndexFunc(cfg.Priorities, func(l PriorityLevel) bool { return strings.EqualFold(l.Name, name) }) + 1
+	if cfg.ReviewPriority == 0 {
+		if g.Priority != nil {
+			return fmt.Errorf("github.priority: no priority level named %q", name)
+		}
+		cfg.ReviewPriority = len(cfg.Priorities)
+	}
+	if p := g.Size; p != nil {
+		size, ok := domain.ParseSize(*p)
+		if *p != "" && (!ok || string(size) != strings.ToLower(*p)) {
+			return fmt.Errorf("github.size: want s, m, l, xl, or \"\" for unsized, got %q", *p)
+		}
+		cfg.ReviewSize = size
+	}
+	return nil
+}
+
 // parseClock parses "HH:MM" (00:00 to 23:59) into an offset from midnight.
 func parseClock(s string) (time.Duration, error) {
 	t, err := time.Parse("15:04", s)
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/config/ ./internal/app/ ./cmd/tt/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/app/`
Expected: PASS (`TestExampleConfigIsTheDefaults` covers the new `[github]` block), no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add cmd/tt/main.go cmd/tt/main_test.go config.example.toml internal/app/day.go internal/app/service.go internal/app/sources.go internal/app/sources_test.go internal/config/config.go internal/config/github_test.go
git commit -m 'feat(app): [github] config, SyncSources reconcile and auto-close, the Reviews lane state (D48, D50)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 4: `PinSource`: create or reopen a review's task, with undo

D20/D48/D49's `p` on a review request, in one transaction: no task yet → create `Review #n: <title>` with the PR link, the source ref, the configured priority and size and no requester, and pin it; a closed task (done or dropped, by me or by GitHub) → reopen that same task (the unique source ref; it keeps its history and its own priority/size) and pin it; an open one → pin it. The undo deletes the created task or closes the reopened one exactly as it was; it refuses with `ErrConflict` once the task changed, a block was placed on it today, or the day moved on (integrity over undo).

**Files:**
- Modify: `internal/app/sources.go`
- Create: `internal/app/pinsource_test.go`

**Interfaces:**
- Consumes: Task 1 (`TaskBySource`, `SourceItems`), Task 3 (`srcService`, `review`, `mustSyncSources`, `ids` test helpers; `config.ReviewPriority`, `ReviewSize`), existing `unchangedSince`, `checkDay`, `pinned` (pins_test.go).
- Produces: `func (*Service) PinSource(ctx, kind, extID string) (domain.Task, Undo, error)`: the task as stored; `ErrNotFound` when the request isn't stored any more; a nil undo when the task was open and pinned already.

- [ ] **Step 1: Write the failing tests**

Create `internal/app/pinsource_test.go`:

```go
package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/source"
	"tt/internal/store"
)

func mustPinSource(t *testing.T, s *Service, extID string) (domain.Task, Undo) {
	t.Helper()
	task, undo, err := s.PinSource(context.Background(), "github", extID)
	if err != nil {
		t.Fatalf("PinSource(%s): %v", extID, err)
	}
	return task, undo
}

func TestPinSourceCreatesTheTask(t *testing.T) {
	s, clock, src, _ := srcService(t)
	ctx := context.Background()
	src.Set([]domain.SourceItem{review(901, "Add idempotency key", clock.now.Add(-time.Hour))}, nil)
	mustSyncSources(t, s)

	task, undo := mustPinSource(t, s, "acme/api#901")
	got := mustGet(t, s, task.ID)
	if got.Title != "Review #901: Add idempotency key" || got.Link != "https://github.com/acme/api/pull/901" ||
		got.Source == nil || *got.Source != (domain.SourceRef{Kind: "github", ID: "acme/api#901"}) ||
		got.Priority != 2 || got.Size != domain.SizeS || got.Requester != "" || got.Status != domain.StatusOpen {
		t.Errorf("task = %+v; want D20b's wrapper with D48's P2 S and no requester", got)
	}
	if !pinned(t, s, task.ID) {
		t.Error("the review is a candidate now")
	}
	if snap := mustLoad(t, s); snap.GitHub.Reviews != nil || len(snap.Snapshot.Inbox()) != 0 {
		t.Errorf("a pinned review leaves the lane (D49) and skips the Inbox: lane %v", ids(snap.GitHub.Reviews))
	}
	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if snap := mustLoad(t, s); len(snap.Snapshot.Tasks()) != 0 || len(snap.GitHub.Reviews) != 1 || len(snap.Plan.Candidates) != 0 {
		t.Errorf("undo deletes the task and the review is back in the lane: %d tasks, lane %v", len(snap.Snapshot.Tasks()), ids(snap.GitHub.Reviews))
	}

	if _, _, err := s.PinSource(ctx, "github", "acme/api#999"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a request no longer waiting: %v; want ErrNotFound", err)
	}
}

func TestPinSourceUsesTheConfiguredDefaults(t *testing.T) {
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "tt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg, err := config.Parse([]byte("[github]\npriority = \"P1\"\nsize = \"\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	src := &source.Fake{}
	s := New(st, &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}, cfg).WithSource(src)
	src.Set([]domain.SourceItem{review(901, "Add idempotency key", time.Time{})}, nil)
	mustSyncSources(t, s)
	task, _ := mustPinSource(t, s, "acme/api#901")
	if got := mustGet(t, s, task.ID); got.Priority != 1 || got.Size != domain.SizeNone {
		t.Errorf("task = P%d %q; want P1 unsized", got.Priority, got.Size)
	}
}

func TestPinSourceUndoRefusesAfterChanges(t *testing.T) {
	s, clock, src, _ := srcService(t)
	ctx := context.Background()
	src.Set([]domain.SourceItem{review(901, "Add idempotency key", clock.now), review(877, "Bump go", clock.now)}, nil)
	mustSyncSources(t, s)

	blocked, undo := mustPinSource(t, s, "acme/api#901")
	if _, _, err := s.Block(ctx, blocked.ID, 10*time.Hour, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := undo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo after a block: %v; want ErrConflict (the block must not go silently)", err)
	}
	if !pinned(t, s, blocked.ID) || len(mustPlan(t, s).Blocks) != 1 {
		t.Error("the refused undo left the task and its block alone")
	}

	edited, undo := mustPinSource(t, s, "acme/api#877")
	clock.Advance(time.Minute)
	if _, err := s.SetSize(ctx, edited.ID, domain.SizeM); err != nil {
		t.Fatal(err)
	}
	if err := undo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo after an edit: %v; want ErrConflict", err)
	}

	_, undo = mustPinSource(t, s, "acme/api#877") // already open and pinned: nothing to undo
	if undo != nil {
		t.Error("pinning a pinned review returns no undo")
	}
}

func TestPinSourceReopensItsTask(t *testing.T) {
	s, clock, src, _ := srcService(t)
	ctx := context.Background()
	a := review(901, "Add idempotency key", clock.now)
	src.Set([]domain.SourceItem{a}, nil)
	mustSyncSources(t, s)
	first, _ := mustPinSource(t, s, "acme/api#901")
	if _, err := s.SetSize(ctx, first.ID, domain.SizeM); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Block(ctx, first.ID, 10*time.Hour, time.Hour); err != nil {
		t.Fatal(err)
	}

	// Removed as a reviewer: auto-closed, still pinned and blocked today.
	clock.Advance(5 * time.Minute)
	src.Set(nil, nil)
	mustSyncSources(t, s)
	closedAt := clock.now
	// Re-requested: back in the lane (D20a), its task stays closed.
	clock.Advance(5 * time.Minute)
	a.RequestedAt = clock.now
	src.Set([]domain.SourceItem{a}, nil)
	mustSyncSources(t, s)
	if got := ids(mustLoad(t, s).GitHub.Reviews); len(got) != 1 {
		t.Fatalf("lane = %v; want the re-requested review", got)
	}
	if got := mustGet(t, s, first.ID); got.Status != domain.StatusDone {
		t.Fatalf("no auto-reopen: %v", got.Status)
	}

	again, undo := mustPinSource(t, s, "acme/api#901")
	got := mustGet(t, s, again.ID)
	if again.ID != first.ID || got.Status != domain.StatusOpen || got.ClosedBy != "" || !got.ClosedAt.IsZero() || got.Size != domain.SizeM {
		t.Errorf("reopened = %+v; want the same task, open, keeping its own size", got)
	}
	if !pinned(t, s, first.ID) || mustLoad(t, s).GitHub.Reviews != nil {
		t.Error("the reopened review is a candidate, not in the lane")
	}
	if err := undo(ctx); err != nil {
		t.Fatalf("undo of the reopen (the block was there before): %v", err)
	}
	got = mustGet(t, s, first.ID)
	if got.Status != domain.StatusDone || got.ClosedBy != domain.ClosedBySource || !got.ClosedAt.Equal(closedAt) {
		t.Errorf("after the undo: %+v; want closed by the source again, as it was", got)
	}
	if !pinned(t, s, first.ID) || len(mustPlan(t, s).Blocks) != 1 {
		t.Error("the undo keeps the pin and block it found")
	}

	// Closed by me and still requested: p reopens it too.
	if _, err := s.Reopen(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Drop(ctx, first.ID, "not mine"); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustLoad(t, s).GitHub.Reviews); len(got) != 1 {
		t.Fatalf("a dropped review that still requests me is in the lane: %v", got)
	}
	if again, _ := mustPinSource(t, s, "acme/api#901"); again.ID != first.ID || mustGet(t, s, first.ID).DropReason != "" {
		t.Errorf("reopened after a drop: %+v", mustGet(t, s, first.ID))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/`
Expected: FAIL (build): `s.PinSource undefined (type *Service has no field or method PinSource)`.

- [ ] **Step 3: Implement**

Patch `internal/app/sources.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/sources.go b/internal/app/sources.go
index 9922737..5fa48db 100644
--- a/internal/app/sources.go
+++ b/internal/app/sources.go
@@ -3,6 +3,7 @@ package app
 import (
 	"cmp"
 	"context"
+	"fmt"
 	"slices"
 	"time"
 
@@ -111,6 +112,94 @@ func (s *Service) SyncSources(ctx context.Context) ([]domain.Task, error) {
 	return closed, nil
 }
 
+// PinSource pins the review request kind/extID to today (D20, D49). It
+// creates the request's task (D20b, D48: "Review #n: <title>", the PR link,
+// the configured priority and size, no requester) or, when that task exists
+// but is closed, reopens it, keeping its history. The undo deletes the task
+// it created, or closes the reopened one again as it was; it refuses
+// (ErrConflict) once the task changed, was blocked today, or the day moved on.
+func (s *Service) PinSource(ctx context.Context, kind, extID string) (domain.Task, Undo, error) {
+	today, now := s.Today(), s.clock.Now()
+	var (
+		before, after domain.Task // before is set when a closed task was reopened
+		created, had  bool
+	)
+	err := s.store.InTx(ctx, func(x *store.Tx) error {
+		items, err := x.SourceItems(ctx, kind)
+		if err != nil {
+			return err
+		}
+		i := slices.IndexFunc(items, func(i domain.SourceItem) bool { return i.ExternalID == extID })
+		if i < 0 {
+			return fmt.Errorf("review request %s isn't waiting any more: %w", extID, ErrNotFound)
+		}
+		item := items[i]
+		t, ok, err := x.TaskBySource(ctx, item.Ref())
+		switch {
+		case err != nil:
+			return err
+		case !ok:
+			ref := item.Ref()
+			after = domain.Task{Title: domain.ReviewTitle(item), Link: item.URL, Source: &ref,
+				Priority: min(s.cfg.ReviewPriority, len(s.cfg.Priorities)), Size: s.cfg.ReviewSize,
+				Status: domain.StatusOpen, CreatedAt: now, UpdatedAt: now}
+			if after.ID, err = x.InsertTask(ctx, after); err != nil {
+				return err
+			}
+			created = true
+		case t.Status != domain.StatusOpen:
+			before, after = t, t
+			after.Status, after.ClosedAt, after.ClosedBy, after.DropReason, after.UpdatedAt = domain.StatusOpen, time.Time{}, "", "", now
+			if err := x.UpdateTask(ctx, after); err != nil {
+				return err
+			}
+		default:
+			after = t
+		}
+		if _, had, err = x.GetPin(ctx, after.ID); err != nil || had {
+			return err
+		}
+		return x.PutPin(ctx, domain.Pin{TaskID: after.ID, PinnedOn: today})
+	})
+	if err != nil {
+		return domain.Task{}, nil, err
+	}
+	if !created && before.ID == 0 && had {
+		return after, nil, nil // open and pinned already: nothing changed
+	}
+	return after, func(ctx context.Context) error {
+		if err := s.checkDay(today); err != nil {
+			return err
+		}
+		return s.store.InTx(ctx, func(x *store.Tx) error {
+			if err := unchangedSince(ctx, x, after); err != nil {
+				return err
+			}
+			if !had { // removing the pin (or the task) would take a block placed since
+				blocks, err := x.TaskBlocksOn(ctx, after.ID, today)
+				if err != nil {
+					return err
+				}
+				if len(blocks) > 0 {
+					return fmt.Errorf("task #%d is blocked today: %w", after.ID, ErrConflict)
+				}
+			}
+			if created {
+				return x.DeleteTask(ctx, after.ID) // its pin goes with it
+			}
+			if before.ID != 0 {
+				if err := x.UpdateTask(ctx, before); err != nil {
+					return err
+				}
+			}
+			if had {
+				return nil
+			}
+			return x.DeletePin(ctx, after.ID)
+		})
+	}, nil
+}
+
 // githubState reads the last fetch and the lane: the stored items without an
 // open task in snap (a pinned review is a candidate instead; a closed one
 // whose PR still requests me is back, D49).
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/app/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/app/`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/app/pinsource_test.go internal/app/sources.go
git commit -m 'feat(app): PinSource creates or reopens a review task, with undo (D20, D48, D49)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 5: The GitHub poll loop, `R`, the header status, auto-close feedback

The TUI side of D50: `calLoop` becomes `pollLoop` and serves both integrations; `syncGitHub` runs at startup, on every `ghTickMsg` of the current `seq` (every `[github] poll`) and on `R` (which now fetches the calendar and GitHub); a new day fetches only the calendar. The header gets `githubStatus` in D44's style (`⚠ gh: <reason>`, dim `gh 12m ago` after two missed polls, nothing while fresh or off). A sync that closed tasks says so (`✓ via GitHub: "Review #901: …"` or `✓ via GitHub: 2 reviews`), and Task detail shows `✓ via GitHub · Mon 12 Oct`. The harness gains `newGHHarness` (a `source.Fake`).

**Files:**
- Modify: `internal/tui/common.go`
- Modify: `internal/tui/detail.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/overlays.go`
- Create: `internal/tui/github_test.go`
- Modify: `internal/tui/harness_test.go`

**Interfaces:**
- Consumes: Task 3 (`SourceSetUp`, `SyncSources`, `State.GitHub`, `Config.GitHubPoll`), Task 4 (`PinSource`, in tests), Task 2 (`source.Fake`, `source.ErrNotLoggedIn`).
- Produces:
  - `pollLoop{syncing bool; seq int}` (was `calLoop`); `Model.gh pollLoop`; `ghTickMsg{seq int}`, `ghSyncedMsg{closed []domain.Task; err error}`; `func (*Model) syncGitHub() tea.Cmd`; `func (*Model) githubStatus() string`.
  - Test helpers: `newGHHarness(t, cfg)`, `h.src`, `startHarness(t, cfg, cal, src)`, `(*harness).reviewItem(repo string, n int, title, author string, ago time.Duration) domain.SourceItem`, `(*harness).setReviews(items ...domain.SourceItem)` (Set + `R`).

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/github_test.go`:

```go
package tui

import (
	"context"
	"strconv"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/source"
)

// reviewItem is a review request for PR n in repo (owner/name), requested
// ago before the harness clock.
func (h *harness) reviewItem(repo string, n int, title, author string, ago time.Duration) domain.SourceItem {
	return domain.SourceItem{Kind: "github", ExternalID: repo + "#" + strconv.Itoa(n), Number: n, Title: title,
		URL: "https://github.com/" + repo + "/pull/" + strconv.Itoa(n), Author: author, Repo: repo, RequestedAt: h.clock.now.Add(-ago)}
}

// setReviews makes items what GitHub reports, and fetches.
func (h *harness) setReviews(items ...domain.SourceItem) {
	h.t.Helper()
	h.src.Set(items, nil)
	h.keys("R")
}

func TestGitHubHeaderStatus(t *testing.T) {
	h := newHarness(t, config.Default())
	h.mustNotSee("gh") // GitHub off: no header text (D50)

	h = newGHHarness(t, config.Default())
	if n := h.src.Calls(); n != 1 {
		t.Fatalf("startup fetches GitHub once, got %d", n)
	}
	h.mustNotSee(" · gh", "⚠") // fresh: nothing

	h.src.Set(nil, source.ErrNotLoggedIn)
	h.keys("R")
	if n := h.src.Calls(); n != 2 {
		t.Fatalf("R fetches GitHub now, got %d fetches", n)
	}
	h.mustSee("tt · Today · Mon 12 Oct · cal: not set up (tt-cal calendars) · ⚠ gh: not logged in · gh auth login")

	h.src.Set(nil, nil)
	h.keys("R")
	h.mustNotSee("⚠")
	h.clock.now = h.clock.now.Add(10 * time.Minute) // two polls of 5m: still fresh
	h.mustNotSee(" · gh")
	h.clock.now = h.clock.now.Add(time.Minute)
	h.mustSee("(tt-cal calendars) · gh 11m ago")
}

func TestGitHubPollLoop(t *testing.T) {
	cfg := config.Default()
	cfg.GitHubPoll = 7 * time.Minute
	h := newGHHarness(t, cfg)
	var waits []time.Duration
	var ticks []ghTickMsg
	h.m.opts.Schedule = func(d time.Duration, msg tea.Msg) tea.Cmd {
		if tick, ok := msg.(ghTickMsg); ok {
			waits, ticks = append(waits, d), append(ticks, tick)
		}
		return nil
	}

	h.send(ghTickMsg{seq: h.m.gh.seq})
	if n := h.src.Calls(); n != 2 || len(ticks) != 1 || waits[0] != 7*time.Minute {
		t.Fatalf("a tick fetches and schedules the next one after the poll: %d fetches, waits %v", n, waits)
	}
	h.keys("R") // overtakes the scheduled tick
	if n := h.src.Calls(); n != 3 || len(ticks) != 2 {
		t.Fatalf("R: %d fetches, %d ticks scheduled", n, len(ticks))
	}
	h.send(ticks[0]) // the overtaken tick
	if n := h.src.Calls(); n != 3 {
		t.Errorf("an overtaken tick doesn't fetch again (one loop only): %d fetches", n)
	}
	h.send(ticks[1])
	if n := h.src.Calls(); n != 4 {
		t.Errorf("the current tick fetches: %d fetches", n)
	}

	h.m.gh.syncing = true // a fetch still running
	h.keys("R")
	if n := h.src.Calls(); n != 4 {
		t.Errorf("one fetch at a time: %d fetches", n)
	}
	h.m.gh.syncing = false

	h.clock.now = h.clock.now.Add(24 * time.Hour) // a new day doesn't fetch GitHub (reviews aren't per day)
	h.send(clockTickMsg{})
	if n := h.src.Calls(); n != 4 || h.m.banner == nil {
		t.Errorf("new day: %d fetches, banner %v", n, h.m.banner != nil)
	}
}

func TestAutoCloseSaysSo(t *testing.T) {
	h := newGHHarness(t, config.Default())
	ctx := context.Background()
	a := h.reviewItem("acme/api", 901, "Add idempotency key", "ana", time.Hour)
	b := h.reviewItem("acme/photos", 877, "Bump go", "tom", time.Hour)
	c := h.reviewItem("acme/backend", 915, "Lint config", "ion", time.Hour)
	h.setReviews(a, b, c)
	for _, i := range []domain.SourceItem{a, b, c} {
		if _, _, err := h.svc.PinSource(ctx, "github", i.ExternalID); err != nil {
			t.Fatal(err)
		}
	}
	h.setReviews(b, c)
	h.mustSee(`✓ via GitHub: "Review #901: Add idempotency key"`) // D49
	h.setReviews()
	h.mustSee("✓ via GitHub: 2 reviews")
	h.setReviews()
	h.mustNotSee("✓ via GitHub")

	h.keys("tab", "enter") // the first candidate's detail
	h.mustSee("✓ via GitHub · Mon 12 Oct")
}
```

Patch `internal/tui/harness_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/harness_test.go b/internal/tui/harness_test.go
index 00cfbe4..285b978 100644
--- a/internal/tui/harness_test.go
+++ b/internal/tui/harness_test.go
@@ -15,6 +15,7 @@ import (
 	"tt/internal/calendar"
 	"tt/internal/config"
 	"tt/internal/domain"
+	"tt/internal/source"
 	"tt/internal/store"
 )
 
@@ -31,6 +32,7 @@ type harness struct {
 	svc   *app.Service
 	clock *fakeClock
 	cal   *calendar.Fake // nil: no calendar set up
+	src   *source.Fake   // nil: GitHub isn't set up
 	path  string
 	quit  bool
 
@@ -43,17 +45,24 @@ type harness struct {
 
 func newHarness(t *testing.T, cfg config.Config) *harness {
 	t.Helper()
-	return startHarness(t, cfg, nil)
+	return startHarness(t, cfg, nil, nil)
 }
 
 // newCalHarness is newHarness with a fake calendar (h.cal), fetched once at
 // startup like the real one; Set its events, then R fetches again.
 func newCalHarness(t *testing.T, cfg config.Config) *harness {
 	t.Helper()
-	return startHarness(t, cfg, &calendar.Fake{})
+	return startHarness(t, cfg, &calendar.Fake{}, nil)
 }
 
-func startHarness(t *testing.T, cfg config.Config, cal *calendar.Fake) *harness {
+// newGHHarness is newHarness with a fake GitHub (h.src), fetched once at
+// startup like the real one; Set its review requests, then R fetches again.
+func newGHHarness(t *testing.T, cfg config.Config) *harness {
+	t.Helper()
+	return startHarness(t, cfg, nil, &source.Fake{})
+}
+
+func startHarness(t *testing.T, cfg config.Config, cal *calendar.Fake, src *source.Fake) *harness {
 	t.Helper()
 	path := filepath.Join(t.TempDir(), "tt.db")
 	st, err := store.Open(context.Background(), path)
@@ -66,7 +75,10 @@ func startHarness(t *testing.T, cfg config.Config, cal *calendar.Fake) *harness
 	if cal != nil {
 		svc.WithCalendar(cal)
 	}
-	h := &harness{t: t, svc: svc, clock: clock, cal: cal, path: path}
+	if src != nil {
+		svc.WithSource(src)
+	}
+	h := &harness{t: t, svc: svc, clock: clock, cal: cal, src: src, path: path}
 	opts := Options{WatchEvery: time.Second, ClockEvery: 30 * time.Second,
 		Schedule: func(time.Duration, tea.Msg) tea.Cmd { return nil },
 		Exec: func(c *exec.Cmd, done tea.ExecCallback) tea.Cmd {
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL (build): `undefined: ghTickMsg`, `h.m.gh undefined (type *Model has no field or method gh)`.

- [ ] **Step 3: Implement**

Patch `internal/tui/common.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/common.go b/internal/tui/common.go
index 8ea7f48..0dd4217 100644
--- a/internal/tui/common.go
+++ b/internal/tui/common.go
@@ -112,7 +112,7 @@ func (m *Model) headerView() string {
 	if m.state.Plan.Committed {
 		h += styleDim.Render(" · plan committed")
 	}
-	return h + m.calendarStatus()
+	return h + m.calendarStatus() + m.githubStatus()
 }
 
 // calendarStatus is the header's calendar part (D44): nothing while fresh,
@@ -131,6 +131,21 @@ func (m *Model) calendarStatus() string {
 	return ""
 }
 
+// githubStatus is the header's GitHub part (D50), worded like the
+// calendar's: nothing while fresh or when GitHub is off.
+func (m *Model) githubStatus() string {
+	gs, now := m.state.GitHub, m.svc.Now()
+	switch {
+	case !gs.SetUp:
+		return ""
+	case gs.Err != "":
+		return styleAlert.Render(" · ⚠ gh: " + gs.Err)
+	case !gs.LastSuccess.IsZero() && now.Sub(gs.LastSuccess) > 2*m.svc.Config().GitHubPoll:
+		return styleDim.Render(" · gh " + ago(now, gs.LastSuccess))
+	}
+	return ""
+}
+
 func (m *Model) bannerView() string {
 	b := m.banner
 	text := "☀ New day " + dayLabel(b.Day)
```

Patch `internal/tui/detail.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/detail.go b/internal/tui/detail.go
index 1e0ec09..b9b90c1 100644
--- a/internal/tui/detail.go
+++ b/internal/tui/detail.go
@@ -166,10 +166,12 @@ func (m *Model) detailHead() []string {
 			head += fmt.Sprint(c.PinnedDays)
 		}
 	}
-	switch t.Status {
-	case domain.StatusDone:
+	switch {
+	case t.Status == domain.StatusDone && t.ClosedBy == domain.ClosedBySource:
+		head += " ✓ via GitHub · " + dayLabel(m.svc.DayOf(t.ClosedAt)) // D20a
+	case t.Status == domain.StatusDone:
 		head += " ✓ " + dayLabel(m.svc.DayOf(t.ClosedAt))
-	case domain.StatusDropped:
+	case t.Status == domain.StatusDropped:
 		head += " ✗ " + dayLabel(m.svc.DayOf(t.ClosedAt))
 	}
 	lines := []string{head}
```

Patch `internal/tui/model.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/model.go b/internal/tui/model.go
index d297da3..74352c4 100644
--- a/internal/tui/model.go
+++ b/internal/tui/model.go
@@ -13,6 +13,7 @@ import (
 	tea "charm.land/bubbletea/v2"
 
 	"tt/internal/app"
+	"tt/internal/domain"
 	"tt/internal/search"
 )
 
@@ -86,7 +87,8 @@ type Model struct {
 	inbox  inboxState
 	detail detailState
 
-	cal calLoop
+	cal pollLoop // the calendar's
+	gh  pollLoop // GitHub's (D50)
 
 	overlay     overlay
 	banner      *app.DayResult
@@ -99,10 +101,11 @@ func New(ctx context.Context, svc *app.Service, opts Options) *Model {
 	return &Model{svc: svc, opts: opts, ctx: ctx, width: 100, height: 30, pick: pickState{group: search.ByPriority}}
 }
 
-// calLoop is the calendar's poll loop (D3): one fetch at a time, then the
-// next tick after the configured interval. R and a new day fetch at once;
-// seq retires the tick they overtook, so there is only ever one loop.
-type calLoop struct {
+// pollLoop is an integration's poll loop (D3): one fetch at a time, then the
+// next tick after the configured interval. R (and, for the calendar, a new
+// day) fetches at once; seq retires the tick it overtook, so there is only
+// ever one loop.
+type pollLoop struct {
 	syncing bool
 	seq     int
 }
@@ -140,6 +143,11 @@ type (
 	uiSavedMsg   struct{ err error }
 	calTickMsg   struct{ seq int }
 	calSyncedMsg struct{ err error }
+	ghTickMsg    struct{ seq int }
+	ghSyncedMsg  struct {
+		closed []domain.Task // tasks the fetch closed (D20a)
+		err    error
+	}
 )
 
 func (m *Model) Init() tea.Cmd {
@@ -151,6 +159,7 @@ func (m *Model) Init() tea.Cmd {
 		},
 		m.startDay(),
 		m.syncCalendar(), // the first load shows the cache meanwhile
+		m.syncGitHub(),
 		m.opts.Schedule(m.opts.WatchEvery, watchTickMsg{}),
 		m.opts.Schedule(m.opts.ClockEvery, clockTickMsg{}),
 	)
@@ -267,6 +276,22 @@ func (m *Model) update(msg tea.Msg) tea.Cmd {
 			m.setErr(fmt.Errorf("can't record the calendar fetch: %w", msg.err))
 		}
 		return tea.Batch(m.load(), m.opts.Schedule(m.svc.Config().CalendarPoll, calTickMsg{seq: m.cal.seq}))
+	case ghTickMsg:
+		if msg.seq == m.gh.seq {
+			return m.syncGitHub()
+		}
+	case ghSyncedMsg:
+		m.gh.syncing = false
+		m.gh.seq++
+		switch {
+		case msg.err != nil && !errors.Is(msg.err, context.Canceled):
+			m.setErr(fmt.Errorf("can't record the GitHub fetch: %w", msg.err))
+		case len(msg.closed) == 1:
+			m.setStatus(fmt.Sprintf("✓ via GitHub: %q", msg.closed[0].Title)) // D49
+		case len(msg.closed) > 1:
+			m.setStatus("✓ via GitHub: " + plural(len(msg.closed), "review", "reviews"))
+		}
+		return tea.Batch(m.load(), m.opts.Schedule(m.svc.Config().GitHubPoll, ghTickMsg{seq: m.gh.seq}))
 	case noteEditedMsg:
 		return m.noteEdited(msg)
 	case tea.KeyPressMsg:
@@ -323,7 +348,7 @@ func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
 	case "ctrl+z":
 		return m.undoLast()
 	case "R":
-		return tea.Batch(m.load(), m.syncCalendar())
+		return tea.Batch(m.load(), m.syncCalendar(), m.syncGitHub())
 	}
 	if m.dayIsStale() {
 		m.setStatus("new day — refreshing")
@@ -373,6 +398,20 @@ func (m *Model) syncCalendar() tea.Cmd {
 	return func() tea.Msg { return calSyncedMsg{err: svc.SyncCalendar(ctx)} }
 }
 
+// syncGitHub fetches the review requests in the background, unless GitHub
+// isn't set up or a fetch is already running.
+func (m *Model) syncGitHub() tea.Cmd {
+	if !m.svc.SourceSetUp() || m.gh.syncing {
+		return nil
+	}
+	m.gh.syncing = true
+	svc, ctx := m.svc, m.ctx
+	return func() tea.Msg {
+		closed, err := svc.SyncSources(ctx)
+		return ghSyncedMsg{closed: closed, err: err}
+	}
+}
+
 func (m *Model) startDay() tea.Cmd {
 	svc, ctx := m.svc, m.ctx
 	return func() tea.Msg {
```

Patch `internal/tui/overlays.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/overlays.go b/internal/tui/overlays.go
index 4eea4f9..61d2734 100644
--- a/internal/tui/overlays.go
+++ b/internal/tui/overlays.go
@@ -254,7 +254,7 @@ func (helpOverlay) view(m *Model) string {
 		"a        quick-add: title @who #tag !1 ~s link -- note (tab completes @/#)",
 		"ctrl+z   undo (refuses if the task changed since)",
 		"ctrl+d / ctrl+u   half a page down / up",
-		"R        reload and fetch the calendar now · q quit · ? this help",
+		"R        reload, and fetch the calendar and GitHub now · q quit · ? this help",
 	}
 	var local []string
 	switch m.screen {
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/common.go internal/tui/detail.go internal/tui/github_test.go internal/tui/harness_test.go internal/tui/model.go internal/tui/overlays.go
git commit -m 'feat(tui): GitHub poll loop, R fetches it too, header status, auto-close status (D49, D50)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 6: The Reviews lane on Today and in the Log; `w`, `p`, `o`

D49 in the TUI (`lane.go`): the lane sits under the candidates on Today (right column) and under the backlog in the Log (left column); rows `#901 api  Add idempotency key… @ana  3d`, longest waiting first; title `REVIEWS · N waiting` / `none waiting` / `not fetched yet`; at most 5 rows plus a blank line and the title, and the pane above keeps its title and 3 rows (the lane gives up rows first, then disappears). `w` focuses it; `w` or `tab` gives the focus back (tab otherwise stays between the two main panes); any screen key (`L`, `B`, `i`, `c`, `esc`, …) leaves the lane and keeps its meaning. In the lane: `j/k`, `ctrl+d/u`, `p` pins (status `pinned "Review #901: …" · P2 S`), `o` opens the PR; task keys say `a review request · p pins it, o opens it`. One cursor serves both screens and follows its request across reloads (same index when it left). Hints get `w reviews · ` while the lane has rows; help lists `w`. `badgeOf` and `openURL` are factored out for it.

**Files:**
- Modify: `internal/tui/common.go`
- Modify: `internal/tui/edit.go`
- Create: `internal/tui/lane.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/pick.go`
- Modify: `internal/tui/today.go`
- Create: `internal/tui/lane_test.go`

**Interfaces:**
- Consumes: Task 3 (`State.GitHub.Reviews`, `SetUp`, `LastSuccess`), Task 4 (`PinSource`), Task 5 (`newGHHarness`, `reviewItem`, `setReviews`), existing `pad`, `oneLine`, `steps`, `repeat`, `opener`.
- Produces: `laneState{focused bool; cursor, offset int; selected string}` with `refresh`, `move`, `follow`; `Model.lane`; `func (*Model) laneLines() int`, `laneUnder(pane) bool`, `laneView(width int) []string`, `reviewLine(item, width, selected) string`, `laneKey(key) (tea.Cmd, bool)`, `pinReview() tea.Cmd`, `laneHints() string`, `paneVisible(pane) int`, `timelineFocused() bool`; `func badgeOf(levels []config.PriorityLevel, p int, size domain.Size) string`; `func (*Model) openURL(link string) tea.Cmd`; `func waited(now, since time.Time) string`.

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/lane_test.go`:

```go
package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/app"
	"tt/internal/config"
	"tt/internal/domain"
)

// threeReviews fills the lane with three requests of different ages.
func threeReviews(h *harness) {
	h.setReviews(
		h.reviewItem("acme/photos", 877, "Bump go", "tom", 26*time.Hour),
		h.reviewItem("acme/api", 901, "Add idempotency key", "ana", 75*time.Hour),
		h.reviewItem("acme/backend", 915, "Lint config", "ion", 2*time.Hour),
	)
}

// lineWith is the screen line containing s ("" if none).
func (h *harness) lineWith(s string) string {
	for _, l := range strings.Split(h.screen(), "\n") {
		if strings.Contains(l, s) {
			return l
		}
	}
	return ""
}

func TestReviewsLaneOnToday(t *testing.T) {
	h := newGHHarness(t, config.Default())
	h.mustSee("REVIEWS · none waiting")
	threeReviews(h)
	h.mustSee("REVIEWS · 3 waiting", "#901 api  Add idempotency key", "@ana  3d", "#877 photos  Bump go", "@tom  1d", "@ion  2h")
	s := h.screen()
	if a, b, c := strings.Index(s, "#901"), strings.Index(s, "#877"), strings.Index(s, "#915"); !(a < b && b < c) {
		t.Errorf("longest waiting first (D49):\n%s", s)
	}
	if l := h.lineWith("REVIEWS"); !strings.Contains(l, "│ REVIEWS") {
		t.Errorf("the lane sits in the right column, under the candidates: %q", l)
	}
	h.mustSee("w reviews · j/k move") // the hint, before the timeline's

	h.keys("w")
	h.mustSee("j/k move · p pin · o open PR · w back")
	h.keys("j", "p") // #877
	h.mustSee(`pinned "Review #877: Bump go" · P2 S`, "REVIEWS · 2 waiting", "Review #877: Bump go")
	if got := h.m.lane.selected; got != "acme/backend#915" {
		t.Errorf("the cursor stays at the same index, now on #915: %q", got)
	}
	if l := h.lineWith("Review #877"); !strings.Contains(l, "[P2 S]") {
		t.Errorf("the review is a candidate with D48's defaults: %q", l)
	}
	h.keys("ctrl+z")
	h.mustSee(`undid pin "Review #877: Bump go"`, "REVIEWS · 3 waiting")

	h.keys("o")
	if len(h.opened) != 1 || h.opened[0] != "https://github.com/acme/backend/pull/915" {
		t.Errorf("o opens the PR under the cursor: %v", h.opened)
	}
	for _, k := range []string{"x", "enter", "b", "e", "d"} {
		h.keys(k)
		h.mustSee("a review request · p pins it, o opens it")
	}
	if n := len(h.m.state.Snapshot.Tasks()); n != 0 {
		t.Fatalf("task keys in the lane changed something: %d tasks", n)
	}

	h.keys("tab") // back to where w came from: the timeline
	if h.m.lane.focused || h.m.today.focus != focusTimeline {
		t.Errorf("tab leaves the lane for the timeline: lane %v, focus %v", h.m.lane.focused, h.m.today.focus)
	}
	h.keys("tab") // tab stays between timeline and candidates
	if h.m.lane.focused || h.m.today.focus != focusCandidates {
		t.Errorf("tab: lane %v, focus %v", h.m.lane.focused, h.m.today.focus)
	}
	h.keys("w", "w")
	if h.m.lane.focused || h.m.today.focus != focusCandidates {
		t.Errorf("w twice comes back to the candidates: lane %v, focus %v", h.m.lane.focused, h.m.today.focus)
	}
}

func TestReviewsLaneInTheLog(t *testing.T) {
	h := newGHHarness(t, config.Default())
	threeReviews(h)
	h.add("Write RFC intro !2 ~m")
	h.keys("w", "L") // a screen key leaves the lane and keeps its meaning
	if h.m.screen != screenLog || h.m.lane.focused {
		t.Fatalf("L from the lane: screen %d, lane focused %v", h.m.screen, h.m.lane.focused)
	}
	if l := h.lineWith("REVIEWS · 3 waiting"); !strings.HasPrefix(l, "REVIEWS") {
		t.Errorf("in the Log the lane sits in the left column, under the backlog: %q", l)
	}
	h.mustSee("w reviews · ⏎ open")
	h.keys("w", "p") // #901, the longest waiting
	h.mustSee(`pinned "Review #901: Add idempotency key" · P2 S`)
	if l := h.lineWith("Review #901"); !strings.Contains(l, "│") || strings.Index(l, "Review #901") < strings.Index(l, "│") {
		t.Errorf("the pinned review is a candidate (right), not in the backlog: %q", l)
	}
	h.keys("w", "esc") // esc keeps the Log's meaning: back to Today
	if h.m.screen != screenToday || h.m.lane.focused {
		t.Errorf("esc from the lane: screen %d, lane focused %v", h.m.screen, h.m.lane.focused)
	}

	// Done by hand while GitHub still asks: back in the lane (D49).
	h.keys("tab", "x")
	h.mustSee(`✓ "Review #901: Add idempotency key"`, "REVIEWS · 3 waiting")
	h.keys("w", "k", "p") // the cursor stayed on #877; #901 is back above it
	if tasks := h.m.state.Snapshot.Tasks(); len(tasks) != 2 || h.task(tasks[1].ID).Status != domain.StatusOpen {
		t.Errorf("p reopens the review's task: %+v", tasks)
	}
}

func TestReviewsLaneEmptyAndOff(t *testing.T) {
	h := newHarness(t, config.Default())
	h.mustNotSee("REVIEWS", "w reviews")
	h.keys("w")
	h.mustSee("GitHub is off · [github] enabled in the config")

	h = newGHHarness(t, config.Default())
	h.keys("w")
	h.mustSee("REVIEWS · none waiting", "no reviews waiting")
	h.mustNotSee("w reviews")
	if h.m.lane.focused {
		t.Error("an empty lane doesn't take the focus")
	}

	h.m.state.GitHub = app.GitHubState{SetUp: true, Err: "not logged in · gh auth login"}
	h.mustSee("REVIEWS · not fetched yet")

	// The lane's last request pinned: the focus goes back.
	h.setReviews(h.reviewItem("acme/api", 901, "Add idempotency key", "ana", time.Hour))
	h.keys("w", "p")
	if h.m.lane.focused {
		t.Error("the lane emptied by p gives the focus back")
	}
	h.mustSee("REVIEWS · none waiting")
}

func TestReviewsLaneHeight(t *testing.T) {
	h := newGHHarness(t, config.Default())
	var items []domain.SourceItem
	for n := range 7 {
		items = append(items, h.reviewItem("acme/api", 900+n, "PR", "ana", time.Duration(10-n)*time.Hour))
	}
	h.setReviews(items...)
	for n := range 6 {
		h.add("Candidate " + string(rune('A'+n)) + " !1 ~s")
	}
	for range 6 {
		h.keys("L", "p", "B", "esc") // pin the first backlog task, back to Today
	}
	if len(h.m.state.Plan.Candidates) != 6 {
		t.Fatalf("candidates = %d", len(h.m.state.Plan.Candidates))
	}
	h.mustSee("#904", "REVIEWS · 7 waiting")
	h.mustNotSee("#905") // five rows at most

	h.keys("w")
	for range 6 {
		h.keys("j")
	}
	h.mustSee("#906", "#902")
	h.mustNotSee("#901 ") // scrolled: the cursor stays in view

	h.send(tea.WindowSizeMsg{Width: 140, Height: 12})
	if lines := strings.Count(h.screen(), "\n") + 1; lines > 12 {
		t.Errorf("12 lines tall: the screen has %d", lines)
	}
	h.mustSee("REVIEWS · 7 waiting", "#906") // the lane gave up rows, kept the cursor
	if n := strings.Count(h.screen(), "@ana"); n != 3 {
		t.Errorf("a short column keeps 3 lane rows: %d", n)
	}
	if n := strings.Count(h.screen(), "Candidate "); n != 3 {
		t.Errorf("the candidates keep 3 rows above the lane: %d", n)
	}
	h.send(tea.WindowSizeMsg{Width: 140, Height: 8})
	h.mustNotSee("REVIEWS") // too short: the lane goes, the candidates stay
	h.mustSee("Candidate A")
}

func TestReviewRowsFitTheirColumn(t *testing.T) {
	h := newGHHarness(t, config.Default())
	h.setReviews(h.reviewItem("acme/api", 901, "A very long title "+strings.Repeat("that goes on ", 20)+"\x1b[31mred\ttab\nnewline", "ana", 3*24*time.Hour))
	l := h.lineWith("#901")
	if !strings.HasSuffix(strings.TrimRight(l, " "), "@ana  3d") || strings.ContainsAny(l, "\x1b\t") {
		t.Errorf("a long title is cut so the author and wait stay; control characters go: %q", l)
	}
	if w := ansi.StringWidth(l); w > 140 {
		t.Errorf("line is %d wide", w)
	}
	if got := waited(h.clock.now, h.clock.now.Add(time.Minute)); got != "now" {
		t.Errorf("a request from the future (clock skew) waits %q", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL (build): `h.m.lane undefined (type *Model has no field or method lane)`, `undefined: waited`.

- [ ] **Step 3: Implement**

Patch `internal/tui/common.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/common.go b/internal/tui/common.go
index 0dd4217..e7369b7 100644
--- a/internal/tui/common.go
+++ b/internal/tui/common.go
@@ -12,6 +12,7 @@ import (
 	"github.com/charmbracelet/x/ansi"
 
 	"tt/internal/app"
+	"tt/internal/config"
 	"tt/internal/domain"
 )
 
@@ -76,18 +77,22 @@ func byUse(counts map[string]int) []string {
 
 // badge renders "[P1 M]": the effective priority's configured name and the size.
 func (m *Model) badge(t domain.Task) string {
-	p := domain.EffectivePriority(t, m.state.Snapshot.ParentOf(t))
+	return badgeOf(m.svc.Config().Priorities, domain.EffectivePriority(t, m.state.Snapshot.ParentOf(t)), t.Size)
+}
+
+// badgeOf renders "[P1 M]" for priority level p of levels and size.
+func badgeOf(levels []config.PriorityLevel, p int, size domain.Size) string {
 	name := "—"
-	if levels := m.svc.Config().Priorities; p > 0 && p <= len(levels) {
+	if p > 0 && p <= len(levels) {
 		name = levels[p-1].Name
 	} else if p > 0 {
 		name = "P?"
 	}
-	size := "—"
-	if t.Size != domain.SizeNone {
-		size = strings.ToUpper(string(t.Size))
+	s := "—"
+	if size != domain.SizeNone {
+		s = strings.ToUpper(string(size))
 	}
-	return "[" + name + " " + size + "]"
+	return "[" + name + " " + s + "]"
 }
 
 // closedToday reports whether t was closed on the plan's day (D29 wins stay visible).
@@ -166,8 +171,16 @@ func (m *Model) statusView() string {
 }
 
 func (m *Model) hintsView() string {
+	lane := m.screen == screenToday || m.screen == screenLog
+	if lane && m.lane.focused {
+		return styleDim.Render(m.laneHints())
+	}
+	reviews := ""
+	if lane && len(m.reviews()) > 0 {
+		reviews = "w reviews · " // D49
+	}
 	if m.screen == screenToday {
-		return styleDim.Render(m.todayHints())
+		return styleDim.Render(reviews + m.todayHints())
 	}
 	if m.screen == screenInbox {
 		if t, ok := m.state.Snapshot.Get(m.inbox.current); ok && t.ParentID != 0 {
@@ -182,7 +195,7 @@ func (m *Model) hintsView() string {
 	if m.pick.filtered() && m.overlay == nil { // in the filter line, esc restores the previous filter
 		hints = "esc clears filter · " + strings.Replace(hints, " · esc today", "", 1)
 	}
-	return styleDim.Render(hints)
+	return styleDim.Render(reviews + hints)
 }
 
 func dayLabel(d domain.Day) string {
```

Patch `internal/tui/edit.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/edit.go b/internal/tui/edit.go
index b661490..a6e2020 100644
--- a/internal/tui/edit.go
+++ b/internal/tui/edit.go
@@ -235,7 +235,12 @@ func (m *Model) openLink(t domain.Task) tea.Cmd {
 		m.setStatus("no link")
 		return nil
 	}
-	open, ctx, link := m.opener(), m.ctx, t.Link
+	return m.openURL(t.Link)
+}
+
+// openURL opens link in the browser.
+func (m *Model) openURL(link string) tea.Cmd {
+	open, ctx := m.opener(), m.ctx
 	return func() tea.Msg {
 		if err := open(ctx, link); err != nil {
 			return actionMsg{err: fmt.Errorf("can't open %s: %w", link, err)}
```

Create `internal/tui/lane.go`:

```go
package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/domain"
)

// laneMaxRows is how many review requests the lane shows at once (D49).
const laneMaxRows = 5

// laneState is the Reviews lane (D49): GitHub's review requests that have no
// open task, under the candidates on Today and under the backlog in the Log.
// Both screens share one cursor. w focuses it; w or tab gives the focus back
// to the pane it came from, which never changed.
type laneState struct {
	focused  bool
	cursor   int
	offset   int    // first request shown
	selected string // external id under the cursor; survives reloads
}

func (m *Model) reviews() []domain.SourceItem { return m.state.GitHub.Reviews }

// refresh puts the cursor back on its request after a load, or on the same
// index when that request left the lane (pinned, or no longer requested).
func (l *laneState) refresh(m *Model) {
	items := m.reviews()
	if i := slices.IndexFunc(items, func(i domain.SourceItem) bool { return i.ExternalID == l.selected }); i >= 0 {
		l.cursor = i
	}
	l.cursor = max(0, min(l.cursor, len(items)-1))
	l.selected = ""
	if l.cursor < len(items) {
		l.selected = items[l.cursor].ExternalID
	}
	if len(items) == 0 {
		l.focused = false
	}
}

func (l *laneState) move(m *Model, delta int) {
	items := m.reviews()
	if c := l.cursor + delta; c >= 0 && c < len(items) {
		l.cursor, l.selected = c, items[c].ExternalID
	}
}

// follow keeps the cursor inside the lane's window. Called after every update.
func (l *laneState) follow(m *Model) {
	rows := m.laneLines() - 2
	if rows <= 0 {
		l.offset = 0
		return
	}
	l.offset = min(l.offset, l.cursor)
	if l.cursor >= l.offset+rows {
		l.offset = l.cursor - rows + 1
	}
	l.offset = max(0, min(l.offset, len(m.reviews())-rows))
}

// laneLines is how many lines the lane takes in its column: a blank line, its
// title and up to laneMaxRows requests. The pane above keeps its title and
// three rows, so in a short column the lane gives up rows first, then
// disappears; 0 when GitHub is off.
func (m *Model) laneLines() int {
	if !m.state.GitHub.SetUp {
		return 0
	}
	room := m.bodyHeight() - 4 - 2
	if room < 0 {
		return 0
	}
	return 2 + min(len(m.reviews()), laneMaxRows, room)
}

// laneUnder reports whether the lane sits under pane pn on this screen.
func (m *Model) laneUnder(pn pane) bool {
	return m.screen == screenToday && pn == paneToday || m.screen == screenLog && pn == paneBacklog
}

// laneView renders the lane's lines for a column width cells wide.
func (m *Model) laneView(width int) []string {
	n := m.laneLines()
	if n == 0 {
		return nil
	}
	gs := m.state.GitHub
	title := "REVIEWS"
	switch {
	case len(gs.Reviews) > 0:
		title += fmt.Sprintf(" · %d waiting", len(gs.Reviews))
	case gs.LastSuccess.IsZero():
		title += " · not fetched yet" // "none waiting" would be a guess
	default:
		title += " · none waiting"
	}
	lines := []string{"", styleTitle.Render(title)}
	for i := m.lane.offset; i < len(gs.Reviews) && len(lines) < n; i++ {
		lines = append(lines, m.reviewLine(gs.Reviews[i], width, m.lane.focused && i == m.lane.cursor))
	}
	return lines
}

// reviewLine renders `#901 api  Add idempotency key… @ana  3d`: the repo
// without its owner, the title cut to fit, the author and the wait.
func (m *Model) reviewLine(i domain.SourceItem, width int, selected bool) string {
	repo := i.Repo
	if _, name, ok := strings.Cut(repo, "/"); ok {
		repo = name
	}
	right := " @" + oneLine(i.Author) + "  " + waited(m.svc.Now(), i.WaitingSince())
	left := fmt.Sprintf("  #%d %s  %s", i.Number, oneLine(repo), oneLine(i.Title))
	line := pad(left, width-ansi.StringWidth(right)) + right
	if selected {
		return styleSelected.Render(pad(line, width))
	}
	return line
}

// waited renders a wait compactly: "now", "5m", "2h", "3d".
func waited(now, since time.Time) string {
	d := now.Sub(since)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
}

// laneKey handles w, and every key while the lane has the focus. ok is
// false for a key the screen should handle: it also gives the focus back,
// so L, B, i, c, esc and the rest keep their screen's meaning (D49).
func (m *Model) laneKey(key string) (cmd tea.Cmd, ok bool) {
	l := &m.lane
	if !l.focused {
		if key != "w" {
			return nil, false
		}
		switch {
		case !m.state.GitHub.SetUp:
			m.setStatus("GitHub is off · [github] enabled in the config")
		case len(m.reviews()) == 0:
			m.setStatus("no reviews waiting")
		default:
			l.focused = true
		}
		return nil, true
	}
	if n := m.steps(key); n != 0 {
		repeat(n, func(delta int) { l.move(m, delta) })
		return nil, true
	}
	switch key {
	case "w", "tab":
		l.focused = false
		return nil, true
	case "p":
		return m.pinReview(), true
	case "o":
		if l.cursor < len(m.reviews()) {
			return m.openURL(m.reviews()[l.cursor].URL), true
		}
		return nil, true
	case "enter", "x", "d", "D", "e", "E", "u", "b", "m", "r", "f", "+", "=", "-":
		m.setStatus("a review request · p pins it, o opens it")
		return nil, true
	}
	l.focused = false
	return nil, false
}

// pinReview pins the request under the lane's cursor (D49): its task is
// created, or reopened, and becomes a candidate.
func (m *Model) pinReview() tea.Cmd {
	if m.lane.cursor >= len(m.reviews()) {
		return nil
	}
	i := m.reviews()[m.lane.cursor]
	label := fmt.Sprintf("%q", domain.ReviewTitle(i))
	svc, ctx, levels := m.svc, m.ctx, m.svc.Config().Priorities
	return func() tea.Msg {
		t, undo, err := svc.PinSource(ctx, i.Kind, i.ExternalID)
		badge := strings.Trim(badgeOf(levels, t.Priority, t.Size), "[]")
		return actionMsg{label: "pin " + label, status: "pinned " + label + " · " + badge, undo: undo, err: err}
	}
}

func (m *Model) laneHints() string {
	return "j/k move · p pin · o open PR · w back · R refresh · ctrl+z undo · ? help"
}
```

Patch `internal/tui/model.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/model.go b/internal/tui/model.go
index 74352c4..ea7f674 100644
--- a/internal/tui/model.go
+++ b/internal/tui/model.go
@@ -87,6 +87,8 @@ type Model struct {
 	inbox  inboxState
 	detail detailState
 
+	lane laneState
+
 	cal pollLoop // the calendar's
 	gh  pollLoop // GitHub's (D50)
 
@@ -170,6 +172,7 @@ func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
 	m.pick.follow(m)
 	m.detail.follow(m)
 	m.today.follow(m)
+	m.lane.follow(m)
 	return m, cmd
 }
 
```

Patch `internal/tui/overlays.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/overlays.go b/internal/tui/overlays.go
index 61d2734..a601aa4 100644
--- a/internal/tui/overlays.go
+++ b/internal/tui/overlays.go
@@ -292,6 +292,7 @@ func (helpOverlay) view(m *Model) string {
 			"/        filter: @who #tag !1 ~s <7d >30d is:done|dropped|all words (live; esc on the list clears)",
 			"g        group by priority → size → created → requester (remembered)",
 			"B        done picking: block today's candidates in Today",
+			"w        the Reviews lane: p pins a review (a task Review #n), o opens the PR · w or tab back",
 			"esc      back to Today (when no filter is set) · i inbox",
 		}
 	}
```

Patch `internal/tui/pick.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/pick.go b/internal/tui/pick.go
index 194e478..d6b0b44 100644
--- a/internal/tui/pick.go
+++ b/internal/tui/pick.go
@@ -68,6 +68,7 @@ func (m *Model) refreshRows() {
 	m.pick.rows[paneToday] = m.todayRows()
 	m.pick.clamp(m)
 	m.today.refresh(m)
+	m.lane.refresh(m)
 	if len(m.detail.stack) > 0 {
 		m.detail.refresh(m)
 	}
@@ -256,8 +257,8 @@ func (m *Model) saveGroup() tea.Cmd {
 // only when the cursor leaves it; a group heading right above the cursor
 // comes along. Called after every update.
 func (p *pickState) follow(m *Model) {
-	visible := m.visibleRows()
 	for pn, rows := range p.rows {
+		visible := m.paneVisible(pane(pn))
 		c, off := p.cursor[pn], p.offset[pn]
 		top := c
 		if top > 0 && top < len(rows) && rows[top-1].kind == rowHeader {
@@ -275,6 +276,15 @@ func (p *pickState) follow(m *Model) {
 // panes, Today's candidates and timeline share this budget).
 func (m *Model) visibleRows() int { return max(m.bodyHeight()-1, 1) }
 
+// paneVisible is visibleRows for pane pn, less the Reviews lane when it
+// sits under pn on this screen (D49).
+func (m *Model) paneVisible(pn pane) int {
+	if m.laneUnder(pn) {
+		return max(m.visibleRows()-m.laneLines(), 1)
+	}
+	return m.visibleRows()
+}
+
 // steps is how far a list key moves the cursor: a row for j/k, half the
 // visible rows for ctrl+d/ctrl+u (as in vim); 0 for any other key.
 func (m *Model) steps(key string) int {
@@ -316,6 +326,9 @@ func (m *Model) selectedIn(pn pane) (row, bool) {
 }
 
 func (m *Model) pickKey(key string) tea.Cmd {
+	if cmd, ok := m.laneKey(key); ok {
+		return cmd
+	}
 	p := &m.pick
 	if n := m.steps(key); n != 0 {
 		repeat(n, p.move)
@@ -478,8 +491,9 @@ func (m *Model) pickView() string {
 		title += " · " + plural(n, "match", "matches") + " · " + m.pick.filterText // the count first: truncation eats the filter text
 		empty = "no matches · esc clears"
 	}
-	left := m.paneLines(paneBacklog, title, empty, lw, m.pick.focus == paneBacklog)
-	right := m.paneLines(paneToday, "TODAY'S CANDIDATES", "nothing pinned · p on a backlog task", rw, m.pick.focus == paneToday)
+	left := m.paneLines(paneBacklog, title, empty, lw, m.pick.focus == paneBacklog && !m.lane.focused)
+	left = append(left, m.laneView(lw)...)
+	right := m.paneLines(paneToday, "TODAY'S CANDIDATES", "nothing pinned · p on a backlog task", rw, m.pick.focus == paneToday && !m.lane.focused)
 	return columns(left, right, lw, rw)
 }
 
@@ -509,7 +523,7 @@ func (m *Model) paneLines(pn pane, title, empty string, width int, focused bool)
 	if len(rows) == 0 {
 		return append(lines, styleDim.Render("  "+empty))
 	}
-	visible := m.visibleRows()
+	visible := m.paneVisible(pn)
 	cursor := m.pick.cursor[pn]
 	start := m.pick.offset[pn]
 	for i := start; i < len(rows) && i < start+visible; i++ {
```

Patch `internal/tui/today.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/today.go b/internal/tui/today.go
index cd53c64..e9c1c69 100644
--- a/internal/tui/today.go
+++ b/internal/tui/today.go
@@ -405,6 +405,9 @@ func (m *Model) todayKey(key string) tea.Cmd {
 	if td.carry != nil {
 		return m.carryKey(key)
 	}
+	if cmd, ok := m.laneKey(key); ok {
+		return cmd
+	}
 	switch key {
 	case "c":
 		return m.commitPlan()
@@ -522,7 +525,8 @@ func (m *Model) todayView() string {
 	lw := total / 2
 	rw := total - lw
 	left := m.timelineLines(lw)
-	right := m.paneLines(paneToday, "CANDIDATES", "nothing pinned · L opens the log to pick", rw, m.today.focus == focusCandidates)
+	right := m.paneLines(paneToday, "CANDIDATES", "nothing pinned · L opens the log to pick", rw, m.today.focus == focusCandidates && !m.lane.focused)
+	right = append(right, m.laneView(rw)...)
 	return columns(left, right, lw, rw)
 }
 
@@ -572,7 +576,7 @@ func (m *Model) timelineRow(i, width int) string {
 	if r.outside {
 		b := r.blocks[0]
 		line := pad(" "+clock(r.start)+" ▓ "+m.blockText(b, true)+" · outside of working hours", width)
-		if td.focus == focusTimeline && i == td.cursor {
+		if m.timelineFocused() && i == td.cursor {
 			return styleSelected.Render(line)
 		}
 		return styleDim.Render(line)
@@ -598,12 +602,16 @@ func (m *Model) timelineRow(i, width int) string {
 	if now := td.tl.now; now >= 0 && r.start+rowLen <= now {
 		line = styleDim.Render(line) // the past
 	}
-	if td.focus == focusTimeline && i == td.cursor {
+	if m.timelineFocused() && i == td.cursor {
 		line = styleSelected.Render(line)
 	}
 	return line
 }
 
+// timelineFocused: the timeline has the focus, not its candidates or the
+// Reviews lane, so its cursor row shows.
+func (m *Model) timelineFocused() bool { return m.today.focus == focusTimeline && !m.lane.focused }
+
 // ghostRow renders a row the carried block would take, with what else is
 // there; it turns the error colour when the block doesn't fit there, the
 // alert colour (and ⚠) when it would overlap a meeting (D42).
@@ -724,4 +732,5 @@ var todayHelp = []string{
 	"e / E    edit the line / the note in $EDITOR · o open the link",
 	"c        commit the plan (again: refresh)",
 	"L        the log (backlog; pick today's work there, B comes back) · i inbox",
+	"w        the Reviews lane: p pins a review (a task Review #n), o opens the PR · w or tab back",
 }
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/common.go internal/tui/edit.go internal/tui/lane.go internal/tui/lane_test.go internal/tui/model.go internal/tui/overlays.go internal/tui/pick.go internal/tui/today.go
git commit -m 'feat(tui): the Reviews lane on Today and in the Log, w to focus it, p pins, o opens (D49)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---


## After the last task

- Build tt into the scratchpad and drive the real binary through a Python `pty` script on a throwaway database (`TT_DB`, `TT_CONFIG=/nonexistent`), with a fake `gh` first on `PATH` that prints a JSON file (`cat "$GH_FAKE_JSON"`): two requests (#901 `acme/api`, requested 2026-10-10; #877 `acme/photos`, no event). Expect `2 waiting`, `#901 api`, `@ana`, `#877 photos`; `esc` (banner), `w` → `p pin` in the hints, `p` → `pinned "Review #901` and `P2 S`; switch the file to only #877, `R` → `via GitHub`; `q`. Then the database: task 1 `Review #901: Add idempotency key`, priority 2, size `s`, done, `closed_by` `source`; `source_items` holds only `acme/photos#877`; `integration_state` `github` has a success and no error. Match short substrings (the renderer repaints changed cells only).
- `go test -race ./internal/tui/ ./internal/app/`.
- Final whole-branch review, one fix wave, park residuals as spec carry-forwards.
- **The user's check:** `make install`, open tt: the lane shows the PRs that request your review directly (`gh search prs --review-requested=@me` lists team requests too; tt shows only direct ones), `w` + `p` on one, `o` opens it in the browser.
