//go:build !windows

package main

import "os/exec"

// shellCommand runs a Verify command the way a user would type it.
func shellCommand(command string) *exec.Cmd {
	shell := "bash"
	if _, err := exec.LookPath(shell); err != nil {
		shell = "sh"
	}
	return exec.Command(shell, "-c", command)
}
