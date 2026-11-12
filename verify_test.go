package main

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// verifyRepo is a git repo with a committed PLAN.md whose T-01 runs command.
func verifyRepo(t *testing.T, scope, command string) string {
	t.Helper()
	root := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	writeTestFile(t, filepath.Join(root, "PLAN.md"), planHeaderLine+"\n"+planSeparatorLine+"\n"+
		"| T-01 | Add JWT check | P1 | 3 | DOING | claude | - | "+scope+" | "+command+" | - | wt-auth |\n")
	git("init", "-q")
	git("add", "PLAN.md")
	git("-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "plan")
	return root
}

// linkedWorktree adds a linked git worktree of main and returns its folder.
func linkedWorktree(t *testing.T, main string) string {
	t.Helper()
	wt := filepath.Join(t.TempDir(), "wt")
	cmd := exec.Command("git", "worktree", "add", "-q", "--detach", wt)
	cmd.Dir = main
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v %s", err, out)
	}
	return wt
}

func TestVerifySharedCheckoutOnlyLooksAtScope(t *testing.T) {
	root := verifyRepo(t, "`src/`", "`git --version`")
	writeTestFile(t, filepath.Join(root, "other.txt"), "x")
	plan := filepath.Join(root, "PLAN.md")
	writeTestFile(t, plan, readText(t, plan)+"\n")
	text, code := runVerifyTest(t, root, "T-01")
	if code != 2 || !strings.Contains(verdictLine(text), "no changed file in the Scope of T-01") {
		t.Fatalf("no file in Scope must be NOT_VERIFIED; got exit %d:\n%s", code, text)
	}
	writeTestFile(t, filepath.Join(root, "src", "a.go"), "package a")
	text, code = runVerifyTest(t, root, "T-01")
	if code != 0 || verdictLine(text) != "VERIFY: PASS" || strings.Contains(text, "WARN") || strings.Contains(text, "other.txt") {
		t.Fatalf("want PASS without warnings about other workers' files; got exit %d:\n%s", code, text)
	}
}

func TestVerifySharedUnityWithEditorOpenSkipsBatchmode(t *testing.T) {
	root := verifyRepo(t, "`Assets/`", "-")
	writeTestFile(t, filepath.Join(root, "ProjectSettings", "ProjectVersion.txt"), "m_EditorVersion: 1.0\n")
	writeTestFile(t, filepath.Join(root, "Temp", "UnityLockfile"), "")
	writeTestFile(t, filepath.Join(root, "Assets", "A.cs"), "class A {}")
	text, code := runVerifyTest(t, root, "T-01")
	if code != 0 || verdictLine(text) != "VERIFY: PASS (scope only: Editor open, compile and tests not run)" {
		t.Fatalf("got exit %d:\n%s", code, text)
	}
}

func runVerifyTest(t *testing.T, root string, args ...string) (string, int) {
	t.Helper()
	var out bytes.Buffer
	err := runVerifyIn(args, &out, root)
	var code exitError
	if err != nil && !errors.As(err, &code) {
		t.Fatalf("unexpected error %v\n%s", err, out.String())
	}
	return out.String(), int(code)
}

func verdictLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return lines[len(lines)-1]
}

func TestVerifyRunsTheTaskCommandAndChecksScope(t *testing.T) {
	root := linkedWorktree(t, verifyRepo(t, "`src/`", "`git --version`"))
	writeTestFile(t, filepath.Join(root, "src", "auth.go"), "package auth")
	writeTestFile(t, filepath.Join(root, "other.txt"), "x")
	writeTestFile(t, filepath.Join(root, ".tmp", "scratch.log"), "x")

	text, code := runVerifyTest(t, root, "T-01")
	if code != 0 || verdictLine(text) != "VERIFY: PASS" {
		t.Fatalf("want PASS as the last line, exit 0; got exit %d:\n%s", code, text)
	}
	if !strings.Contains(text, "WARN out-of-scope: other.txt") {
		t.Errorf("file outside Scope not reported:\n%s", text)
	}
	if strings.Contains(text, "out-of-scope: src/auth.go") || strings.Contains(text, "out-of-scope: .tmp") {
		t.Errorf("in-scope and scratch files must not be reported:\n%s", text)
	}
	if !strings.Contains(text, "--- check: git --version ---") {
		t.Errorf("the Verify command from PLAN.md must run:\n%s", text)
	}
}

func TestVerifyFailIsAStrikeWithItsExitCode(t *testing.T) {
	root := verifyRepo(t, "`src/`", "`git no-such-command`")
	writeTestFile(t, filepath.Join(root, "src", "auth.go"), "package auth")
	text, code := runVerifyTest(t, root, "T-01")
	if code != 1 || !strings.HasPrefix(verdictLine(text), "VERIFY: FAIL (exit ") {
		t.Fatalf("want FAIL, exit 1; got exit %d:\n%s", code, text)
	}
	if !strings.Contains(text, "no-such-command") {
		t.Errorf("the failing output must be shown:\n%s", text)
	}
}

func TestVerifyReportsWhatItCouldNotCheck(t *testing.T) {
	root := verifyRepo(t, "-", "-")
	if text, code := runVerifyTest(t, root, "T-01"); code != 2 || verdictLine(text) != "VERIFY: NOT_VERIFIED (no changes in this worktree)" {
		t.Errorf("a clean worktree is not a pass; got exit %d:\n%s", code, text)
	}
	writeTestFile(t, filepath.Join(root, "notes.txt"), "x")
	if text, code := runVerifyTest(t, root, "T-09"); code != 2 || !strings.Contains(verdictLine(text), "task T-09 not found") {
		t.Errorf("unknown task; got exit %d:\n%s", code, text)
	}
	if text, code := runVerifyTest(t, root, "nonsense"); code != 2 || !strings.Contains(verdictLine(text), "bad task ID") {
		t.Errorf("bad ID; got exit %d:\n%s", code, text)
	}
	// No Verify command and no known stack in this folder.
	if text, code := runVerifyTest(t, root, "T-01"); code != 2 || !strings.Contains(verdictLine(text), "no known stack") {
		t.Errorf("nothing to run; got exit %d:\n%s", code, text)
	}
	if text, code := runVerifyTest(t, t.TempDir()); code != 2 || !strings.Contains(verdictLine(text), "not inside a git repository") {
		t.Errorf("outside git; got exit %d:\n%s", code, text)
	}
}

func TestVerifyWarnsAboutSharedFilesWhenTaskHasNoScope(t *testing.T) {
	root := verifyRepo(t, "-", "`git --version`")
	writeTestFile(t, filepath.Join(root, "CLAUDE.md"), "x")
	writeTestFile(t, filepath.Join(root, ".claude", "settings.json"), "{}")
	plan := filepath.Join(root, "PLAN.md")
	writeTestFile(t, plan, strings.Replace(readText(t, plan), "DOING", "DONE", 1))
	writeTestFile(t, filepath.Join(root, "src", "a.go"), "package a")
	text, _ := runVerifyTest(t, root, "T-01")
	for _, want := range []string{"WARN shared file: CLAUDE.md", "WARN shared file: .claude/settings.json", "WARN PLAN.md was edited"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "shared file: src/a.go") {
		t.Errorf("source files are not shared files:\n%s", text)
	}
}

func TestInScope(t *testing.T) {
	scope := []string{"src/auth.py", "./src/db/", "Assets/Scripts", "tests/*_auth.py", `docs\api`}
	for file, want := range map[string]bool{
		"src/auth.py":                   true,
		"SRC/Auth.py":                   true,
		"src/auth.pyc":                  false,
		"src/db/pool.py":                true,
		"src/dbx/pool.py":               false,
		"Assets/Scripts/Player.cs":      true,
		"Assets/Scripts/Player.cs.meta": true,
		"Assets/Scripts.meta":           true,
		"Assets/ScriptsOld/Player.cs":   false,
		"tests/test_auth.py":            true,
		"tests/unit/test_auth.py":       true,
		"tests/test_db.py":              false,
		"docs/api/index.md":             true,
	} {
		if got := inScope(file, scope); got != want {
			t.Errorf("inScope(%q) = %v, want %v", file, got, want)
		}
	}
}

func TestChangedPathsReadsPorcelainLines(t *testing.T) {
	got := changedPaths([]string{" M src/a.go", "?? new file.txt", "R  old.go -> src/new.go", `?? "quoted name.txt"`, "?? .tmp/log.txt", "x"})
	want := []string{"src/a.go", "new file.txt", "src/new.go", "quoted name.txt"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleanLinesKeepsWhatAReaderNeeds(t *testing.T) {
	got := cleanLines([]string{"\x1b[32mok\x1b[0m  ", "", "10%\r50%\r100% done\r", "   ", "last"})
	want := []string{"ok", "100% done", "last"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseUnityResults(t *testing.T) {
	run := parseUnityResults([]byte(`<?xml version="1.0" encoding="utf-8"?>
<test-run id="2" total="3" passed="2" failed="1">
  <test-suite type="Assembly" result="Failed">
    <failure><message>One or more child tests had errors</message></failure>
    <test-case fullname="Game.Tests.Adds" result="Passed" />
    <test-case fullname="Game.Tests.Divides" result="Failed">
      <failure>
        <message><![CDATA[  Expected: 2
  But was:  0
]]></message>
        <stack-trace>at Game.Tests.Divides ()</stack-trace>
      </failure>
    </test-case>
    <test-case fullname="Game.Tests.Subtracts" result="Passed" />
  </test-suite>
</test-run>`))
	if run.total != "3" || run.passed != "2" || run.failed != "1" {
		t.Errorf("counts = %+v", run)
	}
	if len(run.failures) != 1 || run.failures[0].name != "Game.Tests.Divides" || run.failures[0].message != "Expected: 2" {
		t.Errorf("failures = %+v", run.failures)
	}
}
