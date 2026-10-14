package dsl

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"tt/internal/domain"
)

// Filter is a parsed filter-bar line (D26a, D26b, D26d). Empty fields don't filter.
type Filter struct {
	Requesters []string        // any of; lowercase
	Tags       []string        // all of; normalized
	Impacts    []int           // any of (effective impact)
	Sizes      []domain.Size   // any of
	NewerThan  time.Duration   // created less than this long ago; 0 = no bound
	OlderThan  time.Duration   // created more than this long ago; 0 = no bound
	Statuses   []domain.Status // any of; nil = open only
	Words      []string        // fuzzy words, lowercase; all must match
}

// Empty reports whether the line had no tokens at all.
func (f Filter) Empty() bool {
	return len(f.Requesters) == 0 && len(f.Tags) == 0 && len(f.Impacts) == 0 && len(f.Sizes) == 0 &&
		f.NewerThan == 0 && f.OlderThan == 0 && len(f.Statuses) == 0 && len(f.Words) == 0
}

const day = 24 * time.Hour

// ParseFilter parses a filter-bar line. !n outside 1..3 is an error, like
// in ParseAdd.
//
//	@name        requester (repeats: any of)
//	#tag         tag, own or inherited (repeats: all of)
//	!n  ~size    effective impact / size (repeats: any of)
//	<7d  >30d    created within / more than N days (or w weeks) ago
//	is:open|done|dropped|all
//	a lone sigil (@ # ! ~ < >) is ignored: it's a token still being typed
//	anything else is a fuzzy word
func ParseFilter(line string) (Filter, error) {
	var f Filter
	var tags []string
	for _, tok := range strings.Fields(line) {
		orig := tok
		if len(tok) == 1 && strings.ContainsRune("@#!~<>", rune(tok[0])) {
			continue
		}
		if strings.ContainsRune("@#!~", rune(tok[0])) {
			stripped := strings.TrimRight(tok, trailingPunct)
			if len(stripped) <= 1 || (stripped[0] == '#' && isDigits(stripped[1:])) {
				f.Words = append(f.Words, strings.ToLower(orig))
				continue
			}
			tok = stripped
		}
		switch sigil, rest := tok[0], tok[1:]; {
		case sigil == '@':
			f.Requesters = appendNew(f.Requesters, strings.ToLower(rest))
		case sigil == '#' && !isDigits(rest):
			tags = append(tags, rest)
		case sigil == '!' && isDigits(rest):
			n, err := parseImpact(tok)
			if err != nil {
				return Filter{}, err
			}
			f.Impacts = appendNew(f.Impacts, n)
		case sigil == '~' && isSize(rest):
			s, _ := domain.ParseSize(rest)
			f.Sizes = appendNew(f.Sizes, s)
		case (sigil == '<' || sigil == '>') && isAge(rest):
			age, err := parseAge(rest)
			if err != nil {
				return Filter{}, fmt.Errorf("%s: %w", tok, err)
			}
			if sigil == '<' {
				f.NewerThan = age
			} else {
				f.OlderThan = age
			}
		case strings.HasPrefix(strings.ToLower(tok), "is:"):
			st, err := parseStatus(tok[3:])
			if err != nil {
				return Filter{}, err
			}
			for _, s := range st {
				f.Statuses = appendNew(f.Statuses, s)
			}
		default:
			f.Words = append(f.Words, strings.ToLower(orig))
		}
	}
	f.Tags = nilIfEmpty(domain.NormalizeTags(tags))
	return f, nil
}

// isAge: digits followed by d or w ("7d", "2w").
func isAge(s string) bool {
	if len(s) < 2 {
		return false
	}
	unit := s[len(s)-1]
	return (unit == 'd' || unit == 'w' || unit == 'D' || unit == 'W') && isDigits(s[:len(s)-1])
}

func parseAge(s string) (time.Duration, error) {
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n < 1 || n > 10000 {
		return 0, fmt.Errorf("age must be 1 to 10000 days or weeks")
	}
	if unit := s[len(s)-1]; unit == 'w' || unit == 'W' {
		return time.Duration(n) * 7 * day, nil
	}
	return time.Duration(n) * day, nil
}

func parseStatus(s string) ([]domain.Status, error) {
	switch strings.ToLower(s) {
	case "open":
		return []domain.Status{domain.StatusOpen}, nil
	case "done":
		return []domain.Status{domain.StatusDone}, nil
	case "dropped":
		return []domain.Status{domain.StatusDropped}, nil
	case "all":
		return []domain.Status{domain.StatusOpen, domain.StatusDone, domain.StatusDropped}, nil
	}
	return nil, fmt.Errorf("unknown status is:%s: use is:open, is:done, is:dropped or is:all", s)
}

func appendNew[T comparable](s []T, v T) []T {
	if slices.Contains(s, v) {
		return s
	}
	return append(s, v)
}

func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}
