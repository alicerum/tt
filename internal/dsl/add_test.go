package dsl

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"tt/internal/domain"
)

func TestParseAdd(t *testing.T) {
	cases := []struct {
		line string
		want Draft
	}{
		{"Reply to Ana", Draft{Title: "Reply to Ana"}},
		{
			"Reply to Ana re schema @Ana #photos #Impl !1 ~s https://x.slack.com/archives/C1/p2",
			Draft{Title: "Reply to Ana re schema", Requester: "ana", Tags: []string{"impl", "photos"},
				Priority: 1, Size: domain.SizeS, Link: "https://x.slack.com/archives/C1/p2"},
		},
		{"tokens #a anywhere ~xl in @bob the line", Draft{Title: "tokens anywhere in the line", Requester: "bob", Tags: []string{"a"}, Size: domain.SizeXL}},
		{"~X alias", Draft{Title: "alias", Size: domain.SizeXL}},
		{"last @a requester @b wins", Draft{Title: "last requester wins", Requester: "b"}},
		{"duplicate #x #X tags", Draft{Title: "duplicate tags", Tags: []string{"x"}}},
		// Review Focus: ordinary words that look like tokens stay in the title.
		{"Review PR #482 today", Draft{Title: "Review PR #482 today"}},
		{"clean ~/tmp and ~zz", Draft{Title: "clean ~/tmp and ~zz"}},
		{"Fix this! now !!", Draft{Title: "Fix this! now !!"}},
		{"lone @ # ! ~ sigils", Draft{Title: "lone @ # ! ~ sigils"}},
		{"sigil plus punctuation @. #, ~? !)", Draft{Title: "sigil plus punctuation @. #, ~? !)"}},
		{"mail bob@x.com about #2", Draft{Title: "mail bob@x.com about #2"}},
		{"two https://a.io links https://b.io", Draft{Title: "two links https://b.io", Link: "https://a.io"}},
		{"🚀 launch #Ünïcode", Draft{Title: "🚀 launch", Tags: []string{"ünïcode"}}},
		// Trailing punctuation on sigil tokens.
		{"Fix #482.", Draft{Title: "Fix #482."}},
		{"Review #482, then merge", Draft{Title: "Review #482, then merge"}},
		{"Ping @ana, about schema", Draft{Title: "Ping about schema", Requester: "ana"}},
		{"Ask @ana.", Draft{Title: "Ask", Requester: "ana"}},
		{"Tag it #photos.", Draft{Title: "Tag it", Tags: []string{"photos"}}},
		{"x !1.", Draft{Title: "x", Priority: 1}},
		// Notes.
		{"Call Ivan -- he needs it before Friday's sync", Draft{Title: "Call Ivan", Note: "he needs it before Friday's sync", HasNote: true}},
		{"Ping #infra -- see https://x.io #not-a-tag !1 -- twice", Draft{Title: "Ping", Tags: []string{"infra"}, Note: "see https://x.io #not-a-tag !1 -- twice", HasNote: true}},
		{"Trailing separator --", Draft{Title: "Trailing separator", HasNote: true}},
		{"Trailing separator -- ", Draft{Title: "Trailing separator", HasNote: true}},
		{"en--dash stays", Draft{Title: "en--dash stays"}},
	}
	for _, c := range cases {
		got, err := ParseAdd(c.line, 3)
		if err != nil {
			t.Errorf("ParseAdd(%q) error: %v", c.line, err)
			continue
		}
		if diff := cmp.Diff(c.want, got, cmp.Comparer(tagsEqual)); diff != "" {
			t.Errorf("ParseAdd(%q) (-want +got):\n%s", c.line, diff)
		}
	}
}

// tagsEqual treats nil and empty tag slices as equal.
func tagsEqual(a, b []string) bool { return strings.Join(a, ",") == strings.Join(b, ",") }

func TestParseAddErrors(t *testing.T) {
	for _, line := range []string{"", "   ", "@ana #x !1 ~s", "https://only.a.link", "-- just a note"} {
		if _, err := ParseAdd(line, 3); !errors.Is(err, ErrEmptyTitle) {
			t.Errorf("ParseAdd(%q) error = %v; want ErrEmptyTitle", line, err)
		}
	}
	for _, line := range []string{"x !0", "x !4", "x !99999999999999999999"} {
		_, err := ParseAdd(line, 3)
		if err == nil || !strings.Contains(err.Error(), "use !1 to !3") {
			t.Errorf("ParseAdd(%q) error = %v; want priority range error", line, err)
		}
	}
}

func TestFormatAddRoundTrip(t *testing.T) {
	drafts := []Draft{
		{Title: "Just a title"},
		{Title: "Review PR #482", Requester: "ana", Tags: []string{"impl", "photos"}, Priority: 2, Size: domain.SizeXL, Link: "https://x.io/a?b=c#d"},
		{Title: "With note", Note: "multi -- separators", HasNote: true},
		{Title: "Empty note", HasNote: true},
		{Title: "two links https://b.io", Link: "https://a.io"},
	}
	for _, d := range drafts {
		line := FormatAdd(d)
		got, err := ParseAdd(line, 3)
		if err != nil {
			t.Errorf("ParseAdd(FormatAdd(%+v)) = %q: %v", d, line, err)
			continue
		}
		if diff := cmp.Diff(d, got, cmp.Comparer(tagsEqual)); diff != "" {
			t.Errorf("round trip via %q (-want +got):\n%s", line, diff)
		}
	}
}
