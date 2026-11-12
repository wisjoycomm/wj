//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSystemTrashKeepsTheFileRestorable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	work := filepath.Join(home, "work")
	first := filepath.Join(work, "a", "note.md")
	second := filepath.Join(work, "b", "note.md")
	writeTestFile(t, first, "one")
	writeTestFile(t, second, "two")
	if err := systemTrash([]string{first, second}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{first, second} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("%s is still there", path)
		}
	}
	files := filepath.Join(home, ".Trash")
	if runtime.GOOS != "darwin" {
		files = filepath.Join(home, ".local", "share", "Trash", "files")
		info := readText(t, filepath.Join(home, ".local", "share", "Trash", "info", "note.2.md.trashinfo"))
		if !strings.Contains(info, "Path="+filepath.ToSlash(second)) || !strings.Contains(info, "DeletionDate=") {
			t.Errorf("trashinfo = %q", info)
		}
	}
	// The same name twice: both kept.
	if readText(t, filepath.Join(files, "note.md")) != "one" || readText(t, filepath.Join(files, "note.2.md")) != "two" {
		t.Error("trashed files must keep their content under distinct names")
	}
}
