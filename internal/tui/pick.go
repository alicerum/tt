package tui

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"tt/internal/app"
	"tt/internal/domain"
)

type pane int

const (
	paneBacklog pane = iota
	paneToday
)

type rowKind int

const (
	rowHeader rowKind = iota
	rowTask
	rowHint
)

type row struct {
	kind   rowKind
	text   string // header or hint text
	task   domain.Task
	child  bool              // indented under its parent
	cand   *domain.Candidate // set when the task is pinned
	parent bool              // has children
}

type pickState struct {
	focus    pane
	cursor   [2]int
	expanded map[int64]bool
}

// backlogRows: triaged open top-level tasks that aren't pinned, grouped by
// priority (oldest first within a group, D26). Expanded parents list their
// open, triaged, unpinned children.
func (m *Model) backlogRows() []row {
	snap := m.state.Snapshot
	levels := m.svc.Config().Priorities
	groups := map[int][]domain.Task{}
	for _, t := range snap.Tasks() {
		if t.Status != domain.StatusOpen || t.ParentID != 0 || m.idx.inInbox[t.ID] {
			continue
		}
		if _, pinned := m.idx.cand[t.ID]; pinned {
			continue
		}
		p := t.Priority
		if p > len(levels) {
			p = len(levels) + 1 // shrunk config: "P?" group at the end
		}
		groups[p] = append(groups[p], t)
	}
	var rows []row
	for p := 1; p <= len(levels)+1; p++ {
		items := groups[p]
		if len(items) == 0 {
			continue
		}
		slices.SortFunc(items, func(a, b domain.Task) int { return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID)) })
		header := "P? · above the configured levels"
		if p <= len(levels) {
			header = levels[p-1].Name + " · " + levels[p-1].Definition
		}
		rows = append(rows, row{kind: rowHeader, text: header})
		for _, t := range items {
			kids := m.idx.children[t.ID]
			rows = append(rows, row{kind: rowTask, task: t, parent: len(kids) > 0})
			if !m.pick.expanded[t.ID] {
				continue
			}
			for _, c := range kids {
				if _, pinned := m.idx.cand[c.ID]; c.Status != domain.StatusOpen || m.idx.inInbox[c.ID] || pinned {
					continue
				}
				rows = append(rows, row{kind: rowTask, task: c, child: true})
			}
		}
	}
	return rows
}

// todayRows: today's candidates in plan order. A pinned parent lists its open
// children and children closed today beneath it; a pinned child of a pinned
// parent appears only there.
func (m *Model) todayRows() []row {
	var rows []row
	for _, c := range m.state.Plan.Candidates {
		if _, parentPinned := m.idx.cand[c.Task.ParentID]; c.Task.ParentID != 0 && parentPinned {
			continue
		}
		cc := c
		kids := m.idx.children[c.Task.ID]
		rows = append(rows, row{kind: rowTask, task: c.Task, cand: &cc, parent: len(kids) > 0})
		if c.Alert {
			rows = append(rows, row{kind: rowHint, text: fmt.Sprintf("⚠ rolled over %d× → refine · split · reprioritize · hand off · drop", c.Task.RolloverCount)})
		}
		for _, k := range kids {
			if k.Status != domain.StatusOpen && !m.closedToday(k) {
				continue
			}
			var kc *domain.Candidate
			if x, ok := m.idx.cand[k.ID]; ok {
				kc = &x
			}
			rows = append(rows, row{kind: rowTask, task: k, child: true, cand: kc})
		}
	}
	return rows
}

func (m *Model) rows(p pane) []row {
	if p == paneToday {
		return m.todayRows()
	}
	return m.backlogRows()
}

// clamp keeps each pane's cursor on a selectable row.
func (p *pickState) clamp(m *Model) {
	if p.expanded == nil {
		p.expanded = map[int64]bool{}
	}
	for _, pn := range []pane{paneBacklog, paneToday} {
		rows := m.rows(pn)
		c := min(p.cursor[pn], len(rows)-1)
		for c >= 0 && rows[c].kind != rowTask {
			c--
		}
		if c < 0 {
			c = slices.IndexFunc(rows, func(r row) bool { return r.kind == rowTask })
		}
		p.cursor[pn] = max(c, 0)
	}
}

func (p *pickState) move(m *Model, delta int) {
	rows := m.rows(p.focus)
	for c := p.cursor[p.focus] + delta; c >= 0 && c < len(rows); c += delta {
		if rows[c].kind == rowTask {
			p.cursor[p.focus] = c
			return
		}
	}
}

func (m *Model) selectedRow() (row, bool) {
	rows := m.rows(m.pick.focus)
	c := m.pick.cursor[m.pick.focus]
	if c < 0 || c >= len(rows) || rows[c].kind != rowTask {
		return row{}, false
	}
	return rows[c], true
}

func (m *Model) pickKey(key string) tea.Cmd {
	p := &m.pick
	switch key {
	case "tab":
		p.focus = 1 - p.focus
		p.clamp(m)
		return nil
	case "j", "down":
		p.move(m, +1)
		return nil
	case "k", "up":
		p.move(m, -1)
		return nil
	case "i":
		m.screen = screenInbox
		m.inbox.sync(m)
		return nil
	}
	r, ok := m.selectedRow()
	if !ok {
		return nil
	}
	t := r.task
	label := fmt.Sprintf("%q", t.Title)
	switch key {
	case "right", "l":
		if p.focus == paneBacklog && r.parent {
			p.expanded[t.ID] = true
		}
	case "left", "h":
		if p.focus == paneBacklog && r.child {
			p.expanded[t.ParentID] = false
			p.clamp(m)
		} else if p.focus == paneBacklog {
			p.expanded[t.ID] = false
		}
	case "p":
		if r.cand != nil {
			m.setStatus("already pinned")
			return nil
		}
		return m.act("pin "+label, "pinned "+label, func(ctx context.Context) (app.Undo, error) { return m.svc.Pin(ctx, t.ID) })
	case "u":
		if r.cand == nil {
			m.setStatus("not pinned")
			return nil
		}
		return m.act("unpin "+label, "unpinned "+label, func(ctx context.Context) (app.Undo, error) { return m.svc.Unpin(ctx, t.ID) })
	case "x":
		if t.Status != domain.StatusOpen {
			return m.act("reopen "+label, "reopened "+label, func(ctx context.Context) (app.Undo, error) { return m.svc.Reopen(ctx, t.ID) })
		}
		return m.act("done "+label, "✓ "+label, func(ctx context.Context) (app.Undo, error) { return m.svc.Done(ctx, t.ID) })
	case "d":
		return m.openDropPrompt(t)
	case "D":
		return m.openDeleteConfirm(t)
	}
	return nil
}

func (m *Model) pickView() string {
	total := max(m.width-3, 20)
	lw := total * 55 / 100
	rw := total - lw
	left := m.paneLines(paneBacklog, "BACKLOG", "nothing triaged yet · a to add · i to triage", lw)
	right := m.paneLines(paneToday, "TODAY'S CANDIDATES", "nothing pinned · p on a backlog task", rw)
	var b strings.Builder
	for i := range max(len(left), len(right)) {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(pad(l, lw) + " │ " + pad(r, rw))
	}
	return b.String()
}

func (m *Model) paneLines(pn pane, title, empty string, width int) []string {
	lines := []string{styleTitle.Render(title)}
	rows := m.rows(pn)
	if len(rows) == 0 {
		return append(lines, styleDim.Render("  "+empty))
	}
	visible := max(m.height-6, 3)
	cursor := m.pick.cursor[pn]
	start := max(0, cursor-visible+1)
	for i := start; i < len(rows) && i < start+visible; i++ {
		lines = append(lines, m.rowLine(rows[i], width, pn == m.pick.focus && i == cursor))
	}
	return lines
}

func (m *Model) rowLine(r row, width int, selected bool) string {
	switch r.kind {
	case rowHeader:
		return styleDim.Render(r.text)
	case rowHint:
		return styleAlert.Render("    " + r.text)
	}
	t := r.task
	var b strings.Builder
	switch {
	case r.child:
		b.WriteString("    ")
	case r.parent && r.cand == nil && !m.pick.expanded[t.ID]:
		b.WriteString("▸ ")
	case r.parent:
		b.WriteString("▾ ")
	default:
		b.WriteString("  ")
	}
	b.WriteString(m.badge(t) + " " + t.Title)
	if t.Requester != "" {
		b.WriteString(" @" + t.Requester)
	}
	if kids := m.idx.children[t.ID]; len(kids) > 0 {
		done := 0
		for _, k := range kids {
			if k.Status == domain.StatusDone {
				done++
			}
		}
		fmt.Fprintf(&b, " %d/%d", done, len(kids))
	}
	if c := r.cand; c != nil {
		if c.Blocked {
			b.WriteString(" ▓")
		}
		if t.RolloverCount > 0 {
			fmt.Fprintf(&b, " ↻%d", t.RolloverCount)
		}
		if c.PinnedDays > 1 {
			fmt.Fprintf(&b, " 📌%d", c.PinnedDays)
		}
	}
	switch t.Status {
	case domain.StatusDone:
		b.WriteString(" ✓")
	case domain.StatusDropped:
		b.WriteString(" ✗")
	}
	line := pad(b.String(), width)
	if t.Status != domain.StatusOpen {
		line = styleClosed.Render(line)
	}
	if selected {
		line = styleSelected.Render(line)
	}
	return line
}
