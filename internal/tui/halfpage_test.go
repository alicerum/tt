package tui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/config"
)

// In a 16-line terminal the body has 13 lines and a list shows 12 rows, so
// half a page is 6.
const shortHeight = 16

func TestHalfPageInTheLog(t *testing.T) {
	h := newLogHarness(t, config.Default())
	for i := 1; i <= 20; i++ {
		h.add(fmt.Sprintf("Task %02d !1 ~s", i))
	}
	h.send(tea.WindowSizeMsg{Width: 140, Height: shortHeight})
	title := func() string {
		r, _ := h.m.selectedRow()
		return r.task.Title
	}
	if got := title(); got != "Task 01" {
		t.Fatalf("starts on %q", got)
	}
	h.keys("ctrl+d")
	if got := title(); got != "Task 07" {
		t.Errorf("ctrl+d: %q; want half a page (6 tasks) down", got)
	}
	h.keys("ctrl+u")
	if got := title(); got != "Task 01" {
		t.Errorf("ctrl+u: %q; want back up", got)
	}
	h.keys("ctrl+d", "ctrl+d", "ctrl+d", "ctrl+d")
	if got := title(); got != "Task 20" {
		t.Errorf("ctrl+d past the end: %q; want the last task", got)
	}
	h.mustSee("Task 20") // the scroll window followed
}

func TestHalfPageOnToday(t *testing.T) {
	h := newHarness(t, config.Default())
	h.send(tea.WindowSizeMsg{Width: 140, Height: shortHeight})
	h.keys("ctrl+d")
	if at := h.m.today.at; at != 12*time.Hour {
		t.Errorf("ctrl+d on the timeline from 09:00: %v; want 12:00", at)
	}
	h.keys("ctrl+u", "ctrl+u")
	if at := h.m.today.at; at != 8*time.Hour {
		t.Errorf("ctrl+u past the top: %v; want the first row", at)
	}

	h.pinned("Alpha !1 ~s")
	h.keys("tab", "b", "ctrl+d") // a picked-up ghost moves too
	if at := h.m.today.at; at != 12*time.Hour {
		t.Errorf("ctrl+d while placing: %v; want 12:00", at)
	}
}

func TestHalfPageInTaskDetail(t *testing.T) {
	h := newLogHarness(t, config.Default())
	var lines []string
	for i := 1; i <= 10; i++ {
		lines = append(lines, fmt.Sprintf("Kid %02d ~s", i))
	}
	_, kids := h.family("Parent !1 ~xl", lines...)
	h.send(tea.WindowSizeMsg{Width: 140, Height: shortHeight})
	h.keys("enter", "ctrl+d")
	if h.m.screen != screenDetail || h.m.detail.selected != kids[5].ID {
		t.Errorf("ctrl+d in detail: selected #%d; want %q (6 rows down from the task)", h.m.detail.selected, kids[5].Title)
	}
	h.keys("ctrl+u")
	if h.m.detail.cursor != 0 {
		t.Errorf("ctrl+u: cursor %d; want back on the task", h.m.detail.cursor)
	}
}
