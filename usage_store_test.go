package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func storeTotal(t *testing.T, home string) (*sessionScanner, usageRow) {
	t.Helper()
	s := newSessionScanner(home)
	s.scan(time.Time{})
	_, total := s.summarize(usageFilter{GroupBy: []string{"project"}})
	return s, total
}

func TestUsageStoreReadsOnlyAppendedBytes(t *testing.T) {
	_, home := testScanner(t)
	path := filepath.Join(home, ".claude", "projects", "p", "a.jsonl")
	first := claudeLine("m1", "/w/game", "2026-01-01T00:00:00Z", 10, 1, 0, 0)
	writeTestFile(t, path, first)
	if _, total := storeTotal(t, home); total.Calls != 1 {
		t.Fatalf("first run calls = %d", total.Calls)
	}
	// Change the old bytes without changing the size or making the file older: a
	// second run that re-read them would see m9 instead of m1.
	second := claudeLine("m2", "/w/game", "2026-01-01T00:00:01Z", 5, 1, 0, 0)
	writeTestFile(t, path, claudeLine("m9", "/w/game", "2026-01-01T00:00:00Z", 10, 1, 0, 0)+second)
	s := newSessionScanner(home)
	if got := s.pendingBytes(time.Time{}); got != int64(len(second)) {
		t.Errorf("pending = %d, want %d", got, len(second))
	}
	s.scan(time.Time{})
	_, total := s.summarize(usageFilter{GroupBy: []string{"project"}})
	if total.Calls != 2 || total.Input != 15 {
		t.Errorf("total = %+v", total.usageTotal)
	}
	if _, ok := s.index["claude\x00m9"]; ok {
		t.Error("old bytes were read again")
	}
}

func TestUsageStoreKeepsCallsOfDeletedFile(t *testing.T) {
	_, home := testScanner(t)
	path := filepath.Join(home, ".codex", "sessions", "rollout-2026-01-01-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee.jsonl")
	writeTestFile(t, path, codexContext("/w/app", "gpt-x")+codexRecord("t1", "r1", "2026-01-01T00:00:01Z", 100, 40, 7))
	storeTotal(t, home)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, total := storeTotal(t, home); total.Calls != 1 || total.Output != 7 || total.CacheRead != 40 {
		t.Errorf("after delete total = %+v", total.usageTotal)
	}
}

func TestUsageStoreTruncatedFileNotDoubled(t *testing.T) {
	_, home := testScanner(t)
	path := filepath.Join(home, ".claude", "projects", "p", "a.jsonl")
	writeTestFile(t, path, claudeLine("m1", "/w/game", "2026-01-01T00:00:00Z", 10, 1, 0, 0)+
		claudeLine("m2", "/w/game", "2026-01-01T00:00:01Z", 20, 1, 0, 0))
	storeTotal(t, home)
	writeTestFile(t, path, claudeLine("m3", "/w/game", "2026-01-01T00:00:02Z", 3, 1, 0, 0))
	if _, total := storeTotal(t, home); total.Calls != 1 || total.Input != 3 {
		t.Errorf("after truncate total = %+v", total.usageTotal)
	}
}

func TestUsageStoreCorruptIsRebuilt(t *testing.T) {
	_, home := testScanner(t)
	writeTestFile(t, filepath.Join(home, ".wj", usageStoreName), "not a store")
	writeTestFile(t, filepath.Join(home, ".claude", "projects", "p", "a.jsonl"),
		claudeLine("m1", "/w/game", "2026-01-01T00:00:00Z", 10, 1, 0, 0))
	if _, total := storeTotal(t, home); total.Calls != 1 {
		t.Errorf("calls = %d", total.Calls)
	}
	if _, total := storeTotal(t, home); total.Calls != 1 {
		t.Errorf("rebuilt store calls = %d", total.Calls)
	}
}
