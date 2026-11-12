package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitRemoveKeepsWhatTheUserWrote(t *testing.T) {
	var moved []string
	fakeTrash(t, &moved)
	root := t.TempDir()
	runInitIn(t, root, "-type", "go")

	agents := filepath.Join(root, "AGENTS.md")
	writeTestFile(t, agents, "# My rules\n\nAlways be nice.\n\n"+readText(t, agents))
	plan := filepath.Join(root, "PLAN.md")
	writeTestFile(t, plan, readText(t, plan)+"| T-01 | my task |\n")
	settings := filepath.Join(root, ".claude", "settings.json")
	text := strings.Replace(readText(t, settings), `"PreToolUse": [`,
		`"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "my-hook"}]},`, 1)
	text = strings.Replace(text, `"Agent(orca-worker)",`, `"Agent(orca-worker)", "Bash(make *)",`, 1)
	writeTestFile(t, settings, text)

	before := readText(t, agents)
	out := runInitIn(t, root, "-remove", "-dry-run")
	if len(moved) != 0 || readText(t, agents) != before {
		t.Fatalf("dry run changed files: moved %v", moved)
	}
	if !strings.Contains(out, "CLAUDE.md") || !strings.Contains(out, "gỡ") {
		t.Fatalf("dry run did not list removals:\n%s", out)
	}

	runInitIn(t, root, "-remove")
	if got := readText(t, agents); got != "# My rules\n\nAlways be nice.\n" {
		t.Fatalf("AGENTS.md = %q", got)
	}
	if !strings.Contains(readText(t, plan), "T-01") {
		t.Fatal("edited PLAN.md was removed")
	}
	for _, rel := range []string{"CLAUDE.md", ".claude/skills", ".claude/agents", "PLAN-ARCHIVE.md"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("%s still there: %v", rel, err)
		}
	}
	if len(moved) == 0 {
		t.Fatal("nothing went through the trash")
	}
	got := readText(t, settings)
	for _, gone := range []string{"wj guard", "Agent(orca-worker)", "Bash(wj plan *)", "baseRef"} {
		if strings.Contains(got, gone) {
			t.Errorf("settings.json still has %q:\n%s", gone, got)
		}
	}
	for _, kept := range []string{"my-hook", "Bash(make *)", "Bash(git add *)", "BASH_MAX_OUTPUT_LENGTH"} {
		if !strings.Contains(got, kept) {
			t.Errorf("settings.json lost %q:\n%s", kept, got)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".gitignore")); err != nil {
		t.Errorf(".gitignore should stay: %v", err)
	}
}

func TestInitRemoveTrashesAgentsWithOnlyWjBlocks(t *testing.T) {
	var moved []string
	fakeTrash(t, &moved)
	root := t.TempDir()
	runInitIn(t, root, "-type", "go")
	runInitIn(t, root, "-remove")
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md with only wj blocks should be gone: %v", err)
	}
	if !contains(moved, filepath.Join(root, "AGENTS.md")) {
		t.Fatalf("AGENTS.md did not go to the trash: %v", moved)
	}
}

func TestInitRemoveRejectsInstallFlags(t *testing.T) {
	var out strings.Builder
	if err := runInit([]string{"-remove", "-update", t.TempDir()}, strings.NewReader(""), &out); err == nil {
		t.Fatal("-remove -update should fail")
	}
}

func TestRemoveWjBlocks(t *testing.T) {
	text := "top\n\n" + blockBegin("base") + "\nx\n" + blockEnd("base") + "\n\nmiddle\n\n" +
		blockBegin("go") + "\ny\n" + blockEnd("go") + "\n"
	got, count := removeWjBlocks(text)
	if count != 2 || got != "top\n\nmiddle\n" {
		t.Fatalf("got %q (%d)", got, count)
	}
	broken := "a\n" + blockBegin("go") + "\nno end\n"
	if got, count := removeWjBlocks(broken); count != 0 || got != broken {
		t.Fatalf("unterminated block changed: %q (%d)", got, count)
	}
}
