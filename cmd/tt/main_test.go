package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tt/internal/app"
	"tt/internal/domain"
	"tt/internal/store"
)

type result struct {
	code           int
	stdout, stderr string
}

func runTT(t *testing.T, env map[string]string, args ...string) result {
	t.Helper()
	return runWithInput(t, env, input{r: strings.NewReader("")}, args...)
}

func runWithInput(t *testing.T, env map[string]string, in input, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, in, &out, &errOut, func(k string) string { return env[k] }, noUI)
	return result{code, out.String(), errOut.String()}
}

func storedTask(t *testing.T, env map[string]string, id int64) domain.Task {
	t.Helper()
	st, err := store.Open(context.Background(), env["TT_DB"])
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	snap, err := st.LoadSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	task, ok := snap.Get(id)
	if !ok {
		t.Fatalf("task #%d not stored", id)
	}
	return task
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
	for _, args := range [][]string{{"bogus"}, {"add"}, {"add", "  "}} {
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

// noUI fails the test if the TUI would start.
func noUI(context.Context, *app.Service) error { panic("TUI started") }

func TestNoArgsStartsTheTUI(t *testing.T) {
	env := testEnv(t)
	var got *app.Service
	var out, errOut bytes.Buffer
	code := run(context.Background(), nil, input{}, &out, &errOut, func(k string) string { return env[k] },
		func(_ context.Context, svc *app.Service) error { got = svc; return nil })
	if code != 0 || got == nil {
		t.Errorf("tt with no args: exit %d, ui started: %v, stderr %q", code, got != nil, errOut.String())
	}
}

func TestTUIErrorsExitNonZero(t *testing.T) {
	env := testEnv(t)
	var out, errOut bytes.Buffer
	code := run(context.Background(), nil, input{}, &out, &errOut, func(k string) string { return env[k] },
		func(context.Context, *app.Service) error { return errors.New("terminal went away") })
	if code != 1 || !strings.Contains(errOut.String(), "tt: terminal went away") {
		t.Errorf("ui error: exit %d, stderr %q", code, errOut.String())
	}

	// The database can't be opened: its directory is a file.
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	env["TT_DB"] = filepath.Join(blocker, "tt.db")
	errOut.Reset()
	code = run(context.Background(), nil, input{}, &out, &errOut, func(k string) string { return env[k] }, noUI)
	if code != 1 || !strings.HasPrefix(errOut.String(), "tt: ") {
		t.Errorf("open error: exit %d, stderr %q", code, errOut.String())
	}
}

func TestAddFromAPrompt(t *testing.T) {
	env := testEnv(t)
	in := input{r: strings.NewReader("Reply to Ana! @ana ~s #photos?\nignored second line\n"), terminal: true}
	r := runWithInput(t, env, in, "add")
	if r.code != 0 || r.stdout != "added #1 to inbox\n" || r.stderr != "add › " {
		t.Fatalf("prompted add: %+v", r)
	}
	if got := storedTask(t, env, 1); got.Title != "Reply to Ana!" || got.Requester != "ana" || got.Note != "" ||
		len(got.Tags) != 1 || got.Tags[0] != "photos" {
		t.Errorf("stored: %+v", got)
	}
	// Nothing typed (or ctrl+d): usage, nothing stored.
	if r := runWithInput(t, env, input{r: strings.NewReader("   \n"), terminal: true}, "add"); r.code != 2 || !strings.Contains(r.stderr, "usage:") {
		t.Errorf("empty prompt: %+v", r)
	}
}

func TestAddFromAPipe(t *testing.T) {
	env := testEnv(t)
	in := input{r: strings.NewReader("Look at the schema !1 ~m -- from Slack\r\nAna: can you check\r\n\r\nthe migration?\r\n")}
	if r := runWithInput(t, env, in, "add"); r.code != 0 || r.stdout != "added #1 to backlog\n" || r.stderr != "" {
		t.Fatalf("piped add: %+v", r)
	}
	if got := storedTask(t, env, 1); got.Title != "Look at the schema" || got.Note != "from Slack\nAna: can you check\n\nthe migration?" {
		t.Errorf("stored: %+v", got)
	}
	if r := runWithInput(t, env, input{r: strings.NewReader("\nbody without a title\n")}, "add"); r.code != 2 || !strings.Contains(r.stderr, "usage:") {
		t.Errorf("empty first line: %+v", r)
	}
	// A line on the command line wins; stdin isn't read.
	if r := runWithInput(t, env, input{r: strings.NewReader("from stdin")}, "add", "from args"); r.code != 0 {
		t.Fatalf("args add: %+v", r)
	}
	if got := storedTask(t, env, 2); got.Title != "from args" {
		t.Errorf("stored: %+v", got)
	}
}

func TestHelp(t *testing.T) {
	env := testEnv(t)
	for _, arg := range []string{"-h", "--help", "help"} {
		if r := runTT(t, env, arg); r.code != 0 || !strings.Contains(r.stdout, "usage:") {
			t.Errorf("tt %s: %+v; want exit 0 with usage on stdout", arg, r)
		}
	}
}
