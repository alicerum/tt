package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"tt/internal/app"
	"tt/internal/domain"
	"tt/internal/dsl"
)

// inboxState shows one card at a time (D11a). Skipped cards move to the end
// for this session; they stay in the Inbox. A card stays on screen once
// triaged, until ⏎ moves on (D73): held is that card.
type inboxState struct {
	current int64
	held    int64 // the card being triaged: shown until ⏎ even once it left the Inbox
	skipped map[int64]bool
	from    screen // esc returns here
}

// openInbox shows the Inbox; esc returns to the current screen.
func (m *Model) openInbox() {
	m.inbox.from, m.screen, m.inbox.held = m.screen, screenInbox, 0
	m.inbox.sync(m)
}

// order is the cards: the held one first while it is triaged but still
// open, then the Inbox, skipped ones last.
func (in *inboxState) order(m *Model) []domain.Task {
	tasks := m.state.Snapshot.Inbox()
	if t, ok := m.state.Snapshot.Get(in.held); ok && t.Status == domain.StatusOpen &&
		!slices.ContainsFunc(tasks, func(x domain.Task) bool { return x.ID == t.ID }) {
		tasks = append([]domain.Task{t}, tasks...)
	}
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
	if !ok || !m.idx.inInbox[t.ID] && t.ID != in.held {
		return nil
	}
	label := fmt.Sprintf("%q", t.Title)
	switch {
	case key == "enter":
		if domain.InInbox(t, m.state.Snapshot.ParentOf(t)) {
			if t.ParentID != 0 {
				m.setStatus("set the size first · n skips")
			} else {
				m.setStatus("set impact and size first · n skips")
			}
			return nil
		}
		in.held = 0
		in.sync(m)
	case key == "^":
		in.held = t.ID
		return m.openDuePrompt(t)
	case len(key) == 1 && key[0] >= '1' && key[0] <= '9':
		n := int(key[0] - '0')
		if n > domain.MaxImpact {
			m.setStatus(fmt.Sprintf("impact is 1 to %d", domain.MaxImpact))
			return nil
		}
		if t.ParentID != 0 {
			m.setStatus(fmt.Sprintf("children inherit impact from #%d", t.ParentID)) // D73
			return nil
		}
		in.held = t.ID
		return m.act("impact "+label, "", func(ctx context.Context) (app.Undo, error) { return m.svc.SetImpact(ctx, t.ID, n) })
	case key == "s" || key == "m" || key == "l" || key == "x":
		size, _ := domain.ParseSize(key)
		in.held = t.ID
		return m.act("size "+label, "", func(ctx context.Context) (app.Undo, error) { return m.svc.SetSize(ctx, t.ID, size) })
	case key == "n":
		in.held = 0
		if !m.idx.inInbox[t.ID] { // triaged and held: n moves on, like ⏎
			in.sync(m)
			return nil
		}
		in.skipped[t.ID] = true
		order := in.order(m)
		if in.skipped[order[0].ID] { // everything skipped: start over
			clear(in.skipped)
		}
		in.current = in.order(m)[0].ID
	case key == "d":
		in.held = 0 // undoing the drop shows an ordinary card, not a held one
		return m.openDropPrompt(t)
	case key == "D":
		in.held = 0
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
	// The optional due comes first (D73).
	switch due := domain.EffectiveDue(t, parent); {
	case due == "":
		fmt.Fprintf(&b, "  due       %s\n", styleDim.Render("— · ^ to set"))
	case t.Due == "":
		fmt.Fprintf(&b, "  due       %s %s\n", dayLabel(due), styleDim.Render(fmt.Sprintf("from parent #%d · ^ to set its own", parent.ID)))
	default:
		fmt.Fprintf(&b, "  due       %s %s\n", dayLabel(due), styleDim.Render(dueMarker(m.svc.Today(), due)+" · ^ to change"))
	}
	if parent != nil {
		impact := "not set"
		if parent.Impact > 0 {
			impact = domain.ImpactNames[parent.Impact-1]
		}
		fmt.Fprintf(&b, "  impact    %s\n", styleDim.Render(fmt.Sprintf("from parent #%d: %s", parent.ID, impact)))
	} else {
		fmt.Fprintf(&b, "  impact    %s\n", styleDim.Render("who notices if it slips?"))
		for i, name := range domain.ImpactNames {
			mark := " "
			if t.Impact == i+1 {
				mark = "●"
			}
			fmt.Fprintf(&b, "           %s [%d] %s · %s\n", mark, i+1, name, domain.ImpactExamples[i])
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
	if !domain.InInbox(t, parent) {
		fmt.Fprintf(&b, "\n\n  %s", styleDim.Render("triaged · ⏎ next card"))
	}
	return title, strings.Split(b.String(), "\n")
}

// openDuePrompt asks for t's due date (D73): the ^ forms, with or without
// the ^; empty clears it.
func (m *Model) openDuePrompt(t domain.Task) tea.Cmd {
	label := fmt.Sprintf("%q", t.Title)
	today := m.svc.Today()
	in := newInput("due › ", "fri · tom · 3d · 30oct · 2026-10-30 · empty clears", m.width)
	in.SetValue(string(t.Due))
	in.CursorEnd()
	m.overlay = &prompt{
		input: in,
		check: func(v string) error { _, err := dsl.ParseDue(v, today); return err },
		onEnter: func(v string) tea.Cmd {
			due, _ := dsl.ParseDue(v, today) // check passed
			return m.act("due "+label, "", func(ctx context.Context) (app.Undo, error) { return m.svc.SetDue(ctx, t.ID, due) })
		},
	}
	return nil
}
