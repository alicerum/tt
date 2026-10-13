#!/usr/bin/env python3
"""Generate the Plan 5a doc from the verified scratch commits (main..HEAD).

Template for the next plan: copy it, replace TASKS (one entry per scratch commit:
title, intro, interfaces, step2/step4 text), HEADER and FOOTER.
Usage: gen_plan.py SCRATCH_REPO OUT_DOC"""
import subprocess, sys

REPO, OUT = sys.argv[1], sys.argv[2]
TRAILER = "Co-Authored-By: Claude <noreply@anthropic.com>"


def git(*args):
    return subprocess.run(["git", *args], cwd=REPO, check=True, capture_output=True, text=True).stdout


commits = git("rev-list", "--reverse", "main..HEAD").split()

TASKS = [
    dict(
        title="Service: a pinned parent pins its children (D34); `SetBlockSpan`",
        intro="D34 as amended in the spec: pin and unpin cascade to children in one transaction, a child under a pinned parent can't be unpinned alone, children joining a pinned parent are pinned, and unpinning a parent keeps children that have blocks today. Plus one fine-tune command for D36.",
        interfaces="""- Consumes: `store.Tx` (`GetTask`, `Children`, `GetPin`, `PutPin`, `DeletePin`, `TaskBlocksOn`), `Service.reshape`, `mustBeOpen`; test helpers `startedService`, `mustAdd`, `mustAddChild`, `mustGet`, `mustUndo`, `mustBlock`, `mustPlan`, `candidate`.
- Produces:
  - `var app.ErrParentPinned` ("unpin the parent first"): `Unpin` of a pinned child whose parent is pinned.
  - `Pin`/`Unpin` of a task with children return a **nil** `Undo` (D34); childless pin/unpin undo unchanged.
  - `func (*Service) SetBlockSpan(ctx, blockID int64, start, dur time.Duration) (Undo, error)`: start and length in one undoable step; same errors as `MoveBlock`/`ResizeBlock` (`ErrOverlap`, `ErrInvalidValue`).
  - `type undoStep func(ctx, *store.Tx) error`; `func (*Service) modifyAnd(ctx, id, fn func(*store.Tx, *domain.Task) (undoStep, error)) (Undo, error)`; `modify` is now a wrapper over it.
  - `AddChild`, `SetParent`, `Reopen` pin the child under a pinned parent; the undo of `SetParent`/`Reopen` removes a pin it added (unless the task got a block meanwhile); undoing `AddChild` deletes the child and its pin.""",
        step2="Run: `go test ./internal/app/`\nExpected: FAIL (build): `undefined: ErrParentPinned`, `s.SetBlockSpan undefined`.",
        step4="Run: `go test ./internal/app/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`\nExpected: PASS (the existing pin/unpin/undo tests still pass unchanged), no gofmt output.",
    ),
    dict(
        title="Timeline window config, `Day.Offset`, the timeline grid",
        intro="D38's pure parts: the configured window, the inverse of `Day.At` for \"now\", and the row model the Today screen renders (no rendering yet).",
        interfaces="""- Consumes: `config.parseClock`, `domain.Block`.
- Produces:
  - `config.Config.TimelineStart`, `TimelineEnd time.Duration` (defaults 8h, 19h; TOML `[timeline] start`, `end` as `HH:MM`). Errors: `timeline.start: …`, `timeline.end: …`, `timeline.end must be after timeline.start`, `timeline.start must not be before day_start …`. Unset keys follow a late `day_start` instead of failing.
  - `func (d domain.Day) Offset(t time.Time) time.Duration`: wall-clock offset from midnight of `d` (25h for 01:00 the next date), DST-safe.
  - `internal/tui/timeline.go`: `const rowLen = 30 * time.Minute`; `type tlRow struct{ start time.Duration; blocks []domain.Block }`; `type timeline struct{ rows []tlRow; blocks []domain.Block; nowRow int; dayEnd time.Duration }` (Task 3 adds `now`); `func buildTimeline(blocks []domain.Block, from, to, now, dayStart time.Duration) timeline` (`now < 0` = no now); methods `rowOf(at) int`, `blockAt(i) (domain.Block, bool)`, `gapFrom(at) time.Duration`, `firstFree(i) int`, `clash(b) (domain.Block, bool)`.""",
        step2="Run: `go test ./internal/config/ ./internal/domain/ ./internal/tui/`\nExpected: FAIL (build): `cfg.TimelineStart undefined`, `day.Offset undefined`, `undefined: buildTimeline`.",
        step4="Run: `go test ./internal/config/ ./internal/domain/ ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`\nExpected: PASS, no gofmt output.",
    ),
    dict(
        title="Today is the home screen; Pick becomes the Log; shared height budget",
        intro="D35 and D38's rendering: tt opens on Today (timeline beside the candidates), `L`/`esc`/`B` move between Today and the Log, Inbox and Task detail return to where they were opened, D34's statuses, and one height budget for every screen (Plan 3 carry-forward). Placement comes in Task 4. The test harness now checks that tt opens on Today; most existing tests switch to `newLogHarness`, and \"tt · Pick\" becomes \"tt · Log\".",
        interfaces="""- Consumes: Task 1 (`Pin`/`Unpin` nil undo for parents), Task 2 (`buildTimeline`, `Day.Offset`, `Config.TimelineStart/End`).
- Produces:
  - `screenToday` (the zero value, first in the `screen` consts); `screenPick` renamed `screenLog`.
  - `todayState{focus todayFocus; tl timeline; at time.Duration; placed bool; cursor, offset int}`; `focusTimeline`, `focusCandidates`; `(*todayState).refresh(m)`, `move(delta)`, `follow(m)`.
  - `(*Model).todayKey(key) tea.Cmd`, `todayView() string`, `timelineLines(width) []string`, `timelineRow(i, width) string`, `blockLabel(b) string`, `todayHints() string`; `var todayHelp []string`; `func clock(off time.Duration) string` ("09:30", wraps past 24h).
  - `(*Model).openInbox()`; `inboxState.from`, `detailState.from` (esc returns there).
  - `(*Model).bodyHeight() int` (header, status, hints, banner, non-help overlay subtracted; floor 2), `visibleRows()` = `bodyHeight()-1` (floor 1), `clipLines(s, n)`, `plusChildren(n)`.
  - `(*pickState).moveIn(pn, delta)`, `(*Model).selectedIn(pn) (row, bool)`, `columns(left, right []string, lw, rw int) string`; `paneLines` takes a `focused bool`.
  - `timeline.now` field (now's offset, -1 for none).
  - Test helpers: `newLogHarness(t, cfg)`, `(*harness).block(id, start, dur) domain.Block`.""",
        step2="Run: `go test ./internal/tui/`\nExpected: FAIL (build): `undefined: screenToday`, `h.m.today undefined`, `undefined: focusCandidates`.",
        step4="Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`\nExpected: PASS, no gofmt output.",
    ),
    dict(
        title="Pick up and drop: place, move, resize, fine-tune, unblock, nudges, commit",
        intro="D36, D37 and commit on Today: `b` picks a candidate up as a ghost of D24 length, `m` picks a block up, `⏎` drops (refused onto another block, the pick-up stays), `esc` cancels; `+/-`, `f` (one-line fine-tune with validation that keeps the text), `u` unblock; the L/XL and parent nudges; `c` commits with the status-line summary.",
        interfaces="""- Consumes: Task 1 (`SetBlockSpan`), Tasks 2–3 (`timeline` methods, `todayState`, `clock`, `blockKeys`), `Service.Block`/`MoveBlock`/`ResizeBlock`/`Unblock`/`CommitPlan`/`DefaultBlockLength`.
- Produces:
  - `todayState.carry *carry`; `type carry struct{ task domain.Task; block domain.Block }` (block ID 0 = placing a candidate).
  - `(*Model).pickUp(t)`, `ghost() domain.Block`, `fits(b) error` (`overlaps "Title" 10:00–11:00` / `runs past the end of the planning day`), `blockName(b)`, `drop() tea.Cmd`, `carryKey(key)`, `reshape(old, t, b) tea.Cmd`, `openFineTune(b, t) tea.Cmd`, `commitPlan() tea.Cmd`, `hasOpenChildren(t) bool`; `func span(b) string`; `func parseSpan(s string, dayStart time.Duration) (start, dur time.Duration, err error)`.
  - `prompt.check func(string) error` (enter refused while it fails; the error shows under the input); `type nudge struct{ question string; onEnter func() tea.Cmd }` (⏎ goes ahead, any other key cancels).
  - Statuses: `blocked "X" 09:30–10:30[ · big task · consider splitting]`, `moved "X" to 11:00–12:00`, `"X" now 10:00–11:30`, `unblocked "X" · still a candidate`, `plan committed · 1 block · 1h30m · 1 candidate left` (`· no blocks ·` when none), `stopped moving "X": its block is gone`.
  - Test helpers: `(*harness).pinned(line) domain.Task`, `(*harness).blocks() []domain.Block`.""",
        step2="Run: `go test ./internal/tui/`\nExpected: FAIL (build): `h.m.today.carry undefined`, `undefined: parseSpan`.",
        step4="Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`\nExpected: PASS, no gofmt output.",
    ),
]
assert len(TASKS) == len(commits), (len(TASKS), len(commits))

HEADER = """# tt Plan 5a — Today & Blocking Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** tt opens on Today: a timeline of today's blocks beside the candidates, where `b` picks a candidate up, `j/k` move a ghost block and `⏎` drops it, blocks can be moved, resized, fine-tuned and unblocked, and `c` commits the plan; Pick becomes the Log (`L`, `esc`, `B`), and a pinned parent pins its children (D34).

**Architecture:** D34 and `SetBlockSpan` stay in `app` (one transaction each; undo steps through a new `modifyAnd`). The TUI gets a `screenToday` home screen whose timeline is a pure row model (`internal/tui/timeline.go`, rebuilt on every load like the Log's rows) rendered beside the Log's candidates pane, which it shares. Placement is a small "carry" state on Today (a ghost block that follows the cursor until dropped). All screens share one height budget (`bodyHeight`). No calendar code: Plan 5b adds meetings to the same timeline.

**Tech Stack:** Go 1.27; Bubble Tea v2, bubbles v2 textinput, Lip Gloss v2; pure-Go SQLite. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-10-12-task-tracking-design.md` — D12a, D18 (5b), D22, D24, D31, D34 (amended), D35–D38, open items P3/P4, "Plan 5 split" → Plan 5a, "Carry-forward from Plan 2 review" (DST rows), "Carry-forward from Plan 3 review" (shared height budget), "Carry-forward from Plan 4b review" (D34).

## Global Constraints

- No new dependencies: `go.mod` and `go.sum` stay unchanged. No migration in 5a.
- Only `app` mutates state; `internal/tui` and `cmd/tt` call `app.Service`. Every user command returns an undo, except pin/unpin of a task with children (D34, the status says `· no undo`), commit, and hard delete. Undo refuses with `ErrConflict` rather than damaging state.
- D34: unpinning a parent never deletes a child's block: children with a block today stay pinned on their own.
- D36: outside a pick-up, `⏎` opens Task detail (D31). A drop onto another block is refused and the block stays picked up; placing in the past is allowed.
- D37 wording, exactly: `big task · consider splitting` (L and XL only) and `parent task; blocking its children is recommended · ⏎ block anyway · esc`.
- D38: rows are 30 minutes and wall-clock offsets (no DST special case); the window is `[timeline] start`–`end`, stretched to every block and now.
- Lines never wrap: every rendered line is truncated to the terminal width, and a screen's body never exceeds `bodyHeight()`.
- Every task ends green on: `gofmt -l .` (no output), `go vet ./...`, `go test ./...`; Tasks 3–4 also `go test -race ./internal/tui/ ./internal/app/`.
- Every commit message ends with a blank line and `Co-Authored-By: Claude <noreply@anthropic.com>`. Commit messages use single quotes.
- Sandbox: save each patch to a file and `git apply <file>`; run plain, separate git commands (no `git -C`, no `cd … &&`). If `git apply` fails, stop: an earlier step went wrong.

## Review Focus

1. **Late-night work or an early start** (a block at 21:00, now at 06:40): the timeline stretches to show them; nothing is cut off or unplaceable. Pinned in Task 3 `TestTimelineStretchesToBlocksAndNow` (and Task 2 `TestTimelineWindow`).
2. **Dropping, moving, resizing or fine-tuning onto another block**: refused with `overlaps "X" 10:00–11:00`, the pick-up or the typed line stays, nothing is written. Pinned in Task 4 `TestDropOntoABlockIsRefused` and `TestMoveResizeFineTuneUnblock`.
3. **The picked-up block changes elsewhere** (undo, another `tt`, a reload) while it follows the cursor: the move stops and says why, nothing acts on a stale block. Pinned in Task 4 `TestAMovedBlockThatVanishesStopsTheMove`.
4. **Unpinning a parent whose child is blocked today**: the child's block and pin survive; the status says so and that there's no undo. Pinned in Task 1 `TestUnpinningAParentKeepsBlockedChildren` and Task 3 `TestPinningAParentInTheTUIWithChildren`.
5. **A short terminal (tmux split) or a DST night**: no screen grows past the terminal, the timeline scrolls with its cursor, and rows stay wall-clock labels. Pinned in Task 3 `TestScreensShareTheHeightBudget` and `TestTimelineRowsAcrossADSTGap` (and Task 2 `TestDayOffset`).

## File Structure

```
internal/app/planning.go, pins_test.go      modify/new: D34 in Pin/Unpin/Block, ErrParentPinned, SetBlockSpan, pin helpers
internal/app/service.go                     modify: undoStep, modifyAnd (modify wraps it)
internal/app/tasks.go, children.go          modify: SetParent/Reopen/AddChild pin under a pinned parent
internal/config/config.go, timeline_test.go modify/new: [timeline] start/end
internal/domain/plan.go, plan_test.go       modify: Day.Offset
internal/tui/timeline.go, timeline_test.go  new: the row model (pure)
internal/tui/today.go, today_test.go        new: Today screen, carry/drop, block keys, fine-tune, commit
internal/tui/place_test.go                  new: placement flows
internal/tui/model.go                       modify: screenToday first, screenLog, today state, routing, clipped bodies
internal/tui/pick.go                        modify: L/B/esc, moveIn, selectedIn, columns, D34 statuses, visibleRows
internal/tui/inbox.go, detail.go            modify: return to the screen they came from; childRows on the budget
internal/tui/common.go, overlays.go         modify: header/hints, bodyHeight, clipLines, help, prompt.check, nudge
internal/tui/*_test.go                      modify: newLogHarness, "tt · Log"
```

Work happens on branch `plan-5a` in a worktree; fast-forward merge to `main` when the final review is green.

---
"""

FOOTER = """
## After the last task

- Build into the scratchpad and drive the real binary through a Python `pty` script on a throwaway database (`TT_DB=… TT_CONFIG=/nonexistent`): it opens on `tt · Today` with the timeline and the now line; `esc` (banner), `L` → Log, `a` + `Alpha !1 ~s` + `⏎`, `p`, `B` → Today, `b` → `placing`, `⏎` → `blocked`, `c` → `plan committed`, `q`; then the database has one block for today and `days.committed_at` set.
- `go test -race ./internal/tui/ ./internal/app/`.
- Final whole-branch review, one fix wave, park residuals as spec carry-forwards.
"""


def fence(lang, body):
    return f"```{lang}\n{body.rstrip(chr(10))}\n```\n"


def files_of(c):
    out = []
    for line in git("diff", "--name-status", f"{c}^", c).splitlines():
        st, path = line.split("\t", 1)
        out.append((st, path))
    return out


def block_for(c, st, path):
    if st == "A":
        lang = "go" if path.endswith(".go") else ""
        return f"Create `{path}`:\n\n" + fence(lang, git("show", f"{c}:{path}"))
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
    doc.append("- [ ] **Step 1: Write the failing tests**\n\n")
    for s, p in tests:
        doc.append(block_for(c, s, p) + "\n")
    doc.append(f"- [ ] **Step 2: Run the tests to verify they fail**\n\n{task['step2']}\n\n")
    doc.append("- [ ] **Step 3: Implement**\n\n")
    for s, p in impl:
        doc.append(block_for(c, s, p) + "\n")
    doc.append(f"- [ ] **Step 4: Run the tests to verify they pass**\n\n{task['step4']}\n\n")
    paths = " ".join(p for _, p in fs)
    assert "'" not in subject
    doc.append("- [ ] **Step 5: Commit**\n\n" + fence("bash", f"git add {paths}\ngit commit -m '{subject}\n\n{TRAILER}'") + "\n---\n\n")
doc.append(FOOTER)
open(OUT, "w").write("".join(doc))
print("wrote", OUT)
