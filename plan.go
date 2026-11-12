package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PLAN.md is one Markdown table and nothing else, so wj plan and wj verify can
// read a task from a single line.
var planColumns = []string{"ID", "Task", "Pri", "SP", "Status", "Agent", "Depends", "Scope", "Verify", "Notes", "Session"}

const utf8BOM = "\xef\xbb\xbf"

const (
	planNotesMax = 100
	planTaskWarn = 120
)

var (
	planIDPattern     = regexp.MustCompile(`^[A-Z]+-\d+$`)
	planStatuses      = []string{"TODO", "DOING", "DONE", "BLOCKED", "PARTIAL"}
	planPriorities    = []string{"P0", "P1", "P2", "P3"}
	planPoints        = []string{"1", "2", "3", "5", "8", "13", "?"}
	planAgentNames    = []string{"claude", "codex", "agy"}
	planModelPattern  = regexp.MustCompile(`^(claude|codex|agy):[A-Za-z0-9][A-Za-z0-9._/\[\]-]*$`)
	planHeaderLine    = "| " + strings.Join(planColumns, " | ") + " |"
	planSeparatorLine = "|" + strings.Repeat("---|", len(planColumns))
)

const planHelp = `PLAN.md dạng một bảng duy nhất (cho orchestrator / worker Orca)

  wj plan              bảng tóm tắt: task sẵn sàng chạy, đang chạy, bị chặn
  wj plan check        kiểm tra đúng format (in lỗi kèm số dòng)
  wj plan log          20 task đã archive gần nhất (-last 50: nhiều hơn;
                       -find T-12: một ID, hoặc chữ trong Task / Notes)
  wj plan convert      đổi PLAN.md kiểu cũ (checkbox + Backlog) sang bảng,
                       và cập nhật skill orca trong project cho khớp format mới
                       (-dry-run: chỉ xem; file cũ được sao lưu .bak-<giờ>)
  wj plan archive T-01 T-02 [-commit abc123]
                       chuyển task DONE/PARTIAL đã merge sang PLAN-ARCHIVE.md, kèm
                       commit merge và ngày; không có -commit thì tự tìm commit có ID
                       trong git log. Commit riêng từng task: T-01=abc123

  wj plan worker cx/gpt-6.1-sol
                       tạo worker chạy bằng model hãng khác của gateway (GPT, Gemini...):
                       .claude/agents/orca-worker-<tên>.md, để dùng claude:<model id>
                       trong cột Agent. Không tham số: liệt kê worker đã có.
                       Bảng chọn model theo loại việc: .claude/models.md

  Format: chỉ có dòng tiêu đề, dòng ---, và mỗi task một dòng:
` + "  | ID | Task | Pri | SP | Status | Agent | Depends | Scope | Verify | Notes | Session |" + `

  Pri     P0 (gấp) | P1 | P2 | P3 (để sau)
  SP      story point: 1 | 2 | 3 | 5 | 8 | 13 (13 = tách nhỏ) | ? (chưa ước lượng)
  Status  TODO | DOING | DONE | BLOCKED | PARTIAL
  Agent   claude | codex | agy, hoặc kèm model: claude:haiku, codex:<model>, agy:<model>.
          claude:<model id của gateway> cần chạy wj plan worker <id> trước.
          Orchestrator của Claude Code chỉ tự chạy dòng claude; dòng codex / agy nó liệt kê
          để bạn chạy trong app đó.
  Depends ID cách nhau dấu phẩy, hoặc -
  Scope   đường dẫn trong backtick, cách nhau dấu phẩy
  Verify  một lệnh trong backtick, hoặc - (tự đoán)
  Notes   tối đa 100 ký tự
  Session nhánh worker đã commit, hoặc -
  Không ô nào được chứa ký tự | hay xuống dòng.`

type planTask struct {
	Line                             int
	ID, Task, Pri, SP, Status, Agent string
	Depends, Scope                   []string
	Verify, Notes, Session           string
}

type planIssue struct {
	Line int
	Text string
}

func (i planIssue) String() string { return fmt.Sprintf("dòng %d: %s", i.Line, i.Text) }

func splitPlanRow(line string) ([]string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") || len(trimmed) < 2 {
		return nil, false
	}
	cells := strings.Split(trimmed[1:len(trimmed)-1], "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells, true
}

func backticked(cell string) []string {
	var items []string
	for _, m := range regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(cell, -1) {
		items = append(items, strings.TrimSpace(m[1]))
	}
	return items
}

func dashList(cell string) []string {
	if cell == "-" || cell == "" {
		return nil
	}
	var items []string
	for _, part := range strings.Split(cell, ",") {
		if part = strings.TrimSpace(part); part != "" {
			items = append(items, part)
		}
	}
	return items
}

// parsePlan reads a table PLAN.md. Every problem is reported with its line, so
// one run lists everything to fix; tasks are returned for the lines that parse.
func parsePlan(text string) ([]planTask, []planIssue) {
	text = strings.TrimPrefix(text, utf8BOM)
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	var issues []planIssue
	if len(lines) < 2 {
		return nil, []planIssue{{1, "thiếu dòng tiêu đề bảng và dòng ---"}}
	}
	if header, ok := splitPlanRow(lines[0]); !ok || strings.Join(header, "|") != strings.Join(planColumns, "|") {
		issues = append(issues, planIssue{1, "dòng tiêu đề phải đúng: " + planHeaderLine})
	}
	if sep, ok := splitPlanRow(lines[1]); !ok || len(sep) != len(planColumns) || !allDashes(sep) {
		issues = append(issues, planIssue{2, "dòng 2 phải là dòng --- của bảng: " + planSeparatorLine})
	}
	seen := map[string]int{}
	var tasks []planTask
	for i := 2; i < len(lines); i++ {
		n := i + 1
		cells, ok := splitPlanRow(lines[i])
		if !ok {
			issues = append(issues, planIssue{n, "ngoài bảng (PLAN.md chỉ được có bảng; ghi chú để ở cột Notes)"})
			continue
		}
		if len(cells) != len(planColumns) {
			issues = append(issues, planIssue{n, fmt.Sprintf("có %d ô, cần %d (ô có chứa | không?)", len(cells), len(planColumns))})
			continue
		}
		t := planTask{Line: n, ID: cells[0], Task: cells[1], Pri: cells[2], SP: cells[3], Status: cells[4], Agent: cells[5],
			Depends: dashList(cells[6]), Scope: backticked(cells[7]), Notes: cells[9], Session: cells[10]}
		if v := backticked(cells[8]); len(v) > 0 {
			t.Verify = v[0]
		}
		bad := func(format string, a ...any) {
			issues = append(issues, planIssue{n, t.ID + ": " + fmt.Sprintf(format, a...)})
		}
		if !planIDPattern.MatchString(t.ID) {
			bad("ID phải dạng T-01")
		} else if prev, dup := seen[t.ID]; dup {
			bad("trùng ID với dòng %d", prev)
		}
		seen[t.ID] = n
		if t.Task == "" {
			bad("thiếu Task")
		} else if len([]rune(t.Task)) > planTaskWarn {
			bad("Task dài %d ký tự, tối đa %d (chi tiết để ở file khác)", len([]rune(t.Task)), planTaskWarn)
		}
		if !contains(planPriorities, t.Pri) {
			bad("Pri %q phải là %s", t.Pri, strings.Join(planPriorities, "|"))
		}
		if !contains(planPoints, t.SP) {
			bad("SP %q phải là %s", t.SP, strings.Join(planPoints, "|"))
		}
		if !contains(planStatuses, t.Status) {
			bad("Status %q phải là %s", t.Status, strings.Join(planStatuses, "|"))
		}
		if !planAgentOK(t.Agent) {
			bad("Agent %q phải là %s, hoặc một trong số đó kèm :<model> (vd claude:haiku)", t.Agent, strings.Join(planAgentNames, "|"))
		}
		for _, d := range t.Depends {
			if !planIDPattern.MatchString(d) {
				bad("Depends %q phải là ID (T-01) hoặc -", d)
			}
		}
		if cells[7] != "-" && len(t.Scope) == 0 {
			bad("Scope phải là đường dẫn trong backtick (`src/a.go`) hoặc -")
		}
		if cells[8] != "-" && t.Verify == "" {
			bad("Verify phải là một lệnh trong backtick hoặc -")
		}
		if len([]rune(t.Notes)) > planNotesMax {
			bad("Notes dài %d ký tự, tối đa %d", len([]rune(t.Notes)), planNotesMax)
		}
		if t.Session == "" {
			bad("Session để - khi chưa có ai làm")
		}
		tasks = append(tasks, t)
	}
	return tasks, issues
}

// planAgentOK accepts an agent name, or claude:<model> for a worker that
// starts on a named model.
func planAgentOK(agent string) bool {
	return contains(planAgentNames, agent) || planModelPattern.MatchString(agent)
}

func allDashes(cells []string) bool {
	for _, c := range cells {
		if strings.Trim(c, ":-") != "" || !strings.Contains(c, "-") {
			return false
		}
	}
	return true
}

func nextPlanID(tasks []planTask, archive string) string {
	highest := 0
	for _, id := range append(taskIDs(tasks), regexp.MustCompile(`\bT-\d+\b`).FindAllString(archive, -1)...) {
		if n, err := strconv.Atoi(strings.TrimPrefix(id, "T-")); err == nil && n > highest {
			highest = n
		}
	}
	return fmt.Sprintf("T-%02d", highest+1)
}

func taskIDs(tasks []planTask) []string {
	ids := make([]string, len(tasks))
	for i, t := range tasks {
		ids[i] = t.ID
	}
	return ids
}

func planRow(t planTask) string {
	cell := func(s string) string {
		s = strings.Join(strings.Fields(strings.ReplaceAll(s, "|", "/")), " ")
		if s == "" {
			return "-"
		}
		return s
	}
	code := func(items []string) string {
		if len(items) == 0 {
			return "-"
		}
		quoted := make([]string, len(items))
		for i, item := range items {
			quoted[i] = "`" + strings.ReplaceAll(item, "|", "/") + "`"
		}
		return strings.Join(quoted, ", ")
	}
	verify := "-"
	if t.Verify != "" {
		verify = code([]string{t.Verify})
	}
	deps := "-"
	if len(t.Depends) > 0 {
		deps = strings.Join(t.Depends, ", ")
	}
	return "| " + strings.Join([]string{cell(t.ID), cell(t.Task), cell(t.Pri), cell(t.SP), cell(t.Status), cell(t.Agent),
		deps, code(t.Scope), verify, cell(t.Notes), cell(t.Session)}, " | ") + " |"
}

func cutText(s string, limit int) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) <= limit {
		return string(r)
	}
	return strings.TrimSpace(string(r[:limit-1])) + "…"
}

var (
	oldTaskLine  = regexp.MustCompile(`^\s*-\s*\[(.)\]\s*\*\*([A-Za-z]+-\d+)\*\*\s*(?:\(([^)]*)\))?\s*:?\s*(.*)$`)
	oldFieldLine = regexp.MustCompile(`^\s*-\s*\*\*(Agent|Scope|Verify|Depends|Result)\*\*\s*:\s*(.*)$`)
)

// convertOldPlan turns the checkbox format (Queue + Backlog) into table rows.
// Long titles move to Notes in short form; the full old file stays as a backup.
func convertOldPlan(text string) ([]planTask, int) {
	var tasks []planTask
	var cur *planTask
	backlog, skipped := false, 0
	inComment := false
	maxID := 0
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.Contains(line, "<!--") {
			inComment = true
		}
		if inComment {
			if strings.Contains(line, "-->") {
				inComment = false
			}
			continue
		}
		if m := oldTaskLine.FindStringSubmatch(line); m != nil {
			status := map[string]string{" ": "TODO", "~": "DOING", "x": "DONE", "X": "DONE", "!": "BLOCKED"}[m[1]]
			if status == "" {
				status = "TODO"
			}
			title := strings.TrimSpace(m[4])
			t := planTask{ID: strings.ToUpper(m[2]), Task: cutText(title, planTaskWarn), Pri: "P1", SP: "?", Status: status, Agent: "claude", Session: "-"}
			if m[3] != "" {
				t.Notes = m[3]
			}
			if len([]rune(title)) > planTaskWarn {
				t.Notes = strings.TrimSpace(t.Notes + " full title in PLAN.md backup")
			}
			if n, err := strconv.Atoi(strings.TrimPrefix(t.ID, "T-")); err == nil && n > maxID {
				maxID = n
			}
			tasks = append(tasks, t)
			cur = &tasks[len(tasks)-1]
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			cur = nil
			backlog = strings.Contains(strings.ToLower(line), "backlog")
			continue
		}
		if cur != nil {
			if m := oldFieldLine.FindStringSubmatch(line); m != nil {
				switch m[1] {
				case "Agent":
					if a := strings.ToLower(strings.TrimSpace(m[2])); planAgentOK(a) {
						cur.Agent = a
					}
				case "Scope":
					cur.Scope = backticked(m[2])
				case "Verify":
					if v := backticked(m[2]); len(v) > 0 {
						cur.Verify = v[0]
					}
				case "Depends":
					cur.Depends = regexp.MustCompile(`[A-Za-z]+-\d+`).FindAllString(m[2], -1)
				case "Result":
					if r := strings.TrimSpace(m[2]); r != "" && r != "-" {
						cur.Notes = strings.TrimSpace(cur.Notes + " " + r)
					}
				}
				continue
			}
		}
		// A plain bullet (Backlog or a loose note in Queue) becomes a TODO row.
		if strings.HasPrefix(strings.TrimSpace(line), "- ") && !strings.HasPrefix(line, "  ") {
			title := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
			status := "TODO"
			if m := regexp.MustCompile(`^\[(.)\]\s*`).FindStringSubmatch(title); m != nil {
				status = map[string]string{"x": "DONE", "X": "DONE", "~": "DOING", "!": "BLOCKED"}[m[1]]
				if status == "" {
					status = "TODO"
				}
				title = strings.TrimSpace(title[len(m[0]):])
			}
			if title == "" {
				continue
			}
			pri := "P2"
			if backlog {
				pri = "P3"
			}
			tasks = append(tasks, planTask{Task: cutText(title, planTaskWarn), Pri: pri, SP: "?", Status: status, Agent: "claude", Session: "-"})
			cur = nil
			continue
		}
		if strings.TrimSpace(line) != "" && cur == nil && !strings.HasPrefix(strings.TrimSpace(line), "Next ID") {
			skipped++
		}
	}
	for i := range tasks {
		if tasks[i].ID == "" {
			maxID++
			tasks[i].ID = fmt.Sprintf("T-%02d", maxID)
		}
		tasks[i].Notes = cutText(tasks[i].Notes, planNotesMax)
	}
	return tasks, skipped
}

func renderPlan(tasks []planTask) string {
	var b strings.Builder
	b.WriteString(planHeaderLine + "\n" + planSeparatorLine + "\n")
	for _, t := range tasks {
		b.WriteString(planRow(t) + "\n")
	}
	return b.String()
}

func isTablePlan(text string) bool {
	first, _, _ := strings.Cut(strings.TrimPrefix(text, utf8BOM), "\n")
	return strings.HasPrefix(strings.TrimSpace(first), "|")
}

func runPlan(args []string, out io.Writer) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	root := projectRoot(dir)
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return errors.New("không thấy repo git ở đây hay thư mục cha; cd vào project rồi chạy lại")
	}
	return runPlanIn(args, out, root)
}

func runPlanIn(args []string, out io.Writer, root string) error {
	if len(args) > 0 && contains([]string{"help", "-h", "-help", "--help"}, args[0]) {
		fmt.Fprintln(out, planHelp)
		return nil
	}
	sub := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	flags := flag.NewFlagSet("plan", flag.ContinueOnError)
	flags.SetOutput(out)
	dry := flags.Bool("dry-run", false, "chỉ xem, không ghi file")
	commit := flags.String("commit", "", "archive: commit merge của các task (mặc định tự tìm trong git log)")
	last := flags.Int("last", 20, "log: số task gần nhất")
	find := flags.String("find", "", "log: một ID, hoặc chữ trong Task / Notes")
	if sub == "archive" || sub == "worker" {
		// IDs come first: wj plan archive T-01 T-02 -commit abc123
		var ids, rest []string
		for i, a := range args {
			if strings.HasPrefix(a, "-") {
				rest = args[i:]
				break
			}
			ids = append(ids, a)
		}
		args = append(rest, ids...)
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if sub == "log" {
		if flags.NArg() != 0 {
			return errors.New("cách dùng: wj plan log [-last 50] [-find T-12]")
		}
		return planLog(out, root, *last, *find)
	}
	if sub == "worker" {
		return planWorker(out, root, flags.Args(), *dry)
	}
	path := filepath.Join(root, "PLAN.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("không đọc được %s: %w", path, err)
	}
	if sub == "archive" {
		return planArchive(out, root, string(data), flags.Args(), *commit, *dry)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("lệnh plan %s không nhận thêm tham số", sub)
	}
	switch sub {
	case "", "status":
		return planStatus(out, root, string(data))
	case "check":
		return planCheck(out, string(data))
	case "convert":
		return planConvert(out, root, string(data), *dry)
	}
	return fmt.Errorf("không hiểu lệnh plan %q; xem: wj plan help", sub)
}

func planCheck(out io.Writer, text string) error {
	if !isTablePlan(text) {
		return errors.New("PLAN.md đang là format cũ (checkbox); đổi sang bảng: wj plan convert")
	}
	tasks, issues := parsePlan(text)
	for _, issue := range issues {
		fmt.Fprintln(out, "  ", issue)
	}
	if len(issues) > 0 {
		return fmt.Errorf("PLAN.md có %d lỗi format", len(issues))
	}
	fmt.Fprintf(out, "PLAN.md đúng format: %d task.\n", len(tasks))
	return nil
}

func planStatus(out io.Writer, root, text string) error {
	if !isTablePlan(text) {
		return errors.New("PLAN.md đang là format cũ (checkbox); đổi sang bảng: wj plan convert")
	}
	tasks, issues := parsePlan(text)
	archive, _ := os.ReadFile(filepath.Join(root, "PLAN-ARCHIVE.md"))
	fmt.Fprintln(out, "NEXT ID:", nextPlanID(tasks, string(archive)))
	counts := map[string]int{}
	for _, t := range tasks {
		counts[t.Status]++
	}
	fmt.Fprintf(out, "COUNTS: todo %d, doing %d, done %d, blocked %d, partial %d\n",
		counts["TODO"], counts["DOING"], counts["DONE"], counts["BLOCKED"], counts["PARTIAL"])
	byID := map[string]planTask{}
	for _, t := range tasks {
		byID[t.ID] = t
	}
	var busy, ready []planTask
	var waiting []string
	for _, t := range tasks {
		if t.Status == "DOING" || t.Status == "DONE" {
			busy = append(busy, t)
		}
	}
	todo := make([]planTask, 0)
	for _, t := range tasks {
		if t.Status == "TODO" {
			todo = append(todo, t)
		}
	}
	sort.SliceStable(todo, func(i, j int) bool { return todo[i].Pri < todo[j].Pri })
	for _, t := range todo {
		var open []string
		for _, d := range t.Depends {
			if dep, ok := byID[d]; ok {
				open = append(open, d+"("+dep.Status+")")
			}
		}
		if len(open) > 0 {
			waiting = append(waiting, fmt.Sprintf("%s needs %s merged + archived", t.ID, strings.Join(open, ",")))
			continue
		}
		clash := ""
		for _, o := range append(append([]planTask{}, busy...), ready...) {
			if c := scopeClash(t.Scope, o.Scope); c != "" {
				clash = fmt.Sprintf("%s scope clash with %s: %s", t.ID, o.ID, c)
				break
			}
		}
		if clash != "" {
			waiting = append(waiting, clash)
			continue
		}
		ready = append(ready, t)
	}
	fmt.Fprintln(out, "ACTIVE (line, id, status, session, task):")
	for _, t := range tasks {
		if t.Status != "TODO" {
			fmt.Fprintf(out, "  %-4d %-6s %-8s %-14s %s\n", t.Line, t.ID, t.Status, t.Session, cutText(t.Task, 60))
		}
	}
	fmt.Fprintln(out, "READY (highest priority first; line, id, agent, pri, sp, task):")
	if len(ready) == 0 {
		fmt.Fprintln(out, "  (none)")
	}
	for _, t := range ready {
		fmt.Fprintf(out, "  %-4d %-6s %-14s %s SP%-2s %s\n", t.Line, t.ID, t.Agent, t.Pri, t.SP, cutText(t.Task, 60))
	}
	if len(waiting) > 0 {
		fmt.Fprintln(out, "WAITING:")
		for _, w := range waiting {
			fmt.Fprintln(out, "  "+w)
		}
	}
	if lines := planWorkerLines(root, tasks); len(lines) > 0 {
		fmt.Fprintln(out, "WORKERS (a model of another vendor runs as its own subagent, with no model parameter):")
		for _, line := range lines {
			fmt.Fprintln(out, line)
		}
	}
	for _, issue := range issues {
		fmt.Fprintln(out, "FORMAT:", issue)
	}
	return nil
}

func scopePrefix(s string) string {
	s = strings.TrimPrefix(strings.ReplaceAll(s, `\`, "/"), "./")
	if cut := strings.IndexAny(s, "*?["); cut >= 0 {
		s = s[:cut]
	}
	return strings.ToLower(strings.TrimRight(s, "/"))
}

func scopeClash(a, b []string) string {
	if len(a) == 0 || len(b) == 0 {
		return "(no Scope)"
	}
	for _, x := range a {
		for _, y := range b {
			px, py := scopePrefix(x), scopePrefix(y)
			if px == "" || py == "" || px == py || strings.HasPrefix(px, py+"/") || strings.HasPrefix(py, px+"/") {
				return x + " ~ " + y
			}
		}
	}
	return ""
}

// planLog prints archived tasks, so nobody has to read PLAN-ARCHIVE.md. Only
// table rows with every column count; older free text in the file is skipped.
func planLog(out io.Writer, root string, last int, find string) error {
	data, err := os.ReadFile(filepath.Join(root, "PLAN-ARCHIVE.md"))
	if err != nil {
		return fmt.Errorf("không đọc được PLAN-ARCHIVE.md: %w", err)
	}
	type archived struct {
		line  int
		cells []string
	}
	var rows, hits []archived
	for i, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		cells, ok := splitPlanRow(line)
		if ok && len(cells) == len(archiveColumns) && verifyIDPattern.MatchString(cells[0]) {
			rows = append(rows, archived{i + 1, cells})
		}
	}
	if find != "" {
		needle := strings.ToLower(find)
		for _, row := range rows {
			if strings.EqualFold(row.cells[0], find) || strings.Contains(strings.ToLower(row.cells[1]), needle) ||
				strings.Contains(strings.ToLower(row.cells[9]), needle) {
				hits = append(hits, row)
			}
		}
		fmt.Fprintf(out, "ARCHIVE: %d rows, matching '%s': %d\n", len(rows), find, len(hits))
	} else {
		hits = rows
		if last > 0 && len(hits) > last {
			hits = hits[len(hits)-last:]
		}
		fmt.Fprintf(out, "ARCHIVE: %d rows, last %d (line, id, status, commit, archived, task)\n", len(rows), len(hits))
	}
	for _, row := range hits {
		c := row.cells
		fmt.Fprintf(out, "  %-5d %-6s %-8s %-10s %-10s %s\n", row.line, c[0], c[4], c[11], c[12], cutText(c[1], 70))
	}
	return nil
}

// orcaSkillFiles are the kit's skills and its worker subagent, which wj owns:
// wj plan convert and wj init -update replace an older copy in a project.
var orcaSkillFiles = []string{
	".claude/skills/orca-orchestrator/SKILL.md",
	".claude/skills/orca-dream/SKILL.md",
	".claude/agents/orca-worker.md",
}

// orcaOldFiles are what older kits installed and this one replaced: the
// scripts by wj plan and wj verify, the worker skill by the orca-worker
// subagent. They go when the skills are refreshed. A folder's files are listed
// before the folder's own, so each empty folder can be removed on the way.
var orcaOldFiles = []string{
	".claude/skills/orca-orchestrator/scripts/plan.ps1",
	".claude/skills/orca-stateless-worker/scripts/fast_verify.ps1",
	".claude/skills/orca-stateless-worker/SKILL.md",
}

func planConvert(out io.Writer, root, text string, dry bool) error {
	var rendered string
	if isTablePlan(text) {
		if _, issues := parsePlan(text); len(issues) > 0 {
			return errors.New("PLAN.md đã là bảng nhưng sai format; xem: wj plan check")
		}
		fmt.Fprintln(out, "PLAN.md đã là bảng.")
	} else {
		tasks, skipped := convertOldPlan(text)
		rendered = renderPlan(tasks)
		if _, issues := parsePlan(rendered); len(issues) > 0 {
			for _, issue := range issues {
				fmt.Fprintln(out, "  ", issue)
			}
			return errors.New("bản chuyển đổi chưa đúng format; PLAN.md chưa bị đổi")
		}
		doing := 0
		for _, t := range tasks {
			if t.Status == "DOING" {
				doing++
			}
		}
		fmt.Fprintf(out, "PLAN.md: %d task sang bảng (%d dòng chữ khác bỏ qua, xem bản .bak).\n", len(tasks), skipped)
		if doing > 0 {
			fmt.Fprintf(out, "Lưu ý: %d task đang DOING. Worker đang chạy dùng skill cũ; báo kết quả cho orchestrator như cũ.\n", doing)
		}
	}
	var refresh []string
	for _, rel := range orcaSkillFiles {
		kit, err := kitFS.ReadFile(kitSource(rel))
		if err != nil {
			return err
		}
		path := filepath.Join(root, filepath.FromSlash(rel))
		old, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			// A project without .claude/ never had the skills: don't add them here.
			if !isDir(filepath.Join(root, ".claude")) {
				continue
			}
		} else if err != nil {
			return err
		} else if string(normalizeEOL(old)) == string(normalizeEOL(kit)) {
			continue
		}
		refresh = append(refresh, rel)
	}
	for _, rel := range refresh {
		fmt.Fprintln(out, "Cập nhật skill:", rel)
	}
	var obsolete []string
	for _, rel := range orcaOldFiles {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			obsolete = append(obsolete, rel)
			fmt.Fprintln(out, "Xoá file của kit cũ (kit mới không dùng nữa):", rel)
		}
	}
	if dry {
		fmt.Fprintln(out, "Dry run: không ghi file.")
		return nil
	}
	if rendered != "" {
		if err := writeFile(filepath.Join(root, "PLAN.md"), rendered, false); err != nil {
			return err
		}
	}
	for _, rel := range refresh {
		kit, _ := kitFS.ReadFile(kitSource(rel))
		if err := writeFile(filepath.Join(root, filepath.FromSlash(rel)), string(kit), false); err != nil {
			return err
		}
	}
	for _, rel := range obsolete {
		if err := removeOldFile(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			return err
		}
	}
	fmt.Fprintln(out, "Xong. File cũ được sao lưu cạnh nó (.bak-<giờ>). Commit PLAN.md và .claude/ trước khi chạy worker.")
	return nil
}

// PLAN-ARCHIVE.md is a table too: every PLAN.md column, then the merge commit
// and the archive date, so a finished task keeps its full row.
var archiveColumns = append(append([]string{}, planColumns...), "Commit", "Archived")

var (
	archiveHeaderLine    = "| " + strings.Join(archiveColumns, " | ") + " |"
	archiveSeparatorLine = "|" + strings.Repeat("---|", len(archiveColumns))
	commitPattern        = regexp.MustCompile(`^[0-9a-fA-F]{6,40}$`)
)

// findTaskCommit returns the newest commit on HEAD whose message names id.
func findTaskCommit(root, id string) string {
	cmd := exec.Command("git", "log", "-1", "--format=%h", "-E", "--grep="+regexp.QuoteMeta(id)+"([^0-9]|$)")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func planArchive(out io.Writer, root, text string, args []string, commit string, dry bool) error {
	if len(args) == 0 {
		return errors.New("cách dùng: wj plan archive T-01 [T-02=abc123 ...] [-commit abc123] [-dry-run]")
	}
	if !isTablePlan(text) {
		return errors.New("PLAN.md đang là format cũ; đổi trước: wj plan convert")
	}
	if commit != "" && !commitPattern.MatchString(commit) {
		return fmt.Errorf("-commit %q không phải mã commit", commit)
	}
	tasks, issues := parsePlan(text)
	if len(issues) > 0 {
		return errors.New("PLAN.md sai format; sửa trước (wj plan check)")
	}
	byID := map[string]planTask{}
	for _, t := range tasks {
		byID[t.ID] = t
	}
	commits := map[string]string{}
	var order []string
	for _, arg := range args {
		id, own, _ := strings.Cut(arg, "=")
		id = strings.ToUpper(strings.TrimSpace(id))
		t, ok := byID[id]
		if !ok {
			return fmt.Errorf("không có task %s trong PLAN.md", id)
		}
		if t.Status != "DONE" && t.Status != "PARTIAL" {
			return fmt.Errorf("%s đang %s; chỉ archive task DONE hoặc PARTIAL đã merge", id, t.Status)
		}
		if _, dup := commits[id]; !dup {
			order = append(order, id)
		}
		switch {
		case own != "":
			if !commitPattern.MatchString(own) {
				return fmt.Errorf("%s=%s không phải mã commit", id, own)
			}
			commits[id] = own
		case commit != "":
			commits[id] = commit
		default:
			commits[id] = findTaskCommit(root, id)
		}
	}
	today := time.Now().Format("2006-01-02")
	var rows strings.Builder
	for _, id := range order {
		c := commits[id]
		shown := c
		if c == "" {
			c, shown = "-", "(không tìm thấy, ghi -)"
		}
		rows.WriteString(strings.TrimSuffix(planRow(byID[id]), " |") + " | " + c + " | " + today + " |\n")
		fmt.Fprintf(out, "Archive %s  commit %s\n", id, shown)
	}
	if dry {
		fmt.Fprintln(out, "Dry run: không ghi file.")
		return nil
	}
	archivePath := filepath.Join(root, "PLAN-ARCHIVE.md")
	old, err := os.ReadFile(archivePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var add strings.Builder
	if !strings.Contains(string(old), archiveHeaderLine) {
		if len(bytes.TrimSpace(old)) > 0 {
			add.WriteString("\n")
		}
		add.WriteString(archiveHeaderLine + "\n" + archiveSeparatorLine + "\n")
	} else if len(old) > 0 && !bytes.HasSuffix(old, []byte("\n")) {
		add.WriteString("\n")
	}
	add.WriteString(rows.String())
	f, err := os.OpenFile(archivePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(add.String()); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Drop the archived rows; every other byte of PLAN.md stays as it was.
	drop := map[int]bool{}
	for _, id := range order {
		drop[byID[id].Line] = true
	}
	lines := strings.SplitAfter(text, "\n")
	var kept strings.Builder
	for i, line := range lines {
		if !drop[i+1] {
			kept.WriteString(line)
		}
	}
	if err := replaceFile(filepath.Join(root, "PLAN.md"), kept.String()); err != nil {
		return fmt.Errorf("đã ghi PLAN-ARCHIVE.md nhưng chưa xoá dòng khỏi PLAN.md: %w", err)
	}
	fmt.Fprintf(out, "Đã chuyển %d task sang PLAN-ARCHIVE.md. Commit PLAN.md và PLAN-ARCHIVE.md.\n", len(order))
	return nil
}

// replaceFile writes atomically without a .bak copy: PLAN.md lives in git.
func replaceFile(path, text string) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".wj-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(text); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
