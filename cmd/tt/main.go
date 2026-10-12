// Command tt is a personal task tracker. See docs/superpowers/specs.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"tt/internal/app"
	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/store"
)

const usage = `usage:
  tt add <line>    capture a task, e.g. tt add "Reply to Ana @ana #photos !1 ~s -- before Friday"

environment:
  TT_CONFIG   config file (default ~/.config/tt/config.toml)
  TT_DB       database file (default ~/Library/Application Support/tt/tt.db)
`

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

// run is main without process globals, so tests can call it directly.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	if len(args) == 0 || args[0] != "add" {
		fmt.Fprint(stderr, usage)
		return 2
	}
	// Join the rest so both `tt add "a b"` and `tt add a b` work.
	line := strings.TrimSpace(strings.Join(args[1:], " "))
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
