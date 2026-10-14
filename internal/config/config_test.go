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
	if cfg.DayStart != 4*time.Hour || cfg.Sizes[domain.SizeXL] != 40*time.Hour {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestParseOverrides(t *testing.T) {
	cfg, err := Parse([]byte(`
day_start = "00:00"

[sizes]
m = "3h"
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DayStart != 0 {
		t.Errorf("DayStart = %v; want 0", cfg.DayStart)
	}
	if cfg.Sizes[domain.SizeM] != 3*time.Hour || cfg.Sizes[domain.SizeS] != time.Hour {
		t.Errorf("Sizes = %v; want m overridden, s default", cfg.Sizes)
	}
}

// The keys of the P-levels say what replaced them instead of "unknown
// config key", so an old config.toml tells its owner what to do (D76).
func TestPriorityKeysAreGone(t *testing.T) {
	cases := map[string]string{
		"[[priorities]]\nname = \"P1\"\ndefinition = \"x\"": "[[priorities]] was replaced by impact (D68): remove it, see config.example.toml",
		"priorities = []":             "[[priorities]] was replaced by impact (D68)",
		"[github]\npriority = \"P2\"": "[github] priority was replaced by impact and due (D75): remove it, see config.example.toml",
	}
	for input, want := range cases {
		if _, err := Parse([]byte(input)); err == nil || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("Parse(%q) error = %v; want %q", input, err, want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		`day_start = "4am"`:     "day_start",
		`day_start = "24:00"`:   "day_start",
		"[sizes]\nxs = \"1h\"":  "unknown size",
		"[sizes]\nx = \"1h\"":   "unknown size",
		"[sizes]\ns = \"soon\"": "positive duration",
		`daystart = "04:00"`:    "unknown config key",
	}
	for input, wantErr := range cases {
		_, err := Parse([]byte(input))
		if err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}
