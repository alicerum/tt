package tui

import (
	"context"
	"io"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/app"
	"tt/internal/calendar"
	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/store"
)

// TestRealProgram runs the model in a real tea.Program (Plan 3
// carry-forward): the watch, clock and calendar loops run on real timers and
// their commands on other goroutines, so `go test -race` checks that only
// messages carry state into the model. Another process adds a task meanwhile.
func TestRealProgram(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "tt.db")
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	clock := &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
	cfg := config.Default()
	cfg.CalendarPoll = 10 * time.Millisecond
	cal := &calendar.Fake{}
	cal.Set([]domain.Event{{Title: "Standup", Start: clock.now.Add(30 * time.Minute), End: clock.now.Add(time.Hour)}}, nil)
	opts := DefaultOptions()
	opts.WatchEvery, opts.ClockEvery = 10*time.Millisecond, 10*time.Millisecond
	m := New(ctx, app.New(st, clock, cfg).WithCalendar(cal), opts)

	var sawTask, sawMeeting atomic.Bool
	watch := func(model tea.Model, msg tea.Msg) tea.Msg { // runs on the program's goroutine
		if mm, ok := model.(*Model); ok && mm.loaded {
			sawTask.Store(len(mm.state.Snapshot.Tasks()) > 0)
			sawMeeting.Store(len(mm.state.Calendar.Meetings) > 0)
		}
		return msg
	}
	keys, typing := io.Pipe()
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(keys), tea.WithOutput(io.Discard),
		tea.WithWindowSize(120, 30), tea.WithoutSignals(), tea.WithFilter(watch))
	done := make(chan error, 1)
	go func() {
		_, err := p.Run()
		done <- err
	}()

	deadline := time.Now().Add(10 * time.Second)
	for !(sawMeeting.Load() && cal.Calls() >= 3) { // up and polling, with no task yet
		if time.Now().After(deadline) {
			t.Fatalf("after 10s: meeting seen %v, %d calendar fetches", sawMeeting.Load(), cal.Calls())
		}
		time.Sleep(5 * time.Millisecond)
	}
	other, err := store.Open(ctx, path) // e.g. `tt add` in another pane
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, _, err := app.New(other, clock, cfg).Add(ctx, "From another shell !1 ~s"); err != nil {
		t.Fatal(err)
	}
	for !sawTask.Load() { // only the watch loop can bring it in
		if time.Now().After(deadline) {
			t.Fatal("after 10s: the task another process added was never seen")
		}
		time.Sleep(5 * time.Millisecond)
	}
	go func() { _, _ = typing.Write([]byte("q")) }() // a program that stopped reading mustn't hang the test
	t.Cleanup(func() { typing.Close() })
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(10 * time.Second):
		p.Kill()
		t.Fatal("q didn't quit the program")
	}
}
