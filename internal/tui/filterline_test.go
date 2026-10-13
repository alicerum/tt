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
