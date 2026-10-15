package tui

import (
	"context"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"tt/internal/config"
	"tt/internal/domain"
)

// childIDs lists parentID's children in the order Task detail shows them.
func (h *harness) childIDs(parentID int64) []int64 {
	var ids []int64
	for _, c := range h.m.idx.children[parentID] {
		ids = append(ids, c.ID)
	}
	return ids
}

func TestAddChildrenOneAfterAnother(t *testing.T) {
	h := newLogHarness(t, config.Default())
	parent := h.add("Auth migration !3 ~xl #auth")
	h.keys("enter")
	h.mustSee("no children · a adds one")

	h.keys("a")
	h.mustSee("child of #1 › ")
	h.typeText("Token spike ~s")
	h.keys("enter")
	h.mustSee("added #2 under #1", "CHILDREN ░░░░░░░░ 0/1", "  [50 S] Token spike")
	if _, ok := h.m.overlay.(*addChild); !ok {
		t.Fatal("the add-child input stays open for the next child")
	}
	h.mustSee("child of #1 › title ~s #tag") // emptied: the placeholder shows
	h.typeText("Rollback plan ~m #ops -- ask Ana first")
	h.keys("enter")
	h.mustSee("added #3 under #1", "0/2")

	h.typeText("Urgent bit !3 ~s") // impact is the parent's (D32, D73)
	h.keys("enter")
	h.mustSee("✗ invalid value: children inherit impact from #1; drop !3", "child of #1 › Urgent bit !3 ~s")
	h.keys("esc")
	if h.m.overlay != nil {
		t.Fatal("esc closes the input")
	}

	ids := h.childIDs(parent.ID)
	if !slices.Equal(ids, []int64{2, 3}) {
		t.Fatalf("children: %v", ids)
	}
	got := h.task(3)
	if got.Size != domain.SizeM || !slices.Equal(got.Tags, []string{"ops"}) || got.Note != "ask Ana first" {
		t.Errorf("second child: %+v", got)
	}
	if domain.InInbox(got, &parent) {
		t.Error("a sized child skips the Inbox")
	}

	h.keys("ctrl+z") // undo the last added child
	h.mustSee(`undid add "Rollback plan"`)
	if ids := h.childIDs(parent.ID); !slices.Equal(ids, []int64{2}) {
		t.Errorf("after undo: %v", ids)
	}

	// A child's detail can't take children (D6); a outside detail is quick-add.
	h.keys("j", "enter", "a")
	h.mustSee("one level only — esc to the parent")
	h.keys("esc", "esc", "a")
	h.mustSee("add › ")
}

func TestAddChildKeepsTheLineWhenItFails(t *testing.T) {
	h := newLogHarness(t, config.Default())
	parent := h.add("Auth migration !3 ~xl")
	h.keys("enter", "a")
	if err := h.svc.Delete(context.Background(), parent.ID); err != nil { // behind the TUI's back
		t.Fatal(err)
	}
	h.typeText("Token spike ~s")
	// Deliver the failure by hand: the reload that follows it would close the
	// overlay of a parent that is gone (see TestFallingBackClosesTheOverlay).
	_, cmd := h.m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	h.m.Update(cmd())
	h.mustSee("✗ ", "child of #1 › Token spike ~s")
}

func TestReorderChildren(t *testing.T) {
	h := newLogHarness(t, config.Default())
	parent, kids := h.family("Parent !3 ~l", "A ~s", "B ~s", "C ~s")
	a, b, c := kids[0].ID, kids[1].ID, kids[2].ID
	h.keys("enter", "J")
	h.mustSee("J/K move a child · j selects one")

	h.keys("j", "J") // A down
	h.mustSee(`moved "A"`)
	if ids := h.childIDs(parent.ID); !slices.Equal(ids, []int64{b, a, c}) {
		t.Fatalf("after A down: %v", ids)
	}
	if h.m.detail.selected != a {
		t.Error("the cursor stays on the moved child")
	}
	h.keys("K", "K")
	h.mustSee("already first")
	if ids := h.childIDs(parent.ID); !slices.Equal(ids, []int64{a, b, c}) {
		t.Fatalf("after A up: %v", ids)
	}
	h.keys("ctrl+z") // undoes the last move (A up)
	if ids := h.childIDs(parent.ID); !slices.Equal(ids, []int64{b, a, c}) {
		t.Errorf("after undo: %v", ids)
	}

	// Closed children stay below the open ones; moves skip them.
	if _, err := h.svc.Done(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "K")
	h.mustSee("already first") // B before A is closed: A is the first open child
	h.keys("J")
	h.mustSee(`moved "A"`)
	if ids := h.childIDs(parent.ID); !slices.Equal(ids, []int64{b, c, a}) {
		t.Errorf("A down past C: %v", ids)
	}
	if s := h.screen(); !(strings.Index(s, "] C") < strings.Index(s, "] A") && strings.Index(s, "] A") < strings.Index(s, "] B ✓")) {
		t.Errorf("open children in order, then closed ones:\n%s", s)
	}
}

func TestMultiLinePasteBecomesTheNote(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.keys("a")
	h.send(tea.PasteMsg{Content: "Look at the schema @ana !3 ~s\r\nAna: can you check\r\n\r\nthe migration?\r\n"})
	h.mustSee("add › Look at the schema @ana !3 ~s", "+ 3 lines → note")
	h.keys("enter")
	h.mustSee("added #1 to backlog")
	if got := h.task(1); got.Title != "Look at the schema" || got.Note != "Ana: can you check\n\nthe migration?" {
		t.Errorf("pasted task: %+v", got)
	}

	// esc drops the held lines with the rest of the input.
	h.keys("a")
	h.send(tea.PasteMsg{Content: "Draft\nbody"})
	h.keys("esc", "a")
	h.mustNotSee("→ note")
	h.keys("esc")

	// Add-child takes pastes the same way.
	h.keys("enter", "a")
	h.send(tea.PasteMsg{Content: "Child ~s\nwith details"})
	h.mustSee("+ 1 line → note")
	h.keys("enter")
	if got := h.task(2); got.ParentID != 1 || got.Note != "with details" {
		t.Errorf("pasted child: %+v", got)
	}
}
