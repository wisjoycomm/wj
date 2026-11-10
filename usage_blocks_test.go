package main

import (
	"strings"
	"testing"
	"time"
)

func blockScanner(t *testing.T, when ...time.Time) *sessionScanner {
	t.Helper()
	s, _ := testScanner(t)
	for i, w := range when {
		s.record(itoa(int64(i)), sessionCall{When: w, Project: "game", App: "claude", Model: "claude-sonnet-4-5",
			Input: 1000, Output: 100, CacheRead: 9000, Session: "s1"})
	}
	return s
}

func TestBlocksStartOnTheHourAndSplitAfterFiveHours(t *testing.T) {
	day := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	s := blockScanner(t,
		day.Add(8*time.Hour+20*time.Minute), // block 1 starts 08:00
		day.Add(12*time.Hour+50*time.Minute),
		day.Add(13*time.Hour+10*time.Minute), // past 13:00: block 2 starts 13:00
		day.Add(20*time.Hour),                // more than 5h after the last call: a gap, then block 3
	)
	now := day.Add(21 * time.Hour)
	list := s.blocks(time.Time{}, "", "", now)
	if len(list) != 4 {
		t.Fatalf("blocks = %+v", list)
	}
	if !list[0].Start.Equal(day.Add(8*time.Hour)) || list[0].Calls != 2 || list[0].Active {
		t.Errorf("first = %+v", list[0])
	}
	if !list[1].Start.Equal(day.Add(13*time.Hour)) || list[1].Calls != 1 {
		t.Errorf("second = %+v", list[1])
	}
	if !list[2].Gap || !list[2].Start.Equal(day.Add(18*time.Hour+10*time.Minute)) || !list[2].End.Equal(day.Add(20*time.Hour)) {
		t.Errorf("gap = %+v", list[2])
	}
	if !list[3].Active || !list[3].Start.Equal(day.Add(20*time.Hour)) {
		t.Errorf("last = %+v", list[3])
	}
	if l, _ := blockLimit("max", list); l != 20200 {
		t.Errorf("max limit = %d", l)
	}
}

func TestRunningBlockHasABurnRateAndAProjection(t *testing.T) {
	start := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	s := blockScanner(t, start, start.Add(10*time.Minute))
	now := start.Add(time.Hour)
	b := s.blocks(time.Time{}, "", "", now)[0]
	perMin, plain, _, ok := b.burnRate()
	if !ok || perMin != 2020 || plain != 220 {
		t.Errorf("rate = %v %v %v", perMin, plain, ok)
	}
	tokens, _, ok := b.projection(now)
	if !ok || tokens != 20200+2020*240 {
		t.Errorf("projection = %d", tokens)
	}
	text, err := reportText(s, usageView{Spec: "2026-11-09", Blocks: true, Limit: "100k"}, now, 0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Khối 5 giờ", "▶", "Tốc độ: 2.0k token/phút", "Dự kiến hết khối: 505.0k", "VƯỢT", "% GIỚI HẠN", "20%"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if _, err := reportText(s, usageView{Spec: "today", Blocks: true, Limit: "abc"}, now, 0, time.Time{}); err == nil {
		t.Errorf("a bad limit is an error")
	}
}

func TestBlocksCommandsAtThePrompt(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	v := usageView{Spec: "today"}
	if msg, _ := applyUsageCommand(&v, "blocks", now); msg != "" || !v.Blocks {
		t.Errorf("blocks: %q %+v", msg, v)
	}
	if msg, _ := applyUsageCommand(&v, "limit 5M", now); msg != "" || v.Limit != "5m" {
		t.Errorf("limit: %q %+v", msg, v)
	}
	if msg, _ := applyUsageCommand(&v, "limit xyz", now); msg == "" {
		t.Errorf("a bad limit is refused")
	}
	applyUsageCommand(&v, "limit off", now)
	applyUsageCommand(&v, "blocks off", now)
	if v.Blocks || v.Limit != "" {
		t.Errorf("off: %+v", v)
	}
}

func TestJSONOfAPeriodAndOfBlocks(t *testing.T) {
	start := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	s := blockScanner(t, start, start.Add(10*time.Minute))
	now := start.Add(time.Hour)
	var b strings.Builder
	result, err := periodJSON(s, usageView{Spec: "2026-11-09", Top: -1}, now)
	if err != nil || writeJSON(&b, result) != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"totalTokens": 20200`, `"id": "s1"`, `"date": "2026-11-09"`, `"model": "sonnet-4-5"`} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("missing %q in %s", want, b.String())
		}
	}
	b.Reset()
	result, err = periodJSON(s, usageView{Spec: "2026-11-09", Blocks: true, Limit: "100k"}, now)
	if err != nil || writeJSON(&b, result) != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"active": true`, `"tokensPerMinute": 2020`, `"limitStatus": "exceeds"`, `"remainingMinutes": 240`} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("missing %q in %s", want, b.String())
		}
	}
}
