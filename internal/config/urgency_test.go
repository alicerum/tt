package config

import (
	"strings"
	"testing"

	"tt/internal/domain"
)

func TestParseUrgency(t *testing.T) {
	if w := Default().Urgency; w != (domain.Weights{Impact: 1, Due: 1}) {
		t.Errorf("default weights = %+v; want 1 and 1 (D70)", w)
	}
	cfg, err := Parse([]byte("[urgency]\nimpact = 1\ndue = 2.5\n"))
	if err != nil || cfg.Urgency != (domain.Weights{Impact: 1, Due: 2.5}) {
		t.Errorf("weights 1, 2.5: %+v, %v", cfg.Urgency, err)
	}
	if cfg, err := Parse([]byte("[urgency]\ndue = 0\n")); err != nil || cfg.Urgency != (domain.Weights{Impact: 1, Due: 0}) {
		t.Errorf("due 0 (impact only): %+v, %v", cfg.Urgency, err)
	}
	cases := map[string]string{
		"[urgency]\nimpact = -1":         "urgency.impact: want a weight of 0 or more, got -1",
		"[urgency]\ndue = -0.5":          "urgency.due: want a weight of 0 or more, got -0.5",
		"[urgency]\nimpact = 0\ndue = 0": "urgency: impact and due can't both be 0",
		"[urgency]\nimpact = \"high\"":   "urgency.impact",
		"[urgency]\nimpact = nan":        "urgency.impact",
		"[urgency]\ndue = inf":           "urgency.due",
		"[urgency]\nsize = 1":            "unknown config key",
	}
	for input, wantErr := range cases {
		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}

func TestParseReviewImpactAndDue(t *testing.T) {
	if cfg := Default(); cfg.ReviewImpact != 1 || cfg.ReviewDue != 3 {
		t.Errorf("defaults: impact %d, due %d; want 1 and 3 days (D75)", cfg.ReviewImpact, cfg.ReviewDue)
	}
	for in, want := range map[string]int{`"0d"`: 0, `"5d"`: 5, `" 2d "`: 2, `""`: -1, `"365d"`: 365} {
		if cfg, err := Parse([]byte("[github]\ndue = " + in + "\n")); err != nil || cfg.ReviewDue != want {
			t.Errorf("due = %s: %d, %v; want %d", in, cfg.ReviewDue, err, want)
		}
	}
	if cfg, err := Parse([]byte("[github]\nimpact = 2\n")); err != nil || cfg.ReviewImpact != 2 {
		t.Errorf("impact = 2: %d, %v", cfg.ReviewImpact, err)
	}
	cases := map[string]string{
		"[github]\nimpact = 0":     "github.impact: want 1 to 3, got 0",
		"[github]\nimpact = 4":     "github.impact: want 1 to 3, got 4",
		"[github]\ndue = \"3\"":    `github.due: want whole days like "3d" (0d to 365d), or "" for no due, got "3"`,
		"[github]\ndue = \"1w\"":   "github.due: want whole days",
		"[github]\ndue = \"-1d\"":  "github.due: want whole days",
		"[github]\ndue = \"366d\"": "github.due: want whole days",
		"[github]\ndue = 3":        "github.due",
	}
	for input, wantErr := range cases {
		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}
