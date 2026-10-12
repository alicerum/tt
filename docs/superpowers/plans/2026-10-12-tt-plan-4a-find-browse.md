# tt Plan 4a — Find & Browse Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** In Pick, `/` opens a live filter bar using the token DSL (`@ana #impl !1 ~l <7d is:done schema`) that searches all tasks, and `g` cycles the backlog grouping (priority → size → created → requester), remembered across restarts.

**Architecture:** `dsl.ParseFilter` turns the bar text into a `Filter`. A new pure package `internal/search` decides *which* tasks match (`Match`) and *which group* each lands in (`GroupOf`). `internal/tui` keeps the one placement rule (parent/child/orphan/expanded) in `backlogRows` and caches both panes' rows, rebuilt only on load, filter, grouping and expand/collapse. The grouping is stored in a new `ui_state` table through `app.Service`.

**Tech Stack:** Go 1.27; Bubble Tea v2 / bubbles v2 textinput / Lip Gloss v2 (already in go.mod); pure-Go SQLite; go-cmp.

> **Superseded after execution:** D26c was reversed: the filter searches the backlog only, never the Inbox or pinned tasks (commit "filter searches the backlog only"). Where this plan says "searches all tasks", "Inbox and pinned included" or `pinMark`, the spec wins.

**Spec:** `docs/superpowers/specs/2026-10-12-task-tracking-design.md` — D8a, D26, D26a–e, "Plan 4 split" (4a), carry-forward "from Plan 3 review" (Plan 4 items).

## Global Constraints

- No new dependencies: `go.mod` and `go.sum` stay unchanged.
- `internal/dsl` and `internal/search` are pure: no I/O, no `config` import (priority headings are passed in).
- `internal/tui` never writes to the store; it only calls `app.Service`. UI state is not undoable and never enters the undo stack.
- The filter is never persisted; the grouping is persisted under `ui_state` key `pick.group`; an unknown stored value means priority.
- Overlays that parse tokens validate synchronously and keep the text on error (Plan 3 carry-forward).
- Lines never wrap: every rendered line is truncated to the terminal width.
- Every task ends green on: `gofmt -l .` (no output), `go vet ./...`, `go test ./...`.
- Every commit message ends with a blank line and `Co-Authored-By: Claude <noreply@anthropic.com>`.
- Sandbox: save each patch to a file and `git apply <file>`; run plain, separate git commands (no `git -C`, no `cd … &&`). If `git apply` fails, stop: an earlier step went wrong.

## Review Focus

1. **`tt add` from another shell while the filter line is open**: the new task appears if it matches, and the typed filter stays. Pinned in Task 5 `TestReloadWhileFilteringKeepsTheFilter`.
2. **Half-typed tokens while filtering live** (`sch !`, a lone `@`): the list must not blank out mid-word. Pinned in Task 1 (`"schema @ # ! ~ < >"` case). Accepted for now: `@an` matches requesters exactly, so it shows 0 matches until the name is complete.
3. **A typo that doesn't parse** (`!9`, `is:later`): the last valid results stay, enter is refused, and nothing typed is lost. Pinned in Task 5 `TestFilterBarNarrowsLiveAndKeepsTypingOnError`.
4. **A matching child whose parent doesn't match** must not vanish: it is listed top-level with `↳ #parent`. Pinned in Task 4 `TestFilteredBacklogPlacement`.
5. **A stored grouping this binary doesn't know** (downgrade, hand edit) falls back to priority instead of an empty backlog. Pinned in Task 4 `TestUnknownRememberedGroupingFallsBackToPriority`.

## File Structure

```
internal/dsl/filter.go, filter_test.go          new: ParseFilter, Filter
internal/search/search.go, search_test.go       new: Match, GroupBy, Group, GroupOf, Compare, Oldest
internal/store/migrations/003_ui_state.sql      new: ui_state table
internal/store/uistate.go, uistate_test.go      new: Store.UIState, Tx.PutUIState
internal/store/hardening_test.go                modify: newest migration is now 3
internal/app/uistate.go, uistate_test.go        new: Service.UIState / SetUIState
internal/app/service.go                         modify: Store interface gains UIState
internal/tui/pick.go                            modify: placement rule, row cache, scroll window, g, esc, /
internal/tui/model.go                           modify: update wrapper + follow, grouping load/save, resize, filter line above the backlog
internal/tui/common.go                          modify: plural, hints
internal/tui/tokeninput.go                      new: shared token input (completion at the cursor)
internal/tui/overlays.go                        modify: quick-add on tokenInput, filterBar, fitInput, help
internal/tui/pick_test.go, filter_test.go       new
internal/tui/harness_test.go                    modify: "backspace" key
```

Work happens on branch `plan-4a` in a worktree; fast-forward merge to `main` when the final review is green.

---

### Task 1: Filter-bar parser

D26a, D26b, D26d: the filter bar reuses the token DSL; a lone sigil is ignored while typing.

**Files:**
- Create: `internal/dsl/filter.go`
- Create: `internal/dsl/filter_test.go`

**Interfaces:**
- Consumes: `dsl.trailingPunct`, `isDigits`, `isSize` (add.go); `domain.ParseSize`, `domain.NormalizeTags`.
- Produces:
  - `type dsl.Filter struct { Requesters []string; Tags []string; Priorities []int; Sizes []domain.Size; NewerThan, OlderThan time.Duration; Statuses []domain.Status; Words []string }` (nil `Statuses` = open only).
  - `func (dsl.Filter) Empty() bool`: the line had no tokens at all.
  - `func dsl.ParseFilter(line string, levels int) (dsl.Filter, error)`.

- [ ] **Step 1: Write the failing tests**

Create `internal/dsl/filter_test.go`:

```go
package dsl

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"tt/internal/domain"
)

func TestParseFilter(t *testing.T) {
	open, done, dropped := domain.StatusOpen, domain.StatusDone, domain.StatusDropped
	cases := []struct {
		line string
		want Filter
	}{
		{"", Filter{}},
		{"   ", Filter{}},
		{"@ana #impl !1 ~l <7d schema", Filter{Requesters: []string{"ana"}, Tags: []string{"impl"}, Priorities: []int{1},
			Sizes: []domain.Size{domain.SizeL}, NewerThan: 7 * day, Words: []string{"schema"}}},
		{"@Ana @bob @ana", Filter{Requesters: []string{"ana", "bob"}}},
		{"#B #a #b", Filter{Tags: []string{"a", "b"}}},
		{"!1 !2 !1 ~s ~x ~XL", Filter{Priorities: []int{1, 2}, Sizes: []domain.Size{domain.SizeS, domain.SizeXL}}},
		{">30d", Filter{OlderThan: 30 * day}},
		{"<2w >1d", Filter{NewerThan: 14 * day, OlderThan: day}},
		{"<7D", Filter{NewerThan: 7 * day}},
		{"is:done", Filter{Statuses: []domain.Status{done}}},
		{"is:Dropped IS:done", Filter{Statuses: []domain.Status{dropped, done}}},
		{"is:all is:open", Filter{Statuses: []domain.Status{open, done, dropped}}},
		{"Schema MIGR", Filter{Words: []string{"schema", "migr"}}},
		// Look-alikes stay words (D26d, as in D25).
		{"#482 ~/tmp this! <7 <d <7x", Filter{Words: []string{"#482", "~/tmp", "this!", "<7", "<d", "<7x"}}},
		// A lone sigil is a token still being typed: ignored, so live results don't flicker.
		{"schema @ # ! ~ < >", Filter{Words: []string{"schema"}}},
		{"@. #, foo:bar !x ~zz", Filter{Words: []string{"@.", "#,", "foo:bar", "!x", "~zz"}}},
		{"@ana, #photos.", Filter{Requesters: []string{"ana"}, Tags: []string{"photos"}}},
		{"https://x.io -- note", Filter{Words: []string{"https://x.io", "--", "note"}}},
	}
	for _, c := range cases {
		got, err := ParseFilter(c.line, 3)
		if err != nil {
			t.Errorf("ParseFilter(%q): %v", c.line, err)
			continue
		}
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("ParseFilter(%q) (-want +got):\n%s", c.line, diff)
		}
	}
}

func TestParseFilterErrors(t *testing.T) {
	cases := map[string]string{
		"x !4":     "priority !4 out of range: use !1 to !3",
		"x !0":     "priority !0 out of range",
		"<0d":      "<0d: age must be 1 to 10000 days or weeks",
		">99999w":  ">99999w: age must be",
		"is:":      "unknown status is:",
		"is:later": "unknown status is:later: use is:open, is:done, is:dropped or is:all",
	}
	for line, want := range cases {
		_, err := ParseFilter(line, 3)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseFilter(%q) error = %v; want it to contain %q", line, err, want)
		}
	}
}

func TestFilterEmpty(t *testing.T) {
	for line, want := range map[string]bool{"": true, "  ": true, "x": false, "is:open": false, "<1d": false} {
		f, err := ParseFilter(line, 3)
		if err != nil {
			t.Fatal(err)
		}
		if f.Empty() != want {
			t.Errorf("ParseFilter(%q).Empty() = %v; want %v", line, f.Empty(), want)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/dsl/`
Expected: FAIL: `undefined: ParseFilter`, `undefined: Filter`.

- [ ] **Step 3: Implement**

Create `internal/dsl/filter.go`:

```go
package dsl

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"tt/internal/domain"
)

// Filter is a parsed filter-bar line (D26a, D26b, D26d). Empty fields don't filter.
type Filter struct {
	Requesters []string        // any of; lowercase
	Tags       []string        // all of; normalized
	Priorities []int           // any of (effective priority)
	Sizes      []domain.Size   // any of
	NewerThan  time.Duration   // created less than this long ago; 0 = no bound
	OlderThan  time.Duration   // created more than this long ago; 0 = no bound
	Statuses   []domain.Status // any of; nil = open only
	Words      []string        // fuzzy words, lowercase; all must match
}

// Empty reports whether the line had no tokens at all.
func (f Filter) Empty() bool {
	return len(f.Requesters) == 0 && len(f.Tags) == 0 && len(f.Priorities) == 0 && len(f.Sizes) == 0 &&
		f.NewerThan == 0 && f.OlderThan == 0 && len(f.Statuses) == 0 && len(f.Words) == 0
}

const day = 24 * time.Hour

// ParseFilter parses a filter-bar line. levels is the number of configured
// priority levels; !n outside 1..levels is an error, like in ParseAdd.
//
//	@name        requester (repeats: any of)
//	#tag         tag, own or inherited (repeats: all of)
//	!n  ~size    effective priority / size (repeats: any of)
//	<7d  >30d    created within / more than N days (or w weeks) ago
//	is:open|done|dropped|all
//	a lone sigil (@ # ! ~ < >) is ignored: it's a token still being typed
//	anything else is a fuzzy word
func ParseFilter(line string, levels int) (Filter, error) {
	var f Filter
	var tags []string
	for _, tok := range strings.Fields(line) {
		orig := tok
		if len(tok) == 1 && strings.ContainsRune("@#!~<>", rune(tok[0])) {
			continue
		}
		if strings.ContainsRune("@#!~", rune(tok[0])) {
			stripped := strings.TrimRight(tok, trailingPunct)
			if len(stripped) <= 1 || (stripped[0] == '#' && isDigits(stripped[1:])) {
				f.Words = append(f.Words, strings.ToLower(orig))
				continue
			}
			tok = stripped
		}
		switch sigil, rest := tok[0], tok[1:]; {
		case sigil == '@':
			f.Requesters = appendNew(f.Requesters, strings.ToLower(rest))
		case sigil == '#' && !isDigits(rest):
			tags = append(tags, rest)
		case sigil == '!' && isDigits(rest):
			n, err := strconv.Atoi(rest)
			if err != nil || n < 1 || n > levels {
				return Filter{}, fmt.Errorf("priority %s out of range: use !1 to !%d", tok, levels)
			}
			f.Priorities = appendNew(f.Priorities, n)
		case sigil == '~' && isSize(rest):
			s, _ := domain.ParseSize(rest)
			f.Sizes = appendNew(f.Sizes, s)
		case (sigil == '<' || sigil == '>') && isAge(rest):
			age, err := parseAge(rest)
			if err != nil {
				return Filter{}, fmt.Errorf("%s: %w", tok, err)
			}
			if sigil == '<' {
				f.NewerThan = age
			} else {
				f.OlderThan = age
			}
		case strings.HasPrefix(strings.ToLower(tok), "is:"):
			st, err := parseStatus(tok[3:])
			if err != nil {
				return Filter{}, err
			}
			for _, s := range st {
				f.Statuses = appendNew(f.Statuses, s)
			}
		default:
			f.Words = append(f.Words, strings.ToLower(orig))
		}
	}
	f.Tags = nilIfEmpty(domain.NormalizeTags(tags))
	return f, nil
}

// isAge: digits followed by d or w ("7d", "2w").
func isAge(s string) bool {
	if len(s) < 2 {
		return false
	}
	unit := s[len(s)-1]
	return (unit == 'd' || unit == 'w' || unit == 'D' || unit == 'W') && isDigits(s[:len(s)-1])
}

func parseAge(s string) (time.Duration, error) {
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n < 1 || n > 10000 {
		return 0, fmt.Errorf("age must be 1 to 10000 days or weeks")
	}
	if unit := s[len(s)-1]; unit == 'w' || unit == 'W' {
		return time.Duration(n) * 7 * day, nil
	}
	return time.Duration(n) * day, nil
}

func parseStatus(s string) ([]domain.Status, error) {
	switch strings.ToLower(s) {
	case "open":
		return []domain.Status{domain.StatusOpen}, nil
	case "done":
		return []domain.Status{domain.StatusDone}, nil
	case "dropped":
		return []domain.Status{domain.StatusDropped}, nil
	case "all":
		return []domain.Status{domain.StatusOpen, domain.StatusDone, domain.StatusDropped}, nil
	}
	return nil, fmt.Errorf("unknown status is:%s: use is:open, is:done, is:dropped or is:all", s)
}

func appendNew[T comparable](s []T, v T) []T {
	if slices.Contains(s, v) {
		return s
	}
	return append(s, v)
}

func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/dsl/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/dsl/filter.go internal/dsl/filter_test.go
git commit -m "feat(dsl): filter-bar parser (D26a, D26b, D26d)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 2: `internal/search`: matching and grouping

D8a, D26, D26c, D26d: pure filter evaluation and backlog groups, table-tested.

**Files:**
- Create: `internal/search/search.go`
- Create: `internal/search/search_test.go`

**Interfaces:**
- Consumes: Task 1 `dsl.Filter`, `dsl.ParseFilter`; `domain.EffectivePriority`, `domain.EffectiveTags`, `domain.Sizes`.
- Produces:
  - `func search.Match(f dsl.Filter, t domain.Task, parent *domain.Task, now time.Time) bool`.
  - `type search.GroupBy string`; `ByPriority`, `BySize`, `ByCreated`, `ByRequester`; `var GroupBys []GroupBy` (the `g` cycle order); `func ParseGroupBy(s string) GroupBy` (unknown → `ByPriority`); `func (GroupBy) Next() GroupBy`.
  - `type search.Group struct { Rank int; Label string }`; `func Compare(a, b Group) int`; `func Oldest(a, b domain.Task) int`.
  - `func search.GroupOf(by GroupBy, t domain.Task, parent *domain.Task, now time.Time, priorities []string) Group` (`priorities[i]` is the heading for level i+1).

- [ ] **Step 1: Write the failing tests**

Create `internal/search/search_test.go`:

```go
package search

import (
	"testing"
	"time"

	"tt/internal/domain"
	"tt/internal/dsl"
)

var now = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

func filter(t *testing.T, line string) dsl.Filter {
	t.Helper()
	f, err := dsl.ParseFilter(line, 3)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestMatch(t *testing.T) {
	task := domain.Task{
		ID: 1, Title: "Review schema-migration for Photos", Requester: "ana", Tags: []string{"impl"},
		Priority: 2, Size: domain.SizeM, Status: domain.StatusOpen, CreatedAt: now.Add(-3 * 24 * time.Hour),
	}
	cases := map[string]bool{
		"":                  true,
		"@ana":              true,
		"@bob":              false,
		"@bob @ana":         true, // requesters: any of
		"#impl":             true,
		"#impl #infra":      false, // tags: all of
		"!2":                true,
		"!1 !2":             true,
		"!1":                false,
		"~m":                true,
		"~s ~l":             false,
		"<7d":               true,
		"<3d":               false, // exactly 3 days old is not "within 3 days"
		">2d":               true,
		">3d":               false,
		"<1w >1d":           true,
		"schema":            true,
		"SCHM":              true, // anchored subsequence of "schema"
		"migr pho":          true,
		"chema":             true,  // substring
		"cma":               false, // subsequence not anchored at a word start
		"rvw":               true,  // anchored in "review"
		"ana":               true,  // requester is searched
		"imp":               true,  // tags are searched
		"schema nope":       false, // every word must match
		"is:done":           false,
		"is:all":            true,
		"is:done is:open":   true,
		"photos @ana !2 ~m": true,
	}
	for line, want := range cases {
		if got := Match(filter(t, line), task, nil, now); got != want {
			t.Errorf("Match(%q) = %v; want %v", line, got, want)
		}
	}
}

func TestMatchChildUsesEffectiveFields(t *testing.T) {
	parent := domain.Task{ID: 1, Title: "Auth", Tags: []string{"auth"}, Priority: 1, Status: domain.StatusOpen, CreatedAt: now}
	child := domain.Task{ID: 2, Title: "Rollback plan", Tags: []string{"ops"}, Priority: 3, ParentID: 1, Status: domain.StatusOpen, CreatedAt: now}
	for line, want := range map[string]bool{"!1": true, "!3": false, "#auth #ops": true, "auth": true} {
		if got := Match(filter(t, line), child, &parent, now); got != want {
			t.Errorf("child Match(%q) = %v; want %v", line, got, want)
		}
	}
}

func TestMatchClosedTasks(t *testing.T) {
	done := domain.Task{Title: "Shipped", Status: domain.StatusDone, CreatedAt: now}
	dropped := domain.Task{Title: "Abandoned", Status: domain.StatusDropped, CreatedAt: now}
	if Match(filter(t, "shipped"), done, nil, now) {
		t.Error("done tasks are hidden by default (D26b)")
	}
	if !Match(filter(t, "is:done shipped"), done, nil, now) || Match(filter(t, "is:done"), dropped, nil, now) {
		t.Error("is:done shows only done tasks")
	}
	if !Match(filter(t, "is:dropped"), dropped, nil, now) {
		t.Error("is:dropped shows dropped tasks")
	}
}

func TestGroupOf(t *testing.T) {
	levels := []string{"P1 · blocked", "P2 · soon", "P3 · later"}
	days := func(n int) time.Time { return now.Add(-time.Duration(n) * 24 * time.Hour) }
	parent := domain.Task{ID: 1, Priority: 2}
	cases := []struct {
		by     GroupBy
		task   domain.Task
		parent *domain.Task
		want   Group
	}{
		{ByPriority, domain.Task{Priority: 1}, nil, Group{1, "P1 · blocked"}},
		{ByPriority, domain.Task{Priority: 3, ParentID: 1}, &parent, Group{2, "P2 · soon"}},
		{ByPriority, domain.Task{Priority: 5}, nil, Group{4, "P? · above the configured levels"}},
		{ByPriority, domain.Task{}, nil, Group{5, "unset"}},
		{BySize, domain.Task{Size: domain.SizeS}, nil, Group{0, "S"}},
		{BySize, domain.Task{Size: domain.SizeXL}, nil, Group{3, "XL"}},
		{BySize, domain.Task{}, nil, Group{4, "unsized"}},
		{ByCreated, domain.Task{CreatedAt: now}, nil, Group{0, "this week"}},
		{ByCreated, domain.Task{CreatedAt: days(7)}, nil, Group{1, "this month"}},
		{ByCreated, domain.Task{CreatedAt: days(29)}, nil, Group{1, "this month"}},
		{ByCreated, domain.Task{CreatedAt: days(30)}, nil, Group{2, "older"}},
		{ByCreated, domain.Task{CreatedAt: days(90)}, nil, Group{3, "ancient"}},
		{ByRequester, domain.Task{Requester: "ana"}, nil, Group{0, "@ana"}},
		{ByRequester, domain.Task{}, nil, Group{1, "no requester"}},
	}
	for _, c := range cases {
		if got := GroupOf(c.by, c.task, c.parent, now, levels); got != c.want {
			t.Errorf("GroupOf(%s, %+v) = %+v; want %+v", c.by, c.task, got, c.want)
		}
	}
	if Compare(Group{0, "@bob"}, Group{0, "@ana"}) <= 0 || Compare(Group{0, "@zed"}, Group{1, "no requester"}) >= 0 {
		t.Error("groups sort by rank, then label")
	}
}

func TestGroupByCycle(t *testing.T) {
	g := ByPriority
	var seen []GroupBy
	for range GroupBys {
		g = g.Next()
		seen = append(seen, g)
	}
	want := []GroupBy{BySize, ByCreated, ByRequester, ByPriority}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("cycle = %v; want %v", seen, want)
		}
	}
	for s, want := range map[string]GroupBy{"size": BySize, "requester": ByRequester, "": ByPriority, "bogus": ByPriority} {
		if got := ParseGroupBy(s); got != want {
			t.Errorf("ParseGroupBy(%q) = %s; want %s", s, got, want)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/search/`
Expected: FAIL: the package has no non-test Go files (`Match`, `GroupOf` … undefined).

- [ ] **Step 3: Implement**

Create `internal/search/search.go`:

```go
// Package search evaluates filters (D26a–D26d) and groups tasks for the
// backlog (D26) over an in-memory snapshot. It does no I/O.
package search

import (
	"cmp"
	"slices"
	"strings"
	"time"
	"unicode"

	"tt/internal/domain"
	"tt/internal/dsl"
)

// Match reports whether t passes f at now. parent is t's parent (nil if none):
// children are matched on their effective priority and tags.
func Match(f dsl.Filter, t domain.Task, parent *domain.Task, now time.Time) bool {
	statuses := f.Statuses
	if statuses == nil {
		statuses = []domain.Status{domain.StatusOpen}
	}
	if !slices.Contains(statuses, t.Status) {
		return false
	}
	if len(f.Requesters) > 0 && !slices.Contains(f.Requesters, t.Requester) {
		return false
	}
	tags := domain.EffectiveTags(t, parent)
	for _, want := range f.Tags {
		if !slices.Contains(tags, want) {
			return false
		}
	}
	if len(f.Priorities) > 0 && !slices.Contains(f.Priorities, domain.EffectivePriority(t, parent)) {
		return false
	}
	if len(f.Sizes) > 0 && !slices.Contains(f.Sizes, t.Size) {
		return false
	}
	age := now.Sub(t.CreatedAt)
	if f.NewerThan > 0 && age >= f.NewerThan {
		return false
	}
	if f.OlderThan > 0 && age <= f.OlderThan {
		return false
	}
	if len(f.Words) == 0 {
		return true
	}
	hay := []string{strings.ToLower(t.Title), t.Requester}
	hay = append(hay, tags...)
	for _, w := range f.Words {
		if !slices.ContainsFunc(hay, func(h string) bool { return wordMatches(w, h) }) {
			return false
		}
	}
	return true
}

// wordMatches: w (lowercase) is a substring of hay, or a subsequence of one of
// hay's words that starts at that word's first letter ("schm" → "schema").
func wordMatches(w, hay string) bool {
	if strings.Contains(hay, w) {
		return true
	}
	for _, word := range strings.FieldsFunc(hay, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if anchoredSubsequence([]rune(w), []rune(word)) {
			return true
		}
	}
	return false
}

func anchoredSubsequence(w, word []rune) bool {
	if len(word) == 0 || w[0] != word[0] {
		return false
	}
	i := 0
	for _, r := range word {
		if i < len(w) && r == w[i] {
			i++
		}
	}
	return i == len(w)
}

// GroupBy is a backlog grouping (D26).
type GroupBy string

const (
	ByPriority  GroupBy = "priority"
	BySize      GroupBy = "size"
	ByCreated   GroupBy = "created"
	ByRequester GroupBy = "requester"
)

// GroupBys lists the groupings in the order `g` cycles through them.
var GroupBys = []GroupBy{ByPriority, BySize, ByCreated, ByRequester}

// ParseGroupBy returns the grouping named s; anything unknown is ByPriority.
func ParseGroupBy(s string) GroupBy {
	if g := GroupBy(s); slices.Contains(GroupBys, g) {
		return g
	}
	return ByPriority
}

// Next is the grouping after g in the `g` cycle.
func (g GroupBy) Next() GroupBy {
	i := slices.Index(GroupBys, g)
	return GroupBys[(i+1)%len(GroupBys)]
}

// Group names one backlog heading. Groups sort by Rank, then Label.
type Group struct {
	Rank  int
	Label string
}

// Compare orders groups for display.
func Compare(a, b Group) int {
	return cmp.Or(cmp.Compare(a.Rank, b.Rank), cmp.Compare(a.Label, b.Label))
}

// Oldest orders tasks oldest first (within a group, D26).
func Oldest(a, b domain.Task) int {
	return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID))
}

// Age buckets for ByCreated.
var ageBuckets = []struct {
	under time.Duration
	label string
}{
	{7 * 24 * time.Hour, "this week"},
	{30 * 24 * time.Hour, "this month"},
	{90 * 24 * time.Hour, "older"},
}

// GroupOf returns the group t lands in. priorities holds one heading per
// configured level (e.g. "P1 · someone is blocked").
func GroupOf(by GroupBy, t domain.Task, parent *domain.Task, now time.Time, priorities []string) Group {
	switch by {
	case BySize:
		if i := slices.Index(domain.Sizes, t.Size); i >= 0 {
			return Group{Rank: i, Label: strings.ToUpper(string(t.Size))}
		}
		return Group{Rank: len(domain.Sizes), Label: "unsized"}
	case ByCreated:
		age := now.Sub(t.CreatedAt)
		for i, b := range ageBuckets {
			if age < b.under {
				return Group{Rank: i, Label: b.label}
			}
		}
		return Group{Rank: len(ageBuckets), Label: "ancient"}
	case ByRequester:
		if t.Requester == "" {
			return Group{Rank: 1, Label: "no requester"}
		}
		return Group{Label: "@" + t.Requester}
	}
	switch p := domain.EffectivePriority(t, parent); {
	case p == 0:
		return Group{Rank: len(priorities) + 2, Label: "unset"}
	case p > len(priorities) || p < 0:
		return Group{Rank: len(priorities) + 1, Label: "P? · above the configured levels"}
	default:
		return Group{Rank: p, Label: priorities[p-1]}
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/search/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/search/search.go internal/search/search_test.go
git commit -m "feat(search): filter matching and backlog grouping (D26, D26c, D26d)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 3: Remembered UI state

D26 "last choice remembered": migration 003 and `Service.UIState`/`SetUIState` (not undoable).

**Files:**
- Modify: `internal/app/service.go`
- Create: `internal/app/uistate.go`
- Create: `internal/app/uistate_test.go`
- Modify: `internal/store/hardening_test.go`
- Create: `internal/store/migrations/003_ui_state.sql`
- Create: `internal/store/uistate.go`
- Create: `internal/store/uistate_test.go`

**Interfaces:**
- Consumes: `store.readTx`, `store.Tx`, `store.Open` migrations; `app.Service`, test helpers `openTemp`, `newService`.
- Produces:
  - `internal/store/migrations/003_ui_state.sql`: `ui_state(key TEXT PRIMARY KEY, value TEXT NOT NULL)`.
  - `func (*store.Store) UIState(ctx, key string) (value string, ok bool, err error)`; `func (*store.Tx) PutUIState(ctx, key, value string) error`.
  - `app.Store` interface gains `UIState(ctx context.Context, key string) (string, bool, error)`.
  - `func (*app.Service) UIState(ctx, key string) (string, error)` ("" when unset); `func (*app.Service) SetUIState(ctx, key, value string) error`.

- [ ] **Step 1: Write the failing tests**

Create `internal/app/uistate_test.go`:

```go
package app

import (
	"context"
	"testing"
)

func TestUIState(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	if v, err := s.UIState(ctx, "pick.group"); err != nil || v != "" {
		t.Fatalf("unset UIState = %q, %v; want empty", v, err)
	}
	if err := s.SetUIState(ctx, "pick.group", "size"); err != nil {
		t.Fatal(err)
	}
	if v, err := s.UIState(ctx, "pick.group"); err != nil || v != "size" {
		t.Errorf("UIState = %q, %v; want size", v, err)
	}
}
```

Patch `internal/store/hardening_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/store/hardening_test.go b/internal/store/hardening_test.go
index aa6d32f..546fa88 100644
--- a/internal/store/hardening_test.go
+++ b/internal/store/hardening_test.go
@@ -53,7 +53,7 @@ func TestNewerSchemaIsRefused(t *testing.T) {
 		t.Fatal(err)
 	}
 	// Drop the newest real migration's row: a refused Open must not re-apply it.
-	if _, err := s.db.Exec("DELETE FROM schema_migrations WHERE version = 2"); err != nil {
+	if _, err := s.db.Exec("DELETE FROM schema_migrations WHERE version = 3"); err != nil {
 		t.Fatal(err)
 	}
 	s.Close()
```

Create `internal/store/uistate_test.go`:

```go
package store

import (
	"context"
	"testing"
)

func TestUIState(t *testing.T) {
	ctx := context.Background()
	s, path := openTemp(t)
	if _, ok, err := s.UIState(ctx, "pick.group"); err != nil || ok {
		t.Fatalf("missing key: ok=%v err=%v", ok, err)
	}
	for _, v := range []string{"size", "requester"} {
		if err := s.InTx(ctx, func(x *Tx) error { return x.PutUIState(ctx, "pick.group", v) }); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()

	s, err := Open(ctx, path) // remembered across restarts
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v, ok, err := s.UIState(ctx, "pick.group"); err != nil || !ok || v != "requester" {
		t.Errorf("UIState = %q, %v, %v; want the last value", v, ok, err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/store/ ./internal/app/`
Expected: FAIL: `s.UIState undefined`, `x.PutUIState undefined`.

- [ ] **Step 3: Implement**

Patch `internal/app/service.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/app/service.go b/internal/app/service.go
index 1ed190e..d39e627 100644
--- a/internal/app/service.go
+++ b/internal/app/service.go
@@ -25,6 +25,7 @@ type Store interface {
 	LoadSnapshot(ctx context.Context) (domain.Snapshot, error)
 	LoadPlan(ctx context.Context, day domain.Day) (domain.Snapshot, store.PlanState, error)
 	DataVersion(ctx context.Context) (int64, error)
+	UIState(ctx context.Context, key string) (string, bool, error)
 }
 
 // Undo reverts the command that returned it. The TUI keeps these on a
```

Create `internal/app/uistate.go`:

```go
package app

import (
	"context"

	"tt/internal/store"
)

// UIState returns a remembered UI choice (e.g. the backlog grouping, D26);
// "" when none is stored. UI state is not undoable.
func (s *Service) UIState(ctx context.Context, key string) (string, error) {
	v, _, err := s.store.UIState(ctx, key)
	return v, err
}

// SetUIState remembers a UI choice across restarts.
func (s *Service) SetUIState(ctx context.Context, key, value string) error {
	return s.store.InTx(ctx, func(x *store.Tx) error { return x.PutUIState(ctx, key, value) })
}
```

Create `internal/store/migrations/003_ui_state.sql`:

```sql
-- Remembered UI choices, e.g. the backlog grouping (D26).
CREATE TABLE ui_state (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
```

Create `internal/store/uistate.go`:

```go
package store

import (
	"context"
	"database/sql"
	"errors"
)

// UIState returns the remembered value for key; ok is false if none is stored.
func (s *Store) UIState(ctx context.Context, key string) (value string, ok bool, err error) {
	err = s.readTx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT value FROM ui_state WHERE key = ?", key).Scan(&value)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		ok = err == nil
		return err
	})
	return value, ok, err
}

// PutUIState stores value under key, replacing any previous value.
func (x *Tx) PutUIState(ctx context.Context, key, value string) error {
	_, err := x.tx.ExecContext(ctx, `INSERT INTO ui_state (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/store/ ./internal/app/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/app/service.go internal/app/uistate.go internal/app/uistate_test.go internal/store/hardening_test.go internal/store/migrations/003_ui_state.sql internal/store/uistate.go internal/store/uistate_test.go
git commit -m "feat(store): remembered UI state (ui_state table, D26)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 4: Pick: one placement rule, cached rows, grouping

Carry-forwards "one placement rule" and "compute rows once"; D26 grouping with `g`; D26c filtered placement; leftovers (0/0, scroll window, `svc` capture).

**Files:**
- Modify: `internal/tui/common.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/pick.go`
- Create: `internal/tui/pick_test.go`

**Interfaces:**
- Consumes: Task 1 `dsl.Filter`, `dsl.ParseFilter`; Task 2 `search.*`; Task 3 `Service.UIState`, `Service.SetUIState`.
- Produces (used by Task 5):
  - `pickState` gains `offset [2]int`, `rows [2][]row`, `group search.GroupBy`, `filterText string`, `filter dsl.Filter`; `func (*pickState) filtered() bool`; `func (*pickState) follow(m *Model)`.
  - `func (*Model) refreshRows()`; `func (*Model) applyFilter(text string) error` (parse error → current filter stays).
  - `const groupKey = "pick.group"`; messages `uiStateMsg{group string; err error}`, `uiSavedMsg{err error}`.
  - `Model.Update` now wraps `func (*Model) update(msg tea.Msg) tea.Cmd` and calls `m.pick.follow(m)` after every message.
  - `row` gains `expanded bool` and `pinMark bool`; `func plural(n int, one, many string) string` in common.go.
  - Keys: `g` cycles grouping and saves it; `esc` on Pick clears an active filter.

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/pick_test.go`:

```go
package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/config"
)

func TestGroupingCyclesAndIsRemembered(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("Small one !1 ~s @ana")
	h.add("Medium one !2 ~m")
	h.mustSee("BACKLOG · by priority", "P1 · someone is blocked", "P2 ·")

	h.keys("g")
	h.mustSee("grouped by size", "BACKLOG · by size", "\nS ", "[P1 S] Small one", "\nM ")
	h.mustNotSee("P1 · someone is blocked")
	h.keys("g")
	h.mustSee("BACKLOG · by created", "this week")
	h.keys("g")
	h.mustSee("BACKLOG · by requester", "@ana", "no requester")
	if s := h.screen(); strings.Index(s, "@ana\n") > strings.Index(s, "no requester") {
		t.Errorf("no-requester group comes last:\n%s", s)
	}

	// A new session starts with the remembered grouping (D26).
	h2 := &harness{t: t, svc: h.svc, clock: h.clock, path: h.path,
		m: New(context.Background(), h.svc, h.m.opts)}
	h2.run(h2.m.Init())
	h2.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	h2.mustSee("BACKLOG · by requester")

	h.keys("g")
	h.mustSee("BACKLOG · by priority")
}

func TestUnknownRememberedGroupingFallsBackToPriority(t *testing.T) {
	h := newHarness(t, config.Default())
	if err := h.svc.SetUIState(context.Background(), groupKey, "colour"); err != nil {
		t.Fatal(err)
	}
	h2 := &harness{t: t, svc: h.svc, clock: h.clock, path: h.path,
		m: New(context.Background(), h.svc, h.m.opts)}
	h2.run(h2.m.Init())
	h2.mustSee("BACKLOG · by priority")
}

func TestFilteredBacklogPlacement(t *testing.T) {
	h := newHarness(t, config.Default())
	ctx := context.Background()
	parent := h.add("Auth migration !1 ~xl #auth")
	for _, line := range []string{"Rollback plan ~s", "Token spike ~m"} {
		kid := h.add(line)
		if _, err := h.svc.SetParent(ctx, kid.ID, parent.ID); err != nil {
			t.Fatal(err)
		}
	}
	h.add("Schema idea")                   // Inbox
	pinned := h.add("Schema review !2 ~s") // pinned below
	done := h.add("Schema shipped !1 ~s")
	if _, err := h.svc.Pin(ctx, pinned.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Done(ctx, done.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	h.mustNotSee("Schema idea") // unfiltered: Inbox and pinned tasks aren't backlog

	// D26c: a filter searches everything open, Inbox and pinned included.
	if err := h.m.applyFilter("schema"); err != nil {
		t.Fatal(err)
	}
	h.mustSee("BACKLOG · by priority · schema · 2 matches", "[— —] Schema idea", "[P2 S] Schema review 📌", "unset")
	h.mustNotSee("Schema shipped")
	h.keys("p") // the cursor is on Schema review (P2 sorts before unset)
	h.mustSee("already pinned")

	if err := h.m.applyFilter("schema is:all"); err != nil {
		t.Fatal(err)
	}
	h.mustSee("3 matches", "Schema shipped ✓")

	// A matching child whose parent doesn't match is listed top-level.
	if err := h.m.applyFilter("rollback"); err != nil {
		t.Fatal(err)
	}
	h.mustSee("1 match", fmt.Sprintf("[P1 S] Rollback plan ↳ #%d", parent.ID))

	// A matching parent lists its matching children beneath it, expanded.
	if err := h.m.applyFilter("#auth"); err != nil {
		t.Fatal(err)
	}
	h.mustSee("3 matches", "▾ [P1 XL] Auth migration 0/2", "    [P1 S] Rollback plan", "    [P1 M] Token spike")
	if err := h.m.applyFilter("#auth spike"); err != nil {
		t.Fatal(err)
	}
	h.mustSee("1 match", fmt.Sprintf("[P1 M] Token spike ↳ #%d", parent.ID))

	// A bad filter keeps the current one.
	if err := h.m.applyFilter("!9"); err == nil {
		t.Fatal("!9 must be refused")
	}
	h.mustSee("#auth spike · 1 match")

	if err := h.m.applyFilter("zzz"); err != nil {
		t.Fatal(err)
	}
	h.mustSee("zzz · 0 matches", "no matches · esc clears")
	h.keys("esc")
	h.mustSee("filter cleared", "▸ [P1 XL] Auth migration 0/2")
	h.mustNotSee("matches", "Schema idea")
}

func TestScrollWindowMovesOnlyWhenTheCursorLeavesIt(t *testing.T) {
	h := newHarness(t, config.Default())
	for i := 1; i <= 10; i++ {
		h.add(fmt.Sprintf("Task %02d !1 ~s", i))
	}
	h.send(tea.WindowSizeMsg{Width: 140, Height: 10}) // 6 rows per pane
	for range 9 {
		h.keys("j")
	}
	h.mustSee("Task 05", "Task 10")
	h.mustNotSee("Task 04", "P1 · someone")
	h.keys("k") // the cursor moves up inside the window; the window stays
	h.mustSee("Task 05", "Task 10")
	h.mustNotSee("Task 04")
	for range 5 {
		h.keys("k")
	}
	h.mustSee("Task 04", "Task 09") // left the window at the top: it scrolls by one
	h.mustNotSee("Task 10")
	for range 3 {
		h.keys("k")
	}
	h.mustSee("P1 · someone is blocked", "Task 01") // the heading above the first task comes along
}

func TestAllDroppedChildrenHideProgress(t *testing.T) {
	h := newHarness(t, config.Default())
	ctx := context.Background()
	parent := h.add("Parent !1 ~l")
	kid := h.add("Kid ~s")
	if _, err := h.svc.SetParent(ctx, kid.ID, parent.ID); err != nil {
		t.Fatal(err)
	}
	h.clock.now = h.clock.now.Add(time.Minute)
	if _, err := h.svc.Drop(ctx, kid.ID, ""); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	h.mustSee("[P1 L] Parent")
	h.mustNotSee("0/0")
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL: `h.m.applyFilter undefined`, `undefined: groupKey`.

- [ ] **Step 3: Implement**

Patch `internal/tui/common.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/common.go b/internal/tui/common.go
index 4058549..35fe63b 100644
--- a/internal/tui/common.go
+++ b/internal/tui/common.go
@@ -159,6 +159,14 @@ func ago(now, t time.Time) string {
 	}
 }
 
+// plural renders "1 match" / "3 matches".
+func plural(n int, one, many string) string {
+	if n == 1 {
+		return "1 " + one
+	}
+	return fmt.Sprintf("%d %s", n, many)
+}
+
 // shortDur renders 1h, 4h, 30m, 1h30m.
 func shortDur(d time.Duration) string {
 	s := d.String()
```

Patch `internal/tui/model.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/model.go b/internal/tui/model.go
index ec7592c..dcc1e4a 100644
--- a/internal/tui/model.go
+++ b/internal/tui/model.go
@@ -12,6 +12,7 @@ import (
 	tea "charm.land/bubbletea/v2"
 
 	"tt/internal/app"
+	"tt/internal/search"
 )
 
 // Options tunes the background loops.
@@ -78,7 +79,7 @@ type Model struct {
 }
 
 func New(ctx context.Context, svc *app.Service, opts Options) *Model {
-	return &Model{svc: svc, opts: opts, ctx: ctx, width: 100, height: 30}
+	return &Model{svc: svc, opts: opts, ctx: ctx, width: 100, height: 30, pick: pickState{group: search.ByPriority}}
 }
 
 // Messages.
@@ -106,10 +107,20 @@ type (
 		label string
 		err   error
 	}
+	uiStateMsg struct {
+		group string
+		err   error
+	}
+	uiSavedMsg struct{ err error }
 )
 
 func (m *Model) Init() tea.Cmd {
+	svc, ctx := m.svc, m.ctx
 	return tea.Batch(
+		func() tea.Msg {
+			g, err := svc.UIState(ctx, groupKey)
+			return uiStateMsg{group: g, err: err}
+		},
 		m.startDay(),
 		m.opts.Schedule(m.opts.WatchEvery, watchTickMsg{}),
 		m.opts.Schedule(m.opts.ClockEvery, clockTickMsg{}),
@@ -117,48 +128,54 @@ func (m *Model) Init() tea.Cmd {
 }
 
 func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
+	cmd := m.update(msg)
+	m.pick.follow(m)
+	return m, cmd
+}
+
+func (m *Model) update(msg tea.Msg) tea.Cmd {
 	switch msg := msg.(type) {
 	case tea.WindowSizeMsg:
 		m.width, m.height = msg.Width, msg.Height
 	case loadedMsg:
 		if msg.err != nil {
 			m.setErr(msg.err)
-			return m, nil
+			return nil
 		}
 		m.state, m.loaded = msg.state, true
 		m.idx = buildIndex(msg.state)
-		m.pick.clamp(m)
+		m.refreshRows()
 		m.inbox.sync(m)
 	case watchTickMsg:
 		cmds := []tea.Cmd{m.checkVersion(), m.opts.Schedule(m.opts.WatchEvery, watchTickMsg{})}
 		if m.dayIsStale() {
 			cmds = append(cmds, m.startDay())
 		}
-		return m, tea.Batch(cmds...)
+		return tea.Batch(cmds...)
 	case versionMsg:
 		if msg.err != nil {
 			m.setErr(fmt.Errorf("can't read the database: %w", msg.err))
-			return m, nil
+			return nil
 		}
 		if msg.version != m.state.Version {
-			return m, m.load()
+			return m.load()
 		}
 	case clockTickMsg:
-		return m, tea.Batch(m.startDay(), m.opts.Schedule(m.opts.ClockEvery, clockTickMsg{}))
+		return tea.Batch(m.startDay(), m.opts.Schedule(m.opts.ClockEvery, clockTickMsg{}))
 	case dayStartedMsg:
 		if msg.err != nil {
 			m.setErr(msg.err)
-			return m, m.load()
+			return m.load()
 		}
 		if msg.res.New {
 			res := msg.res
 			m.banner = &res
 		}
-		return m, m.load()
+		return m.load()
 	case actionMsg:
 		if msg.err != nil {
 			m.setErr(msg.err)
-			return m, m.load()
+			return m.load()
 		}
 		if msg.undo != nil {
 			m.undo = append(m.undo, undoEntry{label: msg.label, fn: msg.undo})
@@ -167,7 +184,7 @@ func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
 			}
 		}
 		m.setStatus(msg.status)
-		return m, m.load()
+		return m.load()
 	case undoneMsg:
 		switch {
 		case errors.Is(msg.err, app.ErrConflict) || errors.Is(msg.err, app.ErrNotFound):
@@ -177,15 +194,28 @@ func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
 		default:
 			m.setStatus("undid " + msg.label)
 		}
-		return m, m.load()
+		return m.load()
+	case uiStateMsg:
+		if msg.err != nil {
+			m.setErr(fmt.Errorf("can't read the remembered grouping: %w", msg.err))
+			return nil
+		}
+		m.pick.group = search.ParseGroupBy(msg.group)
+		if m.loaded {
+			m.refreshRows()
+		}
+	case uiSavedMsg:
+		if msg.err != nil {
+			m.setErr(fmt.Errorf("can't remember the grouping: %w", msg.err))
+		}
 	case tea.KeyPressMsg:
-		return m, m.handleKey(msg)
+		return m.handleKey(msg)
 	case tea.PasteMsg:
 		if p, ok := m.overlay.(paster); ok {
-			return m, p.paste(msg)
+			return p.paste(msg)
 		}
 	}
-	return m, nil
+	return nil
 }
 
 func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
```

Patch `internal/tui/pick.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/pick.go b/internal/tui/pick.go
index d3e27fc..7d87aee 100644
--- a/internal/tui/pick.go
+++ b/internal/tui/pick.go
@@ -1,9 +1,9 @@
 package tui
 
 import (
-	"cmp"
 	"context"
 	"fmt"
+	"maps"
 	"slices"
 	"strings"
 
@@ -11,6 +11,8 @@ import (
 
 	"tt/internal/app"
 	"tt/internal/domain"
+	"tt/internal/dsl"
+	"tt/internal/search"
 )
 
 type pane int
@@ -29,86 +31,130 @@ const (
 )
 
 type row struct {
-	kind   rowKind
-	text   string // header or hint text
-	task   domain.Task
-	child  bool              // indented under its parent
-	cand   *domain.Candidate // set when the task is pinned
-	parent bool              // has children
-	orphan int64             // child listed top-level because this parent isn't shown in the backlog
+	kind     rowKind
+	text     string // header or hint text
+	task     domain.Task
+	child    bool              // indented under its parent
+	cand     *domain.Candidate // set when the task is pinned
+	parent   bool              // has children
+	expanded bool              // a parent whose children are listed beneath it
+	pinMark  bool              // a pinned task listed in the backlog (filtered): show 📌
+	orphan   int64             // child listed top-level because this parent isn't shown in the backlog
 }
 
+// groupKey is the ui_state key that remembers the backlog grouping (D26).
+const groupKey = "pick.group"
+
 type pickState struct {
-	focus    pane
-	cursor   [2]int
-	selected [2]int64 // task id under each pane's cursor (0 = none); survives reloads
-	expanded map[int64]bool
+	focus      pane
+	cursor     [2]int
+	offset     [2]int   // first row shown in each pane (scroll window)
+	selected   [2]int64 // task id under each pane's cursor (0 = none); survives reloads
+	expanded   map[int64]bool
+	rows       [2][]row // built by refreshRows, read by keys and render
+	group      search.GroupBy
+	filterText string     // the filter as typed (trimmed); "" = none
+	filter     dsl.Filter // parsed filterText
+}
+
+func (p *pickState) filtered() bool { return !p.filter.Empty() }
+
+// refreshRows rebuilds both panes' rows: after a load, a filter or grouping
+// change, and expand/collapse. Render and keys only read the cache.
+func (m *Model) refreshRows() {
+	m.pick.rows[paneBacklog] = m.backlogRows()
+	m.pick.rows[paneToday] = m.todayRows()
+	m.pick.clamp(m)
 }
 
-// backlogRows: triaged open tasks that aren't pinned, grouped by priority
-// (oldest first within a group, D26). Expanded parents list their open,
-// triaged, unpinned children. A child whose parent isn't itself listed here
-// (closed, in the Inbox, missing) is listed top-level so it never vanishes;
-// children of a pinned parent live under it in Today.
+// applyFilter makes text the backlog filter (D26a). On a parse error the
+// current filter stays and the error is returned.
+func (m *Model) applyFilter(text string) error {
+	f, err := dsl.ParseFilter(text, len(m.svc.Config().Priorities))
+	if err != nil {
+		return err
+	}
+	m.pick.filterText, m.pick.filter = strings.TrimSpace(text), f
+	m.refreshRows()
+	return nil
+}
+
+// backlogRows is the one placement rule for the backlog pane. Unfiltered it
+// lists triaged, open, unpinned tasks; filtered, every task the filter matches,
+// Inbox and pinned ones included (D26c). A listed child sits under its listed
+// parent (unfiltered: while the parent is expanded; filtered: always). A child
+// whose parent isn't listed (closed, in the Inbox, missing, or not matching)
+// is listed top-level with ↳ #parent so it never vanishes; unfiltered, the
+// children of a pinned parent live under it in Today instead. Top-level tasks
+// are grouped (D26), oldest first within a group.
 func (m *Model) backlogRows() []row {
 	snap := m.state.Snapshot
+	now := m.svc.Now()
+	filtered := m.pick.filtered()
+	listed := map[int64]bool{}
+	for _, t := range snap.Tasks() {
+		_, pinned := m.idx.cand[t.ID]
+		if filtered && search.Match(m.pick.filter, t, snap.ParentOf(t), now) ||
+			!filtered && t.Status == domain.StatusOpen && !m.idx.inInbox[t.ID] && !pinned {
+			listed[t.ID] = true
+		}
+	}
 	levels := m.svc.Config().Priorities
-	groups := map[int][]domain.Task{}
+	headings := make([]string, len(levels))
+	for i, l := range levels {
+		headings[i] = l.Name + " · " + l.Definition
+	}
+	groups := map[search.Group][]domain.Task{}
 	orphanOf := map[int64]int64{}
 	for _, t := range snap.Tasks() {
-		if t.Status != domain.StatusOpen || m.idx.inInbox[t.ID] {
-			continue
-		}
-		if _, pinned := m.idx.cand[t.ID]; pinned {
+		if !listed[t.ID] {
 			continue
 		}
-		parent := snap.ParentOf(t)
 		if t.ParentID != 0 {
-			if parent != nil {
-				if _, pinned := m.idx.cand[parent.ID]; pinned {
-					continue
-				}
-				if parent.Status == domain.StatusOpen && !m.idx.inInbox[parent.ID] {
-					continue // listed under its parent
-				}
+			if listed[t.ParentID] {
+				continue // listed under its parent
+			}
+			if _, pinned := m.idx.cand[t.ParentID]; pinned && !filtered {
+				continue // listed under its parent in Today
 			}
 			orphanOf[t.ID] = t.ParentID
 		}
-		p := domain.EffectivePriority(t, parent)
-		if p > len(levels) || p < 1 {
-			p = len(levels) + 1 // shrunk config: "P?" group at the end
-		}
-		groups[p] = append(groups[p], t)
+		g := search.GroupOf(m.pick.group, t, snap.ParentOf(t), now, headings)
+		groups[g] = append(groups[g], t)
 	}
 	var rows []row
-	for p := 1; p <= len(levels)+1; p++ {
-		items := groups[p]
-		if len(items) == 0 {
-			continue
-		}
-		slices.SortFunc(items, func(a, b domain.Task) int { return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID)) })
-		header := "P? · above the configured levels"
-		if p <= len(levels) {
-			header = levels[p-1].Name + " · " + levels[p-1].Definition
-		}
-		rows = append(rows, row{kind: rowHeader, text: header})
+	for _, g := range slices.SortedFunc(maps.Keys(groups), search.Compare) {
+		items := groups[g]
+		slices.SortFunc(items, search.Oldest)
+		rows = append(rows, row{kind: rowHeader, text: g.Label})
 		for _, t := range items {
-			kids := m.idx.children[t.ID]
-			rows = append(rows, row{kind: rowTask, task: t, parent: len(kids) > 0, orphan: orphanOf[t.ID]})
-			if !m.pick.expanded[t.ID] {
+			var kids []domain.Task
+			for _, c := range m.idx.children[t.ID] {
+				if listed[c.ID] {
+					kids = append(kids, c)
+				}
+			}
+			open := len(kids) > 0 && (filtered || m.pick.expanded[t.ID])
+			rows = append(rows, m.backlogRow(t, false, open, orphanOf[t.ID]))
+			if !open {
 				continue
 			}
 			for _, c := range kids {
-				if _, pinned := m.idx.cand[c.ID]; c.Status != domain.StatusOpen || m.idx.inInbox[c.ID] || pinned {
-					continue
-				}
-				rows = append(rows, row{kind: rowTask, task: c, child: true})
+				rows = append(rows, m.backlogRow(c, true, false, 0))
 			}
 		}
 	}
 	return rows
 }
 
+func (m *Model) backlogRow(t domain.Task, child, expanded bool, orphan int64) row {
+	r := row{kind: rowTask, task: t, child: child, parent: len(m.idx.children[t.ID]) > 0, expanded: expanded, orphan: orphan}
+	if c, ok := m.idx.cand[t.ID]; ok {
+		r.cand, r.pinMark = &c, true
+	}
+	return r
+}
+
 // todayRows: today's candidates in plan order. A pinned parent lists its open
 // children and children closed today beneath it; a pinned child of a pinned
 // parent appears only there.
@@ -120,7 +166,7 @@ func (m *Model) todayRows() []row {
 		}
 		cc := c
 		kids := m.idx.children[c.Task.ID]
-		rows = append(rows, row{kind: rowTask, task: c.Task, cand: &cc, parent: len(kids) > 0})
+		rows = append(rows, row{kind: rowTask, task: c.Task, cand: &cc, parent: len(kids) > 0, expanded: len(kids) > 0})
 		if c.Alert {
 			rows = append(rows, rolloverHint(c.Task))
 		}
@@ -145,12 +191,7 @@ func rolloverHint(t domain.Task) row {
 	return row{kind: rowHint, text: fmt.Sprintf("⚠ rolled over %d× → refine · split · reprioritize · hand off · drop", t.RolloverCount)}
 }
 
-func (m *Model) rows(p pane) []row {
-	if p == paneToday {
-		return m.todayRows()
-	}
-	return m.backlogRows()
-}
+func (m *Model) rows(p pane) []row { return m.pick.rows[p] }
 
 // clamp keeps each pane's cursor on a selectable row. It prefers the task the
 // cursor was on (so a reload that reorders rows keeps the highlight on that
@@ -160,7 +201,7 @@ func (p *pickState) clamp(m *Model) {
 		p.expanded = map[int64]bool{}
 	}
 	for _, pn := range []pane{paneBacklog, paneToday} {
-		rows := m.rows(pn)
+		rows := p.rows[pn]
 		c := -1
 		if id := p.selected[pn]; id != 0 {
 			c = slices.IndexFunc(rows, func(r row) bool { return r.kind == rowTask && r.task.ID == id })
@@ -183,8 +224,8 @@ func (p *pickState) clamp(m *Model) {
 	}
 }
 
-func (p *pickState) move(m *Model, delta int) {
-	rows := m.rows(p.focus)
+func (p *pickState) move(delta int) {
+	rows := p.rows[p.focus]
 	for c := p.cursor[p.focus] + delta; c >= 0 && c < len(rows); c += delta {
 		if rows[c].kind == rowTask {
 			p.cursor[p.focus] = c
@@ -194,6 +235,25 @@ func (p *pickState) move(m *Model, delta int) {
 	}
 }
 
+// follow keeps each pane's cursor inside its scroll window, moving the window
+// only when the cursor leaves it; a group heading right above the cursor
+// comes along. Called after every update.
+func (p *pickState) follow(m *Model) {
+	visible := m.visibleRows()
+	for pn, rows := range p.rows {
+		c, off := p.cursor[pn], p.offset[pn]
+		top := c
+		if top > 0 && top < len(rows) && rows[top-1].kind == rowHeader {
+			top--
+		}
+		off = min(off, top)
+		if c >= off+visible {
+			off = c - visible + 1
+		}
+		p.offset[pn] = max(0, min(off, len(rows)-visible))
+	}
+}
+
 // visibleRows is how many task rows fit beside the header, pane titles,
 // status and hints, plus the banner and an open overlay.
 func (m *Model) visibleRows() int {
@@ -218,21 +278,34 @@ func (m *Model) selectedRow() (row, bool) {
 
 func (m *Model) pickKey(key string) tea.Cmd {
 	p := &m.pick
+	svc := m.svc
 	switch key {
 	case "tab":
 		p.focus = 1 - p.focus
 		p.clamp(m)
 		return nil
 	case "j", "down":
-		p.move(m, +1)
+		p.move(+1)
 		return nil
 	case "k", "up":
-		p.move(m, -1)
+		p.move(-1)
 		return nil
 	case "i":
 		m.screen = screenInbox
 		m.inbox.sync(m)
 		return nil
+	case "g":
+		p.group = p.group.Next()
+		m.refreshRows()
+		m.setStatus("grouped by " + string(p.group))
+		ctx, g := m.ctx, p.group
+		return func() tea.Msg { return uiSavedMsg{err: svc.SetUIState(ctx, groupKey, string(g))} }
+	case "esc":
+		if p.filtered() {
+			_ = m.applyFilter("") // an empty line always parses
+			m.setStatus("filter cleared")
+		}
+		return nil
 	}
 	r, ok := m.selectedRow()
 	if !ok {
@@ -244,14 +317,16 @@ func (m *Model) pickKey(key string) tea.Cmd {
 	case "right", "l":
 		if p.focus == paneBacklog && r.parent {
 			p.expanded[t.ID] = true
+			m.refreshRows()
 		}
 	case "left", "h":
 		if p.focus == paneBacklog && r.child {
 			p.expanded[t.ParentID] = false
 			p.selected[paneBacklog] = t.ParentID // the parent takes the cursor
-			p.clamp(m)
+			m.refreshRows()
 		} else if p.focus == paneBacklog {
 			p.expanded[t.ID] = false
+			m.refreshRows()
 		}
 	case "p":
 		if t.Status != domain.StatusOpen {
@@ -262,18 +337,18 @@ func (m *Model) pickKey(key string) tea.Cmd {
 			m.setStatus("already pinned")
 			return nil
 		}
-		return m.act("pin "+label, "pinned "+label, func(ctx context.Context) (app.Undo, error) { return m.svc.Pin(ctx, t.ID) })
+		return m.act("pin "+label, "pinned "+label, func(ctx context.Context) (app.Undo, error) { return svc.Pin(ctx, t.ID) })
 	case "u":
 		if r.cand == nil {
 			m.setStatus("not pinned")
 			return nil
 		}
-		return m.act("unpin "+label, "unpinned "+label, func(ctx context.Context) (app.Undo, error) { return m.svc.Unpin(ctx, t.ID) })
+		return m.act("unpin "+label, "unpinned "+label, func(ctx context.Context) (app.Undo, error) { return svc.Unpin(ctx, t.ID) })
 	case "x":
 		if t.Status != domain.StatusOpen {
-			return m.act("reopen "+label, "reopened "+label, func(ctx context.Context) (app.Undo, error) { return m.svc.Reopen(ctx, t.ID) })
+			return m.act("reopen "+label, "reopened "+label, func(ctx context.Context) (app.Undo, error) { return svc.Reopen(ctx, t.ID) })
 		}
-		return m.act("done "+label, "✓ "+label, func(ctx context.Context) (app.Undo, error) { return m.svc.Done(ctx, t.ID) })
+		return m.act("done "+label, "✓ "+label, func(ctx context.Context) (app.Undo, error) { return svc.Done(ctx, t.ID) })
 	case "d":
 		return m.openDropPrompt(t)
 	case "D":
@@ -286,7 +361,18 @@ func (m *Model) pickView() string {
 	total := max(m.width-3, 20)
 	lw := total * 55 / 100
 	rw := total - lw
-	left := m.paneLines(paneBacklog, "BACKLOG", "nothing triaged yet · a to add · i to triage", lw)
+	title, empty := "BACKLOG · by "+string(m.pick.group), "nothing triaged yet · a to add · i to triage"
+	if m.pick.filtered() {
+		n := 0
+		for _, r := range m.rows(paneBacklog) {
+			if r.kind == rowTask {
+				n++
+			}
+		}
+		title += " · " + m.pick.filterText + " · " + plural(n, "match", "matches")
+		empty = "no matches · esc clears"
+	}
+	left := m.paneLines(paneBacklog, title, empty, lw)
 	right := m.paneLines(paneToday, "TODAY'S CANDIDATES", "nothing pinned · p on a backlog task", rw)
 	var b strings.Builder
 	for i := range max(len(left), len(right)) {
@@ -313,7 +399,7 @@ func (m *Model) paneLines(pn pane, title, empty string, width int) []string {
 	}
 	visible := m.visibleRows()
 	cursor := m.pick.cursor[pn]
-	start := max(0, cursor-visible+1)
+	start := m.pick.offset[pn]
 	for i := start; i < len(rows) && i < start+visible; i++ {
 		lines = append(lines, m.rowLine(rows[i], width, pn == m.pick.focus && i == cursor))
 	}
@@ -332,7 +418,7 @@ func (m *Model) rowLine(r row, width int, selected bool) string {
 	switch {
 	case r.child:
 		b.WriteString("    ")
-	case r.parent && r.cand == nil && !m.pick.expanded[t.ID]:
+	case r.parent && !r.expanded:
 		b.WriteString("▸ ")
 	case r.parent:
 		b.WriteString("▾ ")
@@ -354,7 +440,9 @@ func (m *Model) rowLine(r row, width int, selected bool) string {
 				done++
 			}
 		}
-		fmt.Fprintf(&b, " %d/%d", done, total)
+		if total > 0 {
+			fmt.Fprintf(&b, " %d/%d", done, total)
+		}
 	}
 	if c := r.cand; c != nil {
 		if c.Blocked {
@@ -364,8 +452,11 @@ func (m *Model) rowLine(r row, width int, selected bool) string {
 	if t.RolloverCount > 0 {
 		fmt.Fprintf(&b, " ↻%d", t.RolloverCount)
 	}
-	if c := r.cand; c != nil && c.PinnedDays > 1 {
-		fmt.Fprintf(&b, " 📌%d", c.PinnedDays)
+	if c := r.cand; c != nil && (r.pinMark || c.PinnedDays > 1) {
+		b.WriteString(" 📌")
+		if c.PinnedDays > 1 {
+			fmt.Fprintf(&b, "%d", c.PinnedDays)
+		}
 	}
 	switch t.Status {
 	case domain.StatusDone:
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/common.go internal/tui/model.go internal/tui/pick.go internal/tui/pick_test.go
git commit -m "feat(tui): one placement rule, cached rows, switchable grouping (D26, D26c)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 5: Filter bar and shared token input

D26a, D26e: `/` opens a live filter line above the backlog; carry-forwards "overlays that parse tokens keep the text on error", tab-complete at the cursor and not inside a note, input width follows resizes.

**Files:**
- Modify: `internal/tui/common.go`
- Create: `internal/tui/filter_test.go`
- Modify: `internal/tui/harness_test.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/pick.go`
- Create: `internal/tui/tokeninput.go`

**Interfaces:**
- Consumes: Task 4 `applyFilter`, `pickState.filterText`, `pickState.filtered`; `newInput`, `m.idx.tags/requesters`.
- Produces:
  - `type tokenInput struct{ input textinput.Model }` with `newTokenInput(prompt, placeholder string, width int)`, `value()`, `update(tea.Msg) tea.Cmd`, `resize(width int)`, `suggestions(m) []string`, `complete(s string)`, `view(m, err error) string`.
  - `quickAdd{in tokenInput; err error}`; `filterBar{in tokenInput; prev string; err error}` and `(*Model) openFilter()`.
  - `func fitInput(ti *textinput.Model, width int)`; `paster` becomes `paste(m *Model, msg tea.PasteMsg) tea.Cmd`; new `resizer{ resize(width int) }` honoured on `tea.WindowSizeMsg`.
  - Harness: named key `"backspace"`.

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/filter_test.go`:

```go
package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/app"
	"tt/internal/config"
	"tt/internal/store"
)

func TestFilterBarNarrowsLiveAndKeepsTypingOnError(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("Schema review !1 ~s @ana")
	h.add("Deploy fix !2 ~m")

	h.keys("/")
	if s := h.screen(); strings.Index(s, "filter ›") > strings.Index(s, "BACKLOG") {
		t.Errorf("the filter line sits above the backlog:\n%s", s)
	}
	h.typeText("sch")
	h.mustSee("BACKLOG · by priority · sch · 1 match", "Schema review")
	h.mustNotSee("Deploy fix")

	h.typeText(" !9")
	h.mustSee("✗ priority !9 out of range", "1 match", "Schema review") // the last valid filter ("sch !") stays
	h.keys("enter")
	if _, ok := h.m.overlay.(*filterBar); !ok {
		t.Fatal("enter on a bad filter must keep the line open")
	}
	h.mustSee("filter › sch !9")
	h.keys("backspace", "backspace", "backspace")
	h.mustNotSee("✗")
	h.keys("enter")
	if h.m.overlay != nil {
		t.Fatal("enter closes the filter line")
	}
	h.mustSee("sch · 1 match", "esc clears filter")

	// esc in the line restores the filter from before `/`.
	h.keys("/")
	h.mustSee("filter › sch")
	h.typeText("zzz")
	h.mustSee("sch zzz · 0 matches", "no matches · esc clears")
	h.keys("esc")
	h.mustSee("sch · 1 match")

	// esc on the list clears it; filters are never remembered.
	h.keys("esc")
	h.mustSee("filter cleared", "Deploy fix")
	h.mustNotSee("match")

	// Pasting into the line filters too.
	h.keys("/")
	h.send(tea.PasteMsg{Content: "@ana"})
	h.mustSee("@ana · 1 match", "Schema review")
}

func TestReloadWhileFilteringKeepsTheFilter(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("Schema review !1 ~s")
	h.keys("/")
	h.typeText("schema")
	other, err := store.Open(context.Background(), h.path) // `tt add` from another shell
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, _, err := app.New(other, h.clock, config.Default()).Add(context.Background(), "Schema follow-up"); err != nil {
		t.Fatal(err)
	}
	h.send(watchTickMsg{})
	h.mustSee("filter › schema", "schema · 2 matches", "[— —] Schema follow-up")
}

func TestTokenInputCompletesAtTheCursor(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("one @ana #photos")
	h.add("two @andrei")

	h.keys("a")
	h.typeText("x @an #ph")
	h.keys("left", "left", "left", "left") // into "@an"
	h.mustSee("tab → @ana  @andrei")
	h.keys("tab")
	h.mustSee("add › x @ana #ph")
	h.keys("esc")

	h.keys("a")
	h.typeText("y -- ask #ph") // inside the note: no completion
	h.mustNotSee("tab →")
	h.keys("esc")

	h.keys("/")
	h.typeText("@and")
	h.mustSee("tab → @andrei")
	h.keys("tab")
	h.mustSee("filter › @andrei", "@andrei · 1 match", "two")
}

func TestOverlayInputFollowsResize(t *testing.T) {
	h := newHarness(t, config.Default())
	h.keys("a")
	h.send(tea.WindowSizeMsg{Width: 60, Height: 40})
	if w, want := h.m.overlay.(*quickAdd).in.input.Width(), 60-ansi.StringWidth("add › ")-2; w != want {
		t.Errorf("quick-add input width after resize = %d; want %d", w, want)
	}
	h.keys("esc", "/")
	h.send(tea.WindowSizeMsg{Width: 80, Height: 40})
	if w, want := h.m.overlay.(*filterBar).in.input.Width(), 80-ansi.StringWidth("filter › ")-2; w != want {
		t.Errorf("filter input width after resize = %d; want %d", w, want)
	}
}
```

Patch `internal/tui/harness_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/harness_test.go b/internal/tui/harness_test.go
index f620966..5eba493 100644
--- a/internal/tui/harness_test.go
+++ b/internal/tui/harness_test.go
@@ -82,6 +82,7 @@ var named = map[string]tea.KeyPressMsg{
 	"enter": {Code: tea.KeyEnter}, "tab": {Code: tea.KeyTab}, "esc": {Code: tea.KeyEscape},
 	"up": {Code: tea.KeyUp}, "down": {Code: tea.KeyDown}, "left": {Code: tea.KeyLeft}, "right": {Code: tea.KeyRight},
 	"ctrl+z": {Code: 'z', Mod: tea.ModCtrl}, "ctrl+c": {Code: 'c', Mod: tea.ModCtrl},
+	"backspace": {Code: tea.KeyBackspace},
 }
 
 // keys sends named keys ("enter", "ctrl+z") or single characters.
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL: `undefined: filterBar`, `h.m.overlay.(*quickAdd).in undefined`.

- [ ] **Step 3: Implement**

Patch `internal/tui/common.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/common.go b/internal/tui/common.go
index 35fe63b..f0888a3 100644
--- a/internal/tui/common.go
+++ b/internal/tui/common.go
@@ -133,7 +133,11 @@ func (m *Model) hintsView() string {
 		}
 		return styleDim.Render(fmt.Sprintf("1-%d priority · s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help", len(m.svc.Config().Priorities)))
 	}
-	return styleDim.Render("a add · p pin · u unpin · x done · d drop · tab pane · →/← expand · i inbox · ctrl+z undo · ? help · q quit")
+	hints := "a add · / filter · g group · p pin · u unpin · x done · d drop · tab pane · →/← expand · i inbox · ctrl+z undo · ? help · q quit"
+	if m.pick.filtered() {
+		hints = "esc clears filter · " + hints
+	}
+	return styleDim.Render(hints)
 }
 
 func dayLabel(d domain.Day) string {
```

Patch `internal/tui/model.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/model.go b/internal/tui/model.go
index dcc1e4a..01b4b59 100644
--- a/internal/tui/model.go
+++ b/internal/tui/model.go
@@ -55,7 +55,12 @@ type overlay interface {
 }
 
 // paster is implemented by overlays that accept bracketed-paste text.
-type paster interface{ paste(tea.PasteMsg) tea.Cmd }
+type paster interface {
+	paste(m *Model, msg tea.PasteMsg) tea.Cmd
+}
+
+// resizer is implemented by overlays whose width follows the terminal's.
+type resizer interface{ resize(width int) }
 
 type Model struct {
 	svc  *app.Service
@@ -137,6 +142,9 @@ func (m *Model) update(msg tea.Msg) tea.Cmd {
 	switch msg := msg.(type) {
 	case tea.WindowSizeMsg:
 		m.width, m.height = msg.Width, msg.Height
+		if r, ok := m.overlay.(resizer); ok {
+			r.resize(msg.Width)
+		}
 	case loadedMsg:
 		if msg.err != nil {
 			m.setErr(msg.err)
@@ -212,7 +220,7 @@ func (m *Model) update(msg tea.Msg) tea.Cmd {
 		return m.handleKey(msg)
 	case tea.PasteMsg:
 		if p, ok := m.overlay.(paster); ok {
-			return p.paste(msg)
+			return p.paste(m, msg)
 		}
 	}
 	return nil
@@ -339,12 +347,16 @@ func (m *Model) render() string {
 	if m.banner != nil {
 		parts = append(parts, m.bannerView())
 	}
+	fb, filtering := m.overlay.(*filterBar)
 	if m.screen == screenInbox {
 		parts = append(parts, m.inboxView())
 	} else {
+		if filtering {
+			parts = append(parts, fb.view(m)) // the filter line sits above the backlog
+		}
 		parts = append(parts, m.pickView())
 	}
-	if m.overlay != nil {
+	if m.overlay != nil && !filtering {
 		parts = append(parts, m.overlay.view(m))
 	}
 	parts = append(parts, m.statusView(), m.hintsView())
```

Patch `internal/tui/overlays.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/overlays.go b/internal/tui/overlays.go
index 9a8e01d..bfa920e 100644
--- a/internal/tui/overlays.go
+++ b/internal/tui/overlays.go
@@ -14,12 +14,17 @@ import (
 	"tt/internal/dsl"
 )
 
+// fitInput sizes ti to a terminal width cells wide.
+func fitInput(ti *textinput.Model, width int) {
+	ti.SetWidth(max(width-ansi.StringWidth(ti.Prompt)-2, 10))
+}
+
 func newInput(prompt, placeholder string, width int) textinput.Model {
 	ti := textinput.New()
 	ti.Prompt = prompt
 	ti.Placeholder = placeholder
 	ti.CharLimit = 1000
-	ti.SetWidth(max(width-ansi.StringWidth(prompt)-2, 10))
+	fitInput(&ti, width)
 	st := ti.Styles()
 	st.Cursor.Blink = false // a static cursor: no blink timers
 	ti.SetStyles(st)
@@ -29,12 +34,12 @@ func newInput(prompt, placeholder string, width int) textinput.Model {
 
 // quickAdd is the one-line capture overlay (D25) with @/# autocomplete.
 type quickAdd struct {
-	input textinput.Model
-	err   error // why the last enter was refused; shown under the input
+	in  tokenInput
+	err error // why the last enter was refused; shown under the input
 }
 
 func (m *Model) openQuickAdd() tea.Cmd {
-	m.overlay = &quickAdd{input: newInput("add › ", "title @who #tag !1 ~s link -- note", m.width)}
+	m.overlay = &quickAdd{in: newTokenInput("add › ", "title @who #tag !1 ~s link -- note", m.width)}
 	return nil
 }
 
@@ -44,12 +49,12 @@ func (q *quickAdd) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
 		m.overlay = nil
 		return nil
 	case "tab":
-		if s := q.suggestions(m); len(s) > 0 {
-			q.complete(s[0])
+		if s := q.in.suggestions(m); len(s) > 0 {
+			q.in.complete(s[0])
 		}
 		return nil
 	case "enter":
-		line := q.input.Value()
+		line := q.in.value()
 		if _, err := dsl.ParseAdd(line, len(m.svc.Config().Priorities)); err != nil {
 			q.err = err
 			return nil
@@ -66,69 +71,67 @@ func (q *quickAdd) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
 		}
 	}
 	q.err = nil
-	var cmd tea.Cmd
-	q.input, cmd = q.input.Update(k)
-	return cmd
+	return q.in.update(k)
 }
 
-// currentToken is the word being typed (the input's last word).
-func (q *quickAdd) currentToken() string {
-	v := q.input.Value()
-	if strings.HasSuffix(v, " ") {
-		return ""
-	}
-	fields := strings.Fields(v)
-	if len(fields) == 0 {
-		return ""
+func (q *quickAdd) paste(_ *Model, msg tea.PasteMsg) tea.Cmd { return q.in.update(msg) }
+
+func (q *quickAdd) resize(width int) { q.in.resize(width) }
+
+func (q *quickAdd) view(m *Model) string { return q.in.view(m, q.err) }
+
+// filterBar edits the backlog filter and applies it as you type (D26e).
+type filterBar struct {
+	in   tokenInput
+	prev string // the filter before `/`; esc restores it
+	err  error  // the line doesn't parse; the last valid filter stays applied
+}
+
+func (m *Model) openFilter() tea.Cmd {
+	in := newTokenInput("filter › ", "@who #tag !1 ~s <7d >30d is:done words", m.width)
+	if text := m.pick.filterText; text != "" {
+		in.input.SetValue(text + " ")
+		in.input.CursorEnd()
 	}
-	return fields[len(fields)-1]
+	m.pick.focus = paneBacklog
+	m.overlay = &filterBar{in: in, prev: m.pick.filterText}
+	return nil
 }
 
-// suggestions completes a trailing @requester or #tag from known values, most used first.
-func (q *quickAdd) suggestions(m *Model) []string {
-	tok := q.currentToken()
-	if len(tok) < 1 || (tok[0] != '@' && tok[0] != '#') {
+func (f *filterBar) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
+	switch k.String() {
+	case "esc":
+		m.overlay = nil
+		_ = m.applyFilter(f.prev) // it parsed when it was applied
 		return nil
-	}
-	pool := m.idx.tags
-	if tok[0] == '@' {
-		pool = m.idx.requesters
-	}
-	prefix := strings.ToLower(tok[1:])
-	var out []string
-	for _, v := range pool {
-		if strings.HasPrefix(v, prefix) && v != prefix {
-			out = append(out, string(tok[0])+v)
-			if len(out) == 5 {
-				break
-			}
+	case "enter":
+		if f.err == nil {
+			m.overlay = nil
 		}
+		return nil // refused: the text and the error stay
+	case "tab":
+		if s := f.in.suggestions(m); len(s) > 0 {
+			f.in.complete(s[0])
+			f.apply(m)
+		}
+		return nil
 	}
-	return out
+	cmd := f.in.update(k)
+	f.apply(m)
+	return cmd
 }
 
-func (q *quickAdd) complete(s string) {
-	v := q.input.Value()
-	q.input.SetValue(v[:len(v)-len(q.currentToken())] + s + " ")
-	q.input.CursorEnd()
-}
+func (f *filterBar) apply(m *Model) { f.err = m.applyFilter(f.in.value()) }
 
-func (q *quickAdd) paste(msg tea.PasteMsg) tea.Cmd {
-	var cmd tea.Cmd
-	q.input, cmd = q.input.Update(msg)
+func (f *filterBar) paste(m *Model, msg tea.PasteMsg) tea.Cmd {
+	cmd := f.in.update(msg)
+	f.apply(m)
 	return cmd
 }
 
-func (q *quickAdd) view(m *Model) string {
-	out := q.input.View()
-	if q.err != nil {
-		out += "\n" + styleError.Render("✗ "+q.err.Error())
-	}
-	if s := q.suggestions(m); len(s) > 0 {
-		out += "\n" + styleDim.Render("  tab → "+strings.Join(s, "  "))
-	}
-	return out
-}
+func (f *filterBar) resize(width int) { f.in.resize(width) }
+
+func (f *filterBar) view(m *Model) string { return f.in.view(m, f.err) }
 
 // prompt is a one-line text question, e.g. the optional drop reason (D23).
 type prompt struct {
@@ -150,12 +153,14 @@ func (p *prompt) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
 	return cmd
 }
 
-func (p *prompt) paste(msg tea.PasteMsg) tea.Cmd {
+func (p *prompt) paste(_ *Model, msg tea.PasteMsg) tea.Cmd {
 	var cmd tea.Cmd
 	p.input, cmd = p.input.Update(msg)
 	return cmd
 }
 
+func (p *prompt) resize(width int) { fitInput(&p.input, width) }
+
 func (p *prompt) view(*Model) string { return p.input.View() }
 
 func (m *Model) openDropPrompt(t domain.Task) tea.Cmd {
@@ -231,6 +236,8 @@ func (helpOverlay) view(m *Model) string {
 			"p / u    pin to today / unpin",
 			"x        done (again: reopen)",
 			"d / D    drop with optional reason / delete",
+			"/        filter: @who #tag !1 ~s <7d >30d is:done|dropped|all words (live; esc on the list clears)",
+			"g        group by priority → size → created → requester (remembered)",
 			"i        inbox",
 		}
 	}
```

Patch `internal/tui/pick.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/pick.go b/internal/tui/pick.go
index 7d87aee..e46f80c 100644
--- a/internal/tui/pick.go
+++ b/internal/tui/pick.go
@@ -294,6 +294,8 @@ func (m *Model) pickKey(key string) tea.Cmd {
 		m.screen = screenInbox
 		m.inbox.sync(m)
 		return nil
+	case "/":
+		return m.openFilter()
 	case "g":
 		p.group = p.group.Next()
 		m.refreshRows()
```

Create `internal/tui/tokeninput.go`:

```go
package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// tokenInput is a one-line input in token syntax (D25, D26a) that completes
// the @requester or #tag at the cursor from known values. Quick-add and the
// filter bar share it.
type tokenInput struct{ input textinput.Model }

func newTokenInput(prompt, placeholder string, width int) tokenInput {
	return tokenInput{input: newInput(prompt, placeholder, width)}
}

func (ti *tokenInput) value() string { return ti.input.Value() }

func (ti *tokenInput) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	ti.input, cmd = ti.input.Update(msg)
	return cmd
}

func (ti *tokenInput) resize(width int) { fitInput(&ti.input, width) }

// word returns the value as runes and the bounds [start, end) of the word
// the cursor is in (or right after).
func (ti *tokenInput) word() (r []rune, start, end int) {
	r = []rune(ti.input.Value())
	pos := min(ti.input.Position(), len(r))
	start, end = pos, pos
	for start > 0 && r[start-1] != ' ' {
		start--
	}
	for end < len(r) && r[end] != ' ' {
		end++
	}
	return r, start, end
}

// suggestions completes the @requester or #tag being typed at the cursor,
// most used first. Nothing is completed inside a " -- " note.
func (ti *tokenInput) suggestions(m *Model) []string {
	r, start, _ := ti.word()
	tok := string(r[start:min(ti.input.Position(), len(r))])
	if tok == "" || (tok[0] != '@' && tok[0] != '#') || strings.Contains(" "+string(r[:start]), " -- ") {
		return nil
	}
	pool := m.idx.tags
	if tok[0] == '@' {
		pool = m.idx.requesters
	}
	prefix := strings.ToLower(tok[1:])
	var out []string
	for _, v := range pool {
		if strings.HasPrefix(v, prefix) && v != prefix {
			out = append(out, string(tok[0])+v)
			if len(out) == 5 {
				break
			}
		}
	}
	return out
}

// complete replaces the word at the cursor with s and moves past it.
func (ti *tokenInput) complete(s string) {
	r, start, end := ti.word()
	tail := string(r[end:])
	if tail == "" {
		tail = " "
	}
	ti.input.SetValue(string(r[:start]) + s + tail)
	ti.input.SetCursor(start + len([]rune(s)) + 1)
}

// view renders the input, err (why the last enter was refused) and the
// suggestions, when there are any.
func (ti *tokenInput) view(m *Model, err error) string {
	out := ti.input.View()
	if err != nil {
		out += "\n" + styleError.Render("✗ "+err.Error())
	}
	if s := ti.suggestions(m); len(s) > 0 {
		out += "\n" + styleDim.Render("  tab → "+strings.Join(s, "  "))
	}
	return out
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/common.go internal/tui/filter_test.go internal/tui/harness_test.go internal/tui/model.go internal/tui/overlays.go internal/tui/pick.go internal/tui/tokeninput.go
git commit -m "feat(tui): live filter bar and shared token input (D26a, D26e)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## After the last task

- Build into the scratchpad and drive the real binary through a Python `pty` script (start, `/`, type, `!9`, backspace, enter, esc, bracketed paste, `g`, resize, `q`), checking the stripped output.
- `go test -race ./internal/tui/ ./internal/app/ ./internal/store/`.
- Final whole-branch review, one fix wave, park residuals as spec carry-forwards for Plan 4b.
