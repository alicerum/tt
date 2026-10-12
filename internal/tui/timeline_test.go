package tui

import (
	"testing"
	"time"

	"tt/internal/domain"
)

const (
	hr = time.Hour
	mn = time.Minute
)

func starts(tl timeline) []time.Duration {
	var out []time.Duration
	for _, r := range tl.rows {
		out = append(out, r.start)
	}
	return out
}

func TestTimelineWindow(t *testing.T) {
	const dayStart = 4 * hr
	tl := buildTimeline(nil, 8*hr, 19*hr, 10*hr+12*mn, dayStart)
	if got := starts(tl); len(got) != 22 || got[0] != 8*hr || got[21] != 18*hr+30*mn {
		t.Fatalf("default window: %d rows %v…; want 22 rows 08:00…18:30", len(got), got[:1])
	}
	if tl.nowRow != 4 { // 10:00
		t.Errorf("nowRow = %d; want 4 (the 10:00 row)", tl.nowRow)
	}

	// The window stretches to an early now and a late, off-grid block.
	late := domain.Block{ID: 1, TaskID: 1, Start: 21*hr + 10*mn, Duration: 40 * mn}
	tl = buildTimeline([]domain.Block{late}, 8*hr, 19*hr, 6*hr+45*mn, dayStart)
	got := starts(tl)
	if got[0] != 6*hr+30*mn || got[len(got)-1] != 21*hr+30*mn {
		t.Errorf("stretched window %v–%v; want 06:30–21:30 rows", got[0], got[len(got)-1])
	}
	if tl.nowRow != 0 {
		t.Errorf("nowRow = %d; want 0", tl.nowRow)
	}

	// No now (a stale day): no stretching for it, no now row.
	tl = buildTimeline(nil, 8*hr, 19*hr, -1, dayStart)
	if len(tl.rows) != 22 || tl.nowRow != -1 {
		t.Errorf("without now: %d rows, nowRow %d", len(tl.rows), tl.nowRow)
	}
}

func TestTimelineRowsAndGaps(t *testing.T) {
	a := domain.Block{ID: 1, TaskID: 10, Start: 9 * hr, Duration: 90 * mn}        // 09:00–10:30
	b := domain.Block{ID: 2, TaskID: 11, Start: 11*hr + 10*mn, Duration: 15 * mn} // 11:10–11:25
	c := domain.Block{ID: 3, TaskID: 12, Start: 11*hr + 25*mn, Duration: 5 * mn}  // 11:25–11:30
	tl := buildTimeline([]domain.Block{a, b, c}, 8*hr, 19*hr, 8*hr, 4*hr)

	at := func(off time.Duration) int { return tl.rowOf(off) }
	if blk, ok := tl.blockAt(at(9*hr + 30*mn)); !ok || blk.ID != a.ID {
		t.Errorf("09:30 row: %+v %v; want block 1", blk, ok)
	}
	if blk, ok := tl.blockAt(at(11 * hr)); !ok || blk.ID != b.ID || len(tl.rows[at(11*hr)].blocks) != 2 {
		t.Errorf("11:00 row holds two short blocks, keys act on the earlier: %+v", tl.rows[at(11*hr)].blocks)
	}
	if _, ok := tl.blockAt(at(10*hr + 30*mn)); ok {
		t.Error("10:30 row is free")
	}

	// A row's keys act on the block the row names: a block carried on from
	// an earlier row loses to one starting in it.
	a2 := domain.Block{ID: 1, TaskID: 10, Start: 10 * hr, Duration: 40 * mn}       // 10:00–10:40
	b2 := domain.Block{ID: 2, TaskID: 11, Start: 10*hr + 40*mn, Duration: 20 * mn} // 10:40–11:00
	tl2 := buildTimeline([]domain.Block{a2, b2}, 8*hr, 19*hr, 8*hr, 4*hr)
	if blk, ok := tl2.blockAt(tl2.rowOf(10*hr + 30*mn)); !ok || blk.ID != b2.ID {
		t.Errorf("10:30 row names B (it starts at 10:40 in this row; A only carries on): got %+v %v", blk, ok)
	}
	if blk, ok := tl2.blockAt(tl2.rowOf(10 * hr)); !ok || blk.ID != a2.ID {
		t.Errorf("10:00 row names A: got %+v %v", blk, ok)
	}
	if tl.rowOf(7*hr) != -1 {
		t.Error("07:00 is outside the rows")
	}

	cases := []struct {
		at, want time.Duration
	}{
		{8 * hr, hr},                   // until a starts
		{9*hr + 30*mn, 0},              // inside a
		{10*hr + 30*mn, 40 * mn},       // until b
		{11*hr + 30*mn, 16*hr + 30*mn}, // until the planning day ends (04:00 next date)
	}
	for _, c := range cases {
		if got := tl.gapFrom(c.at); got != c.want {
			t.Errorf("gapFrom(%v) = %v; want %v", c.at, got, c.want)
		}
	}

	if got := tl.firstFree(at(9 * hr)); got != at(10*hr+30*mn) {
		t.Errorf("firstFree(09:00 row) = row %d; want the 10:30 row %d", got, at(10*hr+30*mn))
	}
	if blk, ok := tl.clash(domain.Block{ID: 1, Start: 10 * hr, Duration: 90 * mn}); !ok || blk.ID != b.ID {
		t.Errorf("a moved to 10:00–11:30 clashes with b, not itself: %+v %v", blk, ok)
	}
	if _, ok := tl.clash(domain.Block{Start: 12 * hr, Duration: hr}); ok {
		t.Error("12:00–13:00 is free")
	}
}
