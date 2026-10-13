package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFrame(t *testing.T) {
	got := frame("TIMELINE · 2/5 done", []string{"09:00 ▓ Fix CI", "09:30 ▓"}, 24, 5, false)
	want := []string{
		"╭ TIMELINE · 2/5 done ─╮",
		"│ 09:00 ▓ Fix CI       │",
		"│ 09:30 ▓              │",
		"│                      │",
		"╰──────────────────────╯",
	}
	if len(got) != len(want) {
		t.Fatalf("frame has %d lines; want %d", len(got), len(want))
	}
	for i := range want {
		if s := ansi.Strip(got[i]); s != want[i] {
			t.Errorf("line %d = %q; want %q", i, s, want[i])
		}
		if w := ansi.StringWidth(got[i]); w != 24 {
			t.Errorf("line %d is %d cells wide; want 24", i, w)
		}
	}

	cut := frame("A VERY LONG TITLE THAT DOES NOT FIT", []string{"a line far too long for this box", "2", "3", "4"}, 16, 4, true)
	if s := ansi.Strip(cut[0]); s != "╭ A VERY LONG… ╮" {
		t.Errorf("a long title is cut: %q", s)
	}
	if len(cut) != 4 || ansi.Strip(cut[1]) != "│ a line far t…│" || ansi.Strip(cut[2]) != "│ 2            │" || ansi.Strip(cut[3]) != "╰──────────────╯" {
		t.Errorf("long lines are cut and extra lines dropped: %q", stripAll(cut))
	}
	for i, l := range cut {
		if w := ansi.StringWidth(l); w != 16 {
			t.Errorf("line %d is %d cells wide; want 16", i, w)
		}
	}
	if !strings.Contains(cut[0], "\x1b[34m") || strings.Contains(got[0], "\x1b[34m") {
		t.Error("only the focused frame is drawn in the accent colour (ANSI 4)")
	}
	if !strings.Contains(got[0], "\x1b[2m") {
		t.Error("an unfocused frame is dim")
	}
	if frame("x", nil, 3, 5, false) != nil || frame("x", nil, 10, 1, false) != nil {
		t.Error("no room for a frame: nothing drawn")
	}
}

func stripAll(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ansi.Strip(l)
	}
	return out
}

func TestWithinKeepsTheOuterStyleOverInnerStyles(t *testing.T) {
	inner := styleClosed.Render("done task")
	got := within(styleSelected, "a "+inner+" b")
	if want := "\x1b[7ma " + strings.ReplaceAll(inner, "\x1b[m", "\x1b[m\x1b[7m") + " b\x1b[m"; got != want {
		t.Errorf("within = %q; want %q (reverse video again after the inner reset)", got, want)
	}
	if got := within(styleSelected, "plain"); got != styleSelected.Render("plain") {
		t.Errorf("a plain line renders as before: %q", got)
	}
}

func TestCutHints(t *testing.T) {
	hints := "⏎ open · e edit · a add · / filter · g group · p pin · ctrl+z undo · ? help · q quit"
	if got := cutHints(hints, 200); got != hints {
		t.Errorf("a line that fits stays: %q", got)
	}
	got := cutHints(hints, 40)
	if want := "⏎ open · e edit · a add …"; !strings.HasPrefix(got, want) || !strings.HasSuffix(got, " ? help") {
		t.Errorf("cutHints(40) = %q; want whole hints, …, and ? help at the right end", got)
	}
	if w := ansi.StringWidth(got); w != 40 {
		t.Errorf("the cut line is %d cells wide; want 40 (? help at the right edge)", w)
	}
	if got := cutHints("placing \"Fix CI\" · j/k move · ⏎ drop · esc cancel", 30); got != "placing \"Fix CI\" · j/k move …" {
		t.Errorf("without ? help the cut just ends in …: %q", got)
	}
	if got := cutHints(hints, 12); ansi.StringWidth(got) > 12 || !strings.HasSuffix(got, "? help") {
		t.Errorf("a tiny width still ends in ? help: %q", got)
	}
	long := "w reviews · " + hints + " · tab pane · →/← expand · B block today · esc today · i inbox"
	for w := 12; w <= 160; w++ {
		got := cutHints(long, w)
		if gw := ansi.StringWidth(got); gw > w || ansi.StringWidth(long) > w && gw != w {
			t.Errorf("cutHints at %d is %d cells wide: %q", w, gw, got)
		}
		if !strings.HasSuffix(got, "? help") && ansi.StringWidth(long) > w {
			t.Errorf("cutHints at %d lost ? help: %q", w, got)
		}
	}
}
