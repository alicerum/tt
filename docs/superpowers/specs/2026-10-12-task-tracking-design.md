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
| D7b | ~~PR reviews are unsized~~ Amended by D48: reviews get a configurable default size (S) | Every review differs |
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
| D12d | Planning flow: **Triage** (optional) → **Pick** → **Block** (~~→ commit plan~~: dropped by D54) | Prioritize new asks, choose what matters, then fit it into time |
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
| D20b | Wrapper defaults: title `Review #<n>: <PR title>`, link = PR URL, default priority and size from config (D48; was "no size"), **requester left empty**. The PR author is shown in the Reviews lane (source item data) but never copied into `requester`; I can set it manually | GitHub handles differ from how I name people; auto-filling would create duplicate requesters |
| D21 | Every task has at most one `link`; one key opens it | Context is one keystroke away |
| D22 | Re-planning is free: Pick/Block can be re-entered any time; rollover is evaluated at day end on whatever is blocked then. ~~"Commit plan" is a finish-line moment (summary) that exits planning mode~~ (dropped by D54) | Mid-day reprioritization is normal; rollover is an indicator, not an audit |
| D23 | Tasks end as **done** or **dropped**. Dropping asks for an optional one-line reason (Enter skips). Both stay searchable; only done counts as a win. Hard delete exists for mistakes | Reason captures "handed to @Marko" without a new state |
| D24 | Placing a task in a free gap creates a block of `min(size duration, gap length)`; unsized items default to 30m. `+`/`-` resize (default 30m steps); a fine-tune dialog sets exact start and minutes | Gives size a concrete job; fast default, precise when needed |
| D25 | Quick-add is one line with inline tokens: `@requester`, `#tag` (repeatable), `!<n>` priority, `~<size>`, a URL becomes the link; everything after ` -- ` becomes the note; the rest is the title. Look-alikes stay in the title: `#482` (issue ref), `~/tmp`, `this!`. Priority + size present → skips Inbox. Autocomplete on `@`/`#` | Capture must be the fastest action in the system; context is easiest to record at the moment of the ask |
| D25a | Two entry points sharing one parser: `a` in the TUI (any view) and `tt add "<line>"` from any shell. v1 CLI has only `add` | One syntax everywhere; the CLI is a scripting surface |
| D26 | Backlog grouping switchable: priority (default), size, created date, requester (no-requester group last; groups alphabetical). Within a group: oldest first. Last choice remembered | Different questions need different slices |
| D26a | Filter bar reuses the token DSL: `@ana #impl !1 ~l <7d schema`; remaining words fuzzy-match title/tags/requester | One DSL for capture and retrieval |
| D26b | Status tokens: open items by default; `is:done`, `is:dropped`, `is:all` widen. No separate Search view | Search over finished work without another screen |
| D26c | The filter searches the **backlog only**: triaged tasks that aren't pinned (children of a pinned parent live in Today). Never the Inbox or Today's candidates. Status defaults to open; `is:` widens to closed backlog tasks (D26b). Results keep the current grouping, non-matches disappear | The backlog is groomed work ready to be taken; the Inbox has its own count and is groomed before planning. A stable layout keeps the eye oriented while typing |
| D26d | Filter tokens: `@who` (repeats OR), `#tag` incl. inherited tags (repeats AND), `!n` effective priority and `~size` (repeats OR; `!n` out of range is an error), `<Nd`/`>Nd` created within / more than N days (or `w` weeks) ago, `is:…`. Any other word is fuzzy: a case-insensitive substring of title/tags/requester, or a subsequence anchored at a word start (`schm` → *schema*); every word must match. Look-alikes as in D25 stay words; a lone sigil (`@ # ! ~ < >`) is ignored (a token still being typed); `is:` with another value is an error | One predictable rule per token; anchored subsequences forgive abbreviations without matching noise |
| D26e | `/` opens the filter line prefilled with the current filter and narrows live; a parse error keeps the last valid results and refuses enter; `enter` keeps the filter, `esc` in the line restores the previous one, `esc` on the list clears it. Filters are never persisted. `g` cycles grouping (D26); created-date groups are age buckets: this week (<7d), this month (<30d), older (<90d), ancient. Priority grouping ends with `P?` then `unset`; size with `unsized` | Live feedback, no lost typing; a restart starts clean |
| D27 | No separate Backlog view; Pick doubles as backlog browser/editor outside planning | One fewer view |
| D29 | Notes are edited in the system editor: `E` suspends the TUI (Bubble Tea `ExecProcess`), opens the note as a temp markdown file in `$VISUAL` → `$EDITOR` → `vi`, and saves on exit. Unchanged file → no write; empty file → note cleared; non-zero editor exit → discard; changed text stays in the temp file, whose path the status shows | My own editor and config beat any in-app modal; nothing to build or maintain |
| D29a | A note edit refuses when the note changed elsewhere while the editor was open ("note changed since you opened it"); the edited text stays in the temp file, whose path the status shows | Never overwrite silently, never lose typing |
| D30 | Timezone = the system timezone (Go `time.Local`); the standard `TZ` environment variable overrides it. No config option. The zone is read at process start, so an always-open TUI picks up a changed system zone (travel) only after a restart | System-wide is what the user wants; `TZ` already covers overrides |
| D31 | `⏎` on any task opens Task detail; Pick → Block gets its own key, `B` (Plan 5) | Opening a task is the common action while browsing; Block is a once-a-day step |
| D32 | Children never take `!n`: the edit line of a child leaves priority out, and `!n` in a child's edit or add-child line is refused ("children inherit priority from #parent") | A stored-but-unused value is confusing (D6) |
| D33 | `tt add` with no line: from a terminal it prompts `add › ` on stderr and reads one line (no shell quoting); a multi-line paste there behaves like a pipe: line 1 is the token line, lines 2+ already pending become the note, and nothing is left on the terminal for the shell (`TIOCFLUSH`); from a pipe it reads all of stdin (over 1 MiB fails), line 1 = token line, lines 2+ = note (appended after the line's own ` -- ` note). The same rule maps a multi-line paste in quick-add / add-child | Capture without quoting `!` `~` `#`; pasting a Slack message keeps its body |
| D34 | **A pinned parent pins its children.** Pinning a parent pins all its open children; a child added to or reopened under a pinned parent is pinned too. Unpinning the parent unpins all its children. A child can be pinned on its own; once its parent is pinned, unpinning the child is refused ("unpin the parent first"). Pinning or unpinning a task that has children is **not undoable** (the status says so; `u`/`p` reverses it by hand); pin/unpin of a childless task keeps its undo. Unpinning a parent leaves children that have blocks today pinned on their own (`unpinned "X" + 2 children · 1 blocked child kept · no undo`), so no block is lost without undo; unpinning a parent that has blocks of its own today is refused ("unblock it on the timeline first"). A parent pinned before D34 whose open children aren't pinned is repaired by `p` on it | One easy rule: what's under a pinned parent is pinned. Today "pin parent, then unpin a child" says the child isn't pinned, which is confusing |
| D35 | **Today is the home screen** (`tt · Today`): timeline left, candidates right, `tab` switches. The Pick screen is renamed **Log** (`tt · Log`; the planning step is still called Pick): `L` from Today opens it, `esc` returns; `B` in Log goes to Today (end of Pick). `i` opens Inbox, `esc` returns to the screen it came from. The new-day banner's `⏎` starts planning: Inbox (if anything is waiting) → Log → `B` → Today. No separate planning mode: block keys always work on Today (D22) ~~and `c` always commits (committing again refreshes the time); the summary shows in the status line~~ (no commit step since D54) | Opening the app should show today's plan; the backlog is for planning or a deliberate look |
| D36 | **Placement by pick-up and drop**: `b` on a candidate picks it up and moves the cursor to the first free timeline row from now; `j/k` move a ghost block of D24 length (min(size, free time from the cursor to the next block or the planning day's end)); `⏎` drops it, `esc` cancels. `m` on a block picks it up keeping its length. Outside a pick-up, `⏎` opens Task detail (D31). Dropping onto another block is refused ("overlaps …"); placing in the past is allowed. On a block: `+/-` resize by 30m, `f` one-line fine-tune prefilled `09:00 1h` (`HH:MM` + duration such as `45m`, `1h30m`; one undoable step), `u` unblocks (task stays a candidate), `x d e E o ⏎` act on its task. On a candidate `u` unpins | See where a block lands before committing to it; `⏎` keeps one meaning |
| D37 | **Planning nudges**: placing an L or XL task says "big task · consider splitting"; picking up a parent (`b`) asks "parent task; blocking its children is recommended · ⏎ block anyway · esc" (before positioning, so esc wastes nothing) | Small leaves are the motivation engine; nudge, never block |
| D38 | **Timeline**: the working hours (D39), 30m rows, scrolls within the shared height budget. A block's first row shows `▓ title [P S]`, later rows `▓`; a block off the half-hour grid shows exact times (`▓ 09:10–09:55 title`); a row holding two short blocks shows both titles and keys act on the earlier one. Free rows are blank. Blocks of closed tasks stay, struck through with ✓ (P3); the title shows `TIMELINE · 2/5 done` (blocks whose task is done). A `━ now 10:12 ━` line sits between rows, earlier rows are dimmed, the cursor starts on the current row. Rows are wall-clock labels like stored blocks: a row that doesn't exist on a DST night is still drawn and works; "now" uses the current wall-clock time | One predictable grid; precise times only where they matter |
| D39 | **Working hours** (`[work_hours] start = "08:00"`, `end = "19:00"`, within the planning day): Today's timeline shows only them, and the service refuses any block outside them (place, move, resize, fine-tune: "outside of working hours (08:00–19:00)"); undo still restores what it removed. When now is outside the hours there is no now line; the timeline title says "outside of working hours" and the cursor rests on the last working row after them (the first before them). A block already outside the hours (placed before D39, or the hours changed) keeps a dimmed row of its own (`▓ 21:00–22:00 Title [P S] · outside of working hours`) so it can be moved in or unblocked. Replaces D38's stretching window and 5a's "j/k grow the timeline" | Time-blocking is for work time; past midnight the stretched timeline was noise. A deliberate exception to "nudge, never block": a boundary, not a nag |
| D40 | **Calendar setup**: `[calendar] calendars = ["…"]` names the calendars whose events count as busy (D17a); `poll = "2m"` (D3). `tt-cal calendars` prints the names to copy. No calendars set → nothing is fetched and the header says `cal: not set up` (pointing to `tt-cal calendars`). `helpers/tt-cal/build.sh` builds the helper (swiftc + embedded usage description) into `~/.local/bin/tt-cal`; tt runs the `tt-cal` next to its own binary, else the one on `PATH` | One-time setup; birthdays/holidays/iCloud never sneak in; no new tt CLI command |
| D41 | **Meetings on Today**: today's busy events within the working hours (D39), read-only. First row `■ Standup` (exact times when off the half-hour grid, like blocks), then `■`; overlapping meetings merge into one band `■ Standup +1` (P4). A row with a block and a meeting shows the block first: `▓ Fix CI [P1 S] ⚠ · ■ Standup`. Meetings fully outside the working hours aren't shown. Task keys on a meeting-only row say `meeting · read-only` | Awareness of the day's fixed points while blocking (D12) |
| D42 | **Meetings are busy for placement**: `b` starts on the first row free of blocks and meetings; a ghost's D24 length stops at the next meeting. Dropping over a meeting is allowed: the block gets ⚠ and the status says `blocked "X" 10:00–11:00 · ⚠ overlaps meeting "Standup" 10:00–10:30`. Any block overlapping a meeting shows ⚠, whether the meeting came first or later (D18); the timeline title counts them (`· ⚠ 1`). Blocks still never overlap each other (D36) | Nudge, never block: sometimes a meeting is skipped. D18's marker is the same rule seen from the other side |
| D43 | **`r` re-slots a ⚠ block** (D18): it picks the block up like `m`, with the ghost on the next free slot after the block that fits its whole length inside the working hours (no block, no meeting); `⏎` drops, `j/k` adjust, `esc` leaves it. No slot: `no free slot left today fits 1h` and the ghost stays on the block. On a block without a conflict: `no conflict · m moves it` | See where it goes before it moves; same pick-up model as D36 |
| D44 | **Calendar status** in the header: nothing while fresh; dimmed `cal 12m ago` once the last success is older than two polls; `⚠ cal: <short reason>` after a failed fetch, keeping the last good meetings (D3a): `tt-cal not found · run helpers/tt-cal/build.sh`, `no access · System Settings → Privacy & Security → Calendars` (helper exit 2), `unknown calendar "X"` (exit 3), else the first line of the helper's stderr (also on the 10s timeout or bad JSON). A cached day that isn't today is ignored | Degrade, don't break; every failure says what to do |
| D45 | **Plan 5b rulings** (made during execution, kept after review): ⚠ and the `· ⚠ N` count are for open tasks' blocks only (a closed task's block is history; amends D42's "any block"); `r` searches from the later of the row after the block's start and the now row, so it may overlap the block's own old span but never lands in the past (amends D43; `j/k` still reach the past, D36); `CalendarSource` is defined in `app` (consumer side; `app` never imports `internal/calendar` outside tests); `[calendar] poll` is at least 10s; without calendars the header says `cal: not set up (tt-cal calendars)`; `tt-cal calendars` prints unique names only; meeting titles are made one clean line (control characters become spaces) before rendering; while the permission prompt waits, the helper says `waiting for calendar access · answer the macOS prompt` on stderr, which a timeout reports (D44) | Use first; each is a one-line reversal if it chafes |
| D46 | **The now line sits above the row holding now** (amends D38): the row now falls in comes right after the line, and the line moves down only once that 30-minute row has passed; scrolling onto the now row brings the line into view with it | Reads as "now, then what's next"; from use |
| D47 | **`ctrl+d` / `ctrl+u` move half a page** (half the visible rows, at least 1) wherever `j`/`k` move a cursor: the Log's panes, Today's timeline and candidates, a picked-up block, Task detail. Repeats the one-row move, so group headers are skipped and the ends stop it; text inputs keep their own keys | Long lists (vim muscle memory); from use |
| D48 | **Review defaults** (amends D7b, D20b): `[github] priority = "P2"` (a `[[priorities]]` name, checked at load; unset and no level named P2 → the last level) and `size = "s"` (`s/m/l/xl`, or `""` for unsized). Applied when pinning a review creates its task; a reopened task keeps its own values; `e` changes them per task. A review block's default length follows (S = 1h, D24) | From the user: most reviews are small and mid-priority; skipping per-review triage is D4's point |
| D49 | **Reviews lane** (D4; which requests it lists: amended by D51). Today: under the candidates (right column); Log: under the backlog (left column). Shows review requests with no open linked task, longest waiting first (amended by D60: newest first); a row is `#901 api  Add idempotency… @ana  3d` (repo without owner; wait from `requested_at`, else `first_seen_at`). At most 5 rows (amended by D59: up to a third of the body) plus its title and a blank line above; in a short column the lane gives up rows first, down to its title. Title: `REVIEWS · 4 waiting`, `REVIEWS · none waiting`, or `REVIEWS · not fetched yet` (no successful fetch and no cache). **`w` focuses the lane**; `w` or `tab` returns to the pane it came from; `tab` keeps switching only between the two main panes (timeline ↔ candidates, backlog ↔ candidates); `esc` keeps its screen meaning; `w` on an empty lane says `no reviews waiting`. Keys: `p` pins (status `pinned "Review #901: …" · P2 S`; undoable), `o` opens the PR, `j/k`, `ctrl+d/u`; other task keys say `a review request · p pins it, o opens it`. Hints: `p pin · o open PR · w back`; elsewhere `w reviews` while the lane has rows. A pinned review leaves the lane while its task is open; when that task is closed (by me or by GitHub) and the PR still requests me, it is back in the lane, and `p` reopens the same task (unique source ref; keeps its history). No hide or snooze: to decline, remove myself as reviewer on GitHub. The Log filter doesn't apply to the lane. When a sync auto-closes tasks the status says `✓ via GitHub: "Review #877: …"` (or `✓ via GitHub: 2 reviews`); Task detail's header shows `✓ via GitHub · Tue 13 Oct` | GitHub is the only truth for "awaiting me"; one item, one place; tab stays the two-pane switch from use |
| D50 | **GitHub adapter and sync.** The adapter runs `gh api graphql` (`gh` handles login, SSO, `GH_TOKEN`/`GITHUB_TOKEN`, and `--hostname` for Enterprise), 20s timeout. One search, first 100: `is:open is:pr user-review-requested:@me archived:false` with `issueCount`, `viewer.login`, and per PR number, title, URL, author, repo and the last 10 `ReviewRequestedEvent`s; `requested_at` = the newest event naming the viewer. **`issueCount` over 100 is a failed fetch**, so a truncated set never auto-closes anything. Errors (header, D44 style with `gh`): `gh not found · brew install gh`, `not logged in · gh auth login` (gh exit 4), `over 100 review requests`, else the first line of gh's stderr (also on timeout or bad JSON); nothing while fresh, dim `gh 12m ago` after two missed polls. Config `[github] enabled = true` (false: no fetch, no lane, no header text), `host = ""`, `poll = "5m"` (at least 30s). Sync on success, one tx: replace `source_items` (new ones get `first_seen_at = now`, existing keep it), auto-close each open linked task missing from the set (done, `closed_by = source`, `closed_at = updated_at = now`; pin and blocks stay, so Today shows it struck through until StartDay clears it), no auto-reopen. `R` fetches calendar and GitHub; a new day fetches only the calendar. The `Source` interface lives in `app` (as D45). Measured 2026-10-13: a fetch costs 1 point of the 5000/h GraphQL budget | `gh` already holds the login; one full-set query keeps disappearance as the auto-close signal (D20a) |
| D51 | **The Reviews lane lists every request** (amends D49's "a pinned review leaves the lane while its task is open"): a request whose task isn't dropped (open or done) is drawn in cyan (ANSI 6; yellow stays for ⚠) and names its task after the wait, `@tom  1d · #11`; dropped or taskless rows stay plain; the title counts the whole set. `p` on a request whose task is open and pinned says `already pinned · #11`; otherwise `p` pins, reopens or creates as before. The cursor's row is reverse video, without the colour | From use: a pinned review vanishing from the lane looked like a lost PR; the lane is GitHub's full list, the colour says which ones are planned |
| D52 | **People view** (`tt · People`): `P` from Today or the Log opens it, `esc` returns there. Left pane `PEOPLE · 7`: every requester that some task carries (tasks without one never appear), alphabetical, `ana · 3 open`; a person with no open task is dimmed (`raff · 0 open`, still there for cleanup); `/` filters the names live with the Log's fuzzy matcher, `esc` in the filter clears it. Right pane `@ana`: the person's open tasks grouped by priority like the Log's backlog (pinned ones too, marked 📌), then a `closed` heading with done and dropped tasks, dimmed, newest first, with their close date. `tab` switches panes, `j/k` and `ctrl+d/u` move; in the right pane `⏎ p u x d D e E o` act as in the Log; `r` renames from either pane. Each pane keeps its row across reloads; a person that disappears (renamed elsewhere, last task deleted) leaves the cursor on the nearest row. Editing a task's `@who` with `e` moves it to the other person; the cursor stays on the person | Purpose (from the user): "what do I owe this person?" before a 1:1, and cleaning up names. Requesters are one lowercase word (the `@who` token), so duplicates are variants (`raf`/`raff`), not case |
| D53 | **Rename and merge a requester**: `r` opens a one-line prompt prefilled with the name; the new name is normalized like `@who` (trimmed, leading `@` dropped, lowercased) and must be one non-empty word. Same name: `unchanged`. A name that already exists: `"ana" already exists · ⏎ merge into them · esc`. One transaction sets the requester (and `updated_at = now`) on every task carrying the old one, open and closed, children included; status `renamed raf → raff · 4 tasks` (or `merged raf → ana · 4 tasks`); the cursor follows the new name. Undo restores the old name on exactly the tasks it changed (a merge's undo never touches the target's own tasks), all or nothing: if any of them changed since, it refuses ("it changed since") | One command for rename and merge; integrity over undo |
| D54 | **No commit step** (reverses D22's "commit plan", D35's `c`, D12d's last step): planning is Triage → Pick → Block, and blocks are re-placed freely all day; nothing marks a plan final. `c` does nothing on Today; the header has no `plan committed`. `days.committed_at` stays in the schema, unused (no migration) | From use: committing did nothing for the user; re-blocking during the day is the point, accountability is their own, and the timeline already shows the day against the time (the goal, met) |
| D55 | **Frames, header bar, footer** (UI tweaks backlog "visual polish"). Every pane is a rounded frame `╭ TITLE · counts ─╮ … ╰─╯` with its title in the top border (cut with `…` when it doesn't fit); the focused pane's frame is in the accent colour (ANSI 4, blue), the others dim. Today, the Log and People frame each pane side by side (no `│` divider any more); Inbox and Task detail get one frame around their body; the Reviews lane is a frame of its own under its column, in the accent colour while `w` has it (its frame replaces the blank line and title line, so it costs the same rows as before). **Header:** one full-width reverse-video line with the same content as before (⚠ parts stay yellow, stale markers dim). **Footer:** the status line when there is one, then one hints line, most-used keys first; when too wide it is cut at a ` · ` boundary with `…` and `? help` is always kept at the right end, so hint lines list every key again (Task detail gains `ctrl+z u D`, the Log `E o`, the timeline `a add · q quit`). **Height:** `bodyHeight()` stays the one budget and a pane's frame is inside it; the title moves into the top border, so lists show one row fewer than before. At 4 lines or fewer the screen degrades instead of overflowing: frames go first, then lists shrink to their title. Terminal ANSI colours only (follows the user's theme) | From the backlog: tell panes apart at a glance; the footer never loses `? help` |
| D56 | **The applied filter stays visible**: while a filter applies, the first row inside the Log's backlog frame, and inside People's names frame, is a dim `filter › #photos schema · esc clears`; while typing, the input sits in that same row (the Log's input was above the pane, People's at the bottom of the screen), so nothing moves on `⏎` or `esc`. Cleared → the row goes and the list gets it back. Pane titles keep the count without the filter text (`BACKLOG · by priority · 3 matches`, `PEOPLE · 2 matches`) | From use: a filter shown only at the end of a cut title was easy to lose sight of |
| D57 | **Tab skips an empty pane**: `tab` doesn't move the focus into a pane with no rows, and the status says why: in the Log `nothing pinned yet · p pins a task`, `backlog is empty`, or `no matches · esc clears` (a filter matching nothing); on Today `nothing pinned · L opens the log to pick`; in People `nobody listed`. When a reload empties the focused pane of the screen on show (it had rows before that reload; e.g. the last candidate unpinned), the focus moves to the other pane; both empty, it stays. Not while a filter is being typed, never on a screen that isn't shown | From the backlog; also fixes Plan 7's "focus can stay on an empty tasks pane" |
| D58 | **Blocks coloured per task**: on Today's timeline a block's `▓` rows and label take one of six ANSI colours chosen by task id (id mod 6: 2 green, 4 blue, 5 magenta, 6 cyan, 12 bright blue, 13 bright magenta), so a task keeps its colour all day, also when its block moves; two neighbours share a colour about 1 time in 6 (accepted: colours never shift under the user). Never yellow or red (⚠ and errors). Meetings keep one neutral dim style; closed blocks stay struck through and dim, without colour; ⚠ stays yellow; past rows dim as before (their colour too); the cursor's row stays reverse video; ghost rows keep their D36/D42 styles. Each part of a row gets its own combined style, so a selected or dimmed row's style runs to its end over a struck-through or coloured label | From the backlog: neighbouring blocks easy to tell apart; fixes the Plan 5a "nested styles" carry-forward |
| D59 | **The Reviews lane grows to a third of the body** (amends D49's "at most 5 rows"): one row per request, up to a third of the body's lines (`bodyHeight()/3`, the lane's frame included), never fewer than 5 rows when there's room; past that it scrolls as before. A short list takes only its own rows, and the pane above still keeps its frame and 3 rows. About 5 rows at 24 lines, 10 at 40, 12 at 45, 17 at 60 | From use (while trying Plan 8): 5 rows was too few in a tall tmux pane; a fixed 10–15 doesn't fit every pane, and always taking a third would waste rows on a short list |
| D60 | **The Reviews lane lists the newest request first** (amends D49's "longest waiting first"): sorted by when the request was made (`requested_at`, else `first_seen_at`), newest at the top and the longest waiting last; ties by repo and number. Cursor, pinning and D51's colours unchanged | From use: a new request should show without scrolling; the wait column still says how old each one is |
| D61 | **The pin count shows days carried over** (display of D10d): `📌N` with N = pinned-days − 1, so a task pinned yesterday reads `📌1`, like `↻1` for a block left unfinished yesterday. Shown on the same tasks as before (pinned before today), on Today's rows and in the detail header; the derived counter itself is unchanged | From use (2026-10-14): tasks created yesterday showed `📌2` next to `↻1`, which read as inconsistent |
| D62 | **A timeline block shows ✎ when its task has a note**, after the title like the rows: `Alpha ✎ [P1 S]`, on every timeline label (first rows, outside working hours, other blocks on a moving block's row); the moving block's own ghost label (title and length only) stays bare | From use (2026-10-14): see from the timeline that a task has a description |
| D63 | **Notifications, opt-in per kind** (P8): macOS notifications sent by the running TUI through `osascript` (no new dependency; title and body passed as arguments, never spliced into the script). `[notify]` `meetings`, `blocks`, `reviews`, all `false` by default. Only the TUI notifies (not `tt add`); nothing is sent while tt isn't running. Alerts are remembered in memory only: nothing is persisted, no migration. A failed send shows `can't notify: …` in the status once per run and never stops the loop | Interrupting features are opt-in (tt may be shared); the user runs one tt in tmux, so in-memory is enough. Meetings stay a kind of their own: the user may turn off Calendar's or Google Calendar's own alerts and rely on tt's |
| D64 | **Alert times for meetings and blocks**: `[notify] alerts = [5, 0]`, whole minutes before the start (`0` = when it starts), one list shared by meetings and blocks; each an integer 0–120, no duplicates, the list not empty (turn a kind off instead). An alert is a (kind, id, start, minutes) moment: a block's id is its block id, a meeting's its title; moving a block makes new moments. An alert fires only when its moment passes while tt runs: what started (or whose moment passed) before launch never alerts, nor does a moment already past when the block is placed (a block put 3 minutes ahead gets only its `0`). A moment more than a minute late (the laptop slept) is dropped. Only blocks of open tasks; only today's busy meetings (D17a). Moments due together are one notification: title `tt · in 5 min` (or `tt · now`), body the names joined by ` · ` | Both "get ready" and "it's time"; minutes as plain integers read clearly; one list until a per-kind split is asked for |
| D65 | **New review requests**: after the first good GitHub fetch since launch (silent: everything listed then was there before), each good fetch notifies the requests not in the previous good fetch, as one notification: one request `tt · review requested`, body `@author owner/repo#n Title`; several `tt · 3 review requests`, body their titles joined by ` · `. A request that left and came back (re-requested) counts as new. A failed fetch keeps the previous set | A burst of requests is one interruption, not five |
| D66 | **Quiet while tt is focused**: the view asks for terminal focus reports (`ReportFocus`; tmux needs `set -g focus-events on`). An alert due while tt's pane has the focus is dropped, not queued (it is on the screen). No focus event seen yet → treated as unfocused, so without focus reports tt always notifies | Don't interrupt what is already being looked at; a terminal without focus reports must not silence everything |
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

Plus, outside the task itself: **pins** (task is a Today candidate since day X → pinned-days counter), **blocks** (task, day, start, length), **days** (which planning days happened).

**Parent/child:**
- Children inherit priority and tags from their parent (no override in v1), so a child needs only a title and size and skips the Inbox.
- Any item can be pinned, parents included, without warning. Pinning a parent pins its children (D34). A pinned parent shows in candidates with its open children indented beneath it (plus children done today, struck through); children can be blocked individually.
- Time-blocking a parent asks: "This is a parent task; time-blocking its children is recommended. Proceed?"
- When all children are done, the app suggests closing the parent.

## Motivation & feedback

- **Small leaves (nudge)**: placing an L or XL task says "big task · consider splitting" (D37).
- **Done stays visible**: closed items remain in Today, struck through.
- **Day progress**: visible on the timeline (done blocks vs remaining).
- ~~**Plan lock-in**: "commit plan" is the morning's finish line.~~ Dropped by D54: re-blocking freely during the day is the point.
- **Day recap**: shown in the new-day banner ("Yesterday: 5 done · 2 carried").
- **No punishment mechanics**: no streaks or scores that can break.
- **Rollover as diagnostic** (D10, D10a).

## Views

1. **Today** (home screen, D35): timeline (meetings, task blocks, free gaps, "now" line; D38) beside candidates; Reviews lane under the candidates (D49, `w`). Doubles as the Block phase: placement and block keys always work here (D36); `L` Log, `i` Inbox. *(Today mockup)*
2. **Log** (formerly Pick; planning phase 1 "Pick", also the everyday backlog browser, D35): backlog left with grouping (D26), filter bar (D26a/b) and the Reviews lane under it (D49, `w`); today's candidates right with counters and inline ⚠ rollover prompts. Keys: `p` pin, `u` unpin, `x` done, `d` drop, `e` edit, `/` filter, `g` group, `Tab` pane, `⏎` detail (D31), `B` → Today to block (D35), `esc` → Today. *(Pick mockup)*
3. **Block** (planning phase 2): not a screen of its own; it is Today (D35) with pick-up/drop placement (D36); ⚠ on conflicts (D18, D42) and the calendar status (D44) came with Plan 5b. *(Block mockup; validate in use)*
4. **Inbox**: one card at a time; priority definitions always visible; `1-3` priority, `s/m/l/x` size, `@`/`#`/`>` requester/tags/parent, `n` skip, `d` drop; auto-advance once priority + size are set (2 keystrokes). *(Inbox mockup)*
5. **Task detail** (`⏎` on any task): header, link, and a note preview (first few lines plus size, e.g. "+38 lines"); `E` opens the full note in `$EDITOR` (D29). Lists show a small marker on tasks that have a note. For parents, also children with progress bar, done children kept with date; `a` repeating add-child input (token syntax), `J/K` reorder. *(Parent mockup)*
6. **Quick-add**: overlay from any view (`a`), token syntax with autocomplete.
7. **People** (`P`, D52, D53): requesters left with their open counts, the selected person's tasks right; `r` renames or merges.

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
| `internal/source` | `github` adapter (runs `gh`, D50); the `Source` interface lives in `app` | domain |
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
| `days` | day (PK), started_at, committed_at | A row = a planning day happened. `committed_at` is unused since D54 (kept: no migration for it) |
| `source_items` | kind, external_id (`owner/repo#123`), number, title, url, author, repo, requested_at (nullable), first_seen_at; PK (kind, external_id) | Mirrors the full "awaiting me" set from the last successful fetch; missing items are deleted. `requested_at` = when I was (last) requested, from GitHub, and drives "waiting 2d". `first_seen_at` is the fallback if GitHub returns no request event |
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
    Kind() string                                           // "github"; defined in app (D50)
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
- **Planning**: `Pin`, `Unpin`, `PinSource(kind, extID)`, `Block(id, start, dur)`, `MoveBlock`, `ResizeBlock`, `SetBlockSpan(id, start, dur)` (fine-tune, D36), `Unblock` (`CommitPlan` dropped by D54).
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
| GitHub | 5m (config) | `SyncSources`: fetch, and **only on success** reconcile in one tx (replace source items, auto-close linked tasks no longer awaiting me, D50); same loop shape as the calendar's (one fetch at a time, `seq`) | `SourcesMsg` |
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

- **EventKit (`tt-cal`)**: `tt-cal events --from <iso> --to <iso> [--calendars …]` and `tt-cal calendars`. Prints raw events as JSON (title, start, end, all-day, availability, participation status, calendar). Busy rules (D17a) apply in Go, so the helper stays dumb. Embedded Info.plist with calendar usage description (required since macOS 14). Exit codes: 0 ok, 2 permission denied, 3 unknown calendar name (the name on stderr); anything else is a generic failure (D44).
- **GitHub** (D50): runs `gh api graphql`, so tt never handles a token; host configurable (GitHub Enterprise). **Full-set fetch every poll**: one search for `is:open is:pr user-review-requested:@me archived:false`, returning title, number, repo, author, URL, plus the latest `ReviewRequestedEvent` targeting me from each PR's timeline (same query) for `requested_at`. Direct requests only; team requests are excluded (2026-10-13: 5 direct vs 149 including teams). No incremental "only new" fetching: the question is "what's awaiting me *now*", and disappearance from the set is the auto-close signal (D20a), which an incremental fetch can't see without re-checking every known PR anyway. More than 100 results fail the fetch rather than reconcile a partial set. One request per poll costs 1 GraphQL point (of 5000/h). The DB copy (`source_items`) serves offline display, linking, and "waiting since". Adapter tests drive a fake `gh` script.

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
- **Plan 3: `tt add` without shell quoting.** Consider `tt add` with no line reading one line from stdin or a prompt, so `!`, `~`, `#`, `?` need no quoting. *(Decided: D33, Plan 4b.)*
- Smaller: duplicate priority names aren't rejected; `tt --help` exits 2; undo-path tests for Drop/Reopen/SetNote/SetPriority/detach to add when the undo stack is wired.

## Carry-forward from Plan 2 review

Undo rule established in Plan 2: an undo returns `ErrConflict` when the planning day changed, the row changed since, or the row is gone. The TUI's undo stack treats `ErrConflict` as "can't undo" and drops the entry.

- **Plan 3: Block undo residual.** `Block`'s own undo still returns a raw `ErrNotFound` when the block (or its task) is already gone; map it to `ErrConflict` (or have the TUI treat both as "can't undo").
- **Plan 3: day freshness before commands.** `Pin`/`Block` work before `StartDay` has run for a new day. The TUI's clock tick must call `StartDay` at the boundary, and planning commands should check `Today()` against the day the view shows.
- **Plan 3: "done today".** Use `Service.DayOf(task.ClosedAt)` (converts stored UTC to the clock's zone) for wins and struck-through children.
- **Plan 3: one read for Pick.** `Plan()` discards the snapshot it loads; Pick also needs the backlog. Consider returning both from one read.
- **Plan 3: commit summary wording.** `CommitSummary.Blocked` includes closed tasks' blocks; `Candidates` counts only open unblocked ones. Word the summary accordingly.
- **Plan 5 (was Plan 4; the timeline lands in Plan 5): DST gaps on the timeline.** `Day.At` for wall-clock times inside a DST gap/overlap (e.g. 02:30 on 2026-03-29) is unspecified by Go; the timeline must render those rows deliberately.
- **Plan 6 (auto-close is GitHub's): auto-close must set `UpdatedAt = now`**, or compare-and-set undo won't notice it.
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

**Plan 4b — edit & detail** (D21, D29, D29a, D31–D33, P1):
- **Task detail screen** (full screen, `tt · Detail`). `⏎` on a task in either Pick pane opens it; `esc` goes back one level (child → parent → Pick, Pick's cursor unchanged); detail keeps a stack of task ids. Layout: header `#id title [P S] ↻n`; requester · tags · "captured 9d ago" · drop reason if dropped; 🔗 link; note preview (first 6 lines, then `… +N lines · E edit in $EDITOR`); for parents `CHILDREN ████░░ 2/4` with open children in position order, then closed ones with their close date (`✓ Fri 9 Oct`). The cursor starts on the task itself; `j/k` move through the children; `x d D p u e E o` act on the row under the cursor (same meaning as Pick); `⏎` on a child opens its detail. Rows are cached (rebuilt on load and navigation). If the shown task disappears, detail falls back a level and says so.
- **Note marker**: lists (Pick panes, detail children) show `✎` after a title when the task has a note.
- **`e` edit line** (P1): overlay on the shared token input, prefilled with `app.EditLine` (no note; D32 for children); validates on enter and keeps the text on error; ` -- ` replaces the note; undo via `Edit`.
- **`E` note** (D29, D29a): `tea.ExecProcess` on a temp `tt-note-<id>-*.md` in `$VISUAL` → `$EDITOR` → `vi`; unchanged → no write; empty → cleared; non-zero exit → not saved (status says so; changed text stays in the temp file, path in the status); otherwise a compare-and-set note write (`Service.SetNoteFrom(ctx, id, from, note)`, `ErrConflict` when the note isn't `from` any more) with undo; the temp file is removed unless kept for D29a.
- **`o` open link** (D21): runs `open <link>` in the background; "no link" when empty; errors shown.
- **`a` add child** (detail of a top-level task): repeating `child › …` input, token syntax, `!n` refused (D32); `Service.AddChild(ctx, parentID, line)` inserts appended last in one transaction with one undo. On a child's detail: "one level only — esc to the parent". Inside detail `a` means add child; quick-add is one `esc` away.
- **`J/K` reorder**: `Service.Reorder(ctx, id, delta)` swaps positions with the neighbouring sibling in one transaction; undo swaps back, `ErrConflict` if either row changed; at either end: "already first/last".
- **Capture** (D33): `tt add` with no line (terminal prompt / piped stdin); multi-line paste in quick-add and add-child holds lines 2+ as a note shown as `+ N lines → note`, dropped on esc. One helper `dsl.WithNoteLines(line string, rest []string) string` composes the line for both.
- **4a carry-forwards included**: esc in the filter line restores the selected tasks; `g` saves only the latest grouping and a late startup read can't override a choice already made; filtered `l/h` leave the unfiltered expansion alone and a parent with no listed children shows no `▸`; completion works after `--` in the filter bar. Plus `tt` open/ui error-path tests (Plan 3). Hints overflow stays with the UI tweaks; the hardening-test version waits for the next migration.

## Plan 5 split

Plan 5 ships in two slices, each merged on its own.

**Plan 5a — Today & blocking** (D34–D38, P3, P4 for blocks):
- **D34** in `app`: `Pin` on a parent pins its open children in one transaction; `Unpin` on a parent unpins its children except those with blocks today; both return no undo when the task has children (status `· no undo`). `Unpin` of a child whose parent is pinned is refused ("unpin the parent first"). `AddChild`, `SetParent` and `Reopen` under a pinned parent pin the child. Childless pin/unpin undo unchanged.
- **`Service.SetBlockSpan(ctx, id, start, dur)`**: start and length in one undoable step (fine-tune), same checks as `MoveBlock`/`ResizeBlock`.
- **Config** `[timeline] start`, `end` (`HH:MM`, end after start; defaults 08:00, 19:00); renamed `[work_hours]` by D39.
- **Screens** (D35): Today as start screen; Pick renamed Log (`L`/`esc`/`B`); Inbox and Task detail return to the screen they were opened from; banner `⏎` routes Inbox → Log.
- **Timeline renderer** (D38) over a list of entries (blocks only in 5a, meetings join in 5b), rows cached per load like Log's; pick-up/drop placement and block keys (D36); nudges (D37); `c` commit with the status-line summary (wording per the Plan 2 carry-forward: blocks include closed tasks', candidates are open unblocked ones).
- **Carry-forwards included**: the shared height budget for Inbox, Log and the timeline (Plan 3); DST rows (Plan 2).

**Plan 5b — Calendar** (D40–D44):
- `helpers/tt-cal` (Swift: `main.swift`, `build.sh`): `calendars` and `events --from --to --calendars`; raw JSON; exit codes per Adapters. Compiled in the scratchpad when planning; the first real run (the permission prompt) is the user's.
- `domain.Event` and `domain.Busy(events)` (D17a: all-day, declined and free aren't busy; tentative is).
- `internal/calendar`: the `eventkit` adapter (`CalendarSource` itself lives in `app`, D45) (helper next to tt or on PATH, 10s timeout, exit-code mapping), a fake. Adapter tests drive small fake-helper scripts (JSON, exit 2/3, a hang).
- Migration 004 `integration_state` (fix `TestNewerSchemaIsRefused`'s hardcoded version first); `app.SyncCalendar(ctx)` fetches the planning day, stores the busy events on success, records the error otherwise (payload kept); `Load` returns the meetings and the calendar status; a payload for another day is ignored.
- Config `[calendar] calendars`, `poll`; the TUI poll loop (background command, `R` fetches now, startup shows the cache first).
- Today: meetings (D41), busy placement and ⚠ (D42), `r` (D43), header status (D44).
- The real `tea.Program` test with `-race` (Plan 3), and the "Carry-forward from Plan 5a review" items marked Plan 5b.

## Plan 6 — GitHub reviews lane (designed 2026-10-13)

One slice (D48–D50):
- Migration 005 `source_items`; store reads/writes in a tx; `domain.SourceItem`.
- `app`: the `Source` interface; `SyncSources(ctx)` (D50 reconcile, auto-close with `UpdatedAt = now`, the Plan 2 carry-forward); `PinSource(kind, extID)`: no task → create (`Review #n: <title>`, link, source ref, D48 priority/size, no requester) and pin, undo deletes it; closed task → reopen and pin, undo closes it again as it was; open task → pin. `Load` returns the lane (items without an open linked task, longest waiting first) and the GitHub status.
- `internal/source`: the `gh` adapter (D50), tests with a fake `gh`.
- Config `[github]` (D48, D50) in `config.example.toml`.
- TUI: `ghLoop` (copy of `calLoop`), `R` fetches both, header status, the lane on Today and Log (D49), `w` focus, `p`/`o`, auto-close status, `✓ via GitHub` in Task detail.
- Checks: `app` scenarios with a fake source (create/reopen/undo, auto-close then undo conflicts, no reconcile on error or over 100, `first_seen_at` kept, re-request back in the lane); TUI harness tests; a pty run of the real binary with a fake `gh` on `PATH` (the sandbox never calls real GitHub in tests).

Rulings while drafting the plan (in the code; one-line reversals):
- `w` with GitHub off says `GitHub is off · [github] enabled in the config`.
- In the lane, any screen key (`L`, `B`, `i`, `c`, `esc`, `/`, `g`) gives the focus back and keeps its meaning; the lane's hints are `j/k move · p pin · o open PR · w back · R refresh · ctrl+z undo · ? help`.
- The lane's cursor follows its request across reloads; when that request left (pinned, no longer requested) it stays at the same index.
- Auto-closed reviews are done, so the new-day recap counts them among "done".

On friction (not built):
- GitHub search is eventually consistent: a PR wrongly missing from one result auto-closes its task; it shows again in the lane next poll and `p` reopens it. A guard (missing in two fetches in a row) needs extra state.
- A materialized task's title is a snapshot from pin time; PR renames don't follow.
- A repo rename reads as "request gone" plus a new request (external id `owner/repo#n`).

## Plan 7 — People view (designed 2026-10-13)

One slice (D52, D53). Purpose and scope from the user: **(a)** "what do I owe this person?" (open commitments, e.g. before a 1:1) and **(b)** cleaning up names (rename, merge duplicates). Stats per person (who asks most, whose asks get dropped) are **out of scope** (Deferred: statistics / retro).

- `store`: `Tx.TasksWithRequester(ctx, name) ([]int64, error)`.
- `app`: `RenameRequester(ctx, from, to string) (n int, Undo, error)` (D53): normalize and validate `to` like the `@who` token (`ErrInvalidValue` for empty or more than one word); one transaction over every task with `requester = from`; the undo keeps the changed ids and refuses as a whole (`ErrConflict`) when any of them changed since.
- `tui`: `screenPeople` (`P` from Today/Log, `esc` back to the screen it came from); a people list derived from the snapshot (names, open counts, alphabetical); the left pane with the `/` filter (the Log's fuzzy matcher in `internal/search`, exported for names); the right pane rows (the Log's row renderer and priority grouping, then the closed section); the task keys via the existing `taskKey`; the rename prompt and merge confirmation (overlays like the drop prompt); hints, `?` help, the shared height budget.
- Tests: `app` rename/merge/undo/refusals (closed tasks and children renamed too; a merge's undo leaves the target's own tasks alone; undo refused after one task was edited); TUI harness flows (entry/exit from both screens, counts and dimming, filter, grouping and closed section, task keys in the right pane, rename, merge confirmation, `unchanged`, cursor follows the new name, `ctrl+z`, a short terminal); a pty run of the real binary (`P`, `r`, rename, `q`) checking the database.

Rulings while drafting the plan (in the code; one-line reversals):
- Renaming also drops trailing punctuation (`Ana,` → `ana`), because the `@who` token does (one rule, `dsl.ParseRequester`).
- `⏎` on a person moves the focus to their tasks; task keys in the names pane do nothing.
- A child is its own row in its requester's list (`↳ #parent`); a parent shows its progress but doesn't expand. Untriaged (Inbox) tasks count as open.
- The name filter stays while tt runs, like the Log's; a leading `@` in it is ignored; `esc` on a filtered list clears the filter before leaving.
- A rename to a name the filter would hide clears the filter, so the cursor can follow. After an undo the cursor stays where it is (the nearest row if that name is gone).
- The merge question is the D37 nudge: any key but `⏎` says `cancelled`. A rename of a name no task has any more fails with `no task has @raf any more`.

## Plan 8 — UI polish (designed 2026-10-13)

One slice (D55–D58), the UI tweaks backlog plus the filter line from use. The user tries the branch build on real data before it merges.

- `tui`: a `frame(title, lines, width, height, focused)` helper drawn by hand (Lip Gloss borders can't hold a title, and the width math already lives in `pad`/`ansi.Truncate`); every screen renders its panes through it. Header bar and footer (`hintsLine`: cut at a ` · ` boundary, `? help` kept) in `common.go`; the budget stays `bodyHeight()` with each pane's frame inside it, and a short-terminal fallback (no frames at 4 lines or fewer).
- The filter row (D56) inside the backlog and names frames: the typing input and the applied dim line share it.
- Tab rules (D57) in the Log, Today and People, plus the "focused pane emptied by a reload" move.
- Block colours (D58) and per-segment styles in the timeline renderer.
- Carry-forwards included: the Log's hints overflow (Plan 4a), Task detail's missing hints (Plan 4b), Today's timeline hints and the nested styles and the height ≤ 4 overflow (Plan 5a), the empty tasks pane in People (Plan 7).
- Tests: a sweep rendering every screen (Today, Log, People, Inbox, Detail; with the banner, an applied filter and an open overlay) at 40×8, 80×24 and 140×40 (no line wider than the terminal, never taller); raw-ANSI checks (accent frame, dim frames, a block's colour, a full-width selected row); harness flows for the filter row and tab; a pty run of the real binary at 80×24 and in a short pane.

Rulings while drafting the plan (in the code; one-line reversals):
- A frame's rows are `│ ` + text + `│`: a frame costs 3 columns and 2 rows; with the title in the border, lists show one row fewer than before.
- Task detail's frame is titled `TASK`; the Inbox card's title line moves into its border (the blank line under it goes); People's tasks frame says `TASKS` when nobody is selected.
- Frames need a body of at least 3 lines; below that each pane is its title line and rows. The screen is clipped to the terminal's height, so at 3 lines or fewer the footer goes first.
- A filter matching nothing shows `no matches` (its row already says `esc clears`); People's empty text is `nobody yet · @who names who asked`; People's filter prompt is `filter › `.
- `B` from the Log still focuses Today's candidates, even when there are none. Meetings render dim.

## Plan 9 — Notifications (designed 2026-10-14)

One slice (D63–D66), P8 brainstormed with the user. Approach: in the TUI, in memory (rejected: persisting sent alerts in the database, which only helps with several tt instances; a separate `tt notify` daemon, since tt is always open).

- `config`: `[notify]` `meetings`, `blocks`, `reviews` (bool, default `false`) and `alerts` (integers, default `[5, 0]`, D64 validation); in `config.example.toml` with a comment that macOS asks to allow notifications for Script Editor (osascript's sender) on the first one, and that tmux needs `focus-events on`.
- `tui`, a pure planner: `due(state, cfg, from, to) []alert`, the meeting and block moments in `(from, to]` (D64 rules), and `newReviews(prev, cur) []domain.SourceItem`, plus the message texts (D64, D65) as pure functions.
- `tui`, the loop: `Options.Notify func(ctx, title, body string) error` (default: `osascript -e 'on run argv' … 'end run' title body`; tests capture the calls). The Model keeps the launch time, the last check time (the launch at first), the previous review set (nil until the first good fetch since launch) and the focus. Every watch tick (D13's, 1s) sends what `due` returns for `(last check, now]`, minus anything over a minute late, and nothing at all while focused (D66); the check time then moves to now. Each load whose GitHub `LastSuccess` is new and not before the launch diffs the review set (D65). `View` sets `ReportFocus`; `tea.FocusMsg`/`tea.BlurMsg` set the focus. All of it off when no kind is on: no sends, no focus reports.
- Tests: `config` validation (defaults, bad values, unknown keys); the planner (lead times, late placement, moved block, closed task, declined meeting, grouping, the minute-late drop, the day boundary); harness flows with a fake `Notify` and `Schedule` (launch silence, first fetch silent then a grouped review notification, focus drops, a send error shown once, everything off by default); a pty run of the real binary with a fake `osascript` on `PATH` logging its arguments (the sandbox never sends real notifications in tests).

Rulings while drafting the plan (in the code; one-line reversals):
- No timer or sent set of its own: the 1s watch tick checks the window since the last check, so each moment fires once and a laptop waking from sleep catches up within a second (the minute-late drop keeps it quiet).
- Moments due in one check with different lead times are separate notifications, one per lead time.
- A good GitHub fetch is recognised on load by `LastSuccess` (not before launch, changed since the last one seen), so the diff never depends on which load lands first.
- After a failed send, later sends are still tried; only the first failure is reported. The osascript title always starts with `tt`, so a body starting with `-` isn't read as an option.

## Carry-forward from Plan 4a review

- **Plan 4b: cursor after a filter round-trip.** `/`, a typo, `esc` restores the filter but not the task the cursor was on (live narrowing replaced `selected`). Remember the selected ids with `prev` and restore them on esc.
- **Plan 4b: grouping saves race.** Each `g` saves in its own background command, so a fast `g g` can store the earlier value last; the startup read of the remembered grouping can also land after an early `g`. Save the latest value only (or ignore stale results).
- **Plan 4b: filtered expand/collapse.** `l`/`h` while filtered still write the unfiltered expansion state; a matching parent with no matching children shows `▸` but can't expand.
- **Plan 4b: completion after `--` in the filter bar.** The shared token input suppresses `@`/`#` completion after ` -- ` everywhere; in the filter bar `--` is just a word.
- **Before migration 004:** `TestNewerSchemaIsRefused` hardcodes the newest migration version (3); compute it from the embedded migrations.
- **With the UI tweaks (footer):** Pick hints are ~128 cells, so at 100–120 columns `? help` is cut.
- Accepted, revisit on friction: `@an` matches requesters exactly, so live results show 0 until the name is complete; a partial `is:` shows an error line while typing (results stay).
- Tests to add when touched: `search.Oldest`; token-input completion with multi-byte text and a cursor right after a space; `prompt.resize`; a pinned parent with matching children while filtered; the scroll offset after a filter shrinks a scrolled list.

## Carry-forward from Plan 4b review

- **Plan 5a (from use): a pinned parent pins its children (D34).** `Pin` on a parent pins its open children in the same transaction, and `Unpin` unpins them except children blocked today (D34 as amended); `Unpin` on a child of a pinned parent returns an error the TUI shows as "unpin the parent first"; `AddChild`, `SetParent` and `Reopen` under a pinned parent pin the child. `Pin`/`Unpin` of a task with children return no undo (the TUI shows e.g. `pinned "X" + 3 children · no undo, u unpins`), so no cascade undo logic is needed; childless pin/unpin undo is unchanged. StartDay's carry-over and pinned-days (D10d) apply per child as today.

Plan 4a's carry-forwards are done in Plan 4b, except "before migration 004" and the hints overflow (still open above).

- **With the UI tweaks (footer/hints):** Task detail's hint line leaves out `ctrl+z`, `u` and `D`, and Pick's shows `e` but not `E`/`o`.
- **On friction: quoted `$EDITOR`.** The editor command is split on spaces, so `EDITOR="'/Applications/Sublime Text.app/…/subl' -w"` or `vim -c 'set ft=markdown'` break; git runs `sh -c "$EDITOR \"$@\""`.
- **On friction: an editor that returns at once** (`VISUAL=code` without `--wait`) leaves the file unchanged, so tt says "note unchanged" and removes the file while the editor still has it open. A hint ("does your editor need --wait?") is cheap.
- **Small:**
  - Deleting the shown task from Task detail ends on "#N is gone" instead of "deleted #N".
  - Task detail's child-window height counts the full-screen help overlay, so the list can jump after help closes.
  - Tabs and ANSI codes in a note's first lines are shown as-is in the preview.
  - Every prompted `tt add` waits ~20ms for a paste to settle.
  - A `/dev/null` stdin counts as a terminal (spurious `add ›`, then usage).
  - A multi-line paste into the edit line (`e`) joins into one line; by design, since ` -- ` there replaces the note.
- **Extract when either rule changes:** the child progress count (Pick row and Task detail) and the ✓/✗ close mark are written twice.
- Tests to add when touched:
  - `Reorder` among several closed siblings and with `delta 0`
  - `AddChild` undo conflict
  - `Edit` clearing a child's old own priority
  - `u` in Task detail
  - a stale day while detail is open
  - an empty note left untouched in the editor
  - an editor that can't start
  - `E`/`o` from Task detail
  - the grouping-save error path, and the startup read applying when no `g` was pressed

## Carry-forward from Plan 5a review

Plan 5a decisions made during execution (in the code): keys on a timeline row act on the block the row names (the earliest block starting in it, else the one carrying on from above); `m` keeps a block's minutes within its row. ("`j/k` past either end grow the timeline" was replaced by D39.)

- **Plan 5b (touches the timeline anyway):**
  - Dropping or fine-tuning with unchanged values still calls the service and pushes a no-op undo; short-circuit with "unchanged".
  - The pick-up is cleared before the service call, so a service-side refusal (stale timeline) loses it; keep it until the action succeeds.
  - The fine-tune prompt captures the block when it opens; a reload meanwhile ends in a service error (no damage). Re-read the block on enter.
  - The timeline's scroll offset shifts when the window grows at the top after a reload (the cursor stays visible).
- **From D39 (on friction):**
  - Working hours can't end after midnight (`end` is at most 23:59); accept `24:00`, or an end before the start meaning "after midnight", if late shifts ever matter.
  - When an outside row before the hours disappears (moved in, unblocked) the cursor's fallback in `refresh` can jump to the bottom of the timeline; clamp to the nearest row by time.
  - A block fully outside the hours can't be resized at all (shrinking included); the status could suggest `m`.
  - Tests to add: a block partly inside the hours (18:30–19:30: shown in the working rows, shrink works, grow refused, move works); two outside blocks next to each other; moving an off-grid outside block. Tests reset `today.placed` to simulate a fresh start; a harness option would be cleaner.
- **With the UI tweaks (footer/hints/styles):** Today's timeline hints leave out `a add · q quit`; a closed block's struck-through label inside the selected or dimmed row may end the highlight early (nested styles); at a height of 4 or less the screen is taller than the terminal (`bodyHeight` floor 2 + 3 fixed lines; pre-existing).
- **Small:**
  - Unpinning a parent also unpins closed (done-today) children; pinning is open-only. Harmless (the parent's rows are gone anyway).
  - `Block` on an unpinned parent pins its children; its undo leaves them pinned (not reachable from the TUI: `b` acts on candidates).
  - A parent whose children are all closed and that has a block today can't be unpinned either (conservative, consistent with D34's refusal).
  - `day_start` or `timeline.start` of 23:59 with no `timeline.end` fails "end must be after start" for a key never set.
  - Stale names: `TestDetailFromTodayAndChildScrolling` starts in the Log; tui_test's "straight to Pick" comment.
- Tests to add when touched:
  - `Unpin` of a parent with a closed child; `Pin` on an already-pinned parent with children (nil undo)
  - `Day.Offset` for fall-back and month/year boundaries; a block ending at `day_start + 24h`; `gapFrom` at the day end; an off-half-hour `day_start`
  - the height-budget test with Task detail, an open overlay and the banner; detail opened from Today when its task is deleted; Today at narrow widths
  - undo of `+`/`-`; a resize past the day end; fine-tune after midnight through the UI; `ctrl+z` while a block is picked up; the new-day reset of a pick-up; the "stopped moving … its task is gone" status
  - `TestPlanningNudges` reaches the closed candidate with five `j` presses; select it by id instead

## Carry-forward from Plan 5b review

Plan 5b merged 2026-10-13 (D40–D45). The helper's first real run (`make install`, `tt-cal calendars` for the permission prompt, calendar names into `[calendar] calendars`) is the user's.

- **On friction:**
  - `R` or a new planning day while a fetch is running is dropped, not queued: right at day_start the in-flight fetch may be yesterday's, so today's meetings can be missing for up to one poll. An "again" flag (like `pick.saveAgain`) fixes it.
  - Two calendars with the same name (e.g. "Calendar" in iCloud and Google) can't be told apart: `tt-cal calendars` prints names only and `--calendars` takes both.
  - Every poll writes `integration_state` (even an unchanged failure), so the TUI reloads every poll; skip the write when nothing changed if it ever matters.
  - With `poll` under 30s the stale marker can read `cal just now`.
  - `r` with no free slot on a block starting before the working hours leaves the ghost on the cursor's row, not on the block (`pickUpBlock` keeps the cursor when the block's start has no row).
  - `m ⏎` on a block outside the working hours gives the `fits` error, not `unchanged` (the unchanged check runs after `fits`).
  - A refused drop's restore overwrites a pick-up started in the milliseconds before the refusal lands; guard on `carry == nil`.
- **Small (helper):** a bare `--calendars` with no names passes `[]` to EventKit (Go never sends it); duplicate names add a calendar twice; `nonisolated(unsafe)` needs Swift 5.10+; exit 3 with empty stderr reads `unknown calendar ""`.
- **Small (code):** `plural`'s doc comment sits above `oneLine` in `internal/tui/common.go`; the ⚠ count clones the blocks each render (a counting loop is clearer); `TestTimeoutSaysWhatToDo` races a 500ms timeout against script startup (20/20 under `-race`; bump to 2s if it flakes).
- **Tests to add when touched:** `calSyncedMsg` with a store error (loop continues, status shows it); a current-`seq` tick while a fetch runs; two meeting bands in one row; a ghost over a row with a block and a meeting; the scroll keeping the cursor when the window shrinks.

## Carry-forward from Plan 6 review

Plan 6 merged 2026-10-13 (D48–D50). Fix-wave rulings (in the code): text from any source (title, author, repo, URL, id) is made one clean line (`domain.OneLine`) in `SyncSources` before it is stored, so task titles from PRs never carry control characters; an empty node in gh's answer fails the fetch (`gh returned an incomplete answer`), like more than 100 results; gh's `HTTP 401` maps to `not logged in · gh auth login` (amends D50's "else the first line of gh's stderr"). The real-GitHub check (the lane against your direct requests; once, `gh api -i graphql -f query='{viewer{login}}'` for an `X-GitHub-SSO: partial-results` header, which would mean SSO hides PRs that would then auto-close) is the user's.

- **On friction:**
  - A review reopened by hand after its auto-close is closed again at the next sync (D50: every open linked task missing from the set). Fix: close only tasks whose item was stored before and is gone now.
  - After `x`/`d` on a review task, the request sits in the lane until the next poll (GitHub usually dropped it already); fetch GitHub right after closing a source-linked task.
  - `last: 10` review-request events: a PR with ten or more later requests for others loses mine, so its wait falls back to `first_seen_at`.
  - Changing `[github] host` auto-closes every review task of the old host (same kind `github`).
  - In the lane, keys outside its list fall through and drop the focus: only `h/l/←/→` act (expand/collapse in the Log backlog). Sturdier: list the screen keys that give focus back, make the rest no-ops.
  - `w` focuses the lane when a terminal under ~9 lines hides it; refuse focus when `laneLines() <= 2`, drop it in `follow` when the lane shrinks away.
- **Small:** `PinSource`'s reopen skips `pinChildren`/`pinnedUnderParent` (D34), which matters only once a review task has children; the `Fetch` doc comment in `internal/source/github.go` and the `OneLine` lines in `SyncSources` run past 80 columns; the lane still applies `OneLine` at render time (redundant for new data, harmless).
- **Tests to add when touched:** undo of a dropped-then-reopened review; `PinSource` on an open, unpinned review task; `ghSyncedMsg` with a store error (beside the 5b calendar one); `ctrl+d/u` inside the lane.

## Carry-forward from Plan 7 review

Plan 7 merged 2026-10-13 (D52, D53; drafting rulings in "Plan 7 — People view"). The real-data check (`P` on your own names, merging a duplicate pair with `r`) is the user's.

- **On friction:**
  - `x` in People's tasks pane keeps the cursor on the task, so it jumps into the `closed` section; going down a person's list before a 1:1 takes a `k` after each `x`. Fix: when the selected task went from open to closed, fall back to the old index.
  - The cursor-follow after `r` can lose a race: a load that started before the rename's commit lands first and resets the selection to the old name, so the cursor ends on the nearest row instead of the new name (milliseconds; nothing damaged). Fix: a pending name that `refresh` honours until the action's message arrives.
  - A failed rename (the name vanished in another shell) puts the cursor back on the old name but not a filter the rename cleared.
- **Small:** the gone-name error reads `no task has @raf any more: not found` (the `ErrNotFound` text); with nobody listed, the focus can stay on the empty tasks pane (its hints show, every key is a no-op); unhandled keys in the tasks pane fall through to `taskKey` (no-ops); `RenameRequester` expects a stored name as `from` and doesn't normalize it (a doc line would say so).
- **Tests to add when touched:** a merge into a name the filter hides (asks first: `idx.people` ignores the filter); the short-terminal test with the filter line open or the banner showing; the "cursor stays on ana after undo" check in `TestMergeIntoAnotherPerson` belongs right after its `ctrl+z`.

## Carry-forward from Plan 8 review

Plan 8 merged 2026-10-13 (D55–D58, plus D59 from trying the branch; drafting rulings in "Plan 8 — UI polish"). The user tried the branch on real data before the merge. Review fixes already in: `pad` re-pads when a wide character lands on the cut (every caller); the unreachable unframed lane branch went; a shared `appliedFilterLine`; tests for refocus's typing guard and a selected closed Log row.

- **On friction:**
  - A reload that empties the Log's focused pane while Task detail or the Inbox is on show leaves the focus there (D57 acts on the screen on show only); back in the Log the focus sits on an empty pane until `tab`. Fix if it bites: refocus when a screen comes back on show.
  - ⏎ ✎ → ← are East-Asian ambiguous width: `ansi.StringWidth` counts 1, which macOS terminals and tmux match; a terminal set to wide ambiguous characters would push frames right.
  - Inbox zero keeps a blank line under its frame's top border while a task card starts right under it.
- **Small (tiny panes only):** below width 4 `frame` returns nothing, so the right column starts at 0 (under about 8 columns on Today, 12 in People); at 2–3 lines the footer and then the status go before the body's one row (and at 4–5 with the banner or a multi-line overlay); a very short Log with a filter error and suggestions (up to ~7 lines) floors the list at 1 row and the frame clips. `paneVisible` subtracts filter lines only on the Log (the only screen with that pane today); `filterLines` re-renders the input on each call (trivial). Today's refocus ignores a focused lane (harmless: `w` returns to the timeline anyway). Two blocks sharing a row take the first block's colour for `▓` (labels keep their own). In the lane, `ctrl+d/u` step half the pane, not half the lane (`follow` keeps the cursor in view).
- **Tests to add when touched:** `fits()` in layout_test checks only upper bounds (pin exact width and a closing border for framed body lines; a wide-character title in `busyHarness`); scrolling a list while filter error/suggestion lines show; `B` from the Log focusing empty candidates; highlight-to-the-end checks for ⚠ on selected/past rows, ghost rows, People and the lane; a colour test failure message prints the wrong line (colour_test.go, the Lunch case).

## UI tweaks backlog (from use)

Plan 8 (D55–D58) shipped the earlier list, merged 2026-10-13. Later tweaks are collected here again, batched.

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
- LLM access (adapter, Claude skill, or MCP server over `app`/DB)
- More CLI commands beyond `add`
- Google Calendar API adapter

## Open items

Proposed defaults (pending confirmation):
- **P1 Edit**: `e` opens the token line prefilled with the task's current values. Notes: see D29. *(Accepted; Plan 4b.)*
- **P2 Undo**: session-level undo stack (`ctrl+z`) built from inverse commands; not persisted.
- **P3 Done with future blocks**: blocks stay on the timeline, shown as done; `u` frees the time. *(Accepted; D38, Plan 5a.)*
- **P4 Timeline**: visible hours configurable (default 08:00–19:00), 30m rows, scrolls; off-grid meetings show exact times in the label; overlapping meetings merge into one busy band with a count. *(Accepted for blocks as D38, Plan 5a; the hours became D39's working hours; meetings in Plan 5b.)*
- **P5 Help**: `?` overlay listing keys for the current view.
- **P6 Non-working days**: no working-days config; the new-day banner plus "later" is enough.
- **P7 Backups**: rely on Time Machine for v1.
- **P8 Notifications** (from the user 2026-10-13; to brainstorm as the next plan): macOS notifications sent by the running TUI (start with `osascript`, no new dependency). Opt-in per kind, all off by default (tt may be shared): `[notify]` `meetings`, `blocks` (a task block starts), `reviews` (a new review request). Never for what already started or arrived before tt launched (the first fetch after start is silent). Quiet while tt's pane is focused (terminal focus events; tmux `focus-events on`); no focus event ever seen → assume unfocused. *(Accepted 2026-10-14 as D63–D66, Plan 9: meetings stay a kind, alerts `[5, 0]` minutes before, several new requests grouped.)*

Resolved:
- ~~GitHub scope~~: direct review requests only (`user-review-requested:@me`); team requests not shown.
