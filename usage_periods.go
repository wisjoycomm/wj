package main

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

// The interactive `wj usage` screen works in three kinds of period: a day, a week
// (Monday to Sunday) and a month. ← and → step to the period before or after, and the
// chart shows the token of the periods around the one being viewed.

type periodUnit int

const (
	unitNone periodUnit = iota // a period typed by hand: 7d, 2026-11-01..2026-11-09
	unitDay
	unitWeek
	unitMonth
)

// periodUnits are the kinds of period, in the order of the keys 1-3.
var periodUnits = []periodUnit{unitDay, unitWeek, unitMonth}

func (u periodUnit) command() string {
	return [...]string{"", "day", "week", "month"}[u]
}

func (u periodUnit) name() string {
	return [...]string{"", "ngày", "tuần", "tháng"}[u]
}

// unitOf tells which kind of period p is: one day, one Monday-to-Sunday week, one
// calendar month, or none of these.
func unitOf(p usagePeriod) periodUnit {
	switch {
	case p.singleDay():
		return unitDay
	case p.Start.Weekday() == time.Monday && p.Start.AddDate(0, 0, 7).Equal(p.End):
		return unitWeek
	case p.Start.Day() == 1 && p.Start.AddDate(0, 1, 0).Equal(p.End):
		return unitMonth
	}
	return unitNone
}

// unitStart is the first day of the period of kind u that holds day.
func unitStart(u periodUnit, day time.Time) time.Time {
	day = midnight(day)
	switch u {
	case unitWeek:
		return weekStart(day)
	case unitMonth:
		return time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location())
	}
	return day
}

// unitStep moves start by n periods of kind u.
func unitStep(u periodUnit, start time.Time, n int) time.Time {
	switch u {
	case unitWeek:
		return start.AddDate(0, 0, 7*n)
	case unitMonth:
		return start.AddDate(0, n, 0)
	}
	return start.AddDate(0, 0, n)
}

// unitSpec is the command for the period of kind u that starts at start. Periods near
// now get their short names (today, lastweek...), older ones a date.
func unitSpec(u periodUnit, start, now time.Time) string {
	today := midnight(now)
	switch u {
	case unitWeek:
		switch this := weekStart(today); {
		case start.Equal(this):
			return "week"
		case start.Equal(this.AddDate(0, 0, -7)):
			return "lastweek"
		}
		year, week := start.ISOWeek()
		return fmt.Sprintf("%d-W%02d", year, week)
	case unitMonth:
		switch this := unitStart(unitMonth, today); {
		case start.Equal(this):
			return "month"
		case start.Equal(this.AddDate(0, -1, 0)):
			return "lastmonth"
		}
		return start.Format("2006-01")
	}
	switch {
	case start.Equal(today):
		return "today"
	case start.Equal(today.AddDate(0, 0, -1)):
		return "yesterday"
	}
	return start.Format("2006-01-02")
}

// anchorDay is the day a change of kind keeps: today when the period holds it, else
// its first day.
func anchorDay(p usagePeriod, now time.Time) time.Time {
	if today := midnight(now); !today.Before(p.Start) && today.Before(p.End) {
		return today
	}
	return p.Start
}

// specOfUnit is the period of kind u that holds the anchor day of the view.
func specOfUnit(spec string, u periodUnit, now time.Time) (string, error) {
	p, err := resolvePeriod(spec, now)
	if err != nil {
		return "", err
	}
	return unitSpec(u, unitStart(u, anchorDay(p, now)), now), nil
}

// shiftSpec is the period dir steps before (dir < 0) or after the one typed: the day,
// week or month next to it, or a range of the same length. ok is false when that
// period would start after today.
func shiftSpec(spec string, dir int, now time.Time) (string, bool) {
	p, err := resolvePeriod(spec, now)
	if err != nil {
		return "", false
	}
	tomorrow := midnight(now).AddDate(0, 0, 1)
	if u := unitOf(p); u != unitNone {
		start := unitStep(u, p.Start, dir)
		if !start.Before(tomorrow) {
			return "", false
		}
		return unitSpec(u, start, now), true
	}
	n := dayCount(p.Start, p.End)
	start := p.Start.AddDate(0, 0, dir*n)
	if !start.Before(tomorrow) {
		return "", false
	}
	return start.Format("2006-01-02") + ".." + start.AddDate(0, 0, n-1).Format("2006-01-02"), true
}

// relativeName says how far the period is from now, for the periods that have a name.
func relativeName(u periodUnit, start, now time.Time) string {
	return map[string]string{
		"today": "hôm nay", "yesterday": "hôm qua", "week": "tuần này", "lastweek": "tuần trước",
		"month": "tháng này", "lastmonth": "tháng trước",
	}[unitSpec(u, start, now)]
}

// chartColumn is the width of one bar of the chart, gap included.
func chartColumn(u periodUnit) int {
	if u == unitMonth {
		return 9
	}
	return 8
}

const chartHeight = 6

// chartWindow is the first period of the chart and how many there are: as many as fit
// width, ending with the current one, or around the viewed one when it is older.
func chartWindow(u periodUnit, viewed, now time.Time, width int) (time.Time, int) {
	limit := map[periodUnit]int{unitDay: 31, unitWeek: 26, unitMonth: 24}[u]
	n := map[periodUnit]int{unitDay: 14, unitWeek: 12, unitMonth: 12}[u]
	if width > 0 {
		n = min(limit, max(4, (width-2)/chartColumn(u)))
	}
	last := unitStart(u, now)
	first := unitStep(u, last, -(n - 1))
	if viewed.Before(first) {
		first = unitStep(u, viewed, -(n / 2))
	}
	return first, n
}

// bucketIndex is the number of periods of kind u from first to start.
func bucketIndex(u periodUnit, first, start time.Time) int {
	switch u {
	case unitWeek:
		return dayCount(first, start) / 7
	case unitMonth:
		return (start.Year()-first.Year())*12 + int(start.Month()) - int(first.Month())
	}
	return dayCount(first, start)
}

// unitSeries adds up the token of n periods of kind u from first, in one pass.
func (s *sessionScanner) unitSeries(u periodUnit, first time.Time, n int, project, client string) []int64 {
	series := make([]int64, n)
	p := usagePeriod{Start: first, End: unitStep(u, first, n)}
	for _, call := range s.calls {
		if !callInPeriod(call, p, project, client) {
			continue
		}
		i := bucketIndex(u, first, unitStart(u, call.When.In(first.Location())))
		if i >= 0 && i < n {
			series[i] += call.Input + call.Output + call.CacheRead + call.CacheWrite
		}
	}
	return series
}

func chartLabel(u periodUnit, start time.Time) string {
	switch u {
	case unitWeek:
		_, week := start.ISOWeek()
		return fmt.Sprintf("W%02d", week)
	case unitMonth:
		return start.Format("01/2006")
	}
	return start.Format("02/01")
}

func center(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	left := (width - n) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", width-n-left)
}

// printChart draws the token of the periods around the viewed one as bars, the viewed
// one marked ▲ under its label.
func (s *sessionScanner) printChart(out io.Writer, v usageView, p usagePeriod, now time.Time) {
	u := unitOf(p)
	if u == unitNone {
		return
	}
	first, n := chartWindow(u, p.Start, now, v.Width)
	s.scan(first)
	series := s.unitSeries(u, first, n, v.Project, v.Client)
	// Drop the empty periods before the first one with calls (the session files of
	// Claude Code do not reach back that far), keeping the viewed one and at least 4.
	skip := 0
	for skip < len(series) && series[skip] == 0 {
		skip++
	}
	if skip = min(skip, bucketIndex(u, first, p.Start), n-4); skip > 0 {
		series, first, n = series[skip:], unitStep(u, first, skip), n-skip
	}
	var top int64
	for _, x := range series {
		top = max(top, x)
	}
	fmt.Fprintf(out, "Token theo %s (← → xem %s trước/sau; ▲ là kỳ đang xem):\n", u.name(), u.name())
	if top == 0 {
		fmt.Fprintf(out, "Không có lượt gọi nào trong %d %s này.\n\n", n, u.name())
		return
	}
	col := chartColumn(u)
	bar := col - 2
	lines := make([]strings.Builder, chartHeight+3)
	steps := []string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
	viewed := bucketIndex(u, first, p.Start)
	for i, x := range series {
		value := "·"
		if x > 0 {
			value = compactTokens(x)
		}
		lines[0].WriteString(center(value, col))
		eighths := int64(0)
		if x > 0 {
			eighths = max(1, x*chartHeight*8/top)
		}
		for r := 0; r < chartHeight; r++ {
			fill := min(8, max(0, eighths-int64((chartHeight-1-r)*8)))
			lines[1+r].WriteString(" " + strings.Repeat(steps[fill], bar) + " ")
		}
		lines[chartHeight+1].WriteString(center(chartLabel(u, unitStep(u, first, i)), col))
		marker := ""
		if i == viewed {
			marker = "▲"
		}
		lines[chartHeight+2].WriteString(center(marker, col))
	}
	for i := range lines {
		fmt.Fprintln(out, strings.TrimRight(lines[i].String(), " "))
	}
	fmt.Fprintln(out)
}
