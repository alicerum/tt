package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"tt/internal/app"
	"tt/internal/domain"
)

// inboxState shows one card at a time (D11a). Skipped cards move to the end
// for this session; they stay in the Inbox.
type inboxState struct {
	current int64
	skipped map[int64]bool
	from    screen // esc returns here
}

// openInbox shows the Inbox; esc returns to the current screen.
func (m *Model) openInbox() {
	m.inbox.from, m.screen = m.screen, screenInbox
	m.inbox.sync(m)
}

func (in *inboxState) order(m *Model) []domain.Task {
	tasks := m.state.Snapshot.Inbox()
	slices.SortStableFunc(tasks, func(a, b domain.Task) int {
		switch sa, sb := in.skipped[a.ID], in.skipped[b.ID]; {
		case sa == sb:
			return 0
		case sa:
			return 1
		default:
			return -1
		}
	})
	return tasks
}

// sync keeps the current card if it's still in the Inbox, else shows the first.
func (in *inboxState) sync(m *Model) {
	if in.skipped == nil {
		in.skipped = map[int64]bool{}
	}
	order := in.order(m)
	if slices.ContainsFunc(order, func(t domain.Task) bool { return t.ID == in.current }) {
		return
	}
	in.current = 0
	if len(order) > 0 {
		in.current = order[0].ID
	}
}

func (m *Model) inboxKey(key string) tea.Cmd {
	in := &m.inbox
	if key == "esc" {
		m.screen = in.from
		return nil
	}
	t, ok := m.state.Snapshot.Get(in.current)
	if !ok || !m.idx.inInbox[t.ID] {
		return nil
	}
	label := fmt.Sprintf("%q", t.Title)
	levels := len(m.svc.Config().Priorities)
	switch {
	case len(key) == 1 && key[0] >= '1' && key[0] <= '9':
		n := int(key[0] - '0')
		if n > levels {
			m.setStatus(fmt.Sprintf("there are %d priority levels", levels))
			return nil
		}
		if parent := m.state.Snapshot.ParentOf(t); parent != nil {
			m.setStatus(fmt.Sprintf("priority comes from parent #%d %q", parent.ID, parent.Title))
			return nil
		}
		return m.act("priority "+label, "", func(ctx context.Context) (app.Undo, error) { return m.svc.SetPriority(ctx, t.ID, n) })
	case key == "s" || key == "m" || key == "l" || key == "x":
		size, _ := domain.ParseSize(key)
		return m.act("size "+label, "", func(ctx context.Context) (app.Undo, error) { return m.svc.SetSize(ctx, t.ID, size) })
	case key == "n":
		in.skipped[t.ID] = true
		order := in.order(m)
		if in.skipped[order[0].ID] { // everything skipped: start over
			clear(in.skipped)
		}
		in.current = in.order(m)[0].ID
	case key == "d":
		return m.openDropPrompt(t)
	case key == "D":
		return m.openDeleteConfirm(t)
	}
	return nil
}

// inboxView is the Inbox's frame title and the current card's lines.
func (m *Model) inboxView() (string, []string) {
	order := m.inbox.order(m)
	if len(order) == 0 {
		return "INBOX", []string{"", "  Inbox zero ✓  " + styleDim.Render("esc to go back")}
	}
	pos := slices.IndexFunc(order, func(t domain.Task) bool { return t.ID == m.inbox.current })
	t := order[max(pos, 0)]
	cfg := m.svc.Config()
	var b strings.Builder
	title := fmt.Sprintf("INBOX · %d of %d", pos+1, len(order))
	fmt.Fprintf(&b, "  %s\n", styleTitle.Render(t.Title))
	if t.Link != "" {
		fmt.Fprintf(&b, "  %s\n", styleDim.Render(t.Link))
	}
	meta := []string{"captured " + ago(m.svc.Now(), t.CreatedAt)}
	if t.Requester != "" {
		meta = append(meta, "@"+t.Requester)
	}
	for _, tag := range t.Tags {
		meta = append(meta, "#"+tag)
	}
	if t.RolloverCount > 0 {
		meta = append(meta, fmt.Sprintf("↻%d", t.RolloverCount))
	}
	parent := m.state.Snapshot.ParentOf(t)
	if parent != nil {
		meta = append(meta, fmt.Sprintf("parent #%d %s", parent.ID, parent.Title))
	}
	fmt.Fprintf(&b, "  %s\n", styleDim.Render(strings.Join(meta, " · ")))
	if t.Note != "" {
		note, _, _ := strings.Cut(t.Note, "\n")
		fmt.Fprintf(&b, "  %s\n", styleDim.Render("note: "+note))
	}
	b.WriteString("\n")
	if parent != nil {
		fmt.Fprintf(&b, "  priority  %s\n", styleDim.Render(fmt.Sprintf("from parent: %s", m.badge(*parent))))
	} else {
		for i, lvl := range cfg.Priorities {
			mark := " "
			if t.Priority == i+1 {
				mark = "●"
			}
			lead := "          "
			if i == 0 {
				lead = "  priority"
			}
			fmt.Fprintf(&b, "%s %s [%d] %s · %s\n", lead, mark, i+1, lvl.Name, lvl.Definition)
		}
	}
	var sizes []string
	for _, sz := range domain.Sizes {
		key := string(sz)
		if sz == domain.SizeXL {
			key = "x"
		}
		item := fmt.Sprintf("[%s] %s", key, shortDur(cfg.Sizes[sz]))
		if t.Size == sz {
			item = styleSelected.Render(item)
		}
		sizes = append(sizes, item)
	}
	fmt.Fprintf(&b, "  size       %s", strings.Join(sizes, "  "))
	return title, strings.Split(b.String(), "\n")
}
