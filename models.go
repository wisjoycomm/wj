package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
)

// Claude Code lists gateway ids containing "claude" or "anthropic" by itself and
// drops the rest, so wj adds the rest, verbatim, as rows of the /model picker.
func claudePickerRows(ids []string) []any {
	var rows []any
	for _, id := range ids {
		low := strings.ToLower(id)
		if strings.Contains(low, "claude") || strings.Contains(low, "anthropic") {
			continue
		}
		rows = append(rows, map[string]any{"model": id, "label": id, "description": "Qua gateway; Claude Code không hỗ trợ chính thức"})
	}
	return rows
}

// verifyClaudeModels is the "Claude first" check. It is a variable so tests can
// run without a network.
var verifyClaudeModels = probeClaudeModels

// probeClaudeModels returns the ids that answer a streaming, tool-using
// /v1/messages request sent with Claude Code's identity headers.
func probeClaudeModels(conn connectionProfile, ids []string) []string {
	passed := make([]bool, len(ids))
	limit := make(chan struct{}, 8)
	var wait sync.WaitGroup
	for i, id := range ids {
		wait.Add(1)
		go func() {
			defer wait.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			passed[i] = probeModel(conn, "/messages", probeMessages, id) == "OK"
		}()
	}
	wait.Wait()
	var kept []string
	for i, id := range ids {
		if passed[i] {
			kept = append(kept, id)
		}
	}
	return kept
}

// setClaudePicker adds the non-Claude gateway models to /model. With verify on,
// only those that really work through /v1/messages are added, so a model that
// would break Claude Code never shows up. It returns how many rows were added
// and how many were dropped.
func setClaudePicker(root map[string]any, conn connectionProfile, ids []string, verify bool) (added, dropped int) {
	rows := claudePickerRows(ids)
	if verify && len(rows) > 0 {
		extra := make([]string, len(rows))
		for i, row := range rows {
			extra[i], _ = row.(map[string]any)["model"].(string)
		}
		working := map[string]bool{}
		for _, id := range verifyClaudeModels(conn, extra) {
			working[id] = true
		}
		var kept []any
		for i, row := range rows {
			if working[extra[i]] {
				kept = append(kept, row)
			}
		}
		dropped = len(rows) - len(kept)
		rows = kept
	}
	if len(rows) == 0 {
		delete(root, "modelPicker")
		return 0, dropped
	}
	root["modelPicker"] = map[string]any{"options": rows}
	return len(rows), dropped
}

// claudeTierKeys are the Claude Code settings that pin a family to one gateway id.
// The aliases opus, sonnet, haiku and fable, and the subagents, follow them.
var claudeTierKeys = []struct{ key, family string }{
	{"ANTHROPIC_DEFAULT_OPUS_MODEL", "opus"},
	{"ANTHROPIC_DEFAULT_SONNET_MODEL", "sonnet"},
	{"ANTHROPIC_DEFAULT_HAIKU_MODEL", "haiku"},
	{"ANTHROPIC_DEFAULT_FABLE_MODEL", "fable"},
	{"CLAUDE_CODE_SUBAGENT_MODEL", "haiku"},
}

// upgradeClaudeTiers moves each pinned family to the newest id of that family the
// gateway lists, once it answers through /v1/messages. A default model that was
// the old pin moves with it; one the user picked by hand stays. It returns a line
// per change, "cc/claude-opus-5-5 → cc/claude-opus-5-6".
func upgradeClaudeTiers(root, env map[string]any, conn connectionProfile, ids []string) []string {
	wanted := map[string]string{} // old id -> newest of its family
	var candidates []string
	for _, tier := range claudeTierKeys {
		old, _ := env[tier.key].(string)
		newest := newestModel(claudeIDs(ids), tier.family)
		if old == "" || newest == "" || old == newest || !newerModel(newest, old) {
			continue
		}
		wanted[old] = newest
		if !contains(candidates, newest) {
			candidates = append(candidates, newest)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	working := verifyClaudeModels(conn, candidates)
	var changes []string
	for _, tier := range claudeTierKeys {
		old, _ := env[tier.key].(string)
		newest, ok := wanted[old]
		if !ok || !contains(working, newest) {
			continue
		}
		env[tier.key] = newest
		line := old + " → " + newest
		if !contains(changes, line) {
			changes = append(changes, line)
		}
	}
	if current, _ := root["model"].(string); wanted[current] != "" && contains(working, wanted[current]) {
		root["model"] = wanted[current]
	}
	return changes
}

// claudeIDs keeps the ids Claude Code itself lists: those naming Claude.
func claudeIDs(ids []string) []string {
	var kept []string
	for _, id := range ids {
		if low := strings.ToLower(id); strings.Contains(low, "claude") || strings.Contains(low, "anthropic") {
			kept = append(kept, id)
		}
	}
	return kept
}

// pickerModelIDs lists the models setClaudePicker put in /model.
func pickerModelIDs(root map[string]any) []string {
	picker, _ := root["modelPicker"].(map[string]any)
	rows, _ := picker["options"].([]any)
	var ids []string
	for _, row := range rows {
		if id, _ := row.(map[string]any)["model"].(string); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// modelTarget is an app already connected to a gateway. Each app's own stored
// key is used, so nothing has to be entered again.
type modelTarget struct {
	App   string
	Conn  connectionProfile
	Apply func(ids []string) (string, error)
}

func gatewayTargets(home string) []modelTarget {
	var targets []modelTarget
	if t := claudeTarget(home); t != nil {
		targets = append(targets, *t)
	}
	if t := codexTarget(home); t != nil {
		targets = append(targets, *t)
	}
	if t := opencodeTarget(home); t != nil {
		targets = append(targets, *t)
	}
	return targets
}

func connectionOf(base, key, header string) (connectionProfile, bool) {
	p := connectionProfile{Auth: "gateway", BaseURL: base, Credential: key, Header: header}
	return p, base != "" && key != "" && p.validate() == nil
}

func claudeTarget(home string) *modelTarget {
	path := claudeSettingsPath(home)
	root, err := readSettings(path)
	if err != nil {
		return nil
	}
	env, _ := root["env"].(map[string]any)
	base, _ := env["ANTHROPIC_BASE_URL"].(string)
	key, _ := env["ANTHROPIC_AUTH_TOKEN"].(string)
	header := "bearer"
	if key == "" {
		key, _ = env["ANTHROPIC_API_KEY"].(string)
		header = "x-api-key"
	}
	conn, ok := connectionOf(base, key, header)
	if !ok {
		return nil
	}
	conn.NeutralHeaders = env["ANTHROPIC_CUSTOM_HEADERS"] == neutralHeaders
	return &modelTarget{"claude", conn, func(ids []string) (string, error) {
		added, dropped := setClaudePicker(root, conn, ids, true)
		upgraded := upgradeClaudeTiers(root, env, conn, ids)
		data, err := json.MarshalIndent(root, "", "  ")
		if err != nil {
			return "", err
		}
		if err = writeFile(path, string(data)+"\n", false); err != nil {
			return "", err
		}
		line := fmt.Sprintf("%d model; Claude tự hiện model Claude, thêm %d model khác đã thử chạy được vào /model", len(ids), added)
		if dropped > 0 {
			line += fmt.Sprintf("; bỏ %d model không chạy qua Claude Code", dropped)
		}
		for _, change := range upgraded {
			line += "\n  lên bản mới: " + change
		}
		return line, nil
	}}
}

func codexTarget(home string) *modelTarget {
	doc, err := readConfig(codexConfigPath(home))
	if err != nil {
		return nil
	}
	id, _ := doc["model_provider"].(string)
	providers, _ := doc["model_providers"].(map[string]any)
	provider, _ := providers[id].(map[string]any)
	base, _ := provider["base_url"].(string)
	key, _ := provider["experimental_bearer_token"].(string)
	conn, ok := connectionOf(base, key, "bearer")
	if !ok {
		return nil
	}
	model, _ := doc["model"].(string)
	return &modelTarget{"codex", conn, func(ids []string) (string, error) {
		if _, err := applyCodexModels(home, "gateway", conn.BaseURL, key, model, id, ids, false); err != nil {
			return "", err
		}
		return fmt.Sprintf("%d model trong danh sách /model của Codex", len(ids)), nil
	}}
}

func opencodeTarget(home string) *modelTarget {
	root := os.Getenv("XDG_CONFIG_HOME")
	if root == "" {
		root = filepath.Join(home, ".config")
	}
	doc := map[string]any{}
	for _, name := range []string{"opencode.json", "opencode.jsonc"} {
		layer, err := readConfig(filepath.Join(root, "opencode", name))
		if err != nil {
			return nil
		}
		mergeObject(doc, layer)
	}
	current, _ := doc["model"].(string)
	id, chosen, found := strings.Cut(current, "/")
	if !found {
		return nil
	}
	providers, _ := doc["provider"].(map[string]any)
	provider, _ := providers[id].(map[string]any)
	options, _ := provider["options"].(map[string]any)
	base, _ := options["baseURL"].(string)
	key, _ := options["apiKey"].(string)
	npm, _ := provider["npm"].(string)
	conn, ok := connectionOf(base, key, "bearer")
	if !ok {
		return nil
	}
	return &modelTarget{"opencode", conn, func(ids []string) (string, error) {
		// applyOpenCode makes the first id the default, so keep the current one first.
		if _, err := applyOpenCode(home, conn.BaseURL, key, append([]string{chosen}, ids...), id, npm, false); err != nil {
			return "", err
		}
		return fmt.Sprintf("%d model", len(ids)), nil
	}}
}

func runModels(args []string, input io.Reader, out io.Writer, home string) error {
	if len(args) > 0 && args[0] == "test" {
		return runModelsTest(args[1:], input, out, home)
	}
	if len(args) != 0 {
		return errors.New("cách dùng: wj models | wj models test [-models ID1,ID2] [-yes]")
	}
	targets := gatewayTargets(home)
	if len(targets) == 0 {
		return errors.New("chưa có app nào nối gateway; chạy: wj configure claude hoặc wj configure codex")
	}
	type fetched struct {
		ids []string
		err error
	}
	cache := map[string]fetched{}
	failed := 0
	for _, target := range targets {
		id := target.Conn.BaseURL + "\x00" + target.Conn.Header + "\x00" + target.Conn.Credential
		result, ok := cache[id]
		if !ok {
			result.ids, result.err = discoverModels(target.Conn)
			cache[id] = result
		}
		line := ""
		err := result.err
		if err == nil {
			line, err = target.Apply(result.ids)
		}
		if err != nil {
			failed++
			fmt.Fprintf(out, "%s (%s): lỗi - %v\n", target.App, hostOf(target.Conn.BaseURL), err)
			continue
		}
		fmt.Fprintf(out, "%s (%s): %s\n", target.App, hostOf(target.Conn.BaseURL), line)
	}
	if failed == len(targets) {
		return errors.New("không lấy được danh sách model")
	}
	fmt.Fprintln(out, "Xong. Mở phiên MỚI của app để thấy danh sách mới.")
	return nil
}

// Identity Claude Code sends (captured from v2.1.319). On 2026-11-06 the adnx
// gateway answered 400 "reasoning_effort: Extra inputs are not permitted" for
// every Claude model when either User-Agent or x-app carried this identity, and
// 200 for the same body sent by a plain client.
const (
	claudeCodeUserAgent = "claude-cli/2.1.319 (external, sdk-cli)"
	claudeCodeBeta      = "claude-code-20250219,interleaved-thinking-2025-05-14,context-management-2025-06-27,effort-2025-11-24"
)

// Opt-in workaround (-neutral-headers): Claude Code is told, through
// ANTHROPIC_CUSTOM_HEADERS, to send these instead of its own identity. They name
// this tool honestly and do not imitate another product.
const (
	neutralUserAgent = "wj-gateway-client/1"
	neutralApp       = "wj"
	neutralHeaders   = "User-Agent: " + neutralUserAgent + "\nx-app: " + neutralApp
)

const (
	probeMessages = `{"model":%q,"max_tokens":16,"stream":true,"messages":[{"role":"user","content":"Reply with: ok"}],"tools":[{"name":"noop","description":"Does nothing.","input_schema":{"type":"object","properties":{}}}]}`
	probeResponse = `{"model":%q,"max_output_tokens":16,"stream":true,"input":"Reply with: ok","tools":[{"type":"function","name":"noop","description":"Does nothing.","parameters":{"type":"object","properties":{}}}]}`
)

// probeModel sends the same shape of request the app sends: streaming, with a
// tool. Only a verdict comes back; a body can echo credentials and is never shown.
func probeModel(conn connectionProfile, path, bodyFormat, model string) string {
	target := conn.BaseURL + path
	if path == "/messages" {
		target += "?beta=true"
	}
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(fmt.Sprintf(bodyFormat, model)))
	if err != nil {
		return "lỗi"
	}
	if path == "/messages" {
		// A gateway can treat Claude Code differently from a plain client, so the
		// probe must look like Claude Code or its verdict means nothing.
		agent, app := claudeCodeUserAgent, "cli"
		if conn.NeutralHeaders {
			agent, app = neutralUserAgent, neutralApp
		}
		req.Header.Set("User-Agent", agent)
		req.Header.Set("x-app", app)
		req.Header.Set("anthropic-beta", claudeCodeBeta)
	}
	if conn.Header == "x-api-key" {
		req.Header.Set("x-api-key", conn.Credential)
	} else {
		req.Header.Set("Authorization", "Bearer "+conn.Credential)
	}
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
	client := &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return "không kết nối"
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Sprintf("HTTP %d", res.StatusCode)
	}
	return streamVerdict(res.Body)
}

func streamVerdict(body io.Reader) string {
	started := false
	reader := bufio.NewReader(io.LimitReader(body, 2<<20))
	for {
		line, err := reader.ReadBytes('\n')
		if bytes.HasPrefix(line, []byte("event: error")) || bytes.Contains(line, []byte("response.failed")) ||
			bytes.Contains(line, []byte(`"type":"error"`)) || bytes.Contains(line, []byte(`"type": "error"`)) {
			return "lỗi trong stream"
		}
		if bytes.Contains(line, []byte("message_start")) || bytes.Contains(line, []byte("response.created")) {
			started = true
		}
		if err != nil {
			if started {
				return "OK"
			}
			return "sai định dạng"
		}
	}
}

func runModelsTest(args []string, input io.Reader, out io.Writer, home string) error {
	flags := flag.NewFlagSet("models test", flag.ContinueOnError)
	flags.SetOutput(out)
	only := flags.String("models", "", "chỉ thử các model này, cách nhau dấu phẩy")
	assume := flags.Bool("yes", false, "không hỏi xác nhận")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("cách dùng: wj models test [-models ID1,ID2] [-yes]")
	}
	targets := gatewayTargets(home)
	if len(targets) == 0 {
		return errors.New("chưa có app nào nối gateway; chạy: wj configure claude hoặc wj configure codex")
	}
	conn := targets[0].Conn
	ids := splitList(*only)
	if len(ids) == 0 {
		found, err := discoverModels(conn)
		if err != nil {
			return err
		}
		ids = found
	}
	if !*assume {
		reader := bufio.NewReader(&oneByteReader{input})
		fmt.Fprintf(out, "Sẽ gửi %d request nhỏ tới %s (%d model x 2 đường, tốn vài token mỗi request).\n", len(ids)*2, hostOf(conn.BaseURL), len(ids))
		if !yes(prompt(reader, out, "Tiếp tục? [y/N, Enter = huỷ]")) {
			return nil
		}
	}
	type result struct{ messages, responses string }
	results := make([]result, len(ids))
	limit := make(chan struct{}, 4)
	var wait sync.WaitGroup
	for i, id := range ids {
		wait.Add(1)
		go func() {
			defer wait.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			results[i] = result{probeModel(conn, "/messages", probeMessages, id), probeModel(conn, "/responses", probeResponse, id)}
		}()
	}
	wait.Wait()
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "MODEL\t/v1/messages (Claude Code)\t/v1/responses (Codex)")
	for i, id := range ids {
		fmt.Fprintf(w, "%s\t%s\t%s\n", id, results[i].messages, results[i].responses)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(out, "\nOK ở cột nào thì app đó chọn model này được. Lệnh chỉ thử một câu ngắn có tool; không đảm bảo mọi tính năng của app.")
	return nil
}
