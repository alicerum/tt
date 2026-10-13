package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"tt/internal/calendar"
	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/store"
)

// calService is a started service with a fake calendar; it also returns the
// store, for writing a cache by hand.
func calService(t *testing.T) (*Service, *fakeClock, *calendar.Fake, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "tt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	clock := &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
	cal := &calendar.Fake{}
	s := New(st, clock, config.Default()).WithCalendar(cal)
	if _, err := s.StartDay(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s, clock, cal, st
}

func mustLoad(t *testing.T, s *Service) State {
	t.Helper()
	st, err := s.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func mustSync(t *testing.T, s *Service) {
	t.Helper()
	if err := s.SyncCalendar(context.Background()); err != nil {
		t.Fatalf("SyncCalendar: %v", err)
	}
}

func at(h, m int) time.Time { return time.Date(2026, 10, 12, h, m, 0, 0, time.UTC) }

func meeting(title string, from, to time.Time) domain.Event {
	return domain.Event{Title: title, Start: from, End: to, Availability: "busy", Status: "accepted"}
}

func titles(events []domain.Event) []string {
	var out []string
	for _, e := range events {
		out = append(out, e.Title)
	}
	return out
}

func TestCalendarNotSetUp(t *testing.T) {
	s, _ := startedService(t)
	if err := s.SyncCalendar(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.CalendarSetUp() {
		t.Error("CalendarSetUp without WithCalendar")
	}
	if cs := mustLoad(t, s).Calendar; cs.SetUp || cs.Meetings != nil || cs.Err != "" || !cs.LastSuccess.IsZero() {
		t.Errorf("no calendar configured (D40): %+v", cs)
	}
}

func TestSyncCalendarCachesBusyEvents(t *testing.T) {
	s, clock, cal, _ := calService(t)
	if !s.CalendarSetUp() {
		t.Error("CalendarSetUp after WithCalendar")
	}
	if cs := mustLoad(t, s).Calendar; !cs.SetUp || cs.Meetings != nil || !cs.LastSuccess.IsZero() || cs.Err != "" {
		t.Fatalf("before the first fetch: %+v", cs)
	}
	declined := meeting("Declined", at(10, 0), at(11, 0))
	declined.Status = "declined"
	allDay := meeting("Holiday", at(0, 0), at(24, 0))
	allDay.AllDay = true
	cal.Set([]domain.Event{meeting("Review", at(14, 0), at(15, 0)), declined, allDay, meeting("Standup", at(9, 30), at(9, 45))}, nil)
	mustSync(t, s)

	from, to := cal.Asked()
	if !from.Equal(at(4, 0)) || !to.Equal(at(28, 0)) {
		t.Errorf("fetched %v–%v; want the planning day 04:00–04:00", from, to)
	}
	cs := mustLoad(t, s).Calendar
	if got := titles(cs.Meetings); len(got) != 2 || got[0] != "Standup" || got[1] != "Review" {
		t.Fatalf("meetings = %v; want the busy ones by start (D17a)", got)
	}
	if m := cs.Meetings[0]; !m.Start.Equal(at(9, 30)) || !m.End.Equal(at(9, 45)) {
		t.Errorf("Standup = %v–%v", m.Start, m.End)
	}
	if !cs.LastSuccess.Equal(clock.now) || cs.Err != "" {
		t.Errorf("after a success: %+v", cs)
	}

	// D3a: a failed fetch keeps the last good meetings and says why.
	clock.now = clock.now.Add(2 * time.Minute)
	cal.Set(nil, calendar.ErrNoAccess)
	mustSync(t, s)
	cs = mustLoad(t, s).Calendar
	if len(cs.Meetings) != 2 || cs.Err != calendar.ErrNoAccess.Error() || !cs.LastSuccess.Equal(at(9, 0)) {
		t.Errorf("after a failure: %d meetings, err %q, last success %v", len(cs.Meetings), cs.Err, cs.LastSuccess)
	}

	// A success clears the error; no busy events is an empty day, not a failure.
	clock.now = clock.now.Add(2 * time.Minute)
	cal.Set([]domain.Event{declined}, nil)
	mustSync(t, s)
	if cs = mustLoad(t, s).Calendar; cs.Meetings != nil || cs.Err != "" || !cs.LastSuccess.Equal(clock.now) {
		t.Errorf("after an empty success: %+v", cs)
	}
}

func TestCalendarCacheOfAnotherDayIsIgnored(t *testing.T) {
	s, clock, cal, _ := calService(t)
	cal.Set([]domain.Event{meeting("Standup", at(9, 30), at(9, 45))}, nil)
	mustSync(t, s)

	clock.now = time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC) // the next planning day, before its first fetch
	cs := mustLoad(t, s).Calendar
	if cs.Meetings != nil || !cs.LastSuccess.Equal(at(9, 0)) {
		t.Errorf("yesterday's cache (D44): %d meetings, last success %v; want none and yesterday's time", len(cs.Meetings), cs.LastSuccess)
	}
}

func TestSyncCalendarAfterMidnightFetchesThePlanningDay(t *testing.T) {
	s, clock, cal, _ := calService(t)
	clock.now = time.Date(2026, 10, 13, 1, 30, 0, 0, time.UTC) // still Mon's planning day (day_start 04:00)
	cal.Set([]domain.Event{meeting("Late call", at(23, 30), at(24, 30))}, nil)
	mustSync(t, s)
	if from, to := cal.Asked(); !from.Equal(at(4, 0)) || !to.Equal(at(28, 0)) {
		t.Errorf("fetched %v–%v; want Mon 04:00–Tue 04:00", from, to)
	}
	if cs := mustLoad(t, s).Calendar; len(cs.Meetings) != 1 {
		t.Errorf("the planning day's cache is today's: %+v", cs)
	}
}

func TestSyncCalendarWhileQuittingRecordsNothing(t *testing.T) {
	s, _, cal, _ := calService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cal.Set(nil, context.Canceled)
	if err := s.SyncCalendar(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want context.Canceled", err)
	}
	if cs := mustLoad(t, s).Calendar; cs.Err != "" {
		t.Errorf("a cancelled fetch isn't the calendar's failure: %+v", cs)
	}
}

func TestUnreadableCalendarCacheDoesNotBreakTheLoad(t *testing.T) {
	s, _, _, st := calService(t)
	err := st.InTx(context.Background(), func(x *store.Tx) error {
		return x.PutIntegration(context.Background(), store.Integration{Name: "calendar", LastSuccessAt: at(8, 0), Payload: "{oops"})
	})
	if err != nil {
		t.Fatal(err)
	}
	cs := mustLoad(t, s).Calendar
	if cs.Meetings != nil || cs.Err == "" {
		t.Errorf("a broken cache shows as an error: %+v", cs)
	}
}
