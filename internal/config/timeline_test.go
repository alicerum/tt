package config

import (
	"strings"
	"testing"
	"time"
)

func TestWorkHoursDefaults(t *testing.T) {
	cfg := Default()
	if cfg.WorkStart != 8*time.Hour || cfg.WorkEnd != 19*time.Hour {
		t.Errorf("work hours defaults: %v–%v; want 8h–19h", cfg.WorkStart, cfg.WorkEnd)
	}
}

func TestParseWorkHours(t *testing.T) {
	cfg, err := Parse([]byte("[work_hours]\nstart = \"07:30\"\nend = \"18:00\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorkStart != 7*time.Hour+30*time.Minute || cfg.WorkEnd != 18*time.Hour {
		t.Errorf("work hours: %v–%v", cfg.WorkStart, cfg.WorkEnd)
	}
	// Only the keys you set are checked: a late day_start moves the default window along.
	late, err := Parse([]byte("day_start = \"21:00\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if late.WorkStart != 21*time.Hour || late.WorkEnd != 22*time.Hour {
		t.Errorf("late day_start: work hours %v–%v; want 21h–22h", late.WorkStart, late.WorkEnd)
	}
	cases := map[string]string{
		"[work_hours]\nstart = \"8am\"":                          "work_hours.start",
		"[work_hours]\nend = \"25:00\"":                          "work_hours.end",
		"[work_hours]\nstart = \"19:00\"\nend = \"18:00\"":       "work_hours.end must be after work_hours.start",
		"[work_hours]\nstart = \"10:00\"\nend = \"10:00\"":       "work_hours.end must be after work_hours.start",
		"day_start = \"06:00\"\n[work_hours]\nstart = \"05:00\"": "work_hours.start must not be before day_start",
		"[work_hours]\nrows = \"30m\"":                           "unknown config key",
	}
	for input, wantErr := range cases {
		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}
