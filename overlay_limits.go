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

// text is the label and the share used, and for a running block the time left:
// "5h 34% · còn 2h10".
func (l overlayLimit) text(now time.Time) string {
	text := fmt.Sprintf("%s %d%%", l.Label, l.percent())
	if !l.Reset.IsZero() && l.Reset.After(now) {
		text += " · còn " + shortDuration(l.Reset.Sub(now))
	}
	return text
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
// (20M, 500k), "off", and for the 5-hour block "max", the largest finished block.
func checkOverlayLimit(text string, allowMax bool) error {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "", "off", "0":
		return nil
	case "max":
		if allowMax {
			return nil
		}
		return errors.New("không nhận max, hãy ghi số token, vd 500M")
	}
	_, err := parseTokenCount(text)
	return err
}

// overlayLimits is the overlay's limit row: the running 5-hour block and the last 7
// days, each only when its limit is set.
func (s *sessionScanner) overlayLimits(limit5h, limitWeek string, now time.Time) []overlayLimit {
	var list []overlayLimit
	blocks := s.blocks(now.Add(-30*24*time.Hour), "", "", now)
	if limit, err := blockLimit(limit5h, blocks); err == nil && limit > 0 {
		row := overlayLimit{Label: "5h", Limit: limit}
		if n := len(blocks); n > 0 && blocks[n-1].Active {
			row.Used, row.Reset = blocks[n-1].tokens(), blocks[n-1].End
		}
		list = append(list, row)
	}
	if limit, err := blockLimit(limitWeek, nil); err == nil && limit > 0 {
		_, week := s.summarize(usageFilter{Since: now.Add(-7 * 24 * time.Hour)})
		list = append(list, overlayLimit{Label: "7 ngày", Used: week.Total, Limit: limit})
	}
	return list
}
