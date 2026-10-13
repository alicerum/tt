# tt Plan 7 — People View Implementation Plan

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
### Task 1: Rename and merge in `app`: `dsl.ParseRequester`, `Tx.TasksWithRequester`, `RenameRequester`

The data side of D53. `dsl.ParseRequester` normalizes a typed name exactly as the `@who` token does (trimmed, one leading `@` and trailing punctuation dropped, lowercased) and refuses anything but one non-empty word; a test pins that it agrees with `ParseAdd`. `store.Tx.TasksWithRequester` lists every task naming a requester, open or closed, children included. `app.RenameRequester` sets the new name and `updated_at = now` on all of them in one transaction; a name that already exists simply merges. Its undo restores exactly the tasks it changed (never the merge target's own) and refuses as a whole with `ErrConflict` once any of them changed or went away (integrity over undo). Renaming the empty name is refused, so tasks without a requester can never gain one this way.

**Files:**
- Create: `internal/app/people.go`
- Create: `internal/dsl/requester.go`
- Modify: `internal/store/tasks.go`
- Create: `internal/app/people_test.go`
- Create: `internal/dsl/requester_test.go`
- Create: `internal/store/people_test.go`

**Interfaces:**
- Consumes: `store.Tx.GetTask`/`UpdateTask`, `unchangedSince`, `ErrInvalidValue`, `ErrNotFound`, `ErrConflict` (existing); test helpers `newService`, `mustAdd`, `mustGet`, `mustAddChild` (app), `openTemp`, `sampleTask`, `insert`, `inTx` (store).
- Produces:
  - `func dsl.ParseRequester(s string) (string, error)`; `dsl.ErrEmptyName` (`a name can't be empty`), `dsl.ErrNameWords` (`a name is one word, like @ana`).
  - `func (*store.Tx) TasksWithRequester(ctx, name string) ([]int64, error)`: ids in id order, exact match.
  - `func (*app.Service) RenameRequester(ctx, from, to string) (n int, undo Undo, err error)`: `ErrInvalidValue` for `from == ""` or a bad `to` (the dsl error wrapped: `a name is one word, like @ana: invalid value`); `ErrNotFound` (`no task has @raf any more`) when no task carries `from`; `to` equal to `from` after normalizing → `0, nil, nil`.

- [ ] **Step 1: Write the failing tests**

Create `internal/app/people_test.go`:

```go
package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"tt/internal/domain"
)

func requesters(t *testing.T, s *Service, ids ...int64) []string {
	t.Helper()
	var out []string
	for _, id := range ids {
		out = append(out, mustGet(t, s, id).Requester)
	}
	return out
}

func TestRenameRequester(t *testing.T) {
	s, clock := newService(t)
	ctx := context.Background()
	open := mustAdd(t, s, "Open ask @raf !1 ~s")
	done := mustAdd(t, s, "Done ask @raf !1 ~s")
	parent := mustAdd(t, s, "Parent !1 ~l")
	child := mustAddChild(t, s, parent.ID, "Child @raf ~s")
	other := mustAdd(t, s, "Other @ana !1 ~s")
	nobody := mustAdd(t, s, "Nobody's !1 ~s")
	if _, err := s.Done(ctx, done.ID); err != nil {
		t.Fatal(err)
	}
	untouched := []domain.Task{mustGet(t, s, parent.ID), mustGet(t, s, other.ID), mustGet(t, s, nobody.ID)}

	clock.Advance(time.Minute)
	n, undo, err := s.RenameRequester(ctx, "raf", "  @Raff ")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("renamed %d tasks; want 3 (open, closed and a child)", n)
	}
	if got := requesters(t, s, open.ID, done.ID, child.ID); !cmp.Equal(got, []string{"raff", "raff", "raff"}) {
		t.Errorf("requesters = %v; want raff (normalized like @who)", got)
	}
	for _, id := range []int64{open.ID, done.ID, child.ID} {
		if got := mustGet(t, s, id); !got.UpdatedAt.Equal(clock.now) {
			t.Errorf("#%d updated_at = %v; want now, so older undos see the change", id, got.UpdatedAt)
		}
	}
	if got := mustGet(t, s, done.ID); got.Status != domain.StatusDone {
		t.Errorf("a renamed task keeps its status: %s", got.Status)
	}
	for _, want := range untouched {
		if got := mustGet(t, s, want.ID); !cmp.Equal(got, want) {
			t.Errorf("#%d isn't raf's and must not change:\n%s", want.ID, cmp.Diff(want, got))
		}
	}

	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := requesters(t, s, open.ID, done.ID, child.ID); !cmp.Equal(got, []string{"raf", "raf", "raf"}) {
		t.Errorf("after undo: %v; want raf back", got)
	}
}

func TestRenameRequesterMerges(t *testing.T) {
	s, clock := newService(t)
	ctx := context.Background()
	raf1 := mustAdd(t, s, "One @raf !1 ~s")
	ana := mustAdd(t, s, "Ana's own @ana !1 ~s")
	raf2 := mustAdd(t, s, "Two @raf !2 ~s")

	clock.Advance(time.Minute)
	n, undo, err := s.RenameRequester(ctx, "raf", "ana")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("merged %d tasks; want 2", n)
	}
	if got := requesters(t, s, raf1.ID, ana.ID, raf2.ID); !cmp.Equal(got, []string{"ana", "ana", "ana"}) {
		t.Errorf("after the merge: %v", got)
	}
	ownBefore := mustGet(t, s, ana.ID)

	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := requesters(t, s, raf1.ID, ana.ID, raf2.ID); !cmp.Equal(got, []string{"raf", "ana", "raf"}) {
		t.Errorf("the merge's undo gives raf back exactly the tasks it moved: %v", got)
	}
	if got := mustGet(t, s, ana.ID); !cmp.Equal(got, ownBefore) {
		t.Errorf("the merge's undo touched ana's own task:\n%s", cmp.Diff(ownBefore, got))
	}
}

func TestRenameRequesterUndoIsAllOrNothing(t *testing.T) {
	s, clock := newService(t)
	ctx := context.Background()
	a := mustAdd(t, s, "A @raf !1 ~s")
	b := mustAdd(t, s, "B @raf !1 ~s")
	clock.Advance(time.Minute)
	_, undo, err := s.RenameRequester(ctx, "raf", "raff")
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Minute)
	if _, err := s.Done(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := undo(ctx); !errors.Is(err, ErrConflict) {
		t.Fatalf("undo after one task changed: %v; want ErrConflict", err)
	}
	if got := requesters(t, s, a.ID, b.ID); !cmp.Equal(got, []string{"raff", "raff"}) {
		t.Errorf("a refused undo changes nothing: %v", got)
	}

	clock.Advance(time.Minute)
	_, undo, err = s.RenameRequester(ctx, "raff", "rafa")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := undo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo after one task was deleted: %v; want ErrConflict", err)
	}
	if got := requesters(t, s, b.ID); !cmp.Equal(got, []string{"rafa"}) {
		t.Errorf("a refused undo changes nothing: %v", got)
	}
}

func TestRenameRequesterRefusals(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	raf := mustAdd(t, s, "A @raf !1 ~s")
	nobody := mustAdd(t, s, "No requester !1 ~s")

	for _, to := range []string{"", "  ", "@", "two words"} {
		if _, _, err := s.RenameRequester(ctx, "raf", to); !errors.Is(err, ErrInvalidValue) {
			t.Errorf("rename to %q: %v; want ErrInvalidValue", to, err)
		}
	}
	if _, _, err := s.RenameRequester(ctx, "", "ana"); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("rename of no name: %v; want ErrInvalidValue", err)
	}
	if got := mustGet(t, s, nobody.ID).Requester; got != "" {
		t.Errorf("a task without a requester got %q", got)
	}
	if _, _, err := s.RenameRequester(ctx, "zed", "ana"); !errors.Is(err, ErrNotFound) {
		t.Errorf("rename of a name no task has: %v; want ErrNotFound", err)
	}
	n, undo, err := s.RenameRequester(ctx, "raf", "@RAF")
	if err != nil || n != 0 || undo != nil {
		t.Errorf("rename to the same name = %d, undo %v, %v; want nothing done", n, undo != nil, err)
	}
	if got := mustGet(t, s, raf.ID).Requester; got != "raf" {
		t.Errorf("raf's task: %q", got)
	}
}
```

Create `internal/dsl/requester_test.go`:

```go
package dsl

import (
	"errors"
	"testing"
)

func TestParseRequester(t *testing.T) {
	cases := []struct {
		in, want string
		err      error
	}{
		{"raff", "raff", nil},
		{"  @Raff ", "raff", nil},
		{"Ana,", "ana", nil},
		{"@@ana", "@ana", nil},
		{"ana-maria", "ana-maria", nil},
		{"Ünïcode", "ünïcode", nil},
		{"", "", ErrEmptyName},
		{"  ", "", ErrEmptyName},
		{"@", "", ErrEmptyName},
		{"@.", "", ErrEmptyName},
		{"ana maria", "", ErrNameWords},
		{"@ana\tb", "", ErrNameWords},
	}
	for _, c := range cases {
		got, err := ParseRequester(c.in)
		if got != c.want || !errors.Is(err, c.err) {
			t.Errorf("ParseRequester(%q) = %q, %v; want %q, %v", c.in, got, err, c.want, c.err)
		}
	}
}

// A name typed in the rename prompt is stored exactly as the same name
// typed as @who in a quick-add line (D53).
func TestParseRequesterAgreesWithTheToken(t *testing.T) {
	for _, name := range []string{"ana", "Ana", "ana,", "ANA.", "@ana", "ana-maria", "bob@x.com", "Ünïcode"} {
		d, err := ParseAdd("Ask @"+name, 3)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := ParseRequester("@" + name); err != nil || got != d.Requester {
			t.Errorf("ParseRequester(%q) = %q, %v; the token gives %q", "@"+name, got, err, d.Requester)
		}
	}
}
```

Create `internal/store/people_test.go`:

```go
package store

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"tt/internal/domain"
)

func TestTasksWithRequester(t *testing.T) {
	s, _ := openTemp(t)
	task := func(requester string, status domain.Status, parent int64) int64 {
		x := sampleTask()
		x.Requester, x.Status, x.ParentID = requester, status, parent
		return insert(t, s, x)
	}
	open := task("ana", domain.StatusOpen, 0)
	task("anabel", domain.StatusOpen, 0)
	task("", domain.StatusOpen, 0)
	done := task("ana", domain.StatusDone, 0)
	parent := task("bob", domain.StatusOpen, 0)
	child := task("ana", domain.StatusDropped, parent)

	inTx(t, s, func(ctx context.Context, x *Tx) error {
		ids, err := x.TasksWithRequester(ctx, "ana")
		if err != nil {
			return err
		}
		if want := []int64{open, done, child}; !cmp.Equal(ids, want) {
			t.Errorf("ana's tasks = %v; want %v (open and closed, children too; never anabel's)", ids, want)
		}
		none, err := x.TasksWithRequester(ctx, "raff")
		if err != nil {
			return err
		}
		if len(none) != 0 {
			t.Errorf("nobody called raff: %v", none)
		}
		return nil
	})
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/dsl/ ./internal/store/ ./internal/app/`
Expected: FAIL (build): `undefined: ParseRequester`, `undefined: ErrEmptyName`, `undefined: ErrNameWords`; `x.TasksWithRequester undefined (type *Tx has no field or method TasksWithRequester)`; `s.RenameRequester undefined (type *Service has no field or method RenameRequester)`.

- [ ] **Step 3: Implement**

Create `internal/app/people.go`:

```go
package app

import (
	"context"
	"fmt"

	"tt/internal/domain"
	"tt/internal/dsl"
	"tt/internal/store"
)

// RenameRequester gives every task whose requester is from (open or closed,
// children included) the requester to, normalized like the @who token
// (D53). When to already names someone, that merges the two. n is how many
// tasks changed; renaming to the same name changes nothing. The undo gives
// exactly those tasks their old rows back, so a merge's undo never touches
// the target's own tasks, and refuses as a whole (ErrConflict) once any of
// them changed since.
func (s *Service) RenameRequester(ctx context.Context, from, to string) (n int, undo Undo, err error) {
	if from == "" {
		return 0, nil, fmt.Errorf("no name to rename: %w", ErrInvalidValue)
	}
	name, err := dsl.ParseRequester(to)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %w", err, ErrInvalidValue)
	}
	if name == from {
		return 0, nil, nil
	}
	var before, after []domain.Task
	err = s.store.InTx(ctx, func(x *store.Tx) error {
		ids, err := x.TasksWithRequester(ctx, from)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return fmt.Errorf("no task has @%s any more: %w", from, ErrNotFound)
		}
		now := s.clock.Now()
		for _, id := range ids {
			t, err := x.GetTask(ctx, id)
			if err != nil {
				return err
			}
			before = append(before, t)
			t.Tags = append([]string(nil), t.Tags...)
			t.Requester, t.UpdatedAt = name, now
			if err := x.UpdateTask(ctx, t); err != nil {
				return err
			}
			after = append(after, t)
		}
		return nil
	})
	if err != nil {
		return 0, nil, err
	}
	return len(after), func(ctx context.Context) error {
		return s.store.InTx(ctx, func(x *store.Tx) error {
			for _, t := range after { // all or nothing: check every task before restoring any
				if err := unchangedSince(ctx, x, t); err != nil {
					return err
				}
			}
			for _, t := range before {
				if err := x.UpdateTask(ctx, t); err != nil {
					return err
				}
			}
			return nil
		})
	}, nil
}
```

Create `internal/dsl/requester.go`:

```go
package dsl

import (
	"errors"
	"strings"
	"unicode"
)

var (
	ErrEmptyName = errors.New("a name can't be empty")
	ErrNameWords = errors.New("a name is one word, like @ana")
)

// ParseRequester normalizes a typed name the way the @who token does (D53):
// trimmed, one leading @ and any trailing punctuation dropped, lowercased.
// What is left must be one non-empty word.
func ParseRequester(s string) (string, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "@")
	if strings.ContainsFunc(s, unicode.IsSpace) {
		return "", ErrNameWords
	}
	s = strings.ToLower(strings.TrimRight(s, trailingPunct))
	if s == "" {
		return "", ErrEmptyName
	}
	return s, nil
}
```

Patch `internal/store/tasks.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/store/tasks.go b/internal/store/tasks.go
index 16f51b8..12d50ff 100644
--- a/internal/store/tasks.go
+++ b/internal/store/tasks.go
@@ -120,6 +120,28 @@ func (x *Tx) Children(ctx context.Context, parentID int64) ([]domain.Task, error
 	return kids, nil
 }
 
+// TasksWithRequester returns the ids of every task whose requester is name,
+// open or closed, children included (D53).
+func (x *Tx) TasksWithRequester(ctx context.Context, name string) ([]int64, error) {
+	rows, err := x.tx.QueryContext(ctx, "SELECT id FROM tasks WHERE requester = ? ORDER BY id", name)
+	if err != nil {
+		return nil, err
+	}
+	var ids []int64
+	for rows.Next() {
+		var id int64
+		if err := rows.Scan(&id); err != nil {
+			rows.Close()
+			return nil, err
+		}
+		ids = append(ids, id)
+	}
+	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
+		return nil, err
+	}
+	return ids, nil
+}
+
 // NextChildPosition returns the position after parentID's last child.
 func (x *Tx) NextChildPosition(ctx context.Context, parentID int64) (int, error) {
 	var n int
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/dsl/ ./internal/store/ ./internal/app/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/app/`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/app/people.go internal/app/people_test.go internal/dsl/requester.go internal/dsl/requester_test.go internal/store/people_test.go internal/store/tasks.go
git commit -m 'feat(app): RenameRequester renames or merges a requester, with all-or-nothing undo (D53)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 2: The People view: `P`, the names and a person's tasks, the `/` filter, task keys

D52 in the TUI (`people.go`). `P` on Today or in the Log opens `tt · People`; `esc` returns to the screen it came from (also after a Task detail round-trip). Left, `PEOPLE · N`: every requester some task carries, alphabetical, `ana · 3 open`, dimmed at `0 open`. Right, `@ana`: the person's open tasks grouped by priority like the backlog (oldest first; pinned ones marked 📌; a child is its own row with `↳ #parent`), then a `closed` heading with done and dropped tasks, newest first, with their close date. `tab` switches panes (`⏎` on a person too); `j/k`, `ctrl+d/u` move; in the tasks pane `⏎ p u x d D e E o` go through the existing `taskKey`. `/` filters names live with `search.MatchName` (the Log's anchored-subsequence matcher, a leading `@` ignored): `enter` keeps it, `esc` in the line clears it, `esc` on a filtered list clears it before leaving. Each pane keeps its row across reloads (the nearest row when the person is gone). To share the Log's list machinery, `pick.go`'s cursor clamp, step, scroll and pane render move into small functions (`clampRows`, `stepRows`, `headed`, `scroll`, `listLines`) and a `rowList` type; the Log behaves as before. Hints and help gain `P people`.

**Files:**
- Modify: `internal/search/search.go`
- Modify: `internal/tui/common.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/overlays.go`
- Create: `internal/tui/people.go`
- Modify: `internal/tui/pick.go`
- Modify: `internal/tui/today.go`
- Modify: `internal/search/search_test.go`
- Create: `internal/tui/people_test.go`

**Interfaces:**
- Consumes: Task 1 (`Service.RenameRequester`, in a test only); existing `rowLine`, `taskKey`, `openDetail`, `steps`, `repeat`, `columns`, `pad`, `plural`, `dayLabel`, `newInput`, `fitInput`, `visibleRows`, `search.GroupOf`/`Compare`/`Oldest`; test helpers `newHarness`, `newGHHarness`, `threeReviews`, `h.add`, `h.keys`, `h.typeText`, `h.mustSee`, `h.mustNotSee`, `h.task`, `h.screen`.
- Produces:
  - `func search.MatchName(query, name string) bool`.
  - `screenPeople`; `person{name string; open int}`; `index.people []person` (alphabetical); `Model.people peopleState`.
  - `peopleState{from screen; onTasks bool; filter string; names []person; cursor, offset int; selected string; tasks rowList}` with `refresh(m)`, `setFilter(m, text)`, `move(m, delta)`, `follow(m)`.
  - `rowList{rows []row; cursor, offset int; selected int64}` with `clamp()`, `move(delta)`, `follow(visible)`, `selectedRow() (row, bool)`; `clampRows(rows, cursor, id) (int, int64)`, `stepRows(rows, c, delta) (int, bool)`, `headed(rows, c) int`, `scroll(top, c, off, visible, n) int`; `(*Model).listLines(l rowList, visible int, title, empty string, width int, focused bool) []string`; `(*Model).priorityHeadings() []string`; `row.person bool` (no `@who`, 📌 when pinned, the close date).
  - `(*Model).openPeople()`, `personRows(name) []row`, `peopleKey(key) tea.Cmd`, `peopleView() string`, `personLine(pr person, width int, cursor bool) string`, `peopleHints() string`, `peopleHelp`; `peopleFilter` and `openPeopleFilter()`.
  - Test helper `(*harness).before(parts ...string)`: the screen shows parts in this order.

- [ ] **Step 1: Write the failing tests**

Patch `internal/search/search_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/search/search_test.go b/internal/search/search_test.go
index 6e2c03e..cc906fa 100644
--- a/internal/search/search_test.go
+++ b/internal/search/search_test.go
@@ -140,3 +140,27 @@ func TestGroupByCycle(t *testing.T) {
 		}
 	}
 }
+
+func TestMatchName(t *testing.T) {
+	cases := []struct {
+		query, name string
+		want        bool
+	}{
+		{"", "ana", true},
+		{"an", "ana", true},
+		{"AN", "ana", true},
+		{"@an", "ana", true},
+		{"@", "ana", true}, // a lone @ is a name still being typed
+		{"rf", "raff", true},
+		{"mar", "ana-maria", true},
+		{"aa", "ana-maria", true},
+		{"nm", "ana-maria", false},
+		{"ana x", "ana", false},
+		{"bo", "ana", false},
+	}
+	for _, c := range cases {
+		if got := MatchName(c.query, c.name); got != c.want {
+			t.Errorf("MatchName(%q, %q) = %v; want %v", c.query, c.name, got, c.want)
+		}
+	}
+}
```

Create `internal/tui/people_test.go`:

```go
package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/config"
)

// before fails unless the screen shows each of parts, in this order.
func (h *harness) before(parts ...string) {
	h.t.Helper()
	s, at := h.screen(), 0
	for _, p := range parts {
		i := strings.Index(s[at:], p)
		if i < 0 {
			h.t.Fatalf("screen lacks %q after %q:\n%s", p, s[:at], s)
		}
		at += i + len(p)
	}
}

func TestPeopleOpensFromTodayAndTheLog(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("Fix login @ana !1 ~s")
	h.keys("P")
	h.mustSee("tt · People", "PEOPLE · 1", "ana · 1 open", "@ana", "[P1 S] Fix login")
	h.keys("esc")
	if h.m.screen != screenToday {
		t.Fatalf("esc from People opened from Today: screen %d", h.m.screen)
	}
	h.keys("L", "P")
	h.mustSee("tt · People")
	h.keys("esc")
	if h.m.screen != screenLog {
		t.Fatalf("esc from People opened from the Log: screen %d", h.m.screen)
	}

	h.keys("P", "tab", "enter") // Task detail from People goes back there
	h.mustSee("tt · Detail", "#1 Fix login")
	h.keys("esc")
	h.mustSee("tt · People")
	h.keys("esc")
	if h.m.screen != screenLog {
		t.Errorf("People still returns to the Log after a detail round-trip: screen %d", h.m.screen)
	}
}

func TestPeopleFromTheReviewsLane(t *testing.T) {
	h := newGHHarness(t, config.Default())
	h.add("Fix login @ana !1 ~s")
	threeReviews(h)
	h.keys("w", "P") // a screen key leaves the lane (D49)
	h.mustSee("tt · People")
	h.keys("esc")
	if h.m.screen != screenToday || h.m.lane.focused {
		t.Errorf("back on Today with the lane unfocused: screen %d, lane focused %v", h.m.screen, h.m.lane.focused)
	}
}

func TestPeopleListsEveryRequester(t *testing.T) {
	h := newHarness(t, config.Default())
	ctx := context.Background()
	h.add("Schema review @ana !1 ~s")
	h.add("Later ask @ana !2 ~m")
	old := h.add("Old ask @ana !1 ~s")
	idea := h.add("Raff's idea @raff !3 ~s")
	h.add("Nobody asked !1 ~s")
	if _, err := h.svc.Done(ctx, old.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Drop(ctx, idea.ID, ""); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "P")
	h.mustSee("PEOPLE · 2")
	h.before("ana · 2 open", "raff · 0 open") // alphabetical; closed tasks still name raff
	h.mustNotSee("Nobody asked")
	if got, want := h.m.personLine(person{name: "raff"}, 30, false), styleDim.Render(pad("  raff · 0 open", 30)); got != want {
		t.Errorf("a person with nothing open is dimmed: %q; want %q", got, want)
	}
	if got := h.m.personLine(person{name: "ana", open: 2}, 30, false); got != pad("  ana · 2 open", 30) {
		t.Errorf("ana's line: %q", got)
	}
}

func TestPeopleShowsAPersonsTasks(t *testing.T) {
	h := newHarness(t, config.Default())
	ctx := context.Background()
	login := h.add("Fix login @ana !1 ~s")
	h.add("Write RFC @ana !2 ~m")
	parent := h.add("Profiles !1 ~l")
	if _, _, err := h.svc.AddChild(ctx, parent.ID, "Import flow @ana ~s"); err != nil {
		t.Fatal(err)
	}
	bug := h.add("Old bug @ana !1 ~s")
	idea := h.add("Idea @ana !3 ~s")
	h.add("Bob's ask @bob !1 ~s")
	if _, err := h.svc.Pin(ctx, login.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Done(ctx, bug.ID); err != nil {
		t.Fatal(err)
	}
	h.clock.now = h.clock.now.Add(time.Hour)
	if _, err := h.svc.Drop(ctx, idea.ID, ""); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "P")
	h.before("@ana",
		"P1 · someone is blocked", "[P1 S] Fix login 📌", fmt.Sprintf("[P1 S] Import flow ↳ #%d", parent.ID),
		"P2 · important this month", "[P2 M] Write RFC",
		"closed", "Idea ✗ Mon 12 Oct", "Old bug ✓ Mon 12 Oct") // newest first
	h.mustNotSee("Fix login @ana", "Bob's ask", "Profiles")
}

func TestPeopleTaskKeys(t *testing.T) {
	h := newHarness(t, config.Default())
	login := h.add("Fix login @ana !1 ~s")
	h.add("Write RFC @ana !2 ~m")
	h.add("Bob's ask @bob !1 ~s")
	h.keys("P", "tab")
	h.keys("x")
	h.mustSee(`✓ "Fix login"`)
	h.before("closed", "Fix login ✓ Mon 12 Oct")
	h.keys("ctrl+z")
	h.mustSee(`undid done "Fix login"`, "ana · 2 open")
	h.keys("p")
	h.mustSee(`pinned "Fix login"`, "[P1 S] Fix login 📌")

	h.keys("e", "ctrl+u") // the edit line, cleared
	h.typeText("Fix login @bob !1 ~s")
	h.keys("enter")
	if got := h.task(login.ID).Requester; got != "bob" {
		t.Fatalf("requester after e = %q", got)
	}
	if h.m.people.selected != "ana" {
		t.Errorf("the cursor stays on the person: %q", h.m.people.selected)
	}
	h.mustSee("ana · 1 open", "bob · 2 open", "@ana", "Write RFC", `edited "Fix login"`)
	h.mustNotSee("[P1 S] Fix login")
	h.keys("enter")
	h.mustSee("tt · Detail", "Write RFC")
}

func TestPeopleFilter(t *testing.T) {
	h := newHarness(t, config.Default())
	for _, who := range []string{"ana", "anabel", "raff", "bob"} {
		h.add(fmt.Sprintf("Ask from %s @%s !1 ~s", who, who))
	}
	h.keys("P", "/")
	h.typeText("an")
	h.mustSee("PEOPLE · 2 matches · an", "ana · 1 open", "anabel · 1 open")
	h.mustNotSee("raff · ", "bob · ")
	h.keys("esc") // esc in the filter clears it
	h.mustSee("PEOPLE · 4", "raff · 1 open")

	h.keys("/")
	h.typeText("@rf")
	h.keys("enter")
	h.mustSee("PEOPLE · 1 match · @rf", "@raff", "Ask from raff", "esc clears filter")
	h.keys("esc")
	h.mustSee("filter cleared", "PEOPLE · 4")
	if h.m.screen != screenPeople || h.m.people.selected != "raff" {
		t.Errorf("esc on a filtered list clears the filter and keeps the person: screen %d, %q", h.m.screen, h.m.people.selected)
	}

	h.keys("/")
	h.typeText("zz")
	h.mustSee("PEOPLE · 0 matches · zz", "no matches · esc clears")
	h.mustNotSee("Ask from")
	h.keys("x") // still typing: a letter, not a task key
	h.mustSee("0 matches · zzx")
	h.keys("enter", "esc", "esc")
	if h.m.screen != screenToday {
		t.Errorf("esc, esc: filter cleared, then back to Today: screen %d", h.m.screen)
	}
}

func TestPeopleCursorAcrossReloads(t *testing.T) {
	h := newHarness(t, config.Default())
	ctx := context.Background()
	h.add("Ana's @ana !1 ~s")
	h.add("Bob's @bob !1 ~s")
	h.add("Cat's first @cat !2 ~s")
	second := h.add("Cat's second @cat !2 ~s")
	h.keys("P", "j", "j", "tab", "j")
	if r, _ := h.m.people.tasks.selectedRow(); r.task.ID != second.ID {
		t.Fatalf("on cat's second task, got #%d", r.task.ID)
	}
	h.add("Cat's urgent @cat !1 ~s") // sorts above both
	if r, _ := h.m.people.tasks.selectedRow(); r.task.ID != second.ID {
		t.Errorf("a reload keeps the task under the cursor, got #%d", r.task.ID)
	}

	h.keys("tab", "k") // on bob
	if _, _, err := h.svc.RenameRequester(ctx, "bob", "zed"); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	if h.m.people.selected != "cat" {
		t.Errorf("bob renamed elsewhere: the cursor lands on the nearest row (cat), got %q", h.m.people.selected)
	}
	h.mustSee("@cat", "zed · 1 open")
}

func TestPeopleFitsAShortTerminal(t *testing.T) {
	h := newHarness(t, config.Default())
	for i := 1; i <= 12; i++ {
		h.add(fmt.Sprintf("Ask %02d @p%02d !1 ~s", i, i))
		h.add(fmt.Sprintf("Task %02d @p01 !1 ~s", i))
	}
	h.send(tea.WindowSizeMsg{Width: 100, Height: 8})
	h.keys("P")
	check := func() {
		t.Helper()
		if n := strings.Count(h.m.render(), "\n") + 1; n > 8 {
			t.Fatalf("People renders %d lines in an 8-line terminal:\n%s", n, h.screen())
		}
		for _, l := range strings.Split(h.m.render(), "\n") {
			if w := ansi.StringWidth(l); w > 100 {
				t.Fatalf("a line is %d cells wide: %q", w, ansi.Strip(l))
			}
		}
	}
	check()
	h.keys("tab", "ctrl+d", "ctrl+d", "ctrl+d", "ctrl+d")
	check()
	if r, _ := h.m.people.tasks.selectedRow(); !strings.Contains(h.screen(), r.task.Title) {
		t.Errorf("the cursor's task %q is in view:\n%s", r.task.Title, h.screen())
	}
	h.keys("tab")
	for range 11 {
		h.keys("j")
	}
	check()
	h.mustSee("p12 · 1 open", "@p12")
}

func TestPeopleWithNobody(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("Nobody asked !1 ~s")
	h.keys("P")
	h.mustSee("PEOPLE · 0", "nobody yet · @who on a task names who asked")
	h.keys("tab", "x", "enter", "j") // nothing to act on
	h.mustNotSee("✓")
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/search/ ./internal/tui/`
Expected: FAIL (build): `undefined: MatchName`; `h.m.personLine undefined (type *Model has no field or method personLine)`, `undefined: person`, `h.m.people undefined`, `undefined: screenPeople`.

- [ ] **Step 3: Implement**

Patch `internal/search/search.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/search/search.go b/internal/search/search.go
index 054fd24..361c45b 100644
--- a/internal/search/search.go
+++ b/internal/search/search.go
@@ -58,6 +58,18 @@ func Match(f dsl.Filter, t domain.Task, parent *domain.Task, now time.Time) bool
 	return true
 }
 
+// MatchName reports whether a person's name passes query, the People
+// filter (D52): every word of query, lowercased and without a leading @,
+// matches name like a fuzzy filter word (D26d).
+func MatchName(query, name string) bool {
+	for _, w := range strings.Fields(strings.ToLower(query)) {
+		if w = strings.TrimPrefix(w, "@"); w != "" && !wordMatches(w, name) {
+			return false
+		}
+	}
+	return true
+}
+
 // wordMatches: w (lowercase) is a substring of hay, or a subsequence of one of
 // hay's words that starts at that word's first letter ("schm" → "schema").
 func wordMatches(w, hay string) bool {
```

Patch `internal/tui/common.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/common.go b/internal/tui/common.go
index f29b6c2..0b0b7f1 100644
--- a/internal/tui/common.go
+++ b/internal/tui/common.go
@@ -3,6 +3,7 @@ package tui
 import (
 	"cmp"
 	"fmt"
+	"maps"
 	"slices"
 	"strings"
 	"time"
@@ -35,6 +36,7 @@ type index struct {
 	cand       map[int64]domain.Candidate
 	inInbox    map[int64]bool
 	requesters []string // most used first
+	people     []person // every requester some task carries, alphabetical (D52)
 	tags       []string // most used first
 	// reviewTask is the task of each review request that has one and didn't
 	// drop it, by external id (D51).
@@ -44,7 +46,7 @@ type index struct {
 func buildIndex(st app.State) index {
 	idx := index{children: map[int64][]domain.Task{}, cand: map[int64]domain.Candidate{}, inInbox: map[int64]bool{},
 		reviewTask: map[string]domain.Task{}}
-	reqCount, tagCount := map[string]int{}, map[string]int{}
+	reqCount, tagCount, openCount := map[string]int{}, map[string]int{}, map[string]int{}
 	for _, t := range st.Snapshot.Tasks() {
 		if t.Source != nil && t.Source.Kind == domain.SourceGitHub && t.Status != domain.StatusDropped {
 			idx.reviewTask[t.Source.ID] = t
@@ -54,6 +56,9 @@ func buildIndex(st app.State) index {
 		}
 		if t.Requester != "" {
 			reqCount[t.Requester]++
+			if t.Status == domain.StatusOpen {
+				openCount[t.Requester]++
+			}
 		}
 		for _, tag := range t.Tags {
 			tagCount[tag]++
@@ -71,6 +76,9 @@ func buildIndex(st app.State) index {
 		idx.inInbox[t.ID] = true
 	}
 	idx.requesters, idx.tags = byUse(reqCount), byUse(tagCount)
+	for _, name := range slices.Sorted(maps.Keys(reqCount)) {
+		idx.people = append(idx.people, person{name: name, open: openCount[name]})
+	}
 	return idx
 }
 
@@ -117,6 +125,8 @@ func (m *Model) headerView() string {
 		name = "Inbox"
 	case screenDetail:
 		name = "Detail"
+	case screenPeople:
+		name = "People"
 	}
 	h := styleTitle.Render("tt · "+name) + " · " + dayLabel(m.state.Plan.Day)
 	if n := len(m.idx.inInbox); n > 0 {
@@ -196,10 +206,13 @@ func (m *Model) hintsView() string {
 		}
 		return styleDim.Render(fmt.Sprintf("1-%d priority · s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help", len(m.svc.Config().Priorities)))
 	}
+	if m.screen == screenPeople {
+		return styleDim.Render(m.peopleHints())
+	}
 	if m.screen == screenDetail {
 		return styleDim.Render("j/k move · ⏎ open child · a add child · J/K reorder · e edit · E note · o link · x done · d drop · p pin · esc back · ? help")
 	}
-	hints := "⏎ open · e edit · a add · / filter · g group · p pin · u unpin · x done · d drop · tab pane · →/← expand · B block today · esc today · i inbox · ctrl+z undo · ? help · q quit"
+	hints := "⏎ open · e edit · a add · / filter · g group · p pin · u unpin · x done · d drop · tab pane · →/← expand · B block today · esc today · i inbox · P people · ctrl+z undo · ? help · q quit"
 	if m.pick.filtered() && m.overlay == nil { // in the filter line, esc restores the previous filter
 		hints = "esc clears filter · " + strings.Replace(hints, " · esc today", "", 1)
 	}
```

Patch `internal/tui/model.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/model.go b/internal/tui/model.go
index ea7f674..a526b58 100644
--- a/internal/tui/model.go
+++ b/internal/tui/model.go
@@ -48,6 +48,7 @@ const (
 	screenLog                 // the backlog; the planning step Pick
 	screenInbox
 	screenDetail
+	screenPeople // requesters and what they asked for (D52)
 )
 
 const maxUndo = 50
@@ -86,6 +87,7 @@ type Model struct {
 	pick   pickState
 	inbox  inboxState
 	detail detailState
+	people peopleState
 
 	lane laneState
 
@@ -173,6 +175,7 @@ func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
 	m.detail.follow(m)
 	m.today.follow(m)
 	m.lane.follow(m)
+	m.people.follow(m)
 	return m, cmd
 }
 
@@ -364,6 +367,8 @@ func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
 		return m.inboxKey(key)
 	case screenDetail:
 		return m.detailKey(key)
+	case screenPeople:
+		return m.peopleKey(key)
 	}
 	return m.pickKey(key)
 }
@@ -478,6 +483,8 @@ func (m *Model) render() string {
 		parts = append(parts, clipLines(m.inboxView(), m.bodyHeight()))
 	case screenDetail:
 		parts = append(parts, clipLines(m.detailView(), m.bodyHeight()))
+	case screenPeople:
+		parts = append(parts, m.peopleView())
 	default:
 		if filtering {
 			parts = append(parts, fb.view(m)) // the filter line sits above the backlog
```

Patch `internal/tui/overlays.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/overlays.go b/internal/tui/overlays.go
index a601aa4..568fc8c 100644
--- a/internal/tui/overlays.go
+++ b/internal/tui/overlays.go
@@ -280,6 +280,8 @@ func (helpOverlay) view(m *Model) string {
 		}
 	case screenToday:
 		local = todayHelp
+	case screenPeople:
+		local = peopleHelp
 	default:
 		local = []string{
 			"⏎        task detail",
@@ -293,7 +295,7 @@ func (helpOverlay) view(m *Model) string {
 			"g        group by priority → size → created → requester (remembered)",
 			"B        done picking: block today's candidates in Today",
 			"w        the Reviews lane: p pins a review (a task Review #n), o opens the PR · w or tab back",
-			"esc      back to Today (when no filter is set) · i inbox",
+			"esc      back to Today (when no filter is set) · i inbox · P people (who asked for what)",
 		}
 	}
 	return styleTitle.Render("KEYS") + "\n" + strings.Join(append(local, global...), "\n") + "\n" + styleDim.Render("any key closes")
```

Create `internal/tui/people.go`:

```go
package tui

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"tt/internal/domain"
	"tt/internal/search"
)

// person is one requester in People (D52).
type person struct {
	name string
	open int // open tasks that name them
}

// peopleState is the People view (D52): everyone some task names as its
// requester on the left, the selected person's tasks on the right.
type peopleState struct {
	from     screen   // esc returns here
	onTasks  bool     // the tasks pane has the focus
	filter   string   // the names filter as typed (trimmed); "" = none
	names    []person // the index's people, narrowed by the filter
	cursor   int
	offset   int     // first name shown
	selected string  // the name under the cursor; survives reloads
	tasks    rowList // the selected person's tasks
}

// openPeople shows People; esc returns to the current screen.
func (m *Model) openPeople() {
	m.people.from, m.screen = m.screen, screenPeople
	m.people.refresh(m)
}

// refresh rebuilds both panes after a load or a filter change. The cursor
// stays on its person, or on the nearest row when they're gone (renamed
// elsewhere, their last task deleted); the tasks pane keeps its task the
// same way, and starts at the top for another person.
func (p *peopleState) refresh(m *Model) {
	p.names = nil
	for _, pr := range m.idx.people {
		if search.MatchName(p.filter, pr.name) {
			p.names = append(p.names, pr)
		}
	}
	if i := slices.IndexFunc(p.names, func(pr person) bool { return pr.name == p.selected }); i >= 0 {
		p.cursor = i
	}
	p.cursor = max(0, min(p.cursor, len(p.names)-1))
	name := ""
	if p.cursor < len(p.names) {
		name = p.names[p.cursor].name
	}
	if name != p.selected {
		p.tasks = rowList{}
	}
	p.selected = name
	p.tasks.rows = m.personRows(name)
	p.tasks.clamp()
}

// setFilter narrows the names to those matching text; the cursor goes to
// the first match unless its person still matches.
func (p *peopleState) setFilter(m *Model, text string) {
	p.filter = strings.TrimSpace(text)
	if !search.MatchName(p.filter, p.selected) {
		p.cursor = 0
	}
	p.refresh(m)
}

func (p *peopleState) move(m *Model, delta int) {
	if c := p.cursor + delta; c >= 0 && c < len(p.names) {
		p.cursor, p.selected, p.tasks = c, p.names[c].name, rowList{rows: m.personRows(p.names[c].name)}
		p.tasks.clamp()
	}
}

// follow keeps both cursors inside their windows. Called after every update.
func (p *peopleState) follow(m *Model) {
	visible := m.visibleRows()
	p.offset = scroll(p.cursor, p.cursor, p.offset, visible, len(p.names))
	p.tasks.follow(visible)
}

// personRows lists name's tasks (D52): the open ones grouped by priority
// like the backlog, pinned ones too, oldest first within a group; then a
// closed heading over the done and dropped ones, newest first. Every task
// is a row of its own: a child shows ↳ #parent, a parent its progress.
func (m *Model) personRows(name string) []row {
	if name == "" {
		return nil
	}
	snap, now, headings := m.state.Snapshot, m.svc.Now(), m.priorityHeadings()
	groups := map[search.Group][]domain.Task{}
	var closed []domain.Task
	for _, t := range snap.Tasks() {
		switch {
		case t.Requester != name:
		case t.Status != domain.StatusOpen:
			closed = append(closed, t)
		default:
			g := search.GroupOf(search.ByPriority, t, snap.ParentOf(t), now, headings)
			groups[g] = append(groups[g], t)
		}
	}
	var rows []row
	add := func(t domain.Task) {
		r := row{kind: rowTask, task: t, person: true, orphan: t.ParentID}
		if c, ok := m.idx.cand[t.ID]; ok {
			r.cand = &c
		}
		rows = append(rows, r)
	}
	for _, g := range slices.SortedFunc(maps.Keys(groups), search.Compare) {
		slices.SortFunc(groups[g], search.Oldest)
		rows = append(rows, row{kind: rowHeader, text: g.Label})
		for _, t := range groups[g] {
			add(t)
		}
	}
	if len(closed) > 0 {
		slices.SortFunc(closed, func(a, b domain.Task) int {
			return cmp.Or(b.ClosedAt.Compare(a.ClosedAt), cmp.Compare(b.ID, a.ID))
		})
		rows = append(rows, row{kind: rowHeader, text: "closed"})
		for _, t := range closed {
			add(t)
		}
	}
	return rows
}

func (m *Model) peopleKey(key string) tea.Cmd {
	p := &m.people
	if n := m.steps(key); n != 0 {
		if p.onTasks {
			repeat(n, p.tasks.move)
		} else {
			repeat(n, func(delta int) { p.move(m, delta) })
		}
		return nil
	}
	switch key {
	case "tab":
		p.onTasks = !p.onTasks
		return nil
	case "/":
		return m.openPeopleFilter()
	case "esc":
		if p.filter != "" {
			p.setFilter(m, "")
			m.setStatus("filter cleared")
			return nil
		}
		m.screen = p.from
		return nil
	}
	if !p.onTasks {
		if key == "enter" && p.selected != "" {
			p.onTasks = true // a person's tasks
		}
		return nil
	}
	r, ok := p.tasks.selectedRow()
	if !ok {
		return nil
	}
	if key == "enter" {
		m.openDetail(r.task.ID) // D31
		return nil
	}
	cmd, _ := m.taskKey(key, r.task)
	return cmd
}

func (m *Model) peopleView() string {
	total := max(m.width-3, 20)
	lw := total / 3
	rw := total - lw
	p := &m.people
	title, empty := fmt.Sprintf("PEOPLE · %d", len(m.idx.people)), "nobody yet · @who on a task names who asked"
	if p.filter != "" {
		title = "PEOPLE · " + plural(len(p.names), "match", "matches") + " · " + p.filter
		empty = "no matches · esc clears"
	}
	left := []string{styleTitle.Render(title)}
	if len(p.names) == 0 {
		left = append(left, styleDim.Render("  "+empty))
	}
	visible := m.visibleRows()
	for i := p.offset; i < len(p.names) && i < p.offset+visible; i++ {
		left = append(left, m.personLine(p.names[i], lw, i == p.cursor))
	}
	right := m.listLines(p.tasks, visible, "@"+p.selected, "", rw, p.onTasks)
	if p.selected == "" {
		right = nil
	}
	return columns(left, right, lw, rw)
}

// personLine renders `ana · 3 open`; a person with nothing open is dimmed.
// The cursor's person stays marked while the tasks pane has the focus.
func (m *Model) personLine(pr person, width int, cursor bool) string {
	line := pad(fmt.Sprintf("  %s · %d open", pr.name, pr.open), width)
	switch {
	case cursor && !m.people.onTasks:
		return styleSelected.Render(line)
	case cursor:
		return styleTitle.Render(line) // whose tasks the right pane shows
	case pr.open == 0:
		return styleDim.Render(line) // still listed, for cleaning up names
	}
	return line
}

func (m *Model) peopleHints() string {
	back := "esc back"
	if m.people.filter != "" {
		back = "esc clears filter"
	}
	if m.people.onTasks {
		return "⏎ open · e edit · p pin · u unpin · x done · d drop · tab people · " + back + " · a add · ctrl+z undo · ? help"
	}
	return "j/k move · ⏎/tab tasks · / filter · " + back + " · a add · ctrl+z undo · ? help · q quit"
}

var peopleHelp = []string{
	"j k      move · tab switches people / tasks · ⏎ on a person: their tasks",
	"/        filter the names as you type (esc in the line or on the list clears it)",
	"⏎        task detail · e / E edit the line / the note in $EDITOR · o open the link",
	"p / u    pin to today / unpin",
	"x        done (again: reopen)",
	"d / D    drop with optional reason / delete",
	"esc      back to where you came from",
}

// peopleFilter narrows People's names as you type (D52): enter keeps the
// filter, esc clears it.
type peopleFilter struct{ input textinput.Model }

func (m *Model) openPeopleFilter() tea.Cmd {
	in := newInput("people › ", "name", m.width)
	in.SetValue(m.people.filter)
	in.CursorEnd()
	m.people.onTasks = false
	m.overlay = &peopleFilter{input: in}
	return nil
}

func (f *peopleFilter) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.overlay = nil
		m.people.setFilter(m, "")
		return nil
	case "enter":
		m.overlay = nil
		return nil
	}
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(k)
	m.people.setFilter(m, f.input.Value())
	return cmd
}

func (f *peopleFilter) paste(m *Model, msg tea.PasteMsg) tea.Cmd {
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	m.people.setFilter(m, f.input.Value())
	return cmd
}

func (f *peopleFilter) resize(width int) { fitInput(&f.input, width) }

func (f *peopleFilter) view(*Model) string { return f.input.View() }
```

Patch `internal/tui/pick.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/pick.go b/internal/tui/pick.go
index d6b0b44..d51f668 100644
--- a/internal/tui/pick.go
+++ b/internal/tui/pick.go
@@ -39,6 +39,7 @@ type row struct {
 	parent   bool              // has children
 	expanded bool              // a parent whose children are listed beneath it
 	orphan   int64             // child listed top-level because this parent isn't shown in the backlog
+	person   bool              // listed under its requester in People: no @who, 📌 when pinned, the close date
 }
 
 // groupKey is the ui_state key that remembers the backlog grouping (D26).
@@ -69,6 +70,7 @@ func (m *Model) refreshRows() {
 	m.pick.clamp(m)
 	m.today.refresh(m)
 	m.lane.refresh(m)
+	m.people.refresh(m)
 	if len(m.detail.stack) > 0 {
 		m.detail.refresh(m)
 	}
@@ -108,11 +110,7 @@ func (m *Model) backlogRows() []row {
 			listed[t.ID] = true
 		}
 	}
-	levels := m.svc.Config().Priorities
-	headings := make([]string, len(levels))
-	for i, l := range levels {
-		headings[i] = l.Name + " · " + l.Definition
-	}
+	headings := m.priorityHeadings()
 	groups := map[search.Group][]domain.Task{}
 	orphanOf := map[int64]int64{}
 	for _, t := range snap.Tasks() {
@@ -157,6 +155,17 @@ func (m *Model) backlogRows() []row {
 	return rows
 }
 
+// priorityHeadings are the priority groups' headings, one per configured
+// level: "P1 · someone is blocked" (D26).
+func (m *Model) priorityHeadings() []string {
+	levels := m.svc.Config().Priorities
+	headings := make([]string, len(levels))
+	for i, l := range levels {
+		headings[i] = l.Name + " · " + l.Definition
+	}
+	return headings
+}
+
 // todayRows: today's candidates in plan order. A pinned parent lists its open
 // children and children closed today beneath it; a pinned child of a pinned
 // parent appears only there.
@@ -195,49 +204,88 @@ func rolloverHint(t domain.Task) row {
 
 func (m *Model) rows(p pane) []row { return m.pick.rows[p] }
 
-// clamp keeps each pane's cursor on a selectable row. It prefers the task the
-// cursor was on (so a reload that reorders rows keeps the highlight on that
-// task) and falls back to the old index when that task is gone.
+// clamp keeps each pane's cursor on a selectable row (see clampRows).
 func (p *pickState) clamp(m *Model) {
 	if p.expanded == nil {
 		p.expanded = map[int64]bool{}
 	}
 	for _, pn := range []pane{paneBacklog, paneToday} {
-		rows := p.rows[pn]
-		c := -1
-		if id := p.selected[pn]; id != 0 {
-			c = slices.IndexFunc(rows, func(r row) bool { return r.kind == rowTask && r.task.ID == id })
+		p.cursor[pn], p.selected[pn] = clampRows(p.rows[pn], p.cursor[pn], p.selected[pn])
+	}
+}
+
+// clampRows puts a cursor on a task row of rows. It prefers task id, the
+// one the cursor was on (so a reload that reorders rows keeps the highlight
+// on that task), and falls back to the old index when that task is gone.
+// It returns the cursor and the task id under it (0 = none).
+func clampRows(rows []row, cursor int, id int64) (int, int64) {
+	c := -1
+	if id != 0 {
+		c = slices.IndexFunc(rows, func(r row) bool { return r.kind == rowTask && r.task.ID == id })
+	}
+	if c < 0 {
+		c = min(cursor, len(rows)-1)
+		for c >= 0 && rows[c].kind != rowTask {
+			c--
 		}
 		if c < 0 {
-			c = min(p.cursor[pn], len(rows)-1)
-			for c >= 0 && rows[c].kind != rowTask {
-				c--
-			}
-			if c < 0 {
-				c = slices.IndexFunc(rows, func(r row) bool { return r.kind == rowTask })
-			}
-			c = max(c, 0)
-		}
-		p.cursor[pn] = c
-		p.selected[pn] = 0
-		if c < len(rows) && rows[c].kind == rowTask {
-			p.selected[pn] = rows[c].task.ID
+			c = slices.IndexFunc(rows, func(r row) bool { return r.kind == rowTask })
 		}
+		c = max(c, 0)
+	}
+	if c < len(rows) && rows[c].kind == rowTask {
+		return c, rows[c].task.ID
 	}
+	return c, 0
 }
 
 func (p *pickState) move(delta int) { p.moveIn(p.focus, delta) }
 
 // moveIn moves pane pn's cursor to the next task row in direction delta.
 func (p *pickState) moveIn(pn pane, delta int) {
-	rows := p.rows[pn]
-	for c := p.cursor[pn] + delta; c >= 0 && c < len(rows); c += delta {
+	if c, ok := stepRows(p.rows[pn], p.cursor[pn], delta); ok {
+		p.cursor[pn], p.selected[pn] = c, p.rows[pn][c].task.ID
+	}
+}
+
+// stepRows finds the next task row after c in direction delta; ok is false
+// at the list's end.
+func stepRows(rows []row, c, delta int) (int, bool) {
+	for c += delta; c >= 0 && c < len(rows); c += delta {
 		if rows[c].kind == rowTask {
-			p.cursor[pn] = c
-			p.selected[pn] = rows[c].task.ID
-			return
+			return c, true
 		}
 	}
+	return 0, false
+}
+
+// rowList is a list of rows with its own cursor and scroll window, like
+// one of the Log's panes (People's tasks, D52).
+type rowList struct {
+	rows     []row
+	cursor   int
+	offset   int   // first row shown
+	selected int64 // task id under the cursor (0 = none); survives reloads
+}
+
+func (l *rowList) clamp() { l.cursor, l.selected = clampRows(l.rows, l.cursor, l.selected) }
+
+func (l *rowList) move(delta int) {
+	if c, ok := stepRows(l.rows, l.cursor, delta); ok {
+		l.cursor, l.selected = c, l.rows[c].task.ID
+	}
+}
+
+func (l *rowList) follow(visible int) {
+	l.offset = scroll(headed(l.rows, l.cursor), l.cursor, l.offset, visible, len(l.rows))
+}
+
+// selectedRow is the task row under the cursor.
+func (l *rowList) selectedRow() (row, bool) {
+	if l.cursor < 0 || l.cursor >= len(l.rows) || l.rows[l.cursor].kind != rowTask {
+		return row{}, false
+	}
+	return l.rows[l.cursor], true
 }
 
 // saveGroup remembers the grouping (D26). One save runs at a time; a g
@@ -258,18 +306,29 @@ func (m *Model) saveGroup() tea.Cmd {
 // comes along. Called after every update.
 func (p *pickState) follow(m *Model) {
 	for pn, rows := range p.rows {
-		visible := m.paneVisible(pane(pn))
-		c, off := p.cursor[pn], p.offset[pn]
-		top := c
-		if top > 0 && top < len(rows) && rows[top-1].kind == rowHeader {
-			top--
-		}
-		off = min(off, top)
-		if c >= off+visible {
-			off = c - visible + 1
-		}
-		p.offset[pn] = max(0, min(off, len(rows)-visible))
+		c := p.cursor[pn]
+		p.offset[pn] = scroll(headed(rows, c), c, p.offset[pn], m.paneVisible(pane(pn)), len(rows))
+	}
+}
+
+// headed is the first row that shows with the cursor's row c: the group
+// heading right above it, if there is one.
+func headed(rows []row, c int) int {
+	if c > 0 && c < len(rows) && rows[c-1].kind == rowHeader {
+		return c - 1
+	}
+	return c
+}
+
+// scroll is the first line of a window of visible lines out of n that keeps
+// lines top..c in view (c is the cursor's), given the window started at off:
+// it moves only when they leave it.
+func scroll(top, c, off, visible, n int) int {
+	off = min(off, top)
+	if c >= off+visible {
+		off = c - visible + 1
 	}
+	return max(0, min(off, n-visible))
 }
 
 // visibleRows is how many rows fit in a pane below its title (the Log's
@@ -342,6 +401,9 @@ func (m *Model) pickKey(key string) tea.Cmd {
 	case "i":
 		m.openInbox()
 		return nil
+	case "P":
+		m.openPeople()
+		return nil
 	case "B":
 		m.screen, m.today.focus = screenToday, focusCandidates // Pick done: block in Today (D35)
 		return nil
@@ -518,16 +580,19 @@ func columns(left, right []string, lw, rw int) string {
 
 // paneLines renders pane pn's title and visible rows; focused shows its cursor.
 func (m *Model) paneLines(pn pane, title, empty string, width int, focused bool) []string {
+	l := rowList{rows: m.rows(pn), cursor: m.pick.cursor[pn], offset: m.pick.offset[pn]}
+	return m.listLines(l, m.paneVisible(pn), title, empty, width, focused)
+}
+
+// listLines renders a list's title and the rows in its window of visible
+// rows; focused shows its cursor.
+func (m *Model) listLines(l rowList, visible int, title, empty string, width int, focused bool) []string {
 	lines := []string{styleTitle.Render(title)}
-	rows := m.rows(pn)
-	if len(rows) == 0 {
+	if len(l.rows) == 0 {
 		return append(lines, styleDim.Render("  "+empty))
 	}
-	visible := m.paneVisible(pn)
-	cursor := m.pick.cursor[pn]
-	start := m.pick.offset[pn]
-	for i := start; i < len(rows) && i < start+visible; i++ {
-		lines = append(lines, m.rowLine(rows[i], width, focused && i == cursor))
+	for i := l.offset; i < len(l.rows) && i < l.offset+visible; i++ {
+		lines = append(lines, m.rowLine(l.rows[i], width, focused && i == l.cursor))
 	}
 	return lines
 }
@@ -555,7 +620,7 @@ func (m *Model) rowLine(r row, width int, selected bool) string {
 	if t.Note != "" {
 		b.WriteString(" ✎")
 	}
-	if t.Requester != "" {
+	if t.Requester != "" && !r.person {
 		b.WriteString(" @" + t.Requester)
 	}
 	if kids := m.idx.children[t.ID]; len(kids) > 0 {
@@ -581,8 +646,11 @@ func (m *Model) rowLine(r row, width int, selected bool) string {
 	if t.RolloverCount > 0 {
 		fmt.Fprintf(&b, " ↻%d", t.RolloverCount)
 	}
-	if c := r.cand; c != nil && c.PinnedDays > 1 {
+	switch c := r.cand; {
+	case c != nil && c.PinnedDays > 1:
 		fmt.Fprintf(&b, " 📌%d", c.PinnedDays)
+	case c != nil && r.person:
+		b.WriteString(" 📌") // People lists unpinned tasks too
 	}
 	switch t.Status {
 	case domain.StatusDone:
@@ -590,6 +658,9 @@ func (m *Model) rowLine(r row, width int, selected bool) string {
 	case domain.StatusDropped:
 		b.WriteString(" ✗")
 	}
+	if t.Status != domain.StatusOpen && r.person {
+		b.WriteString(" " + dayLabel(m.svc.DayOf(t.ClosedAt)))
+	}
 	if r.orphan != 0 {
 		suffix := fmt.Sprintf(" ↳ #%d", r.orphan)
 		if !selected {
```

Patch `internal/tui/today.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/today.go b/internal/tui/today.go
index b64f0b7..cb9848b 100644
--- a/internal/tui/today.go
+++ b/internal/tui/today.go
@@ -420,6 +420,9 @@ func (m *Model) todayKey(key string) tea.Cmd {
 	case "i":
 		m.openInbox()
 		return nil
+	case "P":
+		m.openPeople()
+		return nil
 	}
 	if td.focus == focusCandidates {
 		if n := m.steps(key); n != 0 {
@@ -708,9 +711,9 @@ func (m *Model) todayHints() string {
 		return fmt.Sprintf("%s %q · j/k move · ⏎ drop · esc cancel", verb, c.task.Title)
 	}
 	if m.today.focus == focusCandidates {
-		return "b block · ⏎ open · u unpin · x done · d drop · e edit · c commit · tab timeline · L log · i inbox · a add · ctrl+z undo · ? help · q quit"
+		return "b block · ⏎ open · u unpin · x done · d drop · e edit · c commit · tab timeline · L log · P people · i inbox · a add · ctrl+z undo · ? help · q quit"
 	}
-	hints := "j/k move · m move · +/- size · f fine-tune · u unblock · ⏎ open · x done · c commit · tab candidates · L log · i inbox · ctrl+z undo · ? help"
+	hints := "j/k move · m move · +/- size · f fine-tune · u unblock · ⏎ open · x done · c commit · tab candidates · L log · P people · i inbox · ctrl+z undo · ? help"
 	if b, ok := m.today.tl.blockAt(m.today.cursor); ok && m.conflicted(b) {
 		hints = "r re-slot ⚠ · " + hints
 	}
@@ -731,6 +734,6 @@ var todayHelp = []string{
 	"x d D    done (again: reopen) / drop / delete",
 	"e / E    edit the line / the note in $EDITOR · o open the link",
 	"c        commit the plan (again: refresh)",
-	"L        the log (backlog; pick today's work there, B comes back) · i inbox",
+	"L        the log (backlog; pick today's work there, B comes back) · i inbox · P people",
 	"w        the Reviews lane: p pins a review (a task Review #n), o opens the PR · w or tab back",
 }
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/search/ ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`
Expected: PASS (the Log's existing tests cover the `pick.go` refactor), no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/search/search.go internal/search/search_test.go internal/tui/common.go internal/tui/model.go internal/tui/overlays.go internal/tui/people.go internal/tui/people_test.go internal/tui/pick.go internal/tui/today.go
git commit -m 'feat(tui): the People view lists requesters and their tasks; P opens it, / filters names (D52)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 3: `r` renames a person or merges them into another (prompt, confirmation, undo)

D53 in the TUI. `r` in either People pane opens `rename @raf › ` prefilled with the name, on the existing `prompt` overlay: `dsl.ParseRequester` checks it on enter and a refusal keeps the text with the reason under it. The same name says `unchanged`. A name someone already has asks `"ana" already exists · ⏎ merge into them · esc` (the D37 `nudge`: any other key says `cancelled`). Then `RenameRequester` runs; the status says `renamed raf → raff · 2 tasks` or `merged raf → ana · 2 tasks`, the cursor follows the new name (a filter that would hide it is cleared), and `ctrl+z` undoes it (`undid merge raf → ana`, or `can't undo merge raf → ana: it changed since`). A failed rename (the name went away meanwhile) shows the error and leaves the cursor on a real row.

**Files:**
- Modify: `internal/tui/people.go`
- Create: `internal/tui/rename_test.go`

**Interfaces:**
- Consumes: Task 1 (`RenameRequester`, `dsl.ParseRequester`), Task 2 (`peopleState`, `peopleKey`, `peopleHints`, `peopleHelp`, `idx.people`, `search.MatchName`, `h.before`), existing `prompt` (`check`, `onEnter`), `nudge`, `actionMsg.restore`.
- Produces: `(*Model).openRename(from string) tea.Cmd`; `(*Model).rename(from, to, verb, done string) tea.Cmd` (label `rename raf → raff` / `merge raf → ana`); `r` in both People panes (`nobody to rename` when the list is empty).

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/rename_test.go`:

```go
package tui

import (
	"context"
	"testing"
	"time"

	"tt/internal/config"
	"tt/internal/domain"
)

func TestRenameAPerson(t *testing.T) {
	h := newHarness(t, config.Default())
	one := h.add("One @raf !1 ~s")
	two := h.add("Two @raf !2 ~s")
	h.add("Ana's @ana !1 ~s")
	h.keys("P", "j")
	h.keys("r")
	h.mustSee("rename @raf › raf")
	h.keys("enter")
	h.mustSee("unchanged")

	h.keys("r", "ctrl+u", "enter")
	h.mustSee("✗ a name can't be empty")
	h.typeText("two words")
	h.keys("enter")
	h.mustSee("✗ a name is one word, like @ana")
	if h.m.overlay == nil {
		t.Fatal("a refused name keeps the prompt open with its text")
	}
	h.keys("ctrl+u")
	h.typeText(" @Raff, ")
	h.keys("enter")
	h.mustSee("renamed raf → raff · 2 tasks", "raff · 2 open", "@raff", "One", "Two")
	h.mustNotSee("raf · ")
	if h.m.people.selected != "raff" {
		t.Errorf("the cursor follows the new name, got %q", h.m.people.selected)
	}
	if a, b := h.task(one.ID).Requester, h.task(two.ID).Requester; a != "raff" || b != "raff" {
		t.Errorf("requesters = %q, %q", a, b)
	}

	h.keys("ctrl+z")
	h.mustSee("undid rename raf → raff", "raf · 2 open")
	h.mustNotSee("raff · ")
}

func TestMergeIntoAnotherPerson(t *testing.T) {
	h := newHarness(t, config.Default())
	ctx := context.Background()
	one := h.add("One @raf !1 ~s")
	two := h.add("Two @raf !2 ~s")
	own := h.add("Ana's @ana !1 ~s")
	merge := func() {
		t.Helper()
		h.keys("r", "ctrl+u")
		h.typeText("Ana")
		h.keys("enter")
		h.mustSee(`"ana" already exists · ⏎ merge into them · esc`)
	}
	h.keys("P", "j", "tab") // r works from the tasks pane too
	merge()
	h.keys("esc")
	h.mustSee("cancelled", "raf · 2 open")

	merge()
	h.keys("enter")
	h.mustSee("merged raf → ana · 2 tasks", "ana · 3 open", "One", "Two", "Ana's")
	h.mustNotSee("raf · ")
	if h.m.people.selected != "ana" {
		t.Errorf("the cursor follows the merged name, got %q", h.m.people.selected)
	}

	h.keys("ctrl+z")
	h.mustSee("undid merge raf → ana", "ana · 1 open", "raf · 2 open")
	if got := h.task(own.ID).Requester; got != "ana" {
		t.Errorf("the merge's undo moved ana's own task to %q", got)
	}

	if h.m.people.selected != "ana" {
		t.Errorf("after the undo the cursor stays on ana, who is still there: %q", h.m.people.selected)
	}
	h.keys("tab", "j") // the names pane, raf
	merge()
	h.keys("enter")
	h.clock.now = h.clock.now.Add(time.Minute)
	if _, err := h.svc.SetSize(ctx, one.ID, domain.SizeM); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "ctrl+z")
	h.mustSee("can't undo merge raf → ana: it changed since")
	if a, b := h.task(one.ID).Requester, h.task(two.ID).Requester; a != "ana" || b != "ana" {
		t.Errorf("a refused undo changes nothing: %q, %q", a, b)
	}
}

func TestRenameWhileFiltered(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("One @raf !1 ~s")
	h.add("Ana's @ana !1 ~s")
	h.keys("P", "/")
	h.typeText("ra")
	h.keys("enter")
	h.mustSee("PEOPLE · 1 match · ra")

	h.keys("r", "ctrl+u")
	h.typeText("raff")
	h.keys("enter")
	h.mustSee("renamed raf → raff · 1 task", "PEOPLE · 1 match · ra", "@raff") // still matches: the filter stays

	h.keys("r", "ctrl+u")
	h.typeText("bob")
	h.keys("enter")
	h.mustSee("renamed raff → bob · 1 task", "PEOPLE · 2", "@bob") // would be hidden: the filter goes
	if h.m.people.selected != "bob" {
		t.Errorf("the cursor follows the new name, got %q", h.m.people.selected)
	}
}

func TestRenameOfANameGoneMeanwhile(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("One @raf !1 ~s")
	h.add("Ana's @ana !1 ~s")
	h.keys("P", "j")
	if _, _, err := h.svc.RenameRequester(context.Background(), "raf", "zed"); err != nil {
		t.Fatal(err) // another shell; the TUI hasn't reloaded yet
	}
	h.keys("r", "ctrl+u")
	h.typeText("raff")
	h.keys("enter")
	h.mustSee("✗ no task has @raf any more", "zed · 1 open")
	if h.m.people.selected == "raff" {
		t.Error("a failed rename doesn't leave the cursor on a name nobody has")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/ -run 'Rename|Merge'`
Expected: FAIL: `TestRenameAPerson` (`screen lacks "rename @raf › raf"`), `TestMergeIntoAnotherPerson` (`screen lacks "\"ana\" already exists · ⏎ merge into them · esc"`), `TestRenameWhileFiltered` (`screen lacks "renamed raf → raff · 1 task"`), `TestRenameOfANameGoneMeanwhile` (`screen lacks "✗ no task has @raf any more"`).

- [ ] **Step 3: Implement**

Patch `internal/tui/people.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/people.go b/internal/tui/people.go
index a080a20..655e64d 100644
--- a/internal/tui/people.go
+++ b/internal/tui/people.go
@@ -11,6 +11,7 @@ import (
 	tea "charm.land/bubbletea/v2"
 
 	"tt/internal/domain"
+	"tt/internal/dsl"
 	"tt/internal/search"
 )
 
@@ -154,6 +155,12 @@ func (m *Model) peopleKey(key string) tea.Cmd {
 		return nil
 	case "/":
 		return m.openPeopleFilter()
+	case "r":
+		if p.selected == "" {
+			m.setStatus("nobody to rename")
+			return nil
+		}
+		return m.openRename(p.selected)
 	case "esc":
 		if p.filter != "" {
 			p.setFilter(m, "")
@@ -206,6 +213,50 @@ func (m *Model) peopleView() string {
 	return columns(left, right, lw, rw)
 }
 
+// openRename asks for a person's new name (D53), prefilled with the old one.
+// A name someone else already has asks first, then merges the two.
+func (m *Model) openRename(from string) tea.Cmd {
+	in := newInput("rename @"+from+" › ", "new name", m.width)
+	in.SetValue(from)
+	in.CursorEnd()
+	m.overlay = &prompt{
+		input: in,
+		check: func(v string) error { _, err := dsl.ParseRequester(v); return err },
+		onEnter: func(v string) tea.Cmd {
+			to, _ := dsl.ParseRequester(v) // check passed
+			switch {
+			case to == from:
+				m.setStatus("unchanged")
+				return nil
+			case slices.ContainsFunc(m.idx.people, func(pr person) bool { return pr.name == to }):
+				m.overlay = &nudge{
+					question: fmt.Sprintf("%q already exists · ⏎ merge into them · esc", to),
+					onEnter:  func() tea.Cmd { return m.rename(from, to, "merge", "merged") },
+				}
+				return nil
+			}
+			return m.rename(from, to, "rename", "renamed")
+		},
+	}
+	return nil
+}
+
+// rename gives from's tasks the name to. The cursor follows the new name,
+// so a filter that would hide it is cleared; on an error it stays on from.
+func (m *Model) rename(from, to, verb, done string) tea.Cmd {
+	p := &m.people
+	p.selected = to
+	if !search.MatchName(p.filter, to) {
+		p.filter = ""
+	}
+	svc, ctx, label := m.svc, m.ctx, verb+" "+from+" → "+to
+	return func() tea.Msg {
+		n, undo, err := svc.RenameRequester(ctx, from, to)
+		return actionMsg{label: label, status: fmt.Sprintf("%s %s → %s · %s", done, from, to, plural(n, "task", "tasks")),
+			undo: undo, err: err, restore: func(m *Model) { m.people.selected = from }}
+	}
+}
+
 // personLine renders `ana · 3 open`; a person with nothing open is dimmed.
 // The cursor's person stays marked while the tasks pane has the focus.
 func (m *Model) personLine(pr person, width int, cursor bool) string {
@@ -227,14 +278,15 @@ func (m *Model) peopleHints() string {
 		back = "esc clears filter"
 	}
 	if m.people.onTasks {
-		return "⏎ open · e edit · p pin · u unpin · x done · d drop · tab people · " + back + " · a add · ctrl+z undo · ? help"
+		return "⏎ open · e edit · p pin · u unpin · x done · d drop · r rename person · tab people · " + back + " · a add · ctrl+z undo · ? help"
 	}
-	return "j/k move · ⏎/tab tasks · / filter · " + back + " · a add · ctrl+z undo · ? help · q quit"
+	return "j/k move · ⏎/tab tasks · / filter · r rename or merge · " + back + " · a add · ctrl+z undo · ? help · q quit"
 }
 
 var peopleHelp = []string{
 	"j k      move · tab switches people / tasks · ⏎ on a person: their tasks",
 	"/        filter the names as you type (esc in the line or on the list clears it)",
+	"r        rename the person, on every task that names them; a name someone already has merges the two (⏎ confirms)",
 	"⏎        task detail · e / E edit the line / the note in $EDITOR · o open the link",
 	"p / u    pin to today / unpin",
 	"x        done (again: reopen)",
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/people.go internal/tui/rename_test.go
git commit -m 'feat(tui): r renames a person or merges them into another, with undo (D53)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---


## After the last task

- Build tt into the scratchpad and drive the real binary through a Python `pty` script on a throwaway database (`TT_DB`, `TT_CONFIG=/nonexistent`, a 120×30 window). Seed it first with `tt add`: `Fix login @raf !1 ~s`, `Write RFC @raf !2 ~m`, `Ana's ask @ana !1 ~s`, `Nobody asked !1 ~s`. Then: `esc` (banner); `P` → `People`, `PEOPLE · 2`, `ana · 1 open`, `raf · 2 open`; `j` → `Fix login`, `Write RFC`; `r` → `rename @raf`; `ctrl+u`, `Ana`, `⏎` → `already exists`; `⏎` → `merged raf → ana · 2 tasks`, `3 open`; `ctrl+z` → `undid merge` (the cursor stays on ana); `j`, `r` → `rename @raf`; `esc`; `r`, `ctrl+u`, `raff`, `⏎` → `renamed raf → raff · 2 tasks`; `esc` → `Today`; `q`. Then the database: tasks 1 and 2 `raff`, task 3 `ana`, task 4 empty. Match short substrings (the renderer repaints changed cells only).
- `go test -race ./internal/tui/ ./internal/app/`.
- Final whole-branch review, one fix wave, park residuals as spec carry-forwards.
- **The user's check:** `make install`, open tt, `P`: the names you've used, their open counts, a duplicate pair merged with `r` (and `ctrl+z` if it was wrong).
