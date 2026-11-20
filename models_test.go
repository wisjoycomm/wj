package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

const testKey = "test-key-do-not-print"

func fakeGateway(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, "bad key "+testKey)
			return
		}
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"data":[{"id":"cc/claude-opus-9"},{"id":"cx/gpt-9"}]}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(body, &req)
		switch {
		case strings.HasSuffix(r.URL.Path, "/messages") && strings.Contains(req.Model, "claude"):
			fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
		case strings.HasSuffix(r.URL.Path, "/responses") && strings.Contains(req.Model, "gpt"):
			fmt.Fprint(w, "event: response.created\ndata: {}\n\n")
		case strings.HasSuffix(r.URL.Path, "/responses"):
			fmt.Fprint(w, "event: response.created\ndata: {}\n\nevent: response.failed\ndata: {}\n\n")
		default:
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, "secret "+testKey)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func connectedHome(t *testing.T, base string) string {
	t.Helper()
	t.Setenv("CODEX_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	home := t.TempDir()
	settings, _ := json.Marshal(map[string]any{"model": "my-choice", "theme": "dark", "env": map[string]any{"ANTHROPIC_BASE_URL": base, "ANTHROPIC_AUTH_TOKEN": testKey}})
	writeTestFile(t, claudeSettingsPath(home), string(settings))
	if _, err := applyCodexModels(home, "gateway", base+"/v1", testKey, "cx/gpt-9", "gateway", nil, false); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestClaudePickerKeepsOnlyWhatDiscoveryDrops(t *testing.T) {
	rows := claudePickerRows([]string{"cc/claude-opus-9", "anthropic.x", "cx/GPT-9", "gemini-3"})
	if len(rows) != 2 || rows[0].(map[string]any)["model"] != "cx/GPT-9" {
		t.Errorf("rows = %v", rows)
	}
}

func TestApplyClaudeAddsNonClaudeRows(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	home := t.TempDir()
	p := connectionProfile{Auth: "gateway", BaseURL: "https://gw.example/v1", Credential: testKey, Header: "bearer", Models: []string{"cc/claude-opus-9", "cx/gpt-9"}}
	if _, err := applyClaude(home, p, false); err != nil {
		t.Fatal(err)
	}
	if text := readText(t, claudeSettingsPath(home)); !strings.Contains(text, "modelPicker") || !strings.Contains(text, "cx/gpt-9") {
		t.Errorf("settings:\n%s", text)
	}
	p.Models = []string{"cc/claude-opus-9"}
	if _, err := applyClaude(home, p, false); err != nil {
		t.Fatal(err)
	}
	if text := readText(t, claudeSettingsPath(home)); strings.Contains(text, "modelPicker") {
		t.Errorf("stale rows stayed:\n%s", text)
	}
}

func TestModelsRefreshKeepsChoicesAndHidesKey(t *testing.T) {
	server := fakeGateway(t)
	home := connectedHome(t, server.URL)
	var out bytes.Buffer
	if err := runModels(nil, strings.NewReader(""), &out, home); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), testKey) {
		t.Errorf("key printed:\n%s", out.String())
	}
	var settings struct {
		Model, Theme string
		ModelPicker  struct {
			Options []struct{ Model string }
		}
	}
	if err := json.Unmarshal([]byte(readText(t, claudeSettingsPath(home))), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.Model != "my-choice" || settings.Theme != "dark" || len(settings.ModelPicker.Options) != 1 || settings.ModelPicker.Options[0].Model != "cx/gpt-9" {
		t.Errorf("settings = %+v", settings)
	}
	doc, err := readConfig(codexConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if doc["model"] != "cx/gpt-9" || doc["model_catalog_json"] == nil {
		t.Errorf("codex = %v", doc["model"])
	}
	if text := readText(t, codexCatalogPath(home)); !strings.Contains(text, "cc/claude-opus-9") {
		t.Errorf("catalog lacks Claude id")
	}
}

// A gateway that adds a newer Claude: wj models moves the pinned families to it,
// and the default when it was the old pin; a hand-picked default stays.
func TestModelsRefreshMovesPinnedFamiliesToTheNewestClaude(t *testing.T) {
	server := fakeGateway(t)
	home := connectedHome(t, server.URL)
	settings, _ := json.Marshal(map[string]any{"model": "cc/claude-opus-8", "env": map[string]any{
		"ANTHROPIC_BASE_URL": server.URL, "ANTHROPIC_AUTH_TOKEN": testKey,
		"ANTHROPIC_DEFAULT_OPUS_MODEL": "cc/claude-opus-8", "ANTHROPIC_DEFAULT_SONNET_MODEL": "cc/claude-sonnet-8",
	}})
	writeTestFile(t, claudeSettingsPath(home), string(settings))
	var out bytes.Buffer
	if err := runModels(nil, strings.NewReader(""), &out, home); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Model string
		Env   map[string]string
	}
	if err := json.Unmarshal([]byte(readText(t, claudeSettingsPath(home))), &got); err != nil {
		t.Fatal(err)
	}
	if got.Env["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "cc/claude-opus-9" || got.Model != "cc/claude-opus-9" {
		t.Errorf("opus not moved to 9: %+v", got)
	}
	if got.Env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "cc/claude-sonnet-8" {
		t.Errorf("no newer sonnet listed, the pin stays: %+v", got.Env)
	}
	if !strings.Contains(out.String(), "cc/claude-opus-8 → cc/claude-opus-9") {
		t.Errorf("output must say what moved:\n%s", out.String())
	}
}

func TestUpgradeClaudeTiersKeepsAHandPickedDefaultAndANewerPin(t *testing.T) {
	old := verifyClaudeModels
	verifyClaudeModels = func(_ connectionProfile, ids []string) []string { return ids }
	t.Cleanup(func() { verifyClaudeModels = old })
	root := map[string]any{"model": "cx/gpt-9"}
	env := map[string]any{"ANTHROPIC_DEFAULT_HAIKU_MODEL": "cc/claude-haiku-5-5", "CLAUDE_CODE_SUBAGENT_MODEL": "cc/claude-haiku-4-5-20251001"}
	changes := upgradeClaudeTiers(root, env, connectionProfile{}, []string{"cc/claude-haiku-5-5", "cc/claude-haiku-4-5-20251001", "haiku-x"})
	if env["CLAUDE_CODE_SUBAGENT_MODEL"] != "cc/claude-haiku-5-5" || env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "cc/claude-haiku-5-5" {
		t.Errorf("env = %v", env)
	}
	if root["model"] != "cx/gpt-9" || len(changes) != 1 {
		t.Errorf("model = %v, changes = %v", root["model"], changes)
	}
}

func TestModelsRefreshKeepsOpenCodeDefault(t *testing.T) {
	server := fakeGateway(t)
	home := connectedHome(t, server.URL)
	if _, err := setupOpenCode(home, server.URL+"/v1", testKey, []string{"cx/gpt-9"}, "gateway", false); err != nil {
		t.Fatal(err)
	}
	if err := runModels(nil, strings.NewReader(""), io.Discard, home); err != nil {
		t.Fatal(err)
	}
	text := readText(t, filepath.Join(home, ".config", "opencode", "opencode.json"))
	if !strings.Contains(text, `"model": "gateway/cx/gpt-9"`) || !strings.Contains(text, "cc/claude-opus-9") {
		t.Errorf("opencode:\n%s", text)
	}
}

func TestAnyGatewayNamingIsFollowedVerbatim(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"acme/big-3"},{"id":"zeta-4"}]}`)
	}))
	defer server.Close()
	t.Setenv("CODEX_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("SETUP_GATEWAY_KEY", "")
	home := t.TempDir()
	args := []string{"-auth", "gateway", "-only", "claude,codex", "-base-url", server.URL, "-key", testKey}
	if err := runCLIWithHome(args, strings.NewReader(""), io.Discard, home); err != nil {
		t.Fatal(err)
	}
	settings := readText(t, claudeSettingsPath(home))
	for _, id := range []string{"acme/big-3", "zeta-4"} {
		if !strings.Contains(settings, `"model": "`+id+`"`) {
			t.Errorf("Claude picker lacks %s:\n%s", id, settings)
		}
		if !strings.Contains(readText(t, codexCatalogPath(home)), `"slug": "`+id+`"`) {
			t.Errorf("Codex catalog lacks %s", id)
		}
	}
	doc, err := readConfig(codexConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if doc["model"] != "acme/big-3" {
		t.Errorf("Codex default = %v", doc["model"])
	}
}

func TestModelsRefreshNeedsConnection(t *testing.T) {
	t.Setenv("CODEX_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := runModels(nil, strings.NewReader(""), io.Discard, t.TempDir()); err == nil {
		t.Error("expected an error")
	}
}

func TestModelsTestReportsVerdictsWithoutBodies(t *testing.T) {
	server := fakeGateway(t)
	home := connectedHome(t, server.URL)
	var out bytes.Buffer
	if err := runModels([]string{"test", "-yes"}, strings.NewReader(""), &out, home); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if strings.Contains(text, testKey) || strings.Contains(text, "secret") {
		t.Errorf("body leaked:\n%s", text)
	}
	for _, want := range []string{"HTTP 429", "lỗi trong stream", "OK"} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q:\n%s", want, text)
		}
	}
}

func TestModelsTestAsksFirst(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			calls++
		}
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	home := connectedHome(t, server.URL)
	var out bytes.Buffer
	if err := runModels([]string{"test", "-models", "a,b"}, strings.NewReader("n\n"), &out, home); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Errorf("sent %d requests after a no", calls)
	}
	if !strings.Contains(out.String(), "4 request") {
		t.Errorf("out = %s", out.String())
	}
}

func TestStreamVerdict(t *testing.T) {
	for body, want := range map[string]string{
		"event: message_start\ndata: {}\n":          "OK",
		"event: error\ndata: {}\n":                  "lỗi trong stream",
		"data: {\"type\":\"error\",\"error\":{}}\n": "lỗi trong stream",
		"<html>": "sai định dạng",
		"event: response.created\n\nevent: response.failed\ndata: {}\n": "lỗi trong stream",
	} {
		if got := streamVerdict(strings.NewReader(body)); got != want {
			t.Errorf("%q = %q, want %q", body, got, want)
		}
	}
}
