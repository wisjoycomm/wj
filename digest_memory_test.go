package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDigestListsMemoryAndFlagsStaleNotes(t *testing.T) {
	root := t.TempDir()
	memory := filepath.Join(root, ".claude", "memory")
	writeTestFile(t, filepath.Join(root, "src", "cart.go"), "package cart\n")
	writeTestFile(t, filepath.Join(memory, "MEMORY.md"),
		"- [Cart rounding](cart-rounding.md) — totals round half up\n- [Gone](gone.md) — removed\n")
	writeTestFile(t, filepath.Join(memory, "cart-rounding.md"),
		"---\nname: cart-rounding\ndescription: Cart totals round half up\nmetadata:\n  type: project\n---\n\n"+
			"Totals are rounded in `src/cart.go`. Run `go test ./...` and pass `-count=1`.\n")
	writeTestFile(t, filepath.Join(memory, "old-script.md"),
		"---\nname: old-script\ndescription: Verify with the old script\nmetadata:\n  type: feedback\n---\n\n"+
			"Use `scripts/fast_verify.ps1`, not `src/cart.go`.\n")
	old := time.Now().AddDate(0, 0, -30)
	if err := os.Chtimes(filepath.Join(memory, "old-script.md"), old, old); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "AGENTS.md"),
		"# Rules\n\n## Lessons\n- Build with `tools/build.sh`.\n- Tests live next to code, e.g. `src/cart.go`.\n\n## Other\n- not a lesson `nope/x.go`\n")

	var out bytes.Buffer
	printMemory(&out, root, root, time.Now())
	got := out.String()
	for _, want := range []string{
		"MEMORY (.claude/memory): 2 notes; index MEMORY.md 2 lines",
		"old-script.md", "feedback", "30d", "MISSING: scripts/fast_verify.ps1",
		"cart-rounding.md", "project", "Cart totals round half up",
		"INDEX links notes that don't exist: gone.md",
		"NOT IN INDEX (never recalled): old-script.md",
		"LESSONS (AGENTS.md): 2 lines",
		"AGENTS.md:4  MISSING: tools/build.sh",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %q:\n%s", want, got)
		}
	}
	// A path that exists, a command and a flag are not flagged; nor is text outside Lessons.
	for _, unwanted := range []string{"MISSING: src/cart.go", "go test", "-count", "nope/x.go"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("must not show %q:\n%s", unwanted, got)
		}
	}
	// The oldest note comes first: it is the likeliest to be stale.
	if strings.Index(got, "old-script.md") > strings.Index(got, "cart-rounding.md") {
		t.Errorf("oldest note must be listed first:\n%s", got)
	}
}

func TestDigestShowsMemoryOutsideTheCheckoutAsFullPath(t *testing.T) {
	root := t.TempDir()
	memory := filepath.Join(root, ".claude", "memory")
	writeTestFile(t, filepath.Join(memory, "a.md"), "plain note\n")
	worktree := t.TempDir()

	var out bytes.Buffer
	printMemory(&out, root, worktree, time.Now())
	if got := out.String(); !strings.Contains(got, "MEMORY ("+memory+"):") {
		t.Errorf("memory outside the checkout must be a full path:\n%s", got)
	}
}

func TestDigestMemoryFollowsTheConfiguredFolder(t *testing.T) {
	root := t.TempDir()
	elsewhere := filepath.Join(t.TempDir(), "notes")
	writeTestFile(t, filepath.Join(root, ".claude", "settings.local.json"),
		`{"autoMemoryDirectory":`+strings.ReplaceAll(`"`+elsewhere+`"`, `\`, `\\`)+`}`)
	writeTestFile(t, filepath.Join(elsewhere, "a.md"), "plain note, no frontmatter\n")

	var out bytes.Buffer
	printMemory(&out, root, root, time.Now())
	if got := out.String(); !strings.Contains(got, "1 notes") || !strings.Contains(got, "a.md") {
		t.Errorf("configured folder not read:\n%s", got)
	}

	out.Reset()
	empty := t.TempDir()
	printMemory(&out, empty, empty, time.Now())
	if got := out.String(); !strings.Contains(got, "no notes") || strings.Contains(got, "LESSONS") {
		t.Errorf("empty project:\n%s", got)
	}
}
