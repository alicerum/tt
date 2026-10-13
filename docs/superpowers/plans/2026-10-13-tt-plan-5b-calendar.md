# tt Plan 5b — Calendar Implementation Plan

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
### Task 1: The `tt-cal` helper (Swift, EventKit)

D40 and the Adapters section: a dumb Swift CLI that prints calendar names, or the events of a range as raw JSON, with the spec's exit codes. No Go code. The calendar usage description is embedded in the binary by `build.sh` (without it macOS 14+ never asks for access).

**Files:**
- Create: `helpers/tt-cal/Info.plist`
- Create: `helpers/tt-cal/build.sh`
- Create: `helpers/tt-cal/main.swift`

**Interfaces:**
- Consumes: nothing.
- Produces (the CLI contract Task 2's adapter relies on):
  - `tt-cal calendars`: the calendar names, unique and sorted, one per line.
  - `tt-cal events --from <RFC 3339> --to <RFC 3339> [--calendars <name>…]`: every argument after `--calendars` is a name; no `--calendars` means all calendars. Prints one JSON array of `{"title", "start", "end", "all_day", "availability", "status", "calendar"}` (`start`/`end` RFC 3339 in the local zone; `availability` is `busy`/`free`/`tentative`/`unavailable`/`""`; `status` is my participation `accepted`/`declined`/`tentative`/`pending`/`""`).
  - Exit codes: 0 ok, 1 failure (usage errors print the usage; they happen before any calendar access), 2 no calendar access, 3 unknown calendar (the name alone on stderr).
  - `helpers/tt-cal/build.sh [DIR]`: builds into `DIR` (default `~/.local/bin`).

- [ ] **Step 1: Create the files**

Create `helpers/tt-cal/Info.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleIdentifier</key>
	<string>dev.tt.tt-cal</string>
	<key>CFBundleName</key>
	<string>tt-cal</string>
	<key>NSCalendarsFullAccessUsageDescription</key>
	<string>tt shows your meetings on its Today timeline.</string>
</dict>
</plist>
```

Create `helpers/tt-cal/build.sh`:

```sh
#!/bin/sh
# Builds tt-cal (D40) into ~/.local/bin, or into the directory given as $1.
# The usage description is embedded in the binary: without it macOS (14+)
# never asks for calendar access.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
out=${1:-$HOME/.local/bin}
mkdir -p "$out"
swiftc -O "$here/main.swift" -o "$out/tt-cal" \
	-Xlinker -sectcreate -Xlinker __TEXT -Xlinker __info_plist -Xlinker "$here/Info.plist"
echo "built $out/tt-cal · next: tt-cal calendars (the first run asks for calendar access)"
```

Make it executable: `helpers/tt-cal/build.sh` (`chmod +x helpers/tt-cal/build.sh`).

Create `helpers/tt-cal/main.swift`:

```swift
// tt-cal reads macOS Calendar (EventKit) for tt and prints raw events as
// JSON. It has no business rules: tt decides what counts as busy (D17a).
// Build it with build.sh (the calendar usage description must be embedded).
import EventKit
import Foundation

let usage = """
    usage:
      tt-cal calendars
          print the calendar names, one per line (copy them into [calendar] calendars)
      tt-cal events --from <iso8601> --to <iso8601> [--calendars <name>…]
          print the events overlapping from–to as JSON, from the named calendars (default: all)
    exit codes: 0 ok, 1 failure, 2 no calendar access, 3 unknown calendar (its name on stderr)
    """

func fail(_ message: String, _ code: Int32) -> Never {
    FileHandle.standardError.write(Data((message + "\n").utf8))
    exit(code)
}

// requestAccess asks once (the first run shows the macOS prompt) and exits 2
// when access is denied or restricted.
func requestAccess(_ store: EKEventStore) {
    switch EKEventStore.authorizationStatus(for: .event) {
    case .fullAccess:
        return
    case .notDetermined:
        let answered = DispatchSemaphore(value: 0)
        nonisolated(unsafe) var granted = false
        store.requestFullAccessToEvents { ok, _ in
            granted = ok
            answered.signal()
        }
        answered.wait()
        if !granted {
            fail("no calendar access", 2)
        }
    default:
        fail("no calendar access", 2)
    }
}

let iso: ISO8601DateFormatter = {
    let f = ISO8601DateFormatter()
    f.timeZone = TimeZone.current
    return f
}()

func parseDate(_ s: String) -> Date {
    guard let d = iso.date(from: s) else {
        fail("bad date \"\(s)\": want ISO 8601, like 2026-10-12T04:00:00+02:00", 1)
    }
    return d
}

struct Event: Encodable {
    let title: String
    let start: String
    let end: String
    let allDay: Bool
    let availability: String  // busy, free, tentative, unavailable, or "" when the calendar has none
    let status: String  // my participation: accepted, declined, tentative, pending, or "" when not invited
    let calendar: String

    enum CodingKeys: String, CodingKey {
        case title, start, end, availability, status, calendar
        case allDay = "all_day"
    }
}

func availability(_ a: EKEventAvailability) -> String {
    switch a {
    case .busy: return "busy"
    case .free: return "free"
    case .tentative: return "tentative"
    case .unavailable: return "unavailable"
    default: return ""
    }
}

func status(_ e: EKEvent) -> String {
    guard let me = e.attendees?.first(where: { $0.isCurrentUser }) else {
        return ""
    }
    switch me.participantStatus {
    case .accepted: return "accepted"
    case .declined: return "declined"
    case .tentative: return "tentative"
    case .pending: return "pending"
    default: return ""
    }
}

func listCalendars(_ store: EKEventStore) {
    let names = Set(store.calendars(for: .event).map { $0.title })
    for name in names.sorted() {
        print(name)
    }
}

struct Query {
    let from, to: Date
    let names: [String]?  // nil: all calendars
}

func parseQuery(_ args: [String]) -> Query {
    var from: Date?
    var to: Date?
    var names: [String]?
    var i = 0
    while i < args.count {
        switch args[i] {
        case "--from" where i + 1 < args.count:
            from = parseDate(args[i + 1])
            i += 2
        case "--to" where i + 1 < args.count:
            to = parseDate(args[i + 1])
            i += 2
        case "--calendars":
            names = Array(args[(i + 1)...])
            i = args.count
        default:
            fail(usage, 1)
        }
    }
    guard let from = from, let to = to else {
        fail(usage, 1)
    }
    return Query(from: from, to: to, names: names)
}

func listEvents(_ store: EKEventStore, _ q: Query) {
    var calendars: [EKCalendar]?
    if let names = q.names {
        let all = store.calendars(for: .event)
        calendars = []
        for name in names {
            let matching = all.filter { $0.title == name }
            if matching.isEmpty {
                fail(name, 3)
            }
            calendars! += matching
        }
    }
    let predicate = store.predicateForEvents(withStart: q.from, end: q.to, calendars: calendars)
    let events = store.events(matching: predicate).sorted { $0.startDate < $1.startDate }.map {
        Event(
            title: $0.title ?? "", start: iso.string(from: $0.startDate), end: iso.string(from: $0.endDate),
            allDay: $0.isAllDay, availability: availability($0.availability), status: status($0),
            calendar: $0.calendar.title)
    }
    do {
        FileHandle.standardOutput.write(try JSONEncoder().encode(events))
        print()
    } catch {
        fail("can't encode the events: \(error)", 1)
    }
}

let args = Array(CommandLine.arguments.dropFirst())
guard let command = args.first, command == "calendars" || command == "events" else {
    fail(usage, 1)
}
let query = command == "events" ? parseQuery(Array(args.dropFirst())) : nil  // usage errors before any access
let store = EKEventStore()
requestAccess(store)
if let query = query {
    listEvents(store, query)
} else {
    listCalendars(store)
}
```

- [ ] **Step 2: Build and check**

Run (builds into a throwaway directory; **don't** build into `~/.local/bin` and **don't** run `tt-cal calendars` or a valid `tt-cal events` here: the first real run asks for calendar access, and that run is the user's):

```bash
sh helpers/tt-cal/build.sh "$TMPDIR/tt-cal-build"
"$TMPDIR/tt-cal-build/tt-cal"; echo "exit $?"
"$TMPDIR/tt-cal-build/tt-cal" events --from x; echo "exit $?"
"$TMPDIR/tt-cal-build/tt-cal" events --from 2026-10-12T04:00:00+02:00; echo "exit $?"
otool -s __TEXT __info_plist "$TMPDIR/tt-cal-build/tt-cal" | head -2
```

Expected: `built …/tt-cal · next: tt-cal calendars (the first run asks for calendar access)` with no compiler warnings; the usage text and `exit 1`; `bad date "x": want ISO 8601, like 2026-10-12T04:00:00+02:00` and `exit 1`; the usage and `exit 1` (no `--to`); `otool` prints a `(__TEXT,__info_plist) section`. Then `gofmt -l .`, `go vet ./...`, `go test ./...` still pass (no Go changes).

- [ ] **Step 3: Commit**

```bash
git add helpers/tt-cal/Info.plist helpers/tt-cal/build.sh helpers/tt-cal/main.swift
git commit -m 'feat(tt-cal): Swift EventKit helper: calendars and events as raw JSON (D40)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 2: `domain.Event` and `Busy`; the `tt-cal` adapter and a fake

D17a's busy rule in `domain`, and `internal/calendar`: the adapter that runs the helper (next to tt, else on `PATH`; 10s timeout; exit codes mapped to D44's short reasons) and a fake for the app and TUI tests. Adapter tests drive small fake-helper shell scripts.

**Files:**
- Create: `internal/calendar/eventkit.go`
- Create: `internal/calendar/fake.go`
- Create: `internal/domain/event.go`
- Create: `internal/calendar/eventkit_test.go`
- Create: `internal/domain/event_test.go`

**Interfaces:**
- Consumes: Task 1's CLI contract (only through the fake scripts in tests).
- Produces:
  - `domain.Event{Title string; Start, End time.Time; AllDay bool; Availability, Status, Calendar string}`.
  - `func domain.Busy(events []Event) []Event`: drops all-day, `Status == "declined"`, `Availability == "free"` and zero-length events; tentative stays; sorted by start, then end.
  - `calendar.EventKit{Calendars []string; Path string; Timeout time.Duration}` (`Path` "" = next to the running tt, else `tt-cal` on `PATH`; `Timeout` 0 = `calendar.DefaultTimeout`, 10s); `func (*EventKit) Events(ctx, from, to time.Time) ([]domain.Event, error)`.
  - Errors: `calendar.ErrHelperNotFound` (`tt-cal not found · run helpers/tt-cal/build.sh`), `calendar.ErrNoAccess` (`no access · System Settings → Privacy & Security → Calendars`, exit 2), `unknown calendar "X"` (exit 3), `tt-cal timed out after 10s`, `tt-cal printed bad JSON: …`, else the first line of stderr, else `tt-cal failed: exit status N`; a cancelled ctx returns `ctx.Err()`.
  - `calendar.Fake` (mutex-guarded): `Set(events []domain.Event, err error)`, `Events(...)`, `Calls() int`, `Asked() (from, to time.Time)`.

- [ ] **Step 1: Write the failing tests**

Create `internal/calendar/eventkit_test.go`:

```go
package calendar

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// helper writes a fake tt-cal script into dir and returns its path.
func helper(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "tt-cal")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

var (
	from = time.Date(2026, 10, 12, 4, 0, 0, 0, time.FixedZone("CEST", 2*3600))
	to   = from.Add(24 * time.Hour)
)

func TestEventsRunsTheHelper(t *testing.T) {
	dir := t.TempDir()
	path := helper(t, dir, `for a in "$@"; do echo "$a"; done > "$(dirname "$0")/args"
cat <<'JSON'
[{"title":"Standup","start":"2026-10-12T09:00:00+02:00","end":"2026-10-12T09:15:00+02:00","all_day":false,
  "availability":"busy","status":"accepted","calendar":"Work"},
 {"title":"Holiday","start":"2026-10-12T00:00:00+02:00","end":"2026-10-13T00:00:00+02:00","all_day":true,
  "availability":"free","status":"","calendar":"Holidays"}]
JSON`)
	ek := &EventKit{Calendars: []string{"Work", "Team X"}, Path: path}
	events, err := ek.Events(context.Background(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %+v", events)
	}
	e := events[0]
	if e.Title != "Standup" || !e.Start.Equal(from.Add(5*time.Hour)) || e.End.Sub(e.Start) != 15*time.Minute ||
		e.AllDay || e.Availability != "busy" || e.Status != "accepted" || e.Calendar != "Work" {
		t.Errorf("first event = %+v", e)
	}
	if !events[1].AllDay || events[1].Availability != "free" {
		t.Errorf("second event = %+v", events[1])
	}
	args, err := os.ReadFile(filepath.Join(dir, "args"))
	if err != nil {
		t.Fatal(err)
	}
	want := "events\n--from\n2026-10-12T04:00:00+02:00\n--to\n2026-10-13T04:00:00+02:00\n--calendars\nWork\nTeam X\n"
	if string(args) != want {
		t.Errorf("args:\n%s\nwant:\n%s", args, want)
	}
}

func TestEventsFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       string
	}{
		{"no access", "exit 2", ErrNoAccess.Error()},
		{"unknown calendar", "echo 'Team X' >&2; exit 3", `unknown calendar "Team X"`},
		{"stderr", "printf '\\n  EventKit exploded  \\nmore detail\\n' >&2; exit 1", "EventKit exploded"},
		{"silent", "exit 4", "tt-cal failed: exit status 4"},
		{"bad JSON", "echo '{not json'", "tt-cal printed bad JSON: "},
		{"bad time", `echo '[{"title":"x","start":"9am","end":"10am"}]'`, "tt-cal printed bad JSON: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ek := &EventKit{Path: helper(t, t.TempDir(), tc.body)}
			_, err := ek.Events(context.Background(), from, to)
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Errorf("err = %v; want %q", err, tc.want)
			}
		})
	}
}

func TestEventsTimesOut(t *testing.T) {
	// The sleep is a child of the shell: killing the shell leaves it holding
	// the output pipe, which must not keep Events waiting either.
	ek := &EventKit{Path: helper(t, t.TempDir(), "sleep 10"), Timeout: 100 * time.Millisecond}
	began := time.Now()
	_, err := ek.Events(context.Background(), from, to)
	if err == nil || err.Error() != "tt-cal timed out after 100ms" {
		t.Errorf("err = %v; want the timeout", err)
	}
	if took := time.Since(began); took > 3*time.Second {
		t.Errorf("Events took %v after a timeout", took)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ek.Events(ctx, from, to); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled context: err = %v; want context.Canceled", err)
	}
}

func TestEventsFindsTheHelper(t *testing.T) {
	ek := &EventKit{Path: filepath.Join(t.TempDir(), "tt-cal")} // not built
	if _, err := ek.Events(context.Background(), from, to); !errors.Is(err, ErrHelperNotFound) {
		t.Errorf("missing helper: err = %v; want ErrHelperNotFound", err)
	}

	empty := t.TempDir()
	t.Setenv("PATH", empty)
	if _, err := (&EventKit{}).Events(context.Background(), from, to); !errors.Is(err, ErrHelperNotFound) {
		t.Errorf("not on PATH: err = %v; want ErrHelperNotFound", err)
	}
	onPath := t.TempDir()
	helper(t, onPath, "echo '[]'")
	t.Setenv("PATH", onPath+string(os.PathListSeparator)+"/bin:/usr/bin")
	if events, err := (&EventKit{}).Events(context.Background(), from, to); err != nil || len(events) != 0 {
		t.Errorf("on PATH: %v, %v; want no events and no error", events, err)
	}

	beside := t.TempDir() // next to tt wins over PATH (D40)
	if got := locate(beside); got != "tt-cal" {
		t.Errorf("locate without a helper beside tt = %q; want the PATH lookup", got)
	}
	p := helper(t, beside, "echo '[]'")
	if got := locate(beside); got != p {
		t.Errorf("locate = %q; want %q", got, p)
	}
}

func TestFake(t *testing.T) {
	var f Fake
	f.Set(nil, errors.New("down"))
	if _, err := f.Events(context.Background(), from, to); err == nil || f.Calls() != 1 {
		t.Errorf("Set's error comes back: %v, calls %d", err, f.Calls())
	}
	if a, b := f.Asked(); !a.Equal(from) || !b.Equal(to) {
		t.Errorf("Asked = %v, %v", a, b)
	}
}
```

Create `internal/domain/event_test.go`:

```go
package domain

import (
	"testing"
	"time"
)

func TestBusy(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 12, h, m, 0, 0, time.UTC) }
	ev := func(title string, from, to time.Time) Event {
		return Event{Title: title, Start: from, End: to, Availability: "busy", Status: "accepted"}
	}
	late := ev("Late", at(15, 0), at(16, 0))
	standup := ev("Standup", at(9, 0), at(9, 15))
	tentative := ev("Maybe", at(11, 0), at(12, 0))
	tentative.Status, tentative.Availability = "tentative", "tentative"
	own := ev("Own", at(13, 0), at(14, 0)) // my own event: no attendees, no status
	own.Status = ""
	allDay := ev("Holiday", at(0, 0), at(24, 0))
	allDay.AllDay = true
	declined := ev("Declined", at(10, 0), at(11, 0))
	declined.Status = "declined"
	free := ev("Focus (free)", at(10, 0), at(11, 0))
	free.Availability = "free"
	empty := ev("Reminder", at(12, 0), at(12, 0))

	got := Busy([]Event{late, allDay, standup, declined, tentative, free, own, empty})
	var titles []string
	for _, e := range got {
		titles = append(titles, e.Title)
	}
	want := []string{"Standup", "Maybe", "Own", "Late"}
	if len(titles) != len(want) {
		t.Fatalf("Busy = %v; want %v", titles, want)
	}
	for i := range want {
		if titles[i] != want[i] {
			t.Fatalf("Busy = %v; want %v (by start)", titles, want)
		}
	}
	if Busy(nil) != nil {
		t.Error("Busy(nil) is empty")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/domain/ ./internal/calendar/`
Expected: FAIL (build): `undefined: Event`, `undefined: Busy`, `undefined: EventKit`, `undefined: ErrNoAccess`, `undefined: locate`.

- [ ] **Step 3: Implement**

Create `internal/calendar/eventkit.go`:

```go
// Package calendar reads meetings from macOS Calendar through the tt-cal
// helper (spec: Adapters). It has no rules of its own: domain.Busy decides
// what is busy, and app caches the result.
package calendar

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"tt/internal/domain"
)

// The failures with a fix of their own (D44); their text is what the header shows.
var (
	ErrHelperNotFound = errors.New("tt-cal not found · run helpers/tt-cal/build.sh")
	ErrNoAccess       = errors.New("no access · System Settings → Privacy & Security → Calendars")
)

// DefaultTimeout bounds one helper run (D44).
const DefaultTimeout = 10 * time.Second

// EventKit runs `tt-cal events` for the configured calendars (D40).
type EventKit struct {
	Calendars []string      // the calendars whose events count as busy
	Path      string        // the helper; "" finds it next to the running tt, else on PATH
	Timeout   time.Duration // 0 means DefaultTimeout
}

// Events returns the events overlapping from–to. Errors are short and say
// what to do: ErrHelperNotFound, ErrNoAccess (exit 2), `unknown calendar "X"`
// (exit 3), a timeout, bad JSON, else the first line of the helper's stderr.
func (e *EventKit) Events(ctx context.Context, from, to time.Time) ([]domain.Event, error) {
	path := e.Path
	if path == "" {
		path = locate(executableDir())
	}
	timeout := cmp.Or(e.Timeout, DefaultTimeout)
	run, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{"events", "--from", from.Format(time.RFC3339), "--to", to.Format(time.RFC3339)}
	if len(e.Calendars) > 0 {
		args = append(append(args, "--calendars"), e.Calendars...)
	}
	cmd := exec.CommandContext(run, path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = time.Second // a child left holding the pipes can't keep us waiting
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return parse(stdout.Bytes())
	case errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist):
		return nil, ErrHelperNotFound
	case ctx.Err() != nil:
		return nil, ctx.Err() // tt is quitting
	case run.Err() != nil:
		return nil, fmt.Errorf("tt-cal timed out after %s", timeout)
	case errors.As(err, &exit) && exit.ExitCode() == 2:
		return nil, ErrNoAccess
	case errors.As(err, &exit) && exit.ExitCode() == 3:
		return nil, fmt.Errorf("unknown calendar %q", firstLine(stderr.String()))
	}
	if line := firstLine(stderr.String()); line != "" {
		return nil, errors.New(line)
	}
	return nil, fmt.Errorf("tt-cal failed: %v", err)
}

// rawEvent is one event of the helper's JSON.
type rawEvent struct {
	Title        string    `json:"title"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	AllDay       bool      `json:"all_day"`
	Availability string    `json:"availability"`
	Status       string    `json:"status"`
	Calendar     string    `json:"calendar"`
}

func parse(data []byte) ([]domain.Event, error) {
	var raw []rawEvent
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("tt-cal printed bad JSON: %v", err)
	}
	events := make([]domain.Event, 0, len(raw))
	for _, r := range raw {
		events = append(events, domain.Event{Title: r.Title, Start: r.Start, End: r.End, AllDay: r.AllDay,
			Availability: r.Availability, Status: r.Status, Calendar: r.Calendar})
	}
	return events, nil
}

// locate is the tt-cal next to tt (in dir), else the bare name, which exec
// looks up on PATH (D40).
func locate(dir string) string {
	if dir != "" {
		p := filepath.Join(dir, "tt-cal")
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return "tt-cal"
}

func executableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}
```

Create `internal/calendar/fake.go`:

```go
package calendar

import (
	"context"
	"sync"
	"time"

	"tt/internal/domain"
)

// Fake is a calendar for tests: it returns what Set gave it and remembers
// how it was asked. Safe for concurrent use (the TUI fetches in the background).
type Fake struct {
	mu       sync.Mutex
	events   []domain.Event
	err      error
	calls    int
	from, to time.Time
}

// Set makes the next fetches return events, or err when it isn't nil.
func (f *Fake) Set(events []domain.Event, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events, f.err = events, err
}

func (f *Fake) Events(_ context.Context, from, to time.Time) ([]domain.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.from, f.to = from, to
	if f.err != nil {
		return nil, f.err
	}
	return append([]domain.Event(nil), f.events...), nil
}

// Calls is how many fetches there were.
func (f *Fake) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// Asked is the range of the last fetch.
func (f *Fake) Asked() (from, to time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.from, f.to
}
```

Create `internal/domain/event.go`:

```go
package domain

import (
	"cmp"
	"slices"
	"time"
)

// Event is a calendar event as the tt-cal helper reports it (spec: Adapters).
type Event struct {
	Title        string
	Start, End   time.Time
	AllDay       bool
	Availability string // "busy", "free", "tentative", "unavailable"; "" when the calendar has none
	Status       string // my participation: "accepted", "declined", "tentative", "pending"; "" when not invited
	Calendar     string
}

// Busy keeps the events that make time busy (D17a): all-day, declined and
// "free" events aren't busy, nor is an event without length; tentative ones
// are. The result is sorted by start, then end.
func Busy(events []Event) []Event {
	var out []Event
	for _, e := range events {
		if e.AllDay || e.Status == "declined" || e.Availability == "free" || !e.End.After(e.Start) {
			continue
		}
		out = append(out, e)
	}
	slices.SortStableFunc(out, func(a, b Event) int {
		return cmp.Or(a.Start.Compare(b.Start), a.End.Compare(b.End))
	})
	return out
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/domain/ ./internal/calendar/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS (the calendar tests take ~2s: the timeout test waits out `WaitDelay`), no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/calendar/eventkit.go internal/calendar/eventkit_test.go internal/calendar/fake.go internal/domain/event.go internal/domain/event_test.go
git commit -m 'feat(calendar): domain.Event and Busy (D17a); the tt-cal adapter and a fake

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 3: `[calendar]` config, `integration_state`, `SyncCalendar`

D40's config, migration 004 and the calendar cache (D3a): `SyncCalendar` fetches the planning day, stores the busy events on success and records the error otherwise (the cache stays); `Load` returns the meetings and the calendar status, ignoring a cache of another day (D44). `cmd/tt` sets the calendar up only when calendars are configured. `TestNewerSchemaIsRefused` stops hardcoding the newest migration.

**Files:**
- Modify: `cmd/tt/main.go`
- Create: `internal/app/calendar.go`
- Modify: `internal/app/day.go`
- Modify: `internal/app/service.go`
- Modify: `internal/config/config.go`
- Create: `internal/store/integration.go`
- Create: `internal/store/migrations/004_integration_state.sql`
- Modify: `cmd/tt/main_test.go`
- Create: `internal/app/calendar_test.go`
- Create: `internal/config/calendar_test.go`
- Modify: `internal/store/hardening_test.go`
- Create: `internal/store/integration_test.go`

**Interfaces:**
- Consumes: Task 2 (`domain.Event`, `domain.Busy`, `calendar.EventKit`, `calendar.Fake`, `calendar.ErrNoAccess`).
- Produces:
  - `config.Config.Calendars []string` (none = not set up), `CalendarPoll time.Duration` (default 2m); TOML `[calendar] calendars = […]`, `poll = "2m"`. Errors: `calendar.calendars[N]: name is empty (tt-cal calendars prints the names)`, `calendar.poll: want a duration of 10s or more, like "2m", got "…"`.
  - Migration `004_integration_state.sql`: `integration_state(name PK, last_success_at, last_error, last_error_at, payload)`.
  - `store.Integration{Name string; LastSuccessAt time.Time; LastError string; LastErrorAt time.Time; Payload string}`; `(*Store).Integration(ctx, name) (Integration, bool, error)` (read-only tx); `(*Tx).GetIntegration(ctx, name)`, `(*Tx).PutIntegration(ctx, Integration) error` (upsert). `app.Store` gains `Integration`.
  - `app.CalendarSource` interface `{ Events(ctx, from, to time.Time) ([]domain.Event, error) }`; `(*Service).WithCalendar(src) *Service`; `(*Service).CalendarSetUp() bool`; `(*Service).SyncCalendar(ctx) error` (a failed fetch is recorded, not returned; nil without a calendar; a cancelled ctx records nothing).
  - `app.CalendarState{SetUp bool; Meetings []domain.Event; LastSuccess time.Time; Err string}`; `app.State.Calendar` (filled by `Load`; meetings in the clock's zone, only for today's planning day; an unreadable cache becomes `Err`, never a load failure).
  - Test helpers (app): `calService(t) (*Service, *fakeClock, *calendar.Fake, *store.Store)`, `mustLoad`, `mustSync`, `at(h, m)`, `meeting(title, from, to)`, `titles`.

- [ ] **Step 1: Write the failing tests**

Patch `cmd/tt/main_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/cmd/tt/main_test.go b/cmd/tt/main_test.go
index 957921a..a2d1e46 100644
--- a/cmd/tt/main_test.go
+++ b/cmd/tt/main_test.go
@@ -141,6 +141,39 @@ func TestNoArgsStartsTheTUI(t *testing.T) {
 	}
 }
 
+func TestCalendarComesFromTheConfig(t *testing.T) {
+	ctx := context.Background()
+	env := testEnv(t)
+	t.Setenv("PATH", t.TempDir()) // no tt-cal anywhere
+	calendarOf := func() app.CalendarState {
+		t.Helper()
+		var cs app.CalendarState
+		var out, errOut bytes.Buffer
+		code := run(ctx, nil, input{}, &out, &errOut, func(k string) string { return env[k] },
+			func(ctx context.Context, svc *app.Service) error {
+				if err := svc.SyncCalendar(ctx); err != nil {
+					return err
+				}
+				st, err := svc.Load(ctx)
+				cs = st.Calendar
+				return err
+			})
+		if code != 0 {
+			t.Fatalf("exit %d, stderr %q", code, errOut.String())
+		}
+		return cs
+	}
+	if cs := calendarOf(); cs.SetUp {
+		t.Errorf("no [calendar] calendars: %+v; want not set up (D40)", cs)
+	}
+	if err := os.WriteFile(env["TT_CONFIG"], []byte("[calendar]\ncalendars = [\"Work\"]\n"), 0o644); err != nil {
+		t.Fatal(err)
+	}
+	if cs := calendarOf(); !cs.SetUp || cs.Err != "tt-cal not found · run helpers/tt-cal/build.sh" {
+		t.Errorf("calendars set, no helper: %+v; want the D44 hint", cs)
+	}
+}
+
 func TestTUIErrorsExitNonZero(t *testing.T) {
 	env := testEnv(t)
 	var out, errOut bytes.Buffer
```

Create `internal/app/calendar_test.go`:

```go
package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"tt/internal/calendar"
	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/store"
)

// calService is a started service with a fake calendar; it also returns the
// store, for writing a cache by hand.
func calService(t *testing.T) (*Service, *fakeClock, *calendar.Fake, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "tt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	clock := &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
	cal := &calendar.Fake{}
	s := New(st, clock, config.Default()).WithCalendar(cal)
	if _, err := s.StartDay(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s, clock, cal, st
}

func mustLoad(t *testing.T, s *Service) State {
	t.Helper()
	st, err := s.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func mustSync(t *testing.T, s *Service) {
	t.Helper()
	if err := s.SyncCalendar(context.Background()); err != nil {
		t.Fatalf("SyncCalendar: %v", err)
	}
}

func at(h, m int) time.Time { return time.Date(2026, 10, 12, h, m, 0, 0, time.UTC) }

func meeting(title string, from, to time.Time) domain.Event {
	return domain.Event{Title: title, Start: from, End: to, Availability: "busy", Status: "accepted"}
}

func titles(events []domain.Event) []string {
	var out []string
	for _, e := range events {
		out = append(out, e.Title)
	}
	return out
}

func TestCalendarNotSetUp(t *testing.T) {
	s, _ := startedService(t)
	if err := s.SyncCalendar(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.CalendarSetUp() {
		t.Error("CalendarSetUp without WithCalendar")
	}
	if cs := mustLoad(t, s).Calendar; cs.SetUp || cs.Meetings != nil || cs.Err != "" || !cs.LastSuccess.IsZero() {
		t.Errorf("no calendar configured (D40): %+v", cs)
	}
}

func TestSyncCalendarCachesBusyEvents(t *testing.T) {
	s, clock, cal, _ := calService(t)
	if !s.CalendarSetUp() {
		t.Error("CalendarSetUp after WithCalendar")
	}
	if cs := mustLoad(t, s).Calendar; !cs.SetUp || cs.Meetings != nil || !cs.LastSuccess.IsZero() || cs.Err != "" {
		t.Fatalf("before the first fetch: %+v", cs)
	}
	declined := meeting("Declined", at(10, 0), at(11, 0))
	declined.Status = "declined"
	allDay := meeting("Holiday", at(0, 0), at(24, 0))
	allDay.AllDay = true
	cal.Set([]domain.Event{meeting("Review", at(14, 0), at(15, 0)), declined, allDay, meeting("Standup", at(9, 30), at(9, 45))}, nil)
	mustSync(t, s)

	from, to := cal.Asked()
	if !from.Equal(at(4, 0)) || !to.Equal(at(28, 0)) {
		t.Errorf("fetched %v–%v; want the planning day 04:00–04:00", from, to)
	}
	cs := mustLoad(t, s).Calendar
	if got := titles(cs.Meetings); len(got) != 2 || got[0] != "Standup" || got[1] != "Review" {
		t.Fatalf("meetings = %v; want the busy ones by start (D17a)", got)
	}
	if m := cs.Meetings[0]; !m.Start.Equal(at(9, 30)) || !m.End.Equal(at(9, 45)) {
		t.Errorf("Standup = %v–%v", m.Start, m.End)
	}
	if !cs.LastSuccess.Equal(clock.now) || cs.Err != "" {
		t.Errorf("after a success: %+v", cs)
	}

	// D3a: a failed fetch keeps the last good meetings and says why.
	clock.now = clock.now.Add(2 * time.Minute)
	cal.Set(nil, calendar.ErrNoAccess)
	mustSync(t, s)
	cs = mustLoad(t, s).Calendar
	if len(cs.Meetings) != 2 || cs.Err != calendar.ErrNoAccess.Error() || !cs.LastSuccess.Equal(at(9, 0)) {
		t.Errorf("after a failure: %d meetings, err %q, last success %v", len(cs.Meetings), cs.Err, cs.LastSuccess)
	}

	// A success clears the error; no busy events is an empty day, not a failure.
	clock.now = clock.now.Add(2 * time.Minute)
	cal.Set([]domain.Event{declined}, nil)
	mustSync(t, s)
	if cs = mustLoad(t, s).Calendar; cs.Meetings != nil || cs.Err != "" || !cs.LastSuccess.Equal(clock.now) {
		t.Errorf("after an empty success: %+v", cs)
	}
}

func TestCalendarCacheOfAnotherDayIsIgnored(t *testing.T) {
	s, clock, cal, _ := calService(t)
	cal.Set([]domain.Event{meeting("Standup", at(9, 30), at(9, 45))}, nil)
	mustSync(t, s)

	clock.now = time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC) // the next planning day, before its first fetch
	cs := mustLoad(t, s).Calendar
	if cs.Meetings != nil || !cs.LastSuccess.Equal(at(9, 0)) {
		t.Errorf("yesterday's cache (D44): %d meetings, last success %v; want none and yesterday's time", len(cs.Meetings), cs.LastSuccess)
	}
}

func TestSyncCalendarAfterMidnightFetchesThePlanningDay(t *testing.T) {
	s, clock, cal, _ := calService(t)
	clock.now = time.Date(2026, 10, 13, 1, 30, 0, 0, time.UTC) // still Mon's planning day (day_start 04:00)
	cal.Set([]domain.Event{meeting("Late call", at(23, 30), at(24, 30))}, nil)
	mustSync(t, s)
	if from, to := cal.Asked(); !from.Equal(at(4, 0)) || !to.Equal(at(28, 0)) {
		t.Errorf("fetched %v–%v; want Mon 04:00–Tue 04:00", from, to)
	}
	if cs := mustLoad(t, s).Calendar; len(cs.Meetings) != 1 {
		t.Errorf("the planning day's cache is today's: %+v", cs)
	}
}

func TestSyncCalendarWhileQuittingRecordsNothing(t *testing.T) {
	s, _, cal, _ := calService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cal.Set(nil, context.Canceled)
	if err := s.SyncCalendar(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want context.Canceled", err)
	}
	if cs := mustLoad(t, s).Calendar; cs.Err != "" {
		t.Errorf("a cancelled fetch isn't the calendar's failure: %+v", cs)
	}
}

func TestUnreadableCalendarCacheDoesNotBreakTheLoad(t *testing.T) {
	s, _, _, st := calService(t)
	err := st.InTx(context.Background(), func(x *store.Tx) error {
		return x.PutIntegration(context.Background(), store.Integration{Name: "calendar", LastSuccessAt: at(8, 0), Payload: "{oops"})
	})
	if err != nil {
		t.Fatal(err)
	}
	cs := mustLoad(t, s).Calendar
	if cs.Meetings != nil || cs.Err == "" {
		t.Errorf("a broken cache shows as an error: %+v", cs)
	}
}
```

Create `internal/config/calendar_test.go`:

```go
package config

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseCalendar(t *testing.T) {
	if cfg := Default(); cfg.Calendars != nil || cfg.CalendarPoll != 2*time.Minute {
		t.Errorf("defaults: calendars %v, poll %v; want none (not set up) and 2m", cfg.Calendars, cfg.CalendarPoll)
	}
	cfg, err := Parse([]byte("[calendar]\ncalendars = [\"Work\", \"Team X\"]\npoll = \"5m\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cfg.Calendars, []string{"Work", "Team X"}) || cfg.CalendarPoll != 5*time.Minute {
		t.Errorf("calendar: %v every %v", cfg.Calendars, cfg.CalendarPoll)
	}
	cases := map[string]string{
		"[calendar]\ncalendars = [\"Work\", \" \"]": "calendar.calendars[1]: name is empty",
		"[calendar]\npoll = \"2\"":                  "calendar.poll: want a duration of 10s or more",
		"[calendar]\npoll = \"1s\"":                 "calendar.poll: want a duration of 10s or more",
		"[calendar]\nnames = [\"Work\"]":            "unknown config key",
		"[calendar]\ncalendars = \"Work\"":          "calendars",
	}
	for input, wantErr := range cases {
		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}
```

Patch `internal/store/hardening_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/store/hardening_test.go b/internal/store/hardening_test.go
index 546fa88..81f4b88 100644
--- a/internal/store/hardening_test.go
+++ b/internal/store/hardening_test.go
@@ -53,7 +53,7 @@ func TestNewerSchemaIsRefused(t *testing.T) {
 		t.Fatal(err)
 	}
 	// Drop the newest real migration's row: a refused Open must not re-apply it.
-	if _, err := s.db.Exec("DELETE FROM schema_migrations WHERE version = 3"); err != nil {
+	if _, err := s.db.Exec("DELETE FROM schema_migrations WHERE version = (SELECT max(version) FROM schema_migrations WHERE version < 999)"); err != nil {
 		t.Fatal(err)
 	}
 	s.Close()
```

Create `internal/store/integration_test.go`:

```go
package store

import (
	"context"
	"testing"
	"time"
)

func TestIntegrationState(t *testing.T) {
	ctx := context.Background()
	s, path := openTemp(t)
	if in, ok, err := s.Integration(ctx, "calendar"); err != nil || ok || in.Name != "calendar" || !in.LastSuccessAt.IsZero() {
		t.Fatalf("before the first fetch: %+v, %v, %v", in, ok, err)
	}
	ok1 := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		return x.PutIntegration(ctx, Integration{Name: "calendar", LastSuccessAt: ok1, Payload: `{"day":"2026-10-12"}`})
	})
	failed := ok1.Add(2 * time.Minute)
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		in, ok, err := x.GetIntegration(ctx, "calendar")
		if err != nil || !ok {
			t.Fatalf("GetIntegration: %v, %v", ok, err)
		}
		in.LastError, in.LastErrorAt = "no access", failed
		return x.PutIntegration(ctx, in) // replaces the row
	})
	s.Close()

	s, err := Open(ctx, path) // kept across restarts
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	in, ok, err := s.Integration(ctx, "calendar")
	if err != nil || !ok {
		t.Fatalf("Integration: %v, %v", ok, err)
	}
	if !in.LastSuccessAt.Equal(ok1) || in.LastError != "no access" || !in.LastErrorAt.Equal(failed) || in.Payload != `{"day":"2026-10-12"}` {
		t.Errorf("stored state = %+v", in)
	}
	if _, ok, _ := s.Integration(ctx, "github"); ok {
		t.Error("each integration has a row of its own")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/ ./internal/store/ ./internal/app/ ./cmd/tt/`
Expected: FAIL (build): `cfg.Calendars undefined`, `s.Integration undefined`, `x.PutIntegration undefined`, `WithCalendar undefined`, `s.SyncCalendar undefined`, `undefined: app.CalendarState`.

- [ ] **Step 3: Implement**

Patch `cmd/tt/main.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/cmd/tt/main.go b/cmd/tt/main.go
index f4e0c1a..9a0a46b 100644
--- a/cmd/tt/main.go
+++ b/cmd/tt/main.go
@@ -14,6 +14,7 @@ import (
 	tea "charm.land/bubbletea/v2"
 
 	"tt/internal/app"
+	"tt/internal/calendar"
 	"tt/internal/config"
 	"tt/internal/domain"
 	"tt/internal/dsl"
@@ -177,7 +178,11 @@ func open(ctx context.Context, getenv func(string) string) (*app.Service, func()
 	if err != nil {
 		return nil, nil, err
 	}
-	return app.New(st, app.SystemClock{}, cfg), func() { st.Close() }, nil
+	svc := app.New(st, app.SystemClock{}, cfg)
+	if len(cfg.Calendars) > 0 { // D40: no calendars, no calendar
+		svc.WithCalendar(&calendar.EventKit{Calendars: cfg.Calendars})
+	}
+	return svc, func() { st.Close() }, nil
 }
 
 func paths(getenv func(string) string) (configPath, dbPath string, err error) {
```

Create `internal/app/calendar.go`:

```go
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"tt/internal/domain"
	"tt/internal/store"
)

// CalendarSource is where meetings come from (spec: Interfaces); the tt-cal
// adapter in internal/calendar implements it.
type CalendarSource interface {
	Events(ctx context.Context, from, to time.Time) ([]domain.Event, error)
}

// WithCalendar sets the calendar SyncCalendar reads. Without one the
// calendar isn't set up (D40). Call it before the service is used.
func (s *Service) WithCalendar(src CalendarSource) *Service {
	s.cal = src
	return s
}

// CalendarSetUp reports whether a calendar is configured (D40).
func (s *Service) CalendarSetUp() bool { return s.cal != nil }

// CalendarState is what the views show of the calendar (D41, D44).
type CalendarState struct {
	SetUp       bool           // a calendar is configured (D40)
	Meetings    []domain.Event // today's busy events from the last good fetch, by start
	LastSuccess time.Time      // zero: never
	Err         string         // why the last fetch failed; "" once one succeeds
}

const calendarName = "calendar" // its integration_state row

// calPayload is the cached result of the last good fetch.
type calPayload struct {
	Day    domain.Day `json:"day"`
	Events []calEvent `json:"events"`
}

type calEvent struct {
	Title string    `json:"title"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// SyncCalendar fetches the planning day's events. On success it stores the
// busy ones (D17a) as the cache; on failure it records the error and keeps
// the cache (D3a). A failed fetch isn't returned: the views show it from the
// state (D44). Only a failure to record the fetch is.
func (s *Service) SyncCalendar(ctx context.Context) error {
	if s.cal == nil {
		return nil
	}
	today := s.Today()
	loc := s.clock.Now().Location()
	events, fetchErr := s.cal.Events(ctx, today.At(s.cfg.DayStart, loc), today.At(s.cfg.DayStart+24*time.Hour, loc))
	if ctx.Err() != nil {
		return ctx.Err() // tt is quitting; that isn't the calendar's fault
	}
	now := s.clock.Now()
	return s.store.InTx(ctx, func(x *store.Tx) error {
		in, _, err := x.GetIntegration(ctx, calendarName)
		if err != nil {
			return err
		}
		if fetchErr != nil {
			in.LastError, in.LastErrorAt = fetchErr.Error(), now
			return x.PutIntegration(ctx, in)
		}
		p := calPayload{Day: today, Events: []calEvent{}}
		for _, e := range domain.Busy(events) {
			p.Events = append(p.Events, calEvent{Title: e.Title, Start: e.Start, End: e.End})
		}
		data, err := json.Marshal(p)
		if err != nil {
			return err
		}
		in.LastSuccessAt, in.LastError, in.LastErrorAt, in.Payload = now, "", time.Time{}, string(data)
		return x.PutIntegration(ctx, in)
	})
}

// calendarState reads the cache for today. A cache of another day is
// ignored (D44); an unreadable one shows as an error instead of failing
// the load.
func (s *Service) calendarState(ctx context.Context, today domain.Day) (CalendarState, error) {
	if s.cal == nil {
		return CalendarState{}, nil
	}
	cs := CalendarState{SetUp: true}
	in, ok, err := s.store.Integration(ctx, calendarName)
	if err != nil || !ok {
		return cs, err
	}
	cs.LastSuccess, cs.Err = in.LastSuccessAt, in.LastError
	if in.Payload == "" {
		return cs, nil
	}
	var p calPayload
	if err := json.Unmarshal([]byte(in.Payload), &p); err != nil {
		if cs.Err == "" {
			cs.Err = fmt.Sprintf("can't read the cached meetings: %v", err)
		}
		return cs, nil
	}
	if p.Day != today {
		return cs, nil
	}
	loc := s.clock.Now().Location()
	for _, e := range p.Events {
		cs.Meetings = append(cs.Meetings, domain.Event{Title: e.Title, Start: e.Start.In(loc), End: e.End.In(loc)})
	}
	return cs, nil
}
```

Patch `internal/app/day.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/day.go b/internal/app/day.go
index 28da0b8..13bab78 100644
--- a/internal/app/day.go
+++ b/internal/app/day.go
@@ -136,11 +136,12 @@ type State struct {
 	Version  int64 // DataVersion read before loading; compare to detect later writes
 	Snapshot domain.Snapshot
 	Plan     domain.Plan
+	Calendar CalendarState
 }
 
 // Load reads the data version first, then all tasks and today's plan in one
-// read. A write that lands during the load changes the version, so the next
-// version check reloads again.
+// read, then the calendar's cache. A write that lands during the load changes
+// the version, so the next version check reloads again.
 func (s *Service) Load(ctx context.Context) (State, error) {
 	version, err := s.store.DataVersion(ctx)
 	if err != nil {
@@ -151,7 +152,11 @@ func (s *Service) Load(ctx context.Context) (State, error) {
 	if err != nil {
 		return State{}, err
 	}
-	return State{Version: version, Snapshot: snap, Plan: s.buildPlan(today, snap, state)}, nil
+	cal, err := s.calendarState(ctx, today)
+	if err != nil {
+		return State{}, err
+	}
+	return State{Version: version, Snapshot: snap, Plan: s.buildPlan(today, snap, state), Calendar: cal}, nil
 }
 
 func (s *Service) buildPlan(today domain.Day, snap domain.Snapshot, state store.PlanState) domain.Plan {
```

Patch `internal/app/service.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/service.go b/internal/app/service.go
index 139b832..3c54fd1 100644
--- a/internal/app/service.go
+++ b/internal/app/service.go
@@ -26,6 +26,7 @@ type Store interface {
 	LoadPlan(ctx context.Context, day domain.Day) (domain.Snapshot, store.PlanState, error)
 	DataVersion(ctx context.Context) (int64, error)
 	UIState(ctx context.Context, key string) (string, bool, error)
+	Integration(ctx context.Context, name string) (store.Integration, bool, error)
 }
 
 // Undo reverts the command that returned it. The TUI keeps these on a
@@ -36,6 +37,7 @@ type Service struct {
 	store Store
 	clock Clock
 	cfg   config.Config
+	cal   CalendarSource // nil: no calendar set up (D40)
 }
 
 func New(st Store, clock Clock, cfg config.Config) *Service {
```

Patch `internal/config/config.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/config/config.go b/internal/config/config.go
index 49c1d98..8e31416 100644
--- a/internal/config/config.go
+++ b/internal/config/config.go
@@ -30,6 +30,11 @@ type Config struct {
 	// WorkStart and WorkEnd are the working hours (D39), as offsets from
 	// midnight: Today's timeline shows them, and blocks must fit inside them.
 	WorkStart, WorkEnd time.Duration
+	// Calendars names the calendars whose events count as busy (D40); none
+	// means no calendar: nothing is fetched.
+	Calendars []string
+	// CalendarPoll is how often the calendar is fetched (D3).
+	CalendarPoll time.Duration
 }
 
 func Default() Config {
@@ -50,6 +55,7 @@ func Default() Config {
 		RolloverAlert: 3,
 		WorkStart:     8 * time.Hour,
 		WorkEnd:       19 * time.Hour,
+		CalendarPoll:  2 * time.Minute,
 	}
 }
 
@@ -66,6 +72,10 @@ type file struct {
 		Start *string `toml:"start"`
 		End   *string `toml:"end"`
 	} `toml:"work_hours"`
+	Calendar struct {
+		Calendars []string `toml:"calendars"`
+		Poll      *string  `toml:"poll"`
+	} `toml:"calendar"`
 }
 
 // Load reads path. A missing file yields Default().
@@ -150,6 +160,19 @@ func Parse(data []byte) (Config, error) {
 	} else if cfg.WorkEnd <= cfg.WorkStart {
 		cfg.WorkEnd = min(cfg.WorkStart+time.Hour, 24*time.Hour-time.Minute)
 	}
+	for i, name := range f.Calendar.Calendars {
+		if strings.TrimSpace(name) == "" {
+			return Config{}, fmt.Errorf("calendar.calendars[%d]: name is empty (tt-cal calendars prints the names)", i)
+		}
+	}
+	cfg.Calendars = f.Calendar.Calendars
+	if p := f.Calendar.Poll; p != nil {
+		d, err := time.ParseDuration(*p)
+		if err != nil || d < 10*time.Second {
+			return Config{}, fmt.Errorf("calendar.poll: want a duration of 10s or more, like \"2m\", got %q", *p)
+		}
+		cfg.CalendarPoll = d
+	}
 	switch {
 	case cfg.WorkEnd <= cfg.WorkStart:
 		return Config{}, fmt.Errorf("work_hours.end must be after work_hours.start")
```

Create `internal/store/integration.go`:

```go
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Integration is the last fetch of one integration (D3a): when it last
// worked, the last error ("" after a success), and its cached data, which a
// failed fetch leaves alone.
type Integration struct {
	Name          string
	LastSuccessAt time.Time // zero: never
	LastError     string
	LastErrorAt   time.Time
	Payload       string
}

type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Integration returns name's stored state; ok is false before its first fetch.
func (s *Store) Integration(ctx context.Context, name string) (in Integration, ok bool, err error) {
	err = s.readTx(ctx, func(tx *sql.Tx) error {
		in, ok, err = getIntegration(ctx, tx, name)
		return err
	})
	return in, ok, err
}

func (x *Tx) GetIntegration(ctx context.Context, name string) (Integration, bool, error) {
	return getIntegration(ctx, x.tx, name)
}

// PutIntegration stores in, replacing name's previous state.
func (x *Tx) PutIntegration(ctx context.Context, in Integration) error {
	_, err := x.tx.ExecContext(ctx, `INSERT INTO integration_state
		(name, last_success_at, last_error, last_error_at, payload) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (name) DO UPDATE SET last_success_at = excluded.last_success_at,
			last_error = excluded.last_error, last_error_at = excluded.last_error_at, payload = excluded.payload`,
		in.Name, nullTime(in.LastSuccessAt), in.LastError, nullTime(in.LastErrorAt), in.Payload)
	return err
}

func getIntegration(ctx context.Context, q rowQuerier, name string) (Integration, bool, error) {
	in := Integration{Name: name}
	var success, errorAt sql.NullString
	err := q.QueryRowContext(ctx, `SELECT last_success_at, last_error, last_error_at, payload
		FROM integration_state WHERE name = ?`, name).Scan(&success, &in.LastError, &errorAt, &in.Payload)
	if errors.Is(err, sql.ErrNoRows) {
		return in, false, nil
	}
	if err != nil {
		return Integration{}, false, err
	}
	if in.LastSuccessAt, err = parseNullTime(success); err != nil {
		return Integration{}, false, err
	}
	if in.LastErrorAt, err = parseNullTime(errorAt); err != nil {
		return Integration{}, false, err
	}
	return in, true, nil
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return formatTime(t)
}

func parseNullTime(s sql.NullString) (time.Time, error) {
	if !s.Valid {
		return time.Time{}, nil
	}
	return parseTime(s.String)
}
```

Create `internal/store/migrations/004_integration_state.sql`:

```sql
-- The last fetch of each integration (D3a): the calendar's busy events live
-- in payload, kept when a later fetch fails.
CREATE TABLE integration_state (
    name            TEXT PRIMARY KEY,
    last_success_at TEXT,
    last_error      TEXT NOT NULL DEFAULT '',
    last_error_at   TEXT,
    payload         TEXT NOT NULL DEFAULT ''
);
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/config/ ./internal/store/ ./internal/app/ ./cmd/tt/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add cmd/tt/main.go cmd/tt/main_test.go internal/app/calendar.go internal/app/calendar_test.go internal/app/day.go internal/app/service.go internal/config/calendar_test.go internal/config/config.go internal/store/hardening_test.go internal/store/integration.go internal/store/integration_test.go internal/store/migrations/004_integration_state.sql
git commit -m 'feat: [calendar] config, integration_state (migration 004), SyncCalendar caches busy events (D3a, D40, D44)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 4: The calendar poll loop, `R`, the header status; a real `tea.Program` test

D3/D44 in the TUI: startup shows the cache and fetches in the background; then one fetch every `poll`; `R` and a new planning day fetch at once (a sequence number retires the tick they overtook, so there is only ever one loop). The header shows nothing while fresh, `cal 5m ago` once the last success is older than two polls, `⚠ cal: <reason>` after a failure, `cal: not set up (tt-cal calendars)` without calendars. Plus the Plan 3 carry-forward: one test in a real `tea.Program` on real timers, run with `-race`.

**Files:**
- Modify: `internal/tui/common.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/overlays.go`
- Create: `internal/tui/calendar_test.go`
- Modify: `internal/tui/harness_test.go`
- Create: `internal/tui/program_test.go`

**Interfaces:**
- Consumes: Task 3 (`Service.CalendarSetUp`, `SyncCalendar`, `State.Calendar`, `Config.CalendarPoll`), Task 2 (`calendar.Fake`, `calendar.ErrNoAccess`).
- Produces:
  - `Model.cal calLoop{syncing bool; seq int}`; messages `calTickMsg{seq int}`, `calSyncedMsg{err error}`; `(*Model).syncCalendar() tea.Cmd` (nil when not set up or a fetch is running).
  - `(*Model).calendarStatus() string`, appended to `headerView`.
  - `R` = reload + fetch now; help line `R        reload and fetch the calendar now · q quit · ? this help`.
  - Test helpers: `newCalHarness(t, cfg)` (a harness with `h.cal *calendar.Fake`, fetched once at startup), `startHarness(t, cfg, cal)`.

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/calendar_test.go`:

```go
package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/calendar"
	"tt/internal/config"
)

func TestCalendarHeaderStatus(t *testing.T) {
	h := newHarness(t, config.Default())
	h.mustSee("tt · Today · Mon 12 Oct · cal: not set up (tt-cal calendars)") // D40

	h = newCalHarness(t, config.Default())
	if n := h.cal.Calls(); n != 1 {
		t.Fatalf("startup fetches the calendar once, got %d", n)
	}
	h.mustNotSee(" · cal", "⚠") // fresh: nothing (D44)

	h.cal.Set(nil, calendar.ErrNoAccess)
	h.keys("R")
	if n := h.cal.Calls(); n != 2 {
		t.Fatalf("R fetches now, got %d fetches", n)
	}
	h.mustSee("tt · Today · Mon 12 Oct · ⚠ cal: no access · System Settings → Privacy & Security → Calendars")

	h.cal.Set(nil, nil)
	h.keys("R")
	h.mustNotSee("⚠")
	h.clock.now = h.clock.now.Add(4 * time.Minute) // two polls of 2m: still fresh
	h.mustNotSee(" · cal")
	h.clock.now = h.clock.now.Add(time.Minute)
	h.mustSee("Mon 12 Oct · cal 5m ago")
}

func TestCalendarPollLoop(t *testing.T) {
	cfg := config.Default()
	cfg.CalendarPoll = 3 * time.Minute
	h := newCalHarness(t, cfg)
	var waits []time.Duration
	var ticks []calTickMsg
	h.m.opts.Schedule = func(d time.Duration, msg tea.Msg) tea.Cmd {
		if tick, ok := msg.(calTickMsg); ok {
			waits, ticks = append(waits, d), append(ticks, tick)
		}
		return nil
	}

	h.send(calTickMsg{seq: h.m.cal.seq})
	if n := h.cal.Calls(); n != 2 || len(ticks) != 1 || waits[0] != 3*time.Minute {
		t.Fatalf("a tick fetches and schedules the next one after the poll: %d fetches, waits %v", n, waits)
	}
	h.keys("R") // overtakes the scheduled tick
	if n := h.cal.Calls(); n != 3 || len(ticks) != 2 {
		t.Fatalf("R: %d fetches, %d ticks scheduled", n, len(ticks))
	}
	h.send(ticks[0]) // the overtaken tick
	if n := h.cal.Calls(); n != 3 {
		t.Errorf("an overtaken tick doesn't fetch again (one loop only): %d fetches", n)
	}
	h.send(ticks[1])
	if n := h.cal.Calls(); n != 4 {
		t.Errorf("the current tick fetches: %d fetches", n)
	}

	h.m.cal.syncing = true // a fetch still running
	h.keys("R")
	if n := h.cal.Calls(); n != 4 {
		t.Errorf("one fetch at a time: %d fetches", n)
	}
	h.m.cal.syncing = false

	h.clock.now = h.clock.now.Add(24 * time.Hour) // a new planning day fetches at once
	h.send(clockTickMsg{})
	if n := h.cal.Calls(); n != 5 || h.m.banner == nil {
		t.Errorf("new day: %d fetches, banner %v", n, h.m.banner != nil)
	}
}
```

Patch `internal/tui/harness_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/harness_test.go b/internal/tui/harness_test.go
index 1bc65b8..5f7b623 100644
--- a/internal/tui/harness_test.go
+++ b/internal/tui/harness_test.go
@@ -12,6 +12,7 @@ import (
 	"github.com/charmbracelet/x/ansi"
 
 	"tt/internal/app"
+	"tt/internal/calendar"
 	"tt/internal/config"
 	"tt/internal/domain"
 	"tt/internal/store"
@@ -29,6 +30,7 @@ type harness struct {
 	m     *Model
 	svc   *app.Service
 	clock *fakeClock
+	cal   *calendar.Fake // nil: no calendar set up
 	path  string
 	quit  bool
 
@@ -40,6 +42,18 @@ type harness struct {
 }
 
 func newHarness(t *testing.T, cfg config.Config) *harness {
+	t.Helper()
+	return startHarness(t, cfg, nil)
+}
+
+// newCalHarness is newHarness with a fake calendar (h.cal), fetched once at
+// startup like the real one; Set its events, then R fetches again.
+func newCalHarness(t *testing.T, cfg config.Config) *harness {
+	t.Helper()
+	return startHarness(t, cfg, &calendar.Fake{})
+}
+
+func startHarness(t *testing.T, cfg config.Config, cal *calendar.Fake) *harness {
 	t.Helper()
 	path := filepath.Join(t.TempDir(), "tt.db")
 	st, err := store.Open(context.Background(), path)
@@ -49,7 +63,10 @@ func newHarness(t *testing.T, cfg config.Config) *harness {
 	t.Cleanup(func() { st.Close() })
 	clock := &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
 	svc := app.New(st, clock, cfg)
-	h := &harness{t: t, svc: svc, clock: clock, path: path}
+	if cal != nil {
+		svc.WithCalendar(cal)
+	}
+	h := &harness{t: t, svc: svc, clock: clock, cal: cal, path: path}
 	opts := Options{WatchEvery: time.Second, ClockEvery: 30 * time.Second,
 		Schedule: func(time.Duration, tea.Msg) tea.Cmd { return nil },
 		Exec: func(c *exec.Cmd, done tea.ExecCallback) tea.Cmd {
```

Create `internal/tui/program_test.go`:

```go
package tui

import (
	"context"
	"io"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/app"
	"tt/internal/calendar"
	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/store"
)

// TestRealProgram runs the model in a real tea.Program (Plan 3
// carry-forward): the watch, clock and calendar loops run on real timers and
// their commands on other goroutines, so `go test -race` checks that only
// messages carry state into the model. Another process adds a task meanwhile.
func TestRealProgram(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "tt.db")
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	clock := &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
	cfg := config.Default()
	cfg.CalendarPoll = 10 * time.Millisecond
	cal := &calendar.Fake{}
	cal.Set([]domain.Event{{Title: "Standup", Start: clock.now.Add(30 * time.Minute), End: clock.now.Add(time.Hour)}}, nil)
	opts := DefaultOptions()
	opts.WatchEvery, opts.ClockEvery = 10*time.Millisecond, 10*time.Millisecond
	m := New(ctx, app.New(st, clock, cfg).WithCalendar(cal), opts)

	var sawTask, sawMeeting atomic.Bool
	watch := func(model tea.Model, msg tea.Msg) tea.Msg { // runs on the program's goroutine
		if mm, ok := model.(*Model); ok && mm.loaded {
			sawTask.Store(len(mm.state.Snapshot.Tasks()) > 0)
			sawMeeting.Store(len(mm.state.Calendar.Meetings) > 0)
		}
		return msg
	}
	keys, typing := io.Pipe()
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(keys), tea.WithOutput(io.Discard),
		tea.WithWindowSize(120, 30), tea.WithoutSignals(), tea.WithFilter(watch))
	done := make(chan error, 1)
	go func() {
		_, err := p.Run()
		done <- err
	}()

	other, err := store.Open(ctx, path) // e.g. `tt add` in another pane
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, _, err := app.New(other, clock, cfg).Add(ctx, "From another shell !1 ~s"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !(sawTask.Load() && sawMeeting.Load() && cal.Calls() >= 3) {
		if time.Now().After(deadline) {
			t.Fatalf("after 10s: task seen %v, meeting seen %v, %d calendar fetches", sawTask.Load(), sawMeeting.Load(), cal.Calls())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := typing.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(10 * time.Second):
		p.Kill()
		t.Fatal("q didn't quit the program")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL (build): `undefined: calTickMsg`, `h.m.cal undefined`.

- [ ] **Step 3: Implement**

Patch `internal/tui/common.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/common.go b/internal/tui/common.go
index e244999..00944aa 100644
--- a/internal/tui/common.go
+++ b/internal/tui/common.go
@@ -109,7 +109,23 @@ func (m *Model) headerView() string {
 	if m.state.Plan.Committed {
 		h += styleDim.Render(" · plan committed")
 	}
-	return h
+	return h + m.calendarStatus()
+}
+
+// calendarStatus is the header's calendar part (D44): nothing while fresh,
+// the age of the last success once it is older than two polls, the reason
+// after a failed fetch.
+func (m *Model) calendarStatus() string {
+	cs, now := m.state.Calendar, m.svc.Now()
+	switch {
+	case !cs.SetUp:
+		return styleDim.Render(" · cal: not set up (tt-cal calendars)") // D40
+	case cs.Err != "":
+		return styleAlert.Render(" · ⚠ cal: " + cs.Err)
+	case !cs.LastSuccess.IsZero() && now.Sub(cs.LastSuccess) > 2*m.svc.Config().CalendarPoll:
+		return styleDim.Render(" · cal " + ago(now, cs.LastSuccess))
+	}
+	return ""
 }
 
 func (m *Model) bannerView() string {
```

Patch `internal/tui/model.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/model.go b/internal/tui/model.go
index ba130d0..a499d10 100644
--- a/internal/tui/model.go
+++ b/internal/tui/model.go
@@ -86,6 +86,8 @@ type Model struct {
 	inbox  inboxState
 	detail detailState
 
+	cal calLoop
+
 	overlay     overlay
 	banner      *app.DayResult
 	undo        []undoEntry
@@ -97,6 +99,14 @@ func New(ctx context.Context, svc *app.Service, opts Options) *Model {
 	return &Model{svc: svc, opts: opts, ctx: ctx, width: 100, height: 30, pick: pickState{group: search.ByPriority}}
 }
 
+// calLoop is the calendar's poll loop (D3): one fetch at a time, then the
+// next tick after the configured interval. R and a new day fetch at once;
+// seq retires the tick they overtook, so there is only ever one loop.
+type calLoop struct {
+	syncing bool
+	seq     int
+}
+
 // Messages.
 type (
 	loadedMsg struct {
@@ -127,7 +137,9 @@ type (
 		group string
 		err   error
 	}
-	uiSavedMsg struct{ err error }
+	uiSavedMsg   struct{ err error }
+	calTickMsg   struct{ seq int }
+	calSyncedMsg struct{ err error }
 )
 
 func (m *Model) Init() tea.Cmd {
@@ -138,6 +150,7 @@ func (m *Model) Init() tea.Cmd {
 			return uiStateMsg{group: g, err: err}
 		},
 		m.startDay(),
+		m.syncCalendar(), // the first load shows the cache meanwhile
 		m.opts.Schedule(m.opts.WatchEvery, watchTickMsg{}),
 		m.opts.Schedule(m.opts.ClockEvery, clockTickMsg{}),
 	)
@@ -193,6 +206,7 @@ func (m *Model) update(msg tea.Msg) tea.Cmd {
 			m.banner = &res
 			m.today.placed = false // a new day's cursor starts on its now row
 			m.today.carry = nil
+			return tea.Batch(m.load(), m.syncCalendar()) // yesterday's meetings don't count (D44)
 		}
 		return m.load()
 	case actionMsg:
@@ -242,6 +256,17 @@ func (m *Model) update(msg tea.Msg) tea.Cmd {
 			m.pick.saveAgain = false
 			return m.saveGroup()
 		}
+	case calTickMsg:
+		if msg.seq == m.cal.seq {
+			return m.syncCalendar()
+		}
+	case calSyncedMsg:
+		m.cal.syncing = false
+		m.cal.seq++
+		if msg.err != nil && !errors.Is(msg.err, context.Canceled) {
+			m.setErr(fmt.Errorf("can't record the calendar fetch: %w", msg.err))
+		}
+		return tea.Batch(m.load(), m.opts.Schedule(m.svc.Config().CalendarPoll, calTickMsg{seq: m.cal.seq}))
 	case noteEditedMsg:
 		return m.noteEdited(msg)
 	case tea.KeyPressMsg:
@@ -298,7 +323,7 @@ func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
 	case "ctrl+z":
 		return m.undoLast()
 	case "R":
-		return m.load()
+		return tea.Batch(m.load(), m.syncCalendar())
 	}
 	if m.dayIsStale() {
 		m.setStatus("new day — refreshing")
@@ -337,6 +362,17 @@ func (m *Model) checkVersion() tea.Cmd {
 	}
 }
 
+// syncCalendar fetches the calendar in the background, unless none is set
+// up or a fetch is already running.
+func (m *Model) syncCalendar() tea.Cmd {
+	if !m.svc.CalendarSetUp() || m.cal.syncing {
+		return nil
+	}
+	m.cal.syncing = true
+	svc, ctx := m.svc, m.ctx
+	return func() tea.Msg { return calSyncedMsg{err: svc.SyncCalendar(ctx)} }
+}
+
 func (m *Model) startDay() tea.Cmd {
 	svc, ctx := m.svc, m.ctx
 	return func() tea.Msg {
```

Patch `internal/tui/overlays.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/overlays.go b/internal/tui/overlays.go
index 2c74374..186dfc6 100644
--- a/internal/tui/overlays.go
+++ b/internal/tui/overlays.go
@@ -253,7 +253,7 @@ func (helpOverlay) view(m *Model) string {
 	global := []string{
 		"a        quick-add: title @who #tag !1 ~s link -- note (tab completes @/#)",
 		"ctrl+z   undo (refuses if the task changed since)",
-		"R        reload · q quit · ? this help",
+		"R        reload and fetch the calendar now · q quit · ? this help",
 	}
 	var local []string
 	switch m.screen {
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`
Expected: PASS, no gofmt output. (Verified while planning: `TestRealProgram` under `-race` reports a planted read of `m.state` from a command goroutine, so it does catch races.)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/calendar_test.go internal/tui/common.go internal/tui/harness_test.go internal/tui/model.go internal/tui/overlays.go internal/tui/program_test.go
git commit -m 'feat(tui): the calendar poll loop, R fetches now, the calendar status in the header (D3, D44); a real tea.Program test

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 5: Meetings on Today; meetings are busy for placement; ⚠ on conflicts

D41 and D42: the timeline gets the meetings within the working hours, overlapping ones merged into one band (`■ Review +1`, exact times when off the grid, named on their first row inside the hours); a row shows its blocks first, then its meetings. `b`'s first free row and the ghost's D24 length treat meetings as busy; dropping over one is allowed and said. A block of an open task overlapping a meeting shows ⚠, and the timeline title counts them. Task keys on a meeting-only row say `meeting · read-only`.

**Files:**
- Modify: `internal/tui/common.go`
- Modify: `internal/tui/timeline.go`
- Modify: `internal/tui/today.go`
- Create: `internal/tui/meetings_test.go`
- Modify: `internal/tui/timeline_test.go`

**Interfaces:**
- Consumes: Task 3 (`State.Calendar.Meetings`), Task 4 (`newCalHarness`, `h.cal`), `Day.Offset`.
- Produces:
  - `type meeting struct{ title string; start, end time.Duration }`; `type band struct{ title string; more int; start, end time.Duration }`; `tlRow.bands []band`; `timeline.meetings []meeting` (only those touching the hours).
  - `buildTimeline(blocks []domain.Block, meetings []meeting, from, to, now time.Duration) timeline` (new second parameter; existing calls pass `nil`).
  - `(*timeline).conflict(b domain.Block) (meeting, bool)`; `gapFrom` (and so `firstFree` and the ghost's length) count meetings as busy.
  - `(*Model).overMeeting(b) string` (` · ⚠ overlaps meeting "Review" 10:00–11:00`, appended to the blocked/moved/now statuses), `(*Model).conflicted(b) bool` (open task only), `(*Model).meetingParts(r tlRow) []string`; `styleGhostAlert`.
  - Test helpers: `(*harness).meetings(events ...domain.Event)` (Set + R), `ev(title, from, to time.Duration) domain.Event`.

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/meetings_test.go`:

```go
package tui

import (
	"context"
	"testing"
	"time"

	"tt/internal/config"
	"tt/internal/domain"
)

// meetings makes the fake calendar return events on the harness's day
// (each "HH:MM–HH:MM title") and fetches them with R.
func (h *harness) meetings(events ...domain.Event) {
	h.t.Helper()
	h.cal.Set(events, nil)
	h.keys("R")
}

func ev(title string, from, to time.Duration) domain.Event {
	day := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	return domain.Event{Title: title, Start: day.Add(from), End: day.Add(to), Availability: "busy", Status: "accepted"}
}

func TestMeetingsOnToday(t *testing.T) {
	h := newCalHarness(t, config.Default())
	fix := h.pinned("Fix CI !1 ~s")
	h.block(fix.ID, 12*time.Hour, time.Hour)
	h.meetings(
		ev("Early", 6*time.Hour, 7*time.Hour),                                   // outside the working hours: not shown
		ev("Breakfast", 7*time.Hour+30*time.Minute, 8*time.Hour+30*time.Minute), // runs into them
		ev("Standup", 9*time.Hour+30*time.Minute, 9*time.Hour+45*time.Minute),
		ev("Review", 10*time.Hour, 11*time.Hour),
		ev("Sync", 10*time.Hour+30*time.Minute, 11*time.Hour+15*time.Minute),
		ev("Lunch", 12*time.Hour, 13*time.Hour),
	)
	h.mustSee(
		" 08:00 ■ Breakfast",           // named on its first row inside the hours
		" 09:30 ■ 09:30–09:45 Standup", // off the grid: exact times (D41)
		" 10:00 ■ 10:00–11:15 Review +1", " 10:30 ■", " 11:00 ■",
		" 12:00 ▓ Fix CI [P1 S] ⚠ · ■ Lunch", " 12:30 ▓ · ■", // the block first, then the meeting
		"TIMELINE · 0/1 done · ⚠ 1",
	)
	h.mustNotSee("Early", " 11:30 ■")

	h.keys("j", "x") // the Standup row: nothing to act on
	h.mustSee("meeting · read-only")
	if h.task(fix.ID).Status != domain.StatusOpen {
		t.Fatal("a key on a meeting row touched a task")
	}
	h.keys("j", "j", "j", "j", "j", "x") // 12:00: the block's task
	h.mustSee(`✓ "Fix CI"`)
	h.mustNotSee("⚠") // a closed task's block is history: no conflict
}

func TestPlacementAroundMeetings(t *testing.T) {
	h := newCalHarness(t, config.Default())
	h.meetings(
		ev("Standup", 9*time.Hour, 9*time.Hour+30*time.Minute),
		ev("Review", 10*time.Hour, 11*time.Hour),
	)
	h.pinned("Alpha !1 ~s")
	h.keys("tab", "b")
	h.mustSee(" 09:30 ░ Alpha 30m") // D42: the first row free of meetings; the length stops at Review
	h.keys("j")
	h.mustSee(" 10:00 ░ Alpha 1h ⚠ · ■ Review") // over a meeting: the whole length, with ⚠
	h.keys("enter")
	h.mustSee(`blocked "Alpha" 10:00–11:00 · ⚠ overlaps meeting "Review" 10:00–11:00`,
		" 10:00 ▓ Alpha [P1 S] ⚠ · ■ Review", "⚠ 1")

	beta := h.pinned("Beta !2 ~s")
	h.block(beta.ID, 14*time.Hour, time.Hour)
	h.mustSee(" 14:00 ▓ Beta [P2 S]")
	h.mustNotSee("Beta [P2 S] ⚠")
	h.meetings( // a meeting that lands on a block later (D18)
		ev("Standup", 9*time.Hour, 9*time.Hour+30*time.Minute),
		ev("Review", 10*time.Hour, 11*time.Hour),
		ev("Escalation", 14*time.Hour+30*time.Minute, 15*time.Hour),
	)
	h.mustSee(" 14:00 ▓ Beta [P2 S] ⚠", " 14:30 ▓ · ■ Escalation", "⚠ 2")
	if b := h.blocks(); len(b) != 2 || b[1].Start != 14*time.Hour {
		t.Fatalf("the block keeps its time (D18): %+v", b)
	}

	h.keys("j", "m", "j", "j", "enter") // Alpha (10:00) to 11:00
	h.mustSee(`moved "Alpha" to 11:00–12:00`, "⚠ 1")
	h.mustNotSee("overlaps meeting")
	h.keys("+")
	h.mustSee(`"Alpha" now 11:00–12:30`)

	ctx := context.Background()
	if _, err := h.svc.Done(ctx, beta.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	h.mustNotSee("⚠")
}
```

Patch `internal/tui/timeline_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/timeline_test.go b/internal/tui/timeline_test.go
index 4dc8a64..d5f2954 100644
--- a/internal/tui/timeline_test.go
+++ b/internal/tui/timeline_test.go
@@ -21,7 +21,7 @@ func starts(tl timeline) []time.Duration {
 }
 
 func TestTimelineWindow(t *testing.T) {
-	tl := buildTimeline(nil, 8*hr, 19*hr, 10*hr+12*mn)
+	tl := buildTimeline(nil, nil, 8*hr, 19*hr, 10*hr+12*mn)
 	if got := starts(tl); len(got) != 22 || got[0] != 8*hr || got[21] != 18*hr+30*mn {
 		t.Fatalf("working hours: %d rows %v…; want 22 rows 08:00…18:30", len(got), got[:1])
 	}
@@ -33,7 +33,7 @@ func TestTimelineWindow(t *testing.T) {
 	// block outside them (placed before D39) gets a row of its own after them.
 	late := domain.Block{ID: 1, TaskID: 1, Start: 21*hr + 10*mn, Duration: 40 * mn}
 	early := domain.Block{ID: 2, TaskID: 2, Start: 6 * hr, Duration: hr}
-	tl = buildTimeline([]domain.Block{early, late}, 8*hr, 19*hr, 6*hr+45*mn)
+	tl = buildTimeline([]domain.Block{early, late}, nil, 8*hr, 19*hr, 6*hr+45*mn)
 	got := starts(tl)
 	if len(got) != 24 || got[0] != 6*hr || got[1] != 8*hr || got[22] != 18*hr+30*mn || got[23] != late.Start {
 		t.Errorf("rows %v; want 06:00 (outside), 08:00…18:30, 21:10 (outside)", got)
@@ -53,7 +53,7 @@ func TestTimelineWindow(t *testing.T) {
 
 	// After the hours (e.g. past midnight, still the same planning day): the
 	// cursor rests on the last working row.
-	tl = buildTimeline(nil, 8*hr, 19*hr, 24*hr+20*mn)
+	tl = buildTimeline(nil, nil, 8*hr, 19*hr, 24*hr+20*mn)
 	if !tl.outsideNow() || tl.homeRow() != 21 {
 		t.Errorf("after hours: outsideNow %v, homeRow %d; want true, 21", tl.outsideNow(), tl.homeRow())
 	}
@@ -61,14 +61,14 @@ func TestTimelineWindow(t *testing.T) {
 	// Hours off the half-hour grid: an edge row doesn't also list a block
 	// that has an outside row.
 	edge := domain.Block{ID: 3, TaskID: 3, Start: 7*hr + 30*mn, Duration: 15 * mn} // 07:30–07:45
-	tl = buildTimeline([]domain.Block{edge}, 7*hr+45*mn, 19*hr, -1)
+	tl = buildTimeline([]domain.Block{edge}, nil, 7*hr+45*mn, 19*hr, -1)
 	if !tl.rows[0].outside || tl.rows[1].start != 7*hr+30*mn || len(tl.rows[1].blocks) != 0 {
 		t.Errorf("hours from 07:45: rows[0] outside %v, rows[1] %v with %d blocks; want the block only in its outside row",
 			tl.rows[0].outside, tl.rows[1].start, len(tl.rows[1].blocks))
 	}
 
 	// No now (a stale day).
-	tl = buildTimeline(nil, 8*hr, 19*hr, -1)
+	tl = buildTimeline(nil, nil, 8*hr, 19*hr, -1)
 	if len(tl.rows) != 22 || tl.nowRow != -1 || tl.outsideNow() {
 		t.Errorf("without now: %d rows, nowRow %d", len(tl.rows), tl.nowRow)
 	}
@@ -78,7 +78,7 @@ func TestTimelineRowsAndGaps(t *testing.T) {
 	a := domain.Block{ID: 1, TaskID: 10, Start: 9 * hr, Duration: 90 * mn}        // 09:00–10:30
 	b := domain.Block{ID: 2, TaskID: 11, Start: 11*hr + 10*mn, Duration: 15 * mn} // 11:10–11:25
 	c := domain.Block{ID: 3, TaskID: 12, Start: 11*hr + 25*mn, Duration: 5 * mn}  // 11:25–11:30
-	tl := buildTimeline([]domain.Block{a, b, c}, 8*hr, 19*hr, 8*hr)
+	tl := buildTimeline([]domain.Block{a, b, c}, nil, 8*hr, 19*hr, 8*hr)
 
 	at := func(off time.Duration) int { return tl.rowOf(off) }
 	if blk, ok := tl.blockAt(at(9*hr + 30*mn)); !ok || blk.ID != a.ID {
@@ -95,7 +95,7 @@ func TestTimelineRowsAndGaps(t *testing.T) {
 	// an earlier row loses to one starting in it.
 	a2 := domain.Block{ID: 1, TaskID: 10, Start: 10 * hr, Duration: 40 * mn}       // 10:00–10:40
 	b2 := domain.Block{ID: 2, TaskID: 11, Start: 10*hr + 40*mn, Duration: 20 * mn} // 10:40–11:00
-	tl2 := buildTimeline([]domain.Block{a2, b2}, 8*hr, 19*hr, 8*hr)
+	tl2 := buildTimeline([]domain.Block{a2, b2}, nil, 8*hr, 19*hr, 8*hr)
 	if blk, ok := tl2.blockAt(tl2.rowOf(10*hr + 30*mn)); !ok || blk.ID != b2.ID {
 		t.Errorf("10:30 row names B (it starts at 10:40 in this row; A only carries on): got %+v %v", blk, ok)
 	}
@@ -130,3 +130,60 @@ func TestTimelineRowsAndGaps(t *testing.T) {
 		t.Error("12:00–13:00 is free")
 	}
 }
+
+func TestTimelineMeetings(t *testing.T) {
+	ms := []meeting{
+		{"Early", 7 * hr, 7*hr + 45*mn}, // fully before the hours: not shown (D41)
+		{"Breakfast", 7*hr + 30*mn, 8*hr + 30*mn},
+		{"Standup", 9*hr + 30*mn, 9*hr + 45*mn},
+		{"Review", 10 * hr, 11 * hr},
+		{"Sync", 10*hr + 30*mn, 11*hr + 15*mn}, // overlaps Review: one band (P4)
+		{"Late", 18*hr + 45*mn, 19*hr + 30*mn},
+		{"After", 19*hr + 30*mn, 20 * hr},
+	}
+	tl := buildTimeline(nil, ms, 8*hr, 19*hr, 9*hr)
+	if len(tl.meetings) != 5 || tl.meetings[0].title != "Breakfast" || tl.meetings[4].title != "Late" {
+		t.Fatalf("meetings within the hours: %+v", tl.meetings)
+	}
+	bandsAt := func(at time.Duration) []band { return tl.rows[tl.rowOf(at)].bands }
+	if b := bandsAt(8 * hr); len(b) != 1 || b[0].title != "Breakfast" {
+		t.Errorf("08:00: %+v", b)
+	}
+	if b := bandsAt(8*hr + 30*mn); len(b) != 0 {
+		t.Errorf("08:30 is free: %+v", b)
+	}
+	want := band{title: "Review", more: 1, start: 10 * hr, end: 11*hr + 15*mn}
+	for _, at := range []time.Duration{10 * hr, 10*hr + 30*mn, 11 * hr} {
+		if b := bandsAt(at); len(b) != 1 || b[0] != want {
+			t.Errorf("%v: %+v; want the merged band %+v", at, b, want)
+		}
+	}
+	if b := bandsAt(18*hr + 30*mn); len(b) != 1 || b[0].title != "Late" {
+		t.Errorf("18:30: %+v", b)
+	}
+
+	// D42: meetings are busy for placement.
+	for _, c := range []struct{ at, want time.Duration }{
+		{8 * hr, 0},                   // inside Breakfast
+		{8*hr + 30*mn, hr},            // until Standup
+		{9*hr + 30*mn, 0},             // inside Standup
+		{9*hr + 45*mn, 15 * mn},       // until Review
+		{11 * hr, 0},                  // inside Sync, which runs on after Review
+		{11*hr + 15*mn, 7*hr + 30*mn}, // until Late
+		{18*hr + 45*mn, 0},            // inside Late, which runs past the hours
+		{19 * hr, 0},                  // outside the hours
+	} {
+		if got := tl.gapFrom(c.at); got != c.want {
+			t.Errorf("gapFrom(%s) = %v; want %v", clock(c.at), got, c.want)
+		}
+	}
+	if i := tl.firstFree(tl.rowOf(8 * hr)); tl.rows[i].start != 8*hr+30*mn {
+		t.Errorf("firstFree from 08:00 = %s; want 08:30 (Breakfast is busy)", clock(tl.rows[i].start))
+	}
+	if mt, ok := tl.conflict(domain.Block{Start: 10*hr + 45*mn, Duration: 45 * mn}); !ok || mt.title != "Review" {
+		t.Errorf("conflict = %+v, %v; want Review, the first one by start", mt, ok)
+	}
+	if _, ok := tl.conflict(domain.Block{Start: 9*hr + 45*mn, Duration: 15 * mn}); ok {
+		t.Error("a block between meetings, touching both, doesn't conflict")
+	}
+}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL (build): `too many arguments in call to buildTimeline`.

- [ ] **Step 3: Implement**

Patch `internal/tui/common.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/common.go b/internal/tui/common.go
index 00944aa..8e32e50 100644
--- a/internal/tui/common.go
+++ b/internal/tui/common.go
@@ -20,8 +20,10 @@ var (
 	styleSelected = lipgloss.NewStyle().Reverse(true)
 	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
 	styleAlert    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
-	styleClosed   = lipgloss.NewStyle().Strikethrough(true).Faint(true)
-	styleBanner   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
+	// styleGhostAlert is a ghost block over a meeting (D42): selected, in the alert colour.
+	styleGhostAlert = lipgloss.NewStyle().Reverse(true).Foreground(lipgloss.Color("3"))
+	styleClosed     = lipgloss.NewStyle().Strikethrough(true).Faint(true)
+	styleBanner     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
 )
 
 // index holds lookups derived from one loaded state.
```

Patch `internal/tui/timeline.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/timeline.go b/internal/tui/timeline.go
index 3f5d675..be4ae13 100644
--- a/internal/tui/timeline.go
+++ b/internal/tui/timeline.go
@@ -13,9 +13,24 @@ const rowLen = 30 * time.Minute
 type tlRow struct {
 	start   time.Duration  // wall-clock offset from midnight of the plan's day
 	blocks  []domain.Block // the blocks that overlap the row, by start
+	bands   []band         // the meeting bands that overlap the row, by start (D41)
 	outside bool           // a row of its own for a block outside the working hours (D39)
 }
 
+// meeting is a busy calendar event (D17a) as offsets on the plan's day.
+type meeting struct {
+	title      string
+	start, end time.Duration
+}
+
+// band is overlapping meetings merged into one (P4): the first one's
+// title, how many more, and the span they cover together.
+type band struct {
+	title      string
+	more       int
+	start, end time.Duration
+}
+
 // timeline is Today's grid (D38): the working hours in rows (D39), rebuilt
 // on every load. A block outside them (placed before D39, or before the
 // hours changed) gets a row of its own before or after them, so it never
@@ -25,15 +40,30 @@ type tlRow struct {
 type timeline struct {
 	rows       []tlRow
 	blocks     []domain.Block // the day's blocks, by start
+	meetings   []meeting      // the meetings within the working hours, by start (D41)
 	now        time.Duration  // now's wall-clock offset; -1 when there is no now
 	nowRow     int            // the row holding now; -1 when now is outside the hours or there is none
 	start, end time.Duration  // the working hours
 }
 
-// buildTimeline lays out the working hours from–to in rows. now < 0 means
-// no now (the shown day isn't today).
-func buildTimeline(blocks []domain.Block, from, to, now time.Duration) timeline {
+// buildTimeline lays out the working hours from–to in rows, with the
+// meetings that touch them; meetings fully outside them aren't shown (D41).
+// now < 0 means no now (the shown day isn't today).
+func buildTimeline(blocks []domain.Block, meetings []meeting, from, to, now time.Duration) timeline {
 	tl := timeline{blocks: blocks, now: max(now, -1), nowRow: -1, start: from, end: to}
+	var bands []band
+	for _, mt := range meetings {
+		if mt.end <= from || mt.start >= to {
+			continue
+		}
+		tl.meetings = append(tl.meetings, mt)
+		if n := len(bands); n > 0 && mt.start < bands[n-1].end {
+			bands[n-1].more++
+			bands[n-1].end = max(bands[n-1].end, mt.end)
+			continue
+		}
+		bands = append(bands, band{title: mt.title, start: mt.start, end: mt.end})
+	}
 	var before, after []tlRow
 	for _, b := range blocks {
 		switch {
@@ -53,6 +83,11 @@ func buildTimeline(blocks []domain.Block, from, to, now time.Duration) timeline
 				r.blocks = append(r.blocks, b)
 			}
 		}
+		for _, bd := range bands {
+			if bd.start < min(s+rowLen, to) && max(s, from) < bd.end {
+				r.bands = append(r.bands, bd)
+			}
+		}
 		if now >= max(s, from) && now < min(s+rowLen, to) {
 			tl.nowRow = len(tl.rows)
 		}
@@ -111,26 +146,38 @@ func (tl *timeline) blockAt(i int) (domain.Block, bool) {
 	return tl.rows[i].blocks[0], true
 }
 
-// gapFrom is the free time from offset at until the next block starts or
-// the working hours end; 0 when a block covers at or at is outside the hours.
+// gapFrom is the free time from offset at until the next block or meeting
+// starts or the working hours end (D42); 0 when a block or a meeting covers
+// at, or at is outside the hours.
 func (tl *timeline) gapFrom(at time.Duration) time.Duration {
 	if at < tl.start || at >= tl.end {
 		return 0
 	}
 	end := tl.end
+	busy := func(start, stop time.Duration) bool {
+		if start <= at && at < stop {
+			return true
+		}
+		if start > at {
+			end = min(end, start)
+		}
+		return false
+	}
 	for _, b := range tl.blocks {
-		if b.Start <= at && at < b.End() {
+		if busy(b.Start, b.End()) {
 			return 0
 		}
-		if b.Start > at {
-			end = min(end, b.Start)
+	}
+	for _, mt := range tl.meetings {
+		if busy(mt.start, mt.end) {
+			return 0
 		}
 	}
 	return max(end-at, 0)
 }
 
-// firstFree is the first row from i on whose start no block covers; i when
-// there is none.
+// firstFree is the first row from i on whose start no block or meeting
+// covers; i when there is none.
 func (tl *timeline) firstFree(i int) int {
 	for j := max(i, 0); j < len(tl.rows); j++ {
 		if tl.gapFrom(tl.rows[j].start) > 0 {
@@ -140,6 +187,16 @@ func (tl *timeline) firstFree(i int) int {
 	return i
 }
 
+// conflict is the first meeting b overlaps (D18, D42).
+func (tl *timeline) conflict(b domain.Block) (meeting, bool) {
+	for _, mt := range tl.meetings {
+		if b.Start < mt.end && mt.start < b.End() {
+			return mt, true
+		}
+	}
+	return meeting{}, false
+}
+
 // clash is the first block other than b itself (same ID) that b overlaps.
 func (tl *timeline) clash(b domain.Block) (domain.Block, bool) {
 	for _, o := range tl.blocks {
```

Patch `internal/tui/today.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/today.go b/internal/tui/today.go
index 80c49b7..3bf4a61 100644
--- a/internal/tui/today.go
+++ b/internal/tui/today.go
@@ -49,7 +49,11 @@ func (td *todayState) refresh(m *Model) {
 	if m.svc.Today() == plan.Day {
 		now = plan.Day.Offset(m.svc.Now())
 	}
-	td.tl = buildTimeline(plan.Blocks, cfg.WorkStart, cfg.WorkEnd, now)
+	var meetings []meeting
+	for _, e := range m.state.Calendar.Meetings {
+		meetings = append(meetings, meeting{title: e.Title, start: plan.Day.Offset(e.Start), end: plan.Day.Offset(e.End)})
+	}
+	td.tl = buildTimeline(plan.Blocks, meetings, cfg.WorkStart, cfg.WorkEnd, now)
 	rows := td.tl.rows
 	if !td.placed {
 		td.at, td.placed = rows[td.tl.homeRow()].start, true
@@ -107,7 +111,7 @@ func (m *Model) pickUp(t domain.Task) {
 
 // ghost is the carried block as it would land at the cursor: a moved block
 // keeps its length; a candidate gets its D24 length, min(size, the free time
-// from the cursor to the next block or the planning day's end).
+// from the cursor to the next block or meeting, or the hours' end; D42).
 func (m *Model) ghost() domain.Block {
 	td := &m.today
 	c := td.carry
@@ -146,6 +150,27 @@ func (m *Model) blockName(b domain.Block) string {
 
 func span(b domain.Block) string { return clock(b.Start) + "–" + clock(b.End()) }
 
+// overMeeting is the status note for a block placed over a meeting (D42):
+// allowed, since a meeting is sometimes skipped, but said.
+func (m *Model) overMeeting(b domain.Block) string {
+	mt, ok := m.today.tl.conflict(b)
+	if !ok {
+		return ""
+	}
+	return fmt.Sprintf(" · ⚠ overlaps meeting %q %s–%s", mt.title, clock(mt.start), clock(mt.end))
+}
+
+// conflicted reports a block that shows ⚠ (D18, D42): an open task's block
+// overlapping a meeting. A closed task's block is history.
+func (m *Model) conflicted(b domain.Block) bool {
+	t, ok := m.state.Snapshot.Get(b.TaskID)
+	if !ok || t.Status != domain.StatusOpen {
+		return false
+	}
+	_, ok = m.today.tl.conflict(b)
+	return ok
+}
+
 // drop places or moves the carried block at the cursor. A clash keeps it
 // picked up, so another row can be tried.
 func (m *Model) drop() tea.Cmd {
@@ -158,11 +183,11 @@ func (m *Model) drop() tea.Cmd {
 	td.carry = nil
 	svc, label := m.svc, fmt.Sprintf("%q", c.task.Title)
 	if c.block.ID != 0 {
-		return m.act("move "+label, "moved "+label+" to "+span(g), func(ctx context.Context) (app.Undo, error) {
+		return m.act("move "+label, "moved "+label+" to "+span(g)+m.overMeeting(g), func(ctx context.Context) (app.Undo, error) {
 			return svc.MoveBlock(ctx, c.block.ID, g.Start)
 		})
 	}
-	status := "blocked " + label + " " + span(g)
+	status := "blocked " + label + " " + span(g) + m.overMeeting(g)
 	if c.task.Size == domain.SizeL || c.task.Size == domain.SizeXL {
 		status += " · big task · consider splitting" // D37
 	}
@@ -196,7 +221,7 @@ func (m *Model) reshape(old domain.Block, t domain.Task, b domain.Block) tea.Cmd
 		return nil
 	}
 	svc, label := m.svc, fmt.Sprintf("%q", t.Title)
-	return m.act("change "+label, label+" now "+span(b), func(ctx context.Context) (app.Undo, error) {
+	return m.act("change "+label, label+" now "+span(b)+m.overMeeting(b), func(ctx context.Context) (app.Undo, error) {
 		if b.Start == old.Start {
 			return svc.ResizeBlock(ctx, b.ID, b.Duration)
 		}
@@ -370,6 +395,10 @@ func (m *Model) todayKey(key string) tea.Cmd {
 	}
 	b, ok := td.tl.blockAt(td.cursor)
 	if !ok {
+		if len(td.tl.rows[td.cursor].bands) > 0 {
+			m.setStatus("meeting · read-only") // D41
+			return nil
+		}
 		m.setStatus("nothing blocked here · tab to the candidates")
 		return nil
 	}
@@ -436,6 +465,9 @@ func (m *Model) timelineLines(width int) []string {
 		}
 		title += fmt.Sprintf(" · %d/%d done", done, n)
 	}
+	if n := len(slices.DeleteFunc(slices.Clone(td.tl.blocks), func(b domain.Block) bool { return !m.conflicted(b) })); n > 0 {
+		title += fmt.Sprintf(" · ⚠ %d", n) // D42
+	}
 	if td.tl.outsideNow() {
 		title += " · outside of working hours"
 	}
@@ -452,8 +484,9 @@ func (m *Model) timelineLines(width int) []string {
 }
 
 // timelineRow renders row i: its time, then the blocks starting in it
-// (▓ title [P S]), or ▓ where a block carries on from an earlier row. A
-// picked-up block shows as a ░ ghost over the rows it would take.
+// (▓ title [P S]), or ▓ where a block carries on from an earlier row, then
+// its meetings (D41). A picked-up block shows as a ░ ghost over the rows it
+// would take.
 func (m *Model) timelineRow(i, width int) string {
 	td := &m.today
 	r := td.tl.rows[i]
@@ -479,14 +512,15 @@ func (m *Model) timelineRow(i, width int) string {
 			carried = true
 		}
 	}
-	text := " " + clock(r.start) + " "
+	var parts []string
 	switch {
 	case len(labels) > 0:
-		text += "▓ " + strings.Join(labels, " · ")
+		parts = append(parts, "▓ "+strings.Join(labels, " · "))
 	case carried:
-		text += "▓"
+		parts = append(parts, "▓")
 	}
-	line := pad(text, width)
+	parts = append(parts, m.meetingParts(r)...)
+	line := pad(" "+clock(r.start)+" "+strings.Join(parts, " · "), width)
 	if now := td.tl.now; now >= 0 && r.start+rowLen <= now {
 		line = styleDim.Render(line) // the past
 	}
@@ -496,25 +530,59 @@ func (m *Model) timelineRow(i, width int) string {
 	return line
 }
 
-// ghostRow renders a row the carried block would take; it turns the alert
-// colour when the block doesn't fit there.
+// ghostRow renders a row the carried block would take, with what else is
+// there; it turns the error colour when the block doesn't fit there, the
+// alert colour (and ⚠) when it would overlap a meeting (D42).
 func (m *Model) ghostRow(r tlRow, g domain.Block, width int) string {
+	_, overMeeting := m.today.tl.conflict(g)
 	text := " " + clock(r.start) + " ░"
 	if r.start <= g.Start {
 		text += fmt.Sprintf(" %s %s", m.today.carry.task.Title, shortDur(g.Duration))
+		if overMeeting {
+			text += " ⚠"
+		}
 	}
 	for _, b := range r.blocks {
 		if b.ID != g.ID {
 			text += " · " + m.blockLabel(b)
 		}
 	}
+	for _, p := range m.meetingParts(r) {
+		text += " · " + p
+	}
 	line := pad(text, width)
-	if m.fits(g) != nil {
+	switch {
+	case m.fits(g) != nil:
 		return styleError.Render(line)
+	case overMeeting:
+		return styleGhostAlert.Render(line)
 	}
 	return styleSelected.Render(line)
 }
 
+// meetingParts renders row r's meetings (D41): ■ and the title of a band
+// that starts in the row (exact times when off the half-hour grid, +N for
+// the meetings merged into it), or ■ where one carries on from above.
+func (m *Model) meetingParts(r tlRow) []string {
+	var out []string
+	for _, bd := range r.bands {
+		if max(bd.start, m.today.tl.start) < r.start {
+			out = append(out, "■")
+			continue
+		}
+		s := "■ "
+		if bd.start%rowLen != 0 || bd.end%rowLen != 0 {
+			s += clock(bd.start) + "–" + clock(bd.end) + " "
+		}
+		s += bd.title
+		if bd.more > 0 {
+			s += fmt.Sprintf(" +%d", bd.more)
+		}
+		out = append(out, s)
+	}
+	return out
+}
+
 // blockLabel is a block's text on its first row: exact times when it is off
 // the half-hour grid, the task's title and badge, struck through when closed.
 func (m *Model) blockLabel(b domain.Block) string {
@@ -531,6 +599,9 @@ func (m *Model) blockText(b domain.Block, times bool) string {
 	if times {
 		s = span(b) + " " + s
 	}
+	if m.conflicted(b) {
+		s += " ⚠" // D18, D42: r re-slots it
+	}
 	switch t.Status {
 	case domain.StatusDone:
 		return styleClosed.Render(s + " ✓")
@@ -569,6 +640,7 @@ var todayHelp = []string{
 	"f        on a block: fine-tune start and length (09:30 45m)",
 	"u        on a block: unblock (stays a candidate) · on a candidate: unpin",
 	"⏎        task detail (the block's task on the timeline)",
+	"■        a meeting (read-only); a block over one shows ⚠, and placing over one is allowed",
 	"x d D    done (again: reopen) / drop / delete",
 	"e / E    edit the line / the note in $EDITOR · o open the link",
 	"c        commit the plan (again: refresh)",
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/common.go internal/tui/meetings_test.go internal/tui/timeline.go internal/tui/timeline_test.go internal/tui/today.go
git commit -m 'feat(tui): meetings on Today; meetings are busy for placement, a block over one shows ⚠ (D41, D42)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 6: `r` re-slots a ⚠ block; the Plan 5a timeline carry-forwards

D43: `r` on a ⚠ block picks it up like `m`, with the ghost on the first row after the block's own where the whole block fits inside the working hours, clear of blocks and meetings; no such row keeps the ghost on the block and says so. Plus the four "Carry-forward from Plan 5a review" items marked Plan 5b: unchanged drops and fine-tunes skip the service (`unchanged`, no undo pushed); a drop the service refuses gives the pick-up back; fine-tune re-reads its block on enter; the scroll offset keeps the cursor's line when rows appear above it. `TestMovingAnOffGridBlockKeepsItsStart` now expects `unchanged` for `m ⏎`.

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/timeline.go`
- Modify: `internal/tui/today.go`
- Modify: `internal/tui/place_test.go`
- Create: `internal/tui/reslot_test.go`

**Interfaces:**
- Consumes: Task 5 (`conflict`, `conflicted`, `overMeeting`, `ev`, `h.meetings`), `actionMsg.restore`.
- Produces:
  - `(*timeline).block(id int64) (domain.Block, bool)` (also used by `refreshCarry`).
  - `(*Model).pickUpBlock(t, b)` (what `m` did; keeps the cursor when the block starts before the hours), `(*Model).freeSlot(b) (int, bool)`, `(*Model).actOr(label, status string, restore func(*Model), fn) tea.Cmd` (`act` wraps it).
  - `r` in `blockKeys`; statuses `no conflict · m moves it`, `no free slot left today fits 5h`, `unchanged`, `stopped fine-tuning "X": its block is gone`; the timeline hints start with `r re-slot ⚠ · ` on a ⚠ block; a help line for `r`.

- [ ] **Step 1: Write the failing tests**

Patch `internal/tui/place_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/place_test.go b/internal/tui/place_test.go
index ce4186e..7bafcf5 100644
--- a/internal/tui/place_test.go
+++ b/internal/tui/place_test.go
@@ -325,8 +325,12 @@ func TestMovingAnOffGridBlockKeepsItsStart(t *testing.T) {
 	h := newHarness(t, config.Default())
 	a := h.pinned("Alpha !1 ~s")
 	h.block(a.ID, 10*time.Hour+10*time.Minute, 45*time.Minute)
+	undos := len(h.m.undo)
 	h.keys("j", "j", "m", "enter") // m ⏎ leaves it where it was
-	h.mustSee(`moved "Alpha" to 10:10–10:55`)
+	h.mustSee("unchanged")
+	if len(h.m.undo) != undos || h.m.today.carry != nil {
+		t.Fatalf("an unchanged drop calls nothing and pushes no undo: %d undos, carry %v", len(h.m.undo), h.m.today.carry)
+	}
 	h.keys("m", "j", "enter")
 	h.mustSee(`moved "Alpha" to 10:40–11:25`)
 }
```

Create `internal/tui/reslot_test.go`:

```go
package tui

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/config"
)

func TestReslotAConflictedBlock(t *testing.T) {
	h := newCalHarness(t, config.Default())
	h.meetings(ev("Review", 10*time.Hour, 11*time.Hour), ev("Lunch", 12*time.Hour, 13*time.Hour))
	a := h.pinned("Alpha !1 ~s")
	h.block(a.ID, 10*time.Hour, time.Hour)
	b := h.pinned("Beta !1 ~s")
	h.block(b.ID, 11*time.Hour, time.Hour)

	h.keys("j", "j", "j", "j") // 11:00: Beta, no conflict
	h.keys("r")
	h.mustSee("no conflict · m moves it")
	if h.m.today.carry != nil {
		t.Fatal("r on a block without a conflict picks nothing up")
	}

	h.keys("k", "k") // 10:00: Alpha over Review
	h.mustSee("r re-slot ⚠ · j/k move")
	h.keys("r")
	// D43: the first slot after Alpha's that fits 1h: not 10:30 (Review), 11:00
	// or 11:30 (Beta), 12:00 or 12:30 (Lunch).
	h.mustSee(`moving "Alpha"`, " 13:00 ░ Alpha 1h")
	h.keys("j") // j/k adjust
	h.mustSee(" 13:30 ░ Alpha 1h")
	h.keys("k", "enter")
	h.mustSee(`moved "Alpha" to 13:00–14:00`)
	h.mustNotSee("⚠")
	h.keys("ctrl+z")
	h.mustSee(`undid move "Alpha"`, " 10:00 ▓ Alpha [P1 S] ⚠")

	h.keys("r", "esc") // esc leaves it where it is
	if bs := h.blocks(); bs[0].Start != 10*time.Hour || h.m.today.carry != nil {
		t.Errorf("esc after r: %+v, carry %v", bs[0], h.m.today.carry)
	}
}

func TestReslotWithNoFreeSlot(t *testing.T) {
	h := newCalHarness(t, config.Default())
	h.meetings(ev("All hands", 14*time.Hour, 14*time.Hour+30*time.Minute))
	long := h.pinned("Long !1 ~m")
	h.block(long.ID, 14*time.Hour, 5*time.Hour)              // 14:00–19:00: nothing later fits 5h before 19:00
	h.keys("j", "j", "j", "j", "j", "j", "j", "j", "j", "j") // 14:00
	h.keys("r")
	h.mustSee("no free slot left today fits 5h", `moving "Long"`, " 14:00 ░ Long 5h ⚠")
	undos := len(h.m.undo)
	h.keys("enter") // the ghost stayed on the block
	h.mustSee("unchanged")
	if len(h.m.undo) != undos {
		t.Error("an unchanged drop pushes no undo")
	}
}

func TestARefusedDropKeepsThePickUp(t *testing.T) {
	h := newHarness(t, config.Default())
	other := h.pinned("Other !1 ~s")
	h.pinned("Alpha !1 ~s")
	h.keys("tab", "j", "b") // Alpha's ghost at 09:00
	// Meanwhile another tt blocks Other at 09:00; this one hasn't reloaded yet.
	if _, _, err := h.svc.Block(context.Background(), other.ID, 9*time.Hour, time.Hour); err != nil {
		t.Fatal(err)
	}
	h.keys("enter")
	h.mustSee(": overlaps another block") // the service refuses
	if c := h.m.today.carry; c == nil || c.task.Title != "Alpha" || h.m.today.at != 9*time.Hour {
		t.Fatalf("a refused drop gives the pick-up back where it was: %+v at %v", c, h.m.today.at)
	}
	h.mustSee(" 09:00 ░ Alpha 1h · Other [P1 S]") // the reload shows why
	h.keys("j", "j", "enter")
	h.mustSee(`blocked "Alpha" 10:00–11:00`)
}

func TestFineTuneRereadsTheBlock(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, config.Default())
	a := h.pinned("Alpha !1 ~s")
	b := h.block(a.ID, 10*time.Hour, time.Hour)
	h.keys("j", "j", "f")
	undos := len(h.m.undo)
	h.keys("enter")
	h.mustSee("unchanged")
	if len(h.m.undo) != undos {
		t.Error("an unchanged fine-tune pushes no undo")
	}

	h.keys("f")
	if _, err := h.svc.MoveBlock(ctx, b.ID, 13*time.Hour); err != nil { // another tt
		t.Fatal(err)
	}
	h.run(h.m.load()) // the watch loop reloads while the line is open
	for range 10 {
		h.keys("backspace")
	}
	h.typeText("10:00 30m")
	h.keys("enter")
	if bs := h.blocks(); len(bs) != 1 || bs[0].Start != 10*time.Hour || bs[0].Duration != 30*time.Minute {
		t.Fatalf("the typed span applies to the block as it is now: %+v", bs)
	}

	h.keys("f")
	if _, err := h.svc.Unblock(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	h.run(h.m.load())
	h.keys("enter")
	h.mustSee(`stopped fine-tuning "Alpha": its block is gone`)
}

func TestScrollKeepsTheCursorWhenRowsAppearAbove(t *testing.T) {
	h := newHarness(t, config.Default())
	early := h.pinned("Early !1 ~s")
	h.send(tea.WindowSizeMsg{Width: 100, Height: 12})
	for range 10 {
		h.keys("j") // 14:00, scrolled
	}
	h.keys("k", "k", "k") // 12:30, inside the window
	line := func() int {
		return slices.IndexFunc(strings.Split(h.screen(), "\n"), func(l string) bool { return strings.HasPrefix(l, " 12:30") })
	}
	before := line()
	if before < 0 || h.m.today.offset == 0 {
		t.Fatalf("the timeline should have scrolled to 12:30 (line %d, offset %d)", before, h.m.today.offset)
	}
	h.blockDirectly(early.ID, 6*time.Hour, time.Hour) // a row of its own appears above the hours
	h.keys("R")
	if got := line(); got != before || h.m.today.at != 12*time.Hour+30*time.Minute {
		t.Errorf("12:30 moved from line %d to %d (cursor at %v)", before, got, h.m.today.at)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL: `TestMovingAnOffGridBlockKeepsItsStart` (`screen lacks "unchanged"`), `TestReslotAConflictedBlock` (`lacks "no conflict · m moves it"`), `TestReslotWithNoFreeSlot`, `TestARefusedDropKeepsThePickUp` (`a refused drop gives the pick-up back where it was: <nil>`), `TestFineTuneRereadsTheBlock`, `TestScrollKeepsTheCursorWhenRowsAppearAbove` (`12:30 moved from line 6 to 7`).

- [ ] **Step 3: Implement**

Patch `internal/tui/model.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/model.go b/internal/tui/model.go
index a499d10..d297da3 100644
--- a/internal/tui/model.go
+++ b/internal/tui/model.go
@@ -383,10 +383,15 @@ func (m *Model) startDay() tea.Cmd {
 
 // act runs a service command in the background and reports it as actionMsg.
 func (m *Model) act(label, status string, fn func(context.Context) (app.Undo, error)) tea.Cmd {
+	return m.actOr(label, status, nil, fn)
+}
+
+// actOr is act with restore, which runs if the command fails.
+func (m *Model) actOr(label, status string, restore func(*Model), fn func(context.Context) (app.Undo, error)) tea.Cmd {
 	ctx := m.ctx
 	return func() tea.Msg {
 		undo, err := fn(ctx)
-		return actionMsg{label: label, status: status, undo: undo, err: err}
+		return actionMsg{label: label, status: status, undo: undo, err: err, restore: restore}
 	}
 }
 
```

Patch `internal/tui/timeline.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/timeline.go b/internal/tui/timeline.go
index be4ae13..9c4d82a 100644
--- a/internal/tui/timeline.go
+++ b/internal/tui/timeline.go
@@ -1,6 +1,7 @@
 package tui
 
 import (
+	"slices"
 	"time"
 
 	"tt/internal/domain"
@@ -187,6 +188,15 @@ func (tl *timeline) firstFree(i int) int {
 	return i
 }
 
+// block is the day's block with id, as last loaded.
+func (tl *timeline) block(id int64) (domain.Block, bool) {
+	i := slices.IndexFunc(tl.blocks, func(b domain.Block) bool { return b.ID == id })
+	if i < 0 {
+		return domain.Block{}, false
+	}
+	return tl.blocks[i], true
+}
+
 // conflict is the first meeting b overlaps (D18, D42).
 func (tl *timeline) conflict(b domain.Block) (meeting, bool) {
 	for _, mt := range tl.meetings {
```

Patch `internal/tui/today.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/today.go b/internal/tui/today.go
index 3bf4a61..ac9f546 100644
--- a/internal/tui/today.go
+++ b/internal/tui/today.go
@@ -45,6 +45,10 @@ type carry struct {
 // back on its row (the window may have grown or shrunk).
 func (td *todayState) refresh(m *Model) {
 	cfg, plan := m.svc.Config(), m.state.Plan
+	shown := -1 // the cursor's line in the window, kept when rows appear above it
+	if td.placed && td.cursor < len(td.tl.rows) {
+		shown = td.line(td.cursor) - td.offset
+	}
 	now := time.Duration(-1)
 	if m.svc.Today() == plan.Day {
 		now = plan.Day.Offset(m.svc.Now())
@@ -66,6 +70,9 @@ func (td *todayState) refresh(m *Model) {
 		}
 	}
 	td.at = rows[td.cursor].start
+	if shown >= 0 {
+		td.offset = max(0, td.line(td.cursor)-shown)
+	}
 	td.refreshCarry(m)
 }
 
@@ -90,13 +97,13 @@ func (td *todayState) refreshCarry(m *Model) {
 	if c.block.ID == 0 {
 		return
 	}
-	i := slices.IndexFunc(td.tl.blocks, func(b domain.Block) bool { return b.ID == c.block.ID })
-	if i < 0 {
+	b, ok := td.tl.block(c.block.ID)
+	if !ok {
 		td.carry = nil
 		m.setStatus(fmt.Sprintf("stopped moving %q: its block is gone", t.Title))
 		return
 	}
-	c.block = td.tl.blocks[i]
+	c.block = b
 }
 
 // pickUp starts placing candidate t (D36): the cursor goes to the first free
@@ -109,6 +116,39 @@ func (m *Model) pickUp(t domain.Task) {
 	td.at = td.tl.rows[td.cursor].start
 }
 
+// pickUpBlock starts moving block b (D36): the cursor goes to its first row.
+func (m *Model) pickUpBlock(t domain.Task, b domain.Block) {
+	td := &m.today
+	td.carry = &carry{task: t, block: b}
+	if i := td.tl.rowOf(b.Start); i >= 0 { // a block starting before the hours has no row of its start
+		td.cursor = i
+	}
+	td.at = td.tl.rows[td.cursor].start
+}
+
+// freeSlot is the first row after block b's where b, moved there (keeping
+// its minutes within the row, like m), fits whole inside the working hours,
+// clear of every other block and of every meeting (D43).
+func (m *Model) freeSlot(b domain.Block) (int, bool) {
+	tl := &m.today.tl
+	within := b.Start - b.Start.Truncate(rowLen)
+	for i := tl.rowOf(b.Start) + 1; i < len(tl.rows); i++ {
+		nb := b
+		nb.Start = tl.rows[i].start + within
+		if tl.rows[i].outside || nb.Start < tl.start || nb.End() > tl.end {
+			continue
+		}
+		if _, clash := tl.clash(nb); clash {
+			continue
+		}
+		if _, busy := tl.conflict(nb); busy {
+			continue
+		}
+		return i, true
+	}
+	return 0, false
+}
+
 // ghost is the carried block as it would land at the cursor: a moved block
 // keeps its length; a candidate gets its D24 length, min(size, the free time
 // from the cursor to the next block or meeting, or the hours' end; D42).
@@ -172,18 +212,24 @@ func (m *Model) conflicted(b domain.Block) bool {
 }
 
 // drop places or moves the carried block at the cursor. A clash keeps it
-// picked up, so another row can be tried.
+// picked up, so another row can be tried; so does a refusal by the service
+// (the timeline was stale), once it comes back.
 func (m *Model) drop() tea.Cmd {
 	td := &m.today
-	c, g := td.carry, m.ghost()
+	c, g, at := td.carry, m.ghost(), td.at
 	if err := m.fits(g); err != nil {
 		m.setErr(err)
 		return nil
 	}
 	td.carry = nil
+	if c.block.ID != 0 && g.Start == c.block.Start {
+		m.setStatus("unchanged")
+		return nil
+	}
+	back := func(m *Model) { m.today.carry, m.today.at, m.today.focus = c, at, focusTimeline }
 	svc, label := m.svc, fmt.Sprintf("%q", c.task.Title)
 	if c.block.ID != 0 {
-		return m.act("move "+label, "moved "+label+" to "+span(g)+m.overMeeting(g), func(ctx context.Context) (app.Undo, error) {
+		return m.actOr("move "+label, "moved "+label+" to "+span(g)+m.overMeeting(g), back, func(ctx context.Context) (app.Undo, error) {
 			return svc.MoveBlock(ctx, c.block.ID, g.Start)
 		})
 	}
@@ -191,7 +237,7 @@ func (m *Model) drop() tea.Cmd {
 	if c.task.Size == domain.SizeL || c.task.Size == domain.SizeXL {
 		status += " · big task · consider splitting" // D37
 	}
-	return m.act("block "+label, status, func(ctx context.Context) (app.Undo, error) {
+	return m.actOr("block "+label, status, back, func(ctx context.Context) (app.Undo, error) {
 		_, undo, err := svc.Block(ctx, c.task.ID, g.Start, g.Duration)
 		return undo, err
 	})
@@ -229,13 +275,14 @@ func (m *Model) reshape(old domain.Block, t domain.Task, b domain.Block) tea.Cmd
 	})
 }
 
-// openFineTune asks for a block's exact start and length (D36).
+// openFineTune asks for a block's exact start and length (D36). The block
+// is read again on enter: a reload meanwhile may have moved it.
 func (m *Model) openFineTune(b domain.Block, t domain.Task) tea.Cmd {
 	dayStart := m.svc.Config().DayStart
 	in := newInput(fmt.Sprintf("fine-tune %q › ", ansi.Truncate(t.Title, 30, "…")), "09:30 45m", m.width)
 	in.SetValue(clock(b.Start) + " " + shortDur(b.Duration))
 	in.CursorEnd()
-	nb := b
+	var nb domain.Block
 	m.overlay = &prompt{
 		input: in,
 		check: func(v string) error {
@@ -243,10 +290,26 @@ func (m *Model) openFineTune(b domain.Block, t domain.Task) tea.Cmd {
 			if err != nil {
 				return err
 			}
+			cur, ok := m.today.tl.block(b.ID)
+			if !ok {
+				return nil // onEnter says so
+			}
+			nb = cur
 			nb.Start, nb.Duration = start, dur
 			return m.fits(nb)
 		},
-		onEnter: func(string) tea.Cmd { return m.reshape(b, t, nb) },
+		onEnter: func(string) tea.Cmd {
+			cur, ok := m.today.tl.block(b.ID)
+			switch {
+			case !ok:
+				m.setStatus(fmt.Sprintf("stopped fine-tuning %q: its block is gone", t.Title))
+				return nil
+			case nb.Start == cur.Start && nb.Duration == cur.Duration:
+				m.setStatus("unchanged")
+				return nil
+			}
+			return m.reshape(cur, t, nb)
+		},
 	}
 	return nil
 }
@@ -325,7 +388,7 @@ func (td *todayState) follow(m *Model) {
 // blockKeys act on the block under the timeline cursor.
 var blockKeys = map[string]bool{
 	"enter": true, "x": true, "d": true, "D": true, "e": true, "E": true, "o": true,
-	"m": true, "+": true, "=": true, "-": true, "f": true, "u": true,
+	"m": true, "+": true, "=": true, "-": true, "f": true, "u": true, "r": true,
 }
 
 func (m *Model) todayKey(key string) tea.Cmd {
@@ -411,9 +474,19 @@ func (m *Model) todayKey(key string) tea.Cmd {
 		m.openDetail(t.ID)
 		return nil
 	case "m":
-		td.carry = &carry{task: t, block: b}
-		td.cursor = td.tl.rowOf(b.Start) // the block's first row
-		td.at = td.tl.rows[td.cursor].start
+		m.pickUpBlock(t, b)
+		return nil
+	case "r": // D43
+		if !m.conflicted(b) {
+			m.setStatus("no conflict · m moves it")
+			return nil
+		}
+		m.pickUpBlock(t, b)
+		if i, ok := m.freeSlot(b); ok {
+			td.cursor, td.at = i, td.tl.rows[i].start
+		} else {
+			m.setStatus("no free slot left today fits " + shortDur(b.Duration))
+		}
 		return nil
 	case "+", "=":
 		nb := b
@@ -628,7 +701,11 @@ func (m *Model) todayHints() string {
 	if m.today.focus == focusCandidates {
 		return "b block · ⏎ open · u unpin · x done · d drop · e edit · c commit · tab timeline · L log · i inbox · a add · ctrl+z undo · ? help · q quit"
 	}
-	return "j/k move · m move · +/- size · f fine-tune · u unblock · ⏎ open · x done · c commit · tab candidates · L log · i inbox · ctrl+z undo · ? help"
+	hints := "j/k move · m move · +/- size · f fine-tune · u unblock · ⏎ open · x done · c commit · tab candidates · L log · i inbox · ctrl+z undo · ? help"
+	if b, ok := m.today.tl.blockAt(m.today.cursor); ok && m.conflicted(b) {
+		hints = "r re-slot ⚠ · " + hints
+	}
+	return hints
 }
 
 var todayHelp = []string{
@@ -636,6 +713,7 @@ var todayHelp = []string{
 	"j k      move",
 	"b        on a candidate: pick it up; j/k choose the row, ⏎ drops it, esc cancels",
 	"m        on a block: pick it up to move it (keeps its length)",
+	"r        on a ⚠ block: pick it up on the next free slot (no block, no meeting); ⏎ drops, esc leaves it",
 	"+ / -    on a block: 30m longer / shorter (= works for +)",
 	"f        on a block: fine-tune start and length (09:30 45m)",
 	"u        on a block: unblock (stays a candidate) · on a candidate: unpin",
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/place_test.go internal/tui/reslot_test.go internal/tui/timeline.go internal/tui/today.go
git commit -m 'feat(tui): r re-slots a ⚠ block (D43); unchanged drops and fine-tunes skip the service, a refused drop keeps the pick-up, fine-tune re-reads its block, the scroll keeps the cursor

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---


## After the last task

- Build tt and a fake helper into the scratchpad and drive the real binary through a Python `pty` script on a throwaway database: a `tt-cal` script next to the built `tt` that prints today's `Standup` (09:30–10:00) for `events` and `config.toml` with `[calendar] calendars = ["Work"]` (`TT_CONFIG`, `TT_DB`). Today shows `■ Standup` and no calendar status; `L`, `a` + `Alpha !1 ~s` + `⏎`, `p`, `B`, `b`: the ghost avoids the meeting; move it onto the meeting, `⏎`: `⚠ overlaps meeting "Standup"`; `r` → `moving "Alpha"`, `⏎` → `moved`; `q`. Then make the script `exit 2` and `R`: `⚠ cal: no access · …` while `■ Standup` stays. Without `[calendar]`: `cal: not set up (tt-cal calendars)`.
- `go test -race ./internal/tui/ ./internal/app/`.
- Final whole-branch review, one fix wave, park residuals as spec carry-forwards.
- **The user's check, together:** `helpers/tt-cal/build.sh`, then `tt-cal calendars` (the macOS permission prompt), copy the names into `~/.config/tt/config.toml` under `[calendar] calendars`, `go build -o ~/.local/bin/tt ./cmd/tt`, open tt: today's meetings against the real calendar.
