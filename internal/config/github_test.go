package config

import (
	"strings"
	"testing"
	"time"

	"tt/internal/domain"
)

func TestParseGitHub(t *testing.T) {
	cfg := Default()
	if !cfg.GitHub || cfg.GitHubHost != "" || cfg.GitHubPoll != 5*time.Minute || cfg.ReviewPriority != 2 || cfg.ReviewSize != domain.SizeS {
		t.Errorf("defaults: %v %q %v P%d %q; want on, gh's host, 5m, P2, S (D48, D50)",
			cfg.GitHub, cfg.GitHubHost, cfg.GitHubPoll, cfg.ReviewPriority, cfg.ReviewSize)
	}
	cfg, err := Parse([]byte("[github]\nenabled = false\nhost = \" github.acme.com \"\npoll = \"30s\"\npriority = \"p1\"\nsize = \"\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitHub || cfg.GitHubHost != "github.acme.com" || cfg.GitHubPoll != 30*time.Second || cfg.ReviewPriority != 1 || cfg.ReviewSize != domain.SizeNone {
		t.Errorf("github: %v %q %v P%d %q", cfg.GitHub, cfg.GitHubHost, cfg.GitHubPoll, cfg.ReviewPriority, cfg.ReviewSize)
	}
	if cfg, err := Parse([]byte("[github]\nsize = \"xl\"\n")); err != nil || cfg.ReviewSize != domain.SizeXL {
		t.Errorf("size xl: %q, %v", cfg.ReviewSize, err)
	}
	if cfg, err := Parse([]byte("[github]\nsize = \" m \"\n")); err != nil || cfg.ReviewSize != domain.SizeM {
		t.Errorf("size \" m \": %q, %v", cfg.ReviewSize, err)
	}

	// Other priority levels: the default follows the level named P2, else the last.
	two := "[[priorities]]\nname = \"Now\"\n[[priorities]]\nname = \"P2\"\n[[priorities]]\nname = \"Later\"\n[[priorities]]\nname = \"Never\"\n"
	if cfg, err := Parse([]byte(two)); err != nil || cfg.ReviewPriority != 2 {
		t.Errorf("a level named P2: P%d, %v", cfg.ReviewPriority, err)
	}
	none := "[[priorities]]\nname = \"Now\"\n[[priorities]]\nname = \"Soon\"\n[[priorities]]\nname = \"Later\"\n"
	if cfg, err := Parse([]byte(none)); err != nil || cfg.ReviewPriority != 3 {
		t.Errorf("no level named P2: P%d, %v; want the last level", cfg.ReviewPriority, err)
	}
	if cfg, err := Parse([]byte(none + "[github]\npriority = \"Soon\"\n")); err != nil || cfg.ReviewPriority != 2 {
		t.Errorf("priority Soon: P%d, %v", cfg.ReviewPriority, err)
	}

	cases := map[string]string{
		"[github]\npoll = \"10s\"":           "github.poll: want a duration of 30s or more",
		"[github]\npoll = \"5\"":             "github.poll: want a duration of 30s or more",
		"[github]\npriority = \"P7\"":        `github.priority: no priority level named "P7"`,
		none + "[github]\npriority = \"P2\"": `github.priority: no priority level named "P2"`,
		"[github]\nsize = \"huge\"":          "github.size: want s, m, l, xl, or \"\" for unsized",
		"[github]\nsize = \"x\"":             "github.size: want s, m, l, xl",
		"[github]\nenabled = \"yes\"":        "enabled",
		"[github]\ntoken = \"x\"":            "unknown config key",
	}
	for input, wantErr := range cases {
		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}
