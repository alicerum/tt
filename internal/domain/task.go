// Package domain holds tt's entities and pure rules. It does no I/O.
package domain

import (
	"slices"
	"strings"
	"time"
)

type Status string

const (
	StatusOpen    Status = "open"
	StatusDone    Status = "done"
	StatusDropped Status = "dropped"
)

const (
	ClosedByUser   = "user"
	ClosedBySource = "source"
)

type Size string

const (
	SizeNone Size = ""
	SizeS    Size = "s"
	SizeM    Size = "m"
	SizeL    Size = "l"
	SizeXL   Size = "xl"
)

// Sizes lists all sizes, smallest first.
var Sizes = []Size{SizeS, SizeM, SizeL, SizeXL}

// ParseSize accepts s, m, l, xl and x (alias for xl), case-insensitive.
func ParseSize(s string) (Size, bool) {
	switch strings.ToLower(s) {
	case "s":
		return SizeS, true
	case "m":
		return SizeM, true
	case "l":
		return SizeL, true
	case "xl", "x":
		return SizeXL, true
	}
	return SizeNone, false
}

// SourceRef links a task to the source item it was materialized from (D20).
type SourceRef struct {
	Kind string // e.g. "github"
	ID   string // external id, e.g. "owner/repo#482"
}

type Task struct {
	ID            int64
	Title         string
	Note          string
	Link          string
	Requester     string   // lowercase; "" = none
	Tags          []string // normalized: lowercase, unique, sorted
	Impact        int      // 0 = unset; 1–3, who notices if it slips (D68)
	Due           Day      // "" = none (D69)
	Size          Size
	ParentID      int64 // 0 = no parent
	Position      int   // order among siblings
	Status        Status
	DropReason    string
	Source        *SourceRef
	RolloverCount int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ClosedAt      time.Time // zero while open
	ClosedBy      string    // ClosedByUser / ClosedBySource; "" while open
}

// NormalizeTags lowercases, trims, drops empties, sorts and dedupes.
func NormalizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			out = append(out, t)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
