package config

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tt/internal/domain"
)

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DayStart != 4*time.Hour || len(cfg.Priorities) != 3 || cfg.Sizes[domain.SizeXL] != 40*time.Hour {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestParseOverrides(t *testing.T) {
	cfg, err := Parse([]byte(`
day_start = "00:00"

[[priorities]]
name = "Now"
definition = "today or tomorrow"

[[priorities]]
name = "Later"

[sizes]
m = "3h"
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DayStart != 0 {
		t.Errorf("DayStart = %v; want 0", cfg.DayStart)
	}
	if len(cfg.Priorities) != 2 || cfg.Priorities[0].Name != "Now" {
		t.Errorf("Priorities = %+v", cfg.Priorities)
	}
	if cfg.Sizes[domain.SizeM] != 3*time.Hour || cfg.Sizes[domain.SizeS] != time.Hour {
		t.Errorf("Sizes = %v; want m overridden, s default", cfg.Sizes)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		`day_start = "4am"`:                  "day_start",
		`day_start = "24:00"`:                "day_start",
		`priorities = []`:                    "1 to 9 levels",
		"[[priorities]]\ndefinition = \"x\"": "name is required",
		"[sizes]\nxs = \"1h\"":               "unknown size",
		"[sizes]\nx = \"1h\"":                "unknown size",
		"[sizes]\ns = \"soon\"":              "positive duration",
		`daystart = "04:00"`:                 "unknown config key",
	}
	for input, wantErr := range cases {
		_, err := Parse([]byte(input))
		if err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}
