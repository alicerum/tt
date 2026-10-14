package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/config"
	"tt/internal/domain"
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
	fix := h.pinned("Fix CI !3 ~s")    // #1
	rfc := h.pinned("Write RFC !2 ~s") // #2
	h.block(fix.ID, 10*time.Hour, time.Hour)
	h.block(rfc.ID, 12*time.Hour, 30*time.Minute)
	h.block(fix.ID, 14*time.Hour, 30*time.Minute)
	c1, c2 := colour(int(fix.ID)%len(blockColours)), colour(int(rfc.ID)%len(blockColours))
	if c1 == c2 {
		t.Fatal("tasks #1 and #2 get different colours")
	}
	if l := h.rawLine("10:00 ▓ Fix CI"); !strings.Contains(l, c1+"▓") || !strings.Contains(l, c1+"Fix CI [ 50 S]") {
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
	fix := h.pinned("Fix CI !3 ~s")
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
	fix := h.pinned("Fix CI !3 ~s")
	h.block(fix.ID, 9*time.Hour, time.Hour) // the cursor rests on 09:00 (now)
	reversedToTheEnd(t, "the cursor's coloured block row", h.m.timelineRow(h.m.today.cursor, 60))

	if _, err := h.svc.Done(ctx, fix.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	reversedToTheEnd(t, "the cursor's row over a struck-through block", h.m.timelineRow(h.m.today.cursor, 60))

	// A closed Log row nests the closed style under the highlight.
	shipped := h.add("Shipped ~s")
	if _, err := h.svc.Done(ctx, shipped.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "L")
	if err := h.m.applyFilter("shipped is:done"); err != nil {
		t.Fatal(err)
	}
	r, ok := h.m.selectedRow()
	if !ok || r.task.ID != shipped.ID || r.task.Status != domain.StatusDone {
		t.Fatalf("the Log's cursor is on the done task: %+v", r)
	}
	reversedToTheEnd(t, "a selected closed Log row", h.m.rowLine(r, 60, true))
}
