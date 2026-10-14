package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"tt/internal/app"
	"tt/internal/config"
	"tt/internal/domain"
)

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

// osascriptArgs shows a notification with title and body passed as the
// script's arguments, never as script text, so nothing in a title can run
// (D63). The title always starts with "tt", so a body starting with "-"
// isn't read as an option.
func osascriptArgs(title, body string) []string {
	return []string{"-e", "on run argv", "-e", "display notification (item 2 of argv) with title (item 1 of argv)", "-e", "end run", title, body}
}
