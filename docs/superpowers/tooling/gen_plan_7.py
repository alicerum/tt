#!/usr/bin/env python3
"""Generate the Plan 7 doc from the verified scratch commits (main..HEAD).

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
        title="Rename and merge in `app`: `dsl.ParseRequester`, `Tx.TasksWithRequester`, `RenameRequester`",
        intro="The data side of D53. `dsl.ParseRequester` normalizes a typed name exactly as the `@who` token does (trimmed, one leading `@` and trailing punctuation dropped, lowercased) and refuses anything but one non-empty word; a test pins that it agrees with `ParseAdd`. `store.Tx.TasksWithRequester` lists every task naming a requester, open or closed, children included. `app.RenameRequester` sets the new name and `updated_at = now` on all of them in one transaction; a name that already exists simply merges. Its undo restores exactly the tasks it changed (never the merge target's own) and refuses as a whole with `ErrConflict` once any of them changed or went away (integrity over undo). Renaming the empty name is refused, so tasks without a requester can never gain one this way.",
        interfaces="""- Consumes: `store.Tx.GetTask`/`UpdateTask`, `unchangedSince`, `ErrInvalidValue`, `ErrNotFound`, `ErrConflict` (existing); test helpers `newService`, `mustAdd`, `mustGet`, `mustAddChild` (app), `openTemp`, `sampleTask`, `insert`, `inTx` (store).
- Produces:
  - `func dsl.ParseRequester(s string) (string, error)`; `dsl.ErrEmptyName` (`a name can't be empty`), `dsl.ErrNameWords` (`a name is one word, like @ana`).
  - `func (*store.Tx) TasksWithRequester(ctx, name string) ([]int64, error)`: ids in id order, exact match.
  - `func (*app.Service) RenameRequester(ctx, from, to string) (n int, undo Undo, err error)`: `ErrInvalidValue` for `from == ""` or a bad `to` (the dsl error wrapped: `a name is one word, like @ana: invalid value`); `ErrNotFound` (`no task has @raf any more`) when no task carries `from`; `to` equal to `from` after normalizing → `0, nil, nil`.""",
        step2="Run: `go test ./internal/dsl/ ./internal/store/ ./internal/app/`\nExpected: FAIL (build): `undefined: ParseRequester`, `undefined: ErrEmptyName`, `undefined: ErrNameWords`; `x.TasksWithRequester undefined (type *Tx has no field or method TasksWithRequester)`; `s.RenameRequester undefined (type *Service has no field or method RenameRequester)`.",
        step4="Run: `go test ./internal/dsl/ ./internal/store/ ./internal/app/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/app/`\nExpected: PASS, no gofmt output.",
    ),
    dict(
        title="The People view: `P`, the names and a person's tasks, the `/` filter, task keys",
        intro="D52 in the TUI (`people.go`). `P` on Today or in the Log opens `tt · People`; `esc` returns to the screen it came from (also after a Task detail round-trip). Left, `PEOPLE · N`: every requester some task carries, alphabetical, `ana · 3 open`, dimmed at `0 open`. Right, `@ana`: the person's open tasks grouped by priority like the backlog (oldest first; pinned ones marked 📌; a child is its own row with `↳ #parent`), then a `closed` heading with done and dropped tasks, newest first, with their close date. `tab` switches panes (`⏎` on a person too); `j/k`, `ctrl+d/u` move; in the tasks pane `⏎ p u x d D e E o` go through the existing `taskKey`. `/` filters names live with `search.MatchName` (the Log's anchored-subsequence matcher, a leading `@` ignored): `enter` keeps it, `esc` in the line clears it, `esc` on a filtered list clears it before leaving. Each pane keeps its row across reloads (the nearest row when the person is gone). To share the Log's list machinery, `pick.go`'s cursor clamp, step, scroll and pane render move into small functions (`clampRows`, `stepRows`, `headed`, `scroll`, `listLines`) and a `rowList` type; the Log behaves as before. Hints and help gain `P people`.",
        interfaces="""- Consumes: Task 1 (`Service.RenameRequester`, in a test only); existing `rowLine`, `taskKey`, `openDetail`, `steps`, `repeat`, `columns`, `pad`, `plural`, `dayLabel`, `newInput`, `fitInput`, `visibleRows`, `search.GroupOf`/`Compare`/`Oldest`; test helpers `newHarness`, `newGHHarness`, `threeReviews`, `h.add`, `h.keys`, `h.typeText`, `h.mustSee`, `h.mustNotSee`, `h.task`, `h.screen`.
- Produces:
  - `func search.MatchName(query, name string) bool`.
  - `screenPeople`; `person{name string; open int}`; `index.people []person` (alphabetical); `Model.people peopleState`.
  - `peopleState{from screen; onTasks bool; filter string; names []person; cursor, offset int; selected string; tasks rowList}` with `refresh(m)`, `setFilter(m, text)`, `move(m, delta)`, `follow(m)`.
  - `rowList{rows []row; cursor, offset int; selected int64}` with `clamp()`, `move(delta)`, `follow(visible)`, `selectedRow() (row, bool)`; `clampRows(rows, cursor, id) (int, int64)`, `stepRows(rows, c, delta) (int, bool)`, `headed(rows, c) int`, `scroll(top, c, off, visible, n) int`; `(*Model).listLines(l rowList, visible int, title, empty string, width int, focused bool) []string`; `(*Model).priorityHeadings() []string`; `row.person bool` (no `@who`, 📌 when pinned, the close date).
  - `(*Model).openPeople()`, `personRows(name) []row`, `peopleKey(key) tea.Cmd`, `peopleView() string`, `personLine(pr person, width int, cursor bool) string`, `peopleHints() string`, `peopleHelp`; `peopleFilter` and `openPeopleFilter()`.
  - Test helper `(*harness).before(parts ...string)`: the screen shows parts in this order.""",
        step2="Run: `go test ./internal/search/ ./internal/tui/`\nExpected: FAIL (build): `undefined: MatchName`; `h.m.personLine undefined (type *Model has no field or method personLine)`, `undefined: person`, `h.m.people undefined`, `undefined: screenPeople`.",
        step4="Run: `go test ./internal/search/ ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`\nExpected: PASS (the Log's existing tests cover the `pick.go` refactor), no gofmt output.",
    ),
    dict(
        title="`r` renames a person or merges them into another (prompt, confirmation, undo)",
        intro="D53 in the TUI. `r` in either People pane opens `rename @raf › ` prefilled with the name, on the existing `prompt` overlay: `dsl.ParseRequester` checks it on enter and a refusal keeps the text with the reason under it. The same name says `unchanged`. A name someone already has asks `\"ana\" already exists · ⏎ merge into them · esc` (the D37 `nudge`: any other key says `cancelled`). Then `RenameRequester` runs; the status says `renamed raf → raff · 2 tasks` or `merged raf → ana · 2 tasks`, the cursor follows the new name (a filter that would hide it is cleared), and `ctrl+z` undoes it (`undid merge raf → ana`, or `can't undo merge raf → ana: it changed since`). A failed rename (the name went away meanwhile) shows the error and leaves the cursor on a real row.",
        interfaces="""- Consumes: Task 1 (`RenameRequester`, `dsl.ParseRequester`), Task 2 (`peopleState`, `peopleKey`, `peopleHints`, `peopleHelp`, `idx.people`, `search.MatchName`, `h.before`), existing `prompt` (`check`, `onEnter`), `nudge`, `actionMsg.restore`.
- Produces: `(*Model).openRename(from string) tea.Cmd`; `(*Model).rename(from, to, verb, done string) tea.Cmd` (label `rename raf → raff` / `merge raf → ana`); `r` in both People panes (`nobody to rename` when the list is empty).""",
        step2="Run: `go test ./internal/tui/ -run 'Rename|Merge'`\nExpected: FAIL: `TestRenameAPerson` (`screen lacks \"rename @raf › raf\"`), `TestMergeIntoAnotherPerson` (`screen lacks \"\\\"ana\\\" already exists · ⏎ merge into them · esc\"`), `TestRenameWhileFiltered` (`screen lacks \"renamed raf → raff · 1 task\"`), `TestRenameOfANameGoneMeanwhile` (`screen lacks \"✗ no task has @raf any more\"`).",
        step4="Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`\nExpected: PASS, no gofmt output.",
    ),
]
assert len(TASKS) == len(commits), (len(TASKS), len(commits))

HEADER = """# tt Plan 7 — People View Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `P` opens a People view that answers "what do I owe this person?" (everyone who asked for something, their open tasks by priority and what's closed) and cleans up names: `r` renames a requester on every task, or merges two spellings into one, with an undo that refuses rather than damages.

**Architecture:** `app.RenameRequester` does the whole rename or merge in one transaction over `store.Tx.TasksWithRequester`, with a compare-and-set undo over exactly the changed rows; `dsl.ParseRequester` is the one normalization rule, shared with the `@who` token. The TUI adds `screenPeople` (`people.go`): the people list comes from the loaded snapshot (`index.people`), the tasks pane reuses the Log's row renderer and priority grouping through a small `rowList` extracted from `pick.go`, the name filter uses `search.MatchName`, and rename/merge reuse the `prompt` and `nudge` overlays. No migration.

**Tech Stack:** Go 1.27; Bubble Tea v2, Lip Gloss v2, Bubbles v2 textinput; pure-Go SQLite. No new Go dependencies.

**Spec:** `docs/superpowers/specs/2026-10-12-task-tracking-design.md` — D16 (requester), D25 (`@who` token), D26 (grouping), D26d (the fuzzy word rule), D31, D34 (pins), D47 (`ctrl+d/u`), D52, D53, "Plan 7 — People view", Views 7; the "integrity over undo" principle (How we work).

## Global Constraints

- No new Go dependencies: `go.mod` and `go.sum` stay unchanged. No migration: the schema is unchanged.
- Only `app` mutates state; the TUI renames through `Service.RenameRequester`.
- One normalization rule: `dsl.ParseRequester` (the `@who` token's: trimmed, one leading `@` and trailing punctuation dropped, lowercased, one non-empty word). The TUI checks with it; `app` checks again.
- A rename touches only tasks whose requester is exactly the old name, open and closed, children included, and sets their `updated_at = now`. Renaming the empty name is refused. Its undo is all or nothing (`ErrConflict` if any changed task changed since or is gone) and never touches the merge target's own tasks.
- D52/D53 wording, exactly: `tt · People`, `PEOPLE · 7`, `ana · 3 open` (dimmed at `0 open`), `@ana`, `closed`, `rename @raf › `, `unchanged`, `"ana" already exists · ⏎ merge into them · esc`, `renamed raf → raff · 4 tasks`, `merged raf → ana · 4 tasks`, `undid merge raf → ana`, `can't undo merge raf → ana: it changed since`; `P people` in the Today and Log hints.
- The Log behaves exactly as before the `pick.go` refactor (its existing tests stay unchanged and green).
- Lines never wrap: every rendered line is truncated to the terminal width, and a screen's body never exceeds `bodyHeight()`.
- Every task ends green on: `gofmt -l .` (no output), `go vet ./...`, `go test ./...`, plus `go test -race ./internal/app/` (Task 1) or `./internal/tui/ ./internal/app/` (Tasks 2–3).
- Every commit message ends with a blank line and `Co-Authored-By: Claude <noreply@anthropic.com>`. Commit messages use single quotes.
- Sandbox: save each patch to a file and `git apply <file>`; run plain, separate git commands (no `git -C`, no `cd … &&`). If `git apply` fails, stop: an earlier step went wrong.

## Rulings made while drafting (in the code; each a one-line reversal)

- Renaming also drops trailing punctuation (`Ana,` → `ana`), because the `@who` token does; a test pins that the two agree.
- `⏎` on a person moves the focus to their tasks; task keys in the names pane do nothing.
- A child is its own row in its requester's list (`↳ #parent`); a parent shows its progress but doesn't expand. Untriaged (Inbox) tasks count as open and sit under `unset` or their priority.
- The name filter stays while tt runs, like the Log's; a leading `@` in it is ignored; `esc` on a filtered list clears the filter before leaving.
- A rename to a name the filter would hide clears the filter, so the cursor can follow the new name. After an undo the cursor stays where it is (the nearest row if that name is gone).
- The merge question is the D37 nudge: any key but `⏎` says `cancelled`.
- A rename of a name no task has any more (renamed in another shell) fails with `no task has @raf any more`.

## Review Focus

1. **A merge regretted later** (two spellings merged, then one moved task edited, closed or deleted before `ctrl+z`; or undo right away while the target has tasks of its own): the undo restores exactly the moved tasks or refuses as a whole with "it changed since", and the target's own tasks never move. Pinned in Task 1 `TestRenameRequesterMerges`, `TestRenameRequesterUndoIsAllOrNothing`, Task 3 `TestMergeIntoAnotherPerson`.
2. **A typed name that isn't one clean word** (` @Raff, `, `two words`, `@`, an empty line, an existing name in another case such as `Ana`): it is normalized like `@who` or refused with the text kept, and tasks without a requester are never touched. Pinned in Task 1 `TestParseRequester`, `TestParseRequesterAgreesWithTheToken`, `TestRenameRequesterRefusals`, Task 3 `TestRenameAPerson`, `TestMergeIntoAnotherPerson`.
3. **The list changing under the cursor** (a rename in another shell, `e` moving a task to someone else, a rename while filtered, a rename of a name that went away meanwhile): the cursor lands on a real row (the same person, the nearest one, or the new name), never on a name nobody has. Pinned in Task 2 `TestPeopleCursorAcrossReloads`, `TestPeopleTaskKeys`, Task 3 `TestRenameWhileFiltered`, `TestRenameOfANameGoneMeanwhile`.
4. **A long list in a small tmux pane** (a dozen people, someone with 12+ tasks, 8 lines tall): both panes scroll with their cursor and nothing wraps or overflows. Pinned in Task 2 `TestPeopleFitsAShortTerminal`.
5. **Getting back where you were** (`P` from Today with the Reviews lane focused, from the Log, after a Task detail round-trip): `esc` returns to the screen People was opened from. Pinned in Task 2 `TestPeopleOpensFromTodayAndTheLog`, `TestPeopleFromTheReviewsLane`.

## File Structure

```
internal/dsl/requester.go                 new: ParseRequester, ErrEmptyName, ErrNameWords
internal/store/tasks.go                   modify: Tx.TasksWithRequester
internal/app/people.go                    new: Service.RenameRequester (rename, merge, undo)
internal/search/search.go                 modify: MatchName (the People filter)
internal/tui/people.go                    new: the People view (state, rows, keys, view, filter, rename)
internal/tui/pick.go                      modify: rowList + clampRows/stepRows/headed/scroll/listLines, priorityHeadings, row.person, P
internal/tui/model.go, common.go          modify: screenPeople, Model.people, index.people, header, hints
internal/tui/today.go, overlays.go        modify: P, hints, help
*_test.go                                 new: dsl/requester, store/people, app/people, tui/people, tui/rename; search_test gains MatchName
```

Work happens on branch `plan-7` in a worktree; fast-forward merge to `main` when the final review is green.

---
"""

FOOTER = """
## After the last task

- Build tt into the scratchpad and drive the real binary through a Python `pty` script on a throwaway database (`TT_DB`, `TT_CONFIG=/nonexistent`, a 120×30 window). Seed it first with `tt add`: `Fix login @raf !1 ~s`, `Write RFC @raf !2 ~m`, `Ana's ask @ana !1 ~s`, `Nobody asked !1 ~s`. Then: `esc` (banner); `P` → `People`, `PEOPLE · 2`, `ana · 1 open`, `raf · 2 open`; `j` → `Fix login`, `Write RFC`; `r` → `rename @raf`; `ctrl+u`, `Ana`, `⏎` → `already exists`; `⏎` → `merged raf → ana · 2 tasks`, `3 open`; `ctrl+z` → `undid merge` (the cursor stays on ana); `j`, `r` → `rename @raf`; `esc`; `r`, `ctrl+u`, `raff`, `⏎` → `renamed raf → raff · 2 tasks`; `esc` → `Today`; `q`. Then the database: tasks 1 and 2 `raff`, task 3 `ana`, task 4 empty. Match short substrings (the renderer repaints changed cells only).
- `go test -race ./internal/tui/ ./internal/app/`.
- Final whole-branch review, one fix wave, park residuals as spec carry-forwards.
- **The user's check:** `make install`, open tt, `P`: the names you've used, their open counts, a duplicate pair merged with `r` (and `ctrl+z` if it was wrong).
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
