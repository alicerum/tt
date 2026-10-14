package tui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"tt/internal/app"
	"tt/internal/config"
	"tt/internal/domain"
)

var notifyDay = time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)

func dayAt(off time.Duration) time.Time { return notifyDay.Add(off) }

func notifyFixture() app.State {
	tasks := []domain.Task{
		{ID: 1, Title: "Write RFC", Status: domain.StatusOpen},
		{ID: 2, Title: "Fix CI", Status: domain.StatusDone},
		{ID: 3, Title: "Late\nnight", Status: domain.StatusOpen},
	}
	return app.State{
		Snapshot: domain.NewSnapshot(tasks),
		Plan: domain.Plan{Day: "2026-10-12", Blocks: []domain.Block{
			{ID: 10, TaskID: 1, Start: 10 * time.Hour, Duration: time.Hour},
			{ID: 11, TaskID: 2, Start: 11 * time.Hour, Duration: time.Hour},                // a done task: no alerts
			{ID: 12, TaskID: 3, Start: 24*time.Hour + 30*time.Minute, Duration: time.Hour}, // after midnight
			{ID: 13, TaskID: 99, Start: 12 * time.Hour, Duration: time.Hour},               // its task is gone
		}},
		Calendar: app.CalendarState{Meetings: []domain.Event{
			{Title: "Standup", Start: dayAt(9*time.Hour + 30*time.Minute), End: dayAt(9*time.Hour + 45*time.Minute)},
			{Title: "Review", Start: dayAt(10 * time.Hour), End: dayAt(11 * time.Hour)},
		}},
	}
}

func momentNames(ms []moment) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.at.Format("15:04")+" "+m.name)
	}
	return out
}

func TestMomentsFollowTheKindsAndAlerts(t *testing.T) {
	st := notifyFixture()
	all := config.Notify{Meetings: true, Blocks: true, Alerts: []int{5, 0}}
	got := momentNames(moments(st, all, time.UTC))
	want := []string{"09:25 Standup", "09:30 Standup", "09:55 Review", "09:55 Write RFC", "10:00 Review", "10:00 Write RFC", "00:25 Late night", "00:30 Late night"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("moments (-want +got):\n%s", diff)
	}
	if ms := moments(st, all, time.UTC); !ms[6].at.Equal(dayAt(24*time.Hour + 25*time.Minute)) {
		t.Errorf("a block after midnight is due on the next date, got %v", ms[6].at)
	}
	got = momentNames(moments(st, config.Notify{Blocks: true, Alerts: []int{0}}, time.UTC))
	if want := []string{"10:00 Write RFC", "00:30 Late night"}; !slices.Equal(got, want) {
		t.Errorf("blocks at their start only: %v; want %v", got, want)
	}
	if got := moments(st, config.Notify{Reviews: true, Alerts: []int{5, 0}}, time.UTC); got != nil {
		t.Errorf("meetings and blocks off: %v", momentNames(got))
	}
	if got := moments(app.State{}, all, time.UTC); got != nil {
		t.Errorf("before the first load (no day): %v", momentNames(got))
	}
}

func TestDueIsTheWindowWithoutLateMoments(t *testing.T) {
	ms := moments(notifyFixture(), config.Notify{Meetings: true, Blocks: true, Alerts: []int{5, 0}}, time.UTC)
	cases := []struct {
		from, to time.Duration
		want     []string
	}{
		{9*time.Hour + 25*time.Minute, 9*time.Hour + 30*time.Minute, []string{"09:30 Standup"}}, // from is exclusive
		{9*time.Hour + 54*time.Minute, 9*time.Hour + 55*time.Minute, []string{"09:55 Review", "09:55 Write RFC"}},
		{9*time.Hour + 54*time.Minute, 9*time.Hour + 56*time.Minute, []string{"09:55 Review", "09:55 Write RFC"}}, // a minute late still counts
		{9*time.Hour + 54*time.Minute, 9*time.Hour + 56*time.Minute + time.Second, nil},                           // more than a minute late: dropped
		{9 * time.Hour, 10*time.Hour + 30*time.Second, []string{"10:00 Review", "10:00 Write RFC"}},               // after sleep: only the recent ones
		{10*time.Hour + 1*time.Minute, 10*time.Hour + 2*time.Minute, nil},
	}
	for _, c := range cases {
		if got := momentNames(due(ms, dayAt(c.from), dayAt(c.to))); !slices.Equal(got, c.want) {
			t.Errorf("due(%v, %v] = %v; want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestAlertNotesGroupByLeadTime(t *testing.T) {
	ms := []moment{
		{at: dayAt(10 * time.Hour), minutes: 0, name: "Standup"},
		{at: dayAt(10 * time.Hour), minutes: 5, name: "Write RFC"},
		{at: dayAt(10 * time.Hour), minutes: 0, name: "Fix CI"},
		{at: dayAt(10 * time.Hour), minutes: 1, name: "Lunch"},
	}
	want := []note{{"tt · now", "Standup · Fix CI"}, {"tt · in 5 min", "Write RFC"}, {"tt · in 1 min", "Lunch"}}
	if diff := cmp.Diff(want, alertNotes(ms), cmp.AllowUnexported(note{})); diff != "" {
		t.Errorf("notes (-want +got):\n%s", diff)
	}
	if got := alertNotes(nil); got != nil {
		t.Errorf("nothing due: %v", got)
	}
}

func TestReviewNotes(t *testing.T) {
	a := domain.SourceItem{ExternalID: "acme/api#12", Title: "Add\tretries", Author: "ana"}
	b := domain.SourceItem{ExternalID: "acme/web#3", Title: "Fix login"}
	c := domain.SourceItem{ExternalID: "acme/web#4", Title: "Bump deps"}
	prev := reviewIDs([]domain.SourceItem{a})
	if got := newReviews(prev, []domain.SourceItem{b, a, c}); !slices.Equal(got, []domain.SourceItem{b, c}) {
		t.Errorf("new requests: %v", got)
	}
	if got := newReviews(prev, []domain.SourceItem{a}); got != nil {
		t.Errorf("nothing new: %v", got)
	}
	if got := newReviews(reviewIDs(nil), []domain.SourceItem{a}); len(got) != 1 {
		t.Errorf("against an empty set every request is new: %v", got)
	}
	cases := []struct {
		items []domain.SourceItem
		want  note
	}{
		{[]domain.SourceItem{a}, note{"tt · review requested", "@ana acme/api#12 Add retries"}},
		{[]domain.SourceItem{b}, note{"tt · review requested", "acme/web#3 Fix login"}}, // no author known
		{[]domain.SourceItem{a, b, c}, note{"tt · 3 review requests", "Add retries · Fix login · Bump deps"}},
	}
	for _, c := range cases {
		if got := reviewNote(c.items); got != c.want {
			t.Errorf("reviewNote(%d) = %+v; want %+v", len(c.items), got, c.want)
		}
	}
}

func TestOsascriptGetsTheTextAsArguments(t *testing.T) {
	title, body, sound := `tt · now`, `say "hi" & do shell script "rm -rf ~"`, `Glass" & do shell script "x`
	args := osascriptArgs(title, body, sound)
	if n := len(args); n < 3 || args[n-3] != title || args[n-2] != body || args[n-1] != sound {
		t.Fatalf("the title, body and sound are the last three arguments: %q", args)
	}
	for _, a := range args[:len(args)-3] {
		if a != "-e" && (strings.Contains(a, "hi") || strings.Contains(a, "now") || strings.Contains(a, "Glass")) {
			t.Errorf("text spliced into the script: %q", a)
		}
	}
	if !slices.ContainsFunc(args, func(a string) bool { return strings.Contains(a, "sound name (item 3 of argv)") }) {
		t.Errorf("no sound in the script: %q", args)
	}
	silent := osascriptArgs(title, body, "")
	if n := len(silent); silent[n-2] != title || silent[n-1] != body || slices.ContainsFunc(silent, func(a string) bool { return strings.Contains(a, "sound") }) {
		t.Errorf(`sound "" still plays one (D67): %q`, silent)
	}
}
