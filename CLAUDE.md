# tt — personal task tracker (Go TUI)

A staff engineer's personal planner: capture asks fast (`tt add` / quick-add), triage in an Inbox, pick and time-block today's work, track rollovers. Single user, macOS, always open in tmux.

## Where things are

- **Spec (binding authority):** `docs/superpowers/specs/2026-10-12-task-tracking-design.md`. Decisions D1–D30, views, architecture, and the **carry-forward** sections (from Plans 1–3) plus the **UI tweaks backlog**. Read the relevant parts before planning anything.
- **Plans:** `docs/superpowers/plans/` (one per delivered slice; each has Global Constraints and Review Focus).
- **Code:** `internal/domain` (pure types/rules), `internal/dsl` (token parser), `internal/config` (TOML), `internal/store` (SQLite, migrations in `internal/store/migrations/NNN_name.sql`), `internal/app` (Service: the only code that mutates state), `internal/tui` (Bubble Tea v2), `cmd/tt`.

## Status (2026-10-13)

| Plan | State |
|------|-------|
| 1 Core & capture (`tt add`) | merged |
| 2 Planning engine (pins, blocks, StartDay rollover, undo CAS) | merged |
| 3 TUI foundation (Pick, Inbox, quick-add, undo, live reload, new-day banner) | merged |
| 4a Find & browse: filter DSL + fuzzy search, switchable grouping, placement rule, row cache (spec "Plan 4 split") | merged |
| 4b Edit & detail: Task detail (children add/reorder), `e` edit line, `$EDITOR` notes, open link, `tt add` from stdin, plus the "Carry-forward from Plan 4a review" items | merged |
| 5a Today & blocking: D34 (amended), D35 Today home + Log rename, D36 pick-up/drop placement, D37 nudges, D38 timeline, `SetBlockSpan`, shared height budget (spec "Plan 5 split") | merged |
| 5b Calendar: `tt-cal` Swift EventKit helper (spike passed), `CalendarSource`, poll loop + cache, meetings on the timeline, ⚠ conflicts, freshness indicator, real `tea.Program` test, plus the "Carry-forward from Plan 5a review" items marked Plan 5b | merged |
| 6 GitHub reviews lane: D48–D50, spec "Plan 6 — GitHub reviews lane" | merged 2026-10-13 (leftovers in "Carry-forward from Plan 6 review") |
| 7 People view: D52, D53, spec "Plan 7 — People view" | **next** — verified plan written 2026-10-13 (`docs/superpowers/plans/2026-10-13-tt-plan-7-people.md`, 3 tasks); execute via subagent-driven development in a worktree |

Since 5a: D39 (merged) — blocks only inside working hours (`[work_hours]`, replaces `[timeline]`), Today shows only those hours. Plan 5b merged 2026-10-13 (D40–D45; leftovers in the spec's "Carry-forward from Plan 5b review"). `make install` puts `tt` and `tt-cal` side by side in `~/.local/bin`; `make config` installs `config.example.toml` (every default). The user ran the helper against the real calendar on 2026-10-13 (works); never run `tt-cal calendars`/`events` from the sandbox. Small changes since, each a reviewed commit on a branch, decision on `main`: D46 the now line sits above its row; D47 `ctrl+d`/`ctrl+u` half a page.

**Plan 7 handoff.** Design approved 2026-10-13: D52 (the People view), D53 (rename and merge), spec section "Plan 7 — People view". Next: superpowers:writing-plans (pre-verified in a scratch clone, generator copied from `docs/superpowers/tooling/gen_plan_6.py`, replay), then execute via subagent-driven development in a worktree. Reuse: the Log's pane machinery and row renderer (`pick.go`: `rows`, `rowLine`, `paneLines`, `clamp`/`follow`, priority grouping via `search.GroupOf`), `taskKey` for the right pane's task keys, the drop prompt as the model for the rename prompt and the merge confirmation, the Inbox's "esc returns to the screen it came from", the anchored-subsequence matcher in `internal/search` (`wordMatches`, unexported today) for the name filter, and the compare-and-set undo (`unchangedSince`). Requesters are one lowercase word (`dsl` lowercases `@who`). Since Plan 6: D51, the Reviews lane lists every request and colours the ones with a task.

The UI tweaks backlog in the spec (tab skips empty pane, framed/polished panes) is wanted **later, batched** — don't schedule it unasked.

## How we work

- **Product principles (from the user):** start minimal, add on friction; nudge, never block; a clean undamaged state beats undo always working (undo refuses with ErrConflict instead).
- **New work:** brainstorm → spec update → plan via superpowers:writing-plans → execute via superpowers:subagent-driven-development in a worktree → final review → fast-forward merge to `main`. The user approves the design/spec; in 4a and 4b they then said "go" without reviewing the plan doc separately, so after writing a verified plan, proceed to execution unless they ask to review it. Always ask (finishing menu) before merging.
- **Split big plans:** Plan 4 shipped as 4a and 4b, each merged on its own so the user can try it before the next slice. The user iterates by using it: expect design reversals after trying (e.g. D26c), and fold them in quickly as a small reviewed commit.
- **Spec bookkeeping:** decisions go into the Decisions table (next free number after D53); review leftovers go into a "Carry-forward from Plan N review" section naming the plan that owns each; spec-only commits go straight to `main`.
- **Plans are pre-verified:** draft every code block in a scratch clone (scratchpad), run gofmt/vet/tests (and `-race` where concurrency matters), then generate the plan doc from those files (new files in full, changes to existing files as `git diff` patches), and replay the doc mechanically on a fresh clone to prove it reproduces the tree.
- **Execution models:** implementers on haiku (plans contain complete code), task reviewers on sonnet (opus for risky tasks), final whole-branch review on opus. One fix wave after the final review; park residuals as spec carry-forwards.
- **Sandbox quirks:** `git apply <<'PATCH'` heredocs and `git -C`/`cd && git` compounds are often refused inside a worktree session — save patches to a file and `git apply <file>`, and run plain separate git commands. Inside a worktree session, compound shell commands (pipes with `$VAR`s, heredocs feeding python) are refused too: write helper scripts to the scratchpad with the Write tool and run them with literal paths.
- **Worktree setup that works:** `git worktree add -q .claude/worktrees/plan-N -b plan-N main`, then `EnterWorktree` with that `path`; to merge, `ExitWorktree` (keep), `git merge --ff-only plan-N`, run tests, `git worktree remove …`, `git branch -d plan-N`.
- **Plan tooling (in `docs/superpowers/tooling/`, see its README; copy into the scratchpad and run with literal paths):** a generator that turns the verified scratch commits (`main..HEAD`) into the plan doc (new files in full, `git diff` per modified file, test files in Step 1, single-quoted commit messages), a replay script that applies the doc's blocks task by task on a fresh clone (gofmt/vet/test + commit each) and diffs the result against the scratch tree, and a `git apply --check` script to confirm later briefs still apply after a fix round. Commit messages in plans use single quotes (a `$EDITOR` in a double-quoted message got expanded).
- **SDD dispatch shape that worked:** one shared `reviewer-instructions.md` in the plan workspace (method, global constraints, output format) so each review dispatch only names the brief, report, diff and task-specific spec text; fix rounds as a findings file + SendMessage to the original implementer; scoped re-review on sonnet.
- **Sandbox can't read the real DB** (`~/Library/Application Support/tt/`): a real-data check is the user's; say so instead of asking to bypass the sandbox.
- **Rulings the user accepted in 5a:** recommended options are usually picked fast; design reversals after trying come as quick "fix this now" requests (D39) — fold them in as a small reviewed commit on a branch, spec decision committed to `main` after the merge.
- **TUI checks beyond tests:** the test harness runs commands synchronously; for real-terminal behavior, build the binary into the scratchpad and drive it through a Python `pty` script (start, keys, bracketed paste, quit) checking the stripped output and the database. The renderer repaints only changed cells, so match short substrings; send each `esc` as its own write (two in one write read as alt+esc); use `EDITOR=<script>` for `E`, and never trigger `o` (it opens a real browser). To check nothing is left on the terminal for the shell, run `sh -c '"$TT" add; read -t 1 rest …'` under the pty.

## Commands

```
go test ./...                      # all packages
go test -race ./internal/tui/ ./internal/app/
gofmt -l . && go vet ./...
make install                       # tt + tt-cal into ~/.local/bin (make build: ./bin; make clean)
make config                        # config.example.toml (all defaults) → ~/.config/tt/config.toml, if none yet
TT_DB=/tmp/x.db TT_CONFIG=/nonexistent ./tt   # throwaway database
```

Real data: `~/Library/Application Support/tt/tt.db` (migrations apply automatically on open; never delete it). Config: `~/.config/tt/config.toml`.

## Git

- Work on a branch/worktree; fast-forward merge to `main` when green. No remote yet — never push, never create PRs.
- Commit messages end with a blank line and `Co-Authored-By: Claude <noreply@anthropic.com>`.
