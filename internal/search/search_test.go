package search

import (
	"testing"
	"time"

	"tt/internal/domain"
	"tt/internal/dsl"
)

var now = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

func filter(t *testing.T, line string) dsl.Filter {
	t.Helper()
	f, err := dsl.ParseFilter(line, 3)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestMatch(t *testing.T) {
	task := domain.Task{
		ID: 1, Title: "Review schema-migration for Photos", Requester: "ana", Tags: []string{"impl"},
		Priority: 2, Size: domain.SizeM, Status: domain.StatusOpen, CreatedAt: now.Add(-3 * 24 * time.Hour),
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
	parent := domain.Task{ID: 1, Title: "Auth", Tags: []string{"auth"}, Priority: 1, Status: domain.StatusOpen, CreatedAt: now}
	child := domain.Task{ID: 2, Title: "Rollback plan", Tags: []string{"ops"}, Priority: 3, ParentID: 1, Status: domain.StatusOpen, CreatedAt: now}
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
	levels := []string{"P1 · blocked", "P2 · soon", "P3 · later"}
	days := func(n int) time.Time { return now.Add(-time.Duration(n) * 24 * time.Hour) }
	parent := domain.Task{ID: 1, Priority: 2}
	cases := []struct {
		by     GroupBy
		task   domain.Task
		parent *domain.Task
		want   Group
	}{
		{ByPriority, domain.Task{Priority: 1}, nil, Group{1, "P1 · blocked"}},
		{ByPriority, domain.Task{Priority: 3, ParentID: 1}, &parent, Group{2, "P2 · soon"}},
		{ByPriority, domain.Task{Priority: 5}, nil, Group{4, "P? · above the configured levels"}},
		{ByPriority, domain.Task{}, nil, Group{5, "unset"}},
		{BySize, domain.Task{Size: domain.SizeS}, nil, Group{0, "S"}},
		{BySize, domain.Task{Size: domain.SizeXL}, nil, Group{3, "XL"}},
		{BySize, domain.Task{}, nil, Group{4, "unsized"}},
		{ByCreated, domain.Task{CreatedAt: now}, nil, Group{0, "this week"}},
		{ByCreated, domain.Task{CreatedAt: days(7)}, nil, Group{1, "this month"}},
		{ByCreated, domain.Task{CreatedAt: days(29)}, nil, Group{1, "this month"}},
		{ByCreated, domain.Task{CreatedAt: days(30)}, nil, Group{2, "older"}},
		{ByCreated, domain.Task{CreatedAt: days(90)}, nil, Group{3, "ancient"}},
		{ByRequester, domain.Task{Requester: "ana"}, nil, Group{0, "@ana"}},
		{ByRequester, domain.Task{}, nil, Group{1, "no requester"}},
	}
	for _, c := range cases {
		if got := GroupOf(c.by, c.task, c.parent, now, levels); got != c.want {
			t.Errorf("GroupOf(%s, %+v) = %+v; want %+v", c.by, c.task, got, c.want)
		}
	}
	if Compare(Group{0, "@bob"}, Group{0, "@ana"}) <= 0 || Compare(Group{0, "@zed"}, Group{1, "no requester"}) >= 0 {
		t.Error("groups sort by rank, then label")
	}
}

func TestGroupByCycle(t *testing.T) {
	g := ByPriority
	var seen []GroupBy
	for range GroupBys {
		g = g.Next()
		seen = append(seen, g)
	}
	want := []GroupBy{BySize, ByCreated, ByRequester, ByPriority}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("cycle = %v; want %v", seen, want)
		}
	}
	for s, want := range map[string]GroupBy{"size": BySize, "requester": ByRequester, "": ByPriority, "bogus": ByPriority} {
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
