package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Connection scope: "global" writes the app's user config (whole machine),
// "project" writes a personal, gitignored file inside one project so only
// sessions started there use the connection.

const (
	scopeGlobal  = "global"
	scopeProject = "project"
)

// resolveScope checks -scope and finds the project folder. dir is the -dir flag
// ("" = current folder). It returns the scope and, for project scope, the root.
func resolveScope(scope, dir, home string) (string, string, error) {
	switch scope {
	case "", "global", "machine", "may":
		return scopeGlobal, "", nil
	case "project", "local":
	default:
		return "", "", fmt.Errorf("-scope chỉ nhận global (toàn máy) hoặc project (chỉ project này), không phải %q", scope)
	}
	root, err := scopeProjectRoot(dir)
	if err != nil {
		return "", "", err
	}
	if home != "" && strings.EqualFold(filepath.Clean(root), filepath.Clean(home)) {
		return "", "", errors.New("đang đứng ở thư mục home, không phải project: cd vào project hoặc thêm -dir")
	}
	return scopeProject, root, nil
}

func scopeProjectRoot(dir string) (string, error) {
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("không đọc được thư mục hiện tại: %w", err)
		}
		dir = wd
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("thư mục project %s: %w", dir, err)
	}
	if !isDir(abs) {
		return "", fmt.Errorf("không có thư mục project %s", abs)
	}
	return projectRoot(abs), nil
}

// askScope offers project scope only when the current folder is inside a git
// project; elsewhere the connection is machine-wide without a question.
func askScope(input io.Reader, out io.Writer, dir, home string) (string, error) {
	root, err := scopeProjectRoot(dir)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil || strings.EqualFold(filepath.Clean(root), filepath.Clean(home)) {
		return scopeGlobal, nil
	}
	return pickOne(input, out, "Áp dụng kết nối cho đâu?", []string{scopeGlobal, scopeProject},
		[]string{"Toàn máy (mọi project)", "Chỉ project này: " + root}, 1)
}

// claudeProjectBlanks returns the routing keys set in the user settings that a
// project connection must hide: Claude Code merges env from both files, so a
// global gateway token would otherwise win over the project's own login.
func claudeProjectBlanks(home string) []string {
	global, err := readSettings(claudeSettingsPath(home))
	if err != nil {
		return nil
	}
	env, _ := global["env"].(map[string]any)
	var keys []string
	for key := range env {
		if isClaudeRoutingKey(key) {
			keys = append(keys, key)
		}
	}
	return keys
}

// applyClaudeProject writes the connection into <root>/.claude/settings.local.json,
// a personal file that git ignores, so the key never lands in a commit.
func applyClaudeProject(home, root string, p connectionProfile, dry bool) (string, error) {
	line, err := applyClaudeFile(projectSettingsPath(root), p, dry, claudeProjectBlanks(home))
	if err != nil {
		return "", err
	}
	if !dry {
		if _, err := ensureGitIgnoredEntry(root, ".claude/settings.local.json"); err != nil {
			return "", fmt.Errorf("thêm .claude/settings.local.json vào .gitignore: %w", err)
		}
	}
	return line + "\n  - chỉ project này; nơi khác vẫn dùng " + claudeSettingsPath(home), nil
}

// applyOpenCodeProject writes the provider into <root>/opencode.json(c), which
// OpenCode merges over the user config. A file git already tracks is refused:
// the key would end up in a commit.
func applyOpenCodeProject(root, base, key string, ids []string, provider, npm string, dry bool) (string, error) {
	for _, name := range []string{"opencode.json", "opencode.jsonc"} {
		if gitTracked(root, name) {
			return "", fmt.Errorf("%s đã nằm trong git; tool không ghi key vào file sẽ được commit. Dùng -scope global", filepath.Join(root, name))
		}
	}
	path, err := applyOpenCodeIn(root, base, key, ids, provider, npm, dry)
	if err != nil {
		return "", err
	}
	if !dry {
		if _, err := ensureGitIgnoredEntry(root, filepath.Base(path)); err != nil {
			return "", fmt.Errorf("thêm %s vào .gitignore: %w", filepath.Base(path), err)
		}
	}
	return path, nil
}

func gitTracked(root, name string) bool {
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return false
	}
	cmd := exec.Command("git", "-C", root, "ls-files", "--error-unmatch", "--", name)
	return cmd.Run() == nil
}
