package domain

import (
	stdcmp "cmp"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestParseSize(t *testing.T) {
	cases := map[string]Size{"s": SizeS, "M": SizeM, "l": SizeL, "xl": SizeXL, "X": SizeXL}
	for in, want := range cases {
		got, ok := ParseSize(in)
		if !ok || got != want {
			t.Errorf("ParseSize(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "xs", "xxl", "/tmp"} {
		if _, ok := ParseSize(in); ok {
			t.Errorf("ParseSize(%q) ok = true; want false", in)
		}
	}
}

func TestNormalizeTags(t *testing.T) {
	got := NormalizeTags([]string{"Impl", " photos ", "impl", "", "Ünïcode", "🚀"})
	want := []string{"impl", "photos", "ünïcode", "🚀"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("NormalizeTags (-want +got):\n%s", diff)
	}
}

func TestEffectiveFields(t *testing.T) {
	parent := Task{ID: 1, Priority: 1, Tags: []string{"photos"}}
	child := Task{ID: 2, ParentID: 1, Priority: 3, Tags: []string{"impl"}}

	if got := EffectivePriority(child, &parent); got != 1 {
		t.Errorf("child priority = %d; want parent's 1", got)
	}
	if got := EffectivePriority(child, nil); got != 3 {
		t.Errorf("top-level priority = %d; want own 3", got)
	}
	if diff := cmp.Diff([]string{"impl", "photos"}, EffectiveTags(child, &parent)); diff != "" {
		t.Errorf("child tags (-want +got):\n%s", diff)
	}
}

func TestInInbox(t *testing.T) {
	triagedParent := &Task{Priority: 2}
	cases := []struct {
		name   string
		task   Task
		parent *Task
		want   bool
	}{
		{"title only", Task{Status: StatusOpen}, nil, true},
		{"priority only", Task{Status: StatusOpen, Priority: 1}, nil, true},
		{"size only", Task{Status: StatusOpen, Size: SizeS}, nil, true},
		{"priority and size", Task{Status: StatusOpen, Priority: 1, Size: SizeS}, nil, false},
		{"child with size inherits priority", Task{Status: StatusOpen, Size: SizeM}, triagedParent, false},
		{"child of untriaged parent", Task{Status: StatusOpen, Size: SizeM}, &Task{}, true},
		{"done", Task{Status: StatusDone}, nil, false},
		{"source-linked, unsized", Task{Status: StatusOpen, Source: &SourceRef{Kind: "github", ID: "a/b#1"}}, nil, false},
	}
	for _, c := range cases {
		if got := InInbox(c.task, c.parent); got != c.want {
			t.Errorf("%s: InInbox = %v; want %v", c.name, got, c.want)
		}
	}
}

func TestSnapshotInboxOrderAndParent(t *testing.T) {
	t0 := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	snap := NewSnapshot([]Task{
		{ID: 3, Status: StatusOpen, CreatedAt: t0.Add(2 * time.Hour)},
		{ID: 1, Status: StatusOpen, Priority: 1, Size: SizeXL, CreatedAt: t0},
		{ID: 2, Status: StatusOpen, CreatedAt: t0.Add(time.Hour)},
		{ID: 4, Status: StatusOpen, ParentID: 1, Size: SizeS, CreatedAt: t0},
	})
	var ids []int64
	for _, task := range snap.Inbox() {
		ids = append(ids, task.ID)
	}
	if diff := cmp.Diff([]int64{2, 3}, ids); diff != "" {
		t.Errorf("Inbox ids (-want +got):\n%s", diff)
	}
	child, _ := snap.Get(4)
	if p := snap.ParentOf(child); p == nil || p.ID != 1 {
		t.Errorf("ParentOf(4) = %v; want task 1", p)
	}
}

func TestSnapshotTasksIsACopy(t *testing.T) {
	snap := NewSnapshot([]Task{{ID: 1}, {ID: 2}, {ID: 3}})
	got := snap.Tasks()
	slices.SortFunc(got, func(a, b Task) int { return stdcmp.Compare(b.ID, a.ID) })
	for _, id := range []int64{1, 2, 3} {
		if task, ok := snap.Get(id); !ok || task.ID != id {
			t.Errorf("Get(%d) = %+v, %v after sorting Tasks()", id, task, ok)
		}
	}
}
