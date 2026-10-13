package config

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseCalendar(t *testing.T) {
	if cfg := Default(); cfg.Calendars != nil || cfg.CalendarPoll != 2*time.Minute {
		t.Errorf("defaults: calendars %v, poll %v; want none (not set up) and 2m", cfg.Calendars, cfg.CalendarPoll)
	}
	cfg, err := Parse([]byte("[calendar]\ncalendars = [\"Work\", \"Team X\"]\npoll = \"5m\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cfg.Calendars, []string{"Work", "Team X"}) || cfg.CalendarPoll != 5*time.Minute {
		t.Errorf("calendar: %v every %v", cfg.Calendars, cfg.CalendarPoll)
	}
	cases := map[string]string{
		"[calendar]\ncalendars = [\"Work\", \" \"]": "calendar.calendars[1]: name is empty",
		"[calendar]\npoll = \"2\"":                  "calendar.poll: want a duration of 10s or more",
		"[calendar]\npoll = \"1s\"":                 "calendar.poll: want a duration of 10s or more",
		"[calendar]\nnames = [\"Work\"]":            "unknown config key",
		"[calendar]\ncalendars = \"Work\"":          "calendars",
	}
	for input, wantErr := range cases {
		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}
