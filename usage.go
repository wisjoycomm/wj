package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/term"
)

// Claude Code and Codex record token usage in their own session files, so these
// commands need no proxy, no network and also see sessions started by Orca.

const (
	activeWindow   = 2 * time.Minute
	runningWindow  = 10 * time.Minute
	overlayMaxRows = 20
	// overlayMaxLines also counts the per-model lines of the expanded report.
	overlayMaxLines = 32
)

type sessionCall struct {
	When                                 time.Time
	Project, App, Model                  string
	Input, Output, CacheRead, CacheWrite int64
	CacheWrite1h                         int64 // the part of CacheWrite kept for an hour, which costs more
	// Session is the session id. A subagent's own file belongs to the session that
	// started it; Sub only marks that the call was recorded in such a file.
	Session string
	Sub     bool
	Agent   string // the subagent's own id when Sub is set
	// Context is the prompt size of this call; every call reads it again.
	Context int64
}

type sessionFile struct {
	offset     int64
	cwd, model string
	hasRecords bool
	lastTotal  codexTokens
}

type sessionScanner struct {
	claudeDir, codexDir string
	files               map[string]*sessionFile
	index               map[string]int
	calls               []sessionCall
	projects            map[string]string
	titles              map[string]string    // session id -> the title Claude Code gave the chat
	agents              map[string]agentMeta // session id + "/" + agent id -> what the subagent was asked to do
}

// agentMeta is what Claude Code writes next to a subagent's transcript.
type agentMeta struct {
	Type        string `json:"agentType"`
	Description string `json:"description"`
}

func newSessionScanner(home string) *sessionScanner {
	claudeDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if claudeDir == "" {
		claudeDir = filepath.Join(home, ".claude")
	}
	return &sessionScanner{
		claudeDir: claudeDir,
		codexDir:  filepath.Dir(codexConfigPath(home)),
		files:     map[string]*sessionFile{},
		index:     map[string]int{},
		projects:  map[string]string{},
		titles:    map[string]string{},
		agents:    map[string]agentMeta{},
	}
}

// scan reads only what was appended since the last scan. Files untouched since
// `since` cannot hold calls in that window and are skipped.
func (s *sessionScanner) scan(since time.Time) {
	s.scanTree(filepath.Join(s.claudeDir, "projects"), since, s.readClaudeLine)
	for _, dir := range []string{"sessions", "archived_sessions"} {
		s.scanTree(filepath.Join(s.codexDir, dir), since, s.readCodexLine)
	}
}

func (s *sessionScanner) scanTree(root string, since time.Time, parse func(string, *sessionFile, []byte)) {
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".jsonl") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		state := s.files[path]
		if state == nil && info.ModTime().Before(since) {
			return nil
		}
		if state == nil || info.Size() < state.offset {
			state = &sessionFile{}
			s.files[path] = state
		}
		if info.Size() > state.offset {
			s.readAppended(path, state, parse)
		}
		return nil
	})
}

func (s *sessionScanner) readAppended(path string, state *sessionFile, parse func(string, *sessionFile, []byte)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err = f.Seek(state.offset, io.SeekStart); err != nil {
		return
	}
	// Lines can be many megabytes (pasted images), so no bufio.Scanner.
	reader := bufio.NewReaderSize(f, 256<<10)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return // a line still being written is read next time
		}
		state.offset += int64(len(line))
		parse(path, state, line)
	}
}

func (s *sessionScanner) record(key string, call sessionCall) {
	if i, ok := s.index[key]; ok {
		s.calls[i] = call
		return
	}
	s.index[key] = len(s.calls)
	s.calls = append(s.calls, call)
}

func (s *sessionScanner) readClaudeLine(path string, state *sessionFile, raw []byte) {
	if bytes.Contains(raw, []byte(`"type":"ai-title"`)) {
		var title struct {
			Title string `json:"aiTitle"`
		}
		if id, sub := sessionOf(path); !sub && json.Unmarshal(raw, &title) == nil && title.Title != "" {
			s.titles[id] = title.Title
		}
		return
	}
	if !bytes.Contains(raw, []byte(`"usage"`)) {
		return
	}
	var line struct {
		Type      string `json:"type"`
		Cwd       string `json:"cwd"`
		RequestID string `json:"requestId"`
		UUID      string `json:"uuid"`
		Timestamp string `json:"timestamp"`
		Message   struct {
			ID    string `json:"id"`
			Model string `json:"model"`
			Usage struct {
				Input      int64 `json:"input_tokens"`
				Output     int64 `json:"output_tokens"`
				CacheRead  int64 `json:"cache_read_input_tokens"`
				CacheWrite int64 `json:"cache_creation_input_tokens"`
				Creation   struct {
					OneHour int64 `json:"ephemeral_1h_input_tokens"`
				} `json:"cache_creation"`
			} `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal(raw, &line) != nil || line.Type != "assistant" {
		return
	}
	when, err := time.Parse(time.RFC3339Nano, line.Timestamp)
	usage := line.Message.Usage
	if err != nil || usage.Input+usage.Output+usage.CacheRead+usage.CacheWrite == 0 {
		return
	}
	// A streamed reply repeats across lines and a resumed session can copy old
	// lines into a new file; the message id counts each reply once.
	key := line.Message.ID
	if key == "" {
		key = line.RequestID
	}
	if key == "" {
		key = path + "\x00" + line.UUID
	}
	id, sub := sessionOf(path)
	agent := ""
	if sub {
		agent = agentOf(path)
		if _, ok := s.agents[id+"/"+agent]; !ok {
			s.agents[id+"/"+agent] = readAgentMeta(path)
		}
	}
	s.record("claude\x00"+key, sessionCall{
		When: when, Project: s.projectOf(line.Cwd), App: "claude", Model: line.Message.Model,
		Input: usage.Input, Output: usage.Output, CacheRead: usage.CacheRead, CacheWrite: usage.CacheWrite,
		CacheWrite1h: usage.Creation.OneHour,
		Session:      id, Sub: sub, Agent: agent, Context: usage.Input + usage.CacheRead + usage.CacheWrite,
	})
}

// agentOf is the subagent id of a file under <session>/subagents/.
func agentOf(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return strings.TrimPrefix(name, "agent-")
}

// readAgentMeta reads agent-<id>.meta.json beside the transcript; a missing or
// broken file just leaves the subagent without a description.
func readAgentMeta(path string) agentMeta {
	var meta agentMeta
	if data, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".meta.json"); err == nil {
		_ = json.Unmarshal(data, &meta)
	}
	return meta
}

// sessionOf is the id of the session a transcript file belongs to, and whether the
// file is a subagent's own (<session>/subagents/agent-*.jsonl).
func sessionOf(path string) (string, bool) {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	dir := filepath.Dir(path)
	if filepath.Base(dir) == "subagents" {
		return filepath.Base(filepath.Dir(dir)), true
	}
	if strings.HasPrefix(name, "rollout-") && len(name) > 36 { // Codex: rollout-<date>-<uuid>
		name = name[len(name)-36:]
	}
	return name, false
}

type codexTokens struct {
	Input      int64 `json:"input_tokens"`
	Cached     int64 `json:"cached_input_tokens"`
	CacheWrite int64 `json:"cache_write_input_tokens"`
	Output     int64 `json:"output_tokens"`
}

func (s *sessionScanner) readCodexLine(path string, state *sessionFile, raw []byte) {
	if !bytes.Contains(raw, []byte(`"session_meta"`)) && !bytes.Contains(raw, []byte(`"turn_context"`)) &&
		!bytes.Contains(raw, []byte(`"token_usage_record"`)) && !bytes.Contains(raw, []byte(`"token_count"`)) {
		return
	}
	var line struct {
		Timestamp string `json:"timestamp"`
		Type      string `json:"type"`
		Payload   struct {
			Type       string      `json:"type"`
			Cwd        string      `json:"cwd"`
			Model      string      `json:"model"`
			TurnID     string      `json:"turn_id"`
			ResponseID string      `json:"response_id"`
			Usage      codexTokens `json:"usage"`
			Info       struct {
				Total codexTokens `json:"total_token_usage"`
			} `json:"info"`
		} `json:"payload"`
	}
	if json.Unmarshal(raw, &line) != nil {
		return
	}
	payload := line.Payload
	if payload.Cwd != "" {
		state.cwd = payload.Cwd
	}
	if line.Type == "turn_context" && payload.Model != "" {
		state.model = payload.Model
	}
	when, err := time.Parse(time.RFC3339Nano, line.Timestamp)
	if err != nil {
		return
	}
	switch {
	case line.Type == "token_usage_record":
		// One record per response. A forked session copies old records, and the
		// turn + response ids count each of them once.
		state.hasRecords = true
		s.record("codex\x00"+payload.TurnID+"\x00"+payload.ResponseID, s.codexCall(path, state, when, payload.Usage))
	case line.Type == "event_msg" && payload.Type == "token_count" && !state.hasRecords:
		// Older Codex builds only write a running total.
		total := payload.Info.Total
		diff := codexTokens{total.Input - state.lastTotal.Input, total.Cached - state.lastTotal.Cached,
			total.CacheWrite - state.lastTotal.CacheWrite, total.Output - state.lastTotal.Output}
		if diff.Input > 0 || diff.Output > 0 {
			state.lastTotal = total
			s.record("codex\x00"+path+"\x00"+line.Timestamp, s.codexCall(path, state, when, diff))
		}
	}
}

// codexCall removes cached tokens from Codex's input so the columns mean the
// same thing as Claude's, where cache tokens are never part of input.
func (s *sessionScanner) codexCall(path string, state *sessionFile, when time.Time, t codexTokens) sessionCall {
	input := t.Input - t.Cached - t.CacheWrite
	if input < 0 {
		input = 0
	}
	id, _ := sessionOf(path)
	return sessionCall{When: when, Project: s.projectOf(state.cwd), App: "codex", Model: state.model,
		Input: input, Output: t.Output, CacheRead: t.Cached, CacheWrite: t.CacheWrite,
		Session: id, Context: input + t.Cached + t.CacheWrite}
}

func (s *sessionScanner) projectOf(cwd string) string {
	if name, ok := s.projects[cwd]; ok {
		return name
	}
	name := projectNameOf(cwd)
	s.projects[cwd] = name
	return name
}

// projectNameOf is the main repository's folder name, so every Orca worktree of
// a repo counts as one project. Deleted folders fall back to the last path part.
func projectNameOf(cwd string) string {
	cwd = strings.TrimPrefix(cwd, `\\?\`)
	if cwd == "" {
		return "(không rõ)"
	}
	dir := filepath.Clean(cwd)
	// A path from another OS is not a place on this machine: walking it would end
	// in this process's own folder.
	if !filepath.IsAbs(dir) {
		return lastPathPart(cwd)
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, ".git"))
		if err == nil {
			if main := worktreeMain(string(data)); main != "" {
				return filepath.Base(main)
			}
			return filepath.Base(dir)
		}
		if isDir(filepath.Join(dir, ".git")) {
			return filepath.Base(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Base(filepath.Clean(cwd))
		}
		dir = parent
	}
}

func lastPathPart(path string) string {
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}

// worktreeMain reads a worktree's ".git" file: "gitdir: <repo>/.git/worktrees/<name>".
func worktreeMain(gitFile string) string {
	line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(gitFile), "gitdir:"))
	slashed := filepath.ToSlash(line)
	i := strings.LastIndex(slashed, "/worktrees/")
	if i < 0 {
		return ""
	}
	repo := strings.TrimSuffix(strings.TrimSuffix(slashed[:i], "/.git"), ".git")
	return filepath.FromSlash(repo)
}

type usageRow struct {
	usageTotal
	Apps      []string
	LastModel string
	Last      time.Time
	Cost      float64 // list price of the priced calls, in USD
	Unpriced  int64   // tokens of models with no known price
}

// add counts one call into the row.
func (t *usageRow) add(call sessionCall) {
	cost, priced := callCost(call)
	t.Cost += cost
	if !priced {
		t.Unpriced += call.Input + call.Output + call.CacheRead + call.CacheWrite
	}
	t.Calls++
	t.Input += call.Input
	t.Output += call.Output
	t.CacheRead += call.CacheRead
	t.CacheWrite += call.CacheWrite
	t.Total += call.Input + call.Output + call.CacheRead + call.CacheWrite
	t.Apps = appendUnique(t.Apps, call.App)
	if !call.When.Before(t.Last) {
		t.Last, t.LastModel = call.When, call.Model
	}
}

func (s *sessionScanner) summarize(f usageFilter) ([]usageRow, usageRow) {
	groups := map[string]*usageRow{}
	var all usageRow
	for _, call := range s.calls {
		if call.When.Before(f.Since) ||
			f.Project != "" && !strings.EqualFold(call.Project, f.Project) ||
			f.Client != "" && !strings.EqualFold(call.App, f.Client) {
			continue
		}
		var row usageRow
		for _, dim := range f.GroupBy {
			switch dim {
			case "project":
				row.Project = call.Project
			case "client":
				row.Client = call.App
			case "model":
				row.Model = call.Model
			}
		}
		key := row.Project + "\x00" + row.Client + "\x00" + row.Model
		g := groups[key]
		if g == nil {
			g = &row
			groups[key] = g
		}
		g.add(call)
		all.add(call)
	}
	list := make([]usageRow, 0, len(groups))
	for _, g := range groups {
		sort.Strings(g.Apps)
		list = append(list, *g)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Total != list[j].Total {
			return list[i].Total > list[j].Total
		}
		return list[i].Project+list[i].Client+list[i].Model < list[j].Project+list[j].Client+list[j].Model
	})
	return list, all
}

func runUsage(args []string, input io.Reader, out io.Writer) error {
	flags := flag.NewFlagSet("usage", flag.ContinueOnError)
	flags.SetOutput(out)
	window := flags.Duration("since", 24*time.Hour, "khoảng thời gian, ví dụ 1h, 24h, 168h (7 ngày)")
	all := flags.Bool("all", false, "toàn bộ phiên còn lưu trên máy")
	by := flags.String("by", "project-app", "gom theo: project, project-app, project-model, app (app + model), model")
	project := flags.String("project", "", "chỉ tính một project")
	client := flags.String("app", "", "chỉ tính một app: claude, codex")
	top := flags.Int("top", -1, "số session liệt kê khi xem theo ngày/tuần/tháng (0 = tất cả; mặc định 20, một ngày thì tất cả)")
	agents := flags.Bool("agents", false, "liệt kê từng worker (subagent) dưới session của nó, khi xem theo ngày/tuần/tháng")
	// A period (today, week, 2026-11-09...) may come before or after the flags.
	period, rest := "", args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		period, rest = args[0], args[1:]
	}
	if err := flags.Parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if period == "" && flags.NArg() == 1 {
		period = flags.Arg(0)
	} else if flags.NArg() != 0 {
		return errPeriodUsage
	}
	// No period and no other option: the report with a prompt below it. A period or
	// any option keeps the plain output that scripts rely on.
	if period == "" && flags.NFlag() == 0 {
		period = "today"
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		return runUsageShell(input, out, home, usageView{Spec: period, Top: -1, Overview: true})
	}
	if period != "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		text, err := reportText(newSessionScanner(home), usageView{Spec: period, Project: *project, Client: *client, Top: *top, Agents: *agents}, time.Now(), 0, time.Time{})
		if err != nil {
			return err
		}
		_, err = io.WriteString(out, text)
		return err
	}
	groupBy, ok := map[string][]string{
		"app":           {"client", "model"},
		"project":       {"project"},
		"project-app":   {"project", "client"},
		"project-model": {"project", "model"},
		"model":         {"model"},
	}[*by]
	if !ok {
		return errors.New("-by chỉ nhận: project, project-app, project-model, app, model")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	since := time.Now().Add(-*window)
	label := "trong " + shortWindow(*window)
	if *all {
		since, label = time.Time{}, "toàn bộ"
	}
	scanner := newSessionScanner(home)
	scanner.scan(since)
	list, total := scanner.summarize(usageFilter{Since: since, Project: *project, Client: *client, GroupBy: groupBy})
	fmt.Fprintf(out, "Token đã dùng (%s; Claude Code + Codex trên máy này)\n\n", label)
	if total.Calls == 0 {
		fmt.Fprintln(out, "Không có lượt gọi nào trong khoảng này.")
		return nil
	}
	headers := map[string]string{"project": "PROJECT", "client": "APP", "model": "MODEL"}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', tabwriter.AlignRight)
	var head []string
	for _, dim := range groupBy {
		head = append(head, headers[dim])
	}
	fmt.Fprintln(w, strings.Join(head, "\t")+"\tLƯỢT\tVÀO\tRA\tĐỌC CACHE\tGHI CACHE\tTỔNG\tQUY ĐỔI\t")
	row := func(t usageRow, first string) {
		var cells []string
		for i, dim := range groupBy {
			value := map[string]string{"project": t.Project, "client": t.Client, "model": t.Model}[dim]
			if first != "" {
				value = ""
				if i == 0 {
					value = first
				}
			}
			cells = append(cells, value)
		}
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t\n", strings.Join(cells, "\t"), t.Calls,
			groupDigits(t.Input), groupDigits(t.Output), groupDigits(t.CacheRead), groupDigits(t.CacheWrite), groupDigits(t.Total), costText(t))
	}
	for _, t := range list {
		row(t, "")
	}
	row(total, "TỔNG")
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(out, "\nVÀO = token prompt mới; ĐỌC CACHE thường rẻ hơn nhiều so với VÀO. App khác (OpenCode...) đo bằng: wj proxy usage")
	fmt.Fprintf(out, "QUY ĐỔI (USD) = nếu trả theo giá API niêm yết ngày %s. Không phải số tiền thật: gói Pro/Max là phí cố định, gateway có giá riêng.\n", pricesAsOf)
	if total.Unpriced > 0 {
		fmt.Fprintf(out, "Có %s token của model chưa có trong bảng giá, không được tính vào QUY ĐỔI (dấu + hoặc ?).\n", groupDigits(total.Unpriced))
	}
	return nil
}

func compactTokens(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1e9)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprint(n)
}

func agoText(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "vừa xong"
	case d < time.Hour:
		return fmt.Sprintf("%d phút", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d giờ", int(d.Hours()))
	}
	return fmt.Sprintf("%d ngày", int(d.Hours()/24))
}

func cutRunes(text string, limit int) string {
	runes := []rune(text)
	if limit <= 0 || len(runes) <= limit {
		return text
	}
	if limit == 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}

// projectStatus lists projects with the most recent activity first.
func (s *sessionScanner) projectStatus(since time.Time) ([]usageRow, usageRow) {
	list, total := s.summarize(usageFilter{Since: since, GroupBy: []string{"project"}})
	sort.SliceStable(list, func(i, j int) bool { return list[i].Last.After(list[j].Last) })
	return list, total
}

// machineTotals adds up every project on this machine over fixed windows.
func (s *sessionScanner) machineTotals(now time.Time) string {
	_, day := s.summarize(usageFilter{Since: now.Add(-24 * time.Hour)})
	_, week := s.summarize(usageFilter{Since: now.Add(-7 * 24 * time.Hour)})
	_, month := s.summarize(usageFilter{Since: now.Add(-30 * 24 * time.Hour)})
	_, all := s.summarize(usageFilter{})
	return fmt.Sprintf("Cả máy: 24 giờ %s · 7 ngày %s · 30 ngày %s · toàn bộ %s",
		compactTokens(day.Total), compactTokens(week.Total), compactTokens(month.Total), compactTokens(all.Total))
}

// overlayFooter is the machine total for the longer windows.
func (s *sessionScanner) overlayFooter(now time.Time) string {
	_, week := s.summarize(usageFilter{Since: now.Add(-7 * 24 * time.Hour)})
	_, month := s.summarize(usageFilter{Since: now.Add(-30 * 24 * time.Hour)})
	return fmt.Sprintf("7d %s · 30d %s", compactTokens(week.Total), compactTokens(month.Total))
}

// windowTag is the shortest label for a time window, for the overlay's one-line header.
func windowTag(d time.Duration) string {
	if d >= 48*time.Hour && d%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
	if d >= time.Hour && d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return d.String()
}

// openClaudeProjects reads Claude Code's per-process session markers, so an idle
// session still counts as open. Codex has no such marker and is seen by activity.
func (s *sessionScanner) openClaudeProjects() []string {
	paths, _ := filepath.Glob(filepath.Join(s.claudeDir, "sessions", "*.json"))
	var open []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var marker struct {
			PID int    `json:"pid"`
			Cwd string `json:"cwd"`
		}
		if json.Unmarshal(data, &marker) == nil && marker.PID > 0 && marker.Cwd != "" && processAlive(marker.PID) {
			open = appendUnique(open, s.projectOf(marker.Cwd))
		}
	}
	return open
}

// runningProjects keeps projects that are open or used tokens within runningWindow.
// The row limit keeps the overlay on screen.
func runningProjects(list []usageRow, open []string, now time.Time) []usageRow {
	rows := []usageRow{}
	seen := []string{}
	for _, row := range list {
		if now.Sub(row.Last) < runningWindow || containsName(open, row.Project) {
			rows = append(rows, row)
			seen = append(seen, row.Project)
		}
	}
	for _, name := range open {
		if !containsName(seen, name) {
			rows = append(rows, usageRow{usageTotal: usageTotal{Project: name}, Apps: []string{"claude"}})
		}
	}
	if len(rows) > overlayMaxRows {
		rows = rows[:overlayMaxRows]
	}
	return rows
}

// parseTokenCount reads a number of tokens such as 5M, 500k, 1.5B or 20000. G still
// means the same as B, for limits written before the display changed.
func parseTokenCount(text string) (int64, error) {
	number := strings.TrimSpace(text)
	scale := 1.0
	if number != "" {
		switch number[len(number)-1] {
		case 'k', 'K':
			scale = 1e3
		case 'm', 'M':
			scale = 1e6
		case 'b', 'B', 'g', 'G':
			scale = 1e9
		}
		if scale != 1 {
			number = number[:len(number)-1]
		}
	}
	value, err := strconv.ParseFloat(number, 64)
	if err != nil || !(value > 0) || value*scale > 1e15 { // the negated form also rejects NaN
		return 0, fmt.Errorf("số token không hợp lệ: %q (ví dụ: 5M, 500k, 20000)", text)
	}
	return int64(value * scale), nil
}

// warnLevel compares a total with a limit the user chose: 0 is fine (or no limit),
// 1 is from 80% of the limit, 2 is at or over it.
func warnLevel(total, limit int64) int {
	switch {
	case limit <= 0 || total*5 < limit*4:
		return 0
	case total < limit:
		return 1
	}
	return 2
}

// overlayDetailRows is the expanded overlay's report: running projects first, then
// the rest of the window, so the rows add up to the total shown above them.
func overlayDetailRows(running, list []usageRow) []usageRow {
	rows := append([]usageRow{}, running...)
	seen := make([]string, 0, len(running))
	for _, row := range running {
		seen = append(seen, row.Project)
	}
	for _, row := range list {
		if len(rows) == overlayMaxRows {
			break
		}
		if !containsName(seen, row.Project) {
			rows = append(rows, row)
		}
	}
	return rows
}

// overlayDetailEntry is one line of the expanded overlay: a project, or with Sub one
// model inside the project above it.
type overlayDetailEntry struct {
	usageRow
	Sub    bool
	Models int // how many models the project used
}

// modelsByProject is each project's usage per model, biggest first.
func (s *sessionScanner) modelsByProject(since time.Time) map[string][]usageRow {
	list, _ := s.summarize(usageFilter{Since: since, GroupBy: []string{"project", "model"}})
	models := map[string][]usageRow{}
	for _, row := range list {
		models[row.Project] = append(models[row.Project], row)
	}
	return models
}

// detailEntries adds a line per model under each project that used several.
// Every project keeps its line; the model lines stop at maxLines (0 = no limit) so the
// report still fits a screen, and such a project just says how many models it used.
func detailEntries(rows []usageRow, models map[string][]usageRow, maxLines int) []overlayDetailEntry {
	spare := maxLines - len(rows)
	if maxLines <= 0 {
		spare = math.MaxInt
	}
	entries := make([]overlayDetailEntry, 0, len(rows))
	for _, row := range rows {
		used := models[row.Project]
		entries = append(entries, overlayDetailEntry{usageRow: row, Models: len(used)})
		if len(used) < 2 || len(used) > spare {
			continue
		}
		spare -= len(used)
		for _, model := range used {
			entries = append(entries, overlayDetailEntry{usageRow: model, Sub: true})
		}
	}
	return entries
}

func shortWindow(d time.Duration) string {
	if d >= 48*time.Hour && d%(24*time.Hour) == 0 {
		return fmt.Sprintf("%d ngày", int(d.Hours()/24))
	}
	if d >= time.Hour && d%time.Hour == 0 {
		return fmt.Sprintf("%d giờ", int(d.Hours()))
	}
	return d.String()
}

// topReport is everything one frame of wj top shows.
type topReport struct {
	list   []usageRow            // projects in the window, newest first
	total  usageRow              // the whole window
	models map[string][]usageRow // each project's usage per model, biggest first
	apps   []usageRow            // the whole window per app
	open   []string              // projects with an open Claude Code session
}

func (s *sessionScanner) topReport(since time.Time) topReport {
	list, total := s.projectStatus(since)
	apps, _ := s.summarize(usageFilter{Since: since, GroupBy: []string{"client"}})
	return topReport{list: list, total: total, models: s.modelsByProject(since), apps: apps, open: s.openClaudeProjects()}
}

// topLayout is how much a terminal width can hold: the model column, and the cache
// split into read and write.
type topLayout struct{ model, splitCache bool }

// topLinesAround counts the lines of a frame that are not project rows.
const topLinesAround = 12

// renderTop shows the same report as the expanded overlay, plus calls and the cache
// split, and picks the widest layout that fits the terminal (width 0 = any).
func renderTop(r topReport, label string, now time.Time, width, height int) string {
	rows := append([]usageRow{}, r.list...)
	for _, name := range r.open {
		seen := false
		for _, row := range rows {
			seen = seen || row.Project == name
		}
		if !seen {
			rows = append(rows, usageRow{usageTotal: usageTotal{Project: name}, Apps: []string{"claude"}})
		}
	}
	budget := 0
	if height > 0 {
		budget = max(height-topLinesAround, 1)
	}
	hidden := 0
	if budget > 0 && len(rows) > budget {
		hidden = len(rows) - budget
		rows = rows[:budget]
	}
	entries := detailEntries(rows, r.models, budget)

	var out string
	for _, layout := range []topLayout{{true, true}, {true, false}, {false, false}} {
		out = renderTopTable(r, entries, layout, label, now, hidden)
		if width == 0 || widestLine(out) < width {
			break
		}
	}
	return out
}

func widestLine(text string) int {
	widest := 0
	for _, line := range strings.Split(text, "\n") {
		widest = max(widest, len([]rune(line)))
	}
	return widest
}

func renderTopTable(r topReport, entries []overlayDetailEntry, layout topLayout, label string, now time.Time, hidden int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "wj top - token theo project, %s (%s)\n\n", label, now.Format("15:04:05"))
	if len(entries) == 0 {
		b.WriteString("Chưa có lượt gọi nào trong khoảng này.\n")
		return b.String()
	}
	var table strings.Builder
	w := tabwriter.NewWriter(&table, 0, 0, 2, ' ', tabwriter.AlignRight)
	// The first columns read left to right, so they are padded by hand.
	head := []string{"  PROJECT", "APP", "LẦN CUỐI"}
	if layout.model {
		head = append(head, "MODEL")
	}
	head = append(head, "LƯỢT", "VÀO", "RA")
	if layout.splitCache {
		head = append(head, "ĐỌC CACHE", "GHI CACHE")
	} else {
		head = append(head, "CACHE")
	}
	head = append(head, "TỔNG", "≈USD")
	texts := 3
	if layout.model {
		texts = 4
	}
	widths := make([]int, texts)
	var lines [][]string
	add := func(cells []string) {
		for i := range texts {
			widths[i] = max(widths[i], len([]rune(cells[i])))
		}
		lines = append(lines, cells)
	}
	numbers := func(row usageRow) []string {
		cells := []string{fmt.Sprint(row.Calls), compactTokens(row.Input), compactTokens(row.Output)}
		if layout.splitCache {
			cells = append(cells, compactTokens(row.CacheRead), compactTokens(row.CacheWrite))
		} else {
			cells = append(cells, compactTokens(row.CacheRead+row.CacheWrite))
		}
		return append(cells, compactTokens(row.Total), costText(row))
	}
	add(append([]string{}, head...))
	for _, e := range entries {
		ago := "đang mở"
		if !e.Last.IsZero() {
			ago = agoText(now.Sub(e.Last))
		}
		var cells []string
		if e.Sub {
			name := "  └"
			if !layout.model {
				name = "  └ " + cutRunes(e.Model, 22)
			}
			cells = []string{name, strings.Join(e.Apps, ","), ago}
			if layout.model {
				cells = append(cells, cutRunes(e.Model, 26))
			}
		} else {
			mark := " "
			switch {
			case !e.Last.IsZero() && now.Sub(e.Last) < activeWindow:
				mark = "●"
			case containsName(r.open, e.Project):
				mark = "○"
			}
			cells = []string{mark + " " + cutRunes(e.Project, 24), strings.Join(e.Apps, ","), ago}
			if layout.model {
				model := cutRunes(e.LastModel, 26)
				if e.Models > 1 {
					model = fmt.Sprintf("%d model", e.Models)
				}
				cells = append(cells, model)
			}
		}
		add(append(cells, numbers(e.usageRow)...))
	}
	total := []string{"  TỔNG", "", ""}
	if layout.model {
		total = append(total, "")
	}
	add(append(total, numbers(r.total)...))
	for _, cells := range lines {
		for j := range texts {
			cells[j] += strings.Repeat(" ", widths[j]-len([]rune(cells[j])))
		}
		fmt.Fprintf(w, "%s\t\n", strings.Join(cells, "\t"))
	}
	_ = w.Flush()
	// AlignRight also pads the first column on the left; drop that indent.
	for _, line := range strings.SplitAfter(table.String(), "\n") {
		b.WriteString(strings.TrimPrefix(line, "  "))
	}
	if hidden > 0 {
		fmt.Fprintf(&b, "  ...và %d project khác (xem đủ: wj usage)\n", hidden)
	}
	if len(r.apps) > 1 {
		parts := make([]string, 0, len(r.apps))
		for _, app := range r.apps {
			parts = append(parts, fmt.Sprintf("%s %s %s", app.Client, compactTokens(app.Total), costText(app)))
		}
		fmt.Fprintf(&b, "\nTheo app: %s\n", strings.Join(parts, " · "))
	}
	return b.String()
}

func runTop(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("top", flag.ContinueOnError)
	flags.SetOutput(out)
	window := flags.Duration("since", 24*time.Hour, "khoảng thời gian, ví dụ 1h, 24h, 168h (7 ngày)")
	interval := flags.Duration("interval", 3*time.Second, "bao lâu cập nhật một lần")
	once := flags.Bool("once", false, "in một lần rồi thoát")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || *interval < time.Second {
		return errors.New("cách dùng: wj top [-since 24h] [-interval 3s] [-once]")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	scanner := newSessionScanner(home)
	label := "trong " + shortWindow(*window)
	frame := func(width, height int) string {
		now := time.Now()
		scanner.scan(time.Time{})
		report := scanner.topReport(now.Add(-*window))
		return renderTop(report, label, now, width, height) + "\n" + scanner.machineTotals(now) + "\n" +
			fmt.Sprintf("≈USD = nếu trả theo giá API niêm yết ngày %s, không phải số tiền thật. Đủ mọi model: wj usage -by project-model\n", pricesAsOf)
	}
	stdout, isFile := out.(*os.File)
	if *once || !isFile || !term.IsTerminal(int(stdout.Fd())) {
		_, err = io.WriteString(out, frame(0, 0))
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	defer enableTerminalEscapes()()
	fmt.Fprint(out, "\x1b[?1049h\x1b[?25l")
	defer fmt.Fprint(out, "\x1b[?25h\x1b[?1049l")
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for {
		width, height, err := term.GetSize(int(stdout.Fd()))
		if err != nil {
			width, height = 0, 0
		}
		lines := strings.Split(frame(width, height)+"\nCtrl+C để thoát. ● = dùng token trong 2 phút, ○ = phiên Claude Code đang mở.\n", "\n")
		for i, line := range lines {
			if width > 0 {
				lines[i] = cutRunes(line, width-1)
			}
		}
		fmt.Fprint(out, "\x1b[H"+strings.Join(lines, "\x1b[K\n")+"\x1b[J")
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
