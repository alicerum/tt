package tui

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/config"
)

func TestEditLine(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.add("Old title @ana #a !1 ~s https://x.io -- keep me")

	h.keys("e")
	h.mustSee("edit #1 › Old title @ana #a !1 ~s https://x.io")
	h.typeText(" #b @bob")
	h.keys("enter")
	h.mustSee(`edited "Old title"`)
	if got := h.task(a.ID); got.Requester != "bob" || !slices.Equal(got.Tags, []string{"a", "b"}) || got.Note != "keep me" {
		t.Errorf("edited: %+v", got)
	}
	h.keys("ctrl+z")
	h.mustSee(`undid edit "Old title"`)
	if got := h.task(a.ID); got.Requester != "ana" {
		t.Errorf("after undo: %+v", got)
	}

	// A bad line keeps the overlay and the text.
	h.keys("e")
	h.typeText(" !9")
	h.keys("enter")
	if _, ok := h.m.overlay.(*editLine); !ok {
		t.Fatal("a bad edit must keep the overlay open")
	}
	h.mustSee("edit #1 › Old title @ana #a !1 ~s https://x.io !9", "✗ priority !9 out of range")
	h.keys("esc")
	if got := h.task(a.ID); got.Priority != 1 {
		t.Errorf("a refused edit changed the task: %+v", got)
	}
}

func TestEditLineOfAChildRefusesPriority(t *testing.T) {
	h := newHarness(t, config.Default())
	parent, kids := h.family("Parent !1 ~l", "Child ~s")
	h.keys("enter", "j", "e")
	h.mustSee("edit #2 › Child ~s")
	h.typeText(" !2")
	h.keys("enter")
	h.mustSee("✗ invalid value: children inherit priority from #1; drop !2")
	if _, ok := h.m.overlay.(*editLine); !ok {
		t.Fatal("the overlay stays open")
	}
	h.keys("esc")
	if got := h.task(kids[0].ID); got.Priority != 0 || got.ParentID != parent.ID {
		t.Errorf("child: %+v", got)
	}
}

func TestNoteInTheEditor(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.add("Task !1 ~s -- first line")
	var seen string
	h.editor = func(path string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		seen = string(data)
		return os.WriteFile(path, []byte("rewritten\nwith two lines\n\n"), 0o600)
	}
	h.clock.now = h.clock.now.Add(time.Minute)
	h.keys("E")
	if seen != "first line\n" {
		t.Errorf("the editor got %q", seen)
	}
	h.mustSee(`note saved`)
	if got := h.task(a.ID).Note; got != "rewritten\nwith two lines" {
		t.Errorf("note = %q", got)
	}
	h.keys("ctrl+z")
	h.mustSee(`undid note of "Task"`)
	if got := h.task(a.ID).Note; got != "first line" {
		t.Errorf("after undo: %q", got)
	}

	// Unchanged file → no write.
	before := h.task(a.ID).UpdatedAt
	h.editor = func(string) error { return nil }
	h.keys("E")
	h.mustSee("note unchanged")
	if !h.task(a.ID).UpdatedAt.Equal(before) {
		t.Error("an unchanged note must not be written")
	}

	// Empty file → note cleared.
	h.clock.now = h.clock.now.Add(time.Minute)
	h.editor = func(path string) error { return os.WriteFile(path, nil, 0o600) }
	h.keys("E")
	h.mustSee("note cleared")
	if got := h.task(a.ID).Note; got != "" {
		t.Errorf("cleared note = %q", got)
	}

	// The editor fails after typing → not saved, but the text stays in the file.
	var kept string
	h.editor = func(path string) error {
		kept = path
		os.WriteFile(path, []byte("half-typed"), 0o600)
		return errors.New("exit status 1")
	}
	h.keys("E")
	h.mustSee("✗ your text is in "+kept, "editor failed (exit status 1); note unchanged")
	if got := h.task(a.ID).Note; got != "" {
		t.Errorf("a failed editor changed the note: %q", got)
	}
	if data, err := os.ReadFile(kept); err != nil || string(data) != "half-typed" {
		t.Errorf("the typing must survive in %s: %q, %v", kept, data, err)
	}
	os.Remove(kept)

	// The editor fails without changing the file → removed.
	h.editor = func(path string) error {
		kept = path
		return errors.New("exit status 1")
	}
	h.keys("E")
	h.mustSee("✗ editor failed (exit status 1); note unchanged")
	if _, err := os.Stat(kept); !os.IsNotExist(err) {
		t.Errorf("%s left behind: %v", kept, err)
	}
}

func TestNoteChangedElsewhereKeepsYourText(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.add("Task !1 ~s -- original")
	var path string
	h.editor = func(p string) error {
		path = p
		h.clock.now = h.clock.now.Add(time.Minute)
		if _, err := h.svc.SetNote(context.Background(), a.ID, "from another shell"); err != nil {
			return err
		}
		return os.WriteFile(p, []byte("my careful edit\n"), 0o600)
	}
	h.keys("E")
	h.mustSee("✗ your text is kept in ")
	if got := h.task(a.ID).Note; got != "from another shell" {
		t.Errorf("note overwritten: %q", got)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "my careful edit\n" {
		t.Errorf("the edit must survive in %s: %q, %v", path, data, err)
	}
	os.Remove(path)
	if !strings.Contains(h.screen(), path) {
		t.Errorf("the status names the file:\n%s", h.screen())
	}
}

// On a narrow pane the status is cut at the right edge; the path comes first
// so the part the user needs survives the cut (D29a).
func TestNoteConflictPathFitsANarrowPane(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.add("Task !1 ~s -- original")
	h.send(tea.WindowSizeMsg{Width: 100, Height: 40})
	var path string
	h.editor = func(p string) error {
		path = p
		h.clock.now = h.clock.now.Add(time.Minute)
		if _, err := h.svc.SetNote(context.Background(), a.ID, "from another shell"); err != nil {
			return err
		}
		return os.WriteFile(p, []byte("my careful edit\n"), 0o600)
	}
	h.keys("E")
	h.mustSee("your text is kept in " + path)
	os.Remove(path)
}

func TestEditorTempFilesAreRemoved(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("Task !1 ~s -- x")
	var paths []string
	h.editor = func(p string) error {
		paths = append(paths, p)
		return os.WriteFile(p, []byte("y\n"), 0o600)
	}
	h.keys("E")
	h.editor = func(p string) error { paths = append(paths, p); return nil }
	h.keys("E")
	for _, p := range paths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s left behind: %v", p, err)
		}
	}
	if len(paths) != 2 || !strings.Contains(paths[0], "tt-note-1-") || !strings.HasSuffix(paths[0], ".md") {
		t.Errorf("note files: %v", paths)
	}
}

func TestOpenLink(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("With link !1 ~s https://x.io/pr/482")
	h.add("Without !2 ~s")
	h.keys("o")
	h.mustSee("opened https://x.io/pr/482")
	h.keys("j", "o")
	h.mustSee("no link")
	h.openErr = errors.New("no browser")
	h.keys("k", "o")
	h.mustSee("✗ can't open https://x.io/pr/482: no browser")
	if !slices.Equal(h.opened, []string{"https://x.io/pr/482", "https://x.io/pr/482"}) {
		t.Errorf("opened: %v", h.opened)
	}
}

func TestEditorCommand(t *testing.T) {
	env := func(kv ...string) func(string) string {
		m := map[string]string{}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return func(k string) string { return m[k] }
	}
	cases := []struct {
		getenv func(string) string
		want   []string
	}{
		{env("VISUAL", "code --wait", "EDITOR", "nano"), []string{"code", "--wait", "/tmp/n.md"}},
		{env("VISUAL", "  ", "EDITOR", "nvim"), []string{"nvim", "/tmp/n.md"}},
		{env(), []string{"vi", "/tmp/n.md"}},
	}
	for _, c := range cases {
		if got := editorCommand(c.getenv, "/tmp/n.md").Args; !slices.Equal(got, c.want) {
			t.Errorf("editorCommand = %v; want %v", got, c.want)
		}
	}
}

// Task detail points at E for writing or reading the whole note.
func TestDetailNoteHints(t *testing.T) {
	h := newHarness(t, config.Default())
	h.add("No note !1 ~s")
	h.add("Long note !2 ~s -- " + strings.Repeat("line\n", 7) + "last")
	h.keys("enter")
	h.mustSee("no note · E to write one")
	h.keys("esc", "j", "enter")
	h.mustSee("… +2 lines · E edit in $EDITOR")
}
