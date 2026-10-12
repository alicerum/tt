// Package config loads ~/.config/tt/config.toml and applies defaults.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"tt/internal/domain"
)

type PriorityLevel struct {
	Name       string `toml:"name"`
	Definition string `toml:"definition"`
}

type Config struct {
	DayStart   time.Duration // offset from midnight when a new planning day begins (D19)
	Priorities []PriorityLevel
	Sizes      map[domain.Size]time.Duration
}

func Default() Config {
	return Config{
		DayStart: 4 * time.Hour,
		Priorities: []PriorityLevel{
			{Name: "P1", Definition: "someone is blocked / committed this week"},
			{Name: "P2", Definition: "important this month"},
			{Name: "P3", Definition: "worth doing eventually"},
		},
		Sizes: map[domain.Size]time.Duration{
			domain.SizeS:  time.Hour,
			domain.SizeM:  4 * time.Hour,
			domain.SizeL:  8 * time.Hour,
			domain.SizeXL: 40 * time.Hour,
		},
	}
}

// file mirrors the TOML layout; every field is optional.
type file struct {
	DayStart   *string           `toml:"day_start"`
	Priorities []PriorityLevel   `toml:"priorities"`
	Sizes      map[string]string `toml:"sizes"`
}

// Load reads path. A missing file yields Default().
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, err
	}
	cfg, err := Parse(data)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Parse overlays TOML data on Default(). Unknown keys are errors, so typos surface.
func Parse(data []byte) (Config, error) {
	var f file
	md, err := toml.Decode(string(data), &f)
	if err != nil {
		return Config{}, err
	}
	if und := md.Undecoded(); len(und) > 0 {
		return Config{}, fmt.Errorf("unknown config key %q", und[0].String())
	}

	cfg := Default()
	if f.DayStart != nil {
		if cfg.DayStart, err = parseClock(*f.DayStart); err != nil {
			return Config{}, fmt.Errorf("day_start: %w", err)
		}
	}
	if f.Priorities != nil {
		if n := len(f.Priorities); n < 1 || n > 9 {
			return Config{}, fmt.Errorf("priorities: need 1 to 9 levels, got %d", n)
		}
		for i, p := range f.Priorities {
			if strings.TrimSpace(p.Name) == "" {
				return Config{}, fmt.Errorf("priorities[%d]: name is required", i)
			}
		}
		cfg.Priorities = f.Priorities
	}
	for key, val := range f.Sizes {
		size, ok := domain.ParseSize(key)
		if !ok || string(size) != strings.ToLower(key) {
			return Config{}, fmt.Errorf("sizes: unknown size %q (use s, m, l, xl)", key)
		}
		d, err := time.ParseDuration(val)
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("sizes.%s: want a positive duration like \"4h\", got %q", key, val)
		}
		cfg.Sizes[size] = d
	}
	return cfg, nil
}

// parseClock parses "HH:MM" (00:00 to 23:59) into an offset from midnight.
func parseClock(s string) (time.Duration, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, fmt.Errorf("want HH:MM, got %q", s)
	}
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute, nil
}
