package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runInitIn(t *testing.T, root string, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := runInit(append(args, root), strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestEveryStackHasRulesAndGitignore(t *testing.T) {
	for _, stack := range initStacksList {
		for _, file := range []string{"rules.md", "gitignore"} {
			if _, err := kitFS.ReadFile(kitStacks + "/" + stack.id + "/" + file); err != nil {
				t.Errorf("%s: %v", stack.id, err)
			}
		}
	}
}

func TestInitFreshProjectInstallsKitAndChosenRules(t *testing.T) {
	root := t.TempDir()
	runInitIn(t, root, "-type", "unity")
	for _, rel := range []string{
		"CLAUDE.md", "PLAN.md", "PLAN-ARCHIVE.md", ".claude/settings.json",
		".claude/skills/orca-orchestrator/SKILL.md",
		".claude/skills/orca-dream/SKILL.md",
		".claude/agents/orca-worker.md",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s missing", rel)
		}
	}
	for _, rel := range []string{".agents", "dot-claude", ".claude/skills/orca-orchestrator/scripts", ".claude/skills/orca-stateless-worker"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			t.Errorf("%s must not be installed", rel)
		}
	}
	settings := readText(t, filepath.Join(root, ".claude", "settings.json"))
	for _, want := range []string{"Bash(wj verify *)", "Agent(orca-worker)", "Read(/PLAN-ARCHIVE.md)", "Read(/Library/**)",
		"CLAUDE_CODE_GLOB_NO_IGNORE", `"baseRef": "head"`, "BASH_MAX_OUTPUT_LENGTH", "&& wj guard || true"} {
		if !strings.Contains(settings, want) {
			t.Errorf("settings.json lacks %q", want)
		}
	}
	agents := readText(t, filepath.Join(root, "AGENTS.md"))
	for _, want := range []string{"## Read less", "## Unity", ".meta"} {
		if !strings.Contains(agents, want) {
			t.Errorf("AGENTS.md lacks %q", want)
		}
	}
	if strings.Contains(agents, "## Web") {
		t.Error("web rules were not chosen")
	}
	ignore := readText(t, filepath.Join(root, ".gitignore"))
	if !strings.Contains(ignore, ".tmp/") || !strings.Contains(ignore, "/[Ll]ibrary/") || !strings.Contains(ignore, ".claude/worktrees/") {
		t.Errorf("gitignore = %q", ignore)
	}
}

func TestInitUnityInstallsSharedCheckoutWorker(t *testing.T) {
	unity := t.TempDir()
	runInitIn(t, unity, "-type", "unity,web")
	workerPath := filepath.Join(unity, ".claude", "agents", "orca-worker.md")
	worker := readText(t, workerPath)
	if strings.Contains(worker, "isolation:") || strings.Contains(worker, "\ntools:") || !strings.Contains(worker, "Unity MCP") {
		t.Errorf("unity worker should be the shared-checkout one:\n%s", worker)
	}
	if out := runInitIn(t, unity, "-type", "unity", "-dry-run"); strings.Contains(out, "khác bản kit") {
		t.Errorf("rerun should see the worker as up to date:\n%s", out)
	}
	runInitIn(t, unity, "-remove")
	if _, err := os.Stat(workerPath); err == nil {
		t.Error("-remove should take out the unity worker")
	}

	other := t.TempDir()
	runInitIn(t, other, "-type", "go")
	if !strings.Contains(readText(t, filepath.Join(other, ".claude", "agents", "orca-worker.md")), "isolation: worktree") {
		t.Error("non-unity project should keep the worktree worker")
	}
}

func TestInitKitIsGeneralWithoutChosenStack(t *testing.T) {
	root := t.TempDir()
	runInitIn(t, root)
	agents := readText(t, filepath.Join(root, "AGENTS.md"))
	settings := readText(t, filepath.Join(root, ".claude", "settings.json"))
	plan := readText(t, filepath.Join(root, "PLAN.md"))
	for _, unwanted := range []string{"Unity", "*.prefab", ".next"} {
		if strings.Contains(agents, unwanted) {
			t.Errorf("base AGENTS.md mentions %q", unwanted)
		}
	}
	if strings.Contains(settings, "npm") || strings.Contains(settings, "go test") {
		t.Errorf("base settings has stack rules:\n%s", settings)
	}
	if strings.Contains(plan, "**T-") {
		t.Errorf("PLAN.md has sample tasks:\n%s", plan)
	}
}

func TestInitDetectsStackWhenNoAnswer(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "ProjectSettings", "ProjectVersion.txt"), "m_EditorVersion: 6000.0")
	writeTestFile(t, filepath.Join(root, "package.json"), "{}")
	runInitIn(t, root)
	agents := readText(t, filepath.Join(root, "AGENTS.md"))
	if !strings.Contains(agents, "## Unity") || !strings.Contains(agents, "## Web") {
		t.Errorf("detected stacks missing:\n%s", agents)
	}
}

func TestInitNeverOverwritesUserFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "CLAUDE.md"), "mine")
	writeTestFile(t, filepath.Join(root, "PLAN.md"), "my plan")
	writeTestFile(t, filepath.Join(root, "AGENTS.md"), "my rules\n")
	writeTestFile(t, filepath.Join(root, ".gitignore"), ".tmp/\nkeep-me\n")
	writeTestFile(t, filepath.Join(root, ".claude", "skills", "orca-orchestrator", "SKILL.md"), "edited")
	writeTestFile(t, filepath.Join(root, ".claude", "settings.json"),
		`{"theme":"dark","permissions":{"deny":["Bash(npm install)"],"allow":["Bash(mine)"]}}`)
	output := runInitIn(t, root, "-type", "web")

	if readText(t, filepath.Join(root, "CLAUDE.md")) != "mine" || readText(t, filepath.Join(root, "PLAN.md")) != "my plan" {
		t.Error("user file overwritten")
	}
	if readText(t, filepath.Join(root, ".claude", "skills", "orca-orchestrator", "SKILL.md")) != "edited" {
		t.Error("edited skill overwritten")
	}
	if !strings.Contains(output, "giữ nguyên (khác bản kit)") {
		t.Errorf("difference not reported:\n%s", output)
	}
	agents := readText(t, filepath.Join(root, "AGENTS.md"))
	if !strings.HasPrefix(agents, "my rules\n") || !strings.Contains(agents, "## Web") {
		t.Errorf("AGENTS.md = %q", agents)
	}
	if strings.Contains(agents, "## Read less") {
		t.Error("base rules forced into a user's AGENTS.md")
	}
	ignore := readText(t, filepath.Join(root, ".gitignore"))
	if !strings.HasPrefix(ignore, ".tmp/\nkeep-me\n") || strings.Count(ignore, ".tmp/") != 1 {
		t.Errorf("gitignore = %q", ignore)
	}

	var doc struct {
		Theme       string
		Permissions struct{ Allow, Deny []string }
	}
	if err := json.Unmarshal([]byte(readText(t, filepath.Join(root, ".claude", "settings.json"))), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Theme != "dark" || !containsName(doc.Permissions.Allow, "Bash(mine)") || !containsName(doc.Permissions.Allow, "Bash(git status)") {
		t.Errorf("settings = %+v", doc)
	}
	if containsName(doc.Permissions.Allow, "Bash(npm install)") {
		t.Error("user's deny rule was loosened")
	}
}

// -lite installs only what works on a machine without wj. A later full run
// adds the rest.
func TestInitLiteNeedsNoWj(t *testing.T) {
	root := t.TempDir()
	output := runInitIn(t, root, "-lite", "-type", "web")
	for _, rel := range []string{"PLAN.md", "PLAN-ARCHIVE.md", ".claude/skills", ".claude/agents"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			t.Errorf("%s must not be in the lite kit", rel)
		}
	}
	for _, rel := range []string{"AGENTS.md", "CLAUDE.md", ".claude/settings.json"} {
		text := readText(t, filepath.Join(root, filepath.FromSlash(rel)))
		for _, unwanted := range []string{"wj", "orca", "PLAN"} {
			if strings.Contains(text, unwanted) && !(rel == "AGENTS.md" && unwanted == "wj") {
				t.Errorf("%s mentions %q:\n%s", rel, unwanted, text)
			}
		}
	}
	// In AGENTS.md "wj" is only the block markers.
	agents := readText(t, filepath.Join(root, "AGENTS.md"))
	if strings.Contains(strings.ReplaceAll(strings.ReplaceAll(agents, "<!-- wj:begin", ""), "<!-- wj:end", ""), "wj") {
		t.Errorf("lite AGENTS.md calls wj:\n%s", agents)
	}
	settings := readText(t, filepath.Join(root, ".claude", "settings.json"))
	for _, want := range []string{"Read(**/node_modules/**)", "BASH_MAX_OUTPUT_LENGTH", "Bash(git status)", "## Keep output small", "## Web"} {
		if !strings.Contains(settings+agents, want) {
			t.Errorf("lite kit lacks %q", want)
		}
	}
	if strings.Contains(settings, "hooks") || !strings.Contains(output, "Bản lite") {
		t.Errorf("settings:\n%s\noutput:\n%s", settings, output)
	}

	runInitIn(t, root, "-type", "web")
	settings = readText(t, filepath.Join(root, ".claude", "settings.json"))
	agents = readText(t, filepath.Join(root, "AGENTS.md"))
	if !strings.Contains(settings, "wj guard") || !strings.Contains(settings, "Agent(orca-worker)") || !strings.Contains(agents, "wj plan") {
		t.Error("a full run after a lite one must add the Orca layer")
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "agents", "orca-worker.md")); err != nil {
		t.Error("worker subagent missing after the full run")
	}
}

// A project that already has hooks gets the guard added next to them, once.
func TestInitAddsGuardHookNextToUserHooks(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".claude", "settings.json")
	writeTestFile(t, path, `{"hooks":{
		"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"my-check"}]}],
		"PreCompact":[{"hooks":[{"type":"command","command":"my-notes"}]}]}}`)
	runInitIn(t, root)
	output := runInitIn(t, root)
	settings := readText(t, path)
	for _, want := range []string{"my-check", "my-notes"} {
		if !strings.Contains(settings, want) {
			t.Errorf("user hook %q lost:\n%s", want, settings)
		}
	}
	if strings.Count(settings, "wj guard") != 1 {
		t.Errorf("guard hook must be there once:\n%s", settings)
	}
	if !strings.Contains(output, "đã đúng") {
		t.Errorf("second run changed settings:\n%s", output)
	}
}

// The kit's skills must work with nothing but wj: no script files, no pwsh,
// and no search tools the built-in ones replace.
func TestKitSkillsCallOnlyWj(t *testing.T) {
	for _, rel := range orcaSkillFiles {
		data, err := kitFS.ReadFile(kitSource(rel))
		if err != nil {
			t.Fatal(err)
		}
		for _, unwanted := range []string{"pwsh", ".ps1", "scripts/", ".agents", "rg -n", "bat -p"} {
			if strings.Contains(string(data), unwanted) {
				t.Errorf("%s mentions %q", rel, unwanted)
			}
		}
	}
	for _, rel := range []string{"AGENTS.md", "CLAUDE.md"} {
		data, err := kitFS.ReadFile(kitBase + "/" + rel)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "## Shell tools") || strings.Contains(string(data), "shell table") {
			t.Errorf("%s still carries the shell tools table", rel)
		}
	}
}

func TestInitUpdateReplacesOldSkillsAndScripts(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, ".claude", "skills", "orca-orchestrator", "SKILL.md")
	oldWorker := filepath.Join(root, ".claude", "skills", "orca-stateless-worker")
	script := filepath.Join(oldWorker, "scripts", "fast_verify.ps1")
	writeTestFile(t, skill, "old skill printing launch commands")
	writeTestFile(t, filepath.Join(oldWorker, "SKILL.md"), "old worker skill calling fast_verify.ps1")
	writeTestFile(t, script, "old script")
	writeTestFile(t, filepath.Join(root, "CLAUDE.md"), "mine")
	writeTestFile(t, filepath.Join(root, ".claude", "settings.json"), `{"env":{"MINE":"1","CLAUDE_CODE_GLOB_NO_IGNORE":"true"}}`)

	output := runInitIn(t, root)
	if readText(t, skill) != "old skill printing launch commands" || !strings.Contains(output, "wj init -update") {
		t.Fatalf("a plain rerun must keep the old skill and say how to update:\n%s", output)
	}
	if _, err := os.Stat(script); err != nil {
		t.Fatal("a plain rerun must keep what the old skills call")
	}

	runInitIn(t, root, "-update")
	if !strings.Contains(readText(t, skill), "orca-worker") {
		t.Error("skill not replaced by the kit's")
	}
	if backups, _ := filepath.Glob(skill + ".bak-*"); len(backups) != 1 {
		t.Errorf("want one backup of the old skill, got %d", len(backups))
	}
	if _, err := os.Stat(oldWorker); err == nil {
		t.Error("the old worker skill, its script and their folders must be removed")
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "agents", "orca-worker.md")); err != nil {
		t.Error("the worker subagent must be installed")
	}
	if readText(t, filepath.Join(root, "CLAUDE.md")) != "mine" {
		t.Error("-update must not touch CLAUDE.md")
	}
	settings := readText(t, filepath.Join(root, ".claude", "settings.json"))
	if !strings.Contains(settings, `"MINE": "1"`) || !strings.Contains(settings, `"CLAUDE_CODE_GLOB_NO_IGNORE": "true"`) {
		t.Errorf("the user's env values must stay:\n%s", settings)
	}
}

func TestInitKeepsClaudeMemoryInsideTheProject(t *testing.T) {
	root := t.TempDir()
	runInitIn(t, root)
	local := filepath.Join(root, ".claude", "settings.local.json")
	var doc map[string]any
	if err := json.Unmarshal([]byte(readText(t, local)), &doc); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, ".claude", "memory"); doc["autoMemoryDirectory"] != want {
		t.Errorf("autoMemoryDirectory = %v, want %s", doc["autoMemoryDirectory"], want)
	}
	ignore := readText(t, filepath.Join(root, ".gitignore"))
	for _, want := range []string{".claude/memory/", ".claude/settings.local.json", "CLAUDE.local.md"} {
		if !strings.Contains(ignore, want) {
			t.Errorf("%s must be gitignored: memory is local", want)
		}
	}

	// A worktree points at the main checkout's folder, so the repo has one memory.
	worktree := filepath.Join(t.TempDir(), "wt")
	writeTestFile(t, filepath.Join(worktree, ".git"), "gitdir: "+filepath.ToSlash(root)+"/.git/worktrees/wt\n")
	runInitIn(t, worktree)
	if err := json.Unmarshal([]byte(readText(t, filepath.Join(worktree, ".claude", "settings.local.json"))), &doc); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, ".claude", "memory"); doc["autoMemoryDirectory"] != want {
		t.Errorf("worktree autoMemoryDirectory = %v, want the main checkout's %s", doc["autoMemoryDirectory"], want)
	}

	// The user's own choice and other local settings stay.
	writeTestFile(t, local, `{"autoMemoryDirectory":"~/elsewhere","env":{"A":"1"}}`)
	runInitIn(t, root)
	if got := readText(t, local); got != `{"autoMemoryDirectory":"~/elsewhere","env":{"A":"1"}}` {
		t.Errorf("settings.local.json was rewritten: %s", got)
	}
}

func TestInitRerunKeepsUserTextAndChangesNothing(t *testing.T) {
	root := t.TempDir()
	runInitIn(t, root, "-type", "go")
	path := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(path, []byte(readText(t, path)+"\n## My team\n- keep this\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := readText(t, path)
	settingsBefore := readText(t, filepath.Join(root, ".claude", "settings.json"))
	runInitIn(t, root, "-type", "go")
	if got := readText(t, path); got != before {
		t.Errorf("rerun changed AGENTS.md:\n%s", got)
	}
	if got := readText(t, filepath.Join(root, ".claude", "settings.json")); got != settingsBefore {
		t.Error("rerun changed settings.json")
	}

	runInitIn(t, root, "-type", "web")
	got := readText(t, path)
	if !strings.Contains(got, "## My team") || !strings.Contains(got, "## Go") || !strings.Contains(got, "## Web") {
		t.Errorf("adding a stack lost text:\n%s", got)
	}
	if strings.Count(got, "## Read less") != 1 {
		t.Error("base block duplicated")
	}
}

func TestInitRefreshesOldBlockInPlace(t *testing.T) {
	text := "top\n" + blockBegin("go") + "\nold\n" + blockEnd("go") + "\nbottom\n"
	got := upsertBlock(text, "go", "new")
	want := "top\n" + blockBegin("go") + "\nnew\n" + blockEnd("go") + "\nbottom\n"
	if got != want {
		t.Errorf("got %q", got)
	}
}

func TestInitDryRunWritesNothing(t *testing.T) {
	root := t.TempDir()
	output := runInitIn(t, root, "-dry-run", "-type", "unity")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("dry run wrote %d entries", len(entries))
	}
	if !strings.Contains(output, "AGENTS.md") || !strings.Contains(output, "tạo") {
		t.Errorf("output:\n%s", output)
	}
}

func TestInitBrokenSettingsStopsBeforeWriting(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".claude", "settings.json")
	writeTestFile(t, path, "{ broken")
	var out bytes.Buffer
	if err := runInit([]string{"-type", "go", root}, strings.NewReader(""), &out); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Error("files were written before the error")
	}
	if readText(t, path) != "{ broken" {
		t.Error("broken file was changed")
	}
}

func TestInitUsesGitRootFromSubfolder(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "src", "deep")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	runInitIn(t, sub)
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Error("AGENTS.md should be at the git root")
	}
	if _, err := os.Stat(filepath.Join(sub, "AGENTS.md")); err == nil {
		t.Error("AGENTS.md should not be in the subfolder")
	}
}

func TestAddToWjBlock(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "# wj init\na\n"},
		{"x", "x\n\n# wj init\na\n"},
		{"x\n\n# wj init\n.tmp/\n\n# mine\ny\n", "x\n\n# wj init\n.tmp/\na\n\n# mine\ny\n"},
		{"x\r\n\r\n# wj init\r\n.tmp/\r\n", "x\r\n\r\n# wj init\r\n.tmp/\r\na\r\n"},
	}
	for _, c := range cases {
		if got := addToWjBlock(c.in, []string{"a"}); got != c.want {
			t.Errorf("addToWjBlock(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestInitWritesKitVersion(t *testing.T) {
	old := buildVersion
	buildVersion = "0.2.3\n"
	t.Cleanup(func() { buildVersion = old })
	root := t.TempDir()
	runInitIn(t, root, "-type", "go")
	path := filepath.Join(root, ".claude", "wj-kit")
	if got := readText(t, path); got != "0.2.3\n" {
		t.Fatalf("wj-kit = %q", got)
	}
	buildVersion = "0.2.4"
	if out := runInitIn(t, root, "-type", "go"); !strings.Contains(out, "wj init -update") || readText(t, path) != "0.2.3\n" {
		t.Errorf("version changed without -update:\n%s", out)
	}
	runInitIn(t, root, "-type", "go", "-update")
	if got := readText(t, path); got != "0.2.4\n" {
		t.Errorf("wj-kit after -update = %q", got)
	}
}
