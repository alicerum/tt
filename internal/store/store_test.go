package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"tt/internal/domain"
)

// openTemp opens a store in a directory whose name contains a space,
// like ~/Library/Application Support (Review Focus).
func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Application Support", "tt.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func sampleTask() domain.Task {
	now := time.Date(2026, 10, 12, 9, 30, 0, 123, time.UTC)
	return domain.Task{
		Title: "Review PR #482", Note: "line 1\nline 2", Link: "https://github.com/a/b/pull/482",
		Requester: "ivan", Tags: []string{"impl", "photos"}, Priority: 2, Impact: 2, Due: "2026-10-16", Size: domain.SizeM,
		Status: domain.StatusOpen, CreatedAt: now, UpdatedAt: now,
	}
}

func insert(t *testing.T, s *Store, task domain.Task) int64 {
	t.Helper()
	var id int64
	err := s.InTx(context.Background(), func(x *Tx) (err error) {
		id, err = x.InsertTask(context.Background(), task)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func get(t *testing.T, s *Store, id int64) (domain.Task, error) {
	t.Helper()
	var task domain.Task
	err := s.InTx(context.Background(), func(x *Tx) (err error) {
		task, err = x.GetTask(context.Background(), id)
		return err
	})
	return task, err
}

func TestOpenTwiceKeepsData(t *testing.T) {
	s, path := openTemp(t)
	id := insert(t, s, sampleTask())
	s.Close()

	again, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("reopen (migrations must be idempotent): %v", err)
	}
	defer again.Close()
	if _, err := get(t, again, id); err != nil {
		t.Fatal(err)
	}
}

func TestTaskRoundTrip(t *testing.T) {
	s, _ := openTemp(t)
	want := sampleTask()
	want.ID = insert(t, s, want)

	got, err := get(t, s, want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("round trip (-want +got):\n%s", diff)
	}

	want.Status, want.ClosedBy, want.DropReason = domain.StatusDropped, domain.ClosedByUser, "handed to @marko"
	want.ClosedAt = want.CreatedAt.Add(time.Hour)
	want.Tags = []string{"org"}
	want.Priority, want.Impact, want.Due, want.Size = 0, 0, "", domain.SizeNone
	want.Source = &domain.SourceRef{Kind: "github", ID: "a/b#482"}
	err = s.InTx(context.Background(), func(x *Tx) error { return x.UpdateTask(context.Background(), want) })
	if err != nil {
		t.Fatal(err)
	}
	got, _ = get(t, s, want.ID)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("after update (-want +got):\n%s", diff)
	}
}

func TestMissingTask(t *testing.T) {
	s, _ := openTemp(t)
	if _, err := get(t, s, 42); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetTask error = %v; want ErrNotFound", err)
	}
	err := s.InTx(context.Background(), func(x *Tx) error { return x.DeleteTask(context.Background(), 42) })
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteTask error = %v; want ErrNotFound", err)
	}
}

func TestDeleteParentOrphansChildren(t *testing.T) {
	s, _ := openTemp(t)
	parentID := insert(t, s, sampleTask())
	child := sampleTask()
	child.ParentID = parentID
	childID := insert(t, s, child)

	ctx := context.Background()
	err := s.InTx(ctx, func(x *Tx) error {
		if has, _ := x.HasChildren(ctx, parentID); !has {
			t.Error("HasChildren(parent) = false; want true")
		}
		if pos, _ := x.NextChildPosition(ctx, parentID); pos != 1 {
			t.Errorf("NextChildPosition = %d; want 1", pos)
		}
		return x.DeleteTask(ctx, parentID)
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := get(t, s, childID)
	if err != nil || got.ParentID != 0 {
		t.Errorf("child after parent delete: %+v, %v; want ParentID 0", got, err)
	}
}

func TestLoadSnapshot(t *testing.T) {
	s, _ := openTemp(t)
	a := insert(t, s, sampleTask())
	untagged := sampleTask()
	untagged.Tags = nil
	b := insert(t, s, untagged)

	snap, err := s.LoadSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Tasks()) != 2 {
		t.Fatalf("snapshot has %d tasks; want 2", len(snap.Tasks()))
	}
	ta, _ := snap.Get(a)
	tb, _ := snap.Get(b)
	if diff := cmp.Diff([]string{"impl", "photos"}, ta.Tags); diff != "" {
		t.Errorf("tags of a (-want +got):\n%s", diff)
	}
	if len(tb.Tags) != 0 {
		t.Errorf("tags of b = %v; want none", tb.Tags)
	}
}

func TestDataVersionSeesOtherProcessWrites(t *testing.T) {
	tui, path := openTemp(t)
	cli, err := Open(context.Background(), path) // a second process, e.g. `tt add`
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()

	ctx := context.Background()
	before, err := tui.DataVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := tui.DataVersion(ctx); again != before {
		t.Errorf("DataVersion changed without writes: %d → %d", before, again)
	}
	insert(t, cli, sampleTask())
	if after, _ := tui.DataVersion(ctx); after == before {
		t.Error("DataVersion did not change after another connection committed")
	}
}

func TestConcurrentWritersWaitInsteadOfFailing(t *testing.T) {
	a, path := openTemp(t)
	b, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	ctx := context.Background()
	holding := make(chan struct{})
	done := make(chan error)
	go func() {
		done <- a.InTx(ctx, func(x *Tx) error {
			if _, err := x.InsertTask(ctx, sampleTask()); err != nil {
				return err
			}
			close(holding)
			time.Sleep(300 * time.Millisecond) // hold the write lock
			return nil
		})
	}()
	<-holding
	insert(t, b, sampleTask()) // must wait for a, not fail with SQLITE_BUSY
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	snap, _ := b.LoadSnapshot(ctx)
	if n := len(snap.Tasks()); n != 2 {
		t.Errorf("got %d tasks; want 2", n)
	}
}

func TestFormatTimeSortsAsText(t *testing.T) {
	tm := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	if a, b := formatTime(tm), formatTime(tm.Add(500*time.Millisecond)); !(a < b) {
		t.Errorf("formatTime not text-sortable: %q !< %q", a, b)
	}
	got, err := parseTime(formatTime(tm))
	if err != nil || !got.Equal(tm) {
		t.Errorf("parseTime(formatTime(t)) = %v, %v; want %v", got, err, tm)
	}
}
