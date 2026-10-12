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
	h := newHarness(t, config.Default())
	h.add("Schema review !1 ~s @ana")
	h.add("Deploy fix !2 ~m")

	h.keys("/")
	if s := h.screen(); strings.Index(s, "filter ›") > strings.Index(s, "BACKLOG") {
		t.Errorf("the filter line sits above the backlog:\n%s", s)
	}
	h.typeText("sch")
	h.mustSee("BACKLOG · by priority · sch · 1 match", "Schema review")
	h.mustNotSee("Deploy fix")

	h.typeText(" !9")
	h.mustSee("✗ priority !9 out of range", "1 match", "Schema review") // the last valid filter ("sch !") stays
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
	h.mustSee("sch · 1 match", "esc clears filter")

	// esc in the line restores the filter from before `/`.
	h.keys("/")
	h.mustSee("filter › sch")
	h.typeText("zzz")
	h.mustSee("sch zzz · 0 matches", "no matches · esc clears")
	h.keys("esc")
	h.mustSee("sch · 1 match")

	// esc on the list clears it; filters are never remembered.
	h.keys("esc")
	h.mustSee("filter cleared", "Deploy fix")
	h.mustNotSee("match")

	// Pasting into the line filters too.
	h.keys("/")
	h.send(tea.PasteMsg{Content: "@ana"})
	h.mustSee("@ana · 1 match", "Schema review")
}

func TestReloadWhileFilteringKeepsTheFilter(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("Schema review !1 ~s")
	h.keys("/")
	h.typeText("schema")
	other, err := store.Open(context.Background(), h.path) // `tt add` from another shell
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, _, err := app.New(other, h.clock, config.Default()).Add(context.Background(), "Schema follow-up"); err != nil {
		t.Fatal(err)
	}
	h.send(watchTickMsg{})
	h.mustSee("filter › schema", "schema · 2 matches", "[— —] Schema follow-up")
}

func TestTokenInputCompletesAtTheCursor(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("one @ana #photos")
	h.add("two @andrei")

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
	h.mustSee("filter › @andrei", "@andrei · 1 match", "two")
}

func TestOverlayInputFollowsResize(t *testing.T) {
	h := newHarness(t, config.Default())
	h.keys("a")
	h.send(tea.WindowSizeMsg{Width: 60, Height: 40})
	if w, want := h.m.overlay.(*quickAdd).in.input.Width(), 60-ansi.StringWidth("add › ")-2; w != want {
		t.Errorf("quick-add input width after resize = %d; want %d", w, want)
	}
	h.keys("esc", "/")
	h.send(tea.WindowSizeMsg{Width: 80, Height: 40})
	if w, want := h.m.overlay.(*filterBar).in.input.Width(), 80-ansi.StringWidth("filter › ")-2; w != want {
		t.Errorf("filter input width after resize = %d; want %d", w, want)
	}
}
