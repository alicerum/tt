package dsl

import (
	"strings"
	"testing"

	"tt/internal/domain"
)

// 2026-10-14 is a Wednesday.
const wednesday = domain.Day("2026-10-14")

func TestDueWord(t *testing.T) {
	cases := map[string]domain.Day{
		"today":      "2026-10-14",
		"TOM":        "2026-10-15",
		"tomorrow":   "2026-10-15",
		"wed":        "2026-10-14", // today counts
		"thu":        "2026-10-15",
		"Fri":        "2026-10-16",
		"tue":        "2026-10-20",
		"0d":         "2026-10-14",
		"3d":         "2026-10-17",
		"2w":         "2026-10-28",
		"14oct":      "2026-10-14", // today counts
		"30oct":      "2026-10-30",
		"1OCT":       "2027-10-01", // passed this year: next year's
		"3jan":       "2027-01-03",
		"2026-10-30": "2026-10-30",
		"2026-01-02": "2026-01-02", // a past date is allowed: overdue (nudge, never block)
	}
	for in, want := range cases {
		got, ok, err := dueWord(in, wednesday)
		if !ok || err != nil || got != want {
			t.Errorf("dueWord(%q) = %q, %v, %v; want %q", in, got, ok, err, want)
		}
	}
}

func TestDueWordRolloverAndErrors(t *testing.T) {
	if got, _, err := dueWord("2jan", "2026-12-30"); err != nil || got != "2027-01-02" {
		t.Errorf("2jan at the year end = %q, %v; want next year's", got, err)
	}
	if got, _, err := dueWord("sat", "2026-12-30"); err != nil || got != "2027-01-02" {
		t.Errorf("sat at the year end = %q, %v", got, err)
	}
	if got, _, err := dueWord("29feb", "2027-03-01"); err != nil || got != "2028-02-29" {
		t.Errorf("29feb before a leap year = %q, %v; want 2028-02-29", got, err)
	}
	for _, in := range []string{"31feb", "0oct", "32oct", "2026-13-01", "2026-02-30", "29feb", "10000d"} {
		if _, ok, err := dueWord(in, wednesday); !ok || err == nil {
			t.Errorf("dueWord(%q) = ok %v, err %v; want a date-shaped error", in, ok, err)
		}
	}
	for _, in := range []string{"", "_^", "foo", "friday", "fri.", "3", "d", "3x", "oct", "123oct", "2026-10", "10/30"} {
		if _, ok, err := dueWord(in, wednesday); ok || err != nil {
			t.Errorf("dueWord(%q) = ok %v, err %v; want not a date (stays in the title)", in, ok, err)
		}
	}
}

func TestParseDue(t *testing.T) {
	for in, want := range map[string]domain.Day{"": "", "  ": "", "fri": "2026-10-16", "^fri": "2026-10-16", " ^3d ": "2026-10-17"} {
		if got, err := ParseDue(in, wednesday); err != nil || got != want {
			t.Errorf("ParseDue(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ParseDue("someday", wednesday); err == nil || !strings.Contains(err.Error(), "want today, tom, a weekday like fri, 3d, 2w, 30oct or 2026-10-30") {
		t.Errorf("ParseDue(someday) error = %v", err)
	}
	if _, err := ParseDue("31feb", wednesday); err == nil || !strings.Contains(err.Error(), "no such date") {
		t.Errorf("ParseDue(31feb) error = %v", err)
	}
}
