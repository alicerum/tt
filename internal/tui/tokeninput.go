package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"tt/internal/dsl"
)

// tokenInput is a one-line input in token syntax (D25, D26a) that completes
// the @requester or #tag at the cursor from known values. Quick-add, add-child,
// the edit line and the filter bar share it.
type tokenInput struct {
	input textinput.Model
	extra []string // lines 2+ of a multi-line paste: the note (D33)
	plain bool     // the line has no " -- " note (the filter bar)
}

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

// pasteLines pastes the first line into the input and keeps lines 2+ as
// the note (D33); line() adds them on submit.
func (ti *tokenInput) pasteLines(msg tea.PasteMsg) tea.Cmd {
	lines := strings.Split(strings.ReplaceAll(msg.Content, "\r\n", "\n"), "\n")
	cmd := ti.update(tea.PasteMsg{Content: lines[0]})
	ti.extra = append(ti.extra, lines[1:]...)
	return cmd
}

// line is what to submit: the typed line plus any pasted note lines.
func (ti *tokenInput) line() string { return dsl.WithNoteLines(ti.value(), ti.extra) }

// reset empties the input for the next entry.
func (ti *tokenInput) reset() {
	ti.input.SetValue("")
	ti.extra = nil
}

// noteLines counts the pasted note lines, ignoring trailing blank ones.
func (ti *tokenInput) noteLines() int {
	n := len(ti.extra)
	for n > 0 && strings.TrimSpace(ti.extra[n-1]) == "" {
		n--
	}
	return n
}

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
	if tok == "" || (tok[0] != '@' && tok[0] != '#') || !ti.plain && strings.Contains(" "+string(r[:start]), " -- ") {
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
	if n := ti.noteLines(); n > 0 {
		out += "\n" + styleDim.Render(fmt.Sprintf("  + %s → note", plural(n, "line", "lines")))
	}
	if err != nil {
		out += "\n" + styleError.Render("✗ "+err.Error())
	}
	if s := ti.suggestions(m); len(s) > 0 {
		out += "\n" + styleDim.Render("  tab → "+strings.Join(s, "  "))
	}
	return out
}
