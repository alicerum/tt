# Plan tooling

Scripts that make plans "pre-verified" (see CLAUDE.md → How we work). Run them from
the scratchpad with literal paths; they never touch the real repo except to clone it.

| Script | Does |
|---|---|
| `gen_plan_6.py` (latest; `gen_plan_5b.py`, `gen_plan_5a.py` are older) | Turns the verified scratch commits (`main..HEAD`, one per task) into the plan doc: new files in full, `git diff` per modified file, test files in Step 1, single-quoted commit messages. A task without test files (e.g. the Swift helper) gets "Create / Build and check / Commit" steps; an executable new file gets a `Make it executable:` line. Copy it per plan and replace `TASKS`/`HEADER`/`FOOTER`. |
| `step2.sh` | For each scratch commit, runs the tests with only that commit's test files on the parent tree, to get the exact "Step 2: expected failure" messages. |
| `replay.py` | Replays the plan doc on a fresh clone task by task (apply, `Make it executable:` lines, gofmt/vet/test, commit) and checks the final tree equals the scratch branch. |
| `check_brief.py` | `git apply --check` of every patch in a task brief against the current tree (after a fix round changed files a later brief patches). |
| `pty_check.py` | Drives a built `tt` through a pty on a throwaway database (Today → Log → add → pin → B → b → ⏎ → c → q) and prints the blocks/commit rows. Adjust the expected strings per plan. |

Note: the sandbox can't read the real database under `~/Library/Application Support/tt/`, so real-data checks are the user's.
