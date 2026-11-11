package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const digestHelp = `TÓM TẮT LỖI TRONG CÁC PHIÊN CLAUDE CODE CỦA PROJECT (cho /orca-dream)

  wj digest                  7 ngày gần nhất của project đang đứng
  wj digest -days 14         khoảng thời gian khác
  wj digest -session 1acf    mọi lỗi của một phiên (vài ký tự đầu của id là đủ)

Transcript của Claude Code nặng nhiều MB, không nên đưa cho model đọc. Lệnh này đọc
thay, rồi in bản rút gọn:

  REPEATED   cùng một lỗi tool ở 2 phiên trở lên, hoặc 3 lần trở lên
  DENIED     lệnh bị hỏi quyền hoặc bị chặn
  HANDOVERS  worker báo BLOCKED hoặc PARTIAL
  SESSIONS   mỗi phiên: số lượt, context lớn nhất, số lỗi

Tính cả phiên chính, subagent, và mọi worktree của cùng repo. Lệnh chỉ đọc, không
gửi gì đi đâu. Mỗi dòng có id phiên để trích dẫn.`

// digestError is one failed tool call.
type digestError struct {
	session string // session id, plus "/" + agent id for a subagent
	when    time.Time
	tool    string
	input   string // what was asked: the command, the file, the pattern
	text    string // the line of the result that says what went wrong
	denied  bool
}

type digestSession struct {
	id          string
	sub         bool
	agent       agentMeta
	title       string
	first, last time.Time
	turns       int
	peak        int64 // largest prompt of any call
	errors      int
	interrupts  int
}

type digestReport struct {
	sessions  map[string]*digestSession
	errors    []digestError
	handovers []string          // "session  TASK: ... | STATUS: BLOCKED ..."
	passed    int               // handovers with STATUS: PASS
	calls     map[string]string // tool_use id -> "tool\x00input", until its result arrives
	seen      map[string]bool   // tool_use ids and message ids already counted
}

var (
	handoverPattern = regexp.MustCompile(`(?m)^TASK: \S+ \| STATUS: (PASS|PARTIAL|BLOCKED)\b.*$`)
	deniedPattern   = regexp.MustCompile(`(?i)permission|requires approval|not allowed|was blocked|blocked by|denied|doesn't want to proceed`)
	// A line that says what failed is worth more than the last line of output.
	problemPattern = regexp.MustCompile(`(?i)VERIFY: |error|fail|cannot|can't|not found|no such|undefined|exception|panic|invalid|unknown|refus|timed? ?out`)
	numberPattern  = regexp.MustCompile(`\b[0-9a-f]{7,40}\b|\d+`)
)

func runDigest(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("digest", flag.ContinueOnError)
	flags.SetOutput(out)
	days := flags.Int("days", 7, "số ngày gần nhất")
	session := flags.String("session", "", "in mọi lỗi của một phiên (đầu id)")
	if len(args) > 0 && contains([]string{"help", "-h", "-help", "--help"}, args[0]) {
		fmt.Fprintln(out, digestHelp)
		return nil
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *days < 1 {
		return errors.New("cách dùng: wj digest [-days 7] [-session <id>]")
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	// Every worktree of a repo is the same project: count them all.
	root := filepath.Clean(mainRepoRoot(dir))
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return errors.New("không thấy repo git ở đây hay thư mục cha; cd vào project rồi chạy lại")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	since := time.Now().AddDate(0, 0, -*days)
	report := readDigest(claudeDataDirs(home), root, since)
	if *session != "" {
		return report.printSession(out, *session)
	}
	report.print(out, filepath.Base(root), *days)
	return nil
}

// inProject reports whether a session that ran in cwd belongs to the repo whose
// main checkout is root: the repo itself or any of its worktrees. A worktree
// under the repo that was removed since still resolves, by its path.
func inProject(cwd, root string) bool {
	cwd = strings.TrimPrefix(cwd, `\\?\`)
	return filepath.IsAbs(cwd) && strings.EqualFold(filepath.Clean(mainRepoRoot(cwd)), root)
}

func readDigest(claudeDirs []string, root string, since time.Time) *digestReport {
	report := &digestReport{sessions: map[string]*digestSession{}, calls: map[string]string{}, seen: map[string]bool{}}
	member := map[string]bool{} // cwd -> in this project
	for _, dir := range claudeDirs {
		_ = filepath.WalkDir(filepath.Join(dir, "projects"), func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".jsonl") {
				return nil
			}
			if info, err := entry.Info(); err != nil || info.ModTime().Before(since) {
				return nil
			}
			report.readFile(path, root, since, member)
			return nil
		})
	}
	// Oldest first, whatever order the files were read in.
	sort.SliceStable(report.errors, func(a, b int) bool { return report.errors[a].when.Before(report.errors[b].when) })
	return report
}

type digestBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	Text      string          `json:"text"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	Content   json.RawMessage `json:"content"`
}

func (r *digestReport) readFile(path, root string, since time.Time, member map[string]bool) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	id, sub := sessionOf(path)
	key := id
	if sub {
		key += "/" + agentOf(path)
	}
	var session *digestSession
	// Lines can be many megabytes (pasted images), so no bufio.Scanner.
	reader := bufio.NewReaderSize(f, 256<<10)
	for {
		raw, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var line struct {
			Type      string `json:"type"`
			Cwd       string `json:"cwd"`
			Timestamp string `json:"timestamp"`
			Title     string `json:"aiTitle"`
			Message   struct {
				ID    string `json:"id"`
				Usage struct {
					Input      int64 `json:"input_tokens"`
					CacheRead  int64 `json:"cache_read_input_tokens"`
					CacheWrite int64 `json:"cache_creation_input_tokens"`
				} `json:"usage"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if !bytes.Contains(raw, []byte(`"cwd"`)) && !bytes.Contains(raw, []byte(`"aiTitle"`)) {
			continue
		}
		if json.Unmarshal(raw, &line) != nil {
			continue
		}
		if line.Type == "ai-title" {
			if session != nil && line.Title != "" {
				session.title = line.Title
			}
			continue
		}
		if line.Type != "user" && line.Type != "assistant" {
			continue
		}
		if line.Cwd != "" {
			ours, known := member[line.Cwd]
			if !known {
				ours = inProject(line.Cwd, root)
				member[line.Cwd] = ours
			}
			if !ours {
				return // another project's session
			}
		}
		when, err := time.Parse(time.RFC3339Nano, line.Timestamp)
		if err != nil || when.Before(since) {
			continue
		}
		if session == nil {
			if session = r.sessions[key]; session == nil {
				session = &digestSession{id: key, sub: sub, first: when}
				if sub {
					session.agent = readAgentMeta(path)
				}
				r.sessions[key] = session
			}
		}
		if when.After(session.last) {
			session.last = when
		}
		if when.Before(session.first) {
			session.first = when
		}
		if line.Type == "assistant" {
			usage := line.Message.Usage
			if size := usage.Input + usage.CacheRead + usage.CacheWrite; size > session.peak {
				session.peak = size
			}
			// One reply is written as several lines; its message id counts it once.
			if id := line.Message.ID; id != "" && !r.seen["m:"+id] {
				r.seen["m:"+id] = true
				session.turns++
			}
		}
		var blocks []digestBlock
		if json.Unmarshal(line.Message.Content, &blocks) != nil {
			var text string // a plain prompt is a string, not a list of blocks
			if json.Unmarshal(line.Message.Content, &text) == nil && strings.Contains(text, "[Request interrupted by user") {
				session.interrupts++
			}
			continue
		}
		for _, block := range blocks {
			switch block.Type {
			case "tool_use":
				r.calls[block.ID] = block.Name + "\x00" + toolInput(block.Name, block.Input, root)
			case "tool_result":
				call, known := r.calls[block.ToolUseID]
				delete(r.calls, block.ToolUseID)
				if !block.IsError || r.seen["t:"+block.ToolUseID] {
					continue
				}
				r.seen["t:"+block.ToolUseID] = true
				tool, input, _ := strings.Cut(call, "\x00")
				if !known {
					tool = "?"
				}
				text := resultText(block.Content)
				session.errors++
				r.errors = append(r.errors, digestError{session: key, when: when, tool: tool, input: input,
					text: problemLine(text), denied: deniedPattern.MatchString(cutRunes(text, 400))})
			case "text":
				if line.Type == "user" && strings.Contains(block.Text, "[Request interrupted by user") {
					session.interrupts++
				}
				if line.Type != "assistant" {
					continue
				}
				for _, m := range handoverPattern.FindAllStringSubmatch(block.Text, -1) {
					if m[1] == "PASS" {
						r.passed++
					} else {
						r.handovers = append(r.handovers, key+"  "+cutRunes(m[0], 200))
					}
				}
			}
		}
	}
}

// toolInput is the part of a tool call worth showing: the command, the file or
// the pattern, with the project's own path cut off.
func toolInput(tool string, raw json.RawMessage, root string) string {
	var input map[string]any
	if json.Unmarshal(raw, &input) != nil {
		return ""
	}
	text := ""
	for _, field := range []string{"command", "file_path", "pattern", "path", "url", "skill", "subagent_type", "query"} {
		if value, ok := input[field].(string); ok && value != "" {
			text = value
			break
		}
	}
	for _, prefix := range []string{root, filepath.ToSlash(root), strings.ReplaceAll(root, `\`, `\\`)} {
		text = strings.ReplaceAll(text, prefix, ".")
	}
	return cutRunes(strings.Join(strings.Fields(text), " "), 120)
}

// resultText is a tool result as text: Claude Code writes either a string or a
// list of text blocks.
func resultText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		parts = append(parts, block.Text)
	}
	return strings.Join(parts, "\n")
}

// problemLine picks the one line of a failed result that names the problem:
// the first that reads like an error, else the last line of output.
func problemLine(text string) string {
	lines := cleanLines(strings.Split(text, "\n"))
	if len(lines) == 0 {
		return "(no output)"
	}
	pick := lines[len(lines)-1]
	for _, line := range lines {
		if strings.HasPrefix(line, "Exit code ") || strings.HasPrefix(line, "<") {
			continue
		}
		if problemPattern.MatchString(line) {
			pick = line
			break
		}
	}
	return cutRunes(strings.TrimSpace(pick), 160)
}

// signature groups failures that are the same problem: the tool, the start of
// what was asked, and the error with its numbers taken out.
func (e digestError) signature() string {
	words := strings.Fields(e.input)
	if len(words) > 3 {
		words = words[:3]
	}
	head := strings.Join(words, " ")
	if e.tool != "Bash" && e.tool != "PowerShell" {
		head = "" // the same mistake on different files is one pattern
	}
	return e.tool + "\x00" + head + "\x00" + numberPattern.ReplaceAllString(e.text, "N")
}

type digestGroup struct {
	sample   digestError
	count    int
	sessions []string
}

func (r *digestReport) groups(denied bool) []digestGroup {
	index := map[string]int{}
	var groups []digestGroup
	for _, e := range r.errors {
		if e.denied != denied {
			continue
		}
		sig := e.signature()
		i, ok := index[sig]
		if !ok {
			i = len(groups)
			index[sig] = i
			groups = append(groups, digestGroup{sample: e})
		}
		groups[i].count++
		groups[i].sessions = appendUnique(groups[i].sessions, shortSession(e.session))
	}
	sort.SliceStable(groups, func(a, b int) bool {
		if len(groups[a].sessions) != len(groups[b].sessions) {
			return len(groups[a].sessions) > len(groups[b].sessions)
		}
		return groups[a].count > groups[b].count
	})
	return groups
}

// shortSession keeps enough of an id to cite and to pass to -session.
func shortSession(key string) string {
	id, agent, sub := strings.Cut(key, "/")
	if len(id) > 8 {
		id = id[:8]
	}
	if !sub {
		return id
	}
	if len(agent) > 8 {
		agent = agent[:8]
	}
	return id + "/" + agent
}

func (g digestGroup) line() string {
	what := g.sample.tool
	if g.sample.input != "" {
		what += " `" + g.sample.input + "`"
	}
	shown := g.sessions
	more := ""
	if len(shown) > 5 {
		more = fmt.Sprintf(" +%d", len(shown)-5)
		shown = shown[:5]
	}
	return fmt.Sprintf("  %dx in %d sessions  %s -> %s\n      sessions: %s%s", g.count, len(g.sessions), what, g.sample.text,
		strings.Join(shown, ", "), more)
}

func (r *digestReport) print(out io.Writer, project string, days int) {
	mains, subs, denied := 0, 0, 0
	for _, s := range r.sessions {
		if s.sub {
			subs++
		} else {
			mains++
		}
	}
	for _, e := range r.errors {
		if e.denied {
			denied++
		}
	}
	fmt.Fprintf(out, "DIGEST: %s, last %d days: %d sessions + %d subagents, %d tool errors (%d denied), handovers: %d pass, %d not\n",
		project, days, mains, subs, len(r.errors), denied, r.passed, len(r.handovers))
	if len(r.sessions) == 0 {
		fmt.Fprintln(out, "  (no Claude Code sessions for this project in that time)")
		return
	}

	oneOff := 0
	fmt.Fprintln(out, "REPEATED (same failure in 2+ sessions, or 3+ times):")
	shown := 0
	for _, g := range r.groups(false) {
		if len(g.sessions) < 2 && g.count < 3 {
			oneOff += g.count
			continue
		}
		if shown++; shown <= 15 {
			fmt.Fprintln(out, g.line())
		}
	}
	if shown == 0 {
		fmt.Fprintln(out, "  (none)")
	} else if shown > 15 {
		fmt.Fprintf(out, "  ... and %d more patterns\n", shown-15)
	}

	if groups := r.groups(true); len(groups) > 0 {
		fmt.Fprintln(out, "DENIED (permission prompts and blocks):")
		for i, g := range groups {
			if i == 10 {
				fmt.Fprintf(out, "  ... and %d more\n", len(groups)-10)
				break
			}
			fmt.Fprintln(out, g.line())
		}
	}

	if len(r.handovers) > 0 {
		fmt.Fprintln(out, "HANDOVERS not PASS:")
		for i, h := range r.handovers {
			if i == 10 {
				fmt.Fprintf(out, "  ... and %d more\n", len(r.handovers)-10)
				break
			}
			id, text, _ := strings.Cut(h, "  ")
			fmt.Fprintf(out, "  %s  %s\n", shortSession(id), text)
		}
	}

	sessions := make([]*digestSession, 0, len(r.sessions))
	for _, s := range r.sessions {
		sessions = append(sessions, s)
	}
	sort.Slice(sessions, func(a, b int) bool {
		if sessions[a].errors != sessions[b].errors {
			return sessions[a].errors > sessions[b].errors
		}
		return sessions[a].last.After(sessions[b].last)
	})
	fmt.Fprintln(out, "SESSIONS (most errors first; id, last active, turns, peak context, errors, interrupts, what):")
	for i, s := range sessions {
		if i == 20 {
			fmt.Fprintf(out, "  ... and %d more\n", len(sessions)-20)
			break
		}
		what := s.title
		if s.sub {
			what = strings.TrimSpace(s.agent.Type + ": " + s.agent.Description)
		}
		fmt.Fprintf(out, "  %-17s %s  %4d turns  %6s  %3d err  %d int  %s\n", shortSession(s.id), s.last.Local().Format("01-02 15:04"),
			s.turns, compactTokens(s.peak), s.errors, s.interrupts, cutRunes(what, 60))
	}
	if oneOff > 0 {
		fmt.Fprintf(out, "ONE-OFF errors: %d, not listed. For one session's errors: wj digest -session <id>\n", oneOff)
	}
}

func (r *digestReport) printSession(out io.Writer, prefix string) error {
	prefix = strings.ToLower(prefix)
	var keys []string
	for key := range r.sessions {
		if strings.HasPrefix(strings.ToLower(key), prefix) || strings.HasPrefix(shortSession(key), prefix) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return fmt.Errorf("không có phiên nào của project này bắt đầu bằng %q trong khoảng thời gian đó", prefix)
	}
	sort.Strings(keys)
	for _, key := range keys {
		s := r.sessions[key]
		fmt.Fprintf(out, "SESSION %s: %d turns, peak context %s, %d errors, %d interrupts\n", key, s.turns, compactTokens(s.peak), s.errors, s.interrupts)
		shown := 0
		for _, e := range r.errors {
			if e.session != key {
				continue
			}
			if shown++; shown > 60 {
				continue
			}
			mark := "error "
			if e.denied {
				mark = "denied"
			}
			fmt.Fprintf(out, "  %s %s  %s `%s` -> %s\n", e.when.Local().Format("15:04"), mark, e.tool, e.input, e.text)
		}
		if shown > 60 {
			fmt.Fprintf(out, "  ... and %d more\n", shown-60)
		}
	}
	return nil
}
