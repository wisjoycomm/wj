package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

// The interactive `wj usage`: the report fills the screen, a list of commands sits
// above the prompt, and moving in the list shows its result straight away (↑↓ preview,
// Enter keeps it). Typing narrows the list; Tab completes.

type keyKind int

const (
	keyRune keyKind = iota
	keyEnter
	keyTab
	keyBackspace
	keyEsc
	keyUp
	keyDown
	keyLeft
	keyRight
	keyPgUp
	keyPgDn
	keyHome
	keyEnd
	keyCtrlC
	keyCtrlD
	keyCtrlU
	keyCtrlL
	keyOther
	keyEOF
)

type keyEvent struct {
	kind keyKind
	r    rune
}

// readKey reads one key press from a terminal in raw mode. Arrow and paging keys
// arrive as escape sequences; a lone Esc is told apart because nothing follows it in
// the same read.
func readKey(r *bufio.Reader) keyEvent {
	b, err := r.ReadByte()
	if err != nil {
		return keyEvent{kind: keyEOF}
	}
	switch b {
	case '\r':
		if r.Buffered() > 0 {
			if next, _ := r.Peek(1); len(next) == 1 && next[0] == '\n' {
				_, _ = r.ReadByte()
			}
		}
		return keyEvent{kind: keyEnter}
	case '\n':
		return keyEvent{kind: keyEnter}
	case '\t':
		return keyEvent{kind: keyTab}
	case 0x7f, 0x08:
		return keyEvent{kind: keyBackspace}
	case 0x03:
		return keyEvent{kind: keyCtrlC}
	case 0x04:
		return keyEvent{kind: keyCtrlD}
	case 0x15:
		return keyEvent{kind: keyCtrlU}
	case 0x0c:
		return keyEvent{kind: keyCtrlL}
	case 0x1b:
		return readEscape(r)
	}
	if b < 0x20 {
		return keyEvent{kind: keyOther}
	}
	_ = r.UnreadByte()
	ch, _, err := r.ReadRune()
	if err != nil {
		return keyEvent{kind: keyEOF}
	}
	return keyEvent{kind: keyRune, r: ch}
}

func readEscape(r *bufio.Reader) keyEvent {
	if r.Buffered() == 0 {
		return keyEvent{kind: keyEsc}
	}
	next, _ := r.ReadByte()
	if next != '[' && next != 'O' {
		_ = r.UnreadByte()
		return keyEvent{kind: keyEsc}
	}
	var params []byte
	for {
		c, err := r.ReadByte()
		if err != nil {
			return keyEvent{kind: keyOther}
		}
		if c >= 0x40 && c <= 0x7e { // the final byte
			params = append(params, c)
			break
		}
		params = append(params, c)
	}
	switch string(params) {
	case "A":
		return keyEvent{kind: keyUp}
	case "B":
		return keyEvent{kind: keyDown}
	case "C":
		return keyEvent{kind: keyRight}
	case "D":
		return keyEvent{kind: keyLeft}
	case "H", "1~", "7~":
		return keyEvent{kind: keyHome}
	case "F", "4~", "8~":
		return keyEvent{kind: keyEnd}
	case "5~":
		return keyEvent{kind: keyPgUp}
	case "6~":
		return keyEvent{kind: keyPgDn}
	}
	return keyEvent{kind: keyOther}
}

// usageSuggestion is one line of the list above the prompt.
type usageSuggestion struct {
	Name     string // first column
	Insert   string // what Tab puts on the prompt
	Apply    string // command applied when this line is previewed or chosen; "" = needs an argument first
	Desc     string
	Stats    string // right column: sessions · tokens · cost
	Mark     bool   // this is the value in use now
	NeedsArg bool   // choosing it opens the list of its values
}

type projectStat struct {
	Name  string
	Total int64
	Cost  string
}

// projectStats lists the projects with calls in the period, the biggest first.
func (s *sessionScanner) projectStats(p usagePeriod, client string) []projectStat {
	rows := map[string]*usageRow{}
	for _, call := range s.calls {
		if !callInPeriod(call, p, "", client) {
			continue
		}
		row := rows[call.Project]
		if row == nil {
			row = &usageRow{}
			rows[call.Project] = row
		}
		row.add(call)
	}
	list := make([]projectStat, 0, len(rows))
	for name, row := range rows {
		list = append(list, projectStat{Name: name, Total: row.Total, Cost: costText(*row)})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Total != list[j].Total {
			return list[i].Total > list[j].Total
		}
		return list[i].Name < list[j].Name
	})
	return list
}

func commandName(word string) string {
	switch w := strings.ToLower(word); w {
	case "p":
		return "project"
	case "a":
		return "app"
	case "agent", "workers":
		return "agents"
	case "h", "?":
		return "help"
	case "q", "exit", "0":
		return "quit"
	default:
		return w
	}
}

func statsText(r overviewRow) string {
	if r.Calls == 0 {
		return "-"
	}
	return fmt.Sprintf("%d session · %s · %s", r.Sessions, compactTokens(r.Total), costText(r.usageRow))
}

func topText(v usageView) string {
	switch {
	case v.Top == 0:
		return "tất cả"
	case v.Top > 0:
		return strconv.Itoa(v.Top)
	}
	return "tự vừa"
}

// baseSuggestions are the commands shown when the prompt is empty.
func baseSuggestions(v usageView, rows []overviewRow) []usageSuggestion {
	var list []usageSuggestion
	current := canonicalSpec(v.Spec)
	for _, r := range rows {
		if r.Key == "-" {
			continue
		}
		list = append(list, usageSuggestion{Name: r.Spec, Insert: r.Spec, Apply: r.Spec,
			Desc: r.Name + "  " + rangeText(r.Period), Stats: statsText(r), Mark: r.Spec == current})
	}
	agents := usageSuggestion{Name: "agents", Insert: "agents", Mark: v.Agents}
	if v.Agents {
		agents.Apply, agents.Desc = "agents off", "đang bật: tắt liệt kê từng worker dưới session"
	} else {
		agents.Apply, agents.Desc = "agents on", "liệt kê từng worker (subagent) dưới session của nó"
	}
	project := "tất cả"
	if v.Project != "" {
		project = v.Project
	}
	client := "tất cả"
	if v.Client != "" {
		client = v.Client
	}
	return append(list, agents,
		usageSuggestion{Name: "top", Insert: "top ", NeedsArg: true, Desc: "số session hiện ra (đang: " + topText(v) + ")", Mark: v.Top >= 0},
		usageSuggestion{Name: "project", Insert: "project ", NeedsArg: true, Desc: "lọc theo project (đang: " + project + ")", Mark: v.Project != ""},
		usageSuggestion{Name: "app", Insert: "app ", NeedsArg: true, Desc: "lọc theo app claude/codex (đang: " + client + ")", Mark: v.Client != ""},
		usageSuggestion{Name: "reset", Insert: "reset", Apply: "reset", Desc: "về hôm nay, bỏ hết bộ lọc"},
		usageSuggestion{Name: "help", Insert: "help", Apply: "help", Desc: "các phím và lệnh"},
		usageSuggestion{Name: "quit", Insert: "quit", Apply: "q", Desc: "thoát (Esc hoặc Ctrl+C cũng được)"})
}

const periodTypingHint = "gõ thêm để xem kỳ khác: 2026-11-05 · 2026-11 · 14d · 2026-11-01..2026-11-09"

// buildSuggestions is the list under the current prompt text, plus a one-line hint.
func buildSuggestions(input string, v usageView, rows []overviewRow, projects []projectStat, now time.Time) ([]usageSuggestion, string) {
	text := strings.TrimLeft(input, " /")
	word, rest, hasArg := strings.Cut(text, " ")
	cmd := commandName(word)
	rest = strings.TrimSpace(rest)
	if !hasArg {
		var list []usageSuggestion
		for _, s := range baseSuggestions(v, rows) {
			if cmd == "" || strings.HasPrefix(s.Name, cmd) || cmd == commandName(s.Name) {
				list = append(list, s)
			}
		}
		for i, r := range rows { // the keys 1-6 still work
			if cmd == strconv.Itoa(i+1) && r.Key != "-" {
				list = []usageSuggestion{{Name: r.Spec, Insert: r.Spec, Apply: r.Spec, Desc: r.Name + "  " + rangeText(r.Period), Stats: statsText(r), Mark: r.Current}}
			}
		}
		if cmd != "" {
			if p, err := resolvePeriod(word, now); err == nil && !isBaseSpec(word) && !(len(list) == 1 && list[0].Apply == word) {
				list = append([]usageSuggestion{{Name: word, Insert: word, Apply: word, Desc: p.Label}}, list...)
			}
		}
		if len(list) == 0 {
			return nil, fmt.Sprintf("không có lệnh nào khớp %q — %s", word, periodTypingHint)
		}
		if cmd == "" {
			return list, periodTypingHint
		}
		return list, ""
	}
	option := func(name, desc, stats string, mark bool) usageSuggestion {
		return usageSuggestion{Name: name, Insert: cmd + " " + name, Apply: cmd + " " + name, Desc: desc, Stats: stats, Mark: mark}
	}
	has := func(name string) bool {
		return rest == "" || strings.Contains(strings.ToLower(name), strings.ToLower(rest))
	}
	var list []usageSuggestion
	switch cmd {
	case "top":
		for _, o := range []struct{ name, desc string }{{"auto", "tự vừa màn hình"}, {"10", ""}, {"20", ""}, {"30", ""}, {"50", ""}, {"100", ""}, {"0", "tất cả"}} {
			if strings.HasPrefix(o.name, strings.ToLower(rest)) {
				list = append(list, option(o.name, o.desc, "", topText(v) == o.name || (o.name == "0" && v.Top == 0) || (o.name == "auto" && v.Top < 0)))
			}
		}
		if n, err := strconv.Atoi(rest); err == nil && n > 0 && len(list) == 0 {
			list = append(list, option(rest, "hiện "+rest+" session", "", false))
		}
	case "project":
		if has("all") {
			list = append(list, option("all", "bỏ lọc project", "", v.Project == ""))
		}
		for _, ps := range projects {
			if has(ps.Name) {
				list = append(list, option(ps.Name, compactTokens(ps.Total)+" token trong kỳ", ps.Cost, strings.EqualFold(v.Project, ps.Name)))
			}
		}
	case "app":
		for _, name := range []string{"all", "claude", "codex"} {
			if has(name) {
				list = append(list, option(name, "", "", v.Client == name || (name == "all" && v.Client == "")))
			}
		}
	case "agents":
		for _, o := range []struct {
			name, desc string
			on         bool
		}{{"on", "liệt kê từng worker dưới session", true}, {"off", "chỉ hiện số worker", false}} {
			if has(o.name) {
				list = append(list, option(o.name, o.desc, "", v.Agents == o.on))
			}
		}
	default:
		return nil, fmt.Sprintf("%q không có tham số — xoá khoảng trắng để chọn lệnh khác", word)
	}
	if len(list) == 0 {
		return nil, fmt.Sprintf("không có %q nào khớp %q", cmd, rest)
	}
	return list, ""
}

func isBaseSpec(word string) bool {
	canonical := canonicalSpec(word)
	for _, spec := range overviewSpecs {
		if spec == canonical {
			return true
		}
	}
	return false
}

// usageUI is what the screen remembers between key presses.
type usageUI struct {
	v         usageView // the view that was chosen
	input     []rune
	sel       int  // highlighted line of the list; -1 = none
	auto      bool // after typing, highlight the first line once the list is rebuilt
	scroll    int  // first visible line of the report
	page      int  // lines of one PgUp/PgDn step
	help      bool
	message   string
	lastShown usageView
	updated   time.Time // when the numbers on the screen were last read
}

func refreshText() string { return fmt.Sprintf("%.0fs", usageRefreshEvery.Seconds()) }

const usageUIHelp = `Phím:
  ↑ ↓         chọn lệnh trong danh sách — báo cáo đổi theo ngay (xem trước)
  Enter       chốt lệnh đang chọn (lệnh có tham số thì mở danh sách giá trị)
  Tab         điền lệnh vào dòng nhập      Esc  bỏ xem trước / xoá dòng nhập / thoát
  PgUp PgDn   cuộn báo cáo                 Home End  lên đầu / xuống cuối
  Ctrl+U      xoá dòng nhập                Ctrl+C hoặc q  thoát

Gõ để lọc danh sách. Kỳ gõ tay: 2026-11-05, 2026-11, 14d, 2026-11-01..2026-11-09.
Gõ số 1-6 để nhảy tới kỳ đầu danh sách (today … lastmonth).

Bấm phím bất kỳ để quay lại báo cáo.`

// handle changes the state by one key. It returns true when the user quits.
func (ui *usageUI) handle(k keyEvent, list []usageSuggestion, now time.Time) bool {
	if ui.help {
		ui.help = false
		return k.kind == keyCtrlC || k.kind == keyEOF
	}
	ui.message = ""
	switch k.kind {
	case keyEOF, keyCtrlC:
		return true
	case keyCtrlD:
		return len(ui.input) == 0
	case keyRune:
		ui.input = append(ui.input, k.r)
		ui.auto = true
	case keyBackspace:
		if len(ui.input) > 0 {
			ui.input = ui.input[:len(ui.input)-1]
		}
		ui.auto = len(ui.input) > 0
		if !ui.auto {
			ui.sel = -1
		}
	case keyCtrlU:
		ui.input, ui.sel, ui.auto = nil, -1, false
	case keyDown:
		if len(list) > 0 {
			ui.sel = (ui.sel + 1) % len(list)
		}
	case keyUp:
		if len(list) > 0 {
			ui.sel = (ui.sel - 1 + len(list)) % len(list)
			if ui.sel < 0 {
				ui.sel = len(list) - 1
			}
		}
	case keyTab:
		if len(list) > 0 {
			ui.input = []rune(list[max(ui.sel, 0)].Insert)
			ui.auto = true
		}
	case keyEsc:
		switch {
		case len(ui.input) > 0:
			ui.input, ui.sel, ui.auto = nil, -1, false
		case ui.sel >= 0:
			ui.sel = -1
		default:
			return true
		}
	case keyPgUp:
		ui.scroll = max(0, ui.scroll-max(ui.page, 1))
	case keyPgDn:
		ui.scroll += max(ui.page, 1)
	case keyHome:
		ui.scroll = 0
	case keyEnd:
		ui.scroll = 1 << 30 // clamped when drawn
	case keyEnter:
		return ui.commit(list, now)
	}
	return false
}

// commit chooses the highlighted line, or runs the text typed on the prompt.
func (ui *usageUI) commit(list []usageSuggestion, now time.Time) bool {
	line := string(ui.input)
	if ui.sel >= 0 && ui.sel < len(list) {
		s := list[ui.sel]
		if s.NeedsArg {
			ui.input, ui.auto = []rune(s.Insert), true
			return false
		}
		if s.Apply == "" {
			return false
		}
		line = s.Apply
	} else if strings.TrimSpace(line) == "" {
		return false
	}
	msg, quit := applyUsageCommand(&ui.v, line, now)
	if quit {
		return true
	}
	ui.input, ui.sel, ui.auto, ui.scroll = nil, -1, false, 0
	if strings.HasPrefix(msg, "Lệnh gõ") {
		ui.help = true
	} else {
		ui.message = msg
	}
	return false
}

// reportCache keeps the text of the views already drawn, so moving up and down the
// list does not add everything up again. It is dropped when new calls arrive.
type reportCache struct {
	calls int
	text  map[reportKey]string
}

type reportKey struct {
	v   usageView
	day string
}

func (c *reportCache) sync(calls int) {
	if c.text == nil || c.calls != calls {
		c.text, c.calls = map[reportKey]string{}, calls
	}
}

func clipRunes(s string, width int) string {
	if width <= 0 || utf8.RuneCountInString(s) <= width {
		return s
	}
	return string([]rune(s)[:width])
}

func padRight(s string, width int) string {
	if n := utf8.RuneCountInString(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

func padLeft(s string, width int) string {
	if n := utf8.RuneCountInString(s); n < width {
		return strings.Repeat(" ", width-n) + s
	}
	return s
}

const (
	styleDim     = "\x1b[2m"
	styleInverse = "\x1b[7m"
	styleReset   = "\x1b[0m"
)

// listWindow is the part of a long list that is drawn, kept around the highlight.
func listWindow(sel, total, size int) (int, int) {
	if total <= size {
		return 0, total
	}
	start := 0
	if sel >= size {
		start = sel - size + 1
	}
	return start, start + size
}

// suggestionRows draws the lines start..end of the list with aligned columns.
func suggestionRows(list []usageSuggestion, start, end, sel, width int) []string {
	nameW, descW, statsW := 0, 0, 0
	for _, s := range list[start:end] {
		nameW = max(nameW, utf8.RuneCountInString(s.Name))
		descW = max(descW, utf8.RuneCountInString(s.Desc))
		statsW = max(statsW, utf8.RuneCountInString(s.Stats))
	}
	descW = min(descW, max(20, width/2-nameW))
	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		s := list[i]
		pointer, mark := "  ", " "
		if i == sel {
			pointer = "› "
		}
		if s.Mark {
			mark = "●"
		}
		line := pointer + mark + " " + padRight(s.Name, nameW) + "  " + padRight(clipRunes(s.Desc, descW), descW)
		if statsW > 0 {
			line += "  " + padLeft(s.Stats, statsW)
		}
		line = clipRunes(strings.TrimRight(line, " "), width-1)
		if i == sel {
			line = styleInverse + padRight(line, width-1) + styleReset
		}
		rows = append(rows, line)
	}
	return rows
}

// renderUsageFrame lays out the whole screen: the report on top (scrolled), the list
// of commands, a hint line and the prompt on the last line.
func renderUsageFrame(ui *usageUI, reportLines []string, list []usageSuggestion, hint string, width, height int) string {
	maxItems := min(8, max(3, height-14))
	region := height - (maxItems + 3)
	ui.page = region - 1
	lines := make([]string, 0, height)

	source := reportLines
	if ui.help {
		source = strings.Split(usageUIHelp, "\n")
	}
	maxScroll := max(0, len(source)-region)
	ui.scroll = min(max(ui.scroll, 0), maxScroll)
	for i := 0; i < region; i++ {
		if idx := ui.scroll + i; idx < len(source) {
			lines = append(lines, clipRunes(source[idx], width-1))
		} else {
			lines = append(lines, "")
		}
	}

	title := "Lệnh   ↑↓ chọn và xem ngay · Tab điền · Enter chốt · Esc huỷ"
	if maxScroll > 0 && !ui.help {
		title += fmt.Sprintf(" · PgUp/PgDn cuộn (%d–%d/%d)", ui.scroll+1, min(ui.scroll+region, len(source)), len(source))
	}
	if ui.sel >= 0 && ui.sel < len(list) && list[ui.sel].Apply != "" {
		title = "Đang xem trước: " + list[ui.sel].Name + " · Enter để chốt, Esc để bỏ"
	}
	// The clock sits at the right end of the title line: it shows when the numbers were
	// last read, and that they renew themselves.
	if !ui.updated.IsZero() {
		stamp := "cập nhật " + ui.updated.Format("15:04:05") + " · tự làm mới mỗi " + refreshText()
		if gap := width - 1 - utf8.RuneCountInString(title) - utf8.RuneCountInString(stamp); gap >= 2 {
			title += strings.Repeat(" ", gap) + stamp
		}
	}
	lines = append(lines, styleDim+clipRunes(title, width-1)+styleReset)

	rows := []string{}
	if len(list) > 0 {
		start, end := listWindow(ui.sel, len(list), maxItems)
		rows = suggestionRows(list, start, end, ui.sel, width)
		// Say when the list goes on beyond the lines shown.
		switch {
		case end < len(list) && start > 0:
			hint = fmt.Sprintf("▲ %d dòng ở trên · ▼ %d dòng ở dưới · %s", start, len(list)-end, hint)
		case end < len(list):
			hint = fmt.Sprintf("▼ còn %d dòng nữa, bấm ↓ để xem · %s", len(list)-end, hint)
		case start > 0:
			hint = fmt.Sprintf("▲ %d dòng ở trên · %s", start, hint)
		}
	}
	for len(rows) < maxItems {
		rows = append(rows, "")
	}
	lines = append(lines, rows...)

	switch {
	case ui.message != "":
		lines = append(lines, clipRunes(strings.ReplaceAll(ui.message, "\n", " "), width-1))
	default:
		lines = append(lines, styleDim+clipRunes(hint, width-1)+styleReset)
	}
	var b strings.Builder
	b.WriteString("\x1b[H")
	for _, line := range lines {
		b.WriteString(line + "\x1b[K\r\n")
	}
	b.WriteString("> " + string(ui.input) + "\x1b[K\x1b[J")
	return b.String()
}

// usageRefreshEvery is how often the screen looks for new calls while nobody types.
// Every look only reads what was appended to the session files since the last one.
const usageRefreshEvery = 5 * time.Second

// scanFingerprint changes whenever the scanner learned anything new: a call, a chat
// title or a worker description.
func scanFingerprint(sc *sessionScanner) [3]int {
	return [3]int{len(sc.calls), len(sc.titles), len(sc.agents)}
}

// usageUILoop draws and reads keys until the user quits, and returns the view that
// was chosen. size gives the terminal width and height. While no key arrives the loop
// looks for new calls every refresh, and draws again only if there are some.
func usageUILoop(keys keySource, out io.Writer, sc *sessionScanner, v usageView, oldest time.Time, size func() (int, int), refresh time.Duration) (usageView, error) {
	ui := &usageUI{v: v, sel: -1}
	cache := &reportCache{}
	first, _ := resolvePeriod("lastmonth", time.Now())
	for {
		now := time.Now()
		width, height := size()
		first, _ = resolvePeriod("lastmonth", now)
		sc.scan(first.Start)
		cache.sync(len(sc.calls))
		ui.updated = now

		rows := sc.periodOverview(ui.v, now)
		var projects []projectStat
		if strings.HasPrefix(strings.ToLower(strings.TrimLeft(string(ui.input), " /")), "p") {
			if p, err := resolvePeriod(ui.v.Spec, now); err == nil {
				projects = sc.projectStats(p, ui.v.Client)
			}
		}
		list, hint := buildSuggestions(string(ui.input), ui.v, rows, projects, now)
		if ui.auto {
			ui.sel, ui.auto = -1, false
			if len(list) > 0 {
				ui.sel = 0
			}
		}
		if ui.sel >= len(list) {
			ui.sel = len(list) - 1
		}

		shown := ui.v
		if ui.sel >= 0 && list[ui.sel].Apply != "" {
			next := ui.v
			if msg, quit := applyUsageCommand(&next, list[ui.sel].Apply, now); msg == "" && !quit {
				shown = next
			}
		}
		shown.Width = width - 1 // the report is drawn one column short of the edge
		if shown != ui.lastShown {
			if shown.Width == ui.lastShown.Width { // a resize keeps the scroll position
				ui.scroll = 0
			}
			ui.lastShown = shown
		}
		key := reportKey{v: shown, day: now.Format("2006-01-02")}
		text, ok := cache.text[key]
		if !ok {
			var err error
			if text, err = reportText(sc, shown, now, 0, oldest); err != nil {
				text = err.Error()
			}
			cache.text[key] = text
			cache.calls = len(sc.calls)
		}
		frame := renderUsageFrame(ui, strings.Split(strings.TrimRight(text, "\n"), "\n"), list, hint, width, height)
		if _, err := io.WriteString(out, frame); err != nil {
			return ui.v, err
		}

		// Wait for a key. When the wait times out, look for new calls: if none, wait
		// again without drawing; if there are, go round and draw.
		before := scanFingerprint(sc)
		for {
			key, got := keys.next(refresh)
			if got {
				if ui.handle(key, list, now) {
					return ui.v, nil
				}
				break
			}
			sc.scan(first.Start)
			if scanFingerprint(sc) != before {
				break
			}
			if w, h := size(); w != width || h != height { // the window was resized
				break
			}
		}
	}
}

// runUsageScreen switches the terminal to raw mode and the alternate screen, runs the
// loop, and prints the last report on the normal screen when it is done. ok is false
// when the terminal cannot be used this way, and the caller falls back to typed lines.
func runUsageScreen(file *os.File, out io.Writer, sc *sessionScanner, v usageView, oldest time.Time) (ok bool, err error) {
	stdout, isFile := out.(*os.File)
	if !isFile || file == nil {
		return false, nil
	}
	size := func() (int, int) {
		if w, h, err := term.GetSize(int(stdout.Fd())); err == nil && w > 0 && h > 0 {
			return w, h
		}
		return 100, 30
	}
	if w, h := size(); w < 70 || h < 16 {
		return false, nil
	}
	keys, restore, opened := openKeys(file)
	if !opened {
		return false, nil
	}
	defer enableTerminalEscapes()()
	fmt.Fprint(out, "\x1b[?1049h\x1b[2J\x1b[H")
	// Reading a month of session files takes several seconds the first time.
	fmt.Fprint(out, "Đang đọc file phiên của Claude Code và Codex trên máy này (lần đầu vài giây)...")
	final, loopErr := usageUILoop(keys, out, sc, v, oldest, size, usageRefreshEvery)
	fmt.Fprint(out, "\x1b[?1049l")
	restore()
	if loopErr != nil {
		return true, loopErr
	}
	final.Overview = false
	text, err := reportText(sc, final, time.Now(), 0, oldest)
	if err != nil {
		return true, err
	}
	_, err = io.WriteString(out, text)
	return true, err
}
