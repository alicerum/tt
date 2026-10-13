package config

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// The example config at the repo root lists every key with its default, so
// it must parse and stay equal to Default().
func TestExampleConfigIsTheDefaults(t *testing.T) {
	cfg, err := Load("../../config.example.toml")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(Default(), cfg, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("config.example.toml differs from Default() (-default +example):\n%s", diff)
	}
}
