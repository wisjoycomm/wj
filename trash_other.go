//go:build !windows

package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// systemTrash moves files and folders to the user's trash: ~/.Trash on macOS,
// the XDG trash (with its .trashinfo, so the file manager can restore) elsewhere.
// It never deletes: a file it cannot move stays where it is.
func systemTrash(paths []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	files := filepath.Join(home, ".Trash")
	info := ""
	if runtime.GOOS != "darwin" {
		data := os.Getenv("XDG_DATA_HOME")
		if data == "" {
			data = filepath.Join(home, ".local", "share")
		}
		files = filepath.Join(data, "Trash", "files")
		info = filepath.Join(data, "Trash", "info")
		if err := os.MkdirAll(info, 0o700); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(files, 0o700); err != nil {
		return err
	}
	for _, path := range paths {
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		name := filepath.Base(abs)
		target := filepath.Join(files, name)
		for n := 2; ; n++ {
			if _, err := os.Lstat(target); os.IsNotExist(err) {
				break
			}
			ext := filepath.Ext(name)
			target = filepath.Join(files, fmt.Sprintf("%s.%d%s", strings.TrimSuffix(name, ext), n, ext))
		}
		if info != "" {
			record := "[Trash Info]\nPath=" + (&url.URL{Path: abs}).EscapedPath() + "\nDeletionDate=" + time.Now().Format("2006-01-02T15:04:05") + "\n"
			if err := os.WriteFile(filepath.Join(info, filepath.Base(target)+".trashinfo"), []byte(record), 0o600); err != nil {
				return err
			}
		}
		if err := os.Rename(abs, target); err != nil {
			if info != "" {
				_ = os.Remove(filepath.Join(info, filepath.Base(target)+".trashinfo"))
			}
			return fmt.Errorf("không chuyển được %s vào thùng rác (không có gì bị xoá vĩnh viễn): %w", path, err)
		}
	}
	return nil
}
