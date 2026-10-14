package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/app"
	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/store"
)

func TestQuickAddThenTriageInTwoKeystrokes(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.mustSee("tt · Log", "Mon 12 Oct", "nothing triaged yet")

	h.keys("a")
	h.typeText("Reply to Ana @ana #photos")
	h.keys("enter")
	h.mustSee("added #1 to inbox", "inbox 1")

	h.keys("i")
	h.mustSee("INBOX · 1 of 1", "Reply to Ana", "@ana · #photos", "[1] a person · me, one reviewer, one asker")
	h.keys("1", "s") // D11a: impact + size → leaves the inbox
	h.mustSee("Inbox zero ✓")
	h.keys("esc")
	h.mustSee("BACKLOG · by urgency", "[ 17 S] Reply to Ana @ana") // impact 1, no due: 17 (D71)
	if got := h.task(1); got.Impact != 1 || got.Size != domain.SizeS {
		t.Errorf("task after triage: %+v", got)
	}
}

func TestQuickAddDirectlyToBacklogAndEscCancels(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.keys("a")
	h.typeText("Ready !2 ~m")
	h.keys("enter")
	h.mustSee("added #1 to backlog", "[ 33 M] Ready")

	h.keys("a")
	h.typeText("never mind")
	h.keys("esc")
	h.mustNotSee("never mind")
	if snap, _ := h.svc.Snapshot(context.Background()); len(snap.Tasks()) != 1 {
		t.Errorf("esc must not add: %d tasks", len(snap.Tasks()))
	}

	h.keys("a")
	h.typeText("@only-tokens")
	h.keys("enter")
	if h.m.overlay == nil {
		t.Fatal("a failed quick-add must keep the overlay open")
	}
	h.mustSee("add › @only-tokens", "✗ title is required")
	h.keys("esc")
	if h.m.overlay != nil {
		t.Error("esc closes the overlay")
	}

	h.keys("a")
	h.typeText("Long capture @ana !7")
	h.keys("enter")
	if h.m.overlay == nil {
		t.Fatal("an out-of-range impact must keep the overlay open")
	}
	h.mustSee("add › Long capture @ana !7", "✗ impact !7 out of range")
	h.keys("esc")
	if snap, _ := h.svc.Snapshot(context.Background()); len(snap.Tasks()) != 1 {
		t.Errorf("failed adds must not add: %d tasks", len(snap.Tasks()))
	}
}

func TestQuickAddAutocomplete(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.add("one @ana #photos")
	h.add("two @ana")
	h.add("three @andrei")

	h.keys("a")
	h.typeText("x @an")
	h.mustSee("tab → @ana  @andrei") // most used first
	h.keys("tab")
	h.typeText("#ph")
	h.keys("tab")
	h.mustSee("add › x @ana #photos")
	h.keys("enter")
	if got := h.task(4); got.Requester != "ana" || len(got.Tags) != 1 || got.Tags[0] != "photos" {
		t.Errorf("autocompleted task: %+v", got)
	}
}

func TestPickPinDoneUndo(t *testing.T) {
	h := newLogHarness(t, config.Default())
	a := h.add("Alpha !3 ~s")
	h.add("Beta !2 ~m")
	h.mustSee("[ 50 S] Alpha", "[ 33 M] Beta", "nothing pinned")

	h.keys("p") // cursor starts on the first backlog task
	h.mustSee(`pinned "Alpha"`)
	if _, ok := h.m.idx.cand[a.ID]; !ok {
		t.Fatal("Alpha not pinned")
	}
	h.keys("tab", "x")
	h.mustSee(`✓ "Alpha"`, "Alpha ✓")
	h.keys("ctrl+z")
	h.mustSee(`undid done "Alpha"`)
	if got := h.task(a.ID); got.Status != domain.StatusOpen {
		t.Errorf("after undo: %s", got.Status)
	}
	h.keys("u")
	h.mustSee(`unpinned "Alpha"`, "nothing pinned")
	h.keys("ctrl+z", "ctrl+z")
	h.mustSee(`undid pin "Alpha"`)
	h.keys("ctrl+z")
	h.mustSee("nothing to undo")
}

func TestUndoThatConflictsSaysSo(t *testing.T) {
	h := newLogHarness(t, config.Default())
	a := h.add("Alpha !3 ~s")
	h.keys("p", "tab", "x")
	h.clock.now = h.clock.now.Add(time.Minute)
	if _, err := h.svc.SetNote(context.Background(), a.ID, "changed elsewhere"); err != nil {
		t.Fatal(err)
	}
	h.keys("ctrl+z")
	h.mustSee(`✗ can't undo done "Alpha": it changed since`)
	if got := h.task(a.ID); got.Status != domain.StatusDone {
		t.Errorf("refused undo changed the task: %s", got.Status)
	}
}

func TestDropWithReasonAndDeleteConfirm(t *testing.T) {
	h := newLogHarness(t, config.Default())
	a := h.add("Alpha !3 ~s")
	b := h.add("Beta !3 ~s")

	h.keys("d")
	h.typeText("handed to @marko")
	h.keys("enter")
	h.mustSee(`dropped "Alpha"`)
	if got := h.task(a.ID); got.Status != domain.StatusDropped || got.DropReason != "handed to @marko" {
		t.Errorf("dropped task: %+v", got)
	}

	h.keys("D")
	h.mustSee(`(y/n) Delete #2 "Beta"? This can't be undone.`)
	h.keys("n")
	h.mustSee("cancelled")
	h.task(b.ID) // still there
	h.keys("D", "y")
	h.mustSee("deleted #2")
	if snap, _ := h.svc.Snapshot(context.Background()); len(snap.Tasks()) != 1 {
		t.Errorf("after delete: %d tasks", len(snap.Tasks()))
	}
}

func TestBacklogParentExpandsAndTodayShowsChildren(t *testing.T) {
	cfg := config.Default()
	cfg.RolloverAlert = 1
	h := newLogHarness(t, cfg)
	ctx := context.Background()
	parent := h.add("Auth migration !3 ~xl")
	kid1 := h.add("Rollback plan ~s")
	kid2 := h.add("Token spike ~m")
	for _, k := range []int64{kid1.ID, kid2.ID} {
		if _, err := h.svc.SetParent(ctx, k, parent.ID); err != nil {
			t.Fatal(err)
		}
	}
	h.keys("R")
	h.mustSee("▸ [ 50 XL] Auth migration 0/2")
	h.mustNotSee("Rollback plan")
	h.keys("right")
	h.mustSee("▾ [ 50 XL] Auth migration", "    [ 50 S] Rollback plan", "    [ 50 M] Token spike")
	h.keys("down", "left") // collapsing from a child collapses its parent
	h.mustNotSee("Rollback plan")

	if _, _, err := h.svc.Block(ctx, parent.ID, 10*time.Hour, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Done(ctx, kid1.ID); err != nil {
		t.Fatal(err)
	}
	h.clock.now = h.clock.now.Add(24 * time.Hour) // parent rolls over once → alert at threshold 1
	h.send(clockTickMsg{})
	h.keys("esc") // dismiss the banner
	h.mustSee("▾ [ 50 XL] Auth migration 1/2 ↻1", "⚠ rolled over 1×", "    [ 50 M] Token spike")
	h.mustNotSee("Rollback plan") // closed yesterday, not today
}

func TestExternalWriteReloads(t *testing.T) {
	h := newLogHarness(t, config.Default())
	other, err := store.Open(context.Background(), h.path) // e.g. `tt add` from another shell
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	cli := app.New(other, h.clock, config.Default())
	if _, _, err := cli.Add(context.Background(), "From the CLI"); err != nil {
		t.Fatal(err)
	}
	h.mustNotSee("inbox 1")
	h.send(watchTickMsg{})
	h.mustSee("inbox 1")
}

func TestNewDayBannerStartsPlanning(t *testing.T) {
	h := newLogHarness(t, config.Default())
	a := h.add("Blocked !3 ~s")
	if _, _, err := h.svc.Block(context.Background(), a.ID, 10*time.Hour, time.Hour); err != nil {
		t.Fatal(err)
	}
	h.clock.now = h.clock.now.Add(24 * time.Hour)
	h.send(clockTickMsg{})
	h.mustSee("☀ New day Tue 13 Oct · since Mon 12 Oct: 0 done · 1 rolled over · 1 carried", "⏎ plan now")
	h.send(clockTickMsg{}) // same day again: no new banner, nothing changes
	h.keys("enter")
	h.mustNotSee("New day")
	h.mustSee("tt · Log") // empty inbox: straight to Pick

	h.add("Untriaged")
	h.clock.now = h.clock.now.Add(24 * time.Hour)
	h.send(clockTickMsg{})
	h.keys("enter")
	h.mustSee("tt · Inbox", "INBOX · 1 of 1") // D12d: planning starts with triage
}

func TestInboxSkipAndChildImpact(t *testing.T) {
	h := newLogHarness(t, config.Default())
	parent := h.add("Parent !3 ~xl")
	child := h.add("Child")
	h.add("Other")
	if _, err := h.svc.SetParent(context.Background(), child.ID, parent.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "i")
	h.mustSee("INBOX · 1 of 2", "Child", "from parent #1: more than one team")
	h.keys("2")
	h.mustSee("children inherit impact from #1")
	h.keys("n")
	h.mustSee("INBOX · 1 of 2", "Other") // the skipped card moved to the end
	h.keys("n")
	h.mustSee("Child") // everything skipped: start over
	h.keys("m")        // size alone triages a child (D74)
	h.mustSee("INBOX · 1 of 1", "Other")
	h.keys("9")
	h.mustSee("impact is 1 to 3")
}

func TestHelpAndQuit(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.keys("?")
	h.mustSee("KEYS", "p / u    pin to today / unpin", "any key closes")
	h.keys("x")
	h.mustNotSee("KEYS")
	h.keys("q")
	if !h.quit {
		t.Error("q should quit")
	}
}

func TestNarrowTerminalTruncatesInsteadOfWrapping(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.add("A rather long task title that will not fit in a narrow terminal @someone !3 ~s")
	h.keys("p")
	h.send(tea.WindowSizeMsg{Width: 40, Height: 12})
	for _, line := range strings.Split(h.m.render(), "\n") {
		if w := ansi.StringWidth(line); w > 40 {
			t.Errorf("line is %d cells wide (> 40): %q", w, ansi.Strip(line))
		}
	}
	h.mustSee("…")
}

func TestPasteIntoQuickAdd(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.keys("a")
	h.send(tea.PasteMsg{Content: "Pasted title https://x.io/a"})
	h.keys("enter")
	if got := h.task(1); got.Title != "Pasted title" || got.Link != "https://x.io/a" {
		t.Errorf("pasted task: title %q link %q", got.Title, got.Link)
	}
}

func TestCursorFollowsTaskAcrossReload(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.add("Low !2 ~s")
	if _, _, err := h.svc.Add(context.Background(), "High !3 ~s"); err != nil {
		t.Fatal(err)
	}
	h.keys("R") // High now sorts above Low; the cursor must stay on Low
	h.keys("p")
	h.mustSee(`pinned "Low"`)
}

func TestCollapseFromChildSelectsParent(t *testing.T) {
	h := newLogHarness(t, config.Default())
	ctx := context.Background()
	parent := h.add("Auth migration !3 ~xl")
	kid1 := h.add("Rollback plan ~s")
	kid2 := h.add("Token spike ~m")
	h.add("Later !3 ~s")
	for _, k := range []int64{kid1.ID, kid2.ID} {
		if _, err := h.svc.SetParent(ctx, k, parent.ID); err != nil {
			t.Fatal(err)
		}
	}
	h.keys("R", "right", "down", "left", "p")
	h.mustSee(`pinned "Auth migration"`)
}

func TestHelpFitsWithFullBacklog(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.send(tea.WindowSizeMsg{Width: 120, Height: 20})
	for n := 1; n <= 30; n++ {
		h.add(fmt.Sprintf("T%d !3 ~s", n))
	}
	h.keys("?")
	if lines := strings.Count(h.m.render(), "\n") + 1; lines > 20 {
		t.Errorf("help overlay renders %d lines in a 20-line terminal", lines)
	}
	h.mustSee("any key closes")
}

func TestErrorBeforeFirstLoadIsShown(t *testing.T) {
	h := newLogHarness(t, config.Default())
	opts := Options{Schedule: func(time.Duration, tea.Msg) tea.Cmd { return nil }}
	m := New(context.Background(), h.svc, opts)
	m.Update(dayStartedMsg{err: errors.New("boom")})
	if s := ansi.Strip(m.render()); !strings.Contains(s, "✗ boom") {
		t.Errorf("error before the first load is hidden:\n%s", s)
	}
	cmd := m.handleKey(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("q before the first load does nothing")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("q before the first load: got %T, want tea.QuitMsg", cmd())
	}
}

func TestChildProgressExcludesDropped(t *testing.T) {
	h := newLogHarness(t, config.Default())
	ctx := context.Background()
	parent := h.add("Parent !3 ~xl")
	done := h.add("Finished ~s")
	dropped := h.add("Abandoned ~s")
	open := h.add("Pending ~s")
	for _, k := range []int64{done.ID, dropped.ID, open.ID} {
		if _, err := h.svc.SetParent(ctx, k, parent.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.svc.Done(ctx, done.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Drop(ctx, dropped.ID, ""); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	h.mustSee("▸ [ 50 XL] Parent 1/2")
}

func TestOrphanedChildrenStayInBacklog(t *testing.T) {
	h := newLogHarness(t, config.Default())
	ctx := context.Background()
	parent := h.add("P !3 ~xl")
	kid := h.add("Kid ~s")
	if _, err := h.svc.SetParent(ctx, kid.ID, parent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Done(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	h.mustSee("[ 50 S] Kid", "↳ #1")

	// A parent still in the Inbox (no size) doesn't hide its children either.
	p2 := h.add("P2 !3")
	kid2 := h.add("Kid2 ~s")
	if _, err := h.svc.SetParent(ctx, kid2.ID, p2.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	h.mustSee("Kid2")
}

func TestChildOfPinnedParentNotListedInBacklog(t *testing.T) {
	h := newLogHarness(t, config.Default())
	ctx := context.Background()
	parent := h.add("Pinned parent !3 ~xl")
	kid := h.add("Under pin ~s")
	if _, err := h.svc.SetParent(ctx, kid.ID, parent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Pin(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	h.mustSee("Under pin")
	if n := strings.Count(h.screen(), "Under pin"); n != 1 {
		t.Errorf("child shown %d times, want once (under its pinned parent):\n%s", n, h.screen())
	}
}

func TestNewDayShownWithoutWaitingForTheClockTick(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.clock.now = h.clock.now.Add(24 * time.Hour)
	h.send(watchTickMsg{})
	h.mustSee("☀ New day Tue 13 Oct")
}

func TestKeyOnStaleDayRefreshesInstead(t *testing.T) {
	h := newLogHarness(t, config.Default())
	a := h.add("Alpha !3 ~s")
	h.keys("p")
	h.clock.now = h.clock.now.Add(24 * time.Hour)
	h.keys("u")
	h.mustSee("new day — refreshing", "☀ New day Tue 13 Oct")
	if _, ok := h.m.idx.cand[a.ID]; !ok {
		t.Error("the key's action must not run; Alpha stays a candidate")
	}
}

func TestDeleteConfirmKeepsHintOnNarrowScreens(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.add("An extremely long task title that goes on and on and on forever !3 ~s")
	h.send(tea.WindowSizeMsg{Width: 50, Height: 20})
	h.keys("D")
	h.mustSee("(y/n)")
	h.keys("n")
	h.add("Untriaged card with a very long title that goes on and on and on forever @someone #tag")
	h.keys("i")
	h.mustSee("INBOX")
	for _, line := range strings.Split(h.m.render(), "\n") {
		if w := ansi.StringWidth(line); w > 50 {
			t.Errorf("line is %d cells wide (> 50): %q", w, ansi.Strip(line))
		}
	}
}

func TestRolloverMarkerShownInBacklog(t *testing.T) {
	h := newLogHarness(t, config.Default())
	a := h.add("Slipper !3 ~s")
	h.keys("p")
	if _, _, err := h.svc.Block(context.Background(), a.ID, 10*time.Hour, time.Hour); err != nil {
		t.Fatal(err)
	}
	h.clock.now = h.clock.now.Add(24 * time.Hour)
	h.send(clockTickMsg{})
	h.keys("esc", "tab", "u")
	h.mustSee("[ 50 S] Slipper", "↻1")
	if _, ok := h.m.idx.cand[a.ID]; ok {
		t.Fatal("Slipper should be unpinned")
	}
}

func TestPinnedChildGetsAlertLine(t *testing.T) {
	cfg := config.Default()
	cfg.RolloverAlert = 1
	h := newLogHarness(t, cfg)
	ctx := context.Background()
	parent := h.add("Par !3 ~xl")
	kid := h.add("Kid ~s")
	if _, err := h.svc.SetParent(ctx, kid.ID, parent.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{parent.ID, kid.ID} {
		if _, err := h.svc.Pin(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := h.svc.Block(ctx, kid.ID, 10*time.Hour, time.Hour); err != nil {
		t.Fatal(err)
	}
	h.clock.now = h.clock.now.Add(24 * time.Hour)
	h.send(clockTickMsg{})
	h.keys("esc")
	h.mustSee("⚠ rolled over 1×")
}

func TestPinningClosedTaskExplainsItself(t *testing.T) {
	h := newLogHarness(t, config.Default())
	ctx := context.Background()
	parent := h.add("Par !3 ~xl")
	kid := h.add("Kid ~s")
	if _, err := h.svc.SetParent(ctx, kid.ID, parent.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{parent.ID, kid.ID} {
		if _, err := h.svc.Pin(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.svc.Done(ctx, kid.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "tab", "down", "p")
	h.mustSee("can't pin a closed task — x reopens it")
}
