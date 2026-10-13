// Package calendar reads meetings from macOS Calendar through the tt-cal
// helper (spec: Adapters). It has no rules of its own: domain.Busy decides
// what is busy, and app caches the result.
package calendar

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"tt/internal/domain"
)

// The failures with a fix of their own (D44); their text is what the header shows.
var (
	ErrHelperNotFound = errors.New("tt-cal not found · run helpers/tt-cal/build.sh")
	ErrNoAccess       = errors.New("no access · System Settings → Privacy & Security → Calendars")
)

// DefaultTimeout bounds one helper run (D44).
const DefaultTimeout = 10 * time.Second

// EventKit runs `tt-cal events` for the configured calendars (D40).
type EventKit struct {
	Calendars []string      // the calendars whose events count as busy
	Path      string        // the helper; "" finds it next to the running tt, else on PATH
	Timeout   time.Duration // 0 means DefaultTimeout
}

// Events returns the events overlapping from–to. Errors are short and say
// what to do: ErrHelperNotFound, ErrNoAccess (exit 2), `unknown calendar "X"`
// (exit 3), a timeout, bad JSON, else the first line of the helper's stderr.
func (e *EventKit) Events(ctx context.Context, from, to time.Time) ([]domain.Event, error) {
	path := e.Path
	if path == "" {
		path = locate(executableDir())
	}
	timeout := cmp.Or(e.Timeout, DefaultTimeout)
	run, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{"events", "--from", from.Format(time.RFC3339), "--to", to.Format(time.RFC3339)}
	if len(e.Calendars) > 0 {
		args = append(append(args, "--calendars"), e.Calendars...)
	}
	cmd := exec.CommandContext(run, path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = time.Second // a child left holding the pipes can't keep us waiting
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return parse(stdout.Bytes())
	case errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist):
		return nil, ErrHelperNotFound
	case ctx.Err() != nil:
		return nil, ctx.Err() // tt is quitting
	case run.Err() != nil:
		return nil, fmt.Errorf("tt-cal timed out after %s", timeout)
	case errors.As(err, &exit) && exit.ExitCode() == 2:
		return nil, ErrNoAccess
	case errors.As(err, &exit) && exit.ExitCode() == 3:
		return nil, fmt.Errorf("unknown calendar %q", firstLine(stderr.String()))
	}
	if line := firstLine(stderr.String()); line != "" {
		return nil, errors.New(line)
	}
	return nil, fmt.Errorf("tt-cal failed: %v", err)
}

// rawEvent is one event of the helper's JSON.
type rawEvent struct {
	Title        string    `json:"title"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	AllDay       bool      `json:"all_day"`
	Availability string    `json:"availability"`
	Status       string    `json:"status"`
	Calendar     string    `json:"calendar"`
}

func parse(data []byte) ([]domain.Event, error) {
	var raw []rawEvent
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("tt-cal printed bad JSON: %v", err)
	}
	events := make([]domain.Event, 0, len(raw))
	for _, r := range raw {
		events = append(events, domain.Event{Title: r.Title, Start: r.Start, End: r.End, AllDay: r.AllDay,
			Availability: r.Availability, Status: r.Status, Calendar: r.Calendar})
	}
	return events, nil
}

// locate is the tt-cal next to tt (in dir), else the bare name, which exec
// looks up on PATH (D40).
func locate(dir string) string {
	if dir != "" {
		p := filepath.Join(dir, "tt-cal")
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return "tt-cal"
}

func executableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}
