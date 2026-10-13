package source

import (
	"context"
	"sync"

	"tt/internal/domain"
)

// Fake is a GitHub for tests: it returns what Set gave it. Safe for
// concurrent use (the TUI fetches in the background).
type Fake struct {
	mu    sync.Mutex
	items []domain.SourceItem
	err   error
	calls int
}

func (f *Fake) Kind() string { return domain.SourceGitHub }

// Set makes the next fetches return items, or err when it isn't nil.
func (f *Fake) Set(items []domain.SourceItem, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items, f.err = append([]domain.SourceItem(nil), items...), err
}

// Fetch returns a copy of what Set gave it (or its error) and counts the call.
func (f *Fake) Fetch(context.Context) ([]domain.SourceItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return append([]domain.SourceItem(nil), f.items...), nil
}

// Calls is how many fetches there were.
func (f *Fake) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}
