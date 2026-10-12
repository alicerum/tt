# Personal Task Tracking (`tt`) — Design

Status: **design, in review**. Product decisions agreed; architecture section pending review; open items at the bottom.

## Problem

Asks reach me from many directions (mostly Slack, also 1:1s and PR review requests). While helping some people I silently drop others, and sometimes important work. Previous task lists failed because capture was too slow and grooming became a chore, so the list stopped being trusted.

## Goals

- Capture an ask in seconds, triage later.
- Fast grooming: triage the inbox with a few keystrokes.
- A morning ritual (~5 min): triage new asks, review leftovers, pick today's work, time-block it around meetings.
- One place on the laptop showing my own tasks, PR reviews waiting on me, and today's meetings.
- Make progress feel rewarding: small closable tasks, visible wins, a satisfying "plan done" moment. Priorities must be deliberate, not vibed.

## Non-goals

- Multi-user, sharing, collaboration.
- Jira integration.
- Due dates and reminders (I usually know the deadline myself).
- Phone access.
- Rule-based routing or auto-triage.
- **Writing anything to the calendar, ever.** The calendar is my contract with other people; time-blocking is my private space. Calendar adapters are read-only by design.

## Principles

- **Start minimal, add on friction.** No feature, field, or rule until real friction asks for it.
- **Only title is required at capture.** Everything else can wait for triage.
- **Nudge, never block.** The system recommends (split this, block children instead) but always lets me proceed.
- **Long-running, no server.** The TUI stays open (e.g. in a tmux session) and polls integrations on an interval, so nothing needs hosting.
- **Degrade, don't break.** Integration failures never block planning or time-blocking; the app shows last known state plus a small staleness/error indicator.

---

# Part 1 — Product

## Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | Single user, laptop-first (macOS) | Primary goal is having everything easily at hand on the laptop |
| D2 | TUI app, always open (e.g. in tmux) | Structured, keyboard-driven grooming, no deployment |
| D3 | Integrations (GitHub, Calendar) polled on a configurable interval (default: calendar 2m, GitHub 5m) plus a manual refresh key | No server or webhooks needed |
| D3a | Last successful integration state is cached; on fetch failure show last known state (or empty timeline) with a small warning icon | Offline / API errors must never block planning |
| D4 | PR reviews live in their own Reviews lane, outside triage | Similar size, urgency comes from who's waiting; triaging each one is busywork |
| D5 | Slack capture v1 = manual quick-add (paste link + title) | A Slack bot is possible but deferred until manual capture proves too slow |
| D6 | One level of nesting: parent → children | Big efforts need a home; deeper trees bring back grooming debt |
| D7 | Size is an ordinal, non-linear scale with configurable durations (default S 1h · M ½d = 4h · L 1d = 8h · XL 1w = 40h) | Granular sizing doesn't work for staff work; rough buckets grow like story points |
| D7a | No hard size cap; anything can go in any time slot | Working 2h on an XL is legitimate |
| D7b | PR reviews are unsized | Every review differs |
| D8 | **Free-form tags**, any number per item, optional (don't keep an item in Inbox). Tag input autocompletes from existing tags | Purpose is finding and filtering (project, type of work), not classification. Autocomplete limits sprawl |
| D8a | **Fuzzy search** over title, tags and requester is core, in-memory (no index); search can include done/dropped items | "Find that task someone mentioned last month" mostly hits titles; personal scale makes in-memory matching instant |
| D9 | Carry-over: all unfinished Today items (blocked or candidate) stay pinned as the next planning day's candidates; quick "already done" during Pick | Nothing silently disappears; leftovers are reviewed, not auto-scheduled |
| D10 | **Rollover counter**: +1 when a **time-blocked** task is unfinished at day end. Candidates never increment it. Lifetime, never resets. Always visible | Diagnostic: poorly defined, poorly sized, wrong priority, should be handed off. Big tasks accumulating rollovers is intended: it says "break this down" |
| D10a | High-rollover alert during Pick (threshold configurable, default 3), inline, prompting refine / split / reprioritize / hand off / drop | Turn the signal into a decision |
| D10b | Rollover is computed per *planning day* (days the app was used), not per calendar day | Weekends or days without the app don't inflate counts |
| D10c | No "working on this now" tracking; no per-block outcomes | Friction. Done-or-not at day end is enough signal |
| D10d | **Pinned-days counter**: planning days an item has sat pinned as a candidate. Resets on unpin, and when time-blocked (rollover takes over) | Surfaces "wanted to do it for ages, never got to it" separately from "committed time and didn't finish" |
| D11 | Untriaged items live in an **Inbox**: an item is in the Inbox while it is open, not source-linked, and its effective priority or size is missing | Triage is a separate, fast step from capture; no extra "triaged" flag |
| D11a | Triage is available as planning step 0 (skippable) and as a standalone view any time (count badge elsewhere). Items can be skipped and stay in Inbox indefinitely | Morning triage is most valuable, mid-day is useful; rough ideas may legitimately sit |
| D12 | Calendar is read-only and only feeds busy time into the timeline | Awareness, not calendar management |
| D12a | Planning = time-blocking. Today has two tiers: **blocks** (task on a time range, the commitment) and **candidates** (pinned, unslotted, "if time/energy allows") | If a task can't get time, it won't happen; candidates fill leftover gaps |
| D12b | No capacity math or planned-vs-free counter | The timeline shows empty space; numbers add nothing |
| D12c | A task can have multiple blocks per day; blocks can be any length regardless of size | Extending or repeating work on a task is normal |
| D12d | Planning flow: **Triage** (optional) → **Pick** → **Block** → commit plan | Prioritize new asks, choose what matters, then fit it into time |
| D13 | Storage: **SQLite (WAL mode)** behind a store interface. The TUI detects external writes (e.g. `tt add`) via `PRAGMA data_version` and reloads | Safe concurrent writers (TUI + CLI), queryable, easy to browse by hand, stats later for free |
| D13a | Locations: data `~/Library/Application Support/tt/tt.db`, config `~/.config/tt/config.toml` | Conventional locations |
| D14 | Priority: ordered levels (default P1/P2/P3), each with a written definition shown during triage; number of levels and definitions configurable | One-key triage; visible definitions prevent vibed priorities |
| D15 | User-tunable settings (priority levels, size durations, thresholds, intervals, day start, calendars) live in one config file | Single place to tweak as friction shows up |
| D16 | Requester is a separate optional property, not a tag; autocompleted; included in fuzzy search | Targets "dropping people"; usable in Today and future stats |
| D17 | Calendar access behind a `CalendarSource` interface; v1 adapter = macOS Calendar (EventKit), read-only | No OAuth/admin approval on a work laptop; Google Calendar API adapter can come later. Nothing outside the adapter depends on EventKit |
| D17a | Busy-time rules: included calendars configurable; all-day, declined and "free" events aren't busy; tentative is busy | Holidays/birthdays/OOO-style events shouldn't eat the timeline |
| D18 | A meeting that lands on an existing block: the block keeps its time and gets a ⚠ conflict marker; I re-slot with one key or leave it | Never silently reshuffle a deliberate plan |
| D19 | Configurable day-start time (default 04:00; 00:00 = midnight). At the boundary, rollover runs and a banner offers "New day: plan now? (Enter / later)". On "later", the timeline shows meetings only; planning can be started manually any time | Late-night work stays "today"; never force the ritual, never show yesterday's plan as current |
| D20 | **Source adapters** produce read-only **source items** (v1: GitHub PRs awaiting my review). Pinning one materializes a thin **task** with a `source_ref` and `link` | Synced, ephemeral data stays separate from my tasks; future sources plug into the same shape |
| D20a | Source-linked tasks auto-close ("✓ via GitHub") when the source says they're resolved (for PRs: I'm no longer a requested reviewer). Auto-closes never count as rollover. Re-request → reappears in the Reviews lane | No manual bookkeeping for what GitHub already knows |
| D20b | Wrapper defaults: title `Review #<n>: <PR title>`, link = PR URL, no size, **requester left empty**. The PR author is shown in the Reviews lane (source item data) but never copied into `requester`; I can set it manually | GitHub handles differ from how I name people; auto-filling would create duplicate requesters |
| D21 | Every task has at most one `link`; one key opens it | Context is one keystroke away |
| D22 | Re-planning is free: Pick/Block can be re-entered any time; rollover is evaluated at day end on whatever is blocked then. "Commit plan" is a finish-line moment (summary) that exits planning mode | Mid-day reprioritization is normal; rollover is an indicator, not an audit |
| D23 | Tasks end as **done** or **dropped**. Dropping asks for an optional one-line reason (Enter skips). Both stay searchable; only done counts as a win. Hard delete exists for mistakes | Reason captures "handed to @Marko" without a new state |
| D24 | Placing a task in a free gap creates a block of `min(size duration, gap length)`; unsized items default to 30m. `+`/`-` resize (default 30m steps); a fine-tune dialog sets exact start and minutes | Gives size a concrete job; fast default, precise when needed |
| D25 | Quick-add is one line with inline tokens: `@requester`, `#tag` (repeatable), `!<n>` priority, `~<size>`, a URL becomes the link; everything after ` -- ` becomes the note; the rest is the title. Look-alikes stay in the title: `#482` (issue ref), `~/tmp`, `this!`. Priority + size present → skips Inbox. Autocomplete on `@`/`#` | Capture must be the fastest action in the system; context is easiest to record at the moment of the ask |
| D25a | Two entry points sharing one parser: `a` in the TUI (any view) and `tt add "<line>"` from any shell. v1 CLI has only `add` | One syntax everywhere; the CLI is a scripting surface |
| D26 | Backlog grouping switchable: priority (default), size, created date, requester (no-requester group last; groups alphabetical). Within a group: oldest first. Last choice remembered | Different questions need different slices |
| D26a | Filter bar reuses the token DSL: `@ana #impl !1 ~l <7d schema`; remaining words fuzzy-match title/tags/requester | One DSL for capture and retrieval |
| D26b | Status tokens: open items by default; `is:done`, `is:dropped`, `is:all` widen. No separate Search view | Search over finished work without another screen |
| D26c | An active filter searches **all** tasks, Inbox and pinned ones included (existing badges mark them); results keep the current grouping, non-matches disappear | Filtering is "find that task"; a stable layout keeps the eye oriented while typing |
| D26d | Filter tokens: `@who` (repeats OR), `#tag` incl. inherited tags (repeats AND), `!n` effective priority and `~size` (repeats OR; `!n` out of range is an error), `<Nd`/`>Nd` created within / more than N days (or `w` weeks) ago, `is:…`. Any other word is fuzzy: a case-insensitive substring of title/tags/requester, or a subsequence anchored at a word start (`schm` → *schema*); every word must match. Look-alikes as in D25 stay words; a lone sigil (`@ # ! ~ < >`) is ignored (a token still being typed); `is:` with another value is an error | One predictable rule per token; anchored subsequences forgive abbreviations without matching noise |
| D26e | `/` opens the filter line prefilled with the current filter and narrows live; a parse error keeps the last valid results and refuses enter; `enter` keeps the filter, `esc` in the line restores the previous one, `esc` on the list clears it. Filters are never persisted. `g` cycles grouping (D26); created-date groups are age buckets: this week (<7d), this month (<30d), older (<90d), ancient. Priority grouping ends with `P?` then `unset`; size with `unsized` | Live feedback, no lost typing; a restart starts clean |
| D27 | No separate Backlog view; Pick doubles as backlog browser/editor outside planning | One fewer view |
| D29 | Notes are edited in the system editor: `E` suspends the TUI (Bubble Tea `ExecProcess`), opens the note as a temp markdown file in `$VISUAL` → `$EDITOR` → `vi`, and saves on exit. Unchanged file → no write; empty file → note cleared; non-zero editor exit → discard | My own editor and config beat any in-app modal; nothing to build or maintain |
| D30 | Timezone = the system timezone (Go `time.Local`); the standard `TZ` environment variable overrides it. No config option. The zone is read at process start, so an always-open TUI picks up a changed system zone (travel) only after a restart | System-wide is what the user wants; `TZ` already covers overrides |
| D28 | Stack: **Go + Bubble Tea/Lip Gloss**, pure-Go SQLite, plus a small **Swift helper** for EventKit | Strong TUI ecosystem, single fast binary, good typing for refactors; EventKit isolated behind `CalendarSource` |

## Item model

| Field | Required | Notes |
|-------|----------|-------|
| title | yes | Only field needed at capture |
| note | no | Free text |
| link | no | One URL (D21) |
| requester | no | D16 |
| tags | no | Free-form, multiple (D8) |
| priority | triage | Level index (D14). Children: inherited from parent, own value unused |
| size | triage | S/M/L/XL (D7) |
| parent | no | One level only (D6) |
| status | system | open / done / dropped |
| drop_reason | no | D23 |
| source_ref | system | Only on tasks materialized from a source item (D20) |
| rollover_count | system | D10 |
| created_at, closed_at | system | |

Plus, outside the task itself: **pins** (task is a Today candidate since day X → pinned-days counter), **blocks** (task, day, start, length), **days** (which planning days happened, when the plan was committed).

**Parent/child:**
- Children inherit priority and tags from their parent (no override in v1), so a child needs only a title and size and skips the Inbox.
- Any item can be pinned, parents included, without warning. A pinned parent shows in candidates with its open children indented beneath it (plus children done today, struck through); children can be blocked individually.
- Time-blocking a parent asks: "This is a parent task; time-blocking its children is recommended. Proceed?"
- When all children are done, the app suggests closing the parent.

## Motivation & feedback

- **Small leaves (nudge)**: items bigger than a day get a "consider splitting" hint when planned.
- **Done stays visible**: closed items remain in Today, struck through.
- **Day progress**: visible on the timeline (done blocks vs remaining).
- **Plan lock-in**: "commit plan" is the morning's finish line.
- **Day recap**: shown in the new-day banner ("Yesterday: 5 done · 2 carried").
- **No punishment mechanics**: no streaks or scores that can break.
- **Rollover as diagnostic** (D10, D10a).

## Views

1. **Today**: timeline (meetings, task blocks, free gaps, "now" line) beside candidates; Reviews lane docked below. *(Today mockup)*
2. **Pick** (planning phase 1, also the everyday backlog browser): backlog left with grouping (D26), filter bar (D26a/b) and Reviews lane; today's candidates right with counters and inline ⚠ rollover prompts. Keys: `p` pin, `u` unpin, `x` done, `d` drop, `e` edit, `/` filter, `Tab` pane, `⏎` → Block. *(Pick mockup)*
3. **Block** (planning phase 2): Today layout with a timeline cursor. Select candidate → `Tab` → cursor to gap → `⏎` places (D24); `+/-` resize, `f` fine-tune, `m` move, `u` unblock; ⚠ on conflicts (D18); calendar freshness indicator. Commit → summary → Today. *(Block mockup; validate in use)*
4. **Inbox**: one card at a time; priority definitions always visible; `1-3` priority, `s/m/l/x` size, `@`/`#`/`>` requester/tags/parent, `n` skip, `d` drop; auto-advance once priority + size are set (2 keystrokes). *(Inbox mockup)*
5. **Task detail** (`⏎` on any task): header, link, and a note preview (first few lines plus size, e.g. "+38 lines"); `E` opens the full note in `$EDITOR` (D29). Lists show a small marker on tasks that have a note. For parents, also children with progress bar, done children kept with date; `a` repeating add-child input (token syntax), `J/K` reorder. *(Parent mockup)*
6. **Quick-add**: overlay from any view (`a`), token syntax with autocomplete.

---

# Part 2 — Architecture (in review)

## Components

```
                ┌────────────── cmd/tt ──────────────┐
                │  tt          → TUI                  │
                │  tt add "…"  → one-shot capture     │
                └───────┬───────────────────┬─────────┘
                        │                   │
                  ┌─────▼─────┐             │
                  │ internal/ │             │
                  │   tui     │             │
                  └─────┬─────┘             │
                        │ commands/queries  │
                  ┌─────▼───────────────────▼─────┐
                  │        internal/app           │  Service: all rules that touch state
                  │  (commands, queries, StartDay,│  (single entry point for TUI + CLI)
                  │   Reconcile, snapshot)        │
                  └──┬──────┬──────────┬──────┬───┘
         ┌───────────┘      │          │      └─────────────┐
   ┌─────▼─────┐   ┌────────▼───┐  ┌───▼──────────┐  ┌──────▼───────┐
   │  store    │   │  calendar  │  │   source     │  │ dsl / search │
   │ (SQLite)  │   │ (EventKit  │  │  (GitHub)    │  │ (pure)       │
   └───────────┘   │  via helper)│ └──────────────┘  └──────────────┘
                   └────────────┘
            all of the above depend on internal/domain (pure types + rules)
```

| Package | Responsibility | Depends on |
|---------|----------------|------------|
| `internal/domain` | Entities (Task, Pin, Block, Day, SourceItem, Event) and pure rules: inbox rule, effective priority/tags, planning-day mapping, default block length, busy rules | nothing |
| `internal/dsl` | Token lexer/parser for quick-add (→ `TaskDraft`) and filter (→ `Filter`) | domain |
| `internal/search` | Filter evaluation + fuzzy matching over an in-memory snapshot; grouping/sorting | domain, dsl |
| `internal/store` | `Store` interface, SQLite implementation, versioned migrations | domain |
| `internal/calendar` | `CalendarSource` interface; `eventkit` adapter (runs Swift helper); fake | domain |
| `internal/source` | `Source` interface; `github` adapter; fake | domain |
| `internal/config` | Load TOML, apply defaults, validate | domain |
| `internal/app` | `Service`: commands, queries, day rollover, source reconciliation, integration state. Owns transactions | domain, store, calendar, source (interfaces only) |
| `internal/tui` | Bubble Tea root model, views, shared components (token input + autocomplete, timeline renderer) | app, search, dsl |
| `cmd/tt` | Wiring: config → adapters → Service → TUI or CLI | everything |
| `helpers/tt-cal` | Swift CLI: reads EventKit, prints JSON. No business rules | — |

Rule: only `app` mutates state; `tui` and `cmd` call `app`. Adapters never import `app`.

## Data (SQLite)

| Table | Columns | Notes |
|-------|---------|-------|
| `tasks` | id, title, note, link, requester, priority (nullable int), size (nullable), parent_id, position, status, drop_reason, source_kind, source_id, rollover_count, created_at, updated_at, closed_at, closed_by (`user`/`source`) | Unique (source_kind, source_id) when set. `position` orders children |
| `task_tags` | task_id, tag | PK (task_id, tag) |
| `pins` | task_id (PK), pinned_on (day) | Current candidates. Pinned-days = planning days in `days` since `pinned_on` (derived, not stored) |
| `blocks` | id, task_id, day, start_min, duration_min | History kept for stats |
| `days` | day (PK), started_at, committed_at | A row = a planning day happened |
| `source_items` | kind, external_id, title, url, author, context (e.g. repo), requested_at, updated_at, first_seen_at, last_seen_at | Mirrors the full "awaiting me" set from the last successful fetch; missing items are deleted. `requested_at` = when I was (last) requested, from GitHub, and drives "waiting 2d". `first_seen_at` is the fallback if GitHub returns no request event |
| `integration_state` | name (PK), last_success_at, last_error, last_error_at, payload (JSON) | Calendar cache lives in `payload` |
| `ui_state` | key (PK), value | Remembered grouping, etc. |
| `schema_migrations` | version, applied_at | From day one: the impact × urgency split will need one |

Hard delete cascades to tags, pins, blocks.

## Interfaces (sketch)

```go
type Clock interface{ Now() time.Time }

type CalendarSource interface {
    Events(ctx context.Context, from, to time.Time) ([]domain.Event, error)
}

type Source interface {
    Kind() string                                           // "github"
    Fetch(ctx context.Context) ([]domain.SourceItem, error) // items currently awaiting me
}

type Store interface {
    InTx(ctx context.Context, fn func(Tx) error) error
    DataVersion(ctx context.Context) (int64, error)
    LoadSnapshot(ctx context.Context) (domain.Snapshot, error)
}
// Tx: task/tag/pin/block/day/source_item/integration_state reads + writes.
```

`app.Service` surface:
- **Capture/edit**: `Add(line)`, `Update(id, patch)`, `SetParent`, `Reorder`, `Delete`.
- **Lifecycle**: `Done(id)`, `Drop(id, reason)`, `Reopen(id)`.
- **Planning**: `Pin`, `Unpin`, `PinSource(kind, extID)`, `Block(id, start, dur)`, `MoveBlock`, `ResizeBlock`, `Unblock`, `CommitPlan`.
- **Day**: `StartDay(now) (DayResult, error)`: idempotent; returns recap.
- **Integrations**: `SyncCalendar(ctx)`, `SyncSources(ctx)` (fetch + reconcile).
- **Read**: `Snapshot()`, `Today()`, `IntegrationStatus()`.
- Every command returns an **inverse command**, used by the TUI's undo stack. Exception: hard delete is not undoable; the TUI confirms it instead.

## Runtime

Bubble Tea gives one update loop; everything else is a `tea.Cmd` that returns a message. No shared mutable state between goroutines.

| Loop | Interval | Does | Message |
|------|----------|------|---------|
| DB watch | 1s | `DataVersion` changed (e.g. `tt add` from another shell) → reload snapshot | `SnapshotMsg` |
| Clock | 30s | Move "now" line; on day boundary → `StartDay` | `TickMsg`, `NewDayMsg` |
| Calendar | 2m (config) | `SyncCalendar`: run helper (10s timeout), cache on success, record error otherwise | `CalendarMsg` |
| GitHub | 5m (config) | `SyncSources`: fetch, and **only on success** reconcile in one tx (replace source items, auto-close linked tasks no longer awaiting me) | `SourcesMsg` |
| Manual | `R` key | Triggers calendar + GitHub immediately | as above |

Startup: load config → open DB, migrate → `StartDay(now)` → initial snapshot → start loops.

**Day rollover (`StartDay`)**, in one transaction, guarded by `days` PK (safe if TUI and CLI race):
1. `today = planningDay(now, day_start)`. If `days` has today → return.
2. `prev` = latest planning day before today (if any).
3. For each task with a block on `prev` and status open: `rollover_count += 1`, ensure pinned, `pinned_on = today`.
4. Remove pins of done/dropped tasks (their "done today" visibility ends).
5. Insert `days(today)`. Return recap for `prev`.

**Pinned-days reset**: unpin deletes the pin; blocking sets `pinned_on` to the block's day.

**Concurrency**: SQLite WAL, `busy_timeout` 5s. TUI and `tt add` are separate processes; both go through `app.Service`.

## Adapters

- **EventKit (`tt-cal`)**: `tt-cal events --from <iso> --to <iso> [--calendars …]` and `tt-cal calendars`. Prints raw events as JSON (title, start, end, all-day, availability, participation status, calendar). Busy rules (D17a) apply in Go, so the helper stays dumb. Embedded Info.plist with calendar usage description (required since macOS 14). Distinct exit code for "permission denied" → specific UI hint.
- **GitHub**: token from `gh auth token`, fallback `GITHUB_TOKEN`. Host configurable (GitHub Enterprise). **Full-set fetch every poll**: one search (GraphQL) for `is:open is:pr user-review-requested:@me archived:false`, returning title, number, repo, author, URL, plus the latest `ReviewRequestedEvent` targeting me from each PR's timeline (same query) for `requested_at`. Direct requests only; team requests are excluded. No incremental "only new" fetching: the question is "what's awaiting me *now*", and disappearance from the set is the auto-close signal (D20a), which an incremental fetch can't see without re-checking every known PR anyway. Volume is one request per 5m for a few dozen items, well under rate limits. The DB copy (`source_items`) serves offline display, linking, and "waiting since".

## Error handling

- Integration failures → `integration_state` + indicator; UI keeps last known data; never crash.
- Reconcile runs only after a successful fetch, so an outage can't mass-auto-close review tasks.
- DB errors → status-line message; UI stays responsive.
- `tt add`: non-zero exit + stderr message on failure; on success prints id and where it landed (Inbox / backlog).

## Testing

- `domain`, `dsl`, `search`: table-driven unit tests (parser golden cases, inbox/inheritance rules, planning-day mapping).
- `app`: scenario tests on a temp SQLite file with fake `Clock`, `CalendarSource`, `Source`: rollover across days and weekends, pinned-days reset, auto-close, no reconcile on fetch error, StartDay idempotency.
- `store`: migrations apply cleanly from empty and from each version.
- `tui`: a few `teatest` flow tests (two-keystroke triage, place a block).
- `tt-cal`: verified manually via the spike.

## First step: spike

Throwaway check before any real code: a Swift CLI launched from a tmux pane reads today's events from a Google account added to macOS Calendar (Internet Accounts). Confirms permission prompt, usage-description embedding, and sync latency.

**Result (2026-10-12): passed.** A `swiftc` binary with `NSCalendarsFullAccessUsageDescription` embedded via `-Xlinker -sectcreate __TEXT __info_plist` triggered one prompt for the terminal app; `requestFullAccessToEvents` then granted access. The Google account appears as a synced (CalDAV) source, not a subscription. Today's events took 9ms to fetch. `availability` and the current user's `participantStatus` are available per event (D17a). Birthdays, holiday and iCloud calendars also appear, so calendar selection in config is needed.

---

## Carry-forward from Plan 1 review

Must be handled in the named plan:

- *(Plan 2 Task 2)* **Plan 2: undo vs system writes.** Undo currently restores the whole previous row. Once `StartDay` (rollover) and GitHub auto-close write to tasks, undoing an earlier user edit would revert those system writes (e.g. drop a rollover count). Restore only the fields a command changed, or compare-and-set on `updated_at` and return a conflict.
- *(Plan 2 Task 1)* **Plan 2: consistent snapshot.** `LoadSnapshot` must read in one read-only transaction (modernc issues plain `BEGIN` for `ReadOnly`), before it also reads pins/blocks/days.
- *(Plan 2 Task 1)* **Plan 2: newer-DB guard.** Refuse to open a database whose highest `schema_migrations` version is newer than the binary's embedded migrations.
- **Deferred to the first table-rebuild migration: migrations with foreign keys.** Allow a migration to run with `foreign_keys=OFF` + `foreign_key_check`, so table rebuilds don't cascade-delete tags or null parents. Plan 2's migration only creates tables, so it isn't needed yet.
- *(Plan 2 Task 3: wall-clock day mapping)* **Plan 2: local time.** Stored times come back as UTC; planning-day mapping and "done today" must convert with `.In(time.Local)`.
- **Plan 3: children's priority.** Setting priority on a child has no effect (inherited, D6); the Inbox card for a child of an untriaged parent must say so or set the parent.
- **Plan 3: shrinking priority levels.** Tasks with a level above the configured count produce an edit prefill that `Edit` rejects; clamp or show as unset.
- **Plan 3: parser edge.** `@.` / `#.` (sigil + only punctuation) still parse as requester/tag `.`; they should stay in the title.
- **Plan 3: watch order.** Read `DataVersion` before `LoadSnapshot` so a write during the load triggers another reload.
- **Plan 3: `tt add` without shell quoting.** Consider `tt add` with no line reading one line from stdin or a prompt, so `!`, `~`, `#`, `?` need no quoting.
- Smaller: duplicate priority names aren't rejected; `tt --help` exits 2; undo-path tests for Drop/Reopen/SetNote/SetPriority/detach to add when the undo stack is wired.

## Carry-forward from Plan 2 review

Undo rule established in Plan 2: an undo returns `ErrConflict` when the planning day changed, the row changed since, or the row is gone. The TUI's undo stack treats `ErrConflict` as "can't undo" and drops the entry.

- **Plan 3: Block undo residual.** `Block`'s own undo still returns a raw `ErrNotFound` when the block (or its task) is already gone; map it to `ErrConflict` (or have the TUI treat both as "can't undo").
- **Plan 3: day freshness before commands.** `Pin`/`Block` work before `StartDay` has run for a new day. The TUI's clock tick must call `StartDay` at the boundary, and planning commands should check `Today()` against the day the view shows.
- **Plan 3: "done today".** Use `Service.DayOf(task.ClosedAt)` (converts stored UTC to the clock's zone) for wins and struck-through children.
- **Plan 3: one read for Pick.** `Plan()` discards the snapshot it loads; Pick also needs the backlog. Consider returning both from one read.
- **Plan 3: commit summary wording.** `CommitSummary.Blocked` includes closed tasks' blocks; `Candidates` counts only open unblocked ones. Word the summary accordingly.
- **Plan 5 (was Plan 4; the timeline lands in Plan 5): DST gaps on the timeline.** `Day.At` for wall-clock times inside a DST gap/overlap (e.g. 02:30 on 2026-03-29) is unspecified by Go; the timeline must render those rows deliberately.
- **Plan 5: auto-close must set `UpdatedAt = now`**, or compare-and-set undo won't notice it.
- Smaller tests to add when touched: block ending exactly at `day_start + 24h`; Unpin of a closed task; length guards before indexing slices in app planning tests.

Plan 3 status: Block undo residual, day freshness (watch tick + stale-key guard), "done today" via `DayOf`, and one read for Pick (`Service.Load`) are done.

## Carry-forward from Plan 3 review

- **Plan 4: overlays that parse tokens** (edit line, add-child, filter bar) validate synchronously and keep the text on error, like quick-add now does.
- **Plan 4: one placement rule for Pick.** When D26 grouping replaces the priority loop, keep "where a task shows" in one function (grouping + child placement, including orphaned children listed top-level with `↳ #parent`).
- **Plan 4: compute rows once per load** (and on expand/filter change) instead of on every render; the fuzzy filter would make per-render recomputation costly.
- **Plan 4: small leftovers.** All-dropped parent shows `0/0` (hide when total is 0); tab-complete ignores cursor position and completes inside a ` -- ` note; scroll window keeps the cursor on the bottom row when moving up; input width ignores resizes; multi-line paste could map lines 2+ to the note; capture `svc` in pickKey closures like `load()` does; `tt` open/ui error paths untested.
- **Plan 5: shared height budget** for Inbox, Pick and the timeline (Inbox has none; Pick's floor of 3 rows can overflow tiny terminals).
- **Plan 5: one real `tea.Program` test** once calendar/GitHub loops add true concurrency (the harness runs commands synchronously).
- Accepted, not planned: keys pressed before a reload lands act on the state shown (double `x` → two actions); the stale-day guard swallows one navigation key while the new day loads.

## Plan 4 split

Plan 4 ships in two slices, each merged on its own.

**Plan 4a — find & browse** (D26–D26e):
- `dsl.ParseFilter(line, levels) (Filter, error)`; `internal/search` (pure): `Match(filter, task, parent, now)` and grouping (`GroupOf(by, task, parent, now)`, group order and labels).
- **One placement function** in `tui` decides where each task shows. Unfiltered: as before (triaged, open, unpinned; children under expanded parents; orphans top-level with `↳ #parent`). Filtered: a match whose parent also matches is listed under it (parent auto-expanded to its matching children); a match whose parent doesn't match is top-level with `↳ #parent`. Grouping applies to top-level rows; children follow their parent.
- **Row cache**: both panes' rows are rebuilt on load, filter change, `g`, expand/collapse; render and keys only read them.
- **Shared token input** for quick-add and the filter bar: `@`/`#` completion at the cursor (not inside a ` -- ` note), width follows resizes.
- **Grouping remembered**: migration `003_ui_state.sql` (`ui_state(key PK, value)`), `Service.UIState`/`SetUIState`; not undoable; unknown values fall back to priority.
- Leftovers: hide `0/0` progress when all children are dropped; stable scroll window; capture `svc` in pickKey closures.

**Plan 4b — edit & detail**: Task detail (view 5) with children add (`a`, repeating) and `J/K` reorder (`Service.Reorder`), `e` edit line (P1), `E` note in `$EDITOR` (D29), open link (D21), `tt add` with no line reads stdin, multi-line paste maps lines 2+ to the note, `tt` open/ui error-path tests.

## UI tweaks backlog (from use; not yet scheduled)

Collected while using the TUI. The user wants these later, batched with other tweaks; don't schedule them unasked.

- **Tab skips an empty pane.** In Pick, `tab` doesn't move focus into a pane with no rows; it stays put and the status says why ("nothing pinned yet · p pins a task" / "backlog is empty"). If the focused pane becomes empty after a reload (e.g. unpinning the last candidate), focus moves back to the non-empty pane. Both empty: focus stays.
- **Visual polish.** Rounded Lip Gloss frames around each pane with the pane title in the top border; the focused pane's frame in an accent color, the other dim; a header bar and footer for date, inbox count and key hints; terminal ANSI colors (not hex) so it follows the user's theme. Frames cost 2 columns + 2 rows per pane, so pair this with the shared height budget (Plan 3 carry-forward).

## Deferred (add on friction)

- Slack bot / emoji-reaction capture
- Web frontend, possibly self-hosted with WebAuthn
- Phone app + sync (via server/CloudKit)
- Due dates
- Rule-based PR routing (e.g. own-team PRs skip triage)
- Priority override on children
- Splitting priority into impact × urgency (Eisenhower-style); needs a migration, accepted
- Search inside notes and link URLs
- Statistics / personal retro (data already captured: tags, blocks, closed dates, requesters)
- Notifications ("block starting")
- LLM access (adapter, Claude skill, or MCP server over `app`/DB)
- More CLI commands beyond `add`
- Google Calendar API adapter

## Open items

Proposed defaults (pending confirmation):
- **P1 Edit**: `e` opens the token line prefilled with the task's current values. Notes: see D29.
- **P2 Undo**: session-level undo stack (`ctrl+z`) built from inverse commands; not persisted.
- **P3 Done with future blocks**: blocks stay on the timeline, shown as done; `u` frees the time.
- **P4 Timeline**: visible hours configurable (default 08:00–19:00), 30m rows, scrolls; off-grid meetings show exact times in the label; overlapping meetings merge into one busy band with a count.
- **P5 Help**: `?` overlay listing keys for the current view.
- **P6 Non-working days**: no working-days config; the new-day banner plus "later" is enough.
- **P7 Backups**: rely on Time Machine for v1.

Resolved:
- ~~GitHub scope~~: direct review requests only (`user-review-requested:@me`); team requests not shown.
