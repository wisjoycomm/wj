package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// marker writes ~/.claude/sessions/<pid>.json the way Claude Code does.
func writeMarker(t *testing.T, home string, pid int, session, cwd, status, kind string) {
	t.Helper()
	body := fmt.Sprintf(`{"pid":%d,"sessionId":%q,"cwd":%q,"status":%q,"kind":%q,"startedAt":%d,"procStart":"0","pidDomain":"win32:test"}`,
		pid, session, filepath.ToSlash(cwd), status, kind, time.Now().Add(-time.Hour).UnixMilli())
	writeTestFile(t, filepath.Join(home, ".claude", "sessions", fmt.Sprintf("%d.json", pid)), body)
}

func TestLiveSessionsNeedARunningProcessAndKeepTheirStatus(t *testing.T) {
	s, home := testScanner(t)
	game := filepath.Join(home, "w", "game")
	writeMarker(t, home, os.Getpid(), "aaaa1111-2222", game, "busy", "interactive")
	writeMarker(t, home, 2147483000, "bbbb", filepath.Join(home, "w", "closed"), "idle", "interactive") // no such process
	writeTestFile(t, filepath.Join(home, ".claude", "sessions", "broken.json"), "{ nope")
	s.refreshLive()
	if len(s.live) != 1 {
		t.Fatalf("live = %+v", s.live)
	}
	l := s.live[0]
	if l.ID != "aaaa1111-2222" || l.Status != "busy" || l.Kind != "interactive" || l.Project != "game" || l.PID != os.Getpid() || l.Started.IsZero() {
		t.Errorf("live session = %+v", l)
	}
	if _, ok := s.liveByID["aaaa1111-2222"]; !ok || len(s.liveByID) != 1 {
		t.Errorf("liveByID = %v", s.liveByID)
	}
}

func TestLiveStampChangesWhenASessionOpensClosesOrSwitchesStatus(t *testing.T) {
	s, home := testScanner(t)
	s.refreshLive()
	empty := s.liveStamp
	writeMarker(t, home, os.Getpid(), "s1", filepath.Join(home, "p"), "idle", "interactive")
	s.refreshLive()
	idle := s.liveStamp
	writeMarker(t, home, os.Getpid(), "s1", filepath.Join(home, "p"), "busy", "interactive")
	s.refreshLive()
	busy := s.liveStamp
	s.refreshLive()
	again := s.liveStamp
	if empty == idle || idle == busy || empty == busy || busy != again {
		t.Errorf("stamps: empty %d idle %d busy %d again %d", empty, idle, busy, again)
	}
}

func TestMarkNextToTheSessionAndOnlyOpenOnesAreKept(t *testing.T) {
	s, home := testScanner(t)
	game := filepath.Join(home, "w", "game")
	writeTestFile(t, filepath.Join(home, ".claude", "projects", "p", "open1111.jsonl"), claudeLine("a1", game, "2026-11-09T01:00:00Z", 300, 0, 0, 0))
	writeTestFile(t, filepath.Join(home, ".claude", "projects", "p", "idle2222.jsonl"), claudeLine("b1", game, "2026-11-09T02:00:00Z", 200, 0, 0, 0))
	writeTestFile(t, filepath.Join(home, ".claude", "projects", "p", "gone3333.jsonl"), claudeLine("c1", game, "2026-11-09T03:00:00Z", 100, 0, 0, 0))
	writeMarker(t, home, os.Getpid(), "open1111", game, "busy", "interactive")
	writeMarker(t, home, os.Getppid(), "idle2222", game, "idle", "bg")
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, testZone)

	all, err := reportText(s, usageView{Spec: "2026-11-09", Top: -1}, now, 0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(all, "open1111 ●") || !strings.Contains(all, "idle2222 ○") || strings.Contains(all, "gone3333 ●") || strings.Contains(all, "gone3333 ○") {
		t.Errorf("marks next to the ids:\n%s", all)
	}
	only, _ := reportText(s, usageView{Spec: "2026-11-09", Top: -1, Open: true}, now, 0, time.Time{})
	if !strings.Contains(only, "open1111") || !strings.Contains(only, "idle2222") || strings.Contains(only, "gone3333") {
		t.Errorf("open only:\n%s", only)
	}
	if !strings.Contains(only, "Tổng: 2 lượt") || strings.Contains(only, "Tổng hợp theo model") || strings.Contains(only, "Theo ngày") {
		t.Errorf("open-only totals cover just the open sessions, with no tree or day table:\n%s", only)
	}
}

func TestOpenViewWithNothingOpenSaysSoInsteadOfShowingEverything(t *testing.T) {
	s, home := testScanner(t)
	writeTestFile(t, filepath.Join(home, ".claude", "projects", "p", "old.jsonl"), claudeLine("a1", "/w/game", "2026-11-09T01:00:00Z", 300, 0, 0, 0))
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, testZone)
	text, _ := reportText(s, usageView{Spec: "2026-11-09", Top: -1, Open: true}, now, 0, time.Time{})
	if strings.Contains(text, "old.jsonl") || strings.Contains(text, "300") || !strings.Contains(text, "Không có phiên nào đang mở") {
		t.Errorf("nothing is open:\n%s", text)
	}
}

func TestOpenCommandTogglesAndSurvivesReset(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, testZone)
	v := usageView{Spec: "week", Top: -1}
	for _, c := range []struct {
		line string
		want bool
	}{{"open", true}, {"open off", false}, {"live", true}, {"open on", true}, {"running off", false}} {
		if msg, _ := applyUsageCommand(&v, c.line, now); msg != "" || v.Open != c.want {
			t.Errorf("%q: open = %v, msg = %q", c.line, v.Open, msg)
		}
	}
	if msg, _ := applyUsageCommand(&v, "open maybe", now); !strings.Contains(msg, "open on") {
		t.Errorf("a bad value is explained: %q", msg)
	}
	applyUsageCommand(&v, "open", now)
	applyUsageCommand(&v, "reset", now)
	if v.Open || v.Spec != "today" {
		t.Errorf("reset clears the open filter: %+v", v)
	}
}

func TestOpenAppearsInTheListWithItsValues(t *testing.T) {
	s := uiFixture(t)
	v := usageView{Spec: "week", Top: -1, Overview: true}
	rows := s.periodOverview(v, uiNow)
	base, _ := buildSuggestions("", v, rows, nil, uiNow)
	var open usageSuggestion
	for _, x := range base {
		if x.Name == "open" {
			open = x
		}
	}
	if open.Apply != "open on" || open.Mark {
		t.Errorf("open entry = %+v", open)
	}
	on := v
	on.Open = true
	base, _ = buildSuggestions("", on, rows, nil, uiNow)
	for _, x := range base {
		if x.Name == "open" && (x.Apply != "open off" || !x.Mark) {
			t.Errorf("open entry while on = %+v", x)
		}
	}
	values, _ := buildSuggestions("open ", v, rows, nil, uiNow)
	if suggestionNames(values) != "on,off" {
		t.Errorf("values = %s", suggestionNames(values))
	}
	if got, _ := buildSuggestions("liv", v, rows, nil, uiNow); suggestionNames(got) != "" {
		// "live" is an alias typed in full; a half-typed alias is not expanded.
		t.Logf("half-typed alias gives %q", suggestionNames(got))
	}
}

func TestSameProcessRejectsAReusedPid(t *testing.T) {
	if !sameProcess(sessionMarker{PID: os.Getpid()}) {
		t.Error("a marker without a start time is trusted")
	}
	start, ok := processStartTime(os.Getpid())
	if !ok {
		t.Skip("this system does not give a process start time")
	}
	ft := func(tm time.Time) int64 { return tm.UnixNano()/100 + 116444736000000000 }
	good := sessionMarker{PID: os.Getpid(), ProcStart: flexInt(ft(start)), PIDDomain: "win32:host"}
	if !sameProcess(good) {
		t.Errorf("the real start time must match: %+v", good)
	}
	reused := sessionMarker{PID: os.Getpid(), ProcStart: flexInt(ft(start.Add(-3 * time.Hour))), PIDDomain: "win32:host"}
	if sameProcess(reused) {
		t.Error("a pid whose process started hours after the marker belongs to another process")
	}
}

func TestScreenRedrawsWhenASessionSwitchesFromBusyToIdle(t *testing.T) {
	s, home := testScanner(t)
	game := filepath.Join(home, "w", "game")
	today := time.Now().UTC().Format(time.RFC3339)
	writeTestFile(t, filepath.Join(home, ".claude", "projects", "p", "live1111.jsonl"), claudeLine("a1", game, today, 300, 0, 0, 0))
	writeMarker(t, home, os.Getpid(), "live1111", game, "busy", "interactive")
	var out strings.Builder
	toIdle := func() { writeMarker(t, home, os.Getpid(), "live1111", game, "idle", "interactive") }
	keys := &scriptedKeys{steps: []scriptStep{
		{ok: false},                             // nothing changed: no new frame
		{ok: false, before: toIdle},             // the session finished its answer: a new frame
		{key: keyEvent{kind: keyEsc}, ok: true}, // quit
	}}
	if _, err := usageUILoop(keys, &out, s, usageView{Spec: "today", Top: -1, Overview: true}, time.Time{}, func() (int, int) { return 120, 40 }, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	frames := framesOf(out.String())
	if len(frames) != 2 {
		t.Fatalf("frames = %d, want 2 (the first, and one after the status changed)", len(frames))
	}
	if !strings.Contains(frames[0], "live1111 ●") || !strings.Contains(frames[1], "live1111 ○") {
		t.Errorf("the mark must follow the status:\nfirst: %s\nsecond: %s", stripEscapes(frames[0]), stripEscapes(frames[1]))
	}
}

// The files Claude Code really writes: procStart is a string, startedAt a number.
func TestMarkerAsClaudeCodeWritesItIsRead(t *testing.T) {
	s, home := testScanner(t)
	start, ok := processStartTime(os.Getpid())
	filetime := int64(0)
	if ok {
		filetime = start.UnixNano()/100 + 116444736000000000
	}
	body := fmt.Sprintf(`{"pid":%d,"sessionId":"cccc3333-aaaa","cwd":%q,"status":"idle","kind":"interactive","version":"2.1.319","startedAt":1794215822007,"procStart":"%d","pidDomain":"win32:host","peerProtocol":1,"name":"x"}`,
		os.Getpid(), filepath.ToSlash(filepath.Join(home, "w", "game")), filetime)
	writeTestFile(t, filepath.Join(home, ".claude", "sessions", "real.json"), body)
	s.refreshLive()
	if len(s.live) != 1 || s.live[0].ID != "cccc3333-aaaa" || s.live[0].Status != "idle" || s.live[0].Started.IsZero() {
		t.Fatalf("a marker with a string procStart must be read: %+v", s.live)
	}
	var f flexInt
	for text, want := range map[string]flexInt{`"123"`: 123, `456`: 456, `"abc"`: 0, `null`: 0, `""`: 0, `"-5"`: -5} {
		if err := f.UnmarshalJSON([]byte(text)); err != nil || f != want {
			t.Errorf("%s reads as %d, want %d (err %v)", text, f, want, err)
		}
	}
}

func TestHintAfterSwitchListsOpenSessionsWithoutTouchingThem(t *testing.T) {
	_, home := testScanner(t)
	game := filepath.Join(home, "w", "game")
	writeMarker(t, home, os.Getpid(), "busy1111-aaaa", game, "busy", "interactive")
	writeMarker(t, home, os.Getppid(), "idle2222-bbbb", filepath.Join(home, "w", "web"), "idle", "interactive")
	var out strings.Builder
	printOpenSessionsHint(&out, home)
	text := out.String()
	for _, want := range []string{"Có 2 phiên Claude đang mở", "wj không tự đóng phiên nào",
		"claude --resume busy1111-aaaa", "claude --resume idle2222-bbbb",
		"đang trả lời, đợi nó xong", "rảnh, mở lại được ngay", "busy1111 ●", "idle2222 ○", "game", "web"} {
		if !strings.Contains(text, want) {
			t.Errorf("hint lacks %q:\n%s", want, text)
		}
	}
	// Both processes are still running afterwards: nothing was closed.
	if !processAlive(os.Getpid()) || !processAlive(os.Getppid()) {
		t.Error("the hint must not touch any process")
	}
}

func TestHintIsSilentWhenNothingIsOpen(t *testing.T) {
	_, home := testScanner(t)
	var out strings.Builder
	printOpenSessionsHint(&out, home)
	if out.String() != "" {
		t.Errorf("nothing open, nothing printed: %q", out.String())
	}
	// A marker whose process is gone is not "open".
	writeMarker(t, home, 2147483000, "gone", filepath.Join(home, "p"), "idle", "interactive")
	printOpenSessionsHint(&out, home)
	if out.String() != "" {
		t.Errorf("a dead process is not an open session: %q", out.String())
	}
}
