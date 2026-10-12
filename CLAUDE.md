# tt — personal task tracker (Go TUI)

A staff engineer's personal planner: capture asks fast (`tt add` / quick-add), triage in an Inbox, pick and time-block today's work, track rollovers. Single user, macOS, always open in tmux.

## Where things are

- **Spec (binding authority):** `docs/superpowers/specs/2026-10-12-task-tracking-design.md`. Decisions D1–D30, views, architecture, and the **carry-forward** sections (from Plans 1–3) plus the **UI tweaks backlog**. Read the relevant parts before planning anything.
- **Plans:** `docs/superpowers/plans/` (one per delivered slice; each has Global Constraints and Review Focus).
- **Code:** `internal/domain` (pure types/rules), `internal/dsl` (token parser), `internal/config` (TOML), `internal/store` (SQLite, migrations in `internal/store/migrations/NNN_name.sql`), `internal/app` (Service: the only code that mutates state), `internal/tui` (Bubble Tea v2), `cmd/tt`.

## Status (2026-10-12)

| Plan | State |
|------|-------|
| 1 Core & capture (`tt add`) | merged |
| 2 Planning engine (pins, blocks, StartDay rollover, undo CAS) | merged |
| 3 TUI foundation (Pick, Inbox, quick-add, undo, live reload, new-day banner) | merged |
| 4 Pick power tools: filter DSL + fuzzy search, switchable grouping, Task detail (children add/reorder), `e` edit line, `$EDITOR` notes, open link, `tt add` from stdin | **next** — not written yet |
| 5 Calendar & Today/Block: `tt-cal` Swift EventKit helper (spike passed, see spec), `CalendarSource`, timeline, Block phase, commit plan, conflicts | later |
| 6 GitHub reviews lane (direct requests only, `requested_at`, auto-close) | later |

The UI tweaks backlog in the spec (tab skips empty pane, framed/polished panes) is wanted **later, batched** — don't schedule it unasked.

## How we work

- **Product principles (from the user):** start minimal, add on friction; nudge, never block; a clean undamaged state beats undo always working (undo refuses with ErrConflict instead).
- **New work:** brainstorm → spec update → plan via superpowers:writing-plans → execute via superpowers:subagent-driven-development in a worktree → final review → fast-forward merge to `main`. The user approves the plan before execution.
- **Plans are pre-verified:** draft every code block in a scratch clone (scratchpad), run gofmt/vet/tests (and `-race` where concurrency matters), then generate the plan doc from those files (new files in full, changes to existing files as `git diff` patches), and replay the doc mechanically on a fresh clone to prove it reproduces the tree.
- **Execution models:** implementers on haiku (plans contain complete code), task reviewers on sonnet (opus for risky tasks), final whole-branch review on opus. One fix wave after the final review; park residuals as spec carry-forwards.
- **Sandbox quirks:** `git apply <<'PATCH'` heredocs and `git -C`/`cd && git` compounds are often refused inside a worktree session — save patches to a file and `git apply <file>`, and run plain separate git commands.
- **TUI checks beyond tests:** the test harness runs commands synchronously; for real-terminal behavior, build the binary into the scratchpad and drive it through a Python `pty` script (start, keys, bracketed paste, quit) checking the stripped output.

## Commands

```
go test ./...                      # all packages
go test -race ./internal/tui/ ./internal/app/
gofmt -l . && go vet ./...
go build -o ~/.local/bin/tt ./cmd/tt
TT_DB=/tmp/x.db TT_CONFIG=/nonexistent ./tt   # throwaway database
```

Real data: `~/Library/Application Support/tt/tt.db` (migrations apply automatically on open; never delete it). Config: `~/.config/tt/config.toml`.

## Git

- Work on a branch/worktree; fast-forward merge to `main` when green. No remote yet — never push, never create PRs.
- Commit messages end with a blank line and `Co-Authored-By: Claude <noreply@anthropic.com>`.
