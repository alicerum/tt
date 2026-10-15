package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"tt/internal/config"
)

func TestFilterEscRestoresTheSelection(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.add("Alpha !3 ~s")
	h.add("Beta !3 ~s")
	gamma := h.add("Gamma !3 ~s")
	h.keys("j", "j")
	h.keys("/")
	h.typeText("alp") // live narrowing moves the cursor to Alpha
	h.mustSee("1 match", "filter › alp")
	h.keys("esc")
	if r, _ := h.m.selectedRow(); r.task.ID != gamma.ID {
		t.Errorf("after esc the cursor is on %q; want Gamma, where it was before /", r.task.Title)
	}
}

func TestGroupingSavesTheLatestChoice(t *testing.T) {
	h := newLogHarness(t, config.Default())
	g := tea.KeyPressMsg{Code: 'g', Text: "g"}

	// Two quick g presses: the second save waits for the first, then saves the latest.
	_, first := h.m.Update(g)
	_, second := h.m.Update(g)
	if second != nil {
		t.Fatal("a second save must not run while the first is in flight")
	}
	h.run(first)
	h.mustSee("BACKLOG · by size")
	if v, _ := h.svc.UIState(context.Background(), groupKey); v != "size" {
		t.Errorf("remembered %q; want the latest choice, size", v)
	}

	// The startup read landing after a g press doesn't override it.
	h.send(uiStateMsg{group: "requester"})
	h.mustSee("BACKLOG · by size")
}

func TestFilteredExpandCollapseLeavesTheBacklogAlone(t *testing.T) {
	h := newLogHarness(t, config.Default())
	parent, _ := h.family("Auth migration !3 ~xl #auth", "Rollback plan ~s", "Token spike ~m")
	h.keys("right") // expanded before filtering
	h.mustSee("▾ [50 XL] Auth migration", "    [50 S] Rollback plan")

	if err := h.m.applyFilter("#auth"); err != nil {
		t.Fatal(err)
	}
	h.keys("j", "left") // on a child: the parent takes the cursor
	if r, _ := h.m.selectedRow(); r.task.ID != parent.ID {
		t.Errorf("cursor on %q; want the parent", r.task.Title)
	}
	h.keys("left") // on the parent: filtered parents stay expanded
	h.mustSee("    [50 S] Rollback plan")
	// Clearing the filter shows the parent still expanded, as before the filter.
	h.keys("esc")
	h.mustSee("▾ [50 XL] Auth migration", "    [50 S] Rollback plan")
}

func TestParentWithoutListedChildrenHasNoMarker(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.family("Lonely parent !2 ~m", "Untriaged idea") // the child is in the Inbox
	h.mustSee("  [33 M] Lonely parent")
	h.mustNotSee("▸ [33 M] Lonely parent")
	h.keys("right")
	h.mustNotSee("Untriaged idea")
}

func TestFilterBarCompletesAfterDashes(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.add("One @ana !3 ~s")
	h.keys("/")
	h.typeText("-- @an") // in a filter, -- is just a word
	h.mustSee("tab → @ana")
}
