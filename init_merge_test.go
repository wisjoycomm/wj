package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tests never start a real agent: by default nothing is merged.
func init() {
	newInitMerger = func(io.Writer) func(string, string, string) (string, error) { return nil }
}

// fakeMerger appends the kit text under the user's and counts its calls.
func fakeMerger(t *testing.T, calls *int) {
	old := newInitMerger
	newInitMerger = func(io.Writer) func(string, string, string) (string, error) {
		return func(rel, mine, kit string) (string, error) {
			*calls++
			return mine + "\nMERGED " + rel + "\n" + kit, nil
		}
	}
	t.Cleanup(func() { newInitMerger = old })
}

func TestInitMergesKitIntoAUsersFileOnceAndKeepsABackup(t *testing.T) {
	calls := 0
	fakeMerger(t, &calls)
	root := t.TempDir()
	claudeMD := filepath.Join(root, "CLAUDE.md")
	plan := filepath.Join(root, "PLAN.md")
	writeTestFile(t, claudeMD, "my own rules\n")
	writeTestFile(t, plan, "| my plan |\n")

	output := runInitIn(t, root, "-type", "go")
	got := readText(t, claudeMD)
	if !strings.HasPrefix(got, "my own rules\n") || !strings.Contains(got, "MERGED CLAUDE.md") || !strings.Contains(output, "gộp bản kit") {
		t.Fatalf("CLAUDE.md not merged:\n%s\noutput:\n%s", got, output)
	}
	if backups, _ := filepath.Glob(claudeMD + ".bak-*"); len(backups) != 1 {
		t.Errorf("want one backup, got %d", len(backups))
	}
	if readText(t, plan) != "| my plan |\n" {
		t.Error("PLAN.md is the user's and must never be merged")
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "wj-merged.json")); err != nil {
		t.Fatal("the merged kit versions must be recorded")
	}
	before := calls
	runInitIn(t, root, "-type", "go")
	if calls != before {
		t.Errorf("a rerun with the same kit asked the agent again (%d calls)", calls-before)
	}
}

func TestInitKeepsTheFileWhenTheMergeFails(t *testing.T) {
	old := newInitMerger
	newInitMerger = func(io.Writer) func(string, string, string) (string, error) {
		return func(string, string, string) (string, error) { return "", errors.New("no agent") }
	}
	t.Cleanup(func() { newInitMerger = old })
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "CLAUDE.md"), "mine")
	output := runInitIn(t, root, "-type", "go")
	if readText(t, filepath.Join(root, "CLAUDE.md")) != "mine" || !strings.Contains(output, "chưa gộp: no agent") {
		t.Errorf("a failed merge must keep the file and say why:\n%s", output)
	}
}

func TestInitDryRunAndNoMergeAskNoAgent(t *testing.T) {
	calls := 0
	fakeMerger(t, &calls)
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "CLAUDE.md"), "mine")
	if out := runInitIn(t, root, "-type", "go", "-dry-run"); !strings.Contains(out, "lúc chạy thật") {
		t.Errorf("dry run must say a merge would happen:\n%s", out)
	}
	runInitIn(t, root, "-type", "go", "-no-merge")
	if calls != 0 || readText(t, filepath.Join(root, "CLAUDE.md")) != "mine" {
		t.Errorf("-dry-run and -no-merge must not merge (%d calls)", calls)
	}
}

func TestMergedFileRefusesAnAnswerThatLostTheUsersText(t *testing.T) {
	mine := strings.Repeat("a line the user wrote\n", 20)
	if _, err := mergedFile("<merged>\nshort\n</merged>", mine); err == nil {
		t.Error("a much shorter merge must be refused")
	}
	if _, err := mergedFile("no tags here", mine); err == nil {
		t.Error("an answer without the tags must be refused")
	}
	got, err := mergedFile("Sure:\n<merged>\n"+mine+"new kit line\n</merged>\n", mine)
	if err != nil || !strings.HasSuffix(got, "new kit line\n") || strings.Contains(got, "Sure") {
		t.Errorf("got %q, %v", got, err)
	}
}
