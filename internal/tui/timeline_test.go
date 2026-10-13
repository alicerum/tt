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
	tl := buildTimeline(nil, nil, 8*hr, 19*hr, 10*hr+12*mn)
	if got := starts(tl); len(got) != 22 || got[0] != 8*hr || got[21] != 18*hr+30*mn {
		t.Fatalf("working hours: %d rows %v…; want 22 rows 08:00…18:30", len(got), got[:1])
	}
	if tl.nowRow != 4 || tl.homeRow() != 4 || tl.outsideNow() { // 10:00
		t.Errorf("nowRow = %d, homeRow %d; want 4 (the 10:00 row)", tl.nowRow, tl.homeRow())
	}

	// D39: only the working hours. An early now doesn't stretch them; a
	// block outside them (placed before D39) gets a row of its own after them.
	late := domain.Block{ID: 1, TaskID: 1, Start: 21*hr + 10*mn, Duration: 40 * mn}
	early := domain.Block{ID: 2, TaskID: 2, Start: 6 * hr, Duration: hr}
	tl = buildTimeline([]domain.Block{early, late}, nil, 8*hr, 19*hr, 6*hr+45*mn)
	got := starts(tl)
	if len(got) != 24 || got[0] != 6*hr || got[1] != 8*hr || got[22] != 18*hr+30*mn || got[23] != late.Start {
		t.Errorf("rows %v; want 06:00 (outside), 08:00…18:30, 21:10 (outside)", got)
	}
	if !tl.rows[0].outside || !tl.rows[23].outside || tl.rows[1].outside {
		t.Error("only the blocks outside the hours get outside rows")
	}
	if !tl.outsideNow() || tl.nowRow != -1 || tl.homeRow() != 1 {
		t.Errorf("now 06:45 is outside: nowRow %d, homeRow %d; want -1 and the 08:00 row", tl.nowRow, tl.homeRow())
	}
	if tl.rowOf(late.Start) != 23 || tl.rowOf(late.Start+mn) != -1 {
		t.Error("an outside row holds only its block's start")
	}
	if g := tl.gapFrom(7 * hr); g != 0 {
		t.Errorf("gapFrom outside the hours = %v; want 0", g)
	}

	// After the hours (e.g. past midnight, still the same planning day): the
	// cursor rests on the last working row.
	tl = buildTimeline(nil, nil, 8*hr, 19*hr, 24*hr+20*mn)
	if !tl.outsideNow() || tl.homeRow() != 21 {
		t.Errorf("after hours: outsideNow %v, homeRow %d; want true, 21", tl.outsideNow(), tl.homeRow())
	}

	// Hours off the half-hour grid: an edge row doesn't also list a block
	// that has an outside row.
	edge := domain.Block{ID: 3, TaskID: 3, Start: 7*hr + 30*mn, Duration: 15 * mn} // 07:30–07:45
	tl = buildTimeline([]domain.Block{edge}, nil, 7*hr+45*mn, 19*hr, -1)
	if !tl.rows[0].outside || tl.rows[1].start != 7*hr+30*mn || len(tl.rows[1].blocks) != 0 {
		t.Errorf("hours from 07:45: rows[0] outside %v, rows[1] %v with %d blocks; want the block only in its outside row",
			tl.rows[0].outside, tl.rows[1].start, len(tl.rows[1].blocks))
	}

	// No now (a stale day).
	tl = buildTimeline(nil, nil, 8*hr, 19*hr, -1)
	if len(tl.rows) != 22 || tl.nowRow != -1 || tl.outsideNow() {
		t.Errorf("without now: %d rows, nowRow %d", len(tl.rows), tl.nowRow)
	}
}

func TestTimelineRowsAndGaps(t *testing.T) {
	a := domain.Block{ID: 1, TaskID: 10, Start: 9 * hr, Duration: 90 * mn}        // 09:00–10:30
	b := domain.Block{ID: 2, TaskID: 11, Start: 11*hr + 10*mn, Duration: 15 * mn} // 11:10–11:25
	c := domain.Block{ID: 3, TaskID: 12, Start: 11*hr + 25*mn, Duration: 5 * mn}  // 11:25–11:30
	tl := buildTimeline([]domain.Block{a, b, c}, nil, 8*hr, 19*hr, 8*hr)

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
	tl2 := buildTimeline([]domain.Block{a2, b2}, nil, 8*hr, 19*hr, 8*hr)
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
		{8 * hr, hr},                  // until a starts
		{9*hr + 30*mn, 0},             // inside a
		{10*hr + 30*mn, 40 * mn},      // until b
		{11*hr + 30*mn, 7*hr + 30*mn}, // until the working hours end (19:00)
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

func TestTimelineMeetings(t *testing.T) {
	ms := []meeting{
		{"Early", 7 * hr, 7*hr + 45*mn}, // fully before the hours: not shown (D41)
		{"Breakfast", 7*hr + 30*mn, 8*hr + 30*mn},
		{"Standup", 9*hr + 30*mn, 9*hr + 45*mn},
		{"Review", 10 * hr, 11 * hr},
		{"Sync", 10*hr + 30*mn, 11*hr + 15*mn}, // overlaps Review: one band (P4)
		{"Late", 18*hr + 45*mn, 19*hr + 30*mn},
		{"After", 19*hr + 30*mn, 20 * hr},
	}
	tl := buildTimeline(nil, ms, 8*hr, 19*hr, 9*hr)
	if len(tl.meetings) != 5 || tl.meetings[0].title != "Breakfast" || tl.meetings[4].title != "Late" {
		t.Fatalf("meetings within the hours: %+v", tl.meetings)
	}
	bandsAt := func(at time.Duration) []band { return tl.rows[tl.rowOf(at)].bands }
	if b := bandsAt(8 * hr); len(b) != 1 || b[0].title != "Breakfast" {
		t.Errorf("08:00: %+v", b)
	}
	if b := bandsAt(8*hr + 30*mn); len(b) != 0 {
		t.Errorf("08:30 is free: %+v", b)
	}
	want := band{title: "Review", more: 1, start: 10 * hr, end: 11*hr + 15*mn}
	for _, at := range []time.Duration{10 * hr, 10*hr + 30*mn, 11 * hr} {
		if b := bandsAt(at); len(b) != 1 || b[0] != want {
			t.Errorf("%v: %+v; want the merged band %+v", at, b, want)
		}
	}
	if b := bandsAt(18*hr + 30*mn); len(b) != 1 || b[0].title != "Late" {
		t.Errorf("18:30: %+v", b)
	}

	// D42: meetings are busy for placement.
	for _, c := range []struct{ at, want time.Duration }{
		{8 * hr, 0},                   // inside Breakfast
		{8*hr + 30*mn, hr},            // until Standup
		{9*hr + 30*mn, 0},             // inside Standup
		{9*hr + 45*mn, 15 * mn},       // until Review
		{11 * hr, 0},                  // inside Sync, which runs on after Review
		{11*hr + 15*mn, 7*hr + 30*mn}, // until Late
		{18*hr + 45*mn, 0},            // inside Late, which runs past the hours
		{19 * hr, 0},                  // outside the hours
	} {
		if got := tl.gapFrom(c.at); got != c.want {
			t.Errorf("gapFrom(%s) = %v; want %v", clock(c.at), got, c.want)
		}
	}
	if i := tl.firstFree(tl.rowOf(8 * hr)); tl.rows[i].start != 8*hr+30*mn {
		t.Errorf("firstFree from 08:00 = %s; want 08:30 (Breakfast is busy)", clock(tl.rows[i].start))
	}
	if mt, ok := tl.conflict(domain.Block{Start: 10*hr + 45*mn, Duration: 45 * mn}); !ok || mt.title != "Review" {
		t.Errorf("conflict = %+v, %v; want Review, the first one by start", mt, ok)
	}
	if _, ok := tl.conflict(domain.Block{Start: 9*hr + 45*mn, Duration: 15 * mn}); ok {
		t.Error("a block between meetings, touching both, doesn't conflict")
	}
}
