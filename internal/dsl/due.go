package dsl

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"tt/internal/domain"
)

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

var months = map[string]time.Month{
	"jan": time.January, "feb": time.February, "mar": time.March, "apr": time.April,
	"may": time.May, "jun": time.June, "jul": time.July, "aug": time.August,
	"sep": time.September, "oct": time.October, "nov": time.November, "dec": time.December,
}

// dueForms lists the forms for error messages.
const dueForms = "today, tom, a weekday like fri, 3d, 2w, 30oct or 2026-10-30"

// ParseDue reads a due date typed on its own, with or without its ^ (the
// Inbox's due input, D73): "" is no due.
func ParseDue(text string, today domain.Day) (domain.Day, error) {
	s := strings.TrimPrefix(strings.TrimSpace(text), "^")
	if s == "" {
		return "", nil
	}
	due, ok, err := dueWord(s, today)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("due %q: want %s", s, dueForms)
	}
	return due, nil
}

// dueWord reads w, a ^ token without its ^ (D73), against today (a
// planning day): today, tom or tomorrow, mon…sun (the next one; today
// counts), Nd or Nw from today, 30oct (the next 30 Oct; today counts) and
// 2026-10-30, case-insensitive. ok is false when w has none of these shapes
// (it stays in the title); err is set when it has a date's shape but names
// no real date (31feb, 2026-13-01).
func dueWord(w string, today domain.Day) (due domain.Day, ok bool, err error) {
	w = strings.ToLower(w)
	t, perr := time.Parse(time.DateOnly, string(today))
	if perr != nil {
		return "", false, fmt.Errorf("today %q: %w", today, perr)
	}
	switch {
	case w == "today":
		return today, true, nil
	case w == "tom" || w == "tomorrow":
		return today.AddDays(1), true, nil
	}
	if wd, isDay := weekdays[w]; isDay {
		return today.AddDays((int(wd) - int(t.Weekday()) + 7) % 7), true, nil
	}
	if n := len(w); n >= 2 && (w[n-1] == 'd' || w[n-1] == 'w') && isDigits(w[:n-1]) {
		k, err := strconv.Atoi(w[:n-1])
		if err != nil || k > 9999 {
			return "", true, fmt.Errorf("due ^%s: want at most 9999 days or weeks", w)
		}
		if w[n-1] == 'w' {
			k *= 7
		}
		return today.AddDays(k), true, nil
	}
	if len(w) == 10 && w[4] == '-' && w[7] == '-' && isDigits(w[:4]+w[5:7]+w[8:]) {
		if _, err := time.Parse(time.DateOnly, w); err != nil {
			return "", true, fmt.Errorf("due ^%s: no such date", w)
		}
		return domain.Day(w), true, nil
	}
	if n := len(w); n >= 4 && n <= 5 && isDigits(w[:n-3]) {
		if month, isMonth := months[w[n-3:]]; isMonth {
			day, _ := strconv.Atoi(w[:n-3])
			for year := t.Year(); year <= t.Year()+1; year++ {
				d := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
				if d.Month() == month && d.Day() == day && !d.Before(t) {
					return domain.Day(d.Format(time.DateOnly)), true, nil
				}
			}
			return "", true, fmt.Errorf("due ^%s: no such date", w)
		}
	}
	return "", false, nil
}
