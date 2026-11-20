package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// A kit file that the project already has in another form is the user's: wj init
// never overwrites it. When an agent is at hand, wj asks it to merge the kit's
// version into the user's file instead, keeping everything the user wrote, and
// keeps the old file as .bak. Without an agent the file stays as it is.

// initMerger merges kit into mine for the project file rel. nil means no merging
// (a dry run, -no-merge, tests).
var initMerger func(rel, mine, kit string) (string, error)

// initMergePreview is set by a dry run: the plan says a merge would happen.
var initMergePreview bool

// newInitMerger makes the merger of a real run; tests replace it.
var newInitMerger = agentMerger

// mergedKits remembers, per project file, the kit version that was last merged into
// it (.claude/wj-merged.json, committed). A rerun with the same kit asks no agent.
type mergedKits struct {
	path  string
	kits  map[string]string // project file -> sha256 of the kit text merged into it
	dirty bool
}

var initMergeState *mergedKits

func loadMergedKits(root string) *mergedKits {
	m := &mergedKits{path: filepath.Join(root, ".claude", "wj-merged.json"), kits: map[string]string{}}
	if data, err := os.ReadFile(m.path); err == nil {
		_ = json.Unmarshal(data, &m.kits)
	}
	return m
}

func kitHash(kit string) string {
	sum := sha256.Sum256(normalizeEOL([]byte(kit)))
	return hex.EncodeToString(sum[:8])
}

// change is the write of the state file, when a merge happened in this run.
func (m *mergedKits) change() (initChange, bool) {
	if m == nil || !m.dirty {
		return initChange{}, false
	}
	data, _ := json.MarshalIndent(m.kits, "", "  ")
	return initChange{path: m.path, text: string(data) + "\n", write: true, status: "ghi bản kit đã gộp vào từng file"}, true
}

// initNoMerge are kit files the user owns outright: a merge would make no sense.
var initNoMerge = []string{"PLAN.md", "PLAN-ARCHIVE.md", ".claude/models.md"}

// agentMerger merges with Claude Code in print mode (claude -p), and says on out
// which file it is working on.
func agentMerger(out io.Writer) func(rel, mine, kit string) (string, error) {
	return func(rel, mine, kit string) (string, error) {
		claude, err := exec.LookPath("claude")
		if err != nil {
			return "", errors.New("không thấy lệnh claude để gộp")
		}
		fmt.Fprintf(out, "Đang nhờ Claude Code gộp bản kit vào %s (có thể mất một phút)...\n", rel)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, claude, "-p", mergeInstruction)
		cmd.Stdin = strings.NewReader(mergeInput(rel, mine, kit))
		// Run outside the project, so its rules and memory don't steer the merge.
		cmd.Dir = os.TempDir()
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("claude -p: %w %s", err, strings.TrimSpace(cutRunes(stderr.String(), 200)))
		}
		return mergedFile(stdout.String(), mine)
	}
}

const mergeInstruction = "Merge two versions of one file, given on stdin. Reply with only the merged file between " +
	"<merged> and </merged>, nothing else, and use no tools."

func mergeInput(rel, mine, kit string) string {
	return "File: " + rel + "\n\n" +
		"Rules:\n" +
		"- MINE is the project's file. Keep everything in it: every line the project wrote stays, in its place.\n" +
		"- KIT is a newer template of the same file. Add what KIT has that MINE lacks (new sections, steps, rules, " +
		"front-matter keys), where it fits in MINE's structure.\n" +
		"- Where the two say different things about the same point, keep MINE's.\n" +
		"- Don't repeat a point MINE already makes in other words. Keep MINE's language and style.\n\n" +
		"<mine>\n" + mine + "\n</mine>\n\n<kit>\n" + kit + "\n</kit>\n"
}

// mergedFile takes the file out of the agent's answer and refuses one that looks
// like it lost the user's text.
func mergedFile(answer, mine string) (string, error) {
	start := strings.Index(answer, "<merged>")
	end := strings.LastIndex(answer, "</merged>")
	if start < 0 || end < start {
		return "", errors.New("agent không trả về file đã gộp")
	}
	merged := strings.Trim(answer[start+len("<merged>"):end], "\r\n") + "\n"
	if strings.TrimSpace(merged) == "" {
		return "", errors.New("agent trả về file rỗng")
	}
	if len(merged) < len(strings.TrimSpace(mine))*3/4 {
		return "", errors.New("bản gộp ngắn hơn hẳn file của bạn, có thể đã mất nội dung")
	}
	return merged, nil
}

// mergeHint marks a file whose kit version was not merged yet; wj doctor looks for it.
const mergeHint = " - wj init sẽ nhờ Claude Code gộp bản kit vào"

// mergeChange turns a kit file the project has in another form into a merge, if
// an agent is at hand. dry only says what would happen.
func mergeChange(change initChange, rel, kit string, dry bool) initChange {
	if contains(initNoMerge, rel) {
		return change
	}
	hash := kitHash(kit)
	if initMergeState != nil && initMergeState.kits[rel] == hash {
		change.status = "giữ nguyên (đã gộp bản kit này)"
		return change
	}
	if dry {
		change.status += " - lúc chạy thật: nhờ Claude Code gộp bản kit vào (nếu có lệnh claude)"
		return change
	}
	if initMerger == nil {
		change.status += mergeHint
		return change
	}
	mine, err := os.ReadFile(change.path)
	if err != nil {
		return change
	}
	merged, err := initMerger(rel, string(normalizeEOL(mine)), string(normalizeEOL([]byte(kit))))
	if err != nil {
		change.status += " - chưa gộp: " + err.Error()
		return change
	}
	if initMergeState != nil {
		initMergeState.kits[rel], initMergeState.dirty = hash, true
	}
	if merged == string(normalizeEOL(mine)) {
		change.status = "giữ nguyên (đã có đủ phần của bản kit)"
		return change
	}
	return initChange{path: change.path, text: merged, write: true, backup: true, status: "gộp bản kit vào file của bạn bằng Claude Code (bản cũ: .bak)"}
}
