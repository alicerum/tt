package calendar

import (
	"context"
	"sync"
	"time"

	"tt/internal/domain"
)

// Fake is a calendar for tests: it returns what Set gave it and remembers
// how it was asked. Safe for concurrent use (the TUI fetches in the background).
type Fake struct {
	mu       sync.Mutex
	events   []domain.Event
	err      error
	calls    int
	from, to time.Time
}

// Set makes the next fetches return events, or err when it isn't nil.
func (f *Fake) Set(events []domain.Event, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events, f.err = events, err
}

func (f *Fake) Events(_ context.Context, from, to time.Time) ([]domain.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.from, f.to = from, to
	if f.err != nil {
		return nil, f.err
	}
	return append([]domain.Event(nil), f.events...), nil
}

// Calls is how many fetches there were.
func (f *Fake) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// Asked is the range of the last fetch.
func (f *Fake) Asked() (from, to time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.from, f.to
}
