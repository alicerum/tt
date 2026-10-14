package domain

import (
	"fmt"
	"time"
)

// Day is a planning day as "YYYY-MM-DD" (D19). Days compare correctly as strings.
type Day string

// PlanningDay returns the planning day that t (in its own location) falls in.
// Wall-clock times before dayStart belong to the previous date, so late-night
// work stays "today". Uses wall-clock fields, so DST changes don't shift it.
func PlanningDay(t time.Time, dayStart time.Duration) Day {
	y, m, d := t.Date()
	sinceMidnight := time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute +
		time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())
	if sinceMidnight < dayStart {
		d--
	}
	return Day(time.Date(y, m, d, 12, 0, 0, 0, time.UTC).Format(time.DateOnly))
}

// ParseDay validates a "YYYY-MM-DD" string.
func ParseDay(s string) (Day, error) {
	if _, err := time.Parse(time.DateOnly, s); err != nil {
		return "", fmt.Errorf("day %q: want YYYY-MM-DD", s)
	}
	return Day(s), nil
}

// At returns the instant at wall-clock offset off (e.g. 9h30m) on day d in loc.
// Offsets of 24h or more land on the following date (after-midnight work).
func (d Day) At(off time.Duration, loc *time.Location) time.Time {
	t, err := time.Parse(time.DateOnly, string(d))
	if err != nil {
		panic(fmt.Sprintf("invalid Day %q", d))
	}
	minutes := int(off / time.Minute)
	return time.Date(t.Year(), t.Month(), t.Day(), minutes/60, minutes%60, 0, 0, loc)
}

// Offset is t's wall-clock offset from midnight of d, the inverse of At:
// 9h30m for 09:30 on d, 25h for 01:00 on the following date. It reads t's
// wall clock in t's own location, so a DST change doesn't shift it.
func (d Day) Offset(t time.Time) time.Duration {
	day, err := time.Parse(time.DateOnly, string(d))
	if err != nil {
		panic(fmt.Sprintf("invalid Day %q", d))
	}
	y, m, dd := t.Date()
	days := time.Date(y, m, dd, 0, 0, 0, 0, time.UTC).Sub(day) / (24 * time.Hour)
	return days*24*time.Hour + time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute +
		time.Duration(t.Second())*time.Second
}

// Pin marks a task as one of today's candidates since PinnedOn (D12a, D10d).
type Pin struct {
	TaskID   int64
	PinnedOn Day
}

// Block is a task placed on a planning day's timeline (D12a). Start is the
// wall-clock offset from midnight of Day; values past 24h mean after midnight.
type Block struct {
	ID       int64
	TaskID   int64
	Day      Day
	Start    time.Duration
	Duration time.Duration
}

func (b Block) End() time.Duration { return b.Start + b.Duration }

// Overlaps reports whether b and o share any time on the same day.
func (b Block) Overlaps(o Block) bool {
	return b.Day == o.Day && b.Start < o.End() && o.Start < b.End()
}

// DefaultBlockLength (D24): a task's size duration, or unsized for unsized
// tasks, capped at gap. A gap of 0 means "no cap".
func DefaultBlockLength(size Size, gap time.Duration, sizes map[Size]time.Duration, unsized time.Duration) time.Duration {
	length, ok := sizes[size]
	if !ok {
		length = unsized
	}
	if gap > 0 && gap < length {
		length = gap
	}
	return min(length, 24*time.Hour)
}

// Candidate is a pinned task as shown in Today (D12a).
type Candidate struct {
	Task       Task
	PinnedDays int  // planning days pinned, counting today (D10d)
	Blocked    bool // has at least one block today
	Alert      bool // rollover count reached the configured threshold (D10a)
}

// Plan is one planning day: its candidates and its timeline blocks.
type Plan struct {
	Day        Day
	Candidates []Candidate // most urgent first (untriaged last), then task id (D72)
	Blocks     []Block     // by start time
}
