package domain

import (
	"fmt"
	"time"
)

// SourceGitHub is the source kind of GitHub review requests (D20, D50).
const SourceGitHub = "github"

// SourceItem is a read-only item a source reports as awaiting me (D20): for
// GitHub, an open pull request that requests my review. Pinning one
// materializes a task that links back to it.
type SourceItem struct {
	Kind        string // e.g. SourceGitHub
	ExternalID  string // unique per kind: "owner/repo#123"
	Number      int
	Title       string
	URL         string
	Author      string    // shown in the Reviews lane, never copied into a requester (D20b)
	Repo        string    // "owner/repo"
	RequestedAt time.Time // when I was (last) requested; zero when the source doesn't say
	FirstSeenAt time.Time // when tt first saw it (kept across fetches)
}

// Ref is the source ref of a task materialized from i.
func (i SourceItem) Ref() SourceRef { return SourceRef{Kind: i.Kind, ID: i.ExternalID} }

// WaitingSince is how long i has waited on me: since RequestedAt, else
// since tt first saw it.
func (i SourceItem) WaitingSince() time.Time {
	if !i.RequestedAt.IsZero() {
		return i.RequestedAt
	}
	return i.FirstSeenAt
}

// ReviewTitle is the title of the task materialized from a review request (D20b).
func ReviewTitle(i SourceItem) string { return fmt.Sprintf("Review #%d: %s", i.Number, i.Title) }
