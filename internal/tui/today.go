package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/app"
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
	carry  *carry        // a block being placed or moved (D36); nil otherwise
}

// carry is a picked-up block: a candidate being placed, or a block being
// moved. It follows the timeline cursor as a ghost until ⏎ drops it.
type carry struct {
	task  domain.Task
	block domain.Block // the block being moved; ID 0 when placing a candidate
}

// refresh rebuilds the timeline from the loaded plan and puts the cursor
// back on its row (the window may have grown or shrunk).
func (td *todayState) refresh(m *Model) {
	cfg, plan := m.svc.Config(), m.state.Plan
	shown := -1 // the cursor's line in the window, kept when rows appear above it
	if td.placed && td.cursor < len(td.tl.rows) {
		shown = td.line(td.cursor) - td.offset
	}
	now := time.Duration(-1)
	if m.svc.Today() == plan.Day {
		now = plan.Day.Offset(m.svc.Now())
	}
	var meetings []meeting
	for _, e := range m.state.Calendar.Meetings {
		// a title is a third party's text: no newline or escape reaches the screen
		meetings = append(meetings, meeting{title: domain.OneLine(e.Title), start: plan.Day.Offset(e.Start), end: plan.Day.Offset(e.End)})
	}
	td.tl = buildTimeline(plan.Blocks, meetings, cfg.WorkStart, cfg.WorkEnd, now)
	rows := td.tl.rows
	if !td.placed {
		td.at, td.placed = rows[td.tl.homeRow()].start, true
	}
	td.cursor = td.tl.rowOf(td.at)
	if td.cursor < 0 { // the window shrank past the cursor
		td.cursor = 0
		if td.at > rows[0].start {
			td.cursor = len(rows) - 1
		}
	}
	td.at = rows[td.cursor].start
	if shown >= 0 {
		td.offset = max(0, td.line(td.cursor)-shown)
	}
	td.refreshCarry(m)
}

// refreshCarry drops a pick-up whose task or block went away meanwhile (an
// undo, another shell) and picks up the latest copy of a moved block.
func (td *todayState) refreshCarry(m *Model) {
	c := td.carry
	if c == nil {
		return
	}
	t, ok := m.state.Snapshot.Get(c.task.ID)
	if !ok || t.Status != domain.StatusOpen && c.block.ID == 0 {
		td.carry = nil
		if c.block.ID != 0 {
			m.setStatus(fmt.Sprintf("stopped moving %q: its task is gone", c.task.Title))
		} else {
			m.setStatus(fmt.Sprintf("stopped placing %q: it changed meanwhile", c.task.Title))
		}
		return
	}
	c.task = t
	if c.block.ID == 0 {
		return
	}
	b, ok := td.tl.block(c.block.ID)
	if !ok {
		td.carry = nil
		m.setStatus(fmt.Sprintf("stopped moving %q: its block is gone", t.Title))
		return
	}
	c.block = b
}

// pickUp starts placing candidate t (D36): the cursor goes to the first free
// row from now (outside the hours: from where the cursor rests, D39).
func (m *Model) pickUp(t domain.Task) {
	td := &m.today
	td.carry = &carry{task: t}
	td.focus = focusTimeline
	td.cursor = td.tl.firstFree(td.tl.homeRow())
	td.at = td.tl.rows[td.cursor].start
}

// pickUpBlock starts moving block b (D36): the cursor goes to its first row.
func (m *Model) pickUpBlock(t domain.Task, b domain.Block) {
	td := &m.today
	td.carry = &carry{task: t, block: b}
	if i := td.tl.rowOf(b.Start); i >= 0 { // a block starting before the hours has no row of its start
		td.cursor = i
	}
	td.at = td.tl.rows[td.cursor].start
}

// freeSlot is the first row after block b's, and not before now, where b, moved there (keeping
// its minutes within the row, like m), fits whole inside the working hours,
// clear of every other block and of every meeting (D43).
func (m *Model) freeSlot(b domain.Block) (int, bool) {
	tl := &m.today.tl
	within := b.Start - b.Start.Truncate(rowLen)
	first := tl.rowOf(b.Start) + 1
	if tl.nowRow >= 0 {
		first = max(first, tl.nowRow) // not into the past; j/k still reach it (D36)
	}
	for i := first; i < len(tl.rows); i++ {
		nb := b
		nb.Start = tl.rows[i].start + within
		if tl.rows[i].outside || nb.Start < tl.start || nb.End() > tl.end {
			continue
		}
		if _, clash := tl.clash(nb); clash {
			continue
		}
		if _, busy := tl.conflict(nb); busy {
			continue
		}
		return i, true
	}
	return 0, false
}

// ghost is the carried block as it would land at the cursor: a moved block
// keeps its length; a candidate gets its D24 length, min(size, the free time
// from the cursor to the next block or meeting, or the hours' end; D42).
func (m *Model) ghost() domain.Block {
	td := &m.today
	c := td.carry
	if c.block.ID != 0 {
		b := c.block // keeps its offset within the row, so 09:10 moves to 10:10
		b.Start = td.at.Truncate(rowLen) + (c.block.Start - c.block.Start.Truncate(rowLen))
		return b
	}
	// gapFrom is 0 inside a block, and a gap of 0 means "no cap": the ghost
	// keeps its full length there and shows the clash.
	length := m.svc.DefaultBlockLength(c.task, td.tl.gapFrom(td.at))
	return domain.Block{TaskID: c.task.ID, Day: m.state.Plan.Day, Start: td.at, Duration: length}
}

// fits explains why b can't go where it is: another block (D36), or outside
// the working hours (D39). nil when it fits.
func (m *Model) fits(b domain.Block) error {
	tl := &m.today.tl
	if o, clash := tl.clash(b); clash {
		return fmt.Errorf("overlaps %s", m.blockName(o))
	}
	if b.Start < tl.start || b.End() > tl.end {
		return fmt.Errorf("outside of working hours (%s–%s)", clock(tl.start), clock(tl.end))
	}
	return nil
}

// blockName renders `"Title" 10:00–11:00`.
func (m *Model) blockName(b domain.Block) string {
	title := fmt.Sprintf("#%d", b.TaskID)
	if t, ok := m.state.Snapshot.Get(b.TaskID); ok {
		title = fmt.Sprintf("%q", t.Title)
	}
	return title + " " + span(b)
}

func span(b domain.Block) string { return clock(b.Start) + "–" + clock(b.End()) }

// overMeeting is the status note for a block placed over a meeting (D42):
// allowed, since a meeting is sometimes skipped, but said.
func (m *Model) overMeeting(b domain.Block) string {
	mt, ok := m.today.tl.conflict(b)
	if !ok {
		return ""
	}
	return fmt.Sprintf(" · ⚠ overlaps meeting %q %s–%s", mt.title, clock(mt.start), clock(mt.end))
}

// conflicted reports a block that shows ⚠ (D18, D42): an open task's block
// overlapping a meeting. A closed task's block is history.
func (m *Model) conflicted(b domain.Block) bool {
	t, ok := m.state.Snapshot.Get(b.TaskID)
	if !ok || t.Status != domain.StatusOpen {
		return false
	}
	_, ok = m.today.tl.conflict(b)
	return ok
}

// drop places or moves the carried block at the cursor. A clash keeps it
// picked up, so another row can be tried; so does a refusal by the service
// (the timeline was stale), once it comes back.
func (m *Model) drop() tea.Cmd {
	td := &m.today
	c, g, at := td.carry, m.ghost(), td.at
	if err := m.fits(g); err != nil {
		m.setErr(err)
		return nil
	}
	td.carry = nil
	if c.block.ID != 0 && g.Start == c.block.Start {
		m.setStatus("unchanged")
		return nil
	}
	back := func(m *Model) { m.today.carry, m.today.at, m.today.focus = c, at, focusTimeline }
	svc, label := m.svc, fmt.Sprintf("%q", c.task.Title)
	if c.block.ID != 0 {
		return m.actOr("move "+label, "moved "+label+" to "+span(g)+m.overMeeting(g), back, func(ctx context.Context) (app.Undo, error) {
			return svc.MoveBlock(ctx, c.block.ID, g.Start)
		})
	}
	status := "blocked " + label + " " + span(g) + m.overMeeting(g)
	if c.task.Size == domain.SizeL || c.task.Size == domain.SizeXL {
		status += " · big task · consider splitting" // D37
	}
	return m.actOr("block "+label, status, back, func(ctx context.Context) (app.Undo, error) {
		_, undo, err := svc.Block(ctx, c.task.ID, g.Start, g.Duration)
		return undo, err
	})
}

// carryKey handles keys while a block is picked up.
func (m *Model) carryKey(key string) tea.Cmd {
	td := &m.today
	if n := m.steps(key); n != 0 {
		repeat(n, td.move)
		return nil
	}
	switch key {
	case "enter":
		return m.drop()
	case "esc":
		td.carry = nil
		m.setStatus("cancelled")
	}
	return nil
}

// reshape resizes or re-times block b after checking it still fits.
func (m *Model) reshape(old domain.Block, t domain.Task, b domain.Block) tea.Cmd {
	if err := m.fits(b); err != nil {
		m.setErr(err)
		return nil
	}
	svc, label := m.svc, fmt.Sprintf("%q", t.Title)
	return m.act("change "+label, label+" now "+span(b)+m.overMeeting(b), func(ctx context.Context) (app.Undo, error) {
		if b.Start == old.Start {
			return svc.ResizeBlock(ctx, b.ID, b.Duration)
		}
		return svc.SetBlockSpan(ctx, b.ID, b.Start, b.Duration)
	})
}

// openFineTune asks for a block's exact start and length (D36). The block
// is read again on enter: a reload meanwhile may have moved it.
func (m *Model) openFineTune(b domain.Block, t domain.Task) tea.Cmd {
	dayStart := m.svc.Config().DayStart
	in := newInput(fmt.Sprintf("fine-tune %q › ", ansi.Truncate(t.Title, 30, "…")), "09:30 45m", m.width)
	in.SetValue(clock(b.Start) + " " + shortDur(b.Duration))
	in.CursorEnd()
	var nb domain.Block
	m.overlay = &prompt{
		input: in,
		check: func(v string) error {
			start, dur, err := parseSpan(v, dayStart)
			if err != nil {
				return err
			}
			cur, ok := m.today.tl.block(b.ID)
			if !ok {
				return nil // onEnter says so
			}
			nb = cur
			nb.Start, nb.Duration = start, dur
			return m.fits(nb)
		},
		onEnter: func(string) tea.Cmd {
			cur, ok := m.today.tl.block(b.ID)
			switch {
			case !ok:
				m.setStatus(fmt.Sprintf("stopped fine-tuning %q: its block is gone", t.Title))
				return nil
			case nb.Start == cur.Start && nb.Duration == cur.Duration:
				m.setStatus("unchanged")
				return nil
			}
			return m.reshape(cur, t, nb)
		},
	}
	return nil
}

// parseSpan reads a fine-tune line "HH:MM <length>", e.g. "09:30 45m". A
// time before the day start is after midnight, in the same planning day.
func parseSpan(s string, dayStart time.Duration) (start, dur time.Duration, err error) {
	f := strings.Fields(s)
	if len(f) != 2 {
		return 0, 0, errors.New("want a start and a length, like 09:30 45m")
	}
	at, err := time.Parse("15:04", f[0])
	if err != nil {
		return 0, 0, fmt.Errorf("start %q: want HH:MM", f[0])
	}
	start = time.Duration(at.Hour())*time.Hour + time.Duration(at.Minute())*time.Minute
	if start < dayStart {
		start += 24 * time.Hour
	}
	dur, err = time.ParseDuration(f[1])
	if err != nil || dur < time.Minute || dur%time.Minute != 0 {
		return 0, 0, fmt.Errorf("length %q: want whole minutes, like 45m or 1h30m", f[1])
	}
	return start, dur, nil
}

// move steps the cursor, stopping at the first and the last row.
func (td *todayState) move(delta int) {
	td.cursor = max(0, min(td.cursor+delta, len(td.tl.rows)-1))
	td.at = td.tl.rows[td.cursor].start
}

// line is the timeline line row i is drawn on: the now line comes right
// before the row holding now, and moves down once that row has passed.
func (td *todayState) line(i int) int {
	if n := td.tl.nowRow; n >= 0 && i >= n {
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
	top := l
	if td.cursor == td.tl.nowRow {
		top-- // the now line above the cursor's row comes into view with it
	}
	off := min(td.offset, top)
	if l >= off+visible {
		off = l - visible + 1
	}
	td.offset = max(0, min(off, td.lines()-visible))
}

// blockKeys act on the block under the timeline cursor.
var blockKeys = map[string]bool{
	"enter": true, "x": true, "d": true, "D": true, "e": true, "E": true, "o": true,
	"m": true, "+": true, "=": true, "-": true, "f": true, "u": true, "r": true,
}

func (m *Model) todayKey(key string) tea.Cmd {
	td := &m.today
	if td.carry != nil {
		return m.carryKey(key)
	}
	if cmd, ok := m.laneKey(key); ok {
		return cmd
	}
	switch key {
	case "tab":
		if td.focus == focusTimeline && len(m.rows(paneToday)) == 0 { // D57
			m.setStatus("nothing pinned · L opens the log to pick")
			return nil
		}
		td.focus = 1 - td.focus
		return nil
	case "L":
		m.screen = screenLog
		return nil
	case "i":
		m.openInbox()
		return nil
	case "P":
		m.openPeople()
		return nil
	}
	if td.focus == focusCandidates {
		if n := m.steps(key); n != 0 {
			repeat(n, func(delta int) { m.pick.moveIn(paneToday, delta) })
			return nil
		}
		r, ok := m.selectedIn(paneToday)
		if !ok {
			return nil
		}
		t := r.task
		switch key {
		case "enter":
			m.openDetail(t.ID) // D31
			return nil
		case "b":
			if t.Status != domain.StatusOpen {
				m.setStatus("can't block a closed task — x reopens it")
				return nil
			}
			if m.hasOpenChildren(t) { // D37
				m.overlay = &nudge{
					question: "parent task; blocking its children is recommended · ⏎ block anyway · esc",
					onEnter:  func() tea.Cmd { m.pickUp(t); return nil },
				}
				return nil
			}
			m.pickUp(t)
			return nil
		}
		cmd, _ := m.taskKey(key, t)
		return cmd
	}
	if n := m.steps(key); n != 0 {
		repeat(n, td.move)
		return nil
	}
	if !blockKeys[key] {
		return nil
	}
	b, ok := td.tl.blockAt(td.cursor)
	if !ok {
		if len(td.tl.rows[td.cursor].bands) > 0 {
			m.setStatus("meeting · read-only") // D41
			return nil
		}
		m.setStatus("nothing blocked here · tab to the candidates")
		return nil
	}
	t, ok := m.state.Snapshot.Get(b.TaskID)
	if !ok {
		return nil
	}
	switch key {
	case "enter":
		m.openDetail(t.ID)
		return nil
	case "m":
		m.pickUpBlock(t, b)
		return nil
	case "r": // D43
		if !m.conflicted(b) {
			m.setStatus("no conflict · m moves it")
			return nil
		}
		m.pickUpBlock(t, b)
		if i, ok := m.freeSlot(b); ok {
			td.cursor, td.at = i, td.tl.rows[i].start
		} else {
			m.setStatus("no free slot left today fits " + shortDur(b.Duration))
		}
		return nil
	case "+", "=":
		nb := b
		nb.Duration += rowLen
		return m.reshape(b, t, nb)
	case "-":
		if b.Duration <= rowLen {
			m.setStatus("can't be shorter · f sets an exact length · u unblocks")
			return nil
		}
		nb := b
		nb.Duration -= rowLen
		return m.reshape(b, t, nb)
	case "f":
		return m.openFineTune(b, t)
	case "u":
		label := fmt.Sprintf("%q", t.Title)
		return m.act("unblock "+label, "unblocked "+label+" · still a candidate", func(ctx context.Context) (app.Undo, error) {
			return m.svc.Unblock(ctx, b.ID)
		})
	}
	cmd, _ := m.taskKey(key, t)
	return cmd
}

func (m *Model) hasOpenChildren(t domain.Task) bool {
	return slices.ContainsFunc(m.idx.children[t.ID], func(k domain.Task) bool { return k.Status == domain.StatusOpen })
}

func (m *Model) todayView() string {
	lw := m.width / 2
	rw := m.width - lw
	left := m.box(m.timelineTitle(), m.timelineLines(m.inner(lw)), lw, m.bodyHeight(), m.timelineFocused())
	right := m.paneBox(paneToday, "CANDIDATES", "nothing pinned · L opens the log to pick", nil, rw, m.today.focus == focusCandidates && !m.lane.focused)
	right = append(right, m.laneView(rw)...)
	return beside(left, right)
}

// timelineTitle is `TIMELINE · 2/5 done · ⚠ 1`.
func (m *Model) timelineTitle() string {
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
	if n := len(slices.DeleteFunc(slices.Clone(td.tl.blocks), func(b domain.Block) bool { return !m.conflicted(b) })); n > 0 {
		title += fmt.Sprintf(" · ⚠ %d", n) // D42
	}
	if td.tl.outsideNow() {
		title += " · outside of working hours"
	}
	return title
}

// timelineLines renders the lines in the timeline's window, width cells wide.
func (m *Model) timelineLines(width int) []string {
	td := &m.today
	var all []string
	for i := range td.tl.rows {
		if i == td.tl.nowRow {
			now := "━━ now " + clock(td.tl.now) + " "
			all = append(all, styleAlert.Render(now+strings.Repeat("━", max(width-ansi.StringWidth(now), 0))))
		}
		all = append(all, m.timelineRow(i, width))
	}
	end := min(len(all), td.offset+m.visibleRows())
	return all[min(td.offset, end):end]
}

// timelineRow renders row i: its time, then the blocks starting in it
// (▓ title [P S]), or ▓ where a block carries on from an earlier row, then
// its meetings (D41). A picked-up block shows as a ░ ghost over the rows it
// would take.
func (m *Model) timelineRow(i, width int) string {
	td := &m.today
	r := td.tl.rows[i]
	if td.carry != nil {
		if g := m.ghost(); r.start < g.End() && g.Start < r.start+rowLen {
			return m.ghostRow(r, g, width)
		}
	}
	if r.outside {
		b := r.blocks[0]
		line := pad(" "+clock(r.start)+" ▓ "+m.blockText(b, true)+" · outside of working hours", width)
		if m.timelineFocused() && i == td.cursor {
			return styleSelected.Render(line)
		}
		return styleDim.Render(line)
	}
	var labels []string
	carried := false
	for _, b := range r.blocks {
		if b.Start >= r.start {
			labels = append(labels, m.blockLabel(b))
		} else {
			carried = true
		}
	}
	var parts []string
	switch {
	case len(labels) > 0:
		parts = append(parts, "▓ "+strings.Join(labels, " · "))
	case carried:
		parts = append(parts, "▓")
	}
	parts = append(parts, m.meetingParts(r)...)
	line := pad(" "+clock(r.start)+" "+strings.Join(parts, " · "), width)
	if now := td.tl.now; now >= 0 && r.start+rowLen <= now {
		line = styleDim.Render(line) // the past
	}
	if m.timelineFocused() && i == td.cursor {
		line = styleSelected.Render(line)
	}
	return line
}

// timelineFocused: the timeline has the focus, not its candidates or the
// Reviews lane, so its cursor row shows.
func (m *Model) timelineFocused() bool { return m.today.focus == focusTimeline && !m.lane.focused }

// ghostRow renders a row the carried block would take, with what else is
// there; it turns the error colour when the block doesn't fit there, the
// alert colour (and ⚠) when it would overlap a meeting (D42).
func (m *Model) ghostRow(r tlRow, g domain.Block, width int) string {
	_, overMeeting := m.today.tl.conflict(g)
	text := " " + clock(r.start) + " ░"
	if r.start <= g.Start {
		text += fmt.Sprintf(" %s %s", m.today.carry.task.Title, shortDur(g.Duration))
		if overMeeting {
			text += " ⚠"
		}
	}
	for _, b := range r.blocks {
		if b.ID != g.ID {
			text += " · " + m.blockLabel(b)
		}
	}
	for _, p := range m.meetingParts(r) {
		text += " · " + p
	}
	line := pad(text, width)
	switch {
	case m.fits(g) != nil:
		return styleError.Render(line)
	case overMeeting:
		return styleGhostAlert.Render(line)
	}
	return styleSelected.Render(line)
}

// meetingParts renders row r's meetings (D41): ■ and the title of a band
// that starts in the row (exact times when off the half-hour grid, +N for
// the meetings merged into it), or ■ where one carries on from above.
func (m *Model) meetingParts(r tlRow) []string {
	var out []string
	for _, bd := range r.bands {
		if max(bd.start, m.today.tl.start) < r.start {
			out = append(out, "■")
			continue
		}
		s := "■ "
		if bd.start%rowLen != 0 || bd.end%rowLen != 0 {
			s += clock(bd.start) + "–" + clock(bd.end) + " "
		}
		s += bd.title
		if bd.more > 0 {
			s += fmt.Sprintf(" +%d", bd.more)
		}
		out = append(out, s)
	}
	return out
}

// blockLabel is a block's text on its first row: exact times when it is off
// the half-hour grid, the task's title and badge, struck through when closed.
func (m *Model) blockLabel(b domain.Block) string {
	return m.blockText(b, b.Start%rowLen != 0 || b.Duration%rowLen != 0)
}

// blockText is blockLabel with the exact times shown when times is set.
func (m *Model) blockText(b domain.Block, times bool) string {
	t, ok := m.state.Snapshot.Get(b.TaskID)
	if !ok {
		return fmt.Sprintf("#%d", b.TaskID)
	}
	s := t.Title + " " + m.badge(t)
	if times {
		s = span(b) + " " + s
	}
	if m.conflicted(b) {
		s += " ⚠" // D18, D42: r re-slots it
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
	if c := m.today.carry; c != nil {
		verb := "placing"
		if c.block.ID != 0 {
			verb = "moving"
		}
		return fmt.Sprintf("%s %q · j/k move · ⏎ drop · esc cancel", verb, c.task.Title)
	}
	if m.today.focus == focusCandidates {
		return "b block · ⏎ open · u unpin · x done · d drop · e edit · tab timeline · L log · P people · i inbox · a add · ctrl+z undo · ? help · q quit"
	}
	hints := "j/k move · m move · +/- size · f fine-tune · u unblock · ⏎ open · x done · tab candidates · L log · P people · i inbox · a add · ctrl+z undo · ? help · q quit"
	if b, ok := m.today.tl.blockAt(m.today.cursor); ok && m.conflicted(b) {
		hints = "r re-slot ⚠ · " + hints
	}
	return hints
}

var todayHelp = []string{
	"tab      switch timeline / candidates",
	"j k      move",
	"b        on a candidate: pick it up; j/k choose the row, ⏎ drops it, esc cancels",
	"m        on a block: pick it up to move it (keeps its length)",
	"r        on a ⚠ block: pick it up on the next free slot (no block, no meeting); ⏎ drops, esc leaves it",
	"+ / -    on a block: 30m longer / shorter (= works for +)",
	"f        on a block: fine-tune start and length (09:30 45m)",
	"u        on a block: unblock (stays a candidate) · on a candidate: unpin",
	"⏎        task detail (the block's task on the timeline)",
	"■        a meeting (read-only); a block over one shows ⚠, and placing over one is allowed",
	"x d D    done (again: reopen) / drop / delete",
	"e / E    edit the line / the note in $EDITOR · o open the link",
	"L        the log (backlog; pick today's work there, B comes back) · i inbox · P people",
	"w        the Reviews lane: p pins a review (a task Review #n), o opens the PR · w or tab back",
}
