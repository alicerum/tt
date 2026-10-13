package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"tt/internal/domain"
)

func TestSourceItems(t *testing.T) {
	ctx := context.Background()
	s, path := openTemp(t)
	if items, err := s.SourceItems(ctx, "github"); err != nil || len(items) != 0 {
		t.Fatalf("before the first fetch: %v, %v", items, err)
	}
	seen := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	first := []domain.SourceItem{
		{Kind: "github", ExternalID: "acme/photos#877", Number: 877, Title: "Bump go", URL: "https://github.com/acme/photos/pull/877",
			Author: "tom", Repo: "acme/photos", FirstSeenAt: seen}, // no request time
		{Kind: "github", ExternalID: "acme/api#901", Number: 901, Title: "Add idempotency key", URL: "https://github.com/acme/api/pull/901",
			Author: "ana", Repo: "acme/api", RequestedAt: seen.Add(-72 * time.Hour), FirstSeenAt: seen},
	}
	inTx(t, s, func(ctx context.Context, x *Tx) error { return x.ReplaceSourceItems(ctx, "github", first) })
	inTx(t, s, func(ctx context.Context, x *Tx) error { // another kind's set is its own
		return x.ReplaceSourceItems(ctx, "other", []domain.SourceItem{{ExternalID: "x", Title: "x", FirstSeenAt: seen}})
	})
	s.Close()

	s, err := Open(ctx, path) // kept across restarts
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	items, err := s.SourceItems(ctx, "github")
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.SourceItem{first[1], first[0]} // by external id
	if diff := cmp.Diff(want, items); diff != "" {
		t.Errorf("stored items (-want +got):\n%s", diff)
	}

	inTx(t, s, func(ctx context.Context, x *Tx) error { // the next fetch replaces the set
		return x.ReplaceSourceItems(ctx, "github", first[:1])
	})
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		items, err := x.SourceItems(ctx, "github")
		if err != nil {
			return err
		}
		if len(items) != 1 || items[0].ExternalID != "acme/photos#877" || !items[0].RequestedAt.IsZero() {
			t.Errorf("after the replace: %+v", items)
		}
		other, err := x.SourceItems(ctx, "other")
		if err != nil {
			return err
		}
		if len(other) != 1 {
			t.Errorf("the other kind's items: %+v", other)
		}
		return nil
	})
}

func TestTasksBySource(t *testing.T) {
	s, _ := openTemp(t)
	plain := insert(t, s, sampleTask())
	review := sampleTask()
	review.Source = &domain.SourceRef{Kind: "github", ID: "acme/api#901"}
	reviewID := insert(t, s, review)
	closed := sampleTask()
	closed.Source = &domain.SourceRef{Kind: "github", ID: "acme/api#3"}
	closed.Status, closed.ClosedAt, closed.ClosedBy = domain.StatusDone, closed.CreatedAt, domain.ClosedBySource
	closedID := insert(t, s, closed)
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		got, ok, err := x.TaskBySource(ctx, domain.SourceRef{Kind: "github", ID: "acme/api#901"})
		if err != nil || !ok || got.ID != reviewID || len(got.Tags) != 2 {
			t.Errorf("TaskBySource = %+v, %v, %v; want #%d with its tags", got, ok, err, reviewID)
		}
		if got, ok, err := x.TaskBySource(ctx, domain.SourceRef{Kind: "github", ID: "acme/api#3"}); err != nil || !ok || got.ID != closedID {
			t.Errorf("a closed task is found too: %+v, %v, %v", got, ok, err)
		}
		if _, ok, err := x.TaskBySource(ctx, domain.SourceRef{Kind: "github", ID: "acme/api#902"}); err != nil || ok {
			t.Errorf("no task for #902: %v, %v", ok, err)
		}
		open, err := x.OpenSourceTasks(ctx, "github")
		if err != nil {
			return err
		}
		if len(open) != 1 || open[0].ID != reviewID {
			t.Errorf("OpenSourceTasks = %+v; want only #%d (not #%d plain, not #%d closed)", open, reviewID, plain, closedID)
		}
		return nil
	})
}
