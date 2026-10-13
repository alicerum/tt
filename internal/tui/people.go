package tui

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"tt/internal/domain"
	"tt/internal/dsl"
	"tt/internal/search"
)

// person is one requester in People (D52).
type person struct {
	name string
	open int // open tasks that name them
}

// peopleState is the People view (D52): everyone some task names as its
// requester on the left, the selected person's tasks on the right.
type peopleState struct {
	from     screen   // esc returns here
	onTasks  bool     // the tasks pane has the focus
	filter   string   // the names filter as typed (trimmed); "" = none
	names    []person // the index's people, narrowed by the filter
	cursor   int
	offset   int     // first name shown
	selected string  // the name under the cursor; survives reloads
	tasks    rowList // the selected person's tasks
}

// openPeople shows People; esc returns to the current screen.
func (m *Model) openPeople() {
	m.people.from, m.screen = m.screen, screenPeople
	m.people.refresh(m)
}

// refresh rebuilds both panes after a load or a filter change. The cursor
// stays on its person, or on the nearest row when they're gone (renamed
// elsewhere, their last task deleted); the tasks pane keeps its task the
// same way, and starts at the top for another person.
func (p *peopleState) refresh(m *Model) {
	p.names = nil
	for _, pr := range m.idx.people {
		if search.MatchName(p.filter, pr.name) {
			p.names = append(p.names, pr)
		}
	}
	if i := slices.IndexFunc(p.names, func(pr person) bool { return pr.name == p.selected }); i >= 0 {
		p.cursor = i
	}
	p.cursor = max(0, min(p.cursor, len(p.names)-1))
	name := ""
	if p.cursor < len(p.names) {
		name = p.names[p.cursor].name
	}
	if name != p.selected {
		p.tasks = rowList{}
	}
	p.selected = name
	p.tasks.rows = m.personRows(name)
	p.tasks.clamp()
}

// setFilter narrows the names to those matching text; the cursor goes to
// the first match unless its person still matches.
func (p *peopleState) setFilter(m *Model, text string) {
	p.filter = strings.TrimSpace(text)
	if !search.MatchName(p.filter, p.selected) {
		p.cursor = 0
	}
	p.refresh(m)
}

func (p *peopleState) move(m *Model, delta int) {
	if c := p.cursor + delta; c >= 0 && c < len(p.names) {
		p.cursor, p.selected, p.tasks = c, p.names[c].name, rowList{rows: m.personRows(p.names[c].name)}
		p.tasks.clamp()
	}
}

// follow keeps both cursors inside their windows. Called after every update.
func (p *peopleState) follow(m *Model) {
	p.offset = scroll(p.cursor, p.cursor, p.offset, m.namesVisible(), len(p.names))
	p.tasks.follow(m.visibleRows())
}

// namesVisible is how many names fit below the names frame's filter lines.
func (m *Model) namesVisible() int { return max(m.visibleRows()-len(m.peopleFilterLines()), 1) }

// peopleFilterLines is what the names frame shows above the names (D56):
// the filter being typed, or the applied one, dim; nothing without one.
func (m *Model) peopleFilterLines() []string {
	if f, ok := m.overlay.(*peopleFilter); ok {
		return []string{f.view(m)}
	}
	if m.people.filter != "" {
		return []string{appliedFilterLine(m.people.filter)}
	}
	return nil
}

// namesWidth is People's left column for a terminal width cells wide.
func namesWidth(width int) int { return width / 3 }

// personRows lists name's tasks (D52): the open ones grouped by priority
// like the backlog, pinned ones too, oldest first within a group; then a
// closed heading over the done and dropped ones, newest first. Every task
// is a row of its own: a child shows ↳ #parent, a parent its progress.
func (m *Model) personRows(name string) []row {
	if name == "" {
		return nil
	}
	snap, now, headings := m.state.Snapshot, m.svc.Now(), m.priorityHeadings()
	groups := map[search.Group][]domain.Task{}
	var closed []domain.Task
	for _, t := range snap.Tasks() {
		switch {
		case t.Requester != name:
		case t.Status != domain.StatusOpen:
			closed = append(closed, t)
		default:
			g := search.GroupOf(search.ByPriority, t, snap.ParentOf(t), now, headings)
			groups[g] = append(groups[g], t)
		}
	}
	var rows []row
	add := func(t domain.Task) {
		r := row{kind: rowTask, task: t, person: true, orphan: t.ParentID}
		if c, ok := m.idx.cand[t.ID]; ok {
			r.cand = &c
		}
		rows = append(rows, r)
	}
	for _, g := range slices.SortedFunc(maps.Keys(groups), search.Compare) {
		slices.SortFunc(groups[g], search.Oldest)
		rows = append(rows, row{kind: rowHeader, text: g.Label})
		for _, t := range groups[g] {
			add(t)
		}
	}
	if len(closed) > 0 {
		slices.SortFunc(closed, func(a, b domain.Task) int {
			return cmp.Or(b.ClosedAt.Compare(a.ClosedAt), cmp.Compare(b.ID, a.ID))
		})
		rows = append(rows, row{kind: rowHeader, text: "closed"})
		for _, t := range closed {
			add(t)
		}
	}
	return rows
}

func (m *Model) peopleKey(key string) tea.Cmd {
	p := &m.people
	if n := m.steps(key); n != 0 {
		if p.onTasks {
			repeat(n, p.tasks.move)
		} else {
			repeat(n, func(delta int) { p.move(m, delta) })
		}
		return nil
	}
	switch key {
	case "tab":
		if !p.onTasks && len(p.tasks.rows) == 0 { // D57
			m.setStatus("nobody listed")
			return nil
		}
		p.onTasks = !p.onTasks
		return nil
	case "/":
		return m.openPeopleFilter()
	case "r":
		if p.selected == "" {
			m.setStatus("nobody to rename")
			return nil
		}
		return m.openRename(p.selected)
	case "esc":
		if p.filter != "" {
			p.setFilter(m, "")
			m.setStatus("filter cleared")
			return nil
		}
		m.screen = p.from
		return nil
	}
	if !p.onTasks {
		if key == "enter" && p.selected != "" {
			p.onTasks = true // a person's tasks
		}
		return nil
	}
	r, ok := p.tasks.selectedRow()
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

func (m *Model) peopleView() string {
	lw := namesWidth(m.width)
	rw := m.width - lw
	p := &m.people
	title, empty := fmt.Sprintf("PEOPLE · %d", len(m.idx.people)), "nobody yet · @who names who asked"
	if p.filter != "" {
		title = "PEOPLE · " + plural(len(p.names), "match", "matches") // the filter has its own line (D56)
		empty = "no matches"
	}
	names := m.peopleFilterLines()
	if len(p.names) == 0 {
		names = append(names, styleDim.Render("  "+empty))
	}
	visible := m.visibleRows()
	for i := p.offset; i < len(p.names) && i < p.offset+m.namesVisible(); i++ {
		names = append(names, m.personLine(p.names[i], m.inner(lw), i == p.cursor))
	}
	left := m.box(title, names, lw, m.bodyHeight(), !p.onTasks)
	tasksTitle, tasks := "TASKS", []string(nil)
	if p.selected != "" {
		tasksTitle, tasks = "@"+p.selected, m.listLines(p.tasks, visible, "", m.inner(rw), p.onTasks)
	}
	return beside(left, m.box(tasksTitle, tasks, rw, m.bodyHeight(), p.onTasks))
}

// openRename asks for a person's new name (D53), prefilled with the old one.
// A name someone else already has asks first, then merges the two.
func (m *Model) openRename(from string) tea.Cmd {
	in := newInput("rename @"+from+" › ", "new name", m.width)
	in.SetValue(from)
	in.CursorEnd()
	m.overlay = &prompt{
		input: in,
		check: func(v string) error { _, err := dsl.ParseRequester(v); return err },
		onEnter: func(v string) tea.Cmd {
			to, _ := dsl.ParseRequester(v) // check passed
			switch {
			case to == from:
				m.setStatus("unchanged")
				return nil
			case slices.ContainsFunc(m.idx.people, func(pr person) bool { return pr.name == to }):
				m.overlay = &nudge{
					question: fmt.Sprintf("%q already exists · ⏎ merge into them · esc", to),
					onEnter:  func() tea.Cmd { return m.rename(from, to, "merge", "merged") },
				}
				return nil
			}
			return m.rename(from, to, "rename", "renamed")
		},
	}
	return nil
}

// rename gives from's tasks the name to. The cursor follows the new name,
// so a filter that would hide it is cleared; on an error it stays on from.
func (m *Model) rename(from, to, verb, done string) tea.Cmd {
	p := &m.people
	p.selected = to
	if !search.MatchName(p.filter, to) {
		p.filter = ""
	}
	svc, ctx, label := m.svc, m.ctx, verb+" "+from+" → "+to
	return func() tea.Msg {
		n, undo, err := svc.RenameRequester(ctx, from, to)
		return actionMsg{label: label, status: fmt.Sprintf("%s %s → %s · %s", done, from, to, plural(n, "task", "tasks")),
			undo: undo, err: err, restore: func(m *Model) { m.people.selected = from }}
	}
}

// personLine renders `ana · 3 open`; a person with nothing open is dimmed.
// The cursor's person stays marked while the tasks pane has the focus.
func (m *Model) personLine(pr person, width int, cursor bool) string {
	line := pad(fmt.Sprintf("  %s · %d open", pr.name, pr.open), width)
	switch {
	case cursor && !m.people.onTasks:
		return within(styleSelected, line)
	case cursor:
		return styleTitle.Render(line) // whose tasks the right pane shows
	case pr.open == 0:
		return styleDim.Render(line) // still listed, for cleaning up names
	}
	return line
}

func (m *Model) peopleHints() string {
	back := "esc back"
	if m.people.filter != "" {
		back = "esc clears filter"
	}
	if m.people.onTasks {
		return "⏎ open · e edit · p pin · u unpin · x done · d drop · r rename person · tab people · " + back + " · a add · ctrl+z undo · ? help"
	}
	return "j/k move · ⏎/tab tasks · / filter · r rename or merge · " + back + " · a add · ctrl+z undo · ? help · q quit"
}

var peopleHelp = []string{
	"j k      move · tab switches people / tasks · ⏎ on a person: their tasks",
	"/        filter the names as you type (esc in the line or on the list clears it)",
	"r        rename the person, on every task that names them; a name someone already has merges the two (⏎ confirms)",
	"⏎        task detail · e / E edit the line / the note in $EDITOR · o open the link",
	"p / u    pin to today / unpin",
	"x        done (again: reopen)",
	"d / D    drop with optional reason / delete",
	"esc      back to where you came from",
}

// peopleFilter narrows People's names as you type (D52): enter keeps the
// filter, esc clears it.
type peopleFilter struct{ input textinput.Model }

func (*peopleFilter) inPane() {}

func (m *Model) openPeopleFilter() tea.Cmd {
	in := newInput("filter › ", "name", namesWidth(m.width)-3)
	in.SetValue(m.people.filter)
	in.CursorEnd()
	m.people.onTasks = false
	m.overlay = &peopleFilter{input: in}
	return nil
}

func (f *peopleFilter) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.overlay = nil
		m.people.setFilter(m, "")
		return nil
	case "enter":
		m.overlay = nil
		return nil
	}
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(k)
	m.people.setFilter(m, f.input.Value())
	return cmd
}

func (f *peopleFilter) paste(m *Model, msg tea.PasteMsg) tea.Cmd {
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	m.people.setFilter(m, f.input.Value())
	return cmd
}

func (f *peopleFilter) resize(width int) { fitInput(&f.input, namesWidth(width)-3) }

func (f *peopleFilter) view(*Model) string { return f.input.View() }
