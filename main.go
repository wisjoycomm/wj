// wj (formerly setup-gateway) installs the bundled development toolkit and configures coding
// clients for gateway, OAuth, API-key or browser authentication.
//
//	claude    ~/.claude/settings.json            env block (Anthropic wire)
//	codex     ~/.codex/config.toml               [model_providers.<id>] (Responses wire)
//	agy       Antigravity CLI                    no custom endpoint; reported, never written
//	cursor    guided configuration in the application
//	opencode  ~/.config/opencode/opencode.jsonc  provider.<id>
//	proxy     mitmproxy in the middle            local token-usage log, no config writes
//
// Claude authentication changes preserve hooks and unrelated settings. Changed
// configuration files receive unique backups before replacement.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

func main() {
	restore := prepareConsole()
	migrateLegacy(os.Args[1:], os.Stderr)
	err := runApp(os.Args[1:], os.Stdin, os.Stdout)
	restore()
	var code exitError
	if errors.As(err, &code) {
		os.Exit(int(code))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "wj:", err)
		os.Exit(1)
	}
}

// pick returns the first id containing every needle and none of the exclusions.
func pick(ids []string, needles []string, exclude []string) string {
	for _, id := range ids {
		low := strings.ToLower(id)
		ok := true
		for _, n := range needles {
			if !strings.Contains(low, n) {
				ok = false
			}
		}
		for _, x := range exclude {
			if strings.Contains(low, x) {
				ok = false
			}
		}
		if ok {
			return id
		}
	}
	return ""
}

func setupCursor() string {
	return "MANUAL - mở Cursor > Settings > Models, nhập API key và Base URL (nếu Cursor hỗ trợ). Xem: wj configure cursor. Không ghi biến OPENAI_* toàn máy."
}

func setupOpenCode(home, base, key string, ids []string, provider string, dry bool) (string, error) {
	return applyOpenCode(home, base, key, ids, provider, "@ai-sdk/openai-compatible", dry)
}
