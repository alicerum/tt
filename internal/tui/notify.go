package tui

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/app"
	"tt/internal/config"
	"tt/internal/domain"
)

// notifyState is the notifications' (D63–D66). Nothing of it is persisted:
// what came before launch never notifies anyway.
type notifyState struct {
	launched time.Time
	checked  time.Time // the alerts due up to here are handled; launch at first
	// reviews is the last good GitHub fetch's requests (at reviewsAt); nil
	// until the first good fetch since launch.
	reviews   map[string]bool
	reviewsAt time.Time
	focused   bool // tt's pane has the focus (D66); false until a focus event says so
	failed    bool // a send failed, and the status said so (once per run)
}

// checkAlerts sends the meeting and block alerts due since the last check
// (D64). It runs on the watch tick, so a laptop waking from sleep catches
// up within a second, and due drops what's over a minute late.
func (m *Model) checkAlerts() tea.Cmd {
	n := m.svc.Config().Notify
	now := m.svc.Now()
	if !m.loaded || !(n.Meetings || n.Blocks) || !now.Round(0).After(m.notify.checked.Round(0)) { // wall time: Round(0) drops the monotonic reading, so a clock step back can't re-fire a moment
		return nil
	}
	ms := due(moments(m.state, n, now.Location()), m.notify.checked, now)
	m.notify.checked = now
	return m.send(alertNotes(ms)...)
}

// checkReviews notifies the requests a good GitHub fetch found that the
// previous good fetch didn't (D65). The first good fetch since launch only
// remembers them: they were there before. A failed fetch keeps the last
// good one's time, so it changes nothing. Only a fetch newer than the last
// one seen counts, so a stale load landing late changes nothing either.
func (m *Model) checkReviews() tea.Cmd {
	gh := m.state.GitHub
	if !m.svc.Config().Notify.Reviews || gh.LastSuccess.Before(m.notify.launched) || !gh.LastSuccess.After(m.notify.reviewsAt) {
		return nil
	}
	prev := m.notify.reviews
	m.notify.reviews, m.notify.reviewsAt = reviewIDs(gh.Reviews), gh.LastSuccess
	if fresh := newReviews(prev, gh.Reviews); prev != nil && len(fresh) > 0 {
		return m.send(reviewNote(fresh))
	}
	return nil
}

// send shows the notes in the background, unless tt has the focus (D66):
// what is on the screen needs no notification, and it isn't kept for later.
func (m *Model) send(notes ...note) tea.Cmd {
	if m.notify.focused || len(notes) == 0 {
		return nil
	}
	fn, ctx := m.notifier(), m.ctx
	return func() tea.Msg {
		var first error
		for _, n := range notes {
			if err := fn(ctx, n.title, n.body); err != nil && first == nil {
				first = err
			}
		}
		if first != nil {
			return notifiedMsg{err: first}
		}
		return nil
	}
}

// notified reports the first failed send of the run (D63); later ones
// would only repeat it.
func (m *Model) notified(msg notifiedMsg) {
	if msg.err != nil && !m.notify.failed {
		m.notify.failed = true
		m.setErr(fmt.Errorf("can't notify: %w", msg.err))
	}
}

func (m *Model) notifier() func(ctx context.Context, title, body string) error {
	if m.opts.Notify != nil {
		return m.opts.Notify
	}
	sound := m.svc.Config().Notify.Sound
	return func(ctx context.Context, title, body string) error {
		return notifyWithSystem(ctx, title, body, sound)
	}
}

// notifyWithSystem shows a notification through macOS osascript, playing
// sound unless it is "" (D67).
func notifyWithSystem(ctx context.Context, title, body, sound string) error {
	out, err := exec.CommandContext(ctx, "osascript", osascriptArgs(title, body, sound)...).CombinedOutput()
	if err != nil && len(out) > 0 {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return err
}

// moment is one alert of a meeting or a task block (D64): due minutes
// before the start. Moving a block moves its moments.
type moment struct {
	at      time.Time
	minutes int
	name    string
}

// note is one macOS notification.
type note struct{ title, body string }

// moments lists the alert moments of today's busy meetings and of the
// blocks of open tasks, for the kinds n turns on, by when they're due.
func moments(st app.State, n config.Notify, loc *time.Location) []moment {
	var out []moment
	add := func(start time.Time, name string) {
		for _, m := range n.Alerts {
			out = append(out, moment{at: start.Add(-time.Duration(m) * time.Minute), minutes: m, name: domain.OneLine(name)})
		}
	}
	if n.Meetings {
		for _, e := range st.Calendar.Meetings {
			add(e.Start, e.Title)
		}
	}
	if n.Blocks && st.Plan.Day != "" {
		for _, b := range st.Plan.Blocks {
			if t, ok := st.Snapshot.Get(b.TaskID); ok && t.Status == domain.StatusOpen {
				add(st.Plan.Day.At(b.Start, loc), t.Title)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b moment) int { return a.at.Compare(b.at) })
	return out
}

// due keeps the moments in (from, to], except those more than a minute
// late at to: after the laptop slept they'd only be noise (D64).
func due(ms []moment, from, to time.Time) []moment {
	var out []moment
	for _, m := range ms {
		if m.at.After(from) && !m.at.After(to) && !m.at.Before(to.Add(-time.Minute)) {
			out = append(out, m)
		}
	}
	return out
}

// alertNotes makes one notification per lead time among the due moments
// (D64): `tt · in 5 min` or `tt · now`, the names joined by ` · `.
func alertNotes(ms []moment) []note {
	var (
		order []int
		names = map[int][]string{}
	)
	for _, m := range ms {
		if _, ok := names[m.minutes]; !ok {
			order = append(order, m.minutes)
		}
		names[m.minutes] = append(names[m.minutes], m.name)
	}
	var out []note
	for _, min := range order {
		title := "tt · now"
		if min > 0 {
			title = fmt.Sprintf("tt · in %d min", min)
		}
		out = append(out, note{title: title, body: strings.Join(names[min], " · ")})
	}
	return out
}

// reviewIDs is the set of the requests' ids.
func reviewIDs(items []domain.SourceItem) map[string]bool {
	ids := make(map[string]bool, len(items))
	for _, i := range items {
		ids[i.ExternalID] = true
	}
	return ids
}

// newReviews keeps the requests whose ids prev doesn't have (D65).
func newReviews(prev map[string]bool, cur []domain.SourceItem) []domain.SourceItem {
	var out []domain.SourceItem
	for _, i := range cur {
		if !prev[i.ExternalID] {
			out = append(out, i)
		}
	}
	return out
}

// reviewNote is one notification for the new requests (D65).
func reviewNote(items []domain.SourceItem) note {
	if len(items) == 1 {
		i := items[0]
		body := i.ExternalID + " " + domain.OneLine(i.Title)
		if i.Author != "" {
			body = "@" + i.Author + " " + body
		}
		return note{title: "tt · review requested", body: body}
	}
	titles := make([]string, len(items))
	for k, i := range items {
		titles[k] = domain.OneLine(i.Title)
	}
	return note{title: fmt.Sprintf("tt · %d review requests", len(items)), body: strings.Join(titles, " · ")}
}

// osascriptArgs shows a notification with title, body and sound passed as
// the script's arguments, never as script text, so nothing in a title can
// run (D63). The title always starts with "tt", so a body starting with "-"
// isn't read as an option. An empty sound plays none (D67).
func osascriptArgs(title, body, sound string) []string {
	if sound == "" {
		return []string{"-e", "on run argv", "-e", "display notification (item 2 of argv) with title (item 1 of argv)", "-e", "end run", title, body}
	}
	return []string{"-e", "on run argv", "-e", "display notification (item 2 of argv) with title (item 1 of argv) sound name (item 3 of argv)", "-e", "end run", title, body, sound}
}
