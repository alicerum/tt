package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/domain"
)

// laneMaxRows is how many review requests the lane shows at once (D49).
const laneMaxRows = 5

// laneState is the Reviews lane (D49): GitHub's review requests that have no
// open task, under the candidates on Today and under the backlog in the Log.
// Both screens share one cursor. w focuses it; w or tab gives the focus back
// to the pane it came from, which never changed.
type laneState struct {
	focused  bool
	cursor   int
	offset   int    // first request shown
	selected string // external id under the cursor; survives reloads
}

func (m *Model) reviews() []domain.SourceItem { return m.state.GitHub.Reviews }

// refresh puts the cursor back on its request after a load, or on the same
// index when that request left the lane (pinned, or no longer requested).
func (l *laneState) refresh(m *Model) {
	items := m.reviews()
	if i := slices.IndexFunc(items, func(i domain.SourceItem) bool { return i.ExternalID == l.selected }); i >= 0 {
		l.cursor = i
	}
	l.cursor = max(0, min(l.cursor, len(items)-1))
	l.selected = ""
	if l.cursor < len(items) {
		l.selected = items[l.cursor].ExternalID
	}
	if len(items) == 0 {
		l.focused = false
	}
}

func (l *laneState) move(m *Model, delta int) {
	items := m.reviews()
	if c := l.cursor + delta; c >= 0 && c < len(items) {
		l.cursor, l.selected = c, items[c].ExternalID
	}
}

// follow keeps the cursor inside the lane's window. Called after every update.
func (l *laneState) follow(m *Model) {
	rows := m.laneLines() - 2
	if rows <= 0 {
		l.offset = 0
		return
	}
	l.offset = min(l.offset, l.cursor)
	if l.cursor >= l.offset+rows {
		l.offset = l.cursor - rows + 1
	}
	l.offset = max(0, min(l.offset, len(m.reviews())-rows))
}

// laneLines is how many lines the lane takes in its column: its frame (or,
// unframed, a blank line and its title) and up to laneMaxRows requests. The
// pane above keeps its frame and three rows, so in a short column the lane
// gives up rows first, then disappears; 0 when GitHub is off.
func (m *Model) laneLines() int {
	if !m.state.GitHub.SetUp {
		return 0
	}
	room := m.bodyHeight() - m.chrome() - 3 - 2
	if room < 0 {
		return 0
	}
	return 2 + min(len(m.reviews()), laneMaxRows, room)
}

// laneUnder reports whether the lane sits under pane pn on this screen.
func (m *Model) laneUnder(pn pane) bool {
	return m.screen == screenToday && pn == paneToday || m.screen == screenLog && pn == paneBacklog
}

// laneView renders the lane's lines for a column width cells wide.
func (m *Model) laneView(width int) []string {
	n := m.laneLines()
	if n == 0 {
		return nil
	}
	gs := m.state.GitHub
	title := "REVIEWS"
	switch {
	case len(gs.Reviews) > 0:
		title += fmt.Sprintf(" · %d waiting", len(gs.Reviews))
	case gs.LastSuccess.IsZero():
		title += " · not fetched yet" // "none waiting" would be a guess
	default:
		title += " · none waiting"
	}
	var rows []string
	for i := m.lane.offset; i < len(gs.Reviews) && len(rows) < n-2; i++ {
		rows = append(rows, m.reviewLine(gs.Reviews[i], m.inner(width), m.lane.focused && i == m.lane.cursor))
	}
	if !m.framed() {
		return append([]string{"", styleTitle.Render(title)}, rows...)
	}
	return frame(title, rows, width, n, m.lane.focused)
}

// reviewLine renders `#901 api  Add idempotency key… @ana  3d`: the repo
// without its owner, the title cut to fit, the author and the wait. A
// request with a task that isn't dropped names it (` · #11`) and is
// coloured (D51).
func (m *Model) reviewLine(i domain.SourceItem, width int, selected bool) string {
	repo := i.Repo
	if _, name, ok := strings.Cut(repo, "/"); ok {
		repo = name
	}
	right := " @" + domain.OneLine(i.Author) + "  " + waited(m.svc.Now(), i.WaitingSince())
	t, linked := m.idx.reviewTask[i.ExternalID]
	if linked {
		right += fmt.Sprintf(" · #%d", t.ID)
	}
	left := fmt.Sprintf("  #%d %s  %s", i.Number, domain.OneLine(repo), domain.OneLine(i.Title))
	line := pad(left, width-ansi.StringWidth(right)) + right
	switch {
	case selected:
		return styleSelected.Render(pad(line, width))
	case linked:
		return styleLinked.Render(line)
	}
	return line
}

// waited renders a wait compactly: "now", "5m", "2h", "3d".
func waited(now, since time.Time) string {
	d := now.Sub(since)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
}

// laneKey handles w, and every key while the lane has the focus. ok is
// false for a key the screen should handle: it also gives the focus back,
// so L, B, i, P, esc and the rest keep their screen's meaning (D49).
func (m *Model) laneKey(key string) (cmd tea.Cmd, ok bool) {
	l := &m.lane
	if !l.focused {
		if key != "w" {
			return nil, false
		}
		switch {
		case !m.state.GitHub.SetUp:
			m.setStatus("GitHub is off · [github] enabled in the config")
		case len(m.reviews()) == 0:
			m.setStatus("no reviews waiting")
		default:
			l.focused = true
		}
		return nil, true
	}
	if n := m.steps(key); n != 0 {
		repeat(n, func(delta int) { l.move(m, delta) })
		return nil, true
	}
	switch key {
	case "w", "tab":
		l.focused = false
		return nil, true
	case "p":
		return m.pinReview(), true
	case "o":
		if l.cursor < len(m.reviews()) {
			return m.openURL(m.reviews()[l.cursor].URL), true
		}
		return nil, true
	case "enter", "x", "d", "D", "e", "E", "u", "b", "m", "r", "f", "+", "=", "-":
		m.setStatus("a review request · p pins it, o opens it")
		return nil, true
	}
	l.focused = false
	return nil, false
}

// pinReview pins the request under the lane's cursor (D49): its task is
// created, or reopened, and becomes a candidate.
func (m *Model) pinReview() tea.Cmd {
	if m.lane.cursor >= len(m.reviews()) {
		return nil
	}
	i := m.reviews()[m.lane.cursor]
	if t, ok := m.idx.reviewTask[i.ExternalID]; ok && t.Status == domain.StatusOpen {
		if _, pinned := m.idx.cand[t.ID]; pinned {
			m.setStatus(fmt.Sprintf("already pinned · #%d", t.ID))
			return nil
		}
	}
	label := fmt.Sprintf("%q", domain.ReviewTitle(i))
	svc, ctx, levels := m.svc, m.ctx, m.svc.Config().Priorities
	return func() tea.Msg {
		t, undo, err := svc.PinSource(ctx, i.Kind, i.ExternalID)
		badge := strings.Trim(badgeOf(levels, t.Priority, t.Size), "[]")
		return actionMsg{label: "pin " + label, status: "pinned " + label + " · " + badge, undo: undo, err: err}
	}
}

func (m *Model) laneHints() string {
	return "j/k move · p pin · o open PR · w back · R refresh · ctrl+z undo · ? help"
}
