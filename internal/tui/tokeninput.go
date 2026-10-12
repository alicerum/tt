package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// tokenInput is a one-line input in token syntax (D25, D26a) that completes
// the @requester or #tag at the cursor from known values. Quick-add and the
// filter bar share it.
type tokenInput struct{ input textinput.Model }

func newTokenInput(prompt, placeholder string, width int) tokenInput {
	return tokenInput{input: newInput(prompt, placeholder, width)}
}

func (ti *tokenInput) value() string { return ti.input.Value() }

func (ti *tokenInput) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	ti.input, cmd = ti.input.Update(msg)
	return cmd
}

func (ti *tokenInput) resize(width int) { fitInput(&ti.input, width) }

// word returns the value as runes and the bounds [start, end) of the word
// the cursor is in (or right after).
func (ti *tokenInput) word() (r []rune, start, end int) {
	r = []rune(ti.input.Value())
	pos := min(ti.input.Position(), len(r))
	start, end = pos, pos
	for start > 0 && r[start-1] != ' ' {
		start--
	}
	for end < len(r) && r[end] != ' ' {
		end++
	}
	return r, start, end
}

// suggestions completes the @requester or #tag being typed at the cursor,
// most used first. Nothing is completed inside a " -- " note.
func (ti *tokenInput) suggestions(m *Model) []string {
	r, start, _ := ti.word()
	tok := string(r[start:min(ti.input.Position(), len(r))])
	if tok == "" || (tok[0] != '@' && tok[0] != '#') || strings.Contains(" "+string(r[:start]), " -- ") {
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

// complete replaces the word at the cursor with s and moves past it.
func (ti *tokenInput) complete(s string) {
	r, start, end := ti.word()
	tail := string(r[end:])
	if tail == "" {
		tail = " "
	}
	ti.input.SetValue(string(r[:start]) + s + tail)
	ti.input.SetCursor(start + len([]rune(s)) + 1)
}

// view renders the input, err (why the last enter was refused) and the
// suggestions, when there are any.
func (ti *tokenInput) view(m *Model, err error) string {
	out := ti.input.View()
	if err != nil {
		out += "\n" + styleError.Render("✗ "+err.Error())
	}
	if s := ti.suggestions(m); len(s) > 0 {
		out += "\n" + styleDim.Render("  tab → "+strings.Join(s, "  "))
	}
	return out
}
