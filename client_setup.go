package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/tailscale/hujson"
)

func codexConfigPath(home string) string {
	dir := os.Getenv("CODEX_HOME")
	if dir == "" {
		dir = filepath.Join(home, ".codex")
	}
	return filepath.Join(dir, "config.toml")
}

func codexCatalogPath(home string) string {
	return filepath.Join(filepath.Dir(codexConfigPath(home)), "wj-models.json")
}

// codexCatalog lists every gateway model in Codex's model picker. GPT models
// (what a Responses-API agent handles best) come first, newest first; review-only
// variants go last.
func codexCatalog(ids []string) ([]byte, error) {
	var gpt, other, review []string
	for _, id := range ids {
		low := strings.ToLower(id)
		switch {
		case strings.Contains(low, "review"):
			review = append(review, id)
		case strings.Contains(low, "gpt"):
			gpt = append(gpt, id)
		default:
			other = append(other, id)
		}
	}
	sort.SliceStable(gpt, func(i, j int) bool { return newerModel(gpt[i], gpt[j]) })
	other = append(other, review...)
	levels := []map[string]string{}
	for _, effort := range []string{"low", "medium", "high", "xhigh"} {
		levels = append(levels, map[string]string{"effort": effort, "description": effort})
	}
	var models []map[string]any
	for i, id := range append(gpt, other...) {
		models = append(models, map[string]any{
			"slug": id, "display_name": id, "description": "Gateway model",
			"visibility": "list", "supported_in_api": true, "priority": i,
			"supported_reasoning_levels": levels, "shell_type": "unified_exec",
			"base_instructions":            "You are Codex, a coding assistant. Follow the user instructions and repository guidance. Use available tools to inspect, edit and verify work. Report results accurately.",
			"supports_reasoning_summaries": false, "default_reasoning_summary": "none",
			"support_verbosity": false, "experimental_supported_tools": []string{},
			"input_modalities":  []string{"text"},
			"truncation_policy": map[string]any{"mode": "tokens", "limit": 10000},
		})
	}
	data, err := json.MarshalIndent(map[string]any{"models": models}, "", "  ")
	return append(data, '\n'), err
}

// defaultChatModel picks a sensible default for multi-provider clients such as
// OpenCode: the newest Claude Opus/Sonnet, else the newest GPT.
func defaultChatModel(ids []string) string {
	for _, family := range []string{"opus", "sonnet", "gpt"} {
		if model := newestModel(ids, family); model != "" {
			return model
		}
	}
	return ""
}

func applyCodex(home, auth, base, key, model, provider string, dry bool) (string, error) {
	return applyCodexModels(home, auth, base, key, model, provider, nil, dry)
}

func applyCodexModels(home, auth, base, key, model, provider string, ids []string, dry bool) (string, error) {
	path := codexConfigPath(home)
	doc := map[string]any{}
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if len(b) > 0 {
		if err = toml.Unmarshal(b, &doc); err != nil {
			return "", fmt.Errorf("file Codex config.toml bị lỗi cú pháp, tool không sửa nó: %w", err)
		}
	}
	delete(doc, "model_provider")
	delete(doc, "model")
	delete(doc, "forced_login_method")
	if model != "" {
		doc["model"] = model
	}
	if auth == "gateway" {
		providers, ok := doc["model_providers"].(map[string]any)
		if !ok {
			providers = map[string]any{}
		}
		providers[provider] = map[string]any{"name": provider, "base_url": base, "wire_api": "responses", "requires_openai_auth": false, "experimental_bearer_token": key}
		doc["model_providers"] = providers
		doc["model_provider"] = provider
	}
	// A gateway's model list replaces Codex's built-in OpenAI catalog; login and
	// direct API keys go back to the built-in catalog.
	if auth == "gateway" && len(ids) > 0 {
		catalog, err := codexCatalog(ids)
		if err != nil {
			return "", err
		}
		if err = writeFile(codexCatalogPath(home), string(catalog), dry); err != nil {
			return "", err
		}
		doc["model_catalog_json"] = codexCatalogPath(home)
		if !dry {
			_ = os.Remove(filepath.Join(filepath.Dir(codexCatalogPath(home)), legacyName+"-models.json"))
		}
	} else if auth != "gateway" {
		delete(doc, "model_catalog_json")
	}
	data, err := toml.Marshal(doc)
	if err != nil {
		return "", err
	}
	if err = writeFile(path, string(data), dry); err != nil {
		return "", err
	}
	return path, nil
}

func applyOpenCode(home, base, key string, ids []string, provider, npm string, dry bool) (string, error) {
	root := os.Getenv("XDG_CONFIG_HOME")
	if root == "" {
		root = filepath.Join(home, ".config")
	}
	return applyOpenCodeIn(filepath.Join(root, "opencode"), base, key, ids, provider, npm, dry)
}

// applyOpenCodeIn writes opencode.json(c) in dir: the user config folder, or a
// project root for a project-only connection.
func applyOpenCodeIn(dir, base, key string, ids []string, provider, npm string, dry bool) (string, error) {
	// JSONC wins when both exist. Read both to retain unrelated settings.
	doc := map[string]any{}
	path := filepath.Join(dir, "opencode.json")
	for _, name := range []string{"opencode.json", "opencode.jsonc"} {
		file := filepath.Join(dir, name)
		b, err := os.ReadFile(file)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		clean, err := hujson.Standardize(b)
		if err != nil {
			return "", fmt.Errorf("file %s bị lỗi cú pháp, tool không sửa nó: %w", file, err)
		}
		var layer map[string]any
		if err = json.Unmarshal(clean, &layer); err != nil || layer == nil {
			return "", fmt.Errorf("%s must contain a JSON object", file)
		}
		mergeObject(doc, layer)
		path = file
	}
	providers, _ := doc["provider"].(map[string]any)
	if providers == nil {
		providers = map[string]any{}
	}
	entry, _ := providers[provider].(map[string]any)
	if entry == nil {
		entry = map[string]any{}
	}
	models := map[string]any{}
	for _, id := range ids {
		models[id] = map[string]any{"name": id}
	}
	entry["npm"] = npm
	entry["name"] = provider
	entry["options"] = map[string]any{"baseURL": base, "apiKey": key}
	entry["models"] = models
	providers[provider] = entry
	doc["provider"] = providers
	doc["$schema"] = "https://opencode.ai/config.json"
	if len(ids) > 0 {
		doc["model"] = provider + "/" + ids[0]
	}
	// Remove only this tool's old, unsupported provider entry during migration.
	if old, ok := doc["providers"].(map[string]any); ok {
		if item, ok := old[provider].(map[string]any); ok && item["package"] == "@opencode/ai/providers/openai-compatible" {
			delete(old, provider)
			if len(old) == 0 {
				delete(doc, "providers")
			}
		}
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	if err = writeFile(path, string(data)+"\n", dry); err != nil {
		return "", err
	}
	return path, nil
}

func mergeObject(dest, src map[string]any) {
	for k, v := range src {
		a, aOK := dest[k].(map[string]any)
		b, bOK := v.(map[string]any)
		if aOK && bOK {
			mergeObject(a, b)
		} else {
			dest[k] = v
		}
	}
}

func configureClient(args []string, input io.Reader, out io.Writer) error {
	if len(args) == 0 {
		id, err := pickOne(input, out, "Kết nối app nào?",
			[]string{"claude", "codex", "opencode", "cursor", "cursor-agent", "antigravity", "orca"},
			[]string{"Claude Code", "Codex CLI", "OpenCode", "Cursor editor (hướng dẫn)", "Cursor CLI (hướng dẫn)", "Antigravity CLI / IDE (hướng dẫn)", "Orca (hướng dẫn)"}, 0)
		if err != nil || id == "" {
			return err
		}
		args = []string{id}
	}
	id := args[0]
	if id == "claude" {
		return runCLI(append([]string{"-only", "claude"}, args[1:]...), input, out)
	}
	if _, err := appByID(id); err != nil {
		return err
	}
	if id != "codex" && id != "opencode" {
		if len(args) > 1 {
			return errors.New("app này tự đăng nhập trong giao diện của nó; configure chỉ nhận tên app")
		}
		switch id {
		case "cursor-agent":
			fmt.Fprintln(out, "Cursor CLI: chạy  agent login  để đăng nhập tài khoản Cursor. Key Cursor khác token Claude. Tool không cấu hình gateway cho Cursor CLI.")
		case "cursor":
			fmt.Fprintln(out, "Cursor: mở Cursor > Settings > Models, nhập API key và (nếu có) Override OpenAI Base URL. Đăng nhập trong Cursor. Tool không ghi biến OPENAI_* toàn máy.")
		case "antigravity", "agy", "antigravity-ide":
			fmt.Fprintln(out, "Antigravity: mở agy hoặc Antigravity IDE rồi đăng nhập Google. Antigravity không hỗ trợ gateway riêng.")
		case "orca":
			fmt.Fprintln(out, "Orca: mở Orca, thêm thư mục project, chọn Claude hoặc Codex đã cài. Orca dùng kết nối của Claude/Codex, nên hãy kết nối chúng ở menu này trước.")
		}
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return configureAPIClient(id, args[1:], input, out, home)
}

func configureAPIClient(id string, args []string, input io.Reader, out io.Writer, home string) error {
	var err error
	flags := flag.NewFlagSet("configure "+id, flag.ContinueOnError)
	flags.SetOutput(out)
	auth := flags.String("auth", "", "login, api-key, gateway")
	base := flags.String("base-url", "", "custom gateway domain")
	provider := flags.String("provider-id", "gateway", "gateway provider name; direct OpenCode API: openai or anthropic")
	models := flags.String("models", "", "comma-separated model IDs; first is the default")
	keyStdin := flags.Bool("key-stdin", false, "read one credential line from stdin")
	skip := flags.Bool("skip-check", false, "skip gateway model discovery")
	dry := flags.Bool("dry-run", false, "no files, network, installation or login")
	noLogin := flags.Bool("no-login", false, "prepare normal login without starting it")
	device := flags.Bool("device-auth", false, "Codex device-code login")
	fromProfile := flags.String("from-profile", "", "dùng domain + key của profile gateway Claude đã lưu (vd adnx)")
	scope := flags.String("scope", "", "OpenCode: global = toàn máy (mặc định), project = chỉ project đang đứng (opencode.json trong project, git bỏ qua)")
	dir := flags.String("dir", "", "với -scope project: thư mục project (mặc định = thư mục đang đứng)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("có tham số không hiểu; xem: wj help codex")
	}
	if *auth == "" && *fromProfile != "" {
		*auth = "gateway"
	}
	if *auth == "" {
		fmt.Fprintf(out, "Kết nối %s - bạn đang có gì?\n  1. Tài khoản, đăng nhập trên trình duyệt  -> Đăng nhập\n  2. API key trực tiếp (OpenAI / Anthropic)  -> API key\n  3. Domain công ty (vd ai.adnx.vn) + key    -> Gateway\n  0. Quay lại\n", id)
		answer := menuAnswer(input, out, "Chọn [0-3]")
		if answer == "" || answer == "0" {
			return errCancelled
		}
		*auth = map[string]string{"1": "login", "2": "api-key", "3": "gateway"}[answer]
		// Offer gateways already saved for Claude so the key is not typed twice.
		if *auth == "gateway" && *base == "" && !*keyStdin {
			names, labels := savedGatewayProfiles(home)
			if len(names) > 0 {
				names, labels = append(names, ""), append(labels, "Nhập domain + key mới")
				choice, err := pickOne(input, out, "Dùng gateway đã lưu?", names, labels, 1)
				if err != nil {
					return err
				}
				*fromProfile = choice
			}
		}
	}
	if !contains([]string{"login", "api-key", "gateway"}, *auth) {
		return errors.New("hãy chọn 1, 2 hoặc 3 (login, api-key, gateway); token Claude không dùng cho app này")
	}
	if !validName(*provider) {
		return errors.New("provider-id chỉ gồm chữ, số, _ hoặc -")
	}
	if id == "codex" && (*scope != "" && *scope != scopeGlobal || *dir != "") {
		return errors.New("Codex chỉ kết nối toàn máy: Codex bỏ qua provider trong config của project. Bỏ -scope/-dir")
	}
	if id == "opencode" && *auth != "login" && len(args) == 0 {
		if *scope, err = askScope(input, out, "", home); err != nil {
			return err
		}
	}
	scopeName, projectDir, err := resolveScope(*scope, *dir, home)
	if err != nil {
		return err
	}
	if scopeName == scopeProject && *auth == "login" {
		return errors.New("đăng nhập là của tài khoản, luôn toàn máy; bỏ -scope project")
	}
	if *dir != "" && scopeName != scopeProject {
		return errors.New("-dir chỉ dùng với -scope project")
	}
	if *device && (id != "codex" || *auth != "login") {
		return errors.New("-device-auth chỉ dùng với Codex -auth login")
	}
	if *noLogin && *auth != "login" {
		return errors.New("-no-login chỉ dùng với -auth login")
	}
	if *auth == "login" && (*base != "" || *keyStdin || *models != "") {
		return errors.New("-auth login không nhận -base-url, key hay -models")
	}
	if *auth == "api-key" && *base != "" {
		return errors.New("có domain riêng thì chọn -auth gateway")
	}
	if id == "opencode" && *auth == "api-key" {
		if *provider == "gateway" {
			*provider = "openai"
			if len(args) == 0 {
				choice, err := pickOne(input, out, "Key của hãng nào?", []string{"openai", "anthropic"}, []string{"OpenAI", "Anthropic"}, 1)
				if err != nil {
					return err
				}
				*provider = choice
			}
		}
		if !contains([]string{"openai", "anthropic"}, *provider) {
			return errors.New("OpenCode API key cần -provider-id openai hoặc anthropic")
		}
	}
	key := ""
	if *fromProfile != "" {
		p, err := loadProfile(home, *fromProfile)
		if err != nil {
			return err
		}
		if p.Auth != "gateway" || p.Header == "x-api-key" {
			return fmt.Errorf("profile %q không phải gateway dùng header bearer, không dùng chung cho %s được", *fromProfile, id)
		}
		if *auth != "gateway" || *base != "" || *keyStdin {
			return errors.New("-from-profile đã có domain và key; không kèm -auth khác, -base-url hay -key-stdin")
		}
		*base, key = p.BaseURL, p.Credential
	}
	if *auth != "login" && key == "" {
		if *auth == "gateway" && *base == "" {
			if *keyStdin {
				return errors.New("-key-stdin cần kèm -base-url khi dùng gateway")
			}
			*base = menuAnswer(input, out, "Domain gateway (vd ai.adnx.vn; Enter hoặc 0 = quay lại)")
			if *base == "" || *base == "0" {
				return errCancelled
			}
		}
		reader := bufio.NewReader(&oneByteReader{input})
		if *keyStdin {
			key = prompt(reader, io.Discard, "")
		} else {
			key = strings.TrimSpace(os.Getenv("SETUP_GATEWAY_KEY"))
			if key == "" {
				key, err = askKey(input, reader, out, "Dán key")
				if err != nil {
					return err
				}
			}
		}
		if key == "" || strings.ContainsAny(key, " \r\n\t") {
			return errors.New("cần đúng một key, không có khoảng trắng")
		}
		if strings.HasPrefix(key, "sk-ant-oat") || strings.HasPrefix(key, "sk-ant-sid") || strings.HasPrefix(key, "sessionKey=") {
			return errors.New("đây là token Claude (sk-ant-oat/sid), không dùng được cho app này")
		}
		if id == "codex" && *auth == "api-key" && strings.HasPrefix(key, "sk-ant-") {
			return errors.New("Codex API key cần key OpenAI, không phải key Anthropic (sk-ant-...)")
		}
	}
	ids := splitList(*models)
	if *auth == "gateway" {
		*base, err = gatewayURL(*base)
		if err != nil {
			return err
		}
		if !*dry && !*skip {
			found, e := discoverModels(connectionProfile{BaseURL: *base, Credential: key, Header: "bearer"})
			if e != nil {
				return e
			}
			if len(ids) == 0 {
				ids = found
			}
		}
		if len(ids) == 0 && !*dry {
			return errors.New("không lấy được model; nhập tên model của gateway bằng -models")
		}
		if *models == "" && len(ids) > 0 {
			if len(args) == 0 {
				choices := ids
				if id == "codex" {
					// Codex speaks the Responses API: list GPT-style models first.
					var gpt, other []string
					for _, m := range ids {
						if strings.Contains(strings.ToLower(m), "gpt") {
							gpt = append(gpt, m)
						} else {
							other = append(other, m)
						}
					}
					choices = append(gpt, other...)
				}
				model, err := pickOne(input, out, "Chọn model mặc định:", choices, nil, 1)
				if err != nil {
					return err
				}
				ids = append([]string{model}, ids...)
			} else if id == "codex" {
				model := newestModel(ids, "gpt")
				// Gateways name models their own way: a GPT-style name is only a preference.
				if model == "" {
					model = ids[0]
				}
				ids = append([]string{model}, ids...)
			} else if model := defaultChatModel(ids); model != "" {
				ids = append([]string{model}, ids...)
			}
		}
	}
	app, _ := appByID(id)
	binary := findApp(app, home)
	needsProcess := id == "codex" && (*auth == "api-key" || *auth == "login" && !*noLogin) || id == "opencode" && *auth == "login" && !*noLogin
	if needsProcess && binary == "" && !*dry {
		return fmt.Errorf("máy chưa có %s; cài trước bằng: wj apps install %s", id, id)
	}
	if *dry {
		fmt.Fprintf(out, "Dry run: sẽ kết nối %s bằng %s; chưa ghi file, chưa gọi mạng, chưa đăng nhập.\n", id, *auth)
		return nil
	}
	if id == "codex" {
		model := ""
		if len(ids) > 0 {
			model = ids[0]
		}
		path, e := applyCodexModels(home, *auth, *base, key, model, *provider, splitList(strings.Join(ids, ",")), false)
		if e != nil {
			return e
		}
		fmt.Fprintln(out, "Đã ghi cấu hình:", path)
		if *auth == "gateway" {
			fmt.Fprintln(out, "Lưu ý: gateway phải hỗ trợ POST /v1/responses. Nếu terminal còn biến OPENAI_BASE_URL cũ, hãy xoá nó.")
		}
		if needsProcess {
			cmd := []string{binary, "login"}
			stdin := input
			if *auth == "api-key" {
				cmd = append(cmd, "--with-api-key")
				stdin = strings.NewReader(key + "\n")
			} else if *device {
				cmd = append(cmd, "--device-auth")
			}
			return runChild(cmd, stdin, out, cleanCodexEnvironment(os.Environ()))
		}
	} else if *auth == "login" {
		fmt.Fprintln(out, "OpenCode: đăng nhập rồi gõ /models trong OpenCode để chọn model. Gateway đã cấu hình trước đó vẫn còn.")
		if needsProcess {
			return runChild([]string{binary, "auth", "login"}, input, out, os.Environ())
		}
	} else {
		npm := "@ai-sdk/openai-compatible"
		if *auth == "api-key" {
			if *provider == "gateway" {
				*provider = "openai"
			}
			switch *provider {
			case "openai":
				*base = "https://api.openai.com/v1"
				npm = "@ai-sdk/openai"
			case "anthropic":
				*base = "https://api.anthropic.com/v1"
				npm = "@ai-sdk/anthropic"
			default:
				return errors.New("direct OpenCode API requires provider-id openai or anthropic")
			}
		}
		var path string
		var e error
		if scopeName == scopeProject {
			path, e = applyOpenCodeProject(projectDir, *base, key, ids, *provider, npm, false)
		} else {
			path, e = applyOpenCode(home, *base, key, ids, *provider, npm, false)
		}
		if e != nil {
			return e
		}
		fmt.Fprintln(out, "Đã ghi cấu hình:", path)
	}
	return nil
}

func cleanCodexEnvironment(env []string) []string {
	var result []string
	for _, item := range env {
		name, _, _ := strings.Cut(item, "=")
		if !contains([]string{"OPENAI_API_KEY", "OPENAI_BASE_URL", "CODEX_API_KEY", "CODEX_ACCESS_TOKEN", "SETUP_GATEWAY_KEY"}, strings.ToUpper(name)) {
			result = append(result, item)
		}
	}
	return result
}

// Prevent a configuration wizard from consuming the next menu answer.
type oneByteReader struct{ io.Reader }

func (r *oneByteReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return r.Reader.Read(p)
}
