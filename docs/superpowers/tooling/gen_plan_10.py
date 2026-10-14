#!/usr/bin/env python3
"""Generate the Plan 10 doc from the verified scratch commits (main..HEAD).

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
        title="Impact, due date and urgency in the model; migration 006",
        intro="D68–D70, D76, additive: nothing reads the new fields yet, and priority still works everywhere. `domain.Task` gains `Impact` (0 = unset, 1–3) and `Due` (a `domain.Day`, \"\" = none). A new `internal/domain/urgency.go` holds the impact steps' names and examples (shown on the Inbox card later), `EffectiveImpact` (always the parent's for a child) and `EffectiveDue` (own, else the parent's), Taskwarrior's due ramp `DueFactor` in whole planning days, `Urgency` (the weighted mean of impact/3 and the due factor; `ok` false while untriaged), `UrgencyScore` (0–100 as shown), `CompareUrgency` (more urgent first, untriaged last) and day arithmetic (`Day.AddDays`, `DaysBetween`). The store reads and writes `impact` and `due`. Migration `006` adds both columns (`ALTER TABLE … ADD COLUMN`, no table rebuild), gives no task an impact, and gives open source-linked tasks impact 1 and a due 3 days after their request's UTC date (first seen without one; none when the source item is gone). `priority` stays in the schema.",
        interfaces="""- Consumes: existing `domain.Day`, `domain.Task`, store `taskColumns`/`taskArgs`/`scanTask`, `source_items`.
- Produces (internal/domain/urgency.go):
  - `const MaxImpact = 3`; `var ImpactNames, ImpactExamples []string` (index i-1 for step i).
  - `func EffectiveImpact(t Task, parent *Task) int`; `func EffectiveDue(t Task, parent *Task) Day`.
  - `type Weights struct{ Impact, Due float64 }`; `var DefaultWeights = Weights{Impact: 1, Due: 1}`.
  - `func DueFactor(today, due Day) float64`; `func Urgency(t Task, parent *Task, today Day, w Weights) (u float64, ok bool)`; `func UrgencyScore(u float64) int`; `func CompareUrgency(a float64, aok bool, b float64, bok bool) int`.
  - `func (d Day) AddDays(n int) Day`; `func DaysBetween(from, to Day) int` (positive when `to` is later).
- Produces: `domain.Task.Impact int`, `domain.Task.Due Day`; store migration `006_impact_due.sql`.""",
        step2="Run: `go test ./internal/domain/ ./internal/store/`\nExpected: FAIL (build): `today.AddDays undefined (type Day has no field or method AddDays)`, `undefined: DueFactor`, and in store `unknown field Impact in struct literal of type domain.Task`.",
        step4="Run: `go test ./internal/domain/ ./internal/store/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`\nExpected: PASS, no gofmt output.",
    ),
    dict(
        title="`^` due dates and the new config keys",
        intro="D70, D73, D75, still additive. A new `internal/dsl/due.go` reads the `^` forms against a given today (a planning day): `today`, `tom`/`tomorrow`, `mon`…`sun` (the next one, today counts), `Nd`/`Nw`, `30oct` (the next 30 Oct, today counts; a date that doesn't exist this year or next is an error) and `2026-10-30`, case-insensitive. `dueWord` tells \"not a date\" (stays in the title, like `~`) from \"a date's shape but no real date\" (an error). `ParseDue` is the Inbox input's reader: with or without the `^`, empty means no due. In config, `[urgency] impact` and `due` are weights (numbers ≥ 0, not both 0; TOML integers work), and `[github] impact` (1–3) and `due` (`Nd`, 0d–365d, or `\"\"` for no due) are a pinned review's defaults (`ReviewImpact`, `ReviewDue`, -1 = no due). `config.example.toml` lists them with their defaults. `[[priorities]]` and `[github] priority` still work until Task 3.",
        interfaces="""- Consumes: Task 1 (`domain.Day.AddDays`, `domain.MaxImpact`, `domain.Weights`, `domain.DefaultWeights`); existing `isDigits` in internal/dsl.
- Produces:
  - internal/dsl/due.go: `func ParseDue(text string, today domain.Day) (domain.Day, error)`; `func dueWord(w string, today domain.Day) (due domain.Day, ok bool, err error)` (w without its `^`); test constant `wednesday = domain.Day(\"2026-10-14\")` in due_test.go.
  - internal/config/config.go: `Config.ReviewImpact int` (default 1), `Config.ReviewDue int` (days, default 3, -1 = none), `Config.Urgency domain.Weights` (default `domain.DefaultWeights`); `parseUrgency`.""",
        step2="Run: `go test ./internal/dsl/ ./internal/config/`\nExpected: FAIL (build): `undefined: dueWord`, `undefined: ParseDue`, and `Default().Urgency undefined (type Config has no field or method Urgency)`.",
        step4="Run: `go test ./internal/dsl/ ./internal/config/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`\nExpected: PASS, no gofmt output.",
    ),
    dict(
        title="Impact and urgency replace priority",
        intro="D68, D71–D76: the switch, in every package at once (removing `Task.Priority` breaks every user of it). `domain`: `Priority` and `EffectivePriority` go; `InInbox` follows D74 (a top-level task without impact or size; a child without its own size). `store`: `priority` is no longer read or written. `config`: `[[priorities]]`, `PriorityLevel`, `Priorities` and `ReviewPriority` go; the old keys get their own errors (D76). `dsl`: `ParseAdd(line, today)` reads `!1`–`!3` as impact (`impact !4 out of range: use !1 to !3`) and `^` as due (last one wins); `FormatAdd` writes `!n` and an absolute `^YYYY-MM-DD`; `ParseFilter(line)` filters `!n` by effective impact. `app`: `Add`, `Edit`, `EditLine`, `ParseEdit`, `ParseChild`, `AddChild` use impact and due against `s.Today()`; `SetImpact` (refused on a child) replaces `SetPriority`; `SetDue` is new; `PinSource` uses D75's defaults; candidates order by urgency. `search`: `ByUrgency` (the default) and `ByImpact` are flat sorts (`Flat`, `Sorter`), `ByPriority` is gone, a remembered `priority` falls back to urgency. `tui`: the badge is `[ 73 S]` (`badgeOf(u, ok, size)`), the Inbox's `1`–`3` set impact with the steps on the card, People lists by urgency, the help and placeholders show `!1 ^fri`. `cmd/tt` usage and the README show the new syntax. The existing TUI tests swap `!1` and `!3` so each keeps its meaning (old P1, the top, is impact 3, the biggest): with no due, `[P1 S]` reads `[ 50 S]`, `[P2 S]` `[ 33 S]`, `[P3 S]` `[ 17 S]`.",
        interfaces="""- Consumes: Task 1 (urgency.go, `Task.Impact`, `Task.Due`), Task 2 (`dueWord`, `ParseDue`, `Config.Urgency`, `ReviewImpact`, `ReviewDue`).
- Produces:
  - internal/dsl: `Draft.Impact int`, `Draft.Due domain.Day` (no `Priority`); `func ParseAdd(line string, today domain.Day) (Draft, error)`; `func ParseFilter(line string) (Filter, error)`; `Filter.Impacts []int` (no `Priorities`); `func parseImpact(tok string) (int, error)`.
  - internal/app: `func (s *Service) SetImpact(ctx context.Context, id int64, n int) (Undo, error)`; `func (s *Service) SetDue(ctx context.Context, id int64, due domain.Day) (Undo, error)`; `inheritedImpact(parentID int64, n int) error`.
  - internal/search: `ByUrgency`, `ByImpact`; `func (g GroupBy) Flat() bool`; `func Sorter(g GroupBy, parentOf func(domain.Task) *domain.Task, today domain.Day, w domain.Weights) func(a, b domain.Task) int`; `func GroupOf(by GroupBy, t domain.Task, now time.Time) Group` (`Group{}` for a flat sort).
  - internal/config: `func unknownKey(key string) error`.
  - internal/tui: `(*Model).urgency(t domain.Task, parent *domain.Task) (float64, bool)`; `func badgeOf(u float64, ok bool, size domain.Size) string`; `func sizeLabel(size domain.Size) string`.""",
        step2="Run: `go test ./...`\nExpected: FAIL (build): `internal/dsl/add_test.go: unknown field Impact in struct literal of type Draft` and `cannot use wednesday (constant \"2026-10-14\" of string type domain.Day) as int value in argument to ParseAdd`, and the like in every package.",
        step4="Run: `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`\nExpected: PASS, no gofmt output.",
    ),
    dict(
        title="Due on screen: the marker, Task detail, the Inbox's due first and ⏎",
        intro="D71, D73. `dueMarker` words a due against today: `→today`, `→fri` within 6 days, `→30 oct` later (`→15 nov 2027` over a year away), `2d late` in the alert colour; `(*Model).due` uses the effective due of open tasks. It follows the title (and its ✎) on every row (the Log, Today's candidates, People) and on Task detail's children. Task detail gets a line under its header: `impact: one team · due Fri 16 Oct · urgency 73`, with `(from #12)` after an inherited value and `impact: not set` when untriaged. The Inbox card shows the due first (`due       — · ^ to set`; a child's inherited one `from parent #1 · ^ to set its own`). `^` opens a `due › ` prompt (prefilled with the task's own due, any `^` form, empty clears, a bad form keeps the prompt with its error). The card no longer leaves on its own: `inboxState.held` keeps it on screen once a key acted on it, `triaged · ⏎ next card` shows when it is, and `⏎` moves on (`set impact and size first · n skips`, a child `set the size first · n skips`, otherwise). A held card that is dropped or closed leaves at once; ctrl+z of its last triage step leaves it untriaged on screen.",
        interfaces="""- Consumes: Task 1 (`EffectiveDue`, `EffectiveImpact`, `ImpactNames`, `UrgencyScore`, `DaysBetween`), Task 2 (`dsl.ParseDue`), Task 3 (`SetDue`, `(*Model).urgency`, `badgeOf`); existing `prompt`, `newInput`, `dayLabel`, `styleAlert`, harness helpers `before`, `family`.
- Produces:
  - internal/tui/common.go: `func dueMarker(today, due domain.Day) string`; `func (m *Model) due(t domain.Task) string`.
  - internal/tui/detail.go: `func (m *Model) urgencyLine(t domain.Task) string`.
  - internal/tui/inbox.go: `inboxState.held int64`; `func (m *Model) openDuePrompt(t domain.Task) tea.Cmd`.""",
        step2="Run: `go test ./internal/tui/`\nExpected: FAIL (build): `internal/tui/due_test.go:31:24: undefined: dueMarker`.",
        step4="Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`\nExpected: PASS, no gofmt output.",
    ),
]
assert len(TASKS) == len(commits), (len(TASKS), len(commits))

HEADER = """# tt Plan 10 — Impact and Urgency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Priority levels (P1/P2/P3) give way to impact (1–3, who notices if it slips) and an optional due date (`^fri`). tt derives Taskwarrior-style urgency from them, shows it as `[ 73 S]` where the P-level was, sorts the backlog by urgency or impact, and moves existing tasks off priority with one re-triage.

**Architecture:** Impact and due are stored on the task (migration 006); urgency is a pure function in `domain` of the effective impact, the effective due and the planning day, weighted by `[urgency]`, and never stored. The switch happens in one task across `dsl`, `config`, `app`, `search` and `tui`, between two additive tasks before it (the model, the parsers and config) and one after it (the due marker, Task detail and the Inbox's due-first flow).

**Tech Stack:** Go 1.27; Bubble Tea v2.1, Lip Gloss; pure-Go SQLite (modernc), BurntSushi TOML. No new Go dependencies.

**Spec:** `docs/superpowers/specs/2026-10-12-task-tracking-design.md`: D68–D76 and the section "Plan 10 — Impact and urgency" (with D11, D14, D25, D26, D26d, D32, D48 and D52 as amended).

## Global Constraints

- No new Go dependencies: `go.mod` and `go.sum` stay unchanged.
- Impact is 1–3: **1** a person, **2** one team, **3** more than one team (who notices if it slips). Children take their parent's impact, never their own.
- Urgency, exactly: `(w_i·impact/3 + w_d·f) / (w_i + w_d)`; `f` = 0 with no due, 0.2 more than 14 days ahead, `0.2 + (d + 14)·0.8/21` from 14 days ahead to 7 overdue (`d` = today − due in whole planning days), 1.0 after. Default weights 1 and 1. Shown as `UrgencyScore` = round(u·100), padded to three cells: `[ 73 S]`, untriaged `[  — S]`.
- `^` forms: `today`, `tom`, `tomorrow`, `mon`…`sun`, `Nd`, `Nw`, `30oct`, `2026-10-30`; against the planning day (D19), not the calendar date.
- Texts, exactly: `impact !4 out of range: use !1 to !3`; `children inherit impact from #N; drop !n` (the service error, after `invalid value: `); `[[priorities]] was replaced by impact (D68): remove it, see config.example.toml`; `[github] priority was replaced by impact and due (D75): remove it, see config.example.toml`; `due "x": want today, tom, a weekday like fri, 3d, 2w, 30oct or 2026-10-30`; `due ^31feb: no such date`; the Inbox's `set impact and size first · n skips` and `triaged · ⏎ next card`.
- Migration 006 never touches `priority` and never deletes anything; it runs once (schema_migrations version 6).
- Every task ends green on: `gofmt -l .` (no output), `go vet ./...`, `go test ./...`; Tasks 3 and 4 also `go test -race ./internal/tui/ ./internal/app/`.
- Every commit message ends with a blank line and `Co-Authored-By: Claude <noreply@anthropic.com>`. Commit messages use single quotes.
- Sandbox: save each patch to a file and `git apply <file>`; run plain, separate git commands (no `git -C`, no `cd … &&`). If `git apply` fails, stop: an earlier step went wrong.

## Rulings made while drafting (in the code; each a one-line reversal)

- The steps' wording lives in `domain` (`ImpactNames`, `ImpactExamples`); the Inbox card reads `[1] a person · me, one reviewer, one asker`, `[2] one team · my team, or one other team`, `[3] more than one team · several teams, a stream` under `who notices if it slips?`.
- The TUI computes urgency against `svc.Today()` (always a valid day), not the loaded plan's day.
- The due marker goes on list rows and Task detail's children, not on timeline labels (kept short); closed tasks show none.
- `SetImpact` refuses a child itself (not only the parsers), so no path stores a child's own impact.
- Weekdays are their three-letter names only: `^friday` stays in the title, like any other `^word`. `^Nd`/`^Nw` go up to 9999.
- `[github] due` takes whole days `Nd` (0d–365d) or `""`; `Config.ReviewDue` is -1 for no due. The review's due counts from the planning day of the request (`SourceItem.WaitingSince`), in the clock's zone; migration 006 uses the request's UTC date, a one-off.
- `g` says `sorted by urgency` / `sorted by impact` for the flat sorts and `grouped by …` for the rest; the help reads `sort by urgency → impact, group by size → created → requester`. The footer hint stays `g group`.
- The Inbox's `^` prompt is prefilled with the task's own due (`2026-10-16`) and takes the forms with or without the `^`.
- A held Inbox card (one a key acted on) is listed first while it is open, so the title reads `INBOX · 1 of N` until ⏎.

## Review Focus

1. **Opening the real database** (the first run after the upgrade, a database full of P-levels): it migrates once, priority stays untouched, open top-level tasks wait in the Inbox, pinned reviews keep working with impact 1 and a due. Pinned in Task 1 `TestMigration006` (open, closed, child, source-linked with and without a request time, a gone source item) and the pty run after the last task.
2. **An old `config.toml`** (made by `make config` with `[[priorities]]` and `[github] priority`): tt refuses to start with a message that says what to remove, not `unknown config key`. Pinned in Task 3 `TestPriorityKeysAreGone` and `cmd/tt` `TestConfigIsHonoured` (`tt add` exits 1 with the message).
3. **Working past midnight** (02:00 with `day_start = "04:00"`): `^tom` and `^today` count from the planning day, and urgency stays the same until the day starts. Pinned in Task 3 `TestDueDatesFollowThePlanningDay`; `DueFactor` works in whole planning days (Task 1 `TestDueFactor`).
4. **Titles that look like dates, and dates that don't exist** (`x^2`, `^_^`, a lone `^`, `^someday`, `^friday`, `^31feb`, `29feb` outside a leap year, `2jan` typed on 30 Dec): look-alikes stay in the title, impossible dates are errors, and the year rolls over. Pinned in Task 2 `TestDueWordRolloverAndErrors` and Task 3 `TestParseAdd` / `TestParseAddErrors`.
5. **An Inbox card that changes under the cursor** (dropped, closed elsewhere, or a triage step undone with ctrl+z): it never sticks on screen once it is closed, and an undone step shows it untriaged again rather than skipping it. Pinned in Task 4 `TestInboxHeldCardLeavesWhenDropped` and `TestInboxShowsTheDueFirstAndWaitsForEnter` (ctrl+z).

## File Structure

```
internal/domain/urgency.go             new: impact steps, EffectiveImpact/Due, DueFactor, Urgency, Day.AddDays (Task 1)
internal/domain/task.go, rules.go      modify: Impact, Due (1); Priority gone, InInbox per D74 (3)
internal/store/tasks.go                modify: impact and due columns (1); priority unread (3)
internal/store/migrations/006_impact_due.sql   new (1)
internal/dsl/due.go                    new: dueWord, ParseDue (2)
internal/config/config.go, config.example.toml  modify: [urgency], [github] impact/due (2); priorities gone, D76 errors (3)
internal/dsl/add.go, filter.go         modify: !n impact, ^ due (3)
internal/app/tasks.go, children.go, sources.go, day.go   modify: impact, due, SetImpact, SetDue, D75, candidate order (3)
internal/search/search.go              modify: ByUrgency, ByImpact, Sorter (3)
internal/tui/*.go                      modify: badge, Inbox impact, People order, help (3); due marker, detail line, Inbox due and ⏎ (4)
cmd/tt/main.go, README.md              modify: usage and syntax (3)
tests: internal/domain/urgency_test.go, internal/store/migration006_test.go, internal/dsl/due_test.go, internal/config/urgency_test.go, internal/tui/due_test.go new; the rest modified
```

Work happens on branch `plan-10` in a worktree.

---
"""

FOOTER = """
## After the last task

- Build both binaries into the scratchpad: the current `main` as `tt-old` and the branch as `tt-new`. Run `docs/superpowers/tooling/pty_impact.py <tt-old> <tt-new> <db>` (copy it into the scratchpad first; `<db>` a new path there). It adds three tasks and a pinned review with `tt-old`, then opens the database with `tt-new` under a pty. That checks the Inbox shows `INBOX · 1 of 3` with `— · ^ to set` first, and that `⏎` is refused before triage. It sets `^` to 2026-10-30 and impact 3 with size s, waits for `next card`, then checks `⏎` moves on and the Log shows `BACKLOG · by urgency` and `Old top →30 oct`. Finally it prints the tasks: priority unchanged, impact 3 and due 2026-10-30 on the triaged one, impact 1 and due 2026-10-15 on the review. Every line prints `OK`.
- Run the new binary with an old `config.example.toml` (from `main`) as `TT_CONFIG`: `tt add 'x !2'` exits 1 with the `[[priorities]]` message.
- `go test -race ./internal/tui/ ./internal/app/`.
- Final whole-branch review, one fix wave, park residuals as spec carry-forwards.
- **The user tries the branch, on a copy of the database:** migration 006 makes the database version 6, which `main`'s tt refuses to open ("created by a newer tt"). So: `cp ~/Library/Application\\ Support/tt/tt.db /tmp/tt-try.db`, remove `[[priorities]]` and `[github] priority` from a copy of the config, then `make build` in the worktree and `TT_DB=/tmp/tt-try.db TT_CONFIG=<copy> <worktree>/bin/tt`. Merge (fast-forward to `main`) only after the user says so; after the merge, the user edits the real config the same way, and the real database migrates on the next start.
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
