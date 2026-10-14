package domain

import "slices"

// EffectiveTags returns t's own tags merged with its parent's.
func EffectiveTags(t Task, parent *Task) []string {
	if parent == nil {
		return t.Tags
	}
	return NormalizeTags(append(slices.Clone(t.Tags), parent.Tags...))
}

// InInbox reports whether t still needs triage (D11, D74): it is open and
// not source-linked, and it is top-level without an impact or a size, or a
// child without its own size. A child takes its parent's impact, so an
// untriaged parent is triaged in the Inbox for all of its children.
func InInbox(t Task, parent *Task) bool {
	if t.Status != StatusOpen || t.Source != nil {
		return false
	}
	if t.ParentID != 0 {
		return t.Size == SizeNone
	}
	return t.Impact == 0 || t.Size == SizeNone
}
