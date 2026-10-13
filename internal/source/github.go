// Package source fetches what is awaiting me from outside tt (D20): GitHub
// review requests, through the gh CLI (spec: Adapters, D50). It has no rules
// of its own: app reconciles the result.
package source

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"
	"time"

	"tt/internal/domain"
)

// The failures with a fix of their own (D50); their text is what the header shows.
var (
	ErrGHNotFound  = errors.New("gh not found · brew install gh")
	ErrNotLoggedIn = errors.New("not logged in · gh auth login")
	ErrTooMany     = fmt.Errorf("over %d review requests", MaxItems)
)

// DefaultTimeout bounds one gh run (D50).
const DefaultTimeout = 20 * time.Second

// MaxItems is how many review requests one fetch can return. A larger set
// fails the fetch (ErrTooMany): reconciling a truncated set would
// auto-close the requests left out.
const MaxItems = 100

// query is the one search a fetch runs: open PRs that request my review
// directly (team requests aren't included), with the latest review-request
// events for the time I was asked.
const query = `query {
  viewer { login }
  search(type: ISSUE, first: 100, query: "is:open is:pr user-review-requested:@me archived:false") {
    issueCount
    nodes {
      ... on PullRequest {
        number title url
        author { login }
        repository { nameWithOwner }
        timelineItems(itemTypes: [REVIEW_REQUESTED_EVENT], last: 10) {
          nodes { ... on ReviewRequestedEvent { createdAt requestedReviewer { ... on User { login } } } }
        }
      }
    }
  }
}`

// GitHub runs `gh api graphql` for the review requests awaiting me (D50).
type GitHub struct {
	Host    string        // GitHub Enterprise host; "" is gh's default (github.com)
	Path    string        // the gh binary; "" finds gh on PATH
	Timeout time.Duration // 0 means DefaultTimeout
}

func (g *GitHub) Kind() string { return domain.SourceGitHub }

// Fetch returns every open PR that requests my review. Errors are short and
// say what to do: ErrGHNotFound, ErrNotLoggedIn (gh exit 4, or HTTP 401), ErrTooMany, a
// timeout, bad JSON, else the first line of gh's stderr.
func (g *GitHub) Fetch(ctx context.Context) ([]domain.SourceItem, error) {
	timeout := cmp.Or(g.Timeout, DefaultTimeout)
	run, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{"api", "graphql"}
	if g.Host != "" {
		args = append(args, "--hostname", g.Host)
	}
	args = append(args, "-f", "query="+query)
	cmd := exec.CommandContext(run, cmp.Or(g.Path, "gh"), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = time.Second // a child left holding the pipes can't keep us waiting
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return parse(stdout.Bytes())
	case errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist):
		return nil, ErrGHNotFound
	case ctx.Err() != nil:
		return nil, ctx.Err() // tt is quitting
	case run.Err() != nil:
		return nil, fmt.Errorf("gh timed out after %s", timeout)
	case errors.As(err, &exit) && exit.ExitCode() == 4:
		return nil, ErrNotLoggedIn
	}
	if strings.Contains(stderr.String(), "HTTP 401") { // an expired or revoked token
		return nil, ErrNotLoggedIn
	}
	if line := firstLine(stderr.String()); line != "" {
		return nil, errors.New(line)
	}
	return nil, fmt.Errorf("gh failed: %v", err)
}

// response is the part of the GraphQL answer Fetch reads.
type response struct {
	Data struct {
		Viewer struct {
			Login string `json:"login"`
		} `json:"viewer"`
		Search struct {
			IssueCount int `json:"issueCount"`
			Nodes      []struct {
				Number int    `json:"number"`
				Title  string `json:"title"`
				URL    string `json:"url"`
				Author *struct {
					Login string `json:"login"`
				} `json:"author"` // null for a deleted account
				Repository struct {
					NameWithOwner string `json:"nameWithOwner"`
				} `json:"repository"`
				TimelineItems struct {
					Nodes []struct {
						CreatedAt         time.Time `json:"createdAt"`
						RequestedReviewer *struct {
							Login string `json:"login"`
						} `json:"requestedReviewer"` // null for a team
					} `json:"nodes"`
				} `json:"timelineItems"`
			} `json:"nodes"`
		} `json:"search"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func parse(data []byte) ([]domain.SourceItem, error) {
	var r response
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("gh printed bad JSON: %v", err)
	}
	if len(r.Errors) > 0 {
		return nil, errors.New(firstLine(r.Errors[0].Message))
	}
	if r.Data.Search.IssueCount > MaxItems {
		return nil, ErrTooMany
	}
	me := r.Data.Viewer.Login
	items := make([]domain.SourceItem, 0, len(r.Data.Search.Nodes))
	for _, n := range r.Data.Search.Nodes {
		if n.Number == 0 {
			// The search is is:pr, so an empty node is data withheld:
			// reconciling the rest would auto-close what it left out.
			return nil, errors.New("gh returned an incomplete answer")
		}
		i := domain.SourceItem{
			Kind:       domain.SourceGitHub,
			ExternalID: fmt.Sprintf("%s#%d", n.Repository.NameWithOwner, n.Number),
			Number:     n.Number, Title: n.Title, URL: n.URL, Author: "ghost", Repo: n.Repository.NameWithOwner,
		}
		if n.Author != nil {
			i.Author = n.Author.Login
		}
		for _, e := range n.TimelineItems.Nodes {
			if e.RequestedReviewer != nil && strings.EqualFold(e.RequestedReviewer.Login, me) && e.CreatedAt.After(i.RequestedAt) {
				i.RequestedAt = e.CreatedAt
			}
		}
		items = append(items, i)
	}
	return items, nil
}

// firstLine is the first non-empty line of s, without gh's "gh: " prefix.
func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "gh: "))
}
