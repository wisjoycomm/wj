package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestCodexGatewayThenLoginPreservesOtherSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, "custom-codex"))
	path := codexConfigPath(home)
	original := "model = \"old\"\nmodel_provider = \"old\"\nforced_login_method = \"api\"\napproval_policy = \"on-request\"\n[projects.\"C:/repo\"]\ntrust_level = \"trusted\"\n[model_providers.old]\nname = \"old\"\n"
	if err := writeFile(path, original, false); err != nil {
		t.Fatal(err)
	}
	if _, err := applyCodex(home, "gateway", "https://example.com/v1", "fake-key", "example-model", "work", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	var doc map[string]any
	if err := toml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["model"] != "example-model" || doc["model_provider"] != "work" || doc["approval_policy"] != "on-request" || doc["projects"] == nil {
		t.Fatal("settings lost")
	}
	providers := doc["model_providers"].(map[string]any)
	p := providers["work"].(map[string]any)
	if p["experimental_bearer_token"] != "fake-key" || p["requires_openai_auth"] != false || providers["old"] == nil {
		t.Fatal("provider malformed")
	}
	if _, err := applyCodex(home, "login", "", "", "", "", false); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	doc = nil
	_ = toml.Unmarshal(b, &doc)
	if doc["model_provider"] != nil || doc["model"] != nil || doc["forced_login_method"] != nil || doc["projects"] == nil {
		t.Fatal("login retained routing override")
	}
	backups, _ := filepath.Glob(path + ".bak-*")
	if len(backups) != 2 {
		t.Fatal("missing backups")
	}
	_ = os.WriteFile(path, []byte("invalid = ["), 0o600)
	if _, err := applyCodex(home, "login", "", "", "", "", false); err == nil {
		t.Fatal("invalid TOML replaced")
	}
}

func TestOpenCodeCurrentSchemaAndJSONCMerge(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	dir := filepath.Join(home, "config", "opencode")
	_ = writeFile(filepath.Join(dir, "opencode.json"), `{"theme":"custom","provider":{"keep":{"name":"kept"}}}`, false)
	path := filepath.Join(dir, "opencode.jsonc")
	_ = writeFile(path, "{ // comment\n\"instructions\": [\"literal,} text\",],\"providers\":{\"work\":{\"package\":\"@opencode/ai/providers/openai-compatible\"}}}", false)
	if _, err := applyOpenCode(home, "https://example.com/v1", "fake-key", []string{"model-x"}, "work", "@ai-sdk/openai-compatible", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["theme"] != "custom" || doc["model"] != "work/model-x" || doc["providers"] != nil {
		t.Fatal("migration/merge failed")
	}
	if doc["instructions"].([]any)[0] != "literal,} text" {
		t.Fatal("JSONC string corrupted")
	}
	providers := doc["provider"].(map[string]any)
	work := providers["work"].(map[string]any)
	if providers["keep"] == nil || work["npm"] != "@ai-sdk/openai-compatible" || work["options"].(map[string]any)["apiKey"] != "fake-key" {
		t.Fatal("invalid provider")
	}
}

func TestClientCredentialRoutingAndDryRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("SETUP_GATEWAY_KEY", "")
	for _, id := range []string{"codex", "opencode"} {
		var out bytes.Buffer
		args := []string{"-auth", "gateway", "-base-url", "example.com", "-key-stdin", "-models", "model-x", "-dry-run"}
		if err := configureAPIClient(id, args, strings.NewReader("fake-key\n"), &out, home); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "fake-key") {
			t.Fatal("credential printed")
		}
		if err := configureAPIClient(id, args, strings.NewReader("sk-ant-oat-test\n"), &out, home); err == nil {
			t.Fatal("Claude OAuth accepted")
		}
		if err := configureAPIClient(id, []string{"-auth", "oauth", "-dry-run"}, strings.NewReader(""), &out, home); err == nil {
			t.Fatal("unsupported auth accepted")
		}
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 {
		t.Fatal("dry run wrote settings")
	}
	var out bytes.Buffer
	if err := configureAPIClient("codex", []string{"-auth", "login", "-no-login"}, strings.NewReader(""), &out, home); err != nil {
		t.Fatal(err)
	}
}

func TestWizardInputDoesNotConsumeNextMenuChoice(t *testing.T) {
	home := testHome(t)
	// Auth, token, scope (asked: tests run inside this git repo), profile name, install, next menu.
	in := strings.NewReader("2\nsk-ant-oat-test\n\n\nno\n8\n0\n")
	var out bytes.Buffer
	if err := runCLIWithHome(nil, in, &out, home); err != nil {
		t.Fatal(err)
	}
	answer := menuAnswer(in, &out, "Next")
	// When Claude is installed the missing-install question is not asked.
	if answer == "no" {
		answer = menuAnswer(in, &out, "Next")
	}
	if answer != "8" {
		t.Fatal("wizard swallowed menu input", answer)
	}
}
