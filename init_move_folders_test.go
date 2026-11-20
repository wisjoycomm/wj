package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Tests never start a real agent: by default nothing is moved.
func init() {
	newFolderMover = func(io.Writer) func(string, []string) (string, error) { return nil }
}

func fakeFolderMover(t *testing.T, got *[]string, err error) {
	old := newFolderMover
	newFolderMover = func(io.Writer) func(string, []string) (string, error) {
		return func(_ string, folders []string) (string, error) {
			*got = append(*got, folders...)
			return "scripts/a.sh -> .tools/a/a.sh", err
		}
	}
	t.Cleanup(func() { newFolderMover = old })
}

// oldKitProject is a git project on kit 0.2.11 with scripts/ and docs/.
func oldKitProject(t *testing.T) string {
	root := t.TempDir()
	for _, dir := range []string{".git", "scripts", "docs"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, filepath.Join(root, ".claude", "wj-kit"), "0.2.11\n")
	return root
}

func TestOldKitFoldersOnlyForOlderKitsInGit(t *testing.T) {
	root := oldKitProject(t)
	if got := oldKitFolders(root, "0.2.11"); !reflect.DeepEqual(got, []string{"scripts/", "docs/"}) {
		t.Errorf("0.2.11: %v", got)
	}
	for _, kit := range []string{"", "0.2.13", "0.3.0"} {
		if got := oldKitFolders(root, kit); got != nil {
			t.Errorf("kit %q must move nothing, got %v", kit, got)
		}
	}
	if err := os.RemoveAll(filepath.Join(root, ".git")); err != nil {
		t.Fatal(err)
	}
	if got := oldKitFolders(root, "0.2.11"); got != nil {
		t.Errorf("no git, no git mv: got %v", got)
	}
}

func TestInitAsksTheAgentToMoveOldFolders(t *testing.T) {
	old := buildVersion
	buildVersion = "0.2.13"
	t.Cleanup(func() { buildVersion = old })
	var asked []string
	fakeFolderMover(t, &asked, nil)
	root := oldKitProject(t)

	output := runInitIn(t, root, "-type", "go")
	if !reflect.DeepEqual(asked, []string{"scripts/", "docs/"}) || !strings.Contains(output, ".tools/a/a.sh") {
		t.Fatalf("asked %v, output:\n%s", asked, output)
	}
	if got := readText(t, filepath.Join(root, ".claude", "wj-kit")); got != "0.2.13\n" {
		t.Errorf("wj-kit = %q", got)
	}
	asked = nil
	runInitIn(t, root, "-type", "go")
	if asked != nil {
		t.Errorf("a project on the new kit was asked again: %v", asked)
	}
}

func TestInitPutsNoSettingsLocalAtTheRootAndRemoves011Copy(t *testing.T) {
	root := t.TempDir()
	runInitIn(t, root, "-type", "go")
	stray := filepath.Join(root, "settings.local.json")
	if _, err := os.Stat(stray); err == nil {
		t.Fatal("settings.local.json copied to the project root")
	}
	kit, err := kitFS.ReadFile(kitBase + "/settings.local.json")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, stray, string(kit))
	runInitIn(t, root, "-type", "go")
	if _, err := os.Stat(stray); err == nil {
		t.Error("the 0.2.11 copy at the root must be removed")
	}
	writeTestFile(t, stray, "{\"mine\": true}\n")
	runInitIn(t, root, "-type", "go")
	if _, err := os.Stat(stray); err != nil {
		t.Error("a root settings.local.json the user wrote must stay")
	}
}

func TestInitKeepsTheOldKitVersionWhenTheMoveFails(t *testing.T) {
	old := buildVersion
	buildVersion = "0.2.13"
	t.Cleanup(func() { buildVersion = old })
	var asked []string
	fakeFolderMover(t, &asked, errors.New("timeout"))
	root := oldKitProject(t)

	output := runInitIn(t, root, "-type", "go")
	if !strings.Contains(output, "thử lại") {
		t.Errorf("output must say the next run retries:\n%s", output)
	}
	if got := readText(t, filepath.Join(root, ".claude", "wj-kit")); got != "0.2.11\n" {
		t.Errorf("a failed move must keep the old kit version, so the next run retries: wj-kit = %q", got)
	}
}
