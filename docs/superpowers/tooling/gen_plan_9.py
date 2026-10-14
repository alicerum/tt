#!/usr/bin/env python3
"""Generate the Plan 9 doc from the verified scratch commits (main..HEAD).

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
        title="`[notify]` in the config: kinds off by default, alert minutes",
        intro="D63, D64. A `Notify` struct in `config.Config` holds the three opt-in kinds (`Meetings`, `Blocks`, `Reviews`, all `false` by default) and `Alerts`, the whole minutes before a meeting or block starts at which it notifies (default `[5, 0]`; `0` is the start itself). `parseNotify` applies `[notify]`. Each alert must be an integer from 0 to 120 and listed once, and the list can't be empty (a kind is turned off instead). TOML rejects fractions and strings itself, and the error names `notify.alerts`. `config.example.toml` gains the section with every default, which `TestExampleConfigIsTheDefaults` already checks. Nothing reads the config yet.",
        interfaces="""- Consumes: existing `file`, `Parse`, `Default`.
- Produces (internal/config/config.go):
  - `type Notify struct { Meetings, Blocks, Reviews bool; Alerts []int }`, `func (n Notify) On() bool` (any kind on).
  - `Config.Notify Notify`; `Default().Notify == Notify{Alerts: []int{5, 0}}`.""",
        step2="Run: `go test ./internal/config/`\nExpected: FAIL (build): `Default().Notify undefined (type Config has no field or method Notify)`.",
        step4="Run: `go test ./internal/config/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`\nExpected: PASS, no gofmt output.",
    ),
    dict(
        title="The notification planner: alert moments, the due window, the texts",
        intro="D64, D65, as pure functions in a new `internal/tui/notify.go`, tested on their own before anything sends. `moments` lists the alert moments for the kinds that are on. Meetings come from today's busy meetings (`State.Calendar.Meetings`, D17a already applied). Blocks are those whose task is open; a block past midnight is due on the next date (`Day.At`). Each gets one moment per alert minute, sorted by when it's due, with names cleaned to one line (`domain.OneLine`). `due` keeps the moments in `(from, to]`, dropping any more than a minute late at `to` (the laptop slept). `alertNotes` makes one notification per lead time: `tt · in 5 min` or `tt · now`, with the names joined by ` · `. `reviewIDs`/`newReviews`/`reviewNote` are D65's set difference and its texts. `osascriptArgs` passes the title and body as the script's arguments, never as script text. Nothing on screen changes in this task.",
        interfaces="""- Consumes: Task 1 (`config.Notify`); existing `app.State`, `domain.OneLine`, `domain.Day.At`, `domain.Snapshot.Get`.
- Produces (internal/tui/notify.go):
  - `type moment struct { at time.Time; minutes int; name string }`; `type note struct { title, body string }`.
  - `func moments(st app.State, n config.Notify, loc *time.Location) []moment` (nil when nothing is on or `st.Plan.Day == ""` and no meetings).
  - `func due(ms []moment, from, to time.Time) []moment`; `func alertNotes(ms []moment) []note`.
  - `func reviewIDs(items []domain.SourceItem) map[string]bool`; `func newReviews(prev map[string]bool, cur []domain.SourceItem) []domain.SourceItem`; `func reviewNote(items []domain.SourceItem) note`.
  - `func osascriptArgs(title, body string) []string`.
  - Test helpers (notify_test.go): `notifyDay`, `dayAt(off time.Duration) time.Time` (2026-10-12 UTC plus off, the harness's day), `notifyFixture() app.State`, `momentNames([]moment) []string`.""",
        step2="Run: `go test ./internal/tui/ -run 'Moments|Due|AlertNotes|ReviewNotes|Osascript'`\nExpected: FAIL (build): `undefined: moment`, `undefined: moments`, `undefined: due`, `undefined: note`.",
        step4="Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`\nExpected: PASS, no gofmt output.",
    ),
    dict(
        title="The notify loop: alerts on the watch tick, new reviews on load, quiet while focused",
        intro="D63–D66 in the running TUI. `Options.Notify` sends a notification: `nil` means `notifyWithSystem` (osascript with `osascriptArgs`), and the harness captures the calls in `h.notes`. `Model.notify` (`notifyState`) records the launch time, the last check (the launch at first), the last good GitHub fetch's request ids and time, the focus, and whether a failure was reported. `checkAlerts` runs on every watch tick (D13's 1-second tick, so no timer or seq of its own). It sends `alertNotes(due(moments(...), checked, now))` and moves `checked` to now, so each moment falls in exactly one window and fires once. `checkReviews` runs after every load. When `State.GitHub.LastSuccess` is not before the launch and differs from the last one seen, it diffs the requests (D65): the first such fetch only remembers them, and a failed fetch leaves `LastSuccess` alone. `send` does nothing while focused, and the dropped alerts aren't kept for later (D66). The first failure of the run shows `can't notify: …`. `View` sets `ReportFocus` only when a kind is on; `tea.FocusMsg`/`tea.BlurMsg` set the focus.",
        interfaces="""- Consumes: Task 1 (`Config.Notify`, `Notify.On`), Task 2 (`moments`, `due`, `alertNotes`, `reviewIDs`, `newReviews`, `reviewNote`, `osascriptArgs`, `note`; test helper `dayAt`); existing `watchTickMsg`, `loadedMsg`, `setErr`, harness helpers `pinned`, `block`, `meetings`, `ev`, `reviewItem`, `setReviews`.
- Produces:
  - `Options.Notify func(ctx context.Context, title, body string) error`; `notifiedMsg struct{ err error }`.
  - internal/tui/notify.go: `type notifyState struct { launched, checked time.Time; reviews map[string]bool; reviewsAt time.Time; focused, failed bool }`; `(*Model).checkAlerts() tea.Cmd`, `checkReviews() tea.Cmd`, `send(notes ...note) tea.Cmd`, `notified(notifiedMsg)`, `notifier()`; `notifyWithSystem(ctx, title, body string) error`.
  - Harness: `h.notes []note`, `h.notifyErr error`; helpers (notifications_test.go) `notifyCfg(config.Notify) config.Config`, `(*harness).tickAt(off)`, `relaunch()`, `wantNotes(...note)`.""",
        step2="Run: `go test ./internal/tui/`\nExpected: FAIL (build): `internal/tui/harness_test.go: unknown field Notify in struct literal of type Options`.",
        step4="Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`\nExpected: PASS, no gofmt output.",
    ),
]
assert len(TASKS) == len(commits), (len(TASKS), len(commits))

HEADER = """# tt Plan 9 — Notifications Implementation Plan

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
3. **Titles with quotes, `&`, newlines, tabs, escape sequences or a leading `-`** (task, meeting and PR titles): the text is never run as script, and it arrives on one line. Pinned in Task 2 `TestOsascriptGetsTheTextAsArguments`, `TestReviewNotes` (a tab), `TestMomentsFollowTheKindsAndAlerts` (`Late\\nnight`).
4. **Focus reports missing or stale** (tmux without `focus-events on`, a terminal that never reports): with no focus event ever seen tt notifies; alerts due while focused are dropped, not sent on blur. Pinned in Task 3 `TestMeetingAndBlockAlerts` (never focused), `TestNoAlertsWhileFocused`, and the pty run (`\\x1b[I`/`\\x1b[O`).
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
"""

FOOTER = """
## After the last task

- Build tt into the scratchpad and run `docs/superpowers/tooling/pty_notify.py <tt> <workdir>` (copy it into the scratchpad first). It puts a fake `osascript` on `PATH` that logs its arguments and makes three runs on a throwaway database. Run 1 adds `Alpha`, pins it and places a block two rows below the now row. Run 2 starts with `[notify] blocks = true` and two alerts timed to land a minute and two minutes after launch; it sends focus-in before the first (nothing logged), then focus-out before the second (exactly one call, title `tt · in N min`, body `Alpha`, passed as arguments). Run 3, with every kind off, asks for no focus reports (`\\x1b[?1004h` absent). Every line prints `OK`. It takes about three minutes.
- `go test -race ./internal/tui/ ./internal/app/`.
- Final whole-branch review, one fix wave, park residuals as spec carry-forwards.
- **The user tries the branch:** `make build` in the worktree, add `[notify]` with the kinds wanted to `~/.config/tt/config.toml`, set `set -g focus-events on` in tmux, and run `<worktree>/bin/tt` in the usual pane. The first notification makes macOS ask to allow notifications for Script Editor. Merge (fast-forward to `main`) only after the user says so.
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
