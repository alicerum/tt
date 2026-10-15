package tui

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

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
	// styleLinked is a review request that already has a task (D51).
	styleLinked = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	// styleScoreLow is the green of a middling urgency score; yellow
	// (styleAlert) and red (styleError) take the higher bands.
	styleScoreLow = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleBanner   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
)

// index holds lookups derived from one loaded state.
type index struct {
	children   map[int64][]domain.Task // by parent id, in Position order
	cand       map[int64]domain.Candidate
	inInbox    map[int64]bool
	requesters []string // most used first
	people     []person // every requester some task carries, alphabetical (D52)
	tags       []string // most used first
	// reviewTask is the task of each review request that has one and didn't
	// drop it, by external id (D51).
	reviewTask map[string]domain.Task
}

func buildIndex(st app.State) index {
	idx := index{children: map[int64][]domain.Task{}, cand: map[int64]domain.Candidate{}, inInbox: map[int64]bool{},
		reviewTask: map[string]domain.Task{}}
	reqCount, tagCount, openCount := map[string]int{}, map[string]int{}, map[string]int{}
	for _, t := range st.Snapshot.Tasks() {
		if t.Source != nil && t.Source.Kind == domain.SourceGitHub && t.Status != domain.StatusDropped {
			idx.reviewTask[t.Source.ID] = t
		}
		if t.ParentID != 0 {
			idx.children[t.ParentID] = append(idx.children[t.ParentID], t)
		}
		if t.Requester != "" {
			reqCount[t.Requester]++
			if t.Status == domain.StatusOpen {
				openCount[t.Requester]++
			}
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
	for _, name := range slices.Sorted(maps.Keys(reqCount)) {
		idx.people = append(idx.people, person{name: name, open: openCount[name]})
	}
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

// badge renders "[73 M]": the task's urgency today and its size (D71), the
// score in its band's colour while the task is open.
func (m *Model) badge(t domain.Task) string {
	u, ok := m.urgency(t, m.state.Snapshot.ParentOf(t))
	score := scoreText(u, ok)
	if ok && t.Status == domain.StatusOpen {
		score = scoreStyle(domain.UrgencyScore(u)).Render(score)
	}
	return "[" + score + " " + sizeLabel(t.Size) + "]"
}

// plainBadge is badge without colour, for text that takes a colour of its
// own (a timeline block, D58).
func (m *Model) plainBadge(t domain.Task) string {
	u, ok := m.urgency(t, m.state.Snapshot.ParentOf(t))
	return badgeOf(u, ok, t.Size)
}

// urgency is t's urgency today (D70); ok is false while t is untriaged.
func (m *Model) urgency(t domain.Task, parent *domain.Task) (float64, bool) {
	return domain.Urgency(t, parent, m.svc.Today(), m.svc.Config().Urgency)
}

// badgeOf renders "[73 M]" for urgency u (0 to 1; ok false: " —") and a
// size, without colour.
func badgeOf(u float64, ok bool, size domain.Size) string {
	return "[" + scoreText(u, ok) + " " + sizeLabel(size) + "]"
}

// scoreText is urgency u as two cells, so every badge is as wide: 100 shows
// as 99, an untriaged task (ok false) as " —".
func scoreText(u float64, ok bool) string {
	if !ok {
		return " —"
	}
	return fmt.Sprintf("%2d", min(domain.UrgencyScore(u), 99))
}

// scoreStyle is the colour of an urgency score's band: plain under 30,
// green to 49, yellow to 74, red from 75.
func scoreStyle(score int) lipgloss.Style {
	switch {
	case score >= 75:
		return styleError
	case score >= 50:
		return styleAlert
	case score >= 30:
		return styleScoreLow
	}
	return lipgloss.NewStyle()
}

// dueMarker is the marker after an open task's title when it has a due
// (D71), worded against today: "→today"; "→fri" within the next 6 days;
// "→30 oct" later, with the year when it is more than a year away; "2d late"
// once overdue, in the alert colour. "" without a due.
func dueMarker(today, due domain.Day) string {
	if due == "" {
		return ""
	}
	date, err := time.Parse(time.DateOnly, string(due))
	if err != nil {
		return ""
	}
	d := domain.DaysBetween(today, due)
	switch {
	case d < 0:
		return styleAlert.Render(fmt.Sprintf("%dd late", -d))
	case d == 0:
		return "→today"
	case d < 7:
		return "→" + strings.ToLower(date.Format("Mon"))
	case d > 365:
		return "→" + strings.ToLower(date.Format("2 Jan 2006"))
	}
	return "→" + strings.ToLower(date.Format("2 Jan"))
}

// due is t's due marker today (D71): its effective due, open tasks only.
func (m *Model) due(t domain.Task) string {
	if t.Status != domain.StatusOpen {
		return ""
	}
	return dueMarker(m.svc.Today(), domain.EffectiveDue(t, m.state.Snapshot.ParentOf(t)))
}

// sizeLabel is "M" for a size, "—" for none.
func sizeLabel(size domain.Size) string {
	s := "—"
	if size != domain.SizeNone {
		s = strings.ToUpper(string(size))
	}
	return s
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
	case screenPeople:
		name = "People"
	}
	h := styleTitle.Render("tt · "+name) + " · " + dayLabel(m.state.Plan.Day)
	if n := len(m.idx.inInbox); n > 0 {
		h += styleDim.Render(fmt.Sprintf(" · inbox %d", n))
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

// hintsView is the footer's hints line, cut to the width with ? help kept (D55).
func (m *Model) hintsView() string { return styleDim.Render(cutHints(m.hintsText(), m.width)) }

// hintsText is the current screen's key hints, before cutHints fits them.
func (m *Model) hintsText() string {
	lane := m.screen == screenToday || m.screen == screenLog
	if lane && m.lane.focused {
		return m.laneHints()
	}
	reviews := ""
	if lane && len(m.reviews()) > 0 {
		reviews = "w reviews · " // D49
	}
	if m.screen == screenToday {
		return reviews + m.todayHints()
	}
	if m.screen == screenInbox {
		if t, ok := m.state.Snapshot.Get(m.inbox.current); ok && t.ParentID != 0 {
			return "^ due · s/m/l/x size · ⏎ next · n skip · d drop · esc back · a add · ctrl+z undo · ? help"
		}
		return "^ due · 1-3 impact · s/m/l/x size · ⏎ next · n skip · d drop · esc back · a add · ctrl+z undo · ? help"
	}
	if m.screen == screenPeople {
		return m.peopleHints()
	}
	if m.screen == screenDetail {
		return "j/k move · ⏎ open child · a add child · J/K reorder · e edit · E note · o link · x done · d drop · p pin · u unpin · D delete · esc back · ctrl+z undo · ? help"
	}
	hints := "⏎ open · e edit · E note · o link · a add · / filter · g group · p pin · u unpin · x done · d drop · tab pane · →/← expand · B block today · esc today · i inbox · P people · ctrl+z undo · ? help · q quit"
	if m.pick.filtered() && m.overlay == nil { // in the filter line, esc restores the previous filter
		hints = "esc clears filter · " + strings.Replace(hints, " · esc today", "", 1)
	}
	return reviews + hints
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
	_, help := m.overlay.(helpOverlay)
	if _, inPane := m.overlay.(paneOverlay); m.overlay != nil && !help && !inPane {
		used += strings.Count(m.overlay.view(m), "\n") + 1
	}
	return max(m.height-used, 1)
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
	if ansi.StringWidth(s) > width {
		s = ansi.Truncate(s, width, "…")
	}
	// a wide character on the cut can leave the cut one cell short
	return s + strings.Repeat(" ", max(width-ansi.StringWidth(s), 0))
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
