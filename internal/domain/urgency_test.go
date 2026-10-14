package domain

import (
	"math"
	"testing"
)

const today = Day("2026-10-14")

func near(a, b float64) bool { return math.Abs(a-b) < 0.005 }

func TestDueFactor(t *testing.T) {
	cases := []struct {
		name string
		due  Day
		want float64
	}{
		{"no due", "", 0},
		{"15 days ahead", today.AddDays(15), 0.2},
		{"14 days ahead", today.AddDays(14), 0.2},
		{"7 days ahead", today.AddDays(7), 0.2 + 7*0.8/21},
		{"tomorrow", today.AddDays(1), 0.2 + 13*0.8/21},
		{"today", today, 0.2 + 14*0.8/21},
		{"yesterday", today.AddDays(-1), 0.2 + 15*0.8/21},
		{"7 days overdue", today.AddDays(-7), 1},
		{"8 days overdue", today.AddDays(-8), 1},
		{"a year overdue", today.AddDays(-365), 1},
	}
	for _, c := range cases {
		if got := DueFactor(today, c.due); !near(got, c.want) {
			t.Errorf("%s: DueFactor = %.3f; want %.3f", c.name, got, c.want)
		}
	}
	if got := DueFactor(today, today); !near(got, 0.733) {
		t.Errorf("due today = %.3f; want about 0.73 (D70)", got)
	}
}

// The worked examples of D70, with the default weights.
func TestUrgencyExamples(t *testing.T) {
	cases := []struct {
		name string
		task Task
		want float64
	}{
		{"step 3, no due", Task{Impact: 3}, 0.50},
		{"step 1, due in a month", Task{Impact: 1, Due: today.AddDays(30)}, 0.27},
		{"step 1, due today", Task{Impact: 1, Due: today}, 0.53},
		{"step 1, 7 days overdue", Task{Impact: 1, Due: today.AddDays(-7)}, 0.67},
		{"step 3, due in 3 weeks", Task{Impact: 3, Due: today.AddDays(21)}, 0.60},
		{"step 2, due in 10 days", Task{Impact: 2, Due: today.AddDays(10)}, 0.51},
		{"step 1, no due", Task{Impact: 1}, 0.17},
		{"step 3, a week overdue", Task{Impact: 3, Due: today.AddDays(-7)}, 1},
	}
	for _, c := range cases {
		u, ok := Urgency(c.task, nil, today, DefaultWeights)
		if !ok || !near(u, c.want) {
			t.Errorf("%s: Urgency = %.3f, %v; want %.2f, true", c.name, u, ok, c.want)
		}
	}
	if u, ok := Urgency(Task{Due: today}, nil, today, DefaultWeights); ok || u != 0 {
		t.Errorf("untriaged: %.2f, %v; want no urgency", u, ok)
	}
}

func TestUrgencyWeights(t *testing.T) {
	task := Task{Impact: 1, Due: today}
	if u, _ := Urgency(task, nil, today, Weights{Impact: 1, Due: 0}); !near(u, 1.0/3) {
		t.Errorf("impact only: %.3f; want 0.333", u)
	}
	if u, _ := Urgency(task, nil, today, Weights{Impact: 0, Due: 2}); !near(u, 0.733) {
		t.Errorf("due only: %.3f; want 0.733", u)
	}
	if u, _ := Urgency(task, nil, today, Weights{Impact: 1, Due: 3}); !near(u, (1.0/3+3*0.7333)/4) {
		t.Errorf("1:3: %.3f", u)
	}
}

func TestEffectiveImpactAndDue(t *testing.T) {
	parent := Task{ID: 1, Impact: 3, Due: "2026-10-20"}
	child := Task{ID: 2, ParentID: 1, Impact: 1}
	if got := EffectiveImpact(child, &parent); got != 3 {
		t.Errorf("child impact = %d; want the parent's 3 (D68)", got)
	}
	if got := EffectiveImpact(child, nil); got != 1 {
		t.Errorf("top-level impact = %d; want its own 1", got)
	}
	if got := EffectiveDue(child, &parent); got != "2026-10-20" {
		t.Errorf("child without a due = %q; want the parent's (D69)", got)
	}
	child.Due = "2026-10-16"
	if got := EffectiveDue(child, &parent); got != "2026-10-16" {
		t.Errorf("child with its own due = %q; want its own", got)
	}
	if u, ok := Urgency(Task{ParentID: 1}, &Task{ID: 1}, today, DefaultWeights); ok {
		t.Errorf("child of an untriaged parent: %.2f, true; want no urgency", u)
	}
}

func TestUrgencyScoreAndOrder(t *testing.T) {
	for u, want := range map[float64]int{0: 0, 1.0 / 6: 17, 0.5: 50, 0.7333: 73, 1: 100} {
		if got := UrgencyScore(u); got != want {
			t.Errorf("UrgencyScore(%.4f) = %d; want %d", u, got, want)
		}
	}
	if CompareUrgency(0.6, true, 0.5, true) >= 0 || CompareUrgency(0.1, true, 0.9, false) >= 0 ||
		CompareUrgency(0.9, false, 0.1, true) <= 0 || CompareUrgency(0.5, true, 0.5, true) != 0 {
		t.Error("CompareUrgency: want the more urgent first, untriaged last, 0 on a tie")
	}
}

func TestDayArithmetic(t *testing.T) {
	if got := Day("2026-12-30").AddDays(3); got != "2027-01-02" {
		t.Errorf("AddDays over the year end = %q", got)
	}
	if got := Day("2026-03-01").AddDays(-1); got != "2026-02-28" {
		t.Errorf("AddDays(-1) = %q", got)
	}
	if got := DaysBetween("2026-10-14", "2026-10-21"); got != 7 {
		t.Errorf("DaysBetween forward = %d; want 7", got)
	}
	if got := DaysBetween("2026-10-21", "2026-10-14"); got != -7 {
		t.Errorf("DaysBetween backward = %d; want -7", got)
	}
	if got := DaysBetween("2026-03-28", "2026-03-30"); got != 2 {
		t.Errorf("DaysBetween over a DST change = %d; want 2", got)
	}
}
