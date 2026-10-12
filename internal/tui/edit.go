package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"tt/internal/app"
	"tt/internal/domain"
)

// editLine is the `e` overlay (P1): the task's token line, prefilled.
type editLine struct {
	in   tokenInput
	task domain.Task
	err  error // why the last enter was refused; shown under the input
}

func (m *Model) openEdit(t domain.Task) tea.Cmd {
	in := newTokenInput(fmt.Sprintf("edit #%d › ", t.ID), "title @who #tag !1 ~s link -- note (replaces it)", m.width)
	in.input.SetValue(app.EditLine(t))
	in.input.CursorEnd()
	m.overlay = &editLine{in: in, task: t}
	return nil
}

func (e *editLine) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.overlay = nil
		return nil
	case "tab":
		if s := e.in.suggestions(m); len(s) > 0 {
			e.in.complete(s[0])
		}
		return nil
	case "enter":
		line := e.in.value()
		if _, err := m.svc.ParseEdit(e.task, line); err != nil {
			e.err = err
			return nil
		}
		m.overlay = nil
		svc, id, label := m.svc, e.task.ID, fmt.Sprintf("%q", e.task.Title)
		return m.act("edit "+label, "edited "+label, func(ctx context.Context) (app.Undo, error) { return svc.Edit(ctx, id, line) })
	}
	e.err = nil
	return e.in.update(k)
}

func (e *editLine) paste(_ *Model, msg tea.PasteMsg) tea.Cmd { return e.in.update(msg) }

func (e *editLine) resize(width int) { e.in.resize(width) }

func (e *editLine) view(m *Model) string { return e.in.view(m, e.err) }

// addChild is the repeating add-child input of Task detail (view 5): each
// enter adds a child and the input stays open for the next one.
type addChild struct {
	in     tokenInput
	parent domain.Task
	err    error
}

func (m *Model) openAddChild() tea.Cmd {
	t := m.detail.rows[0]
	if t.ParentID != 0 {
		m.setStatus("one level only — esc to the parent")
		return nil
	}
	m.overlay = &addChild{in: newTokenInput(fmt.Sprintf("child of #%d › ", t.ID), "title ~s #tag @who -- note", m.width), parent: t}
	return nil
}

func (a *addChild) update(m *Model, k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.overlay = nil
		return nil
	case "tab":
		if s := a.in.suggestions(m); len(s) > 0 {
			a.in.complete(s[0])
		}
		return nil
	case "enter":
		line := a.in.line()
		if _, err := m.svc.ParseChild(a.parent.ID, line); err != nil {
			a.err = err
			return nil
		}
		typed, extra := a.in.value(), a.in.extra
		a.in.reset() // ready for the next child
		svc, ctx, parentID := m.svc, m.ctx, a.parent.ID
		return func() tea.Msg {
			c, undo, err := svc.AddChild(ctx, parentID, line)
			return actionMsg{label: fmt.Sprintf("add %q", c.Title), status: fmt.Sprintf("added #%d under #%d", c.ID, parentID), undo: undo, err: err,
				restore: func(m *Model) { a.restore(m, typed, extra) }}
		}
	}
	a.err = nil
	return a.in.update(k)
}

// restore puts a refused line back (with its pasted note lines) unless the
// overlay was closed or the user already started the next one.
func (a *addChild) restore(m *Model, typed string, extra []string) {
	if m.overlay != a || a.in.value() != "" || len(a.in.extra) > 0 {
		return
	}
	a.in.input.SetValue(typed)
	a.in.input.CursorEnd()
	a.in.extra = extra
}

func (a *addChild) paste(_ *Model, msg tea.PasteMsg) tea.Cmd { return a.in.pasteLines(msg) }

func (a *addChild) resize(width int) { a.in.resize(width) }

func (a *addChild) view(m *Model) string { return a.in.view(m, a.err) }

// moveChild moves the selected child one place among its siblings.
func (m *Model) moveChild(t domain.Task, delta int) tea.Cmd {
	svc, ctx, label := m.svc, m.ctx, fmt.Sprintf("%q", t.Title)
	return func() tea.Msg {
		undo, err := svc.Reorder(ctx, t.ID, delta)
		if errors.Is(err, app.ErrNoMove) {
			edge := "already last"
			if delta < 0 {
				edge = "already first"
			}
			return actionMsg{status: edge}
		}
		return actionMsg{label: "move " + label, status: "moved " + label, undo: undo, err: err}
	}
}

// editorCommand is $VISUAL, else $EDITOR, else vi, on path (D29). The
// variable may carry arguments, e.g. "code --wait".
func editorCommand(getenv func(string) string, path string) *exec.Cmd {
	editor := getenv("VISUAL")
	if strings.TrimSpace(editor) == "" {
		editor = getenv("EDITOR")
	}
	if strings.TrimSpace(editor) == "" {
		editor = "vi"
	}
	f := strings.Fields(editor)
	return exec.Command(f[0], append(f[1:], path)...)
}

// noteEditedMsg reports that the editor exited.
type noteEditedMsg struct {
	task domain.Task
	from string // the note the file started with
	path string
	err  error
}

// editNote opens t's note in the user's editor (D29): the TUI is suspended
// until the editor exits.
func (m *Model) editNote(t domain.Task) tea.Cmd {
	f, err := os.CreateTemp("", fmt.Sprintf("tt-note-%d-*.md", t.ID))
	if err != nil {
		m.setErr(fmt.Errorf("can't create the note file: %w", err))
		return nil
	}
	body := t.Note
	if body != "" {
		body += "\n"
	}
	_, werr := f.WriteString(body)
	if err := errors.Join(werr, f.Close()); err != nil {
		os.Remove(f.Name())
		m.setErr(fmt.Errorf("can't write the note file: %w", err))
		return nil
	}
	path, from := f.Name(), t.Note
	return m.exec(editorCommand(os.Getenv, path), func(err error) tea.Msg {
		return noteEditedMsg{task: t, from: from, path: path, err: err}
	})
}

// noteEdited saves what the editor left: unchanged → no write; empty →
// cleared; a failed editor → not saved, changed text kept in the file. A note that changed elsewhere in the
// meantime is not overwritten; the text stays in the file (D29a).
func (m *Model) noteEdited(msg noteEditedMsg) tea.Cmd {
	if msg.err != nil { // never saved; but typing is not thrown away (D29a)
		if data, rerr := os.ReadFile(msg.path); rerr == nil &&
			strings.TrimRight(string(data), "\n") != strings.TrimRight(msg.from, "\n") {
			// the path comes first so a narrow pane's cut keeps it
			m.setErr(fmt.Errorf("your text is in %s — editor failed (%v); note unchanged", msg.path, msg.err))
			return nil
		}
		os.Remove(msg.path)
		m.setErr(fmt.Errorf("editor failed (%v); note unchanged", msg.err))
		return nil
	}
	data, err := os.ReadFile(msg.path)
	if err != nil {
		m.setErr(fmt.Errorf("can't read the note file %s: %w", msg.path, err))
		return nil
	}
	note := strings.TrimRight(string(data), "\n")
	if note == strings.TrimRight(msg.from, "\n") {
		os.Remove(msg.path)
		m.setStatus("note unchanged")
		return nil
	}
	svc, ctx, t, path := m.svc, m.ctx, msg.task, msg.path
	label, status := fmt.Sprintf("note of %q", t.Title), "note saved"
	if note == "" {
		status = "note cleared"
	}
	return func() tea.Msg {
		undo, err := svc.SetNoteFrom(ctx, t.ID, msg.from, note)
		if errors.Is(err, app.ErrConflict) {
			err = errors.New("note changed since you opened it")
		}
		if err != nil {
			return actionMsg{err: fmt.Errorf("your text is kept in %s — %w", path, err)}
		}
		os.Remove(path)
		return actionMsg{label: label, status: status, undo: undo}
	}
}

// openLink opens t's link in the browser (D21).
func (m *Model) openLink(t domain.Task) tea.Cmd {
	if t.Link == "" {
		m.setStatus("no link")
		return nil
	}
	open, ctx, link := m.opener(), m.ctx, t.Link
	return func() tea.Msg {
		if err := open(ctx, link); err != nil {
			return actionMsg{err: fmt.Errorf("can't open %s: %w", link, err)}
		}
		return actionMsg{status: "opened " + link}
	}
}

func (m *Model) exec(c *exec.Cmd, done tea.ExecCallback) tea.Cmd {
	if m.opts.Exec != nil {
		return m.opts.Exec(c, done)
	}
	return tea.ExecProcess(c, done)
}

func (m *Model) opener() func(context.Context, string) error {
	if m.opts.Open != nil {
		return m.opts.Open
	}
	return openWithSystem
}

// openWithSystem hands url to macOS `open`.
func openWithSystem(ctx context.Context, url string) error {
	out, err := exec.CommandContext(ctx, "open", url).CombinedOutput()
	if err != nil && len(out) > 0 {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return err
}
