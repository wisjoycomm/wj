package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// A worker on a model of another vendor. Claude Code's Agent tool takes only
// its own aliases as the model of a subagent, but a subagent file may name any
// model id, which is then sent to the gateway as it is. So every such model
// gets its own copy of the worker: orca-worker-<slug>.md with model: <id>.

const workerAgent = "orca-worker"

var (
	// workerAliases are the models the Agent tool accepts itself.
	workerAliases     = []string{"haiku", "sonnet", "opus", "fable"}
	workerModelID     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/\[\]-]*$`)
	workerSlugCut     = regexp.MustCompile(`[^a-z0-9]+`)
	workerNameLine    = regexp.MustCompile(`(?m)^name:.*$`)
	workerModelLine   = regexp.MustCompile(`(?m)^model:.*\n`)
	workerDescription = regexp.MustCompile(`(?m)^description:.*$`)
)

// workerFor is the subagent that runs a task whose Agent is claude:<model>:
// the plain worker for an alias (the model goes in the Agent call), and the
// model's own worker for any other id.
func workerFor(model string) string {
	if model == "" || contains(workerAliases, strings.ToLower(model)) {
		return workerAgent
	}
	return workerAgent + "-" + strings.Trim(workerSlugCut.ReplaceAllString(strings.ToLower(model), "-"), "-")
}

func workerPath(root, name string) string {
	return filepath.Join(root, ".claude", "agents", name+".md")
}

// workerText is the plain worker's file turned into the worker of one model.
func workerText(base, name, model string) (string, error) {
	text := string(normalizeEOL([]byte(base)))
	if !strings.HasPrefix(text, "---\n") || !workerNameLine.MatchString(text) {
		return "", errors.New(".claude/agents/orca-worker.md không có frontmatter với dòng name")
	}
	text = workerModelLine.ReplaceAllString(text, "")
	text = workerNameLine.ReplaceAllString(text, "name: "+name+"\nmodel: "+model)
	if line := workerDescription.FindString(text); line != "" && !strings.Contains(line, model) {
		text = strings.Replace(text, line, strings.TrimRight(line, " ")+" This one runs on the model "+model+".", 1)
	}
	return text, nil
}

func planWorker(out io.Writer, root string, models []string, dry bool) error {
	if len(models) == 0 {
		files, _ := filepath.Glob(workerPath(root, workerAgent+"-*"))
		if len(files) == 0 {
			fmt.Fprintln(out, "Chưa có worker nào cho model hãng khác. Tạo: wj plan worker <model id của gateway>")
			return nil
		}
		sort.Strings(files)
		fmt.Fprintln(out, "Worker cho model hãng khác (giá trị cột Agent, subagent):")
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			model := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(workerModelLine.FindString(string(normalizeEOL(data)))), "model:"))
			fmt.Fprintf(out, "  claude:%-28s %s\n", model, strings.TrimSuffix(filepath.Base(file), ".md"))
		}
		return nil
	}
	base, err := os.ReadFile(workerPath(root, workerAgent))
	if errors.Is(err, os.ErrNotExist) {
		base, err = kitFS.ReadFile(kitSource(".claude/agents/" + workerAgent + ".md"))
	}
	if err != nil {
		return fmt.Errorf("không đọc được worker gốc: %w", err)
	}
	settingsPath := filepath.Join(root, ".claude", "settings.json")
	doc, err := readConfig(settingsPath)
	if err != nil {
		return fmt.Errorf("file %s bị lỗi cú pháp; tool không sửa nó", settingsPath)
	}
	var allow []any
	for _, model := range models {
		if !workerModelID.MatchString(model) {
			return fmt.Errorf("model id %q không hợp lệ (chỉ chữ, số và . _ / - [ ])", model)
		}
		name := workerFor(model)
		if name == workerAgent {
			fmt.Fprintf(out, "%s: không cần worker riêng, ghi thẳng claude:%s vào cột Agent.\n", model, strings.ToLower(model))
			continue
		}
		text, err := workerText(string(base), name, model)
		if err != nil {
			return err
		}
		path := workerPath(root, name)
		status := "tạo"
		if old, err := os.ReadFile(path); err == nil {
			status = "cập nhật"
			if bytes.Equal(normalizeEOL(old), []byte(text)) {
				status = "đã đúng"
			}
		}
		if !dry && status != "đã đúng" {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
				return err
			}
		}
		fmt.Fprintf(out, "  .claude/agents/%s.md  %s   -> cột Agent: claude:%s\n", name, status, model)
		allow = append(allow, "Agent("+name+")")
	}
	if len(allow) == 0 {
		return nil
	}
	if mergeSettings(doc, map[string]any{"permissions": map[string]any{"allow": allow}}) && !dry {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(doc); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(settingsPath, buf.Bytes(), 0o644); err != nil {
			return err
		}
	}
	if dry {
		fmt.Fprintln(out, "Dry run: không ghi file.")
		return nil
	}
	fmt.Fprintln(out, "\nTiếp theo:")
	fmt.Fprintln(out, "  1. Model phải chạy được qua Claude Code: wj models test (cột Claude Code phải OK).")
	fmt.Fprintln(out, "  2. Ghi giá trị cột Agent ở trên vào .claude/models.md cho loại việc model đó nhận.")
	fmt.Fprintln(out, "  3. Commit, rồi mở lại Claude Code: subagent mới chỉ được nạp khi mở phiên.")
	return nil
}

// planWorkerLines tells the orchestrator which subagent runs each claude:<id>
// that is not an alias, and whether that subagent is installed.
func planWorkerLines(root string, tasks []planTask) []string {
	var lines []string
	seen := map[string]bool{}
	for _, t := range tasks {
		model, found := strings.CutPrefix(t.Agent, "claude:")
		if !found || t.Status == "DONE" || seen[model] {
			continue
		}
		seen[model] = true
		name := workerFor(model)
		if name == workerAgent {
			continue
		}
		state := "ok"
		if _, err := os.Stat(workerPath(root, name)); err != nil {
			state = "MISSING: run wj plan worker " + model + ", commit, reopen Claude Code"
		}
		lines = append(lines, fmt.Sprintf("  claude:%s -> subagent_type %s (%s)", model, name, state))
	}
	return lines
}
