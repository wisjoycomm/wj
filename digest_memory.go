package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// What /orca-dream curates besides failures: the notes Claude Code wrote on its
// own (the memory folder) and the approved lessons. The digest lists them with
// what a program can tell cheaply, so the model reads only the notes worth a look.

const (
	lessonsHeading = "## Lessons"
	lessonsLimit   = 40 // lines; the skill keeps each list under this
	memoryListed   = 40 // notes printed
)

var (
	// lessonFiles hold a "## Lessons" list: the committed one every agent reads,
	// and the one for facts of this machine only.
	lessonFiles = []string{"AGENTS.md", "CLAUDE.local.md"}

	codeSpan     = regexp.MustCompile("`([^`\n]+)`")
	pathLike     = regexp.MustCompile(`^[\w.@~\-]+([/\\][\w.@\-]+)+/?$|^[\w\-]+\.(go|md|json|ya?ml|toml|py|ts|tsx|js|jsx|cs|ps1|sh|cmd|txt|mod|sum|lock)$`)
	memoryLink   = regexp.MustCompile(`\]\(([^)\s]+\.md)\)`)
	frontmatterK = regexp.MustCompile(`^\s*(description|type):\s*(.*)$`)
)

type memoryNote struct {
	file        string
	kind        string // user, feedback, project, reference
	description string
	lines       int
	bytes       int64
	modified    time.Time
	missing     []string // paths the note names that are not in the project
}

// memoryDir is where Claude Code keeps this project's notes: the folder wj init
// set, or the one the user chose.
func memoryDir(root string) string {
	for _, name := range []string{"settings.local.json", "settings.json"} {
		doc, err := readConfig(filepath.Join(root, ".claude", name))
		if err != nil {
			continue
		}
		if dir, ok := doc["autoMemoryDirectory"].(string); ok && dir != "" {
			if strings.HasPrefix(dir, "~") {
				if home, err := os.UserHomeDir(); err == nil {
					dir = filepath.Join(home, dir[1:])
				}
			}
			return filepath.Clean(dir)
		}
	}
	return filepath.Join(root, ".claude", "memory")
}

// missingPaths lists the paths a text names in backticks that the project does
// not have. A name that is not a path at all (a command, a flag) is skipped, so
// what is left is worth a check, not proof.
func missingPaths(text, root string) []string {
	var missing []string
	for _, match := range codeSpan.FindAllStringSubmatch(text, -1) {
		name := strings.TrimSpace(match[1])
		if !pathLike.MatchString(name) || strings.HasPrefix(name, "~") {
			continue
		}
		path := filepath.FromSlash(name)
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		if _, err := os.Stat(path); err != nil {
			missing = appendUnique(missing, name)
		}
	}
	return missing
}

func readMemoryNote(path, root string) (memoryNote, error) {
	info, err := os.Stat(path)
	if err != nil {
		return memoryNote{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return memoryNote{}, err
	}
	text := string(normalizeEOL(data))
	note := memoryNote{file: filepath.Base(path), bytes: info.Size(), modified: info.ModTime(),
		lines: strings.Count(strings.TrimRight(text, "\n"), "\n") + 1}
	body := text
	if strings.HasPrefix(text, "---\n") {
		if end := strings.Index(text[4:], "\n---"); end >= 0 {
			for _, line := range strings.Split(text[4:4+end], "\n") {
				if field := frontmatterK.FindStringSubmatch(line); field != nil {
					value := strings.Trim(strings.TrimSpace(field[2]), `"'`)
					if field[1] == "type" {
						note.kind = value
					} else {
						note.description = value
					}
				}
			}
			body = text[4+end:]
		}
	}
	note.missing = missingPaths(body, root)
	return note, nil
}

// lessonLines returns the lines of a file's "## Lessons" section, with the
// number of each in the file.
func lessonLines(text string) (lines []string, numbers []int) {
	in := false
	for i, line := range strings.Split(string(normalizeEOL([]byte(text))), "\n") {
		switch {
		case strings.TrimSpace(line) == lessonsHeading:
			in = true
		case in && (strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "<!-- wj:begin")):
			in = false
		case in && strings.TrimSpace(line) != "":
			lines = append(lines, line)
			numbers = append(numbers, i+1)
		}
	}
	return lines, numbers
}

func printMemory(out io.Writer, root string, now time.Time) {
	dir := memoryDir(root)
	shownDir := dir
	if rel, err := filepath.Rel(root, dir); err == nil && !strings.HasPrefix(rel, "..") {
		shownDir = filepath.ToSlash(rel)
	}
	entries, _ := os.ReadDir(dir)
	var notes []memoryNote
	index := ""
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if strings.EqualFold(entry.Name(), "MEMORY.md") {
			if data, err := os.ReadFile(path); err == nil {
				index = string(normalizeEOL(data))
			}
			continue
		}
		if note, err := readMemoryNote(path, root); err == nil {
			notes = append(notes, note)
		}
	}
	if len(notes) == 0 && strings.TrimSpace(index) == "" {
		fmt.Fprintf(out, "MEMORY (%s): no notes\n", shownDir)
	} else {
		indexLines := 0
		if strings.TrimSpace(index) != "" {
			indexLines = strings.Count(strings.TrimRight(index, "\n"), "\n") + 1
		}
		fmt.Fprintf(out, "MEMORY (%s): %d notes; index MEMORY.md %d lines, ~%s tokens, loaded in every session\n",
			shownDir, len(notes), indexLines, compactTokens(int64(len(index)/4)))
		fmt.Fprintln(out, "  (file, type, age, lines, then what it is about; MISSING = a path it names is not in the project)")
		sort.Slice(notes, func(a, b int) bool { return notes[a].modified.Before(notes[b].modified) })
		linked := map[string]bool{}
		for _, match := range memoryLink.FindAllStringSubmatch(index, -1) {
			linked[strings.ToLower(filepath.Base(match[1]))] = true
		}
		have := map[string]bool{}
		var unlisted []string
		for i, note := range notes {
			have[strings.ToLower(note.file)] = true
			if !linked[strings.ToLower(note.file)] {
				unlisted = append(unlisted, note.file)
			}
			if i >= memoryListed {
				continue
			}
			kind := note.kind
			if kind == "" {
				kind = "?"
			}
			line := fmt.Sprintf("  %-34s %-9s %4dd %4d  %s", cutRunes(note.file, 34), kind,
				int(now.Sub(note.modified).Hours()/24), note.lines, cutRunes(note.description, 70))
			if len(note.missing) > 0 {
				line += "  MISSING: " + strings.Join(note.missing, ", ")
			}
			fmt.Fprintln(out, strings.TrimRight(line, " "))
		}
		if len(notes) > memoryListed {
			fmt.Fprintf(out, "  ... and %d newer notes\n", len(notes)-memoryListed)
		}
		var dangling []string
		for name := range linked {
			if !have[name] {
				dangling = append(dangling, name)
			}
		}
		sort.Strings(dangling)
		if len(dangling) > 0 {
			fmt.Fprintln(out, "  INDEX links notes that don't exist:", strings.Join(dangling, ", "))
		}
		if len(unlisted) > 0 {
			fmt.Fprintln(out, "  NOT IN INDEX (never recalled):", strings.Join(unlisted, ", "))
		}
	}

	for _, name := range lessonFiles {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		lines, numbers := lessonLines(string(data))
		if len(lines) == 0 {
			continue
		}
		over := ""
		if len(lines) > lessonsLimit {
			over = fmt.Sprintf(", over the limit of %d", lessonsLimit)
		}
		fmt.Fprintf(out, "LESSONS (%s): %d lines%s, loaded in every session\n", name, len(lines), over)
		for i, line := range lines {
			if missing := missingPaths(line, root); len(missing) > 0 {
				fmt.Fprintf(out, "  %s:%d  MISSING: %s  | %s\n", name, numbers[i], strings.Join(missing, ", "), cutRunes(strings.TrimSpace(line), 80))
			}
		}
	}
}
