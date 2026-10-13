package tui

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"tt/internal/app"
	"tt/internal/domain"
	"tt/internal/dsl"
	"tt/internal/search"
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
	kind     rowKind
	text     string // header or hint text
	task     domain.Task
	child    bool              // indented under its parent
	cand     *domain.Candidate // set when the task is pinned
	parent   bool              // has children
	expanded bool              // a parent whose children are listed beneath it
	orphan   int64             // child listed top-level because this parent isn't shown in the backlog
}

// groupKey is the ui_state key that remembers the backlog grouping (D26).
const groupKey = "pick.group"

type pickState struct {
	focus       pane
	cursor      [2]int
	offset      [2]int   // first row shown in each pane (scroll window)
	selected    [2]int64 // task id under each pane's cursor (0 = none); survives reloads
	expanded    map[int64]bool
	rows        [2][]row // built by refreshRows, read by keys and render
	group       search.GroupBy
	groupChosen bool       // g was pressed: the remembered grouping read at startup no longer applies
	saving      bool       // a grouping save is in flight
	saveAgain   bool       // g was pressed again meanwhile: save the latest when it lands
	filterText  string     // the filter as typed (trimmed); "" = none
	filter      dsl.Filter // parsed filterText
}

func (p *pickState) filtered() bool { return !p.filter.Empty() }

// refreshRows rebuilds both panes' rows: after a load, a filter or grouping
// change, and expand/collapse. Render and keys only read the cache.
func (m *Model) refreshRows() {
	m.pick.rows[paneBacklog] = m.backlogRows()
	m.pick.rows[paneToday] = m.todayRows()
	m.pick.clamp(m)
	m.today.refresh(m)
	if len(m.detail.stack) > 0 {
		m.detail.refresh(m)
	}
}

// applyFilter makes text the backlog filter (D26a). On a parse error the
// current filter stays and the error is returned.
func (m *Model) applyFilter(text string) error {
	f, err := dsl.ParseFilter(text, len(m.svc.Config().Priorities))
	if err != nil {
		return err
	}
	m.pick.filterText, m.pick.filter = strings.TrimSpace(text), f
	m.refreshRows()
	return nil
}

// backlogRows is the one placement rule for the backlog pane: triaged,
// unpinned tasks, never the Inbox's or Today's (D26c). Unfiltered it lists the
// open ones; filtered, the ones the filter matches (open by default, closed
// with is:). A listed child sits under its listed parent (unfiltered: while
// the parent is expanded; filtered: always). A child whose parent isn't listed
// (closed, in the Inbox, missing, or not matching) is listed top-level with
// ↳ #parent so it never vanishes; the children of a pinned parent live under
// it in Today instead. Top-level tasks are grouped (D26), oldest first within
// a group.
func (m *Model) backlogRows() []row {
	snap := m.state.Snapshot
	now := m.svc.Now()
	filtered := m.pick.filtered()
	listed := map[int64]bool{}
	for _, t := range snap.Tasks() {
		if _, pinned := m.idx.cand[t.ID]; pinned || m.idx.inInbox[t.ID] {
			continue // Today and the Inbox show these
		}
		if filtered && search.Match(m.pick.filter, t, snap.ParentOf(t), now) || !filtered && t.Status == domain.StatusOpen {
			listed[t.ID] = true
		}
	}
	levels := m.svc.Config().Priorities
	headings := make([]string, len(levels))
	for i, l := range levels {
		headings[i] = l.Name + " · " + l.Definition
	}
	groups := map[search.Group][]domain.Task{}
	orphanOf := map[int64]int64{}
	for _, t := range snap.Tasks() {
		if !listed[t.ID] {
			continue
		}
		if t.ParentID != 0 {
			if listed[t.ParentID] {
				continue // listed under its parent
			}
			if _, pinned := m.idx.cand[t.ParentID]; pinned {
				continue // listed under its parent in Today
			}
			orphanOf[t.ID] = t.ParentID
		}
		g := search.GroupOf(m.pick.group, t, snap.ParentOf(t), now, headings)
		groups[g] = append(groups[g], t)
	}
	var rows []row
	for _, g := range slices.SortedFunc(maps.Keys(groups), search.Compare) {
		items := groups[g]
		slices.SortFunc(items, search.Oldest)
		rows = append(rows, row{kind: rowHeader, text: g.Label})
		for _, t := range items {
			var kids []domain.Task
			for _, c := range m.idx.children[t.ID] {
				if listed[c.ID] {
					kids = append(kids, c)
				}
			}
			// parent: only listed children count, so ▸ always has something to show.
			open := len(kids) > 0 && (filtered || m.pick.expanded[t.ID])
			rows = append(rows, row{kind: rowTask, task: t, parent: len(kids) > 0, expanded: open, orphan: orphanOf[t.ID]})
			if !open {
				continue
			}
			for _, c := range kids {
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
		rows = append(rows, row{kind: rowTask, task: c.Task, cand: &cc, parent: len(kids) > 0, expanded: len(kids) > 0})
		if c.Alert {
			rows = append(rows, rolloverHint(c.Task))
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
			if kc != nil && kc.Alert {
				rows = append(rows, rolloverHint(k))
			}
		}
	}
	return rows
}

func rolloverHint(t domain.Task) row {
	return row{kind: rowHint, text: fmt.Sprintf("⚠ rolled over %d× → refine · split · reprioritize · hand off · drop", t.RolloverCount)}
}

func (m *Model) rows(p pane) []row { return m.pick.rows[p] }

// clamp keeps each pane's cursor on a selectable row. It prefers the task the
// cursor was on (so a reload that reorders rows keeps the highlight on that
// task) and falls back to the old index when that task is gone.
func (p *pickState) clamp(m *Model) {
	if p.expanded == nil {
		p.expanded = map[int64]bool{}
	}
	for _, pn := range []pane{paneBacklog, paneToday} {
		rows := p.rows[pn]
		c := -1
		if id := p.selected[pn]; id != 0 {
			c = slices.IndexFunc(rows, func(r row) bool { return r.kind == rowTask && r.task.ID == id })
		}
		if c < 0 {
			c = min(p.cursor[pn], len(rows)-1)
			for c >= 0 && rows[c].kind != rowTask {
				c--
			}
			if c < 0 {
				c = slices.IndexFunc(rows, func(r row) bool { return r.kind == rowTask })
			}
			c = max(c, 0)
		}
		p.cursor[pn] = c
		p.selected[pn] = 0
		if c < len(rows) && rows[c].kind == rowTask {
			p.selected[pn] = rows[c].task.ID
		}
	}
}

func (p *pickState) move(delta int) { p.moveIn(p.focus, delta) }

// moveIn moves pane pn's cursor to the next task row in direction delta.
func (p *pickState) moveIn(pn pane, delta int) {
	rows := p.rows[pn]
	for c := p.cursor[pn] + delta; c >= 0 && c < len(rows); c += delta {
		if rows[c].kind == rowTask {
			p.cursor[pn] = c
			p.selected[pn] = rows[c].task.ID
			return
		}
	}
}

// saveGroup remembers the grouping (D26). One save runs at a time; a g
// pressed meanwhile is saved when it lands, so the last choice wins.
func (m *Model) saveGroup() tea.Cmd {
	p := &m.pick
	if p.saving {
		p.saveAgain = true
		return nil
	}
	p.saving = true
	svc, ctx, g := m.svc, m.ctx, p.group
	return func() tea.Msg { return uiSavedMsg{err: svc.SetUIState(ctx, groupKey, string(g))} }
}

// follow keeps each pane's cursor inside its scroll window, moving the window
// only when the cursor leaves it; a group heading right above the cursor
// comes along. Called after every update.
func (p *pickState) follow(m *Model) {
	visible := m.visibleRows()
	for pn, rows := range p.rows {
		c, off := p.cursor[pn], p.offset[pn]
		top := c
		if top > 0 && top < len(rows) && rows[top-1].kind == rowHeader {
			top--
		}
		off = min(off, top)
		if c >= off+visible {
			off = c - visible + 1
		}
		p.offset[pn] = max(0, min(off, len(rows)-visible))
	}
}

// visibleRows is how many rows fit in a pane below its title (the Log's
// panes, Today's candidates and timeline share this budget).
func (m *Model) visibleRows() int { return max(m.bodyHeight()-1, 1) }

// steps is how far a list key moves the cursor: a row for j/k, half the
// visible rows for ctrl+d/ctrl+u (as in vim); 0 for any other key.
func (m *Model) steps(key string) int {
	switch key {
	case "j", "down":
		return 1
	case "k", "up":
		return -1
	case "ctrl+d":
		return max(m.visibleRows()/2, 1)
	case "ctrl+u":
		return -max(m.visibleRows()/2, 1)
	}
	return 0
}

// repeat moves n rows one step at a time, so headers are skipped and the
// list's ends stop it, as with j/k.
func repeat(n int, step func(delta int)) {
	delta := 1
	if n < 0 {
		n, delta = -n, -1
	}
	for range n {
		step(delta)
	}
}

func (m *Model) selectedRow() (row, bool) { return m.selectedIn(m.pick.focus) }

// selectedIn is the task row under pane pn's cursor.
func (m *Model) selectedIn(pn pane) (row, bool) {
	rows := m.rows(pn)
	c := m.pick.cursor[pn]
	if c < 0 || c >= len(rows) || rows[c].kind != rowTask {
		return row{}, false
	}
	return rows[c], true
}

func (m *Model) pickKey(key string) tea.Cmd {
	p := &m.pick
	if n := m.steps(key); n != 0 {
		repeat(n, p.move)
		return nil
	}
	switch key {
	case "tab":
		p.focus = 1 - p.focus
		p.clamp(m)
		return nil
	case "i":
		m.openInbox()
		return nil
	case "B":
		m.screen, m.today.focus = screenToday, focusCandidates // Pick done: block in Today (D35)
		return nil
	case "/":
		return m.openFilter()
	case "g":
		p.group, p.groupChosen = p.group.Next(), true
		m.refreshRows()
		m.setStatus("grouped by " + string(p.group))
		return m.saveGroup()
	case "esc":
		if p.filtered() {
			_ = m.applyFilter("") // an empty line always parses
			m.setStatus("filter cleared")
			return nil
		}
		m.screen = screenToday
		return nil
	}
	r, ok := m.selectedRow()
	if !ok {
		return nil
	}
	t := r.task
	switch key {
	case "enter":
		m.openDetail(t.ID) // D31
	case "right", "l":
		if p.focus == paneBacklog && r.parent && !p.filtered() { // filtered parents are always expanded
			p.expanded[t.ID] = true
			m.refreshRows()
		}
	case "left", "h":
		switch {
		case p.focus != paneBacklog:
		case r.child:
			if !p.filtered() { // the filter mustn't change the unfiltered expansion
				p.expanded[t.ParentID] = false
			}
			p.selected[paneBacklog] = t.ParentID // the parent takes the cursor
			m.refreshRows()
		case !p.filtered():
			p.expanded[t.ID] = false
			m.refreshRows()
		}
	default:
		cmd, _ := m.taskKey(key, t)
		return cmd
	}
	return nil
}

// taskKey runs the keys that act on one task the same way wherever it is
// shown (Pick panes, Task detail). ok is false for any other key.
func (m *Model) taskKey(key string, t domain.Task) (cmd tea.Cmd, ok bool) {
	svc := m.svc
	label := fmt.Sprintf("%q", t.Title)
	_, pinned := m.idx.cand[t.ID]
	switch key {
	case "p":
		if t.Status != domain.StatusOpen {
			m.setStatus("can't pin a closed task — x reopens it")
			return nil, true
		}
		n := 0 // open children without a pin: new, or left by a parent pinned before D34
		for _, k := range m.idx.children[t.ID] {
			if _, ok := m.idx.cand[k.ID]; !ok && k.Status == domain.StatusOpen {
				n++
			}
		}
		if pinned && n == 0 {
			m.setStatus("already pinned")
			return nil, true
		}
		status := "pinned " + label
		if len(m.idx.children[t.ID]) > 0 { // D34: no undo
			status += plusChildren(n) + " · no undo, u unpins"
		}
		return m.act("pin "+label, status, func(ctx context.Context) (app.Undo, error) { return svc.Pin(ctx, t.ID) }), true
	case "u":
		// Before the "not pinned" check: a child left unpinned under a pinned
		// parent (pre-D34 data) is shown with the parent all the same.
		if _, ok := m.idx.cand[t.ParentID]; t.ParentID != 0 && ok {
			m.setStatus(fmt.Sprintf("pinned with its parent #%d · unpin the parent first", t.ParentID)) // D34
			return nil, true
		}
		if !pinned {
			m.setStatus("not pinned")
			return nil, true
		}
		if c := m.idx.cand[t.ID]; c.Blocked && len(m.idx.children[t.ID]) > 0 {
			m.setStatus(fmt.Sprintf("%q is blocked today · unblock it on the timeline first", t.Title))
			return nil, true
		}
		status := "unpinned " + label
		if kids := m.idx.children[t.ID]; len(kids) > 0 { // D34: no undo
			n, kept := 0, 0
			for _, k := range kids {
				if c, ok := m.idx.cand[k.ID]; ok && c.Blocked {
					kept++
				} else if ok {
					n++
				}
			}
			status += plusChildren(n)
			if kept > 0 {
				status += " · " + plural(kept, "blocked child", "blocked children") + " kept"
			}
			status += " · no undo, p pins"
		}
		return m.act("unpin "+label, status, func(ctx context.Context) (app.Undo, error) { return svc.Unpin(ctx, t.ID) }), true
	case "x":
		if t.Status != domain.StatusOpen {
			return m.act("reopen "+label, "reopened "+label, func(ctx context.Context) (app.Undo, error) { return svc.Reopen(ctx, t.ID) }), true
		}
		return m.act("done "+label, "✓ "+label, func(ctx context.Context) (app.Undo, error) { return svc.Done(ctx, t.ID) }), true
	case "d":
		if t.Status == domain.StatusDropped {
			m.setStatus("already dropped — x reopens it") // Drop would overwrite the reason
			return nil, true
		}
		return m.openDropPrompt(t), true
	case "D":
		return m.openDeleteConfirm(t), true
	case "e":
		return m.openEdit(t), true
	case "E":
		return m.editNote(t), true
	case "o":
		return m.openLink(t), true
	}
	return nil, false
}

func (m *Model) pickView() string {
	total := max(m.width-3, 20)
	lw := total * 55 / 100
	rw := total - lw
	title, empty := "BACKLOG · by "+string(m.pick.group), "nothing triaged yet · a to add · i to triage"
	if m.pick.filtered() {
		n := 0
		for _, r := range m.rows(paneBacklog) {
			if r.kind == rowTask {
				n++
			}
		}
		title += " · " + plural(n, "match", "matches") + " · " + m.pick.filterText // the count first: truncation eats the filter text
		empty = "no matches · esc clears"
	}
	left := m.paneLines(paneBacklog, title, empty, lw, m.pick.focus == paneBacklog)
	right := m.paneLines(paneToday, "TODAY'S CANDIDATES", "nothing pinned · p on a backlog task", rw, m.pick.focus == paneToday)
	return columns(left, right, lw, rw)
}

// columns renders two panes side by side, lw and rw cells wide.
func columns(left, right []string, lw, rw int) string {
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

// paneLines renders pane pn's title and visible rows; focused shows its cursor.
func (m *Model) paneLines(pn pane, title, empty string, width int, focused bool) []string {
	lines := []string{styleTitle.Render(title)}
	rows := m.rows(pn)
	if len(rows) == 0 {
		return append(lines, styleDim.Render("  "+empty))
	}
	visible := m.visibleRows()
	cursor := m.pick.cursor[pn]
	start := m.pick.offset[pn]
	for i := start; i < len(rows) && i < start+visible; i++ {
		lines = append(lines, m.rowLine(rows[i], width, focused && i == cursor))
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
	case r.parent && !r.expanded:
		b.WriteString("▸ ")
	case r.parent:
		b.WriteString("▾ ")
	default:
		b.WriteString("  ")
	}
	b.WriteString(m.badge(t) + " " + t.Title)
	if t.Note != "" {
		b.WriteString(" ✎")
	}
	if t.Requester != "" {
		b.WriteString(" @" + t.Requester)
	}
	if kids := m.idx.children[t.ID]; len(kids) > 0 {
		done, total := 0, 0
		for _, k := range kids {
			if k.Status == domain.StatusDropped {
				continue // dropped work isn't part of the remaining total
			}
			total++
			if k.Status == domain.StatusDone {
				done++
			}
		}
		if total > 0 {
			fmt.Fprintf(&b, " %d/%d", done, total)
		}
	}
	if c := r.cand; c != nil {
		if c.Blocked {
			b.WriteString(" ▓")
		}
	}
	if t.RolloverCount > 0 {
		fmt.Fprintf(&b, " ↻%d", t.RolloverCount)
	}
	if c := r.cand; c != nil && c.PinnedDays > 1 {
		fmt.Fprintf(&b, " 📌%d", c.PinnedDays)
	}
	switch t.Status {
	case domain.StatusDone:
		b.WriteString(" ✓")
	case domain.StatusDropped:
		b.WriteString(" ✗")
	}
	if r.orphan != 0 {
		suffix := fmt.Sprintf(" ↳ #%d", r.orphan)
		if !selected {
			suffix = styleDim.Render(suffix)
		}
		b.WriteString(suffix)
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
