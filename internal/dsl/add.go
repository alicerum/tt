// Package dsl parses tt's one-line token syntax (D25).
package dsl

import (
	"errors"
	"fmt"
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
	Priority  int // 0 = not given
	Size      domain.Size
	Link      string
	Note      string
	HasNote   bool // the line contained a " -- " separator, even if the note is empty
}

const noteSep = " -- "

// ParseAdd parses a quick-add line. levels is the number of configured
// priority levels; !n outside 1..levels is an error.
//
//	@name   requester (last one wins)
//	#tag    tag, repeatable (#123 stays in the title: it's an issue ref)
//	!n      priority
//	~size   s, m, l, xl or x (~anything-else stays in the title)
//	URL     first http(s) URL becomes the link; later URLs stay in the title
//	 -- …   everything after the first " -- " is the note, verbatim
func ParseAdd(line string, levels int) (Draft, error) {
	var d Draft
	// Padding lets " -- " also match a leading "-- " and a trailing " --".
	head := line
	if padded := " " + line + " "; strings.Contains(padded, noteSep) {
		i := strings.Index(padded, noteSep)
		head, d.Note, d.HasNote = padded[:i], strings.TrimSpace(padded[i+len(noteSep):]), true
	}

	var title, tags []string
	for _, tok := range strings.Fields(head) {
		switch sigil, rest := tok[0], tok[1:]; {
		case sigil == '@' && rest != "":
			d.Requester = strings.ToLower(rest)
		case sigil == '#' && rest != "" && !isDigits(rest):
			tags = append(tags, rest)
		case sigil == '!' && isDigits(rest):
			n, err := strconv.Atoi(rest)
			if err != nil || n < 1 || n > levels {
				return Draft{}, fmt.Errorf("priority %s out of range: use !1 to !%d", tok, levels)
			}
			d.Priority = n
		case sigil == '~' && isSize(rest):
			d.Size, _ = domain.ParseSize(rest)
		case d.Link == "" && isURL(tok):
			d.Link = tok
		default:
			title = append(title, tok)
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
	if d.Requester != "" {
		parts = append(parts, "@"+d.Requester)
	}
	for _, t := range d.Tags {
		parts = append(parts, "#"+t)
	}
	if d.Priority > 0 {
		parts = append(parts, "!"+strconv.Itoa(d.Priority))
	}
	if d.Size != domain.SizeNone {
		parts = append(parts, "~"+string(d.Size))
	}
	if d.Link != "" {
		parts = append(parts, d.Link)
	}
	s := strings.Join(parts, " ")
	if d.HasNote {
		s += noteSep + d.Note
	}
	return s
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
