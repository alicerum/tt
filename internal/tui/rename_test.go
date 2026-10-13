package tui

import (
	"context"
	"testing"
	"time"

	"tt/internal/config"
	"tt/internal/domain"
)

func TestRenameAPerson(t *testing.T) {
	h := newHarness(t, config.Default())
	one := h.add("One @raf !1 ~s")
	two := h.add("Two @raf !2 ~s")
	h.add("Ana's @ana !1 ~s")
	h.keys("P", "j")
	h.keys("r")
	h.mustSee("rename @raf › raf")
	h.keys("enter")
	h.mustSee("unchanged")

	h.keys("r", "ctrl+u", "enter")
	h.mustSee("✗ a name can't be empty")
	h.typeText("two words")
	h.keys("enter")
	h.mustSee("✗ a name is one word, like @ana")
	if h.m.overlay == nil {
		t.Fatal("a refused name keeps the prompt open with its text")
	}
	h.keys("ctrl+u")
	h.typeText(" @Raff, ")
	h.keys("enter")
	h.mustSee("renamed raf → raff · 2 tasks", "raff · 2 open", "@raff", "One", "Two")
	h.mustNotSee("raf · ")
	if h.m.people.selected != "raff" {
		t.Errorf("the cursor follows the new name, got %q", h.m.people.selected)
	}
	if a, b := h.task(one.ID).Requester, h.task(two.ID).Requester; a != "raff" || b != "raff" {
		t.Errorf("requesters = %q, %q", a, b)
	}

	h.keys("ctrl+z")
	h.mustSee("undid rename raf → raff", "raf · 2 open")
	h.mustNotSee("raff · ")
}

func TestMergeIntoAnotherPerson(t *testing.T) {
	h := newHarness(t, config.Default())
	ctx := context.Background()
	one := h.add("One @raf !1 ~s")
	two := h.add("Two @raf !2 ~s")
	own := h.add("Ana's @ana !1 ~s")
	merge := func() {
		t.Helper()
		h.keys("r", "ctrl+u")
		h.typeText("Ana")
		h.keys("enter")
		h.mustSee(`"ana" already exists · ⏎ merge into them · esc`)
	}
	h.keys("P", "j", "tab") // r works from the tasks pane too
	merge()
	h.keys("esc")
	h.mustSee("cancelled", "raf · 2 open")

	merge()
	h.keys("enter")
	h.mustSee("merged raf → ana · 2 tasks", "ana · 3 open", "One", "Two", "Ana's")
	h.mustNotSee("raf · ")
	if h.m.people.selected != "ana" {
		t.Errorf("the cursor follows the merged name, got %q", h.m.people.selected)
	}

	h.keys("ctrl+z")
	h.mustSee("undid merge raf → ana", "ana · 1 open", "raf · 2 open")
	if got := h.task(own.ID).Requester; got != "ana" {
		t.Errorf("the merge's undo moved ana's own task to %q", got)
	}

	if h.m.people.selected != "ana" {
		t.Errorf("after the undo the cursor stays on ana, who is still there: %q", h.m.people.selected)
	}
	h.keys("tab", "j") // the names pane, raf
	merge()
	h.keys("enter")
	h.clock.now = h.clock.now.Add(time.Minute)
	if _, err := h.svc.SetSize(ctx, one.ID, domain.SizeM); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "ctrl+z")
	h.mustSee("can't undo merge raf → ana: it changed since")
	if a, b := h.task(one.ID).Requester, h.task(two.ID).Requester; a != "ana" || b != "ana" {
		t.Errorf("a refused undo changes nothing: %q, %q", a, b)
	}
}

func TestRenameWhileFiltered(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("One @raf !1 ~s")
	h.add("Ana's @ana !1 ~s")
	h.keys("P", "/")
	h.typeText("ra")
	h.keys("enter")
	h.mustSee("PEOPLE · 1 match", "filter › ra · esc clears")

	h.keys("r", "ctrl+u")
	h.typeText("raff")
	h.keys("enter")
	h.mustSee("renamed raf → raff · 1 task", "PEOPLE · 1 match", "filter › ra · esc clears", "@raff") // still matches: the filter stays

	h.keys("r", "ctrl+u")
	h.typeText("bob")
	h.keys("enter")
	h.mustSee("renamed raff → bob · 1 task", "PEOPLE · 2", "@bob") // would be hidden: the filter goes
	if h.m.people.selected != "bob" {
		t.Errorf("the cursor follows the new name, got %q", h.m.people.selected)
	}
}

func TestRenameOfANameGoneMeanwhile(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("One @raf !1 ~s")
	h.add("Ana's @ana !1 ~s")
	h.keys("P", "j")
	if _, _, err := h.svc.RenameRequester(context.Background(), "raf", "zed"); err != nil {
		t.Fatal(err) // another shell; the TUI hasn't reloaded yet
	}
	h.keys("r", "ctrl+u")
	h.typeText("raff")
	h.keys("enter")
	h.mustSee("✗ no task has @raf any more", "zed · 1 open")
	if h.m.people.selected == "raff" {
		t.Error("a failed rename doesn't leave the cursor on a name nobody has")
	}
}
