// Package search evaluates filters (D26a–D26d) and sorts or groups tasks
// for the backlog (D26, D72) over an in-memory snapshot. It does no I/O.
package search

import (
	"cmp"
	"slices"
	"strings"
	"time"
	"unicode"

	"tt/internal/domain"
	"tt/internal/dsl"
)

// Match reports whether t passes f at now. parent is t's parent (nil if none):
// children are matched on their effective impact and tags.
func Match(f dsl.Filter, t domain.Task, parent *domain.Task, now time.Time) bool {
	statuses := f.Statuses
	if statuses == nil {
		statuses = []domain.Status{domain.StatusOpen}
	}
	if !slices.Contains(statuses, t.Status) {
		return false
	}
	if len(f.Requesters) > 0 && !slices.Contains(f.Requesters, t.Requester) {
		return false
	}
	tags := domain.EffectiveTags(t, parent)
	for _, want := range f.Tags {
		if !slices.Contains(tags, want) {
			return false
		}
	}
	if len(f.Impacts) > 0 && !slices.Contains(f.Impacts, domain.EffectiveImpact(t, parent)) {
		return false
	}
	if len(f.Sizes) > 0 && !slices.Contains(f.Sizes, t.Size) {
		return false
	}
	age := now.Sub(t.CreatedAt)
	if f.NewerThan > 0 && age >= f.NewerThan {
		return false
	}
	if f.OlderThan > 0 && age <= f.OlderThan {
		return false
	}
	if len(f.Words) == 0 {
		return true
	}
	hay := []string{strings.ToLower(t.Title), t.Requester}
	hay = append(hay, tags...)
	for _, w := range f.Words {
		if !slices.ContainsFunc(hay, func(h string) bool { return wordMatches(w, h) }) {
			return false
		}
	}
	return true
}

// MatchName reports whether a person's name passes query, the People
// filter (D52): every word of query, lowercased and without a leading @,
// matches name like a fuzzy filter word (D26d).
func MatchName(query, name string) bool {
	for _, w := range strings.Fields(strings.ToLower(query)) {
		if w = strings.TrimPrefix(w, "@"); w != "" && !wordMatches(w, name) {
			return false
		}
	}
	return true
}

// wordMatches: w (lowercase) is a substring of hay, or a subsequence of one of
// hay's words that starts at that word's first letter ("schm" → "schema").
func wordMatches(w, hay string) bool {
	if strings.Contains(hay, w) {
		return true
	}
	for _, word := range strings.FieldsFunc(hay, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if anchoredSubsequence([]rune(w), []rune(word)) {
			return true
		}
	}
	return false
}

func anchoredSubsequence(w, word []rune) bool {
	if len(word) == 0 || w[0] != word[0] {
		return false
	}
	i := 0
	for _, r := range word {
		if i < len(w) && r == w[i] {
			i++
		}
	}
	return i == len(w)
}

// GroupBy is a backlog grouping (D26) or, for urgency and impact, a flat
// sort (D72).
type GroupBy string

const (
	ByUrgency   GroupBy = "urgency"
	ByImpact    GroupBy = "impact"
	BySize      GroupBy = "size"
	ByCreated   GroupBy = "created"
	ByRequester GroupBy = "requester"
)

// GroupBys lists the groupings in the order `g` cycles through them.
var GroupBys = []GroupBy{ByUrgency, ByImpact, BySize, ByCreated, ByRequester}

// ParseGroupBy returns the grouping named s; anything unknown (a remembered
// "priority" among them) is ByUrgency.
func ParseGroupBy(s string) GroupBy {
	if g := GroupBy(s); slices.Contains(GroupBys, g) {
		return g
	}
	return ByUrgency
}

// Flat reports whether g is a sort with no headings (D72).
func (g GroupBy) Flat() bool { return g == ByUrgency || g == ByImpact }

// Sorter is the order of tasks within g's list (flat) or within one of its
// groups, on today with weights w (D72): ByUrgency most urgent first,
// ByImpact biggest effective impact first, then most urgent; an untriaged
// task after the triaged ones; then oldest first. The groupings sort oldest
// first (D26). parentOf finds a task's parent (nil for none).
func Sorter(g GroupBy, parentOf func(domain.Task) *domain.Task, today domain.Day, w domain.Weights) func(a, b domain.Task) int {
	if !g.Flat() {
		return Oldest
	}
	return func(a, b domain.Task) int {
		pa, pb := parentOf(a), parentOf(b)
		ua, oka := domain.Urgency(a, pa, today, w)
		ub, okb := domain.Urgency(b, pb, today, w)
		byImpact := 0
		if g == ByImpact {
			byImpact = cmp.Compare(domain.EffectiveImpact(b, pb), domain.EffectiveImpact(a, pa))
		}
		return cmp.Or(byImpact, domain.CompareUrgency(ua, oka, ub, okb), Oldest(a, b))
	}
}

// Next is the grouping after g in the `g` cycle.
func (g GroupBy) Next() GroupBy {
	i := slices.Index(GroupBys, g)
	return GroupBys[(i+1)%len(GroupBys)]
}

// Group names one backlog heading. Groups sort by Rank, then Label.
type Group struct {
	Rank  int
	Label string
}

// Compare orders groups for display.
func Compare(a, b Group) int {
	return cmp.Or(cmp.Compare(a.Rank, b.Rank), cmp.Compare(a.Label, b.Label))
}

// Oldest orders tasks oldest first (within a group, D26).
func Oldest(a, b domain.Task) int {
	return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID))
}

// Age buckets for ByCreated.
var ageBuckets = []struct {
	under time.Duration
	label string
}{
	{7 * 24 * time.Hour, "this week"},
	{30 * 24 * time.Hour, "this month"},
	{90 * 24 * time.Hour, "older"},
}

// GroupOf returns the group t lands in at now. A flat sort (D72) has one
// group, Group{}, shown without a heading.
func GroupOf(by GroupBy, t domain.Task, now time.Time) Group {
	switch by {
	case BySize:
		if i := slices.Index(domain.Sizes, t.Size); i >= 0 {
			return Group{Rank: i, Label: strings.ToUpper(string(t.Size))}
		}
		return Group{Rank: len(domain.Sizes), Label: "unsized"}
	case ByCreated:
		age := now.Sub(t.CreatedAt)
		for i, b := range ageBuckets {
			if age < b.under {
				return Group{Rank: i, Label: b.label}
			}
		}
		return Group{Rank: len(ageBuckets), Label: "ancient"}
	case ByRequester:
		if t.Requester == "" {
			return Group{Rank: 1, Label: "no requester"}
		}
		return Group{Label: "@" + t.Requester}
	}
	return Group{}
}
