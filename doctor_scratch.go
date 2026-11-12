package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Scratch folders only ever grow: the rule "delete them when done" is a
// sentence a model forgets. Doctor finds what is old and offers to delete it.

const scratchMaxAge = 7 * 24 * time.Hour

// scratchFolders are where agents leave scratch files. The kit names .tmp/;
// .temp/ is what an agent picks when it has not read that rule.
var scratchFolders = []string{".tmp", ".temp"}

type scratchFile struct {
	path     string
	size     int64
	modified time.Time
}

// oldScratch lists a folder's files older than the limit, and counts the rest.
func oldScratch(dir string, before time.Time) (old []scratchFile, kept int) {
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().Before(before) {
			old = append(old, scratchFile{path, info.Size(), info.ModTime()})
		} else {
			kept++
		}
		return nil
	})
	return old, kept
}

func fileSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%d KB", (n+1023)/1024)
}

// removeScratch moves the files to the trash, then removes the folders they
// left empty. The scratch folder itself stays.
func removeScratch(dir string, files []scratchFile) error {
	var paths []string
	for _, file := range files {
		if _, err := os.Lstat(file.path); err == nil {
			paths = append(paths, file.path)
		}
	}
	if len(paths) > 0 {
		if err := moveToTrash(paths); err != nil {
			return err
		}
	}
	var folders []string
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.IsDir() && path != dir {
			folders = append(folders, path)
		}
		return nil
	})
	// Deepest first; Remove fails on a folder that still has files, which is right.
	sort.Slice(folders, func(a, b int) bool { return len(folders[a]) > len(folders[b]) })
	for _, folder := range folders {
		_ = os.Remove(folder)
	}
	return nil
}

func checkScratch(env doctorEnv) []doctorFinding {
	roots := map[string]bool{}
	if env.cwd != "" {
		if root := projectRoot(env.cwd); isGitRepo(root) {
			roots[root] = true
			roots[mainRepoRoot(root)] = true
		}
	}
	for root := range loadProjectIndex(env.home) {
		roots[root] = true
	}
	var sorted []string
	for root := range roots {
		sorted = append(sorted, root)
	}
	sort.Strings(sorted)
	before := time.Now().Add(-scratchMaxAge)
	days := int(scratchMaxAge.Hours() / 24)
	var findings []doctorFinding
	for _, root := range sorted {
		for _, name := range scratchFolders {
			dir := filepath.Join(root, name)
			old, kept := oldScratch(dir, before)
			if len(old) == 0 {
				continue
			}
			var total int64
			oldest := old[0].modified
			for _, file := range old {
				total += file.size
				if file.modified.Before(oldest) {
					oldest = file.modified
				}
			}
			sort.Slice(old, func(a, b int) bool { return old[a].size > old[b].size })
			var names []string
			for i, file := range old {
				if i == 5 {
					names = append(names, fmt.Sprintf("... và %d file khác", len(old)-5))
					break
				}
				rel, err := filepath.Rel(root, file.path)
				if err != nil {
					rel = file.path
				}
				names = append(names, fmt.Sprintf("%s (%s)", filepath.ToSlash(rel), fileSize(file.size)))
			}
			detail := fmt.Sprintf("Cũ nhất: %s. Giữ lại %d file mới hơn %d ngày.\nLớn nhất: %s",
				oldest.Format("2006-01-02"), kept, days, strings.Join(names, ", "))
			if name != ".tmp" {
				detail += "\nKit dùng .tmp/ (đã gitignore); " + name + "/ là do agent tự đặt."
			}
			findings = append(findings, doctorFinding{
				Key: "scratch:" + filepath.Base(root) + "/" + name,
				Title: fmt.Sprintf("File tạm cũ hơn %d ngày trong %s: %d file, %s: %s",
					days, name, len(old), fileSize(total), root),
				Detail: detail,
				Fix:    fmt.Sprintf("chuyển %d file đó vào thùng rác (khôi phục được); file mới hơn %d ngày giữ nguyên", len(old), days),
				fix: func(out io.Writer) error {
					return removeScratch(dir, old)
				},
			})
		}
	}
	return findings
}
