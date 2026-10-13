package source

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"tt/internal/domain"
)

// fakeGH writes a fake gh script into dir and returns its path.
func fakeGH(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "gh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

const answer = `{"data":{"viewer":{"login":"Me"},"search":{"issueCount":3,"nodes":[
 {"number":901,"title":"Add idempotency key","url":"https://github.com/acme/api/pull/901","author":{"login":"ana"},
  "repository":{"nameWithOwner":"acme/api"},"timelineItems":{"nodes":[
   {"createdAt":"2026-10-09T08:00:00Z","requestedReviewer":{"login":"me"}},
   {"createdAt":"2026-10-10T08:00:00Z","requestedReviewer":{"login":"me"}},
   {"createdAt":"2026-10-11T08:00:00Z","requestedReviewer":{"login":"tom"}},
   {"createdAt":"2026-10-11T09:00:00Z","requestedReviewer":null}]}},
 {"number":877,"title":"Bump go","url":"https://github.com/acme/photos/pull/877","author":null,
  "repository":{"nameWithOwner":"acme/photos"},"timelineItems":{"nodes":[]}}]}}}`

func TestFetchRunsGH(t *testing.T) {
	dir := t.TempDir()
	path := fakeGH(t, dir, `for a in "$@"; do echo "$a"; done > "$(dirname "$0")/args"
cat <<'JSON'
`+answer+`
JSON`)
	items, err := (&GitHub{Path: path}).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.SourceItem{
		{Kind: "github", ExternalID: "acme/api#901", Number: 901, Title: "Add idempotency key", URL: "https://github.com/acme/api/pull/901",
			Author: "ana", Repo: "acme/api", RequestedAt: time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)}, // my latest request, not tom's
		{Kind: "github", ExternalID: "acme/photos#877", Number: 877, Title: "Bump go", URL: "https://github.com/acme/photos/pull/877",
			Author: "ghost", Repo: "acme/photos"}, // a deleted author; no request event
	}
	if diff := cmp.Diff(want, items); diff != "" {
		t.Errorf("items (-want +got):\n%s", diff)
	}
	args, err := os.ReadFile(filepath.Join(dir, "args"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitN(string(args), "\n", 4)
	if lines[0] != "api" || lines[1] != "graphql" || lines[2] != "-f" || !strings.HasPrefix(lines[3], "query=query {") ||
		!strings.Contains(lines[3], `user-review-requested:@me archived:false`) {
		t.Errorf("args:\n%s", args)
	}

	if _, err := (&GitHub{Path: path, Host: "github.acme.com"}).Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	args, _ = os.ReadFile(filepath.Join(dir, "args"))
	if !strings.HasPrefix(string(args), "api\ngraphql\n--hostname\ngithub.acme.com\n-f\nquery=") {
		t.Errorf("an Enterprise host goes to gh:\n%s", args)
	}
	if k := (&GitHub{}).Kind(); k != domain.SourceGitHub {
		t.Errorf("Kind = %q", k)
	}
}

func TestFetchFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       string
	}{
		{"not logged in", "echo 'To get started with GitHub CLI, please run:  gh auth login' >&2; exit 4", ErrNotLoggedIn.Error()},
		{"bad token", "echo 'gh: Bad credentials (HTTP 401)' >&2; exit 1", ErrNotLoggedIn.Error()},
		{"bad token, other wording", "echo 'gh: HTTP 401: Bad credentials (https://api.github.com/graphql)' >&2; exit 1", ErrNotLoggedIn.Error()},
		{"incomplete answer", `echo '{"data":{"viewer":{"login":"me"},"search":{"issueCount":2,"nodes":[{"number":5,"title":"t","repository":{"nameWithOwner":"a/b"}},{}]}}}'`,
			"gh returned an incomplete answer"},
		{"silent", "exit 3", "gh failed: exit status 3"},
		{"bad JSON", "echo '{not json'", "gh printed bad JSON: "},
		{"GraphQL error", `echo '{"errors":[{"message":"Resource protected by organization SAML enforcement."}]}'`,
			"Resource protected by organization SAML enforcement."},
		{"too many", `echo '{"data":{"viewer":{"login":"me"},"search":{"issueCount":101,"nodes":[]}}}'`, "over 100 review requests"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items, err := (&GitHub{Path: fakeGH(t, t.TempDir(), tc.body)}).Fetch(context.Background())
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) || items != nil {
				t.Errorf("items %v, err = %v; want %q", items, err, tc.want)
			}
		})
	}
	ok := `echo '{"data":{"viewer":{"login":"me"},"search":{"issueCount":100,"nodes":[]}}}'`
	if items, err := (&GitHub{Path: fakeGH(t, t.TempDir(), ok)}).Fetch(context.Background()); err != nil || len(items) != 0 {
		t.Errorf("exactly 100 is fine: %v, %v", items, err)
	}
}

func TestFetchTimesOut(t *testing.T) {
	g := &GitHub{Path: fakeGH(t, t.TempDir(), "sleep 10"), Timeout: 100 * time.Millisecond}
	began := time.Now()
	_, err := g.Fetch(context.Background())
	if err == nil || err.Error() != "gh timed out after 100ms" {
		t.Errorf("err = %v; want the timeout", err)
	}
	if took := time.Since(began); took > 3*time.Second {
		t.Errorf("Fetch took %v after a timeout", took)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.Fetch(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled context: err = %v; want context.Canceled", err)
	}
}

func TestFetchFindsGH(t *testing.T) {
	if _, err := (&GitHub{Path: filepath.Join(t.TempDir(), "gh")}).Fetch(context.Background()); !errors.Is(err, ErrGHNotFound) {
		t.Errorf("missing gh: err = %v; want ErrGHNotFound", err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := (&GitHub{}).Fetch(context.Background()); !errors.Is(err, ErrGHNotFound) {
		t.Errorf("not on PATH: err = %v; want ErrGHNotFound", err)
	}
	onPath := t.TempDir()
	fakeGH(t, onPath, `echo '{"data":{"viewer":{"login":"me"},"search":{"issueCount":0,"nodes":[]}}}'`)
	t.Setenv("PATH", onPath+string(os.PathListSeparator)+"/bin:/usr/bin")
	if items, err := (&GitHub{}).Fetch(context.Background()); err != nil || len(items) != 0 {
		t.Errorf("on PATH: %v, %v; want no items and no error", items, err)
	}
}

func TestFake(t *testing.T) {
	var f Fake
	f.Set([]domain.SourceItem{{ExternalID: "a#1"}}, nil)
	items, err := f.Fetch(context.Background())
	if err != nil || len(items) != 1 || f.Calls() != 1 || f.Kind() != domain.SourceGitHub {
		t.Errorf("Fetch = %v, %v; calls %d", items, err, f.Calls())
	}
	f.Set(nil, errors.New("down"))
	if _, err := f.Fetch(context.Background()); err == nil || f.Calls() != 2 {
		t.Errorf("Set's error comes back: %v, calls %d", err, f.Calls())
	}
}
