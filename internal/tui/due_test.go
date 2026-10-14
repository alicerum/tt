package tui

import (
	"context"
	"fmt"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"tt/internal/config"
	"tt/internal/domain"
)

// The harness's today is Monday 12 Oct 2026.
func TestDueMarker(t *testing.T) {
	const today = domain.Day("2026-10-12")
	cases := map[domain.Day]string{
		"":           "",
		"garbage":    "",
		"2026-10-12": "→today",
		"2026-10-13": "→tue",
		"2026-10-16": "→fri",
		"2026-10-18": "→sun",
		"2026-10-19": "→19 oct",
		"2027-01-03": "→3 jan",
		"2027-10-12": "→12 oct",
		"2027-11-15": "→15 nov 2027",
		"2026-10-11": "1d late",
		"2026-10-02": "10d late",
	}
	for due, want := range cases {
		if got := ansi.Strip(dueMarker(today, due)); got != want {
			t.Errorf("dueMarker(%s) = %q; want %q", due, got, want)
		}
	}
}

func TestRowsShowTheDueAndUrgency(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.add("Undated stream task !3 ~xl")
	h.add("Ask due today @ana !1 ~s ^today")
	parent := h.add("Migration !2 ~l ^2026-10-30")
	if _, _, err := h.svc.AddChild(context.Background(), parent.ID, "Dry run ~s"); err != nil {
		t.Fatal(err)
	}
	h.add("Overdue ask !1 ~s ^2026-10-09")
	h.keys("R", "j", "j", "j", "right")
	// On the urgency sort (D72): impact 1 three days overdue 0.59, impact 1
	// due today 0.53, impact 3 undated 0.50, impact 2 due in 18 days 0.43.
	h.before("BACKLOG · by urgency",
		"[ 59 S] Overdue ask 3d late",
		"[ 53 S] Ask due today →today @ana",
		"[ 50 XL] Undated stream task",
		"[ 43 L] Migration →30 oct 0/1", "[ 43 S] Dry run →30 oct") // the child takes its parent's due (D69)
	h.mustNotSee("Undated stream task →")
}

func TestDetailShowsImpactDueAndUrgency(t *testing.T) {
	h := newLogHarness(t, config.Default())
	parent, kids := h.family("Migration !2 ~l ^fri", "Dry run ~s", "Cutover ~s")
	if _, err := h.svc.SetDue(context.Background(), kids[1].ID, "2026-10-13"); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "enter")
	// Impact 2 due in 4 days: 0.62; due tomorrow: 0.68.
	h.mustSee("impact: one team · due Fri 16 Oct · urgency 62", "[ 62 S] Dry run →fri", "[ 68 S] Cutover →tue")
	h.keys("j", "enter")
	h.mustSee("#2 Dry run", fmt.Sprintf("impact: one team (from #%d) · due Fri 16 Oct (from #%[1]d) · urgency 62", parent.ID))
	h.keys("esc", "j", "j", "enter")
	h.mustSee("#3 Cutover", fmt.Sprintf("impact: one team (from #%d) · due Tue 13 Oct · urgency 68", parent.ID))

	h.add("Untriaged thing ~s")
	h.keys("esc", "esc", "i", "enter")
	h.mustSee("set impact and size first · n skips")
}

func TestInboxShowsTheDueFirstAndWaitsForEnter(t *testing.T) {
	h := newLogHarness(t, config.Default())
	first := h.add("Ship it")
	h.add("Second")
	h.keys("R", "i")
	h.before("INBOX · 1 of 2", "Ship it", "due       — · ^ to set", "impact", "size") // D73
	h.keys("enter")
	h.mustSee("set impact and size first · n skips", "INBOX · 1 of 2", "Ship it")

	h.keys("^")
	h.mustSee("due › ")
	h.typeText("someday")
	h.keys("enter")
	h.mustSee(`✗ due "someday": want today, tom, a weekday like fri, 3d, 2w, 30oct or 2026-10-30`)
	h.keys("esc", "^")
	h.typeText("^fri")
	h.keys("enter")
	h.mustSee("due       Fri 16 Oct →fri · ^ to change")
	if got := h.task(first.ID); got.Due != "2026-10-16" {
		t.Errorf("due = %q; want Fri 16 Oct", got.Due)
	}

	h.keys("3", "s")
	h.mustSee("INBOX · 1 of 2", "Ship it", "triaged · ⏎ next card")
	h.keys("^") // still the same card: the due can change after triage
	h.mustSee("due › 2026-10-16")
	h.keys("esc", "enter")
	h.mustSee("INBOX · 1 of 1", "Second")
	h.keys("esc")
	h.mustSee("[ 79 S] Ship it →fri") // impact 3 due in 4 days

	// ctrl+z undoes the last triage step; the card is back in the Inbox.
	h.keys("i", "1", "m")
	h.mustSee("triaged · ⏎ next card")
	h.keys("ctrl+z")
	h.mustSee("INBOX · 1 of 1", "Second")
	h.mustNotSee("triaged · ⏎ next card")
}

func TestInboxChildCardShowsTheInheritedDue(t *testing.T) {
	h := newLogHarness(t, config.Default())
	parent := h.add("Migration !2 ~l ^fri")
	if _, _, err := h.svc.AddChild(context.Background(), parent.ID, "Dry run"); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "i")
	h.mustSee("INBOX · 1 of 1", "Dry run", "due       Fri 16 Oct from parent #1 · ^ to set its own", "from parent #1: one team")
	h.keys("enter")
	h.mustSee("set the size first · n skips")
	h.keys("^")
	h.typeText("tom")
	h.keys("enter", "s")
	h.mustSee("due       Tue 13 Oct →tue · ^ to change", "triaged · ⏎ next card")
	h.keys("enter")
	h.mustSee("Inbox zero ✓")
}

// Review Focus: a held card that is dropped (or closed elsewhere) leaves the
// screen at once; the next card shows.
func TestInboxHeldCardLeavesWhenDropped(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.add("Doomed")
	h.add("Next one")
	h.keys("R", "i", "2", "s")
	h.mustSee("INBOX · 1 of 2", "Doomed", "triaged · ⏎ next card")
	h.keys("d")
	h.mustSee("reason (enter to skip)")
	h.keys("enter")
	h.mustSee("INBOX · 1 of 1", "Next one", `dropped "Doomed"`)
	h.mustNotSee("triaged · ⏎ next card")
	h.keys("ctrl+z")
	h.mustNotSee("triaged · ⏎ next card")
	if got := h.task(1); got.Status != domain.StatusOpen {
		t.Fatalf("Doomed after undo: %v", got.Status)
	}
}

// n on a triaged (held) card moves on like ⏎, even when it was the only
// card: nothing is left to skip to.
func TestInboxSkipOnAHeldCardMovesOn(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.add("Only one")
	h.keys("R", "i", "1", "s")
	h.mustSee("INBOX · 1 of 1", "triaged · ⏎ next card")
	h.keys("n")
	h.mustSee("Inbox zero ✓")
}
