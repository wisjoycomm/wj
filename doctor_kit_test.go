package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func gitProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestDoctorKitFindsWhatInitWouldAdd(t *testing.T) {
	home := t.TempDir()
	root := gitProject(t)
	env := doctorEnv{home: home, cwd: root}
	if f := checkKit(env); len(f) != 0 {
		t.Fatalf("a repo no agent works in is left alone: %+v", f)
	}

	// A project an agent already works in, without the kit: told, not changed.
	writeTestFile(t, filepath.Join(root, "CLAUDE.md"), "my notes\n")
	findings := checkKit(env)
	if len(findings) != 1 || findings[0].fix != nil || !strings.Contains(findings[0].Title, "chưa cài kit") {
		t.Fatalf("findings = %+v", findings)
	}

	// A current kit: nothing to say.
	if err := os.Remove(filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
	runInitIn(t, root, "-type", "go")
	if f := checkKit(env); len(f) != 0 {
		t.Fatalf("fresh kit reported: %s", f[0].Detail)
	}

	// The guard hook and a deny rule go missing: found, and the fix puts them back.
	path := filepath.Join(root, ".claude", "settings.json")
	writeTestFile(t, path, `{"permissions":{"allow":["Bash(mine)"]}}`)
	findings = checkKit(env)
	if len(findings) != 1 || findings[0].fix == nil || !strings.Contains(findings[0].Detail, ".claude/settings.json") {
		t.Fatalf("findings = %+v", findings)
	}
	var out bytes.Buffer
	if err := findings[0].fix(&out); err != nil {
		t.Fatal(err)
	}
	settings := readText(t, path)
	for _, want := range []string{"wj guard", "Read(**/go.sum)", "Bash(mine)"} {
		if !strings.Contains(settings, want) {
			t.Errorf("after the fix settings lack %q:\n%s", want, settings)
		}
	}
	if f := checkKit(env); len(f) != 0 {
		t.Fatalf("still reported after the fix: %s", f[0].Detail)
	}
}

func TestDoctorKitKeepsALiteProjectLite(t *testing.T) {
	root := gitProject(t)
	runInitIn(t, root, "-lite", "-type", "web")
	if f := checkKit(doctorEnv{home: t.TempDir(), cwd: root}); len(f) != 0 {
		t.Fatalf("a lite kit must not be told to add the wj layer: %s", f[0].Detail)
	}
}

func TestDoctorMeasuresFixedContext(t *testing.T) {
	home := t.TempDir()
	root := gitProject(t)
	writeTestFile(t, filepath.Join(root, "CLAUDE.md"), "@AGENTS.md\n\n"+strings.Repeat("x", 400))
	writeTestFile(t, filepath.Join(root, "AGENTS.md"), strings.Repeat("y", 4000))
	writeTestFile(t, filepath.Join(root, "NOT-LOADED.md"), strings.Repeat("z", 40000))
	writeTestFile(t, filepath.Join(home, ".claude", "CLAUDE.md"), strings.Repeat("u", 800))
	writeTestFile(t, filepath.Join(root, ".claude", "memory", "MEMORY.md"), strings.Repeat("m", 400))
	writeTestFile(t, filepath.Join(root, ".claude", "skills", "deploy", "SKILL.md"),
		"---\nname: deploy\ndescription: >\n  "+strings.Repeat("d", 396)+"\n---\nbody is not counted "+strings.Repeat("b", 9000))
	writeTestFile(t, filepath.Join(root, ".claude", "skills", "manual", "SKILL.md"),
		"---\nname: manual\ndescription: "+strings.Repeat("h", 4000)+"\ndisable-model-invocation: true\n---\n")
	writeTestFile(t, filepath.Join(root, ".claude", "agents", "worker.md"), "---\nname: worker\ndescription: "+strings.Repeat("w", 194)+"\n---\n")

	ctx := readFixedContext(root, home)
	if got := sumTokens(ctx.rules); got != 103+1000+200+100 {
		t.Errorf("rules = %d tokens: %+v", got, ctx.rules)
	}
	if got := sumTokens(ctx.list); got != 100+50 {
		t.Errorf("list = %d tokens: %+v", got, ctx.list)
	}
	env := doctorEnv{home: home, cwd: root}
	if f := checkFixedContext(env); len(f) != 0 {
		t.Fatalf("under budget but reported: %+v", f)
	}
	var out bytes.Buffer
	printFixedContext(&out, env)
	if !strings.Contains(out.String(), "~1.6k token mỗi phiên") {
		t.Errorf("summary: %s", out.String())
	}

	writeTestFile(t, filepath.Join(root, "AGENTS.md"), strings.Repeat("y", 16000))
	findings := checkFixedContext(env)
	if len(findings) != 1 || findings[0].Key != "context:rules" || !strings.Contains(findings[0].Detail, "~4.0k   AGENTS.md") {
		t.Fatalf("findings = %+v", findings)
	}
}

func TestDoctorCleansOldScratchFilesOnly(t *testing.T) {
	root := gitProject(t)
	var trashed []string
	fakeTrash(t, &trashed)
	old := time.Now().Add(-10 * 24 * time.Hour)
	for _, rel := range []string{".tmp/t-01/build.log", ".tmp/t-01/deep/out.json", ".temp/dump.txt"} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		writeTestFile(t, path, strings.Repeat("x", 2048))
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, filepath.Join(root, ".tmp", "handover.md"), "today's notes")
	writeTestFile(t, filepath.Join(root, "src", "keep.go"), "package src")

	findings := checkScratch(doctorEnv{home: t.TempDir(), cwd: root})
	if len(findings) != 2 {
		t.Fatalf("want one finding per scratch folder, got %+v", findings)
	}
	if f := findings[0]; !strings.Contains(f.Title, ".tmp: 2 file, 4 KB") || !strings.Contains(f.Detail, "Giữ lại 1 file") {
		t.Errorf("finding: %s\n%s", f.Title, f.Detail)
	}
	if !strings.Contains(findings[1].Detail, ".temp/ là do agent tự đặt") {
		t.Errorf("finding: %s", findings[1].Detail)
	}
	var out bytes.Buffer
	for _, f := range findings {
		if err := f.fix(&out); err != nil {
			t.Fatal(err)
		}
	}
	if len(trashed) != 3 {
		t.Errorf("old files must go to the trash, got %v", trashed)
	}
	for _, gone := range []string{".tmp/t-01", ".temp/dump.txt"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(gone))); err == nil {
			t.Errorf("%s must be removed", gone)
		}
	}
	for _, kept := range []string{".tmp/handover.md", "src/keep.go", ".tmp"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(kept))); err != nil {
			t.Errorf("%s must stay", kept)
		}
	}
	if f := checkScratch(doctorEnv{home: t.TempDir(), cwd: root}); len(f) != 0 {
		t.Errorf("nothing old is left, but: %+v", f)
	}
}
