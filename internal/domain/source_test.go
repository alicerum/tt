package domain

import (
	"testing"
	"time"
)

func TestSourceItem(t *testing.T) {
	seen := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	i := SourceItem{Kind: SourceGitHub, ExternalID: "acme/api#901", Number: 901, Title: "Add idempotency key", FirstSeenAt: seen}
	if got := i.WaitingSince(); !got.Equal(seen) {
		t.Errorf("without a request time, waiting since first seen: %v", got)
	}
	i.RequestedAt = seen.Add(-48 * time.Hour)
	if got := i.WaitingSince(); !got.Equal(i.RequestedAt) {
		t.Errorf("waiting since the request: %v", got)
	}
	if got := ReviewTitle(i); got != "Review #901: Add idempotency key" {
		t.Errorf("ReviewTitle = %q", got)
	}
	if got := i.Ref(); got != (SourceRef{Kind: "github", ID: "acme/api#901"}) {
		t.Errorf("Ref = %+v", got)
	}
}
