package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

const guardHelp = `CHẶN LƯỢT ĐỌC TỐN TOKEN (hook PreToolUse của Claude Code)

  wj guard          đọc JSON của hook từ stdin, in quyết định ra stdout

wj init gắn lệnh này vào .claude/settings.json. Người dùng không cần tự chạy.
Luật trong AGENTS.md chỉ là lời nhắc; lệnh này chặn hẳn và nói cho model cách làm đúng:

  Read    file dài hơn 500 dòng mà không có limit (hoặc limit quá 500)
  Bash    cat / type / Get-Content in cả một file dài hơn 500 dòng
  Bash    cat, grep, head, tail, sed... vào đường dẫn đã có rule deny Read(...)
          trong .claude/settings.json (node_modules, lockfile, .env, Library...)

Đổi ngưỡng: đặt WJ_GUARD_MAX_LINES trong khối env của settings (0 = bỏ kiểm tra số dòng).
Lệnh không bao giờ làm hỏng phiên: gặp gì không hiểu thì cho qua.`

const (
	guardDefaultMaxLines = 500
	guardReadCap         = 16 << 20 // bytes of a file looked at when counting lines
)

// guardRule is one Read(...) deny rule of the project.
type guardRule struct {
	pattern string
	match   *regexp.Regexp
}

type guard struct {
	root     string // project (or worktree) root
	cwd      string
	rules    []guardRule
	maxLines int // 0 = no size check
}

// Commands that print file content. The dumpers print a whole file.
var (
	guardReaders = map[string]bool{"cat": true, "bat": true, "nl": true, "type": true, "get-content": true, "gc": true,
		"less": true, "more": true, "head": true, "tail": true, "grep": true, "egrep": true, "fgrep": true, "rg": true,
		"sed": true, "awk": true}
	guardDumpers = map[string]bool{"cat": true, "bat": true, "nl": true, "type": true, "get-content": true, "gc": true}
	// Their first plain argument is a pattern or a script, not a path.
	guardPatternFirst = map[string]bool{"grep": true, "egrep": true, "fgrep": true, "rg": true, "sed": true, "awk": true}
	// Read shows these as an image or a document, not as lines.
	guardNotText   = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true, ".pdf": true, ".ipynb": true}
	gitBashDrive   = regexp.MustCompile(`^/([a-zA-Z])/`)
	guardPartFlags = []string{"-total", "-tail", "-first", "-head", "-last"}
)

func runGuard(args []string, input io.Reader, out io.Writer) error {
	if len(args) > 0 {
		fmt.Fprintln(out, guardHelp)
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(input, 4<<20))
	if err != nil {
		return nil // a hook that fails would only add noise to the session
	}
	var call struct {
		Tool  string         `json:"tool_name"`
		Cwd   string         `json:"cwd"`
		Input map[string]any `json:"tool_input"`
	}
	if json.Unmarshal(data, &call) != nil || call.Input == nil {
		return nil
	}
	if call.Cwd == "" {
		if call.Cwd, err = os.Getwd(); err != nil {
			return nil
		}
	}
	g := newGuard(call.Cwd)
	reason := ""
	switch call.Tool {
	case "Read":
		path, _ := call.Input["file_path"].(string)
		reason = g.checkRead(path, guardNumber(call.Input["offset"]), guardNumber(call.Input["limit"]))
	case "Bash", "PowerShell":
		command, _ := call.Input["command"].(string)
		reason = g.checkCommand(command)
	}
	if reason == "" {
		return nil
	}
	return json.NewEncoder(out).Encode(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName":            "PreToolUse",
		"permissionDecision":       "deny",
		"permissionDecisionReason": reason,
	}})
}

func guardNumber(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

func newGuard(cwd string) *guard {
	g := &guard{root: projectRoot(cwd), cwd: cwd, maxLines: guardDefaultMaxLines}
	for _, name := range []string{"settings.json", "settings.local.json"} {
		doc, err := readConfig(filepath.Join(g.root, ".claude", name))
		if err != nil {
			continue
		}
		permissions, _ := doc["permissions"].(map[string]any)
		deny, _ := permissions["deny"].([]any)
		for _, value := range deny {
			rule, _ := value.(string)
			if !strings.HasPrefix(rule, "Read(") || !strings.HasSuffix(rule, ")") {
				continue
			}
			pattern := rule[len("Read(") : len(rule)-1]
			if match := guardPattern(pattern); match != nil {
				g.rules = append(g.rules, guardRule{pattern, match})
			}
		}
		// The session's env block reaches the hook; reading it here also covers a
		// hook started some other way.
		env, _ := doc["env"].(map[string]any)
		if text, ok := env["WJ_GUARD_MAX_LINES"].(string); ok {
			if n, err := strconv.Atoi(text); err == nil && n >= 0 {
				g.maxLines = n
			}
		}
	}
	if n, err := strconv.Atoi(os.Getenv("WJ_GUARD_MAX_LINES")); err == nil && n >= 0 {
		g.maxLines = n
	}
	return g
}

// guardPattern turns a Read(...) rule into a matcher for a path relative to the
// project root. A leading / ties the pattern to the root; any other pattern
// matches at any depth, which is stricter than Claude Code for a pattern with a
// folder in it and the same for the rest. A pattern that names a folder also
// covers what is inside. Rules for paths outside the project (//abs, ~/) are
// left to Claude Code.
func guardPattern(pattern string) *regexp.Regexp {
	if pattern == "" || strings.HasPrefix(pattern, "//") || strings.HasPrefix(pattern, "~") {
		return nil
	}
	var expr strings.Builder
	expr.WriteString("(?i)^")
	if strings.HasPrefix(pattern, "/") {
		pattern = pattern[1:]
	} else {
		pattern = strings.TrimPrefix(pattern, "./")
		expr.WriteString("(?:.*/)?")
	}
	pattern = strings.TrimSuffix(pattern, "/")
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; {
		case strings.HasPrefix(pattern[i:], "**/"):
			expr.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(pattern[i:], "**"):
			expr.WriteString(".*")
			i++
		case c == '*':
			expr.WriteString("[^/]*")
		case c == '?':
			expr.WriteString("[^/]")
		case c == '[':
			if end := strings.IndexByte(pattern[i:], ']'); end > 1 {
				expr.WriteString(pattern[i : i+end+1])
				i += end
				continue
			}
			expr.WriteString(`\[`)
		default:
			expr.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	expr.WriteString("(?:/.*)?$")
	match, err := regexp.Compile(expr.String())
	if err != nil {
		return nil
	}
	return match
}

// resolve gives the absolute path a tool or a shell command named.
func (g *guard) resolve(path string) string {
	if runtime.GOOS == "windows" {
		path = gitBashDrive.ReplaceAllString(path, "$1:/") // Git Bash writes C:\x as /c/x
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(g.cwd, path)
	}
	return filepath.Clean(path)
}

// blocked returns the deny rule that covers a path inside the project.
func (g *guard) blocked(abs string) string {
	rel, err := filepath.Rel(g.root, abs)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return ""
	}
	rel = filepath.ToSlash(rel)
	for _, rule := range g.rules {
		// rel + "/" lets a rule for a folder's content (x/**) cover the folder itself.
		if rule.match.MatchString(rel) || rule.match.MatchString(rel+"/") {
			return rule.pattern
		}
	}
	return ""
}

func (g *guard) show(abs string) string {
	if rel, err := filepath.Rel(g.root, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(abs)
}

// textLines counts the lines of a text file. ok is false for a folder, a
// missing file and a binary file.
func textLines(path string) (lines int, ok bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return 0, false
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer file.Close()
	buf := make([]byte, 64<<10)
	var total int64
	last := byte('\n')
	for total < guardReadCap {
		n, err := file.Read(buf)
		if n > 0 {
			if total == 0 && bytes.IndexByte(buf[:n], 0) >= 0 {
				return 0, false
			}
			lines += bytes.Count(buf[:n], []byte{'\n'})
			last = buf[n-1]
			total += int64(n)
		}
		if err != nil {
			break
		}
	}
	if last != '\n' {
		lines++
	}
	return lines, true
}

func (g *guard) checkRead(path string, offset, limit int) string {
	if path == "" {
		return ""
	}
	abs := g.resolve(path)
	if rule := g.blocked(abs); rule != "" {
		return fmt.Sprintf("%s is blocked in this project (deny rule Read(%s)). Don't read it another way either.", g.show(abs), rule)
	}
	if g.maxLines == 0 || guardNotText[strings.ToLower(filepath.Ext(abs))] {
		return ""
	}
	lines, ok := textLines(abs)
	if !ok {
		return ""
	}
	asked := lines
	if offset > 1 {
		asked = lines - (offset - 1)
	}
	if limit > 0 && limit < asked {
		asked = limit
	}
	if asked <= g.maxLines {
		return ""
	}
	return fmt.Sprintf("%s has %d lines and this Read would load %d; the project allows %d lines per Read. "+
		"Find the part you need with Grep (output_mode \"content\"), then Read it with offset and a limit of at most %d.",
		g.show(abs), lines, asked, g.maxLines, g.maxLines)
}

func (g *guard) checkCommand(command string) string {
	for _, pipeline := range shellPipelines(command) {
		for i, stage := range pipeline {
			if reason := g.checkStage(stage, i == len(pipeline)-1); reason != "" {
				return reason
			}
		}
	}
	return ""
}

// checkStage looks at one command of a pipeline. shown says its output goes to
// the model instead of into the next command.
func (g *guard) checkStage(words []string, shown bool) string {
	for len(words) > 0 && strings.Contains(words[0], "=") && !strings.ContainsAny(words[0], `/\`) {
		words = words[1:] // VAR=value before the command
	}
	if len(words) < 2 {
		return ""
	}
	name := strings.ToLower(filepath.Base(strings.ReplaceAll(words[0], `\`, "/")))
	name = strings.TrimSuffix(name, ".exe")
	if !guardReaders[name] {
		return ""
	}
	skipFirst := guardPatternFirst[name]
	var paths []string
	for i := 1; i < len(words); i++ {
		word := words[i]
		switch {
		case word == ">":
			shown = false
			i++
		case word == "2>":
			i++
		case word == "<":
		case strings.HasPrefix(word, "-"):
			lower := strings.ToLower(word)
			for _, flag := range guardPartFlags {
				if strings.HasPrefix(lower, flag) {
					shown = false // Get-Content -TotalCount 40: only a part is printed
				}
			}
			if skipFirst && (word == "-e" || word == "-f") {
				skipFirst = false // the pattern came with the flag
				i++
			}
		case skipFirst:
			skipFirst = false
		default:
			paths = append(paths, word)
		}
	}
	for _, path := range paths {
		abs := g.resolve(path)
		if rule := g.blocked(abs); rule != "" {
			return fmt.Sprintf("%s is blocked in this project (deny rule Read(%s)). Don't read it through the shell either.", g.show(abs), rule)
		}
		if !shown || !guardDumpers[name] || g.maxLines == 0 {
			continue
		}
		if lines, ok := textLines(abs); ok && lines > g.maxLines {
			return fmt.Sprintf("`%s` would print all %d lines of %s; the project allows %d lines per read. "+
				"Use Grep to find the part you need, then the Read tool with offset and limit.",
				name, lines, g.show(abs), g.maxLines)
		}
	}
	return ""
}

// shellPipelines splits a command line into pipelines (separated by && || ; &
// or a new line) and each pipeline into its commands, as words. It knows
// quotes and nothing else of the shell. It stops at a here-document, whose
// body is data, not commands.
func shellPipelines(command string) [][][]string {
	var (
		pipelines [][][]string
		pipeline  [][]string
		words     []string
		word      strings.Builder
		inWord    bool
	)
	endWord := func() {
		if inWord {
			words = append(words, word.String())
			word.Reset()
			inWord = false
		}
	}
	endStage := func() {
		endWord()
		if len(words) > 0 {
			pipeline = append(pipeline, words)
			words = nil
		}
	}
	endPipeline := func() {
		endStage()
		if len(pipeline) > 0 {
			pipelines = append(pipelines, pipeline)
			pipeline = nil
		}
	}
	runes := []rune(command)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}
		switch {
		case c == '\'' || c == '"':
			inWord = true
			for i++; i < len(runes) && runes[i] != c; i++ {
				if c == '"' && runes[i] == '\\' && i+1 < len(runes) && strings.ContainsRune("\"\\$`", runes[i+1]) {
					i++
				}
				word.WriteRune(runes[i])
			}
		// A backslash before a letter is a Windows path, not an escape.
		case c == '\\' && next != 0 && strings.ContainsRune(" \t'\"|&;<>\\", next):
			inWord = true
			word.WriteRune(next)
			i++
		case c == ' ' || c == '\t' || c == '\r':
			endWord()
		case c == '<' && next == '<':
			endPipeline()
			return pipelines
		case c == '>' || c == '<':
			// 2> and >& move stderr or join streams: stdout is still shown.
			token := string(c)
			if inWord && word.String() == "2" {
				word.Reset()
				inWord = false
				token = "2>"
			}
			endWord()
			if c == '>' && next == '>' {
				i++
				next = 0
				if i+1 < len(runes) {
					next = runes[i+1]
				}
			}
			if c == '>' && next == '&' {
				i++
				token = "2>"
			}
			words = append(words, token)
		case c == '|' && next != '|':
			endStage()
		case c == '|' || c == '&' || c == ';' || c == '\n':
			endPipeline()
			if (c == '|' || c == '&') && next == c {
				i++
			}
		default:
			inWord = true
			word.WriteRune(c)
		}
	}
	endPipeline()
	return pipelines
}
