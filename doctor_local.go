package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Checks about this machine's own leftovers: program backups that updates could
// not delete, and a kit project whose Claude memory is not inside the project.

// selfExecutable is the running program; tests replace it.
var selfExecutable = func() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	return path
}

// checkOldExecutables finds .old-* backups of wj beyond the newest, which stays
// as a way back. The fix moves the rest to the trash.
func checkOldExecutables(env doctorEnv) []doctorFinding {
	dest := selfExecutable()
	if dest == "" {
		return nil
	}
	backups, _ := filepath.Glob(dest + ".old-*")
	sort.Strings(backups) // the name ends in a UTC time, so this is oldest first
	if len(backups) < 2 {
		return nil
	}
	old := backups[:len(backups)-1]
	var size int64
	var names []string
	for _, path := range old {
		if info, err := os.Stat(path); err == nil {
			size += info.Size()
		}
		names = append(names, filepath.Base(path))
	}
	return []doctorFinding{{
		Key:    "old-programs",
		Title:  fmt.Sprintf("%d bản wj cũ (.old-*) còn nằm cạnh chương trình", len(old)),
		Detail: fmt.Sprintf("%s\nChiếm ~%d MB; bản mới nhất được giữ để quay lại.", strings.Join(names, "\n"), size>>20),
		Fix:    "chuyển các bản cũ vào thùng rác (giữ bản mới nhất)",
		fix: func(out io.Writer) error {
			if err := moveToTrash(old); err != nil {
				return fmt.Errorf("chuyển vào thùng rác: %w", err)
			}
			fmt.Fprintf(out, "   đã chuyển %d bản cũ vào thùng rác\n", len(old))
			return nil
		},
	}}
}

// checkLocalMemory: a project with the kit keeps Claude's auto memory inside
// the project, via autoMemoryDirectory in settings.local.json.
func checkLocalMemory(env doctorEnv) []doctorFinding {
	root := projectRoot(env.cwd)
	if env.cwd == "" || !isGitRepo(root) {
		return nil
	}
	agents, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if !strings.Contains(string(agents), "<!-- wj:begin ") {
		return nil
	}
	change, err := planLocalMemory(root, nil)
	if err != nil || !change.write {
		return nil
	}
	return []doctorFinding{{
		Key:    "memory:" + filepath.Base(root),
		Title:  "Memory của Claude Code chưa nằm trong project: " + root,
		Detail: "settings.local.json chưa có autoMemoryDirectory nên memory nằm dưới ~/.claude, không theo repo.",
		Fix:    "thêm autoMemoryDirectory vào " + filepath.ToSlash(change.path) + " (file không commit)",
		fix: func(out io.Writer) error {
			if err := os.MkdirAll(filepath.Dir(change.path), 0o755); err != nil {
				return err
			}
			return os.WriteFile(change.path, []byte(change.text), 0o644)
		},
	}}
}
