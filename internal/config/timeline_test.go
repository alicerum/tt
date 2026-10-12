package config

import (
	"strings"
	"testing"
	"time"
)

func TestTimelineDefaults(t *testing.T) {
	cfg := Default()
	if cfg.TimelineStart != 8*time.Hour || cfg.TimelineEnd != 19*time.Hour {
		t.Errorf("timeline defaults: %v–%v; want 8h–19h", cfg.TimelineStart, cfg.TimelineEnd)
	}
}

func TestParseTimeline(t *testing.T) {
	cfg, err := Parse([]byte("[timeline]\nstart = \"07:30\"\nend = \"18:00\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TimelineStart != 7*time.Hour+30*time.Minute || cfg.TimelineEnd != 18*time.Hour {
		t.Errorf("timeline: %v–%v", cfg.TimelineStart, cfg.TimelineEnd)
	}
	// Only the keys you set are checked: a late day_start moves the default window along.
	late, err := Parse([]byte("day_start = \"21:00\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if late.TimelineStart != 21*time.Hour || late.TimelineEnd != 22*time.Hour {
		t.Errorf("late day_start: timeline %v–%v; want 21h–22h", late.TimelineStart, late.TimelineEnd)
	}
	cases := map[string]string{
		"[timeline]\nstart = \"8am\"":                          "timeline.start",
		"[timeline]\nend = \"25:00\"":                          "timeline.end",
		"[timeline]\nstart = \"19:00\"\nend = \"18:00\"":       "timeline.end must be after timeline.start",
		"[timeline]\nstart = \"10:00\"\nend = \"10:00\"":       "timeline.end must be after timeline.start",
		"day_start = \"06:00\"\n[timeline]\nstart = \"05:00\"": "timeline.start must not be before day_start",
		"[timeline]\nrows = \"30m\"":                           "unknown config key",
	}
	for input, wantErr := range cases {
		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}
