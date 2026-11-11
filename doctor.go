package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const doctorHelp = `wj doctor - tự kiểm tra máy, báo vấn đề, sửa được thì sửa

  wj doctor             kiểm tra rồi hỏi sửa từng mục (Enter = sửa)
  wj doctor -yes        sửa hết không hỏi
  wj doctor -dry-run    chỉ báo, không sửa gì
  wj doctor -ai         nhờ AI (qua gateway của bạn) đọc báo cáo và gợi ý thêm;
                        chỉ gửi các dòng báo cáo, không gửi key hay nội dung code
  wj doctor -since 72h  khoảng thời gian xem lại phiên Claude (mặc định 24h)

  Đang kiểm tra:
  - Tool search bị tắt giữa phiên (gateway làm hỏng tool_reference): mỗi lượt
    gửi lại mọi định nghĩa tool, đọc cache tăng vọt. Sửa: cho project đó đi qua
    proxy wj (proxy tự vá tên tool), bật proxy chạy nền.
  - Proxy đang chạy bản addon cũ: sửa bằng chạy lại proxy với addon mới.
  - Project bật đo token nhưng proxy không chạy: Claude không kết nối được.
  - MCP lớn cài cho mọi project nhưng chỉ vài project dùng: chuyển về đúng project.
  - PLAN.md sai format bảng: sửa bằng wj plan convert.

  Không có AI (không gateway, mất mạng) thì chỉ dùng các luật sửa cứng ở trên.`

// doctorFinding is one problem. fix is nil when it needs a human; the text then
// says what to do.
type doctorFinding struct {
	Key    string
	Title  string
	Detail string
	Fix    string
	fix    func(out io.Writer) error
}

type doctorEnv struct {
	home  string
	since time.Duration
	cwd   string
}

func runDoctor(args []string, input io.Reader, out io.Writer) error {
	if len(args) > 0 && contains([]string{"help", "-h", "-help", "--help"}, args[0]) {
		fmt.Fprintln(out, doctorHelp)
		return nil
	}
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(out)
	assumeYes := flags.Bool("yes", false, "sửa hết, không hỏi")
	dry := flags.Bool("dry-run", false, "chỉ báo, không sửa")
	ai := flags.Bool("ai", false, "nhờ AI qua gateway gợi ý thêm")
	since := flags.Duration("since", 24*time.Hour, "khoảng xem lại phiên Claude")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cwd, _ := os.Getwd()
	env := doctorEnv{home: home, since: *since, cwd: cwd}
	findings := doctorCheck(env)
	if len(findings) == 0 {
		fmt.Fprintln(out, "Không thấy vấn đề nào.")
	}
	for i, f := range findings {
		fmt.Fprintf(out, "\n%d. %s\n", i+1, f.Title)
		for _, line := range strings.Split(f.Detail, "\n") {
			fmt.Fprintln(out, "   "+line)
		}
		if f.Fix != "" {
			fmt.Fprintln(out, "   Sửa:", f.Fix)
		}
	}
	if *ai {
		doctorAdvice(out, home, findings)
	}
	fixable := 0
	for _, f := range findings {
		if f.fix != nil {
			fixable++
		}
	}
	if *dry || fixable == 0 {
		return nil
	}
	fmt.Fprintln(out)
	var failures []error
	for i, f := range findings {
		if f.fix == nil {
			continue
		}
		if !*assumeYes {
			if !interactive(input) {
				fmt.Fprintln(out, "Không có terminal để hỏi; chạy lại với -yes để sửa, hoặc -dry-run để chỉ xem.")
				return nil
			}
			answer := menuAnswer(input, out, fmt.Sprintf("Sửa mục %d (%s)? [Y/n]", i+1, f.Key))
			if answer != "" && !yes(answer) {
				continue
			}
		}
		if err := f.fix(out); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", f.Key, err))
			fmt.Fprintf(out, "   [x] %s: %v\n", f.Key, err)
			continue
		}
		fmt.Fprintf(out, "   [ok] %s\n", f.Key)
	}
	return errors.Join(failures...)
}

func doctorCheck(env doctorEnv) []doctorFinding {
	var findings []doctorFinding
	findings = append(findings, checkToolSearch(env)...)
	findings = append(findings, checkProxyHealth(env)...)
	findings = append(findings, checkUserMCP(env)...)
	findings = append(findings, checkPlanFormat(env)...)
	return findings
}

// --- tool search fallback ---------------------------------------------------

const toolSearchMarker = "ToolSearch is no longer available"

type fallbackHit struct {
	Root     string
	Sessions int
	After    int64 // cache read after the fallback, in the window
}

// scanToolSearchFallback walks recent Claude transcripts and finds sessions
// where Claude Code dropped tool search mid-session.
func scanToolSearchFallback(home string, since time.Time) map[string]*fallbackHit {
	hits := map[string]*fallbackHit{}
	root := filepath.Join(home, ".claude", "projects")
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		root = filepath.Join(dir, "projects")
	}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.ModTime().Before(since) {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		reader := bufio.NewReaderSize(f, 1<<20)
		cwd, fallen := "", false
		var after int64
		seen := map[string]bool{}
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				if cwd == "" {
					if i := bytes.Index(line, []byte(`"cwd":"`)); i >= 0 {
						var v struct {
							Cwd string `json:"cwd"`
						}
						if json.Unmarshal(line, &v) == nil {
							cwd = v.Cwd
						}
					}
				}
				if !fallen && bytes.Contains(line, []byte(toolSearchMarker)) {
					fallen = true
				}
				if fallen && bytes.Contains(line, []byte(`"cache_read_input_tokens"`)) && bytes.Contains(line, []byte(`"type":"assistant"`)) {
					var v struct {
						Timestamp string `json:"timestamp"`
						Message   struct {
							ID    string `json:"id"`
							Usage struct {
								CacheRead int64 `json:"cache_read_input_tokens"`
							} `json:"usage"`
						} `json:"message"`
					}
					if json.Unmarshal(line, &v) == nil && !seen[v.Message.ID] {
						seen[v.Message.ID] = true
						if when, err := time.Parse(time.RFC3339Nano, v.Timestamp); err == nil && !when.Before(since) {
							after += v.Message.Usage.CacheRead
						}
					}
				}
			}
			if err != nil {
				break
			}
		}
		if !fallen || cwd == "" {
			return nil
		}
		projectRootDir := mainRepoRoot(cwd)
		h := hits[projectRootDir]
		if h == nil {
			h = &fallbackHit{Root: projectRootDir}
			hits[projectRootDir] = h
		}
		h.Sessions++
		h.After += after
		return nil
	})
	return hits
}

// mainRepoRoot is the main checkout of cwd's repository (worktrees fold into it).
func mainRepoRoot(cwd string) string {
	dir := filepath.Clean(cwd)
	for {
		data, err := os.ReadFile(filepath.Join(dir, ".git"))
		if err == nil {
			if main := worktreeMain(string(data)); main != "" {
				return main
			}
			return dir
		}
		if isDir(filepath.Join(dir, ".git")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Clean(cwd)
		}
		dir = parent
	}
}

func projectProxied(root string) bool {
	doc, err := readJSONObject(projectSettingsPath(root))
	if err != nil {
		return false
	}
	env, _ := doc["env"].(map[string]any)
	return env != nil && env[projectMarkerKey] != nil
}

func checkToolSearch(env doctorEnv) []doctorFinding {
	hits := scanToolSearchFallback(env.home, time.Now().Add(-env.since))
	var roots []string
	for root := range hits {
		roots = append(roots, root)
	}
	sort.Slice(roots, func(i, j int) bool { return hits[roots[i]].After > hits[roots[j]].After })
	var findings []doctorFinding
	for _, root := range roots {
		h := hits[root]
		if !isDir(root) {
			continue
		}
		proxied := projectProxied(root)
		detail := fmt.Sprintf("%s: %d phiên Claude bị tắt tool search; sau đó đọc cache %s token.\n"+
			"Nguyên nhân hay gặp: gateway đổi tên tool nhưng không đổi tool_reference (lỗi 400 Tool reference not found).",
			root, h.Sessions, compactTokens(h.After))
		f := doctorFinding{Key: "tool-search:" + filepath.Base(root), Title: "Tool search bị tắt giữa phiên: " + filepath.Base(root), Detail: detail}
		if proxied {
			f.Detail += "\nProject đã đi qua proxy wj. Phiên cũ vẫn ở chế độ tắt: mở phiên Claude MỚI."
			if !portOpen(defaultProxyPort) {
				f.Fix = "bật proxy chạy nền"
				f.fix = func(out io.Writer) error { return ensureProxyRunning(env.home, out) }
			}
		} else {
			f.Fix = "cho project đi qua proxy wj (tự vá tên tool) và bật proxy chạy nền; mở phiên Claude mới sau đó"
			f.fix = func(out io.Writer) error {
				if err := ensureProxySetup(env.home, out); err != nil {
					return err
				}
				if err := projectOn([]string{"-dir", root}, out, env.home); err != nil {
					return err
				}
				return ensureProxyRunning(env.home, out)
			}
		}
		findings = append(findings, f)
	}
	return findings
}

// --- proxy health ------------------------------------------------------------

func checkProxyHealth(env doctorEnv) []doctorFinding {
	var findings []doctorFinding
	index := loadProjectIndex(env.home)
	running := portOpen(defaultProxyPort)
	if len(index) > 0 && !running {
		var names []string
		for _, name := range index {
			names = append(names, name)
		}
		sort.Strings(names)
		findings = append(findings, doctorFinding{
			Key:    "proxy-down",
			Title:  "Proxy đo token không chạy",
			Detail: fmt.Sprintf("Các project %s đi qua proxy ở cổng %d; khi proxy tắt, Claude ở đó không kết nối được.", strings.Join(names, ", "), defaultProxyPort),
			Fix:    "bật proxy chạy nền",
			fix:    func(out io.Writer) error { return ensureProxyRunning(env.home, out) },
		})
	}
	installed, _ := os.ReadFile(filepath.Join(proxyDir(env.home), "usage_meter.py"))
	if len(installed) > 0 && !bytes.Equal(normalizeEOL(installed), normalizeEOL(proxyAddon)) {
		f := doctorFinding{
			Key:    "proxy-addon",
			Title:  "Proxy đang dùng addon cũ",
			Detail: "~/.wj/proxy/usage_meter.py khác bản trong wj này (thiếu bản vá mới nhất).",
			Fix:    "ghi addon mới; nếu proxy đang chạy thì khởi động lại nó",
		}
		f.fix = func(out io.Writer) error {
			if err := writeAddon(env.home); err != nil {
				return err
			}
			if running {
				return restartProxy(env.home, out)
			}
			return nil
		}
		findings = append(findings, f)
	}
	return findings
}

func ensureProxySetup(home string, out io.Writer) error {
	if _, err := os.Stat(mitmCAPath(home)); err == nil {
		return nil
	}
	return proxySetup([]string{"-yes"}, strings.NewReader(""), out, home)
}

// --- MCP scope ---------------------------------------------------------------

// heavyMCPThreshold: user-scope servers used by at most this many projects are
// moved to those projects, so other projects stop loading their schemas.
const heavyMCPThreshold = 2

func claudeJSONPath(home string) string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, ".claude.json")
	}
	return filepath.Join(home, ".claude.json")
}

// mcpUsage counts real MCP tool calls per server and project root.
func mcpUsage(home string, since time.Time) map[string]map[string]int {
	usage := map[string]map[string]int{}
	root := filepath.Join(home, ".claude", "projects")
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.ModTime().Before(since) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || !bytes.Contains(data, []byte(`"name":"mcp__`)) {
			return nil
		}
		cwd := ""
		if i := bytes.Index(data, []byte(`"cwd":"`)); i >= 0 {
			end := bytes.IndexByte(data[i+7:], '"')
			if end > 0 {
				var s string
				if json.Unmarshal(data[i+6:i+7+end+1], &s) == nil {
					cwd = s
				}
			}
		}
		if cwd == "" {
			return nil
		}
		project := mainRepoRoot(cwd)
		for _, chunk := range bytes.Split(data, []byte(`"type":"tool_use","id":"`))[1:] {
			i := bytes.Index(chunk, []byte(`"name":"mcp__`))
			if i < 0 || i > 80 {
				continue
			}
			rest := chunk[i+len(`"name":"mcp__`):]
			end := bytes.Index(rest, []byte("__"))
			if end <= 0 {
				continue
			}
			server := string(rest[:end])
			if usage[server] == nil {
				usage[server] = map[string]int{}
			}
			usage[server][project]++
		}
		return nil
	})
	return usage
}

func checkUserMCP(env doctorEnv) []doctorFinding {
	doc, err := readJSONObject(claudeJSONPath(env.home))
	if err != nil {
		return nil
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	if len(servers) == 0 {
		return nil
	}
	usage := mcpUsage(env.home, time.Now().Add(-30*24*time.Hour))
	var names []string
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	var findings []doctorFinding
	for _, name := range names {
		projects := usage[name]
		if len(projects) > heavyMCPThreshold {
			continue
		}
		var roots []string
		for root := range projects {
			if isDir(root) {
				roots = append(roots, root)
			}
		}
		sort.Strings(roots)
		where := "không project nào gọi nó trong 30 ngày"
		if len(roots) > 0 {
			where = "chỉ " + strings.Join(roots, ", ") + " gọi nó trong 30 ngày"
		}
		server, _ := json.Marshal(servers[name])
		name, roots := name, roots
		findings = append(findings, doctorFinding{
			Key:   "mcp-scope:" + name,
			Title: fmt.Sprintf("MCP %q nạp cho mọi project", name),
			Detail: where + ".\nMỗi phiên ở project khác vẫn nạp định nghĩa tool của nó; nếu tool search bị tắt, " +
				"chúng bị gửi lại ở mọi lượt.",
			Fix: moveMCPFixText(roots),
			fix: func(out io.Writer) error { return moveMCPToProjects(env.home, name, string(server), roots, out) },
		})
	}
	return findings
}

func moveMCPFixText(roots []string) string {
	if len(roots) == 0 {
		return "gỡ khỏi phạm vi mọi project (thêm lại khi cần: claude mcp add-json -s local ...)"
	}
	return "chuyển về phạm vi riêng của " + strings.Join(roots, ", ")
}

// moveMCPToProjects adds the server to each project's local scope first, then
// removes the user-scope copy, using Claude Code's own command so its config
// file is never edited by hand. A failure leaves the user-scope copy in place.
func moveMCPToProjects(home, name, server string, roots []string, out io.Writer) error {
	claude, err := exec.LookPath("claude")
	if err != nil {
		return errors.New("không thấy lệnh claude trong PATH")
	}
	for _, root := range roots {
		cmd := exec.Command(claude, "mcp", "add-json", "-s", "local", name, server)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil && !bytes.Contains(output, []byte("already exists")) {
			return fmt.Errorf("thêm %s vào %s: %w: %s", name, root, err, strings.TrimSpace(lastLine(output)))
		}
		fmt.Fprintf(out, "   đã thêm MCP %s cho %s\n", name, root)
	}
	cmd := exec.Command(claude, "mcp", "remove", "-s", "user", name)
	cmd.Dir = home
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gỡ %s khỏi phạm vi mọi project: %w: %s", name, err, strings.TrimSpace(lastLine(output)))
	}
	fmt.Fprintf(out, "   đã gỡ MCP %s khỏi phạm vi mọi project (bản cũ: %s.backup của claude)\n", name, claudeJSONPath(home))
	return nil
}

func lastLine(b []byte) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	return lines[len(lines)-1]
}

// --- PLAN.md format ------------------------------------------------------------

func checkPlanFormat(env doctorEnv) []doctorFinding {
	roots := map[string]bool{}
	if env.cwd != "" {
		roots[projectRoot(env.cwd)] = true
	}
	for root := range loadProjectIndex(env.home) {
		roots[root] = true
	}
	var findings []doctorFinding
	var sorted []string
	for root := range roots {
		sorted = append(sorted, root)
	}
	sort.Strings(sorted)
	for _, root := range sorted {
		data, err := os.ReadFile(filepath.Join(root, "PLAN.md"))
		if err != nil {
			continue
		}
		text := string(data)
		root := root
		if !isTablePlan(text) {
			findings = append(findings, doctorFinding{
				Key:    "plan:" + filepath.Base(root),
				Title:  "PLAN.md kiểu cũ: " + root,
				Detail: "PLAN.md chưa phải một bảng duy nhất nên không trích task nhanh được.",
				Fix:    "wj plan convert (sao lưu bản cũ, cập nhật skill orca của project)",
				fix: func(out io.Writer) error {
					return planConvert(out, root, text, false)
				},
			})
			continue
		}
		if _, issues := parsePlan(text); len(issues) > 0 {
			var lines []string
			for i, issue := range issues {
				if i == 5 {
					lines = append(lines, fmt.Sprintf("... và %d lỗi khác (wj plan check)", len(issues)-5))
					break
				}
				lines = append(lines, issue.String())
			}
			findings = append(findings, doctorFinding{
				Key:    "plan:" + filepath.Base(root),
				Title:  fmt.Sprintf("PLAN.md sai format (%d lỗi): %s", len(issues), root),
				Detail: strings.Join(lines, "\n"),
				Fix:    "sửa tay theo từng dòng trên; orchestrator cũng thấy các lỗi này trong wj plan",
			})
		}
	}
	return findings
}

// --- AI advice -------------------------------------------------------------------

// doctorAdvice sends only the finding titles and details (paths and counts, no
// keys, no code) to the user's own gateway and prints the answer. No gateway or
// no network: it says so and the fixed rules above still apply.
func doctorAdvice(out io.Writer, home string, findings []doctorFinding) {
	target := claudeTarget(home)
	if target == nil {
		fmt.Fprintln(out, "\nAI: chưa có gateway Claude trong ~/.claude/settings.json; chỉ dùng luật sửa cứng.")
		return
	}
	var report strings.Builder
	for i, f := range findings {
		fmt.Fprintf(&report, "%d. %s\n%s\nFix wj will apply: %s\n\n", i+1, f.Title, f.Detail, f.Fix)
	}
	if len(findings) == 0 {
		report.WriteString("No problems found by the fixed rules.\n")
	}
	prompt := "You advise on the cost and health of a developer machine running Claude Code and Codex through a company gateway. " +
		"Below is the report from the machine's doctor tool. In at most 8 short bullet lines, in Vietnamese: say which fix matters most, " +
		"anything the fixed rules may have missed or got wrong, and one concrete next step per item. Do not repeat the report.\n\n" + report.String()
	model := "claude-sonnet-5-5"
	if root, err := readSettings(claudeSettingsPath(home)); err == nil {
		if env, _ := root["env"].(map[string]any); env != nil {
			if m, _ := env["ANTHROPIC_DEFAULT_SONNET_MODEL"].(string); m != "" {
				model = m
			}
		}
	}
	answer, err := askGateway(target.Conn, model, prompt)
	if err != nil {
		fmt.Fprintln(out, "\nAI: không hỏi được gateway ("+err.Error()+"); chỉ dùng luật sửa cứng.")
		return
	}
	fmt.Fprintln(out, "\nAI gợi ý ("+model+"):")
	for _, line := range strings.Split(strings.TrimSpace(answer), "\n") {
		fmt.Fprintln(out, "   "+line)
	}
}

func askGateway(conn connectionProfile, model, prompt string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model": model, "max_tokens": 1200,
		"messages": []map[string]any{{"role": "user", "content": prompt}},
	})
	req, err := http.NewRequest(http.MethodPost, strings.TrimSuffix(conn.BaseURL, "/")+"/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	agent, app := claudeCodeUserAgent, "cli"
	if conn.NeutralHeaders {
		agent, app = neutralUserAgent, neutralApp
	}
	req.Header.Set("User-Agent", agent)
	req.Header.Set("x-app", app)
	if conn.Header == "x-api-key" {
		req.Header.Set("x-api-key", conn.Credential)
	} else {
		req.Header.Set("Authorization", "Bearer "+conn.Credential)
	}
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
	client := &http.Client{Timeout: 120 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", errors.New("không kết nối được")
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	var reply struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &reply); err != nil {
		return "", errors.New("trả lời không đọc được")
	}
	var text []string
	for _, c := range reply.Content {
		if c.Type == "text" {
			text = append(text, c.Text)
		}
	}
	if len(text) == 0 {
		return "", errors.New("trả lời rỗng")
	}
	return strings.Join(text, "\n"), nil
}
