package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Checks about token use in the project wj doctor runs in: is the kit there and
// current, and how much context every session pays before any work starts.

const (
	// Rules, lessons and the memory index are sent with every turn of every
	// session and every worker. Above this, pruning pays for itself quickly.
	fixedRulesBudget = 3000 // tokens
	// The list of skills and agents the model can pick from is also always there.
	fixedListBudget = 2000 // tokens
)

// contextItem is one thing loaded into every session.
type contextItem struct {
	name   string
	tokens int // estimate: bytes / 4
}

// fixedContext is what a session in root starts with, as far as files show:
// instruction files (with the files they import), the memory index, and the
// one-line descriptions of skills and agents. The system prompt, built-in tools
// and MCP tools are not files, so they are not counted here.
type fixedContext struct {
	rules []contextItem
	list  []contextItem
}

func sumTokens(items []contextItem) int {
	total := 0
	for _, item := range items {
		total += item.tokens
	}
	return total
}

func readFixedContext(root, home string) fixedContext {
	var ctx fixedContext
	seen := map[string]bool{}
	var load func(path, label string, depth int)
	load = func(path, label string, depth int) {
		path = filepath.Clean(path)
		if seen[strings.ToLower(path)] || depth > 4 {
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		seen[strings.ToLower(path)] = true
		ctx.rules = append(ctx.rules, contextItem{label, len(data) / 4})
		// A line that is only @path imports that file.
		for _, line := range strings.Split(string(normalizeEOL(data)), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "@") || strings.ContainsAny(line, " \t") || len(line) < 2 {
				continue
			}
			target := line[1:]
			if strings.HasPrefix(target, "~") {
				target = filepath.Join(home, target[1:])
			} else if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(path), target)
			}
			load(target, filepath.ToSlash(strings.TrimPrefix(line[1:], "./")), depth+1)
		}
	}
	load(filepath.Join(root, "CLAUDE.md"), "CLAUDE.md", 0)
	load(filepath.Join(root, ".claude", "CLAUDE.md"), ".claude/CLAUDE.md", 0)
	load(filepath.Join(root, "CLAUDE.local.md"), "CLAUDE.local.md", 0)
	load(filepath.Join(home, ".claude", "CLAUDE.md"), "~/.claude/CLAUDE.md", 0)
	if data, err := os.ReadFile(filepath.Join(memoryDir(mainRepoRoot(root)), "MEMORY.md")); err == nil && len(data) > 0 {
		ctx.rules = append(ctx.rules, contextItem{"memory index MEMORY.md", len(data) / 4})
	}

	for _, source := range []struct{ dir, label, pattern string }{
		{filepath.Join(root, ".claude", "skills"), "skill ", filepath.Join("*", "SKILL.md")},
		{filepath.Join(home, ".claude", "skills"), "skill ~/", filepath.Join("*", "SKILL.md")},
		{filepath.Join(root, ".claude", "agents"), "agent ", "*.md"},
		{filepath.Join(home, ".claude", "agents"), "agent ~/", "*.md"},
	} {
		files, _ := filepath.Glob(filepath.Join(source.dir, source.pattern))
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			name, description, hidden := listedAs(string(normalizeEOL(data)))
			if hidden || description == "" {
				continue
			}
			if name == "" {
				name = strings.TrimSuffix(filepath.Base(file), ".md")
				if strings.EqualFold(filepath.Base(file), "SKILL.md") {
					name = filepath.Base(filepath.Dir(file))
				}
			}
			ctx.list = append(ctx.list, contextItem{source.label + name, (len(name) + len(description)) / 4})
		}
	}
	return ctx
}

// listedAs reads a skill's or agent's frontmatter: its name and description are
// what the model sees in every session. hidden is true for a skill only the
// user can start, which is not listed to the model.
func listedAs(text string) (name, description string, hidden bool) {
	if !strings.HasPrefix(text, "---\n") {
		return "", "", false
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return "", "", false
	}
	inDescription := false
	for _, line := range strings.Split(text[4:4+end], "\n") {
		key, value, found := strings.Cut(line, ":")
		indented := strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
		switch {
		case inDescription && indented:
			description += " " + strings.TrimSpace(line) // a folded description
			continue
		case !found || indented:
			inDescription = false
			continue
		}
		inDescription = false
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.TrimSpace(key) {
		case "name":
			name = value
		case "description":
			description = strings.TrimLeft(value, ">|-+ ")
			inDescription = true
		case "disable-model-invocation":
			hidden = value == "true"
		}
	}
	return name, strings.TrimSpace(description), hidden
}

// largest lists the biggest items, biggest first.
func largest(items []contextItem, count int) string {
	sorted := append([]contextItem{}, items...)
	sort.SliceStable(sorted, func(a, b int) bool { return sorted[a].tokens > sorted[b].tokens })
	var lines []string
	for i, item := range sorted {
		if i == count {
			lines = append(lines, fmt.Sprintf("... và %d mục khác", len(sorted)-count))
			break
		}
		lines = append(lines, fmt.Sprintf("~%-6s %s", compactTokens(int64(item.tokens)), item.name))
	}
	return strings.Join(lines, "\n")
}

// printFixedContext is the one line doctor always shows for a project, so the
// number is known before it becomes a problem.
func printFixedContext(out io.Writer, env doctorEnv) {
	root := projectRoot(env.cwd)
	if env.cwd == "" || !isGitRepo(root) {
		return
	}
	ctx := readFixedContext(root, env.home)
	if len(ctx.rules) == 0 && len(ctx.list) == 0 {
		return
	}
	fmt.Fprintf(out, "Context cố định của project này: ~%s token mỗi phiên (rule và memory ~%s, danh sách skill/agent ~%s).\n",
		compactTokens(int64(sumTokens(ctx.rules)+sumTokens(ctx.list))), compactTokens(int64(sumTokens(ctx.rules))),
		compactTokens(int64(sumTokens(ctx.list))))
	fmt.Fprintln(out, "Chưa tính system prompt, tool có sẵn và tool MCP. Số ước lượng: byte / 4.")
}

func checkFixedContext(env doctorEnv) []doctorFinding {
	root := projectRoot(env.cwd)
	if env.cwd == "" || !isGitRepo(root) {
		return nil
	}
	ctx := readFixedContext(root, env.home)
	var findings []doctorFinding
	if total := sumTokens(ctx.rules); total > fixedRulesBudget {
		findings = append(findings, doctorFinding{
			Key: "context:rules",
			Title: fmt.Sprintf("Rule và memory nạp mỗi phiên quá lớn: ~%s token (ngưỡng %s): %s",
				compactTokens(int64(total)), compactTokens(fixedRulesBudget), root),
			Detail: "Mọi lượt của mọi phiên và mọi worker đều gửi lại phần này.\n" + largest(ctx.rules, 6),
			Fix: "chạy /orca-dream để xoá rule và bài học cũ; phần chỉ vài task cần thì chuyển thành skill " +
				"hoặc file trong docs/ (chỉ nạp khi dùng tới)",
		})
	}
	if total := sumTokens(ctx.list); total > fixedListBudget {
		findings = append(findings, doctorFinding{
			Key: "context:skills",
			Title: fmt.Sprintf("Danh sách skill và agent nạp mỗi phiên quá lớn: ~%s token (ngưỡng %s)",
				compactTokens(int64(total)), compactTokens(fixedListBudget)),
			Detail: "Mô tả của từng skill và agent luôn nằm trong context, kể cả khi không dùng.\n" + largest(ctx.list, 6),
			Fix: "rút ngắn mô tả, xoá skill không dùng, hoặc thêm disable-model-invocation: true " +
				"cho skill chỉ gọi bằng /tên",
		})
	}
	return findings
}

// checkKit compares the project's kit with what this wj would install: a dry
// wj init. Whatever init would write is missing or out of date.
func checkKit(env doctorEnv) []doctorFinding {
	root := projectRoot(env.cwd)
	if env.cwd == "" || !isGitRepo(root) {
		return nil
	}
	agents, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	_, settingsErr := os.Stat(filepath.Join(root, ".claude", "settings.json"))
	hasKit := strings.Contains(string(agents), "<!-- wj:begin ")
	if !hasKit {
		// Only a project that already works with an agent is told about the kit.
		_, claudeErr := os.Stat(filepath.Join(root, "CLAUDE.md"))
		if settingsErr != nil && claudeErr != nil && len(agents) == 0 {
			return nil
		}
		return []doctorFinding{{
			Key:    "kit:" + filepath.Base(root),
			Title:  "Project chưa cài kit tiết kiệm token: " + root,
			Detail: "Chưa có rule đọc ít, file cấm đọc theo job, giới hạn output Bash và hook wj guard.",
			Fix:    "wj init (xem trước: wj init -dry-run; máy khác không có wj: wj init -lite)",
		}}
	}
	stacks := detectStacks(root)
	for _, stack := range initStacksList {
		if strings.Contains(string(agents), blockBegin(stack.id)) {
			stacks = appendUnique(stacks, stack.id)
		}
	}
	// A project set up with -lite stays lite: doctor adds nothing that needs wj.
	lite := !strings.Contains(string(agents), blockBegin("orca")) &&
		!isDir(filepath.Join(root, ".claude", "skills", "orca-orchestrator"))
	changes, err := planInit(root, stacks, false, lite)
	if err != nil {
		return nil
	}
	var lines []string
	for _, change := range changes {
		// settings.local.json is this machine's own file, not part of the kit's state.
		if !change.write && !strings.Contains(change.status, "wj init -update") || change.path == projectSettingsPath(root) {
			continue
		}
		rel, err := filepath.Rel(root, change.path)
		if err != nil {
			rel = change.path
		}
		lines = append(lines, filepath.ToSlash(rel)+": "+change.status)
	}
	if len(lines) == 0 {
		return nil
	}
	return []doctorFinding{{
		Key:    "kit:" + filepath.Base(root),
		Title:  fmt.Sprintf("Kit của project cũ hơn wj này (%d mục): %s", len(lines), root),
		Detail: strings.Join(lines, "\n"),
		Fix:    "wj init (chỉ thêm, không ghi đè file của bạn); skill cũ thì wj init -update",
		fix: func(out io.Writer) error {
			return applyInit(root, stacks, changes, false, lite, out)
		},
	}}
}
