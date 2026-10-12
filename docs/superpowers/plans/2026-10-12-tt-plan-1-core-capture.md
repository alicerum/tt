# tt Plan 1 — Core & Capture Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A tested Go core (domain rules, config, token parser, SQLite store, app service) plus a working `tt add "<line>"` command that captures tasks into the database.

**Architecture:** Pure packages (`domain`, `dsl`, `config`) have no I/O. `store` persists to SQLite in WAL mode. `app.Service` is the only code that mutates state and is shared by the CLI now and the TUI later. `cmd/tt` wires config → store → service.

**Tech Stack:** Go 1.27, `modernc.org/sqlite` v1.60.1 (pure Go), `github.com/BurntSushi/toml` v1.6.0, `github.com/google/go-cmp` v0.7.0 (tests only).

**Spec:** `docs/superpowers/specs/2026-10-12-task-tracking-design.md`

## Roadmap (this plan is 1 of 5)

| Plan | Delivers | Spec coverage |
|------|----------|---------------|
| **1. Core & capture** (this) | domain, config, quick-add DSL, SQLite store, `app.Service` task commands, `tt add` | D6, D7, D8, D11, D13–D16, D21, D23, D25, D25a, D29 (service side) |
| 2. Planning engine | pins, blocks, days, `StartDay` rollover, pinned-days, `CommitPlan`, default block length | D9, D10–D10d, D12a–D12d, D19, D22, D24 |
| 3. TUI: capture & grooming | Bubble Tea shell, DB watch loop, Inbox cards, Pick (grouping, filter DSL, fuzzy search), Task detail, quick-add overlay, `$EDITOR` notes, undo, help | D8a, D11a, D26–D27, D29, P1–P5 |
| 4. Calendar & Today/Block | EventKit spike → `tt-cal` Swift helper, `CalendarSource`, polling, cache, Today and Block views, conflicts | D3, D3a, D12, D17, D17a, D18, P4 |
| 5. GitHub reviews | `Source` interface, GitHub GraphQL adapter, reconcile + auto-close, Reviews lane, pin-to-wrap | D4, D20–D20b |

Each plan ends with software that works and is tested. Plans 2–5 are written after the previous one lands, against the code that actually exists.

## Global Constraints

- Go module path is `tt`; `go.mod` declares `go 1.27.2` (whatever `go mod init` writes with the installed Go 1.27.x).
- Dependencies for this plan: exactly `modernc.org/sqlite@v1.60.1`, `github.com/BurntSushi/toml@v1.6.0`, `github.com/google/go-cmp@v0.7.0`. Add no others.
- Only `internal/app` mutates state; `cmd/tt` calls `app.Service`, never `store` writes directly.
- Default paths: config `~/.config/tt/config.toml`, data `~/Library/Application Support/tt/tt.db`. Override with `TT_CONFIG` / `TT_DB`.
- SQLite opened with `foreign_keys(1)`, `journal_mode(WAL)`, `busy_timeout(5000)`, `_txlock=immediate`.
- Times stored as RFC 3339 (nanoseconds) in UTC.
- Tags and requesters are stored lowercase.
- Every task ends green on: `gofmt -l .` (no output), `go vet ./...`, `go test ./...`.
- Every commit message ends with a blank line and `Co-Authored-By: Claude <noreply@anthropic.com>`.

## Review Focus

1. **Look-alike tokens in titles**: `Review PR #482`, `clean ~/tmp`, `Fix this!`, `bob@x.com` must keep those words in the title. Pinned in Task 3 `TestParseAdd`.
2. **DB path with spaces**: the default path contains `Application Support`. Pinned in Task 4 `openTemp` (every store test uses such a path) and Task 6 `testEnv`.
3. **`tt add` while the TUI holds a write transaction**: the CLI must wait (busy timeout), not fail with `SQLITE_BUSY`, and the TUI must notice the write. Pinned in Task 4 `TestConcurrentWritersWaitInsteadOfFailing` and `TestDataVersionSeesOtherProcessWrites`.
4. **Unquoted `tt add` arguments**: `tt add Look into it #x` must equal the quoted form. Pinned in Task 6 `TestAddReportsWhereTaskLanded`.
5. **Case and Unicode in tags**: `#Impl` and `#impl` are one tag; `#Ünïcode` and emoji survive. Pinned in Task 1 `TestNormalizeTags` and Task 3 `TestParseAdd`.

## File Structure

```
go.mod, go.sum
.gitignore
cmd/tt/main.go                      CLI entry: `tt add`, paths, wiring
cmd/tt/main_test.go
internal/domain/task.go             Task, Status, Size, SourceRef, NormalizeTags
internal/domain/rules.go            EffectivePriority, EffectiveTags, InInbox
internal/domain/snapshot.go         Snapshot: Get, ParentOf, Inbox
internal/domain/domain_test.go
internal/config/config.go           Config, Default, Load, Parse
internal/config/config_test.go
internal/dsl/add.go                 ParseAdd, FormatAdd, Draft
internal/dsl/add_test.go
internal/store/store.go             Open, migrate, InTx, DataVersion, Close
internal/store/tasks.go             Tx task CRUD, LoadSnapshot, row mapping
internal/store/migrations/001_tasks.sql
internal/store/store_test.go
internal/app/service.go             Service, Clock, Store interface, Undo, modify
internal/app/tasks.go               Add, Edit, EditLine, SetNote, SetPriority, SetSize, SetParent, Done, Drop, Reopen, Delete
internal/app/service_test.go
```

---

### Task 1: Module scaffold and domain model

**Files:**
- Create: `go.mod`, `.gitignore`, `internal/domain/task.go`, `internal/domain/rules.go`, `internal/domain/snapshot.go`
- Test: `internal/domain/domain_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type Status string` with `StatusOpen`, `StatusDone`, `StatusDropped`; `ClosedByUser`, `ClosedBySource` string consts.
  - `type Size string` with `SizeNone ""`, `SizeS "s"`, `SizeM "m"`, `SizeL "l"`, `SizeXL "xl"`; `var Sizes []Size`; `func ParseSize(string) (Size, bool)` (accepts `x` as alias of `xl`).
  - `type SourceRef struct{ Kind, ID string }`.
  - `type Task struct` (fields: ID int64, Title, Note, Link, Requester string, Tags []string, Priority int (0 = unset), Size, ParentID int64 (0 = none), Position int, Status, DropReason string, Source *SourceRef, RolloverCount int, CreatedAt, UpdatedAt, ClosedAt time.Time, ClosedBy string).
  - `func NormalizeTags([]string) []string`, `func EffectivePriority(Task, *Task) int`, `func EffectiveTags(Task, *Task) []string`, `func InInbox(Task, *Task) bool`.
  - `type Snapshot`; `func NewSnapshot([]Task) Snapshot`; methods `Tasks() []Task`, `Get(int64) (Task, bool)`, `ParentOf(Task) *Task`, `Inbox() []Task` (oldest first, ties by ID).

- [ ] **Step 1: Initialize the module**

```bash
go mod init tt
go get github.com/google/go-cmp@v0.7.0
printf '/tt\n*.db\n*.db-shm\n*.db-wal\n' > .gitignore
```

- [ ] **Step 2: Write the failing tests**

`internal/domain/domain_test.go`:

```go
package domain

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestParseSize(t *testing.T) {
	cases := map[string]Size{"s": SizeS, "M": SizeM, "l": SizeL, "xl": SizeXL, "X": SizeXL}
	for in, want := range cases {
		got, ok := ParseSize(in)
		if !ok || got != want {
			t.Errorf("ParseSize(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "xs", "xxl", "/tmp"} {
		if _, ok := ParseSize(in); ok {
			t.Errorf("ParseSize(%q) ok = true; want false", in)
		}
	}
}

func TestNormalizeTags(t *testing.T) {
	got := NormalizeTags([]string{"Impl", " photos ", "impl", "", "Ünïcode", "🚀"})
	want := []string{"impl", "photos", "ünïcode", "🚀"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("NormalizeTags (-want +got):\n%s", diff)
	}
}

func TestEffectiveFields(t *testing.T) {
	parent := Task{ID: 1, Priority: 1, Tags: []string{"photos"}}
	child := Task{ID: 2, ParentID: 1, Priority: 3, Tags: []string{"impl"}}

	if got := EffectivePriority(child, &parent); got != 1 {
		t.Errorf("child priority = %d; want parent's 1", got)
	}
	if got := EffectivePriority(child, nil); got != 3 {
		t.Errorf("top-level priority = %d; want own 3", got)
	}
	if diff := cmp.Diff([]string{"impl", "photos"}, EffectiveTags(child, &parent)); diff != "" {
		t.Errorf("child tags (-want +got):\n%s", diff)
	}
}

func TestInInbox(t *testing.T) {
	triagedParent := &Task{Priority: 2}
	cases := []struct {
		name   string
		task   Task
		parent *Task
		want   bool
	}{
		{"title only", Task{Status: StatusOpen}, nil, true},
		{"priority only", Task{Status: StatusOpen, Priority: 1}, nil, true},
		{"size only", Task{Status: StatusOpen, Size: SizeS}, nil, true},
		{"priority and size", Task{Status: StatusOpen, Priority: 1, Size: SizeS}, nil, false},
		{"child with size inherits priority", Task{Status: StatusOpen, Size: SizeM}, triagedParent, false},
		{"child of untriaged parent", Task{Status: StatusOpen, Size: SizeM}, &Task{}, true},
		{"done", Task{Status: StatusDone}, nil, false},
		{"source-linked, unsized", Task{Status: StatusOpen, Source: &SourceRef{Kind: "github", ID: "a/b#1"}}, nil, false},
	}
	for _, c := range cases {
		if got := InInbox(c.task, c.parent); got != c.want {
			t.Errorf("%s: InInbox = %v; want %v", c.name, got, c.want)
		}
	}
}

func TestSnapshotInboxOrderAndParent(t *testing.T) {
	t0 := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	snap := NewSnapshot([]Task{
		{ID: 3, Status: StatusOpen, CreatedAt: t0.Add(2 * time.Hour)},
		{ID: 1, Status: StatusOpen, Priority: 1, Size: SizeXL, CreatedAt: t0},
		{ID: 2, Status: StatusOpen, CreatedAt: t0.Add(time.Hour)},
		{ID: 4, Status: StatusOpen, ParentID: 1, Size: SizeS, CreatedAt: t0},
	})
	var ids []int64
	for _, task := range snap.Inbox() {
		ids = append(ids, task.ID)
	}
	if diff := cmp.Diff([]int64{2, 3}, ids); diff != "" {
		t.Errorf("Inbox ids (-want +got):\n%s", diff)
	}
	child, _ := snap.Get(4)
	if p := snap.ParentOf(child); p == nil || p.ID != 1 {
		t.Errorf("ParentOf(4) = %v; want task 1", p)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/domain/`
Expected: FAIL to build: `undefined: Size`, `undefined: ParseSize`, …

- [ ] **Step 4: Implement the domain model**

`internal/domain/task.go`:

```go
// Package domain holds tt's entities and pure rules. It does no I/O.
package domain

import (
	"slices"
	"strings"
	"time"
)

type Status string

const (
	StatusOpen    Status = "open"
	StatusDone    Status = "done"
	StatusDropped Status = "dropped"
)

const (
	ClosedByUser   = "user"
	ClosedBySource = "source"
)

type Size string

const (
	SizeNone Size = ""
	SizeS    Size = "s"
	SizeM    Size = "m"
	SizeL    Size = "l"
	SizeXL   Size = "xl"
)

// Sizes lists all sizes, smallest first.
var Sizes = []Size{SizeS, SizeM, SizeL, SizeXL}

// ParseSize accepts s, m, l, xl and x (alias for xl), case-insensitive.
func ParseSize(s string) (Size, bool) {
	switch strings.ToLower(s) {
	case "s":
		return SizeS, true
	case "m":
		return SizeM, true
	case "l":
		return SizeL, true
	case "xl", "x":
		return SizeXL, true
	}
	return SizeNone, false
}

// SourceRef links a task to the source item it was materialized from (D20).
type SourceRef struct {
	Kind string // e.g. "github"
	ID   string // external id, e.g. "owner/repo#482"
}

type Task struct {
	ID            int64
	Title         string
	Note          string
	Link          string
	Requester     string   // lowercase; "" = none
	Tags          []string // normalized: lowercase, unique, sorted
	Priority      int      // 0 = unset; 1 = highest
	Size          Size
	ParentID      int64 // 0 = no parent
	Position      int   // order among siblings
	Status        Status
	DropReason    string
	Source        *SourceRef
	RolloverCount int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ClosedAt      time.Time // zero while open
	ClosedBy      string    // ClosedByUser / ClosedBySource; "" while open
}

// NormalizeTags lowercases, trims, drops empties, sorts and dedupes.
func NormalizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			out = append(out, t)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
```

`internal/domain/rules.go`:

```go
package domain

import "slices"

// EffectivePriority returns the priority that applies to t. Children always
// inherit their parent's priority; their own value is ignored (no override in v1).
func EffectivePriority(t Task, parent *Task) int {
	if parent != nil {
		return parent.Priority
	}
	return t.Priority
}

// EffectiveTags returns t's own tags merged with its parent's.
func EffectiveTags(t Task, parent *Task) []string {
	if parent == nil {
		return t.Tags
	}
	return NormalizeTags(append(slices.Clone(t.Tags), parent.Tags...))
}

// InInbox reports whether t still needs triage (D11): it is open, not
// source-linked, and lacks an effective priority or a size.
func InInbox(t Task, parent *Task) bool {
	if t.Status != StatusOpen || t.Source != nil {
		return false
	}
	return EffectivePriority(t, parent) == 0 || t.Size == SizeNone
}
```

`internal/domain/snapshot.go`:

```go
package domain

import (
	"cmp"
	"slices"
)

// Snapshot is an immutable in-memory view of all tasks.
type Snapshot struct {
	tasks []Task
	byID  map[int64]int
}

func NewSnapshot(tasks []Task) Snapshot {
	s := Snapshot{tasks: tasks, byID: make(map[int64]int, len(tasks))}
	for i, t := range tasks {
		s.byID[t.ID] = i
	}
	return s
}

func (s Snapshot) Tasks() []Task { return s.tasks }

func (s Snapshot) Get(id int64) (Task, bool) {
	i, ok := s.byID[id]
	if !ok {
		return Task{}, false
	}
	return s.tasks[i], true
}

// ParentOf returns t's parent, or nil if t has none.
func (s Snapshot) ParentOf(t Task) *Task {
	if t.ParentID == 0 {
		return nil
	}
	p, ok := s.Get(t.ParentID)
	if !ok {
		return nil
	}
	return &p
}

// Inbox returns tasks needing triage, oldest first.
func (s Snapshot) Inbox() []Task {
	var out []Task
	for _, t := range s.tasks {
		if InInbox(t, s.ParentOf(t)) {
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b Task) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID))
	})
	return out
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go mod tidy && gofmt -l . && go vet ./... && go test ./internal/domain/`
Expected: no gofmt output; `ok  tt/internal/domain`

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum .gitignore internal/domain
git commit -m "feat(domain): task model, inbox and inheritance rules" -m "Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 2: Config loading

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: `domain.Size`, `domain.ParseSize`, `domain.SizeS/M/L/XL` (Task 1).
- Produces:
  - `type PriorityLevel struct{ Name, Definition string }` (TOML keys `name`, `definition`).
  - `type Config struct{ DayStart time.Duration; Priorities []PriorityLevel; Sizes map[domain.Size]time.Duration }`.
  - `func Default() Config` (day start 04:00; P1/P2/P3 with definitions; S 1h, M 4h, L 8h, XL 40h).
  - `func Load(path string) (Config, error)` (missing file → `Default()`), `func Parse([]byte) (Config, error)`.

Config file format (all keys optional; `priorities`, if present, replaces the whole list; `sizes` entries override one by one; unknown keys are errors):

```toml
day_start = "04:00"

[[priorities]]
name = "P1"
definition = "someone is blocked / committed this week"

[sizes]
m = "3h"
```

- [ ] **Step 1: Add the dependency**

```bash
go get github.com/BurntSushi/toml@v1.6.0
```

- [ ] **Step 2: Write the failing tests**

`internal/config/config_test.go`:

```go
package config

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tt/internal/domain"
)

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DayStart != 4*time.Hour || len(cfg.Priorities) != 3 || cfg.Sizes[domain.SizeXL] != 40*time.Hour {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestParseOverrides(t *testing.T) {
	cfg, err := Parse([]byte(`
day_start = "00:00"

[[priorities]]
name = "Now"
definition = "today or tomorrow"

[[priorities]]
name = "Later"

[sizes]
m = "3h"
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DayStart != 0 {
		t.Errorf("DayStart = %v; want 0", cfg.DayStart)
	}
	if len(cfg.Priorities) != 2 || cfg.Priorities[0].Name != "Now" {
		t.Errorf("Priorities = %+v", cfg.Priorities)
	}
	if cfg.Sizes[domain.SizeM] != 3*time.Hour || cfg.Sizes[domain.SizeS] != time.Hour {
		t.Errorf("Sizes = %v; want m overridden, s default", cfg.Sizes)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		`day_start = "4am"`:                  "day_start",
		`day_start = "24:00"`:                "day_start",
		`priorities = []`:                    "1 to 9 levels",
		"[[priorities]]\ndefinition = \"x\"": "name is required",
		"[sizes]\nxs = \"1h\"":               "unknown size",
		"[sizes]\nx = \"1h\"":                "unknown size",
		"[sizes]\ns = \"soon\"":              "positive duration",
		`daystart = "04:00"`:                 "unknown config key",
	}
	for input, wantErr := range cases {
		_, err := Parse([]byte(input))
		if err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/config/`
Expected: FAIL to build: `undefined: Load`, `undefined: Parse`

- [ ] **Step 4: Implement config**

`internal/config/config.go`:

```go
// Package config loads ~/.config/tt/config.toml and applies defaults.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"tt/internal/domain"
)

type PriorityLevel struct {
	Name       string `toml:"name"`
	Definition string `toml:"definition"`
}

type Config struct {
	DayStart   time.Duration // offset from midnight when a new planning day begins (D19)
	Priorities []PriorityLevel
	Sizes      map[domain.Size]time.Duration
}

func Default() Config {
	return Config{
		DayStart: 4 * time.Hour,
		Priorities: []PriorityLevel{
			{Name: "P1", Definition: "someone is blocked / committed this week"},
			{Name: "P2", Definition: "important this month"},
			{Name: "P3", Definition: "worth doing eventually"},
		},
		Sizes: map[domain.Size]time.Duration{
			domain.SizeS:  time.Hour,
			domain.SizeM:  4 * time.Hour,
			domain.SizeL:  8 * time.Hour,
			domain.SizeXL: 40 * time.Hour,
		},
	}
}

// file mirrors the TOML layout; every field is optional.
type file struct {
	DayStart   *string           `toml:"day_start"`
	Priorities []PriorityLevel   `toml:"priorities"`
	Sizes      map[string]string `toml:"sizes"`
}

// Load reads path. A missing file yields Default().
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, err
	}
	cfg, err := Parse(data)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Parse overlays TOML data on Default(). Unknown keys are errors, so typos surface.
func Parse(data []byte) (Config, error) {
	var f file
	md, err := toml.Decode(string(data), &f)
	if err != nil {
		return Config{}, err
	}
	if und := md.Undecoded(); len(und) > 0 {
		return Config{}, fmt.Errorf("unknown config key %q", und[0].String())
	}

	cfg := Default()
	if f.DayStart != nil {
		if cfg.DayStart, err = parseClock(*f.DayStart); err != nil {
			return Config{}, fmt.Errorf("day_start: %w", err)
		}
	}
	if f.Priorities != nil {
		if n := len(f.Priorities); n < 1 || n > 9 {
			return Config{}, fmt.Errorf("priorities: need 1 to 9 levels, got %d", n)
		}
		for i, p := range f.Priorities {
			if strings.TrimSpace(p.Name) == "" {
				return Config{}, fmt.Errorf("priorities[%d]: name is required", i)
			}
		}
		cfg.Priorities = f.Priorities
	}
	for key, val := range f.Sizes {
		size, ok := domain.ParseSize(key)
		if !ok || string(size) != strings.ToLower(key) {
			return Config{}, fmt.Errorf("sizes: unknown size %q (use s, m, l, xl)", key)
		}
		d, err := time.ParseDuration(val)
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("sizes.%s: want a positive duration like \"4h\", got %q", key, val)
		}
		cfg.Sizes[size] = d
	}
	return cfg, nil
}

// parseClock parses "HH:MM" (00:00 to 23:59) into an offset from midnight.
func parseClock(s string) (time.Duration, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, fmt.Errorf("want HH:MM, got %q", s)
	}
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute, nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go mod tidy && gofmt -l . && go vet ./... && go test ./internal/config/`
Expected: no gofmt output; `ok  tt/internal/config`

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/config
git commit -m "feat(config): TOML config with defaults and validation" -m "Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 3: Quick-add token parser

**Files:**
- Create: `internal/dsl/add.go`
- Test: `internal/dsl/add_test.go`

**Interfaces:**
- Consumes: `domain.Size`, `domain.SizeNone`, `domain.ParseSize`, `domain.NormalizeTags` (Task 1).
- Produces:
  - `type Draft struct{ Title, Requester string; Tags []string; Priority int; Size domain.Size; Link, Note string; HasNote bool }`.
  - `var ErrEmptyTitle error`.
  - `func ParseAdd(line string, levels int) (Draft, error)`: `levels` = number of configured priorities.
  - `func FormatAdd(Draft) string`: inverse of `ParseAdd` (note included only if `HasNote`).

Rules (spec D25): `@name` requester (lowercased, last wins); `#tag` repeatable (an all-digit `#482` stays in the title); `!n` priority, error outside `1..levels`; `~s|m|l|xl|x` size (any other `~word` stays in the title); the first `http(s)://` token is the link; everything after the first ` -- ` is the note verbatim (also matches a leading `-- ` and a trailing ` --`); the remaining words, single-spaced, are the title; an empty title is `ErrEmptyTitle`.

- [ ] **Step 1: Write the failing tests**

`internal/dsl/add_test.go`:

```go
package dsl

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"tt/internal/domain"
)

func TestParseAdd(t *testing.T) {
	cases := []struct {
		line string
		want Draft
	}{
		{"Reply to Ana", Draft{Title: "Reply to Ana"}},
		{
			"Reply to Ana re schema @Ana #photos #Impl !1 ~s https://x.slack.com/archives/C1/p2",
			Draft{Title: "Reply to Ana re schema", Requester: "ana", Tags: []string{"impl", "photos"},
				Priority: 1, Size: domain.SizeS, Link: "https://x.slack.com/archives/C1/p2"},
		},
		{"tokens #a anywhere ~xl in @bob the line", Draft{Title: "tokens anywhere in the line", Requester: "bob", Tags: []string{"a"}, Size: domain.SizeXL}},
		{"~X alias", Draft{Title: "alias", Size: domain.SizeXL}},
		{"last @a requester @b wins", Draft{Title: "last requester wins", Requester: "b"}},
		{"duplicate #x #X tags", Draft{Title: "duplicate tags", Tags: []string{"x"}}},
		// Review Focus: ordinary words that look like tokens stay in the title.
		{"Review PR #482 today", Draft{Title: "Review PR #482 today"}},
		{"clean ~/tmp and ~zz", Draft{Title: "clean ~/tmp and ~zz"}},
		{"Fix this! now !!", Draft{Title: "Fix this! now !!"}},
		{"lone @ # ! ~ sigils", Draft{Title: "lone @ # ! ~ sigils"}},
		{"mail bob@x.com about #2", Draft{Title: "mail bob@x.com about #2"}},
		{"two https://a.io links https://b.io", Draft{Title: "two links https://b.io", Link: "https://a.io"}},
		{"🚀 launch #Ünïcode", Draft{Title: "🚀 launch", Tags: []string{"ünïcode"}}},
		// Notes.
		{"Call Ivan -- he needs it before Friday's sync", Draft{Title: "Call Ivan", Note: "he needs it before Friday's sync", HasNote: true}},
		{"Ping #infra -- see https://x.io #not-a-tag !1 -- twice", Draft{Title: "Ping", Tags: []string{"infra"}, Note: "see https://x.io #not-a-tag !1 -- twice", HasNote: true}},
		{"Trailing separator --", Draft{Title: "Trailing separator", HasNote: true}},
		{"Trailing separator -- ", Draft{Title: "Trailing separator", HasNote: true}},
		{"en--dash stays", Draft{Title: "en--dash stays"}},
	}
	for _, c := range cases {
		got, err := ParseAdd(c.line, 3)
		if err != nil {
			t.Errorf("ParseAdd(%q) error: %v", c.line, err)
			continue
		}
		if diff := cmp.Diff(c.want, got, cmp.Comparer(tagsEqual)); diff != "" {
			t.Errorf("ParseAdd(%q) (-want +got):\n%s", c.line, diff)
		}
	}
}

// tagsEqual treats nil and empty tag slices as equal.
func tagsEqual(a, b []string) bool { return strings.Join(a, ",") == strings.Join(b, ",") }

func TestParseAddErrors(t *testing.T) {
	for _, line := range []string{"", "   ", "@ana #x !1 ~s", "https://only.a.link", "-- just a note"} {
		if _, err := ParseAdd(line, 3); !errors.Is(err, ErrEmptyTitle) {
			t.Errorf("ParseAdd(%q) error = %v; want ErrEmptyTitle", line, err)
		}
	}
	for _, line := range []string{"x !0", "x !4", "x !99999999999999999999"} {
		_, err := ParseAdd(line, 3)
		if err == nil || !strings.Contains(err.Error(), "use !1 to !3") {
			t.Errorf("ParseAdd(%q) error = %v; want priority range error", line, err)
		}
	}
}

func TestFormatAddRoundTrip(t *testing.T) {
	drafts := []Draft{
		{Title: "Just a title"},
		{Title: "Review PR #482", Requester: "ana", Tags: []string{"impl", "photos"}, Priority: 2, Size: domain.SizeXL, Link: "https://x.io/a?b=c#d"},
		{Title: "With note", Note: "multi -- separators", HasNote: true},
		{Title: "Empty note", HasNote: true},
	}
	for _, d := range drafts {
		line := FormatAdd(d)
		got, err := ParseAdd(line, 3)
		if err != nil {
			t.Errorf("ParseAdd(FormatAdd(%+v)) = %q: %v", d, line, err)
			continue
		}
		if diff := cmp.Diff(d, got, cmp.Comparer(tagsEqual)); diff != "" {
			t.Errorf("round trip via %q (-want +got):\n%s", line, diff)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/dsl/`
Expected: FAIL to build: `undefined: Draft`, `undefined: ParseAdd`

- [ ] **Step 3: Implement the parser**

`internal/dsl/add.go`:

```go
// Package dsl parses tt's one-line token syntax (D25).
package dsl

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"tt/internal/domain"
)

var ErrEmptyTitle = errors.New("title is required")

// Draft is the result of parsing a quick-add line.
type Draft struct {
	Title     string
	Requester string // lowercase
	Tags      []string
	Priority  int // 0 = not given
	Size      domain.Size
	Link      string
	Note      string
	HasNote   bool // the line contained a " -- " separator, even if the note is empty
}

const noteSep = " -- "

// ParseAdd parses a quick-add line. levels is the number of configured
// priority levels; !n outside 1..levels is an error.
//
//	@name   requester (last one wins)
//	#tag    tag, repeatable (#123 stays in the title: it's an issue ref)
//	!n      priority
//	~size   s, m, l, xl or x (~anything-else stays in the title)
//	URL     first http(s) URL becomes the link; later URLs stay in the title
//	 -- …   everything after the first " -- " is the note, verbatim
func ParseAdd(line string, levels int) (Draft, error) {
	var d Draft
	// Padding lets " -- " also match a leading "-- " and a trailing " --".
	head := line
	if padded := " " + line + " "; strings.Contains(padded, noteSep) {
		i := strings.Index(padded, noteSep)
		head, d.Note, d.HasNote = padded[:i], strings.TrimSpace(padded[i+len(noteSep):]), true
	}

	var title, tags []string
	for _, tok := range strings.Fields(head) {
		switch sigil, rest := tok[0], tok[1:]; {
		case sigil == '@' && rest != "":
			d.Requester = strings.ToLower(rest)
		case sigil == '#' && rest != "" && !isDigits(rest):
			tags = append(tags, rest)
		case sigil == '!' && isDigits(rest):
			n, err := strconv.Atoi(rest)
			if err != nil || n < 1 || n > levels {
				return Draft{}, fmt.Errorf("priority %s out of range: use !1 to !%d", tok, levels)
			}
			d.Priority = n
		case sigil == '~' && isSize(rest):
			d.Size, _ = domain.ParseSize(rest)
		case d.Link == "" && isURL(tok):
			d.Link = tok
		default:
			title = append(title, tok)
		}
	}
	d.Title = strings.Join(title, " ")
	if d.Title == "" {
		return Draft{}, ErrEmptyTitle
	}
	d.Tags = domain.NormalizeTags(tags)
	return d, nil
}

// FormatAdd renders d back into token syntax (used to prefill the edit line).
// The note is included only when d.HasNote is set.
func FormatAdd(d Draft) string {
	parts := []string{d.Title}
	if d.Requester != "" {
		parts = append(parts, "@"+d.Requester)
	}
	for _, t := range d.Tags {
		parts = append(parts, "#"+t)
	}
	if d.Priority > 0 {
		parts = append(parts, "!"+strconv.Itoa(d.Priority))
	}
	if d.Size != domain.SizeNone {
		parts = append(parts, "~"+string(d.Size))
	}
	if d.Link != "" {
		parts = append(parts, d.Link)
	}
	s := strings.Join(parts, " ")
	if d.HasNote {
		s += noteSep + d.Note
	}
	return s
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isSize(s string) bool {
	_, ok := domain.ParseSize(s)
	return ok
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test ./internal/dsl/`
Expected: no gofmt output; `ok  tt/internal/dsl`

- [ ] **Step 5: Commit**

```bash
git add internal/dsl
git commit -m "feat(dsl): quick-add token parser and formatter" -m "Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 4: SQLite store

**Files:**
- Create: `internal/store/store.go`, `internal/store/tasks.go`, `internal/store/migrations/001_tasks.sql`
- Test: `internal/store/store_test.go`

**Interfaces:**
- Consumes: `domain.Task`, `domain.SourceRef`, `domain.Size`, `domain.Status`, `domain.NewSnapshot`, `domain.Snapshot` (Task 1).
- Produces:
  - `var ErrNotFound error` (wrapped as `task #<id>: not found`).
  - `func Open(ctx, path string) (*Store, error)`: path is a plain file path (spaces allowed). Its parent directory must exist. Applies all migrations.
  - `(*Store) Close() error`, `(*Store) DataVersion(ctx) (int64, error)`, `(*Store) InTx(ctx, func(*Tx) error) error`, `(*Store) LoadSnapshot(ctx) (domain.Snapshot, error)`.
  - `(*Tx) InsertTask(ctx, domain.Task) (int64, error)` (ignores `ID`), `(*Tx) UpdateTask(ctx, domain.Task) error` (all columns + tags), `(*Tx) GetTask(ctx, id) (domain.Task, error)`, `(*Tx) DeleteTask(ctx, id) error`, `(*Tx) HasChildren(ctx, id) (bool, error)`, `(*Tx) NextChildPosition(ctx, parentID) (int, error)`.
  - Migration files are `migrations/NNN_name.sql`, applied in order, once each, tracked in `schema_migrations`. Later plans add `002_…`.

Design notes for the implementer:
- `PRAGMA data_version` is **per connection**, so `Store` keeps one dedicated `*sql.Conn` (`watch`) just for reading it. Reading it through the pool would compare counters from different connections, which means nothing.
- Zero values map to SQL `NULL` for `priority`, `size`, `parent_id`, `closed_by`; `closed_at` is `NULL` while open.
- `_txlock=immediate` makes every transaction `BEGIN IMMEDIATE`, so two processes queue on the write lock (up to `busy_timeout`) instead of failing at commit.

- [ ] **Step 1: Add the dependency**

```bash
go get modernc.org/sqlite@v1.60.1
```

- [ ] **Step 2: Write the failing tests**

`internal/store/store_test.go`:

```go
package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"tt/internal/domain"
)

// openTemp opens a store in a directory whose name contains a space,
// like ~/Library/Application Support (Review Focus).
func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Application Support", "tt.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func sampleTask() domain.Task {
	now := time.Date(2026, 10, 12, 9, 30, 0, 123, time.UTC)
	return domain.Task{
		Title: "Review PR #482", Note: "line 1\nline 2", Link: "https://github.com/a/b/pull/482",
		Requester: "ivan", Tags: []string{"impl", "photos"}, Priority: 2, Size: domain.SizeM,
		Status: domain.StatusOpen, CreatedAt: now, UpdatedAt: now,
	}
}

func insert(t *testing.T, s *Store, task domain.Task) int64 {
	t.Helper()
	var id int64
	err := s.InTx(context.Background(), func(x *Tx) (err error) {
		id, err = x.InsertTask(context.Background(), task)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func get(t *testing.T, s *Store, id int64) (domain.Task, error) {
	t.Helper()
	var task domain.Task
	err := s.InTx(context.Background(), func(x *Tx) (err error) {
		task, err = x.GetTask(context.Background(), id)
		return err
	})
	return task, err
}

func TestOpenTwiceKeepsData(t *testing.T) {
	s, path := openTemp(t)
	id := insert(t, s, sampleTask())
	s.Close()

	again, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("reopen (migrations must be idempotent): %v", err)
	}
	defer again.Close()
	if _, err := get(t, again, id); err != nil {
		t.Fatal(err)
	}
}

func TestTaskRoundTrip(t *testing.T) {
	s, _ := openTemp(t)
	want := sampleTask()
	want.ID = insert(t, s, want)

	got, err := get(t, s, want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("round trip (-want +got):\n%s", diff)
	}

	want.Status, want.ClosedBy, want.DropReason = domain.StatusDropped, domain.ClosedByUser, "handed to @marko"
	want.ClosedAt = want.CreatedAt.Add(time.Hour)
	want.Tags = []string{"org"}
	want.Priority, want.Size = 0, domain.SizeNone
	want.Source = &domain.SourceRef{Kind: "github", ID: "a/b#482"}
	err = s.InTx(context.Background(), func(x *Tx) error { return x.UpdateTask(context.Background(), want) })
	if err != nil {
		t.Fatal(err)
	}
	got, _ = get(t, s, want.ID)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("after update (-want +got):\n%s", diff)
	}
}

func TestMissingTask(t *testing.T) {
	s, _ := openTemp(t)
	if _, err := get(t, s, 42); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetTask error = %v; want ErrNotFound", err)
	}
	err := s.InTx(context.Background(), func(x *Tx) error { return x.DeleteTask(context.Background(), 42) })
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteTask error = %v; want ErrNotFound", err)
	}
}

func TestDeleteParentOrphansChildren(t *testing.T) {
	s, _ := openTemp(t)
	parentID := insert(t, s, sampleTask())
	child := sampleTask()
	child.ParentID = parentID
	childID := insert(t, s, child)

	ctx := context.Background()
	err := s.InTx(ctx, func(x *Tx) error {
		if has, _ := x.HasChildren(ctx, parentID); !has {
			t.Error("HasChildren(parent) = false; want true")
		}
		if pos, _ := x.NextChildPosition(ctx, parentID); pos != 1 {
			t.Errorf("NextChildPosition = %d; want 1", pos)
		}
		return x.DeleteTask(ctx, parentID)
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := get(t, s, childID)
	if err != nil || got.ParentID != 0 {
		t.Errorf("child after parent delete: %+v, %v; want ParentID 0", got, err)
	}
}

func TestLoadSnapshot(t *testing.T) {
	s, _ := openTemp(t)
	a := insert(t, s, sampleTask())
	untagged := sampleTask()
	untagged.Tags = nil
	b := insert(t, s, untagged)

	snap, err := s.LoadSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Tasks()) != 2 {
		t.Fatalf("snapshot has %d tasks; want 2", len(snap.Tasks()))
	}
	ta, _ := snap.Get(a)
	tb, _ := snap.Get(b)
	if diff := cmp.Diff([]string{"impl", "photos"}, ta.Tags); diff != "" {
		t.Errorf("tags of a (-want +got):\n%s", diff)
	}
	if len(tb.Tags) != 0 {
		t.Errorf("tags of b = %v; want none", tb.Tags)
	}
}

func TestDataVersionSeesOtherProcessWrites(t *testing.T) {
	tui, path := openTemp(t)
	cli, err := Open(context.Background(), path) // a second process, e.g. `tt add`
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()

	ctx := context.Background()
	before, err := tui.DataVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := tui.DataVersion(ctx); again != before {
		t.Errorf("DataVersion changed without writes: %d → %d", before, again)
	}
	insert(t, cli, sampleTask())
	if after, _ := tui.DataVersion(ctx); after == before {
		t.Error("DataVersion did not change after another connection committed")
	}
}

func TestConcurrentWritersWaitInsteadOfFailing(t *testing.T) {
	a, path := openTemp(t)
	b, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	ctx := context.Background()
	holding := make(chan struct{})
	done := make(chan error)
	go func() {
		done <- a.InTx(ctx, func(x *Tx) error {
			if _, err := x.InsertTask(ctx, sampleTask()); err != nil {
				return err
			}
			close(holding)
			time.Sleep(300 * time.Millisecond) // hold the write lock
			return nil
		})
	}()
	<-holding
	insert(t, b, sampleTask()) // must wait for a, not fail with SQLITE_BUSY
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	snap, _ := b.LoadSnapshot(ctx)
	if n := len(snap.Tasks()); n != 2 {
		t.Errorf("got %d tasks; want 2", n)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/store/`
Expected: FAIL to build: `undefined: Store`, `undefined: Open`, …

- [ ] **Step 4: Write the migration**

`internal/store/migrations/001_tasks.sql`:

```sql
CREATE TABLE tasks (
    id             INTEGER PRIMARY KEY,
    title          TEXT    NOT NULL,
    note           TEXT    NOT NULL DEFAULT '',
    link           TEXT    NOT NULL DEFAULT '',
    requester      TEXT    NOT NULL DEFAULT '',
    priority       INTEGER,
    size           TEXT,
    parent_id      INTEGER REFERENCES tasks(id) ON DELETE SET NULL,
    position       INTEGER NOT NULL DEFAULT 0,
    status         TEXT    NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'done', 'dropped')),
    drop_reason    TEXT    NOT NULL DEFAULT '',
    source_kind    TEXT,
    source_id      TEXT,
    rollover_count INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT    NOT NULL,
    updated_at     TEXT    NOT NULL,
    closed_at      TEXT,
    closed_by      TEXT
);

CREATE UNIQUE INDEX tasks_source ON tasks (source_kind, source_id) WHERE source_kind IS NOT NULL;
CREATE INDEX tasks_parent ON tasks (parent_id);

CREATE TABLE task_tags (
    task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    tag     TEXT    NOT NULL,
    PRIMARY KEY (task_id, tag)
);
```

- [ ] **Step 5: Implement open, migrations and transactions**

`internal/store/store.go`:

```go
// Package store persists tt's data in SQLite (D13).
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

//go:embed migrations/*.sql
var migrationFS embed.FS

type Store struct {
	db    *sql.DB
	watch *sql.Conn // dedicated connection: PRAGMA data_version is per-connection
}

// Open opens (creating if needed) the database at path and applies migrations.
// path is a plain filesystem path; spaces are fine (e.g. "Application Support").
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	watch, err := db.Conn(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, watch: watch}, nil
}

func (s *Store) Close() error {
	return errors.Join(s.watch.Close(), s.db.Close())
}

// DataVersion changes whenever another connection or process commits a write.
// Poll it to notice external changes such as `tt add` from another shell.
func (s *Store) DataVersion(ctx context.Context) (int64, error) {
	var v int64
	err := s.watch.QueryRowContext(ctx, "PRAGMA data_version").Scan(&v)
	return v, err
}

func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	names, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	slices.Sort(names)
	for _, name := range names {
		base := strings.TrimPrefix(name, "migrations/")
		version, err := strconv.Atoi(base[:strings.IndexByte(base, '_')])
		if err != nil {
			return fmt.Errorf("migration %s: name must start with a number", name)
		}
		if err := applyMigration(ctx, db, name, version); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, name string, version int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var applied int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version = ?", version).Scan(&applied); err != nil {
		return err
	}
	if applied > 0 {
		return nil
	}
	body, err := migrationFS.ReadFile(name)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)",
		version, formatTime(time.Now())); err != nil {
		return err
	}
	return tx.Commit()
}

// InTx runs fn in a write transaction (BEGIN IMMEDIATE); fn's error rolls back.
func (s *Store) InTx(ctx context.Context, fn func(*Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(&Tx{tx: tx}); err != nil {
		return err
	}
	return tx.Commit()
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339Nano, s) }
```

- [ ] **Step 6: Implement task persistence**

`internal/store/tasks.go`:

```go
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"tt/internal/domain"
)

// Tx is a write transaction. Get one from Store.InTx.
type Tx struct{ tx *sql.Tx }

const taskColumns = `id, title, note, link, requester, priority, size, parent_id, position, status,
	drop_reason, source_kind, source_id, rollover_count, created_at, updated_at, closed_at, closed_by`

// InsertTask inserts t (ignoring t.ID) with its tags and returns the new id.
func (x *Tx) InsertTask(ctx context.Context, t domain.Task) (int64, error) {
	res, err := x.tx.ExecContext(ctx, `INSERT INTO tasks (`+strings.TrimPrefix(taskColumns, "id, ")+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, taskArgs(t)...)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, x.setTags(ctx, id, t.Tags)
}

// UpdateTask overwrites every column and the tag set of task t.ID.
func (x *Tx) UpdateTask(ctx context.Context, t domain.Task) error {
	res, err := x.tx.ExecContext(ctx, `UPDATE tasks SET title = ?, note = ?, link = ?, requester = ?,
		priority = ?, size = ?, parent_id = ?, position = ?, status = ?, drop_reason = ?, source_kind = ?,
		source_id = ?, rollover_count = ?, created_at = ?, updated_at = ?, closed_at = ?, closed_by = ?
		WHERE id = ?`, append(taskArgs(t), t.ID)...)
	if err != nil {
		return err
	}
	if err := mustAffect(res, t.ID); err != nil {
		return err
	}
	return x.setTags(ctx, t.ID, t.Tags)
}

func (x *Tx) GetTask(ctx context.Context, id int64) (domain.Task, error) {
	t, err := scanTask(x.tx.QueryRowContext(ctx, "SELECT "+taskColumns+" FROM tasks WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Task{}, fmt.Errorf("task #%d: %w", id, ErrNotFound)
	}
	if err != nil {
		return domain.Task{}, err
	}
	rows, err := x.tx.QueryContext(ctx, "SELECT tag FROM task_tags WHERE task_id = ? ORDER BY tag", id)
	if err != nil {
		return domain.Task{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return domain.Task{}, err
		}
		t.Tags = append(t.Tags, tag)
	}
	return t, rows.Err()
}

// DeleteTask hard-deletes a task. Its tags go with it; its children become top-level.
func (x *Tx) DeleteTask(ctx context.Context, id int64) error {
	res, err := x.tx.ExecContext(ctx, "DELETE FROM tasks WHERE id = ?", id)
	if err != nil {
		return err
	}
	return mustAffect(res, id)
}

func (x *Tx) HasChildren(ctx context.Context, id int64) (bool, error) {
	var n int
	err := x.tx.QueryRowContext(ctx, "SELECT count(*) FROM tasks WHERE parent_id = ?", id).Scan(&n)
	return n > 0, err
}

// NextChildPosition returns the position after parentID's last child.
func (x *Tx) NextChildPosition(ctx context.Context, parentID int64) (int, error) {
	var n int
	err := x.tx.QueryRowContext(ctx, "SELECT coalesce(max(position) + 1, 0) FROM tasks WHERE parent_id = ?", parentID).Scan(&n)
	return n, err
}

func (x *Tx) setTags(ctx context.Context, id int64, tags []string) error {
	if _, err := x.tx.ExecContext(ctx, "DELETE FROM task_tags WHERE task_id = ?", id); err != nil {
		return err
	}
	for _, tag := range tags {
		if _, err := x.tx.ExecContext(ctx, "INSERT INTO task_tags (task_id, tag) VALUES (?, ?)", id, tag); err != nil {
			return err
		}
	}
	return nil
}

// LoadSnapshot reads every task with its tags.
func (s *Store) LoadSnapshot(ctx context.Context) (domain.Snapshot, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+taskColumns+" FROM tasks ORDER BY id")
	if err != nil {
		return domain.Snapshot{}, err
	}
	defer rows.Close()
	var tasks []domain.Task
	index := map[int64]int{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return domain.Snapshot{}, err
		}
		index[t.ID] = len(tasks)
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return domain.Snapshot{}, err
	}

	tagRows, err := s.db.QueryContext(ctx, "SELECT task_id, tag FROM task_tags ORDER BY task_id, tag")
	if err != nil {
		return domain.Snapshot{}, err
	}
	defer tagRows.Close()
	for tagRows.Next() {
		var id int64
		var tag string
		if err := tagRows.Scan(&id, &tag); err != nil {
			return domain.Snapshot{}, err
		}
		if i, ok := index[id]; ok {
			tasks[i].Tags = append(tasks[i].Tags, tag)
		}
	}
	if err := tagRows.Err(); err != nil {
		return domain.Snapshot{}, err
	}
	return domain.NewSnapshot(tasks), nil
}

func taskArgs(t domain.Task) []any {
	var sourceKind, sourceID any
	if t.Source != nil {
		sourceKind, sourceID = t.Source.Kind, t.Source.ID
	}
	var closedAt any
	if !t.ClosedAt.IsZero() {
		closedAt = formatTime(t.ClosedAt)
	}
	return []any{t.Title, t.Note, t.Link, t.Requester, nullIfZero(t.Priority), nullIfZero(string(t.Size)),
		nullIfZero(t.ParentID), t.Position, string(t.Status), t.DropReason, sourceKind, sourceID,
		t.RolloverCount, formatTime(t.CreatedAt), formatTime(t.UpdatedAt), closedAt, nullIfZero(t.ClosedBy)}
}

type scanner interface{ Scan(dest ...any) error }

func scanTask(row scanner) (domain.Task, error) {
	var (
		t                                    domain.Task
		priority, parentID                   sql.NullInt64
		size, sourceKind, sourceID, closedBy sql.NullString
		status, createdAt, updatedAt         string
		closedAt                             sql.NullString
	)
	err := row.Scan(&t.ID, &t.Title, &t.Note, &t.Link, &t.Requester, &priority, &size, &parentID,
		&t.Position, &status, &t.DropReason, &sourceKind, &sourceID, &t.RolloverCount,
		&createdAt, &updatedAt, &closedAt, &closedBy)
	if err != nil {
		return domain.Task{}, err
	}
	t.Priority = int(priority.Int64)
	t.Size = domain.Size(size.String)
	t.ParentID = parentID.Int64
	t.Status = domain.Status(status)
	t.ClosedBy = closedBy.String
	if sourceKind.Valid {
		t.Source = &domain.SourceRef{Kind: sourceKind.String, ID: sourceID.String}
	}
	if t.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Task{}, err
	}
	if t.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.Task{}, err
	}
	if closedAt.Valid {
		if t.ClosedAt, err = parseTime(closedAt.String); err != nil {
			return domain.Task{}, err
		}
	}
	return t, nil
}

func nullIfZero[T comparable](v T) any {
	var zero T
	if v == zero {
		return nil
	}
	return v
}

func mustAffect(res sql.Result, id int64) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("task #%d: %w", id, ErrNotFound)
	}
	return nil
}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go mod tidy && gofmt -l . && go vet ./... && go test ./internal/store/`
Expected: no gofmt output; `ok  tt/internal/store` (the concurrency test takes ~0.3s)

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum internal/store
git commit -m "feat(store): SQLite store with migrations, task CRUD and data_version watch" -m "Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 5: App service: task commands with undo

**Files:**
- Create: `internal/app/service.go`, `internal/app/tasks.go`
- Test: `internal/app/service_test.go`

**Interfaces:**
- Consumes: `store.Store`, `store.Tx` and its methods, `store.ErrNotFound` (Task 4); `dsl.ParseAdd`, `dsl.FormatAdd`, `dsl.Draft` (Task 3); `config.Config`, `config.Default` (Task 2); `domain.*` (Task 1).
- Produces:
  - `type Clock interface{ Now() time.Time }`, `type SystemClock struct{}`.
  - `type Store interface{ InTx; LoadSnapshot; DataVersion }` (satisfied by `*store.Store`).
  - `type Undo func(ctx context.Context) error`.
  - `func New(Store, Clock, config.Config) *Service`; `(*Service) Config() config.Config`, `Snapshot(ctx) (domain.Snapshot, error)`, `DataVersion(ctx) (int64, error)`.
  - `var ErrNotFound` (= `store.ErrNotFound`), `ErrInvalidParent`, `ErrInvalidValue`.
  - `Add(ctx, line) (domain.Task, Undo, error)`; `func EditLine(domain.Task) string`; `Edit(ctx, id, line) (Undo, error)`; `SetNote(ctx, id, note) (Undo, error)`; `SetPriority(ctx, id, p int) (Undo, error)` (0 clears); `SetSize(ctx, id, domain.Size) (Undo, error)` (`SizeNone` clears); `SetParent(ctx, id, parentID int64) (Undo, error)` (0 detaches); `Done`, `Reopen(ctx, id) (Undo, error)`; `Drop(ctx, id, reason) (Undo, error)`; `Delete(ctx, id) error` (not undoable, per spec).

Semantics (spec D6, D23, D25, D29, P1):
- `Edit` treats the line as the full truth for title, requester, tags, priority, size and link: a missing token clears the field. The note changes only if the line contains ` -- `. `EditLine` produces the prefill (no note).
- `SetParent`: no self-parenting, the parent must not itself be a child, and a task with children can't become a child. The new child is appended last (`Position`).
- `Done`/`Drop` set `ClosedAt = clock.Now()` and `ClosedBy = "user"`; `Reopen` clears both and the drop reason.
- Every mutating command updates `UpdatedAt`; its `Undo` restores the full previous row.

- [ ] **Step 1: Write the failing tests**

`internal/app/service_test.go`:

```go
package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/store"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newService(t *testing.T) (*Service, *fakeClock) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "tt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	clock := &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
	return New(st, clock, config.Default()), clock
}

func mustAdd(t *testing.T, s *Service, line string) domain.Task {
	t.Helper()
	task, _, err := s.Add(context.Background(), line)
	if err != nil {
		t.Fatalf("Add(%q): %v", line, err)
	}
	return task
}

func mustGet(t *testing.T, s *Service, id int64) domain.Task {
	t.Helper()
	snap, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	task, ok := snap.Get(id)
	if !ok {
		t.Fatalf("task #%d not in snapshot", id)
	}
	return task
}

// mustUndo returns a helper that fails the test on error: must(s.Done(ctx, id)).
func mustUndo(t *testing.T) func(Undo, error) Undo {
	return func(undo Undo, err error) Undo {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return undo
	}
}

func TestAddAndUndo(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()

	task, undo, err := s.Add(ctx, "Reply to Ana @ana #photos !1 ~s https://x.io -- before Friday")
	if err != nil {
		t.Fatal(err)
	}
	got := mustGet(t, s, task.ID)
	want := domain.Task{
		ID: task.ID, Title: "Reply to Ana", Note: "before Friday", Link: "https://x.io", Requester: "ana",
		Tags: []string{"photos"}, Priority: 1, Size: domain.SizeS, Status: domain.StatusOpen,
		CreatedAt: got.CreatedAt, UpdatedAt: got.UpdatedAt,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("added task (-want +got):\n%s", diff)
	}
	if domain.InInbox(got, nil) {
		t.Error("task with priority and size should skip the Inbox")
	}

	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Snapshot(ctx)
	if len(snap.Tasks()) != 0 {
		t.Error("undo of Add should delete the task")
	}
}

func TestAddRejectsBadLines(t *testing.T) {
	s, _ := newService(t)
	for _, line := range []string{"", "@ana #x", "x !4"} {
		if _, _, err := s.Add(context.Background(), line); err == nil {
			t.Errorf("Add(%q) succeeded; want error", line)
		}
	}
}

func TestEditReplacesFieldsAndKeepsNote(t *testing.T) {
	must := mustUndo(t)
	s, clock := newService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Old title @ana #a !1 ~s -- keep me")

	if line := EditLine(mustGet(t, s, task.ID)); line != "Old title @ana #a !1 ~s" {
		t.Errorf("EditLine = %q", line)
	}

	clock.Advance(time.Minute)
	undo := must(s.Edit(ctx, task.ID, "New title #b ~l"))
	got := mustGet(t, s, task.ID)
	if got.Title != "New title" || got.Requester != "" || got.Priority != 0 || got.Size != domain.SizeL ||
		!cmp.Equal(got.Tags, []string{"b"}) || got.Note != "keep me" {
		t.Errorf("after Edit: %+v", got)
	}
	if !got.UpdatedAt.Equal(clock.now) {
		t.Errorf("UpdatedAt = %v; want %v", got.UpdatedAt, clock.now)
	}

	must(s.Edit(ctx, task.ID, "New title -- "))
	if got := mustGet(t, s, task.ID); got.Note != "" {
		t.Errorf("note = %q; an explicit empty note should clear it", got.Note)
	}

	// Undo restores the state before the first edit, note included.
	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, s, task.ID); got.Title != "Old title" || got.Note != "keep me" || got.Requester != "ana" {
		t.Errorf("after undo: %+v", got)
	}
}

func TestSetNote(t *testing.T) {
	must := mustUndo(t)
	s, _ := newService(t)
	task := mustAdd(t, s, "Task")
	must(s.SetNote(context.Background(), task.ID, "# Heading\n\nbody\n\n"))
	if got := mustGet(t, s, task.ID).Note; got != "# Heading\n\nbody" {
		t.Errorf("note = %q; want trailing newlines trimmed", got)
	}
}

func TestTriageSetters(t *testing.T) {
	must := mustUndo(t)
	s, _ := newService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Triage me")

	must(s.SetPriority(ctx, task.ID, 2))
	undo := must(s.SetSize(ctx, task.ID, domain.SizeXL))
	got := mustGet(t, s, task.ID)
	if got.Priority != 2 || got.Size != domain.SizeXL || domain.InInbox(got, nil) {
		t.Errorf("after triage: %+v", got)
	}
	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, s, task.ID); got.Size != domain.SizeNone || got.Priority != 2 {
		t.Errorf("undo of SetSize: %+v", got)
	}

	for _, p := range []int{-1, 4} {
		if _, err := s.SetPriority(ctx, task.ID, p); !errors.Is(err, ErrInvalidValue) {
			t.Errorf("SetPriority(%d) error = %v; want ErrInvalidValue", p, err)
		}
	}
	for _, size := range []domain.Size{"x", "xs"} {
		if _, err := s.SetSize(ctx, task.ID, size); !errors.Is(err, ErrInvalidValue) {
			t.Errorf("SetSize(%q) error = %v; want ErrInvalidValue", size, err)
		}
	}
	if _, err := s.SetPriority(ctx, 999, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetPriority on missing task error = %v; want ErrNotFound", err)
	}
}

func TestSetParentRules(t *testing.T) {
	must := mustUndo(t)
	s, _ := newService(t)
	ctx := context.Background()
	parent := mustAdd(t, s, "Auth migration !1 #photos ~xl")
	a := mustAdd(t, s, "Rollback plan ~s")
	b := mustAdd(t, s, "Token refresh spike ~m")
	other := mustAdd(t, s, "Other")

	must(s.SetParent(ctx, a.ID, parent.ID))
	undo := must(s.SetParent(ctx, b.ID, parent.ID))

	snap, _ := s.Snapshot(ctx)
	gotA, _ := snap.Get(a.ID)
	gotB, _ := snap.Get(b.ID)
	if gotA.Position != 0 || gotB.Position != 1 {
		t.Errorf("positions = %d, %d; want 0, 1", gotA.Position, gotB.Position)
	}
	if domain.InInbox(gotB, snap.ParentOf(gotB)) {
		t.Error("sized child of a prioritized parent should not be in the Inbox")
	}

	cases := []struct {
		name          string
		child, parent int64
	}{
		{"self", other.ID, other.ID},
		{"grandchild", other.ID, a.ID},
		{"parent becoming a child", parent.ID, other.ID},
	}
	for _, c := range cases {
		if _, err := s.SetParent(ctx, c.child, c.parent); !errors.Is(err, ErrInvalidParent) {
			t.Errorf("%s: error = %v; want ErrInvalidParent", c.name, err)
		}
	}
	if _, err := s.SetParent(ctx, other.ID, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing parent: error = %v; want ErrNotFound", err)
	}

	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, s, b.ID); got.ParentID != 0 {
		t.Errorf("undo of SetParent: ParentID = %d; want 0", got.ParentID)
	}
	must(s.SetParent(ctx, a.ID, 0))
	if got := mustGet(t, s, a.ID); got.ParentID != 0 {
		t.Errorf("detach: ParentID = %d; want 0", got.ParentID)
	}
}

func TestLifecycle(t *testing.T) {
	must := mustUndo(t)
	s, clock := newService(t)
	ctx := context.Background()
	task := mustAdd(t, s, "Lifecycle !1 ~s")

	clock.Advance(time.Hour)
	undo := must(s.Done(ctx, task.ID))
	got := mustGet(t, s, task.ID)
	if got.Status != domain.StatusDone || got.ClosedBy != domain.ClosedByUser || !got.ClosedAt.Equal(clock.now) {
		t.Errorf("after Done: %+v", got)
	}
	if err := undo(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, s, task.ID); got.Status != domain.StatusOpen || !got.ClosedAt.IsZero() {
		t.Errorf("after undo Done: %+v", got)
	}

	must(s.Drop(ctx, task.ID, "  handed to @marko  "))
	if got := mustGet(t, s, task.ID); got.Status != domain.StatusDropped || got.DropReason != "handed to @marko" {
		t.Errorf("after Drop: %+v", got)
	}
	must(s.Reopen(ctx, task.ID))
	if got := mustGet(t, s, task.ID); got.Status != domain.StatusOpen || got.DropReason != "" || got.ClosedBy != "" {
		t.Errorf("after Reopen: %+v", got)
	}

	if err := s.Delete(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, task.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete error = %v; want ErrNotFound", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/`
Expected: FAIL to build: `undefined: Service`, `undefined: New`, …

- [ ] **Step 3: Implement the service core**

`internal/app/service.go`:

```go
// Package app is the only place that changes tt's state (spec: Architecture).
// The TUI and the CLI both call Service.
package app

import (
	"context"
	"time"

	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/store"
)

type Clock interface{ Now() time.Time }

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

// Store is what Service needs from persistence; *store.Store implements it.
type Store interface {
	InTx(ctx context.Context, fn func(*store.Tx) error) error
	LoadSnapshot(ctx context.Context) (domain.Snapshot, error)
	DataVersion(ctx context.Context) (int64, error)
}

// Undo reverts the command that returned it. The TUI keeps these on a
// session-only undo stack (spec: Open items P2).
type Undo func(ctx context.Context) error

type Service struct {
	store Store
	clock Clock
	cfg   config.Config
}

func New(st Store, clock Clock, cfg config.Config) *Service {
	return &Service{store: st, clock: clock, cfg: cfg}
}

func (s *Service) Config() config.Config { return s.cfg }

func (s *Service) Snapshot(ctx context.Context) (domain.Snapshot, error) {
	return s.store.LoadSnapshot(ctx)
}

func (s *Service) DataVersion(ctx context.Context) (int64, error) {
	return s.store.DataVersion(ctx)
}

// modify loads task id, applies fn, saves it, and returns an Undo that
// restores the previous row.
func (s *Service) modify(ctx context.Context, id int64, fn func(x *store.Tx, t *domain.Task) error) (Undo, error) {
	var before domain.Task
	err := s.store.InTx(ctx, func(x *store.Tx) error {
		var err error
		if before, err = x.GetTask(ctx, id); err != nil {
			return err
		}
		after := before
		after.Tags = append([]string(nil), before.Tags...)
		if err := fn(x, &after); err != nil {
			return err
		}
		after.UpdatedAt = s.clock.Now()
		return x.UpdateTask(ctx, after)
	})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) error {
		return s.store.InTx(ctx, func(x *store.Tx) error { return x.UpdateTask(ctx, before) })
	}, nil
}
```

- [ ] **Step 4: Implement the task commands**

`internal/app/tasks.go`:

```go
package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"tt/internal/domain"
	"tt/internal/dsl"
	"tt/internal/store"
)

var (
	ErrNotFound      = store.ErrNotFound
	ErrInvalidParent = errors.New("invalid parent")
	ErrInvalidValue  = errors.New("invalid value")
)

// Add captures a task from a quick-add line (D25).
func (s *Service) Add(ctx context.Context, line string) (domain.Task, Undo, error) {
	d, err := dsl.ParseAdd(line, len(s.cfg.Priorities))
	if err != nil {
		return domain.Task{}, nil, err
	}
	now := s.clock.Now()
	t := domain.Task{
		Title: d.Title, Note: d.Note, Link: d.Link, Requester: d.Requester, Tags: d.Tags,
		Priority: d.Priority, Size: d.Size, Status: domain.StatusOpen, CreatedAt: now, UpdatedAt: now,
	}
	err = s.store.InTx(ctx, func(x *store.Tx) (err error) {
		t.ID, err = x.InsertTask(ctx, t)
		return err
	})
	if err != nil {
		return domain.Task{}, nil, err
	}
	id := t.ID
	undo := func(ctx context.Context) error {
		return s.store.InTx(ctx, func(x *store.Tx) error { return x.DeleteTask(ctx, id) })
	}
	return t, undo, nil
}

// EditLine renders a task as a token line for prefilling the edit input.
// The note is left out; Edit keeps it unless the new line has " -- ".
func EditLine(t domain.Task) string {
	return dsl.FormatAdd(dsl.Draft{
		Title: t.Title, Requester: t.Requester, Tags: t.Tags, Priority: t.Priority, Size: t.Size, Link: t.Link,
	})
}

// Edit replaces title, requester, tags, priority, size and link from line;
// a token missing from line clears that field. The note changes only if
// line contains " -- ".
func (s *Service) Edit(ctx context.Context, id int64, line string) (Undo, error) {
	d, err := dsl.ParseAdd(line, len(s.cfg.Priorities))
	if err != nil {
		return nil, err
	}
	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
		t.Title, t.Requester, t.Tags, t.Priority, t.Size, t.Link = d.Title, d.Requester, d.Tags, d.Priority, d.Size, d.Link
		if d.HasNote {
			t.Note = d.Note
		}
		return nil
	})
}

// SetNote replaces the note (used after editing it in $EDITOR, D29).
func (s *Service) SetNote(ctx context.Context, id int64, note string) (Undo, error) {
	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
		t.Note = strings.TrimRight(note, "\n")
		return nil
	})
}

// SetPriority sets priority p (1..levels); 0 clears it.
func (s *Service) SetPriority(ctx context.Context, id int64, p int) (Undo, error) {
	if p < 0 || p > len(s.cfg.Priorities) {
		return nil, fmt.Errorf("priority %d: %w (use 1 to %d, or 0 to clear)", p, ErrInvalidValue, len(s.cfg.Priorities))
	}
	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
		t.Priority = p
		return nil
	})
}

// SetSize sets the size; domain.SizeNone clears it.
func (s *Service) SetSize(ctx context.Context, id int64, size domain.Size) (Undo, error) {
	if size != domain.SizeNone && !slices.Contains(domain.Sizes, size) {
		return nil, fmt.Errorf("size %q: %w", size, ErrInvalidValue)
	}
	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
		t.Size = size
		return nil
	})
}

// SetParent makes id a child of parentID (appended last); 0 detaches it.
// Only one level of nesting is allowed (D6).
func (s *Service) SetParent(ctx context.Context, id, parentID int64) (Undo, error) {
	return s.modify(ctx, id, func(x *store.Tx, t *domain.Task) error {
		if parentID == 0 {
			t.ParentID, t.Position = 0, 0
			return nil
		}
		if parentID == id {
			return fmt.Errorf("%w: a task can't be its own parent", ErrInvalidParent)
		}
		parent, err := x.GetTask(ctx, parentID)
		if err != nil {
			return err
		}
		if parent.ParentID != 0 {
			return fmt.Errorf("%w: #%d is already a child; only one level of nesting", ErrInvalidParent, parentID)
		}
		if has, err := x.HasChildren(ctx, id); err != nil {
			return err
		} else if has {
			return fmt.Errorf("%w: #%d has children; only one level of nesting", ErrInvalidParent, id)
		}
		pos, err := x.NextChildPosition(ctx, parentID)
		if err != nil {
			return err
		}
		t.ParentID, t.Position = parentID, pos
		return nil
	})
}

// Done marks a task done.
func (s *Service) Done(ctx context.Context, id int64) (Undo, error) {
	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
		t.Status, t.ClosedAt, t.ClosedBy, t.DropReason = domain.StatusDone, s.clock.Now(), domain.ClosedByUser, ""
		return nil
	})
}

// Drop marks a task dropped with an optional reason (D23).
func (s *Service) Drop(ctx context.Context, id int64, reason string) (Undo, error) {
	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
		t.Status, t.ClosedAt, t.ClosedBy, t.DropReason = domain.StatusDropped, s.clock.Now(), domain.ClosedByUser, strings.TrimSpace(reason)
		return nil
	})
}

// Reopen returns a done or dropped task to open.
func (s *Service) Reopen(ctx context.Context, id int64) (Undo, error) {
	return s.modify(ctx, id, func(_ *store.Tx, t *domain.Task) error {
		t.Status, t.ClosedAt, t.ClosedBy, t.DropReason = domain.StatusOpen, time.Time{}, "", ""
		return nil
	})
}

// Delete hard-deletes a task (D23). It is not undoable; the TUI confirms first.
// Children of a deleted parent become top-level.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.store.InTx(ctx, func(x *store.Tx) error { return x.DeleteTask(ctx, id) })
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test ./internal/app/`
Expected: no gofmt output; `ok  tt/internal/app`

- [ ] **Step 6: Commit**

```bash
git add internal/app
git commit -m "feat(app): task commands with undo" -m "Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 6: `tt add` command

**Files:**
- Create: `cmd/tt/main.go`
- Test: `cmd/tt/main_test.go`

**Interfaces:**
- Consumes: `app.New`, `app.SystemClock`, `(*app.Service).Add` (Task 5); `config.Load` (Task 2); `store.Open`, `(*store.Store).Close` (Task 4); `domain.InInbox` (Task 1).
- Produces: the `tt` binary. `tt add <words…>` prints `added #<id> to inbox|backlog` and exits 0. Usage errors exit 2 with usage on stderr. Other errors exit 1 with `tt: <message>` on stderr. `run(ctx, args, stdout, stderr, getenv) int` and `paths(getenv) (configPath, dbPath string, err error)` are the testable seams.

- [ ] **Step 1: Write the failing tests**

`cmd/tt/main_test.go`:

```go
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tt/internal/store"
)

type result struct {
	code           int
	stdout, stderr string
}

func runTT(t *testing.T, env map[string]string, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, &out, &errOut, func(k string) string { return env[k] })
	return result{code, out.String(), errOut.String()}
}

func testEnv(t *testing.T) map[string]string {
	dir := t.TempDir()
	return map[string]string{
		"TT_CONFIG": filepath.Join(dir, "missing.toml"),
		"TT_DB":     filepath.Join(dir, "data dir", "tt.db"),
	}
}

func TestAddReportsWhereTaskLanded(t *testing.T) {
	env := testEnv(t)
	if r := runTT(t, env, "add", "Reply to Ana @ana !1 ~s"); r.code != 0 || r.stdout != "added #1 to backlog\n" {
		t.Errorf("triaged add: %+v", r)
	}
	// Unquoted words are joined, as a shell would split them.
	if r := runTT(t, env, "add", "Look", "into", "flaky", "e2e", "#profiles"); r.code != 0 || r.stdout != "added #2 to inbox\n" {
		t.Errorf("untriaged add: %+v", r)
	}

	st, err := store.Open(context.Background(), env["TT_DB"])
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	snap, _ := st.LoadSnapshot(context.Background())
	task, ok := snap.Get(2)
	if !ok || task.Title != "Look into flaky e2e" || task.Tags[0] != "profiles" {
		t.Errorf("stored task: %+v", task)
	}
}

func TestUsageErrors(t *testing.T) {
	env := testEnv(t)
	for _, args := range [][]string{nil, {"bogus"}, {"add"}, {"add", "  "}} {
		if r := runTT(t, env, args...); r.code != 2 || !strings.Contains(r.stderr, "usage:") {
			t.Errorf("tt %v: %+v; want exit 2 with usage", args, r)
		}
	}
}

func TestAddErrorsExitNonZero(t *testing.T) {
	env := testEnv(t)
	if r := runTT(t, env, "add", "x !9"); r.code != 1 || !strings.Contains(r.stderr, "use !1 to !3") {
		t.Errorf("bad priority: %+v", r)
	}
	if r := runTT(t, env, "add", "@ana #only-tokens"); r.code != 1 || !strings.Contains(r.stderr, "title is required") {
		t.Errorf("no title: %+v", r)
	}
}

func TestConfigIsHonoured(t *testing.T) {
	env := testEnv(t)
	if err := os.WriteFile(env["TT_CONFIG"], []byte("[[priorities]]\nname = \"Now\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := runTT(t, env, "add", "x !2"); r.code != 1 || !strings.Contains(r.stderr, "use !1 to !1") {
		t.Errorf("one configured level: %+v", r)
	}
	if err := os.WriteFile(env["TT_CONFIG"], []byte("bogus = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := runTT(t, env, "add", "x"); r.code != 1 || !strings.Contains(r.stderr, "unknown config key") {
		t.Errorf("broken config: %+v", r)
	}
}

func TestDefaultPaths(t *testing.T) {
	cfg, db, err := paths(func(k string) string { return map[string]string{"HOME": "/Users/me"}[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg != "/Users/me/.config/tt/config.toml" || db != "/Users/me/Library/Application Support/tt/tt.db" {
		t.Errorf("paths = %q, %q", cfg, db)
	}
	if _, _, err := paths(func(string) string { return "" }); err == nil {
		t.Error("paths without HOME should fail")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/tt/`
Expected: FAIL to build: `undefined: run`, `undefined: paths`

- [ ] **Step 3: Implement the command**

`cmd/tt/main.go`:

```go
// Command tt is a personal task tracker. See docs/superpowers/specs.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"tt/internal/app"
	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/store"
)

const usage = `usage:
  tt add <line>    capture a task, e.g. tt add "Reply to Ana @ana #photos !1 ~s -- before Friday"

environment:
  TT_CONFIG   config file (default ~/.config/tt/config.toml)
  TT_DB       database file (default ~/Library/Application Support/tt/tt.db)
`

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

// run is main without process globals, so tests can call it directly.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	if len(args) == 0 || args[0] != "add" {
		fmt.Fprint(stderr, usage)
		return 2
	}
	// Join the rest so both `tt add "a b"` and `tt add a b` work.
	line := strings.TrimSpace(strings.Join(args[1:], " "))
	if line == "" {
		fmt.Fprint(stderr, usage)
		return 2
	}

	svc, closeFn, err := open(ctx, getenv)
	if err != nil {
		fmt.Fprintf(stderr, "tt: %v\n", err)
		return 1
	}
	defer closeFn()

	task, _, err := svc.Add(ctx, line)
	if err != nil {
		fmt.Fprintf(stderr, "tt: %v\n", err)
		return 1
	}
	where := "backlog"
	if domain.InInbox(task, nil) {
		where = "inbox"
	}
	fmt.Fprintf(stdout, "added #%d to %s\n", task.ID, where)
	return 0
}

func open(ctx context.Context, getenv func(string) string) (*app.Service, func(), error) {
	configPath, dbPath, err := paths(getenv)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, nil, err
	}
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return nil, nil, err
	}
	return app.New(st, app.SystemClock{}, cfg), func() { st.Close() }, nil
}

func paths(getenv func(string) string) (configPath, dbPath string, err error) {
	configPath, dbPath = getenv("TT_CONFIG"), getenv("TT_DB")
	if configPath != "" && dbPath != "" {
		return configPath, dbPath, nil
	}
	home := getenv("HOME")
	if home == "" {
		return "", "", fmt.Errorf("HOME is not set; set TT_CONFIG and TT_DB")
	}
	if configPath == "" {
		configPath = filepath.Join(home, ".config", "tt", "config.toml")
	}
	if dbPath == "" {
		dbPath = filepath.Join(home, "Library", "Application Support", "tt", "tt.db")
	}
	return configPath, dbPath, nil
}
```

- [ ] **Step 4: Run the full suite and a smoke test**

```bash
gofmt -l . && go vet ./... && go test ./...
go build -o tt ./cmd/tt
TT_DB="$(mktemp -d)/smoke dir/tt.db" TT_CONFIG=/nonexistent ./tt add "Hello @me ~s !2 -- note"
```

Expected: all packages `ok`, then `added #1 to backlog`. The `smoke dir` directory doesn't exist beforehand; `open` creates it. Then `rm tt`.

- [ ] **Step 5: Commit**

```bash
git add cmd/tt
git commit -m "feat(cli): tt add captures tasks from the shell" -m "Co-Authored-By: Claude <noreply@anthropic.com>"
```
