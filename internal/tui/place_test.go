package tui

import (
	"context"
	"testing"
	"time"

	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/store"
)

// pinned adds a task from line and pins it.
func (h *harness) pinned(line string) domain.Task {
	h.t.Helper()
	t := h.add(line)
	if _, err := h.svc.Pin(context.Background(), t.ID); err != nil {
		h.t.Fatal(err)
	}
	h.keys("R")
	return t
}

func (h *harness) blocks() []domain.Block {
	h.t.Helper()
	plan, err := h.svc.Plan(context.Background())
	if err != nil {
		h.t.Fatal(err)
	}
	return plan.Blocks
}

func TestPlaceACandidate(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.pinned("Alpha !1 ~s")
	h.keys("tab", "b")
	if h.m.today.focus != focusTimeline || h.m.today.at != 9*time.Hour {
		t.Fatalf("b moves to the timeline's first free row from now: focus %d at %v", h.m.today.focus, h.m.today.at)
	}
	h.mustSee(`placing "Alpha" · j/k move · ⏎ drop · esc cancel`, " 09:00 ░ Alpha 1h", " 09:30 ░")
	h.keys("j")
	h.mustSee(" 09:30 ░ Alpha 1h", " 10:00 ░")
	h.keys("enter")
	h.mustSee(`blocked "Alpha" 09:30–10:30`, " 09:30 ▓ Alpha [P1 S]")
	if b := h.blocks(); len(b) != 1 || b[0].TaskID != a.ID || b[0].Start != 9*time.Hour+30*time.Minute || b[0].Duration != time.Hour {
		t.Fatalf("blocks = %+v", b)
	}
	h.keys("ctrl+z")
	h.mustSee(`undid block "Alpha"`)
	if b := h.blocks(); len(b) != 0 {
		t.Errorf("after undo: %+v", b)
	}
}

func TestPlacementLengthAndFreeRows(t *testing.T) {
	h := newHarness(t, config.Default())
	busy := h.pinned("Busy !1 ~s")
	h.block(busy.ID, 9*time.Hour, time.Hour)
	later := h.pinned("Later !1 ~s")
	h.block(later.ID, 11*time.Hour, time.Hour)
	h.pinned("Medium !2 ~m")
	h.pinned("Unsized !3")

	h.keys("tab", "j", "j", "b") // Medium (candidates in plan order: Busy, Later, Medium, Unsized)
	if h.m.today.at != 10*time.Hour {
		t.Fatalf("the first free row from now is 10:00, got %v", h.m.today.at)
	}
	h.mustSee(" 10:00 ░ Medium 1h") // D24: min(4h, the hour before Later)
	h.keys("enter")
	h.mustSee(`blocked "Medium" 10:00–11:00`)

	h.keys("tab", "j", "j", "j", "b")
	h.mustSee(" 12:00 ░ Unsized 30m") // unsized tasks default to 30m
	h.keys("esc")
	h.mustSee("cancelled")
	if h.m.today.carry != nil {
		t.Error("esc cancels the pick-up")
	}
}

func TestDropOntoABlockIsRefused(t *testing.T) {
	h := newHarness(t, config.Default())
	busy := h.pinned("Busy !1 ~s")
	h.block(busy.ID, 10*time.Hour, time.Hour)
	h.pinned("Alpha !1 ~s")
	h.keys("tab", "j", "b") // at 09:00: Alpha would need 09:00–10:00, which fits
	h.keys("j", "j")        // 10:00 is inside Busy
	h.keys("enter")
	h.mustSee(`✗ overlaps "Busy" 10:00–11:00`)
	if h.m.today.carry == nil {
		t.Fatal("a refused drop keeps the block picked up")
	}
	h.keys("j", "j", "enter") // 11:00
	h.mustSee(`blocked "Alpha" 11:00–12:00`)
}

func TestPlanningNudges(t *testing.T) {
	h := newHarness(t, config.Default())
	h.pinned("Large !1 ~l")
	h.keys("tab", "b", "enter")
	h.mustSee(`blocked "Large" 09:00–17:00 · big task · consider splitting`) // D37

	ctx := context.Background()
	parent := h.pinned("Parent !1 ~xl")
	if _, _, err := h.svc.AddChild(ctx, parent.ID, "Child ~s"); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "tab", "j", "b")
	h.mustSee("parent task; blocking its children is recommended · ⏎ block anyway · esc")
	h.keys("esc")
	h.mustSee("cancelled")
	if h.m.today.carry != nil {
		t.Fatal("esc on the nudge picks nothing up")
	}
	h.keys("b", "enter")
	if c := h.m.today.carry; c == nil || c.task.ID != parent.ID {
		t.Fatalf("⏎ on the nudge picks the parent up: %+v", c)
	}
	h.mustSee(" 17:00 ░ Parent") // the first free row after Large
	h.keys("enter")
	h.mustSee(`blocked "Parent" 17:00–19:00 · big task · consider splitting`) // XL: the rest of the working hours (D39)

	done := h.pinned("Done already !2 ~s")
	if _, err := h.svc.Done(ctx, done.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "tab")
	for range 5 {
		h.keys("j")
	}
	h.keys("b")
	h.mustSee("can't block a closed task — x reopens it")
}

func TestMoveResizeFineTuneUnblock(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.pinned("Alpha !1 ~s")
	h.block(a.ID, 10*time.Hour, time.Hour)
	other := h.pinned("Other !1 ~s")
	h.block(other.ID, 13*time.Hour, time.Hour)

	h.keys("j", "j", "m") // on Alpha at 10:00
	h.mustSee(`moving "Alpha" · j/k move · ⏎ drop · esc cancel`)
	h.keys("j", "j", "enter")
	h.mustSee(`moved "Alpha" to 11:00–12:00`)
	if b := h.blocks()[0]; b.Start != 11*time.Hour || b.Duration != time.Hour {
		t.Fatalf("moved block: %+v", b)
	}
	h.keys("ctrl+z")
	h.mustSee(`undid move "Alpha"`)

	h.keys("k", "k", "+") // back on 10:00, where the undo put Alpha
	h.mustSee(`"Alpha" now 10:00–11:30`)
	h.keys("j", "j", "+") // 11:00, the last row of Alpha's block, acts on it too
	h.mustSee(`"Alpha" now 10:00–12:00`)
	h.keys("k", "k", "-", "-", "-")
	h.mustSee(`"Alpha" now 10:00–10:30`)
	h.keys("-")
	h.mustSee("can't be shorter")

	h.keys("f")
	h.mustSee(`fine-tune "Alpha" › 10:00 30m`)
	for range 10 {
		h.keys("backspace")
	}
	h.typeText("12:30 45m")
	h.keys("enter")
	h.mustSee(`✗ overlaps "Other" 13:00–14:00`)
	if h.m.overlay == nil {
		t.Fatal("a refused fine-tune keeps the line")
	}
	for range 10 {
		h.keys("backspace")
	}
	h.typeText("10:10 45m")
	h.keys("enter")
	h.mustSee(`"Alpha" now 10:10–10:55`, " 10:00 ▓ 10:10–10:55 Alpha [P1 S]")
	h.keys("ctrl+z")
	if b := h.blocks()[0]; b.Start != 10*time.Hour || b.Duration != 30*time.Minute {
		t.Errorf("fine-tune undoes in one step: %+v", b)
	}

	h.keys("u")
	h.mustSee(`unblocked "Alpha" · still a candidate`)
	if len(h.blocks()) != 1 {
		t.Error("u removes Alpha's block only")
	}
	if _, ok := h.m.idx.cand[a.ID]; !ok {
		t.Error("an unblocked task stays a candidate")
	}
}

func TestAMovedBlockThatVanishesStopsTheMove(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.pinned("Alpha !1 ~s")
	b := h.block(a.ID, 10*time.Hour, time.Hour)
	h.keys("j", "j", "m")
	if _, err := h.svc.Unblock(context.Background(), b.ID); err != nil { // e.g. from another tt
		t.Fatal(err)
	}
	h.keys("R")
	h.mustSee(`stopped moving "Alpha": its block is gone`)
	if h.m.today.carry != nil {
		t.Error("the move stops")
	}
}

func TestCommitPlan(t *testing.T) {
	h := newHarness(t, config.Default())
	h.pinned("Waiting !2 ~s")
	h.keys("c")
	h.mustSee("plan committed · no blocks · 1 candidate left", "· plan committed")

	a := h.pinned("Alpha !1 ~s")
	h.block(a.ID, 10*time.Hour, 90*time.Minute)
	h.keys("tab", "c")
	h.mustSee("plan committed · 1 block · 1h30m · 1 candidate left")
}

func TestUnpinningABlockedParentIsRefusedInTheTUI(t *testing.T) {
	h := newHarness(t, config.Default())
	ctx := context.Background()
	parent := h.pinned("Parent !1 ~xl")
	if _, _, err := h.svc.AddChild(ctx, parent.ID, "Child ~s"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Pin(ctx, parent.ID); err != nil { // pins the child too
		t.Fatal(err)
	}
	h.block(parent.ID, 10*time.Hour, time.Hour)
	h.keys("tab", "u") // on the parent, the first candidate
	h.mustSee(`"Parent" is blocked today · unblock it on the timeline first`)
	if len(h.blocks()) != 1 {
		t.Error("the parent's block must stay")
	}
	if _, ok := h.m.idx.cand[parent.ID]; !ok {
		t.Error("the parent stays pinned")
	}
}

// unpinDirectly removes a pin behind the service's back: real data has
// parents pinned before D34 whose children have no pin.
func (h *harness) unpinDirectly(id int64) {
	h.t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, h.path)
	if err != nil {
		h.t.Fatal(err)
	}
	defer st.Close()
	if err := st.InTx(ctx, func(x *store.Tx) error { return x.DeletePin(ctx, id) }); err != nil {
		h.t.Fatal(err)
	}
	h.keys("R")
}

func TestPinRepairsAParentPinnedBeforeD34(t *testing.T) {
	h := newHarness(t, config.Default())
	ctx := context.Background()
	parent := h.pinned("Parent !1 ~xl")
	kid, _, err := h.svc.AddChild(ctx, parent.ID, "Child ~s")
	if err != nil {
		t.Fatal(err)
	}
	h.unpinDirectly(kid.ID)
	if _, ok := h.m.idx.cand[kid.ID]; ok {
		t.Fatal("setup: the child is still pinned")
	}

	h.keys("tab", "j", "u") // the legacy child, shown under its pinned parent
	h.mustSee("pinned with its parent #1 · unpin the parent first")

	h.keys("k", "L", "tab") // the Log's today pane, cursor on the parent
	h.keys("p")
	h.mustSee(`pinned "Parent" + 1 child · no undo, u unpins`)
	if _, ok := h.m.idx.cand[kid.ID]; !ok {
		t.Error("p on the parent pins its unpinned child")
	}
	h.keys("p")
	h.mustSee("already pinned")
}

func TestBlockingOutsideWorkingHours(t *testing.T) {
	h := newHarness(t, config.Default()) // working hours 08:00–19:00 (D39)
	late := h.pinned("Late !1 ~s")
	h.clock.now = time.Date(2026, 10, 13, 0, 20, 0, 0, time.UTC) // past midnight: still the 12th's planning day
	h.m.today.placed = false                                     // as when tt starts now
	h.keys("R")
	h.mustSee("TIMELINE · outside of working hours", " 18:30 ")
	h.mustNotSee("━━ now", " 19:00 ", " 00:00 ")
	if h.m.today.at != 18*time.Hour+30*time.Minute {
		t.Errorf("after hours the cursor rests on the last working row, got %v", h.m.today.at)
	}

	h.keys("tab", "b") // placing in the past is allowed (D36): from the first free row
	for range 30 {
		h.keys("j")
	}
	if h.m.today.at != 18*time.Hour+30*time.Minute {
		t.Fatalf("j stops at the last working row, got %v", h.m.today.at)
	}
	h.mustSee(" 18:30 ░ Late 30m") // D24: capped by the end of the hours
	h.keys("enter")
	h.mustSee(`blocked "Late" 18:30–19:00`)

	h.keys("+")
	h.mustSee("✗ outside of working hours (08:00–19:00)")
	h.keys("f")
	for range 10 {
		h.keys("backspace")
	}
	h.typeText("22:00 1h")
	h.keys("enter")
	h.mustSee("✗ outside of working hours (08:00–19:00)")
	h.keys("esc")
	if b := h.blocks(); len(b) != 1 || b[0].TaskID != late.ID || b[0].Start != 18*time.Hour+30*time.Minute {
		t.Errorf("refused changes change nothing: %+v", b)
	}
}

func TestMovingAnOffGridBlockKeepsItsStart(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.pinned("Alpha !1 ~s")
	h.block(a.ID, 10*time.Hour+10*time.Minute, 45*time.Minute)
	h.keys("j", "j", "m", "enter") // m ⏎ leaves it where it was
	h.mustSee(`moved "Alpha" to 10:10–10:55`)
	h.keys("m", "j", "enter")
	h.mustSee(`moved "Alpha" to 10:40–11:25`)
}

func TestParseSpan(t *testing.T) {
	const dayStart = 4 * time.Hour
	good := []struct {
		in         string
		start, dur time.Duration
	}{
		{"09:30 45m", 9*time.Hour + 30*time.Minute, 45 * time.Minute},
		{"9:05 1h30m", 9*time.Hour + 5*time.Minute, 90 * time.Minute},
		{"00:30 1h", 24*time.Hour + 30*time.Minute, time.Hour}, // after midnight, same planning day
	}
	for _, c := range good {
		start, dur, err := parseSpan(c.in, dayStart)
		if err != nil || start != c.start || dur != c.dur {
			t.Errorf("parseSpan(%q) = %v %v %v; want %v %v", c.in, start, dur, err, c.start, c.dur)
		}
	}
	for _, in := range []string{"", "09:30", "09:30 45m extra", "9.30 45m", "09:30 0m", "09:30 90s", "09:30 soon", "25:00 1h"} {
		if _, _, err := parseSpan(in, dayStart); err == nil {
			t.Errorf("parseSpan(%q) accepted", in)
		}
	}
}
