package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// guardProject is a project with a git root, deny rules and a few files.
func guardProject(t *testing.T) string {
	t.Helper()
	t.Setenv("WJ_GUARD_MAX_LINES", "")
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeTestFile(t, filepath.Join(root, ".claude", "settings.json"), `{"permissions":{"deny":[
		"Read(**/node_modules/**)", "Read(/Library/**)", "Read(**/.env)", "Read(**/package-lock.json)", "Bash(git clean *)"]}}`)
	writeTestFile(t, filepath.Join(root, "small.go"), strings.Repeat("line\n", 40))
	writeTestFile(t, filepath.Join(root, "big.go"), strings.Repeat("line\n", 900))
	writeTestFile(t, filepath.Join(root, "apps", "web", "node_modules", "x", "index.js"), "x\n")
	writeTestFile(t, filepath.Join(root, "apps", "web", ".env"), "KEY=1\n")
	writeTestFile(t, filepath.Join(root, "apps", "web", ".env.example"), "KEY=\n")
	writeTestFile(t, filepath.Join(root, "Library", "cache.txt"), "x\n")
	writeTestFile(t, filepath.Join(root, "src", "Library", "ok.txt"), "x\n")
	writeTestFile(t, filepath.Join(root, "logo.png"), "\x89PNG\x00"+strings.Repeat("\n", 900))
	return root
}

// guardReply runs the hook and returns the deny reason, or "" when it allows.
func guardReply(t *testing.T, root, tool string, input map[string]any) string {
	t.Helper()
	call, err := json.Marshal(map[string]any{"tool_name": tool, "cwd": root, "tool_input": input})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runGuard(nil, bytes.NewReader(call), &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		return ""
	}
	var reply struct {
		Output struct {
			Event    string `json:"hookEventName"`
			Decision string `json:"permissionDecision"`
			Reason   string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out.Bytes(), &reply); err != nil {
		t.Fatalf("reply is not JSON: %s", out.String())
	}
	if reply.Output.Event != "PreToolUse" || reply.Output.Decision != "deny" || reply.Output.Reason == "" {
		t.Fatalf("reply = %s", out.String())
	}
	return reply.Output.Reason
}

func TestGuardRead(t *testing.T) {
	root := guardProject(t)
	cases := []struct {
		name  string
		input map[string]any
		deny  string // text the reason must hold; "" = allowed
	}{
		{"small file", map[string]any{"file_path": filepath.Join(root, "small.go")}, ""},
		{"big file, whole", map[string]any{"file_path": filepath.Join(root, "big.go")}, "900 lines"},
		{"big file, a range", map[string]any{"file_path": filepath.Join(root, "big.go"), "offset": 300, "limit": 200}, ""},
		{"big file, limit too large", map[string]any{"file_path": filepath.Join(root, "big.go"), "limit": 800}, "would load 800"},
		{"big file, the short end", map[string]any{"file_path": filepath.Join(root, "big.go"), "offset": 700}, ""},
		{"relative path", map[string]any{"file_path": "big.go"}, "big.go has 900 lines"},
		{"image", map[string]any{"file_path": filepath.Join(root, "logo.png")}, ""},
		{"missing file", map[string]any{"file_path": filepath.Join(root, "nope.go")}, ""},
		{"denied folder at depth", map[string]any{"file_path": filepath.Join(root, "apps", "web", "node_modules", "x", "index.js")}, "Read(**/node_modules/**)"},
		{"denied at root only", map[string]any{"file_path": filepath.Join(root, "Library", "cache.txt")}, "Read(/Library/**)"},
		{"same name deeper is fine", map[string]any{"file_path": filepath.Join(root, "src", "Library", "ok.txt")}, ""},
		{"env file", map[string]any{"file_path": filepath.Join(root, "apps", "web", ".env")}, "Read(**/.env)"},
		{"env example", map[string]any{"file_path": filepath.Join(root, "apps", "web", ".env.example")}, ""},
	}
	for _, c := range cases {
		reason := guardReply(t, root, "Read", c.input)
		if (c.deny == "") != (reason == "") || !strings.Contains(reason, c.deny) {
			t.Errorf("%s: reason = %q, want %q", c.name, reason, c.deny)
		}
	}
}

func TestGuardCommand(t *testing.T) {
	root := guardProject(t)
	cases := []struct {
		command string
		deny    string
	}{
		{"go test ./...", ""},
		{"cat small.go", ""},
		{"cat big.go", "all 900 lines"},
		{"cat big.go 2>&1", "all 900 lines"},
		{"cat big.go 2>/dev/null", "all 900 lines"},
		{"cd src && cat ../big.go", ""}, // the folder change is not followed: allowed, not guessed
		{"git status && cat big.go", "all 900 lines"},
		{"cat big.go | grep -n func", ""},
		{"cat big.go > .tmp/copy.go", ""},
		{"cat big.go | head -n 20", ""},
		{"head -n 20 big.go", ""},
		{"Get-Content big.go", "all 900 lines"},
		{"Get-Content big.go -TotalCount 40", ""},
		{"Get-Content big.go | Select-Object -First 40", ""},
		{"cat apps/web/.env", "Read(**/.env)"},
		{"cat 'apps/web/.env'", "Read(**/.env)"},
		{"grep -rn TODO apps/web/node_modules", "Read(**/node_modules/**)"},
		{"rg -l foo apps/web/node_modules/x", "Read(**/node_modules/**)"},
		{"grep node_modules small.go", ""}, // the pattern is not a path
		{"grep -e node_modules small.go", ""},
		{"head -n 5 Library/cache.txt", "Read(/Library/**)"},
		{"echo cat big.go", ""},
		{"git grep -n foo -- big.go", ""},
		{"python - <<'EOF'\ncat big.go\nEOF", ""},
		{"ls apps/web/node_modules", ""},
	}
	for _, tool := range []string{"Bash", "PowerShell"} {
		for _, c := range cases {
			reason := guardReply(t, root, tool, map[string]any{"command": c.command})
			if (c.deny == "") != (reason == "") || !strings.Contains(reason, c.deny) {
				t.Errorf("%s %q: reason = %q, want %q", tool, c.command, reason, c.deny)
			}
		}
	}
}

func TestGuardMaxLinesSetting(t *testing.T) {
	root := guardProject(t)
	big := map[string]any{"file_path": filepath.Join(root, "big.go")}
	t.Setenv("WJ_GUARD_MAX_LINES", "1000")
	if reason := guardReply(t, root, "Read", big); reason != "" {
		t.Errorf("limit 1000 still denies: %s", reason)
	}
	t.Setenv("WJ_GUARD_MAX_LINES", "0")
	if reason := guardReply(t, root, "Read", big); reason != "" {
		t.Errorf("0 must turn the size check off: %s", reason)
	}
	if reason := guardReply(t, root, "Bash", map[string]any{"command": "cat apps/web/.env"}); reason == "" {
		t.Error("deny rules must still hold with the size check off")
	}
	// The project's own env block sets it too.
	t.Setenv("WJ_GUARD_MAX_LINES", "")
	writeTestFile(t, filepath.Join(root, ".claude", "settings.local.json"), `{"env":{"WJ_GUARD_MAX_LINES":"2000"}}`)
	if reason := guardReply(t, root, "Read", big); reason != "" {
		t.Errorf("settings env ignored: %s", reason)
	}
}

// A hook must never break the session: anything it can't read is allowed.
func TestGuardAllowsWhatItCannotRead(t *testing.T) {
	for _, input := range []string{"", "not json", `{"tool_name":"Read"}`, `{"tool_name":"Edit","tool_input":{"file_path":"x"}}`,
		`{"tool_name":"Bash","tool_input":{"command":"cat 'unclosed"}}`} {
		var out bytes.Buffer
		if err := runGuard(nil, strings.NewReader(input), &out); err != nil || out.Len() != 0 {
			t.Errorf("%q: err = %v, out = %q", input, err, out.String())
		}
	}
}

func TestShellPipelines(t *testing.T) {
	got := shellPipelines(`A=1 cat "my file.txt" 2>&1 | grep -n 'a b' >> out.txt && ls C:\dir; echo done`)
	want := [][][]string{
		{{"A=1", "cat", "my file.txt", "2>", "1"}, {"grep", "-n", "a b", ">", "out.txt"}},
		{{"ls", `C:\dir`}},
		{{"echo", "done"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// WJ_GUARD=off: the hook allows everything, and init does not add the hook back.
func TestGuardCanBeTurnedOffForAProject(t *testing.T) {
	root := guardProject(t)
	t.Setenv("WJ_GUARD", "")
	big := map[string]any{"file_path": filepath.Join(root, "big.go")}
	writeTestFile(t, filepath.Join(root, ".claude", "settings.local.json"), `{"env":{"WJ_GUARD":"off"}}`)
	if reason := guardReply(t, root, "Read", big); reason != "" {
		t.Errorf("guard is off but denied: %s", reason)
	}
	if reason := guardReply(t, root, "Bash", map[string]any{"command": "cat apps/web/.env"}); reason != "" {
		t.Errorf("guard is off but denied: %s", reason)
	}

	project := t.TempDir()
	path := filepath.Join(project, ".claude", "settings.json")
	writeTestFile(t, path, `{"env":{"WJ_GUARD":"off"}}`)
	runInitIn(t, project, "-type", "go")
	settings := readText(t, path)
	if strings.Contains(settings, "wj guard") || !strings.Contains(settings, "Read(**/go.sum)") {
		t.Errorf("init must leave the hook out and still add the rest:\n%s", settings)
	}
	if f := checkKit(doctorEnv{home: t.TempDir(), cwd: project}); len(f) != 0 {
		t.Errorf("doctor must not ask for the hook back: %s", f[0].Detail)
	}
}
