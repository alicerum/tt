package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/config"
)

// before fails unless the screen shows each of parts, in this order.
func (h *harness) before(parts ...string) {
	h.t.Helper()
	s, at := h.screen(), 0
	for _, p := range parts {
		i := strings.Index(s[at:], p)
		if i < 0 {
			h.t.Fatalf("screen lacks %q after %q:\n%s", p, s[:at], s)
		}
		at += i + len(p)
	}
}

func TestPeopleOpensFromTodayAndTheLog(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("Fix login @ana !1 ~s")
	h.keys("P")
	h.mustSee("tt · People", "PEOPLE · 1", "ana · 1 open", "@ana", "[P1 S] Fix login")
	h.keys("esc")
	if h.m.screen != screenToday {
		t.Fatalf("esc from People opened from Today: screen %d", h.m.screen)
	}
	h.keys("L", "P")
	h.mustSee("tt · People")
	h.keys("esc")
	if h.m.screen != screenLog {
		t.Fatalf("esc from People opened from the Log: screen %d", h.m.screen)
	}

	h.keys("P", "tab", "enter") // Task detail from People goes back there
	h.mustSee("tt · Detail", "#1 Fix login")
	h.keys("esc")
	h.mustSee("tt · People")
	h.keys("esc")
	if h.m.screen != screenLog {
		t.Errorf("People still returns to the Log after a detail round-trip: screen %d", h.m.screen)
	}
}

func TestPeopleFromTheReviewsLane(t *testing.T) {
	h := newGHHarness(t, config.Default())
	h.add("Fix login @ana !1 ~s")
	threeReviews(h)
	h.keys("w", "P") // a screen key leaves the lane (D49)
	h.mustSee("tt · People")
	h.keys("esc")
	if h.m.screen != screenToday || h.m.lane.focused {
		t.Errorf("back on Today with the lane unfocused: screen %d, lane focused %v", h.m.screen, h.m.lane.focused)
	}
}

func TestPeopleListsEveryRequester(t *testing.T) {
	h := newHarness(t, config.Default())
	ctx := context.Background()
	h.add("Schema review @ana !1 ~s")
	h.add("Later ask @ana !2 ~m")
	old := h.add("Old ask @ana !1 ~s")
	idea := h.add("Raff's idea @raff !3 ~s")
	h.add("Nobody asked !1 ~s")
	if _, err := h.svc.Done(ctx, old.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Drop(ctx, idea.ID, ""); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "P")
	h.mustSee("PEOPLE · 2")
	h.before("ana · 2 open", "raff · 0 open") // alphabetical; closed tasks still name raff
	h.mustNotSee("Nobody asked")
	if got, want := h.m.personLine(person{name: "raff"}, 30, false), styleDim.Render(pad("  raff · 0 open", 30)); got != want {
		t.Errorf("a person with nothing open is dimmed: %q; want %q", got, want)
	}
	if got := h.m.personLine(person{name: "ana", open: 2}, 30, false); got != pad("  ana · 2 open", 30) {
		t.Errorf("ana's line: %q", got)
	}
}

func TestPeopleShowsAPersonsTasks(t *testing.T) {
	h := newHarness(t, config.Default())
	ctx := context.Background()
	login := h.add("Fix login @ana !1 ~s")
	h.add("Write RFC @ana !2 ~m")
	parent := h.add("Profiles !1 ~l")
	if _, _, err := h.svc.AddChild(ctx, parent.ID, "Import flow @ana ~s"); err != nil {
		t.Fatal(err)
	}
	bug := h.add("Old bug @ana !1 ~s")
	idea := h.add("Idea @ana !3 ~s")
	h.add("Bob's ask @bob !1 ~s")
	if _, err := h.svc.Pin(ctx, login.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Done(ctx, bug.ID); err != nil {
		t.Fatal(err)
	}
	h.clock.now = h.clock.now.Add(time.Hour)
	if _, err := h.svc.Drop(ctx, idea.ID, ""); err != nil {
		t.Fatal(err)
	}
	h.keys("R", "P")
	h.before("@ana",
		"P1 · someone is blocked", "[P1 S] Fix login 📌", fmt.Sprintf("[P1 S] Import flow ↳ #%d", parent.ID),
		"P2 · important this month", "[P2 M] Write RFC",
		"closed", "Idea ✗ Mon 12 Oct", "Old bug ✓ Mon 12 Oct") // newest first
	h.mustNotSee("Fix login @ana", "Bob's ask", "Profiles")
}

func TestPeopleTaskKeys(t *testing.T) {
	h := newHarness(t, config.Default())
	login := h.add("Fix login @ana !1 ~s")
	h.add("Write RFC @ana !2 ~m")
	h.add("Bob's ask @bob !1 ~s")
	h.keys("P", "tab")
	h.keys("x")
	h.mustSee(`✓ "Fix login"`)
	h.before("closed", "Fix login ✓ Mon 12 Oct")
	h.keys("ctrl+z")
	h.mustSee(`undid done "Fix login"`, "ana · 2 open")
	h.keys("p")
	h.mustSee(`pinned "Fix login"`, "[P1 S] Fix login 📌")

	h.keys("e", "ctrl+u") // the edit line, cleared
	h.typeText("Fix login @bob !1 ~s")
	h.keys("enter")
	if got := h.task(login.ID).Requester; got != "bob" {
		t.Fatalf("requester after e = %q", got)
	}
	if h.m.people.selected != "ana" {
		t.Errorf("the cursor stays on the person: %q", h.m.people.selected)
	}
	h.mustSee("ana · 1 open", "bob · 2 open", "@ana", "Write RFC", `edited "Fix login"`)
	h.mustNotSee("[P1 S] Fix login")
	h.keys("enter")
	h.mustSee("tt · Detail", "Write RFC")
}

func TestPeopleFilter(t *testing.T) {
	h := newHarness(t, config.Default())
	for _, who := range []string{"ana", "anabel", "raff", "bob"} {
		h.add(fmt.Sprintf("Ask from %s @%s !1 ~s", who, who))
	}
	h.keys("P", "/")
	h.typeText("an")
	h.mustSee("PEOPLE · 2 matches · an", "ana · 1 open", "anabel · 1 open")
	h.mustNotSee("raff · ", "bob · ")
	h.keys("esc") // esc in the filter clears it
	h.mustSee("PEOPLE · 4", "raff · 1 open")

	h.keys("/")
	h.typeText("@rf")
	h.keys("enter")
	h.mustSee("PEOPLE · 1 match · @rf", "@raff", "Ask from raff", "esc clears filter")
	h.keys("esc")
	h.mustSee("filter cleared", "PEOPLE · 4")
	if h.m.screen != screenPeople || h.m.people.selected != "raff" {
		t.Errorf("esc on a filtered list clears the filter and keeps the person: screen %d, %q", h.m.screen, h.m.people.selected)
	}

	h.keys("/")
	h.typeText("zz")
	h.mustSee("PEOPLE · 0 matches · zz", "no matches · esc clears")
	h.mustNotSee("Ask from")
	h.keys("x") // still typing: a letter, not a task key
	h.mustSee("0 matches · zzx")
	h.keys("enter", "esc", "esc")
	if h.m.screen != screenToday {
		t.Errorf("esc, esc: filter cleared, then back to Today: screen %d", h.m.screen)
	}
}

func TestPeopleCursorAcrossReloads(t *testing.T) {
	h := newHarness(t, config.Default())
	ctx := context.Background()
	h.add("Ana's @ana !1 ~s")
	h.add("Bob's @bob !1 ~s")
	h.add("Cat's first @cat !2 ~s")
	second := h.add("Cat's second @cat !2 ~s")
	h.keys("P", "j", "j", "tab", "j")
	if r, _ := h.m.people.tasks.selectedRow(); r.task.ID != second.ID {
		t.Fatalf("on cat's second task, got #%d", r.task.ID)
	}
	h.add("Cat's urgent @cat !1 ~s") // sorts above both
	if r, _ := h.m.people.tasks.selectedRow(); r.task.ID != second.ID {
		t.Errorf("a reload keeps the task under the cursor, got #%d", r.task.ID)
	}

	h.keys("tab", "k") // on bob
	if _, _, err := h.svc.RenameRequester(ctx, "bob", "zed"); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	if h.m.people.selected != "cat" {
		t.Errorf("bob renamed elsewhere: the cursor lands on the nearest row (cat), got %q", h.m.people.selected)
	}
	h.mustSee("@cat", "zed · 1 open")
}

func TestPeopleFitsAShortTerminal(t *testing.T) {
	h := newHarness(t, config.Default())
	for i := 1; i <= 12; i++ {
		h.add(fmt.Sprintf("Ask %02d @p%02d !1 ~s", i, i))
		h.add(fmt.Sprintf("Task %02d @p01 !1 ~s", i))
	}
	h.send(tea.WindowSizeMsg{Width: 100, Height: 8})
	h.keys("P")
	check := func() {
		t.Helper()
		if n := strings.Count(h.m.render(), "\n") + 1; n > 8 {
			t.Fatalf("People renders %d lines in an 8-line terminal:\n%s", n, h.screen())
		}
		for _, l := range strings.Split(h.m.render(), "\n") {
			if w := ansi.StringWidth(l); w > 100 {
				t.Fatalf("a line is %d cells wide: %q", w, ansi.Strip(l))
			}
		}
	}
	check()
	h.keys("tab", "ctrl+d", "ctrl+d", "ctrl+d", "ctrl+d")
	check()
	if r, _ := h.m.people.tasks.selectedRow(); !strings.Contains(h.screen(), r.task.Title) {
		t.Errorf("the cursor's task %q is in view:\n%s", r.task.Title, h.screen())
	}
	h.keys("tab")
	for range 11 {
		h.keys("j")
	}
	check()
	h.mustSee("p12 · 1 open", "@p12")
}

func TestPeopleWithNobody(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("Nobody asked !1 ~s")
	h.keys("P")
	h.mustSee("PEOPLE · 0", "nobody yet · @who names who asked")
	h.keys("tab", "x", "enter", "j") // nothing to act on
	h.mustNotSee("✓")
}
