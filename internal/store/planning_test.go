package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"tt/internal/domain"
)

func inTx(t *testing.T, s *Store, fn func(ctx context.Context, x *Tx) error) {
	t.Helper()
	ctx := context.Background()
	if err := s.InTx(ctx, func(x *Tx) error { return fn(ctx, x) }); err != nil {
		t.Fatal(err)
	}
}

func TestPins(t *testing.T) {
	s, _ := openTemp(t)
	id := insert(t, s, sampleTask())
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		if _, ok, _ := x.GetPin(ctx, id); ok {
			t.Error("new task is pinned")
		}
		if err := x.PutPin(ctx, domain.Pin{TaskID: id, PinnedOn: "2026-10-12"}); err != nil {
			return err
		}
		if err := x.PutPin(ctx, domain.Pin{TaskID: id, PinnedOn: "2026-10-13"}); err != nil { // upsert
			return err
		}
		p, ok, err := x.GetPin(ctx, id)
		if err != nil || !ok || p.PinnedOn != "2026-10-13" {
			t.Errorf("GetPin = %+v, %v, %v; want pinned on 2026-10-13", p, ok, err)
		}
		if err := x.DeletePin(ctx, id); err != nil {
			return err
		}
		if _, ok, _ := x.GetPin(ctx, id); ok {
			t.Error("pin survived DeletePin")
		}
		return x.DeletePin(ctx, id) // deleting a missing pin is fine
	})
}

func TestBlocks(t *testing.T) {
	s, _ := openTemp(t)
	id := insert(t, s, sampleTask())
	b := domain.Block{TaskID: id, Day: "2026-10-12", Start: 11 * time.Hour, Duration: 90 * time.Minute}
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		var err error
		if b.ID, err = x.InsertBlock(ctx, b); err != nil {
			return err
		}
		early := domain.Block{TaskID: id, Day: b.Day, Start: 9 * time.Hour, Duration: time.Hour}
		if early.ID, err = x.InsertBlock(ctx, early); err != nil {
			return err
		}
		if _, err := x.InsertBlock(ctx, domain.Block{TaskID: id, Day: "2026-10-13", Start: 9 * time.Hour, Duration: time.Hour}); err != nil {
			return err
		}

		got, err := x.BlocksOn(ctx, b.Day)
		if err != nil {
			return err
		}
		if diff := cmp.Diff([]domain.Block{early, b}, got); diff != "" {
			t.Errorf("BlocksOn (-want +got):\n%s", diff)
		}

		b.Start, b.Duration = 25*time.Hour, 30*time.Minute // after midnight
		if err := x.UpdateBlock(ctx, b); err != nil {
			return err
		}
		if got, _ := x.GetBlock(ctx, b.ID); got != b {
			t.Errorf("after update: %+v; want %+v", got, b)
		}

		if err := x.DeleteBlock(ctx, b.ID); err != nil {
			return err
		}
		if _, err := x.GetBlock(ctx, b.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("GetBlock after delete: %v; want ErrNotFound", err)
		}
		if err := x.DeleteBlock(ctx, b.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("second DeleteBlock: %v; want ErrNotFound", err)
		}

		// Undo restores a block under its old id.
		if restored, err := x.InsertBlock(ctx, b); err != nil || restored != b.ID {
			t.Errorf("InsertBlock with id = %d, %v; want %d", restored, err, b.ID)
		}
		return nil
	})
}

func TestDeletingTaskRemovesItsPlanning(t *testing.T) {
	s, _ := openTemp(t)
	id := insert(t, s, sampleTask())
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		if err := x.PutPin(ctx, domain.Pin{TaskID: id, PinnedOn: "2026-10-12"}); err != nil {
			return err
		}
		if _, err := x.InsertBlock(ctx, domain.Block{TaskID: id, Day: "2026-10-12", Start: 9 * time.Hour, Duration: time.Hour}); err != nil {
			return err
		}
		return x.DeleteTask(ctx, id)
	})
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		if _, ok, _ := x.GetPin(ctx, id); ok {
			t.Error("pin survived task delete")
		}
		if blocks, _ := x.BlocksOn(ctx, "2026-10-12"); len(blocks) != 0 {
			t.Errorf("blocks survived task delete: %v", blocks)
		}
		return nil
	})
}

func TestDays(t *testing.T) {
	s, _ := openTemp(t)
	at := time.Date(2026, 10, 12, 4, 0, 0, 0, time.UTC)
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		if _, ok, _ := x.LatestDayBefore(ctx, "2026-10-12"); ok {
			t.Error("LatestDayBefore on empty days found something")
		}
		for _, d := range []domain.Day{"2026-10-09", "2026-10-12"} {
			if err := x.InsertDay(ctx, d, at); err != nil {
				return err
			}
		}
		if err := x.InsertDay(ctx, "2026-10-12", at); err == nil {
			t.Error("InsertDay twice for one day succeeded; the PK must guard StartDay races")
		}
		if ok, _ := x.HasDay(ctx, "2026-10-12"); !ok {
			t.Error("HasDay(2026-10-12) = false")
		}
		if d, ok, _ := x.LatestDayBefore(ctx, "2026-10-12"); !ok || d != "2026-10-09" {
			t.Errorf("LatestDayBefore = %q, %v; want 2026-10-09 (skips the weekend)", d, ok)
		}
		if err := x.CommitDay(ctx, "2026-10-12", at); err != nil {
			return err
		}
		if err := x.CommitDay(ctx, "2026-10-13", at); !errors.Is(err, ErrNotFound) {
			t.Errorf("CommitDay on a missing day: %v; want ErrNotFound", err)
		}
		return nil
	})
}

func TestDayQueries(t *testing.T) {
	s, _ := openTemp(t)
	closedAt := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	open := insert(t, s, sampleTask())
	done := sampleTask()
	done.Status, done.ClosedAt, done.ClosedBy = domain.StatusDone, closedAt, domain.ClosedByUser
	doneID := insert(t, s, done)
	dropped := sampleTask()
	dropped.Status, dropped.ClosedAt, dropped.ClosedBy = domain.StatusDropped, closedAt, domain.ClosedByUser
	droppedID := insert(t, s, dropped)

	inTx(t, s, func(ctx context.Context, x *Tx) error {
		for _, id := range []int64{open, doneID, droppedID} {
			if err := x.PutPin(ctx, domain.Pin{TaskID: id, PinnedOn: "2026-10-12"}); err != nil {
				return err
			}
			if _, err := x.InsertBlock(ctx, domain.Block{TaskID: id, Day: "2026-10-12", Start: 9 * time.Hour, Duration: time.Hour}); err != nil {
				return err
			}
		}
		if ids, _ := x.OpenTasksBlockedOn(ctx, "2026-10-12"); !cmp.Equal(ids, []int64{open}) {
			t.Errorf("OpenTasksBlockedOn = %v; want [%d]", ids, open)
		}
		from, to := closedAt.Add(-time.Hour), closedAt.Add(time.Hour)
		if n, _ := x.CountDone(ctx, from, to); n != 1 {
			t.Errorf("CountDone in window = %d; want 1 (dropped doesn't count)", n)
		}
		if n, _ := x.CountDone(ctx, to, to.Add(time.Hour)); n != 0 {
			t.Errorf("CountDone outside window = %d; want 0", n)
		}
		if n, _ := x.CountDone(ctx, closedAt, to); n != 1 {
			t.Errorf("CountDone with from == closed_at = %d; want 1 (from is inclusive)", n)
		}
		if err := x.DeletePinsOfClosedTasks(ctx); err != nil {
			return err
		}
		if n, _ := x.CountOpenPins(ctx); n != 1 {
			t.Errorf("CountOpenPins = %d; want 1", n)
		}
		if _, ok, _ := x.GetPin(ctx, doneID); ok {
			t.Error("done task still pinned")
		}
		return nil
	})
}

func TestLoadPlan(t *testing.T) {
	s, _ := openTemp(t)
	a := insert(t, s, sampleTask())
	b := insert(t, s, sampleTask())
	at := time.Date(2026, 10, 12, 4, 0, 0, 0, time.UTC)
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		for _, d := range []domain.Day{"2026-10-08", "2026-10-09", "2026-10-12"} {
			if err := x.InsertDay(ctx, d, at); err != nil {
				return err
			}
		}
		if err := x.PutPin(ctx, domain.Pin{TaskID: a, PinnedOn: "2026-10-09"}); err != nil { // Fri, Mon → 2
			return err
		}
		if err := x.PutPin(ctx, domain.Pin{TaskID: b, PinnedOn: "2026-10-12"}); err != nil { // today → 1
			return err
		}
		if _, err := x.InsertBlock(ctx, domain.Block{TaskID: b, Day: "2026-10-12", Start: 10 * time.Hour, Duration: time.Hour}); err != nil {
			return err
		}
		return x.CommitDay(ctx, "2026-10-12", at)
	})

	snap, state, err := s.LoadPlan(context.Background(), "2026-10-12")
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Tasks()) != 2 {
		t.Errorf("snapshot has %d tasks; want 2", len(snap.Tasks()))
	}
	want := []PinState{
		{Pin: domain.Pin{TaskID: a, PinnedOn: "2026-10-09"}, PinnedDays: 2},
		{Pin: domain.Pin{TaskID: b, PinnedOn: "2026-10-12"}, PinnedDays: 1},
	}
	if diff := cmp.Diff(want, state.Pins); diff != "" {
		t.Errorf("pins (-want +got):\n%s", diff)
	}
	if len(state.Blocks) != 1 || state.Blocks[0].TaskID != b || !state.Committed {
		t.Errorf("state = %+v", state)
	}
	if _, state, _ := s.LoadPlan(context.Background(), "2026-10-13"); state.Committed || len(state.Blocks) != 0 {
		t.Errorf("unplanned day state = %+v", state)
	}
}
