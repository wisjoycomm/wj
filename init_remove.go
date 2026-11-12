package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

// runInitRemove takes the kit back out of a project, but only what wj owns:
// the wj:begin/wj:end blocks of AGENTS.md, kit files nobody changed, the
// version file, the guard hook and the Orca permission rules. A file the user
// edited stays, and so does everything they wrote around wj's blocks. Files go
// to the trash, never deleted for good.
func runInitRemove(root string, dry bool, out io.Writer) error {
	changes, err := planRemove(root)
	if err != nil {
		return err
	}
	if dry {
		fmt.Fprintf(out, "Xem trước, chưa gỡ gì: %s\n", root)
	} else {
		fmt.Fprintf(out, "Gỡ kit wj khỏi project: %s\n", root)
	}
	if !dry {
		if err := applyRemove(root, changes); err != nil {
			return err
		}
	}
	for _, change := range changes {
		rel, err := filepath.Rel(root, change.path)
		if err != nil {
			rel = change.path
		}
		fmt.Fprintf(out, "  %-62s %s\n", filepath.ToSlash(rel), change.status)
	}
	fmt.Fprintln(out, "\nGiữ lại: .gitignore, .claude/settings.local.json (memory), file bạn đã sửa và phần bạn tự viết.")
	if !dry {
		fmt.Fprintln(out, "File đã gỡ nằm trong thùng rác. Mở lại Claude Code để nó nhận settings mới.")
	}
	return nil
}

// planRemove works out every change before anything is touched. A file that
// is not there is left out of the list.
func planRemove(root string) ([]initChange, error) {
	var changes []initChange
	seen := map[string]bool{}
	for _, layer := range kitLayers(false) {
		err := fs.WalkDir(kitFS, layer, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			rel := kitTarget(strings.TrimPrefix(path, layer+"/"))
			if rel == "AGENTS.md" || rel == "rules.md" || rel == ".claude/settings.json" || seen[rel] {
				return nil
			}
			seen[rel] = true
			kit, err := kitFS.ReadFile(path)
			if err != nil {
				return err
			}
			target := filepath.Join(root, filepath.FromSlash(rel))
			have, err := os.ReadFile(target)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			same := bytes.Equal(normalizeEOL(have), normalizeEOL(kit))
			// A unity project got the shared-checkout worker instead.
			if source, ok := kitOverrides([]string{"unity"})[rel]; ok && !same {
				alt, err := kitFS.ReadFile(source)
				if err != nil {
					return err
				}
				same = bytes.Equal(normalizeEOL(have), normalizeEOL(alt))
			}
			if same {
				changes = append(changes, initChange{path: target, remove: true, status: "gỡ (giống bản kit)"})
			} else {
				changes = append(changes, initChange{path: target, status: "giữ nguyên (bạn đã sửa)"})
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	version := filepath.Join(root, filepath.FromSlash(kitVersionFile))
	if _, err := os.Stat(version); err == nil {
		changes = append(changes, initChange{path: version, remove: true, status: "gỡ"})
	}
	agents, ok, err := planRemoveAgents(root)
	if err != nil {
		return nil, err
	}
	if ok {
		changes = append(changes, agents)
	}
	settings, ok, err := planRemoveSettings(root)
	if err != nil {
		return nil, err
	}
	if ok {
		changes = append(changes, settings)
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].path < changes[j].path })
	return changes, nil
}

// planRemoveAgents drops every wj block from AGENTS.md. A file with nothing
// left in it was wj's alone, so it goes to the trash.
func planRemoveAgents(root string) (initChange, bool, error) {
	path := filepath.Join(root, "AGENTS.md")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return initChange{}, false, nil
	}
	if err != nil {
		return initChange{}, false, err
	}
	text, count := removeWjBlocks(string(normalizeEOL(data)))
	switch {
	case count == 0:
		return initChange{path: path, status: "giữ nguyên (không có khối wj)"}, true, nil
	case strings.TrimSpace(text) == "":
		return initChange{path: path, remove: true, status: fmt.Sprintf("gỡ (chỉ có %d khối wj)", count)}, true, nil
	}
	return initChange{path: path, text: text, write: true, status: fmt.Sprintf("bỏ %d khối wj, giữ phần bạn viết", count)}, true, nil
}

// removeWjBlocks cuts every wj:begin <id> ... wj:end <id> block, with the
// blank lines upsertBlock put before it. A block with no end marker stays.
func removeWjBlocks(text string) (string, int) {
	const prefix = "<!-- wj:begin "
	count := 0
	from := 0
	for {
		start := strings.Index(text[from:], prefix)
		if start < 0 {
			break
		}
		start += from
		closing := strings.Index(text[start:], " -->")
		if closing < 0 {
			break
		}
		id := text[start+len(prefix) : start+closing]
		length := strings.Index(text[start:], blockEnd(id))
		if length < 0 {
			from = start + closing
			continue
		}
		end := start + length + len(blockEnd(id))
		if end < len(text) && text[end] == '\n' {
			end++
		}
		head := strings.TrimRight(text[:start], "\n")
		tail := strings.TrimLeft(text[end:], "\n")
		switch {
		case head == "":
			text = tail
		case tail == "":
			text = head + "\n"
		default:
			text = head + "\n\n" + tail
		}
		from = len(head)
		if from > len(text) {
			from = len(text)
		}
		count++
	}
	return text, count
}

// planRemoveSettings takes out what the Orca layer added to settings.json: its
// hooks, its permission rules and its other keys while they still hold the
// kit's value, plus the WJ_GUARD switch. The base and job rules stay: they are
// plain git and tool rules a project keeps using without wj.
func planRemoveSettings(root string) (initChange, bool, error) {
	path := filepath.Join(root, ".claude", "settings.json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return initChange{}, false, nil
	}
	doc, err := readConfig(path)
	if err != nil {
		return initChange{}, false, fmt.Errorf("file %s bị lỗi cú pháp; tool không sửa nó", path)
	}
	data, err := kitFS.ReadFile(kitOrca + "/dot-claude/settings.json")
	if err != nil {
		return initChange{}, false, err
	}
	var kit map[string]any
	if err = json.Unmarshal(data, &kit); err != nil {
		return initChange{}, false, fmt.Errorf("%s/dot-claude/settings.json: %w", kitOrca, err)
	}
	if !unmergeSettings(doc, kit) {
		return initChange{path: path, status: "giữ nguyên (không có phần của wj)"}, true, nil
	}
	if len(doc) == 0 {
		return initChange{path: path, remove: true, status: "gỡ (chỉ có phần của wj)"}, true, nil
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(doc); err != nil {
		return initChange{}, false, err
	}
	return initChange{path: path, text: buf.String(), write: true, status: "bỏ hook wj guard và quyền Orca"}, true, nil
}

// unmergeSettings is mergeSettings run backwards for one kit layer. It
// reports whether it took anything out.
func unmergeSettings(doc, kit map[string]any) bool {
	removed := false
	for key, value := range kit {
		if key == "hooks" || key == "permissions" {
			continue
		}
		if have, exists := doc[key]; exists && reflect.DeepEqual(have, value) {
			delete(doc, key)
			removed = true
		}
	}
	if env, _ := doc["env"].(map[string]any); env != nil {
		if _, set := env[guardOffKey]; set {
			delete(env, guardOffKey)
			removed = true
		}
		if len(env) == 0 {
			delete(doc, "env")
		}
	}
	want, _ := kit["hooks"].(map[string]any)
	if have, _ := doc["hooks"].(map[string]any); have != nil && want != nil {
		removed = unmergeHooks(have, want) || removed
		if len(have) == 0 {
			delete(doc, "hooks")
		}
	}
	source, _ := kit["permissions"].(map[string]any)
	target, _ := doc["permissions"].(map[string]any)
	if source == nil || target == nil {
		return removed
	}
	for list, value := range source {
		rules, _ := value.([]any)
		drop := map[string]bool{}
		for _, rule := range rules {
			if name, ok := rule.(string); ok {
				drop[name] = true
			}
		}
		current, isList := target[list].([]any)
		if !isList {
			continue
		}
		kept := []any{}
		for _, rule := range current {
			if name, ok := rule.(string); ok && drop[name] {
				removed = true
				continue
			}
			kept = append(kept, rule)
		}
		if len(kept) == 0 {
			delete(target, list)
		} else {
			target[list] = kept
		}
	}
	if len(target) == 0 {
		delete(doc, "permissions")
	}
	return removed
}

// unmergeHooks drops the kit's hook commands from every matcher of their
// event, then the matchers and events left with no hook.
func unmergeHooks(have, want map[string]any) bool {
	removed := false
	for event, value := range want {
		drop := map[string]bool{}
		entries, _ := value.([]any)
		for _, entry := range entries {
			for _, command := range hookCommands(entry) {
				drop[command] = true
			}
		}
		current, isList := have[event].([]any)
		if !isList {
			continue
		}
		var keptEntries []any
		for _, entry := range current {
			group, _ := entry.(map[string]any)
			hooks, _ := group["hooks"].([]any)
			if group == nil || hooks == nil {
				keptEntries = append(keptEntries, entry)
				continue
			}
			var keptHooks []any
			for _, hook := range hooks {
				fields, _ := hook.(map[string]any)
				if command, ok := fields["command"].(string); ok && drop[command] {
					removed = true
					continue
				}
				keptHooks = append(keptHooks, hook)
			}
			if len(keptHooks) == 0 {
				continue
			}
			group["hooks"] = keptHooks
			keptEntries = append(keptEntries, group)
		}
		if len(keptEntries) == 0 {
			delete(have, event)
		} else {
			have[event] = keptEntries
		}
	}
	return removed
}

// applyRemove rewrites the files that keep the user's part, then sends the
// rest to the trash in one go, then drops the folders that left empty.
func applyRemove(root string, changes []initChange) error {
	var trash []string
	for _, change := range changes {
		switch {
		case change.remove:
			trash = append(trash, change.path)
		case change.write:
			mode := os.FileMode(0o644)
			if info, err := os.Stat(change.path); err == nil {
				mode = info.Mode().Perm()
			}
			if err := os.WriteFile(change.path, []byte(change.text), mode); err != nil {
				return fmt.Errorf("không ghi được %s: %w", change.path, err)
			}
		}
	}
	if len(trash) == 0 {
		return nil
	}
	if err := moveToTrash(trash); err != nil {
		return fmt.Errorf("không chuyển được vào thùng rác: %w", err)
	}
	for _, path := range trash {
		pruneEmptyDirs(root, filepath.Dir(path))
	}
	return nil
}

// pruneEmptyDirs removes dir and its parents up to root while they are empty.
// os.Remove fails on a folder with anything in it, which is what stops it.
func pruneEmptyDirs(root, dir string) {
	root = filepath.Clean(root)
	for dir = filepath.Clean(dir); dir != root && strings.HasPrefix(dir, root+string(filepath.Separator)); dir = filepath.Dir(dir) {
		if os.Remove(dir) != nil {
			return
		}
	}
}
