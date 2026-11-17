package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Skip("git not available:", string(out))
	}
	return root
}

func TestClaudeProjectScopeLeavesGlobalAlone(t *testing.T) {
	home := testHome(t)
	global := putSettings(t, home, `{"env":{"ANTHROPIC_AUTH_TOKEN":"global-gw","ANTHROPIC_BASE_URL":"https://gw.example","KEEP":"1"},"theme":"dark"}`)
	before, _ := os.ReadFile(global)
	root := gitRepo(t)
	sub := filepath.Join(root, "src")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	args := []string{"-auth", "oauth", "-key", "sk-ant-oat-project", "-scope", "project", "-dir", sub}
	if err := runCLIWithHome(args, strings.NewReader(""), &out, home); err != nil {
		t.Fatal(err, out.String())
	}
	if after, _ := os.ReadFile(global); !bytes.Equal(before, after) {
		t.Fatal("global settings changed")
	}
	data, err := os.ReadFile(filepath.Join(root, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct{ Env map[string]string }
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Env["CLAUDE_CODE_OAUTH_TOKEN"] != "sk-ant-oat-project" {
		t.Errorf("token not written: %v", doc.Env)
	}
	// The global gateway must not leak into the project through env merging.
	for _, key := range []string{"ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL"} {
		if v, ok := doc.Env[key]; !ok || v != "" {
			t.Errorf("%s = %q (present %v), want blanked", key, v, ok)
		}
	}
	if _, ok := doc.Env["KEEP"]; ok {
		t.Error("non-routing global key copied into project")
	}
	ignore, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if !strings.Contains(string(ignore), ".claude/settings.local.json") {
		t.Errorf(".gitignore = %q", ignore)
	}
}

func TestScopeRejectsWhatCannotBeProjectOnly(t *testing.T) {
	home := testHome(t)
	root := gitRepo(t)
	var out bytes.Buffer
	cases := [][]string{
		{"-auth", "gateway", "-base-url", "gw.example", "-key", "k", "-only", "claude,codex", "-scope", "project", "-dir", root, "-skip-check", "-models", "cc/claude-opus-9"},
		{"-auth", "oauth", "-key", "sk-ant-oat-x", "-scope", "everywhere"},
		{"-auth", "oauth", "-key", "sk-ant-oat-x", "-dir", root},
	}
	for _, args := range cases {
		if err := runCLIWithHome(append(args, "-dry-run"), strings.NewReader(""), &out, home); err == nil {
			t.Errorf("%v: no error", args)
		}
	}
	if err := configureAPIClient("codex", []string{"-auth", "gateway", "-base-url", "gw.example", "-scope", "project", "-dir", root, "-dry-run"}, strings.NewReader("k\n"), &out, home); err == nil {
		t.Error("codex accepted project scope")
	}
}

func TestOpenCodeProjectScope(t *testing.T) {
	home := testHome(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "cfg"))
	root := gitRepo(t)
	var out bytes.Buffer
	args := []string{"-auth", "gateway", "-base-url", "gw.example", "-key-stdin", "-skip-check", "-models", "cx/gpt-9", "-scope", "project", "-dir", root}
	if err := configureAPIClient("opencode", args, strings.NewReader("proj-key\n"), &out, home); err != nil {
		t.Fatal(err, out.String())
	}
	if _, err := os.Stat(filepath.Join(home, "cfg", "opencode")); err == nil {
		t.Error("user OpenCode config was written")
	}
	data, err := os.ReadFile(filepath.Join(root, "opencode.json"))
	if err != nil || !strings.Contains(string(data), "proj-key") {
		t.Fatalf("project config: %s %v", data, err)
	}
	ignore, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if !strings.Contains(string(ignore), "opencode.json") {
		t.Errorf(".gitignore = %q", ignore)
	}
}

func TestOpenCodeProjectScopeRefusesTrackedConfig(t *testing.T) {
	root := gitRepo(t)
	if err := os.WriteFile(filepath.Join(root, "opencode.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	add := exec.Command("git", "-C", root, "add", "opencode.json")
	if out, err := add.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	if _, err := applyOpenCodeProject(root, "https://gw/v1", "k", []string{"m"}, "gateway", "@ai-sdk/openai-compatible", false); err == nil {
		t.Fatal("wrote a key into a tracked file")
	}
}

func TestEnsureGitIgnoredEntryHonoursFolders(t *testing.T) {
	root := gitRepo(t)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("/.claude/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if changed, err := ensureGitIgnoredEntry(root, ".claude/settings.local.json"); err != nil || changed != "" {
		t.Fatalf("changed %q, err %v", changed, err)
	}
	if changed, err := ensureGitIgnoredEntry(root, "opencode.json"); err != nil || changed == "" {
		t.Fatalf("opencode.json not added: %q %v", changed, err)
	}
}
