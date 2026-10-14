package dsl

import (
	"errors"
	"testing"
)

func TestParseRequester(t *testing.T) {
	cases := []struct {
		in, want string
		err      error
	}{
		{"raff", "raff", nil},
		{"  @Raff ", "raff", nil},
		{"Ana,", "ana", nil},
		{"@@ana", "@ana", nil},
		{"ana-maria", "ana-maria", nil},
		{"Ünïcode", "ünïcode", nil},
		{"", "", ErrEmptyName},
		{"  ", "", ErrEmptyName},
		{"@", "", ErrEmptyName},
		{"@.", "", ErrEmptyName},
		{"ana maria", "", ErrNameWords},
		{"@ana\tb", "", ErrNameWords},
	}
	for _, c := range cases {
		got, err := ParseRequester(c.in)
		if got != c.want || !errors.Is(err, c.err) {
			t.Errorf("ParseRequester(%q) = %q, %v; want %q, %v", c.in, got, err, c.want, c.err)
		}
	}
}

// A name typed in the rename prompt is stored exactly as the same name
// typed as @who in a quick-add line (D53).
func TestParseRequesterAgreesWithTheToken(t *testing.T) {
	for _, name := range []string{"ana", "Ana", "ana,", "ANA.", "@ana", "ana-maria", "bob@x.com", "Ünïcode"} {
		d, err := ParseAdd("Ask @"+name, wednesday)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := ParseRequester("@" + name); err != nil || got != d.Requester {
			t.Errorf("ParseRequester(%q) = %q, %v; the token gives %q", "@"+name, got, err, d.Requester)
		}
	}
}
