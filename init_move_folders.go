package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Kits before 0.2.12 told agents to put reusable scripts in scripts/ and docs in
// docs/. Now scripts go in .tools/ (one folder per area of work, long plain names)
// and docs in .docs/. When wj init updates a project from an older kit that still
// has those folders, Claude Code moves what the rules put there and fixes every
// reference. Files that a build, CI or a docs site uses stay where they are.

// dotFoldersKit is the first kit that moves them (0.2.12 had the new rule only).
const dotFoldersKit = "0.2.13"

// initFolderMover moves the old folders of the project at root. nil means no
// moving (a dry run, -no-merge, tests).
var initFolderMover func(root string, folders []string) (string, error)

// newFolderMover makes the mover of a real run; tests replace it.
var newFolderMover = agentFolderMover

// oldKitFolders lists the folders to move: only for a git project set up with a
// kit older than dotFoldersKit.
func oldKitFolders(root, oldKit string) []string {
	if oldKit == "" || !versionLess(oldKit, dotFoldersKit) || !isGitRepo(root) {
		return nil
	}
	var found []string
	for _, name := range []string{"scripts", "docs"} {
		if isDir(filepath.Join(root, name)) {
			found = append(found, name+"/")
		}
	}
	return found
}

// moveOldKitFolders runs before the kit is planned, so the merges see the moved
// paths. It reports whether the kit version may be recorded: a failed move keeps
// the old version, so the next wj init tries again.
func moveOldKitFolders(root, oldKit string, dry bool, out io.Writer) bool {
	folders := oldKitFolders(root, oldKit)
	if len(folders) == 0 {
		return true
	}
	list := strings.Join(folders, ", ")
	if dry {
		fmt.Fprintf(out, "Lúc chạy thật: nhờ Claude Code chuyển %s sang .tools/ và .docs/, sửa mọi chỗ trỏ tới.\n", list)
		return true
	}
	if initFolderMover == nil {
		return true
	}
	fmt.Fprintf(out, "Kit mới để script trong .tools/ và docs trong .docs/. Đang nhờ Claude Code chuyển %s (có thể mất vài phút)...\n", list)
	report, err := initFolderMover(root, folders)
	if err != nil {
		fmt.Fprintf(out, "Chưa chuyển được: %v\nLần wj init sau sẽ thử lại.\n\n", err)
		return false
	}
	fmt.Fprintf(out, "%s\n\n", strings.TrimSpace(cutRunes(report, 2000)))
	return true
}

// strayLocalSettings is where wj 0.2.11 copied the kit's settings.local.json by
// mistake: the project root, where Claude Code never reads it.
const strayLocalSettings = "settings.local.json"

// planStrayLocalSettings removes that copy, but only while it is still the kit's
// text: one the user changed is theirs.
func planStrayLocalSettings(root string) (initChange, bool) {
	path := filepath.Join(root, strayLocalSettings)
	have, err := os.ReadFile(path)
	if err != nil {
		return initChange{}, false
	}
	kit, err := kitFS.ReadFile(kitBase + "/settings.local.json")
	if err != nil || !bytes.Equal(normalizeEOL(have), normalizeEOL(kit)) {
		return initChange{}, false
	}
	return initChange{path: path, remove: true, status: "xoá (bản sao nhầm chỗ của wj 0.2.11, bản đúng ở .claude/)"}, true
}

func withoutPath(changes []initChange, path string) []initChange {
	kept := changes[:0]
	for _, change := range changes {
		if change.path != path {
			kept = append(kept, change)
		}
	}
	return kept
}

// agentFolderMover runs Claude Code in print mode inside the project. It may only
// read, edit, make folders and git mv: no commit, no delete, no other command.
func agentFolderMover(io.Writer) func(string, []string) (string, error) {
	return func(root string, folders []string) (string, error) {
		claude, err := exec.LookPath("claude")
		if err != nil {
			return "", errors.New("không thấy lệnh claude để chuyển")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, claude, "-p", moveInstruction(folders),
			"--permission-mode", "acceptEdits",
			"--allowedTools", "Read", "Edit", "Write", "Glob", "Grep",
			"Bash(git mv:*)", "Bash(git status:*)", "Bash(git ls-files:*)", "Bash(git grep:*)", "Bash(mkdir:*)")
		cmd.Dir = root
		cmd.Stdin = strings.NewReader("")
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("claude -p: %w %s", err, strings.TrimSpace(cutRunes(stderr.String(), 200)))
		}
		return stdout.String(), nil
	}
}

func moveInstruction(folders []string) string {
	return "This project was set up with an older wj kit, whose rules put reusable scripts in `scripts/` and docs in " +
		"`docs/`. The rules now say: scripts go in `.tools/`, grouped in one folder per broad area of work " +
		"(e.g. `.tools/release-new-version/`, not one folder per script and not a flat pile), with long plain names " +
		"for folders and scripts that say what they do; docs go in `.docs/`. Move the contents of " +
		strings.Join(folders, " and ") + " accordingly.\n" +
		"- First check each file. Leave it where it is if a build, CI, package manager, container or docs site " +
		"uses it by path (package.json scripts, Makefile, .github/workflows, Dockerfile, mkdocs.yml, GitHub Pages, " +
		"an import in code), or if it is not tracked by git.\n" +
		"- Move with `mkdir -p` and `git mv`. Rename a script only when the new name is clearly better.\n" +
		"- Fix every reference to a moved path in the repo: docs, README, AGENTS.md, CLAUDE.md, .claude/skills, " +
		"and paths inside the moved scripts (a script that finds the repo root from its own folder is now one level deeper).\n" +
		"- Don't commit, don't delete anything, run no other commands.\n" +
		"Reply in Vietnamese, plain and short: one line per move (old path -> new path), then what you left and why."
}
