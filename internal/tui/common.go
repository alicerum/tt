package tui

import (
	"cmp"
	"fmt"
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
	styleClosed   = lipgloss.NewStyle().Strikethrough(true).Faint(true)
	styleBanner   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
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
	name := "Pick"
	if m.screen == screenInbox {
		name = "Inbox"
	}
	h := styleTitle.Render("tt · "+name) + " · " + dayLabel(m.state.Plan.Day)
	if n := len(m.idx.inInbox); n > 0 {
		h += styleDim.Render(fmt.Sprintf(" · inbox %d", n))
	}
	if m.state.Plan.Committed {
		h += styleDim.Render(" · plan committed")
	}
	return h
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
	if m.screen == screenInbox {
		if t, ok := m.state.Snapshot.Get(m.inbox.current); ok && t.ParentID != 0 {
			return styleDim.Render("s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help")
		}
		return styleDim.Render(fmt.Sprintf("1-%d priority · s/m/l/x size · n skip · d drop · esc back · a add · ctrl+z undo · ? help", len(m.svc.Config().Priorities)))
	}
	return styleDim.Render("a add · p pin · u unpin · x done · d drop · tab pane · →/← expand · i inbox · ctrl+z undo · ? help · q quit")
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
