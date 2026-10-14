package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"tt/internal/config"
	"tt/internal/domain"
)

// notifyCfg is the default config with n as [notify]; no alerts given
// means the default [5, 0].
func notifyCfg(n config.Notify) config.Config {
	cfg := config.Default()
	if n.Alerts == nil {
		n.Alerts = []int{5, 0}
	}
	cfg.Notify = n
	return cfg
}

// tickAt moves the clock to off on the harness's day and runs the watch tick.
func (h *harness) tickAt(off time.Duration) {
	h.t.Helper()
	h.clock.now = dayAt(off)
	h.send(watchTickMsg{})
}

// relaunch starts a new Model on the same database at the clock's time,
// like quitting tt and starting it again.
func (h *harness) relaunch() {
	h.t.Helper()
	h.m = New(context.Background(), h.svc, h.m.opts)
	h.run(h.m.Init())
	h.send(tea.WindowSizeMsg{Width: 140, Height: 40})
}

func (h *harness) wantNotes(want ...note) {
	h.t.Helper()
	if diff := cmp.Diff(want, h.notes, cmp.AllowUnexported(note{})); diff != "" {
		h.t.Fatalf("notifications (-want +got):\n%s", diff)
	}
}

func TestMeetingAndBlockAlerts(t *testing.T) {
	h := newCalHarness(t, notifyCfg(config.Notify{Meetings: true, Blocks: true}))
	rfc := h.pinned("Write RFC !1 ~s")
	h.block(rfc.ID, 10*time.Hour, time.Hour)
	h.meetings(ev("Standup", 10*time.Hour, 10*time.Hour+15*time.Minute))
	if !h.m.View().ReportFocus {
		t.Error("with a kind on, tt asks for focus reports (D66)")
	}

	h.tickAt(9*time.Hour + 54*time.Minute)
	h.wantNotes()
	h.tickAt(9*time.Hour + 55*time.Minute) // never focused: notifies (D66)
	soon := note{"tt · in 5 min", "Standup · Write RFC"}
	h.wantNotes(soon)
	h.tickAt(9*time.Hour + 55*time.Minute + 30*time.Second)
	h.wantNotes(soon) // once
	h.tickAt(10 * time.Hour)
	now := note{"tt · now", "Standup · Write RFC"}
	h.wantNotes(soon, now)
	h.tickAt(10*time.Hour + time.Minute)
	h.wantNotes(soon, now)
}

func TestAlertsOnlyForMomentsPassedWhileRunning(t *testing.T) {
	h := newHarness(t, notifyCfg(config.Notify{Blocks: true}))
	early := h.pinned("Early !1 ~s")
	h.block(early.ID, 8*time.Hour+30*time.Minute, 30*time.Minute) // started before launch
	h.tickAt(9*time.Hour + 20*time.Second)
	h.wantNotes()

	h.clock.now = dayAt(9*time.Hour + 27*time.Minute)
	late := h.pinned("Late !1 ~s")
	h.block(late.ID, 9*time.Hour+30*time.Minute, 30*time.Minute) // its 5-minute moment passed already
	moved := h.pinned("Moved !1 ~s")
	b := h.block(moved.ID, 10*time.Hour, 30*time.Minute)
	if _, err := h.svc.MoveBlock(context.Background(), b.ID, 11*time.Hour); err != nil {
		t.Fatal(err)
	}
	done := h.pinned("Done !1 ~s")
	h.block(done.ID, 12*time.Hour, 30*time.Minute)
	if _, err := h.svc.Done(context.Background(), done.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")

	h.tickAt(9*time.Hour + 28*time.Minute)
	h.wantNotes()
	h.tickAt(9*time.Hour + 30*time.Minute)
	h.wantNotes(note{"tt · now", "Late"})
	h.tickAt(9*time.Hour + 55*time.Minute) // Moved's old moment
	h.tickAt(10 * time.Hour)
	h.wantNotes(note{"tt · now", "Late"})
	h.tickAt(10*time.Hour + 55*time.Minute)
	h.tickAt(11*time.Hour + 55*time.Minute) // Done's block: a closed task's
	h.tickAt(12 * time.Hour)
	h.wantNotes(note{"tt · now", "Late"}, note{"tt · in 5 min", "Moved"})

	// Asleep from 12:00 to 13:58: the moments it slept through stay quiet.
	again := h.pinned("Again !1 ~s")
	h.block(again.ID, 13*time.Hour, 30*time.Minute)
	h.tickAt(13*time.Hour + 58*time.Minute)
	h.wantNotes(note{"tt · now", "Late"}, note{"tt · in 5 min", "Moved"})

	// Quit and start again at 15:55:30: the 15:55 moment was before launch.
	last := h.pinned("Last !1 ~s")
	h.block(last.ID, 16*time.Hour, 30*time.Minute)
	h.clock.now = dayAt(15*time.Hour + 55*time.Minute + 30*time.Second)
	h.relaunch()
	h.tickAt(15*time.Hour + 55*time.Minute + 40*time.Second)
	h.tickAt(16 * time.Hour)
	h.wantNotes(note{"tt · now", "Late"}, note{"tt · in 5 min", "Moved"}, note{"tt · now", "Last"})
}

func TestNoAlertsWhileFocused(t *testing.T) {
	h := newHarness(t, notifyCfg(config.Notify{Blocks: true}))
	rfc := h.pinned("Write RFC !1 ~s")
	h.block(rfc.ID, 10*time.Hour, time.Hour)
	h.send(tea.FocusMsg{})
	h.tickAt(9*time.Hour + 55*time.Minute)
	h.wantNotes() // on the screen already
	h.send(tea.BlurMsg{})
	h.tickAt(9*time.Hour + 56*time.Minute)
	h.wantNotes() // dropped, not kept for later
	h.tickAt(10 * time.Hour)
	h.wantNotes(note{"tt · now", "Write RFC"})
}

func TestNotificationsAreOffByDefault(t *testing.T) {
	h := newCalHarness(t, config.Default())
	rfc := h.pinned("Write RFC !1 ~s")
	h.block(rfc.ID, 10*time.Hour, time.Hour)
	h.meetings(ev("Standup", 10*time.Hour, 10*time.Hour+15*time.Minute))
	h.tickAt(9*time.Hour + 55*time.Minute)
	h.tickAt(10 * time.Hour)
	if h.m.View().ReportFocus {
		t.Error("every kind off: no focus reports")
	}

	g := newGHHarness(t, config.Default())
	g.clock.now = g.clock.now.Add(time.Minute)
	g.setReviews(g.reviewItem("acme/api", 12, "Add retries", "ana", time.Hour))
	h.notes = append(h.notes, g.notes...)
	h.wantNotes()
}

func TestNewReviewRequestsNotify(t *testing.T) {
	h := newGHHarness(t, notifyCfg(config.Notify{Reviews: true}))
	a := h.reviewItem("acme/api", 12, "Add retries", "ana", time.Hour)
	b := h.reviewItem("acme/web", 3, "Fix login", "bo", 2*time.Hour)
	c := h.reviewItem("acme/web", 4, "Bump deps", "bo", 3*time.Hour)
	later := func() { h.clock.now = h.clock.now.Add(time.Minute) }

	later()
	h.setReviews(a) // the startup fetch saw none
	one := note{"tt · review requested", "@ana acme/api#12 Add retries"}
	h.wantNotes(one)
	later()
	h.setReviews(a)
	h.wantNotes(one)
	later()
	h.src.Set(nil, errors.New("offline"))
	h.keys("R")
	h.wantNotes(one) // a failed fetch keeps the last good set
	later()
	h.setReviews(a, b, c)
	two := note{"tt · 2 review requests", "Fix login · Bump deps"}
	h.wantNotes(one, two)
	later()
	h.setReviews(b, c)
	later()
	h.setReviews(a, b, c) // re-requested: new again (D65)
	h.wantNotes(one, two, one)

	// What arrived while tt was closed: the first fetch since launch is silent.
	d := h.reviewItem("acme/api", 13, "Drop v1", "ana", time.Minute)
	later()
	h.src.Set([]domain.SourceItem{a, b, c, d}, nil)
	h.relaunch()
	later()
	h.setReviews(a, b, c, d)
	h.wantNotes(one, two, one)
}

func TestAFailedSendSaysSoOnce(t *testing.T) {
	h := newHarness(t, notifyCfg(config.Notify{Blocks: true}))
	h.notifyErr = errors.New("exec: \"osascript\": executable file not found in $PATH")
	rfc := h.pinned("Write RFC !1 ~s")
	h.block(rfc.ID, 10*time.Hour, time.Hour)
	h.tickAt(9*time.Hour + 55*time.Minute)
	h.mustSee(`can't notify: exec: "osascript": executable file not found`)
	h.m.setStatus("")
	h.tickAt(10 * time.Hour)
	if len(h.notes) != 2 || h.m.status != "" {
		t.Errorf("the second failure: %d sends, status %q; want it tried and not reported again", len(h.notes), h.m.status)
	}
}
