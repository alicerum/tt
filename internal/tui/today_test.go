package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"tt/internal/config"
	"tt/internal/domain"
)

// block places task id on today's timeline through the service and reloads.
func (h *harness) block(id int64, start, dur time.Duration) domain.Block {
	h.t.Helper()
	b, _, err := h.svc.Block(context.Background(), id, start, dur)
	if err != nil {
		h.t.Fatal(err)
	}
	h.keys("R")
	return b
}

func TestTodayIsTheHomeScreen(t *testing.T) {
	h := newHarness(t, config.Default())
	h.mustSee("tt · Today", "TIMELINE", "CANDIDATES", " 08:00 ", " 18:30 ", "━━ now 09:00", "nothing pinned · L opens the log")
	h.mustNotSee(" 19:00 ")

	h.keys("L")
	h.mustSee("tt · Log", "BACKLOG")
	h.keys("esc")
	h.mustSee("tt · Today")

	// esc in the Log clears a filter first, then goes back to Today.
	h.add("Alpha !1 ~s")
	h.keys("L", "/")
	h.typeText("zzz")
	h.keys("enter", "esc")
	h.mustSee("tt · Log", "filter cleared")
	h.keys("esc")
	h.mustSee("tt · Today")

	// B ends Pick: Today with the candidates focused.
	h.keys("L", "p", "B")
	h.mustSee("tt · Today", "Alpha")
	if h.m.today.focus != focusCandidates {
		t.Error("B focuses the candidates")
	}

	// The Inbox returns to where it was opened from.
	h.add("Untriaged")
	h.keys("i")
	h.mustSee("tt · Inbox")
	h.keys("esc")
	h.mustSee("tt · Today")
	h.keys("L", "i", "esc")
	h.mustSee("tt · Log")
}

func TestTimelineShowsBlocks(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.add("Alpha !1 ~s")
	b := h.add("Beta !2 ~m")
	h.block(a.ID, 10*time.Hour, time.Hour)
	h.block(b.ID, 11*time.Hour+10*time.Minute, 45*time.Minute)
	h.mustSee("TIMELINE · 0/2 done", " 10:00 ▓ Alpha [P1 S]", " 10:30 ▓ ", " 11:00 ▓ 11:10–11:55 Beta [P2 M]", " 11:30 ▓ ")
	h.mustSee("CANDIDATES", "[P1 S] Alpha ▓") // blocking pins (D12a)

	if _, err := h.svc.Done(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	h.keys("R")
	h.mustSee("TIMELINE · 1/2 done", " 10:00 ▓ Alpha [P1 S] ✓") // P3: done blocks stay
}

func TestTimelineStretchesToBlocksAndNow(t *testing.T) {
	h := newHarness(t, config.Default())
	late := h.add("Late one !1 ~s")
	h.block(late.ID, 21*time.Hour, time.Hour)
	h.mustSee(" 21:00 ▓ Late one", " 21:30 ▓ ")
	h.mustNotSee(" 22:00 ")

	h.clock.now = time.Date(2026, 10, 12, 6, 40, 0, 0, time.UTC)
	h.keys("R")
	h.mustSee(" 06:30 ", "━━ now 06:40")
	h.mustNotSee(" 06:00 ")
}

func TestTimelineCursorAndBlockKeys(t *testing.T) {
	h := newHarness(t, config.Default())
	a := h.add("Alpha !1 ~s")
	h.block(a.ID, 10*time.Hour, time.Hour)
	if got := h.m.today.at; got != 9*time.Hour {
		t.Fatalf("the cursor starts on the now row: %v", got)
	}
	h.keys("x")
	h.mustSee("nothing blocked here")

	h.keys("j", "j", "enter") // 10:00
	h.mustSee("tt · Detail", "#1 Alpha")
	h.keys("esc")
	h.mustSee("tt · Today")
	if got := h.m.today.at; got != 10*time.Hour {
		t.Errorf("the cursor stays on its row: %v", got)
	}
	h.keys("j", "x") // 10:30 is still Alpha's block
	h.mustSee(`✓ "Alpha"`, "1/1 done")

	// The candidates pane: its own cursor and keys.
	h.keys("tab", "enter")
	h.mustSee("tt · Detail", "#1 Alpha")
	h.keys("esc", "x")
	h.mustSee(`reopened "Alpha"`)
}

func TestPinningAParentInTheTUIWithChildren(t *testing.T) {
	h := newLogHarness(t, config.Default())
	ctx := context.Background()
	parent, _, err := h.svc.Add(ctx, "Parent !1 ~xl")
	if err != nil {
		t.Fatal(err)
	}
	var kids []domain.Task
	for _, line := range []string{"A ~s", "B ~s"} {
		k, _, err := h.svc.AddChild(ctx, parent.ID, line)
		if err != nil {
			t.Fatal(err)
		}
		kids = append(kids, k)
	}
	h.keys("R", "p")
	h.mustSee(`pinned "Parent" + 2 children · no undo, u unpins`)

	h.keys("tab", "j") // on child A in today's candidates
	h.keys("u")
	h.mustSee("pinned with its parent #1 · unpin the parent first")

	h.block(kids[1].ID, 10*time.Hour, time.Hour)
	h.keys("k", "u")
	h.mustSee(`unpinned "Parent" + 1 child · 1 blocked child kept · no undo, p pins`)
	if _, ok := h.m.idx.cand[kids[1].ID]; !ok {
		t.Error("the blocked child stays pinned")
	}
}

func TestScreensShareTheHeightBudget(t *testing.T) {
	h := newHarness(t, config.Default())
	for i := range 8 {
		h.add("Task " + string(rune('A'+i)) + " !1 ~s")
	}
	h.add("Untriaged @ana #x https://example.com -- a note")
	for _, height := range []int{6, 10, 16} {
		h.send(tea.WindowSizeMsg{Width: 100, Height: height})
		for _, keys := range [][]string{{}, {"L"}, {"i"}} {
			h.keys(keys...)
			if n := strings.Count(h.screen(), "\n") + 1; n > height {
				t.Errorf("height %d, after %v: %d lines\n%s", height, keys, n, h.screen())
			}
			h.keys("esc")
		}
	}

	// The timeline scrolls to keep its cursor visible.
	h.send(tea.WindowSizeMsg{Width: 100, Height: 10})
	for range 20 {
		h.keys("j")
	}
	h.mustSee(" 18:30 ")
	h.mustNotSee(" 09:00 ")
	for range 30 {
		h.keys("k")
	}
	h.mustSee(" 04:00 ") // k past the window's start grows it, down to the day start
}

func TestTimelineRowsAcrossADSTGap(t *testing.T) {
	ams, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.DayStart, cfg.TimelineStart, cfg.TimelineEnd = 0, 0, 6*time.Hour
	h := newHarness(t, cfg)
	h.clock.now = time.Date(2026, 3, 29, 3, 40, 0, 0, ams) // 02:00–03:00 doesn't exist this night
	h.keys("R")
	h.mustSee("Sun 29 Mar", " 02:00 ", " 02:30 ", " 03:30 ", "━━ now 03:40")
	if got := h.m.today.tl.rows[h.m.today.tl.nowRow].start; got != 3*time.Hour+30*time.Minute {
		t.Errorf("now row = %v; want 03:30 by the wall clock", got)
	}
}
