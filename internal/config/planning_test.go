package config

import (
	"strings"
	"testing"
	"time"
)

func TestPlanningDefaults(t *testing.T) {
	cfg := Default()
	if cfg.UnsizedBlock != 30*time.Minute || cfg.RolloverAlert != 3 {
		t.Errorf("planning defaults: unsized %v, alert %d", cfg.UnsizedBlock, cfg.RolloverAlert)
	}
}

func TestParsePlanning(t *testing.T) {
	cfg, err := Parse([]byte("[planning]\nunsized_block = \"45m\"\nrollover_alert = 5\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UnsizedBlock != 45*time.Minute || cfg.RolloverAlert != 5 {
		t.Errorf("planning: unsized %v, alert %d", cfg.UnsizedBlock, cfg.RolloverAlert)
	}
	cases := map[string]string{
		"[planning]\nunsized_block = \"0m\"":   "planning.unsized_block",
		"[planning]\nunsized_block = \"soon\"": "planning.unsized_block",
		"[planning]\nrollover_alert = 0":       "planning.rollover_alert",
		"[planning]\nresize = \"30m\"":         "unknown config key",
	}
	for input, wantErr := range cases {
		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}
