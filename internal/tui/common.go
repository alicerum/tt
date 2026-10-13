package tui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/app"
	"tt/internal/domain"
)

var (
	styleTitle    = lipgloss.NewStyle().Bold(true)
	styleDim      = lipgloss.NewStyle().Faint(true)
	styleSelected = lipgloss.NewStyle().Reverse(true)
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleAlert    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	// styleGhostAlert is a ghost block over a meeting (D42): selected, in the alert colour.
	styleGhostAlert = lipgloss.NewStyle().Reverse(true).Foreground(lipgloss.Color("3"))
	styleClosed     = lipgloss.NewStyle().Strikethrough(true).Faint(true)
	styleBanner     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
)

// index holds lookups derived from one loaded state.
type index struct {
	children   map[int64][]domain.Task // by parent id, in Position order
	cand       map[int64]domain.Candidate
	inInbox    map[int64]bool
	requesters []string // most used first
	tags       []string // most used first
}

func buildIndex(st app.State) index {
	idx := index{children: map[int64][]domain.Task{}, cand: map[int64]domain.Candidate{}, inInbox: map[int64]bool{}}
	reqCount, tagCount := map[string]int{}, map[string]int{}
	for _, t := range st.Snapshot.Tasks() {
		if t.ParentID != 0 {
			idx.children[t.ParentID] = append(idx.children[t.ParentID], t)
		}
		if t.Requester != "" {
			reqCount[t.Requester]++
		}
		for _, tag := range t.Tags {
			tagCount[tag]++
		}
	}
	for _, kids := range idx.children {
		slices.SortFunc(kids, func(a, b domain.Task) int {
			return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID))
		})
	}
	for _, c := range st.Plan.Candidates {
		idx.cand[c.Task.ID] = c
	}
	for _, t := range st.Snapshot.Inbox() {
		idx.inInbox[t.ID] = true
	}
	idx.requesters, idx.tags = byUse(reqCount), byUse(tagCount)
	return idx
}

func byUse(counts map[string]int) []string {
	out := make([]string, 0, len(counts))
	for k := range counts {
		out = append(out, k)
	}
	slices.SortFunc(out, func(a, b string) int { return cmp.Or(cmp.Compare(counts[b], counts[a]), cmp.Compare(a, b)) })
	return out
}

// badge renders "[P1 M]": the effective priority's configured name and the size.
func (m *Model) badge(t domain.Task) string {
	p := domain.EffectivePriority(t, m.state.Snapshot.ParentOf(t))
	name := "—"
	if levels := m.svc.Config().Priorities; p > 0 && p <= len(levels) {
		name = levels[p-1].Name
	} else if p > 0 {
		name = "P?"
	}
	size := "—"
	if t.Size != domain.SizeNone {
		size = strings.ToUpper(string(t.Size))
	}
	return "[" + name + " " + size + "]"
}

// closedToday reports whether t was closed on the plan's day (D29 wins stay visible).
func (m *Model) closedToday(t domain.Task) bool {
	return t.Status != domain.StatusOpen && !t.ClosedAt.IsZero() && m.svc.DayOf(t.ClosedAt) == m.state.Plan.Day
}

func (m *Model) headerView() string {
	name := "Log"
	switch m.screen {
	case screenToday:
		name = "Today"
	case screenInbox:
		name = "Inbox"
	case screenDetail:
		name = "Detail"
	}
	h := styleTitle.Render("tt · "+name) + " · " + dayLabel(m.state.Plan.Day)
	if n := len(m.idx.inInbox); n > 0 {
		h += styleDim.Render(fmt.Sprintf(" · inbox %d", n))
	}
	if m.state.Plan.Committed {
		h += styleDim.Render(" · plan committed")
	}
	return h + m.calendarStatus() + m.githubStatus()
}

// calendarStatus is the header's calendar part (D44): nothing while fresh,
// the age of the last success once it is older than two polls, the reason
// after a failed fetch.
func (m *Model) calendarStatus() string {
	cs, now := m.state.Calendar, m.svc.Now()
	switch {
	case !cs.SetUp:
		return styleDim.Render(" · cal: not set up (tt-cal calendars)") // D40
	case cs.Err != "":
		return styleAlert.Render(" · ⚠ cal: " + cs.Err)
	case !cs.LastSuccess.IsZero() && now.Sub(cs.LastSuccess) > 2*m.svc.Config().CalendarPoll:
		return styleDim.Render(" · cal " + ago(now, cs.LastSuccess))
	}
	return ""
}

// githubStatus is the header's GitHub part (D50), worded like the
// calendar's: nothing while fresh or when GitHub is off.
func (m *Model) githubStatus() string {
	gs, now := m.state.GitHub, m.svc.Now()
	switch {
	case !gs.SetUp:
		return ""
	case gs.Err != "":
		return styleAlert.Render(" · ⚠ gh: " + gs.Err)
	case !gs.LastSuccess.IsZero() && now.Sub(gs.LastSuccess) > 2*m.svc.Config().GitHubPoll:
		return styleDim.Render(" · gh " + ago(now, gs.LastSuccess))
	}
	return ""
}

func (m *Model) bannerView() string {
	b := m.banner
	text := "☀ New day " + dayLabel(b.Day)
	if r := b.Recap; r != nil {
		text += fmt.Sprintf(" · since %s: %d done · %d rolled over · %d carried", dayLabel(r.Since), r.Done, r.RolledOver, r.Carried)
	}
	return styleBanner.Render(text) + styleDim.Render(" · ⏎ plan now · esc later")
}

func (m *Model) statusView() string {
	if m.status == "" {
		return ""
	}
	if m.statusIsErr {
		return styleError.Render("✗ " + m.status)
	}
	return styleDim.Render(m.status)
}

func (m *Model) hintsView() string {
	if m.screen == screenToday {
		return styleDim.Render(m.todayHints())
	}
	if m.screen == screenInbox {
		if t, ok := m.state.Snapshot.Get(m.inbox.current); ok && t.ParentID != 0 {
			return styleDim.Render("s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help")
		}
		return styleDim.Render(fmt.Sprintf("1-%d priority · s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help", len(m.svc.Config().Priorities)))
	}
	if m.screen == screenDetail {
		return styleDim.Render("j/k move · ⏎ open child · a add child · J/K reorder · e edit · E note · o link · x done · d drop · p pin · esc back · ? help")
	}
	hints := "⏎ open · e edit · a add · / filter · g group · p pin · u unpin · x done · d drop · tab pane · →/← expand · B block today · esc today · i inbox · ctrl+z undo · ? help · q quit"
	if m.pick.filtered() && m.overlay == nil { // in the filter line, esc restores the previous filter
		hints = "esc clears filter · " + strings.Replace(hints, " · esc today", "", 1)
	}
	return styleDim.Render(hints)
}

func dayLabel(d domain.Day) string {
	t, err := time.Parse(time.DateOnly, string(d))
	if err != nil {
		return string(d)
	}
	return t.Format("Mon 2 Jan")
}

// ago renders a coarse relative time: "just now", "5m ago", "3h ago", "2d ago".
func ago(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	}
}

// bodyHeight is how many lines a screen's body gets: the terminal minus the
// header, status and hints lines, the banner and an open overlay (help takes
// the whole screen and doesn't count). Every screen shares this budget.
func (m *Model) bodyHeight() int {
	used := 3
	if m.banner != nil {
		used++
	}
	if _, help := m.overlay.(helpOverlay); m.overlay != nil && !help {
		used += strings.Count(m.overlay.view(m), "\n") + 1
	}
	return max(m.height-used, 2)
}

// clipLines keeps the first n lines of s.
func clipLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	return strings.Join(lines[:min(len(lines), n)], "\n")
}

// plusChildren renders " + 3 children" (D34 statuses); "" for none.
func plusChildren(n int) string {
	if n == 0 {
		return ""
	}
	return " + " + plural(n, "child", "children")
}

// plural renders "1 match" / "3 matches".
// oneLine makes text from outside (a meeting title) safe to render in a row:
// every control character (newline, tab, ESC, BEL, C1) becomes a space, runs
// of spaces collapse, the ends are trimmed.
func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)), " ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// shortDur renders 1h, 4h, 30m, 1h30m.
func shortDur(d time.Duration) string {
	s := d.String()
	s = strings.TrimSuffix(s, "0s")
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// pad truncates or right-pads s (which may contain ANSI styles) to width cells.
func pad(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if w := ansi.StringWidth(s); w > width {
		return ansi.Truncate(s, width, "…")
	} else {
		return s + strings.Repeat(" ", width-w)
	}
}

// fitWidth truncates every line of s to width cells.
func fitWidth(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if ansi.StringWidth(l) > width {
			lines[i] = ansi.Truncate(l, width, "…")
		}
	}
	return strings.Join(lines, "\n")
}
