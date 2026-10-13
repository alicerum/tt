package domain

import (
	"testing"
	"time"
)

func TestBusy(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 12, h, m, 0, 0, time.UTC) }
	ev := func(title string, from, to time.Time) Event {
		return Event{Title: title, Start: from, End: to, Availability: "busy", Status: "accepted"}
	}
	late := ev("Late", at(15, 0), at(16, 0))
	standup := ev("Standup", at(9, 0), at(9, 15))
	tentative := ev("Maybe", at(11, 0), at(12, 0))
	tentative.Status, tentative.Availability = "tentative", "tentative"
	own := ev("Own", at(13, 0), at(14, 0)) // my own event: no attendees, no status
	own.Status = ""
	allDay := ev("Holiday", at(0, 0), at(24, 0))
	allDay.AllDay = true
	declined := ev("Declined", at(10, 0), at(11, 0))
	declined.Status = "declined"
	free := ev("Focus (free)", at(10, 0), at(11, 0))
	free.Availability = "free"
	empty := ev("Reminder", at(12, 0), at(12, 0))

	got := Busy([]Event{late, allDay, standup, declined, tentative, free, own, empty})
	var titles []string
	for _, e := range got {
		titles = append(titles, e.Title)
	}
	want := []string{"Standup", "Maybe", "Own", "Late"}
	if len(titles) != len(want) {
		t.Fatalf("Busy = %v; want %v", titles, want)
	}
	for i := range want {
		if titles[i] != want[i] {
			t.Fatalf("Busy = %v; want %v (by start)", titles, want)
		}
	}
	if Busy(nil) != nil {
		t.Error("Busy(nil) is empty")
	}
}
