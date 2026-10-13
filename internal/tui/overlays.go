package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tt/internal/app"
	"tt/internal/domain"
	"tt/internal/dsl"
)

// fitInput sizes ti to a terminal width cells wide.
func fitInput(ti *textinput.Model, width int) {
	ti.SetWidth(max(width-ansi.StringWidth(ti.Prompt)-2, 10))
}

func newInput(prompt, placeholder string, width int) textinput.Model {
	ti := textinput.New()
	ti.Prompt = prompt
	ti.Placeholder = placeholder
	ti.CharLimit = 1000
	fitInput(&ti, width)
	st := ti.Styles()
	st.Cursor.Blink = false // a static cursor: no blink timers
	ti.SetStyles(st)
	ti.Focus()
	return ti
}

// quickAdd is the one-line capture overlay (D25) with @/# autocomplete.
type quickAdd struct {
	in  tokenInput
	err error // why the last enter was refused; shown under the input
}

func (m *Model) openQuickAdd() tea.Cmd {
	m.overlay = &quickAdd{in: newTokenInput("add › ", "title @who #tag !1 ~s link -- note", m.width)}
	return nil
}

func (q *quickAdd) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.overlay = nil
		return nil
	case "tab":
		if s := q.in.suggestions(m); len(s) > 0 {
			q.in.complete(s[0])
		}
		return nil
	case "enter":
		line := q.in.line()
		if _, err := dsl.ParseAdd(line, len(m.svc.Config().Priorities)); err != nil {
			q.err = err
			return nil
		}
		m.overlay = nil
		svc, ctx := m.svc, m.ctx
		return func() tea.Msg {
			t, undo, err := svc.Add(ctx, line)
			where := "backlog"
			if domain.InInbox(t, nil) {
				where = "inbox"
			}
			return actionMsg{label: fmt.Sprintf("add %q", t.Title), status: fmt.Sprintf("added #%d to %s", t.ID, where), undo: undo, err: err}
		}
	}
	q.err = nil
	return q.in.update(k)
}

func (q *quickAdd) paste(_ *Model, msg tea.PasteMsg) tea.Cmd { return q.in.pasteLines(msg) }

func (q *quickAdd) resize(width int) { q.in.resize(width) }

func (q *quickAdd) view(m *Model) string { return q.in.view(m, q.err) }

// filterBar edits the backlog filter and applies it as you type (D26e).
type filterBar struct {
	in      tokenInput
	prev    string   // the filter before `/`; esc restores it
	prevSel [2]int64 // and the tasks the cursors were on
	err     error    // the line doesn't parse; the last valid filter stays applied
}

func (m *Model) openFilter() tea.Cmd {
	in := newTokenInput("filter › ", "@who #tag !1 ~s <7d >30d is:done words", m.width)
	in.plain = true // no " -- " note in a filter: complete everywhere
	if text := m.pick.filterText; text != "" {
		in.input.SetValue(text + " ")
		in.input.CursorEnd()
	}
	m.pick.focus = paneBacklog
	m.overlay = &filterBar{in: in, prev: m.pick.filterText, prevSel: m.pick.selected}
	return nil
}

func (f *filterBar) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.overlay = nil
		m.pick.selected = f.prevSel // live narrowing moved the cursors
		_ = m.applyFilter(f.prev)   // it parsed when it was applied
		return nil
	case "enter":
		if f.err == nil {
			m.overlay = nil
		}
		return nil // refused: the text and the error stay
	case "tab":
		if s := f.in.suggestions(m); len(s) > 0 {
			f.in.complete(s[0])
			f.apply(m)
		}
		return nil
	}
	cmd := f.in.update(k)
	f.apply(m)
	return cmd
}

func (f *filterBar) apply(m *Model) { f.err = m.applyFilter(f.in.value()) }

func (f *filterBar) paste(m *Model, msg tea.PasteMsg) tea.Cmd {
	cmd := f.in.update(msg)
	f.apply(m)
	return cmd
}

func (f *filterBar) resize(width int) { f.in.resize(width) }

func (f *filterBar) view(m *Model) string { return f.in.view(m, f.err) }

// prompt is a one-line text question, e.g. the optional drop reason (D23).
// With check set, enter is refused while check fails; the text stays.
type prompt struct {
	input   textinput.Model
	check   func(value string) error
	err     error
	onEnter func(value string) tea.Cmd
}

func (p *prompt) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.overlay = nil
		return nil
	case "enter":
		if p.check != nil {
			if p.err = p.check(p.input.Value()); p.err != nil {
				return nil
			}
		}
		m.overlay = nil
		return p.onEnter(p.input.Value())
	}
	p.err = nil
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(k)
	return cmd
}

func (p *prompt) paste(_ *Model, msg tea.PasteMsg) tea.Cmd {
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return cmd
}

func (p *prompt) resize(width int) { fitInput(&p.input, width) }

func (p *prompt) view(*Model) string {
	if p.err != nil {
		return p.input.View() + "\n" + styleError.Render("✗ "+p.err.Error())
	}
	return p.input.View()
}

func (m *Model) openDropPrompt(t domain.Task) tea.Cmd {
	label := fmt.Sprintf("%q", t.Title)
	shown := fmt.Sprintf("%q", ansi.Truncate(t.Title, 30, "…"))
	m.overlay = &prompt{
		input: newInput("drop "+shown+" · reason (enter to skip) › ", "", m.width),
		onEnter: func(reason string) tea.Cmd {
			return m.act("drop "+label, "dropped "+label, func(ctx context.Context) (app.Undo, error) { return m.svc.Drop(ctx, t.ID, reason) })
		},
	}
	return nil
}

// confirm asks y/n; anything but y cancels.
type confirm struct {
	question string
	onYes    func() tea.Cmd
}

func (c *confirm) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
	m.overlay = nil
	if k.String() == "y" {
		return c.onYes()
	}
	m.setStatus("cancelled")
	return nil
}

func (c *confirm) view(*Model) string { return styleAlert.Render(c.question) }

func (m *Model) openDeleteConfirm(t domain.Task) tea.Cmd {
	m.overlay = &confirm{
		question: fmt.Sprintf("(y/n) Delete #%d %q? This can't be undone.", t.ID, ansi.Truncate(t.Title, 30, "…")),
		onYes: func() tea.Cmd {
			ctx, svc := m.ctx, m.svc
			return func() tea.Msg {
				err := svc.Delete(ctx, t.ID)
				return actionMsg{status: fmt.Sprintf("deleted #%d", t.ID), err: err}
			}
		},
	}
	return nil
}

// nudge asks before something discouraged (D37): ⏎ goes ahead, any other
// key cancels.
type nudge struct {
	question string
	onEnter  func() tea.Cmd
}

func (n *nudge) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
	m.overlay = nil
	if k.String() == "enter" {
		return n.onEnter()
	}
	m.setStatus("cancelled")
	return nil
}

func (n *nudge) view(*Model) string { return styleAlert.Render(n.question) }

// helpOverlay lists the keys; any key closes it.
type helpOverlay struct{}

func (helpOverlay) update(m *Model, _ tea.KeyPressMsg) tea.Cmd {
	m.overlay = nil
	return nil
}

func (helpOverlay) view(m *Model) string {
	global := []string{
		"a        quick-add: title @who #tag !1 ~s link -- note (tab completes @/#)",
		"ctrl+z   undo (refuses if the task changed since)",
		"ctrl+d / ctrl+u   half a page down / up",
		"R        reload and fetch the calendar now · q quit · ? this help",
	}
	var local []string
	switch m.screen {
	case screenDetail:
		local = []string{
			"j k      move between the task and its children",
			"⏎        open the child under the cursor · esc back (child → parent → where you came from)",
			"e / E    edit the line (title @who #tag !1 ~s link) / the note in $EDITOR",
			"o        open the link",
			"a        add children, one per line (title ~s #tag -- note; priority is the parent's)",
			"J / K    move the selected child down / up",
			"p / u    pin to today / unpin",
			"x        done (again: reopen)",
			"d / D    drop with optional reason / delete",
		}
	case screenInbox:
		local = []string{
			fmt.Sprintf("1-%d      set priority", len(m.svc.Config().Priorities)),
			"s m l x  set size (x = XL); a card with priority and size leaves the inbox",
			"n        skip (stays in the inbox)",
			"d / D    drop with optional reason / delete",
			"esc      back",
		}
	case screenToday:
		local = todayHelp
	default:
		local = []string{
			"⏎        task detail",
			"e / E    edit the line / the note in $EDITOR · o open the link",
			"tab      switch backlog / today",
			"j k      move · → ← expand / collapse a parent",
			"p / u    pin to today / unpin",
			"x        done (again: reopen)",
			"d / D    drop with optional reason / delete",
			"/        filter: @who #tag !1 ~s <7d >30d is:done|dropped|all words (live; esc on the list clears)",
			"g        group by priority → size → created → requester (remembered)",
			"B        done picking: block today's candidates in Today",
			"esc      back to Today (when no filter is set) · i inbox",
		}
	}
	return styleTitle.Render("KEYS") + "\n" + strings.Join(append(local, global...), "\n") + "\n" + styleDim.Render("any key closes")
}
