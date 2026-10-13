#!/usr/bin/env python3
"""Generate the Plan 8 doc from the verified scratch commits (main..HEAD).

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
        title="Frame, `within` and `cutHints`: the pure drawing helpers",
        intro="Three small helpers in a new `internal/tui/frame.go`, tested on their own before any screen uses them. `frame` draws a rounded box with its title in the top border (accent colour ANSI 4 when focused, dim otherwise), every line padded or cut to the inner width, exactly `height` lines. `within` renders a line in an outer style and re-applies that style after every reset inside it, so a struck-through or coloured part no longer ends a selected or dimmed row's style early. It builds the escape sequence by hand, because Lip Gloss restyles a string that already holds escapes one character at a time. `cutHints` fits a hints line: whole hints are dropped at a ` · ` boundary, `…` marks the cut, and `? help`, when present, stays at the right edge. Nothing on screen changes in this task.",
        interfaces="""- Consumes: existing `pad`, `styleTitle`, `styleDim`, `styleSelected`, `styleClosed` (common.go).
- Produces (internal/tui/frame.go):
  - `styleAccent` (ANSI 4) and `styleHeader` (reverse video) styles.
  - `func frame(title string, lines []string, width, height int, focused bool) []string`: nil when `width < 4` or `height < 2`; rows are `│ ` + line padded to `width-3` + `│`.
  - `func within(outer lipgloss.Style, s string) string`.
  - `func cutHints(hints string, width int) string`.""",
        step2="Run: `go test ./internal/tui/ -run 'Frame|Within|CutHints'`\nExpected: FAIL (build): `undefined: frame`, `undefined: within`, `undefined: cutHints`.",
        step4="Run: `go test ./internal/tui/ -run 'Frame|Within|CutHints'`, then `gofmt -l .`, `go vet ./...`, `go test ./...`\nExpected: PASS, no gofmt output.",
    ),
    dict(
        title="Every pane in a frame; the header bar, the footer, the height budget",
        intro="D55 across all five screens. `frame.go` gains the layout methods: `framed` (frames only when the body has room for a frame and a row), `chrome`, `inner`, `box` (a frame, or the unframed fallback: title line plus rows) and `beside` (two columns, no `│` divider). Today, the Log and People draw each pane through `box`, and the Reviews lane is a frame of its own whose borders replace its old blank and title lines. Inbox and Task detail get one frame each: `INBOX · 1 of 3` and `TASK`. `inboxView` now returns its title and lines, and `detailView` its lines. `visibleRows` is the body less the frame (one row fewer than before: the title moved into the border), and `paneBox` renders a pane at its column's height. The header is a full-width reverse-video bar, built with `within` so its dim and yellow parts don't end it. The footer's hints go through `cutHints` (`hintsView` → `hintsText`) and list every key again: Task detail `u unpin · D delete · ctrl+z undo`, the Log `E note · o link`, the timeline `a add · q quit`. `bodyHeight` can go down to 1, and `render` clips to the terminal's height, so the screen is never taller than the terminal. Existing tests that pinned row counts or line starts are updated for the frames: `│ ` prefixes, one row fewer per pane. Their behaviour is unchanged.",
        interfaces="""- Consumes: Task 1 (`frame`, `within`, `cutHints`, `styleHeader`).
- Produces:
  - `func (*Model) framed() bool`, `chrome() int` (2 framed, 1 not), `inner(width int) int` (`width-3` framed), `box(title string, lines []string, width, height int, focused bool) []string`; `func beside(left, right []string) string`.
  - `func (*Model) paneBox(pn pane, title, empty string, width int, focused bool) []string` (Task 3 adds an `above []string` parameter after `empty`); `listLines(l rowList, visible int, empty string, width int, focused bool) []string` (rows only, no title).
  - `func (*Model) timelineTitle() string`; `timelineLines(width int) []string` (rows only).
  - `func (*Model) inboxView() (string, []string)`; `detailView() []string`; `hintsText() string` (hintsView cuts it).
  - Test helpers (layout_test.go): `busyHarness(t)`, `fits(t, h, what, width, height)`, `accentOn()`.""",
        step2="Run: `go test ./internal/tui/`\nExpected: FAIL: the new `TestEveryScreenFitsTheTerminal` (`Today at 60x4: 5 lines`), `TestTheFocusedPaneIsTheAccentFrame`, `TestHeaderBarAndFooter`, `TestShortTerminalDropsFramesFirst`, and the updated `TestReviewsLaneOnToday`, `TestReviewsLaneInTheLog`, `TestReviewsLaneHeight`, `TestPeopleWithNobody`, `TestGroupingCyclesAndIsRemembered`, `TestScrollWindowMovesOnlyWhenTheCursorLeavesIt`, `TestScrollKeepsTheCursorWhenRowsAppearAbove`, `TestNowLineSitsAboveItsRow` (they expect frames).",
        step4="Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`\nExpected: PASS, no gofmt output.",
    ),
    dict(
        title="The applied filter keeps its own row (Log and People)",
        intro="D56. The first row inside the Log's backlog frame and inside People's names frame is the filter. While you type, it is the input (with its error and suggestion lines). Once applied, it is a dim `filter › #photos schema · esc clears`. Cleared, the row is gone. The filter overlays implement `paneOverlay`, so they take no height from the body and `render` no longer draws them above or below the screen. Their inputs are sized to their column (`backlogWidth`, `namesWidth`). Pane titles keep the count without the filter text (`BACKLOG · by priority · 3 matches`, `PEOPLE · 2 matches`). The empty text becomes `no matches`, and People's filter prompt becomes `filter › `. `paneVisible` and People's `namesVisible` take the filter rows off the list. Tests that pinned the old title text are updated.",
        interfaces="""- Consumes: Task 2 (`box`, `paneBox`, `listLines`, `inner`, `visibleRows`).
- Produces: `type paneOverlay interface{ inPane() }` (implemented by `*filterBar`, `*peopleFilter`); `func (*Model) filterLines() []string`; `func backlogWidth(width int) int` (`width*55/100`); `func (*Model) peopleFilterLines() []string`; `func (*Model) namesVisible() int`; `func namesWidth(width int) int` (`width/3`); `paneBox(pn pane, title, empty string, above []string, width int, focused bool)`; test helper `(*harness).lineOf(s string) int`.""",
        step2="Run: `go test ./internal/tui/`\nExpected: FAIL (build): `undefined: backlogWidth` (filter_test.go).",
        step4="Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`\nExpected: PASS, no gofmt output.",
    ),
    dict(
        title="Tab skips an empty pane; a reload that empties the focused pane moves the focus",
        intro="D57. In the Log, `tab` into an empty pane stays put and says `nothing pinned yet · p pins a task`, `backlog is empty`, or `no matches · esc clears` (`emptyPane`). On Today, `tab` from the timeline to empty candidates says `nothing pinned · L opens the log to pick`. In People, `tab` with nobody listed says `nobody listed`. After a load, `refocus` moves the focus off a pane of the screen on show that this reload emptied (it had rows before the reload, has none now) to the other pane when that one has rows: the Log's two panes, Today's candidates back to the timeline, People's tasks back to the names. It does nothing while a filter is being typed, and never for a screen that isn't on show. One lane test pins a candidate first, since `tab` into empty candidates is now refused.",
        interfaces="""- Consumes: existing `pick.rows`, `today.focus`, `people.onTasks`, `people.tasks`.
- Produces: `func (*Model) emptyPane(pn pane) string`; `type paneSizes struct{ log [2]int; tasks int }`, `func (*Model) paneSizes() paneSizes`, `func (*Model) refocus(had paneSizes)` (called from the `loadedMsg` case).""",
        step2="Run: `go test ./internal/tui/ -run 'TabSkips|Refocus'`\nExpected: FAIL: `TestTabSkipsAnEmptyPaneInTheLog` (`screen lacks \"nothing pinned yet · p pins a task\"`), `TestTabSkipsAnEmptyPaneOnToday`, `TestTabSkipsAnEmptyPaneInPeople`; `TestARefocusLeavesScreensNotOnShowAlone` already passes (it guards against over-eager refocusing).",
        step4="Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`\nExpected: PASS, no gofmt output.",
    ),
    dict(
        title="Blocks coloured per task; meetings dim; highlights run to the end of the row",
        intro="D58. A block's `▓` (on its first row and on the rows it carries on into) and its label take one of `blockColours` (ANSI 2, 4, 5, 6, 12, 13), chosen by task id mod 6 through `blockStyle`. A closed (or missing) task's block is dim, and its label stays struck through without colour. A conflicted block's ` ⚠` is yellow (`styleAlert`), outside the colour. Meeting parts render dim. Every style laid over a whole row now goes through `within`: the timeline's past and cursor rows, the outside-hours rows, the ghost rows, the Log's closed and selected rows, Task detail's head and child rows, the Reviews lane's cursor row and People's cursor row. So the highlight runs to the row's end over a coloured, struck-through or dim part (the Plan 5a nested-styles carry-forward).",
        interfaces="""- Consumes: Task 1 (`within`), existing `blockText`, `meetingParts`, `timelineRow`, `ghostRow`, `rowLine`, `childLine`, `reviewLine`, `personLine`.
- Produces: `var blockColours = []string{"2", "4", "5", "6", "12", "13"}`; `func (*Model) blockStyle(b domain.Block) lipgloss.Style`; test helpers (colour_test.go) `on(st lipgloss.Style) string`, `(*harness).rawLine(s string) string`, `colour(i int) string`, `reversedToTheEnd(t, what, line string)`.""",
        step2="Run: `go test ./internal/tui/ -run 'Coloured|OwnStyles|Highlight'`\nExpected: FAIL (build): `undefined: blockColours`.",
        step4="Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`\nExpected: PASS, no gofmt output.",
    ),
]
assert len(TASKS) == len(commits), (len(TASKS), len(commits))

HEADER = """# tt Plan 8 — UI Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every pane sits in a rounded frame with its title in the border and the focused one in the accent colour; a header bar and a footer whose hints never lose `? help`; an applied filter keeps its own visible row; `tab` never lands on an empty pane; timeline blocks are coloured per task, with highlights that run to the end of the row.

**Architecture:** All drawing stays in `internal/tui`. A hand-drawn `frame` helper (Lip Gloss borders can't hold a title, and the width math already lives in `pad`/`ansi.Truncate`) is wrapped by `Model.box`, which every screen renders its panes through; `bodyHeight()` stays the single height budget with each frame inside it, and a short terminal drops the frames. `within` fixes nested styles once for every row style. No service, store or schema change.

**Tech Stack:** Go 1.27; Bubble Tea v2, Lip Gloss v2, Bubbles v2 textinput, `charmbracelet/x/ansi`. No new Go dependencies.

**Spec:** `docs/superpowers/specs/2026-10-12-task-tracking-design.md` — D55, D56, D57, D58, section "Plan 8 — UI polish", and the carry-forwards it names (Plan 4a hints overflow, Plan 4b Task detail hints, Plan 5a timeline hints, nested styles and height ≤ 4, Plan 7 empty tasks pane).

## Global Constraints

- No new Go dependencies: `go.mod` and `go.sum` stay unchanged. Only `internal/tui` changes; no service, store, config or schema change.
- Terminal ANSI colours only: accent ANSI 4 for the focused frame; block colours exactly `2, 4, 5, 6, 12, 13` by task id mod 6; never yellow (3) or red (1) for blocks; ⚠ stays yellow, errors red, linked reviews cyan, the banner green.
- Lines never wrap and the screen is never taller than the terminal: every rendered line is cut to the width, and `render` clips to the height; a screen's body never exceeds `bodyHeight()`.
- D55/D56/D57 wording, exactly: titles in the top border (`╭ TIMELINE · 2/5 done ─╮`), `INBOX · 1 of 3`, `TASK`, `BACKLOG · by priority · 3 matches`, `PEOPLE · 2 matches`; the filter row `filter › <text> · esc clears`; footer `…` at a ` · ` boundary with `? help` at the right edge; statuses `nothing pinned yet · p pins a task`, `backlog is empty`, `no matches · esc clears`, `nothing pinned · L opens the log to pick`, `nobody listed`.
- Behaviour other than layout stays as it was: keys, statuses, undo, reload, placement. Existing tests change only where they pinned the old layout (row counts, line prefixes, the filter in the title).
- Every task ends green on: `gofmt -l .` (no output), `go vet ./...`, `go test ./...`; Tasks 2–5 also `go test -race ./internal/tui/ ./internal/app/`.
- Every commit message ends with a blank line and `Co-Authored-By: Claude <noreply@anthropic.com>`. Commit messages use single quotes.
- Sandbox: save each patch to a file and `git apply <file>`; run plain, separate git commands (no `git -C`, no `cd … &&`). If `git apply` fails, stop: an earlier step went wrong.

## Rulings made while drafting (in the code; each a one-line reversal)

- A frame's rows are `│ ` + text + `│` (one space on the left), so a frame costs 3 columns and 2 rows; its title moves into the border, so lists show one row fewer than before.
- Task detail's frame is titled `TASK`; the Inbox card's title line moves into its border and the blank line under it goes. People's tasks frame says `TASKS` when nobody is selected.
- Frames need a body of at least 3 lines; below that each pane is its title line and rows (no divider). The screen is clipped to the terminal's height, so at 3 lines or fewer the footer goes first.
- The empty text of a filter that matches nothing is `no matches` (the filter row already says `esc clears`); People's empty text is `nobody yet · @who names who asked` (fits its column); People's filter prompt is `filter › ` like the Log's.
- `refocus` acts only on the screen on show, only for a pane this reload emptied (a pane that was always empty doesn't move the focus), and never while a filter is being typed. `B` from the Log still focuses Today's candidates, as before.
- Meetings render dim (the backlog's "one neutral, dim style").

## Review Focus

1. **Real widths with wide characters** (📌, ✎, ⏎, →/← in rows and hints; a long title in a narrow column; 40 to 140 columns): every line is exactly its column's width, frames stay closed on the right, and the footer keeps `? help`. Pinned in Task 1 `TestFrame`, `TestCutHints` (every width 12–160), Task 2 `TestEveryScreenFitsTheTerminal`, `TestHeaderBarAndFooter`.
2. **A tiny or live-resized tmux pane** (8, 5, 4, 3, 2, 1 lines; the banner or an overlay open at the same time): the screen is never taller than the terminal, frames go first, nothing panics. Pinned in Task 2 `TestEveryScreenFitsTheTerminal`, `TestShortTerminalDropsFramesFirst`.
3. **Typing a filter inside the frame** (a parse error line, completion suggestions, zero matches, pasting): the rows below don't jump on `⏎` or `esc`, the list's cursor stays in view, and the input never spills out of its column. Pinned in Task 3 `TestTheAppliedFilterStaysVisibleInTheLog`, `TestTheAppliedFilterStaysVisibleInPeople`, the updated `TestFilterBarNarrowsLiveAndKeepsTypingOnError`, `TestOverlayInputFollowsResize`, Task 2's sweep ("Log, typing a filter").
4. **The focused pane emptied from elsewhere** (the last candidate unpinned in another shell, a person's last task deleted, the backlog emptied while Today is on show, a filter typed down to nothing): the focus lands on a pane with rows on the screen on show and never moves a hidden screen's focus. Pinned in Task 4 `TestTabSkipsAnEmptyPaneInTheLog`, `TestTabSkipsAnEmptyPaneOnToday`, `TestTabSkipsAnEmptyPaneInPeople`, `TestARefocusLeavesScreensNotOnShowAlone`.
5. **Styles layered over styles** (the cursor on a coloured block, on a struck-through closed block, on a Log row with a dim `↳ #parent`, a past row with a block, a ⚠ block): the highlight or dimming runs to the end of the row, ⚠ stays yellow, closed blocks show no colour. Pinned in Task 1 `TestWithinKeepsTheOuterStyleOverInnerStyles`, Task 5 `TestBlocksAreColouredPerTask`, `TestConflictsAndMeetingsKeepTheirOwnStyles`, `TestTheHighlightRunsToTheEndOfTheRow`.

## File Structure

```
internal/tui/frame.go           new: frame, within, cutHints (Task 1); framed/chrome/inner/box/beside (Task 2)
internal/tui/model.go           modify: header bar, inbox/detail boxes, clip to height (2); in-pane overlays (3); refocus (4)
internal/tui/common.go          modify: hintsView/hintsText, fuller hints, bodyHeight floor (2); in-pane overlays (3)
internal/tui/pick.go            modify: visibleRows, paneBox, listLines (2); filterLines, backlogWidth (3); tab, emptyPane (4); within (5)
internal/tui/today.go           modify: framed timeline, timelineTitle (2); tab (4); block colours, dim meetings, within (5)
internal/tui/lane.go            modify: the lane's own frame (2); within (5)
internal/tui/people.go          modify: framed panes (2); filter row (3); tab (4); within (5)
internal/tui/inbox.go, detail.go modify: framed bodies (2); within (5)
internal/tui/overlays.go        modify: paneOverlay, filter input width (3)
internal/tui/*_test.go          new: frame, layout, filterline, tab, colour; existing tests follow the new layout
```

Work happens on branch `plan-8` in a worktree. **The user tries the branch build on real data before it merges** (see "After the last task").

---
"""

FOOTER = """
## After the last task

- Build tt into the scratchpad and drive the real binary through a Python `pty` script on a throwaway database (`TT_DB`, `TT_CONFIG=/nonexistent`, an 80×24 window). Seed it with `tt add`: `Fix CI @ana !1 ~s`, `Write the schema RFC @ana !2 ~m`, `Deploy fix !2 ~s`. Then: `esc` (banner); `L` → `BACKLOG`, `TODAY'S CANDIDATES`; `p` → `pinned`; `/`, `schema` → `schema`, `1 match`; `⏎` → `esc clears`; `esc` → `filter cleared`; `B` → `Today`; `b`, `⏎` → `blocked`; resize to 80×10 (TIOCSWINSZ + SIGWINCH) → `TIMELINE`; to 80×4 (no crash); back to 80×24 → `TIMELINE`, `CANDIDATES`; `P` → `PEOPLE`, `ana`; `q`. The database has one block for task 1 and task 1 pinned. Match short substrings (the renderer repaints changed cells only, so text that stays put, like `? help`, may not be re-sent).
- `go test -race ./internal/tui/ ./internal/app/`.
- Final whole-branch review, one fix wave, park residuals as spec carry-forwards.
- **The user tries the branch before anything merges:** `make build` in the worktree, then run `<worktree>/bin/tt` in the usual tmux pane (it opens the real database; no migration in this plan). Look at Today, the Log with a filter applied, People, the Inbox and Task detail, a narrow and a short pane. Merge (fast-forward to `main`) only after the user says so; fold any "change this" into a small reviewed commit on the branch first.
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
