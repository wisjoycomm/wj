package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Token limits on the overlay: how much of a 5-hour block and of the last 7 days is
// used, in percent of a limit the user sets. The 5-hour block is counted the way
// the Claude plans do (see usage_blocks.go); the week is the last 7 days, rolling,
// the same number as "7d" in the overlay's header. wj cannot read a plan's real
// limits, so they are the user's numbers.

type overlayLimit struct {
	Label       string
	Used, Limit int64
	Reset       time.Time // when the 5-hour block ends; zero for the week
}

func (l overlayLimit) percent() int64 {
	if l.Limit <= 0 {
		return 0
	}
	return l.Used * 100 / l.Limit
}

func (l overlayLimit) level() int { return warnLevel(l.Used, l.Limit) }

// text is the label, the tokens used of the limit and the share, and for a running
// block the time left: "5h 12.3M/76.1M · 16% · còn 2h15".
func (l overlayLimit) text(now time.Time) string {
	text := fmt.Sprintf("%s %s/%s · %d%%", l.Label, compactTokens(l.Used), compactTokens(l.Limit), l.percent())
	if !l.Reset.IsZero() && l.Reset.After(now) {
		text += " · còn " + shortDuration(l.Reset.Sub(now))
	}
	return text
}

// overlayTitle is the word before the total: none for today, the default, which the
// chart's green column already shows; "tuần này " or "tháng này " otherwise.
func overlayTitle(tag string) string {
	if tag == "" || tag == "hôm nay" {
		return ""
	}
	return tag + " "
}

// moneyText is costText with the dollar sign after the number, the way it is read
// aloud in Vietnamese: "46.11$", "<0.01$", "12.00+$".
func moneyText(row usageRow) string {
	text := costText(row)
	if !strings.Contains(text, "$") {
		return text
	}
	return strings.Replace(text, "$", "", 1) + "$"
}

// shortDuration is "2h10" or "45p".
func shortDuration(d time.Duration) string {
	minutes := int(d.Round(time.Minute).Minutes())
	if minutes >= 60 {
		return fmt.Sprintf("%dh%02d", minutes/60, minutes%60)
	}
	return fmt.Sprintf("%dp", minutes)
}

// checkOverlayLimit accepts what -limit-5h and -limit-week take: a token count
// (20M, 500k), "max" (your own largest block or week) or "off".
func checkOverlayLimit(text string) error {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "", "off", "0", "max":
		return nil
	}
	if _, err := parseTokenCount(text); err != nil {
		return errors.New("ghi số token (vd 20M, 500k), max hoặc off")
	}
	return nil
}

// limitSetting is a saved limit, with nothing saved meaning "max": the overlay
// shows both limits until the user turns one off.
func limitSetting(text string) string {
	if strings.TrimSpace(text) == "" {
		return "max"
	}
	return text
}

// limitWeeks is how far back "max" looks for the largest week.
const limitWeeks = 12

// overlayLimits is the overlay's limit rows: the running 5-hour block and the last
// 7 days. "max" is the largest finished block of the last 30 days, or the largest
// of the 12 earlier 7-day spans.
func (s *sessionScanner) overlayLimits(limit5h, limitWeek string, now time.Time) []overlayLimit {
	var list []overlayLimit
	blocks := s.blocks(now.Add(-30*24*time.Hour), "", "", now)
	if limit, err := blockLimit(limitSetting(limit5h), blocks); err == nil && limit > 0 {
		row := overlayLimit{Label: "5h", Limit: limit}
		if n := len(blocks); n > 0 && blocks[n-1].Active {
			row.Used, row.Reset = blocks[n-1].tokens(), blocks[n-1].End
		}
		list = append(list, row)
	}
	weeks := s.rollingWeeks(now)
	limit, err := blockLimit(limitSetting(limitWeek), nil)
	if strings.EqualFold(strings.TrimSpace(limitSetting(limitWeek)), "max") {
		limit, err = 0, nil
		for _, week := range weeks[1:] {
			limit = max(limit, week)
		}
	}
	if err == nil && limit > 0 {
		list = append(list, overlayLimit{Label: "tuần", Used: weeks[0], Limit: limit})
	}
	return list
}

// rollingWeeks is the token of the last 7 days, then of the 7 days before that,
// and so on, limitWeeks+1 spans in all.
func (s *sessionScanner) rollingWeeks(now time.Time) []int64 {
	weeks := make([]int64, limitWeeks+1)
	span := 7 * 24 * time.Hour
	for _, call := range s.calls {
		if call.When.After(now) {
			continue
		}
		if i := int(now.Sub(call.When) / span); i < len(weeks) {
			weeks[i] += call.Input + call.Output + call.CacheRead + call.CacheWrite
		}
	}
	return weeks
}
