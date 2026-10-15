package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"tt/internal/config"
	"tt/internal/domain"
)

// meetings makes the fake calendar return events on the harness's day
// (each "HH:MM–HH:MM title") and fetches them with R.
func (h *harness) meetings(events ...domain.Event) {
	h.t.Helper()
	h.cal.Set(events, nil)
	h.keys("R")
}

func ev(title string, from, to time.Duration) domain.Event {
	day := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	return domain.Event{Title: title, Start: day.Add(from), End: day.Add(to), Availability: "busy", Status: "accepted"}
}

func TestMeetingsOnToday(t *testing.T) {
	h := newCalHarness(t, config.Default())
	fix := h.pinned("Fix CI !3 ~s")
	h.block(fix.ID, 12*time.Hour, time.Hour)
	h.meetings(
		ev("Early", 6*time.Hour, 7*time.Hour),                                   // outside the working hours: not shown
		ev("Breakfast", 7*time.Hour+30*time.Minute, 8*time.Hour+30*time.Minute), // runs into them
		ev("Standup", 9*time.Hour+30*time.Minute, 9*time.Hour+45*time.Minute),
		ev("Review", 10*time.Hour, 11*time.Hour),
		ev("Sync", 10*time.Hour+30*time.Minute, 11*time.Hour+15*time.Minute),
		ev("Lunch", 12*time.Hour, 13*time.Hour),
	)
	h.mustSee(
		" 08:00 ■ Breakfast",           // named on its first row inside the hours
		" 09:30 ■ 09:30–09:45 Standup", // off the grid: exact times (D41)
		" 10:00 ■ 10:00–11:15 Review +1", " 10:30 ■", " 11:00 ■",
		" 12:00 ▓ Fix CI [50 S] ⚠ · ■ Lunch", " 12:30 ▓ · ■", // the block first, then the meeting
		"TIMELINE · 0/1 done · ⚠ 1",
	)
	h.mustNotSee("Early", " 11:30 ■")

	h.keys("j", "x") // the Standup row: nothing to act on
	h.mustSee("meeting · read-only")
	if h.task(fix.ID).Status != domain.StatusOpen {
		t.Fatal("a key on a meeting row touched a task")
	}
	h.keys("j", "j", "j", "j", "j", "x") // 12:00: the block's task
	h.mustSee(`✓ "Fix CI"`)
	h.mustNotSee("⚠") // a closed task's block is history: no conflict
}

func TestMeetingTitlesAreOneCleanLine(t *testing.T) {
	h := newCalHarness(t, config.Default())
	h.meetings(ev("Stand\nup\t\x1b]52;c;aGk=\x07", 10*time.Hour, 11*time.Hour))
	h.mustSee(" 10:00 ■ Stand up ]52;c;aGk=")
	raw := h.m.render()
	if strings.Contains(raw, "\x1b]52") || strings.Contains(raw, "\x07") {
		t.Errorf("a meeting title reached the terminal raw: %q", raw)
	}
	if lines := strings.Count(raw, "\n") + 1; lines > 40 {
		t.Errorf("the screen has %d lines on a 40-line terminal", lines)
	}
}

func TestPlacementAroundMeetings(t *testing.T) {
	h := newCalHarness(t, config.Default())
	h.meetings(
		ev("Standup", 9*time.Hour, 9*time.Hour+30*time.Minute),
		ev("Review", 10*time.Hour, 11*time.Hour),
	)
	h.pinned("Alpha !3 ~s")
	h.keys("tab", "b")
	h.mustSee(" 09:30 ░ Alpha 30m") // D42: the first row free of meetings; the length stops at Review
	h.keys("j")
	h.mustSee(" 10:00 ░ Alpha 1h ⚠ · ■ Review") // over a meeting: the whole length, with ⚠
	h.keys("enter")
	h.mustSee(`blocked "Alpha" 10:00–11:00 · ⚠ overlaps meeting "Review" 10:00–11:00`,
		" 10:00 ▓ Alpha [50 S] ⚠ · ■ Review", "⚠ 1")

	beta := h.pinned("Beta !2 ~s")
	h.block(beta.ID, 14*time.Hour, time.Hour)
	h.mustSee(" 14:00 ▓ Beta [33 S]")
	h.mustNotSee("Beta [33 S] ⚠")
	h.meetings( // a meeting that lands on a block later (D18)
		ev("Standup", 9*time.Hour, 9*time.Hour+30*time.Minute),
		ev("Review", 10*time.Hour, 11*time.Hour),
		ev("Escalation", 14*time.Hour+30*time.Minute, 15*time.Hour),
	)
	h.mustSee(" 14:00 ▓ Beta [33 S] ⚠", " 14:30 ▓ · ■ Escalation", "⚠ 2")
	if b := h.blocks(); len(b) != 2 || b[1].Start != 14*time.Hour {
		t.Fatalf("the block keeps its time (D18): %+v", b)
	}

	h.keys("j", "m", "j", "j", "enter") // Alpha (10:00) to 11:00
	h.mustSee(`moved "Alpha" to 11:00–12:00`, "⚠ 1")
	h.mustNotSee("overlaps meeting")
	h.keys("+")
	h.mustSee(`"Alpha" now 11:00–12:30`)

	ctx := context.Background()
	if _, err := h.svc.Done(ctx, beta.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	h.mustNotSee("⚠")
}

func TestEnterListsTheMeetingsOfARow(t *testing.T) {
	h := newCalHarness(t, config.Default())
	fix := h.pinned("Fix CI !3 ~s")
	h.block(fix.ID, 12*time.Hour, time.Hour)
	h.meetings(
		ev("Coffee", 10*time.Hour, 10*time.Hour+15*time.Minute), // two bands in one row
		ev("Check-in", 10*time.Hour+15*time.Minute, 10*time.Hour+30*time.Minute),
		ev("Lunch", 12*time.Hour, 13*time.Hour),
		ev("Weekly sync", 15*time.Hour, 16*time.Hour),
		ev("1:1 Ana", 15*time.Hour+30*time.Minute, 16*time.Hour+30*time.Minute),
		ev("Design review", 16*time.Hour, 17*time.Hour),
	)
	h.mustSee(" 15:00 ■ Weekly sync +2")

	h.m.today.cursor = h.m.today.tl.rowOf(16*time.Hour + 30*time.Minute) // a later row of the band
	h.keys("enter")
	h.before("MEETINGS · 15:00–17:00", "  15:00–16:00  Weekly sync", "  15:30–16:30  1:1 Ana", "  16:00–17:00  Design review", "any key closes")
	h.mustNotSee("12:00–13:00  Lunch") // not in the band
	if lines := strings.Count(h.m.render(), "\n") + 1; lines > 40 {
		t.Errorf("the screen has %d lines on a 40-line terminal", lines)
	}
	h.keys("enter") // closes the list, and nothing else: on the row it would open it again
	h.mustNotSee("MEETINGS ·", "any key closes")

	h.m.today.cursor = h.m.today.tl.rowOf(10 * time.Hour)
	h.keys("enter")
	h.before("MEETINGS · 10:00–10:30", "  10:00–10:15  Coffee", "  10:15–10:30  Check-in", "any key closes")
	h.keys("esc")

	h.m.today.cursor = h.m.today.tl.rowOf(12 * time.Hour) // a block over a meeting: ⏎ opens its task (D31)
	h.keys("enter")
	h.mustSee(fmt.Sprintf("#%d Fix CI", fix.ID), "TASK")
}
