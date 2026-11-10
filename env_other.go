//go:build !windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func prepareConsole() func() { return func() {} }

func enableTerminalEscapes() func() { return func() {} }

// processStartTime is not looked up here: markers from this system carry no start time
// to compare, so the pid is trusted.
func processStartTime(pid int) (time.Time, bool) { return time.Time{}, false }

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func refreshProcessPath() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	paths := filepath.SplitList(os.Getenv("PATH"))
	for _, dir := range []string{filepath.Join(home, ".local", "bin"), filepath.Join(home, ".opencode", "bin"), filepath.Join(home, ".agy", "bin"), "/opt/homebrew/bin", "/usr/local/bin"} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() && !contains(paths, dir) {
			paths = append(paths, dir)
		}
	}
	_ = os.Setenv("PATH", strings.Join(paths, string(os.PathListSeparator)))
}
