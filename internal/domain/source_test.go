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

func TestOneLine(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"plain", "Standup", "Standup"},
		{"newline and tab", "a\nb\tc", "a b c"},
		{"runs of spaces", "a    b", "a b"},
		{"leading and trailing", "  a \n\n b\t\tc  ", "a b c"},
		{"CSI", "\x1b[31mred", "[31mred"},
		{"OSC", "\x1b]52;c;aGk=\x07x", "]52;c;aGk= x"},
		{"C1", "x\u009by", "x y"},
		{"only controls", "\x07\x1b", ""},
		{"unicode kept", "Café ■ 日本", "Café ■ 日本"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := OneLine(tc.in); got != tc.want {
				t.Errorf("OneLine(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}
