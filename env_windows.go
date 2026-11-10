//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const cpUTF8 = 65001

// prepareConsole switches the console to UTF-8 so Vietnamese text renders,
// and returns a function restoring the previous code page for the parent shell.
func prepareConsole() func() {
	old, err := windows.GetConsoleOutputCP()
	if err != nil || old == cpUTF8 {
		return func() {}
	}
	if windows.SetConsoleOutputCP(cpUTF8) != nil {
		return func() {}
	}
	return func() { _ = windows.SetConsoleOutputCP(old) }
}

// processStartTime is when the process was created, which tells a live process from a
// later one that was given the same pid.
func processStartTime(pid int) (time.Time, bool) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return time.Time{}, false
	}
	defer windows.CloseHandle(handle)
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(handle, &created, &exited, &kernel, &user) != nil {
		return time.Time{}, false
	}
	return time.Unix(0, created.Nanoseconds()), true
}

func processAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var code uint32
	const stillActive = 259
	return windows.GetExitCodeProcess(handle, &code) == nil && code == stillActive
}

// enableTerminalEscapes turns on ANSI handling for consoles that default to off.
func enableTerminalEscapes() func() {
	handle := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if windows.GetConsoleMode(handle, &mode) != nil {
		return func() {}
	}
	if windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) != nil {
		return func() {}
	}
	return func() { _ = windows.SetConsoleMode(handle, mode) }
}

// Refresh only this process so later wizard steps can see freshly installed apps.
func refreshProcessPath() {
	paths := filepath.SplitList(os.Getenv("PATH"))
	for _, location := range []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`},
		{registry.CURRENT_USER, `Environment`},
	} {
		key, err := registry.OpenKey(location.root, location.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		value, _, err := key.GetStringValue("Path")
		key.Close()
		if err != nil {
			continue
		}
		if expanded, err := registry.ExpandString(value); err == nil {
			value = expanded
		}
		for _, path := range filepath.SplitList(value) {
			found := false
			for _, old := range paths {
				if strings.EqualFold(old, path) {
					found = true
					break
				}
			}
			if path != "" && !found {
				paths = append(paths, path)
			}
		}
	}
	_ = os.Setenv("PATH", strings.Join(paths, string(os.PathListSeparator)))
}
