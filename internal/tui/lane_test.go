package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/app"
	"tt/internal/config"
	"tt/internal/domain"
)

// threeReviews fills the lane with three requests of different ages.
func threeReviews(h *harness) {
	h.setReviews(
		h.reviewItem("acme/photos", 877, "Bump go", "tom", 26*time.Hour),
		h.reviewItem("acme/api", 901, "Add idempotency key", "ana", 75*time.Hour),
		h.reviewItem("acme/backend", 915, "Lint config", "ion", 2*time.Hour),
	)
}

// lineWith is the screen line containing s ("" if none).
func (h *harness) lineWith(s string) string {
	for _, l := range strings.Split(h.screen(), "\n") {
		if strings.Contains(l, s) {
			return l
		}
	}
	return ""
}

func TestReviewsLaneOnToday(t *testing.T) {
	h := newGHHarness(t, config.Default())
	h.mustSee("REVIEWS · none waiting")
	threeReviews(h)
	h.mustSee("REVIEWS · 3 waiting", "#901 api  Add idempotency key", "@ana  3d", "#877 photos  Bump go", "@tom  1d", "@ion  2h")
	s := h.screen()
	if a, b, c := strings.Index(s, "#915"), strings.Index(s, "#877"), strings.Index(s, "#901"); !(a < b && b < c) {
		t.Errorf("the newest request first (D60):\n%s", s)
	}
	if l := h.lineWith("REVIEWS"); !strings.Contains(l, "│╭ REVIEWS") {
		t.Errorf("the lane sits in the right column, under the candidates: %q", l)
	}
	h.mustSee("w reviews · j/k move") // the hint, before the timeline's

	h.keys("w")
	h.mustSee("j/k move · p pin · o open PR · w back")
	h.keys("j", "p") // #877
	// Impact 1, due 3 days after the request a day ago (D75).
	h.mustSee(`pinned "Review #877: Bump go" · 50 S`, "REVIEWS · 3 waiting", "Review #877: Bump go")
	h.keys("k") // up to #915, the newest
	if l := h.lineWith("Review #877"); !strings.Contains(l, "[ 50 S]") {
		t.Errorf("the review is a candidate with D48's and D75's defaults: %q", l)
	}
	h.keys("ctrl+z")
	h.mustSee(`undid pin "Review #877: Bump go"`, "REVIEWS · 3 waiting")

	h.keys("o")
	if len(h.opened) != 1 || h.opened[0] != "https://github.com/acme/backend/pull/915" {
		t.Errorf("o opens the PR under the cursor: %v", h.opened)
	}
	for _, k := range []string{"x", "enter", "b", "e", "d"} {
		h.keys(k)
		h.mustSee("a review request · p pins it, o opens it")
	}
	if n := len(h.m.state.Snapshot.Tasks()); n != 0 {
		t.Fatalf("task keys in the lane changed something: %d tasks", n)
	}

	h.keys("tab") // back to where w came from: the timeline
	if h.m.lane.focused || h.m.today.focus != focusTimeline {
		t.Errorf("tab leaves the lane for the timeline: lane %v, focus %v", h.m.lane.focused, h.m.today.focus)
	}
	h.pinned("A candidate !3 ~s") // tab skips an empty candidates pane (D57)
	h.keys("tab")                 // tab stays between timeline and candidates
	if h.m.lane.focused || h.m.today.focus != focusCandidates {
		t.Errorf("tab: lane %v, focus %v", h.m.lane.focused, h.m.today.focus)
	}
	h.keys("w", "w")
	if h.m.lane.focused || h.m.today.focus != focusCandidates {
		t.Errorf("w twice comes back to the candidates: lane %v, focus %v", h.m.lane.focused, h.m.today.focus)
	}
}

func TestReviewsLaneInTheLog(t *testing.T) {
	h := newGHHarness(t, config.Default())
	threeReviews(h)
	h.add("Write RFC intro !2 ~m")
	h.keys("w", "L") // a screen key leaves the lane and keeps its meaning
	if h.m.screen != screenLog || h.m.lane.focused {
		t.Fatalf("L from the lane: screen %d, lane focused %v", h.m.screen, h.m.lane.focused)
	}
	if l := h.lineWith("REVIEWS · 3 waiting"); !strings.HasPrefix(l, "╭ REVIEWS") {
		t.Errorf("in the Log the lane sits in the left column, under the backlog: %q", l)
	}
	h.mustSee("w reviews · ⏎ open")
	h.keys("w", "j", "j", "p") // #901, the longest waiting, is last (D60)
	// Requested 3 days ago: due today (D75).
	h.mustSee(`pinned "Review #901: Add idempotency key" · 53 S`)
	if l := h.lineWith("Review #901"); !strings.Contains(l, "│") || strings.Index(l, "Review #901") < strings.Index(l, "│") {
		t.Errorf("the pinned review is a candidate (right), not in the backlog: %q", l)
	}
	h.keys("w", "esc") // esc keeps the Log's meaning: back to Today
	if h.m.screen != screenToday || h.m.lane.focused {
		t.Errorf("esc from the lane: screen %d, lane focused %v", h.m.screen, h.m.lane.focused)
	}

	// Done by hand while GitHub still asks: the row stays marked (D51), p reopens.
	h.keys("tab", "x")
	h.mustSee(`✓ "Review #901: Add idempotency key"`, "REVIEWS · 3 waiting")
	if l := h.lineWith("#901 api"); !strings.Contains(l, "· #2") {
		t.Errorf("a done review stays marked: %q", l)
	}
	h.keys("w", "p") // the cursor stayed on #901
	if tasks := h.m.state.Snapshot.Tasks(); len(tasks) != 2 || h.task(tasks[1].ID).Status != domain.StatusOpen {
		t.Errorf("p reopens the review's task: %+v", tasks)
	}
}

func TestReviewsLaneEmptyAndOff(t *testing.T) {
	h := newHarness(t, config.Default())
	h.mustNotSee("REVIEWS", "w reviews")
	h.keys("w")
	h.mustSee("GitHub is off · [github] enabled in the config")

	h = newGHHarness(t, config.Default())
	h.keys("w")
	h.mustSee("REVIEWS · none waiting", "no reviews waiting")
	h.mustNotSee("w reviews")
	if h.m.lane.focused {
		t.Error("an empty lane doesn't take the focus")
	}

	h.m.state.GitHub = app.GitHubState{SetUp: true, Err: "not logged in · gh auth login"}
	h.mustSee("REVIEWS · not fetched yet")

	// The lane emptied by a fetch: the focus goes back.
	h.setReviews(h.reviewItem("acme/api", 901, "Add idempotency key", "ana", time.Hour))
	h.keys("w", "p")
	if !h.m.lane.focused {
		t.Error("p keeps the lane's focus: the pinned request stays in it (D51)")
	}
	h.setReviews()
	if h.m.lane.focused {
		t.Error("the lane emptied by a fetch gives the focus back")
	}
	h.mustSee("REVIEWS · none waiting")
}

func TestReviewsLaneHeight(t *testing.T) {
	h := newGHHarness(t, config.Default())
	var items []domain.SourceItem
	for n := range 7 {
		items = append(items, h.reviewItem("acme/api", 900+n, "PR", "ana", time.Duration(n+1)*time.Hour)) // #900 newest, first
	}
	h.setReviews(items...)
	h.send(tea.WindowSizeMsg{Width: 140, Height: 24})
	for n := range 6 {
		h.add("Candidate " + string(rune('A'+n)) + " !3 ~s")
	}
	for range 6 {
		h.keys("L", "p", "B", "esc") // pin the first backlog task, back to Today
	}
	if len(h.m.state.Plan.Candidates) != 6 {
		t.Fatalf("candidates = %d", len(h.m.state.Plan.Candidates))
	}
	h.mustSee("#904", "REVIEWS · 7 waiting")
	h.mustNotSee("#905") // 24 lines: five rows (a third of the body is fewer)

	h.keys("w")
	for range 6 {
		h.keys("j")
	}
	h.mustSee("#906", "#902")
	h.mustNotSee("#901 ") // scrolled: the cursor stays in view

	h.send(tea.WindowSizeMsg{Width: 140, Height: 13})
	if lines := strings.Count(h.screen(), "\n") + 1; lines > 13 {
		t.Errorf("13 lines tall: the screen has %d", lines)
	}
	h.mustSee("REVIEWS · 7 waiting", "#906") // the lane gave up rows, kept the cursor
	if n := strings.Count(h.screen(), "@ana"); n != 3 {
		t.Errorf("a short column keeps 3 lane rows: %d", n)
	}
	if n := strings.Count(h.screen(), "Candidate "); n != 3 {
		t.Errorf("the candidates keep 3 rows above the lane: %d", n)
	}
	h.send(tea.WindowSizeMsg{Width: 140, Height: 8})
	h.mustNotSee("REVIEWS") // too short: the lane goes, the candidates stay
	h.mustSee("Candidate A")
}

func TestReviewsLaneGrowsToAThirdOfTheBody(t *testing.T) {
	h := newGHHarness(t, config.Default())
	var items []domain.SourceItem
	for n := range 30 {
		items = append(items, h.reviewItem("acme/api", 900+n, "PR", "ana", time.Duration(40-n)*time.Hour))
	}
	h.setReviews(items...)
	for _, c := range []struct{ height, rows int }{
		{24, 5},  // body 21: a third is 7 lines, under the 5-row floor
		{40, 10}, // body 37: 12 lines, the frame and 10 rows
		{60, 17}, // body 57: 19 lines
	} {
		h.send(tea.WindowSizeMsg{Width: 140, Height: c.height})
		if n := strings.Count(h.screen(), "@ana"); n != c.rows {
			t.Errorf("%d lines tall: the lane shows %d rows, want %d", c.height, n, c.rows)
		}
	}
	h.setReviews(items[:3]...)
	if n := strings.Count(h.screen(), "@ana"); n != 3 {
		t.Errorf("3 requests take 3 rows, not a third of the body: %d", n)
	}
}

func TestReviewRowsFitTheirColumn(t *testing.T) {
	h := newGHHarness(t, config.Default())
	h.setReviews(h.reviewItem("acme/api", 901, "A very long title "+strings.Repeat("that goes on ", 20)+"\x1b[31mred\ttab\nnewline", "ana", 3*24*time.Hour))
	l := h.lineWith("#901")
	if !strings.HasSuffix(strings.TrimRight(l, " │"), "@ana  3d") || strings.ContainsAny(l, "\x1b\t") {
		t.Errorf("a long title is cut so the author and wait stay; control characters go: %q", l)
	}
	if w := ansi.StringWidth(l); w > 140 {
		t.Errorf("line is %d wide", w)
	}
	if got := waited(h.clock.now, h.clock.now.Add(time.Minute)); got != "now" {
		t.Errorf("a request from the future (clock skew) waits %q", got)
	}
}

func TestReviewsWithATaskAreMarked(t *testing.T) {
	h := newGHHarness(t, config.Default())
	threeReviews(h)
	h.keys("w", "j", "p")                                                    // #877
	h.mustSee(`pinned "Review #877: Bump go" · 50 S`, "REVIEWS · 3 waiting") // it stays in the lane (D51)
	if l := h.lineWith("#877 photos"); !strings.HasSuffix(strings.TrimRight(l, " │"), "@tom  1d · #1") {
		t.Errorf("a review with a task names it: %q", l)
	}
	if h.m.lane.selected != "acme/photos#877" {
		t.Errorf("the cursor stays on the pinned review: %q", h.m.lane.selected)
	}
	on := strings.Split(styleLinked.Render("x"), "x")[0]
	if line := h.m.reviewLine(h.m.reviews()[1], 100, false); on == "" || !strings.HasPrefix(line, on) {
		t.Errorf("a review with a task is coloured: %q", line)
	}
	if line := h.m.reviewLine(h.m.reviews()[0], 100, false); strings.Contains(line, on) || strings.Contains(line, " · #") {
		t.Errorf("a review without a task is plain: %q", line)
	}

	h.keys("p")
	h.mustSee("already pinned · #1")
	if len(h.m.state.Snapshot.Tasks()) != 1 || len(h.m.undo) != 1 {
		t.Errorf("p on a pinned review changes nothing: %d tasks, %d undos", len(h.m.state.Snapshot.Tasks()), len(h.m.undo))
	}

	ctx := context.Background()
	if _, err := h.svc.Done(ctx, 1); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	if l := h.lineWith("#877 photos"); !strings.Contains(l, "· #1") {
		t.Errorf("a done task still marks its review: %q", l)
	}
	if _, err := h.svc.Drop(ctx, 1, ""); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	if l := h.lineWith("#877 photos"); strings.Contains(l, "· #1") {
		t.Errorf("a dropped task doesn't mark its review: %q", l)
	}
	h.keys("p") // the lane kept its focus: reopens the dropped task
	h.mustSee(`pinned "Review #877: Bump go"`)
	if got := h.task(1); got.Status != domain.StatusOpen {
		t.Errorf("p on a dropped review's row reopens its task: %v", got.Status)
	}
}
