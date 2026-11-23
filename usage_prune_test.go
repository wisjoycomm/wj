package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func agePath(t *testing.T, path string, age time.Duration) {
	t.Helper()
	when := time.Now().Add(-age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func TestUsagePrune(t *testing.T) {
	_, home := testScanner(t)
	projects := filepath.Join(home, ".claude", "projects", "p")
	old := filepath.Join(projects, "old1.jsonl")
	fresh := filepath.Join(projects, "new1.jsonl")
	sub := filepath.Join(projects, "old1", "subagents", "agent-a1.jsonl")
	codex := filepath.Join(home, ".codex", "sessions", "rollout-2026-01-01-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee.jsonl")
	partial := filepath.Join(projects, "partial.jsonl")
	unread := filepath.Join(projects, "unread.jsonl")
	liveFile := filepath.Join(projects, "live1.jsonl")
	writeTestFile(t, old, claudeLine("m1", "/w/game", "2026-01-01T00:00:00Z", 10, 1, 0, 0))
	writeTestFile(t, fresh, claudeLine("m2", "/w/game", "2026-01-01T00:00:00Z", 20, 1, 0, 0))
	writeTestFile(t, sub, claudeLine("m3", "/w/game", "2026-01-01T00:00:00Z", 30, 1, 0, 0))
	writeTestFile(t, strings.TrimSuffix(sub, ".jsonl")+".meta.json", `{"agentType":"x"}`)
	writeTestFile(t, codex, codexContext("/w/app", "gpt-x")+codexRecord("t1", "r1", "2026-01-01T00:00:01Z", 100, 40, 7))
	// A line still being written: the store stops before it, so the file is not fully read.
	writeTestFile(t, partial, claudeLine("m4", "/w/game", "2026-01-01T00:00:00Z", 1, 1, 0, 0)+`{"type":"assi`)
	// Not one whole line yet: nothing of it is in the store.
	writeTestFile(t, unread, `{"type":"assistant"`)
	writeTestFile(t, liveFile, claudeLine("m5", "/w/game", "2026-01-01T00:00:00Z", 2, 1, 0, 0))
	writeMarker(t, home, os.Getpid(), "live1", filepath.Join(home, "w", "game"), "idle", "interactive")
	for _, p := range []string{old, sub, codex, partial, unread, liveFile} {
		agePath(t, p, 60*24*time.Hour)
	}
	_, before := storeTotal(t, home)

	var out strings.Builder
	if err := pruneSessions(newSessionScanner(home), time.Now().Add(-30*24*time.Hour), false, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Sẽ xoá 3 file") {
		t.Errorf("dry run = %q", out.String())
	}
	for _, p := range []string{old, sub, codex} {
		if !exists(p) {
			t.Fatalf("dry run deleted %s", p)
		}
	}

	out.Reset()
	if err := pruneSessions(newSessionScanner(home), time.Now().Add(-30*24*time.Hour), true, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Đã xoá 3 file") {
		t.Errorf("prune = %q", out.String())
	}
	for _, p := range []string{old, codex, filepath.Dir(sub)} {
		if exists(p) {
			t.Errorf("%s still there", p)
		}
	}
	for _, p := range []string{fresh, partial, unread, liveFile, filepath.Join(projects, "old1")} {
		if !exists(p) {
			t.Errorf("%s was deleted", p)
		}
	}
	if _, after := storeTotal(t, home); after.usageTotal != before.usageTotal || after.Calls != before.Calls {
		t.Errorf("total before %+v, after %+v", before.usageTotal, after.usageTotal)
	}
}

func TestInsideRoots(t *testing.T) {
	root := t.TempDir()
	in := filepath.Join(root, "a.jsonl")
	outside := filepath.Join(t.TempDir(), "b.jsonl")
	writeTestFile(t, in, "x")
	writeTestFile(t, outside, "x")
	real, _ := filepath.EvalSymlinks(root)
	if !insideRoots(in, []string{real}) || insideRoots(outside, []string{real}) || insideRoots(root, []string{real}) {
		t.Error("insideRoots")
	}
}

func TestParseAge(t *testing.T) {
	for in, want := range map[string]time.Duration{"30d": 720 * time.Hour, "0d": 0, "36h": 36 * time.Hour} {
		if got, err := parseAge(in); err != nil || got != want {
			t.Errorf("parseAge(%q) = %v, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "x", "-3d", "3w"} {
		if _, err := parseAge(in); err == nil {
			t.Errorf("parseAge(%q) accepted", in)
		}
	}
}
