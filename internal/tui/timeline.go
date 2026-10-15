package tui

import (
	"slices"
	"time"

	"tt/internal/domain"
)

// rowLen is the length of one timeline row (D38).
const rowLen = 30 * time.Minute

// tlRow is one row of Today's timeline.
type tlRow struct {
	start   time.Duration  // wall-clock offset from midnight of the plan's day
	blocks  []domain.Block // the blocks that overlap the row, by start
	slots   []slot         // the meetings that overlap the row, by start (D80)
	outside bool           // a row of its own for a block outside the working hours (D39)
}

// meeting is a busy calendar event (D17a) as offsets on the plan's day.
type meeting struct {
	title      string
	start, end time.Duration
}

// maxCols is how many meeting columns a row draws (D80); a meeting past
// them shares the last one.
const maxCols = 3

// slot is a meeting in its column (D80): the leftmost one free at its start.
// group numbers meetings that overlap one another, directly or through
// others; cols is how many columns the group takes, at most maxCols, so its
// rows line up.
type slot struct {
	meeting
	col, group, cols int
}

// timeline is Today's grid (D38): the working hours in rows (D39), rebuilt
// on every load. A block outside them (placed before D39, or before the
// hours changed) gets a row of its own before or after them, so it never
// vanishes. Rows are wall-clock offsets like stored blocks, so a DST night
// needs no special case: a row that doesn't exist on the clock that night
// is still a row.
type timeline struct {
	rows       []tlRow
	blocks     []domain.Block // the day's blocks, by start
	meetings   []meeting      // the meetings within the working hours, by start (D41)
	slots      []slot         // the same meetings in their columns (D80)
	now        time.Duration  // now's wall-clock offset; -1 when there is no now
	nowRow     int            // the row holding now; -1 when now is outside the hours or there is none
	start, end time.Duration  // the working hours
}

// buildTimeline lays out the working hours from–to in rows, with the
// meetings that touch them; meetings fully outside them aren't shown (D41).
// now < 0 means no now (the shown day isn't today).
func buildTimeline(blocks []domain.Block, meetings []meeting, from, to, now time.Duration) timeline {
	tl := timeline{blocks: blocks, now: max(now, -1), nowRow: -1, start: from, end: to}
	var frees []time.Duration // when each column of the current group frees up
	var groupEnd time.Duration
	var groupCols []int
	for _, mt := range meetings {
		if mt.end <= from || mt.start >= to {
			continue
		}
		tl.meetings = append(tl.meetings, mt)
		if len(tl.slots) == 0 || mt.start >= groupEnd { // overlaps nothing so far: a new group
			frees, groupCols = frees[:0], append(groupCols, 0)
		}
		col := slices.IndexFunc(frees, func(f time.Duration) bool { return f <= mt.start })
		if col < 0 {
			col, frees = len(frees), append(frees, 0)
		}
		frees[col], groupEnd = mt.end, max(groupEnd, mt.end)
		g := len(groupCols) - 1
		groupCols[g] = max(groupCols[g], min(col+1, maxCols))
		tl.slots = append(tl.slots, slot{meeting: mt, col: min(col, maxCols-1), group: g})
	}
	for i := range tl.slots {
		tl.slots[i].cols = groupCols[tl.slots[i].group]
	}
	var before, after []tlRow
	for _, b := range blocks {
		switch {
		case b.End() <= from:
			before = append(before, tlRow{start: b.Start, blocks: []domain.Block{b}, outside: true})
		case b.Start >= to:
			after = append(after, tlRow{start: b.Start, blocks: []domain.Block{b}, outside: true})
		}
	}
	tl.rows = before
	for s := from.Truncate(rowLen); s < to; s += rowLen {
		r := tlRow{start: s}
		for _, b := range blocks {
			// Only the hours count: an edge row of hours off the half-hour
			// grid mustn't also list a block that has an outside row.
			if b.Start < min(s+rowLen, to) && max(s, from) < b.End() {
				r.blocks = append(r.blocks, b)
			}
		}
		for _, sl := range tl.slots {
			if sl.start < min(s+rowLen, to) && max(s, from) < sl.end {
				r.slots = append(r.slots, sl)
			}
		}
		if now >= max(s, from) && now < min(s+rowLen, to) {
			tl.nowRow = len(tl.rows)
		}
		tl.rows = append(tl.rows, r)
	}
	tl.rows = append(tl.rows, after...)
	return tl
}

// outsideNow reports a now outside the working hours.
func (tl *timeline) outsideNow() bool { return tl.now >= 0 && tl.nowRow < 0 }

// rowOf is the index of the row holding offset at, or -1. An outside row
// holds only its block's start.
func (tl *timeline) rowOf(at time.Duration) int {
	for i, r := range tl.rows {
		if r.outside && at == r.start || !r.outside && at >= r.start && at < r.start+rowLen {
			return i
		}
	}
	return -1
}

// homeRow is where the cursor starts: the row holding now, else the last
// working row once the hours are over, else the first.
func (tl *timeline) homeRow() int {
	if tl.nowRow >= 0 {
		return tl.nowRow
	}
	first, last := -1, -1
	for i, r := range tl.rows {
		if !r.outside {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if tl.now >= tl.end {
		return last
	}
	return first
}

// blockAt is the block keys on row i act on: the earliest one starting in it,
// else the one carrying on from above (the row names it, see timelineRow).
func (tl *timeline) blockAt(i int) (domain.Block, bool) {
	if i < 0 || i >= len(tl.rows) || len(tl.rows[i].blocks) == 0 {
		return domain.Block{}, false
	}
	for _, b := range tl.rows[i].blocks {
		if b.Start >= tl.rows[i].start {
			return b, true
		}
	}
	return tl.rows[i].blocks[0], true
}

// gapFrom is the free time from offset at until the next block or meeting
// starts or the working hours end (D42); 0 when a block or a meeting covers
// at, or at is outside the hours.
func (tl *timeline) gapFrom(at time.Duration) time.Duration {
	if at < tl.start || at >= tl.end {
		return 0
	}
	end := tl.end
	busy := func(start, stop time.Duration) bool {
		if start <= at && at < stop {
			return true
		}
		if start > at {
			end = min(end, start)
		}
		return false
	}
	for _, b := range tl.blocks {
		if busy(b.Start, b.End()) {
			return 0
		}
	}
	for _, mt := range tl.meetings {
		if busy(mt.start, mt.end) {
			return 0
		}
	}
	return max(end-at, 0)
}

// firstFree is the first row from i on whose start no block or meeting
// covers; i when there is none.
func (tl *timeline) firstFree(i int) int {
	for j := max(i, 0); j < len(tl.rows); j++ {
		if tl.gapFrom(tl.rows[j].start) > 0 {
			return j
		}
	}
	return i
}

// block is the day's block with id, as last loaded.
func (tl *timeline) block(id int64) (domain.Block, bool) {
	i := slices.IndexFunc(tl.blocks, func(b domain.Block) bool { return b.ID == id })
	if i < 0 {
		return domain.Block{}, false
	}
	return tl.blocks[i], true
}

// groupMeetings are the meetings of the groups that row r's meetings belong
// to, by start (D79).
func (tl *timeline) groupMeetings(r tlRow) []meeting {
	var out []meeting
	for _, sl := range tl.slots {
		if slices.ContainsFunc(r.slots, func(o slot) bool { return o.group == sl.group }) {
			out = append(out, sl.meeting)
		}
	}
	return out
}

// conflict is the first meeting b overlaps (D18, D42).
func (tl *timeline) conflict(b domain.Block) (meeting, bool) {
	for _, mt := range tl.meetings {
		if b.Start < mt.end && mt.start < b.End() {
			return mt, true
		}
	}
	return meeting{}, false
}

// clash is the first block other than b itself (same ID) that b overlaps.
func (tl *timeline) clash(b domain.Block) (domain.Block, bool) {
	for _, o := range tl.blocks {
		if o.ID != b.ID && b.Start < o.End() && o.Start < b.End() {
			return o, true
		}
	}
	return domain.Block{}, false
}
