// Package tui is tt's terminal UI (Bubble Tea). It never changes state itself:
// every change goes through app.Service, and the view reloads afterwards.
package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/app"
)

// Options tunes the background loops.
type Options struct {
	WatchEvery time.Duration // how often to check for writes from other processes (D13)
	ClockEvery time.Duration // how often to check for a new planning day (D19)
	// Schedule returns a command that delivers msg after d. Tests replace it
	// so Update round-trips never wait on timers.
	Schedule func(d time.Duration, msg tea.Msg) tea.Cmd
}

func DefaultOptions() Options {
	return Options{
		WatchEvery: time.Second,
		ClockEvery: 30 * time.Second,
		Schedule: func(d time.Duration, msg tea.Msg) tea.Cmd {
			return tea.Tick(d, func(time.Time) tea.Msg { return msg })
		},
	}
}

type screen int

const (
	screenPick screen = iota
	screenInbox
)

const maxUndo = 50

type undoEntry struct {
	label string
	fn    app.Undo
}

// overlay is a modal input (quick-add, prompts, help) that takes all keys.
type overlay interface {
	update(m *Model, k tea.KeyPressMsg) tea.Cmd
	view(m *Model) string
}

type Model struct {
	svc  *app.Service
	opts Options
	ctx  context.Context

	state         app.State
	idx           index
	loaded        bool
	width, height int

	screen screen
	pick   pickState
	inbox  inboxState

	overlay     overlay
	banner      *app.DayResult
	undo        []undoEntry
	status      string
	statusIsErr bool
}

func New(ctx context.Context, svc *app.Service, opts Options) *Model {
	return &Model{svc: svc, opts: opts, ctx: ctx, width: 100, height: 30}
}

// Messages.
type (
	loadedMsg struct {
		state app.State
		err   error
	}
	watchTickMsg struct{}
	versionMsg   struct {
		version int64
		err     error
	}
	clockTickMsg  struct{}
	dayStartedMsg struct {
		res app.DayResult
		err error
	}
	actionMsg struct {
		label, status string
		undo          app.Undo
		err           error
	}
	undoneMsg struct {
		label string
		err   error
	}
)

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.startDay(),
		m.opts.Schedule(m.opts.WatchEvery, watchTickMsg{}),
		m.opts.Schedule(m.opts.ClockEvery, clockTickMsg{}),
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case loadedMsg:
		if msg.err != nil {
			m.setErr(msg.err)
			return m, nil
		}
		m.state, m.loaded = msg.state, true
		m.idx = buildIndex(msg.state)
		m.pick.clamp(m)
		m.inbox.sync(m)
	case watchTickMsg:
		return m, tea.Batch(m.checkVersion(), m.opts.Schedule(m.opts.WatchEvery, watchTickMsg{}))
	case versionMsg:
		if msg.err == nil && msg.version != m.state.Version {
			return m, m.load()
		}
	case clockTickMsg:
		return m, tea.Batch(m.startDay(), m.opts.Schedule(m.opts.ClockEvery, clockTickMsg{}))
	case dayStartedMsg:
		if msg.err != nil {
			m.setErr(msg.err)
			return m, m.load()
		}
		if msg.res.New {
			res := msg.res
			m.banner = &res
		}
		return m, m.load()
	case actionMsg:
		if msg.err != nil {
			m.setErr(msg.err)
			return m, m.load()
		}
		if msg.undo != nil {
			m.undo = append(m.undo, undoEntry{label: msg.label, fn: msg.undo})
			if len(m.undo) > maxUndo {
				m.undo = m.undo[1:]
			}
		}
		m.setStatus(msg.status)
		return m, m.load()
	case undoneMsg:
		switch {
		case errors.Is(msg.err, app.ErrConflict) || errors.Is(msg.err, app.ErrNotFound):
			m.setErr(fmt.Errorf("can't undo %s: it changed since", msg.label))
		case msg.err != nil:
			m.setErr(msg.err)
		default:
			m.setStatus("undid " + msg.label)
		}
		return m, m.load()
	case tea.KeyPressMsg:
		return m, m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if key == "ctrl+c" {
		return tea.Quit
	}
	if m.overlay != nil {
		return m.overlay.update(m, k)
	}
	if !m.loaded {
		return nil
	}
	if m.banner != nil {
		switch key {
		case "enter":
			m.banner = nil
			m.screen = screenPick
			if len(m.state.Snapshot.Inbox()) > 0 {
				m.screen = screenInbox // planning starts with triage (D12d)
			}
			return nil
		case "esc":
			m.banner = nil
			return nil
		}
	}
	m.status, m.statusIsErr = "", false
	switch key {
	case "q":
		return tea.Quit
	case "?":
		m.overlay = helpOverlay{}
		return nil
	case "a":
		return m.openQuickAdd()
	case "ctrl+z":
		return m.undoLast()
	case "R":
		return m.load()
	}
	if m.screen == screenInbox {
		return m.inboxKey(key)
	}
	return m.pickKey(key)
}

func (m *Model) load() tea.Cmd {
	svc, ctx := m.svc, m.ctx
	return func() tea.Msg {
		st, err := svc.Load(ctx)
		return loadedMsg{state: st, err: err}
	}
}

func (m *Model) checkVersion() tea.Cmd {
	svc, ctx := m.svc, m.ctx
	return func() tea.Msg {
		v, err := svc.DataVersion(ctx)
		return versionMsg{version: v, err: err}
	}
}

func (m *Model) startDay() tea.Cmd {
	svc, ctx := m.svc, m.ctx
	return func() tea.Msg {
		res, err := svc.StartDay(ctx)
		return dayStartedMsg{res: res, err: err}
	}
}

// act runs a service command in the background and reports it as actionMsg.
func (m *Model) act(label, status string, fn func(context.Context) (app.Undo, error)) tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		undo, err := fn(ctx)
		return actionMsg{label: label, status: status, undo: undo, err: err}
	}
}

func (m *Model) undoLast() tea.Cmd {
	if len(m.undo) == 0 {
		m.setStatus("nothing to undo")
		return nil
	}
	e := m.undo[len(m.undo)-1]
	m.undo = m.undo[:len(m.undo)-1]
	ctx := m.ctx
	return func() tea.Msg { return undoneMsg{label: e.label, err: e.fn(ctx)} }
}

func (m *Model) setStatus(s string) { m.status, m.statusIsErr = s, false }
func (m *Model) setErr(err error)   { m.status, m.statusIsErr = err.Error(), true }

func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "tt"
	return v
}

func (m *Model) render() string {
	if !m.loaded {
		return "loading…"
	}
	var parts []string
	parts = append(parts, m.headerView())
	if m.banner != nil {
		parts = append(parts, m.bannerView())
	}
	if m.screen == screenInbox {
		parts = append(parts, m.inboxView())
	} else {
		parts = append(parts, m.pickView())
	}
	if m.overlay != nil {
		parts = append(parts, m.overlay.view(m))
	}
	parts = append(parts, m.statusView(), m.hintsView())
	return fitWidth(strings.Join(parts, "\n"), m.width)
}
