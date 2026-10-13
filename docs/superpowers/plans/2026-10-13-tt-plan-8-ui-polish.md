# tt Plan 8 — UI Polish Implementation Plan

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
### Task 1: Frame, `within` and `cutHints`: the pure drawing helpers

Three small helpers in a new `internal/tui/frame.go`, tested on their own before any screen uses them. `frame` draws a rounded box with its title in the top border (accent colour ANSI 4 when focused, dim otherwise), every line padded or cut to the inner width, exactly `height` lines. `within` renders a line in an outer style and re-applies that style after every reset inside it, so a struck-through or coloured part no longer ends a selected or dimmed row's style early. It builds the escape sequence by hand, because Lip Gloss restyles a string that already holds escapes one character at a time. `cutHints` fits a hints line: whole hints are dropped at a ` · ` boundary, `…` marks the cut, and `? help`, when present, stays at the right edge. Nothing on screen changes in this task.

**Files:**
- Create: `internal/tui/frame.go`
- Create: `internal/tui/frame_test.go`

**Interfaces:**
- Consumes: existing `pad`, `styleTitle`, `styleDim`, `styleSelected`, `styleClosed` (common.go).
- Produces (internal/tui/frame.go):
  - `styleAccent` (ANSI 4) and `styleHeader` (reverse video) styles.
  - `func frame(title string, lines []string, width, height int, focused bool) []string`: nil when `width < 4` or `height < 2`; rows are `│ ` + line padded to `width-3` + `│`.
  - `func within(outer lipgloss.Style, s string) string`.
  - `func cutHints(hints string, width int) string`.

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/frame_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFrame(t *testing.T) {
	got := frame("TIMELINE · 2/5 done", []string{"09:00 ▓ Fix CI", "09:30 ▓"}, 24, 5, false)
	want := []string{
		"╭ TIMELINE · 2/5 done ─╮",
		"│ 09:00 ▓ Fix CI       │",
		"│ 09:30 ▓              │",
		"│                      │",
		"╰──────────────────────╯",
	}
	if len(got) != len(want) {
		t.Fatalf("frame has %d lines; want %d", len(got), len(want))
	}
	for i := range want {
		if s := ansi.Strip(got[i]); s != want[i] {
			t.Errorf("line %d = %q; want %q", i, s, want[i])
		}
		if w := ansi.StringWidth(got[i]); w != 24 {
			t.Errorf("line %d is %d cells wide; want 24", i, w)
		}
	}

	cut := frame("A VERY LONG TITLE THAT DOES NOT FIT", []string{"a line far too long for this box", "2", "3", "4"}, 16, 4, true)
	if s := ansi.Strip(cut[0]); s != "╭ A VERY LONG… ╮" {
		t.Errorf("a long title is cut: %q", s)
	}
	if len(cut) != 4 || ansi.Strip(cut[1]) != "│ a line far t…│" || ansi.Strip(cut[2]) != "│ 2            │" || ansi.Strip(cut[3]) != "╰──────────────╯" {
		t.Errorf("long lines are cut and extra lines dropped: %q", stripAll(cut))
	}
	for i, l := range cut {
		if w := ansi.StringWidth(l); w != 16 {
			t.Errorf("line %d is %d cells wide; want 16", i, w)
		}
	}
	if !strings.Contains(cut[0], "\x1b[34m") || strings.Contains(got[0], "\x1b[34m") {
		t.Error("only the focused frame is drawn in the accent colour (ANSI 4)")
	}
	if !strings.Contains(got[0], "\x1b[2m") {
		t.Error("an unfocused frame is dim")
	}
	if frame("x", nil, 3, 5, false) != nil || frame("x", nil, 10, 1, false) != nil {
		t.Error("no room for a frame: nothing drawn")
	}
}

func stripAll(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ansi.Strip(l)
	}
	return out
}

func TestWithinKeepsTheOuterStyleOverInnerStyles(t *testing.T) {
	inner := styleClosed.Render("done task")
	got := within(styleSelected, "a "+inner+" b")
	if want := "\x1b[7ma " + strings.ReplaceAll(inner, "\x1b[m", "\x1b[m\x1b[7m") + " b\x1b[m"; got != want {
		t.Errorf("within = %q; want %q (reverse video again after the inner reset)", got, want)
	}
	if got := within(styleSelected, "plain"); got != styleSelected.Render("plain") {
		t.Errorf("a plain line renders as before: %q", got)
	}
}

func TestCutHints(t *testing.T) {
	hints := "⏎ open · e edit · a add · / filter · g group · p pin · ctrl+z undo · ? help · q quit"
	if got := cutHints(hints, 200); got != hints {
		t.Errorf("a line that fits stays: %q", got)
	}
	got := cutHints(hints, 40)
	if want := "⏎ open · e edit · a add …"; !strings.HasPrefix(got, want) || !strings.HasSuffix(got, " ? help") {
		t.Errorf("cutHints(40) = %q; want whole hints, …, and ? help at the right end", got)
	}
	if w := ansi.StringWidth(got); w != 40 {
		t.Errorf("the cut line is %d cells wide; want 40 (? help at the right edge)", w)
	}
	if got := cutHints("placing \"Fix CI\" · j/k move · ⏎ drop · esc cancel", 30); got != "placing \"Fix CI\" · j/k move …" {
		t.Errorf("without ? help the cut just ends in …: %q", got)
	}
	if got := cutHints(hints, 12); ansi.StringWidth(got) > 12 || !strings.HasSuffix(got, "? help") {
		t.Errorf("a tiny width still ends in ? help: %q", got)
	}
	long := "w reviews · " + hints + " · tab pane · →/← expand · B block today · esc today · i inbox"
	for w := 12; w <= 160; w++ {
		got := cutHints(long, w)
		if gw := ansi.StringWidth(got); gw > w || ansi.StringWidth(long) > w && gw != w {
			t.Errorf("cutHints at %d is %d cells wide: %q", w, gw, got)
		}
		if !strings.HasSuffix(got, "? help") && ansi.StringWidth(long) > w {
			t.Errorf("cutHints at %d lost ? help: %q", w, got)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/ -run 'Frame|Within|CutHints'`
Expected: FAIL (build): `undefined: frame`, `undefined: within`, `undefined: cutHints`.

- [ ] **Step 3: Implement**

Create `internal/tui/frame.go`:

```go
package tui

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	// styleAccent is the focused pane's frame (D55): ANSI blue, so it
	// follows the terminal's theme.
	styleAccent = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	// styleHeader is the header bar (D55).
	styleHeader = lipgloss.NewStyle().Reverse(true)
)

// frame draws lines inside a rounded box width cells wide and height lines
// tall, borders included (D55). The title sits in the top border, cut with
// … when it doesn't fit. A focused box is drawn in the accent colour, the
// others dim. Each line gets one space on the left and is padded or cut to
// the inner width; missing lines are blank, extra ones are dropped.
func frame(title string, lines []string, width, height int, focused bool) []string {
	if width < 4 || height < 2 {
		return nil
	}
	border, head := styleDim, styleTitle
	if focused {
		border, head = styleAccent, styleTitle.Foreground(lipgloss.Color("4"))
	}
	inner := width - 3 // "│ " on the left, "│" on the right
	title = ansi.Truncate(title, width-4, "…")
	top := border.Render("╭ ") + head.Render(title) + border.Render(" "+strings.Repeat("─", max(width-4-ansi.StringWidth(title), 0))+"╮")
	out := []string{top}
	for i := range height - 2 {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		out = append(out, border.Render("│")+" "+pad(l, inner)+border.Render("│"))
	}
	return append(out, border.Render("╰"+strings.Repeat("─", width-2)+"╯"))
}

// within renders s in outer, re-applying outer after every reset inside s,
// so a styled part (a dim suffix, a struck-through or coloured label) no
// longer ends a selected or dimmed line's style early (D58).
func within(outer lipgloss.Style, s string) string {
	probe := outer.Render("x")
	on := probe[:strings.Index(probe, "x")]
	if on == "" {
		return s
	}
	// Built by hand: Lip Gloss restyles a string that already holds escape
	// sequences one character at a time.
	s = strings.ReplaceAll(s, "\x1b[0m", "\x1b[m")
	return on + strings.ReplaceAll(s, "\x1b[m", "\x1b[m"+on) + "\x1b[m"
}

// cutHints fits a hints line into width cells (D55): when it is too wide,
// whole hints are dropped from the end at a " · " boundary and "…" marks
// the cut; "? help", when the line has it, always stays, at the right end.
func cutHints(hints string, width int) string {
	if ansi.StringWidth(hints) <= width {
		return hints
	}
	parts := strings.Split(hints, " · ")
	help := ""
	if i := slices.Index(parts, "? help"); i >= 0 {
		parts, help = slices.Delete(parts, i, i+1), "? help"
	}
	room := width - ansi.StringWidth(help) - 3 // " …" and at least one space before help
	kept := ""
	for _, p := range parts {
		next := p
		if kept != "" {
			next = kept + " · " + p
		}
		if ansi.StringWidth(next) > room {
			break
		}
		kept = next
	}
	left := strings.TrimLeft(kept+" …", " ")
	if help == "" {
		return ansi.Truncate(left, width, "…")
	}
	return left + strings.Repeat(" ", max(width-ansi.StringWidth(left)-len(help), 1)) + help
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run 'Frame|Within|CutHints'`, then `gofmt -l .`, `go vet ./...`, `go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/frame.go internal/tui/frame_test.go
git commit -m 'feat(tui): frame, within and cutHints helpers for framed panes and the footer (D55, D58)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 2: Every pane in a frame; the header bar, the footer, the height budget

D55 across all five screens. `frame.go` gains the layout methods: `framed` (frames only when the body has room for a frame and a row), `chrome`, `inner`, `box` (a frame, or the unframed fallback: title line plus rows) and `beside` (two columns, no `│` divider). Today, the Log and People draw each pane through `box`, and the Reviews lane is a frame of its own whose borders replace its old blank and title lines. Inbox and Task detail get one frame each: `INBOX · 1 of 3` and `TASK`. `inboxView` now returns its title and lines, and `detailView` its lines. `visibleRows` is the body less the frame (one row fewer than before: the title moved into the border), and `paneBox` renders a pane at its column's height. The header is a full-width reverse-video bar, built with `within` so its dim and yellow parts don't end it. The footer's hints go through `cutHints` (`hintsView` → `hintsText`) and list every key again: Task detail `u unpin · D delete · ctrl+z undo`, the Log `E note · o link`, the timeline `a add · q quit`. `bodyHeight` can go down to 1, and `render` clips to the terminal's height, so the screen is never taller than the terminal. Existing tests that pinned row counts or line starts are updated for the frames: `│ ` prefixes, one row fewer per pane. Their behaviour is unchanged.

**Files:**
- Modify: `internal/tui/common.go`
- Modify: `internal/tui/detail.go`
- Modify: `internal/tui/frame.go`
- Modify: `internal/tui/inbox.go`
- Modify: `internal/tui/lane.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/people.go`
- Modify: `internal/tui/pick.go`
- Modify: `internal/tui/today.go`
- Modify: `internal/tui/halfpage_test.go`
- Modify: `internal/tui/lane_test.go`
- Create: `internal/tui/layout_test.go`
- Modify: `internal/tui/people_test.go`
- Modify: `internal/tui/pick_test.go`
- Modify: `internal/tui/reslot_test.go`
- Modify: `internal/tui/today_test.go`

**Interfaces:**
- Consumes: Task 1 (`frame`, `within`, `cutHints`, `styleHeader`).
- Produces:
  - `func (*Model) framed() bool`, `chrome() int` (2 framed, 1 not), `inner(width int) int` (`width-3` framed), `box(title string, lines []string, width, height int, focused bool) []string`; `func beside(left, right []string) string`.
  - `func (*Model) paneBox(pn pane, title, empty string, width int, focused bool) []string` (Task 3 adds an `above []string` parameter after `empty`); `listLines(l rowList, visible int, empty string, width int, focused bool) []string` (rows only, no title).
  - `func (*Model) timelineTitle() string`; `timelineLines(width int) []string` (rows only).
  - `func (*Model) inboxView() (string, []string)`; `detailView() []string`; `hintsText() string` (hintsView cuts it).
  - Test helpers (layout_test.go): `busyHarness(t)`, `fits(t, h, what, width, height)`, `accentOn()`.

- [ ] **Step 1: Write the failing tests**

Patch `internal/tui/halfpage_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/halfpage_test.go b/internal/tui/halfpage_test.go
index cc3dc2e..ddf0659 100644
--- a/internal/tui/halfpage_test.go
+++ b/internal/tui/halfpage_test.go
@@ -10,9 +10,9 @@ import (
 	"tt/internal/config"
 )
 
-// In a 16-line terminal the body has 13 lines and a list shows 12 rows, so
-// half a page is 6.
-const shortHeight = 16
+// In a 17-line terminal the body has 14 lines and a framed list shows 12
+// rows, so half a page is 6.
+const shortHeight = 17
 
 func TestHalfPageInTheLog(t *testing.T) {
 	h := newLogHarness(t, config.Default())
```

Patch `internal/tui/lane_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/lane_test.go b/internal/tui/lane_test.go
index b8508ce..cecf4ab 100644
--- a/internal/tui/lane_test.go
+++ b/internal/tui/lane_test.go
@@ -42,7 +42,7 @@ func TestReviewsLaneOnToday(t *testing.T) {
 	if a, b, c := strings.Index(s, "#901"), strings.Index(s, "#877"), strings.Index(s, "#915"); !(a < b && b < c) {
 		t.Errorf("longest waiting first (D49):\n%s", s)
 	}
-	if l := h.lineWith("REVIEWS"); !strings.Contains(l, "│ REVIEWS") {
+	if l := h.lineWith("REVIEWS"); !strings.Contains(l, "│╭ REVIEWS") {
 		t.Errorf("the lane sits in the right column, under the candidates: %q", l)
 	}
 	h.mustSee("w reviews · j/k move") // the hint, before the timeline's
@@ -92,7 +92,7 @@ func TestReviewsLaneInTheLog(t *testing.T) {
 	if h.m.screen != screenLog || h.m.lane.focused {
 		t.Fatalf("L from the lane: screen %d, lane focused %v", h.m.screen, h.m.lane.focused)
 	}
-	if l := h.lineWith("REVIEWS · 3 waiting"); !strings.HasPrefix(l, "REVIEWS") {
+	if l := h.lineWith("REVIEWS · 3 waiting"); !strings.HasPrefix(l, "╭ REVIEWS") {
 		t.Errorf("in the Log the lane sits in the left column, under the backlog: %q", l)
 	}
 	h.mustSee("w reviews · ⏎ open")
@@ -174,9 +174,9 @@ func TestReviewsLaneHeight(t *testing.T) {
 	h.mustSee("#906", "#902")
 	h.mustNotSee("#901 ") // scrolled: the cursor stays in view
 
-	h.send(tea.WindowSizeMsg{Width: 140, Height: 12})
-	if lines := strings.Count(h.screen(), "\n") + 1; lines > 12 {
-		t.Errorf("12 lines tall: the screen has %d", lines)
+	h.send(tea.WindowSizeMsg{Width: 140, Height: 13})
+	if lines := strings.Count(h.screen(), "\n") + 1; lines > 13 {
+		t.Errorf("13 lines tall: the screen has %d", lines)
 	}
 	h.mustSee("REVIEWS · 7 waiting", "#906") // the lane gave up rows, kept the cursor
 	if n := strings.Count(h.screen(), "@ana"); n != 3 {
@@ -194,7 +194,7 @@ func TestReviewRowsFitTheirColumn(t *testing.T) {
 	h := newGHHarness(t, config.Default())
 	h.setReviews(h.reviewItem("acme/api", 901, "A very long title "+strings.Repeat("that goes on ", 20)+"\x1b[31mred\ttab\nnewline", "ana", 3*24*time.Hour))
 	l := h.lineWith("#901")
-	if !strings.HasSuffix(strings.TrimRight(l, " "), "@ana  3d") || strings.ContainsAny(l, "\x1b\t") {
+	if !strings.HasSuffix(strings.TrimRight(l, " │"), "@ana  3d") || strings.ContainsAny(l, "\x1b\t") {
 		t.Errorf("a long title is cut so the author and wait stay; control characters go: %q", l)
 	}
 	if w := ansi.StringWidth(l); w > 140 {
@@ -210,7 +210,7 @@ func TestReviewsWithATaskAreMarked(t *testing.T) {
 	threeReviews(h)
 	h.keys("w", "j", "p")                                                    // #877
 	h.mustSee(`pinned "Review #877: Bump go" · P2 S`, "REVIEWS · 3 waiting") // it stays in the lane (D51)
-	if l := h.lineWith("#877 photos"); !strings.HasSuffix(strings.TrimRight(l, " "), "@tom  1d · #1") {
+	if l := h.lineWith("#877 photos"); !strings.HasSuffix(strings.TrimRight(l, " │"), "@tom  1d · #1") {
 		t.Errorf("a review with a task names it: %q", l)
 	}
 	if h.m.lane.selected != "acme/photos#877" {
```

Create `internal/tui/layout_test.go`:

```go
package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/app"
	"tt/internal/config"
)

// busyHarness is a GitHub harness with something on every screen: two
// candidates (one blocked), a backlog task with children and a note, an
// untriaged one, three review requests and a requester.
func busyHarness(t *testing.T) *harness {
	t.Helper()
	h := newGHHarness(t, config.Default())
	ctx := context.Background()
	a := h.pinned("Fix CI on the release branch @ana !1 ~s")
	h.pinned("Write the schema RFC !2 ~m")
	h.block(a.ID, 10*time.Hour, time.Hour)
	parent := h.add("Profiles migration #photos !1 ~l -- a note\nline two")
	for _, kid := range []string{"Import flow ~s", "Report export ~m"} {
		if _, _, err := h.svc.AddChild(ctx, parent.ID, kid); err != nil {
			t.Fatal(err)
		}
	}
	h.add("An idea nobody triaged")
	threeReviews(h)
	return h
}

// fits fails unless the whole screen fits a width×height terminal.
func fits(t *testing.T, h *harness, what string, width, height int) {
	t.Helper()
	lines := strings.Split(h.m.render(), "\n")
	if len(lines) > height {
		t.Errorf("%s at %dx%d: %d lines:\n%s", what, width, height, len(lines), h.screen())
	}
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > width {
			t.Errorf("%s at %dx%d: a line is %d cells wide: %q", what, width, height, w, ansi.Strip(l))
			return
		}
	}
}

func TestEveryScreenFitsTheTerminal(t *testing.T) {
	states := []struct {
		name string
		keys []string
		set  func(h *harness)
	}{
		{name: "Today"},
		{name: "Today, candidates focused", keys: []string{"tab"}},
		{name: "Today, lane focused", keys: []string{"w"}},
		{name: "Today, new-day banner", set: func(h *harness) { h.m.banner = &app.DayResult{Day: h.m.state.Plan.Day} }},
		{name: "Today, quick-add open", keys: []string{"a"}},
		{name: "Log", keys: []string{"L"}},
		{name: "Log, filter applied", keys: []string{"L", "/", "w", "a", "l", "enter"}},
		{name: "Log, typing a filter", keys: []string{"L", "/", "#"}},
		{name: "People", keys: []string{"P"}},
		{name: "People, rename prompt", keys: []string{"P", "r"}},
		{name: "Inbox", keys: []string{"i"}},
		{name: "Task detail", keys: []string{"L", "enter"}},
		{name: "help", keys: []string{"?"}},
	}
	sizes := [][2]int{{40, 8}, {80, 24}, {140, 40}, {60, 5}, {60, 4}, {60, 3}}
	for _, st := range states {
		h := busyHarness(t)
		h.keys(st.keys...)
		if st.set != nil {
			st.set(h)
		}
		for _, sz := range sizes {
			h.send(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
			fits(t, h, st.name, sz[0], sz[1])
		}
	}
}

// accentOn is the start of a focused frame's title (D55).
func accentOn() string {
	r := styleTitle.Foreground(lipgloss.Color("4")).Render("x")
	return r[:strings.Index(r, "x")]
}

func TestTheFocusedPaneIsTheAccentFrame(t *testing.T) {
	h := busyHarness(t)
	on := accentOn()
	focused := func(title string, want bool) {
		t.Helper()
		if got := strings.Contains(h.m.render(), on+title); got != want {
			t.Errorf("%s in the accent colour: %v; want %v\n%q", title, got, want, h.m.render())
		}
	}
	focused("TIMELINE", true)
	focused("CANDIDATES", false)
	h.keys("tab")
	focused("TIMELINE", false)
	focused("CANDIDATES", true)
	h.keys("w")
	focused("CANDIDATES", false)
	focused("REVIEWS", true)
	h.keys("w", "L")
	focused("BACKLOG", true)
	focused("TODAY'S CANDIDATES", false)
	h.keys("P")
	focused("PEOPLE", true)
	h.keys("tab")
	focused("PEOPLE", false)
	focused("@ana", true)
	h.keys("esc", "i")
	focused("INBOX", true)
}

func TestHeaderBarAndFooter(t *testing.T) {
	h := busyHarness(t)
	h.keys("L")
	h.send(tea.WindowSizeMsg{Width: 90, Height: 30})
	lines := strings.Split(h.m.render(), "\n")
	if !strings.HasPrefix(lines[0], "\x1b[7m") || ansi.StringWidth(lines[0]) != 90 {
		t.Errorf("the header is a full-width reverse-video bar: %q", lines[0])
	}
	if s := ansi.Strip(lines[0]); !strings.HasPrefix(s, "tt · Log · Mon 12 Oct · inbox 1") {
		t.Errorf("the header keeps its content: %q", s)
	}
	footer := ansi.Strip(lines[len(lines)-1])
	if !strings.HasSuffix(footer, "? help") || !strings.Contains(footer, " … ") || ansi.StringWidth(footer) != 90 {
		t.Errorf("a long hints line is cut at a hint and keeps ? help at the right end: %q", footer)
	}
	if !strings.HasPrefix(footer, "w reviews · ⏎ open · e edit · E note · o link") {
		t.Errorf("the Log's hints list E and o (Plan 4b carry-forward): %q", footer)
	}

	h.send(tea.WindowSizeMsg{Width: 300, Height: 30})
	h.keys("enter")
	h.mustSee("u unpin · D delete · esc back · ctrl+z undo · ? help") // Task detail's full hints
	h.keys("esc", "esc")
	h.mustSee("i inbox · a add · ctrl+z undo · ? help · q quit") // the timeline's
}

func TestShortTerminalDropsFramesFirst(t *testing.T) {
	h := busyHarness(t)
	h.send(tea.WindowSizeMsg{Width: 100, Height: 6}) // body 3: a frame and one row
	h.mustSee("╭ TIMELINE", "╰")
	h.send(tea.WindowSizeMsg{Width: 100, Height: 5}) // body 2: no frames, titles stay
	h.mustNotSee("╭", "╰")
	h.mustSee("TIMELINE", "CANDIDATES")
	for _, height := range []int{4, 3, 2, 1} {
		h.send(tea.WindowSizeMsg{Width: 100, Height: height})
		fits(t, h, fmt.Sprintf("Today at height %d", height), 100, height)
	}
}
```

Patch `internal/tui/people_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/people_test.go b/internal/tui/people_test.go
index b175f85..c460d86 100644
--- a/internal/tui/people_test.go
+++ b/internal/tui/people_test.go
@@ -247,7 +247,7 @@ func TestPeopleWithNobody(t *testing.T) {
 	h := newHarness(t, config.Default())
 	h.add("Nobody asked !1 ~s")
 	h.keys("P")
-	h.mustSee("PEOPLE · 0", "nobody yet · @who on a task names who asked")
+	h.mustSee("PEOPLE · 0", "nobody yet · @who names who asked")
 	h.keys("tab", "x", "enter", "j") // nothing to act on
 	h.mustNotSee("✓")
 }
```

Patch `internal/tui/pick_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/pick_test.go b/internal/tui/pick_test.go
index 622b923..5dcfccf 100644
--- a/internal/tui/pick_test.go
+++ b/internal/tui/pick_test.go
@@ -19,13 +19,13 @@ func TestGroupingCyclesAndIsRemembered(t *testing.T) {
 	h.mustSee("BACKLOG · by priority", "P1 · someone is blocked", "P2 ·")
 
 	h.keys("g")
-	h.mustSee("grouped by size", "BACKLOG · by size", "\nS ", "[P1 S] Small one", "\nM ")
+	h.mustSee("grouped by size", "BACKLOG · by size", "\n│ S ", "[P1 S] Small one", "\n│ M ")
 	h.mustNotSee("P1 · someone is blocked")
 	h.keys("g")
 	h.mustSee("BACKLOG · by created", "this week")
 	h.keys("g")
 	h.mustSee("BACKLOG · by requester", "@ana", "no requester")
-	if s := h.screen(); strings.Index(s, "@ana\n") > strings.Index(s, "no requester") {
+	if s := h.screen(); strings.Index(s, "\n│ @ana ") > strings.Index(s, "no requester") {
 		t.Errorf("no-requester group comes last:\n%s", s)
 	}
 
@@ -136,7 +136,7 @@ func TestScrollWindowMovesOnlyWhenTheCursorLeavesIt(t *testing.T) {
 	for i := 1; i <= 10; i++ {
 		h.add(fmt.Sprintf("Task %02d !1 ~s", i))
 	}
-	h.send(tea.WindowSizeMsg{Width: 140, Height: 10}) // 6 rows per pane
+	h.send(tea.WindowSizeMsg{Width: 140, Height: 11}) // 6 rows per framed pane
 	for range 9 {
 		h.keys("j")
 	}
```

Patch `internal/tui/reslot_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/reslot_test.go b/internal/tui/reslot_test.go
index 1a8dedb..15a4187 100644
--- a/internal/tui/reslot_test.go
+++ b/internal/tui/reslot_test.go
@@ -140,7 +140,7 @@ func TestScrollKeepsTheCursorWhenRowsAppearAbove(t *testing.T) {
 	}
 	h.keys("k", "k", "k") // 12:30, inside the window
 	line := func() int {
-		return slices.IndexFunc(strings.Split(h.screen(), "\n"), func(l string) bool { return strings.HasPrefix(l, " 12:30") })
+		return slices.IndexFunc(strings.Split(h.screen(), "\n"), func(l string) bool { return strings.HasPrefix(l, "│  12:30") })
 	}
 	before := line()
 	if before < 0 || h.m.today.offset == 0 {
```

Patch `internal/tui/today_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/today_test.go b/internal/tui/today_test.go
index 03cd1cf..f18d340 100644
--- a/internal/tui/today_test.go
+++ b/internal/tui/today_test.go
@@ -238,8 +238,8 @@ func TestNowLineSitsAboveItsRow(t *testing.T) {
 	above := func(row string) {
 		t.Helper()
 		lines := strings.Split(h.screen(), "\n")
-		i := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "━━ now ") })
-		if i < 0 || i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], " "+row+" ") {
+		i := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "│ ━━ now ") })
+		if i < 0 || i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "│  "+row+" ") {
 			t.Fatalf("the now line should sit right above the %s row:\n%s", row, h.screen())
 		}
 	}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL: the new `TestEveryScreenFitsTheTerminal` (`Today at 60x4: 5 lines`), `TestTheFocusedPaneIsTheAccentFrame`, `TestHeaderBarAndFooter`, `TestShortTerminalDropsFramesFirst`, and the updated `TestReviewsLaneOnToday`, `TestReviewsLaneInTheLog`, `TestReviewsLaneHeight`, `TestPeopleWithNobody`, `TestGroupingCyclesAndIsRemembered`, `TestScrollWindowMovesOnlyWhenTheCursorLeavesIt`, `TestScrollKeepsTheCursorWhenRowsAppearAbove`, `TestNowLineSitsAboveItsRow` (they expect frames).

- [ ] **Step 3: Implement**

Patch `internal/tui/common.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/common.go b/internal/tui/common.go
index bcfe2cc..4bd0280 100644
--- a/internal/tui/common.go
+++ b/internal/tui/common.go
@@ -185,35 +185,39 @@ func (m *Model) statusView() string {
 	return styleDim.Render(m.status)
 }
 
-func (m *Model) hintsView() string {
+// hintsView is the footer's hints line, cut to the width with ? help kept (D55).
+func (m *Model) hintsView() string { return styleDim.Render(cutHints(m.hintsText(), m.width)) }
+
+// hintsText is the current screen's key hints, before cutHints fits them.
+func (m *Model) hintsText() string {
 	lane := m.screen == screenToday || m.screen == screenLog
 	if lane && m.lane.focused {
-		return styleDim.Render(m.laneHints())
+		return m.laneHints()
 	}
 	reviews := ""
 	if lane && len(m.reviews()) > 0 {
 		reviews = "w reviews · " // D49
 	}
 	if m.screen == screenToday {
-		return styleDim.Render(reviews + m.todayHints())
+		return reviews + m.todayHints()
 	}
 	if m.screen == screenInbox {
 		if t, ok := m.state.Snapshot.Get(m.inbox.current); ok && t.ParentID != 0 {
-			return styleDim.Render("s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help")
+			return "s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help"
 		}
-		return styleDim.Render(fmt.Sprintf("1-%d priority · s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help", len(m.svc.Config().Priorities)))
+		return fmt.Sprintf("1-%d priority · s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help", len(m.svc.Config().Priorities))
 	}
 	if m.screen == screenPeople {
-		return styleDim.Render(m.peopleHints())
+		return m.peopleHints()
 	}
 	if m.screen == screenDetail {
-		return styleDim.Render("j/k move · ⏎ open child · a add child · J/K reorder · e edit · E note · o link · x done · d drop · p pin · esc back · ? help")
+		return "j/k move · ⏎ open child · a add child · J/K reorder · e edit · E note · o link · x done · d drop · p pin · u unpin · D delete · esc back · ctrl+z undo · ? help"
 	}
-	hints := "⏎ open · e edit · a add · / filter · g group · p pin · u unpin · x done · d drop · tab pane · →/← expand · B block today · esc today · i inbox · P people · ctrl+z undo · ? help · q quit"
+	hints := "⏎ open · e edit · E note · o link · a add · / filter · g group · p pin · u unpin · x done · d drop · tab pane · →/← expand · B block today · esc today · i inbox · P people · ctrl+z undo · ? help · q quit"
 	if m.pick.filtered() && m.overlay == nil { // in the filter line, esc restores the previous filter
 		hints = "esc clears filter · " + strings.Replace(hints, " · esc today", "", 1)
 	}
-	return styleDim.Render(reviews + hints)
+	return reviews + hints
 }
 
 func dayLabel(d domain.Day) string {
@@ -250,7 +254,7 @@ func (m *Model) bodyHeight() int {
 	if _, help := m.overlay.(helpOverlay); m.overlay != nil && !help {
 		used += strings.Count(m.overlay.view(m), "\n") + 1
 	}
-	return max(m.height-used, 2)
+	return max(m.height-used, 1)
 }
 
 // clipLines keeps the first n lines of s.
```

Patch `internal/tui/detail.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/detail.go b/internal/tui/detail.go
index b9b90c1..74083d9 100644
--- a/internal/tui/detail.go
+++ b/internal/tui/detail.go
@@ -83,9 +83,9 @@ func (d *detailState) move(delta int) {
 	d.selected = d.rows[d.cursor].ID
 }
 
-// childRows is how many children fit below the task's own lines and a
-// "… N more" line.
-func (m *Model) childRows() int { return max(m.bodyHeight()-1-len(m.detailHead()), 1) }
+// childRows is how many children fit in the frame below the task's own
+// lines and a "… N more" line.
+func (m *Model) childRows() int { return max(m.bodyHeight()-m.chrome()-1-len(m.detailHead()), 1) }
 
 // follow keeps the cursor's child inside the scroll window (see pickState.follow).
 func (d *detailState) follow(m *Model) {
@@ -234,14 +234,15 @@ func (m *Model) progress(t domain.Task) string {
 	return fmt.Sprintf(" %s%s %d/%d", strings.Repeat("█", filled), strings.Repeat("░", cells-filled), done, total)
 }
 
-func (m *Model) detailView() string {
+// detailView is Task detail's lines inside its frame.
+func (m *Model) detailView() []string {
 	d := &m.detail
 	head := m.detailHead()
 	if len(head) == 0 {
-		return ""
+		return nil
 	}
 	if d.cursor == 0 {
-		head[0] = styleSelected.Render(pad(head[0], m.width))
+		head[0] = styleSelected.Render(pad(head[0], m.inner(m.width)))
 	}
 	lines := head
 	if d.rows[0].ParentID == 0 {
@@ -257,7 +258,7 @@ func (m *Model) detailView() string {
 			lines = append(lines, styleDim.Render(fmt.Sprintf("  … %d more", more)))
 		}
 	}
-	return strings.Join(lines, "\n")
+	return lines
 }
 
 func (m *Model) childLine(c domain.Task, selected bool) string {
@@ -274,7 +275,7 @@ func (m *Model) childLine(c domain.Task, selected bool) string {
 	case domain.StatusDropped:
 		line += " ✗ " + dayLabel(m.svc.DayOf(c.ClosedAt))
 	}
-	line = pad(line, m.width)
+	line = pad(line, m.inner(m.width))
 	if c.Status != domain.StatusOpen {
 		line = styleClosed.Render(line)
 	}
```

Patch `internal/tui/frame.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/frame.go b/internal/tui/frame.go
index 7183dd5..5a25ed7 100644
--- a/internal/tui/frame.go
+++ b/internal/tui/frame.go
@@ -88,3 +88,59 @@ func cutHints(hints string, width int) string {
 	}
 	return left + strings.Repeat(" ", max(width-ansi.StringWidth(left)-len(help), 1)) + help
 }
+
+// framed reports whether panes get frames: not when the body is too short
+// to hold a frame and a row, so a short terminal loses frames first (D55).
+func (m *Model) framed() bool { return m.bodyHeight() >= 3 }
+
+// chrome is how many lines a pane spends outside its rows: the frame's two
+// borders, or the title line when unframed.
+func (m *Model) chrome() int {
+	if m.framed() {
+		return 2
+	}
+	return 1
+}
+
+// inner is the width a pane width cells wide leaves its rows.
+func (m *Model) inner(width int) int {
+	if m.framed() {
+		return width - 3
+	}
+	return width
+}
+
+// box renders one pane width cells wide and height lines tall: its lines in
+// a frame titled title, or, when the screen is too short for frames, the
+// title above the lines.
+func (m *Model) box(title string, lines []string, width, height int, focused bool) []string {
+	if m.framed() {
+		return frame(title, lines, width, height, focused)
+	}
+	out := []string{pad(styleTitle.Render(title), width)}
+	for i := range height - 1 {
+		l := ""
+		if i < len(lines) {
+			l = lines[i]
+		}
+		out = append(out, pad(l, width))
+	}
+	return out[:min(len(out), max(height, 0))]
+}
+
+// beside joins two columns of panes line by line.
+func beside(left, right []string) string {
+	var b strings.Builder
+	for i := range max(len(left), len(right)) {
+		if i > 0 {
+			b.WriteByte('\n')
+		}
+		if i < len(left) {
+			b.WriteString(left[i])
+		}
+		if i < len(right) {
+			b.WriteString(right[i])
+		}
+	}
+	return b.String()
+}
```

Patch `internal/tui/inbox.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/inbox.go b/internal/tui/inbox.go
index 2167008..9fc6f0b 100644
--- a/internal/tui/inbox.go
+++ b/internal/tui/inbox.go
@@ -98,16 +98,17 @@ func (m *Model) inboxKey(key string) tea.Cmd {
 	return nil
 }
 
-func (m *Model) inboxView() string {
+// inboxView is the Inbox's frame title and the current card's lines.
+func (m *Model) inboxView() (string, []string) {
 	order := m.inbox.order(m)
 	if len(order) == 0 {
-		return styleTitle.Render("INBOX") + "\n\n  Inbox zero ✓  " + styleDim.Render("esc to go back")
+		return "INBOX", []string{"", "  Inbox zero ✓  " + styleDim.Render("esc to go back")}
 	}
 	pos := slices.IndexFunc(order, func(t domain.Task) bool { return t.ID == m.inbox.current })
 	t := order[max(pos, 0)]
 	cfg := m.svc.Config()
 	var b strings.Builder
-	fmt.Fprintf(&b, "%s\n\n", styleTitle.Render(fmt.Sprintf("INBOX · %d of %d", pos+1, len(order))))
+	title := fmt.Sprintf("INBOX · %d of %d", pos+1, len(order))
 	fmt.Fprintf(&b, "  %s\n", styleTitle.Render(t.Title))
 	if t.Link != "" {
 		fmt.Fprintf(&b, "  %s\n", styleDim.Render(t.Link))
@@ -160,5 +161,5 @@ func (m *Model) inboxView() string {
 		sizes = append(sizes, item)
 	}
 	fmt.Fprintf(&b, "  size       %s", strings.Join(sizes, "  "))
-	return b.String()
+	return title, strings.Split(b.String(), "\n")
 }
```

Patch `internal/tui/lane.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/lane.go b/internal/tui/lane.go
index 6f9f6b4..074eacb 100644
--- a/internal/tui/lane.go
+++ b/internal/tui/lane.go
@@ -66,15 +66,15 @@ func (l *laneState) follow(m *Model) {
 	l.offset = max(0, min(l.offset, len(m.reviews())-rows))
 }
 
-// laneLines is how many lines the lane takes in its column: a blank line, its
-// title and up to laneMaxRows requests. The pane above keeps its title and
-// three rows, so in a short column the lane gives up rows first, then
-// disappears; 0 when GitHub is off.
+// laneLines is how many lines the lane takes in its column: its frame (or,
+// unframed, a blank line and its title) and up to laneMaxRows requests. The
+// pane above keeps its frame and three rows, so in a short column the lane
+// gives up rows first, then disappears; 0 when GitHub is off.
 func (m *Model) laneLines() int {
 	if !m.state.GitHub.SetUp {
 		return 0
 	}
-	room := m.bodyHeight() - 4 - 2
+	room := m.bodyHeight() - m.chrome() - 3 - 2
 	if room < 0 {
 		return 0
 	}
@@ -102,11 +102,14 @@ func (m *Model) laneView(width int) []string {
 	default:
 		title += " · none waiting"
 	}
-	lines := []string{"", styleTitle.Render(title)}
-	for i := m.lane.offset; i < len(gs.Reviews) && len(lines) < n; i++ {
-		lines = append(lines, m.reviewLine(gs.Reviews[i], width, m.lane.focused && i == m.lane.cursor))
+	var rows []string
+	for i := m.lane.offset; i < len(gs.Reviews) && len(rows) < n-2; i++ {
+		rows = append(rows, m.reviewLine(gs.Reviews[i], m.inner(width), m.lane.focused && i == m.lane.cursor))
 	}
-	return lines
+	if !m.framed() {
+		return append([]string{"", styleTitle.Render(title)}, rows...)
+	}
+	return frame(title, rows, width, n, m.lane.focused)
 }
 
 // reviewLine renders `#901 api  Add idempotency key… @ana  3d`: the repo
```

Patch `internal/tui/model.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/model.go b/internal/tui/model.go
index a526b58..416ed0d 100644
--- a/internal/tui/model.go
+++ b/internal/tui/model.go
@@ -468,9 +468,9 @@ func (m *Model) render() string {
 		return fitWidth("loading…\n"+m.statusView(), m.width)
 	}
 	var parts []string
-	parts = append(parts, m.headerView())
-	if h, ok := m.overlay.(helpOverlay); ok { // help takes the whole screen
-		return fitWidth(strings.Join(append(parts, h.view(m)), "\n"), m.width)
+	parts = append(parts, within(styleHeader, pad(m.headerView(), m.width))) // the header bar (D55)
+	if h, ok := m.overlay.(helpOverlay); ok {                                // help takes the whole screen
+		return fitWidth(clipLines(strings.Join(append(parts, h.view(m)), "\n"), m.height), m.width)
 	}
 	if m.banner != nil {
 		parts = append(parts, m.bannerView())
@@ -480,9 +480,10 @@ func (m *Model) render() string {
 	case screenToday:
 		parts = append(parts, m.todayView())
 	case screenInbox:
-		parts = append(parts, clipLines(m.inboxView(), m.bodyHeight()))
+		title, lines := m.inboxView()
+		parts = append(parts, strings.Join(m.box(title, lines, m.width, m.bodyHeight(), true), "\n"))
 	case screenDetail:
-		parts = append(parts, clipLines(m.detailView(), m.bodyHeight()))
+		parts = append(parts, strings.Join(m.box("TASK", m.detailView(), m.width, m.bodyHeight(), true), "\n"))
 	case screenPeople:
 		parts = append(parts, m.peopleView())
 	default:
@@ -495,5 +496,5 @@ func (m *Model) render() string {
 		parts = append(parts, m.overlay.view(m))
 	}
 	parts = append(parts, m.statusView(), m.hintsView())
-	return fitWidth(strings.Join(parts, "\n"), m.width)
+	return fitWidth(clipLines(strings.Join(parts, "\n"), m.height), m.width) // never taller than the terminal
 }
```

Patch `internal/tui/people.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/people.go b/internal/tui/people.go
index 655e64d..d24f7cd 100644
--- a/internal/tui/people.go
+++ b/internal/tui/people.go
@@ -189,28 +189,28 @@ func (m *Model) peopleKey(key string) tea.Cmd {
 }
 
 func (m *Model) peopleView() string {
-	total := max(m.width-3, 20)
-	lw := total / 3
-	rw := total - lw
+	lw := m.width / 3
+	rw := m.width - lw
 	p := &m.people
-	title, empty := fmt.Sprintf("PEOPLE · %d", len(m.idx.people)), "nobody yet · @who on a task names who asked"
+	title, empty := fmt.Sprintf("PEOPLE · %d", len(m.idx.people)), "nobody yet · @who names who asked"
 	if p.filter != "" {
 		title = "PEOPLE · " + plural(len(p.names), "match", "matches") + " · " + p.filter
 		empty = "no matches · esc clears"
 	}
-	left := []string{styleTitle.Render(title)}
+	var names []string
 	if len(p.names) == 0 {
-		left = append(left, styleDim.Render("  "+empty))
+		names = append(names, styleDim.Render("  "+empty))
 	}
 	visible := m.visibleRows()
 	for i := p.offset; i < len(p.names) && i < p.offset+visible; i++ {
-		left = append(left, m.personLine(p.names[i], lw, i == p.cursor))
+		names = append(names, m.personLine(p.names[i], m.inner(lw), i == p.cursor))
 	}
-	right := m.listLines(p.tasks, visible, "@"+p.selected, "", rw, p.onTasks)
-	if p.selected == "" {
-		right = nil
+	left := m.box(title, names, lw, m.bodyHeight(), !p.onTasks)
+	tasksTitle, tasks := "TASKS", []string(nil)
+	if p.selected != "" {
+		tasksTitle, tasks = "@"+p.selected, m.listLines(p.tasks, visible, "", m.inner(rw), p.onTasks)
 	}
-	return columns(left, right, lw, rw)
+	return beside(left, m.box(tasksTitle, tasks, rw, m.bodyHeight(), p.onTasks))
 }
 
 // openRename asks for a person's new name (D53), prefilled with the old one.
```

Patch `internal/tui/pick.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/pick.go b/internal/tui/pick.go
index d51f668..131c773 100644
--- a/internal/tui/pick.go
+++ b/internal/tui/pick.go
@@ -331,9 +331,9 @@ func scroll(top, c, off, visible, n int) int {
 	return max(0, min(off, n-visible))
 }
 
-// visibleRows is how many rows fit in a pane below its title (the Log's
-// panes, Today's candidates and timeline share this budget).
-func (m *Model) visibleRows() int { return max(m.bodyHeight()-1, 1) }
+// visibleRows is how many rows fit in a pane inside its frame (the Log's
+// panes, Today's candidates and timeline, People's panes share this budget).
+func (m *Model) visibleRows() int { return max(m.bodyHeight()-m.chrome(), 1) }
 
 // paneVisible is visibleRows for pane pn, less the Reviews lane when it
 // sits under pn on this screen (D49).
@@ -539,9 +539,8 @@ func (m *Model) taskKey(key string, t domain.Task) (cmd tea.Cmd, ok bool) {
 }
 
 func (m *Model) pickView() string {
-	total := max(m.width-3, 20)
-	lw := total * 55 / 100
-	rw := total - lw
+	lw := m.width * 55 / 100
+	rw := m.width - lw
 	title, empty := "BACKLOG · by "+string(m.pick.group), "nothing triaged yet · a to add · i to triage"
 	if m.pick.filtered() {
 		n := 0
@@ -553,41 +552,28 @@ func (m *Model) pickView() string {
 		title += " · " + plural(n, "match", "matches") + " · " + m.pick.filterText // the count first: truncation eats the filter text
 		empty = "no matches · esc clears"
 	}
-	left := m.paneLines(paneBacklog, title, empty, lw, m.pick.focus == paneBacklog && !m.lane.focused)
+	left := m.paneBox(paneBacklog, title, empty, lw, m.pick.focus == paneBacklog && !m.lane.focused)
 	left = append(left, m.laneView(lw)...)
-	right := m.paneLines(paneToday, "TODAY'S CANDIDATES", "nothing pinned · p on a backlog task", rw, m.pick.focus == paneToday && !m.lane.focused)
-	return columns(left, right, lw, rw)
+	right := m.paneBox(paneToday, "TODAY'S CANDIDATES", "nothing pinned · p on a backlog task", rw, m.pick.focus == paneToday && !m.lane.focused)
+	return beside(left, right)
 }
 
-// columns renders two panes side by side, lw and rw cells wide.
-func columns(left, right []string, lw, rw int) string {
-	var b strings.Builder
-	for i := range max(len(left), len(right)) {
-		l, r := "", ""
-		if i < len(left) {
-			l = left[i]
-		}
-		if i < len(right) {
-			r = right[i]
-		}
-		if i > 0 {
-			b.WriteByte('\n')
-		}
-		b.WriteString(pad(l, lw) + " │ " + pad(r, rw))
-	}
-	return b.String()
-}
-
-// paneLines renders pane pn's title and visible rows; focused shows its cursor.
-func (m *Model) paneLines(pn pane, title, empty string, width int, focused bool) []string {
+// paneBox renders pane pn as a box width cells wide, its rows in its
+// window; focused shows its cursor. The Reviews lane under it on this screen
+// takes the rest of the column.
+func (m *Model) paneBox(pn pane, title, empty string, width int, focused bool) []string {
 	l := rowList{rows: m.rows(pn), cursor: m.pick.cursor[pn], offset: m.pick.offset[pn]}
-	return m.listLines(l, m.paneVisible(pn), title, empty, width, focused)
+	height := m.bodyHeight()
+	if m.laneUnder(pn) {
+		height -= m.laneLines()
+	}
+	return m.box(title, m.listLines(l, m.paneVisible(pn), empty, m.inner(width), focused), width, height, focused)
 }
 
-// listLines renders a list's title and the rows in its window of visible
-// rows; focused shows its cursor.
-func (m *Model) listLines(l rowList, visible int, title, empty string, width int, focused bool) []string {
-	lines := []string{styleTitle.Render(title)}
+// listLines renders the rows in a list's window of visible rows, rows
+// width cells wide; focused shows its cursor.
+func (m *Model) listLines(l rowList, visible int, empty string, width int, focused bool) []string {
+	var lines []string
 	if len(l.rows) == 0 {
 		return append(lines, styleDim.Render("  "+empty))
 	}
```

Patch `internal/tui/today.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/today.go b/internal/tui/today.go
index 85b6df3..2d7aac1 100644
--- a/internal/tui/today.go
+++ b/internal/tui/today.go
@@ -508,17 +508,16 @@ func (m *Model) hasOpenChildren(t domain.Task) bool {
 }
 
 func (m *Model) todayView() string {
-	total := max(m.width-3, 20)
-	lw := total / 2
-	rw := total - lw
-	left := m.timelineLines(lw)
-	right := m.paneLines(paneToday, "CANDIDATES", "nothing pinned · L opens the log to pick", rw, m.today.focus == focusCandidates && !m.lane.focused)
+	lw := m.width / 2
+	rw := m.width - lw
+	left := m.box(m.timelineTitle(), m.timelineLines(m.inner(lw)), lw, m.bodyHeight(), m.timelineFocused())
+	right := m.paneBox(paneToday, "CANDIDATES", "nothing pinned · L opens the log to pick", rw, m.today.focus == focusCandidates && !m.lane.focused)
 	right = append(right, m.laneView(rw)...)
-	return columns(left, right, lw, rw)
+	return beside(left, right)
 }
 
-// timelineLines renders the timeline's title and the lines in its window.
-func (m *Model) timelineLines(width int) []string {
+// timelineTitle is `TIMELINE · 2/5 done · ⚠ 1`.
+func (m *Model) timelineTitle() string {
 	td := &m.today
 	title := "TIMELINE"
 	if n := len(td.tl.blocks); n > 0 {
@@ -536,6 +535,12 @@ func (m *Model) timelineLines(width int) []string {
 	if td.tl.outsideNow() {
 		title += " · outside of working hours"
 	}
+	return title
+}
+
+// timelineLines renders the lines in the timeline's window, width cells wide.
+func (m *Model) timelineLines(width int) []string {
+	td := &m.today
 	var all []string
 	for i := range td.tl.rows {
 		if i == td.tl.nowRow {
@@ -545,7 +550,7 @@ func (m *Model) timelineLines(width int) []string {
 		all = append(all, m.timelineRow(i, width))
 	}
 	end := min(len(all), td.offset+m.visibleRows())
-	return append([]string{styleTitle.Render(title)}, all[min(td.offset, end):end]...)
+	return all[min(td.offset, end):end]
 }
 
 // timelineRow renders row i: its time, then the blocks starting in it
@@ -697,7 +702,7 @@ func (m *Model) todayHints() string {
 	if m.today.focus == focusCandidates {
 		return "b block · ⏎ open · u unpin · x done · d drop · e edit · tab timeline · L log · P people · i inbox · a add · ctrl+z undo · ? help · q quit"
 	}
-	hints := "j/k move · m move · +/- size · f fine-tune · u unblock · ⏎ open · x done · tab candidates · L log · P people · i inbox · ctrl+z undo · ? help"
+	hints := "j/k move · m move · +/- size · f fine-tune · u unblock · ⏎ open · x done · tab candidates · L log · P people · i inbox · a add · ctrl+z undo · ? help · q quit"
 	if b, ok := m.today.tl.blockAt(m.today.cursor); ok && m.conflicted(b) {
 		hints = "r re-slot ⚠ · " + hints
 	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/common.go internal/tui/detail.go internal/tui/frame.go internal/tui/halfpage_test.go internal/tui/inbox.go internal/tui/lane.go internal/tui/lane_test.go internal/tui/layout_test.go internal/tui/model.go internal/tui/people.go internal/tui/people_test.go internal/tui/pick.go internal/tui/pick_test.go internal/tui/reslot_test.go internal/tui/today.go internal/tui/today_test.go
git commit -m 'feat(tui): every pane in a rounded frame, a header bar, a footer that keeps ? help (D55)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 3: The applied filter keeps its own row (Log and People)

D56. The first row inside the Log's backlog frame and inside People's names frame is the filter. While you type, it is the input (with its error and suggestion lines). Once applied, it is a dim `filter › #photos schema · esc clears`. Cleared, the row is gone. The filter overlays implement `paneOverlay`, so they take no height from the body and `render` no longer draws them above or below the screen. Their inputs are sized to their column (`backlogWidth`, `namesWidth`). Pane titles keep the count without the filter text (`BACKLOG · by priority · 3 matches`, `PEOPLE · 2 matches`). The empty text becomes `no matches`, and People's filter prompt becomes `filter › `. `paneVisible` and People's `namesVisible` take the filter rows off the list. Tests that pinned the old title text are updated.

**Files:**
- Modify: `internal/tui/common.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/overlays.go`
- Modify: `internal/tui/people.go`
- Modify: `internal/tui/pick.go`
- Modify: `internal/tui/today.go`
- Modify: `internal/tui/browse_test.go`
- Modify: `internal/tui/filter_test.go`
- Create: `internal/tui/filterline_test.go`
- Modify: `internal/tui/people_test.go`
- Modify: `internal/tui/pick_test.go`
- Modify: `internal/tui/rename_test.go`

**Interfaces:**
- Consumes: Task 2 (`box`, `paneBox`, `listLines`, `inner`, `visibleRows`).
- Produces: `type paneOverlay interface{ inPane() }` (implemented by `*filterBar`, `*peopleFilter`); `func (*Model) filterLines() []string`; `func backlogWidth(width int) int` (`width*55/100`); `func (*Model) peopleFilterLines() []string`; `func (*Model) namesVisible() int`; `func namesWidth(width int) int` (`width/3`); `paneBox(pn pane, title, empty string, above []string, width int, focused bool)`; test helper `(*harness).lineOf(s string) int`.

- [ ] **Step 1: Write the failing tests**

Patch `internal/tui/browse_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/browse_test.go b/internal/tui/browse_test.go
index ee44f77..fd6f756 100644
--- a/internal/tui/browse_test.go
+++ b/internal/tui/browse_test.go
@@ -17,7 +17,7 @@ func TestFilterEscRestoresTheSelection(t *testing.T) {
 	h.keys("j", "j")
 	h.keys("/")
 	h.typeText("alp") // live narrowing moves the cursor to Alpha
-	h.mustSee("1 match · alp")
+	h.mustSee("1 match", "filter › alp")
 	h.keys("esc")
 	if r, _ := h.m.selectedRow(); r.task.ID != gamma.ID {
 		t.Errorf("after esc the cursor is on %q; want Gamma, where it was before /", r.task.Title)
```

Patch `internal/tui/filter_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/filter_test.go b/internal/tui/filter_test.go
index e603184..7f5f84b 100644
--- a/internal/tui/filter_test.go
+++ b/internal/tui/filter_test.go
@@ -19,15 +19,15 @@ func TestFilterBarNarrowsLiveAndKeepsTypingOnError(t *testing.T) {
 	h.add("Deploy fix !2 ~m")
 
 	h.keys("/")
-	if s := h.screen(); strings.Index(s, "filter ›") > strings.Index(s, "BACKLOG") {
-		t.Errorf("the filter line sits above the backlog:\n%s", s)
+	if l := strings.Split(h.screen(), "\n"); !strings.HasPrefix(l[1], "╭ BACKLOG") || !strings.HasPrefix(l[2], "│ filter ›") {
+		t.Errorf("the filter line is the first row inside the backlog's frame (D56):\n%s", h.screen())
 	}
 	h.typeText("sch")
-	h.mustSee("BACKLOG · by priority · 1 match · sch", "Schema review")
+	h.mustSee("BACKLOG · by priority · 1 match ─", "filter › sch", "Schema review")
 	h.mustNotSee("Deploy fix")
 
 	h.typeText(" !9")
-	h.mustSee("✗ priority !9 out of range", "BACKLOG · by priority · 1 match · sch !", "Schema review") // the last valid filter ("sch !") stays
+	h.mustSee("✗ priority !9 out of range", "BACKLOG · by priority · 1 match", "Schema review") // the last valid filter ("sch !") stays
 	h.keys("enter")
 	if _, ok := h.m.overlay.(*filterBar); !ok {
 		t.Fatal("enter on a bad filter must keep the line open")
@@ -39,25 +39,25 @@ func TestFilterBarNarrowsLiveAndKeepsTypingOnError(t *testing.T) {
 	if h.m.overlay != nil {
 		t.Fatal("enter closes the filter line")
 	}
-	h.mustSee("1 match · sch", "esc clears filter")
+	h.mustSee("1 match", "filter › sch · esc clears", "esc clears filter")
 
 	// esc in the line restores the filter from before `/`.
 	h.keys("/")
 	h.mustSee("filter › sch")
 	h.typeText("zzz")
-	h.mustSee("0 matches · sch zzz", "no matches · esc clears")
+	h.mustSee("0 matches", "filter › sch zzz", "no matches")
 	h.keys("esc")
-	h.mustSee("1 match · sch")
+	h.mustSee("1 match", "filter › sch · esc clears")
 
 	// esc on the list clears it; filters are never remembered.
 	h.keys("esc")
 	h.mustSee("filter cleared", "Deploy fix")
-	h.mustNotSee("match")
+	h.mustNotSee("match", "filter ›")
 
 	// Pasting into the line filters too.
 	h.keys("/")
 	h.send(tea.PasteMsg{Content: "@ana"})
-	h.mustSee("1 match · @ana", "Schema review")
+	h.mustSee("1 match", "filter › @ana", "Schema review")
 }
 
 func TestReloadWhileFilteringKeepsTheFilter(t *testing.T) {
@@ -74,7 +74,7 @@ func TestReloadWhileFilteringKeepsTheFilter(t *testing.T) {
 		t.Fatal(err)
 	}
 	h.send(watchTickMsg{})
-	h.mustSee("filter › schema", "2 matches · schema", "[P1 S] Schema follow-up")
+	h.mustSee("filter › schema", "2 matches", "[P1 S] Schema follow-up")
 }
 
 func TestTokenInputCompletesAtTheCursor(t *testing.T) {
@@ -99,7 +99,7 @@ func TestTokenInputCompletesAtTheCursor(t *testing.T) {
 	h.typeText("@and")
 	h.mustSee("tab → @andrei")
 	h.keys("tab")
-	h.mustSee("filter › @andrei", "1 match · @andrei", "two")
+	h.mustSee("filter › @andrei", "1 match", "two")
 }
 
 func TestOverlayInputFollowsResize(t *testing.T) {
@@ -111,7 +111,7 @@ func TestOverlayInputFollowsResize(t *testing.T) {
 	}
 	h.keys("esc", "/")
 	h.send(tea.WindowSizeMsg{Width: 80, Height: 40})
-	if w, want := h.m.overlay.(*filterBar).in.input.Width(), 80-ansi.StringWidth("filter › ")-2; w != want {
+	if w, want := h.m.overlay.(*filterBar).in.input.Width(), backlogWidth(80)-3-ansi.StringWidth("filter › ")-2; w != want { // inside the backlog frame (D56)
 		t.Errorf("filter input width after resize = %d; want %d", w, want)
 	}
 }
```

Create `internal/tui/filterline_test.go`:

```go
package tui

import (
	"slices"
	"strings"
	"testing"

	"tt/internal/config"
)

// lineOf is the index of the first screen line containing s (-1 if none).
func (h *harness) lineOf(s string) int {
	return slices.IndexFunc(strings.Split(h.screen(), "\n"), func(l string) bool { return strings.Contains(l, s) })
}

func TestTheAppliedFilterStaysVisibleInTheLog(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.add("Schema review !1 ~s")
	h.add("Deploy fix !2 ~m")
	h.keys("/")
	h.typeText("schema")
	typing := h.lineOf("Schema review")
	if got := h.lineOf("filter › schema"); got != 2 {
		t.Errorf("the input is the first row inside the backlog's frame: line %d", got)
	}
	h.keys("enter")
	h.mustSee("BACKLOG · by priority · 1 match ─", "filter › schema · esc clears")
	h.mustNotSee("1 match · schema") // the title keeps only the count
	if got := h.lineOf("filter › schema · esc clears"); got != 2 {
		t.Errorf("the applied filter keeps the input's row: line %d", got)
	}
	if got := h.lineOf("Schema review"); got != typing {
		t.Errorf("nothing moves on ⏎: the task went from line %d to %d", typing, got)
	}
	if !strings.Contains(h.m.render(), styleDim.Render("filter › schema · esc clears")) {
		t.Error("the applied filter is dim")
	}

	h.keys("tab", "tab") // still there after moving around
	h.mustSee("filter › schema · esc clears")
	h.keys("esc")
	h.mustSee("filter cleared", "Deploy fix")
	h.mustNotSee("filter ›")
	if got := h.lineOf("Schema review"); got != typing-1 {
		t.Errorf("cleared, the list gets the row back: line %d; want %d", got, typing-1)
	}
}

func TestTheAppliedFilterStaysVisibleInPeople(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("Ask @ana !1 ~s")
	h.add("Ask @bob !1 ~s")
	h.keys("P", "/")
	h.typeText("an")
	if got := h.lineOf("filter › an"); got != 2 {
		t.Errorf("the input is the first row inside the names frame: line %d", got)
	}
	h.mustSee("PEOPLE · 1 match ─", "ana · 1 open")
	h.mustNotSee("bob · ")
	h.keys("enter")
	h.mustSee("filter › an · esc clears", "@ana")
	h.keys("tab") // the tasks pane: the filter stays in view
	h.mustSee("filter › an · esc clears")
	h.keys("esc")
	h.mustSee("filter cleared", "bob · 1 open")
	h.mustNotSee("filter ›")
}
```

Patch `internal/tui/people_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/people_test.go b/internal/tui/people_test.go
index c460d86..ee356e4 100644
--- a/internal/tui/people_test.go
+++ b/internal/tui/people_test.go
@@ -156,7 +156,7 @@ func TestPeopleFilter(t *testing.T) {
 	}
 	h.keys("P", "/")
 	h.typeText("an")
-	h.mustSee("PEOPLE · 2 matches · an", "ana · 1 open", "anabel · 1 open")
+	h.mustSee("PEOPLE · 2 matches", "filter › an", "ana · 1 open", "anabel · 1 open")
 	h.mustNotSee("raff · ", "bob · ")
 	h.keys("esc") // esc in the filter clears it
 	h.mustSee("PEOPLE · 4", "raff · 1 open")
@@ -164,7 +164,7 @@ func TestPeopleFilter(t *testing.T) {
 	h.keys("/")
 	h.typeText("@rf")
 	h.keys("enter")
-	h.mustSee("PEOPLE · 1 match · @rf", "@raff", "Ask from raff", "esc clears filter")
+	h.mustSee("PEOPLE · 1 match", "filter › @rf · esc clears", "@raff", "Ask from raff", "esc clears filter")
 	h.keys("esc")
 	h.mustSee("filter cleared", "PEOPLE · 4")
 	if h.m.screen != screenPeople || h.m.people.selected != "raff" {
@@ -173,10 +173,10 @@ func TestPeopleFilter(t *testing.T) {
 
 	h.keys("/")
 	h.typeText("zz")
-	h.mustSee("PEOPLE · 0 matches · zz", "no matches · esc clears")
+	h.mustSee("PEOPLE · 0 matches", "filter › zz", "no matches")
 	h.mustNotSee("Ask from")
 	h.keys("x") // still typing: a letter, not a task key
-	h.mustSee("0 matches · zzx")
+	h.mustSee("filter › zzx")
 	h.keys("enter", "esc", "esc")
 	if h.m.screen != screenToday {
 		t.Errorf("esc, esc: filter cleared, then back to Today: screen %d", h.m.screen)
```

Patch `internal/tui/pick_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/pick_test.go b/internal/tui/pick_test.go
index 5dcfccf..835a37f 100644
--- a/internal/tui/pick_test.go
+++ b/internal/tui/pick_test.go
@@ -80,7 +80,7 @@ func TestFilteredBacklogPlacement(t *testing.T) {
 	if err := h.m.applyFilter("schema"); err != nil {
 		t.Fatal(err)
 	}
-	h.mustSee("BACKLOG · by priority · 1 match · schema", "[P3 M] Schema backlog")
+	h.mustSee("BACKLOG · by priority · 1 match", "filter › schema · esc clears", "[P3 M] Schema backlog")
 	h.mustNotSee("Schema idea", "Schema shipped", "unset")
 
 	// is: widens it to closed backlog tasks (D26b).
@@ -110,12 +110,12 @@ func TestFilteredBacklogPlacement(t *testing.T) {
 	if err := h.m.applyFilter("!9"); err == nil {
 		t.Fatal("!9 must be refused")
 	}
-	h.mustSee("1 match · #auth spike")
+	h.mustSee("1 match", "filter › #auth spike · esc clears")
 
 	if err := h.m.applyFilter("zzz"); err != nil {
 		t.Fatal(err)
 	}
-	h.mustSee("0 matches · zzz", "no matches · esc clears")
+	h.mustSee("0 matches", "filter › zzz · esc clears", "no matches")
 	h.keys("esc")
 	h.mustSee("filter cleared", "▸ [P1 XL] Auth migration 0/2")
 	h.mustNotSee("matches", "Schema idea")
@@ -128,7 +128,7 @@ func TestFilteredBacklogPlacement(t *testing.T) {
 	if err := h.m.applyFilter("rollback"); err != nil {
 		t.Fatal(err)
 	}
-	h.mustSee("0 matches · rollback", "    [P1 S] Rollback plan")
+	h.mustSee("0 matches", "filter › rollback · esc clears", "    [P1 S] Rollback plan")
 }
 
 func TestScrollWindowMovesOnlyWhenTheCursorLeavesIt(t *testing.T) {
```

Patch `internal/tui/rename_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/rename_test.go b/internal/tui/rename_test.go
index 3a5e7a2..85ee4c3 100644
--- a/internal/tui/rename_test.go
+++ b/internal/tui/rename_test.go
@@ -101,12 +101,12 @@ func TestRenameWhileFiltered(t *testing.T) {
 	h.keys("P", "/")
 	h.typeText("ra")
 	h.keys("enter")
-	h.mustSee("PEOPLE · 1 match · ra")
+	h.mustSee("PEOPLE · 1 match", "filter › ra · esc clears")
 
 	h.keys("r", "ctrl+u")
 	h.typeText("raff")
 	h.keys("enter")
-	h.mustSee("renamed raf → raff · 1 task", "PEOPLE · 1 match · ra", "@raff") // still matches: the filter stays
+	h.mustSee("renamed raf → raff · 1 task", "PEOPLE · 1 match", "filter › ra · esc clears", "@raff") // still matches: the filter stays
 
 	h.keys("r", "ctrl+u")
 	h.typeText("bob")
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/`
Expected: FAIL (build): `undefined: backlogWidth` (filter_test.go).

- [ ] **Step 3: Implement**

Patch `internal/tui/common.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/common.go b/internal/tui/common.go
index 4bd0280..bebbb18 100644
--- a/internal/tui/common.go
+++ b/internal/tui/common.go
@@ -251,7 +251,8 @@ func (m *Model) bodyHeight() int {
 	if m.banner != nil {
 		used++
 	}
-	if _, help := m.overlay.(helpOverlay); m.overlay != nil && !help {
+	_, help := m.overlay.(helpOverlay)
+	if _, inPane := m.overlay.(paneOverlay); m.overlay != nil && !help && !inPane {
 		used += strings.Count(m.overlay.view(m), "\n") + 1
 	}
 	return max(m.height-used, 1)
```

Patch `internal/tui/model.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/model.go b/internal/tui/model.go
index 416ed0d..e9fb588 100644
--- a/internal/tui/model.go
+++ b/internal/tui/model.go
@@ -475,7 +475,6 @@ func (m *Model) render() string {
 	if m.banner != nil {
 		parts = append(parts, m.bannerView())
 	}
-	fb, filtering := m.overlay.(*filterBar)
 	switch m.screen {
 	case screenToday:
 		parts = append(parts, m.todayView())
@@ -487,12 +486,9 @@ func (m *Model) render() string {
 	case screenPeople:
 		parts = append(parts, m.peopleView())
 	default:
-		if filtering {
-			parts = append(parts, fb.view(m)) // the filter line sits above the backlog
-		}
 		parts = append(parts, m.pickView())
 	}
-	if m.overlay != nil && !filtering {
+	if _, inPane := m.overlay.(paneOverlay); m.overlay != nil && !inPane {
 		parts = append(parts, m.overlay.view(m))
 	}
 	parts = append(parts, m.statusView(), m.hintsView())
```

Patch `internal/tui/overlays.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/overlays.go b/internal/tui/overlays.go
index 568fc8c..b07f4ba 100644
--- a/internal/tui/overlays.go
+++ b/internal/tui/overlays.go
@@ -88,8 +88,14 @@ type filterBar struct {
 	err     error    // the line doesn't parse; the last valid filter stays applied
 }
 
+// paneOverlay is an overlay drawn inside a pane instead of under the
+// screen's body, so it takes no height from the body: the filter lines (D56).
+type paneOverlay interface{ inPane() }
+
+func (*filterBar) inPane() {}
+
 func (m *Model) openFilter() tea.Cmd {
-	in := newTokenInput("filter › ", "@who #tag !1 ~s <7d >30d is:done words", m.width)
+	in := newTokenInput("filter › ", "@who #tag !1 ~s <7d >30d is:done words", backlogWidth(m.width)-3)
 	in.plain = true // no " -- " note in a filter: complete everywhere
 	if text := m.pick.filterText; text != "" {
 		in.input.SetValue(text + " ")
@@ -132,7 +138,7 @@ func (f *filterBar) paste(m *Model, msg tea.PasteMsg) tea.Cmd {
 	return cmd
 }
 
-func (f *filterBar) resize(width int) { f.in.resize(width) }
+func (f *filterBar) resize(width int) { f.in.resize(backlogWidth(width) - 3) }
 
 func (f *filterBar) view(m *Model) string { return f.in.view(m, f.err) }
 
```

Patch `internal/tui/people.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/people.go b/internal/tui/people.go
index d24f7cd..37b1ba5 100644
--- a/internal/tui/people.go
+++ b/internal/tui/people.go
@@ -86,11 +86,28 @@ func (p *peopleState) move(m *Model, delta int) {
 
 // follow keeps both cursors inside their windows. Called after every update.
 func (p *peopleState) follow(m *Model) {
-	visible := m.visibleRows()
-	p.offset = scroll(p.cursor, p.cursor, p.offset, visible, len(p.names))
-	p.tasks.follow(visible)
+	p.offset = scroll(p.cursor, p.cursor, p.offset, m.namesVisible(), len(p.names))
+	p.tasks.follow(m.visibleRows())
+}
+
+// namesVisible is how many names fit below the names frame's filter lines.
+func (m *Model) namesVisible() int { return max(m.visibleRows()-len(m.peopleFilterLines()), 1) }
+
+// peopleFilterLines is what the names frame shows above the names (D56):
+// the filter being typed, or the applied one, dim; nothing without one.
+func (m *Model) peopleFilterLines() []string {
+	if f, ok := m.overlay.(*peopleFilter); ok {
+		return []string{f.view(m)}
+	}
+	if m.people.filter != "" {
+		return []string{styleDim.Render("filter › " + m.people.filter + " · esc clears")}
+	}
+	return nil
 }
 
+// namesWidth is People's left column for a terminal width cells wide.
+func namesWidth(width int) int { return width / 3 }
+
 // personRows lists name's tasks (D52): the open ones grouped by priority
 // like the backlog, pinned ones too, oldest first within a group; then a
 // closed heading over the done and dropped ones, newest first. Every task
@@ -189,20 +206,20 @@ func (m *Model) peopleKey(key string) tea.Cmd {
 }
 
 func (m *Model) peopleView() string {
-	lw := m.width / 3
+	lw := namesWidth(m.width)
 	rw := m.width - lw
 	p := &m.people
 	title, empty := fmt.Sprintf("PEOPLE · %d", len(m.idx.people)), "nobody yet · @who names who asked"
 	if p.filter != "" {
-		title = "PEOPLE · " + plural(len(p.names), "match", "matches") + " · " + p.filter
-		empty = "no matches · esc clears"
+		title = "PEOPLE · " + plural(len(p.names), "match", "matches") // the filter has its own line (D56)
+		empty = "no matches"
 	}
-	var names []string
+	names := m.peopleFilterLines()
 	if len(p.names) == 0 {
 		names = append(names, styleDim.Render("  "+empty))
 	}
 	visible := m.visibleRows()
-	for i := p.offset; i < len(p.names) && i < p.offset+visible; i++ {
+	for i := p.offset; i < len(p.names) && i < p.offset+m.namesVisible(); i++ {
 		names = append(names, m.personLine(p.names[i], m.inner(lw), i == p.cursor))
 	}
 	left := m.box(title, names, lw, m.bodyHeight(), !p.onTasks)
@@ -298,8 +315,10 @@ var peopleHelp = []string{
 // filter, esc clears it.
 type peopleFilter struct{ input textinput.Model }
 
+func (*peopleFilter) inPane() {}
+
 func (m *Model) openPeopleFilter() tea.Cmd {
-	in := newInput("people › ", "name", m.width)
+	in := newInput("filter › ", "name", namesWidth(m.width)-3)
 	in.SetValue(m.people.filter)
 	in.CursorEnd()
 	m.people.onTasks = false
@@ -330,6 +349,6 @@ func (f *peopleFilter) paste(m *Model, msg tea.PasteMsg) tea.Cmd {
 	return cmd
 }
 
-func (f *peopleFilter) resize(width int) { fitInput(&f.input, width) }
+func (f *peopleFilter) resize(width int) { fitInput(&f.input, namesWidth(width)-3) }
 
 func (f *peopleFilter) view(*Model) string { return f.input.View() }
```

Patch `internal/tui/pick.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/pick.go b/internal/tui/pick.go
index 131c773..f93555c 100644
--- a/internal/tui/pick.go
+++ b/internal/tui/pick.go
@@ -336,14 +336,34 @@ func scroll(top, c, off, visible, n int) int {
 func (m *Model) visibleRows() int { return max(m.bodyHeight()-m.chrome(), 1) }
 
 // paneVisible is visibleRows for pane pn, less the Reviews lane when it
-// sits under pn on this screen (D49).
+// sits under pn on this screen (D49) and the backlog's filter lines (D56).
 func (m *Model) paneVisible(pn pane) int {
+	n := m.visibleRows()
 	if m.laneUnder(pn) {
-		return max(m.visibleRows()-m.laneLines(), 1)
+		n -= m.laneLines()
 	}
-	return m.visibleRows()
+	if m.screen == screenLog && pn == paneBacklog {
+		n -= len(m.filterLines())
+	}
+	return max(n, 1)
+}
+
+// filterLines is what the backlog's frame shows above its rows (D56): the
+// filter being typed (with its error and suggestions), or the applied
+// filter, dim; nothing without a filter.
+func (m *Model) filterLines() []string {
+	if fb, ok := m.overlay.(*filterBar); ok {
+		return strings.Split(fb.view(m), "\n")
+	}
+	if m.pick.filtered() {
+		return []string{styleDim.Render("filter › " + m.pick.filterText + " · esc clears")}
+	}
+	return nil
 }
 
+// backlogWidth is the Log's left column for a terminal width cells wide.
+func backlogWidth(width int) int { return width * 55 / 100 }
+
 // steps is how far a list key moves the cursor: a row for j/k, half the
 // visible rows for ctrl+d/ctrl+u (as in vim); 0 for any other key.
 func (m *Model) steps(key string) int {
@@ -539,7 +559,7 @@ func (m *Model) taskKey(key string, t domain.Task) (cmd tea.Cmd, ok bool) {
 }
 
 func (m *Model) pickView() string {
-	lw := m.width * 55 / 100
+	lw := backlogWidth(m.width)
 	rw := m.width - lw
 	title, empty := "BACKLOG · by "+string(m.pick.group), "nothing triaged yet · a to add · i to triage"
 	if m.pick.filtered() {
@@ -549,25 +569,26 @@ func (m *Model) pickView() string {
 				n++
 			}
 		}
-		title += " · " + plural(n, "match", "matches") + " · " + m.pick.filterText // the count first: truncation eats the filter text
-		empty = "no matches · esc clears"
+		title += " · " + plural(n, "match", "matches") // the filter itself has its own line (D56)
+		empty = "no matches"
 	}
-	left := m.paneBox(paneBacklog, title, empty, lw, m.pick.focus == paneBacklog && !m.lane.focused)
+	left := m.paneBox(paneBacklog, title, empty, m.filterLines(), lw, m.pick.focus == paneBacklog && !m.lane.focused)
 	left = append(left, m.laneView(lw)...)
-	right := m.paneBox(paneToday, "TODAY'S CANDIDATES", "nothing pinned · p on a backlog task", rw, m.pick.focus == paneToday && !m.lane.focused)
+	right := m.paneBox(paneToday, "TODAY'S CANDIDATES", "nothing pinned · p on a backlog task", nil, rw, m.pick.focus == paneToday && !m.lane.focused)
 	return beside(left, right)
 }
 
-// paneBox renders pane pn as a box width cells wide, its rows in its
-// window; focused shows its cursor. The Reviews lane under it on this screen
-// takes the rest of the column.
-func (m *Model) paneBox(pn pane, title, empty string, width int, focused bool) []string {
+// paneBox renders pane pn as a box width cells wide: the lines above (the
+// filter's, D56), then its rows in its window; focused shows its cursor.
+// The Reviews lane under it on this screen takes the rest of the column.
+func (m *Model) paneBox(pn pane, title, empty string, above []string, width int, focused bool) []string {
 	l := rowList{rows: m.rows(pn), cursor: m.pick.cursor[pn], offset: m.pick.offset[pn]}
 	height := m.bodyHeight()
 	if m.laneUnder(pn) {
 		height -= m.laneLines()
 	}
-	return m.box(title, m.listLines(l, m.paneVisible(pn), empty, m.inner(width), focused), width, height, focused)
+	lines := append(above, m.listLines(l, m.paneVisible(pn), empty, m.inner(width), focused)...)
+	return m.box(title, lines, width, height, focused)
 }
 
 // listLines renders the rows in a list's window of visible rows, rows
```

Patch `internal/tui/today.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/today.go b/internal/tui/today.go
index 2d7aac1..3b596f9 100644
--- a/internal/tui/today.go
+++ b/internal/tui/today.go
@@ -511,7 +511,7 @@ func (m *Model) todayView() string {
 	lw := m.width / 2
 	rw := m.width - lw
 	left := m.box(m.timelineTitle(), m.timelineLines(m.inner(lw)), lw, m.bodyHeight(), m.timelineFocused())
-	right := m.paneBox(paneToday, "CANDIDATES", "nothing pinned · L opens the log to pick", rw, m.today.focus == focusCandidates && !m.lane.focused)
+	right := m.paneBox(paneToday, "CANDIDATES", "nothing pinned · L opens the log to pick", nil, rw, m.today.focus == focusCandidates && !m.lane.focused)
 	right = append(right, m.laneView(rw)...)
 	return beside(left, right)
 }
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/browse_test.go internal/tui/common.go internal/tui/filter_test.go internal/tui/filterline_test.go internal/tui/model.go internal/tui/overlays.go internal/tui/people.go internal/tui/people_test.go internal/tui/pick.go internal/tui/pick_test.go internal/tui/rename_test.go internal/tui/today.go
git commit -m 'feat(tui): the applied filter keeps its own row in the Log and People (D56)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 4: Tab skips an empty pane; a reload that empties the focused pane moves the focus

D57. In the Log, `tab` into an empty pane stays put and says `nothing pinned yet · p pins a task`, `backlog is empty`, or `no matches · esc clears` (`emptyPane`). On Today, `tab` from the timeline to empty candidates says `nothing pinned · L opens the log to pick`. In People, `tab` with nobody listed says `nobody listed`. After a load, `refocus` moves the focus off a pane of the screen on show that this reload emptied (it had rows before the reload, has none now) to the other pane when that one has rows: the Log's two panes, Today's candidates back to the timeline, People's tasks back to the names. It does nothing while a filter is being typed, and never for a screen that isn't on show. One lane test pins a candidate first, since `tab` into empty candidates is now refused.

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/people.go`
- Modify: `internal/tui/pick.go`
- Modify: `internal/tui/today.go`
- Modify: `internal/tui/lane_test.go`
- Create: `internal/tui/tab_test.go`

**Interfaces:**
- Consumes: existing `pick.rows`, `today.focus`, `people.onTasks`, `people.tasks`.
- Produces: `func (*Model) emptyPane(pn pane) string`; `type paneSizes struct{ log [2]int; tasks int }`, `func (*Model) paneSizes() paneSizes`, `func (*Model) refocus(had paneSizes)` (called from the `loadedMsg` case).

- [ ] **Step 1: Write the failing tests**

Patch `internal/tui/lane_test.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/lane_test.go b/internal/tui/lane_test.go
index cecf4ab..92062d9 100644
--- a/internal/tui/lane_test.go
+++ b/internal/tui/lane_test.go
@@ -74,7 +74,8 @@ func TestReviewsLaneOnToday(t *testing.T) {
 	if h.m.lane.focused || h.m.today.focus != focusTimeline {
 		t.Errorf("tab leaves the lane for the timeline: lane %v, focus %v", h.m.lane.focused, h.m.today.focus)
 	}
-	h.keys("tab") // tab stays between timeline and candidates
+	h.pinned("A candidate !1 ~s") // tab skips an empty candidates pane (D57)
+	h.keys("tab")                 // tab stays between timeline and candidates
 	if h.m.lane.focused || h.m.today.focus != focusCandidates {
 		t.Errorf("tab: lane %v, focus %v", h.m.lane.focused, h.m.today.focus)
 	}
```

Create `internal/tui/tab_test.go`:

```go
package tui

import (
	"context"
	"testing"

	"tt/internal/config"
)

func TestTabSkipsAnEmptyPaneInTheLog(t *testing.T) {
	h := newLogHarness(t, config.Default())
	ctx := context.Background()
	a := h.add("Alpha !1 ~s")
	h.keys("tab")
	h.mustSee("nothing pinned yet · p pins a task")
	if h.m.pick.focus != paneBacklog {
		t.Fatal("tab into an empty candidates pane keeps the focus on the backlog")
	}

	h.keys("p") // Alpha is a candidate now: its reload emptied the backlog
	if h.m.pick.focus != paneToday {
		t.Fatal("a reload that empties the focused pane moves the focus to the other one")
	}
	h.keys("tab")
	h.mustSee("backlog is empty")
	if h.m.pick.focus != paneToday {
		t.Error("tab into an empty backlog keeps the focus on the candidates")
	}

	h.add("Beta !2 ~s")
	if err := h.m.applyFilter("zzz"); err != nil {
		t.Fatal(err)
	}
	h.keys("tab")
	h.mustSee("no matches · esc clears") // a filter matching nothing says so
	if err := h.m.applyFilter(""); err != nil {
		t.Fatal(err)
	}

	if _, err := h.svc.Unpin(ctx, a.ID); err != nil { // another shell empties the focused pane
		t.Fatal(err)
	}
	h.keys("R")
	if h.m.pick.focus != paneBacklog {
		t.Error("a reload that empties the focused pane moves the focus to the other one")
	}
}

func TestTabSkipsAnEmptyPaneOnToday(t *testing.T) {
	h := newHarness(t, config.Default())
	h.keys("tab")
	h.mustSee("nothing pinned · L opens the log to pick")
	if h.m.today.focus != focusTimeline {
		t.Fatal("tab into empty candidates keeps the focus on the timeline")
	}
	a := h.pinned("Alpha !1 ~s")
	h.keys("tab")
	if h.m.today.focus != focusCandidates {
		t.Fatal("tab moves to the candidates once there are some")
	}
	if _, err := h.svc.Unpin(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	if h.m.today.focus != focusTimeline {
		t.Error("the last candidate gone: the focus goes back to the timeline")
	}
}

func TestTabSkipsAnEmptyPaneInPeople(t *testing.T) {
	h := newHarness(t, config.Default())
	h.keys("P", "tab")
	h.mustSee("nobody listed")
	if h.m.people.onTasks {
		t.Fatal("tab with nobody listed keeps the focus on the names")
	}
	a := h.add("Ask @ana !1 ~s")
	h.keys("tab")
	if !h.m.people.onTasks {
		t.Fatal("tab moves to ana's tasks")
	}
	if err := h.svc.Delete(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	if h.m.people.onTasks {
		t.Error("ana's last task gone: the focus goes back to the (empty) names, not an empty tasks pane")
	}
	h.mustSee("nobody yet")
}

func TestARefocusLeavesScreensNotOnShowAlone(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.add("Alpha !1 ~s")
	h.keys("L") // the Log's focus is on the backlog
	h.keys("esc")
	if _, err := h.svc.Pin(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R") // the backlog emptied while Today was on show
	if h.m.pick.focus != paneBacklog {
		t.Error("the Log's focus only moves while the Log is on show")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TabSkips|Refocus'`
Expected: FAIL: `TestTabSkipsAnEmptyPaneInTheLog` (`screen lacks "nothing pinned yet · p pins a task"`), `TestTabSkipsAnEmptyPaneOnToday`, `TestTabSkipsAnEmptyPaneInPeople`; `TestARefocusLeavesScreensNotOnShowAlone` already passes (it guards against over-eager refocusing).

- [ ] **Step 3: Implement**

Patch `internal/tui/model.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/model.go b/internal/tui/model.go
index e9fb588..c61d12c 100644
--- a/internal/tui/model.go
+++ b/internal/tui/model.go
@@ -193,7 +193,9 @@ func (m *Model) update(msg tea.Msg) tea.Cmd {
 		}
 		m.state, m.loaded = msg.state, true
 		m.idx = buildIndex(msg.state)
+		had := m.paneSizes()
 		m.refreshRows()
+		m.refocus(had)
 		m.inbox.sync(m)
 	case watchTickMsg:
 		cmds := []tea.Cmd{m.checkVersion(), m.opts.Schedule(m.opts.WatchEvery, watchTickMsg{})}
@@ -373,6 +375,36 @@ func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
 	return m.pickKey(key)
 }
 
+// paneSizes counts the rows of the panes a reload can empty (D57): the
+// Log's two (Today's candidates are the Log's), People's tasks.
+type paneSizes struct {
+	log   [2]int
+	tasks int
+}
+
+func (m *Model) paneSizes() paneSizes {
+	return paneSizes{log: [2]int{len(m.pick.rows[paneBacklog]), len(m.pick.rows[paneToday])}, tasks: len(m.people.tasks.rows)}
+}
+
+// refocus moves the focus off a pane of the screen on show that this
+// reload emptied (D57; had is before it), to the other pane when that one
+// has rows. Not while a filter is being typed: the narrowing list is the
+// one being looked at.
+func (m *Model) refocus(had paneSizes) {
+	p := &m.pick
+	emptied := func(pn pane) bool { return had.log[pn] > 0 && len(p.rows[pn]) == 0 }
+	_, typing := m.overlay.(*filterBar)
+	switch {
+	case m.screen == screenLog && !typing && emptied(p.focus) && len(p.rows[1-p.focus]) > 0:
+		p.focus = 1 - p.focus
+		p.clamp(m)
+	case m.screen == screenToday && m.today.focus == focusCandidates && emptied(paneToday):
+		m.today.focus = focusTimeline
+	case m.screen == screenPeople && m.people.onTasks && had.tasks > 0 && len(m.people.tasks.rows) == 0:
+		m.people.onTasks = false
+	}
+}
+
 // dayIsStale: the planning day rolled over since the last load, so the view
 // still shows yesterday until StartDay runs.
 func (m *Model) dayIsStale() bool {
```

Patch `internal/tui/people.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/people.go b/internal/tui/people.go
index 37b1ba5..580783c 100644
--- a/internal/tui/people.go
+++ b/internal/tui/people.go
@@ -168,6 +168,10 @@ func (m *Model) peopleKey(key string) tea.Cmd {
 	}
 	switch key {
 	case "tab":
+		if !p.onTasks && len(p.tasks.rows) == 0 { // D57
+			m.setStatus("nobody listed")
+			return nil
+		}
 		p.onTasks = !p.onTasks
 		return nil
 	case "/":
```

Patch `internal/tui/pick.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/pick.go b/internal/tui/pick.go
index f93555c..1d9c4f0 100644
--- a/internal/tui/pick.go
+++ b/internal/tui/pick.go
@@ -204,6 +204,17 @@ func rolloverHint(t domain.Task) row {
 
 func (m *Model) rows(p pane) []row { return m.pick.rows[p] }
 
+// emptyPane says why the Log's pane pn has nothing to focus (D57).
+func (m *Model) emptyPane(pn pane) string {
+	switch {
+	case pn == paneToday:
+		return "nothing pinned yet · p pins a task"
+	case m.pick.filtered():
+		return "no matches · esc clears"
+	}
+	return "backlog is empty"
+}
+
 // clamp keeps each pane's cursor on a selectable row (see clampRows).
 func (p *pickState) clamp(m *Model) {
 	if p.expanded == nil {
@@ -415,7 +426,12 @@ func (m *Model) pickKey(key string) tea.Cmd {
 	}
 	switch key {
 	case "tab":
-		p.focus = 1 - p.focus
+		to := 1 - p.focus
+		if len(p.rows[to]) == 0 { // D57: say why instead of focusing nothing
+			m.setStatus(m.emptyPane(to))
+			return nil
+		}
+		p.focus = to
 		p.clamp(m)
 		return nil
 	case "i":
```

Patch `internal/tui/today.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/today.go b/internal/tui/today.go
index 3b596f9..2eb9a69 100644
--- a/internal/tui/today.go
+++ b/internal/tui/today.go
@@ -396,6 +396,10 @@ func (m *Model) todayKey(key string) tea.Cmd {
 	}
 	switch key {
 	case "tab":
+		if td.focus == focusTimeline && len(m.rows(paneToday)) == 0 { // D57
+			m.setStatus("nothing pinned · L opens the log to pick")
+			return nil
+		}
 		td.focus = 1 - td.focus
 		return nil
 	case "L":
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/lane_test.go internal/tui/model.go internal/tui/people.go internal/tui/pick.go internal/tui/tab_test.go internal/tui/today.go
git commit -m 'feat(tui): tab skips an empty pane and says why; a reload that empties the focused pane moves the focus (D57)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---

### Task 5: Blocks coloured per task; meetings dim; highlights run to the end of the row

D58. A block's `▓` (on its first row and on the rows it carries on into) and its label take one of `blockColours` (ANSI 2, 4, 5, 6, 12, 13), chosen by task id mod 6 through `blockStyle`. A closed (or missing) task's block is dim, and its label stays struck through without colour. A conflicted block's ` ⚠` is yellow (`styleAlert`), outside the colour. Meeting parts render dim. Every style laid over a whole row now goes through `within`: the timeline's past and cursor rows, the outside-hours rows, the ghost rows, the Log's closed and selected rows, Task detail's head and child rows, the Reviews lane's cursor row and People's cursor row. So the highlight runs to the row's end over a coloured, struck-through or dim part (the Plan 5a nested-styles carry-forward).

**Files:**
- Modify: `internal/tui/detail.go`
- Modify: `internal/tui/lane.go`
- Modify: `internal/tui/people.go`
- Modify: `internal/tui/pick.go`
- Modify: `internal/tui/today.go`
- Create: `internal/tui/colour_test.go`

**Interfaces:**
- Consumes: Task 1 (`within`), existing `blockText`, `meetingParts`, `timelineRow`, `ghostRow`, `rowLine`, `childLine`, `reviewLine`, `personLine`.
- Produces: `var blockColours = []string{"2", "4", "5", "6", "12", "13"}`; `func (*Model) blockStyle(b domain.Block) lipgloss.Style`; test helpers (colour_test.go) `on(st lipgloss.Style) string`, `(*harness).rawLine(s string) string`, `colour(i int) string`, `reversedToTheEnd(t, what, line string)`.

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/colour_test.go`:

```go
package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/config"
)

// on is the escape sequence that starts style st.
func on(st lipgloss.Style) string {
	r := st.Render("x")
	return r[:strings.Index(r, "x")]
}

// rawLine is the rendered (unstripped) line whose text contains s.
func (h *harness) rawLine(s string) string {
	for _, l := range strings.Split(h.m.render(), "\n") {
		if strings.Contains(ansi.Strip(l), s) {
			return l
		}
	}
	h.t.Fatalf("no line shows %q:\n%s", s, h.screen())
	return ""
}

// colour is block colour i's escape sequence (D58).
func colour(i int) string { return on(lipgloss.NewStyle().Foreground(lipgloss.Color(blockColours[i]))) }

func TestBlocksAreColouredPerTask(t *testing.T) {
	h := newHarness(t, config.Default())
	ctx := context.Background()
	fix := h.pinned("Fix CI !1 ~s")    // #1
	rfc := h.pinned("Write RFC !2 ~s") // #2
	h.block(fix.ID, 10*time.Hour, time.Hour)
	h.block(rfc.ID, 12*time.Hour, 30*time.Minute)
	h.block(fix.ID, 14*time.Hour, 30*time.Minute)
	c1, c2 := colour(int(fix.ID)%len(blockColours)), colour(int(rfc.ID)%len(blockColours))
	if c1 == c2 {
		t.Fatal("tasks #1 and #2 get different colours")
	}
	if l := h.rawLine("10:00 ▓ Fix CI"); !strings.Contains(l, c1+"▓") || !strings.Contains(l, c1+"Fix CI [P1 S]") {
		t.Errorf("a block's ▓ and label take its task's colour: %q", l)
	}
	if l := h.rawLine("10:30 ▓"); !strings.Contains(l, c1+"▓") {
		t.Errorf("the rows a block carries on into share its colour: %q", l)
	}
	if l := h.rawLine("12:00 ▓ Write RFC"); !strings.Contains(l, c2+"Write RFC") {
		t.Errorf("another task, another colour: %q", l)
	}
	if l := h.rawLine("14:00 ▓ Fix CI"); !strings.Contains(l, c1+"Fix CI") {
		t.Errorf("a task's blocks all share its colour: %q", l)
	}
	for _, c := range blockColours {
		if c == "1" || c == "3" || c == "9" || c == "11" {
			t.Errorf("block colour %s is red or yellow, which errors and ⚠ use", c)
		}
	}

	if _, err := h.svc.Done(ctx, rfc.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	if l := h.rawLine("12:00 ▓ Write RFC"); strings.Contains(l, c2) || !strings.Contains(l, on(styleClosed)) {
		t.Errorf("a closed task's block is struck through and dim, without colour: %q", l)
	}
}

func TestConflictsAndMeetingsKeepTheirOwnStyles(t *testing.T) {
	h := newCalHarness(t, config.Default())
	fix := h.pinned("Fix CI !1 ~s")
	h.block(fix.ID, 10*time.Hour, time.Hour)
	h.meetings(ev("Standup", 10*time.Hour, 10*time.Hour+30*time.Minute), ev("Lunch", 12*time.Hour, 13*time.Hour))
	l := h.rawLine("10:00 ▓ Fix CI")
	if !strings.Contains(l, on(styleAlert)+" ⚠") {
		t.Errorf("⚠ stays yellow inside a coloured label: %q", l)
	}
	if !strings.Contains(l, on(styleDim)+"■ Standup") || !strings.Contains(h.rawLine("12:00 ■ Lunch"), on(styleDim)+"■ Lunch") {
		t.Errorf("meetings keep one neutral dim style: %q", l)
	}
}

// reversedToTheEnd fails unless line is reverse video from its start to its
// end: every reset inside it is followed by reverse video again.
func reversedToTheEnd(t *testing.T, what, line string) {
	t.Helper()
	rev := on(styleSelected)
	if !strings.HasPrefix(line, rev) {
		t.Errorf("%s doesn't start reversed: %q", what, line)
	}
	body := strings.TrimSuffix(line, "\x1b[m")
	for i := strings.Index(body, "\x1b[m"); i >= 0; i = strings.Index(body, "\x1b[m") {
		if body = body[i+len("\x1b[m"):]; !strings.HasPrefix(body, rev) {
			t.Errorf("%s: the highlight ends early, before %q", what, ansi.Strip(body))
			return
		}
	}
}

func TestTheHighlightRunsToTheEndOfTheRow(t *testing.T) {
	h := newHarness(t, config.Default())
	ctx := context.Background()
	fix := h.pinned("Fix CI !1 ~s")
	h.block(fix.ID, 9*time.Hour, time.Hour) // the cursor rests on 09:00 (now)
	reversedToTheEnd(t, "the cursor's coloured block row", h.m.timelineRow(h.m.today.cursor, 60))

	if _, err := h.svc.Done(ctx, fix.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	reversedToTheEnd(t, "the cursor's row over a struck-through block", h.m.timelineRow(h.m.today.cursor, 60))

	parent := h.add("Parent !1 ~l")
	kid := h.add("Kid ~s")
	if _, err := h.svc.SetParent(ctx, kid.ID, parent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Done(ctx, parent.ID); err != nil { // the kid is listed on its own, with a dim ↳ #parent
		t.Fatal(err)
	}
	h.keys("R", "L")
	r, ok := h.m.selectedRow()
	if !ok || r.task.ID != kid.ID || r.orphan == 0 {
		t.Fatalf("the Log's cursor is on the orphaned kid: %+v", r)
	}
	reversedToTheEnd(t, "a Log row with a dim suffix", h.m.rowLine(r, 60, true))
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/ -run 'Coloured|OwnStyles|Highlight'`
Expected: FAIL (build): `undefined: blockColours`.

- [ ] **Step 3: Implement**

Patch `internal/tui/detail.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/detail.go b/internal/tui/detail.go
index 74083d9..781424a 100644
--- a/internal/tui/detail.go
+++ b/internal/tui/detail.go
@@ -242,7 +242,7 @@ func (m *Model) detailView() []string {
 		return nil
 	}
 	if d.cursor == 0 {
-		head[0] = styleSelected.Render(pad(head[0], m.inner(m.width)))
+		head[0] = within(styleSelected, pad(head[0], m.inner(m.width)))
 	}
 	lines := head
 	if d.rows[0].ParentID == 0 {
@@ -277,10 +277,10 @@ func (m *Model) childLine(c domain.Task, selected bool) string {
 	}
 	line = pad(line, m.inner(m.width))
 	if c.Status != domain.StatusOpen {
-		line = styleClosed.Render(line)
+		line = within(styleClosed, line)
 	}
 	if selected {
-		line = styleSelected.Render(line)
+		line = within(styleSelected, line)
 	}
 	return line
 }
```

Patch `internal/tui/lane.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/lane.go b/internal/tui/lane.go
index 074eacb..2d6e4d2 100644
--- a/internal/tui/lane.go
+++ b/internal/tui/lane.go
@@ -130,7 +130,7 @@ func (m *Model) reviewLine(i domain.SourceItem, width int, selected bool) string
 	line := pad(left, width-ansi.StringWidth(right)) + right
 	switch {
 	case selected:
-		return styleSelected.Render(pad(line, width))
+		return within(styleSelected, pad(line, width))
 	case linked:
 		return styleLinked.Render(line)
 	}
```

Patch `internal/tui/people.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/people.go b/internal/tui/people.go
index 580783c..a94cf04 100644
--- a/internal/tui/people.go
+++ b/internal/tui/people.go
@@ -284,7 +284,7 @@ func (m *Model) personLine(pr person, width int, cursor bool) string {
 	line := pad(fmt.Sprintf("  %s · %d open", pr.name, pr.open), width)
 	switch {
 	case cursor && !m.people.onTasks:
-		return styleSelected.Render(line)
+		return within(styleSelected, line)
 	case cursor:
 		return styleTitle.Render(line) // whose tasks the right pane shows
 	case pr.open == 0:
```

Patch `internal/tui/pick.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/pick.go b/internal/tui/pick.go
index 1d9c4f0..ff75c60 100644
--- a/internal/tui/pick.go
+++ b/internal/tui/pick.go
@@ -693,10 +693,10 @@ func (m *Model) rowLine(r row, width int, selected bool) string {
 	}
 	line := pad(b.String(), width)
 	if t.Status != domain.StatusOpen {
-		line = styleClosed.Render(line)
+		line = within(styleClosed, line)
 	}
 	if selected {
-		line = styleSelected.Render(line)
+		line = within(styleSelected, line) // the highlight runs to the end over styled parts (D58)
 	}
 	return line
 }
```

Patch `internal/tui/today.go` (save the block to a file, then `git apply <file>` from the repo root):

```diff
diff --git a/internal/tui/today.go b/internal/tui/today.go
index 2eb9a69..8536137 100644
--- a/internal/tui/today.go
+++ b/internal/tui/today.go
@@ -9,6 +9,7 @@ import (
 	"time"
 
 	tea "charm.land/bubbletea/v2"
+	"charm.land/lipgloss/v2"
 	"github.com/charmbracelet/x/ansi"
 
 	"tt/internal/app"
@@ -571,39 +572,56 @@ func (m *Model) timelineRow(i, width int) string {
 	}
 	if r.outside {
 		b := r.blocks[0]
-		line := pad(" "+clock(r.start)+" ▓ "+m.blockText(b, true)+" · outside of working hours", width)
+		line := pad(" "+clock(r.start)+" "+m.blockStyle(b).Render("▓")+" "+m.blockText(b, true)+" · outside of working hours", width)
 		if m.timelineFocused() && i == td.cursor {
-			return styleSelected.Render(line)
+			return within(styleSelected, line)
 		}
-		return styleDim.Render(line)
+		return within(styleDim, line)
 	}
 	var labels []string
-	carried := false
+	var first, carried *domain.Block // the first block starting in the row; one carrying on into it
 	for _, b := range r.blocks {
 		if b.Start >= r.start {
 			labels = append(labels, m.blockLabel(b))
+			if first == nil {
+				first = &b
+			}
 		} else {
-			carried = true
+			carried = &b
 		}
 	}
 	var parts []string
 	switch {
-	case len(labels) > 0:
-		parts = append(parts, "▓ "+strings.Join(labels, " · "))
-	case carried:
-		parts = append(parts, "▓")
+	case first != nil:
+		parts = append(parts, m.blockStyle(*first).Render("▓")+" "+strings.Join(labels, " · "))
+	case carried != nil:
+		parts = append(parts, m.blockStyle(*carried).Render("▓"))
 	}
 	parts = append(parts, m.meetingParts(r)...)
 	line := pad(" "+clock(r.start)+" "+strings.Join(parts, " · "), width)
 	if now := td.tl.now; now >= 0 && r.start+rowLen <= now {
-		line = styleDim.Render(line) // the past
+		line = within(styleDim, line) // the past
 	}
 	if m.timelineFocused() && i == td.cursor {
-		line = styleSelected.Render(line)
+		line = within(styleSelected, line)
 	}
 	return line
 }
 
+// blockColours are the timeline's block colours (D58): ANSI green, blue,
+// magenta, cyan, bright blue and bright magenta; never yellow or red, which
+// ⚠ and errors use.
+var blockColours = []string{"2", "4", "5", "6", "12", "13"}
+
+// blockStyle is block b's colour, chosen by its task's id so it never
+// changes during the day; a closed (or missing) task's block is dim.
+func (m *Model) blockStyle(b domain.Block) lipgloss.Style {
+	if t, ok := m.state.Snapshot.Get(b.TaskID); !ok || t.Status != domain.StatusOpen {
+		return styleDim
+	}
+	return lipgloss.NewStyle().Foreground(lipgloss.Color(blockColours[b.TaskID%int64(len(blockColours))]))
+}
+
 // timelineFocused: the timeline has the focus, not its candidates or the
 // Reviews lane, so its cursor row shows.
 func (m *Model) timelineFocused() bool { return m.today.focus == focusTimeline && !m.lane.focused }
@@ -631,11 +649,11 @@ func (m *Model) ghostRow(r tlRow, g domain.Block, width int) string {
 	line := pad(text, width)
 	switch {
 	case m.fits(g) != nil:
-		return styleError.Render(line)
+		return within(styleError, line)
 	case overMeeting:
-		return styleGhostAlert.Render(line)
+		return within(styleGhostAlert, line)
 	}
-	return styleSelected.Render(line)
+	return within(styleSelected, line)
 }
 
 // meetingParts renders row r's meetings (D41): ■ and the title of a band
@@ -656,7 +674,7 @@ func (m *Model) meetingParts(r tlRow) []string {
 		if bd.more > 0 {
 			s += fmt.Sprintf(" +%d", bd.more)
 		}
-		out = append(out, s)
+		out = append(out, styleDim.Render(s)) // one neutral style, never like a block (D58)
 	}
 	return out
 }
@@ -677,15 +695,16 @@ func (m *Model) blockText(b domain.Block, times bool) string {
 	if times {
 		s = span(b) + " " + s
 	}
-	if m.conflicted(b) {
-		s += " ⚠" // D18, D42: r re-slots it
-	}
 	switch t.Status {
 	case domain.StatusDone:
 		return styleClosed.Render(s + " ✓")
 	case domain.StatusDropped:
 		return styleClosed.Render(s + " ✗")
 	}
+	s = m.blockStyle(b).Render(s) // its task's colour (D58)
+	if m.conflicted(b) {
+		s += styleAlert.Render(" ⚠") // D18, D42: r re-slots it
+	}
 	return s
 }
 
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/`, then `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./internal/tui/ ./internal/app/`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/colour_test.go internal/tui/detail.go internal/tui/lane.go internal/tui/people.go internal/tui/pick.go internal/tui/today.go
git commit -m 'feat(tui): blocks coloured per task, meetings dim, highlights run to the end of the row (D58)

Co-Authored-By: Claude <noreply@anthropic.com>'
```

---


## After the last task

- Build tt into the scratchpad and drive the real binary through a Python `pty` script on a throwaway database (`TT_DB`, `TT_CONFIG=/nonexistent`, an 80×24 window). Seed it with `tt add`: `Fix CI @ana !1 ~s`, `Write the schema RFC @ana !2 ~m`, `Deploy fix !2 ~s`. Then: `esc` (banner); `L` → `BACKLOG`, `TODAY'S CANDIDATES`; `p` → `pinned`; `/`, `schema` → `schema`, `1 match`; `⏎` → `esc clears`; `esc` → `filter cleared`; `B` → `Today`; `b`, `⏎` → `blocked`; resize to 80×10 (TIOCSWINSZ + SIGWINCH) → `TIMELINE`; to 80×4 (no crash); back to 80×24 → `TIMELINE`, `CANDIDATES`; `P` → `PEOPLE`, `ana`; `q`. The database has one block for task 1 and task 1 pinned. Match short substrings (the renderer repaints changed cells only, so text that stays put, like `? help`, may not be re-sent).
- `go test -race ./internal/tui/ ./internal/app/`.
- Final whole-branch review, one fix wave, park residuals as spec carry-forwards.
- **The user tries the branch before anything merges:** `make build` in the worktree, then run `<worktree>/bin/tt` in the usual tmux pane (it opens the real database; no migration in this plan). Look at Today, the Log with a filter applied, People, the Inbox and Task detail, a narrow and a short pane. Merge (fast-forward to `main`) only after the user says so; fold any "change this" into a small reviewed commit on the branch first.
