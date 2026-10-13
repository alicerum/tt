package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/app"
	"tt/internal/config"
)

// busyHarness is a GitHub harness with something on every screen: two
// candidates (one blocked), a backlog task with children and a note, an
// untriaged one, three review requests and a requester.
func busyHarness(t *testing.T) *harness {
	t.Helper()
	h := newGHHarness(t, config.Default())
	ctx := context.Background()
	a := h.pinned("Fix CI on the release branch @ana !1 ~s")
	h.pinned("Write the schema RFC !2 ~m")
	h.block(a.ID, 10*time.Hour, time.Hour)
	parent := h.add("Profiles migration #photos !1 ~l -- a note\nline two")
	for _, kid := range []string{"Import flow ~s", "Report export ~m"} {
		if _, _, err := h.svc.AddChild(ctx, parent.ID, kid); err != nil {
			t.Fatal(err)
		}
	}
	h.add("An idea nobody triaged")
	threeReviews(h)
	return h
}

// fits fails unless the whole screen fits a width×height terminal.
func fits(t *testing.T, h *harness, what string, width, height int) {
	t.Helper()
	lines := strings.Split(h.m.render(), "\n")
	if len(lines) > height {
		t.Errorf("%s at %dx%d: %d lines:\n%s", what, width, height, len(lines), h.screen())
	}
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > width {
			t.Errorf("%s at %dx%d: a line is %d cells wide: %q", what, width, height, w, ansi.Strip(l))
			return
		}
	}
}

func TestEveryScreenFitsTheTerminal(t *testing.T) {
	states := []struct {
		name string
		keys []string
		set  func(h *harness)
	}{
		{name: "Today"},
		{name: "Today, candidates focused", keys: []string{"tab"}},
		{name: "Today, lane focused", keys: []string{"w"}},
		{name: "Today, new-day banner", set: func(h *harness) { h.m.banner = &app.DayResult{Day: h.m.state.Plan.Day} }},
		{name: "Today, quick-add open", keys: []string{"a"}},
		{name: "Log", keys: []string{"L"}},
		{name: "Log, filter applied", keys: []string{"L", "/", "w", "a", "l", "enter"}},
		{name: "Log, typing a filter", keys: []string{"L", "/", "#"}},
		{name: "People", keys: []string{"P"}},
		{name: "People, rename prompt", keys: []string{"P", "r"}},
		{name: "Inbox", keys: []string{"i"}},
		{name: "Task detail", keys: []string{"L", "enter"}},
		{name: "help", keys: []string{"?"}},
	}
	sizes := [][2]int{{40, 8}, {80, 24}, {140, 40}, {60, 5}, {60, 4}, {60, 3}}
	for _, st := range states {
		h := busyHarness(t)
		h.keys(st.keys...)
		if st.set != nil {
			st.set(h)
		}
		for _, sz := range sizes {
			h.send(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
			fits(t, h, st.name, sz[0], sz[1])
		}
	}
}

// accentOn is the start of a focused frame's title (D55).
func accentOn() string {
	r := styleTitle.Foreground(lipgloss.Color("4")).Render("x")
	return r[:strings.Index(r, "x")]
}

func TestTheFocusedPaneIsTheAccentFrame(t *testing.T) {
	h := busyHarness(t)
	on := accentOn()
	focused := func(title string, want bool) {
		t.Helper()
		if got := strings.Contains(h.m.render(), on+title); got != want {
			t.Errorf("%s in the accent colour: %v; want %v\n%q", title, got, want, h.m.render())
		}
	}
	focused("TIMELINE", true)
	focused("CANDIDATES", false)
	h.keys("tab")
	focused("TIMELINE", false)
	focused("CANDIDATES", true)
	h.keys("w")
	focused("CANDIDATES", false)
	focused("REVIEWS", true)
	h.keys("w", "L")
	focused("BACKLOG", true)
	focused("TODAY'S CANDIDATES", false)
	h.keys("P")
	focused("PEOPLE", true)
	h.keys("tab")
	focused("PEOPLE", false)
	focused("@ana", true)
	h.keys("esc", "i")
	focused("INBOX", true)
}

func TestHeaderBarAndFooter(t *testing.T) {
	h := busyHarness(t)
	h.keys("L")
	h.send(tea.WindowSizeMsg{Width: 90, Height: 30})
	lines := strings.Split(h.m.render(), "\n")
	if !strings.HasPrefix(lines[0], "\x1b[7m") || ansi.StringWidth(lines[0]) != 90 {
		t.Errorf("the header is a full-width reverse-video bar: %q", lines[0])
	}
	if s := ansi.Strip(lines[0]); !strings.HasPrefix(s, "tt · Log · Mon 12 Oct · inbox 1") {
		t.Errorf("the header keeps its content: %q", s)
	}
	footer := ansi.Strip(lines[len(lines)-1])
	if !strings.HasSuffix(footer, "? help") || !strings.Contains(footer, " … ") || ansi.StringWidth(footer) != 90 {
		t.Errorf("a long hints line is cut at a hint and keeps ? help at the right end: %q", footer)
	}
	if !strings.HasPrefix(footer, "w reviews · ⏎ open · e edit · E note · o link") {
		t.Errorf("the Log's hints list E and o (Plan 4b carry-forward): %q", footer)
	}

	h.send(tea.WindowSizeMsg{Width: 300, Height: 30})
	h.keys("enter")
	h.mustSee("u unpin · D delete · esc back · ctrl+z undo · ? help") // Task detail's full hints
	h.keys("esc", "esc")
	h.mustSee("i inbox · a add · ctrl+z undo · ? help · q quit") // the timeline's
}

func TestShortTerminalDropsFramesFirst(t *testing.T) {
	h := busyHarness(t)
	h.send(tea.WindowSizeMsg{Width: 100, Height: 6}) // body 3: a frame and one row
	h.mustSee("╭ TIMELINE", "╰")
	h.send(tea.WindowSizeMsg{Width: 100, Height: 5}) // body 2: no frames, titles stay
	h.mustNotSee("╭", "╰")
	h.mustSee("TIMELINE", "CANDIDATES")
	for _, height := range []int{4, 3, 2, 1} {
		h.send(tea.WindowSizeMsg{Width: 100, Height: height})
		fits(t, h, fmt.Sprintf("Today at height %d", height), 100, height)
	}
}
