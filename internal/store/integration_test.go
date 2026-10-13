package store

import (
	"context"
	"testing"
	"time"
)

func TestIntegrationState(t *testing.T) {
	ctx := context.Background()
	s, path := openTemp(t)
	if in, ok, err := s.Integration(ctx, "calendar"); err != nil || ok || in.Name != "calendar" || !in.LastSuccessAt.IsZero() {
		t.Fatalf("before the first fetch: %+v, %v, %v", in, ok, err)
	}
	ok1 := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		return x.PutIntegration(ctx, Integration{Name: "calendar", LastSuccessAt: ok1, Payload: `{"day":"2026-10-12"}`})
	})
	failed := ok1.Add(2 * time.Minute)
	inTx(t, s, func(ctx context.Context, x *Tx) error {
		in, ok, err := x.GetIntegration(ctx, "calendar")
		if err != nil || !ok {
			t.Fatalf("GetIntegration: %v, %v", ok, err)
		}
		in.LastError, in.LastErrorAt = "no access", failed
		return x.PutIntegration(ctx, in) // replaces the row
	})
	s.Close()

	s, err := Open(ctx, path) // kept across restarts
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	in, ok, err := s.Integration(ctx, "calendar")
	if err != nil || !ok {
		t.Fatalf("Integration: %v, %v", ok, err)
	}
	if !in.LastSuccessAt.Equal(ok1) || in.LastError != "no access" || !in.LastErrorAt.Equal(failed) || in.Payload != `{"day":"2026-10-12"}` {
		t.Errorf("stored state = %+v", in)
	}
	if _, ok, _ := s.Integration(ctx, "github"); ok {
		t.Error("each integration has a row of its own")
	}
}
