package domain

import (
	"cmp"
	"fmt"
	"math"
	"time"
)

// MaxImpact is the biggest impact step (D68).
const MaxImpact = 3

// Impact steps (D68): who notices if a task slips, counted in teams.
// ImpactNames[i-1] names step i; ImpactExamples[i-1] goes with it on the
// Inbox card.
var (
	ImpactNames    = []string{"a person", "one team", "more than one team"}
	ImpactExamples = []string{"me, one reviewer, one asker", "my team, or one other team", "several teams, a stream"}
)

// EffectiveImpact is the impact that applies to t: a child always takes its
// parent's, with no override (D68).
func EffectiveImpact(t Task, parent *Task) int {
	if parent != nil {
		return parent.Impact
	}
	return t.Impact
}

// EffectiveDue is the due date that applies to t: its own, else its
// parent's (D69). "" means none.
func EffectiveDue(t Task, parent *Task) Day {
	if t.Due == "" && parent != nil {
		return parent.Due
	}
	return t.Due
}

// Weights weigh impact against the due date in urgency (D70).
type Weights struct{ Impact, Due float64 }

// DefaultWeights count both the same.
var DefaultWeights = Weights{Impact: 1, Due: 1}

// DueFactor is Taskwarrior's due ramp in whole planning days (D70): 0 with
// no due; 0.2 more than 14 days ahead; rising in a straight line to 1.0 at 7
// days overdue, and 1.0 after that. Due today is about 0.73.
func DueFactor(today, due Day) float64 {
	if due == "" {
		return 0
	}
	switch d := float64(DaysBetween(due, today)); { // days overdue
	case d >= 7:
		return 1
	case d >= -14:
		return 0.2 + (d+14)*0.8/21
	}
	return 0.2
}

// Urgency is t's urgency on today, 0 to 1 (D70): the weighted mean of its
// effective impact (scaled to 0–1) and its due factor. ok is false while t
// has no effective impact (untriaged): it has no urgency then.
func Urgency(t Task, parent *Task, today Day, w Weights) (u float64, ok bool) {
	impact := EffectiveImpact(t, parent)
	if impact == 0 {
		return 0, false
	}
	f := DueFactor(today, EffectiveDue(t, parent))
	return (w.Impact*float64(impact)/MaxImpact + w.Due*f) / (w.Impact + w.Due), true
}

// UrgencyScore is urgency as shown, 0 to 100 (D71).
func UrgencyScore(u float64) int { return int(math.Round(u * 100)) }

// CompareUrgency orders the more urgent first and an untriaged task (ok
// false) after every triaged one; 0 on a tie (D72).
func CompareUrgency(a float64, aok bool, b float64, bok bool) int {
	if aok != bok {
		if aok {
			return -1
		}
		return 1
	}
	return cmp.Compare(b, a)
}

// AddDays is the day n days after d (before it for n < 0).
func (d Day) AddDays(n int) Day {
	return Day(d.date().AddDate(0, 0, n).Format(time.DateOnly))
}

// DaysBetween is how many days from comes before to: positive when to is
// later.
func DaysBetween(from, to Day) int {
	return int(math.Round(to.date().Sub(from.date()).Hours() / 24))
}

func (d Day) date() time.Time {
	t, err := time.Parse(time.DateOnly, string(d))
	if err != nil {
		panic(fmt.Sprintf("invalid Day %q", d))
	}
	return t
}
