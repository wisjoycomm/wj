package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

var testZone = time.FixedZone("test", 7*3600)

func TestResolvePeriod(t *testing.T) {
	// Monday 2026-11-09 15:30 local.
	now := time.Date(2026, 11, 9, 15, 30, 0, 0, testZone)
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, testZone) }
	cases := []struct {
		spec       string
		start, end time.Time
	}{
		{"today", day(2026, 11, 9), day(2026, 11, 10)},
		{"yesterday", day(2026, 11, 8), day(2026, 11, 9)},
		{"week", day(2026, 11, 9), day(2026, 11, 16)},
		{"lastweek", day(2026, 11, 2), day(2026, 11, 9)},
		{"month", day(2026, 11, 1), day(2026, 12, 1)},
		{"lastmonth", day(2026, 10, 1), day(2026, 11, 1)},
		{"7d", day(2026, 11, 3), day(2026, 11, 10)},
		{"1d", day(2026, 11, 9), day(2026, 11, 10)},
		{"2026-11-05", day(2026, 11, 5), day(2026, 11, 6)},
		{"2026-10", day(2026, 10, 1), day(2026, 11, 1)},
		{"2026-11-01..2026-11-03", day(2026, 11, 1), day(2026, 11, 4)},
		{" Today ", day(2026, 11, 9), day(2026, 11, 10)},
	}
	for _, c := range cases {
		p, err := resolvePeriod(c.spec, now)
		if err != nil {
			t.Errorf("%q: %v", c.spec, err)
			continue
		}
		if !p.Start.Equal(c.start) || !p.End.Equal(c.end) {
			t.Errorf("%q: %v → %v, want %v → %v", c.spec, p.Start, p.End, c.start, c.end)
		}
	}
}

func TestResolvePeriodWeekStartsOnMondayEvenOnSunday(t *testing.T) {
	sunday := time.Date(2026, 11, 8, 23, 0, 0, 0, testZone)
	p, err := resolvePeriod("week", sunday)
	if err != nil || !p.Start.Equal(time.Date(2026, 11, 2, 0, 0, 0, 0, testZone)) {
		t.Errorf("sunday week = %v, %v", p.Start, err)
	}
}

func TestResolvePeriodRejectsNonsense(t *testing.T) {
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, testZone)
	for _, spec := range []string{"", "tomorrow", "0d", "-3d", "d", "2026-13", "2026-02-30", "2026-11-05..2026-11-01", "2026-11-01..x"} {
		if _, err := resolvePeriod(spec, now); err == nil {
			t.Errorf("%q should be rejected", spec)
		}
	}
}

// A call at 20:00 UTC on the 8th is 03:00 on the 9th in UTC+7, so it belongs to the 9th.
func TestPeriodReportUsesLocalDays(t *testing.T) {
	s, home := testScanner(t)
	path := filepath.Join(home, ".claude", "projects", "p", "late.jsonl")
	writeTestFile(t, path, claudeLine("m1", "/w/game", "2026-11-08T20:00:00Z", 1, 1, 10, 0)+
		claudeLine("m2", "/w/game", "2026-11-09T01:00:00Z", 1, 1, 10, 0)+
		claudeLine("m3", "/w/game", "2026-11-08T10:00:00Z", 1, 1, 10, 0))
	s.scan(time.Time{})
	p, _ := resolvePeriod("today", time.Date(2026, 11, 9, 12, 0, 0, 0, testZone))
	days, sessions, total := s.periodReport(p, "", "")
	if total.Calls != 2 || len(days) != 1 || len(sessions) != 1 {
		t.Fatalf("calls = %d, days = %d, sessions = %d", total.Calls, len(days), len(sessions))
	}
	if days[0].Day.Format("2006-01-02") != "2026-11-09" {
		t.Errorf("day = %v", days[0].Day)
	}
}

func TestPeriodReportGroupsSubagentsUnderTheirSession(t *testing.T) {
	s, home := testScanner(t)
	projects := filepath.Join(home, ".claude", "projects", "p")
	writeTestFile(t, filepath.Join(projects, "main1.jsonl"),
		`{"type":"ai-title","aiTitle":"Fix the shop","sessionId":"main1"}`+"\n"+
			claudeLine("a1", "/w/game", "2026-11-09T01:00:00Z", 100, 0, 0, 0)+
			claudeLine("a2", "/w/game", "2026-11-09T02:00:00Z", 200, 0, 0, 0))
	writeTestFile(t, filepath.Join(projects, "main1", "subagents", "agent-x.jsonl"),
		claudeLine("s1", "/w/game", "2026-11-09T03:00:00Z", 1000, 0, 0, 0))
	writeTestFile(t, filepath.Join(projects, "other.jsonl"),
		claudeLine("o1", "/w/game", "2026-11-09T04:00:00Z", 50, 0, 0, 0))
	s.scan(time.Time{})
	p, _ := resolvePeriod("2026-11-09", time.Date(2026, 11, 9, 12, 0, 0, 0, testZone))
	_, sessions, total := s.periodReport(p, "", "")
	if len(sessions) != 2 || total.Total != 1350 {
		t.Fatalf("sessions = %d, total = %d", len(sessions), total.Total)
	}
	first := sessions[0]
	if first.ID != "main1" || first.Total != 1300 || first.Agent != 1000 || first.Calls != 3 || first.Title != "Fix the shop" {
		t.Errorf("first = %+v", first)
	}
	if sessions[1].ID != "other" || sessions[1].Agent != 0 || sessions[1].Title != "" {
		t.Errorf("second = %+v", sessions[1])
	}
}

func TestPeriodReportFiltersAndMaxContext(t *testing.T) {
	s, home := testScanner(t)
	writeTestFile(t, filepath.Join(home, ".claude", "projects", "p", "a.jsonl"),
		claudeLine("m1", "/w/game", "2026-11-09T01:00:00Z", 10, 5, 100, 20)+
			claudeLine("m2", "/w/tool", "2026-11-09T02:00:00Z", 1, 1, 1, 1))
	writeTestFile(t, filepath.Join(home, ".codex", "sessions", "rollout-2026-11-09T00-00-00-0123456789abcdef0123456789abcdef0123.jsonl"),
		codexContext("/w/game", "gpt-x")+codexRecord("t1", "r1", "2026-11-09T03:00:00Z", 1000, 400, 50))
	s.scan(time.Time{})
	p, _ := resolvePeriod("2026-11-09", time.Date(2026, 11, 9, 12, 0, 0, 0, testZone))
	_, all, _ := s.periodReport(p, "", "")
	if len(all) != 2 { // the two Claude calls share one file, so one session
		t.Fatalf("all = %d sessions", len(all))
	}
	_, onlyGame, _ := s.periodReport(p, "game", "claude")
	if len(onlyGame) != 1 || onlyGame[0].MaxCtx != 130 {
		t.Errorf("claude game = %+v", onlyGame)
	}
	_, codex, _ := s.periodReport(p, "", "codex")
	if len(codex) != 1 || codex[0].Client != "codex" || codex[0].ID != "0123456789abcdef0123456789abcdef0123" {
		t.Errorf("codex = %+v", codex)
	}
}

func TestPeriodReportByDayListsEveryDayAndKeepsTheOrder(t *testing.T) {
	s, home := testScanner(t)
	writeTestFile(t, filepath.Join(home, ".claude", "projects", "p", "a.jsonl"),
		claudeLine("m1", "/w/game", "2026-11-03T05:00:00Z", 30, 0, 0, 0)+
			claudeLine("m2", "/w/game", "2026-11-01T05:00:00Z", 10, 0, 0, 0)+
			claudeLine("m3", "/w/game", "2026-11-01T06:00:00Z", 10, 0, 0, 0))
	s.scan(time.Time{})
	p, _ := resolvePeriod("2026-11-01..2026-11-05", time.Date(2026, 11, 9, 0, 0, 0, 0, testZone))
	days, _, _ := s.periodReport(p, "", "")
	if len(days) != 2 || days[0].Total != 20 || days[1].Total != 30 || days[0].Sessions != 1 {
		t.Errorf("days = %+v", days)
	}
	var out bytes.Buffer
	printPeriodReport(&out, p, days, nil, usageRow{}, 0, false, 0, nil, nil, false, time.Time{})
	if !strings.Contains(out.String(), "Không có lượt gọi") {
		t.Errorf("empty total should say so: %s", out.String())
	}
}

func TestRunUsagePeriodPrintsDaysSessionsAndTopLimit(t *testing.T) {
	_, home := testScanner(t)
	for _, id := range []string{"aaaaaaaa1", "bbbbbbbb2", "cccccccc3"} {
		n := map[string]int{"aaaaaaaa1": 300, "bbbbbbbb2": 200, "cccccccc3": 100}[id]
		writeTestFile(t, filepath.Join(home, ".claude", "projects", "p", id+".jsonl"),
			`{"type":"ai-title","aiTitle":"Chat `+id+`","sessionId":"`+id+`"}`+"\n"+
				claudeLine(id, "/w/game", "2026-11-09T01:00:00Z", n, 0, 0, 0))
	}
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, testZone)
	var week bytes.Buffer
	if err := runReport(&week, home, usageView{Spec: "week", Top: 2}, now); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"tuần này", "Theo ngày:", "2026-11-09 T2", "Từng session", "(2/3)", "aaaaaaaa", "Chat aaaaaaaa1", "còn 1 session nhỏ hơn"} {
		if !strings.Contains(week.String(), want) {
			t.Errorf("week output lacks %q:\n%s", want, week.String())
		}
	}
	if strings.Contains(week.String(), "cccccccc") {
		t.Errorf("-top 2 should hide the smallest session:\n%s", week.String())
	}
	var today bytes.Buffer
	if err := runReport(&today, home, usageView{Spec: "today", Top: -1}, now); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(today.String(), "Theo ngày:") || !strings.Contains(today.String(), "cccccccc") {
		t.Errorf("a single day lists every session and no day table:\n%s", today.String())
	}
}

func TestRunUsageAcceptsPeriodBeforeOrAfterFlags(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	for _, args := range [][]string{{"today"}, {"today", "-top", "5"}, {"-top", "5", "today"}, {"2026-11-01..2026-11-02", "-app", "claude"}} {
		var out bytes.Buffer
		if err := runUsage(args, strings.NewReader(""), &out); err != nil {
			t.Errorf("%v: %v", args, err)
		}
		if !strings.Contains(out.String(), "Không có lượt gọi") {
			t.Errorf("%v: %s", args, out.String())
		}
	}
	var out bytes.Buffer
	if err := runUsage([]string{"today", "week"}, strings.NewReader(""), &out); err == nil {
		t.Error("two periods should be rejected")
	}
	if err := runUsage([]string{"nonsense"}, strings.NewReader(""), &out); err == nil || !strings.Contains(err.Error(), "yesterday") {
		t.Errorf("unknown period error = %v", err)
	}
}

func TestRenderTableAlignsUnicodeAndLeavesNoTrailingSpaces(t *testing.T) {
	var out bytes.Buffer
	renderTable(&out, []string{"TÊN", "SỐ", "GHI CHÚ"}, []bool{true, false, true},
		[][]string{{"Điều phối", "5", "tiếng Việt"}, {"a", "12345", "x"}})
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %q", lines)
	}
	for _, line := range lines {
		if strings.HasSuffix(line, " ") {
			t.Errorf("trailing space: %q", line)
		}
	}
	col := func(line string) int { return len([]rune(line[:strings.Index(line, "5")+1])) }
	if col(lines[1]) != len([]rune(lines[2][:strings.Index(lines[2], "12345")+5])) {
		t.Errorf("numbers should end in the same column:\n%s", out.String())
	}
}

func TestMainModelNamesTheBiggestAndCountsTheRest(t *testing.T) {
	r := sessionRow{models: map[string]int64{"cc/claude-opus-5-5": 900, "claude-sonnet-5-5": 80, "claude-haiku-5-5": 20}}
	if got := r.mainModel(); got != "opus-5-5 +2" {
		t.Errorf("mainModel = %q", got)
	}
	if got := (sessionRow{}).mainModel(); got != "-" {
		t.Errorf("empty mainModel = %q", got)
	}
}

func TestSessionOf(t *testing.T) {
	cases := []struct {
		path, id string
		sub      bool
	}{
		{filepath.Join("p", "abc.jsonl"), "abc", false},
		{filepath.Join("p", "abc", "subagents", "agent-1.jsonl"), "abc", true},
		{filepath.Join("s", "rollout-2026-11-09T00-00-00-0123456789abcdef0123456789abcdef0123.jsonl"), "0123456789abcdef0123456789abcdef0123", false},
	}
	for _, c := range cases {
		if id, sub := sessionOf(c.path); id != c.id || sub != c.sub {
			t.Errorf("%s: %q, %v", c.path, id, sub)
		}
	}
}

func TestPeriodReportListsWorkersWithTheirDescription(t *testing.T) {
	s, home := testScanner(t)
	projects := filepath.Join(home, ".claude", "projects", "p")
	writeTestFile(t, filepath.Join(projects, "main1.jsonl"), claudeLine("a1", "/w/game", "2026-11-09T01:00:00Z", 100, 0, 0, 0))
	sub := filepath.Join(projects, "main1", "subagents")
	writeTestFile(t, filepath.Join(sub, "agent-aaaa1111bbbb.jsonl"),
		claudeLine("s1", "/w/game", "2026-11-09T03:00:00Z", 1000, 0, 0, 0)+claudeLine("s2", "/w/game", "2026-11-09T03:01:00Z", 500, 0, 0, 0))
	writeTestFile(t, filepath.Join(sub, "agent-aaaa1111bbbb.meta.json"), `{"agentType":"general-purpose","description":"T5.10 event shop"}`)
	writeTestFile(t, filepath.Join(sub, "agent-cccc2222dddd.jsonl"), claudeLine("s3", "/w/game", "2026-11-09T04:00:00Z", 200, 0, 0, 0))
	s.scan(time.Time{})
	p, _ := resolvePeriod("2026-11-09", time.Date(2026, 11, 9, 12, 0, 0, 0, testZone))
	_, sessions, total := s.periodReport(p, "", "")
	if len(sessions) != 1 || len(sessions[0].workers) != 2 || sessions[0].Agent != 1700 {
		t.Fatalf("sessions = %+v", sessions)
	}
	workers := sessions[0].sortedWorkers()
	if workers[0].ID != "aaaa1111bbbb" || workers[0].Total != 1500 || workers[0].Calls != 2 || workers[0].Description != "T5.10 event shop" || workers[0].Type != "general-purpose" {
		t.Errorf("first worker = %+v", workers[0])
	}
	if workers[1].Description != "" || workers[1].Total != 200 {
		t.Errorf("worker without a meta file = %+v", workers[1])
	}

	var with, without bytes.Buffer
	printPeriodReport(&with, p, nil, sessions, total, 0, true, 0, nil, nil, false, time.Time{})
	printPeriodReport(&without, p, nil, sessions, total, 0, false, 0, nil, nil, false, time.Time{})
	if !strings.Contains(with.String(), "└ aaaa1111") || !strings.Contains(with.String(), "T5.10 event shop") {
		t.Errorf("-agents should list the workers:\n%s", with.String())
	}
	if strings.Contains(without.String(), "└ ") && strings.Contains(without.String(), "aaaa1111") || !strings.Contains(without.String(), "1.7k (2)") || !strings.Contains(without.String(), "xem từng worker: agents") {
		t.Errorf("without -agents the workers stay folded and the count shows:\n%s", without.String())
	}
}

func TestPeriodReportCapsTheWorkersShownPerSession(t *testing.T) {
	s, home := testScanner(t)
	projects := filepath.Join(home, ".claude", "projects", "p")
	writeTestFile(t, filepath.Join(projects, "main1.jsonl"), claudeLine("a1", "/w/game", "2026-11-09T01:00:00Z", 1, 0, 0, 0))
	for i := 0; i < maxWorkersShown+3; i++ {
		id := fmt.Sprintf("w%02d", i)
		writeTestFile(t, filepath.Join(projects, "main1", "subagents", "agent-"+id+".jsonl"),
			claudeLine("s"+id, "/w/game", "2026-11-09T03:00:00Z", 10+i, 0, 0, 0))
	}
	s.scan(time.Time{})
	p, _ := resolvePeriod("2026-11-09", time.Date(2026, 11, 9, 12, 0, 0, 0, testZone))
	_, sessions, total := s.periodReport(p, "", "")
	var out bytes.Buffer
	printPeriodReport(&out, p, nil, sessions, total, 0, true, 0, nil, nil, false, time.Time{})
	if got := strings.Count(out.String(), "└ w"); got != maxWorkersShown || !strings.Contains(out.String(), "còn 3 worker nhỏ hơn") {
		t.Errorf("workers shown = %d:\n%s", got, out.String())
	}
}

// runReport prints the report of a view once, the way `wj usage <period>` does.
func runReport(out *bytes.Buffer, home string, v usageView, now time.Time) error {
	text, err := reportText(newSessionScanner(home), v, now, 0, time.Time{})
	out.WriteString(text)
	return err
}

func TestModelTreeSumsEachModelAndItsProjects(t *testing.T) {
	s, home := testScanner(t)
	line := func(id, cwd, model string, input int) string {
		return strings.Replace(claudeLine(id, cwd, "2026-11-09T01:00:00Z", input, 0, 0, 0), `"claude-x"`, `"`+model+`"`, 1)
	}
	writeTestFile(t, filepath.Join(home, ".claude", "projects", "p", "a.jsonl"),
		line("m1", "/w/game", "claude-opus-5-5", 300)+
			line("m2", "/w/tool", "cc/claude-opus-5-5", 100)+ // the gateway prefix is the same model
			line("m3", "/w/game", "claude-sonnet-5-5", 50))
	s.scan(time.Time{})
	p, _ := resolvePeriod("2026-11-09", time.Date(2026, 11, 9, 12, 0, 0, 0, testZone))
	tree := s.periodTree(p, "", "")
	if len(tree) != 2 || tree[0].Name != "opus-5-5" || tree[0].Total != 400 || tree[1].Name != "sonnet-5-5" {
		t.Fatalf("tree = %+v", tree)
	}
	if len(tree[0].Projects) != 2 || tree[0].Projects[0].Project != "game" || tree[0].Projects[0].Total != 300 || tree[0].Projects[1].Project != "tool" {
		t.Errorf("projects of opus = %+v", tree[0].Projects)
	}
	var out bytes.Buffer
	if err := runReport(&out, home, usageView{Spec: "2026-11-09", Top: -1}, time.Date(2026, 11, 9, 12, 0, 0, 0, testZone)); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"Tổng hợp theo model", "TỔNG", "├─ opus-5-5", "│  ├─ game", "│  └─ tool", "└─ sonnet-5-5", "   └─ game"} {
		if !strings.Contains(text, want) {
			t.Errorf("report lacks %q:\n%s", want, text)
		}
	}
	// The summary is the last block before the legend, after the session list.
	if strings.Index(text, "Từng session") > strings.Index(text, "Tổng hợp theo model") {
		t.Errorf("the tree should come after the session list:\n%s", text)
	}
}

func TestModelTreeFollowsTheFilters(t *testing.T) {
	s, home := testScanner(t)
	writeTestFile(t, filepath.Join(home, ".claude", "projects", "p", "a.jsonl"),
		claudeLine("m1", "/w/game", "2026-11-09T01:00:00Z", 300, 0, 0, 0)+claudeLine("m2", "/w/tool", "2026-11-09T01:00:00Z", 100, 0, 0, 0))
	s.scan(time.Time{})
	p, _ := resolvePeriod("2026-11-09", time.Date(2026, 11, 9, 12, 0, 0, 0, testZone))
	tree := s.periodTree(p, "tool", "")
	if len(tree) != 1 || tree[0].Total != 100 || len(tree[0].Projects) != 1 {
		t.Errorf("tree for project tool = %+v", tree)
	}
	if got := s.periodTree(p, "", "codex"); len(got) != 0 {
		t.Errorf("no codex calls, tree = %+v", got)
	}
}

func TestApplyUsageCommandChangesTheView(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, testZone)
	v := usageView{Spec: "today", Top: -1}
	steps := []struct {
		line string
		want usageView
		msg  string
		quit bool
	}{
		{"week", usageView{Spec: "week", Top: -1}, "", false},
		{"  Last", usageView{Spec: "week", Top: -1}, "không hiểu", false}, // unknown: view unchanged
		{"lastmonth", usageView{Spec: "lastmonth", Top: -1}, "", false},
		{"agents", usageView{Spec: "lastmonth", Top: -1, Agents: true}, "", false},
		{"top 30", usageView{Spec: "lastmonth", Top: 30, Agents: true}, "", false},
		{"/project game", usageView{Spec: "lastmonth", Top: 30, Agents: true, Project: "game"}, "", false},
		{"app codex", usageView{Spec: "lastmonth", Top: 30, Agents: true, Project: "game", Client: "codex"}, "", false},
		{"2026-11-05", usageView{Spec: "2026-11-05", Top: 30, Agents: true, Project: "game", Client: "codex"}, "", false},
		{"top 0", usageView{Spec: "2026-11-05", Top: 0, Agents: true, Project: "game", Client: "codex"}, "", false},
		{"top auto", usageView{Spec: "2026-11-05", Top: -1, Agents: true, Project: "game", Client: "codex"}, "", false},
		{"agents off", usageView{Spec: "2026-11-05", Top: -1, Project: "game", Client: "codex"}, "", false},
		{"project all", usageView{Spec: "2026-11-05", Top: -1, Client: "codex"}, "", false},
		{"app all", usageView{Spec: "2026-11-05", Top: -1}, "", false},
		{"top x", usageView{Spec: "2026-11-05", Top: -1}, "top N", false},
		{"app gemini", usageView{Spec: "2026-11-05", Top: -1}, "app claude", false},
		{"help", usageView{Spec: "2026-11-05", Top: -1}, "Lệnh gõ", false},
		{"reset", usageView{Spec: "today", Top: -1}, "", false},
		{"", usageView{Spec: "today", Top: -1}, "", true},
		{"q", usageView{Spec: "today", Top: -1}, "", true},
		{"0", usageView{Spec: "today", Top: -1}, "", true},
	}
	for _, s := range steps {
		msg, quit := applyUsageCommand(&v, s.line, now)
		if v != s.want || quit != s.quit || !strings.Contains(msg, s.msg) || (s.msg == "" && msg != "") {
			t.Errorf("%q: view = %+v, msg = %q, quit = %v; want %+v, %q, %v", s.line, v, msg, quit, s.want, s.msg, s.quit)
		}
	}
}

func TestRunUsageWithoutTerminalPrintsTodayOnce(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	var out bytes.Buffer
	if err := runUsage(nil, strings.NewReader("week\nq\n"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "hôm nay") || strings.Contains(out.String(), "> ") || strings.Contains(out.String(), "\x1b[") {
		t.Errorf("a pipe gets the plain report of today, no prompt and no screen codes:\n%q", out.String())
	}
}

func TestUsageLoopRedrawsAfterEachCommandAndStopsOnQuit(t *testing.T) {
	s, home := testScanner(t)
	writeTestFile(t, filepath.Join(home, ".claude", "projects", "p", "a.jsonl"),
		claudeLine("m1", "/w/game", time.Now().UTC().Format(time.RFC3339), 100, 0, 0, 0))
	var out bytes.Buffer
	height := func() int { return 0 }
	if err := usageLoop(strings.NewReader("week\nagents\nnonsense\nproject game\nq\nweek\n"), &out, s, usageView{Spec: "today", Top: -1}, time.Time{}, height, false); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if got := strings.Count(text, "> "); got != 5 { // one prompt per screen: start + 4 commands, none after q
		t.Errorf("prompts = %d:\n%s", got, text)
	}
	for _, want := range []string{"hôm nay", "tuần này", "agents: bật", "không hiểu", "project: game", "Tổng hợp theo model"} {
		if !strings.Contains(text, want) {
			t.Errorf("screens lack %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\x1b[") {
		t.Errorf("clear=false must not write screen codes: %q", text)
	}
}

func TestUsageLoopClearsTheScreenAndEndsOnEOF(t *testing.T) {
	s, _ := testScanner(t)
	var out bytes.Buffer
	if err := usageLoop(strings.NewReader("week\n"), &out, s, usageView{Spec: "today", Top: -1}, time.Time{}, func() int { return 0 }, true); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(out.String(), "\x1b[2J\x1b[H"); got != 2 { // start + after "week", then EOF ends it
		t.Errorf("screens cleared = %d", got)
	}
}

func TestReportTextShrinksTheSessionListToFitTheScreen(t *testing.T) {
	s, home := testScanner(t)
	body := ""
	for i := 0; i < 30; i++ {
		body += claudeLine(fmt.Sprintf("m%02d", i), "/w/game", "2026-11-09T01:00:00Z", 10+i, 0, 0, 0)
		writeTestFile(t, filepath.Join(home, ".claude", "projects", "p", fmt.Sprintf("s%02d.jsonl", i)),
			claudeLine(fmt.Sprintf("m%02d", i), "/w/game", "2026-11-09T01:00:00Z", 10+i, 0, 0, 0))
	}
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, testZone)
	full, _ := reportText(s, usageView{Spec: "today", Top: -1}, now, 0, time.Time{})
	fit, err := reportText(s, usageView{Spec: "today", Top: -1}, now, 40, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(fit, "\n") > 40 || strings.Count(fit, "\n") >= strings.Count(full, "\n") {
		t.Errorf("lines: fit = %d (max 40), full = %d", strings.Count(fit, "\n"), strings.Count(full, "\n"))
	}
	if !strings.Contains(fit, "Tổng hợp theo model") || !strings.Contains(fit, "còn ") {
		t.Errorf("the summary stays and the hidden sessions are counted:\n%s", fit)
	}
	explicit, _ := reportText(s, usageView{Spec: "today", Top: 25}, now, 40, time.Time{})
	if !strings.Contains(explicit, "(25/30)") {
		t.Errorf("an explicit top is never shrunk:\n%s", explicit)
	}
	_ = body
}

func periodFixture(t *testing.T) (*sessionScanner, time.Time) {
	t.Helper()
	s, home := testScanner(t)
	// "now" is Monday 2026-11-09 12:00 local (UTC+7).
	writeTestFile(t, filepath.Join(home, ".claude", "projects", "p", "a.jsonl"),
		claudeLine("t1", "/w/game", "2026-11-09T01:00:00Z", 1000, 0, 0, 0)+ // today
			claudeLine("y1", "/w/game", "2026-11-08T01:00:00Z", 100, 0, 0, 0)+ // yesterday (Sunday): last week
			claudeLine("l1", "/w/tool", "2026-11-04T01:00:00Z", 10, 0, 0, 0)+ // last week, this month
			claudeLine("m1", "/w/tool", "2026-10-20T01:00:00Z", 1, 0, 0, 0)) // last month
	s.scan(time.Time{})
	return s, time.Date(2026, 11, 9, 12, 0, 0, 0, testZone)
}

func TestPeriodOverviewShowsTheDayWeekAndMonthOfTheView(t *testing.T) {
	s, now := periodFixture(t)
	rows := s.periodOverview(usageView{Spec: "today"}, now)
	if len(rows) != 3 {
		t.Fatalf("rows = %d", len(rows))
	}
	for i, want := range []struct {
		spec  string
		total int64
	}{{"today", 1000}, {"week", 1000}, {"month", 1110}} {
		if rows[i].Spec != want.spec || rows[i].Total != want.total || rows[i].Key != fmt.Sprint(i+1) {
			t.Errorf("row %d = %s %d, want %s %d", i, rows[i].Spec, rows[i].Total, want.spec, want.total)
		}
	}
	if !rows[0].Current || rows[1].Current || rows[2].Current {
		t.Errorf("only the day is current: %+v", rows)
	}
	// An older day: the rows follow it to its own week and month.
	rows = s.periodOverview(usageView{Spec: "2026-10-20", Project: "tool"}, now)
	if rows[0].Spec != "2026-10-20" || rows[1].Spec != "2026-W43" || rows[2].Spec != "lastmonth" || rows[2].Total != 1 {
		t.Errorf("rows of an old day = %+v", rows)
	}
	rows = s.periodOverview(usageView{Spec: "lastweek"}, now)
	if !rows[1].Current || rows[1].Total != 110 || rows[0].Spec != "2026-11-02" {
		t.Errorf("last week is the week row, its first day the day row: %+v", rows)
	}
}

func TestPeriodOverviewAddsARowForATypedRange(t *testing.T) {
	s, now := periodFixture(t)
	rows := s.periodOverview(usageView{Spec: "7d"}, now)
	if len(rows) != 4 || rows[3].Spec != "7d" || !rows[3].Current || rows[3].Key != "-" || rows[3].Total != 1110 {
		t.Fatalf("a typed range gets its own current row: %+v", rows)
	}
	if rows[0].Current || rows[1].Current || rows[2].Current {
		t.Errorf("no kind is current for a range")
	}
}

func TestReportTextStartsWithTheOverviewAndAChart(t *testing.T) {
	s, now := periodFixture(t)
	text, err := reportText(s, usageView{Spec: "week", Top: -1, Overview: true, Width: 120}, now, 0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(text, "Các kỳ") || strings.Index(text, "Token theo tuần") > strings.Index(text, "Token tuần này") {
		t.Errorf("the overview and the chart come first:\n%s", text)
	}
	for _, want := range []string{"1 day", "2 week", "3 month", "Token theo tuần", "W45", "W46", "▲", "█", "110"} {
		if !strings.Contains(text, want) {
			t.Errorf("report lacks %q:\n%s", want, text)
		}
	}
	chart := text[:strings.Index(text, "Token tuần này")]
	for _, line := range strings.Split(chart, "\n") {
		if n := len([]rune(line)); n > 120 {
			t.Errorf("a chart line is %d wide: %q", n, line)
		}
	}
	days, _ := reportText(s, usageView{Spec: "today", Top: -1, Overview: true, Width: 120}, now, 0, time.Time{})
	if !strings.Contains(days, "Token theo ngày") || !strings.Contains(days, "09/11") {
		t.Errorf("a day shows the chart of days:\n%s", days)
	}
	plain, _ := reportText(s, usageView{Spec: "today", Top: -1}, now, 0, time.Time{})
	if strings.Contains(plain, "Các kỳ") || strings.Contains(plain, "Token theo") {
		t.Errorf("no overview unless asked: %s", plain)
	}
}

func TestUnitSeriesCountsEachPeriod(t *testing.T) {
	s, now := periodFixture(t)
	first := time.Date(2026, 10, 1, 0, 0, 0, 0, testZone)
	if got := s.unitSeries(unitMonth, first, 2, "", ""); got[0] != 1 || got[1] != 1110 {
		t.Errorf("months = %v", got)
	}
	weeks := s.unitSeries(unitWeek, weekStart(midnight(now)).AddDate(0, 0, -21), 4, "", "")
	if weeks[0] != 1 || weeks[2] != 110 || weeks[3] != 1000 {
		t.Errorf("weeks = %v", weeks)
	}
}

func TestDayWeekMonthAndArrowsMoveThePeriod(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, testZone)
	v := usageView{Spec: "today", Top: -1, Overview: true, Project: "game"}
	start := func() time.Time {
		p, err := resolvePeriod(v.Spec, now)
		if err != nil {
			t.Fatalf("%q: %v", v.Spec, err)
		}
		return p.Start
	}
	d := func(m, day int) time.Time { return time.Date(2026, time.Month(m), day, 0, 0, 0, 0, testZone) }
	steps := []struct {
		line  string
		spec  string
		start time.Time
	}{
		{"prev", "yesterday", d(11, 8)},
		{"next", "today", d(11, 9)},
		{"2", "week", d(11, 9)},
		{"prev", "lastweek", d(11, 2)},
		{"<", "", d(10, 26)},
		{"day", "2026-10-26", d(10, 26)},
		{"next", "2026-10-27", d(10, 27)},
		{"month", "lastmonth", d(10, 1)},
		{">", "month", d(11, 1)},
		{"1", "today", d(11, 9)},
	}
	for _, step := range steps {
		if msg, quit := applyUsageCommand(&v, step.line, now); msg != "" || quit {
			t.Fatalf("%s: msg = %q", step.line, msg)
		}
		if (step.spec != "" && v.Spec != step.spec) || !start().Equal(step.start) || v.Project != "game" {
			t.Errorf("%s: spec %q starts %v, want %q %v", step.line, v.Spec, start(), step.spec, step.start)
		}
	}
	if msg, _ := applyUsageCommand(&v, "next", now); !strings.Contains(msg, "mới nhất") || v.Spec != "today" {
		t.Errorf("there is nothing after today: %q %q", msg, v.Spec)
	}
	v.Spec = "7d"
	applyUsageCommand(&v, "prev", now)
	if v.Spec != "2026-10-27..2026-11-02" {
		t.Errorf("a range steps by its own length: %q", v.Spec)
	}
	if msg, _ := applyUsageCommand(&v, "4", now); !strings.Contains(msg, "không hiểu") {
		t.Errorf("4 is not a period: %q", msg)
	}
	if _, quit := applyUsageCommand(&v, "0", now); !quit {
		t.Error("0 still quits")
	}
	applyUsageCommand(&v, "open", now)
	applyUsageCommand(&v, "reset", now)
	if v.Open || v.Spec != "today" || v.Project != "" || !v.Overview {
		t.Errorf("reset: %+v", v)
	}
}

func TestISOWeekSpec(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, testZone)
	for spec, want := range map[string]string{"2026-W46": "2026-11-09", "2026-w01": "2025-12-29", "2020-W53": "2020-12-28"} {
		p, err := resolvePeriod(spec, now)
		if err != nil || p.Start.Format("2006-01-02") != want || unitOf(p) != unitWeek {
			t.Errorf("%s = %v (%v), want %s", spec, p.Start, err, want)
		}
	}
	for _, bad := range []string{"2026-W54", "2026-W0", "2026-W1"} {
		if _, err := resolvePeriod(bad, now); err == nil {
			t.Errorf("%s should not parse", bad)
		}
	}
}

func TestSessionRowsCarryEveryTokenKind(t *testing.T) {
	s, home := testScanner(t)
	projects := filepath.Join(home, ".claude", "projects", "p")
	writeTestFile(t, filepath.Join(projects, "main1.jsonl"), claudeLine("a1", "/w/game", "2026-11-09T01:00:00Z", 10, 20, 300, 4000))
	writeTestFile(t, filepath.Join(projects, "main1", "subagents", "agent-w1.jsonl"), claudeLine("s1", "/w/game", "2026-11-09T02:00:00Z", 1, 2, 30, 400))
	s.scan(time.Time{})
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, testZone)
	text, err := reportText(s, usageView{Spec: "2026-11-09", Top: -1, Agents: true}, now, 0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var header, main, worker string
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "SESSION"):
			header = line
		case strings.HasPrefix(line, "main1"):
			main = line
		case strings.Contains(line, "└ w1"):
			worker = line
		}
	}
	for _, h := range []string{"VÀO", "RA", "ĐỌC CACHE", "GHI CACHE", "TỔNG", "QUY ĐỔI"} {
		if !strings.Contains(header, h) {
			t.Errorf("header lacks %s: %q", h, header)
		}
	}
	// The session's row adds its own calls and its worker's: 11 in, 22 out, 330 read, 4400 written.
	for _, want := range []string{" 11 ", " 22 ", "330", "4.4k", "4.8k"} {
		if !strings.Contains(main, want) {
			t.Errorf("session row lacks %q: %q", want, main)
		}
	}
	for _, want := range []string{" 1 ", " 2 ", "30", "400", "433"} {
		if !strings.Contains(worker, want) {
			t.Errorf("worker row lacks %q: %q", want, worker)
		}
	}
}

func TestFitColumnsCutsTheTitleThenDropsLowPriorityColumns(t *testing.T) {
	headers := []string{"ID", "N", "TOKENS", "MODEL", "TITLE"}
	left := []bool{true, false, false, true, true}
	priority := []int{9, 5, 9, 1, 0}
	rows := [][]string{
		{"a1", "100", "12.3M", "opus-5-5", "a very long title that takes a lot of room"},
		{"b2", "7", "1.1M", "sonnet-5-5", "short"},
	}
	width := func(h []string, r [][]string) int {
		w := 2 * (len(h) - 1)
		for i := range h {
			col := utf8.RuneCountInString(h[i])
			for _, row := range r {
				col = max(col, utf8.RuneCountInString(row[i]))
			}
			w += col
		}
		return w
	}
	h, _, r, hidden := fitColumns(headers, left, rows, priority, 0)
	if len(h) != 5 || len(hidden) != 0 || r[0][4] != rows[0][4] {
		t.Errorf("width 0 changes nothing: %v %v", h, hidden)
	}
	h, _, r, hidden = fitColumns(headers, left, rows, priority, 45)
	if len(hidden) != 0 || width(h, r) > 45 || utf8.RuneCountInString(r[0][4]) >= utf8.RuneCountInString(rows[0][4]) {
		t.Errorf("only the title is cut to fit 45: %v, widest %d, title %q", hidden, width(h, r), r[0][4])
	}
	h, _, r, hidden = fitColumns(headers, left, rows, priority, 20)
	if strings.Join(hidden, ",") != "TITLE,MODEL" || width(h, r) > 30 {
		t.Errorf("the title goes first, then the model: hidden %v, widest %d, %v", hidden, width(h, r), h)
	}
	h, _, _, hidden = fitColumns(headers, left, rows, priority, 5)
	if strings.Join(h, ",") != "ID,TOKENS" || strings.Join(hidden, ",") != "TITLE,MODEL,N" {
		t.Errorf("protected columns stay even when too wide: %v hidden %v", h, hidden)
	}
}

func TestReportFitsTheWidthItIsGiven(t *testing.T) {
	s, home := testScanner(t)
	writeTestFile(t, filepath.Join(home, ".claude", "projects", "p", "abcdef12.jsonl"),
		`{"type":"ai-title","aiTitle":"A rather long chat title that needs trimming","sessionId":"abcdef12"}`+"\n"+
			claudeLine("a1", "/w/endlesswinter", "2026-11-09T01:00:00Z", 59900, 233500, 11300000, 732800))
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, testZone)
	for _, w := range []int{0, 140, 100, 80, 70} {
		text, err := reportText(s, usageView{Spec: "2026-11-09", Top: -1, Width: w}, now, 0, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, "SESSION") || strings.HasPrefix(line, "abcdef12") {
				if w > 0 && utf8.RuneCountInString(line) > w {
					t.Errorf("width %d: line is %d wide: %q", w, utf8.RuneCountInString(line), line)
				}
				if !strings.Contains(line, "VÀO") && strings.HasPrefix(line, "SESSION") && w >= 70 {
					t.Errorf("width %d lost the token columns: %q", w, line)
				}
			}
		}
		if w > 0 && w < 100 && !strings.Contains(text, "cửa sổ hẹp") {
			t.Errorf("width %d should say what it hid", w)
		}
		if w == 0 && strings.Contains(text, "cửa sổ hẹp") {
			t.Errorf("no width limit, nothing hidden")
		}
	}
}
