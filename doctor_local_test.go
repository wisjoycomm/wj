package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestDoctorOldExecutablesKeepsNewestAndTrashesRest(t *testing.T) {
	var trashed []string
	fakeTrash(t, &trashed)
	dir := t.TempDir()
	exe := filepath.Join(dir, "wj.exe")
	writeTestFile(t, exe, "current")
	for _, name := range []string{"wj.exe.old-20260101T000000Z", "wj.exe.old-20260201T000000Z", "wj.exe.old-20260301T000000Z"} {
		writeTestFile(t, filepath.Join(dir, name), "old")
	}
	old := selfExecutable
	selfExecutable = func() string { return exe }
	t.Cleanup(func() { selfExecutable = old })

	findings := checkOldExecutables(doctorEnv{})
	if len(findings) != 1 || findings[0].fix == nil {
		t.Fatalf("findings = %+v", findings)
	}
	var out bytes.Buffer
	if err := findings[0].fix(&out); err != nil {
		t.Fatal(err)
	}
	if len(trashed) != 2 || filepath.Base(trashed[0]) != "wj.exe.old-20260101T000000Z" {
		t.Fatalf("trashed = %v", trashed)
	}
	if f := checkOldExecutables(doctorEnv{}); len(f) != 0 {
		t.Fatalf("still reported after the fix: %+v", f)
	}
}

func TestDoctorLocalMemorySetOnKitProject(t *testing.T) {
	root := gitProject(t)
	env := doctorEnv{home: t.TempDir(), cwd: root}
	if f := checkLocalMemory(env); len(f) != 0 {
		t.Fatalf("a project without the kit is left alone: %+v", f)
	}
	writeTestFile(t, filepath.Join(root, "AGENTS.md"), "<!-- wj:begin core -->\nx\n<!-- wj:end core -->\n")
	findings := checkLocalMemory(env)
	if len(findings) != 1 || findings[0].fix == nil {
		t.Fatalf("findings = %+v", findings)
	}
	var out bytes.Buffer
	if err := findings[0].fix(&out); err != nil {
		t.Fatal(err)
	}
	if f := checkLocalMemory(env); len(f) != 0 {
		t.Fatalf("still reported after the fix: %+v", f)
	}
}
