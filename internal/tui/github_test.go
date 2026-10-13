package tui

import (
	"context"
	"strconv"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/source"
)

// reviewItem is a review request for PR n in repo (owner/name), requested
// ago before the harness clock.
func (h *harness) reviewItem(repo string, n int, title, author string, ago time.Duration) domain.SourceItem {
	return domain.SourceItem{Kind: "github", ExternalID: repo + "#" + strconv.Itoa(n), Number: n, Title: title,
		URL: "https://github.com/" + repo + "/pull/" + strconv.Itoa(n), Author: author, Repo: repo, RequestedAt: h.clock.now.Add(-ago)}
}

// setReviews makes items what GitHub reports, and fetches.
func (h *harness) setReviews(items ...domain.SourceItem) {
	h.t.Helper()
	h.src.Set(items, nil)
	h.keys("R")
}

func TestGitHubHeaderStatus(t *testing.T) {
	h := newHarness(t, config.Default())
	h.mustNotSee("gh") // GitHub off: no header text (D50)

	h = newGHHarness(t, config.Default())
	if n := h.src.Calls(); n != 1 {
		t.Fatalf("startup fetches GitHub once, got %d", n)
	}
	h.mustNotSee(" · gh", "⚠") // fresh: nothing

	h.src.Set(nil, source.ErrNotLoggedIn)
	h.keys("R")
	if n := h.src.Calls(); n != 2 {
		t.Fatalf("R fetches GitHub now, got %d fetches", n)
	}
	h.mustSee("tt · Today · Mon 12 Oct · cal: not set up (tt-cal calendars) · ⚠ gh: not logged in · gh auth login")

	h.src.Set(nil, nil)
	h.keys("R")
	h.mustNotSee("⚠")
	h.clock.now = h.clock.now.Add(10 * time.Minute) // two polls of 5m: still fresh
	h.mustNotSee(" · gh")
	h.clock.now = h.clock.now.Add(time.Minute)
	h.mustSee("(tt-cal calendars) · gh 11m ago")
}

func TestGitHubPollLoop(t *testing.T) {
	cfg := config.Default()
	cfg.GitHubPoll = 7 * time.Minute
	h := newGHHarness(t, cfg)
	var waits []time.Duration
	var ticks []ghTickMsg
	h.m.opts.Schedule = func(d time.Duration, msg tea.Msg) tea.Cmd {
		if tick, ok := msg.(ghTickMsg); ok {
			waits, ticks = append(waits, d), append(ticks, tick)
		}
		return nil
	}

	h.send(ghTickMsg{seq: h.m.gh.seq})
	if n := h.src.Calls(); n != 2 || len(ticks) != 1 || waits[0] != 7*time.Minute {
		t.Fatalf("a tick fetches and schedules the next one after the poll: %d fetches, waits %v", n, waits)
	}
	h.keys("R") // overtakes the scheduled tick
	if n := h.src.Calls(); n != 3 || len(ticks) != 2 {
		t.Fatalf("R: %d fetches, %d ticks scheduled", n, len(ticks))
	}
	h.send(ticks[0]) // the overtaken tick
	if n := h.src.Calls(); n != 3 {
		t.Errorf("an overtaken tick doesn't fetch again (one loop only): %d fetches", n)
	}
	h.send(ticks[1])
	if n := h.src.Calls(); n != 4 {
		t.Errorf("the current tick fetches: %d fetches", n)
	}

	h.m.gh.syncing = true // a fetch still running
	h.keys("R")
	if n := h.src.Calls(); n != 4 {
		t.Errorf("one fetch at a time: %d fetches", n)
	}
	h.m.gh.syncing = false

	h.clock.now = h.clock.now.Add(24 * time.Hour) // a new day doesn't fetch GitHub (reviews aren't per day)
	h.send(clockTickMsg{})
	if n := h.src.Calls(); n != 4 || h.m.banner == nil {
		t.Errorf("new day: %d fetches, banner %v", n, h.m.banner != nil)
	}
}

func TestAutoCloseSaysSo(t *testing.T) {
	h := newGHHarness(t, config.Default())
	ctx := context.Background()
	a := h.reviewItem("acme/api", 901, "Add idempotency key", "ana", time.Hour)
	b := h.reviewItem("acme/photos", 877, "Bump go", "tom", time.Hour)
	c := h.reviewItem("acme/backend", 915, "Lint config", "ion", time.Hour)
	h.setReviews(a, b, c)
	for _, i := range []domain.SourceItem{a, b, c} {
		if _, _, err := h.svc.PinSource(ctx, "github", i.ExternalID); err != nil {
			t.Fatal(err)
		}
	}
	h.setReviews(b, c)
	h.mustSee(`✓ via GitHub: "Review #901: Add idempotency key"`) // D49
	h.setReviews()
	h.mustSee("✓ via GitHub: 2 reviews")
	h.setReviews()
	h.mustNotSee("✓ via GitHub")

	h.keys("tab", "enter") // the first candidate's detail
	h.mustSee("✓ via GitHub · Mon 12 Oct")
}
