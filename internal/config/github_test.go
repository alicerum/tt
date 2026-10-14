package config

import (
	"strings"
	"testing"
	"time"

	"tt/internal/domain"
)

func TestParseGitHub(t *testing.T) {
	cfg := Default()
	if !cfg.GitHub || cfg.GitHubHost != "" || cfg.GitHubPoll != 5*time.Minute || cfg.ReviewSize != domain.SizeS {
		t.Errorf("defaults: %v %q %v %q; want on, gh's host, 5m, S (D48, D50)",
			cfg.GitHub, cfg.GitHubHost, cfg.GitHubPoll, cfg.ReviewSize)
	}
	cfg, err := Parse([]byte("[github]\nenabled = false\nhost = \" github.acme.com \"\npoll = \"30s\"\nsize = \"\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitHub || cfg.GitHubHost != "github.acme.com" || cfg.GitHubPoll != 30*time.Second || cfg.ReviewSize != domain.SizeNone {
		t.Errorf("github: %v %q %v %q", cfg.GitHub, cfg.GitHubHost, cfg.GitHubPoll, cfg.ReviewSize)
	}
	if cfg, err := Parse([]byte("[github]\nsize = \"xl\"\n")); err != nil || cfg.ReviewSize != domain.SizeXL {
		t.Errorf("size xl: %q, %v", cfg.ReviewSize, err)
	}
	if cfg, err := Parse([]byte("[github]\nsize = \" m \"\n")); err != nil || cfg.ReviewSize != domain.SizeM {
		t.Errorf("size \" m \": %q, %v", cfg.ReviewSize, err)
	}

	cases := map[string]string{
		"[github]\npoll = \"10s\"":    "github.poll: want a duration of 30s or more",
		"[github]\npoll = \"5\"":      "github.poll: want a duration of 30s or more",
		"[github]\nsize = \"huge\"":   "github.size: want s, m, l, xl, or \"\" for unsized",
		"[github]\nsize = \"x\"":      "github.size: want s, m, l, xl",
		"[github]\nenabled = \"yes\"": "enabled",
		"[github]\ntoken = \"x\"":     "unknown config key",
	}
	for input, wantErr := range cases {
		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}
