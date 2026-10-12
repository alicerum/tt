package dsl

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"tt/internal/domain"
)

func TestParseFilter(t *testing.T) {
	open, done, dropped := domain.StatusOpen, domain.StatusDone, domain.StatusDropped
	cases := []struct {
		line string
		want Filter
	}{
		{"", Filter{}},
		{"   ", Filter{}},
		{"@ana #impl !1 ~l <7d schema", Filter{Requesters: []string{"ana"}, Tags: []string{"impl"}, Priorities: []int{1},
			Sizes: []domain.Size{domain.SizeL}, NewerThan: 7 * day, Words: []string{"schema"}}},
		{"@Ana @bob @ana", Filter{Requesters: []string{"ana", "bob"}}},
		{"#B #a #b", Filter{Tags: []string{"a", "b"}}},
		{"!1 !2 !1 ~s ~x ~XL", Filter{Priorities: []int{1, 2}, Sizes: []domain.Size{domain.SizeS, domain.SizeXL}}},
		{">30d", Filter{OlderThan: 30 * day}},
		{"<2w >1d", Filter{NewerThan: 14 * day, OlderThan: day}},
		{"<7D", Filter{NewerThan: 7 * day}},
		{"is:done", Filter{Statuses: []domain.Status{done}}},
		{"is:Dropped IS:done", Filter{Statuses: []domain.Status{dropped, done}}},
		{"is:all is:open", Filter{Statuses: []domain.Status{open, done, dropped}}},
		{"Schema MIGR", Filter{Words: []string{"schema", "migr"}}},
		// Look-alikes stay words (D26d, as in D25).
		{"#482 ~/tmp this! <7 <d <7x", Filter{Words: []string{"#482", "~/tmp", "this!", "<7", "<d", "<7x"}}},
		// A lone sigil is a token still being typed: ignored, so live results don't flicker.
		{"schema @ # ! ~ < >", Filter{Words: []string{"schema"}}},
		{"@. #, foo:bar !x ~zz", Filter{Words: []string{"@.", "#,", "foo:bar", "!x", "~zz"}}},
		{"@ana, #photos.", Filter{Requesters: []string{"ana"}, Tags: []string{"photos"}}},
		{"https://x.io -- note", Filter{Words: []string{"https://x.io", "--", "note"}}},
	}
	for _, c := range cases {
		got, err := ParseFilter(c.line, 3)
		if err != nil {
			t.Errorf("ParseFilter(%q): %v", c.line, err)
			continue
		}
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("ParseFilter(%q) (-want +got):\n%s", c.line, diff)
		}
	}
}

func TestParseFilterErrors(t *testing.T) {
	cases := map[string]string{
		"x !4":     "priority !4 out of range: use !1 to !3",
		"x !0":     "priority !0 out of range",
		"<0d":      "<0d: age must be 1 to 10000 days or weeks",
		">99999w":  ">99999w: age must be",
		"is:":      "unknown status is:",
		"is:later": "unknown status is:later: use is:open, is:done, is:dropped or is:all",
	}
	for line, want := range cases {
		_, err := ParseFilter(line, 3)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseFilter(%q) error = %v; want it to contain %q", line, err, want)
		}
	}
}

func TestFilterEmpty(t *testing.T) {
	for line, want := range map[string]bool{"": true, "  ": true, "x": false, "is:open": false, "<1d": false} {
		f, err := ParseFilter(line, 3)
		if err != nil {
			t.Fatal(err)
		}
		if f.Empty() != want {
			t.Errorf("ParseFilter(%q).Empty() = %v; want %v", line, f.Empty(), want)
		}
	}
}
