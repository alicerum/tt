# tt Plan 9 — Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The running TUI sends opt-in macOS notifications: a few minutes before, and when, a meeting or a task block starts (`[notify] alerts`, default `[5, 0]`), and for new review requests, grouped. Each kind is off by default. Nothing notifies for what came before launch, and tt stays quiet while its pane has the focus.

**Architecture:** Everything lives in the TUI, in memory (no persistence, no migration, no new process). A pure planner (`moments`, `due`, the texts) turns `app.State` into notifications. The Model checks the window `(last check, now]` on the existing 1-second watch tick, and diffs the review requests whenever a load shows a new good GitHub fetch. Notifications go through `Options.Notify`: osascript by default, captured in tests. Focus comes from terminal focus reports (`View.ReportFocus`, `tea.FocusMsg`/`tea.BlurMsg`).

**Tech Stack:** Go 1.27; Bubble Tea v2.1 (focus reports), BurntSushi TOML; macOS `osascript`. No new Go dependencies.

**Spec:** `docs/superpowers/specs/2026-10-12-task-tracking-design.md`: D63, D64, D65, D66, the section "Plan 9 — Notifications", and Open items P8.

## Global Constraints

- No new Go dependencies: `go.mod` and `go.sum` stay unchanged. Only `internal/config`, `config.example.toml` and `internal/tui` change; no service, store or schema change.
- Every kind is off by default (`meetings = false`, `blocks = false`, `reviews = false`); `alerts = [5, 0]`. With every kind off, tt sends nothing and asks for no focus reports.
- Texts, exactly: `tt · in 5 min`, `tt · now`, names joined by ` · `; `tt · review requested` with `@author owner/repo#n Title` (no `@author ` part when the author is unknown); `tt · 3 review requests` with the titles joined by ` · `; the status `can't notify: <error>`, once per run.
- Config errors, exactly: `notify.alerts: list at least one, like [5, 0] (turn a kind off instead)`, `notify.alerts[i]: want whole minutes from 0 to 120, got N`, `notify.alerts: N is listed twice`.
- Title and body reach osascript as arguments, never inside the script text.
- Tests never send a real notification: the harness's `Options.Notify` captures them, and the pty run puts a fake `osascript` on `PATH`. Never run tt from the sandbox with a `[notify]` kind on and the real `osascript` on `PATH`.
- Every task ends green on: `gofmt -l .` (no output), `go vet ./...`, `go test ./...`; Task 3 also `go test -race ./internal/tui/ ./internal/app/`.
- Every commit message ends with a blank line and `Co-Authored-By: Claude <noreply@anthropic.com>`. Commit messages use single quotes.
- Sandbox: save each patch to a file and `git apply <file>`; run plain, separate git commands (no `git -C`, no `cd … &&`). If `git apply` fails, stop: an earlier step went wrong.

## Rulings made while drafting (in the code; each a one-line reversal)

- No timer and no "sent" set of its own: alerts are checked on the 1-second watch tick over the window `(last check, now]`. Each moment is in exactly one window, so it fires once. A laptop waking from sleep catches up within a second, and `due` drops what is over a minute late. The spec's Plan 9 text (a `notifyTickMsg` with a seq, "minus the sent ones") is updated to match.
- A meeting's moments come from its start; its title is only the name shown.
- Moments due in one check with different lead times are separate notifications, one per lead time (a `tt · now` and a `tt · in 5 min`).
- A good GitHub fetch is recognised on load by `State.GitHub.LastSuccess` (not before launch, and changed since the last one seen), so the diff never depends on which load lands first. A failed fetch doesn't change `LastSuccess`.
- After a failed send, later sends are still tried; only the first failure is reported.
- The osascript title always starts with `tt`, so a body starting with `-` is never read as an option.

## Review Focus

1. **The laptop sleeps or the clock jumps** (lid closed over several block starts, waking mid-afternoon): no burst of stale notifications; only moments at most a minute old are sent. Pinned in Task 2 `TestDueIsTheWindowWithoutLateMoments`, Task 3 `TestAlertsOnlyForMomentsPassedWhileRunning` ("Asleep from 12:00 to 13:58").
2. **Quitting and restarting tt near an alert** (a tmux pane restarted, a crash): no repeat of a moment from before the launch, even one under a minute old; review requests that arrived while tt was closed stay quiet. Pinned in Task 3 `TestAlertsOnlyForMomentsPassedWhileRunning` (relaunch at 15:55:30) and `TestNewReviewRequestsNotify` (relaunch).
3. **Titles with quotes, `&`, newlines, tabs, escape sequences or a leading `-`** (task, meeting and PR titles): the text is never run as script, and it arrives on one line. Pinned in Task 2 `TestOsascriptGetsTheTextAsArguments`, `TestReviewNotes` (a tab), `TestMomentsFollowTheKindsAndAlerts` (`Late\nnight`).
4. **Focus reports missing or stale** (tmux without `focus-events on`, a terminal that never reports): with no focus event ever seen tt notifies; alerts due while focused are dropped, not sent on blur. Pinned in Task 3 `TestMeetingAndBlockAlerts` (never focused), `TestNoAlertsWhileFocused`, and the pty run (`\x1b[I`/`\x1b[O`).
5. **osascript missing or failing** (another OS, a broken PATH): one status line per run, not one every second, and planning goes on. Pinned in Task 3 `TestAFailedSendSaysSoOnce`. macOS denying notifications to Script Editor makes osascript succeed silently, which tt can't see: the user checks this on first use.

## File Structure

```
internal/config/config.go         modify: Notify, parseNotify (Task 1)
config.example.toml               modify: [notify] with every default (1)
internal/tui/notify.go            new: moments, due, alertNotes, review notes, osascriptArgs (2); notifyState, checkAlerts, checkReviews, send (3)
internal/tui/model.go             modify: Options.Notify, Model.notify, watch tick, load, focus messages, ReportFocus (3)
internal/tui/harness_test.go      modify: the harness captures notifications (3)
internal/config/notify_test.go, internal/tui/notify_test.go, internal/tui/notifications_test.go   new
```

Work happens on branch `plan-9` in a worktree.

---
### Task 1: `[notify]` in the config: kinds off by default, alert minutes

D63, D64. A `Notify` struct in `config.Config` holds the three opt-in kinds (`Meetings`, `Blocks`, `Reviews`, all `false` by default) and `Alerts`, the whole minutes before a meeting or block starts at which it notifies (default `[5, 0]`; `0` is the start itself). `parseNotify` applies `[notify]`. Each alert must be an integer from 0 to 120 and listed once, and the list can't be empty (a kind is turned off instead). TOML rejects fractions and strings itself, and the error names `notify.alerts`. `config.example.toml` gains the section with every default, which `TestExampleConfigIsTheDefaults` already checks. Nothing reads the config yet.

**Files:**
- Modify: `config.example.toml`
- Modify: `internal/config/config.go`
- Create: `internal/config/notify_test.go`

**Interfaces:**
- Consumes: existing `file`, `Parse`, `Default`.
- Produces (internal/config/config.go):
  - `type Notify struct { Meetings, Blocks, Reviews bool; Alerts []int }`, `func (n Notify) On() bool` (any kind on).
  - `Config.Notify Notify`; `Default().Notify == Notify{Alerts: []int{5, 0}}`.

- [ ] **Step 1: Write the failing tests**

Create `internal/config/notify_test.go`:

```go
package config

import (
	"slices"
	"strings"
	"testing"
)

func TestParseNotify(t *testing.T) {
	n := Default().Notify
	if n.Meetings || n.Blocks || n.Reviews || n.On() || !slices.Equal(n.Alerts, []int{5, 0}) {
		t.Errorf("defaults: %+v; want every kind off, alerts [5 0] (D63, D64)", n)
	}
	cfg, err := Parse([]byte("[notify]\nmeetings = true\nreviews = true\nalerts = [10, 1, 0]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if n := cfg.Notify; !n.Meetings || n.Blocks || !n.Reviews || !n.On() || !slices.Equal(n.Alerts, []int{10, 1, 0}) {
		t.Errorf("notify: %+v", n)
	}
	if cfg, err := Parse([]byte("[notify]\nblocks = true\n")); err != nil || !cfg.Notify.On() || !slices.Equal(cfg.Notify.Alerts, []int{5, 0}) {
		t.Errorf("blocks only: %+v, %v; want on, with the default alerts", cfg.Notify, err)
	}
	if cfg, err := Parse([]byte("[notify]\nalerts = [120]\n")); err != nil || !slices.Equal(cfg.Notify.Alerts, []int{120}) {
		t.Errorf("alerts [120]: %v, %v", cfg.Notify.Alerts, err)
	}

	cases := map[string]string{
		"[notify]\nalerts = []":        "notify.alerts: list at least one",
		"[notify]\nalerts = [5, -1]":   "notify.alerts[1]: want whole minutes from 0 to 120, got -1",
		"[notify]\nalerts = [121]":     "notify.alerts[0]: want whole minutes from 0 to 120, got 121",
		"[notify]\nalerts = [5, 0, 5]": "notify.alerts: 5 is listed twice",
		"[notify]\nalerts = [2.5]":     "notify.alerts",
		"[notify]\nalerts = [\"5m\"]":  "notify.alerts",
		"[notify]\nalerts = 5":         "notify.alerts",
		"[notify]\nmeetings = \"yes\"": "notify.meetings",
		"[notify]\nsound = true":       "unknown config key",
	}
	for input, wantErr := range cases {
		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/`
Expected: FAIL (build): `Default().Notify undefined (type Config has no field or method Notify)`.

- [ ] **Step 3: Implement**

Patch `config.example.toml` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/config.example.toml b/config.example.toml
index 06a7805..b9b702b 100644
--- a/config.example.toml
+++ b/config.example.toml
@@ -52,3 +52,13 @@ host = ""       # a GitHub Enterprise host; empty: gh's default (github.com)
 poll = "5m"     # how often GitHub is fetched; 30s or more
 priority = "P2" # a pinned review's priority: one of the [[priorities]] names
 size = "s"      # a pinned review's size: s, m, l, xl, or "" for unsized (D48)
+
+# macOS notifications from the running tt (D63), each kind off until you turn
+# it on. The first one makes macOS ask to allow notifications for Script
+# Editor (osascript sends them). Quiet while tt's pane has the focus (D66);
+# in tmux that needs `set -g focus-events on`.
+[notify]
+meetings = false # a meeting is about to start (today's busy ones)
+blocks = false   # a task block is about to start
+reviews = false  # a new review request (D65)
+alerts = [5, 0]  # minutes before a meeting or block starts; 0 = when it starts
```

Patch `internal/config/config.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/config/config.go b/internal/config/config.go
index 29acea6..e8e6bf6 100644
--- a/internal/config/config.go
+++ b/internal/config/config.go
@@ -46,8 +46,21 @@ type Config struct {
 	// pinned review request starts with (D48).
 	ReviewPriority int
 	ReviewSize     domain.Size
+	// Notify is what the running TUI notifies about (D63) and when (D64).
+	Notify Notify
 }
 
+// Notify is the [notify] section. Each kind is opt-in (D63); Alerts are the
+// minutes before a meeting or block starts at which it notifies, 0 being the
+// start itself (D64).
+type Notify struct {
+	Meetings, Blocks, Reviews bool
+	Alerts                    []int
+}
+
+// On reports whether any kind of notification is on.
+func (n Notify) On() bool { return n.Meetings || n.Blocks || n.Reviews }
+
 func Default() Config {
 	return Config{
 		DayStart: 4 * time.Hour,
@@ -71,6 +84,7 @@ func Default() Config {
 		GitHubPoll:     5 * time.Minute,
 		ReviewPriority: 2,
 		ReviewSize:     domain.SizeS,
+		Notify:         Notify{Alerts: []int{5, 0}},
 	}
 }
 
@@ -98,6 +112,12 @@ type file struct {
 		Priority *string `toml:"priority"`
 		Size     *string `toml:"size"`
 	} `toml:"github"`
+	Notify struct {
+		Meetings *bool  `toml:"meetings"`
+		Blocks   *bool  `toml:"blocks"`
+		Reviews  *bool  `toml:"reviews"`
+		Alerts   *[]int `toml:"alerts"`
+	} `toml:"notify"`
 }
 
 // Load reads path. A missing file yields Default().
@@ -198,6 +218,9 @@ func Parse(data []byte) (Config, error) {
 	if err := parseGitHub(f, &cfg); err != nil {
 		return Config{}, err
 	}
+	if err := parseNotify(f, &cfg); err != nil {
+		return Config{}, err
+	}
 	switch {
 	case cfg.WorkEnd <= cfg.WorkStart:
 		return Config{}, fmt.Errorf("work_hours.end must be after work_hours.start")
@@ -247,6 +270,37 @@ func parseGitHub(f file, cfg *Config) error {
 	return nil
 }
 
+// parseNotify applies [notify] (D63, D64): alerts are whole minutes from 0
+// to 120, each listed once, at least one (a kind is turned off instead).
+func parseNotify(f file, cfg *Config) error {
+	n := f.Notify
+	for _, b := range []struct {
+		from *bool
+		to   *bool
+	}{{n.Meetings, &cfg.Notify.Meetings}, {n.Blocks, &cfg.Notify.Blocks}, {n.Reviews, &cfg.Notify.Reviews}} {
+		if b.from != nil {
+			*b.to = *b.from
+		}
+	}
+	if n.Alerts == nil {
+		return nil
+	}
+	alerts := *n.Alerts
+	if len(alerts) == 0 {
+		return fmt.Errorf("notify.alerts: list at least one, like [5, 0] (turn a kind off instead)")
+	}
+	for i, a := range alerts {
+		if a < 0 || a > 120 {
+			return fmt.Errorf("notify.alerts[%d]: want whole minutes from 0 to 120, got %d", i, a)
+		}
+		if slices.Contains(alerts[:i], a) {
+			return fmt.Errorf("notify.alerts: %d is listed twice", a)
+		}
+	}
+	cfg.Notify.Alerts = alerts
+	return nil
+}
+
 // parseClock parses "HH:MM" (00:00 to 23:59) into an offset from midnight.
 func parseClock(s string) (time.Duration, error) {
 	t, err := time.Parse("15:04", s)
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/config/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add config.example.toml internal/config/config.go internal/config/notify_test.go
git commit -m 'feat(config): [notify] kinds, off by default, and alert minutes (D63, D64)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 2: The notification planner: alert moments, the due window, the texts

D64, D65, as pure functions in a new `internal/tui/notify.go`, tested on their own before anything sends. `moments` lists the alert moments for the kinds that are on. Meetings come from today's busy meetings (`State.Calendar.Meetings`, D17a already applied). Blocks are those whose task is open; a block past midnight is due on the next date (`Day.At`). Each gets one moment per alert minute, sorted by when it's due, with names cleaned to one line (`domain.OneLine`). `due` keeps the moments in `(from, to]`, dropping any more than a minute late at `to` (the laptop slept). `alertNotes` makes one notification per lead time: `tt · in 5 min` or `tt · now`, with the names joined by ` · `. `reviewIDs`/`newReviews`/`reviewNote` are D65's set difference and its texts. `osascriptArgs` passes the title and body as the script's arguments, never as script text. Nothing on screen changes in this task.

**Files:**
- Create: `internal/tui/notify.go`
- Create: `internal/tui/notify_test.go`

**Interfaces:**
- Consumes: Task 1 (`config.Notify`); existing `app.State`, `domain.OneLine`, `domain.Day.At`, `domain.Snapshot.Get`.
- Produces (internal/tui/notify.go):
  - `type moment struct { at time.Time; minutes int; name string }`; `type note struct { title, body string }`.
  - `func moments(st app.State, n config.Notify, loc *time.Location) []moment` (nil when nothing is on or `st.Plan.Day == ""` and no meetings).
  - `func due(ms []moment, from, to time.Time) []moment`; `func alertNotes(ms []moment) []note`.
  - `func reviewIDs(items []domain.SourceItem) map[string]bool`; `func newReviews(prev map[string]bool, cur []domain.SourceItem) []domain.SourceItem`; `func reviewNote(items []domain.SourceItem) note`.
  - `func osascriptArgs(title, body string) []string`.
  - Test helpers (notify_test.go): `notifyDay`, `dayAt(off time.Duration) time.Time` (2026-10-12 UTC plus off, the harness's day), `notifyFixture() app.State`, `momentNames([]moment) []string`.

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/notify_test.go`:

```go
package tui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"tt/internal/app"
	"tt/internal/config"
	"tt/internal/domain"
)

var notifyDay = time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)

func dayAt(off time.Duration) time.Time { return notifyDay.Add(off) }

func notifyFixture() app.State {
	tasks := []domain.Task{
		{ID: 1, Title: "Write RFC", Status: domain.StatusOpen},
		{ID: 2, Title: "Fix CI", Status: domain.StatusDone},
		{ID: 3, Title: "Late\nnight", Status: domain.StatusOpen},
	}
	return app.State{
		Snapshot: domain.NewSnapshot(tasks),
		Plan: domain.Plan{Day: "2026-10-12", Blocks: []domain.Block{
			{ID: 10, TaskID: 1, Start: 10 * time.Hour, Duration: time.Hour},
			{ID: 11, TaskID: 2, Start: 11 * time.Hour, Duration: time.Hour},                // a done task: no alerts
			{ID: 12, TaskID: 3, Start: 24*time.Hour + 30*time.Minute, Duration: time.Hour}, // after midnight
			{ID: 13, TaskID: 99, Start: 12 * time.Hour, Duration: time.Hour},               // its task is gone
		}},
		Calendar: app.CalendarState{Meetings: []domain.Event{
			{Title: "Standup", Start: dayAt(9*time.Hour + 30*time.Minute), End: dayAt(9*time.Hour + 45*time.Minute)},
			{Title: "Review", Start: dayAt(10 * time.Hour), End: dayAt(11 * time.Hour)},
		}},
	}
}

func momentNames(ms []moment) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.at.Format("15:04")+" "+m.name)
	}
	return out
}

func TestMomentsFollowTheKindsAndAlerts(t *testing.T) {
	st := notifyFixture()
	all := config.Notify{Meetings: true, Blocks: true, Alerts: []int{5, 0}}
	got := momentNames(moments(st, all, time.UTC))
	want := []string{"09:25 Standup", "09:30 Standup", "09:55 Review", "09:55 Write RFC", "10:00 Review", "10:00 Write RFC", "00:25 Late night", "00:30 Late night"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("moments (-want +got):\n%s", diff)
	}
	if ms := moments(st, all, time.UTC); !ms[6].at.Equal(dayAt(24*time.Hour + 25*time.Minute)) {
		t.Errorf("a block after midnight is due on the next date, got %v", ms[6].at)
	}
	got = momentNames(moments(st, config.Notify{Blocks: true, Alerts: []int{0}}, time.UTC))
	if want := []string{"10:00 Write RFC", "00:30 Late night"}; !slices.Equal(got, want) {
		t.Errorf("blocks at their start only: %v; want %v", got, want)
	}
	if got := moments(st, config.Notify{Reviews: true, Alerts: []int{5, 0}}, time.UTC); got != nil {
		t.Errorf("meetings and blocks off: %v", momentNames(got))
	}
	if got := moments(app.State{}, all, time.UTC); got != nil {
		t.Errorf("before the first load (no day): %v", momentNames(got))
	}
}

func TestDueIsTheWindowWithoutLateMoments(t *testing.T) {
	ms := moments(notifyFixture(), config.Notify{Meetings: true, Blocks: true, Alerts: []int{5, 0}}, time.UTC)
	cases := []struct {
		from, to time.Duration
		want     []string
	}{
		{9*time.Hour + 25*time.Minute, 9*time.Hour + 30*time.Minute, []string{"09:30 Standup"}}, // from is exclusive
		{9*time.Hour + 54*time.Minute, 9*time.Hour + 55*time.Minute, []string{"09:55 Review", "09:55 Write RFC"}},
		{9*time.Hour + 54*time.Minute, 9*time.Hour + 56*time.Minute, []string{"09:55 Review", "09:55 Write RFC"}}, // a minute late still counts
		{9*time.Hour + 54*time.Minute, 9*time.Hour + 56*time.Minute + time.Second, nil},                           // more than a minute late: dropped
		{9 * time.Hour, 10*time.Hour + 30*time.Second, []string{"10:00 Review", "10:00 Write RFC"}},               // after sleep: only the recent ones
		{10*time.Hour + 1*time.Minute, 10*time.Hour + 2*time.Minute, nil},
	}
	for _, c := range cases {
		if got := momentNames(due(ms, dayAt(c.from), dayAt(c.to))); !slices.Equal(got, c.want) {
			t.Errorf("due(%v, %v] = %v; want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestAlertNotesGroupByLeadTime(t *testing.T) {
	ms := []moment{
		{at: dayAt(10 * time.Hour), minutes: 0, name: "Standup"},
		{at: dayAt(10 * time.Hour), minutes: 5, name: "Write RFC"},
		{at: dayAt(10 * time.Hour), minutes: 0, name: "Fix CI"},
		{at: dayAt(10 * time.Hour), minutes: 1, name: "Lunch"},
	}
	want := []note{{"tt · now", "Standup · Fix CI"}, {"tt · in 5 min", "Write RFC"}, {"tt · in 1 min", "Lunch"}}
	if diff := cmp.Diff(want, alertNotes(ms), cmp.AllowUnexported(note{})); diff != "" {
		t.Errorf("notes (-want +got):\n%s", diff)
	}
	if got := alertNotes(nil); got != nil {
		t.Errorf("nothing due: %v", got)
	}
}

func TestReviewNotes(t *testing.T) {
	a := domain.SourceItem{ExternalID: "acme/api#12", Title: "Add\tretries", Author: "ana"}
	b := domain.SourceItem{ExternalID: "acme/web#3", Title: "Fix login"}
	c := domain.SourceItem{ExternalID: "acme/web#4", Title: "Bump deps"}
	prev := reviewIDs([]domain.SourceItem{a})
	if got := newReviews(prev, []domain.SourceItem{b, a, c}); !slices.Equal(got, []domain.SourceItem{b, c}) {
		t.Errorf("new requests: %v", got)
	}
	if got := newReviews(prev, []domain.SourceItem{a}); got != nil {
		t.Errorf("nothing new: %v", got)
	}
	if got := newReviews(reviewIDs(nil), []domain.SourceItem{a}); len(got) != 1 {
		t.Errorf("against an empty set every request is new: %v", got)
	}
	cases := []struct {
		items []domain.SourceItem
		want  note
	}{
		{[]domain.SourceItem{a}, note{"tt · review requested", "@ana acme/api#12 Add retries"}},
		{[]domain.SourceItem{b}, note{"tt · review requested", "acme/web#3 Fix login"}}, // no author known
		{[]domain.SourceItem{a, b, c}, note{"tt · 3 review requests", "Add retries · Fix login · Bump deps"}},
	}
	for _, c := range cases {
		if got := reviewNote(c.items); got != c.want {
			t.Errorf("reviewNote(%d) = %+v; want %+v", len(c.items), got, c.want)
		}
	}
}

func TestOsascriptGetsTheTextAsArguments(t *testing.T) {
	title, body := `tt · now`, `say "hi" & do shell script "rm -rf ~"`
	args := osascriptArgs(title, body)
	if n := len(args); n < 2 || args[n-2] != title || args[n-1] != body {
		t.Fatalf("the title and body are the last two arguments: %q", args)
	}
	for _, a := range args[:len(args)-2] {
		if a != "-e" && (strings.Contains(a, "hi") || strings.Contains(a, "now")) {
			t.Errorf("text spliced into the script: %q", a)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/ -run 'Moments|Due|AlertNotes|ReviewNotes|Osascript'`
Expected: FAIL (build): `undefined: moment`, `undefined: moments`, `undefined: due`, `undefined: note`.

- [ ] **Step 3: Implement**

Create `internal/tui/notify.go`:

```go
package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"tt/internal/app"
	"tt/internal/config"
	"tt/internal/domain"
)

// moment is one alert of a meeting or a task block (D64): due minutes
// before the start. Moving a block moves its moments.
type moment struct {
	at      time.Time
	minutes int
	name    string
}

// note is one macOS notification.
type note struct{ title, body string }

// moments lists the alert moments of today's busy meetings and of the
// blocks of open tasks, for the kinds n turns on, by when they're due.
func moments(st app.State, n config.Notify, loc *time.Location) []moment {
	var out []moment
	add := func(start time.Time, name string) {
		for _, m := range n.Alerts {
			out = append(out, moment{at: start.Add(-time.Duration(m) * time.Minute), minutes: m, name: domain.OneLine(name)})
		}
	}
	if n.Meetings {
		for _, e := range st.Calendar.Meetings {
			add(e.Start, e.Title)
		}
	}
	if n.Blocks && st.Plan.Day != "" {
		for _, b := range st.Plan.Blocks {
			if t, ok := st.Snapshot.Get(b.TaskID); ok && t.Status == domain.StatusOpen {
				add(st.Plan.Day.At(b.Start, loc), t.Title)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b moment) int { return a.at.Compare(b.at) })
	return out
}

// due keeps the moments in (from, to], except those more than a minute
// late at to: after the laptop slept they'd only be noise (D64).
func due(ms []moment, from, to time.Time) []moment {
	var out []moment
	for _, m := range ms {
		if m.at.After(from) && !m.at.After(to) && !m.at.Before(to.Add(-time.Minute)) {
			out = append(out, m)
		}
	}
	return out
}

// alertNotes makes one notification per lead time among the due moments
// (D64): `tt · in 5 min` or `tt · now`, the names joined by ` · `.
func alertNotes(ms []moment) []note {
	var (
		order []int
		names = map[int][]string{}
	)
	for _, m := range ms {
		if _, ok := names[m.minutes]; !ok {
			order = append(order, m.minutes)
		}
		names[m.minutes] = append(names[m.minutes], m.name)
	}
	var out []note
	for _, min := range order {
		title := "tt · now"
		if min > 0 {
			title = fmt.Sprintf("tt · in %d min", min)
		}
		out = append(out, note{title: title, body: strings.Join(names[min], " · ")})
	}
	return out
}

// reviewIDs is the set of the requests' ids.
func reviewIDs(items []domain.SourceItem) map[string]bool {
	ids := make(map[string]bool, len(items))
	for _, i := range items {
		ids[i.ExternalID] = true
	}
	return ids
}

// newReviews keeps the requests whose ids prev doesn't have (D65).
func newReviews(prev map[string]bool, cur []domain.SourceItem) []domain.SourceItem {
	var out []domain.SourceItem
	for _, i := range cur {
		if !prev[i.ExternalID] {
			out = append(out, i)
		}
	}
	return out
}

// reviewNote is one notification for the new requests (D65).
func reviewNote(items []domain.SourceItem) note {
	if len(items) == 1 {
		i := items[0]
		body := i.ExternalID + " " + domain.OneLine(i.Title)
		if i.Author != "" {
			body = "@" + i.Author + " " + body
		}
		return note{title: "tt · review requested", body: body}
	}
	titles := make([]string, len(items))
	for k, i := range items {
		titles[k] = domain.OneLine(i.Title)
	}
	return note{title: fmt.Sprintf("tt · %d review requests", len(items)), body: strings.Join(titles, " · ")}
}

// osascriptArgs shows a notification with title and body passed as the
// script's arguments, never as script text, so nothing in a title can run
// (D63). The title always starts with "tt", so a body starting with "-"
// isn't read as an option.
func osascriptArgs(title, body string) []string {
	return []string{"-e", "on run argv", "-e", "display notification (item 2 of argv) with title (item 1 of argv)", "-e", "end run", title, body}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/notify.go internal/tui/notify_test.go
git commit -m 'feat(tui): the notification planner: alert moments, grouping, review notes (D64, D65)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 3: The notify loop: alerts on the watch tick, new reviews on load, quiet while focused

D63–D66 in the running TUI. `Options.Notify` sends a notification: `nil` means `notifyWithSystem` (osascript with `osascriptArgs`), and the harness captures the calls in `h.notes`. `Model.notify` (`notifyState`) records the launch time, the last check (the launch at first), the last good GitHub fetch's request ids and time, the focus, and whether a failure was reported. `checkAlerts` runs on every watch tick (D13's 1-second tick, so no timer or seq of its own). It sends `alertNotes(due(moments(...), checked, now))` and moves `checked` to now, so each moment falls in exactly one window and fires once. `checkReviews` runs after every load. When `State.GitHub.LastSuccess` is not before the launch and differs from the last one seen, it diffs the requests (D65): the first such fetch only remembers them, and a failed fetch leaves `LastSuccess` alone. `send` does nothing while focused, and the dropped alerts aren't kept for later (D66). The first failure of the run shows `can't notify: …`. `View` sets `ReportFocus` only when a kind is on; `tea.FocusMsg`/`tea.BlurMsg` set the focus.

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/notify.go`
- Modify: `internal/tui/harness_test.go`
- Create: `internal/tui/notifications_test.go`

**Interfaces:**
- Consumes: Task 1 (`Config.Notify`, `Notify.On`), Task 2 (`moments`, `due`, `alertNotes`, `reviewIDs`, `newReviews`, `reviewNote`, `osascriptArgs`, `note`; test helper `dayAt`); existing `watchTickMsg`, `loadedMsg`, `setErr`, harness helpers `pinned`, `block`, `meetings`, `ev`, `reviewItem`, `setReviews`.
- Produces:
  - `Options.Notify func(ctx context.Context, title, body string) error`; `notifiedMsg struct{ err error }`.
  - internal/tui/notify.go: `type notifyState struct { launched, checked time.Time; reviews map[string]bool; reviewsAt time.Time; focused, failed bool }`; `(*Model).checkAlerts() tea.Cmd`, `checkReviews() tea.Cmd`, `send(notes ...note) tea.Cmd`, `notified(notifiedMsg)`, `notifier()`; `notifyWithSystem(ctx, title, body string) error`.
  - Harness: `h.notes []note`, `h.notifyErr error`; helpers (notifications_test.go) `notifyCfg(config.Notify) config.Config`, `(*harness).tickAt(off)`, `relaunch()`, `wantNotes(...note)`.

- [ ] **Step 1: Write the failing tests**

Patch `internal/tui/harness_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/harness_test.go b/internal/tui/harness_test.go
index 285b978..00e9a53 100644
--- a/internal/tui/harness_test.go
+++ b/internal/tui/harness_test.go
@@ -41,6 +41,9 @@ type harness struct {
 	editor  func(path string) error
 	opened  []string // links passed to Open
 	openErr error
+
+	notes     []note // notifications passed to Notify
+	notifyErr error
 }
 
 func newHarness(t *testing.T, cfg config.Config) *harness {
@@ -93,6 +96,10 @@ func startHarness(t *testing.T, cfg config.Config, cal *calendar.Fake, src *sour
 			h.opened = append(h.opened, url)
 			return h.openErr
 		},
+		Notify: func(_ context.Context, title, body string) error {
+			h.notes = append(h.notes, note{title: title, body: body})
+			return h.notifyErr
+		},
 	}
 	h.m = New(context.Background(), svc, opts)
 	h.run(h.m.Init())
```

Create `internal/tui/notifications_test.go`:

```go
package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"tt/internal/config"
	"tt/internal/domain"
)

// notifyCfg is the default config with n as [notify]; no alerts given
// means the default [5, 0].
func notifyCfg(n config.Notify) config.Config {
	cfg := config.Default()
	if n.Alerts == nil {
		n.Alerts = []int{5, 0}
	}
	cfg.Notify = n
	return cfg
}

// tickAt moves the clock to off on the harness's day and runs the watch tick.
func (h *harness) tickAt(off time.Duration) {
	h.t.Helper()
	h.clock.now = dayAt(off)
	h.send(watchTickMsg{})
}

// relaunch starts a new Model on the same database at the clock's time,
// like quitting tt and starting it again.
func (h *harness) relaunch() {
	h.t.Helper()
	h.m = New(context.Background(), h.svc, h.m.opts)
	h.run(h.m.Init())
	h.send(tea.WindowSizeMsg{Width: 140, Height: 40})
}

func (h *harness) wantNotes(want ...note) {
	h.t.Helper()
	if diff := cmp.Diff(want, h.notes, cmp.AllowUnexported(note{})); diff != "" {
		h.t.Fatalf("notifications (-want +got):\n%s", diff)
	}
}

func TestMeetingAndBlockAlerts(t *testing.T) {
	h := newCalHarness(t, notifyCfg(config.Notify{Meetings: true, Blocks: true}))
	rfc := h.pinned("Write RFC !1 ~s")
	h.block(rfc.ID, 10*time.Hour, time.Hour)
	h.meetings(ev("Standup", 10*time.Hour, 10*time.Hour+15*time.Minute))
	if !h.m.View().ReportFocus {
		t.Error("with a kind on, tt asks for focus reports (D66)")
	}

	h.tickAt(9*time.Hour + 54*time.Minute)
	h.wantNotes()
	h.tickAt(9*time.Hour + 55*time.Minute) // never focused: notifies (D66)
	soon := note{"tt · in 5 min", "Standup · Write RFC"}
	h.wantNotes(soon)
	h.tickAt(9*time.Hour + 55*time.Minute + 30*time.Second)
	h.wantNotes(soon) // once
	h.tickAt(10 * time.Hour)
	now := note{"tt · now", "Standup · Write RFC"}
	h.wantNotes(soon, now)
	h.tickAt(10*time.Hour + time.Minute)
	h.wantNotes(soon, now)
}

func TestAlertsOnlyForMomentsPassedWhileRunning(t *testing.T) {
	h := newHarness(t, notifyCfg(config.Notify{Blocks: true}))
	early := h.pinned("Early !1 ~s")
	h.block(early.ID, 8*time.Hour+30*time.Minute, 30*time.Minute) // started before launch
	h.tickAt(9*time.Hour + 20*time.Second)
	h.wantNotes()

	h.clock.now = dayAt(9*time.Hour + 27*time.Minute)
	late := h.pinned("Late !1 ~s")
	h.block(late.ID, 9*time.Hour+30*time.Minute, 30*time.Minute) // its 5-minute moment passed already
	moved := h.pinned("Moved !1 ~s")
	b := h.block(moved.ID, 10*time.Hour, 30*time.Minute)
	if _, err := h.svc.MoveBlock(context.Background(), b.ID, 11*time.Hour); err != nil {
		t.Fatal(err)
	}
	done := h.pinned("Done !1 ~s")
	h.block(done.ID, 12*time.Hour, 30*time.Minute)
	if _, err := h.svc.Done(context.Background(), done.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")

	h.tickAt(9*time.Hour + 28*time.Minute)
	h.wantNotes()
	h.tickAt(9*time.Hour + 30*time.Minute)
	h.wantNotes(note{"tt · now", "Late"})
	h.tickAt(9*time.Hour + 55*time.Minute) // Moved's old moment
	h.tickAt(10 * time.Hour)
	h.wantNotes(note{"tt · now", "Late"})
	h.tickAt(10*time.Hour + 55*time.Minute)
	h.tickAt(11*time.Hour + 55*time.Minute) // Done's block: a closed task's
	h.tickAt(12 * time.Hour)
	h.wantNotes(note{"tt · now", "Late"}, note{"tt · in 5 min", "Moved"})

	// Asleep from 12:00 to 13:58: the moments it slept through stay quiet.
	again := h.pinned("Again !1 ~s")
	h.block(again.ID, 13*time.Hour, 30*time.Minute)
	h.tickAt(13*time.Hour + 58*time.Minute)
	h.wantNotes(note{"tt · now", "Late"}, note{"tt · in 5 min", "Moved"})

	// Quit and start again at 15:55:30: the 15:55 moment was before launch.
	last := h.pinned("Last !1 ~s")
	h.block(last.ID, 16*time.Hour, 30*time.Minute)
	h.clock.now = dayAt(15*time.Hour + 55*time.Minute + 30*time.Second)
	h.relaunch()
	h.tickAt(15*time.Hour + 55*time.Minute + 40*time.Second)
	h.tickAt(16 * time.Hour)
	h.wantNotes(note{"tt · now", "Late"}, note{"tt · in 5 min", "Moved"}, note{"tt · now", "Last"})
}

func TestNoAlertsWhileFocused(t *testing.T) {
	h := newHarness(t, notifyCfg(config.Notify{Blocks: true}))
	rfc := h.pinned("Write RFC !1 ~s")
	h.block(rfc.ID, 10*time.Hour, time.Hour)
	h.send(tea.FocusMsg{})
	h.tickAt(9*time.Hour + 55*time.Minute)
	h.wantNotes() // on the screen already
	h.send(tea.BlurMsg{})
	h.tickAt(9*time.Hour + 56*time.Minute)
	h.wantNotes() // dropped, not kept for later
	h.tickAt(10 * time.Hour)
	h.wantNotes(note{"tt · now", "Write RFC"})
}

func TestNotificationsAreOffByDefault(t *testing.T) {
	h := newCalHarness(t, config.Default())
	rfc := h.pinned("Write RFC !1 ~s")
	h.block(rfc.ID, 10*time.Hour, time.Hour)
	h.meetings(ev("Standup", 10*time.Hour, 10*time.Hour+15*time.Minute))
	h.tickAt(9*time.Hour + 55*time.Minute)
	h.tickAt(10 * time.Hour)
	if h.m.View().ReportFocus {
		t.Error("every kind off: no focus reports")
	}

	g := newGHHarness(t, config.Default())
	g.clock.now = g.clock.now.Add(time.Minute)
	g.setReviews(g.reviewItem("acme/api", 12, "Add retries", "ana", time.Hour))
	h.notes = append(h.notes, g.notes...)
	h.wantNotes()
}

func TestNewReviewRequestsNotify(t *testing.T) {
	h := newGHHarness(t, notifyCfg(config.Notify{Reviews: true}))
	a := h.reviewItem("acme/api", 12, "Add retries", "ana", time.Hour)
	b := h.reviewItem("acme/web", 3, "Fix login", "bo", 2*time.Hour)
	c := h.reviewItem("acme/web", 4, "Bump deps", "bo", 3*time.Hour)
	later := func() { h.clock.now = h.clock.now.Add(time.Minute) }

	later()
	h.setReviews(a) // the startup fetch saw none
	one := note{"tt · review requested", "@ana acme/api#12 Add retries"}
	h.wantNotes(one)
	later()
	h.setReviews(a)
	h.wantNotes(one)
	later()
	h.src.Set(nil, errors.New("offline"))
	h.keys("R")
	h.wantNotes(one) // a failed fetch keeps the last good set
	later()
	h.setReviews(a, b, c)
	two := note{"tt · 2 review requests", "Fix login · Bump deps"}
	h.wantNotes(one, two)
	later()
	h.setReviews(b, c)
	later()
	h.setReviews(a, b, c) // re-requested: new again (D65)
	h.wantNotes(one, two, one)

	// What arrived while tt was closed: the first fetch since launch is silent.
	d := h.reviewItem("acme/api", 13, "Drop v1", "ana", time.Minute)
	later()
	h.src.Set([]domain.SourceItem{a, b, c, d}, nil)
	h.relaunch()
	later()
	h.setReviews(a, b, c, d)
	h.wantNotes(one, two, one)
}

func TestAFailedSendSaysSoOnce(t *testing.T) {
	h := newHarness(t, notifyCfg(config.Notify{Blocks: true}))
	h.notifyErr = errors.New("exec: \"osascript\": executable file not found in $PATH")
	rfc := h.pinned("Write RFC !1 ~s")
	h.block(rfc.ID, 10*time.Hour, time.Hour)
	h.tickAt(9*time.Hour + 55*time.Minute)
	h.mustSee(`can't notify: exec: "osascript": executable file not found`)
	h.m.setStatus("")
	h.tickAt(10 * time.Hour)
	if len(h.notes) != 2 || h.m.status != "" {
		t.Errorf("the second failure: %d sends, status %q; want it tried and not reported again", len(h.notes), h.m.status)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL (build): `internal/tui/harness_test.go: unknown field Notify in struct literal of type Options`.

- [ ] **Step 3: Implement**

Patch `internal/tui/model.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/model.go b/internal/tui/model.go
index c61d12c..8b0b38d 100644
--- a/internal/tui/model.go
+++ b/internal/tui/model.go
@@ -29,6 +29,8 @@ type Options struct {
 	Exec func(c *exec.Cmd, done tea.ExecCallback) tea.Cmd
 	// Open opens a link (D21). nil means macOS `open`.
 	Open func(ctx context.Context, url string) error
+	// Notify shows a macOS notification (D63). nil means osascript.
+	Notify func(ctx context.Context, title, body string) error
 }
 
 func DefaultOptions() Options {
@@ -94,6 +96,8 @@ type Model struct {
 	cal pollLoop // the calendar's
 	gh  pollLoop // GitHub's (D50)
 
+	notify notifyState
+
 	overlay     overlay
 	banner      *app.DayResult
 	undo        []undoEntry
@@ -102,7 +106,9 @@ type Model struct {
 }
 
 func New(ctx context.Context, svc *app.Service, opts Options) *Model {
-	return &Model{svc: svc, opts: opts, ctx: ctx, width: 100, height: 30, pick: pickState{group: search.ByPriority}}
+	now := svc.Now()
+	return &Model{svc: svc, opts: opts, ctx: ctx, width: 100, height: 30, pick: pickState{group: search.ByPriority},
+		notify: notifyState{launched: now, checked: now}}
 }
 
 // pollLoop is an integration's poll loop (D3): one fetch at a time, then the
@@ -152,6 +158,7 @@ type (
 		closed []domain.Task // tasks the fetch closed (D20a)
 		err    error
 	}
+	notifiedMsg struct{ err error }
 )
 
 func (m *Model) Init() tea.Cmd {
@@ -197,8 +204,9 @@ func (m *Model) update(msg tea.Msg) tea.Cmd {
 		m.refreshRows()
 		m.refocus(had)
 		m.inbox.sync(m)
+		return m.checkReviews()
 	case watchTickMsg:
-		cmds := []tea.Cmd{m.checkVersion(), m.opts.Schedule(m.opts.WatchEvery, watchTickMsg{})}
+		cmds := []tea.Cmd{m.checkVersion(), m.checkAlerts(), m.opts.Schedule(m.opts.WatchEvery, watchTickMsg{})}
 		if m.dayIsStale() {
 			cmds = append(cmds, m.startDay())
 		}
@@ -302,6 +310,12 @@ func (m *Model) update(msg tea.Msg) tea.Cmd {
 		return tea.Batch(m.load(), m.opts.Schedule(m.svc.Config().GitHubPoll, ghTickMsg{seq: m.gh.seq}))
 	case noteEditedMsg:
 		return m.noteEdited(msg)
+	case notifiedMsg:
+		m.notified(msg)
+	case tea.FocusMsg:
+		m.notify.focused = true
+	case tea.BlurMsg:
+		m.notify.focused = false
 	case tea.KeyPressMsg:
 		return m.handleKey(msg)
 	case tea.PasteMsg:
@@ -492,6 +506,7 @@ func (m *Model) View() tea.View {
 	v := tea.NewView(m.render())
 	v.AltScreen = true
 	v.WindowTitle = "tt"
+	v.ReportFocus = m.svc.Config().Notify.On() // only notifications care (D66)
 	return v
 }
 
```

Patch `internal/tui/notify.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/notify.go b/internal/tui/notify.go
index b66e9a1..b127707 100644
--- a/internal/tui/notify.go
+++ b/internal/tui/notify.go
@@ -1,16 +1,106 @@
 package tui
 
 import (
+	"context"
 	"fmt"
+	"os/exec"
 	"slices"
 	"strings"
 	"time"
 
+	tea "charm.land/bubbletea/v2"
+
 	"tt/internal/app"
 	"tt/internal/config"
 	"tt/internal/domain"
 )
 
+// notifyState is the notifications' (D63–D66). Nothing of it is persisted:
+// what came before launch never notifies anyway.
+type notifyState struct {
+	launched time.Time
+	checked  time.Time // the alerts due up to here are handled; launch at first
+	// reviews is the last good GitHub fetch's requests (at reviewsAt); nil
+	// until the first good fetch since launch.
+	reviews   map[string]bool
+	reviewsAt time.Time
+	focused   bool // tt's pane has the focus (D66); false until a focus event says so
+	failed    bool // a send failed, and the status said so (once per run)
+}
+
+// checkAlerts sends the meeting and block alerts due since the last check
+// (D64). It runs on the watch tick, so a laptop waking from sleep catches
+// up within a second, and due drops what's over a minute late.
+func (m *Model) checkAlerts() tea.Cmd {
+	n := m.svc.Config().Notify
+	now := m.svc.Now()
+	if !m.loaded || !(n.Meetings || n.Blocks) || !now.After(m.notify.checked) {
+		return nil
+	}
+	ms := due(moments(m.state, n, now.Location()), m.notify.checked, now)
+	m.notify.checked = now
+	return m.send(alertNotes(ms)...)
+}
+
+// checkReviews notifies the requests a good GitHub fetch found that the
+// previous good fetch didn't (D65). The first good fetch since launch only
+// remembers them: they were there before. A failed fetch keeps the last
+// good one's time, so it changes nothing.
+func (m *Model) checkReviews() tea.Cmd {
+	gh := m.state.GitHub
+	if !m.svc.Config().Notify.Reviews || gh.LastSuccess.Before(m.notify.launched) || gh.LastSuccess.Equal(m.notify.reviewsAt) {
+		return nil
+	}
+	prev := m.notify.reviews
+	m.notify.reviews, m.notify.reviewsAt = reviewIDs(gh.Reviews), gh.LastSuccess
+	if fresh := newReviews(prev, gh.Reviews); prev != nil && len(fresh) > 0 {
+		return m.send(reviewNote(fresh))
+	}
+	return nil
+}
+
+// send shows the notes in the background, unless tt has the focus (D66):
+// what is on the screen needs no notification, and it isn't kept for later.
+func (m *Model) send(notes ...note) tea.Cmd {
+	if m.notify.focused || len(notes) == 0 {
+		return nil
+	}
+	fn, ctx := m.notifier(), m.ctx
+	return func() tea.Msg {
+		for _, n := range notes {
+			if err := fn(ctx, n.title, n.body); err != nil {
+				return notifiedMsg{err: err}
+			}
+		}
+		return nil
+	}
+}
+
+// notified reports the first failed send of the run (D63); later ones
+// would only repeat it.
+func (m *Model) notified(msg notifiedMsg) {
+	if msg.err != nil && !m.notify.failed {
+		m.notify.failed = true
+		m.setErr(fmt.Errorf("can't notify: %w", msg.err))
+	}
+}
+
+func (m *Model) notifier() func(ctx context.Context, title, body string) error {
+	if m.opts.Notify != nil {
+		return m.opts.Notify
+	}
+	return notifyWithSystem
+}
+
+// notifyWithSystem shows a notification through macOS osascript.
+func notifyWithSystem(ctx context.Context, title, body string) error {
+	out, err := exec.CommandContext(ctx, "osascript", osascriptArgs(title, body)...).CombinedOutput()
+	if err != nil && len(out) > 0 {
+		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
+	}
+	return err
+}
+
 // moment is one alert of a meeting or a task block (D64): due minutes
 // before the start. Moving a block moves its moments.
 type moment struct {
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/harness_test.go internal/tui/model.go internal/tui/notifications_test.go internal/tui/notify.go
git commit -m 'feat(tui): send meeting, block and review notifications, quiet while focused (D63–D66)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---


## After the last task

- Build tt into the scratchpad and run `docs/superpowers/tooling/pty_notify.py <tt> <workdir>` (copy it into the scratchpad first). It puts a fake `osascript` on `PATH` that logs its arguments and makes three runs on a throwaway database. Run 1 adds `Alpha`, pins it and places a block two rows below the now row. Run 2 starts with `[notify] blocks = true` and two alerts timed to land a minute and two minutes after launch; it sends focus-in before the first (nothing logged), then focus-out before the second (exactly one call, title `tt · in N min`, body `Alpha`, passed as arguments). Run 3, with every kind off, asks for no focus reports (`\x1b[?1004h` absent). Every line prints `OK`. It takes about three minutes.
- `go test -race ./internal/tui/ ./internal/app/`.
- Final whole-branch review, one fix wave, park residuals as spec carry-forwards.
- **The user tries the branch:** `make build` in the worktree, add `[notify]` with the kinds wanted to `~/.config/tt/config.toml`, set `set -g focus-events on` in tmux, and run `<worktree>/bin/tt` in the usual pane. The first notification makes macOS ask to allow notifications for Script Editor. Merge (fast-forward to `main`) only after the user says so.
