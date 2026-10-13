#!/usr/bin/env python3
"""Generate the Plan 5b doc from the verified scratch commits (main..HEAD).

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
        title="The `tt-cal` helper (Swift, EventKit)",
        intro="D40 and the Adapters section: a dumb Swift CLI that prints calendar names, or the events of a range as raw JSON, with the spec's exit codes. No Go code. The calendar usage description is embedded in the binary by `build.sh` (without it macOS 14+ never asks for access).",
        interfaces="""- Consumes: nothing.
- Produces (the CLI contract Task 2's adapter relies on):
  - `tt-cal calendars`: the calendar names, unique and sorted, one per line.
  - `tt-cal events --from <RFC 3339> --to <RFC 3339> [--calendars <name>…]`: every argument after `--calendars` is a name; no `--calendars` means all calendars. Prints one JSON array of `{"title", "start", "end", "all_day", "availability", "status", "calendar"}` (`start`/`end` RFC 3339 in the local zone; `availability` is `busy`/`free`/`tentative`/`unavailable`/`""`; `status` is my participation `accepted`/`declined`/`tentative`/`pending`/`""`).
  - Exit codes: 0 ok, 1 failure (usage errors print the usage; they happen before any calendar access), 2 no calendar access, 3 unknown calendar (the name alone on stderr).
  - `helpers/tt-cal/build.sh [DIR]`: builds into `DIR` (default `~/.local/bin`).""",
        build="""Run (builds into a throwaway directory; **don't** build into `~/.local/bin` and **don't** run `tt-cal calendars` or a valid `tt-cal events` here: the first real run asks for calendar access, and that run is the user's):

```bash
sh helpers/tt-cal/build.sh "$TMPDIR/tt-cal-build"
"$TMPDIR/tt-cal-build/tt-cal"; echo "exit $?"
"$TMPDIR/tt-cal-build/tt-cal" events --from x; echo "exit $?"
"$TMPDIR/tt-cal-build/tt-cal" events --from 2026-10-12T04:00:00+02:00; echo "exit $?"
otool -s __TEXT __info_plist "$TMPDIR/tt-cal-build/tt-cal" | head -2
```

Expected: `built …/tt-cal · next: tt-cal calendars (the first run asks for calendar access)` with no compiler warnings; the usage text and `exit 1`; `bad date "x": want ISO 8601, like 2026-10-12T04:00:00+02:00` and `exit 1`; the usage and `exit 1` (no `--to`); `otool` prints a `(__TEXT,__info_plist) section`. Then `gofmt -l .`, `go vet ./...`, `go test ./...` still pass (no Go changes).""",
    ),
    dict(
        title="`domain.Event` and `Busy`; the `tt-cal` adapter and a fake",
        intro="D17a's busy rule in `domain`, and `internal/calendar`: the adapter that runs the helper (next to tt, else on `PATH`; 10s timeout; exit codes mapped to D44's short reasons) and a fake for the app and TUI tests. Adapter tests drive small fake-helper shell scripts.",
        interfaces="""- Consumes: Task 1's CLI contract (only through the fake scripts in tests).
- Produces:
  - `domain.Event{Title string; Start, End time.Time; AllDay bool; Availability, Status, Calendar string}`.
  - `func domain.Busy(events []Event) []Event`: drops all-day, `Status == "declined"`, `Availability == "free"` and zero-length events; tentative stays; sorted by start, then end.
  - `calendar.EventKit{Calendars []string; Path string; Timeout time.Duration}` (`Path` "" = next to the running tt, else `tt-cal` on `PATH`; `Timeout` 0 = `calendar.DefaultTimeout`, 10s); `func (*EventKit) Events(ctx, from, to time.Time) ([]domain.Event, error)`.
  - Errors: `calendar.ErrHelperNotFound` (`tt-cal not found · run helpers/tt-cal/build.sh`), `calendar.ErrNoAccess` (`no access · System Settings → Privacy & Security → Calendars`, exit 2), `unknown calendar "X"` (exit 3), `tt-cal timed out after 10s`, `tt-cal printed bad JSON: …`, else the first line of stderr, else `tt-cal failed: exit status N`; a cancelled ctx returns `ctx.Err()`.
  - `calendar.Fake` (mutex-guarded): `Set(events []domain.Event, err error)`, `Events(...)`, `Calls() int`, `Asked() (from, to time.Time)`.""",
        step2="Run: `go test ./internal/domain/ ./internal/calendar/`\nExpected: FAIL (build): `undefined: Event`, `undefined: Busy`, `undefined: EventKit`, `undefined: ErrNoAccess`, `undefined: locate`.",
        step4="Run: `go test ./internal/domain/ ./internal/calendar/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`\nExpected: PASS (the calendar tests take ~2s: the timeout test waits out `WaitDelay`), no gofmt output.",
    ),
    dict(
        title="`[calendar]` config, `integration_state`, `SyncCalendar`",
        intro="D40's config, migration 004 and the calendar cache (D3a): `SyncCalendar` fetches the planning day, stores the busy events on success and records the error otherwise (the cache stays); `Load` returns the meetings and the calendar status, ignoring a cache of another day (D44). `cmd/tt` sets the calendar up only when calendars are configured. `TestNewerSchemaIsRefused` stops hardcoding the newest migration.",
        interfaces="""- Consumes: Task 2 (`domain.Event`, `domain.Busy`, `calendar.EventKit`, `calendar.Fake`, `calendar.ErrNoAccess`).
- Produces:
  - `config.Config.Calendars []string` (none = not set up), `CalendarPoll time.Duration` (default 2m); TOML `[calendar] calendars = […]`, `poll = "2m"`. Errors: `calendar.calendars[N]: name is empty (tt-cal calendars prints the names)`, `calendar.poll: want a duration of 10s or more, like "2m", got "…"`.
  - Migration `004_integration_state.sql`: `integration_state(name PK, last_success_at, last_error, last_error_at, payload)`.
  - `store.Integration{Name string; LastSuccessAt time.Time; LastError string; LastErrorAt time.Time; Payload string}`; `(*Store).Integration(ctx, name) (Integration, bool, error)` (read-only tx); `(*Tx).GetIntegration(ctx, name)`, `(*Tx).PutIntegration(ctx, Integration) error` (upsert). `app.Store` gains `Integration`.
  - `app.CalendarSource` interface `{ Events(ctx, from, to time.Time) ([]domain.Event, error) }`; `(*Service).WithCalendar(src) *Service`; `(*Service).CalendarSetUp() bool`; `(*Service).SyncCalendar(ctx) error` (a failed fetch is recorded, not returned; nil without a calendar; a cancelled ctx records nothing).
  - `app.CalendarState{SetUp bool; Meetings []domain.Event; LastSuccess time.Time; Err string}`; `app.State.Calendar` (filled by `Load`; meetings in the clock's zone, only for today's planning day; an unreadable cache becomes `Err`, never a load failure).
  - Test helpers (app): `calService(t) (*Service, *fakeClock, *calendar.Fake, *store.Store)`, `mustLoad`, `mustSync`, `at(h, m)`, `meeting(title, from, to)`, `titles`.""",
        step2="Run: `go test ./internal/config/ ./internal/store/ ./internal/app/ ./cmd/tt/`\nExpected: FAIL (build): `cfg.Calendars undefined`, `s.Integration undefined`, `x.PutIntegration undefined`, `WithCalendar undefined`, `s.SyncCalendar undefined`, `undefined: app.CalendarState`.",
        step4="Run: `go test ./internal/config/ ./internal/store/ ./internal/app/ ./cmd/tt/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`\nExpected: PASS, no gofmt output.",
    ),
    dict(
        title="The calendar poll loop, `R`, the header status; a real `tea.Program` test",
        intro="D3/D44 in the TUI: startup shows the cache and fetches in the background; then one fetch every `poll`; `R` and a new planning day fetch at once (a sequence number retires the tick they overtook, so there is only ever one loop). The header shows nothing while fresh, `cal 5m ago` once the last success is older than two polls, `⚠ cal: <reason>` after a failure, `cal: not set up (tt-cal calendars)` without calendars. Plus the Plan 3 carry-forward: one test in a real `tea.Program` on real timers, run with `-race`.",
        interfaces="""- Consumes: Task 3 (`Service.CalendarSetUp`, `SyncCalendar`, `State.Calendar`, `Config.CalendarPoll`), Task 2 (`calendar.Fake`, `calendar.ErrNoAccess`).
- Produces:
  - `Model.cal calLoop{syncing bool; seq int}`; messages `calTickMsg{seq int}`, `calSyncedMsg{err error}`; `(*Model).syncCalendar() tea.Cmd` (nil when not set up or a fetch is running).
  - `(*Model).calendarStatus() string`, appended to `headerView`.
  - `R` = reload + fetch now; help line `R        reload and fetch the calendar now · q quit · ? this help`.
  - Test helpers: `newCalHarness(t, cfg)` (a harness with `h.cal *calendar.Fake`, fetched once at startup), `startHarness(t, cfg, cal)`.""",
        step2="Run: `go test ./internal/tui/`\nExpected: FAIL (build): `undefined: calTickMsg`, `h.m.cal undefined`.",
        step4="Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`\nExpected: PASS, no gofmt output. (Verified while planning: `TestRealProgram` under `-race` reports a planted read of `m.state` from a command goroutine, so it does catch races.)",
    ),
    dict(
        title="Meetings on Today; meetings are busy for placement; ⚠ on conflicts",
        intro="D41 and D42: the timeline gets the meetings within the working hours, overlapping ones merged into one band (`■ Review +1`, exact times when off the grid, named on their first row inside the hours); a row shows its blocks first, then its meetings. `b`'s first free row and the ghost's D24 length treat meetings as busy; dropping over one is allowed and said. A block of an open task overlapping a meeting shows ⚠, and the timeline title counts them. Task keys on a meeting-only row say `meeting · read-only`.",
        interfaces="""- Consumes: Task 3 (`State.Calendar.Meetings`), Task 4 (`newCalHarness`, `h.cal`), `Day.Offset`.
- Produces:
  - `type meeting struct{ title string; start, end time.Duration }`; `type band struct{ title string; more int; start, end time.Duration }`; `tlRow.bands []band`; `timeline.meetings []meeting` (only those touching the hours).
  - `buildTimeline(blocks []domain.Block, meetings []meeting, from, to, now time.Duration) timeline` (new second parameter; existing calls pass `nil`).
  - `(*timeline).conflict(b domain.Block) (meeting, bool)`; `gapFrom` (and so `firstFree` and the ghost's length) count meetings as busy.
  - `(*Model).overMeeting(b) string` (` · ⚠ overlaps meeting "Review" 10:00–11:00`, appended to the blocked/moved/now statuses), `(*Model).conflicted(b) bool` (open task only), `(*Model).meetingParts(r tlRow) []string`; `styleGhostAlert`.
  - Test helpers: `(*harness).meetings(events ...domain.Event)` (Set + R), `ev(title, from, to time.Duration) domain.Event`.""",
        step2="Run: `go test ./internal/tui/`\nExpected: FAIL (build): `too many arguments in call to buildTimeline`.",
        step4="Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`\nExpected: PASS, no gofmt output.",
    ),
    dict(
        title="`r` re-slots a ⚠ block; the Plan 5a timeline carry-forwards",
        intro="D43: `r` on a ⚠ block picks it up like `m`, with the ghost on the first row after the block's own where the whole block fits inside the working hours, clear of blocks and meetings; no such row keeps the ghost on the block and says so. Plus the four \"Carry-forward from Plan 5a review\" items marked Plan 5b: unchanged drops and fine-tunes skip the service (`unchanged`, no undo pushed); a drop the service refuses gives the pick-up back; fine-tune re-reads its block on enter; the scroll offset keeps the cursor's line when rows appear above it. `TestMovingAnOffGridBlockKeepsItsStart` now expects `unchanged` for `m ⏎`.",
        interfaces="""- Consumes: Task 5 (`conflict`, `conflicted`, `overMeeting`, `ev`, `h.meetings`), `actionMsg.restore`.
- Produces:
  - `(*timeline).block(id int64) (domain.Block, bool)` (also used by `refreshCarry`).
  - `(*Model).pickUpBlock(t, b)` (what `m` did; keeps the cursor when the block starts before the hours), `(*Model).freeSlot(b) (int, bool)`, `(*Model).actOr(label, status string, restore func(*Model), fn) tea.Cmd` (`act` wraps it).
  - `r` in `blockKeys`; statuses `no conflict · m moves it`, `no free slot left today fits 5h`, `unchanged`, `stopped fine-tuning "X": its block is gone`; the timeline hints start with `r re-slot ⚠ · ` on a ⚠ block; a help line for `r`.""",
        step2="Run: `go test ./internal/tui/`\nExpected: FAIL: `TestMovingAnOffGridBlockKeepsItsStart` (`screen lacks \"unchanged\"`), `TestReslotAConflictedBlock` (`lacks \"no conflict · m moves it\"`), `TestReslotWithNoFreeSlot`, `TestARefusedDropKeepsThePickUp` (`a refused drop gives the pick-up back where it was: <nil>`), `TestFineTuneRereadsTheBlock`, `TestScrollKeepsTheCursorWhenRowsAppearAbove` (`12:30 moved from line 6 to 7`).",
        step4="Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`\nExpected: PASS, no gofmt output.",
    ),
]
assert len(TASKS) == len(commits), (len(TASKS), len(commits))

HEADER = """# tt Plan 5b — Calendar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Today shows the day's meetings from macOS Calendar beside the blocks: a Swift `tt-cal` helper reads EventKit, tt polls it in the background and caches the busy events, meetings are busy for placement, a block over a meeting shows ⚠ and `r` re-slots it, and the header says when the calendar is stale or broken and what to do.

**Architecture:** `helpers/tt-cal` is a dumb Swift CLI (names, or raw events as JSON, exit codes). `internal/calendar` runs it (next to tt or on `PATH`, 10s timeout) and maps failures to D44's short reasons; `domain.Busy` applies D17a. `app.SyncCalendar` fetches the planning day and caches the busy events in `integration_state` (migration 004), keeping them when a fetch fails; `Load` returns them with the status. The TUI polls with one sequenced loop, renders meetings as bands in the existing timeline row model, and treats them as busy for placement.

**Tech Stack:** Go 1.27; Bubble Tea v2, Lip Gloss v2; pure-Go SQLite; Swift 6 (`swiftc`, EventKit, Foundation) for the helper. No new Go dependencies.

**Spec:** `docs/superpowers/specs/2026-10-12-task-tracking-design.md` — D3, D3a, D17a, D18, D36, D39, D40–D44, open item P4, Adapters (EventKit), Runtime (Calendar loop), "Plan 5 split" → Plan 5b, "Carry-forward from Plan 3 review" (the real `tea.Program` test), "Carry-forward from Plan 5a review" (the four items marked Plan 5b).

## Global Constraints

- No new Go dependencies: `go.mod` and `go.sum` stay unchanged. The helper uses only EventKit and Foundation and is built with `swiftc` by `build.sh` (no Xcode project).
- Migration 004 only adds `integration_state`; earlier migrations are untouched.
- Only `app` mutates state. `app` defines `CalendarSource` and never imports `internal/calendar` outside tests; adapters never import `app`.
- D17a, exactly: all-day, declined and `free` events aren't busy; tentative is busy.
- D3a/D44: a calendar failure never blocks planning or fails a load: the error is recorded and shown, the last good meetings stay; a cached day that isn't today is ignored.
- D44 wording, exactly: `tt-cal not found · run helpers/tt-cal/build.sh`, `no access · System Settings → Privacy & Security → Calendars`, `unknown calendar "X"`, `⚠ cal: <reason>`, dimmed `cal 12m ago` once the last success is older than two polls; D40 `cal: not set up`; D41 `meeting · read-only`; D42 `· ⚠ overlaps meeting "Standup" 10:00–10:30`; D43 `no free slot left today fits 1h`, `no conflict · m moves it`.
- D36 still holds: blocks never overlap each other; only meetings may be overlapped.
- Lines never wrap: every rendered line is truncated to the terminal width, and a screen's body never exceeds `bodyHeight()`.
- The helper is only compiled while implementing (into `$TMPDIR`), never run against the real calendar: its first real run (the permission prompt) and the check against the real calendar are the user's.
- Every task ends green on: `gofmt -l .` (no output), `go vet ./...`, `go test ./...`; Tasks 4–6 also `go test -race ./internal/tui/ ./internal/app/`.
- Every commit message ends with a blank line and `Co-Authored-By: Claude <noreply@anthropic.com>`. Commit messages use single quotes.
- Sandbox: save each patch to a file and `git apply <file>`; run plain, separate git commands (no `git -C`, no `cd … &&`). If `git apply` fails, stop: an earlier step went wrong.

## Review Focus

1. **The helper's first run, or a broken one** (not built yet, access denied, the permission prompt waiting for a click while tt fetches in the background, a typo in a calendar name): the fetch fails or times out after 10s, Today keeps working, and the header says what to do. Pinned in Task 2 `TestEventsFailures`, `TestEventsTimesOut`, `TestEventsFindsTheHelper`, Task 3 `TestCalendarComesFromTheConfig`, Task 4 `TestCalendarHeaderStatus`.
2. **tt left open overnight (or the laptop asleep across the day boundary)**: yesterday's cached meetings never show on the new day; the new day fetches at once. Pinned in Task 3 `TestCalendarCacheOfAnotherDayIsIgnored` and Task 4 `TestCalendarPollLoop`.
3. **Working after midnight** (still the same planning day): the fetch covers the planning day (day_start to day_start), not the calendar date. Pinned in Task 3 `TestSyncCalendarAfterMidnightFetchesThePlanningDay`.
4. **A real calendar's shapes**: back-to-back and overlapping meetings, 15-minute standups, a meeting that starts before the working hours: one band with `+N`, exact times off the grid, named on its first row inside the hours; nothing outside the hours shown. Pinned in Task 5 `TestTimelineMeetings` and `TestMeetingsOnToday`.
5. **A stale timeline when a key lands** (another tt blocked the slot, or moved the block being fine-tuned): the service refuses and the pick-up comes back; fine-tune applies to the block as it is now. Pinned in Task 6 `TestARefusedDropKeepsThePickUp` and `TestFineTuneRereadsTheBlock`.

## File Structure

```
helpers/tt-cal/main.swift, Info.plist, build.sh   new: the EventKit helper (D40)
internal/domain/event.go                           new: Event, Busy (D17a)
internal/calendar/eventkit.go, fake.go             new: the tt-cal adapter (D44 errors), a fake
internal/config/config.go                          modify: [calendar] calendars, poll
internal/store/migrations/004_integration_state.sql new
internal/store/integration.go                      new: Integration, read/upsert
internal/app/calendar.go                           new: CalendarSource, WithCalendar, SyncCalendar, CalendarState
internal/app/service.go, day.go                    modify: Store.Integration, Service.cal, State.Calendar in Load
cmd/tt/main.go                                     modify: EventKit when calendars are configured
internal/tui/model.go                              modify: calLoop, calTickMsg/calSyncedMsg, syncCalendar, R, actOr
internal/tui/common.go, overlays.go                modify: calendarStatus in the header, styleGhostAlert, help
internal/tui/timeline.go                           modify: meetings, bands, busy gapFrom, conflict, block
internal/tui/today.go                              modify: meeting rows, ⚠, statuses, r, carry-forward fixes
internal/tui/*_test.go                             new/modify: harness with a fake calendar, meetings, re-slot, real program
```

Work happens on branch `plan-5b` in a worktree; fast-forward merge to `main` when the final review is green.

---
"""

FOOTER = """
## After the last task

- Build tt and a fake helper into the scratchpad and drive the real binary through a Python `pty` script on a throwaway database: a `tt-cal` script next to the built `tt` that prints today's `Standup` (09:30–10:00) for `events` and `config.toml` with `[calendar] calendars = ["Work"]` (`TT_CONFIG`, `TT_DB`). Today shows `■ Standup` and no calendar status; `L`, `a` + `Alpha !1 ~s` + `⏎`, `p`, `B`, `b`: the ghost avoids the meeting; move it onto the meeting, `⏎`: `⚠ overlaps meeting "Standup"`; `r` → `moving "Alpha"`, `⏎` → `moved`; `q`. Then make the script `exit 2` and `R`: `⚠ cal: no access · …` while `■ Standup` stays. Without `[calendar]`: `cal: not set up (tt-cal calendars)`.
- `go test -race ./internal/tui/ ./internal/app/`.
- Final whole-branch review, one fix wave, park residuals as spec carry-forwards.
- **The user's check, together:** `helpers/tt-cal/build.sh`, then `tt-cal calendars` (the macOS permission prompt), copy the names into `~/.config/tt/config.toml` under `[calendar] calendars`, `go build -o ~/.local/bin/tt ./cmd/tt`, open tt: today's meetings against the real calendar.
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
