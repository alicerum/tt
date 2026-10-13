package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/app"
	"tt/internal/config"
	"tt/internal/store"
)

func TestFilterBarNarrowsLiveAndKeepsTypingOnError(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.add("Schema review !1 ~s @ana")
	h.add("Deploy fix !2 ~m")

	h.keys("/")
	if l := strings.Split(h.screen(), "\n"); !strings.HasPrefix(l[1], "╭ BACKLOG") || !strings.HasPrefix(l[2], "│ filter ›") {
		t.Errorf("the filter line is the first row inside the backlog's frame (D56):\n%s", h.screen())
	}
	h.typeText("sch")
	h.mustSee("BACKLOG · by priority · 1 match ─", "filter › sch", "Schema review")
	h.mustNotSee("Deploy fix")

	h.typeText(" !9")
	h.mustSee("✗ priority !9 out of range", "BACKLOG · by priority · 1 match", "Schema review") // the last valid filter ("sch !") stays
	h.keys("enter")
	if _, ok := h.m.overlay.(*filterBar); !ok {
		t.Fatal("enter on a bad filter must keep the line open")
	}
	h.mustSee("filter › sch !9")
	h.keys("backspace", "backspace", "backspace")
	h.mustNotSee("✗")
	h.keys("enter")
	if h.m.overlay != nil {
		t.Fatal("enter closes the filter line")
	}
	h.mustSee("1 match", "filter › sch · esc clears", "esc clears filter")

	// esc in the line restores the filter from before `/`.
	h.keys("/")
	h.mustSee("filter › sch")
	h.typeText("zzz")
	h.mustSee("0 matches", "filter › sch zzz", "no matches")
	h.keys("esc")
	h.mustSee("1 match", "filter › sch · esc clears")

	// esc on the list clears it; filters are never remembered.
	h.keys("esc")
	h.mustSee("filter cleared", "Deploy fix")
	h.mustNotSee("match", "filter ›")

	// Pasting into the line filters too.
	h.keys("/")
	h.send(tea.PasteMsg{Content: "@ana"})
	h.mustSee("1 match", "filter › @ana", "Schema review")
}

func TestReloadWhileFilteringKeepsTheFilter(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.add("Schema review !1 ~s")
	h.keys("/")
	h.typeText("schema")
	other, err := store.Open(context.Background(), h.path) // `tt add` from another shell
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, _, err := app.New(other, h.clock, config.Default()).Add(context.Background(), "Schema follow-up !1 ~s"); err != nil {
		t.Fatal(err)
	}
	h.send(watchTickMsg{})
	h.mustSee("filter › schema", "2 matches", "[P1 S] Schema follow-up")
}

func TestTokenInputCompletesAtTheCursor(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.add("one @ana #photos !1 ~s")
	h.add("two @andrei !1 ~s")

	h.keys("a")
	h.typeText("x @an #ph")
	h.keys("left", "left", "left", "left") // into "@an"
	h.mustSee("tab → @ana  @andrei")
	h.keys("tab")
	h.mustSee("add › x @ana #ph")
	h.keys("esc")

	h.keys("a")
	h.typeText("y -- ask #ph") // inside the note: no completion
	h.mustNotSee("tab →")
	h.keys("esc")

	h.keys("/")
	h.typeText("@and")
	h.mustSee("tab → @andrei")
	h.keys("tab")
	h.mustSee("filter › @andrei", "1 match", "two")
}

func TestOverlayInputFollowsResize(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.keys("a")
	h.send(tea.WindowSizeMsg{Width: 60, Height: 40})
	if w, want := h.m.overlay.(*quickAdd).in.input.Width(), 60-ansi.StringWidth("add › ")-2; w != want {
		t.Errorf("quick-add input width after resize = %d; want %d", w, want)
	}
	h.keys("esc", "/")
	h.send(tea.WindowSizeMsg{Width: 80, Height: 40})
	if w, want := h.m.overlay.(*filterBar).in.input.Width(), backlogWidth(80)-3-ansi.StringWidth("filter › ")-2; w != want { // inside the backlog frame (D56)
		t.Errorf("filter input width after resize = %d; want %d", w, want)
	}
}

func TestDropKeyKeepsTheReasonOfADroppedTask(t *testing.T) {
	h := newLogHarness(t, config.Default())
	a := h.add("Alpha !1 ~s")
	h.keys("d")
	h.typeText("handed to @marko")
	h.keys("enter")
	h.keys("/")
	h.typeText("is:dropped")
	h.keys("enter")
	h.mustSee("Alpha")
	h.keys("d")
	h.mustSee("already dropped — x reopens it")
	if h.m.overlay != nil {
		t.Fatal("d on a dropped task must not open the drop prompt")
	}
	if got := h.task(a.ID); got.DropReason != "handed to @marko" {
		t.Errorf("drop reason = %q; want it kept", got.DropReason)
	}
}

func TestMatchCountSurvivesNarrowLeftPane(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.send(tea.WindowSizeMsg{Width: 100, Height: 30})
	h.add("Schema migration !1 ~s @ana #impl")
	h.add("Deploy fix !2 ~m")
	h.keys("/")
	h.typeText("@ana #impl schema migration")
	h.mustSee("1 match")
}

func TestEscHintHiddenWhileFilterLineOpen(t *testing.T) {
	h := newLogHarness(t, config.Default())
	h.add("Schema review !1 ~s")
	h.keys("/")
	h.typeText("sch")
	h.keys("enter")
	h.mustSee("esc clears filter")
	h.keys("/")
	h.mustNotSee("esc clears filter")
}
