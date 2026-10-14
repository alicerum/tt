package config

import (
	"slices"
	"strings"
	"testing"
)

func TestParseNotify(t *testing.T) {
	n := Default().Notify
	if n.Meetings || n.Blocks || n.Reviews || n.On() || !slices.Equal(n.Alerts, []int{5, 0}) || n.Sound != "Glass" {
		t.Errorf("defaults: %+v; want every kind off, alerts [5 0], sound Glass (D63, D64, D67)", n)
	}
	cfg, err := Parse([]byte("[notify]\nmeetings = true\nreviews = true\nalerts = [10, 1, 0]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if n := cfg.Notify; !n.Meetings || n.Blocks || !n.Reviews || !n.On() || !slices.Equal(n.Alerts, []int{10, 1, 0}) {
		t.Errorf("notify: %+v", n)
	}
	if cfg, err := Parse([]byte("[notify]\nblocks = true\n")); err != nil || !cfg.Notify.On() || !slices.Equal(cfg.Notify.Alerts, []int{5, 0}) {
		t.Errorf("blocks only: %+v, %v; want on, with the default alerts", cfg.Notify, err)
	}
	for in, want := range map[string]string{`"Ping"`: "Ping", `""`: "", `" Hero "`: "Hero"} {
		if cfg, err := Parse([]byte("[notify]\nsound = " + in + "\n")); err != nil || cfg.Notify.Sound != want {
			t.Errorf("sound = %s: %q, %v; want %q", in, cfg.Notify.Sound, err, want)
		}
	}
	if cfg, err := Parse([]byte("[notify]\nalerts = [120]\n")); err != nil || !slices.Equal(cfg.Notify.Alerts, []int{120}) {
		t.Errorf("alerts [120]: %v, %v", cfg.Notify.Alerts, err)
	}

	cases := map[string]string{
		"[notify]\nalerts = []":        "notify.alerts: list at least one",
		"[notify]\nalerts = [5, -1]":   "notify.alerts[1]: want whole minutes from 0 to 120, got -1",
		"[notify]\nalerts = [121]":     "notify.alerts[0]: want whole minutes from 0 to 120, got 121",
		"[notify]\nalerts = [5, 0, 5]": "notify.alerts: 5 is listed twice",
		"[notify]\nalerts = [2.5]":     "notify.alerts",
		"[notify]\nalerts = [\"5m\"]":  "notify.alerts",
		"[notify]\nalerts = 5":         "notify.alerts",
		"[notify]\nmeetings = \"yes\"": "notify.meetings",
		"[notify]\nsound = true":       "notify.sound",
		"[notify]\nsund = \"Ping\"":    "unknown config key",
	}
	for input, wantErr := range cases {
		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("Parse(%q) error = %v; want containing %q", input, err, wantErr)
		}
	}
}
