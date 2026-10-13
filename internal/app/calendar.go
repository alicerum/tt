package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"tt/internal/domain"
	"tt/internal/store"
)

// CalendarSource is where meetings come from (spec: Interfaces); the tt-cal
// adapter in internal/calendar implements it.
type CalendarSource interface {
	Events(ctx context.Context, from, to time.Time) ([]domain.Event, error)
}

// WithCalendar sets the calendar SyncCalendar reads. Without one the
// calendar isn't set up (D40). Call it before the service is used.
func (s *Service) WithCalendar(src CalendarSource) *Service {
	s.cal = src
	return s
}

// CalendarSetUp reports whether a calendar is configured (D40).
func (s *Service) CalendarSetUp() bool { return s.cal != nil }

// CalendarState is what the views show of the calendar (D41, D44).
type CalendarState struct {
	SetUp       bool           // a calendar is configured (D40)
	Meetings    []domain.Event // today's busy events from the last good fetch, by start
	LastSuccess time.Time      // zero: never
	Err         string         // why the last fetch failed; "" once one succeeds
}

const calendarName = "calendar" // its integration_state row

// calPayload is the cached result of the last good fetch.
type calPayload struct {
	Day    domain.Day `json:"day"`
	Events []calEvent `json:"events"`
}

type calEvent struct {
	Title string    `json:"title"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// SyncCalendar fetches the planning day's events. On success it stores the
// busy ones (D17a) as the cache; on failure it records the error and keeps
// the cache (D3a). A failed fetch isn't returned: the views show it from the
// state (D44). Only a failure to record the fetch is.
func (s *Service) SyncCalendar(ctx context.Context) error {
	if s.cal == nil {
		return nil
	}
	today := s.Today()
	loc := s.clock.Now().Location()
	events, fetchErr := s.cal.Events(ctx, today.At(s.cfg.DayStart, loc), today.At(s.cfg.DayStart+24*time.Hour, loc))
	if ctx.Err() != nil {
		return ctx.Err() // tt is quitting; that isn't the calendar's fault
	}
	now := s.clock.Now()
	return s.store.InTx(ctx, func(x *store.Tx) error {
		in, _, err := x.GetIntegration(ctx, calendarName)
		if err != nil {
			return err
		}
		if fetchErr != nil {
			in.LastError, in.LastErrorAt = fetchErr.Error(), now
			return x.PutIntegration(ctx, in)
		}
		p := calPayload{Day: today, Events: []calEvent{}}
		for _, e := range domain.Busy(events) {
			p.Events = append(p.Events, calEvent{Title: e.Title, Start: e.Start, End: e.End})
		}
		data, err := json.Marshal(p)
		if err != nil {
			return err
		}
		in.LastSuccessAt, in.LastError, in.LastErrorAt, in.Payload = now, "", time.Time{}, string(data)
		return x.PutIntegration(ctx, in)
	})
}

// calendarState reads the cache for today. A cache of another day is
// ignored (D44); an unreadable one shows as an error instead of failing
// the load.
func (s *Service) calendarState(ctx context.Context, today domain.Day) (CalendarState, error) {
	if s.cal == nil {
		return CalendarState{}, nil
	}
	cs := CalendarState{SetUp: true}
	in, ok, err := s.store.Integration(ctx, calendarName)
	if err != nil || !ok {
		return cs, err
	}
	cs.LastSuccess, cs.Err = in.LastSuccessAt, in.LastError
	if in.Payload == "" {
		return cs, nil
	}
	var p calPayload
	if err := json.Unmarshal([]byte(in.Payload), &p); err != nil {
		if cs.Err == "" {
			cs.Err = fmt.Sprintf("can't read the cached meetings: %v", err)
		}
		return cs, nil
	}
	if p.Day != today {
		return cs, nil
	}
	loc := s.clock.Now().Location()
	for _, e := range p.Events {
		cs.Meetings = append(cs.Meetings, domain.Event{Title: e.Title, Start: e.Start.In(loc), End: e.End.In(loc)})
	}
	return cs, nil
}
