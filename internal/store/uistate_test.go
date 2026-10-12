package store

import (
	"context"
	"testing"
)

func TestUIState(t *testing.T) {
	ctx := context.Background()
	s, path := openTemp(t)
	if _, ok, err := s.UIState(ctx, "pick.group"); err != nil || ok {
		t.Fatalf("missing key: ok=%v err=%v", ok, err)
	}
	for _, v := range []string{"size", "requester"} {
		if err := s.InTx(ctx, func(x *Tx) error { return x.PutUIState(ctx, "pick.group", v) }); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()

	s, err := Open(ctx, path) // remembered across restarts
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v, ok, err := s.UIState(ctx, "pick.group"); err != nil || !ok || v != "requester" {
		t.Errorf("UIState = %q, %v, %v; want the last value", v, ok, err)
	}
}
