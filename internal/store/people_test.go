package store

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"tt/internal/domain"
)

func TestTasksWithRequester(t *testing.T) {
	s, _ := openTemp(t)
	task := func(requester string, status domain.Status, parent int64) int64 {
		x := sampleTask()
		x.Requester, x.Status, x.ParentID = requester, status, parent
		return insert(t, s, x)
	}
	open := task("ana", domain.StatusOpen, 0)
	task("anabel", domain.StatusOpen, 0)
	task("", domain.StatusOpen, 0)
	done := task("ana", domain.StatusDone, 0)
	parent := task("bob", domain.StatusOpen, 0)
	child := task("ana", domain.StatusDropped, parent)

	inTx(t, s, func(ctx context.Context, x *Tx) error {
		ids, err := x.TasksWithRequester(ctx, "ana")
		if err != nil {
			return err
		}
		if want := []int64{open, done, child}; !cmp.Equal(ids, want) {
			t.Errorf("ana's tasks = %v; want %v (open and closed, children too; never anabel's)", ids, want)
		}
		none, err := x.TasksWithRequester(ctx, "raff")
		if err != nil {
			return err
		}
		if len(none) != 0 {
			t.Errorf("nobody called raff: %v", none)
		}
		return nil
	})
}
