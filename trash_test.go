package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeTrash stands in for the real trash, so tests never fill the machine's own.
func fakeTrash(t *testing.T, moved *[]string) {
	t.Helper()
	old := moveToTrash
	moveToTrash = func(paths []string) error {
		*moved = append(*moved, paths...)
		for _, path := range paths {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
		}
		return nil
	}
	t.Cleanup(func() { moveToTrash = old })
}

func TestTrashMovesOnlyWhenEveryPathExists(t *testing.T) {
	var moved []string
	fakeTrash(t, &moved)
	dir := t.TempDir()
	file := filepath.Join(dir, "a.log")
	folder := filepath.Join(dir, "t-01")
	writeTestFile(t, file, "x")
	writeTestFile(t, filepath.Join(folder, "b.log"), "x")

	var out bytes.Buffer
	if err := runTrash([]string{file, filepath.Join(dir, "typo.log")}, &out); err == nil || len(moved) != 0 {
		t.Fatalf("a missing path must move nothing: err=%v moved=%v", err, moved)
	}
	if err := runTrash([]string{file, folder}, &out); err != nil {
		t.Fatal(err)
	}
	if len(moved) != 2 || strings.Count(out.String(), "đã chuyển vào thùng rác") != 2 {
		t.Errorf("moved=%v out=%s", moved, out.String())
	}
	out.Reset()
	if err := runTrash(nil, &out); err != nil || !strings.Contains(out.String(), "wj trash") {
		t.Errorf("no arguments must print the help: %v %s", err, out.String())
	}
}
