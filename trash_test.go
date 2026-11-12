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

func TestUnknownCommandSuggestsTheNearestOne(t *testing.T) {
	var out bytes.Buffer
	for word, want := range map[string]string{"doctr": "wj doctor", "usgae": "wj usage", "plna": "wj plan", "xyzzy123": "wj help"} {
		err := runApp([]string{word}, strings.NewReader(""), &out)
		if err == nil || !strings.Contains(err.Error(), "không có lệnh") || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", word, err)
		}
	}
	if editDistance("doctor", "doctor") != 0 || editDistance("", "abc") != 3 || editDistance("kitten", "sitting") != 3 {
		t.Error("editDistance is wrong")
	}
}

func TestUpdateKeepsOnlyTheNewestBackup(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "wj.exe")
	for _, name := range []string{"wj.exe", "wj.exe.old-20261105T121155.1", "wj.exe.old-20261110T162612.2", "wj.exe.old-20261110T213238", "other.exe.old-20261101T000000"} {
		writeTestFile(t, filepath.Join(dir, name), name)
	}
	removeOldExecutables(dest)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var left []string
	for _, entry := range entries {
		left = append(left, entry.Name())
	}
	if got := strings.Join(left, " "); got != "other.exe.old-20261101T000000 wj.exe wj.exe.old-20261110T213238" {
		t.Errorf("left: %s", got)
	}
}
