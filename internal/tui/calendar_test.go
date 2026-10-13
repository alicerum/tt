package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/calendar"
	"tt/internal/config"
)

func TestCalendarHeaderStatus(t *testing.T) {
	h := newHarness(t, config.Default())
	h.mustSee("tt · Today · Mon 12 Oct · cal: not set up (tt-cal calendars)") // D40

	h = newCalHarness(t, config.Default())
	if n := h.cal.Calls(); n != 1 {
		t.Fatalf("startup fetches the calendar once, got %d", n)
	}
	h.mustNotSee(" · cal", "⚠") // fresh: nothing (D44)

	h.cal.Set(nil, calendar.ErrNoAccess)
	h.keys("R")
	if n := h.cal.Calls(); n != 2 {
		t.Fatalf("R fetches now, got %d fetches", n)
	}
	h.mustSee("tt · Today · Mon 12 Oct · ⚠ cal: no access · System Settings → Privacy & Security → Calendars")

	h.cal.Set(nil, nil)
	h.keys("R")
	h.mustNotSee("⚠")
	h.clock.now = h.clock.now.Add(4 * time.Minute) // two polls of 2m: still fresh
	h.mustNotSee(" · cal")
	h.clock.now = h.clock.now.Add(time.Minute)
	h.mustSee("Mon 12 Oct · cal 5m ago")
}

func TestCalendarPollLoop(t *testing.T) {
	cfg := config.Default()
	cfg.CalendarPoll = 3 * time.Minute
	h := newCalHarness(t, cfg)
	var waits []time.Duration
	var ticks []calTickMsg
	h.m.opts.Schedule = func(d time.Duration, msg tea.Msg) tea.Cmd {
		if tick, ok := msg.(calTickMsg); ok {
			waits, ticks = append(waits, d), append(ticks, tick)
		}
		return nil
	}

	h.send(calTickMsg{seq: h.m.cal.seq})
	if n := h.cal.Calls(); n != 2 || len(ticks) != 1 || waits[0] != 3*time.Minute {
		t.Fatalf("a tick fetches and schedules the next one after the poll: %d fetches, waits %v", n, waits)
	}
	h.keys("R") // overtakes the scheduled tick
	if n := h.cal.Calls(); n != 3 || len(ticks) != 2 {
		t.Fatalf("R: %d fetches, %d ticks scheduled", n, len(ticks))
	}
	h.send(ticks[0]) // the overtaken tick
	if n := h.cal.Calls(); n != 3 {
		t.Errorf("an overtaken tick doesn't fetch again (one loop only): %d fetches", n)
	}
	h.send(ticks[1])
	if n := h.cal.Calls(); n != 4 {
		t.Errorf("the current tick fetches: %d fetches", n)
	}

	h.m.cal.syncing = true // a fetch still running
	h.keys("R")
	if n := h.cal.Calls(); n != 4 {
		t.Errorf("one fetch at a time: %d fetches", n)
	}
	h.m.cal.syncing = false

	h.clock.now = h.clock.now.Add(24 * time.Hour) // a new planning day fetches at once
	h.send(clockTickMsg{})
	if n := h.cal.Calls(); n != 5 || h.m.banner == nil {
		t.Errorf("new day: %d fetches, banner %v", n, h.m.banner != nil)
	}
}
