package tui

import (
	"time"

	"tt/internal/domain"
)

// rowLen is the length of one timeline row (D38).
const rowLen = 30 * time.Minute

// tlRow is one row of Today's timeline.
type tlRow struct {
	start  time.Duration  // wall-clock offset from midnight of the plan's day
	blocks []domain.Block // the blocks that overlap the row, by start
}

// timeline is Today's grid (D38), rebuilt on every load. Rows are
// wall-clock offsets like stored blocks, so a DST night needs no special
// case: a row that doesn't exist on the clock that night is still a row.
type timeline struct {
	rows   []tlRow
	blocks []domain.Block // the day's blocks, by start
	now    time.Duration  // now's wall-clock offset; -1 when there is no now
	nowRow int            // the row holding now; -1 when there is no now
	dayEnd time.Duration  // the planning day's end: day start + 24h
}

// buildTimeline lays out the hours from–to in rows, stretched to include
// every block and now. now < 0 means no now (the shown day isn't today).
func buildTimeline(blocks []domain.Block, from, to, now, dayStart time.Duration) timeline {
	if now >= 0 {
		from, to = min(from, now), max(to, now+time.Minute)
	}
	for _, b := range blocks {
		from, to = min(from, b.Start), max(to, b.End())
	}
	from = from.Truncate(rowLen)
	if to%rowLen != 0 {
		to = to.Truncate(rowLen) + rowLen
	}
	tl := timeline{blocks: blocks, now: max(now, -1), nowRow: -1, dayEnd: dayStart + 24*time.Hour}
	for s := from; s < to; s += rowLen {
		r := tlRow{start: s}
		for _, b := range blocks {
			if b.Start < s+rowLen && s < b.End() {
				r.blocks = append(r.blocks, b)
			}
		}
		if now >= s && now < s+rowLen {
			tl.nowRow = len(tl.rows)
		}
		tl.rows = append(tl.rows, r)
	}
	return tl
}

// rowOf is the index of the row holding offset at, or -1.
func (tl *timeline) rowOf(at time.Duration) int {
	for i, r := range tl.rows {
		if at >= r.start && at < r.start+rowLen {
			return i
		}
	}
	return -1
}

// blockAt is the earliest block in row i; keys on a row act on it.
func (tl *timeline) blockAt(i int) (domain.Block, bool) {
	if i < 0 || i >= len(tl.rows) || len(tl.rows[i].blocks) == 0 {
		return domain.Block{}, false
	}
	return tl.rows[i].blocks[0], true
}

// gapFrom is the free time from offset at until the next block starts or
// the planning day ends; 0 when a block covers at.
func (tl *timeline) gapFrom(at time.Duration) time.Duration {
	end := tl.dayEnd
	for _, b := range tl.blocks {
		if b.Start <= at && at < b.End() {
			return 0
		}
		if b.Start > at {
			end = min(end, b.Start)
		}
	}
	return max(end-at, 0)
}

// firstFree is the first row from i on whose start no block covers; i when
// there is none.
func (tl *timeline) firstFree(i int) int {
	for j := max(i, 0); j < len(tl.rows); j++ {
		if tl.gapFrom(tl.rows[j].start) > 0 {
			return j
		}
	}
	return i
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
