// Command tt is a personal task tracker. See docs/superpowers/specs.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"tt/internal/app"
	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/dsl"
	"tt/internal/store"
	"tt/internal/tui"
)

const usage = `usage:
  tt               open the planner (TUI)
  tt add <line>    capture a task, e.g. tt add 'Reply to Ana @ana #photos !1 ~s -- before Friday'
                   quote the line with single quotes: ! and ~ are special in zsh/bash
  tt add           no line: type it at the add › prompt (no quoting needed), or pipe it in;
                   piped lines after the first become the note, e.g. pbpaste | tt add

environment:
  TT_CONFIG   config file (default ~/.config/tt/config.toml)
  TT_DB       database file (default ~/Library/Application Support/tt/tt.db)
`

func main() {
	in := input{r: os.Stdin}
	if fi, err := os.Stdin.Stat(); err == nil {
		in.terminal = fi.Mode()&os.ModeCharDevice != 0
	}
	if in.terminal {
		in.pending = ttyPending(os.Stdin)
	}
	os.Exit(run(context.Background(), os.Args[1:], in, os.Stdout, os.Stderr, os.Getenv, runTUI))
}

// input is where `tt add` with no line reads from (D33).
type input struct {
	r        io.Reader
	terminal bool // prompt for one line; otherwise read a pipe to the end
	// pending returns the complete lines already waiting on the terminal
	// after line 1 (a paste) and discards anything left, so no pasted line
	// reaches the shell once tt exits. Nil: nothing is waiting.
	pending func() (string, error)
}

// maxPiped bounds what `tt add` reads from a pipe.
const maxPiped = 1 << 20

// readLine reads the capture line: from a terminal, one line after an
// "add › " prompt on stderr; from a pipe, line 1 plus lines 2+ as the note.
func (in input) readLine(stderr io.Writer) (string, error) {
	if in.terminal {
		fmt.Fprint(stderr, "add › ")
		br := bufio.NewReader(in.r)
		line, err := br.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		// A paste behaves like a pipe: line 1 is the line, the rest the note.
		buffered, _ := br.Peek(br.Buffered())
		rest := string(buffered)
		if in.pending != nil {
			more, err := in.pending()
			if err != nil {
				return "", err
			}
			rest += more
		}
		first := strings.TrimSpace(line)
		if first == "" {
			return "", nil
		}
		return dsl.WithNoteLines(first, strings.Split(strings.ReplaceAll(rest, "\r\n", "\n"), "\n")), nil
	}
	data, err := io.ReadAll(io.LimitReader(in.r, maxPiped+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxPiped {
		return "", errors.New("input is over 1 MiB")
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	first := strings.TrimSpace(lines[0])
	if first == "" {
		return "", nil
	}
	return dsl.WithNoteLines(first, lines[1:]), nil
}

// runTUI runs the full-screen planner until the user quits.
func runTUI(ctx context.Context, svc *app.Service) error {
	_, err := tea.NewProgram(tui.New(ctx, svc, tui.DefaultOptions()), tea.WithContext(ctx)).Run()
	return err
}

// run is main without process globals, so tests can call it directly.
func run(ctx context.Context, args []string, in input, stdout, stderr io.Writer, getenv func(string) string,
	ui func(context.Context, *app.Service) error) int {
	switch {
	case len(args) == 0:
		svc, closeFn, err := open(ctx, getenv)
		if err != nil {
			fmt.Fprintf(stderr, "tt: %v\n", err)
			return 1
		}
		defer closeFn()
		if err := ui(ctx, svc); err != nil {
			fmt.Fprintf(stderr, "tt: %v\n", err)
			return 1
		}
		return 0
	case args[0] == "-h" || args[0] == "--help" || args[0] == "help":
		fmt.Fprint(stdout, usage)
		return 0
	case args[0] != "add":
		fmt.Fprint(stderr, usage)
		return 2
	}
	// Join the rest so both `tt add "a b"` and `tt add a b` work.
	line := strings.TrimSpace(strings.Join(args[1:], " "))
	if len(args) == 1 {
		var err error
		if line, err = in.readLine(stderr); err != nil {
			fmt.Fprintf(stderr, "tt: %v\n", err)
			return 1
		}
	}
	if line == "" {
		fmt.Fprint(stderr, usage)
		return 2
	}

	svc, closeFn, err := open(ctx, getenv)
	if err != nil {
		fmt.Fprintf(stderr, "tt: %v\n", err)
		return 1
	}
	defer closeFn()

	task, _, err := svc.Add(ctx, line)
	if err != nil {
		fmt.Fprintf(stderr, "tt: %v\n", err)
		return 1
	}
	where := "backlog"
	if domain.InInbox(task, nil) {
		where = "inbox"
	}
	fmt.Fprintf(stdout, "added #%d to %s\n", task.ID, where)
	return 0
}

func open(ctx context.Context, getenv func(string) string) (*app.Service, func(), error) {
	configPath, dbPath, err := paths(getenv)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, nil, err
	}
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return nil, nil, err
	}
	return app.New(st, app.SystemClock{}, cfg), func() { st.Close() }, nil
}

func paths(getenv func(string) string) (configPath, dbPath string, err error) {
	configPath, dbPath = getenv("TT_CONFIG"), getenv("TT_DB")
	if configPath != "" && dbPath != "" {
		return configPath, dbPath, nil
	}
	home := getenv("HOME")
	if home == "" {
		return "", "", fmt.Errorf("HOME is not set; set TT_CONFIG and TT_DB")
	}
	if configPath == "" {
		configPath = filepath.Join(home, ".config", "tt", "config.toml")
	}
	if dbPath == "" {
		dbPath = filepath.Join(home, "Library", "Application Support", "tt", "tt.db")
	}
	return configPath, dbPath, nil
}
