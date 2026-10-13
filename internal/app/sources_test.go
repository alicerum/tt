package app

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"tt/internal/config"
	"tt/internal/domain"
	"tt/internal/source"
	"tt/internal/store"
)

// srcService is a started service with a fake GitHub; it also returns the
// store, for inspecting rows by hand.
func srcService(t *testing.T) (*Service, *fakeClock, *source.Fake, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "tt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	clock := &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
	src := &source.Fake{}
	s := New(st, clock, config.Default()).WithSource(src)
	if _, err := s.StartDay(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s, clock, src, st
}

// review is a review request for PR n, requested at requested (zero: unknown).
func review(n int, title string, requested time.Time) domain.SourceItem {
	return domain.SourceItem{Kind: "github", ExternalID: "acme/api#" + strconv.Itoa(n), Number: n, Title: title,
		URL: "https://github.com/acme/api/pull/" + strconv.Itoa(n), Author: "ana", Repo: "acme/api", RequestedAt: requested}
}

func mustSyncSources(t *testing.T, s *Service) []domain.Task {
	t.Helper()
	closed, err := s.SyncSources(context.Background())
	if err != nil {
		t.Fatalf("SyncSources: %v", err)
	}
	return closed
}

func ids(items []domain.SourceItem) []string {
	var out []string
	for _, i := range items {
		out = append(out, i.ExternalID)
	}
	return out
}

// linked inserts an open task materialized from item (what PinSource does).
func linked(t *testing.T, st *store.Store, item domain.SourceItem, now time.Time) int64 {
	t.Helper()
	var id int64
	err := st.InTx(context.Background(), func(x *store.Tx) (err error) {
		ref := item.Ref()
		id, err = x.InsertTask(context.Background(), domain.Task{Title: domain.ReviewTitle(item), Link: item.URL, Source: &ref,
			Status: domain.StatusOpen, CreatedAt: now, UpdatedAt: now})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestGitHubNotSetUp(t *testing.T) {
	s, _ := startedService(t)
	if closed, err := s.SyncSources(context.Background()); err != nil || closed != nil {
		t.Fatal(closed, err)
	}
	if s.SourceSetUp() {
		t.Error("SourceSetUp without WithSource")
	}
	if gs := mustLoad(t, s).GitHub; gs.SetUp || gs.Reviews != nil || gs.Err != "" || !gs.LastSuccess.IsZero() {
		t.Errorf("GitHub off (D50): %+v", gs)
	}
}

func TestSyncSourcesFillsTheLane(t *testing.T) {
	s, clock, src, _ := srcService(t)
	if !s.SourceSetUp() {
		t.Error("SourceSetUp after WithSource")
	}
	if gs := mustLoad(t, s).GitHub; !gs.SetUp || gs.Reviews != nil || !gs.LastSuccess.IsZero() || gs.Err != "" {
		t.Fatalf("before the first fetch: %+v", gs)
	}
	start := clock.now
	src.Set([]domain.SourceItem{
		review(877, "Bump go", start.Add(-24*time.Hour)),
		review(901, "Add idempotency key", start.Add(-72*time.Hour)),
		review(920, "Fix nil deref", time.Time{}), // no request event: waits since first seen
		review(877, "Bump go", start.Add(-24*time.Hour)),
	}, nil)
	if closed := mustSyncSources(t, s); closed != nil {
		t.Errorf("nothing to close: %v", closed)
	}
	gs := mustLoad(t, s).GitHub
	if got := ids(gs.Reviews); len(got) != 3 || got[0] != "acme/api#901" || got[1] != "acme/api#877" || got[2] != "acme/api#920" {
		t.Fatalf("lane = %v; want longest waiting first, one row per PR", got)
	}
	if r := gs.Reviews[2]; !r.FirstSeenAt.Equal(start) || !r.WaitingSince().Equal(start) || r.Author != "ana" || r.Kind != "github" {
		t.Errorf("#920 = %+v", r)
	}
	if !gs.LastSuccess.Equal(start) || gs.Err != "" {
		t.Errorf("after a success: %+v", gs)
	}

	// The next fetch keeps first_seen_at; a PR gone from the set leaves the lane.
	clock.Advance(5 * time.Minute)
	src.Set([]domain.SourceItem{review(920, "Fix nil deref (v2)", time.Time{}), review(901, "Add idempotency key", start.Add(-72*time.Hour))}, nil)
	mustSyncSources(t, s)
	gs = mustLoad(t, s).GitHub
	if got := ids(gs.Reviews); len(got) != 2 || got[1] != "acme/api#920" {
		t.Fatalf("lane = %v", got)
	}
	if r := gs.Reviews[1]; !r.FirstSeenAt.Equal(start) || r.Title != "Fix nil deref (v2)" {
		t.Errorf("#920 after a refetch = %+v; want first seen kept, title refreshed", r)
	}

	// D3a: a failed fetch keeps the last good lane and says why.
	clock.Advance(5 * time.Minute)
	src.Set(nil, source.ErrNotLoggedIn)
	mustSyncSources(t, s)
	gs = mustLoad(t, s).GitHub
	if len(gs.Reviews) != 2 || gs.Err != source.ErrNotLoggedIn.Error() || !gs.LastSuccess.Equal(start.Add(5*time.Minute)) {
		t.Errorf("after a failure: %v, err %q, last success %v", ids(gs.Reviews), gs.Err, gs.LastSuccess)
	}

	// A success clears the error; an empty set is an empty lane, not a failure.
	clock.Advance(5 * time.Minute)
	src.Set(nil, nil)
	mustSyncSources(t, s)
	if gs = mustLoad(t, s).GitHub; gs.Reviews != nil || gs.Err != "" || !gs.LastSuccess.Equal(clock.now) {
		t.Errorf("after an empty success: %+v", gs)
	}
}

func TestSyncSourcesStoresOneCleanLine(t *testing.T) {
	s, clock, src, _ := srcService(t)
	ctx := context.Background()
	dirty := review(901, "Fix\n\tthe \x1b]52;c;aGk=\x07 thing", clock.now)
	dirty.Author, dirty.Repo, dirty.URL = "a\x1b[31mna", "acme\n/api", "https://x/\x07pull/901"
	dirty.ExternalID = "acme/api#9\n01"
	src.Set([]domain.SourceItem{dirty}, nil)
	mustSyncSources(t, s)
	hasControl := func(s string) bool { return strings.IndexFunc(s, unicode.IsControl) >= 0 }
	reviews := mustLoad(t, s).GitHub.Reviews
	if len(reviews) != 1 {
		t.Fatalf("lane = %v", ids(reviews))
	}
	r := reviews[0]
	for _, f := range []string{r.Title, r.Author, r.Repo, r.URL, r.ExternalID} {
		if hasControl(f) {
			t.Errorf("a stored field has control characters: %q in %+v", f, r)
		}
	}
	task, _, err := s.PinSource(ctx, "github", r.ExternalID)
	if err != nil {
		t.Fatal(err)
	}
	if hasControl(task.Title) || hasControl(task.Link) {
		t.Errorf("task title %q, link %q have control characters", task.Title, task.Link)
	}
}

func TestSyncSourcesAutoCloses(t *testing.T) {
	s, clock, src, st := srcService(t)
	ctx := context.Background()
	a, b, c := review(901, "Add idempotency key", clock.now), review(877, "Bump go", clock.now), review(915, "Lint config", clock.now)
	src.Set([]domain.SourceItem{a, b, c}, nil)
	mustSyncSources(t, s)
	aID, bID := linked(t, st, a, clock.now), linked(t, st, b, clock.now)
	cID := linked(t, st, c, clock.now)
	if _, err := s.Done(ctx, cID); err != nil { // done by me while GitHub still asks
		t.Fatal(err)
	}
	if _, _, err := s.Block(ctx, aID, 10*time.Hour, time.Hour); err != nil { // blocked today, pinned
		t.Fatal(err)
	}
	edit := mustUndo(t)(s.Edit(ctx, aID, "Review #901 with notes"))
	if got := ids(mustLoad(t, s).GitHub.Reviews); len(got) != 1 || got[0] != "acme/api#915" {
		t.Fatalf("lane = %v; want only #915: reviews with an open task leave it, a closed one comes back (D49)", got)
	}

	// D20a: #901 is no longer requested; #877 still is.
	clock.Advance(5 * time.Minute)
	src.Set([]domain.SourceItem{b, c}, nil)
	closed := mustSyncSources(t, s)
	if len(closed) != 1 || closed[0].ID != aID || closed[0].Title != "Review #901 with notes" {
		t.Fatalf("closed = %+v; want #%d", closed, aID)
	}
	got := mustGet(t, s, aID)
	if got.Status != domain.StatusDone || got.ClosedBy != domain.ClosedBySource || !got.ClosedAt.Equal(clock.now) || !got.UpdatedAt.Equal(clock.now) {
		t.Errorf("auto-closed task = %+v; want done by the source now, UpdatedAt now", got)
	}
	if got := mustGet(t, s, bID); got.Status != domain.StatusOpen {
		t.Errorf("#877 is still requested: %v", got.Status)
	}
	plan := mustPlan(t, s)
	if len(plan.Candidates) != 1 || plan.Candidates[0].Task.ID != aID || len(plan.Blocks) != 1 {
		t.Errorf("the auto-closed review stays on Today, done, with its block: %+v", plan)
	}
	if err := edit(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo of an edit made before the auto-close: %v; want ErrConflict (integrity over undo)", err)
	}
	if got := mustGet(t, s, aID); got.Status != domain.StatusDone {
		t.Errorf("the refused undo left %v", got.Status)
	}

	// A sync that changes nothing closes nothing; a closed task stays closed.
	clock.Advance(5 * time.Minute)
	if closed := mustSyncSources(t, s); closed != nil {
		t.Errorf("second sync closed %v", closed)
	}
	if got := mustGet(t, s, cID); got.ClosedBy != domain.ClosedByUser {
		t.Errorf("a task I closed keeps closed_by user: %+v", got)
	}

	// Auto-closes never count as rollover (D20a): the next day, #901's block
	// isn't an unfinished one.
	clock.now = time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	res, err := s.StartDay(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Recap == nil || res.Recap.RolledOver != 0 || mustGet(t, s, aID).RolloverCount != 0 {
		t.Errorf("rollover after an auto-close: %+v", res.Recap)
	}
}

func TestNoReconcileOnAFailedFetch(t *testing.T) {
	s, clock, src, st := srcService(t)
	a := review(901, "Add idempotency key", clock.now)
	src.Set([]domain.SourceItem{a}, nil)
	mustSyncSources(t, s)
	id := linked(t, st, a, clock.now)
	for _, err := range []error{source.ErrTooMany, errors.New("could not resolve host api.github.com"), source.ErrGHNotFound} {
		clock.Advance(5 * time.Minute)
		src.Set(nil, err)
		if closed := mustSyncSources(t, s); closed != nil {
			t.Errorf("%v: closed %v", err, closed)
		}
		if got := mustGet(t, s, id); got.Status != domain.StatusOpen {
			t.Errorf("%v: an outage closed the review (spec: Error handling)", err)
		}
		if gs := mustLoad(t, s).GitHub; gs.Err != err.Error() {
			t.Errorf("Err = %q; want %q", gs.Err, err)
		}
	}
}

func TestSyncSourcesStopsWhenQuitting(t *testing.T) {
	s, _, src, _ := srcService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	src.Set(nil, context.Canceled)
	if _, err := s.SyncSources(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want context.Canceled", err)
	}
	if gs := mustLoad(t, s).GitHub; gs.Err != "" {
		t.Errorf("quitting isn't GitHub's failure: %q", gs.Err)
	}
}
