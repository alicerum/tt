package tui

import (
	"context"
	"path/filepath"
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

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

// harness drives a Model synchronously: every command a key or message
// returns is executed and its message fed back, until nothing is left.
// Timers are disabled (Schedule returns nil).
type harness struct {
	t     *testing.T
	m     *Model
	svc   *app.Service
	clock *fakeClock
	path  string
	quit  bool
}

func newHarness(t *testing.T, cfg config.Config) *harness {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tt.db")
	st, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	clock := &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
	svc := app.New(st, clock, cfg)
	opts := Options{WatchEvery: time.Second, ClockEvery: 30 * time.Second,
		Schedule: func(time.Duration, tea.Msg) tea.Cmd { return nil }}
	h := &harness{t: t, m: New(context.Background(), svc, opts), svc: svc, clock: clock, path: path}
	h.run(h.m.Init())
	h.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	if h.m.banner == nil {
		t.Fatal("the first launch of a day shows the new-day banner")
	}
	h.keys("esc") // tests start with the banner dismissed
	return h
}

func (h *harness) send(msg tea.Msg) {
	h.t.Helper()
	_, cmd := h.m.Update(msg)
	h.run(cmd)
}

func (h *harness) run(cmd tea.Cmd) {
	h.t.Helper()
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
	case tea.QuitMsg:
		h.quit = true
	default:
		h.send(msg)
	}
}

var named = map[string]tea.KeyPressMsg{
	"enter": {Code: tea.KeyEnter}, "tab": {Code: tea.KeyTab}, "esc": {Code: tea.KeyEscape},
	"up": {Code: tea.KeyUp}, "down": {Code: tea.KeyDown}, "left": {Code: tea.KeyLeft}, "right": {Code: tea.KeyRight},
	"ctrl+z": {Code: 'z', Mod: tea.ModCtrl}, "ctrl+c": {Code: 'c', Mod: tea.ModCtrl},
	"backspace": {Code: tea.KeyBackspace},
}

// keys sends named keys ("enter", "ctrl+z") or single characters.
func (h *harness) keys(keys ...string) {
	h.t.Helper()
	for _, k := range keys {
		msg, ok := named[k]
		if !ok {
			r := []rune(k)
			if len(r) != 1 {
				h.t.Fatalf("unknown key %q", k)
			}
			msg = tea.KeyPressMsg{Code: r[0], Text: k}
		}
		h.send(msg)
	}
}

// typeText types s one character at a time.
func (h *harness) typeText(s string) {
	h.t.Helper()
	for _, r := range s {
		h.send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func (h *harness) screen() string { return ansi.Strip(h.m.render()) }

func (h *harness) mustSee(parts ...string) {
	h.t.Helper()
	s := h.screen()
	for _, p := range parts {
		if !strings.Contains(s, p) {
			h.t.Fatalf("screen lacks %q:\n%s", p, s)
		}
	}
}

func (h *harness) mustNotSee(parts ...string) {
	h.t.Helper()
	s := h.screen()
	for _, p := range parts {
		if strings.Contains(s, p) {
			h.t.Fatalf("screen unexpectedly shows %q:\n%s", p, s)
		}
	}
}

func (h *harness) add(line string) domain.Task {
	h.t.Helper()
	t, _, err := h.svc.Add(context.Background(), line)
	if err != nil {
		h.t.Fatal(err)
	}
	h.keys("R")
	return t
}

func (h *harness) task(id int64) domain.Task {
	h.t.Helper()
	snap, err := h.svc.Snapshot(context.Background())
	if err != nil {
		h.t.Fatal(err)
	}
	t, ok := snap.Get(id)
	if !ok {
		h.t.Fatalf("task #%d missing", id)
	}
	return t
}
