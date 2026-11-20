package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGroupFilesSplitsTheHeaviestFoldersFirst(t *testing.T) {
	files := []string{"README.md", "Assets/Scripts/Battle/Unit.cs", "Assets/Scripts/Battle/Hero.cs", "Assets/Scripts/UI/Menu.cs"}
	for i := 0; i < 30; i++ {
		files = append(files, "Assets/Art/Icons/icon"+string(rune('a'+i%26))+string(rune('a'+i/26))+".png")
	}
	rows := groupFiles(t.TempDir(), files, 6)
	for _, want := range []string{".", "Assets/Scripts/Battle", "Assets/Scripts/UI"} {
		if rows[want] == nil {
			t.Errorf("no row for %s: %v", want, keysOf(rows))
		}
	}
	total := 0
	for _, f := range rows {
		total += f.files
	}
	if total != len(files) || len(rows) > 6 {
		t.Errorf("%d rows hold %d files, want %d files in at most 6 rows: %v", len(rows), total, len(files), keysOf(rows))
	}
	if rows["Assets/Scripts/Battle"].kinds["cs"] != 2 {
		t.Errorf("kinds = %v", rows["Assets/Scripts/Battle"].kinds)
	}
}

func keysOf(m map[string]*mapFolder) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestWriteProjectMapKeepsNotes(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	writeTestFile(t, filepath.Join(root, "Assets", "Scripts", "Unit.cs"), "class Unit {}")
	writeTestFile(t, filepath.Join(root, "Assets", "Scripts", "Unit.cs.meta"), "guid: 1")
	writeTestFile(t, filepath.Join(root, "Assets", "Prefabs", "Hero.prefab"), "yaml")
	now := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	if _, _, err := writeProjectMap(root, now); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".claude", "map.md")
	text := readText(t, path)
	if !strings.Contains(text, "| `Assets/Scripts` | 1 | 1 cs | - | `Unit.cs` | - |") || strings.Contains(text, ".meta") {
		t.Fatalf("map:\n%s", text)
	}
	text = strings.Replace(text, "| `Unit.cs` | - |", "| `Unit.cs` | Combat units |", 1)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "Assets", "Audio", "hit.wav"), "x")
	_, added, err := writeProjectMap(root, now)
	if err != nil {
		t.Fatal(err)
	}
	if again := readText(t, path); !strings.Contains(again, "Combat units") || added != 1 {
		t.Errorf("the Note must survive a rewrite and the new folder must count (added %d):\n%s", added, again)
	}
}

func TestChangesSinceShowsOnlyNewerVersions(t *testing.T) {
	text := "# Changelog\n\n## 0.2.11\n- new\n\n## 0.2.10\n- old\n\n## 0.2.9\n- older\n"
	got := changesSince(text, "0.2.9", "0.2.11")
	if !strings.Contains(got, "- new") || !strings.Contains(got, "- old") || strings.Contains(got, "older") {
		t.Errorf("got %q", got)
	}
	if changesSince(text, "0.2.10", "0.2.10") != "" || changesSince(text, "", "0.2.11") != "" {
		t.Error("nothing is newer, or the project has no version: no entries")
	}
	if !strings.HasPrefix(changelogText, "# Changelog") {
		t.Error("CHANGELOG.md is not embedded")
	}
}
