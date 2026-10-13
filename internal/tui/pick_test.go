package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/config"
)

func TestGroupingCyclesAndIsRemembered(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.add("Small one !1 ~s @ana")
	h.add("Medium one !2 ~m")
	h.mustSee("BACKLOG · by priority", "P1 · someone is blocked", "P2 ·")

	h.keys("g")
	h.mustSee("grouped by size", "BACKLOG · by size", "\n│ S ", "[P1 S] Small one", "\n│ M ")
	h.mustNotSee("P1 · someone is blocked")
	h.keys("g")
	h.mustSee("BACKLOG · by created", "this week")
	h.keys("g")
	h.mustSee("BACKLOG · by requester", "@ana", "no requester")
	if s := h.screen(); strings.Index(s, "\n│ @ana ") > strings.Index(s, "no requester") {
		t.Errorf("no-requester group comes last:\n%s", s)
	}

	// A new session starts with the remembered grouping (D26).
	h2 := &harness{t: t, svc: h.svc, clock: h.clock, path: h.path,
		m: New(context.Background(), h.svc, h.m.opts)}
	h2.run(h2.m.Init())
	h2.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	h2.keys("L") // a new session opens on Today
	h2.mustSee("BACKLOG · by requester")

	h.keys("g")
	h.mustSee("BACKLOG · by priority")
}

func TestUnknownRememberedGroupingFallsBackToPriority(t *testing.T) {
	h := newLogHarness(t, config.Default())
	if err := h.svc.SetUIState(context.Background(), groupKey, "colour"); err != nil {
		t.Fatal(err)
	}
	h2 := &harness{t: t, svc: h.svc, clock: h.clock, path: h.path,
		m: New(context.Background(), h.svc, h.m.opts)}
	h2.run(h2.m.Init())
	h2.keys("L")
	h2.mustSee("BACKLOG · by priority")
}

func TestFilteredBacklogPlacement(t *testing.T) {
	h := newLogHarness(t, config.Default())
	ctx := context.Background()
	parent := h.add("Auth migration !1 ~xl #auth")
	for _, line := range []string{"Rollback plan ~s", "Token spike ~m"} {
		kid := h.add(line)
		if _, err := h.svc.SetParent(ctx, kid.ID, parent.ID); err != nil {
			t.Fatal(err)
		}
	}
	h.add("Schema idea")                   // Inbox
	pinned := h.add("Schema review !2 ~s") // pinned below
	h.add("Schema backlog !3 ~m")
	done := h.add("Schema shipped !1 ~s")
	if _, err := h.svc.Pin(ctx, pinned.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Done(ctx, done.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	h.mustNotSee("Schema idea") // unfiltered: Inbox and pinned tasks aren't backlog

	// D26c: a filter searches the backlog only, never the Inbox or Today's candidates.
	if err := h.m.applyFilter("schema"); err != nil {
		t.Fatal(err)
	}
	h.mustSee("BACKLOG · by priority · 1 match · schema", "[P3 M] Schema backlog")
	h.mustNotSee("Schema idea", "Schema shipped", "unset")

	// is: widens it to closed backlog tasks (D26b).
	if err := h.m.applyFilter("schema is:all"); err != nil {
		t.Fatal(err)
	}
	h.mustSee("2 matches", "Schema shipped ✓")
	h.mustNotSee("Schema idea")

	// A matching child whose parent doesn't match is listed top-level.
	if err := h.m.applyFilter("rollback"); err != nil {
		t.Fatal(err)
	}
	h.mustSee("1 match", fmt.Sprintf("[P1 S] Rollback plan ↳ #%d", parent.ID))

	// A matching parent lists its matching children beneath it, expanded.
	if err := h.m.applyFilter("#auth"); err != nil {
		t.Fatal(err)
	}
	h.mustSee("3 matches", "▾ [P1 XL] Auth migration 0/2", "    [P1 S] Rollback plan", "    [P1 M] Token spike")
	if err := h.m.applyFilter("#auth spike"); err != nil {
		t.Fatal(err)
	}
	h.mustSee("1 match", fmt.Sprintf("[P1 M] Token spike ↳ #%d", parent.ID))

	// A bad filter keeps the current one.
	if err := h.m.applyFilter("!9"); err == nil {
		t.Fatal("!9 must be refused")
	}
	h.mustSee("1 match · #auth spike")

	if err := h.m.applyFilter("zzz"); err != nil {
		t.Fatal(err)
	}
	h.mustSee("0 matches · zzz", "no matches · esc clears")
	h.keys("esc")
	h.mustSee("filter cleared", "▸ [P1 XL] Auth migration 0/2")
	h.mustNotSee("matches", "Schema idea")

	// Children of a pinned parent live under it in Today, filtered or not.
	if _, err := h.svc.Pin(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	if err := h.m.applyFilter("rollback"); err != nil {
		t.Fatal(err)
	}
	h.mustSee("0 matches · rollback", "    [P1 S] Rollback plan")
}

func TestScrollWindowMovesOnlyWhenTheCursorLeavesIt(t *testing.T) {
	h := newLogHarness(t, config.Default())
	for i := 1; i <= 10; i++ {
		h.add(fmt.Sprintf("Task %02d !1 ~s", i))
	}
	h.send(tea.WindowSizeMsg{Width: 140, Height: 11}) // 6 rows per framed pane
	for range 9 {
		h.keys("j")
	}
	h.mustSee("Task 05", "Task 10")
	h.mustNotSee("Task 04", "P1 · someone")
	h.keys("k") // the cursor moves up inside the window; the window stays
	h.mustSee("Task 05", "Task 10")
	h.mustNotSee("Task 04")
	for range 5 {
		h.keys("k")
	}
	h.mustSee("Task 04", "Task 09") // left the window at the top: it scrolls by one
	h.mustNotSee("Task 10")
	for range 3 {
		h.keys("k")
	}
	h.mustSee("P1 · someone is blocked", "Task 01") // the heading above the first task comes along
}

func TestAllDroppedChildrenHideProgress(t *testing.T) {
	h := newLogHarness(t, config.Default())
	ctx := context.Background()
	parent := h.add("Parent !1 ~l")
	kid := h.add("Kid ~s")
	if _, err := h.svc.SetParent(ctx, kid.ID, parent.ID); err != nil {
		t.Fatal(err)
	}
	h.clock.now = h.clock.now.Add(time.Minute)
	if _, err := h.svc.Drop(ctx, kid.ID, ""); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	h.mustSee("[P1 L] Parent")
	h.mustNotSee("0/0")
}
