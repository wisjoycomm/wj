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
	if got := block.text(now); got != "5h 20.2k/40.4k · 50% · còn 3h30" {
		t.Errorf("5h text = %q", got)
	}
	if week.Used != 30300 || week.percent() != 50 || !week.Reset.IsZero() {
		t.Errorf("week = %+v", week)
	}
	if got := week.text(now); got != "tuần 30.3k/60.6k · 50%" {
		t.Errorf("week text = %q", got)
	}
}

// Nothing saved means max: both rows show, measured against your own record.
func TestOverlayLimitsDefaultToYourLargestBlockAndWeek(t *testing.T) {
	now := time.Date(2026, 11, 20, 15, 30, 0, 0, time.UTC)
	// Last week: 3 calls in one block 10 days ago. This week: 1 call now.
	s := blockScanner(t, now.Add(-10*24*time.Hour), now.Add(-10*24*time.Hour+time.Minute),
		now.Add(-10*24*time.Hour+2*time.Minute), now.Add(-10*time.Minute))
	list := s.overlayLimits("", "", now)
	if len(list) != 2 {
		t.Fatalf("got %+v", list)
	}
	if list[0].Limit != 30300 || list[0].Used != 10100 {
		t.Errorf("5h max is the largest finished block: %+v", list[0])
	}
	if list[1].Limit != 30300 || list[1].Used != 10100 {
		t.Errorf("week max is the largest earlier 7 days: %+v", list[1])
	}
	if got := s.overlayLimits("off", "off", now); len(got) != 0 {
		t.Errorf("off hides both: %+v", got)
	}
}

func TestOverlayLimitWithNoRunningBlockIsZero(t *testing.T) {
	now := time.Date(2026, 11, 20, 15, 30, 0, 0, time.UTC)
	s := blockScanner(t, now.Add(-7*time.Hour))
	list := s.overlayLimits("max", "off", now)
	if len(list) != 1 || list[0].Used != 0 || list[0].Limit != 10100 || list[0].text(now) != "5h 0/10.1k · 0%" {
		t.Errorf("nothing runs now: %+v %q", list, list[0].text(now))
	}
}

func TestCheckOverlayLimit(t *testing.T) {
	for _, ok := range []string{"", "off", "max", "20M", "500k"} {
		if err := checkOverlayLimit(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	if checkOverlayLimit("lots") == nil {
		t.Error("a word that is not a count must fail")
	}
}

func TestOverlayTitleAndMoney(t *testing.T) {
	if overlayTitle("hôm nay") != "" || overlayTitle("tuần này") != "tuần này " {
		t.Error("today needs no title, the others do")
	}
	if got := moneyText(usageRow{Cost: 46.111}); got != "46.11$" {
		t.Errorf("money = %q", got)
	}
}

func TestShortDuration(t *testing.T) {
	if shortDuration(45*time.Minute) != "45p" || shortDuration(130*time.Minute) != "2h10" {
		t.Error(shortDuration(45*time.Minute), shortDuration(130*time.Minute))
	}
}
