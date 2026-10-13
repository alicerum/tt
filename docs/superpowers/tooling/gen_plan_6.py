#!/usr/bin/env python3
"""Generate the Plan 6 doc from the verified scratch commits (main..HEAD).

Template for the next plan: copy it, replace TASKS (one entry per scratch commit:
title, intro, interfaces, step2/step4 text; a task without test files gets
"build" text instead of step2), HEADER and FOOTER.
Usage: gen_plan.py SCRATCH_REPO OUT_DOC"""
import subprocess, sys

REPO, OUT = sys.argv[1], sys.argv[2]
TRAILER = "Co-Authored-By: Claude <noreply@anthropic.com>"


def git(*args):
    return subprocess.run(["git", *args], cwd=REPO, check=True, capture_output=True, text=True).stdout


commits = git("rev-list", "--reverse", "main..HEAD").split()

TASKS = [
    dict(
        title="Source items: `domain.SourceItem`, migration 005, store lookups",
        intro="The data side of D20/D50: `domain.SourceItem` (a review request; `WaitingSince` is the request time, else when tt first saw it; `ReviewTitle` is D20b's `Review #n: <title>`), migration 005 `source_items` (the spec's Data row, trimmed to what is read), and the store calls the app needs: read and replace a kind's set, find the task materialized from a ref, list a kind's open linked tasks.",
        interfaces="""- Consumes: `store.Tx`, `querier`, `nullTime`/`parseNullTime`/`formatTime`/`parseTime` (existing).
- Produces:
  - `domain.SourceGitHub = "github"`; `domain.SourceItem{Kind, ExternalID string; Number int; Title, URL, Author, Repo string; RequestedAt, FirstSeenAt time.Time}` (`ExternalID` is `owner/repo#123`; `RequestedAt` zero = unknown).
  - `func (SourceItem) Ref() domain.SourceRef`, `func (SourceItem) WaitingSince() time.Time`, `func domain.ReviewTitle(SourceItem) string`.
  - `func (*store.Store) SourceItems(ctx, kind string) ([]domain.SourceItem, error)` and `func (*store.Tx) SourceItems(ctx, kind string)` (by external id); `func (*store.Tx) ReplaceSourceItems(ctx, kind string, items []domain.SourceItem) error` (the whole set; stores `kind`, not `items[i].Kind`).
  - `func (*store.Tx) TaskBySource(ctx, ref domain.SourceRef) (domain.Task, bool, error)` (open or closed); `func (*store.Tx) OpenSourceTasks(ctx, kind string) ([]domain.Task, error)` (by id).""",
        step2="Run: `go test ./internal/domain/ ./internal/store/`\nExpected: FAIL (build): `undefined: SourceItem`, `undefined: SourceGitHub`, `undefined: ReviewTitle`, `s.SourceItems undefined`, `x.ReplaceSourceItems undefined`.",
        step4="Run: `go test ./internal/domain/ ./internal/store/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`\nExpected: PASS (`TestNewerSchemaIsRefused` already finds the newest migration itself), no gofmt output.",
    ),
    dict(
        title="The `gh` adapter (`internal/source`) and a fake",
        intro="D50's adapter: one `gh api graphql` run (20s timeout; `--hostname` for Enterprise) with the search `is:open is:pr user-review-requested:@me archived:false`, first 100. `RequestedAt` is the newest `ReviewRequestedEvent` that names the viewer. More than 100 results fail the fetch, so a truncated set can never auto-close anything. Failures map to short reasons that say what to do. Tests drive fake `gh` scripts; nothing reaches GitHub.",
        interfaces="""- Consumes: Task 1 (`domain.SourceItem`, `domain.SourceGitHub`).
- Produces:
  - `source.GitHub{Host, Path string; Timeout time.Duration}` (`Path` "" = `gh` on `PATH`; `Timeout` 0 = `source.DefaultTimeout`, 20s); `func (*GitHub) Kind() string` (`"github"`); `func (*GitHub) Fetch(ctx) ([]domain.SourceItem, error)` (`FirstSeenAt` left zero: the app sets it; a deleted author is `ghost`).
  - Errors: `source.ErrGHNotFound` (`gh not found · brew install gh`), `source.ErrNotLoggedIn` (`not logged in · gh auth login`, gh exit 4), `source.ErrTooMany` (`over 100 review requests`), `gh timed out after 20s`, `gh printed bad JSON: …`, a GraphQL error's message, else the first line of gh's stderr without its `gh: ` prefix, else `gh failed: exit status N`; a cancelled ctx returns `ctx.Err()`. `source.MaxItems = 100`.
  - `source.Fake` (mutex-guarded): `Kind()`, `Set(items []domain.SourceItem, err error)`, `Fetch(ctx)`, `Calls() int`.""",
        step2="Run: `go test ./internal/source/`\nExpected: FAIL (build): `undefined: GitHub`, `undefined: ErrNotLoggedIn`, `undefined: ErrGHNotFound` (no non-test files yet).",
        step4="Run: `go test ./internal/source/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`\nExpected: PASS (~2s: the timeout test waits out `WaitDelay`), no gofmt output.",
    ),
    dict(
        title="`[github]` config, `SyncSources` (reconcile, auto-close), the lane state; `cmd/tt` wiring",
        intro="D48's review defaults and D50's `[github]` keys; `app.Source` (consumer side, as D45 did for the calendar), `WithSource`/`SourceSetUp`; `SyncSources` records a failed fetch and, only on success, replaces the stored set (keeping `first_seen_at`) and auto-closes every open linked task whose PR is gone (done, `closed_by = source`, `closed_at = updated_at = now`: the Plan 2 carry-forward), returning those tasks. `Load` returns `State.GitHub`: the lane (items with no open task, longest waiting first) and the fetch status. `cmd/tt` sets GitHub up unless `enabled = false`.",
        interfaces="""- Consumes: Task 1 (`SourceItems`, `ReplaceSourceItems`, `OpenSourceTasks`, `domain.SourceItem`), Task 2 (`source.Fake`, `source.ErrNotLoggedIn`, `source.ErrTooMany`, `source.ErrGHNotFound` in tests; `source.GitHub` in `cmd/tt`).
- Produces:
  - `config.Config.GitHub bool` (default true), `GitHubHost string`, `GitHubPoll time.Duration` (5m; at least 30s), `ReviewPriority int` (a level index; default the level named P2, else the last), `ReviewSize domain.Size` (default S; `""` = unsized). Errors: `github.poll: want a duration of 30s or more, like "5m", got "…"`, `github.priority: no priority level named "X"`, `github.size: want s, m, l, xl, or "" for unsized, got "…"`.
  - `app.Source interface { Kind() string; Fetch(ctx) ([]domain.SourceItem, error) }`; `func (*Service) WithSource(Source) *Service`; `func (*Service) SourceSetUp() bool`.
  - `func (*Service) SyncSources(ctx) ([]domain.Task, error)`: the tasks it closed; a failed fetch is recorded, not returned; a cancelled ctx returns `ctx.Err()` and records nothing.
  - `app.GitHubState{SetUp bool; Reviews []domain.SourceItem; LastSuccess time.Time; Err string}`; `app.State.GitHub`. `app.Store` gains `SourceItems(ctx, kind)`.""",
        step2="Run: `go test ./internal/config/ ./internal/app/ ./cmd/tt/`\nExpected: FAIL (build) in all three: `cfg.GitHub undefined (type Config has no field or method GitHub)`, `cfg.ReviewPriority undefined`; in app and cmd/tt `WithSource`, `SyncSources` and `GitHub` (the State field and `app.GitHubState`) undefined.",
        step4="Run: `go test ./internal/config/ ./internal/app/ ./cmd/tt/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/app/`\nExpected: PASS (`TestExampleConfigIsTheDefaults` covers the new `[github]` block), no gofmt output.",
    ),
    dict(
        title="`PinSource`: create or reopen a review's task, with undo",
        intro="D20/D48/D49's `p` on a review request, in one transaction: no task yet → create `Review #n: <title>` with the PR link, the source ref, the configured priority and size and no requester, and pin it; a closed task (done or dropped, by me or by GitHub) → reopen that same task (the unique source ref; it keeps its history and its own priority/size) and pin it; an open one → pin it. The undo deletes the created task or closes the reopened one exactly as it was; it refuses with `ErrConflict` once the task changed, a block was placed on it today, or the day moved on (integrity over undo).",
        interfaces="""- Consumes: Task 1 (`TaskBySource`, `SourceItems`), Task 3 (`srcService`, `review`, `mustSyncSources`, `ids` test helpers; `config.ReviewPriority`, `ReviewSize`), existing `unchangedSince`, `checkDay`, `pinned` (pins_test.go).
- Produces: `func (*Service) PinSource(ctx, kind, extID string) (domain.Task, Undo, error)`: the task as stored; `ErrNotFound` when the request isn't stored any more; a nil undo when the task was open and pinned already.""",
        step2="Run: `go test ./internal/app/`\nExpected: FAIL (build): `s.PinSource undefined (type *Service has no field or method PinSource)`.",
        step4="Run: `go test ./internal/app/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/app/`\nExpected: PASS, no gofmt output.",
    ),
    dict(
        title="The GitHub poll loop, `R`, the header status, auto-close feedback",
        intro="The TUI side of D50: `calLoop` becomes `pollLoop` and serves both integrations; `syncGitHub` runs at startup, on every `ghTickMsg` of the current `seq` (every `[github] poll`) and on `R` (which now fetches the calendar and GitHub); a new day fetches only the calendar. The header gets `githubStatus` in D44's style (`⚠ gh: <reason>`, dim `gh 12m ago` after two missed polls, nothing while fresh or off). A sync that closed tasks says so (`✓ via GitHub: \"Review #901: …\"` or `✓ via GitHub: 2 reviews`), and Task detail shows `✓ via GitHub · Mon 12 Oct`. The harness gains `newGHHarness` (a `source.Fake`).",
        interfaces="""- Consumes: Task 3 (`SourceSetUp`, `SyncSources`, `State.GitHub`, `Config.GitHubPoll`), Task 4 (`PinSource`, in tests), Task 2 (`source.Fake`, `source.ErrNotLoggedIn`).
- Produces:
  - `pollLoop{syncing bool; seq int}` (was `calLoop`); `Model.gh pollLoop`; `ghTickMsg{seq int}`, `ghSyncedMsg{closed []domain.Task; err error}`; `func (*Model) syncGitHub() tea.Cmd`; `func (*Model) githubStatus() string`.
  - Test helpers: `newGHHarness(t, cfg)`, `h.src`, `startHarness(t, cfg, cal, src)`, `(*harness).reviewItem(repo string, n int, title, author string, ago time.Duration) domain.SourceItem`, `(*harness).setReviews(items ...domain.SourceItem)` (Set + `R`).""",
        step2="Run: `go test ./internal/tui/`\nExpected: FAIL (build): `undefined: ghTickMsg`, `h.m.gh undefined (type *Model has no field or method gh)`.",
        step4="Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`\nExpected: PASS, no gofmt output.",
    ),
    dict(
        title="The Reviews lane on Today and in the Log; `w`, `p`, `o`",
        intro="D49 in the TUI (`lane.go`): the lane sits under the candidates on Today (right column) and under the backlog in the Log (left column); rows `#901 api  Add idempotency key… @ana  3d`, longest waiting first; title `REVIEWS · N waiting` / `none waiting` / `not fetched yet`; at most 5 rows plus a blank line and the title, and the pane above keeps its title and 3 rows (the lane gives up rows first, then disappears). `w` focuses it; `w` or `tab` gives the focus back (tab otherwise stays between the two main panes); any screen key (`L`, `B`, `i`, `c`, `esc`, …) leaves the lane and keeps its meaning. In the lane: `j/k`, `ctrl+d/u`, `p` pins (status `pinned \"Review #901: …\" · P2 S`), `o` opens the PR; task keys say `a review request · p pins it, o opens it`. One cursor serves both screens and follows its request across reloads (same index when it left). Hints get `w reviews · ` while the lane has rows; help lists `w`. `badgeOf` and `openURL` are factored out for it.",
        interfaces="""- Consumes: Task 3 (`State.GitHub.Reviews`, `SetUp`, `LastSuccess`), Task 4 (`PinSource`), Task 5 (`newGHHarness`, `reviewItem`, `setReviews`), existing `pad`, `oneLine`, `steps`, `repeat`, `opener`.
- Produces: `laneState{focused bool; cursor, offset int; selected string}` with `refresh`, `move`, `follow`; `Model.lane`; `func (*Model) laneLines() int`, `laneUnder(pane) bool`, `laneView(width int) []string`, `reviewLine(item, width, selected) string`, `laneKey(key) (tea.Cmd, bool)`, `pinReview() tea.Cmd`, `laneHints() string`, `paneVisible(pane) int`, `timelineFocused() bool`; `func badgeOf(levels []config.PriorityLevel, p int, size domain.Size) string`; `func (*Model) openURL(link string) tea.Cmd`; `func waited(now, since time.Time) string`.""",
        step2="Run: `go test ./internal/tui/`\nExpected: FAIL (build): `h.m.lane undefined (type *Model has no field or method lane)`, `undefined: waited`.",
        step4="Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`\nExpected: PASS, no gofmt output.",
    ),
]
assert len(TASKS) == len(commits), (len(TASKS), len(commits))

HEADER = """# tt Plan 6 — GitHub Reviews Lane Implementation Plan

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
"""

FOOTER = """
## After the last task

- Build tt into the scratchpad and drive the real binary through a Python `pty` script on a throwaway database (`TT_DB`, `TT_CONFIG=/nonexistent`), with a fake `gh` first on `PATH` that prints a JSON file (`cat "$GH_FAKE_JSON"`): two requests (#901 `acme/api`, requested 2026-10-10; #877 `acme/photos`, no event). Expect `2 waiting`, `#901 api`, `@ana`, `#877 photos`; `esc` (banner), `w` → `p pin` in the hints, `p` → `pinned "Review #901` and `P2 S`; switch the file to only #877, `R` → `via GitHub`; `q`. Then the database: task 1 `Review #901: Add idempotency key`, priority 2, size `s`, done, `closed_by` `source`; `source_items` holds only `acme/photos#877`; `integration_state` `github` has a success and no error. Match short substrings (the renderer repaints changed cells only).
- `go test -race ./internal/tui/ ./internal/app/`.
- Final whole-branch review, one fix wave, park residuals as spec carry-forwards.
- **The user's check:** `make install`, open tt: the lane shows the PRs that request your review directly (`gh search prs --review-requested=@me` lists team requests too; tt shows only direct ones), `w` + `p` on one, `o` opens it in the browser.
"""
LANG = {".go": "go", ".swift": "swift", ".sh": "sh", ".plist": "xml", ".sql": "sql"}


def fence(lang, body):
    return f"```{lang}\n{body.rstrip(chr(10))}\n```\n"


def files_of(c):
    out = []
    for line in git("diff", "--name-status", f"{c}^", c).splitlines():
        st, path = line.split("\t", 1)
        out.append((st, path))
    return out


def executable(c, path):
    return git("ls-tree", c, "--", path).split()[0] == "100755"


def block_for(c, st, path):
    if st == "A":
        lang = next((v for k, v in LANG.items() if path.endswith(k)), "")
        out = f"Create `{path}`:\n\n" + fence(lang, git("show", f"{c}:{path}"))
        if executable(c, path):
            out += f"\nMake it executable: `{path}` (`chmod +x {path}`).\n"
        return out
    return (f"Patch `{path}` (save the block to a file, then `git apply <file>` from the repo root):\n\n"
            + fence("diff", git("diff", f"{c}^", c, "--", path)))


doc = [HEADER]
for n, (c, task) in enumerate(zip(commits, TASKS), 1):
    fs = files_of(c)
    tests = [(s, p) for s, p in fs if p.endswith("_test.go")]
    impl = [(s, p) for s, p in fs if not p.endswith("_test.go")]
    subject = git("log", "-1", "--format=%s", c).strip()
    doc.append(f"### Task {n}: {task['title']}\n\n{task['intro']}\n\n**Files:**\n")
    for s, p in impl + tests:
        doc.append(f"- {'Create' if s == 'A' else 'Modify'}: `{p}`\n")
    doc.append(f"\n**Interfaces:**\n{task['interfaces']}\n\n")
    step = 1
    if tests:
        doc.append(f"- [ ] **Step {step}: Write the failing tests**\n\n")
        for s, p in tests:
            doc.append(block_for(c, s, p) + "\n")
        doc.append(f"- [ ] **Step {step + 1}: Run the tests to verify they fail**\n\n{task['step2']}\n\n")
        doc.append(f"- [ ] **Step {step + 2}: Implement**\n\n")
        step += 3
    else:
        doc.append(f"- [ ] **Step {step}: Create the files**\n\n")
        step += 1
    for s, p in impl:
        doc.append(block_for(c, s, p) + "\n")
    if tests:
        doc.append(f"- [ ] **Step {step}: Run the tests to verify they pass**\n\n{task['step4']}\n\n")
    else:
        doc.append(f"- [ ] **Step {step}: Build and check**\n\n{task['build']}\n\n")
    step += 1
    paths = " ".join(p for _, p in fs)
    assert "'" not in subject
    doc.append(f"- [ ] **Step {step}: Commit**\n\n" + fence("bash", f"git add {paths}\ngit commit -m '{subject}\n\n{TRAILER}'") + "\n---\n\n")
doc.append(FOOTER)
open(OUT, "w").write("".join(doc))
print("wrote", OUT)
