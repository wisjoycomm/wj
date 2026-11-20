package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The file map (.claude/map.md) is one table row per folder: how many scripts and
// which kinds of assets it holds, and its biggest files. Agents read it to find
// where things are instead of searching the whole tree. wj writes the counts from
// git's file list; the Note column belongs to the user and /orca-dream and is kept
// when the map is written again.

const (
	projectMapFile  = ".claude/map.md"
	mapMaxRows      = 250
	mapMainFiles    = 3
	mapNoteMarker   = "| Folder |"
	mapScriptWeight = 20 // a script counts like this many assets when choosing which folders get their own row
)

var mapKinds = map[string]string{
	".cs": "cs", ".go": "go", ".ts": "ts", ".tsx": "tsx", ".js": "js", ".jsx": "jsx", ".py": "py", ".lua": "lua",
	".gd": "gd", ".cpp": "cpp", ".c": "c", ".h": "h", ".hpp": "h", ".shader": "shader", ".hlsl": "shader",
	".cginc": "shader", ".compute": "shader", ".java": "java", ".kt": "kt", ".swift": "swift", ".rs": "rs",
	".unity": "scene", ".prefab": "prefab", ".mat": "mat", ".asset": "asset", ".anim": "anim",
	".controller": "anim", ".overrideController": "anim", ".png": "img", ".jpg": "img", ".jpeg": "img",
	".psd": "img", ".tga": "img", ".tif": "img", ".exr": "img", ".webp": "img", ".svg": "img", ".gif": "img",
	".wav": "audio", ".mp3": "audio", ".ogg": "audio", ".aif": "audio", ".aiff": "audio",
	".fbx": "model", ".obj": "model", ".blend": "model", ".glb": "model", ".gltf": "model",
	".ttf": "font", ".otf": "font", ".fontsettings": "font", ".spriteatlas": "atlas", ".spriteatlasv2": "atlas",
	".mixer": "audio", ".playable": "timeline", ".shadergraph": "shader", ".vfx": "vfx", ".mp4": "video",
	".json": "data", ".csv": "data", ".xml": "data", ".yaml": "data", ".yml": "data", ".bytes": "data",
}

// mapScriptKinds are code; every other kind counts as an asset.
var mapScriptKinds = map[string]bool{"cs": true, "go": true, "ts": true, "tsx": true, "js": true, "jsx": true,
	"py": true, "lua": true, "gd": true, "cpp": true, "c": true, "h": true, "shader": true, "java": true,
	"kt": true, "swift": true, "rs": true}

type mapFolder struct {
	files   int
	kinds   map[string]int
	biggest []mapFile
}

type mapFile struct {
	name string
	size int64
}

// writeProjectMap writes .claude/map.md for the repo at root. It reports how many
// folders the map has and how many are new since the last one.
func writeProjectMap(root string, now time.Time) (folders, added int, err error) {
	list, err := gitOutput(root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return 0, 0, fmt.Errorf("git ls-files: %w", err)
	}
	var files []string
	for _, name := range strings.Split(list, "\x00") {
		if name == "" || strings.HasSuffix(name, ".meta") || strings.HasPrefix(name, ".claude/") || strings.HasPrefix(name, ".tmp/") {
			continue
		}
		files = append(files, name)
	}
	if len(files) == 0 {
		return 0, 0, errors.New("repo chưa có file nào")
	}
	path := filepath.Join(root, filepath.FromSlash(projectMapFile))
	old, _ := os.ReadFile(path)
	notes, seen := readMapNotes(string(old))
	byFolder := groupFiles(root, files, mapMaxRows)
	text := renderProjectMap(byFolder, notes, len(files), now)
	for folder := range byFolder {
		if len(seen) > 0 && !seen[folder] {
			added++
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, 0, err
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return 0, 0, err
	}
	return len(byFolder), added, nil
}

// groupFiles gives folders their own row where it helps most: it starts with the
// whole repo as one row and keeps splitting the heaviest row into its subfolders
// (scripts weigh more than assets) while the map stays within maxRows rows. A row
// that was split keeps only the files directly in it.
func groupFiles(root string, files []string, maxRows int) map[string]*mapFolder {
	weight := map[string]int{}
	children := map[string][]string{}
	hasDirect := map[string]bool{}
	for _, name := range files {
		w := 1
		if mapScriptKinds[mapKinds[strings.ToLower(path.Ext(name))]] {
			w = mapScriptWeight
		}
		dir := path.Dir(name)
		hasDirect[dir] = true
		for d := dir; ; d = path.Dir(d) {
			weight[d] += w
			if d == "." {
				break
			}
			parent := path.Dir(d)
			if !contains(children[parent], d) {
				children[parent] = append(children[parent], d)
			}
		}
	}
	split := map[string]bool{}
	rows := map[string]bool{".": true}
	for {
		best := ""
		for row := range rows {
			if !split[row] && len(children[row]) > 0 && (best == "" || weight[row] > weight[best]) {
				best = row
			}
		}
		if best == "" {
			break
		}
		grow := len(children[best])
		if !hasDirect[best] {
			grow-- // the split row itself goes away
		}
		if len(rows)+grow > maxRows {
			break
		}
		split[best] = true
		if !hasDirect[best] {
			delete(rows, best)
		}
		for _, child := range children[best] {
			rows[child] = true
		}
	}
	out := map[string]*mapFolder{}
	for _, name := range files {
		dir := path.Dir(name)
		owner := dir
		for !rows[owner] || (split[owner] && owner != dir) {
			owner = path.Dir(owner)
		}
		f := out[owner]
		if f == nil {
			f = &mapFolder{kinds: map[string]int{}}
			out[owner] = f
		}
		f.files++
		kind := mapKinds[strings.ToLower(path.Ext(name))]
		if kind != "" {
			f.kinds[kind]++
		}
		f.add(root, name, mapScriptKinds[kind])
	}
	return out
}

// add keeps the files a row names: its biggest scripts, or for a folder without
// scripts the first assets by name. Only scripts are measured, so a big asset tree
// costs no disk reads.
func (f *mapFolder) add(root, name string, script bool) {
	base := path.Base(name)
	for _, have := range f.biggest {
		if have.name == base {
			return
		}
	}
	var size int64 = -1
	if script {
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err == nil {
			size = info.Size()
		}
	}
	f.biggest = append(f.biggest, mapFile{base, size})
	sort.SliceStable(f.biggest, func(a, b int) bool {
		if f.biggest[a].size != f.biggest[b].size {
			return f.biggest[a].size > f.biggest[b].size
		}
		return f.biggest[a].name < f.biggest[b].name
	})
	if len(f.biggest) > mapMainFiles {
		f.biggest = f.biggest[:mapMainFiles]
	}
}

func renderProjectMap(byFolder map[string]*mapFolder, notes map[string]string, total int, now time.Time) string {
	folders := make([]string, 0, len(byFolder))
	scripts, assets := 0, 0
	for name, f := range byFolder {
		folders = append(folders, name)
		for kind, n := range f.kinds {
			if mapScriptKinds[kind] {
				scripts += n
			} else {
				assets += n
			}
		}
	}
	sort.Strings(folders)
	var b strings.Builder
	b.WriteString("# File map\n\n")
	b.WriteString("One row per folder: scripts, assets by kind, its main files (biggest scripts, else the first assets). Read this to find where something is before\n")
	b.WriteString("searching the tree. The counts are rewritten from git's file list; the Note column is kept, so write in it\n")
	b.WriteString("what a folder is for.\n\n")
	fmt.Fprintf(&b, "Updated %s · %d files · %d scripts · %d assets\n\n", now.Format("2006-01-02"), total, scripts, assets)
	b.WriteString("| Folder | Files | Scripts | Assets | Main files | Note |\n|---|---|---|---|---|---|\n")
	for _, name := range folders {
		f := byFolder[name]
		var code, other []string
		kinds := make([]string, 0, len(f.kinds))
		for kind := range f.kinds {
			kinds = append(kinds, kind)
		}
		sort.Slice(kinds, func(a, c int) bool {
			if f.kinds[kinds[a]] != f.kinds[kinds[c]] {
				return f.kinds[kinds[a]] > f.kinds[kinds[c]]
			}
			return kinds[a] < kinds[c]
		})
		for _, kind := range kinds {
			item := fmt.Sprintf("%d %s", f.kinds[kind], kind)
			if mapScriptKinds[kind] {
				code = append(code, item)
			} else {
				other = append(other, item)
			}
		}
		var big []string
		for _, file := range f.biggest {
			big = append(big, "`"+cutRunes(strings.ReplaceAll(file.name, "|", "/"), 40)+"`")
		}
		fmt.Fprintf(&b, "| `%s` | %d | %s | %s | %s | %s |\n", name, f.files, orDash(strings.Join(code, ", ")),
			orDash(strings.Join(other, ", ")), orDash(strings.Join(big, ", ")), orDash(notes[name]))
	}
	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// readMapNotes takes the Note of every row of an earlier map, and which folders it had.
func readMapNotes(text string) (map[string]string, map[string]bool) {
	notes, seen := map[string]string{}, map[string]bool{}
	inTable := false
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, mapNoteMarker) {
			inTable = true
			continue
		}
		if !inTable || !strings.HasPrefix(line, "| `") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) < 6 {
			continue
		}
		folder := strings.Trim(strings.TrimSpace(cells[0]), "`")
		seen[folder] = true
		if note := strings.TrimSpace(cells[len(cells)-1]); note != "" && note != "-" {
			notes[folder] = note
		}
	}
	return notes, seen
}

// printProjectMap writes the map and says so in one line, for wj digest and wj map.
func printProjectMap(out io.Writer, root string) {
	folders, added, err := writeProjectMap(root, time.Now())
	if err != nil {
		fmt.Fprintf(out, "MAP: không ghi được %s (%v)\n", projectMapFile, err)
		return
	}
	line := fmt.Sprintf("MAP (%s): %d folders", projectMapFile, folders)
	if added > 0 {
		line += fmt.Sprintf(", %d new since the last map", added)
	}
	empty := 0
	data, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(projectMapFile)))
	notes, _ := readMapNotes(string(data))
	empty = folders - len(notes)
	if empty > 0 {
		line += fmt.Sprintf(", %d without a Note", empty)
	}
	fmt.Fprintln(out, line)
}

// runMap is wj map: write the file map of the repo the command runs in.
func runMap(out io.Writer) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	root := filepath.Clean(mainRepoRoot(dir))
	if !isGitRepo(root) {
		return errors.New("không thấy repo git ở đây hay thư mục cha; cd vào project rồi chạy lại")
	}
	printProjectMap(out, root)
	return nil
}
