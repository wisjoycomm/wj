//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// shellCommand runs a Verify command the way a user would type it. cmd.exe has
// its own quoting rules, so the line is passed as written, not re-escaped.
func shellCommand(command string) *exec.Cmd {
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /d /s /c "` + command + `"`}
	return cmd
}
