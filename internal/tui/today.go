package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/domain"
)

type todayFocus int

const (
	focusTimeline todayFocus = iota
	focusCandidates
)

// todayState is the Today screen (D35): the timeline (D38) beside today's
// candidates. The candidates pane shares the Log's candidates rows and cursor.
type todayState struct {
	focus  todayFocus
	tl     timeline
	at     time.Duration // start of the row under the timeline cursor; survives reloads
	placed bool          // at has been set; until then the cursor goes to the now row
	cursor int           // the row index of at
	offset int           // first timeline line shown (the now line counts as one)
}

// refresh rebuilds the timeline from the loaded plan and puts the cursor
// back on its row (the window may have grown or shrunk).
func (td *todayState) refresh(m *Model) {
	cfg, plan := m.svc.Config(), m.state.Plan
	now := time.Duration(-1)
	if m.svc.Today() == plan.Day {
		now = plan.Day.Offset(m.svc.Now())
	}
	td.tl = buildTimeline(plan.Blocks, cfg.TimelineStart, cfg.TimelineEnd, now, cfg.DayStart)
	rows := td.tl.rows
	if !td.placed {
		td.at, td.placed = rows[0].start, true
		if td.tl.nowRow >= 0 {
			td.at = rows[td.tl.nowRow].start
		}
	}
	td.cursor = td.tl.rowOf(td.at)
	if td.cursor < 0 { // the window shrank past the cursor
		td.cursor = 0
		if td.at > rows[0].start {
			td.cursor = len(rows) - 1
		}
	}
	td.at = rows[td.cursor].start
}

func (td *todayState) move(delta int) {
	td.cursor = max(0, min(td.cursor+delta, len(td.tl.rows)-1))
	td.at = td.tl.rows[td.cursor].start
}

// line is the timeline line row i is drawn on: the now line comes right
// after the row holding now.
func (td *todayState) line(i int) int {
	if n := td.tl.nowRow; n >= 0 && i > n {
		return i + 1
	}
	return i
}

func (td *todayState) lines() int {
	if td.tl.nowRow >= 0 {
		return len(td.tl.rows) + 1
	}
	return len(td.tl.rows)
}

// follow keeps the cursor's row inside the timeline's scroll window.
func (td *todayState) follow(m *Model) {
	if len(td.tl.rows) == 0 {
		return
	}
	visible, l := m.visibleRows(), td.line(td.cursor)
	off := min(td.offset, l)
	if l >= off+visible {
		off = l - visible + 1
	}
	td.offset = max(0, min(off, td.lines()-visible))
}

// blockKeys act on the block under the timeline cursor.
var blockKeys = map[string]bool{"enter": true, "x": true, "d": true, "D": true, "e": true, "E": true, "o": true}

func (m *Model) todayKey(key string) tea.Cmd {
	td := &m.today
	switch key {
	case "tab":
		td.focus = 1 - td.focus
		return nil
	case "L":
		m.screen = screenLog
		return nil
	case "i":
		m.openInbox()
		return nil
	}
	if td.focus == focusCandidates {
		switch key {
		case "j", "down":
			m.pick.moveIn(paneToday, +1)
			return nil
		case "k", "up":
			m.pick.moveIn(paneToday, -1)
			return nil
		}
		r, ok := m.selectedIn(paneToday)
		if !ok {
			return nil
		}
		if key == "enter" {
			m.openDetail(r.task.ID) // D31
			return nil
		}
		cmd, _ := m.taskKey(key, r.task)
		return cmd
	}
	switch key {
	case "j", "down":
		td.move(+1)
		return nil
	case "k", "up":
		td.move(-1)
		return nil
	}
	if !blockKeys[key] {
		return nil
	}
	b, ok := td.tl.blockAt(td.cursor)
	if !ok {
		m.setStatus("nothing blocked here · tab to the candidates")
		return nil
	}
	t, ok := m.state.Snapshot.Get(b.TaskID)
	if !ok {
		return nil
	}
	if key == "enter" {
		m.openDetail(t.ID)
		return nil
	}
	cmd, _ := m.taskKey(key, t)
	return cmd
}

func (m *Model) todayView() string {
	total := max(m.width-3, 20)
	lw := total / 2
	rw := total - lw
	left := m.timelineLines(lw)
	right := m.paneLines(paneToday, "CANDIDATES", "nothing pinned · L opens the log to pick", rw, m.today.focus == focusCandidates)
	return columns(left, right, lw, rw)
}

// timelineLines renders the timeline's title and the lines in its window.
func (m *Model) timelineLines(width int) []string {
	td := &m.today
	title := "TIMELINE"
	if n := len(td.tl.blocks); n > 0 {
		done := 0
		for _, b := range td.tl.blocks {
			if t, ok := m.state.Snapshot.Get(b.TaskID); ok && t.Status == domain.StatusDone {
				done++
			}
		}
		title += fmt.Sprintf(" · %d/%d done", done, n)
	}
	var all []string
	for i := range td.tl.rows {
		all = append(all, m.timelineRow(i, width))
		if i == td.tl.nowRow {
			now := "━━ now " + m.svc.Now().Format("15:04") + " "
			all = append(all, styleAlert.Render(now+strings.Repeat("━", max(width-ansi.StringWidth(now), 0))))
		}
	}
	end := min(len(all), td.offset+m.visibleRows())
	return append([]string{styleTitle.Render(title)}, all[min(td.offset, end):end]...)
}

// timelineRow renders row i: its time, then the blocks starting in it
// (▓ title [P S]), or ▓ where a block carries on from an earlier row.
func (m *Model) timelineRow(i, width int) string {
	td := &m.today
	r := td.tl.rows[i]
	var labels []string
	carried := false
	for _, b := range r.blocks {
		if b.Start >= r.start {
			labels = append(labels, m.blockLabel(b))
		} else {
			carried = true
		}
	}
	text := " " + clock(r.start) + " "
	switch {
	case len(labels) > 0:
		text += "▓ " + strings.Join(labels, " · ")
	case carried:
		text += "▓"
	}
	line := pad(text, width)
	if now := td.tl.now; now >= 0 && r.start+rowLen <= now {
		line = styleDim.Render(line) // the past
	}
	if td.focus == focusTimeline && i == td.cursor {
		line = styleSelected.Render(line)
	}
	return line
}

// blockLabel is a block's text on its first row: exact times when it is off
// the half-hour grid, the task's title and badge, struck through when closed.
func (m *Model) blockLabel(b domain.Block) string {
	t, ok := m.state.Snapshot.Get(b.TaskID)
	if !ok {
		return fmt.Sprintf("#%d", b.TaskID)
	}
	s := t.Title + " " + m.badge(t)
	if b.Start%rowLen != 0 || b.Duration%rowLen != 0 {
		s = clock(b.Start) + "–" + clock(b.End()) + " " + s
	}
	switch t.Status {
	case domain.StatusDone:
		return styleClosed.Render(s + " ✓")
	case domain.StatusDropped:
		return styleClosed.Render(s + " ✗")
	}
	return s
}

// clock renders a wall-clock offset as "09:30"; offsets past 24h wrap.
func clock(off time.Duration) string {
	minutes := int(off / time.Minute)
	return fmt.Sprintf("%02d:%02d", minutes/60%24, minutes%60)
}

func (m *Model) todayHints() string {
	if m.today.focus == focusCandidates {
		return "⏎ open · u unpin · x done · d drop · e edit · tab timeline · L log · i inbox · a add · ctrl+z undo · ? help · q quit"
	}
	return "j/k move · ⏎ open · x done · e edit · tab candidates · L log · i inbox · a add · ctrl+z undo · ? help · q quit"
}

var todayHelp = []string{
	"tab      switch timeline / candidates",
	"j k      move",
	"⏎        task detail (the block's task on the timeline)",
	"x d D    done (again: reopen) / drop / delete",
	"e / E    edit the line / the note in $EDITOR · o open the link",
	"u        on a candidate: unpin",
	"L        the log (backlog; pick today's work there, B comes back) · i inbox",
}
