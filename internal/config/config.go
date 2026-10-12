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
	// UnsizedBlock is the default block length for unsized tasks such as PR reviews (D24).
	UnsizedBlock time.Duration
	// RolloverAlert is the rollover count at which planning flags a task (D10a).
	RolloverAlert int
	// WorkStart and WorkEnd are the working hours (D39), as offsets from
	// midnight: Today's timeline shows them, and blocks must fit inside them.
	WorkStart, WorkEnd time.Duration
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
		UnsizedBlock:  30 * time.Minute,
		RolloverAlert: 3,
		WorkStart:     8 * time.Hour,
		WorkEnd:       19 * time.Hour,
	}
}

// file mirrors the TOML layout; every field is optional.
type file struct {
	DayStart   *string           `toml:"day_start"`
	Priorities []PriorityLevel   `toml:"priorities"`
	Sizes      map[string]string `toml:"sizes"`
	Planning   struct {
		UnsizedBlock  *string `toml:"unsized_block"`
		RolloverAlert *int    `toml:"rollover_alert"`
	} `toml:"planning"`
	WorkHours struct {
		Start *string `toml:"start"`
		End   *string `toml:"end"`
	} `toml:"work_hours"`
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
	if p := f.Planning.UnsizedBlock; p != nil {
		d, err := time.ParseDuration(*p)
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("planning.unsized_block: want a positive duration like \"30m\", got %q", *p)
		}
		cfg.UnsizedBlock = d
	}
	if p := f.Planning.RolloverAlert; p != nil {
		if *p < 1 {
			return Config{}, fmt.Errorf("planning.rollover_alert: want 1 or more, got %d", *p)
		}
		cfg.RolloverAlert = *p
	}
	if p := f.WorkHours.Start; p != nil {
		if cfg.WorkStart, err = parseClock(*p); err != nil {
			return Config{}, fmt.Errorf("work_hours.start: %w", err)
		}
	} else {
		cfg.WorkStart = max(cfg.WorkStart, cfg.DayStart) // a late day_start moves the default along
	}
	if p := f.WorkHours.End; p != nil {
		if cfg.WorkEnd, err = parseClock(*p); err != nil {
			return Config{}, fmt.Errorf("work_hours.end: %w", err)
		}
	} else if cfg.WorkEnd <= cfg.WorkStart {
		cfg.WorkEnd = min(cfg.WorkStart+time.Hour, 24*time.Hour-time.Minute)
	}
	switch {
	case cfg.WorkEnd <= cfg.WorkStart:
		return Config{}, fmt.Errorf("work_hours.end must be after work_hours.start")
	case cfg.WorkStart < cfg.DayStart:
		return Config{}, fmt.Errorf("work_hours.start must not be before day_start (rows before it belong to the previous day)")
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
