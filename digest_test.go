package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// transcript builds the lines Claude Code writes for a session in cwd.
type transcript struct {
	t     *testing.T
	cwd   string
	when  time.Time
	lines []string
	calls int
}

func (tr *transcript) add(kind string, message any) {
	tr.t.Helper()
	tr.when = tr.when.Add(time.Second)
	raw, err := json.Marshal(map[string]any{"type": kind, "cwd": tr.cwd, "timestamp": tr.when.UTC().Format(time.RFC3339Nano), "message": message})
	if err != nil {
		tr.t.Fatal(err)
	}
	tr.lines = append(tr.lines, string(raw))
}

// tool records one tool call and its result.
func (tr *transcript) tool(name string, input map[string]any, result string, failed bool) {
	tr.calls++
	id := "toolu_" + filepath.Base(tr.cwd) + "_" + tr.when.Format("150405") + "_" + string(rune('a'+tr.calls))
	tr.add("assistant", map[string]any{"id": "msg_" + id, "usage": map[string]any{"input_tokens": 1000 * tr.calls},
		"content": []any{map[string]any{"type": "tool_use", "id": id, "name": name, "input": input}}})
	tr.add("user", map[string]any{"content": []any{map[string]any{"type": "tool_result", "tool_use_id": id, "is_error": failed, "content": result}}})
}

func (tr *transcript) say(text string) {
	tr.add("assistant", map[string]any{"id": "msg_say_" + tr.when.Format("150405"), "content": []any{map[string]any{"type": "text", "text": text}}})
}

func (tr *transcript) write(path string) {
	writeTestFile(tr.t, path, strings.Join(tr.lines, "\n")+"\n")
}

func TestDigestGroupsRepeatedFailuresAcrossSessions(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(t.TempDir(), "shop")
	other := filepath.Join(t.TempDir(), "other")
	worktree := filepath.Join(repo, ".claude", "worktrees", "agent-a1")
	for _, dir := range []string{filepath.Join(repo, ".git"), filepath.Join(other, ".git"), worktree} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, filepath.Join(worktree, ".git"), "gitdir: "+filepath.ToSlash(repo)+"/.git/worktrees/agent-a1\n")
	projects := filepath.Join(home, ".claude", "projects")
	start := time.Now().Add(-time.Hour)

	main := &transcript{t: t, cwd: repo, when: start}
	main.tool("Bash", map[string]any{"command": "npm test -- --watch=false"}, "Exit code 1\n> shop@1.0.0 test\nError: Cannot find module 'vitest' (run 12)\nnpm ERR! code 1", true)
	main.tool("Read", map[string]any{"file_path": filepath.Join(repo, "src", "cart.ts")}, "ok", false)
	main.tool("Bash", map[string]any{"command": "git push"}, "Permission to use Bash with command git push has been denied.", true)
	main.tool("Edit", map[string]any{"file_path": filepath.Join(repo, "src", "cart.ts")}, "String to replace not found in file.", true)
	main.write(filepath.Join(projects, "shop", "11111111-aaaa.jsonl"))

	// A worker subagent in its own worktree hits the same missing module.
	worker := &transcript{t: t, cwd: worktree, when: start.Add(time.Minute)}
	worker.tool("Bash", map[string]any{"command": "npm test -- --watch=false"}, "Exit code 1\nError: Cannot find module 'vitest' (run 7)", true)
	worker.say("TASK: T-02 | STATUS: BLOCKED | FILES: src/cart.ts | SUMMARY: vitest missing in worktree")
	worker.write(filepath.Join(projects, "shop", "11111111-aaaa", "subagents", "agent-a1b2c3d4e5.jsonl"))
	writeTestFile(t, filepath.Join(projects, "shop", "11111111-aaaa", "subagents", "agent-a1b2c3d4e5.meta.json"),
		`{"agentType":"orca-worker","description":"T-02 cart discount"}`)

	// Another project's failures must not leak in.
	foreign := &transcript{t: t, cwd: other, when: start}
	foreign.tool("Bash", map[string]any{"command": "make deploy"}, "Exit code 2\nmake: *** No rule to make target", true)
	foreign.write(filepath.Join(projects, "other", "22222222-bbbb.jsonl"))

	report := readDigest([]string{filepath.Join(home, ".claude")}, repo, time.Now().AddDate(0, 0, -7))
	var out bytes.Buffer
	report.print(&out, "shop", 7)
	text := out.String()

	for _, want := range []string{
		"1 sessions + 1 subagents, 4 tool errors (1 denied), handovers: 0 pass, 1 not",
		"2x in 2 sessions  Bash `npm test -- --watch=false` -> Error: Cannot find module 'vitest' (run 12)",
		"sessions: 11111111, 11111111/a1b2c3d4",
		"1x in 1 sessions  Bash `git push`",
		"11111111/a1b2c3d4  TASK: T-02 | STATUS: BLOCKED",
		"orca-worker: T-02 cart discount",
		"ONE-OFF errors: 1",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "make deploy") || strings.Contains(text, "22222222") {
		t.Errorf("another project's session leaked in:\n%s", text)
	}
	if strings.Contains(text, repo) {
		t.Errorf("paths must be shown relative to the project:\n%s", text)
	}

	out.Reset()
	if err := report.printSession(&out, "11111111/a1b2"); err != nil {
		t.Fatal(err)
	}
	if detail := out.String(); !strings.Contains(detail, "Cannot find module 'vitest' (run 7)") || strings.Contains(detail, "git push") {
		t.Errorf("-session must list only that session's errors:\n%s", detail)
	}
	if err := report.printSession(&out, "nope"); err == nil {
		t.Error("an unknown session id must be an error")
	}
}

func TestDigestCountsACopiedCallOnce(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(t.TempDir(), "shop")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A resumed session copies its old lines into a new file.
	tr := &transcript{t: t, cwd: repo, when: time.Now().Add(-time.Hour)}
	tr.tool("Bash", map[string]any{"command": "go vet ./..."}, "Exit code 1\nvet: undefined: foo", true)
	projects := filepath.Join(home, ".claude", "projects", "shop")
	tr.write(filepath.Join(projects, "aaaaaaaa-1111.jsonl"))
	tr.write(filepath.Join(projects, "bbbbbbbb-2222.jsonl"))
	report := readDigest([]string{filepath.Join(home, ".claude")}, repo, time.Now().AddDate(0, 0, -7))
	if len(report.errors) != 1 {
		t.Errorf("want the copied failure counted once, got %d", len(report.errors))
	}
}

func TestProblemLinePicksTheLineThatNamesTheProblem(t *testing.T) {
	for text, want := range map[string]string{
		"Exit code 1\nrunning 3 tests\n--- FAIL: TestCart (0.01s)\nFAIL\nexit status 1": "--- FAIL: TestCart (0.01s)",
		"Exit code 1\nall good\nVERIFY: NOT_VERIFIED (node_modules missing)":            "VERIFY: NOT_VERIFIED (node_modules missing)",
		"Exit code 127\nline one\nline two":                                             "line two",
		"":                                                                              "(no output)",
	} {
		if got := problemLine(text); got != want {
			t.Errorf("problemLine(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestInProjectRecognisesWorktreesAndRemovedFolders(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "shop")
	linked := filepath.Join(t.TempDir(), "orca", "shop-wt")
	for _, dir := range []string{filepath.Join(repo, ".git"), filepath.Join(repo, "src"), linked} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, filepath.Join(linked, ".git"), "gitdir: "+filepath.ToSlash(repo)+"/.git/worktrees/shop-wt\n")
	for cwd, want := range map[string]bool{
		repo:                       true,
		filepath.Join(repo, "src"): true,
		linked:                     true, // a worktree outside the repo folder
		filepath.Join(repo, ".claude", "worktrees", "gone"): true, // removed worktree, known by its path
		repo + "-two":      false,
		filepath.Dir(repo): false,
		"relative/path":    false,
	} {
		if got := inProject(cwd, repo); got != want {
			t.Errorf("inProject(%q) = %v, want %v", cwd, got, want)
		}
	}
}
