package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"tt/internal/domain"
)

// notePreviewLines is how much of a note Task detail shows (view 5).
const notePreviewLines = 6

// detailState is Task detail (view 5): a stack of open tasks (a child on top
// of its parent) and a cursor over the top task (row 0) and its children.
type detailState struct {
	stack    []int64
	rows     []domain.Task // the task, then its children: open ones in sibling order, then closed ones
	cursor   int
	selected int64 // task id under the cursor; survives reloads
	offset   int   // first child shown (scroll window)
}

func (d *detailState) top() int64 {
	if len(d.stack) == 0 {
		return 0
	}
	return d.stack[len(d.stack)-1]
}

// openDetail shows task id's detail, starting a new stack (from Pick).
func (m *Model) openDetail(id int64) {
	m.detail = detailState{stack: []int64{id}, selected: id}
	m.screen = screenDetail
	m.detail.refresh(m)
}

// refresh rebuilds the rows. A task that disappeared falls back one level
// (to Pick when none is left) and says so.
func (d *detailState) refresh(m *Model) {
	snap := m.state.Snapshot
	for len(d.stack) > 0 {
		if _, ok := snap.Get(d.top()); ok {
			break
		}
		if m.screen == screenDetail {
			m.setStatus(fmt.Sprintf("#%d is gone", d.top()))
		}
		d.selected = d.top()
		d.stack = d.stack[:len(d.stack)-1]
		m.overlay = nil // it was bound to the task that is gone
	}
	if len(d.stack) == 0 {
		d.rows = nil
		if m.screen == screenDetail {
			m.screen = screenPick
		}
		return
	}
	t, _ := snap.Get(d.top())
	var open, closed []domain.Task
	for _, c := range m.idx.children[t.ID] { // sibling order
		if c.Status == domain.StatusOpen {
			open = append(open, c)
		} else {
			closed = append(closed, c)
		}
	}
	d.rows = append(append([]domain.Task{t}, open...), closed...)
	if c := slices.IndexFunc(d.rows, func(r domain.Task) bool { return r.ID == d.selected }); c >= 0 {
		d.cursor = c
	} else {
		d.cursor = min(d.cursor, len(d.rows)-1) // the selected child is gone: stay put
	}
	d.selected = d.rows[d.cursor].ID
}

func (d *detailState) move(delta int) {
	d.cursor = max(0, min(d.cursor+delta, len(d.rows)-1))
	d.selected = d.rows[d.cursor].ID
}

// childRows is how many children fit below the task's own lines.
func (m *Model) childRows() int {
	reserved := 4 + len(m.detailHead())
	if m.banner != nil {
		reserved++
	}
	if m.overlay != nil {
		reserved += strings.Count(m.overlay.view(m), "\n") + 1
	}
	return max(m.height-reserved, 3)
}

// follow keeps the cursor's child inside the scroll window (see pickState.follow).
func (d *detailState) follow(m *Model) {
	if len(d.rows) == 0 {
		return
	}
	visible, kids := m.childRows(), len(d.rows)-1
	c := d.cursor - 1 // the cursor's child index; -1 on the task itself
	off := min(d.offset, max(c, 0))
	if c >= off+visible {
		off = c - visible + 1
	}
	d.offset = max(0, min(off, kids-visible))
}

func (m *Model) detailKey(key string) tea.Cmd {
	d := &m.detail
	switch key {
	case "j", "down":
		d.move(+1)
		return nil
	case "k", "up":
		d.move(-1)
		return nil
	case "esc":
		from := d.top()
		d.stack = d.stack[:len(d.stack)-1]
		if len(d.stack) == 0 {
			m.detail = detailState{}
			m.screen = screenPick
			return nil
		}
		d.selected, d.offset = from, 0 // back on the child we came from
		d.refresh(m)
		return nil
	}
	if len(d.rows) == 0 {
		return nil
	}
	t := d.rows[d.cursor]
	if key == "J" || key == "K" {
		if d.cursor == 0 {
			m.setStatus("J/K move a child · j selects one")
			return nil
		}
		delta := +1
		if key == "K" {
			delta = -1
		}
		return m.moveChild(t, delta)
	}
	if key == "enter" {
		if d.cursor == 0 {
			return nil
		}
		d.stack = append(d.stack, t.ID)
		d.selected, d.offset = t.ID, 0
		d.refresh(m)
		return nil
	}
	cmd, _ := m.taskKey(key, t)
	return cmd
}

// detailHead renders the task's own lines: header, facts, link, note preview
// and the children heading. The first line is the cursor's row 0.
func (m *Model) detailHead() []string {
	if len(m.detail.rows) == 0 {
		return nil
	}
	t := m.detail.rows[0]
	snap := m.state.Snapshot
	head := fmt.Sprintf("#%d %s  %s", t.ID, t.Title, m.badge(t))
	if t.RolloverCount > 0 {
		head += fmt.Sprintf(" ↻%d", t.RolloverCount)
	}
	if c, ok := m.idx.cand[t.ID]; ok {
		head += " 📌"
		if c.PinnedDays > 1 {
			head += fmt.Sprint(c.PinnedDays)
		}
	}
	switch t.Status {
	case domain.StatusDone:
		head += " ✓ " + dayLabel(m.svc.DayOf(t.ClosedAt))
	case domain.StatusDropped:
		head += " ✗ " + dayLabel(m.svc.DayOf(t.ClosedAt))
	}
	lines := []string{head}

	var facts []string
	if p := snap.ParentOf(t); p != nil {
		facts = append(facts, fmt.Sprintf("child of #%d %s", p.ID, p.Title))
	}
	if t.Requester != "" {
		facts = append(facts, "@"+t.Requester)
	}
	if tags := domain.EffectiveTags(t, snap.ParentOf(t)); len(tags) > 0 {
		facts = append(facts, "#"+strings.Join(tags, " #"))
	}
	facts = append(facts, "captured "+ago(m.svc.Now(), t.CreatedAt))
	if t.Status == domain.StatusDropped && t.DropReason != "" {
		facts = append(facts, "dropped: "+t.DropReason)
	}
	lines = append(lines, styleDim.Render(strings.Join(facts, " · ")))
	if t.Link != "" {
		lines = append(lines, "🔗 "+t.Link)
	}

	lines = append(lines, "", styleTitle.Render("NOTE"))
	if t.Note == "" {
		lines = append(lines, styleDim.Render("  no note · E to write one"))
	} else {
		note := strings.Split(t.Note, "\n")
		for _, l := range note[:min(len(note), notePreviewLines)] {
			lines = append(lines, "  "+l)
		}
		if extra := len(note) - notePreviewLines; extra > 0 {
			lines = append(lines, styleDim.Render(fmt.Sprintf("  … +%d lines · E edit in $EDITOR", extra)))
		}
	}

	if t.ParentID == 0 {
		lines = append(lines, "", styleTitle.Render("CHILDREN")+m.progress(t))
	}
	return lines
}

// progress renders " ████░░░░ 2/4" for a parent; "" when nothing counts.
func (m *Model) progress(t domain.Task) string {
	done, total := 0, 0
	for _, k := range m.idx.children[t.ID] {
		if k.Status == domain.StatusDropped {
			continue
		}
		total++
		if k.Status == domain.StatusDone {
			done++
		}
	}
	if total == 0 {
		return ""
	}
	const cells = 8
	filled := done * cells / total
	return fmt.Sprintf(" %s%s %d/%d", strings.Repeat("█", filled), strings.Repeat("░", cells-filled), done, total)
}

func (m *Model) detailView() string {
	d := &m.detail
	head := m.detailHead()
	if len(head) == 0 {
		return ""
	}
	if d.cursor == 0 {
		head[0] = styleSelected.Render(pad(head[0], m.width))
	}
	lines := head
	if d.rows[0].ParentID == 0 {
		kids := d.rows[1:]
		if len(kids) == 0 {
			lines = append(lines, styleDim.Render("  no children · a adds one"))
		}
		visible := m.childRows()
		for i := d.offset; i < len(kids) && i < d.offset+visible; i++ {
			lines = append(lines, m.childLine(kids[i], d.cursor == i+1))
		}
		if more := len(kids) - d.offset - visible; more > 0 {
			lines = append(lines, styleDim.Render(fmt.Sprintf("  … %d more", more)))
		}
	}
	return strings.Join(lines, "\n")
}

func (m *Model) childLine(c domain.Task, selected bool) string {
	line := "  " + m.badge(c) + " " + c.Title
	if c.Note != "" {
		line += " ✎"
	}
	if _, pinned := m.idx.cand[c.ID]; pinned {
		line += " 📌"
	}
	switch c.Status {
	case domain.StatusDone:
		line += " ✓ " + dayLabel(m.svc.DayOf(c.ClosedAt))
	case domain.StatusDropped:
		line += " ✗ " + dayLabel(m.svc.DayOf(c.ClosedAt))
	}
	line = pad(line, m.width)
	if c.Status != domain.StatusOpen {
		line = styleClosed.Render(line)
	}
	if selected {
		line = styleSelected.Render(line)
	}
	return line
}
