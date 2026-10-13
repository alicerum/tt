package calendar

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// helper writes a fake tt-cal script into dir and returns its path.
func helper(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "tt-cal")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

var (
	from = time.Date(2026, 10, 12, 4, 0, 0, 0, time.FixedZone("CEST", 2*3600))
	to   = from.Add(24 * time.Hour)
)

func TestEventsRunsTheHelper(t *testing.T) {
	dir := t.TempDir()
	path := helper(t, dir, `for a in "$@"; do echo "$a"; done > "$(dirname "$0")/args"
cat <<'JSON'
[{"title":"Standup","start":"2026-10-12T09:00:00+02:00","end":"2026-10-12T09:15:00+02:00","all_day":false,
  "availability":"busy","status":"accepted","calendar":"Work"},
 {"title":"Holiday","start":"2026-10-12T00:00:00+02:00","end":"2026-10-13T00:00:00+02:00","all_day":true,
  "availability":"free","status":"","calendar":"Holidays"}]
JSON`)
	ek := &EventKit{Calendars: []string{"Work", "Team X"}, Path: path}
	events, err := ek.Events(context.Background(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %+v", events)
	}
	e := events[0]
	if e.Title != "Standup" || !e.Start.Equal(from.Add(5*time.Hour)) || e.End.Sub(e.Start) != 15*time.Minute ||
		e.AllDay || e.Availability != "busy" || e.Status != "accepted" || e.Calendar != "Work" {
		t.Errorf("first event = %+v", e)
	}
	if !events[1].AllDay || events[1].Availability != "free" {
		t.Errorf("second event = %+v", events[1])
	}
	args, err := os.ReadFile(filepath.Join(dir, "args"))
	if err != nil {
		t.Fatal(err)
	}
	want := "events\n--from\n2026-10-12T04:00:00+02:00\n--to\n2026-10-13T04:00:00+02:00\n--calendars\nWork\nTeam X\n"
	if string(args) != want {
		t.Errorf("args:\n%s\nwant:\n%s", args, want)
	}
}

func TestEventsFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       string
	}{
		{"no access", "exit 2", ErrNoAccess.Error()},
		{"unknown calendar", "echo 'Team X' >&2; exit 3", `unknown calendar "Team X"`},
		{"stderr", "printf '\\n  EventKit exploded  \\nmore detail\\n' >&2; exit 1", "EventKit exploded"},
		{"silent", "exit 4", "tt-cal failed: exit status 4"},
		{"bad JSON", "echo '{not json'", "tt-cal printed bad JSON: "},
		{"bad time", `echo '[{"title":"x","start":"9am","end":"10am"}]'`, "tt-cal printed bad JSON: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ek := &EventKit{Path: helper(t, t.TempDir(), tc.body)}
			_, err := ek.Events(context.Background(), from, to)
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Errorf("err = %v; want %q", err, tc.want)
			}
		})
	}
}

func TestTimeoutSaysWhatToDo(t *testing.T) {
	// The macOS permission prompt waiting for a click is the likeliest timeout.
	const line = "waiting for calendar access · answer the macOS prompt"
	ek := &EventKit{Path: helper(t, t.TempDir(), "echo '"+line+"' >&2; sleep 10"), Timeout: 500 * time.Millisecond}
	began := time.Now()
	_, err := ek.Events(context.Background(), from, to)
	if err == nil || err.Error() != line {
		t.Errorf("err = %v; want %q", err, line)
	}
	if took := time.Since(began); took > 3*time.Second {
		t.Errorf("Events took %v after a timeout", took)
	}
}

func TestEventsTimesOut(t *testing.T) {
	// The sleep is a child of the shell: killing the shell leaves it holding
	// the output pipe, which must not keep Events waiting either.
	ek := &EventKit{Path: helper(t, t.TempDir(), "sleep 10"), Timeout: 100 * time.Millisecond}
	began := time.Now()
	_, err := ek.Events(context.Background(), from, to)
	if err == nil || err.Error() != "tt-cal timed out after 100ms" {
		t.Errorf("err = %v; want the timeout", err)
	}
	if took := time.Since(began); took > 3*time.Second {
		t.Errorf("Events took %v after a timeout", took)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ek.Events(ctx, from, to); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled context: err = %v; want context.Canceled", err)
	}
}

func TestEventsFindsTheHelper(t *testing.T) {
	ek := &EventKit{Path: filepath.Join(t.TempDir(), "tt-cal")} // not built
	if _, err := ek.Events(context.Background(), from, to); !errors.Is(err, ErrHelperNotFound) {
		t.Errorf("missing helper: err = %v; want ErrHelperNotFound", err)
	}

	empty := t.TempDir()
	t.Setenv("PATH", empty)
	if _, err := (&EventKit{}).Events(context.Background(), from, to); !errors.Is(err, ErrHelperNotFound) {
		t.Errorf("not on PATH: err = %v; want ErrHelperNotFound", err)
	}
	onPath := t.TempDir()
	helper(t, onPath, "echo '[]'")
	t.Setenv("PATH", onPath+string(os.PathListSeparator)+"/bin:/usr/bin")
	if events, err := (&EventKit{}).Events(context.Background(), from, to); err != nil || len(events) != 0 {
		t.Errorf("on PATH: %v, %v; want no events and no error", events, err)
	}

	beside := t.TempDir() // next to tt wins over PATH (D40)
	if got := locate(beside); got != "tt-cal" {
		t.Errorf("locate without a helper beside tt = %q; want the PATH lookup", got)
	}
	p := helper(t, beside, "echo '[]'")
	if got := locate(beside); got != p {
		t.Errorf("locate = %q; want %q", got, p)
	}
}

func TestFake(t *testing.T) {
	var f Fake
	f.Set(nil, errors.New("down"))
	if _, err := f.Events(context.Background(), from, to); err == nil || f.Calls() != 1 {
		t.Errorf("Set's error comes back: %v, calls %d", err, f.Calls())
	}
	if a, b := f.Asked(); !a.Equal(from) || !b.Equal(to) {
		t.Errorf("Asked = %v, %v", a, b)
	}
}
