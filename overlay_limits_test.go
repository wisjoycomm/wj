package main

import (
	"testing"
	"time"
)

func TestOverlayLimitsCountTheRunningBlockAndTheLastSevenDays(t *testing.T) {
	now := time.Date(2026, 11, 20, 15, 30, 0, 0, time.UTC)
	// Each call is 10,100 tokens. One 9 days ago (outside the week), one 3 days ago,
	// two in the block that started at 14:00.
	s := blockScanner(t, now.Add(-9*24*time.Hour), now.Add(-3*24*time.Hour),
		now.Add(-80*time.Minute), now.Add(-10*time.Minute))

	list := s.overlayLimits("40400", "60600", now)
	if len(list) != 2 {
		t.Fatalf("want 5h and week, got %+v", list)
	}
	block, week := list[0], list[1]
	if block.Used != 20200 || block.percent() != 50 || !block.Reset.Equal(time.Date(2026, 11, 20, 19, 0, 0, 0, time.UTC)) {
		t.Errorf("5h = %+v", block)
	}
	if got := block.text(now); got != "5h 50% · còn 3h30" {
		t.Errorf("5h text = %q", got)
	}
	if week.Used != 30300 || week.percent() != 50 || !week.Reset.IsZero() {
		t.Errorf("week = %+v", week)
	}
	if got := week.text(now); got != "7 ngày 50%" {
		t.Errorf("week text = %q", got)
	}
}

func TestOverlayLimitsOnlyWhenSet(t *testing.T) {
	now := time.Date(2026, 11, 20, 15, 30, 0, 0, time.UTC)
	s := blockScanner(t, now.Add(-10*time.Minute))
	if list := s.overlayLimits("", "off", now); len(list) != 0 {
		t.Errorf("no limits set, got %+v", list)
	}
	if list := s.overlayLimits("", "10k", now); len(list) != 1 || list[0].Label != "7 ngày" || list[0].level() != 2 {
		t.Errorf("week only, over the limit: %+v", list)
	}
}

func TestOverlayLimitWithNoRunningBlockIsZero(t *testing.T) {
	now := time.Date(2026, 11, 20, 15, 30, 0, 0, time.UTC)
	s := blockScanner(t, now.Add(-7*time.Hour))
	list := s.overlayLimits("max", "", now)
	if len(list) != 1 || list[0].Used != 0 || list[0].Limit != 10100 || list[0].text(now) != "5h 0%" {
		t.Errorf("max is the largest finished block; nothing runs now: %+v", list)
	}
}

func TestCheckOverlayLimit(t *testing.T) {
	for _, ok := range []string{"", "off", "20M", "500k"} {
		if err := checkOverlayLimit(ok, false); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	if checkOverlayLimit("max", true) != nil || checkOverlayLimit("max", false) == nil {
		t.Error("max only for the 5-hour limit")
	}
	if checkOverlayLimit("lots", true) == nil {
		t.Error("a word that is not a count must fail")
	}
}

func TestShortDuration(t *testing.T) {
	if shortDuration(45*time.Minute) != "45p" || shortDuration(130*time.Minute) != "2h10" {
		t.Error(shortDuration(45*time.Minute), shortDuration(130*time.Minute))
	}
}
