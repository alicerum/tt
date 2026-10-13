package tui

import (
	"context"
	"testing"

	"tt/internal/config"
)

func TestTabSkipsAnEmptyPaneInTheLog(t *testing.T) {
	h := newLogHarness(t, config.Default())
	ctx := context.Background()
	a := h.add("Alpha !1 ~s")
	h.keys("tab")
	h.mustSee("nothing pinned yet · p pins a task")
	if h.m.pick.focus != paneBacklog {
		t.Fatal("tab into an empty candidates pane keeps the focus on the backlog")
	}

	h.keys("p") // Alpha is a candidate now: its reload emptied the backlog
	if h.m.pick.focus != paneToday {
		t.Fatal("a reload that empties the focused pane moves the focus to the other one")
	}
	h.keys("tab")
	h.mustSee("backlog is empty")
	if h.m.pick.focus != paneToday {
		t.Error("tab into an empty backlog keeps the focus on the candidates")
	}

	h.add("Beta !2 ~s")
	if err := h.m.applyFilter("zzz"); err != nil {
		t.Fatal(err)
	}
	h.keys("tab")
	h.mustSee("no matches · esc clears") // a filter matching nothing says so
	if err := h.m.applyFilter(""); err != nil {
		t.Fatal(err)
	}

	if _, err := h.svc.Unpin(ctx, a.ID); err != nil { // another shell empties the focused pane
		t.Fatal(err)
	}
	h.keys("R")
	if h.m.pick.focus != paneBacklog {
		t.Error("a reload that empties the focused pane moves the focus to the other one")
	}
}

func TestTabSkipsAnEmptyPaneOnToday(t *testing.T) {
	h := newHarness(t, config.Default())
	h.keys("tab")
	h.mustSee("nothing pinned · L opens the log to pick")
	if h.m.today.focus != focusTimeline {
		t.Fatal("tab into empty candidates keeps the focus on the timeline")
	}
	a := h.pinned("Alpha !1 ~s")
	h.keys("tab")
	if h.m.today.focus != focusCandidates {
		t.Fatal("tab moves to the candidates once there are some")
	}
	if _, err := h.svc.Unpin(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	if h.m.today.focus != focusTimeline {
		t.Error("the last candidate gone: the focus goes back to the timeline")
	}
}

func TestTabSkipsAnEmptyPaneInPeople(t *testing.T) {
	h := newHarness(t, config.Default())
	h.keys("P", "tab")
	h.mustSee("nobody listed")
	if h.m.people.onTasks {
		t.Fatal("tab with nobody listed keeps the focus on the names")
	}
	a := h.add("Ask @ana !1 ~s")
	h.keys("tab")
	if !h.m.people.onTasks {
		t.Fatal("tab moves to ana's tasks")
	}
	if err := h.svc.Delete(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	if h.m.people.onTasks {
		t.Error("ana's last task gone: the focus goes back to the (empty) names, not an empty tasks pane")
	}
	h.mustSee("nobody yet")
}

func TestARefocusLeavesScreensNotOnShowAlone(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.add("Alpha !1 ~s")
	h.keys("L") // the Log's focus is on the backlog
	h.keys("esc")
	if _, err := h.svc.Pin(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R") // the backlog emptied while Today was on show
	if h.m.pick.focus != paneBacklog {
		t.Error("the Log's focus only moves while the Log is on show")
	}
}

func TestTypingAFilterKeepsTheFocusThroughAReload(t *testing.T) {
	h := newLogHarness(t, config.Default())
	a := h.add("Alpha !1 ~s")
	h.pinned("Beta !2 ~s")
	if h.m.pick.focus != paneBacklog {
		t.Fatal("the focus starts on the backlog")
	}
	h.keys("/")
	if _, err := h.svc.Pin(context.Background(), a.ID); err != nil { // another shell empties the backlog
		t.Fatal(err)
	}
	h.run(h.m.load())
	if len(h.m.pick.rows[paneBacklog]) != 0 {
		t.Fatal("the reload emptied the backlog")
	}
	if h.m.pick.focus != paneBacklog {
		t.Error("a reload while the filter is typed leaves the focus alone")
	}
}
