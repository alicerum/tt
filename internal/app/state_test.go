package app

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestLoadReadsVersionSnapshotAndPlan(t *testing.T) {
	s, _ := startedService(t)
	ctx := context.Background()
	a := mustAdd(t, s, "A !1 ~s")
	mustBlock(t, s, a.ID, 10*h, h)

	st, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Snapshot.Get(a.ID); !ok {
		t.Error("snapshot misses the task")
	}
	if diff := cmp.Diff(mustPlan(t, s), st.Plan); diff != "" {
		t.Errorf("Load's plan differs from Plan() (-Plan +Load):\n%s", diff)
	}

	mustAdd(t, s, "B")
	again, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again.Version == st.Version {
		t.Error("Version did not change after a write")
	}
	if len(again.Snapshot.Tasks()) != 2 {
		t.Errorf("snapshot has %d tasks; want 2", len(again.Snapshot.Tasks()))
	}
}

func TestBlockUndoAfterUnblockConflicts(t *testing.T) {
	s, _ := startedService(t)
	ctx := context.Background()
	a := mustAdd(t, s, "A !1 ~s")
	b, blockUndo := mustBlock(t, s, a.ID, 10*h, h)
	if _, err := s.Unblock(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := blockUndo(ctx); !errors.Is(err, ErrConflict) {
		t.Errorf("undo Block after the block is gone: %v; want ErrConflict", err)
	}
}
