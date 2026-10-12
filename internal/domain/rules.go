package domain

import "slices"

// EffectivePriority returns the priority that applies to t. Children always
// inherit their parent's priority; their own value is ignored (no override in v1).
func EffectivePriority(t Task, parent *Task) int {
	if parent != nil {
		return parent.Priority
	}
	return t.Priority
}

// EffectiveTags returns t's own tags merged with its parent's.
func EffectiveTags(t Task, parent *Task) []string {
	if parent == nil {
		return t.Tags
	}
	return NormalizeTags(append(slices.Clone(t.Tags), parent.Tags...))
}

// InInbox reports whether t still needs triage (D11): it is open, not
// source-linked, and lacks an effective priority or a size.
func InInbox(t Task, parent *Task) bool {
	if t.Status != StatusOpen || t.Source != nil {
		return false
	}
	return EffectivePriority(t, parent) == 0 || t.Size == SizeNone
}
