package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitTestRepo(t *testing.T, user string) string {
	t.Helper()
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", user}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return root
}

func TestNextPlanIDCountsOnlyItsPrefix(t *testing.T) {
	plan := goodPlan + "| SON-03 | Prefixed | P1 | 1 | TODO | claude | T-01, SON-02 | - | - | - | - |\n"
	tasks, issues := parsePlan(plan)
	if len(issues) != 0 {
		t.Fatalf("SON-03 with Depends SON-02 must be valid: %v", issues)
	}
	archive := "| SON-07 | old |\n| VIE-20 | theirs |\n"
	if got := nextPlanIDFor("SON", tasks, archive); got != "SON-08" {
		t.Errorf("SON next = %s, want SON-08", got)
	}
	if got := nextPlanIDFor("VIE", tasks, archive); got != "VIE-21" {
		t.Errorf("VIE next = %s, want VIE-21", got)
	}
	if got := nextPlanID(tasks, archive); got != "T-05" {
		t.Errorf("legacy next = %s, want T-05", got)
	}
}

func TestPlanPrefixClaimAndClash(t *testing.T) {
	root := gitTestRepo(t, "alice")
	writeTestFile(t, filepath.Join(root, "PLAN.md"), goodPlan)
	var out bytes.Buffer
	if err := runPlanIn([]string{"prefix", "son"}, &out, root); err != nil {
		t.Fatal(err)
	}
	reg := readText(t, filepath.Join(root, ".claude", "plan-prefixes"))
	if !strings.Contains(reg, "\nSON alice\n") {
		t.Fatalf("registry missing SON alice:\n%s", reg)
	}
	// claiming again is a no-op, not a duplicate line
	if err := runPlanIn([]string{"prefix", "SON"}, &out, root); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(readText(t, filepath.Join(root, ".claude", "plan-prefixes")), "SON "); n != 1 {
		t.Fatalf("SON listed %d times", n)
	}
	out.Reset()
	if err := runPlanIn(nil, &out, root); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "NEXT ID: SON-01") {
		t.Errorf("want NEXT ID SON-01, got:\n%s", out.String())
	}
	exec.Command("git", "-C", root, "config", "user.name", "bob").Run()
	if err := runPlanIn([]string{"prefix", "SON"}, &out, root); err == nil || !strings.Contains(err.Error(), "alice") {
		t.Errorf("bob claiming SON must fail naming alice, got %v", err)
	}
	for _, bad := range []string{"T", "S", "SONNY", "S0N"} {
		if err := runPlanIn([]string{"prefix", bad}, &out, root); err == nil {
			t.Errorf("prefix %q must be refused", bad)
		}
	}
}

func TestPlanPrefixDuplicatesFailCheck(t *testing.T) {
	root := gitTestRepo(t, "alice")
	writeTestFile(t, filepath.Join(root, "PLAN.md"), goodPlan)
	writeTestFile(t, filepath.Join(root, ".claude", "plan-prefixes"), "# c\nSON alice\nVIE bob\nSON carol\n")
	var out bytes.Buffer
	err := runPlanIn([]string{"check"}, &out, root)
	if err == nil || !strings.Contains(out.String(), "trùng với dòng 2") {
		t.Errorf("duplicate SON must fail check, err=%v out:\n%s", err, out.String())
	}
	out.Reset()
	if err := runPlanIn(nil, &out, root); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "PREFIX: máy này chưa có prefix") || !strings.Contains(out.String(), "PREFIX: .claude/plan-prefixes dòng 4") {
		t.Errorf("status must warn about missing prefix and the clash:\n%s", out.String())
	}
}
