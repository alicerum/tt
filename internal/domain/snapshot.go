package domain

import (
	"cmp"
	"slices"
)

// Snapshot is an immutable in-memory view of all tasks.
type Snapshot struct {
	tasks []Task
	byID  map[int64]int
}

func NewSnapshot(tasks []Task) Snapshot {
	s := Snapshot{tasks: tasks, byID: make(map[int64]int, len(tasks))}
	for i, t := range tasks {
		s.byID[t.ID] = i
	}
	return s
}

// Tasks returns a copy; callers may sort it freely.
func (s Snapshot) Tasks() []Task { return slices.Clone(s.tasks) }

func (s Snapshot) Get(id int64) (Task, bool) {
	i, ok := s.byID[id]
	if !ok {
		return Task{}, false
	}
	return s.tasks[i], true
}

// ParentOf returns t's parent, or nil if t has none.
func (s Snapshot) ParentOf(t Task) *Task {
	if t.ParentID == 0 {
		return nil
	}
	p, ok := s.Get(t.ParentID)
	if !ok {
		return nil
	}
	return &p
}

// Inbox returns tasks needing triage, oldest first.
func (s Snapshot) Inbox() []Task {
	var out []Task
	for _, t := range s.tasks {
		if InInbox(t, s.ParentOf(t)) {
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b Task) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID))
	})
	return out
}
