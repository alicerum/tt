package domain

import (
	"cmp"
	"slices"
	"time"
)

// Event is a calendar event as the tt-cal helper reports it (spec: Adapters).
type Event struct {
	Title        string
	Start, End   time.Time
	AllDay       bool
	Availability string // "busy", "free", "tentative", "unavailable"; "" when the calendar has none
	Status       string // my participation: "accepted", "declined", "tentative", "pending"; "" when not invited
	Calendar     string
}

// Busy keeps the events that make time busy (D17a): all-day, declined and
// "free" events aren't busy, nor is an event without length; tentative ones
// are. The result is sorted by start, then end.
func Busy(events []Event) []Event {
	var out []Event
	for _, e := range events {
		if e.AllDay || e.Status == "declined" || e.Availability == "free" || !e.End.After(e.Start) {
			continue
		}
		out = append(out, e)
	}
	slices.SortStableFunc(out, func(a, b Event) int {
		return cmp.Or(a.Start.Compare(b.Start), a.End.Compare(b.End))
	})
	return out
}
