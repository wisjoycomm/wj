package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// `wj usage <period>`: token by calendar day, and every session of the period with
// what it cost, so the expensive chat is easy to find.

// usagePeriod is a run of whole local days: Start is midnight of the first day and
// End is midnight after the last one.
type usagePeriod struct {
	Start, End time.Time
	Label      string
}

func (p usagePeriod) singleDay() bool { return p.Start.AddDate(0, 0, 1).Equal(p.End) }

const periodHelp = "today, yesterday, week, lastweek, month, lastmonth, 7d (N ngày gần nhất), 2026-11-09, 2026-11, 2026-11-01..2026-11-09"

var weekdayNames = []string{"CN", "T2", "T3", "T4", "T5", "T6", "T7"}

func dayText(t time.Time) string {
	return t.Format("2006-01-02") + " " + weekdayNames[int(t.Weekday())]
}

// resolvePeriod turns what the user typed into whole local days. Weeks start on Monday.
func resolvePeriod(spec string, now time.Time) (usagePeriod, error) {
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	span := func(start, end time.Time, name string) usagePeriod {
		last := end.AddDate(0, 0, -1)
		label := name
		if !start.Equal(last) {
			label += " (" + start.Format("2006-01-02") + " → " + last.Format("2006-01-02") + ")"
		}
		return usagePeriod{Start: start, End: end, Label: label}
	}
	monday := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	spec = strings.ToLower(strings.TrimSpace(spec))
	switch spec {
	case "today":
		return span(today, today.AddDate(0, 0, 1), "hôm nay "+dayText(today)), nil
	case "yesterday":
		y := today.AddDate(0, 0, -1)
		return span(y, today, "hôm qua "+dayText(y)), nil
	case "week", "thisweek":
		return span(monday, monday.AddDate(0, 0, 7), "tuần này"), nil
	case "lastweek":
		return span(monday.AddDate(0, 0, -7), monday, "tuần trước"), nil
	case "month", "thismonth":
		return span(firstOfMonth, firstOfMonth.AddDate(0, 1, 0), "tháng này"), nil
	case "lastmonth":
		return span(firstOfMonth.AddDate(0, -1, 0), firstOfMonth, "tháng trước"), nil
	}
	if digits, ok := strings.CutSuffix(spec, "d"); ok {
		var n int
		if _, err := fmt.Sscanf(digits, "%d", &n); err == nil && fmt.Sprint(n) == digits && n >= 1 {
			return span(today.AddDate(0, 0, -(n-1)), today.AddDate(0, 0, 1), fmt.Sprintf("%d ngày gần nhất", n)), nil
		}
	}
	if from, to, ok := strings.Cut(spec, ".."); ok {
		a, errA := time.ParseInLocation("2006-01-02", from, loc)
		b, errB := time.ParseInLocation("2006-01-02", to, loc)
		if errA != nil || errB != nil || b.Before(a) {
			return usagePeriod{}, fmt.Errorf("khoảng ngày %q không đúng, dạng: 2026-11-01..2026-11-09 (ngày đầu không sau ngày cuối)", spec)
		}
		return span(a, b.AddDate(0, 0, 1), "khoảng ngày"), nil
	}
	if d, err := time.ParseInLocation("2006-01-02", spec, loc); err == nil {
		return span(d, d.AddDate(0, 0, 1), "ngày "+dayText(d)), nil
	}
	if m, err := time.ParseInLocation("2006-01", spec, loc); err == nil {
		return span(m, m.AddDate(0, 1, 0), "tháng "+m.Format("2006-01")), nil
	}
	return usagePeriod{}, fmt.Errorf("không hiểu khoảng %q; dùng: %s", spec, periodHelp)
}

type dayRow struct {
	usageRow
	Day      time.Time
	Sessions int
}

type sessionRow struct {
	usageRow
	ID, Title string
	Agent     int64 // tokens of the subagents this session started
	MaxCtx    int64 // the biggest prompt of any call; every call reads it again
	models    map[string]int64
	workers   map[string]*workerRow // subagent id -> its own totals
}

// workerRow is one subagent (worker) of a session.
type workerRow struct {
	usageRow
	ID, Type, Description string
	MaxCtx                int64
	models                map[string]int64
}

func topModel(models map[string]int64) string {
	return sessionRow{models: models}.mainModel()
}

// sortedWorkers lists a session's workers, the most expensive first.
func (r sessionRow) sortedWorkers() []*workerRow {
	list := make([]*workerRow, 0, len(r.workers))
	for _, w := range r.workers {
		list = append(list, w)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Total != list[j].Total {
			return list[i].Total > list[j].Total
		}
		return list[i].ID < list[j].ID
	})
	return list
}

// mainModel is the model that used most of the session's tokens, with how many others.
func (r sessionRow) mainModel() string {
	best, others := "", 0
	var bestTokens int64 = -1
	for m, t := range r.models {
		if t > bestTokens || t == bestTokens && m < best {
			if best != "" {
				others++
			}
			best, bestTokens = m, t
		} else {
			others++
		}
	}
	if best == "" {
		return "-"
	}
	best = shortModel(best)
	if others > 0 {
		best += fmt.Sprintf(" +%d", others)
	}
	return best
}

// periodReport adds up the calls of the period by day and by session.
func (s *sessionScanner) periodReport(p usagePeriod, project, client string) ([]dayRow, []sessionRow, usageRow) {
	loc := p.Start.Location()
	days := map[string]*dayRow{}
	daySessions := map[string]map[string]bool{}
	sessions := map[string]*sessionRow{}
	var total usageRow
	for _, call := range s.calls {
		if !callInPeriod(call, p, project, client) {
			continue
		}
		local := call.When.In(loc)
		dayKey := local.Format("2006-01-02")
		d := days[dayKey]
		if d == nil {
			d = &dayRow{Day: time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)}
			days[dayKey] = d
			daySessions[dayKey] = map[string]bool{}
		}
		d.add(call)
		total.add(call)
		key := call.App + "\x00" + call.Session
		se := sessions[key]
		if se == nil {
			se = &sessionRow{ID: call.Session, Title: s.titles[call.Session], models: map[string]int64{}}
			se.Project, se.Client = call.Project, call.App
			sessions[key] = se
		}
		se.add(call)
		tokens := call.Input + call.Output + call.CacheRead + call.CacheWrite
		se.models[call.Model] += tokens
		if call.Sub {
			se.Agent += tokens
			w := se.workers[call.Agent]
			if w == nil {
				meta := s.agents[call.Session+"/"+call.Agent]
				w = &workerRow{ID: call.Agent, Type: meta.Type, Description: meta.Description, models: map[string]int64{}}
				w.Project, w.Client = call.Project, call.App
				if se.workers == nil {
					se.workers = map[string]*workerRow{}
				}
				se.workers[call.Agent] = w
			}
			w.add(call)
			w.models[call.Model] += tokens
			w.MaxCtx = max(w.MaxCtx, call.Context)
		}
		se.MaxCtx = max(se.MaxCtx, call.Context)
		daySessions[dayKey][key] = true
	}
	dayList := make([]dayRow, 0, len(days))
	for key, d := range days {
		d.Sessions = len(daySessions[key])
		dayList = append(dayList, *d)
	}
	sort.Slice(dayList, func(i, j int) bool { return dayList[i].Day.Before(dayList[j].Day) })
	sessionList := make([]sessionRow, 0, len(sessions))
	for _, se := range sessions {
		sessionList = append(sessionList, *se)
	}
	sort.Slice(sessionList, func(i, j int) bool {
		if sessionList[i].Total != sessionList[j].Total {
			return sessionList[i].Total > sessionList[j].Total
		}
		return sessionList[i].ID < sessionList[j].ID
	})
	return dayList, sessionList, total
}

// oldestSession is when the oldest session file on this machine was last written.
// Claude Code deletes old session files (cleanupPeriodDays), so a period before it
// may be missing calls.
func (s *sessionScanner) oldestSession() time.Time {
	var oldest time.Time
	roots := []string{filepath.Join(s.claudeDir, "projects"), filepath.Join(s.codexDir, "sessions"), filepath.Join(s.codexDir, "archived_sessions")}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".jsonl") {
				return nil
			}
			if info, err := entry.Info(); err == nil && (oldest.IsZero() || info.ModTime().Before(oldest)) {
				oldest = info.ModTime()
			}
			return nil
		})
	}
	return oldest
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func percentText(part, whole int64) string {
	if whole <= 0 {
		return "-"
	}
	return fmt.Sprintf("%d%%", (part*100+whole/2)/whole)
}

// shortModel is a model name without the gateway prefix and the "claude-" part, so
// "cc/claude-opus-5-5" and "claude-opus-5-5" count as one model.
func shortModel(model string) string {
	if model == "" {
		return "(không rõ)"
	}
	if i := strings.LastIndex(model, "/"); i >= 0 {
		model = model[i+1:]
	}
	return strings.TrimPrefix(model, "claude-")
}

// modelNode is one model of the period with the projects that used it.
type modelNode struct {
	usageRow
	Name     string
	Projects []usageRow
}

func callInPeriod(call sessionCall, p usagePeriod, project, client string) bool {
	return !call.When.Before(p.Start) && call.When.Before(p.End) &&
		(project == "" || strings.EqualFold(call.Project, project)) &&
		(client == "" || strings.EqualFold(call.App, client))
}

// periodTree adds up the calls of the period per model, and per project inside each
// model. The biggest comes first at both levels.
func (s *sessionScanner) periodTree(p usagePeriod, project, client string) []modelNode {
	models := map[string]*modelNode{}
	projects := map[string]map[string]*usageRow{}
	for _, call := range s.calls {
		if !callInPeriod(call, p, project, client) {
			continue
		}
		name := shortModel(call.Model)
		node := models[name]
		if node == nil {
			node = &modelNode{Name: name}
			models[name] = node
			projects[name] = map[string]*usageRow{}
		}
		node.add(call)
		row := projects[name][call.Project]
		if row == nil {
			row = &usageRow{}
			row.Project = call.Project
			projects[name][call.Project] = row
		}
		row.add(call)
	}
	tree := make([]modelNode, 0, len(models))
	for name, node := range models {
		for _, row := range projects[name] {
			node.Projects = append(node.Projects, *row)
		}
		sort.Slice(node.Projects, func(i, j int) bool {
			if node.Projects[i].Total != node.Projects[j].Total {
				return node.Projects[i].Total > node.Projects[j].Total
			}
			return node.Projects[i].Project < node.Projects[j].Project
		})
		tree = append(tree, *node)
	}
	sort.Slice(tree, func(i, j int) bool {
		if tree[i].Total != tree[j].Total {
			return tree[i].Total > tree[j].Total
		}
		return tree[i].Name < tree[j].Name
	})
	return tree
}

// printModelTree writes the summary at the end of the report: the total, then each
// model, then the projects that used that model.
func printModelTree(out io.Writer, tree []modelNode, total usageRow) {
	fmt.Fprintln(out, "\nTổng hợp theo model, rồi theo project:")
	cells := func(name string, r usageRow) []string {
		return []string{name, fmt.Sprint(r.Calls), compactTokens(r.Input), compactTokens(r.Output), compactTokens(r.CacheRead),
			compactTokens(r.CacheWrite), compactTokens(r.Total), percentText(r.Total, total.Total), costText(r)}
	}
	rows := [][]string{cells("TỔNG", total)}
	for i, m := range tree {
		branch, indent := "├─ ", "│  "
		if i == len(tree)-1 {
			branch, indent = "└─ ", "   "
		}
		rows = append(rows, cells(branch+m.Name, m.usageRow))
		for j, pr := range m.Projects {
			leaf := "├─ "
			if j == len(m.Projects)-1 {
				leaf = "└─ "
			}
			rows = append(rows, cells(indent+leaf+pr.Project, pr))
		}
	}
	renderTable(out, []string{"MODEL / PROJECT", "LƯỢT", "VÀO", "RA", "ĐỌC CACHE", "GHI CACHE", "TỔNG", "%", "QUY ĐỔI"},
		[]bool{true, false, false, false, false, false, false, false, false}, rows)
}

// printPeriodReport writes the report and returns how many sessions it listed.
// top is how many sessions to list; 0 lists all.
func printPeriodReport(out io.Writer, p usagePeriod, days []dayRow, sessions []sessionRow, total usageRow, top int, agents bool, width int, tree []modelNode, oldest time.Time) int {
	fmt.Fprintf(out, "Token %s (Claude Code + Codex trên máy này)\n\n", p.Label)
	shownCount := 0
	if total.Calls == 0 {
		fmt.Fprintln(out, "Không có lượt gọi nào trong khoảng này.")
	} else {
		fmt.Fprintf(out, "Tổng: %d lượt, %s token, %s, %d session\n", total.Calls, compactTokens(total.Total), costText(total), len(sessions))
		if !p.singleDay() {
			fmt.Fprintln(out, "\nTheo ngày:")
			rows := [][]string{}
			for _, d := range days {
				rows = append(rows, []string{dayText(d.Day), fmt.Sprint(d.Calls), groupDigits(d.Input), groupDigits(d.Output),
					groupDigits(d.CacheRead), groupDigits(d.CacheWrite), groupDigits(d.Total), costText(d.usageRow), fmt.Sprint(d.Sessions)})
			}
			rows = append(rows, []string{"TỔNG", fmt.Sprint(total.Calls), groupDigits(total.Input), groupDigits(total.Output),
				groupDigits(total.CacheRead), groupDigits(total.CacheWrite), groupDigits(total.Total), costText(total), fmt.Sprint(len(sessions))})
			renderTable(out, []string{"NGÀY", "LƯỢT", "VÀO", "RA", "ĐỌC CACHE", "GHI CACHE", "TỔNG", "QUY ĐỔI", "SESSION"},
				[]bool{true, false, false, false, false, false, false, false, false}, rows)
		}
		shown := sessions
		if top > 0 && len(shown) > top {
			shown = shown[:top]
		}
		shownCount = len(shown)
		fmt.Fprintf(out, "\nTừng session, tốn nhiều nhất lên đầu (%d/%d):\n", len(shown), len(sessions))
		rows := [][]string{}
		for _, se := range shown {
			title := se.Title
			if title == "" {
				title = "-"
			}
			agentText := compactTokens(se.Agent)
			if n := len(se.workers); n > 0 {
				agentText += fmt.Sprintf(" (%d)", n)
			}
			rows = append(rows, []string{shortID(se.ID), se.Project, fmt.Sprint(se.Calls),
				compactTokens(se.Input), compactTokens(se.Output), compactTokens(se.CacheRead), compactTokens(se.CacheWrite), compactTokens(se.Total),
				percentText(se.Total, total.Total), agentText, compactTokens(se.MaxCtx), costText(se.usageRow),
				se.mainModel(), cutRunes(title, 40)})
			if !agents {
				continue
			}
			workers := se.sortedWorkers()
			for i, w := range workers {
				if i == maxWorkersShown {
					rows = append(rows, []string{"  └ ...", "", "", "", "", "", "", "", "", "", "", "", "", fmt.Sprintf("còn %d worker nhỏ hơn", len(workers)-i)})
					break
				}
				desc := w.Description
				if desc == "" {
					desc = "-"
				}
				rows = append(rows, []string{"  └ " + shortID(w.ID), w.Type, fmt.Sprint(w.Calls),
					compactTokens(w.Input), compactTokens(w.Output), compactTokens(w.CacheRead), compactTokens(w.CacheWrite), compactTokens(w.Total),
					percentText(w.Total, total.Total), "", compactTokens(w.MaxCtx), costText(w.usageRow),
					topModel(w.models), cutRunes(desc, 40)})
			}
		}
		headers, left, fitted, hidden := fitColumns(
			[]string{"SESSION", "PROJECT", "LƯỢT", "VÀO", "RA", "ĐỌC CACHE", "GHI CACHE", "TỔNG", "%", "AGENT PHỤ", "CTX MAX", "QUY ĐỔI", "MODEL", "TIÊU ĐỀ"},
			[]bool{true, true, false, false, false, false, false, false, false, false, false, false, true, true},
			rows, sessionColumnPriority, width)
		renderTable(out, headers, left, fitted)
		if len(hidden) > 0 {
			fmt.Fprintf(out, "(cửa sổ hẹp: đã ẩn cột %s; kéo rộng cửa sổ để xem đủ)\n", strings.Join(hidden, ", "))
		}
		if len(shown) < len(sessions) {
			fmt.Fprintf(out, "... còn %d session nhỏ hơn (top 0 hoặc -top 0 để xem hết).\n", len(sessions)-len(shown))
		}
		if len(tree) > 0 {
			printModelTree(out, tree, total)
		}
		fmt.Fprintln(out, "\n% = phần trong tổng. AGENT PHỤ = token của các worker (subagent) session đó đã chạy (số worker trong ngoặc), đã nằm trong TỔNG; xem từng worker: agents.")
		fmt.Fprintln(out, "CTX MAX = prompt lớn nhất của một lượt gọi; mỗi lượt đọc lại cả prompt đó, nên context lớn × nhiều lượt là chỗ tốn.")
		fmt.Fprintf(out, "QUY ĐỔI (USD) = nếu trả theo giá API niêm yết ngày %s. Không phải số tiền thật: gói Pro/Max là phí cố định, gateway có giá riêng.\n", pricesAsOf)
		if total.Unpriced > 0 {
			fmt.Fprintf(out, "Có %s token của model chưa có trong bảng giá, không được tính vào QUY ĐỔI (dấu + hoặc ?).\n", groupDigits(total.Unpriced))
		}
	}
	if !oldest.IsZero() && p.Start.Before(oldest.Add(-24*time.Hour)) {
		fmt.Fprintf(out, "Lưu ý: file phiên cũ nhất trên máy được ghi lần cuối ngày %s; Claude Code xoá phiên cũ theo cleanupPeriodDays, nên phần trước đó có thể thiếu.\n", oldest.In(p.Start.Location()).Format("2006-01-02"))
	}
	return shownCount
}

// maxWorkersShown caps the workers listed under one session with -agents.
const maxWorkersShown = 15

// usageView is what the report shows. The commands typed at the prompt change it.
type usageView struct {
	Spec            string // the period as typed: today, week, 2026-11-09...
	Project, Client string // filters; empty = all
	Top             int    // sessions listed; below 0 = as many as fit
	Agents          bool   // list the workers under each session
	Overview        bool   // start the report with one line per period (today, week, month...)
	Width           int    // terminal width the tables must fit; 0 = no limit
}

// overviewSpecs are the periods of the overview, in the order of the keys 1-6.
var overviewSpecs = []string{"today", "yesterday", "week", "lastweek", "month", "lastmonth"}

var overviewNames = map[string]string{
	"today": "hôm nay", "yesterday": "hôm qua", "week": "tuần này",
	"lastweek": "tuần trước", "month": "tháng này", "lastmonth": "tháng trước",
}

// canonicalSpec is the period as the overview names it, so "ThisWeek" and "week" match.
func canonicalSpec(spec string) string {
	spec = strings.ToLower(strings.TrimSpace(spec))
	switch spec {
	case "thisweek":
		return "week"
	case "thismonth":
		return "month"
	}
	return spec
}

type overviewRow struct {
	usageRow
	Key, Spec, Name string
	Period          usagePeriod
	Sessions        int
	Current         bool
}

// periodOverview adds up every period of the overview in one pass over the calls. A
// period typed by hand (a date, 7d...) gets a row of its own, so the view is never
// missing from the table.
func (s *sessionScanner) periodOverview(v usageView, now time.Time) []overviewRow {
	current := canonicalSpec(v.Spec)
	var rows []overviewRow
	known := false
	for i, spec := range overviewSpecs {
		p, err := resolvePeriod(spec, now)
		if err != nil {
			continue
		}
		known = known || spec == current
		rows = append(rows, overviewRow{Key: fmt.Sprint(i + 1), Spec: spec, Name: overviewNames[spec], Period: p, Current: spec == current})
	}
	if !known {
		if p, err := resolvePeriod(v.Spec, now); err == nil {
			rows = append(rows, overviewRow{Key: "-", Spec: strings.TrimSpace(v.Spec), Name: "đang xem", Period: p, Current: true})
		}
	}
	sessions := make([]map[string]bool, len(rows))
	for i := range sessions {
		sessions[i] = map[string]bool{}
	}
	for _, call := range s.calls {
		for i := range rows {
			if callInPeriod(call, rows[i].Period, v.Project, v.Client) {
				rows[i].add(call)
				sessions[i][call.App+"\x00"+call.Session] = true
			}
		}
	}
	for i := range rows {
		rows[i].Sessions = len(sessions[i])
	}
	return rows
}

func rangeText(p usagePeriod) string {
	if p.singleDay() {
		return dayText(p.Start)
	}
	return p.Start.Format("2006-01-02") + " → " + p.End.AddDate(0, 0, -1).Format("2006-01-02")
}

func printOverview(out io.Writer, rows []overviewRow) {
	fmt.Fprintln(out, "Các kỳ (gõ số hoặc tên kỳ để chuyển; ▶ là kỳ đang xem):")
	cells := [][]string{}
	for _, r := range rows {
		marker := " "
		if r.Current {
			marker = "▶"
		}
		row := []string{marker, r.Key + " " + r.Spec, r.Name, rangeText(r.Period), "-", "-", "-", "-"}
		if r.Calls > 0 {
			row[4], row[5], row[6], row[7] = fmt.Sprint(r.Sessions), fmt.Sprint(r.Calls), compactTokens(r.Total), costText(r.usageRow)
		}
		cells = append(cells, row)
	}
	renderTable(out, []string{"", "KỲ", "TÊN", "NGÀY", "SESSION", "LƯỢT", "TỔNG", "QUY ĐỔI"},
		[]bool{true, true, true, true, false, false, false, false}, cells)
	fmt.Fprintln(out)
}

// reportText renders the report of the view. When maxLines > 0 and the session count
// is automatic, it lists fewer sessions until the text fits that many lines.
func reportText(sc *sessionScanner, v usageView, now time.Time, maxLines int, oldest time.Time) (string, error) {
	p, err := resolvePeriod(v.Spec, now)
	if err != nil {
		return "", err
	}
	if v.Project != "" {
		p.Label += ", project " + v.Project
	}
	if v.Client != "" {
		p.Label += ", app " + v.Client
	}
	since := p.Start
	if v.Overview {
		// The overview reaches back to the start of last month.
		if first, err := resolvePeriod("lastmonth", now); err == nil && first.Start.Before(since) {
			since = first.Start
		}
	}
	sc.scan(since)
	overview := ""
	if v.Overview {
		var b bytes.Buffer
		printOverview(&b, sc.periodOverview(v, now))
		overview = b.String()
	}
	days, sessions, total := sc.periodReport(p, v.Project, v.Client)
	tree := sc.periodTree(p, v.Project, v.Client)
	top := v.Top
	auto := top < 0
	if auto {
		top = 20
		if p.singleDay() {
			top = 0
		}
	}
	for {
		var b bytes.Buffer
		shown := printPeriodReport(&b, p, days, sessions, total, top, v.Agents, v.Width, tree, oldest)
		text := overview + b.String()
		lines := strings.Count(text, "\n")
		if !auto || maxLines <= 0 || lines <= maxLines || shown <= 3 {
			return text, nil
		}
		top = max(3, shown-(lines-maxLines))
	}
}

// sessionColumnPriority orders the session columns for a narrow window: 0 is the title,
// which shrinks first and is hidden first; the larger the number, the longer a column
// stays; 9 never goes (the id, the five token columns and the cost; the project goes last).
var sessionColumnPriority = []int{9, 6, 5, 9, 9, 9, 9, 9, 4, 3, 2, 9, 1, 0}

// fitColumns makes the table fit width: the title column (priority 0) is cut down to
// at least 12 characters, then the columns with the lowest priority are hidden until it
// fits. It returns what it hid. A width of 0 changes nothing.
func fitColumns(headers []string, left []bool, rows [][]string, priority []int, width int) ([]string, []bool, [][]string, []string) {
	if width <= 0 {
		return headers, left, rows, nil
	}
	const minTitle = 12
	kept := make([]int, len(headers))
	for i := range kept {
		kept[i] = i
	}
	titleLimit := 0 // 0 = the title is not cut yet
	colWidth := func(c int) int {
		w := utf8.RuneCountInString(headers[c])
		for _, row := range rows {
			n := utf8.RuneCountInString(row[c])
			if priority[c] == 0 && titleLimit > 0 {
				n = min(n, titleLimit)
			}
			w = max(w, n)
		}
		return w
	}
	tableWidth := func() int {
		sum := 2 * (len(kept) - 1)
		for _, c := range kept {
			sum += colWidth(c)
		}
		return sum
	}
	var hidden []string
	for tableWidth() > width {
		title := -1
		for _, c := range kept {
			if priority[c] == 0 {
				title = c
			}
		}
		if title >= 0 && (titleLimit == 0 || titleLimit > minTitle) {
			titleLimit = max(minTitle, colWidth(title)-(tableWidth()-width))
			if titleLimit >= colWidth(title) && colWidth(title) <= minTitle {
				titleLimit = minTitle
			}
			continue
		}
		drop := -1
		for i, c := range kept {
			if drop < 0 || priority[c] < priority[kept[drop]] {
				drop = i
			}
		}
		if drop < 0 || priority[kept[drop]] >= 9 {
			break
		}
		hidden = append(hidden, headers[kept[drop]])
		kept = append(kept[:drop], kept[drop+1:]...)
	}
	outHeaders, outLeft := make([]string, 0, len(kept)), make([]bool, 0, len(kept))
	for _, c := range kept {
		outHeaders, outLeft = append(outHeaders, headers[c]), append(outLeft, left[c])
	}
	outRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		cells := make([]string, 0, len(kept))
		for _, c := range kept {
			cell := row[c]
			if priority[c] == 0 && titleLimit > 0 {
				cell = cutRunes(cell, titleLimit)
			}
			cells = append(cells, cell)
		}
		outRows = append(outRows, cells)
	}
	return outHeaders, outLeft, outRows, hidden
}

// renderTable prints a table with two spaces between columns. left marks the columns
// that are left-aligned; the others are right-aligned (numbers).
func renderTable(out io.Writer, headers []string, left []bool, rows [][]string) {
	width := make([]int, len(headers))
	for i, h := range headers {
		width[i] = utf8.RuneCountInString(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			width[i] = max(width[i], utf8.RuneCountInString(cell))
		}
	}
	line := func(cells []string) {
		var b strings.Builder
		for i, cell := range cells {
			pad := strings.Repeat(" ", width[i]-utf8.RuneCountInString(cell))
			switch {
			case i == len(cells)-1 && left[i]:
				b.WriteString(cell) // no trailing spaces after the last text column
			case left[i]:
				b.WriteString(cell + pad)
			default:
				b.WriteString(pad + cell)
			}
			if i < len(cells)-1 {
				b.WriteString("  ")
			}
		}
		fmt.Fprintln(out, b.String())
	}
	line(headers)
	for _, row := range rows {
		line(row)
	}
}
