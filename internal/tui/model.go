// Package tui is tt's terminal UI (Bubble Tea). It never changes state itself:
// every change goes through app.Service, and the view reloads afterwards.
package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/app"
	"tt/internal/domain"
	"tt/internal/search"
)

// Options tunes the background loops and the outside programs the TUI runs.
type Options struct {
	WatchEvery time.Duration // how often to check for writes from other processes (D13)
	ClockEvery time.Duration // how often to check for a new planning day (D19)
	// Schedule returns a command that delivers msg after d. Tests replace it
	// so Update round-trips never wait on timers.
	Schedule func(d time.Duration, msg tea.Msg) tea.Cmd
	// Exec hands the terminal to c (the note editor, D29) and reports its
	// exit through done. nil means tea.ExecProcess; tests edit in-process.
	Exec func(c *exec.Cmd, done tea.ExecCallback) tea.Cmd
	// Open opens a link (D21). nil means macOS `open`.
	Open func(ctx context.Context, url string) error
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
	screenToday screen = iota // the home screen (D35)
	screenLog                 // the backlog; the planning step Pick
	screenInbox
	screenDetail
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

// paster is implemented by overlays that accept bracketed-paste text.
type paster interface {
	paste(m *Model, msg tea.PasteMsg) tea.Cmd
}

// resizer is implemented by overlays whose width follows the terminal's.
type resizer interface{ resize(width int) }

type Model struct {
	svc  *app.Service
	opts Options
	ctx  context.Context

	state         app.State
	idx           index
	loaded        bool
	width, height int

	screen screen
	today  todayState
	pick   pickState
	inbox  inboxState
	detail detailState

	cal pollLoop // the calendar's
	gh  pollLoop // GitHub's (D50)

	overlay     overlay
	banner      *app.DayResult
	undo        []undoEntry
	status      string
	statusIsErr bool
}

func New(ctx context.Context, svc *app.Service, opts Options) *Model {
	return &Model{svc: svc, opts: opts, ctx: ctx, width: 100, height: 30, pick: pickState{group: search.ByPriority}}
}

// pollLoop is an integration's poll loop (D3): one fetch at a time, then the
// next tick after the configured interval. R (and, for the calendar, a new
// day) fetches at once; seq retires the tick it overtook, so there is only
// ever one loop.
type pollLoop struct {
	syncing bool
	seq     int
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
		restore       func(*Model) // on error: give back what the user typed
	}
	undoneMsg struct {
		label string
		err   error
	}
	uiStateMsg struct {
		group string
		err   error
	}
	uiSavedMsg   struct{ err error }
	calTickMsg   struct{ seq int }
	calSyncedMsg struct{ err error }
	ghTickMsg    struct{ seq int }
	ghSyncedMsg  struct {
		closed []domain.Task // tasks the fetch closed (D20a)
		err    error
	}
)

func (m *Model) Init() tea.Cmd {
	svc, ctx := m.svc, m.ctx
	return tea.Batch(
		func() tea.Msg {
			g, err := svc.UIState(ctx, groupKey)
			return uiStateMsg{group: g, err: err}
		},
		m.startDay(),
		m.syncCalendar(), // the first load shows the cache meanwhile
		m.syncGitHub(),
		m.opts.Schedule(m.opts.WatchEvery, watchTickMsg{}),
		m.opts.Schedule(m.opts.ClockEvery, clockTickMsg{}),
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.update(msg)
	m.pick.follow(m)
	m.detail.follow(m)
	m.today.follow(m)
	return m, cmd
}

func (m *Model) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if r, ok := m.overlay.(resizer); ok {
			r.resize(msg.Width)
		}
	case loadedMsg:
		if msg.err != nil {
			m.setErr(msg.err)
			return nil
		}
		m.state, m.loaded = msg.state, true
		m.idx = buildIndex(msg.state)
		m.refreshRows()
		m.inbox.sync(m)
	case watchTickMsg:
		cmds := []tea.Cmd{m.checkVersion(), m.opts.Schedule(m.opts.WatchEvery, watchTickMsg{})}
		if m.dayIsStale() {
			cmds = append(cmds, m.startDay())
		}
		return tea.Batch(cmds...)
	case versionMsg:
		if msg.err != nil {
			m.setErr(fmt.Errorf("can't read the database: %w", msg.err))
			return nil
		}
		if msg.version != m.state.Version {
			return m.load()
		}
	case clockTickMsg:
		return tea.Batch(m.startDay(), m.opts.Schedule(m.opts.ClockEvery, clockTickMsg{}))
	case dayStartedMsg:
		if msg.err != nil {
			m.setErr(msg.err)
			return m.load()
		}
		if msg.res.New {
			res := msg.res
			m.banner = &res
			m.today.placed = false // a new day's cursor starts on its now row
			m.today.carry = nil
			return tea.Batch(m.load(), m.syncCalendar()) // yesterday's meetings don't count (D44)
		}
		return m.load()
	case actionMsg:
		if msg.err != nil {
			m.setErr(msg.err)
			if msg.restore != nil {
				msg.restore(m)
			}
			return m.load()
		}
		if msg.undo != nil {
			m.undo = append(m.undo, undoEntry{label: msg.label, fn: msg.undo})
			if len(m.undo) > maxUndo {
				m.undo = m.undo[1:]
			}
		}
		m.setStatus(msg.status)
		return m.load()
	case undoneMsg:
		switch {
		case errors.Is(msg.err, app.ErrConflict) || errors.Is(msg.err, app.ErrNotFound):
			m.setErr(fmt.Errorf("can't undo %s: it changed since", msg.label))
		case msg.err != nil:
			m.setErr(msg.err)
		default:
			m.setStatus("undid " + msg.label)
		}
		return m.load()
	case uiStateMsg:
		if msg.err != nil {
			m.setErr(fmt.Errorf("can't read the remembered grouping: %w", msg.err))
			return nil
		}
		if m.pick.groupChosen {
			return nil // a g pressed before the read landed wins
		}
		m.pick.group = search.ParseGroupBy(msg.group)
		if m.loaded {
			m.refreshRows()
		}
	case uiSavedMsg:
		m.pick.saving = false
		if msg.err != nil {
			m.setErr(fmt.Errorf("can't remember the grouping: %w", msg.err))
		}
		if m.pick.saveAgain {
			m.pick.saveAgain = false
			return m.saveGroup()
		}
	case calTickMsg:
		if msg.seq == m.cal.seq {
			return m.syncCalendar()
		}
	case calSyncedMsg:
		m.cal.syncing = false
		m.cal.seq++
		if msg.err != nil && !errors.Is(msg.err, context.Canceled) {
			m.setErr(fmt.Errorf("can't record the calendar fetch: %w", msg.err))
		}
		return tea.Batch(m.load(), m.opts.Schedule(m.svc.Config().CalendarPoll, calTickMsg{seq: m.cal.seq}))
	case ghTickMsg:
		if msg.seq == m.gh.seq {
			return m.syncGitHub()
		}
	case ghSyncedMsg:
		m.gh.syncing = false
		m.gh.seq++
		switch {
		case msg.err != nil && !errors.Is(msg.err, context.Canceled):
			m.setErr(fmt.Errorf("can't record the GitHub fetch: %w", msg.err))
		case len(msg.closed) == 1:
			m.setStatus(fmt.Sprintf("✓ via GitHub: %q", msg.closed[0].Title)) // D49
		case len(msg.closed) > 1:
			m.setStatus("✓ via GitHub: " + plural(len(msg.closed), "review", "reviews"))
		}
		return tea.Batch(m.load(), m.opts.Schedule(m.svc.Config().GitHubPoll, ghTickMsg{seq: m.gh.seq}))
	case noteEditedMsg:
		return m.noteEdited(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.PasteMsg:
		if p, ok := m.overlay.(paster); ok {
			return p.paste(m, msg)
		}
	}
	return nil
}

func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if key == "ctrl+c" {
		return tea.Quit
	}
	if m.overlay != nil {
		return m.overlay.update(m, k)
	}
	if key == "q" && !m.loaded {
		return tea.Quit // quit works even when the first load failed
	}
	if !m.loaded {
		return nil
	}
	if m.banner != nil {
		switch key {
		case "enter":
			m.banner = nil
			m.detail = detailState{} // a detail left open overnight must not survive the banner
			m.screen = screenLog     // planning: Inbox → Pick (in the Log) → B → Today (D35)
			if len(m.state.Snapshot.Inbox()) > 0 {
				m.openInbox() // planning starts with triage (D12d)
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
		if m.screen == screenDetail && len(m.detail.rows) > 0 {
			return m.openAddChild() // in Task detail, a adds a child (view 5)
		}
		return m.openQuickAdd()
	case "ctrl+z":
		return m.undoLast()
	case "R":
		return tea.Batch(m.load(), m.syncCalendar(), m.syncGitHub())
	}
	if m.dayIsStale() {
		m.setStatus("new day — refreshing")
		return m.startDay()
	}
	switch m.screen {
	case screenToday:
		return m.todayKey(key)
	case screenInbox:
		return m.inboxKey(key)
	case screenDetail:
		return m.detailKey(key)
	}
	return m.pickKey(key)
}

// dayIsStale: the planning day rolled over since the last load, so the view
// still shows yesterday until StartDay runs.
func (m *Model) dayIsStale() bool {
	return m.loaded && m.svc.Today() != m.state.Plan.Day
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

// syncCalendar fetches the calendar in the background, unless none is set
// up or a fetch is already running.
func (m *Model) syncCalendar() tea.Cmd {
	if !m.svc.CalendarSetUp() || m.cal.syncing {
		return nil
	}
	m.cal.syncing = true
	svc, ctx := m.svc, m.ctx
	return func() tea.Msg { return calSyncedMsg{err: svc.SyncCalendar(ctx)} }
}

// syncGitHub fetches the review requests in the background, unless GitHub
// isn't set up or a fetch is already running.
func (m *Model) syncGitHub() tea.Cmd {
	if !m.svc.SourceSetUp() || m.gh.syncing {
		return nil
	}
	m.gh.syncing = true
	svc, ctx := m.svc, m.ctx
	return func() tea.Msg {
		closed, err := svc.SyncSources(ctx)
		return ghSyncedMsg{closed: closed, err: err}
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
	return m.actOr(label, status, nil, fn)
}

// actOr is act with restore, which runs if the command fails.
func (m *Model) actOr(label, status string, restore func(*Model), fn func(context.Context) (app.Undo, error)) tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		undo, err := fn(ctx)
		return actionMsg{label: label, status: status, undo: undo, err: err, restore: restore}
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
		return fitWidth("loading…\n"+m.statusView(), m.width)
	}
	var parts []string
	parts = append(parts, m.headerView())
	if h, ok := m.overlay.(helpOverlay); ok { // help takes the whole screen
		return fitWidth(strings.Join(append(parts, h.view(m)), "\n"), m.width)
	}
	if m.banner != nil {
		parts = append(parts, m.bannerView())
	}
	fb, filtering := m.overlay.(*filterBar)
	switch m.screen {
	case screenToday:
		parts = append(parts, m.todayView())
	case screenInbox:
		parts = append(parts, clipLines(m.inboxView(), m.bodyHeight()))
	case screenDetail:
		parts = append(parts, clipLines(m.detailView(), m.bodyHeight()))
	default:
		if filtering {
			parts = append(parts, fb.view(m)) // the filter line sits above the backlog
		}
		parts = append(parts, m.pickView())
	}
	if m.overlay != nil && !filtering {
		parts = append(parts, m.overlay.view(m))
	}
	parts = append(parts, m.statusView(), m.hintsView())
	return fitWidth(strings.Join(parts, "\n"), m.width)
}
