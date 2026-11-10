package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func keys(t *testing.T, raw string) []keyEvent {
	t.Helper()
	r := bufio.NewReader(strings.NewReader(raw))
	var out []keyEvent
	for {
		k := readKey(r)
		if k.kind == keyEOF {
			return out
		}
		out = append(out, k)
	}
}

func TestReadKeyParsesArrowsAndControlKeys(t *testing.T) {
	got := keys(t, "a\x1b[A\x1b[B\x1b[5~\x1b[6~\x1b[H\x1b[F\x1bOA\t\r\n\x7f\x03\x15\x04é")
	want := []keyKind{keyRune, keyUp, keyDown, keyPgUp, keyPgDn, keyHome, keyEnd, keyUp, keyTab, keyEnter, keyBackspace, keyCtrlC, keyCtrlU, keyCtrlD, keyRune}
	if len(got) != len(want) {
		t.Fatalf("got %d keys: %+v", len(got), got)
	}
	for i, k := range got {
		if k.kind != want[i] {
			t.Errorf("key %d = %v, want %v", i, k.kind, want[i])
		}
	}
	if got[0].r != 'a' || got[len(got)-1].r != 'é' {
		t.Errorf("runes = %q, %q", got[0].r, got[len(got)-1].r)
	}
}

func TestReadKeyTellsALoneEscFromASequenceAndCRLFIsOneEnter(t *testing.T) {
	if got := keys(t, "\x1b"); len(got) != 1 || got[0].kind != keyEsc {
		t.Errorf("lone esc = %+v", got)
	}
	if got := keys(t, "\r\n"); len(got) != 1 || got[0].kind != keyEnter {
		t.Errorf("CRLF = %+v", got)
	}
	if got := keys(t, "\x1b[200~"); len(got) != 1 || got[0].kind != keyOther {
		t.Errorf("unknown sequence = %+v", got)
	}
}

var uiNow = time.Date(2026, 11, 9, 12, 0, 0, 0, testZone)

func uiFixture(t *testing.T) *sessionScanner {
	t.Helper()
	s, home := testScanner(t)
	writeTestFile(t, filepath.Join(home, ".claude", "projects", "p", "a.jsonl"),
		claudeLine("t1", "/w/game", "2026-11-09T01:00:00Z", 1000, 0, 0, 0)+claudeLine("t2", "/w/tool", "2026-11-09T02:00:00Z", 300, 0, 0, 0)+
			claudeLine("l1", "/w/game", "2026-11-04T01:00:00Z", 50, 0, 0, 0))
	s.scan(time.Time{})
	return s
}

func suggestionNames(list []usageSuggestion) string {
	var n []string
	for _, x := range list {
		n = append(n, x.Name)
	}
	return strings.Join(n, ",")
}

func TestSuggestionsForAnEmptyPromptListPeriodsThenCommands(t *testing.T) {
	s := uiFixture(t)
	v := usageView{Spec: "week", Top: -1, Overview: true}
	list, hint := buildSuggestions("", v, s.periodOverview(v, uiNow), nil, uiNow)
	want := "day,week,month,analytics,agents,open,blocks,limit,top,project,app,reset,help,quit"
	if suggestionNames(list) != want || hint == "" {
		t.Errorf("names = %s, hint = %q", suggestionNames(list), hint)
	}
	if !list[1].Mark || list[0].Mark || !strings.HasPrefix(list[1].Stats, "1 session · 1.3k") || !strings.Contains(list[1].Desc, "tuần này") {
		t.Errorf("week is marked and carries its numbers: %+v / %+v", list[1], list[0])
	}
	if list[3].Apply != "analytics on" || list[4].Apply != "agents on" || list[5].Apply != "open on" || list[6].Apply != "blocks on" || list[8].Apply != "" || !list[8].NeedsArg {
		t.Errorf("agents toggles, top needs a value: %+v %+v", list[3], list[4])
	}
}

func TestSuggestionsNarrowAsYouType(t *testing.T) {
	s := uiFixture(t)
	v := usageView{Spec: "today", Top: -1, Overview: true}
	rows := s.periodOverview(v, uiNow)
	cases := map[string]string{
		"w":          "week",
		"/we":        "week",
		"last":       "lastweek,lastmonth",
		"to":         "today,top",
		"2":          "week",
		"3":          "month",
		"y":          "yesterday",
		"m":          "month",
		"WEEK":       "week",
		"q":          "quit",
		"p":          "project",
		"zzz":        "",
		"2026-11-05": "2026-11-05",
		"14d":        "14d",
	}
	for in, want := range cases {
		list, _ := buildSuggestions(in, v, rows, nil, uiNow)
		if got := suggestionNames(list); got != want {
			t.Errorf("%q gives %q, want %q", in, got, want)
		}
	}
	if _, hint := buildSuggestions("zzz", v, rows, nil, uiNow); !strings.Contains(hint, "không có lệnh nào khớp") {
		t.Errorf("hint = %q", hint)
	}
}

func TestSuggestionsOfAnArgumentListItsValues(t *testing.T) {
	s := uiFixture(t)
	p, _ := resolvePeriod("week", uiNow)
	projects := s.projectStats(p, "")
	if len(projects) != 2 || projects[0].Name != "game" {
		t.Fatalf("projects = %+v", projects)
	}
	v := usageView{Spec: "week", Top: -1, Overview: true, Project: "tool"}
	rows := s.periodOverview(v, uiNow)
	list, _ := buildSuggestions("project ", v, rows, projects, uiNow)
	if suggestionNames(list) != "all,game,tool" || !list[2].Mark || list[0].Mark {
		t.Fatalf("project values = %+v", list)
	}
	if list[1].Apply != "project game" || !strings.Contains(list[1].Desc, "token") || list[1].Stats == "" {
		t.Errorf("a project line applies the command and shows its size: %+v", list[1])
	}
	if got, _ := buildSuggestions("project to", v, rows, projects, uiNow); suggestionNames(got) != "tool" {
		t.Errorf("filter by typing = %+v", got)
	}
	for in, want := range map[string]string{"app ": "all,claude,codex,opencode,antigravity", "top ": "auto,10,20,30,50,100,0", "agents ": "on,off", "top 7": "7"} {
		list, _ := buildSuggestions(in, v, rows, nil, uiNow)
		if got := suggestionNames(list); got != want {
			t.Errorf("%q gives %s, want %s", in, got, want)
		}
	}
	if list, hint := buildSuggestions("help x", v, rows, nil, uiNow); list != nil || !strings.Contains(hint, "không có tham số") {
		t.Errorf("a command without values: %v, %q", list, hint)
	}
}

// press feeds keys through usageUILoop and returns the view chosen and everything drawn.
func press(t *testing.T, s *sessionScanner, v usageView, raw string) (usageView, string) {
	t.Helper()
	var out bytes.Buffer
	final, err := usageUILoop(readerKeys{bufio.NewReader(strings.NewReader(raw))}, &out, s, v, time.Time{}, func() (int, int) { return 120, 40 }, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return final, out.String()
}

func TestArrowsPreviewTheReportAndEnterKeepsIt(t *testing.T) {
	s := uiFixture(t)
	start := usageView{Spec: "today", Top: -1, Overview: true}
	// Tab enters the list on the line in use (day); two Downs reach "month".
	final, drawn := press(t, s, start, "\t\x1b[B\x1b[B\r")
	if final.Spec != "month" {
		t.Fatalf("spec = %q, want month", final.Spec)
	}
	frames := strings.Split(drawn, "\x1b[H")
	previews := 0
	for _, f := range frames {
		if strings.Contains(f, "Đang xem trước: ") {
			previews++
		}
	}
	if previews < 3 {
		t.Errorf("every arrow press should draw a preview, got %d", previews)
	}
	// The frame drawn while "month" was highlighted already shows this month's report.
	found := false
	for _, f := range frames {
		if strings.Contains(f, "Đang xem trước: month") && strings.Contains(f, "Token tháng này") {
			found = true
		}
	}
	if !found {
		t.Errorf("highlighting month must redraw the report for this month")
	}
}

func TestArrowsOutsideTheListScrollAndStepThePeriod(t *testing.T) {
	s := uiFixture(t)
	start := usageView{Spec: "today", Top: -1, Overview: true}
	// Without Tab, ↓ (a mouse wheel too) scrolls the report and never changes the view.
	if final, drawn := press(t, s, start, "\x1b[B\x1b[B\x1b[B\r"); final.Spec != "today" || strings.Contains(drawn, "Đang xem trước") {
		t.Errorf("↓ outside the list changed the view: %q", final.Spec)
	}
	// press runs on the real clock, so the dates are counted from today.
	today := midnight(time.Now())
	twoMonthsBack := time.Date(today.Year(), today.Month()-2, 1, 0, 0, 0, 0, today.Location())
	for raw, want := range map[string]string{
		"\x1b[D":             "yesterday",
		"\x1b[D\x1b[D":       today.AddDate(0, 0, -2).Format("2006-01-02"),
		"\x1b[D\x1b[C":       "today",
		"\x1b[C":             "today", // nothing after today
		"2\r\x1b[D":          "lastweek",
		"3\r\x1b[D\x1b[D":    twoMonthsBack.Format("2006-01"),
		"3\r\x1b[D\x1b[D1\r": twoMonthsBack.Format("2006-01-02"),
		"\t\x1b[B\x1b[D":     "yesterday", // ← leaves the list and steps the period in use
	} {
		if final, _ := press(t, s, start, raw); final.Spec != want {
			t.Errorf("%q gives %q, want %q", raw, final.Spec, want)
		}
	}
}

func TestEscapeDropsThePreviewWithoutChangingTheView(t *testing.T) {
	s := uiFixture(t)
	start := usageView{Spec: "today", Top: -1, Overview: true}
	if final, _ := press(t, s, start, "\x1b[B\x1b[B\x1b[B"); final.Spec != "today" {
		t.Errorf("arrows alone never change the chosen view: %q", final.Spec)
	}
	// Esc drops the highlight, then Esc again leaves; the view stays what it was.
	if final, _ := press(t, s, start, "\x1b[B\x1b[B\x1b"+"\x1b"); final.Spec != "today" {
		t.Errorf("Esc leaves the view unchanged: %q", final.Spec)
	}
}

func TestTypingFiltersThenEnterAppliesTheFirstMatch(t *testing.T) {
	s := uiFixture(t)
	start := usageView{Spec: "today", Top: -1, Overview: true}
	for raw, want := range map[string]string{"lastm\r": "lastmonth", "2026-11-05\r": "2026-11-05", "2\r": "week", "wee\t\r": "week"} {
		if final, _ := press(t, s, start, raw); final.Spec != want {
			t.Errorf("%q gives %q, want %q", raw, final.Spec, want)
		}
	}
}

func TestCommandsWithValuesOpenTheirListAndApplyOnEnter(t *testing.T) {
	s := uiFixture(t)
	start := usageView{Spec: "week", Top: -1, Overview: true}
	// "project" + Enter opens the project list with its first value ("all") highlighted;
	// each Down moves one line (game, then tool); Enter applies the highlighted one.
	for raw, want := range map[string]string{"project\r\r": "", "project\r\x1b[B\r": "game", "project\r\x1b[B\x1b[B\r": "tool"} {
		if final, _ := press(t, s, start, raw); final.Project != want {
			t.Errorf("%q gives project %q, want %q", raw, final.Project, want)
		}
	}
	final, _ := press(t, s, usageView{Spec: "week", Top: -1, Overview: true, Project: "tool"}, "project\r\r")
	if final.Project != "" {
		t.Errorf("choosing all clears the filter, got %q", final.Project)
	}
	if final, _ = press(t, s, start, "agents\r"); !final.Agents {
		t.Errorf("agents should turn on: %+v", final)
	}
	if final, _ = press(t, s, usageView{Spec: "week", Top: -1, Overview: true, Agents: true}, "agents\r"); final.Agents {
		t.Errorf("agents should turn off: %+v", final)
	}
	if final, _ = press(t, s, start, "top 50\r"); final.Top != 50 {
		t.Errorf("top = %d", final.Top)
	}
	if final, _ = press(t, s, usageView{Spec: "week", Top: 30, Project: "game", Overview: true}, "reset\r"); final.Spec != "today" || final.Project != "" || final.Top != -1 {
		t.Errorf("reset = %+v", final)
	}
}

func TestQuitKeysAndTheHelpScreen(t *testing.T) {
	s := uiFixture(t)
	start := usageView{Spec: "today", Top: -1, Overview: true}
	for _, raw := range []string{"\x03", "\x1b", "q\r", "quit\r", "\x04"} {
		if final, _ := press(t, s, start, raw+"week\r"); final.Spec != "today" {
			t.Errorf("%q should quit before anything else: %q", raw, final.Spec)
		}
	}
	_, drawn := press(t, s, start, "help\rx")
	if !strings.Contains(drawn, "Phím:") || !strings.Contains(drawn, "chốt lệnh") {
		t.Errorf("help screen missing")
	}
}

func TestFrameFitsTheScreenAndScrollsTheReport(t *testing.T) {
	ui := &usageUI{sel: -1}
	report := make([]string, 100)
	for i := range report {
		report[i] = "line " + string(rune('A'+i%26))
	}
	list := []usageSuggestion{{Name: "today", Desc: "hôm nay", Stats: "1 session"}, {Name: "week", Desc: "tuần này", Mark: true}}
	frame := renderUsageFrame(ui, report, list, "gợi ý", 80, 30)
	rows := strings.Split(strings.ReplaceAll(frame, "\x1b[K", ""), "\r\n")
	if len(rows) != 30 { // 29 full lines and the prompt line
		t.Errorf("rows = %d", len(rows))
	}
	if !strings.Contains(rows[len(rows)-1], "> ") || !strings.Contains(frame, "PgUp/PgDn") {
		t.Errorf("prompt on the last line and a scroll hint")
	}
	ui.handle(keyEvent{kind: keyPgDn}, list, uiNow)
	if ui.scroll == 0 || ui.page < 5 {
		t.Errorf("scroll = %d, page = %d", ui.scroll, ui.page)
	}
	ui.handle(keyEvent{kind: keyEnd}, list, uiNow)
	renderUsageFrame(ui, report, list, "", 80, 30)
	if want := 100 - (30 - (8 + 3)); ui.scroll != want {
		t.Errorf("End scrolls to the last page: scroll = %d, want %d", ui.scroll, want)
	}
	ui.handle(keyEvent{kind: keyHome}, list, uiNow)
	if ui.scroll != 0 {
		t.Errorf("Home gives %d", ui.scroll)
	}
	for _, line := range rows {
		if n := len([]rune(stripEscapes(line))); n > 80 {
			t.Errorf("a line is %d wide: %q", n, line)
		}
	}
}

func stripEscapes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			i++
			for i < len(s) && !(s[i] >= 'A' && s[i] <= 'Z' || s[i] >= 'a' && s[i] <= 'z') {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestHighlightedLineIsDrawnInverseAndLongListsScroll(t *testing.T) {
	var list []usageSuggestion
	for i := 0; i < 20; i++ {
		list = append(list, usageSuggestion{Name: "p" + string(rune('a'+i)), Desc: "d"})
	}
	start, end := listWindow(15, len(list), 8)
	if start != 8 || end != 16 {
		t.Errorf("window = %d..%d", start, end)
	}
	rows := suggestionRows(list, start, end, 15, 60)
	if len(rows) != 8 || !strings.Contains(rows[7], "\x1b[7m") || !strings.Contains(rows[7], "› ") || strings.Contains(rows[0], "\x1b[7m") {
		t.Errorf("only the highlighted row is inverse: %q", rows)
	}
	if s, e := listWindow(-1, 5, 8); s != 0 || e != 5 {
		t.Errorf("a short list is drawn whole: %d..%d", s, e)
	}
}

func TestScreenNeedsARealTerminalFile(t *testing.T) {
	s, _ := testScanner(t)
	var out bytes.Buffer
	// A buffer is not a console, so the caller gets ok=false and falls back to typed lines.
	if ok, err := runUsageScreen(nil, &out, s, usageView{Spec: "today"}, time.Time{}); ok || err != nil {
		t.Errorf("ok = %v, err = %v", ok, err)
	}
}

// scriptedKeys plays a script of steps: a key, or a timeout (ok = false). After each
// step, onStep may change the world (add a call to a file). When the script ends the
// source reports end of input.
type scriptedKeys struct {
	steps  []scriptStep
	timers int
}

type scriptStep struct {
	key    keyEvent
	ok     bool
	before func() // runs before this step is returned
}

func (s *scriptedKeys) next(time.Duration) (keyEvent, bool) {
	if len(s.steps) == 0 {
		return keyEvent{kind: keyEOF}, true
	}
	step := s.steps[0]
	s.steps = s.steps[1:]
	if step.before != nil {
		step.before()
	}
	if !step.ok {
		s.timers++
	}
	return step.key, step.ok
}

func framesOf(drawn string) []string {
	parts := strings.Split(drawn, "\x1b[H")
	return parts[1:]
}

func TestScreenRefreshesByItselfOnlyWhenNewCallsArrive(t *testing.T) {
	s, home := testScanner(t)
	file := filepath.Join(home, ".claude", "projects", "p", "a.jsonl")
	today := time.Now().UTC().Format(time.RFC3339)
	writeTestFile(t, file, claudeLine("m1", "/w/game", today, 1000, 0, 0, 0))
	var out bytes.Buffer
	appendCall := func() {
		old, _ := os.ReadFile(file)
		writeTestFile(t, file, string(old)+claudeLine("m2", "/w/game", today, 500, 0, 0, 0))
	}
	keys := &scriptedKeys{steps: []scriptStep{
		{ok: false},                             // timeout, nothing new: no new frame
		{ok: false},                             // timeout, nothing new: no new frame
		{ok: false, before: appendCall},         // timeout, a call arrived: a new frame
		{ok: false},                             // timeout, nothing new again
		{key: keyEvent{kind: keyEsc}, ok: true}, // Esc with an empty prompt quits
	}}
	if _, err := usageUILoop(keys, &out, s, usageView{Spec: "today", Top: -1, Overview: true}, time.Time{}, func() (int, int) { return 120, 40 }, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	frames := framesOf(out.String())
	if len(frames) != 2 {
		t.Fatalf("frames = %d, want 2: the first one and the one after the new call", len(frames))
	}
	if !strings.Contains(frames[0], "1 session") && !strings.Contains(frames[0], "1.0k") {
		t.Errorf("first frame should show the first call:\n%s", frames[0])
	}
	if !strings.Contains(frames[1], "1.5k") || strings.Contains(frames[0], "1.5k") {
		t.Errorf("second frame should show both calls (1.5k), the first only one")
	}
	if keys.timers != 4 {
		t.Errorf("timeouts consumed = %d", keys.timers)
	}
}

func TestRefreshRedrawsWhenTheWindowIsResized(t *testing.T) {
	s := uiFixture(t)
	var out bytes.Buffer
	width := 120
	keys := &scriptedKeys{steps: []scriptStep{
		{ok: false, before: func() { width = 90 }}, // the window got narrower during the wait
		{key: keyEvent{kind: keyEsc}, ok: true},
	}}
	_, err := usageUILoop(keys, &out, s, usageView{Spec: "week", Top: -1, Overview: true}, time.Time{}, func() (int, int) { return width, 40 }, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(framesOf(out.String())); got != 2 {
		t.Errorf("frames = %d, want 2 (a resize draws again)", got)
	}
}

func TestFrameShowsWhenTheNumbersWereReadAndThatTheyRenew(t *testing.T) {
	ui := &usageUI{sel: -1, updated: time.Date(2026, 11, 10, 9, 5, 7, 0, time.Local)}
	frame := stripEscapes(renderUsageFrame(ui, []string{"report"}, nil, "", 130, 30))
	if !strings.Contains(frame, "cập nhật 09:05:07") || !strings.Contains(frame, "tự làm mới mỗi 5s") {
		t.Errorf("the clock is missing:\n%s", frame)
	}
	narrow := stripEscapes(renderUsageFrame(ui, []string{"report"}, nil, "", 60, 30))
	if strings.Contains(narrow, "tự làm mới") {
		t.Errorf("the clock is dropped when the line has no room for it:\n%s", narrow)
	}
	if usageRefreshEvery < 5*time.Second || usageRefreshEvery > 10*time.Second {
		t.Errorf("the refresh is meant to be 5-10 s, got %v", usageRefreshEvery)
	}
}

func TestRefreshKeepsWhatTheUserIsDoing(t *testing.T) {
	s, home := testScanner(t)
	file := filepath.Join(home, ".claude", "projects", "p", "a.jsonl")
	today := time.Now().UTC().Format(time.RFC3339)
	writeTestFile(t, file, claudeLine("m1", "/w/game", today, 1000, 0, 0, 0)+claudeLine("m2", "/w/tool", today, 100, 0, 0, 0))
	var out bytes.Buffer
	appendCall := func() { // a call for the project "tool" arrives while its line is highlighted
		old, _ := os.ReadFile(file)
		writeTestFile(t, file, string(old)+claudeLine("m3", "/w/tool", today, 500, 0, 0, 0))
	}
	key := func(k keyKind) scriptStep { return scriptStep{key: keyEvent{kind: k}, ok: true} }
	var steps []scriptStep
	for _, r := range "project " {
		steps = append(steps, scriptStep{key: keyEvent{kind: keyRune, r: r}, ok: true})
	}
	// Typing highlights "all"; two Downs reach "tool". Then a timeout brings the new call.
	steps = append(steps, key(keyDown), key(keyDown), scriptStep{ok: false, before: appendCall}, key(keyEsc), key(keyEsc), key(keyEsc))
	final, err := usageUILoop(&scriptedKeys{steps: steps}, &out, s, usageView{Spec: "today", Top: -1, Overview: true}, time.Time{}, func() (int, int) { return 120, 40 }, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if final.Project != "" {
		t.Errorf("nothing was chosen: %+v", final)
	}
	var before, after string
	for _, f := range framesOf(out.String()) {
		plain := stripEscapes(f)
		switch {
		case strings.Contains(plain, "Đang xem trước: tool") && strings.Contains(plain, "Tổng: 1 lượt, 100 token"):
			before = plain
		case strings.Contains(plain, "Đang xem trước: tool") && strings.Contains(plain, "Tổng: 2 lượt, 600 token"):
			after = plain
		}
	}
	if before == "" || after == "" {
		t.Fatalf("expected a frame before (100 tokens) and one after (600 tokens) the new call; before=%v after=%v", before != "", after != "")
	}
	lines := strings.Split(after, "\n")
	if prompt := strings.TrimSpace(lines[len(lines)-1]); prompt != "> project" {
		t.Errorf("the typed text must survive the refresh, prompt = %q", prompt)
	}
	if !strings.Contains(after, "› ") || !strings.Contains(after, "tool") {
		t.Errorf("the highlighted line must survive the refresh:\n%s", after)
	}
}
