package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tt/internal/store"
)

type result struct {
	code           int
	stdout, stderr string
}

func runTT(t *testing.T, env map[string]string, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, &out, &errOut, func(k string) string { return env[k] })
	return result{code, out.String(), errOut.String()}
}

func testEnv(t *testing.T) map[string]string {
	dir := t.TempDir()
	return map[string]string{
		"TT_CONFIG": filepath.Join(dir, "missing.toml"),
		"TT_DB":     filepath.Join(dir, "data dir", "tt.db"),
	}
}

func TestAddReportsWhereTaskLanded(t *testing.T) {
	env := testEnv(t)
	if r := runTT(t, env, "add", "Reply to Ana @ana !1 ~s"); r.code != 0 || r.stdout != "added #1 to backlog\n" {
		t.Errorf("triaged add: %+v", r)
	}
	// Unquoted words are joined, as a shell would split them.
	if r := runTT(t, env, "add", "Look", "into", "flaky", "e2e", "#profiles"); r.code != 0 || r.stdout != "added #2 to inbox\n" {
		t.Errorf("untriaged add: %+v", r)
	}

	st, err := store.Open(context.Background(), env["TT_DB"])
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	snap, _ := st.LoadSnapshot(context.Background())
	task, ok := snap.Get(2)
	if !ok || task.Title != "Look into flaky e2e" || task.Tags[0] != "profiles" {
		t.Errorf("stored task: %+v", task)
	}
}

func TestUsageErrors(t *testing.T) {
	env := testEnv(t)
	for _, args := range [][]string{nil, {"bogus"}, {"add"}, {"add", "  "}} {
		if r := runTT(t, env, args...); r.code != 2 || !strings.Contains(r.stderr, "usage:") {
			t.Errorf("tt %v: %+v; want exit 2 with usage", args, r)
		}
	}
}

func TestAddErrorsExitNonZero(t *testing.T) {
	env := testEnv(t)
	if r := runTT(t, env, "add", "x !9"); r.code != 1 || !strings.Contains(r.stderr, "use !1 to !3") {
		t.Errorf("bad priority: %+v", r)
	}
	if r := runTT(t, env, "add", "@ana #only-tokens"); r.code != 1 || !strings.Contains(r.stderr, "title is required") {
		t.Errorf("no title: %+v", r)
	}
}

func TestConfigIsHonoured(t *testing.T) {
	env := testEnv(t)
	if err := os.WriteFile(env["TT_CONFIG"], []byte("[[priorities]]\nname = \"Now\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := runTT(t, env, "add", "x !2"); r.code != 1 || !strings.Contains(r.stderr, "use !1 to !1") {
		t.Errorf("one configured level: %+v", r)
	}
	if err := os.WriteFile(env["TT_CONFIG"], []byte("bogus = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := runTT(t, env, "add", "x"); r.code != 1 || !strings.Contains(r.stderr, "unknown config key") {
		t.Errorf("broken config: %+v", r)
	}
}

func TestDefaultPaths(t *testing.T) {
	cfg, db, err := paths(func(k string) string { return map[string]string{"HOME": "/Users/me"}[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg != "/Users/me/.config/tt/config.toml" || db != "/Users/me/Library/Application Support/tt/tt.db" {
		t.Errorf("paths = %q, %q", cfg, db)
	}
	if _, _, err := paths(func(string) string { return "" }); err == nil {
		t.Error("paths without HOME should fail")
	}
}
