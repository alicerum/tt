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
| D7 | Size is an ordinal, non-linear scale with configurable durations (default S 1h · M ½d · L 1d · XL 1w) | Granular sizing doesn't work for staff work; rough buckets grow like story points |
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
| D11 | Untriaged items live in an **Inbox**: an item is in the Inbox while its effective priority or size is missing | Triage is a separate, fast step from capture; no extra "triaged" flag |
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
| D25 | Quick-add is one line with inline tokens: `@requester`, `#tag` (repeatable), `!<n>` priority, `~<size>`, a URL becomes the link; the rest is the title. Priority + size present → skips Inbox. Autocomplete on `@`/`#` | Capture must be the fastest action in the system |
| D25a | Two entry points sharing one parser: `a` in the TUI (any view) and `tt add "<line>"` from any shell. v1 CLI has only `add` | One syntax everywhere; the CLI is a scripting surface |
| D26 | Backlog grouping switchable: priority (default), size, created date, requester (no-requester group last; groups alphabetical). Within a group: oldest first. Last choice remembered | Different questions need different slices |
| D26a | Filter bar reuses the token DSL: `@ana #impl !1 ~l <7d schema`; remaining words fuzzy-match title/tags/requester | One DSL for capture and retrieval |
| D26b | Status tokens: open items by default; `is:done`, `is:dropped`, `is:all` widen. No separate Search view | Search over finished work without another screen |
| D27 | No separate Backlog view; Pick doubles as backlog browser/editor outside planning | One fewer view |
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
5. **Parent detail**: header, note, link; children with progress bar, done children kept with date; `a` repeating add-child input (token syntax), `J/K` reorder. *(Parent mockup)*
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
- Every command returns an **inverse command**, used by the TUI's undo stack.

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

---

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
- **P1 Edit**: `e` opens the token line prefilled with the task's current values; `E` opens the note in `$EDITOR`.
- **P2 Undo**: session-level undo stack (`ctrl+z`) built from inverse commands; not persisted.
- **P3 Done with future blocks**: blocks stay on the timeline, shown as done; `u` frees the time.
- **P4 Timeline**: visible hours configurable (default 08:00–19:00), 30m rows, scrolls; off-grid meetings show exact times in the label; overlapping meetings merge into one busy band with a count.
- **P5 Help**: `?` overlay listing keys for the current view.
- **P6 Non-working days**: no working-days config; the new-day banner plus "later" is enough.
- **P7 Backups**: rely on Time Machine for v1.

Resolved:
- ~~GitHub scope~~: direct review requests only (`user-review-requested:@me`); team requests not shown.
