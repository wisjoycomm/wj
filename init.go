package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed all:kit
var kitFS embed.FS

const (
	// The kit has two layers. kit/base is plain files that work on any machine
	// (wj init -lite installs only this). kit/orca is what calls wj: the plan,
	// the skills, the worker subagent and the guard hook.
	kitBase   = "kit/base"
	kitOrca   = "kit/orca"
	kitStacks = "kit/stacks"
)

// kitLayers are the layers of one run, in the order their rules appear.
func kitLayers(lite bool) []string {
	if lite {
		return []string{kitBase}
	}
	return []string{kitBase, kitOrca}
}

// kitTarget is where a kit/base file goes in a project. Folders that start
// with a dot are stored as dot-<name>, so tools working on this repo don't
// take the kit's .claude folder for the repo's own.
func kitTarget(rel string) string {
	return strings.TrimPrefix(strings.ReplaceAll("/"+rel, "/dot-", "/."), "/")
}

// kitSource is the kit/orca path of a project file wj owns (a skill, the worker).
func kitSource(rel string) string {
	if strings.HasPrefix(rel, ".") {
		rel = "dot-" + rel[1:]
	}
	return kitOrca + "/" + rel
}

// workerRel is the project path of the worker subagent.
const workerRel = ".claude/agents/orca-worker.md"

// unityWorkerSource is the worker for a project with a unity job: it works in
// the shared checkout, with no worktree.
const unityWorkerSource = kitStacks + "/unity/dot-claude/agents/orca-worker.md"

// kitOverrides maps project paths to the kit file that replaces the Orca
// layer's copy for these jobs.
func kitOverrides(stacks []string) map[string]string {
	if containsName(stacks, "unity") {
		return map[string]string{workerRel: unityWorkerSource}
	}
	return nil
}

// initStack is one kind of job. Its files live in kit/stacks/<id>/: rules.md
// goes into AGENTS.md, gitignore into .gitignore, settings.json (optional) into
// .claude/settings.json.
type initStack struct {
	id     string
	label  string
	detect []string // any of these files in the project root means this stack
}

var initStacksList = []initStack{
	{"unity", "Unity", []string{"ProjectSettings/ProjectVersion.txt"}},
	{"web", "Web (Node, React, Next...)", []string{"package.json"}},
	{"go", "Go", []string{"go.mod"}},
	{"python", "Python", []string{"pyproject.toml", "requirements.txt", "setup.py"}},
}

// initChange is one planned file result. Nothing is written until every change
// has been worked out, so a broken file stops the run before it touches anything.
type initChange struct {
	path   string
	text   string
	write  bool
	backup bool // keep the old file next to it as .bak-<time>
	remove bool
	status string
}

func runInit(args []string, input io.Reader, out io.Writer) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(out)
	types := flags.String("type", "", "loại job, vd unity hoặc web,go (bỏ trống thì tool hỏi)")
	dry := flags.Bool("dry-run", false, "chỉ in ra sẽ làm gì, không ghi file")
	update := flags.Bool("update", false, "thay skill Orca cũ trong project bằng bản của kit (bản cũ sao lưu .bak)")
	lite := flags.Bool("lite", false, "chỉ cài phần không cần wj: rule, quyền, file cấm đọc (không skill Orca, không hook)")
	remove := flags.Bool("remove", false, "gỡ kit khỏi project: chỉ phần của wj, file bạn đã sửa giữ nguyên (vào thùng rác)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return errors.New("cách dùng: wj init [-type unity,web] [-lite] [-update] [-remove] [-dry-run] [thư mục]")
	}
	if *remove && (*types != "" || *update || *lite) {
		return errors.New("-remove không đi cùng -type, -update hay -lite")
	}
	dir := "."
	if flags.NArg() == 1 {
		dir = flags.Arg(0)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if !isDir(abs) {
		return fmt.Errorf("không có thư mục %s", abs)
	}
	root := projectRoot(abs)
	if *remove {
		return runInitRemove(root, *dry, out)
	}
	stacks, err := chooseStacks(*types, root, input, out)
	if err != nil {
		return err
	}
	changes, err := planInit(root, stacks, *update, *lite)
	if err != nil {
		return err
	}
	return applyInit(root, stacks, changes, *dry, *lite, out)
}

// initMenu is the numbered-menu entry: it opens from anywhere, so ask where.
func initMenu(input io.Reader, out io.Writer) error {
	dir := strings.Trim(menuAnswer(input, out, "Thư mục project (Enter = thư mục hiện tại, 0 = quay lại)"), `"' `)
	if dir == "0" {
		return errCancelled
	}
	if dir == "" {
		return runInit(nil, input, out)
	}
	return runInit([]string{dir}, input, out)
}

func chooseStacks(types, root string, input io.Reader, out io.Writer) ([]string, error) {
	names := make([]string, 0, len(initStacksList))
	for _, stack := range initStacksList {
		names = append(names, stack.id)
	}
	if types != "" {
		return parseChoices(types, names)
	}
	detected := detectStacks(root)
	labels := make([]string, 0, len(initStacksList))
	for _, stack := range initStacksList {
		label := stack.label
		if containsName(detected, stack.id) {
			label += "  (phát hiện trong project)"
		}
		labels = append(labels, label)
	}
	printOptions(out, "Project này làm gì? Rule chung luôn được cài, đây là rule thêm theo job:", names, labels, 0)
	hint := "Gõ số (vd 1 3), Enter = chỉ rule chung"
	if len(detected) > 0 {
		hint = "Gõ số (vd 1 3), Enter = dùng " + strings.Join(detected, ", ")
	}
	answer := menuAnswer(input, out, hint+", 0 = quay lại")
	if answer == "0" {
		return nil, errCancelled
	}
	if answer == "" {
		return detected, nil
	}
	return parseChoices(answer, names)
}

func detectStacks(root string) []string {
	var found []string
	for _, stack := range initStacksList {
		for _, file := range stack.detect {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(file))); err == nil {
				found = appendUnique(found, stack.id)
			}
		}
	}
	return found
}

func planInit(root string, stacks []string, update, lite bool) ([]initChange, error) {
	var changes []initChange
	for _, layer := range kitLayers(lite) {
		copied, err := planLayer(root, layer, update, kitOverrides(stacks))
		if err != nil {
			return nil, err
		}
		changes = append(changes, copied...)
	}
	// Removals keep their own order (a folder's files before the folder), after
	// everything else.
	var old []initChange
	if update {
		for _, rel := range orcaOldFiles {
			path := filepath.Join(root, filepath.FromSlash(rel))
			if _, err := os.Stat(path); err == nil {
				old = append(old, initChange{path: path, remove: true, status: "xoá (kit mới không dùng nữa)"})
			}
		}
	}
	agents, err := planAgents(root, stacks, lite)
	if err != nil {
		return nil, err
	}
	settings, err := planSettings(root, stacks, lite)
	if err != nil {
		return nil, err
	}
	changes = append(changes, agents, settings)
	for _, plan := range []func(string, []string) (initChange, error){planLocalMemory, planGitignore} {
		change, err := plan(root, stacks)
		if err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}
	// The skills read the version file, so a lite kit has no use for it.
	if change, ok := planKitVersion(root, update); ok && !lite {
		changes = append(changes, change)
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].path < changes[j].path })
	return append(changes, old...), nil
}

// kitVersionFile records the wj version that installed the kit. The skills
// read it: a wj older than this number may lack a command they call, so they
// fall back to doing the step by hand.
const kitVersionFile = ".claude/wj-kit"

// planKitVersion writes the running wj's version to kitVersionFile: on first
// install, and on -update. A dev build (no version) writes nothing.
func planKitVersion(root string, update bool) (initChange, bool) {
	version := strings.TrimSpace(buildVersion)
	if version == "" {
		return initChange{}, false
	}
	path := filepath.Join(root, filepath.FromSlash(kitVersionFile))
	old, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return initChange{path: path, text: version + "\n", write: true, status: "tạo (wj " + version + ")"}, true
	case err != nil:
		return initChange{}, false
	case strings.TrimSpace(string(old)) == version:
		return initChange{path: path, status: "đã đúng"}, true
	case update:
		return initChange{path: path, text: version + "\n", write: true, status: "cập nhật (wj " + version + ")"}, true
	}
	return initChange{path: path, status: "giữ nguyên (kit từ wj " + strings.TrimSpace(string(old)) + ") - lấy bản mới: wj init -update"}, true
}

// addToWjBlock appends lines to the file's "# wj init" block, so running init
// again (or after an update) never stacks a new header. A file without one
// gets the block at the end. The file's own line ending is kept.
func addToWjBlock(text string, lines []string) string {
	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	rows := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(rows) > 0 && rows[len(rows)-1] == "" {
		rows = rows[:len(rows)-1]
	}
	header := -1
	for i, row := range rows {
		if strings.TrimSpace(row) == "# wj init" {
			header = i
		}
	}
	if header < 0 {
		if len(rows) > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, "# wj init")
		rows = append(rows, lines...)
	} else {
		end := header + 1
		for end < len(rows) && strings.TrimSpace(rows[end]) != "" {
			end++
		}
		rows = append(rows[:end], append(append([]string{}, lines...), rows[end:]...)...)
	}
	return strings.Join(rows, eol) + eol
}

// planLayer copies a layer's files. Its rules and settings are merged into the
// project's own files instead, by planAgents and planSettings.
func planLayer(root, layer string, update bool, overrides map[string]string) ([]initChange, error) {
	var changes []initChange
	err := fs.WalkDir(kitFS, layer, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel := kitTarget(strings.TrimPrefix(path, layer+"/"))
		if rel == "AGENTS.md" || rel == "rules.md" || rel == ".claude/settings.json" {
			return nil
		}
		if source, ok := overrides[rel]; ok {
			path = source
		}
		data, err := kitFS.ReadFile(path)
		if err != nil {
			return err
		}
		change, err := planCopy(filepath.Join(root, filepath.FromSlash(rel)), string(data))
		if err != nil {
			return err
		}
		// The Orca skills are wj's own files: an older copy can be replaced on request.
		if strings.HasPrefix(change.status, "giữ nguyên") && contains(orcaSkillFiles, rel) {
			if update {
				change = initChange{path: change.path, text: string(data), write: true, backup: true, status: "cập nhật (bản cũ: .bak)"}
			} else {
				change.status += " - lấy bản mới: wj init -update"
			}
		}
		changes = append(changes, change)
		return nil
	})
	return changes, err
}

// planCopy never overwrites: a file already in the project is the user's.
func planCopy(path, text string) (initChange, error) {
	old, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return initChange{path: path, text: text, write: true, status: "tạo"}, nil
	}
	if err != nil {
		return initChange{}, err
	}
	if bytes.Equal(normalizeEOL(old), normalizeEOL([]byte(text))) {
		return initChange{path: path, status: "đã đúng"}, nil
	}
	return initChange{path: path, status: "giữ nguyên (khác bản kit)"}, nil
}

func normalizeEOL(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}

// planAgents keeps wj's rules between markers, so a rerun refreshes them and
// never touches what the user wrote around them.
func planAgents(root string, stacks []string, lite bool) (initChange, error) {
	path := filepath.Join(root, "AGENTS.md")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return initChange{}, err
	}
	text := string(normalizeEOL(data))
	hasFile := err == nil
	note := ""
	if !hasFile || strings.Contains(text, blockBegin("base")) {
		base, err := kitFS.ReadFile(kitBase + "/AGENTS.md")
		if err != nil {
			return initChange{}, err
		}
		text = upsertBlock(text, "base", string(normalizeEOL(base)))
	} else {
		note = " - rule chung chưa thêm vì AGENTS.md là file của bạn"
	}
	blocks := map[string]string{}
	ids := append([]string{}, stacks...)
	for _, id := range stacks {
		blocks[id] = kitStacks + "/" + id + "/rules.md"
	}
	if !lite {
		// The workers need these rules, so they go in like a job's rules do.
		ids = append([]string{"orca"}, ids...)
		blocks["orca"] = kitOrca + "/rules.md"
	}
	for _, id := range ids {
		rules, err := kitFS.ReadFile(blocks[id])
		if err != nil {
			return initChange{}, err
		}
		text = upsertBlock(text, id, string(normalizeEOL(rules)))
	}
	if hasFile && text == string(normalizeEOL(data)) {
		return initChange{path: path, status: "đã đúng" + note}, nil
	}
	status := "tạo"
	if hasFile {
		status = "cập nhật" + note
	}
	return initChange{path: path, text: text, write: true, status: status}, nil
}

func blockBegin(id string) string { return "<!-- wj:begin " + id + " -->" }
func blockEnd(id string) string   { return "<!-- wj:end " + id + " -->" }

func upsertBlock(text, id, body string) string {
	block := blockBegin(id) + "\n" + strings.TrimSpace(body) + "\n" + blockEnd(id) + "\n"
	start := strings.Index(text, blockBegin(id))
	if start >= 0 {
		if length := strings.Index(text[start:], blockEnd(id)); length >= 0 {
			end := start + length + len(blockEnd(id))
			if end < len(text) && text[end] == '\n' {
				end++
			}
			return text[:start] + block + text[end:]
		}
	}
	if text == "" {
		return block
	}
	return strings.TrimRight(text, "\n") + "\n\n" + block
}

// planSettings only ever adds permission rules; everything else in the file stays.
func planSettings(root string, stacks []string, lite bool) (initChange, error) {
	path := filepath.Join(root, ".claude", "settings.json")
	doc, err := readConfig(path)
	if err != nil {
		return initChange{}, fmt.Errorf("file %s bị lỗi cú pháp; tool không sửa nó", path)
	}
	_, statErr := os.Stat(path)
	hasFile := statErr == nil
	var sources []string
	for _, layer := range kitLayers(lite) {
		sources = append(sources, layer+"/dot-claude/settings.json")
	}
	for _, id := range stacks {
		sources = append(sources, kitStacks+"/"+id+"/settings.json")
	}
	added := false
	for _, source := range sources {
		data, err := kitFS.ReadFile(source)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return initChange{}, err
		}
		var kit map[string]any
		if err = json.Unmarshal(data, &kit); err != nil {
			return initChange{}, fmt.Errorf("%s: %w", source, err)
		}
		// The project turned the guard off: don't put its hook back.
		if env, _ := doc["env"].(map[string]any); env != nil {
			if value, _ := env[guardOffKey].(string); guardIsOff(value) {
				delete(kit, "hooks")
			}
		}
		added = mergeSettings(doc, kit) || added
	}
	if hasFile && !added {
		return initChange{path: path, status: "đã đúng"}, nil
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(doc); err != nil {
		return initChange{}, err
	}
	status := "tạo"
	if hasFile {
		status = "thêm quyền"
	}
	return initChange{path: path, text: buf.String(), write: true, status: status}, nil
}

// planLocalMemory keeps Claude Code's auto memory inside the project instead of
// under ~/.claude. The setting needs an absolute path, so it goes in the
// uncommitted settings.local.json; the folder is the main checkout's, so every
// worktree of the repo shares one memory. A value already there is the user's.
func planLocalMemory(root string, _ []string) (initChange, error) {
	path := projectSettingsPath(root)
	doc, err := readConfig(path)
	if err != nil {
		return initChange{}, fmt.Errorf("file %s bị lỗi cú pháp; tool không sửa nó", path)
	}
	if _, set := doc["autoMemoryDirectory"]; set {
		return initChange{path: path, status: "đã đúng"}, nil
	}
	_, statErr := os.Stat(path)
	doc["autoMemoryDirectory"] = filepath.Join(mainRepoRoot(root), ".claude", "memory")
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(doc); err != nil {
		return initChange{}, err
	}
	status := "tạo (memory của Claude Code nằm trong project)"
	if statErr == nil {
		status = "thêm autoMemoryDirectory (memory nằm trong project)"
	}
	return initChange{path: path, text: buf.String(), write: true, status: status}, nil
}

// mergeSettings reports whether it added anything. A rule already in any list
// stays where it is, so a stricter choice the user made is never loosened.
func mergeSettings(doc, kit map[string]any) bool {
	added := false
	for key, value := range kit {
		if key == "permissions" {
			continue
		}
		if _, exists := doc[key]; !exists {
			doc[key] = value
			added = true
			continue
		}
		// env: add the variables the user hasn't set; theirs keep their values.
		have, _ := doc[key].(map[string]any)
		want, _ := value.(map[string]any)
		if key == "hooks" && have != nil {
			added = mergeHooks(have, want) || added
			continue
		}
		if key != "env" || have == nil {
			continue
		}
		for name, setting := range want {
			if _, exists := have[name]; !exists {
				have[name] = setting
				added = true
			}
		}
	}
	source, _ := kit["permissions"].(map[string]any)
	if source == nil {
		return added
	}
	target, _ := doc["permissions"].(map[string]any)
	if target == nil {
		target = map[string]any{}
		doc["permissions"] = target
	}
	existing := map[string]bool{}
	for _, list := range []string{"allow", "ask", "deny"} {
		values, _ := target[list].([]any)
		for _, value := range values {
			if rule, ok := value.(string); ok {
				existing[rule] = true
			}
		}
	}
	for key, value := range source {
		rules, isList := value.([]any)
		if !isList {
			if _, exists := target[key]; !exists {
				target[key] = value
				added = true
			}
			continue
		}
		current, isList := target[key].([]any)
		if had, exists := target[key]; exists && !isList {
			if had != nil {
				continue // not a list: the user's value, leave it
			}
			// null fails Claude Code's settings check; older wj versions wrote it.
			current = []any{}
			added = true
		}
		for _, rule := range rules {
			name, _ := rule.(string)
			if !existing[name] {
				current = append(current, rule)
				existing[name] = true
				added = true
			}
		}
		// Every kit rule can already sit in another list: don't create the
		// list then, a nil one is written as null.
		if current != nil {
			target[key] = current
		}
	}
	return added
}

// mergeHooks adds the kit's hooks to the events the user already has hooks
// for. A kit hook whose command is already there, under any matcher, is left
// as the user set it up.
func mergeHooks(have, want map[string]any) bool {
	added := false
	for event, value := range want {
		entries, _ := value.([]any)
		current, isList := have[event].([]any)
		if _, exists := have[event]; exists && !isList {
			continue
		}
		present := map[string]bool{}
		for _, entry := range current {
			for _, command := range hookCommands(entry) {
				present[command] = true
			}
		}
		for _, entry := range entries {
			commands := hookCommands(entry)
			missing := len(commands) > 0
			for _, command := range commands {
				missing = missing && !present[command]
			}
			if missing {
				current = append(current, entry)
				added = true
			}
		}
		have[event] = current
	}
	return added
}

// hookCommands lists the commands of one matcher entry of a hook event.
func hookCommands(entry any) []string {
	group, _ := entry.(map[string]any)
	hooks, _ := group["hooks"].([]any)
	var commands []string
	for _, hook := range hooks {
		fields, _ := hook.(map[string]any)
		if command, ok := fields["command"].(string); ok {
			commands = append(commands, command)
		}
	}
	return commands
}

func planGitignore(root string, stacks []string) (initChange, error) {
	path := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return initChange{}, err
	}
	hasFile := err == nil
	present := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		present[strings.TrimSpace(line)] = true
	}
	// .claude/worktrees/ is where Claude Code puts each worker subagent's worktree.
	// Memory stays on this machine: Claude Code's own notes in .claude/memory/,
	// lessons about this machine in CLAUDE.local.md. Lessons about the project
	// are committed, in AGENTS.md.
	wanted := []string{".tmp/", ".claude/settings.local.json", ".claude/worktrees/", ".claude/memory/", "CLAUDE.local.md"}
	for _, id := range stacks {
		extra, err := kitFS.ReadFile(kitStacks + "/" + id + "/gitignore")
		if err != nil {
			return initChange{}, err
		}
		wanted = append(wanted, strings.Fields(string(extra))...)
	}
	var missing []string
	for _, line := range wanted {
		if !present[line] {
			present[line] = true
			missing = append(missing, line)
		}
	}
	if len(missing) == 0 {
		return initChange{path: path, status: "đã đúng"}, nil
	}
	text := addToWjBlock(string(data), missing)
	status := "tạo"
	if hasFile {
		status = fmt.Sprintf("thêm %d dòng", len(missing))
	}
	return initChange{path: path, text: text, write: true, status: status}, nil
}

func applyInit(root string, stacks []string, changes []initChange, dry, lite bool, out io.Writer) error {
	jobs := "chỉ rule chung"
	if len(stacks) > 0 {
		jobs = "rule chung + " + strings.Join(stacks, ", ")
	}
	if lite {
		jobs += ", bản lite"
	}
	if dry {
		fmt.Fprintf(out, "Xem trước, chưa ghi gì: %s (%s)\n", root, jobs)
	} else {
		fmt.Fprintf(out, "Setup project: %s (%s)\n", root, jobs)
	}
	for _, change := range changes {
		switch {
		case dry:
		case change.remove:
			if err := removeOldFile(change.path); err != nil {
				return err
			}
		case change.backup:
			if err := writeFile(change.path, change.text, false); err != nil {
				return err
			}
		case change.write:
			if err := os.MkdirAll(filepath.Dir(change.path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(change.path, []byte(change.text), 0o644); err != nil {
				return err
			}
		}
		rel, err := filepath.Rel(root, change.path)
		if err != nil {
			rel = change.path
		}
		fmt.Fprintf(out, "  %-62s %s\n", filepath.ToSlash(rel), change.status)
	}
	if lite {
		fmt.Fprintln(out, "\nBản lite: chạy được trên máy không có wj. Không có skill Orca, worker và hook wj guard;")
		fmt.Fprintln(out, "muốn thêm thì chạy lại wj init (không -lite). Mở lại Claude Code để nó nhận settings mới.")
		return nil
	}
	if !isGitRepo(root) {
		fmt.Fprintln(out, "\nProject chưa có git. Worker cần git: chạy git init rồi commit các file này.")
		return nil
	}
	if !dry {
		fmt.Fprintln(out, "\nCommit các file này trước khi chạy worker: worktree của worker chỉ thấy file đã commit.")
		fmt.Fprintln(out, "Mở lại Claude Code để nó nhận subagent orca-worker và settings mới.")
	}
	return nil
}

// removeOldFile deletes a file the kit no longer ships, and its folder once
// that is empty.
func removeOldFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("không xoá được %s: %w", path, err)
	}
	// Fails while other files remain there, which is what we want.
	_ = os.Remove(filepath.Dir(path))
	return nil
}

func isGitRepo(root string) bool {
	_, err := os.Stat(filepath.Join(root, ".git"))
	return err == nil
}
