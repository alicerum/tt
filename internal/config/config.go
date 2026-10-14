// Package config loads ~/.config/tt/config.toml and applies defaults.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"tt/internal/domain"
)

type Config struct {
	DayStart time.Duration // offset from midnight when a new planning day begins (D19)
	Sizes    map[domain.Size]time.Duration
	// UnsizedBlock is the default block length for unsized tasks such as PR reviews (D24).
	UnsizedBlock time.Duration
	// RolloverAlert is the rollover count at which planning flags a task (D10a).
	RolloverAlert int
	// WorkStart and WorkEnd are the working hours (D39), as offsets from
	// midnight: Today's timeline shows them, and blocks must fit inside them.
	WorkStart, WorkEnd time.Duration
	// Calendars names the calendars whose events count as busy (D40); none
	// means no calendar: nothing is fetched.
	Calendars []string
	// CalendarPoll is how often the calendar is fetched (D3).
	CalendarPoll time.Duration
	// GitHub turns on the Reviews lane: review requests are fetched through
	// gh (D50). GitHubHost is a GitHub Enterprise host; "" is gh's default.
	GitHub     bool
	GitHubHost string
	// GitHubPoll is how often GitHub is fetched (D3).
	GitHubPoll time.Duration
	// ReviewSize, ReviewImpact and ReviewDue are what the task of a pinned
	// review request starts with (D48, D75): its size, its impact, and how
	// many days after the request it is due; ReviewDue -1 is no due.
	ReviewSize   domain.Size
	ReviewImpact int
	ReviewDue    int
	// Urgency weighs impact against the due date (D70).
	Urgency domain.Weights
	// Notify is what the running TUI notifies about (D63) and when (D64).
	Notify Notify
}

// Notify is the [notify] section. Each kind is opt-in (D63); Alerts are the
// minutes before a meeting or block starts at which it notifies, 0 being the
// start itself (D64). Sound is the macOS sound each one plays, "" for none
// (D67).
type Notify struct {
	Meetings, Blocks, Reviews bool
	Alerts                    []int
	Sound                     string
}

// On reports whether any kind of notification is on.
func (n Notify) On() bool { return n.Meetings || n.Blocks || n.Reviews }

func Default() Config {
	return Config{
		DayStart: 4 * time.Hour,
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
		CalendarPoll:  2 * time.Minute,
		GitHub:        true,
		GitHubPoll:    5 * time.Minute,
		ReviewSize:    domain.SizeS,
		ReviewImpact:  1,
		ReviewDue:     3,
		Urgency:       domain.DefaultWeights,
		Notify:        Notify{Alerts: []int{5, 0}, Sound: "Glass"},
	}
}

// file mirrors the TOML layout; every field is optional.
type file struct {
	DayStart *string           `toml:"day_start"`
	Sizes    map[string]string `toml:"sizes"`
	Planning struct {
		UnsizedBlock  *string `toml:"unsized_block"`
		RolloverAlert *int    `toml:"rollover_alert"`
	} `toml:"planning"`
	WorkHours struct {
		Start *string `toml:"start"`
		End   *string `toml:"end"`
	} `toml:"work_hours"`
	Calendar struct {
		Calendars []string `toml:"calendars"`
		Poll      *string  `toml:"poll"`
	} `toml:"calendar"`
	GitHub struct {
		Enabled *bool   `toml:"enabled"`
		Host    *string `toml:"host"`
		Poll    *string `toml:"poll"`
		Size    *string `toml:"size"`
		Impact  *int    `toml:"impact"`
		Due     *string `toml:"due"`
	} `toml:"github"`
	Urgency struct {
		Impact *float64 `toml:"impact"`
		Due    *float64 `toml:"due"`
	} `toml:"urgency"`
	Notify struct {
		Meetings *bool   `toml:"meetings"`
		Blocks   *bool   `toml:"blocks"`
		Reviews  *bool   `toml:"reviews"`
		Alerts   *[]int  `toml:"alerts"`
		Sound    *string `toml:"sound"`
	} `toml:"notify"`
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
		return Config{}, unknownKeys(und)
	}

	cfg := Default()
	if f.DayStart != nil {
		if cfg.DayStart, err = parseClock(*f.DayStart); err != nil {
			return Config{}, fmt.Errorf("day_start: %w", err)
		}
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
	for i, name := range f.Calendar.Calendars {
		if strings.TrimSpace(name) == "" {
			return Config{}, fmt.Errorf("calendar.calendars[%d]: name is empty (tt-cal calendars prints the names)", i)
		}
	}
	cfg.Calendars = f.Calendar.Calendars
	if p := f.Calendar.Poll; p != nil {
		d, err := time.ParseDuration(*p)
		if err != nil || d < 10*time.Second {
			return Config{}, fmt.Errorf("calendar.poll: want a duration of 10s or more, like \"2m\", got %q", *p)
		}
		cfg.CalendarPoll = d
	}
	if err := parseGitHub(f, &cfg); err != nil {
		return Config{}, err
	}
	if err := parseNotify(f, &cfg); err != nil {
		return Config{}, err
	}
	if err := parseUrgency(f, &cfg); err != nil {
		return Config{}, err
	}
	switch {
	case cfg.WorkEnd <= cfg.WorkStart:
		return Config{}, fmt.Errorf("work_hours.end must be after work_hours.start")
	case cfg.WorkStart < cfg.DayStart:
		return Config{}, fmt.Errorf("work_hours.start must not be before day_start (rows before it belong to the previous day)")
	}
	return cfg, nil
}

// unknownKeys reports every D76 key present at once (so an old config needs
// one edit, not two), else the first unknown key.
func unknownKeys(und []toml.Key) error {
	var gone []string
	for _, wantPriorities := range []bool{true, false} {
		for _, k := range und {
			key := k.String()
			if wantPriorities == (key == "priorities" || strings.HasPrefix(key, "priorities.")) && (wantPriorities || key == "github.priority") {
				gone = append(gone, unknownKey(key).Error())
				break
			}
		}
	}
	if len(gone) > 0 {
		return errors.New(strings.Join(gone, "; "))
	}
	return unknownKey(und[0].String())
}

// unknownKey is the error for a key tt doesn't know (typos surface). The
// keys priority had before D68 say what replaced them (D76).
func unknownKey(key string) error {
	switch {
	case key == "priorities" || strings.HasPrefix(key, "priorities."):
		return fmt.Errorf("[[priorities]] was replaced by impact (D68): remove it, see config.example.toml")
	case key == "github.priority":
		return fmt.Errorf("[github] priority was replaced by impact and due (D75): remove it, see config.example.toml")
	}
	return fmt.Errorf("unknown config key %q", key)
}

// parseGitHub applies [github] (D48, D50, D75).
func parseGitHub(f file, cfg *Config) error {
	g := f.GitHub
	if g.Enabled != nil {
		cfg.GitHub = *g.Enabled
	}
	if g.Host != nil {
		cfg.GitHubHost = strings.TrimSpace(*g.Host)
	}
	if p := g.Poll; p != nil {
		d, err := time.ParseDuration(*p)
		if err != nil || d < 30*time.Second {
			return fmt.Errorf("github.poll: want a duration of 30s or more, like \"5m\", got %q", *p)
		}
		cfg.GitHubPoll = d
	}
	if g.Size != nil {
		p := strings.TrimSpace(*g.Size)
		size, ok := domain.ParseSize(p)
		if p != "" && (!ok || string(size) != strings.ToLower(p)) {
			return fmt.Errorf("github.size: want s, m, l, xl, or \"\" for unsized, got %q", p)
		}
		cfg.ReviewSize = size
	}
	if p := g.Impact; p != nil {
		if *p < 1 || *p > domain.MaxImpact {
			return fmt.Errorf("github.impact: want 1 to %d, got %d", domain.MaxImpact, *p)
		}
		cfg.ReviewImpact = *p
	}
	if p := g.Due; p != nil {
		v := strings.TrimSpace(*p)
		n, err := strconv.Atoi(strings.TrimSuffix(v, "d"))
		switch {
		case v == "":
			cfg.ReviewDue = -1
		case !strings.HasSuffix(v, "d") || err != nil || n < 0 || n > 365:
			return fmt.Errorf("github.due: want whole days like \"3d\" (0d to 365d), or \"\" for no due, got %q", *p)
		default:
			cfg.ReviewDue = n
		}
	}
	return nil
}

// parseUrgency applies [urgency] (D70): two weights, each 0 or more, not
// both 0.
func parseUrgency(f file, cfg *Config) error {
	u := f.Urgency
	for _, w := range []struct {
		name string
		from *float64
		to   *float64
	}{{"impact", u.Impact, &cfg.Urgency.Impact}, {"due", u.Due, &cfg.Urgency.Due}} {
		if w.from == nil {
			continue
		}
		if *w.from < 0 || math.IsNaN(*w.from) || math.IsInf(*w.from, 0) {
			return fmt.Errorf("urgency.%s: want a weight of 0 or more, got %v", w.name, *w.from)
		}
		*w.to = *w.from
	}
	if cfg.Urgency.Impact+cfg.Urgency.Due == 0 {
		return fmt.Errorf("urgency: impact and due can't both be 0")
	}
	return nil
}

// parseNotify applies [notify] (D63, D64, D67): alerts are whole minutes
// from 0 to 120, each listed once, at least one (a kind is turned off
// instead); sound is any name, "" for silent (macOS skips one it can't find).
func parseNotify(f file, cfg *Config) error {
	n := f.Notify
	if n.Sound != nil {
		cfg.Notify.Sound = strings.TrimSpace(*n.Sound)
	}
	for _, b := range []struct {
		from *bool
		to   *bool
	}{{n.Meetings, &cfg.Notify.Meetings}, {n.Blocks, &cfg.Notify.Blocks}, {n.Reviews, &cfg.Notify.Reviews}} {
		if b.from != nil {
			*b.to = *b.from
		}
	}
	if n.Alerts == nil {
		return nil
	}
	alerts := *n.Alerts
	if len(alerts) == 0 {
		return fmt.Errorf("notify.alerts: list at least one, like [5, 0] (turn a kind off instead)")
	}
	for i, a := range alerts {
		if a < 0 || a > 120 {
			return fmt.Errorf("notify.alerts[%d]: want whole minutes from 0 to 120, got %d", i, a)
		}
		if slices.Contains(alerts[:i], a) {
			return fmt.Errorf("notify.alerts: %d is listed twice", a)
		}
	}
	cfg.Notify.Alerts = alerts
	return nil
}

// parseClock parses "HH:MM" (00:00 to 23:59) into an offset from midnight.
func parseClock(s string) (time.Duration, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, fmt.Errorf("want HH:MM, got %q", s)
	}
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute, nil
}
