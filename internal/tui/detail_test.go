package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/config"
	"tt/internal/domain"
)

// family adds a parent with children (each line a child) and reloads.
func (h *harness) family(parentLine string, kids ...string) (domain.Task, []domain.Task) {
	h.t.Helper()
	ctx := context.Background()
	parent := h.add(parentLine)
	var out []domain.Task
	for _, line := range kids {
		k, _, err := h.svc.AddChild(ctx, parent.ID, line)
		if err != nil {
			h.t.Fatal(err)
		}
		out = append(out, k)
	}
	h.keys("R")
	return parent, out
}

func TestDetailShowsTheTaskAndEscGoesBack(t *testing.T) {
	h := newLogHarness(t, config.Default())
	ctx := context.Background()
	note := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8"
	parent, kids := h.family("Auth migration !3 ~xl @ana #auth https://x.io/pr/482 -- "+note,
		"Token spike ~s", "Rollback plan ~m", "Dry run ~s -- notes here")
	if _, err := h.svc.Done(ctx, kids[0].ID); err != nil {
		t.Fatal(err)
	}
	h.add("Other !2 ~s")
	h.mustSee("[50 XL] Auth migration ✎ @ana")

	h.keys("enter") // the cursor starts on Auth migration (P1 sorts first)
	h.mustSee("tt · Detail",
		fmt.Sprintf("#%d Auth migration  [50 XL]", parent.ID),
		"@ana · #auth · captured just now",
		"🔗 https://x.io/pr/482",
		"NOTE", "  line 1", "  line 6", "… +2 lines",
		"CHILDREN ██░░░░░░ 1/3",
		"  [50 M] Rollback plan", // open children first, in sibling order
		"  [50 S] Dry run ✎",
		"  [50 S] Token spike ✓ Mon 12 Oct") // then closed ones with their close date
	h.mustNotSee("line 7", "Other")
	if s := h.screen(); strings.Index(s, "Rollback plan") > strings.Index(s, "Dry run") ||
		strings.Index(s, "Dry run") > strings.Index(s, "Token spike") {
		t.Errorf("children order:\n%s", s)
	}

	// ⏎ on a child opens its detail; esc goes back one level, then to Pick.
	h.keys("j", "j", "enter")
	h.mustSee(fmt.Sprintf("#%d Dry run  [50 S]", kids[2].ID), fmt.Sprintf("child of #%d Auth migration", parent.ID), "  notes here")
	h.mustNotSee("CHILDREN") // one level only (D6)
	h.keys("esc")
	h.mustSee("CHILDREN")
	if got := h.m.detail.selected; got != kids[2].ID {
		t.Errorf("back on the parent, the cursor is on #%d; want the child we came from (#%d)", got, kids[2].ID)
	}
	h.keys("esc")
	h.mustSee("tt · Log", "BACKLOG")
	if r, _ := h.m.selectedRow(); r.task.ID != parent.ID {
		t.Errorf("Pick's cursor moved to #%d", r.task.ID)
	}
}

func TestDetailKeysActOnTheRowUnderTheCursor(t *testing.T) {
	h := newLogHarness(t, config.Default())
	parent, kids := h.family("Parent !3 ~l", "First ~s", "Second ~s")
	h.keys("enter", "j")
	h.keys("x")
	h.mustSee(`✓ "First"`, "  [50 S] First ✓ Mon 12 Oct", "1/2")
	if got := h.task(kids[0].ID); got.Status != domain.StatusDone {
		t.Fatalf("x on a child: %s", got.Status)
	}
	if h.m.detail.selected != kids[0].ID {
		t.Error("the cursor follows the task into the closed section")
	}
	h.keys("ctrl+z")
	h.mustSee(`undid done "First"`, "0/2")

	h.keys("k") // back on the task itself
	h.keys("p")
	h.mustSee(`pinned "Parent"`, fmt.Sprintf("#%d Parent  [50 L] 📌", parent.ID))
	h.keys("d")
	h.typeText("handed over")
	h.keys("enter")
	h.mustSee(`dropped "Parent"`, "dropped: handed over")
	h.keys("d")
	h.mustSee("already dropped — x reopens it")
}

func TestDetailFallsBackWhenTheTaskIsGone(t *testing.T) {
	h := newLogHarness(t, config.Default())
	ctx := context.Background()
	parent, kids := h.family("Parent !3 ~l", "Child ~s")
	h.keys("enter", "j", "enter")
	h.mustSee("child of #1 Parent")

	if err := h.svc.Delete(ctx, kids[0].ID); err != nil { // e.g. from another shell
		t.Fatal(err)
	}
	h.keys("R")
	h.mustSee(fmt.Sprintf("#%d is gone", kids[0].ID), fmt.Sprintf("#%d Parent", parent.ID), "no children")

	if err := h.svc.Delete(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	h.mustSee(fmt.Sprintf("#%d is gone", parent.ID), "tt · Log")
}

func TestFallingBackClosesTheOverlay(t *testing.T) {
	h := newLogHarness(t, config.Default())
	parent := h.add("Parent !3 ~l")
	h.keys("enter", "a")
	h.mustSee("child of #1 › ")
	if err := h.svc.Delete(context.Background(), parent.ID); err != nil {
		t.Fatal(err)
	}
	h.run(h.m.load()) // as the live reload does
	if h.m.overlay != nil {
		t.Errorf("the overlay of a gone task stays open: %T", h.m.overlay)
	}
	h.mustSee(fmt.Sprintf("#%d is gone", parent.ID))
}

func TestDetailFromTodayAndChildScrolling(t *testing.T) {
	h := newLogHarness(t, config.Default())
	var kids []string
	for i := 1; i <= 12; i++ {
		kids = append(kids, fmt.Sprintf("Step %02d ~s", i))
	}
	parent, _ := h.family("Big effort !3 ~xl", kids...)
	if _, err := h.svc.Pin(context.Background(), parent.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "tab", "enter") // ⏎ in Today's candidates opens detail too
	h.mustSee("#1 Big effort")

	h.send(tea.WindowSizeMsg{Width: 120, Height: 16})
	for range 12 {
		h.keys("j")
	}
	h.mustSee("Step 12")
	h.mustNotSee("Step 01")
	h.keys("k")
	h.mustSee("Step 12") // the window stays while the cursor moves inside it
	for range 11 {
		h.keys("k")
	}
	h.mustSee("Step 01", "… ")
	h.mustNotSee("Step 12")
}

func TestDetailOfAClosedTaskAndHelp(t *testing.T) {
	h := newLogHarness(t, config.Default())
	a := h.add("Shipped !3 ~s")
	h.clock.now = h.clock.now.Add(time.Hour)
	if _, err := h.svc.Done(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	if err := h.m.applyFilter("is:done"); err != nil {
		t.Fatal(err)
	}
	h.keys("enter")
	h.mustSee("#1 Shipped  [50 S] ✓ Mon 12 Oct", "no note", "no children")
	h.keys("?")
	h.mustSee("open the child under the cursor")
}

func TestNewDayBannerLeavesDetailBehind(t *testing.T) {
	h := newLogHarness(t, config.Default())
	a := h.add("Gone !3 ~s")
	h.keys("enter") // detail is open when the day rolls over
	h.mustSee("#1 Gone")

	h.clock.now = h.clock.now.Add(24 * time.Hour)
	h.send(clockTickMsg{})
	h.mustSee("tt · Detail")
	h.keys("enter") // ⏎ on the new-day banner goes to planning
	h.mustSee("tt · Log")
	if len(h.m.detail.stack) != 0 {
		t.Fatalf("detail stack survived the banner: %v", h.m.detail.stack)
	}

	h.keys("D", "y")
	h.mustSee(fmt.Sprintf("deleted #%d", a.ID))
	h.mustNotSee("is gone")
}
