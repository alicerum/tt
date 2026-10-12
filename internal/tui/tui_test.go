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
	h := newHarness(t, config.Default())
	h.mustSee("tt · Pick", "Mon 12 Oct", "nothing triaged yet")

	h.keys("a")
	h.typeText("Reply to Ana @ana #photos")
	h.keys("enter")
	h.mustSee("added #1 to inbox", "inbox 1")

	h.keys("i")
	h.mustSee("INBOX · 1 of 1", "Reply to Ana", "@ana · #photos", "[1] P1 · someone is blocked")
	h.keys("1", "s") // D11a: priority + size → leaves the inbox
	h.mustSee("Inbox zero ✓")
	h.keys("esc")
	h.mustSee("P1 · someone is blocked", "[P1 S] Reply to Ana @ana")
	if got := h.task(1); got.Priority != 1 || got.Size != domain.SizeS {
		t.Errorf("task after triage: %+v", got)
	}
}

func TestQuickAddDirectlyToBacklogAndEscCancels(t *testing.T) {
	h := newHarness(t, config.Default())
	h.keys("a")
	h.typeText("Ready !2 ~m")
	h.keys("enter")
	h.mustSee("added #1 to backlog", "[P2 M] Ready")

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
	h.mustSee("✗ title is required")
}

func TestQuickAddAutocomplete(t *testing.T) {
	h := newHarness(t, config.Default())
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
	h := newHarness(t, config.Default())
	a := h.add("Alpha !1 ~s")
	h.add("Beta !2 ~m")
	h.mustSee("[P1 S] Alpha", "[P2 M] Beta", "nothing pinned")

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
	h := newHarness(t, config.Default())
	a := h.add("Alpha !1 ~s")
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
	h := newHarness(t, config.Default())
	a := h.add("Alpha !1 ~s")
	b := h.add("Beta !1 ~s")

	h.keys("d")
	h.typeText("handed to @marko")
	h.keys("enter")
	h.mustSee(`dropped "Alpha"`)
	if got := h.task(a.ID); got.Status != domain.StatusDropped || got.DropReason != "handed to @marko" {
		t.Errorf("dropped task: %+v", got)
	}

	h.keys("D")
	h.mustSee(`Delete #2 "Beta"? This can't be undone. (y/n)`)
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
	h := newHarness(t, cfg)
	ctx := context.Background()
	parent := h.add("Auth migration !1 ~xl")
	kid1 := h.add("Rollback plan ~s")
	kid2 := h.add("Token spike ~m")
	for _, k := range []int64{kid1.ID, kid2.ID} {
		if _, err := h.svc.SetParent(ctx, k, parent.ID); err != nil {
			t.Fatal(err)
		}
	}
	h.keys("R")
	h.mustSee("▸ [P1 XL] Auth migration 0/2")
	h.mustNotSee("Rollback plan")
	h.keys("right")
	h.mustSee("▾ [P1 XL] Auth migration", "    [P1 S] Rollback plan", "    [P1 M] Token spike")
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
	h.mustSee("▾ [P1 XL] Auth migration 1/2 ↻1", "⚠ rolled over 1×", "    [P1 M] Token spike")
	h.mustNotSee("Rollback plan") // closed yesterday, not today
}

func TestExternalWriteReloads(t *testing.T) {
	h := newHarness(t, config.Default())
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
	h := newHarness(t, config.Default())
	a := h.add("Blocked !1 ~s")
	if _, _, err := h.svc.Block(context.Background(), a.ID, 10*time.Hour, time.Hour); err != nil {
		t.Fatal(err)
	}
	h.clock.now = h.clock.now.Add(24 * time.Hour)
	h.send(clockTickMsg{})
	h.mustSee("☀ New day Tue 13 Oct · since Mon 12 Oct: 0 done · 1 rolled over · 1 carried", "⏎ plan now")
	h.send(clockTickMsg{}) // same day again: no new banner, nothing changes
	h.keys("enter")
	h.mustNotSee("New day")
	h.mustSee("tt · Pick") // empty inbox: straight to Pick

	h.add("Untriaged")
	h.clock.now = h.clock.now.Add(24 * time.Hour)
	h.send(clockTickMsg{})
	h.keys("enter")
	h.mustSee("tt · Inbox", "INBOX · 1 of 1") // D12d: planning starts with triage
}

func TestInboxSkipAndChildPriority(t *testing.T) {
	h := newHarness(t, config.Default())
	parent := h.add("Parent !1 ~xl")
	child := h.add("Child")
	h.add("Other")
	if _, err := h.svc.SetParent(context.Background(), child.ID, parent.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "i")
	h.mustSee("INBOX · 1 of 2", "Child", "from parent: [P1 XL]")
	h.keys("2")
	h.mustSee(`priority comes from parent #1 "Parent"`)
	h.keys("n")
	h.mustSee("INBOX · 1 of 2", "Other") // the skipped card moved to the end
	h.keys("n")
	h.mustSee("Child") // everything skipped: start over
	h.keys("m")        // size alone triages a child of a prioritized parent
	h.mustSee("INBOX · 1 of 1", "Other")
	h.keys("9")
	h.mustSee("there are 3 priority levels")
}

func TestHelpAndQuit(t *testing.T) {
	h := newHarness(t, config.Default())
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
	h := newHarness(t, config.Default())
	h.add("A rather long task title that will not fit in a narrow terminal @someone !1 ~s")
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
	h := newHarness(t, config.Default())
	h.keys("a")
	h.send(tea.PasteMsg{Content: "Pasted title https://x.io/a"})
	h.keys("enter")
	if got := h.task(1); got.Title != "Pasted title" || got.Link != "https://x.io/a" {
		t.Errorf("pasted task: title %q link %q", got.Title, got.Link)
	}
}

func TestCursorFollowsTaskAcrossReload(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("Low !2 ~s")
	if _, _, err := h.svc.Add(context.Background(), "High !1 ~s"); err != nil {
		t.Fatal(err)
	}
	h.keys("R") // High now sorts above Low; the cursor must stay on Low
	h.keys("p")
	h.mustSee(`pinned "Low"`)
}

func TestCollapseFromChildSelectsParent(t *testing.T) {
	h := newHarness(t, config.Default())
	ctx := context.Background()
	parent := h.add("Auth migration !1 ~xl")
	kid1 := h.add("Rollback plan ~s")
	kid2 := h.add("Token spike ~m")
	h.add("Later !1 ~s")
	for _, k := range []int64{kid1.ID, kid2.ID} {
		if _, err := h.svc.SetParent(ctx, k, parent.ID); err != nil {
			t.Fatal(err)
		}
	}
	h.keys("R", "right", "down", "left", "p")
	h.mustSee(`pinned "Auth migration"`)
}

func TestHelpFitsWithFullBacklog(t *testing.T) {
	h := newHarness(t, config.Default())
	h.send(tea.WindowSizeMsg{Width: 120, Height: 20})
	for n := 1; n <= 30; n++ {
		h.add(fmt.Sprintf("T%d !1 ~s", n))
	}
	h.keys("?")
	if lines := strings.Count(h.m.render(), "\n") + 1; lines > 20 {
		t.Errorf("help overlay renders %d lines in a 20-line terminal", lines)
	}
	h.mustSee("any key closes")
}

func TestErrorBeforeFirstLoadIsShown(t *testing.T) {
	h := newHarness(t, config.Default())
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
	h := newHarness(t, config.Default())
	ctx := context.Background()
	parent := h.add("Parent !1 ~xl")
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
	h.mustSee("▸ [P1 XL] Parent 1/2")
}
