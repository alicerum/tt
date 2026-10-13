package tui

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/config"
)

func TestReslotAConflictedBlock(t *testing.T) {
	h := newCalHarness(t, config.Default())
	h.meetings(ev("Review", 10*time.Hour, 11*time.Hour), ev("Lunch", 12*time.Hour, 13*time.Hour))
	a := h.pinned("Alpha !1 ~s")
	h.block(a.ID, 10*time.Hour, time.Hour)
	b := h.pinned("Beta !1 ~s")
	h.block(b.ID, 11*time.Hour, time.Hour)

	h.keys("j", "j", "j", "j") // 11:00: Beta, no conflict
	h.keys("r")
	h.mustSee("no conflict · m moves it")
	if h.m.today.carry != nil {
		t.Fatal("r on a block without a conflict picks nothing up")
	}

	h.keys("k", "k") // 10:00: Alpha over Review
	h.mustSee("r re-slot ⚠ · j/k move")
	h.keys("r")
	// D43: the first slot after Alpha's that fits 1h: not 10:30 (Review), 11:00
	// or 11:30 (Beta), 12:00 or 12:30 (Lunch).
	h.mustSee(`moving "Alpha"`, " 13:00 ░ Alpha 1h")
	h.keys("j") // j/k adjust
	h.mustSee(" 13:30 ░ Alpha 1h")
	h.keys("k", "enter")
	h.mustSee(`moved "Alpha" to 13:00–14:00`)
	h.mustNotSee("⚠")
	h.keys("ctrl+z")
	h.mustSee(`undid move "Alpha"`, " 10:00 ▓ Alpha [P1 S] ⚠")

	h.keys("r", "esc") // esc leaves it where it is
	if bs := h.blocks(); bs[0].Start != 10*time.Hour || h.m.today.carry != nil {
		t.Errorf("esc after r: %+v, carry %v", bs[0], h.m.today.carry)
	}
}

func TestReslotWithNoFreeSlot(t *testing.T) {
	h := newCalHarness(t, config.Default())
	h.meetings(ev("All hands", 14*time.Hour, 14*time.Hour+30*time.Minute))
	long := h.pinned("Long !1 ~m")
	h.block(long.ID, 14*time.Hour, 5*time.Hour)              // 14:00–19:00: nothing later fits 5h before 19:00
	h.keys("j", "j", "j", "j", "j", "j", "j", "j", "j", "j") // 14:00
	h.keys("r")
	h.mustSee("no free slot left today fits 5h", `moving "Long"`, " 14:00 ░ Long 5h ⚠")
	undos := len(h.m.undo)
	h.keys("enter") // the ghost stayed on the block
	h.mustSee("unchanged")
	if len(h.m.undo) != undos {
		t.Error("an unchanged drop pushes no undo")
	}
}

func TestARefusedDropKeepsThePickUp(t *testing.T) {
	h := newHarness(t, config.Default())
	other := h.pinned("Other !1 ~s")
	h.pinned("Alpha !1 ~s")
	h.keys("tab", "j", "b") // Alpha's ghost at 09:00
	// Meanwhile another tt blocks Other at 09:00; this one hasn't reloaded yet.
	if _, _, err := h.svc.Block(context.Background(), other.ID, 9*time.Hour, time.Hour); err != nil {
		t.Fatal(err)
	}
	h.keys("enter")
	h.mustSee(": overlaps another block") // the service refuses
	if c := h.m.today.carry; c == nil || c.task.Title != "Alpha" || h.m.today.at != 9*time.Hour {
		t.Fatalf("a refused drop gives the pick-up back where it was: %+v at %v", c, h.m.today.at)
	}
	h.mustSee(" 09:00 ░ Alpha 1h · Other [P1 S]") // the reload shows why
	h.keys("j", "j", "enter")
	h.mustSee(`blocked "Alpha" 10:00–11:00`)
}

func TestFineTuneRereadsTheBlock(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, config.Default())
	a := h.pinned("Alpha !1 ~s")
	b := h.block(a.ID, 10*time.Hour, time.Hour)
	h.keys("j", "j", "f")
	undos := len(h.m.undo)
	h.keys("enter")
	h.mustSee("unchanged")
	if len(h.m.undo) != undos {
		t.Error("an unchanged fine-tune pushes no undo")
	}

	h.keys("f")
	if _, err := h.svc.MoveBlock(ctx, b.ID, 13*time.Hour); err != nil { // another tt
		t.Fatal(err)
	}
	h.run(h.m.load()) // the watch loop reloads while the line is open
	for range 10 {
		h.keys("backspace")
	}
	h.typeText("10:00 30m")
	h.keys("enter")
	if bs := h.blocks(); len(bs) != 1 || bs[0].Start != 10*time.Hour || bs[0].Duration != 30*time.Minute {
		t.Fatalf("the typed span applies to the block as it is now: %+v", bs)
	}

	h.keys("f")
	if _, err := h.svc.Unblock(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	h.run(h.m.load())
	h.keys("enter")
	h.mustSee(`stopped fine-tuning "Alpha": its block is gone`)
}

func TestScrollKeepsTheCursorWhenRowsAppearAbove(t *testing.T) {
	h := newHarness(t, config.Default())
	early := h.pinned("Early !1 ~s")
	h.send(tea.WindowSizeMsg{Width: 100, Height: 12})
	for range 10 {
		h.keys("j") // 14:00, scrolled
	}
	h.keys("k", "k", "k") // 12:30, inside the window
	line := func() int {
		return slices.IndexFunc(strings.Split(h.screen(), "\n"), func(l string) bool { return strings.HasPrefix(l, " 12:30") })
	}
	before := line()
	if before < 0 || h.m.today.offset == 0 {
		t.Fatalf("the timeline should have scrolled to 12:30 (line %d, offset %d)", before, h.m.today.offset)
	}
	h.blockDirectly(early.ID, 6*time.Hour, time.Hour) // a row of its own appears above the hours
	h.keys("R")
	if got := line(); got != before || h.m.today.at != 12*time.Hour+30*time.Minute {
		t.Errorf("12:30 moved from line %d to %d (cursor at %v)", before, got, h.m.today.at)
	}
}
