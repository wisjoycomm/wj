package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// legacyName is what this tool was called up to v1.5.
const legacyName = "setup-gateway"

// stateDir is where profiles and proxy data live. It falls back to the legacy
// folder until migrateState has moved it (a running proxy can hold it open).
func stateDir(home string) string {
	current := filepath.Join(home, ".wj")
	legacy := filepath.Join(home, "."+legacyName)
	if !isDir(current) && isDir(legacy) {
		return legacy
	}
	return current
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// migrateState moves the legacy folder to its new name. Failure is harmless:
// stateDir keeps using the old folder and the next run tries again.
func migrateState(home string) {
	current := filepath.Join(home, ".wj")
	legacy := filepath.Join(home, "."+legacyName)
	if !isDir(current) && isDir(legacy) {
		_ = os.Rename(legacy, current)
	}
}

// ensureShortCommand handles a v1.5 install that self-updated in place: the
// new program still sits under the old file name, so `wj` is not on PATH yet.
func ensureShortCommand(exe string, out io.Writer) error {
	ext := filepath.Ext(exe)
	if !strings.EqualFold(strings.TrimSuffix(filepath.Base(exe), ext), legacyName) {
		return nil
	}
	short := filepath.Join(filepath.Dir(exe), "wj"+ext)
	if _, err := os.Stat(short); err == nil {
		return nil
	}
	if err := replaceExecutable(exe, short); err != nil {
		return err
	}
	fmt.Fprintf(out, "Tool đã đổi tên: từ giờ gõ wj (đã tạo %s). Lệnh cũ vẫn chạy, có thể xóa file cũ.\n", short)
	return nil
}

// shouldMigrate is false for commands promised not to change the machine.
func shouldMigrate(args []string) bool {
	if len(args) > 0 {
		switch args[0] {
		case "check", "usage", "overlay", "version", "--version", "-version", "help", "-h", "--help", "-help", "/?":
			return false
		}
	}
	for _, arg := range args {
		if arg == "-dry-run" || arg == "--dry-run" {
			return false
		}
	}
	return true
}

// migrateLegacy runs once per start and never blocks the requested command.
func migrateLegacy(args []string, out io.Writer) {
	if !shouldMigrate(args) {
		return
	}
	if home, err := os.UserHomeDir(); err == nil {
		migrateState(home)
	}
	if exe, err := os.Executable(); err == nil {
		if err = ensureShortCommand(exe, out); err != nil {
			fmt.Fprintln(out, "Chưa tạo được lệnh wj:", err)
		}
	}
}
