package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSummarizeUsageGroupsAndFilters(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour).Format(time.RFC3339)
	recent := now.Add(-time.Hour).Format(time.RFC3339)
	log := strings.Join([]string{
		`{"ts":"` + recent + `","client":"claude","model":"cc/claude-opus-5-5","status":200,"input":100,"output":50,"cache_read":1000,"cache_write":10}`,
		`{"ts":"` + recent + `","client":"claude","model":"cc/claude-opus-5-5","status":200,"input":1,"output":2,"cache_read":3,"cache_write":4}`,
		`{"ts":"` + recent + `","client":"codex","model":"cx/gpt-6.1-sol","status":429,"input":0,"output":0}`,
		`{"ts":"` + old + `","client":"codex","model":"cx/gpt-6.1-sol","status":200,"input":999,"output":999}`,
		`not json`,
	}, "\n")
	list, total, err := summarizeUsage(strings.NewReader(log), now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if total.Calls != 3 || total.Failed != 1 || total.Input != 101 || total.CacheRead != 1003 || total.Total != 1170 {
		t.Fatalf("bad total %+v", total)
	}
	if len(list) != 2 || list[0].Client != "claude" || list[0].Calls != 2 {
		t.Fatalf("bad groups %+v", list)
	}
	_, all, _ := summarizeUsage(strings.NewReader(log), time.Time{})
	if all.Calls != 4 {
		t.Fatal("-all ignored old records")
	}
	if groupDigits(1234567) != "1,234,567" || groupDigits(12) != "12" {
		t.Fatal("digit grouping")
	}
}

func TestProxyEnvRoutesOnlyTheChild(t *testing.T) {
	env := proxyEnv([]string{"PATH=/bin", "https_proxy=http://old", "NODE_EXTRA_CA_CERTS=/old.pem", "KEEP=1"}, "http://127.0.0.1:8877", "/ca.pem")
	joined := "\n" + strings.Join(env, "\n") + "\n"
	for _, want := range []string{"\nPATH=/bin\n", "\nKEEP=1\n", "\nHTTPS_PROXY=http://127.0.0.1:8877\n", "\nNODE_EXTRA_CA_CERTS=/ca.pem\n", "\nCODEX_CA_CERTIFICATE=/ca.pem\n", "\nNO_PROXY=localhost,127.0.0.1,::1\n"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %v", want, env)
		}
	}
	if strings.Contains(joined, "http://old") || strings.Contains(joined, "/old.pem") {
		t.Fatal("stale proxy settings kept")
	}
}

func TestProxyUsageWithoutLogAndDryRuns(t *testing.T) {
	home := t.TempDir()
	var out bytes.Buffer
	if err := proxyUsage(nil, &out, home); err != nil || !strings.Contains(out.String(), "Chưa có dữ liệu") {
		t.Fatal(err, out.String())
	}
	if err := proxyStart([]string{"-dry-run", "-port", "9001"}, strings.NewReader(""), &out, home); err != nil {
		t.Fatal(err)
	}
	if err := proxyRun([]string{"-dry-run", "-proxy", "http://10.0.0.5:8080", "codex"}, strings.NewReader(""), &out, home); err != nil {
		t.Fatal(err)
	}
	if err := proxyRun([]string{"-proxy", "socks5://x", "codex"}, strings.NewReader(""), &out, home); err == nil {
		t.Fatal("unsupported proxy URL accepted")
	}
	if _, err := os.Stat(filepath.Join(home, ".wj")); !os.IsNotExist(err) {
		t.Fatal("dry run wrote files")
	}
	if !strings.Contains(out.String(), "9001") || !strings.Contains(out.String(), "10.0.0.5:8080") {
		t.Fatal(out.String())
	}
}

func TestEnsureMitmAutoInstallNeedsConsent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PATH", "")
	t.Setenv("ProgramFiles", home)
	t.Setenv("LOCALAPPDATA", home)
	var calls [][]string
	previous := installApps
	installApps = func(args []string, _ io.Reader, _ io.Writer) error {
		calls = append(calls, args)
		return nil
	}
	t.Cleanup(func() { installApps = previous })
	var out bytes.Buffer
	// Scripts (non-terminal input) without -yes must not install anything.
	if _, err := ensureMitm(home, "mitmdump", false, strings.NewReader("y\n"), &out); err == nil || len(calls) != 0 {
		t.Fatal("installed without consent", err, calls)
	}
	// -yes installs through the normal app manager path.
	if _, err := ensureMitm(home, "mitmdump", true, strings.NewReader(""), &out); err == nil {
		t.Fatal("stub install cannot produce a binary; expected the new-terminal hint")
	}
	if len(calls) != 1 || strings.Join(calls[0], " ") != "install mitmproxy" {
		t.Fatal("wrong install call", calls)
	}
	// Proxy run with -yes also reaches the installer when nothing listens on the port.
	self, _ := os.Executable()
	if err := proxyRun([]string{"-yes", "-port", "1", self}, strings.NewReader(""), &out, home); err == nil || len(calls) != 2 {
		t.Fatal("one-command mode did not auto-install", err, calls)
	}
}

func TestHelpTopicsAndAddonEmbedded(t *testing.T) {
	for _, topic := range []string{"", "claude", "codex", "proxy", "apps", "tools", "words"} {
		var out bytes.Buffer
		args := []string{"help"}
		if topic != "" {
			args = append(args, topic)
		}
		if err := runApp(args, strings.NewReader(""), &out); err != nil || out.Len() < 200 {
			t.Fatalf("help %q: %v", topic, err)
		}
	}
	if err := runApp([]string{"help", "nope"}, strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Fatal("unknown help topic accepted")
	}
	if !bytes.Contains(proxyAddon, []byte("class UsageMeter")) {
		t.Fatal("mitmproxy addon not embedded")
	}
	// The usage log must never carry request content or credentials.
	for _, forbidden := range []string{"headers.items", "authorization\"]", "get_content()"} {
		if bytes.Contains(proxyAddon, []byte(forbidden)) {
			t.Fatalf("addon may record sensitive data: %s", forbidden)
		}
	}
}

func TestNumericChoices(t *testing.T) {
	names := []string{"claude", "codex", "opencode", "cursor"}
	for answer, want := range map[string]string{
		"1":          "claude",
		"1 3":        "claude,opencode",
		"2,4":        "codex,cursor",
		"2-4":        "codex,opencode,cursor",
		"all":        "claude,codex,opencode,cursor",
		"3 3 1":      "opencode,claude",
		"codex 1":    "codex,claude",
		"  CLAUDE  ": "claude",
	} {
		got, err := parseChoices(answer, names)
		if err != nil || strings.Join(got, ",") != want {
			t.Fatalf("%q -> %v %v, want %s", answer, got, err, want)
		}
	}
	for _, bad := range []string{"0", "5", "3-9", "4-2", "nope"} {
		if _, err := parseChoices(bad, names); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	var out bytes.Buffer
	if got, _ := pickOne(strings.NewReader("\n"), &out, "", names, nil, 2); got != "codex" {
		t.Fatal("Enter did not take the default", got)
	}
	if got, _ := pickOne(strings.NewReader("\n"), &out, "", names, nil, 0); got != "" {
		t.Fatal("Enter without default must go back", got)
	}
	if _, err := pickOne(strings.NewReader("1 2\n"), &out, "", names, nil, 0); err == nil {
		t.Fatal("single choice accepted two")
	}
	if !strings.Contains(out.String(), " 2. codex") {
		t.Fatal("options not numbered", out.String())
	}
}

func TestAppMenuTakesNumbers(t *testing.T) {
	ids, err := parseChoices("1 2 9", appIDs())
	if err != nil || strings.Join(ids, ",") != "claude,codex,mitmproxy" {
		t.Fatal(ids, err)
	}
}

func TestUsageByProjectAndFilters(t *testing.T) {
	ts := time.Now().UTC().Format(time.RFC3339)
	log := strings.Join([]string{
		`{"ts":"` + ts + `","project":"alpha","client":"claude","model":"m1","status":200,"input":10,"output":1}`,
		`{"ts":"` + ts + `","project":"alpha","client":"codex","model":"m2","status":200,"input":20,"output":2}`,
		`{"ts":"` + ts + `","project":"beta","client":"claude","model":"m1","status":200,"input":30,"output":3}`,
		`{"ts":"` + ts + `","client":"claude","model":"m1","status":200,"input":40,"output":4}`,
	}, "\n")
	list, total, _ := summarizeUsageBy(strings.NewReader(log), usageFilter{GroupBy: []string{"project"}})
	if len(list) != 3 || total.Input != 100 || list[0].Project != "(không rõ)" || list[0].Total != 44 {
		t.Fatalf("by project %+v", list)
	}
	list, total, _ = summarizeUsageBy(strings.NewReader(log), usageFilter{Project: "ALPHA", GroupBy: []string{"client"}})
	if len(list) != 2 || total.Input != 30 {
		t.Fatalf("project filter %+v", list)
	}
	_, total, _ = summarizeUsageBy(strings.NewReader(log), usageFilter{Client: "claude", GroupBy: []string{"model"}})
	if total.Calls != 3 {
		t.Fatal("app filter", total)
	}
	var out bytes.Buffer
	home := t.TempDir()
	path := proxyLogPath(home)
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, []byte(log), 0o600)
	if err := proxyUsage([]string{"-by", "project-app"}, &out, home); err != nil || !strings.Contains(out.String(), "PROJECT") || !strings.Contains(out.String(), "beta") {
		t.Fatal(err, out.String())
	}
	if err := proxyUsage([]string{"-by", "nope"}, &out, home); err == nil {
		t.Fatal("bad -by accepted")
	}
	if !bytes.Contains(proxyAddon, []byte("def http_connect")) || !bytes.Contains(proxyAddon, []byte(`"project"`)) {
		t.Fatal("addon does not tag projects")
	}
	// Gateways that rename tools upstream break Claude Code tool search unless
	// tool_reference names get the same suffix (see .docs/model.md).
	for _, want := range []string{`"tool_ref_suffix", str, "_ide"`, "not found in available tools", "def _suffix_refs"} {
		if !bytes.Contains(proxyAddon, []byte(want)) {
			t.Fatalf("addon lost tool_reference repair: %s", want)
		}
	}
}

func TestProxyRunTagsProject(t *testing.T) {
	var out bytes.Buffer
	if err := proxyRun([]string{"-dry-run", "-project", "alpha", "codex"}, strings.NewReader(""), &out, t.TempDir()); err != nil || !strings.Contains(out.String(), "Project: alpha") {
		t.Fatal(err, out.String())
	}
	if currentProject() != "remora" && currentProject() == "" {
		t.Fatal("empty default project")
	}
}

func TestCodexGatewayCatalogListsEveryModel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex"))
	ids := []string{"cc/claude-opus-5-5", "cx/gpt-6.1-sol", "ds/deepseek-v4-pro", "cx/gpt-5.6-luna"}
	if _, err := applyCodexModels(home, "gateway", "https://example.com/v1", "fake-key", "cx/gpt-6.1-sol", "gw", ids, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(codexCatalogPath(home))
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil || len(catalog.Models) != 4 {
		t.Fatal(err, len(catalog.Models))
	}
	if catalog.Models[0]["slug"] != "cx/gpt-6.1-sol" || catalog.Models[1]["slug"] != "cx/gpt-5.6-luna" || newestModel(ids, "gpt") != "cx/gpt-6.1-sol" {
		t.Fatal("GPT models not first", catalog.Models[0]["slug"])
	}
	cfg, _ := os.ReadFile(codexConfigPath(home))
	if !strings.Contains(string(cfg), "wj-models.json") {
		t.Fatal("catalog not wired into config.toml")
	}
	if _, err := applyCodex(home, "login", "", "", "", "", false); err != nil {
		t.Fatal(err)
	}
	cfg, _ = os.ReadFile(codexConfigPath(home))
	if strings.Contains(string(cfg), "model_catalog_json") {
		t.Fatal("login kept the gateway catalog")
	}
}

func TestClaudeGatewayNewestModelsAndDiscovery(t *testing.T) {
	ids := []string{"claude-fable", "cc/claude-fable-5", "cc/claude-fable-5-1", "cc/claude-opus-5-5", "cc/claude-sonnet-5-5-review"}
	if got := newestModel(ids, "fable"); got != "cc/claude-fable-5-1" {
		t.Fatal(got)
	}
	if got := newestModel(ids, "sonnet"); got != "" {
		t.Fatal("review model chosen", got)
	}
	home := testHome(t)
	p := connectionProfile{Auth: "gateway", BaseURL: "https://example.com", Credential: "k", Models: ids}
	if _, err := applyClaude(home, p, false); err != nil {
		t.Fatal(err)
	}
	root, _ := readSettings(claudeSettingsPath(home))
	env := root["env"].(map[string]any)
	if env["CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY"] != "1" || env["ANTHROPIC_DEFAULT_FABLE_MODEL"] != "cc/claude-fable-5-1" {
		t.Fatalf("env %v", env)
	}
	if _, err := applyClaude(home, connectionProfile{Auth: "oauth", Credential: "sk-ant-oat-x"}, false); err != nil {
		t.Fatal(err)
	}
	root, _ = readSettings(claudeSettingsPath(home))
	if _, ok := root["env"].(map[string]any)["CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY"]; ok {
		t.Fatal("discovery flag left after leaving the gateway")
	}
}

func TestConfigureFromSavedGatewayProfile(t *testing.T) {
	home := testHome(t)
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex"))
	if err := saveProfile(home, "adnx", connectionProfile{Auth: "gateway", BaseURL: "https://gw.example/v1", Credential: "shared-key", Header: "bearer"}, false); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	args := []string{"-from-profile", "adnx", "-skip-check", "-models", "cx/gpt-6.1-sol,cc/claude-opus-5-5"}
	if err := configureAPIClient("codex", args, strings.NewReader(""), &out, home); err != nil {
		t.Fatal(err, out.String())
	}
	cfg, _ := os.ReadFile(codexConfigPath(home))
	if !strings.Contains(string(cfg), "shared-key") || !strings.Contains(string(cfg), "https://gw.example/v1") || strings.Contains(out.String(), "shared-key") {
		t.Fatal("profile key not reused or printed", string(cfg))
	}
	if err := configureAPIClient("codex", []string{"-from-profile", "adnx", "-auth", "api-key"}, strings.NewReader(""), &out, home); err == nil {
		t.Fatal("from-profile with api-key accepted")
	}
}

func TestNewestModelOrdering(t *testing.T) {
	ids := []string{"gpt-5.6-terra", "gpt-5.6-sol", "cx/gpt-reserve", "cx/gpt-daybreak-blue-latest", "cx/gpt-6.1-sol", "cx/gpt-6-sol", "cx/gpt-6-sol[1m]", "cx/gpt-5.5-high", "cx/gpt-6.1-sol-review"}
	if got := newestModel(ids, "gpt"); got != "cx/gpt-6.1-sol" {
		t.Fatal(got)
	}
	claude := []string{"cc/claude-haiku-4-5-20251001", "cc/claude-opus-5-5", "claude-opus", "cc/claude-fable-5", "cc/claude-fable-5-1"}
	if newestModel(claude, "opus") != "cc/claude-opus-5-5" || newestModel(claude, "fable") != "cc/claude-fable-5-1" || newestModel(claude, "haiku") != "cc/claude-haiku-4-5-20251001" {
		t.Fatal("claude families")
	}
}
