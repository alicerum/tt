package search

import (
	"slices"
	"testing"
	"time"

	"tt/internal/domain"
	"tt/internal/dsl"
)

var now = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

func filter(t *testing.T, line string) dsl.Filter {
	t.Helper()
	f, err := dsl.ParseFilter(line)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestMatch(t *testing.T) {
	task := domain.Task{
		ID: 1, Title: "Review schema-migration for Photos", Requester: "ana", Tags: []string{"impl"},
		Impact: 2, Size: domain.SizeM, Status: domain.StatusOpen, CreatedAt: now.Add(-3 * 24 * time.Hour),
	}
	cases := map[string]bool{
		"":                  true,
		"@ana":              true,
		"@bob":              false,
		"@bob @ana":         true, // requesters: any of
		"#impl":             true,
		"#impl #infra":      false, // tags: all of
		"!2":                true,
		"!1 !2":             true,
		"!1":                false,
		"~m":                true,
		"~s ~l":             false,
		"<7d":               true,
		"<3d":               false, // exactly 3 days old is not "within 3 days"
		">2d":               true,
		">3d":               false,
		"<1w >1d":           true,
		"schema":            true,
		"SCHM":              true, // anchored subsequence of "schema"
		"migr pho":          true,
		"chema":             true,  // substring
		"cma":               false, // subsequence not anchored at a word start
		"rvw":               true,  // anchored in "review"
		"ana":               true,  // requester is searched
		"imp":               true,  // tags are searched
		"schema nope":       false, // every word must match
		"is:done":           false,
		"is:all":            true,
		"is:done is:open":   true,
		"photos @ana !2 ~m": true,
	}
	for line, want := range cases {
		if got := Match(filter(t, line), task, nil, now); got != want {
			t.Errorf("Match(%q) = %v; want %v", line, got, want)
		}
	}
}

func TestMatchChildUsesEffectiveFields(t *testing.T) {
	parent := domain.Task{ID: 1, Title: "Auth", Tags: []string{"auth"}, Impact: 1, Status: domain.StatusOpen, CreatedAt: now}
	child := domain.Task{ID: 2, Title: "Rollback plan", Tags: []string{"ops"}, Impact: 3, ParentID: 1, Status: domain.StatusOpen, CreatedAt: now}
	for line, want := range map[string]bool{"!1": true, "!3": false, "#auth #ops": true, "auth": true} {
		if got := Match(filter(t, line), child, &parent, now); got != want {
			t.Errorf("child Match(%q) = %v; want %v", line, got, want)
		}
	}
}

func TestMatchClosedTasks(t *testing.T) {
	done := domain.Task{Title: "Shipped", Status: domain.StatusDone, CreatedAt: now}
	dropped := domain.Task{Title: "Abandoned", Status: domain.StatusDropped, CreatedAt: now}
	if Match(filter(t, "shipped"), done, nil, now) {
		t.Error("done tasks are hidden by default (D26b)")
	}
	if !Match(filter(t, "is:done shipped"), done, nil, now) || Match(filter(t, "is:done"), dropped, nil, now) {
		t.Error("is:done shows only done tasks")
	}
	if !Match(filter(t, "is:dropped"), dropped, nil, now) {
		t.Error("is:dropped shows dropped tasks")
	}
}

func TestGroupOf(t *testing.T) {
	days := func(n int) time.Time { return now.Add(-time.Duration(n) * 24 * time.Hour) }
	cases := []struct {
		by   GroupBy
		task domain.Task
		want Group
	}{
		{ByUrgency, domain.Task{Impact: 3}, Group{}},
		{ByImpact, domain.Task{Impact: 1}, Group{}},
		{BySize, domain.Task{Size: domain.SizeS}, Group{0, "S"}},
		{BySize, domain.Task{Size: domain.SizeXL}, Group{3, "XL"}},
		{BySize, domain.Task{}, Group{4, "unsized"}},
		{ByCreated, domain.Task{CreatedAt: now}, Group{0, "this week"}},
		{ByCreated, domain.Task{CreatedAt: days(7)}, Group{1, "this month"}},
		{ByCreated, domain.Task{CreatedAt: days(29)}, Group{1, "this month"}},
		{ByCreated, domain.Task{CreatedAt: days(30)}, Group{2, "older"}},
		{ByCreated, domain.Task{CreatedAt: days(90)}, Group{3, "ancient"}},
		{ByRequester, domain.Task{Requester: "ana"}, Group{0, "@ana"}},
		{ByRequester, domain.Task{}, Group{1, "no requester"}},
	}
	for _, c := range cases {
		if got := GroupOf(c.by, c.task, now); got != c.want {
			t.Errorf("GroupOf(%s, %+v) = %+v; want %+v", c.by, c.task, got, c.want)
		}
	}
	if Compare(Group{0, "@bob"}, Group{0, "@ana"}) <= 0 || Compare(Group{0, "@zed"}, Group{1, "no requester"}) >= 0 {
		t.Error("groups sort by rank, then label")
	}
}

func TestGroupByCycle(t *testing.T) {
	g := ByUrgency
	var seen []GroupBy
	for range GroupBys {
		g = g.Next()
		seen = append(seen, g)
	}
	want := []GroupBy{ByImpact, BySize, ByCreated, ByRequester, ByUrgency}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("cycle = %v; want %v", seen, want)
		}
	}
	for s, want := range map[string]GroupBy{"size": BySize, "impact": ByImpact, "": ByUrgency, "priority": ByUrgency, "bogus": ByUrgency} {
		if got := ParseGroupBy(s); got != want {
			t.Errorf("ParseGroupBy(%q) = %s; want %s", s, got, want)
		}
	}
}

func TestMatchName(t *testing.T) {
	cases := []struct {
		query, name string
		want        bool
	}{
		{"", "ana", true},
		{"an", "ana", true},
		{"AN", "ana", true},
		{"@an", "ana", true},
		{"@", "ana", true}, // a lone @ is a name still being typed
		{"rf", "raff", true},
		{"mar", "ana-maria", true},
		{"aa", "ana-maria", true},
		{"nm", "ana-maria", false},
		{"ana x", "ana", false},
		{"bo", "ana", false},
	}
	for _, c := range cases {
		if got := MatchName(c.query, c.name); got != c.want {
			t.Errorf("MatchName(%q, %q) = %v; want %v", c.query, c.name, got, c.want)
		}
	}
}

func TestSorter(t *testing.T) {
	const today = domain.Day("2026-10-14")
	day := func(n int) time.Time { return now.Add(time.Duration(n) * time.Hour) }
	tasks := []domain.Task{
		{ID: 1, Title: "undated stream task", Impact: 3, CreatedAt: day(1)},                         // 0.50
		{ID: 2, Title: "ask due today", Impact: 1, Due: today, CreatedAt: day(2)},                   // 0.53
		{ID: 3, Title: "team task due in 10 days", Impact: 2, Due: "2026-10-24", CreatedAt: day(3)}, // 0.51
		{ID: 4, Title: "untriaged", CreatedAt: day(0)},
		{ID: 5, Title: "older undated stream task", Impact: 3, CreatedAt: day(0)}, // 0.50, older
		{ID: 6, Title: "child of #3", ParentID: 3, Due: today, CreatedAt: day(4)}, // impact 2, due today: 0.70
	}
	byID := map[int64]domain.Task{}
	for _, tk := range tasks {
		byID[tk.ID] = tk
	}
	parentOf := func(tk domain.Task) *domain.Task {
		if p, ok := byID[tk.ParentID]; ok {
			return &p
		}
		return nil
	}
	order := func(g GroupBy, w domain.Weights) []int64 {
		got := slices.Clone(tasks)
		slices.SortFunc(got, Sorter(g, parentOf, today, w))
		var ids []int64
		for _, tk := range got {
			ids = append(ids, tk.ID)
		}
		return ids
	}
	cases := []struct {
		g    GroupBy
		w    domain.Weights
		want []int64
	}{
		{ByUrgency, domain.DefaultWeights, []int64{6, 2, 3, 5, 1, 4}},
		{ByImpact, domain.DefaultWeights, []int64{5, 1, 6, 3, 2, 4}},
		{ByUrgency, domain.Weights{Impact: 1, Due: 0}, []int64{5, 1, 3, 6, 2, 4}}, // impact only: ties oldest first
		{BySize, domain.DefaultWeights, []int64{4, 5, 1, 2, 3, 6}},                // a grouping: oldest first
	}
	for _, c := range cases {
		if got := order(c.g, c.w); !slices.Equal(got, c.want) {
			t.Errorf("%s with %+v: %v; want %v", c.g, c.w, got, c.want)
		}
	}
	if !ByUrgency.Flat() || !ByImpact.Flat() || BySize.Flat() || ByRequester.Flat() {
		t.Error("urgency and impact are flat sorts; the rest are groupings (D72)")
	}
}
