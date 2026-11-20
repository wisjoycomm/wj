package main

import "time"

// The overlay draws the same chart as wj usage, small: one column per day, week or
// month up to the current one, which is coloured; and under each project a bar of
// its share of the period's total.

// overlaySeries is the token per period of the overlay's kind (today: days, week:
// weeks, month: months), ending with the current one. Empty periods before the first
// with calls are dropped, keeping at least 4.
func (s *sessionScanner) overlaySeries(window string, now time.Time) []int64 {
	p, err := resolvePeriod(window, now)
	if err != nil {
		return nil
	}
	u := unitOf(p)
	if u == unitNone {
		return nil
	}
	first, n := chartWindow(u, p.Start, now, 0)
	return trimLeadingEmpty(s.unitSeries(u, first, n, "", ""), 4)
}

func trimLeadingEmpty(series []int64, keep int) []int64 {
	skip := 0
	for skip < len(series)-keep && series[skip] == 0 {
		skip++
	}
	return series[skip:]
}

// columnHeights scales series to at most maxHeight pixels; a period with calls is
// never shorter than 1 pixel.
func columnHeights(series []int64, maxHeight int) []int {
	var top int64
	for _, x := range series {
		top = max(top, x)
	}
	heights := make([]int, len(series))
	if top == 0 {
		return heights
	}
	for i, x := range series {
		if x > 0 {
			heights[i] = max(1, int(x*int64(maxHeight)/top))
		}
	}
	return heights
}

// composeColumns draws series as columns in the box x, y, width x height on a base
// line; the last column, the current period, gets current, the others past.
func composeColumns(pix []byte, imgWidth, imgHeight, x, y, width, height, gap int, series []int64, past, current, base [3]byte) {
	n := len(series)
	if n == 0 || width <= 0 {
		return
	}
	composeRule(pix, imgWidth, imgHeight, x, x+width, y+height-1, 1, base)
	column := width / n
	bar := max(1, column-gap)
	offset := x + (width-column*n)/2
	for i, h := range columnHeights(series, height-1) {
		colour := past
		if i == n-1 {
			colour = current
		}
		left := offset + i*column
		fillRect(pix, imgWidth, imgHeight, left, y+height-1-h, left+bar, y+height-1, colour)
	}
}
