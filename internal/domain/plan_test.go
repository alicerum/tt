package domain

import (
	"testing"
	"time"
)

func TestPlanningDay(t *testing.T) {
	ams, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	const four = 4 * time.Hour
	cases := []struct {
		name     string
		t        time.Time
		dayStart time.Duration
		want     Day
	}{
		{"morning", time.Date(2026, 10, 12, 9, 0, 0, 0, ams), four, "2026-10-12"},
		{"exactly day start", time.Date(2026, 10, 12, 4, 0, 0, 0, ams), four, "2026-10-12"},
		{"just before day start", time.Date(2026, 10, 12, 3, 59, 59, 0, ams), four, "2026-10-11"},
		{"late night", time.Date(2026, 10, 13, 1, 30, 0, 0, ams), four, "2026-10-12"},
		{"midnight start", time.Date(2026, 10, 13, 0, 0, 0, 0, ams), 0, "2026-10-13"},
		{"month boundary", time.Date(2026, 11, 1, 2, 0, 0, 0, ams), four, "2026-10-31"},
		{"year boundary", time.Date(2027, 1, 1, 3, 0, 0, 0, ams), four, "2026-12-31"},
		// Review Focus: DST. Absolute-duration arithmetic gets these wrong.
		{"spring forward, after day start", time.Date(2026, 3, 29, 4, 30, 0, 0, ams), four, "2026-03-29"},
		{"spring forward, before day start", time.Date(2026, 3, 29, 3, 30, 0, 0, ams), four, "2026-03-28"},
		{"fall back, after day start", time.Date(2026, 10, 25, 4, 0, 0, 0, ams), four, "2026-10-25"},
		{"fall back, before day start", time.Date(2026, 10, 25, 3, 59, 0, 0, ams), four, "2026-10-24"},
	}
	for _, c := range cases {
		if got := PlanningDay(c.t, c.dayStart); got != c.want {
			t.Errorf("%s: PlanningDay(%v) = %s; want %s", c.name, c.t, got, c.want)
		}
	}
}

func TestDayAt(t *testing.T) {
	ams, _ := time.LoadLocation("Europe/Amsterdam")
	d := Day("2026-03-29") // DST starts at 02:00
	if got, want := d.At(9*time.Hour+30*time.Minute, ams), time.Date(2026, 3, 29, 9, 30, 0, 0, ams); !got.Equal(want) {
		t.Errorf("At(9:30) = %v; want %v", got, want)
	}
	if got, want := d.At(25*time.Hour, ams), time.Date(2026, 3, 30, 1, 0, 0, 0, ams); !got.Equal(want) {
		t.Errorf("At(25h) = %v; want next day 01:00 (%v)", got, want)
	}
	if PlanningDay(d.At(4*time.Hour, ams), 4*time.Hour) != d {
		t.Error("PlanningDay(d.At(dayStart)) should be d")
	}
}

func TestParseDay(t *testing.T) {
	if d, err := ParseDay("2026-10-12"); err != nil || d != "2026-10-12" {
		t.Errorf("ParseDay = %q, %v", d, err)
	}
	for _, bad := range []string{"", "2026-13-01", "12-10-2026", "2026-10-12T00:00"} {
		if _, err := ParseDay(bad); err == nil {
			t.Errorf("ParseDay(%q) succeeded; want error", bad)
		}
	}
}

func TestBlockOverlaps(t *testing.T) {
	b := Block{Day: "2026-10-12", Start: 10 * time.Hour, Duration: time.Hour}
	cases := []struct {
		name string
		o    Block
		want bool
	}{
		{"same slot", b, true},
		{"inside", Block{Day: b.Day, Start: 10*time.Hour + 15*time.Minute, Duration: 15 * time.Minute}, true},
		{"overlapping end", Block{Day: b.Day, Start: 10*time.Hour + 30*time.Minute, Duration: time.Hour}, true},
		{"touching after", Block{Day: b.Day, Start: 11 * time.Hour, Duration: time.Hour}, false},
		{"touching before", Block{Day: b.Day, Start: 9 * time.Hour, Duration: time.Hour}, false},
		{"other day", Block{Day: "2026-10-13", Start: 10 * time.Hour, Duration: time.Hour}, false},
	}
	for _, c := range cases {
		if got := b.Overlaps(c.o); got != c.want {
			t.Errorf("%s: Overlaps = %v; want %v", c.name, got, c.want)
		}
	}
}

func TestDefaultBlockLength(t *testing.T) {
	sizes := map[Size]time.Duration{SizeS: time.Hour, SizeM: 4 * time.Hour, SizeL: 8 * time.Hour, SizeXL: 40 * time.Hour}
	unsized := 30 * time.Minute
	cases := []struct {
		size Size
		gap  time.Duration
		want time.Duration
	}{
		{SizeS, 3 * time.Hour, time.Hour},
		{SizeXL, 2 * time.Hour, 2 * time.Hour},
		{SizeM, 0, 4 * time.Hour},
		{SizeXL, 0, 24 * time.Hour},
		{SizeNone, 3 * time.Hour, 30 * time.Minute},
		{SizeNone, 20 * time.Minute, 20 * time.Minute},
	}
	for _, c := range cases {
		if got := DefaultBlockLength(c.size, c.gap, sizes, unsized); got != c.want {
			t.Errorf("DefaultBlockLength(%q, %v) = %v; want %v", c.size, c.gap, got, c.want)
		}
	}
}
