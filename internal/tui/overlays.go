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

func newInput(prompt, placeholder string, width int) textinput.Model {
	ti := textinput.New()
	ti.Prompt = prompt
	ti.Placeholder = placeholder
	ti.CharLimit = 1000
	ti.SetWidth(max(width-ansi.StringWidth(prompt)-2, 10))
	st := ti.Styles()
	st.Cursor.Blink = false // a static cursor: no blink timers
	ti.SetStyles(st)
	ti.Focus()
	return ti
}

// quickAdd is the one-line capture overlay (D25) with @/# autocomplete.
type quickAdd struct {
	input textinput.Model
	err   error // why the last enter was refused; shown under the input
}

func (m *Model) openQuickAdd() tea.Cmd {
	m.overlay = &quickAdd{input: newInput("add › ", "title @who #tag !1 ~s link -- note", m.width)}
	return nil
}

func (q *quickAdd) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.overlay = nil
		return nil
	case "tab":
		if s := q.suggestions(m); len(s) > 0 {
			q.complete(s[0])
		}
		return nil
	case "enter":
		line := q.input.Value()
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
	var cmd tea.Cmd
	q.input, cmd = q.input.Update(k)
	return cmd
}

// currentToken is the word being typed (the input's last word).
func (q *quickAdd) currentToken() string {
	v := q.input.Value()
	if strings.HasSuffix(v, " ") {
		return ""
	}
	fields := strings.Fields(v)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// suggestions completes a trailing @requester or #tag from known values, most used first.
func (q *quickAdd) suggestions(m *Model) []string {
	tok := q.currentToken()
	if len(tok) < 1 || (tok[0] != '@' && tok[0] != '#') {
		return nil
	}
	pool := m.idx.tags
	if tok[0] == '@' {
		pool = m.idx.requesters
	}
	prefix := strings.ToLower(tok[1:])
	var out []string
	for _, v := range pool {
		if strings.HasPrefix(v, prefix) && v != prefix {
			out = append(out, string(tok[0])+v)
			if len(out) == 5 {
				break
			}
		}
	}
	return out
}

func (q *quickAdd) complete(s string) {
	v := q.input.Value()
	q.input.SetValue(v[:len(v)-len(q.currentToken())] + s + " ")
	q.input.CursorEnd()
}

func (q *quickAdd) paste(msg tea.PasteMsg) tea.Cmd {
	var cmd tea.Cmd
	q.input, cmd = q.input.Update(msg)
	return cmd
}

func (q *quickAdd) view(m *Model) string {
	out := q.input.View()
	if q.err != nil {
		out += "\n" + styleError.Render("✗ "+q.err.Error())
	}
	if s := q.suggestions(m); len(s) > 0 {
		out += "\n" + styleDim.Render("  tab → "+strings.Join(s, "  "))
	}
	return out
}

// prompt is a one-line text question, e.g. the optional drop reason (D23).
type prompt struct {
	input   textinput.Model
	onEnter func(value string) tea.Cmd
}

func (p *prompt) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.overlay = nil
		return nil
	case "enter":
		m.overlay = nil
		return p.onEnter(p.input.Value())
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(k)
	return cmd
}

func (p *prompt) paste(msg tea.PasteMsg) tea.Cmd {
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return cmd
}

func (p *prompt) view(*Model) string { return p.input.View() }

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
		"R        reload · q quit · ? this help",
	}
	var local []string
	if m.screen == screenInbox {
		local = []string{
			fmt.Sprintf("1-%d      set priority", len(m.svc.Config().Priorities)),
			"s m l x  set size (x = XL); a card with priority and size leaves the inbox",
			"n        skip (stays in the inbox)",
			"d / D    drop with optional reason / delete",
			"esc      back to Pick",
		}
	} else {
		local = []string{
			"tab      switch backlog / today",
			"j k      move · → ← expand / collapse a parent",
			"p / u    pin to today / unpin",
			"x        done (again: reopen)",
			"d / D    drop with optional reason / delete",
			"i        inbox",
		}
	}
	return styleTitle.Render("KEYS") + "\n" + strings.Join(append(local, global...), "\n") + "\n" + styleDim.Render("any key closes")
}
