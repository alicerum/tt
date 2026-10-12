package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"tt/internal/config"
)

func TestFilterEscRestoresTheSelection(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("Alpha !1 ~s")
	h.add("Beta !1 ~s")
	gamma := h.add("Gamma !1 ~s")
	h.keys("j", "j")
	h.keys("/")
	h.typeText("alp") // live narrowing moves the cursor to Alpha
	h.mustSee("1 match · alp")
	h.keys("esc")
	if r, _ := h.m.selectedRow(); r.task.ID != gamma.ID {
		t.Errorf("after esc the cursor is on %q; want Gamma, where it was before /", r.task.Title)
	}
}

func TestGroupingSavesTheLatestChoice(t *testing.T) {
	h := newHarness(t, config.Default())
	g := tea.KeyPressMsg{Code: 'g', Text: "g"}

	// Two quick g presses: the second save waits for the first, then saves the latest.
	_, first := h.m.Update(g)
	_, second := h.m.Update(g)
	if second != nil {
		t.Fatal("a second save must not run while the first is in flight")
	}
	h.run(first)
	h.mustSee("BACKLOG · by created")
	if v, _ := h.svc.UIState(context.Background(), groupKey); v != "created" {
		t.Errorf("remembered %q; want the latest choice, created", v)
	}

	// The startup read landing after a g press doesn't override it.
	h.send(uiStateMsg{group: "requester"})
	h.mustSee("BACKLOG · by created")
}

func TestFilteredExpandCollapseLeavesTheBacklogAlone(t *testing.T) {
	h := newHarness(t, config.Default())
	parent, _ := h.family("Auth migration !1 ~xl #auth", "Rollback plan ~s", "Token spike ~m")
	h.keys("right") // expanded before filtering
	h.mustSee("▾ [P1 XL] Auth migration", "    [P1 S] Rollback plan")

	if err := h.m.applyFilter("#auth"); err != nil {
		t.Fatal(err)
	}
	h.keys("j", "left") // on a child: the parent takes the cursor
	if r, _ := h.m.selectedRow(); r.task.ID != parent.ID {
		t.Errorf("cursor on %q; want the parent", r.task.Title)
	}
	h.keys("left") // on the parent: filtered parents stay expanded
	h.mustSee("    [P1 S] Rollback plan")
	// Clearing the filter shows the parent still expanded, as before the filter.
	h.keys("esc")
	h.mustSee("▾ [P1 XL] Auth migration", "    [P1 S] Rollback plan")
}

func TestParentWithoutListedChildrenHasNoMarker(t *testing.T) {
	h := newHarness(t, config.Default())
	h.family("Lonely parent !2 ~m", "Untriaged idea") // the child is in the Inbox
	h.mustSee("  [P2 M] Lonely parent")
	h.mustNotSee("▸ [P2 M] Lonely parent")
	h.keys("right")
	h.mustNotSee("Untriaged idea")
}

func TestFilterBarCompletesAfterDashes(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("One @ana !1 ~s")
	h.keys("/")
	h.typeText("-- @an") // in a filter, -- is just a word
	h.mustSee("tab → @ana")
}
