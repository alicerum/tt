// Package dsl parses tt's one-line token syntax (D25).
package dsl

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"tt/internal/domain"
)

var ErrEmptyTitle = errors.New("title is required")

// Draft is the result of parsing a quick-add line.
type Draft struct {
	Title     string
	Requester string // lowercase
	Tags      []string
	Impact    int        // 0 = not given; 1–3 (D68)
	Due       domain.Day // "" = not given (D69)
	Size      domain.Size
	Link      string
	Note      string
	HasNote   bool // the line contained a " -- " separator, even if the note is empty
}

const noteSep = " -- "

// trailingPunct is stripped from the end of sigil tokens ("@ana," is @ana).
const trailingPunct = ".,;:!?)]}'\""

// ParseAdd parses a quick-add line; today (a planning day) resolves ^dates.
//
//	@name   requester (last one wins)
//	#tag    tag, repeatable (#123 stays in the title: it's an issue ref)
//	!n      impact, 1 to 3 (D68); out of range is an error
//	^date   due (D73): today, tom, fri, 3d, 2w, 30oct, 2026-10-30 (last one
//	        wins; a date that doesn't exist is an error; ^anything-else
//	        stays in the title)
//	~size   s, m, l, xl or x (~anything-else stays in the title)
//	URL     first http(s) URL becomes the link; later URLs stay in the title
//	 -- …   everything after the first " -- " is the note, verbatim
func ParseAdd(line string, today domain.Day) (Draft, error) {
	var d Draft
	// Padding lets " -- " also match a leading "-- " and a trailing " --".
	head := line
	if padded := " " + line + " "; strings.Contains(padded, noteSep) {
		i := strings.Index(padded, noteSep)
		head, d.Note, d.HasNote = padded[:i], strings.TrimSpace(padded[i+len(noteSep):]), true
	}

	var title, tags []string
	for _, tok := range strings.Fields(head) {
		orig := tok
		if strings.ContainsRune("@#!~^", rune(tok[0])) {
			stripped := strings.TrimRight(tok, trailingPunct)
			// "#482." stays whole (issue ref); "@." has nothing left to classify.
			if len(stripped) <= 1 || (stripped[0] == '#' && isDigits(stripped[1:])) {
				title = append(title, orig)
				continue
			}
			tok = stripped
		}
		switch sigil, rest := tok[0], tok[1:]; {
		case sigil == '@' && rest != "":
			d.Requester = strings.ToLower(rest)
		case sigil == '#' && rest != "" && !isDigits(rest):
			tags = append(tags, rest)
		case sigil == '!' && isDigits(rest):
			n, err := parseImpact(tok)
			if err != nil {
				return Draft{}, err
			}
			d.Impact = n
		case sigil == '^':
			due, ok, err := dueWord(rest, today)
			if err != nil {
				return Draft{}, err
			}
			if !ok {
				title = append(title, orig)
				continue
			}
			d.Due = due
		case sigil == '~' && isSize(rest):
			d.Size, _ = domain.ParseSize(rest)
		case d.Link == "" && isURL(tok):
			d.Link = tok
		default:
			title = append(title, orig)
		}
	}
	d.Title = strings.Join(title, " ")
	if d.Title == "" {
		return Draft{}, ErrEmptyTitle
	}
	d.Tags = domain.NormalizeTags(tags)
	return d, nil
}

// FormatAdd renders d back into token syntax (used to prefill the edit line).
// The note is included only when d.HasNote is set.
func FormatAdd(d Draft) string {
	parts := []string{d.Title}
	// A URL-looking word in the title would be re-parsed as the link, so put
	// the real link first.
	linkFirst := d.Link != "" && slices.ContainsFunc(strings.Fields(d.Title), isURL)
	if linkFirst {
		parts = []string{d.Link, d.Title}
	}
	if d.Requester != "" {
		parts = append(parts, "@"+d.Requester)
	}
	for _, t := range d.Tags {
		parts = append(parts, "#"+t)
	}
	if d.Impact > 0 {
		parts = append(parts, "!"+strconv.Itoa(d.Impact))
	}
	if d.Due != "" {
		parts = append(parts, "^"+string(d.Due)) // absolute: an edit doesn't drift (D73)
	}
	if d.Size != domain.SizeNone {
		parts = append(parts, "~"+string(d.Size))
	}
	if d.Link != "" && !linkFirst {
		parts = append(parts, d.Link)
	}
	s := strings.Join(parts, " ")
	if d.HasNote {
		s += noteSep + d.Note
	}
	return s
}

// parseImpact reads !n (tok, all digits after the !) as impact 1–3 (D68).
func parseImpact(tok string) (int, error) {
	n, err := strconv.Atoi(tok[1:])
	if err != nil || n < 1 || n > domain.MaxImpact {
		return 0, fmt.Errorf("impact %s out of range: use !1 to !%d", tok, domain.MaxImpact)
	}
	return n, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isSize(s string) bool {
	_, ok := domain.ParseSize(s)
	return ok
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

// WithNoteLines appends rest (extra lines from a multi-line paste or piped
// stdin, D33) to line as its note: after the line's own " -- " note on a new
// line, or as a new " -- " note. Trailing blank lines are dropped.
func WithNoteLines(line string, rest []string) string {
	for len(rest) > 0 && strings.TrimSpace(rest[len(rest)-1]) == "" {
		rest = rest[:len(rest)-1]
	}
	if len(rest) == 0 {
		return line
	}
	note := strings.Join(rest, "\n")
	switch head := strings.TrimRight(line, " "); {
	case strings.HasSuffix(" "+head, " --"): // an empty " --" note: fill it
		return head + " " + note
	case strings.Contains(" "+line+" ", noteSep):
		return line + "\n" + note
	default:
		return line + noteSep + note
	}
}
