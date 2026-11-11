package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const goodPlan = "| ID | Task | Pri | SP | Status | Agent | Depends | Scope | Verify | Notes | Session |\n" +
	"|---|---|---|---|---|---|---|---|---|---|---|\n" +
	"| T-01 | Add JWT check | P1 | 3 | DOING | claude | - | `src/auth.py` | `pytest -q` | - | wt-auth |\n" +
	"| T-02 | Fix DB pool | P0 | 2 | TODO | codex | - | `src/db/` | - | retry 3x | - |\n" +
	"| T-03 | Tax logic | P2 | 5 | TODO | claude | T-01 | `src/billing/tax.py` | - | - | - |\n" +
	"| T-04 | Auth refresh | P1 | 1 | TODO | claude | - | `src/auth.py` | - | - | - |\n"

func TestParsePlanAcceptsStrictTable(t *testing.T) {
	tasks, issues := parsePlan(goodPlan)
	if len(issues) != 0 || len(tasks) != 4 {
		t.Fatalf("issues %v tasks %d", issues, len(tasks))
	}
	if tasks[0].Verify != "pytest -q" || tasks[0].Scope[0] != "src/auth.py" || tasks[2].Depends[0] != "T-01" || tasks[0].Line != 3 {
		t.Fatalf("bad parse %+v", tasks)
	}
	if got := planRow(tasks[0]); !strings.Contains(goodPlan, got) {
		t.Fatalf("round trip changed the row:\n%s", got)
	}
}

func TestParsePlanRejectsLooseFormat(t *testing.T) {
	cases := map[string]string{
		"# PLAN.md\n" + goodPlan:   "tiêu đề",
		goodPlan + "\nsome note\n": "ngoài bảng",
		strings.Replace(goodPlan, "| P1 | 3 | DOING", "| high | 3 | DOING", 1): "Pri",
		strings.Replace(goodPlan, "| P1 | 3 | DOING", "| P1 | 4 | DOING", 1):   "SP",
		strings.Replace(goodPlan, "DOING", "RUNNING", 1):                       "Status",
		strings.Replace(goodPlan, "| T-02 |", "| T-01 |", 1):                   "trùng ID",
		strings.Replace(goodPlan, "retry 3x", "a | b", 1):                      "ô",
		strings.Replace(goodPlan, "| wt-auth |", "|  |", 1):                    "Session",
		strings.Replace(goodPlan, "`src/db/`", "src/db/", 1):                   "Scope",
	}
	for text, want := range cases {
		_, issues := parsePlan(text)
		joined := ""
		for _, i := range issues {
			joined += i.String() + "\n"
		}
		if !strings.Contains(joined, want) {
			t.Errorf("want issue %q, got:\n%s", want, joined)
		}
	}
}

func TestPlanStatusOrdersReadyByPriorityAndHonoursScope(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "PLAN.md"), goodPlan)
	writeTestFile(t, filepath.Join(root, "PLAN-ARCHIVE.md"), "| T-09 | old |\n")
	var out bytes.Buffer
	if err := runPlanIn(nil, &out, root); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"NEXT ID: T-10", "  4    T-02   codex", "T-03 needs T-01(DOING)", "T-04 scope clash with T-01"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}

func TestPlanAgentCanNameAModel(t *testing.T) {
	plan := strings.Replace(goodPlan, "| TODO | codex |", "| TODO | claude:haiku |", 1)
	if _, issues := parsePlan(plan); len(issues) != 0 {
		t.Fatalf("claude:<model> must be a valid Agent: %v", issues)
	}
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "PLAN.md"), plan)
	var out bytes.Buffer
	if err := runPlanIn(nil, &out, root); err != nil {
		t.Fatal(err)
	}
	// The orchestrator reads the model for the worker from the READY line.
	if !strings.Contains(out.String(), "T-02   claude:haiku") {
		t.Errorf("READY must show the task's model:\n%s", out.String())
	}
	for _, bad := range []string{"claude:", "claude:two words", "gpt", "claude:a;rm"} {
		if _, issues := parsePlan(strings.Replace(goodPlan, "| TODO | codex |", "| TODO | "+bad+" |", 1)); len(issues) == 0 {
			t.Errorf("Agent %q must be rejected", bad)
		}
	}
}

func TestPlanLogQueriesTheArchive(t *testing.T) {
	root := t.TempDir()
	row := func(id, task, commit string) string {
		return "| " + id + " | " + task + " | P1 | 3 | DONE | claude | - | `src/a.go` | - | - | wt | " + commit + " | 2026-11-01 |\n"
	}
	writeTestFile(t, filepath.Join(root, "PLAN-ARCHIVE.md"), "old free text T-00\n\n"+archiveHeaderLine+"\n"+archiveSeparatorLine+"\n"+
		row("T-01", "Add JWT check", "abc1234")+row("T-02", "Fix DB pool", "def5678")+row("T-12", "Tax logic", "-"))
	run := func(args ...string) string {
		var out bytes.Buffer
		if err := runPlanIn(append([]string{"log"}, args...), &out, root); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	// No PLAN.md here: the archive is read on its own.
	if text := run("-last", "2"); !strings.Contains(text, "3 rows, last 2") || strings.Contains(text, "T-01") || !strings.Contains(text, "def5678") {
		t.Errorf("-last 2:\n%s", text)
	}
	if text := run("-find", "t-12"); !strings.Contains(text, "matching 't-12': 1") || !strings.Contains(text, "Tax logic") {
		t.Errorf("-find by ID:\n%s", text)
	}
	if text := run("-find", "db pool"); !strings.Contains(text, "T-02") || strings.Contains(text, "T-12") {
		t.Errorf("-find by text:\n%s", text)
	}
}

const oldPlan = `# PLAN.md

Next ID: T-05

<!--
How this file works
- [ ] **T-99**: inside a comment, ignored
-->

## Queue

- [~] **T-03** (user priority; Opus): Snowstorm | schedule server
  - **Agent**: codex
  - **Scope**: ` + "`src/storm/`, `tests/storm/`" + `
  - **Verify**: ` + "`go test ./...`" + `
  - **Depends**: T-01, T-02
  - **Result**: TASK: T-03 | STATUS: PASS

- [x] **T-04**: Mail
  - **Agent**:
  - **Scope**: ` + "`src/mail/`" + `
  - **Verify**:
  - **Depends**: -
  - **Result**:
- 08:00 also: restart the local server

## Backlog

- World density from the LD captures
  - nested detail line
`

func TestConvertOldPlanKeepsEveryTask(t *testing.T) {
	tasks, _ := convertOldPlan(oldPlan)
	rendered := renderPlan(tasks)
	if _, issues := parsePlan(rendered); len(issues) != 0 {
		t.Fatalf("converted plan breaks the format: %v\n%s", issues, rendered)
	}
	if len(tasks) != 4 {
		t.Fatalf("want 4 rows (2 tasks, 1 loose note, 1 backlog), got %d:\n%s", len(tasks), rendered)
	}
	storm := tasks[0]
	if storm.ID != "T-03" || storm.Status != "DOING" || storm.Agent != "codex" || len(storm.Scope) != 2 || storm.Verify != "go test ./..." || len(storm.Depends) != 2 {
		t.Fatalf("bad task %+v", storm)
	}
	if strings.Contains(planRow(storm), "Snowstorm | schedule") {
		t.Fatal("pipe inside a cell must be escaped")
	}
	if tasks[1].Status != "DONE" || tasks[1].Agent != "claude" {
		t.Fatalf("bad task %+v", tasks[1])
	}
	if tasks[2].ID != "T-05" || tasks[3].ID != "T-06" || tasks[3].Pri != "P3" || tasks[2].Pri != "P2" {
		t.Fatalf("loose lines must get new IDs after the highest one: %+v %+v", tasks[2], tasks[3])
	}
}

func TestPlanConvertBacksUpAndRefreshesSkills(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "PLAN.md"), oldPlan)
	writeTestFile(t, filepath.Join(root, ".claude", "skills", "orca-orchestrator", "SKILL.md"), "old skill")
	oldScript := filepath.Join(root, ".claude", "skills", "orca-orchestrator", "scripts", "plan.ps1")
	writeTestFile(t, oldScript, "old script")
	var out bytes.Buffer
	if err := runPlanIn([]string{"convert"}, &out, root); err != nil {
		t.Fatal(err, out.String())
	}
	if !isTablePlan(readText(t, filepath.Join(root, "PLAN.md"))) {
		t.Fatal("PLAN.md not converted")
	}
	if !strings.Contains(readText(t, filepath.Join(root, ".claude", "skills", "orca-orchestrator", "SKILL.md")), "ONE Markdown table") {
		t.Fatal("orchestrator skill not refreshed")
	}
	if _, err := os.Stat(oldScript); err == nil {
		t.Fatal("the script the refreshed skill no longer calls must go")
	}
	backups, _ := filepath.Glob(filepath.Join(root, "PLAN.md.bak-*"))
	if len(backups) != 1 || readText(t, backups[0]) != oldPlan {
		t.Fatal("old PLAN.md not backed up")
	}
	if _, err := os.Stat(filepath.Join(root, ".agents")); err == nil {
		t.Fatal("convert created .agents skills the project never had")
	}
}

func TestKitPlanIsAnEmptyStrictTable(t *testing.T) {
	data, err := kitFS.ReadFile(kitBase + "/PLAN.md")
	if err != nil {
		t.Fatal(err)
	}
	tasks, issues := parsePlan(string(data))
	if len(tasks) != 0 || len(issues) != 0 {
		t.Fatalf("kit PLAN.md: %d tasks, issues %v", len(tasks), issues)
	}
	for _, rel := range orcaSkillFiles {
		text, err := kitFS.ReadFile(kitSource(rel))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(text, []byte(`\*\*Scope\*\*`)) || bytes.Contains(text, []byte("## Backlog")) || bytes.Contains(text, []byte("- [ ]")) {
			t.Errorf("%s still describes the old checkbox format", rel)
		}
	}
}

func TestPlanArchiveKeepsCommit(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init", "-q")
	git("-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "Merge T-01 jwt check")
	plan := strings.Replace(goodPlan, "| DOING | claude | - | `src/auth.py`", "| DONE | claude | - | `src/auth.py`", 1)
	plan = strings.Replace(plan, "| P0 | 2 | TODO |", "| P0 | 2 | PARTIAL |", 1)
	writeTestFile(t, filepath.Join(root, "PLAN.md"), plan)
	writeTestFile(t, filepath.Join(root, "PLAN-ARCHIVE.md"), "# PLAN-ARCHIVE\n\nold free text line T-00\n")

	var out bytes.Buffer
	if err := runPlanIn([]string{"archive", "T-03"}, &out, root); err == nil {
		t.Fatal("a TODO task must not be archived")
	}
	if err := runPlanIn([]string{"archive", "T-01", "T-02=abc1234"}, &out, root); err != nil {
		t.Fatal(err, out.String())
	}
	archive := readText(t, filepath.Join(root, "PLAN-ARCHIVE.md"))
	if !strings.HasPrefix(archive, "# PLAN-ARCHIVE\n\nold free text line T-00\n\n"+archiveHeaderLine) {
		t.Fatalf("old archive text must stay and a table header follow:\n%s", archive)
	}
	lines := strings.Split(strings.TrimSpace(archive), "\n")
	t01, t02 := lines[len(lines)-2], lines[len(lines)-1]
	if !strings.HasPrefix(t01, "| T-01 |") || strings.Contains(t01, "| - | 20") || !strings.HasPrefix(t02, "| T-02 |") || !strings.Contains(t02, "| abc1234 | ") {
		t.Fatalf("rows must carry the merge commit:\n%s\n%s", t01, t02)
	}
	cells, _ := splitPlanRow(t01)
	if len(cells) != len(archiveColumns) || !commitPattern.MatchString(cells[11]) {
		t.Fatalf("T-01 commit must be found in git log: %v", cells)
	}
	left, issues := parsePlan(readText(t, filepath.Join(root, "PLAN.md")))
	if len(issues) != 0 || len(left) != 2 || left[0].ID != "T-03" {
		t.Fatalf("archived rows must leave PLAN.md: %v %+v", issues, left)
	}

	// A second archive appends rows under the same header.
	writeTestFile(t, filepath.Join(root, "PLAN.md"), strings.Replace(readText(t, filepath.Join(root, "PLAN.md")), "| TODO |", "| DONE |", 1))
	if err := runPlanIn([]string{"archive", "T-03", "-commit", "def5678"}, &out, root); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(readText(t, filepath.Join(root, "PLAN-ARCHIVE.md")), archiveHeaderLine); n != 1 {
		t.Fatalf("header written %d times", n)
	}
}

func TestKitArchiveHeaderMatchesArchiveCommand(t *testing.T) {
	data, err := kitFS.ReadFile(kitBase + "/PLAN-ARCHIVE.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != archiveHeaderLine+"\n"+archiveSeparatorLine+"\n" {
		t.Fatalf("kit PLAN-ARCHIVE.md must be the empty archive table:\n%s", data)
	}
	// wj plan archive appends rows under the kit header without writing another one.
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "PLAN-ARCHIVE.md"), string(data))
	writeTestFile(t, filepath.Join(root, "PLAN.md"), strings.Replace(goodPlan, "| DOING |", "| DONE |", 1))
	var out bytes.Buffer
	if err := runPlanIn([]string{"archive", "T-01", "-commit", "abc1234"}, &out, root); err != nil {
		t.Fatal(err)
	}
	archive := readText(t, filepath.Join(root, "PLAN-ARCHIVE.md"))
	if strings.Count(archive, archiveHeaderLine) != 1 || !strings.HasPrefix(strings.Split(archive, "\n")[2], "| T-01 |") {
		t.Fatalf("row must follow the kit header directly:\n%s", archive)
	}
}
