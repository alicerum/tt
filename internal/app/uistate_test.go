package app

import (
	"context"
	"testing"
)

func TestUIState(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	if v, err := s.UIState(ctx, "pick.group"); err != nil || v != "" {
		t.Fatalf("unset UIState = %q, %v; want empty", v, err)
	}
	if err := s.SetUIState(ctx, "pick.group", "size"); err != nil {
		t.Fatal(err)
	}
	if v, err := s.UIState(ctx, "pick.group"); err != nil || v != "size" {
		t.Errorf("UIState = %q, %v; want size", v, err)
	}
}
