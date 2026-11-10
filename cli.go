package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

func runCLI(args []string, input io.Reader, out io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return runCLIWithHome(args, input, out, home)
}

func runCLIWithHome(args []string, input io.Reader, out io.Writer, home string) error {
	flags := flag.NewFlagSet("wj", flag.ContinueOnError)
	flags.SetOutput(out)
	auth := flags.String("auth", "", "cách kết nối: gateway, oauth (token sk-ant-oat), api-key, login (đăng nhập web)")
	base := flags.String("base-url", "", "domain gateway, vd ai.adnx.vn (không cần https://)")
	key := flags.String("key", "", "key/token (nên để tool hỏi ẩn, hoặc dùng -key-stdin / biến SETUP_GATEWAY_KEY)")
	keyStdin := flags.Bool("key-stdin", false, "đọc key từ một dòng stdin (cho script)")
	header := flags.String("auth-header", "bearer", "header gửi key tới gateway: bearer hoặc x-api-key (chỉ Claude)")
	only := flags.String("only", "", "app cần cấu hình, cách nhau dấu phẩy: claude,codex,cursor,opencode,agy")
	provider := flags.String("provider-id", "gateway", "tên provider ghi vào Codex/OpenCode")
	profile := flags.String("profile", "", "lưu kết nối Claude này với tên này rồi dùng luôn")
	use := flags.String("use-profile", "", "chuyển sang kết nối Claude đã lưu")
	list := flags.Bool("list-profiles", false, "xem các kết nối đã lưu (không hiện key)")
	models := flags.String("models", "", "tên model của gateway, cách nhau dấu phẩy (thay cho danh sách tự lấy)")
	skip := flags.Bool("skip-check", false, "không gọi /v1/models (khi đó nhập -models)")
	install := flags.Bool("install-claude", false, "tự cài Claude Code nếu máy chưa có")
	noLogin := flags.Bool("no-login", false, "đăng nhập web: chỉ dọn cấu hình, chưa mở trình duyệt")
	neutral := flags.Bool("neutral-headers", false, "gateway: Claude Code gửi header trung tính thay vì danh tính Claude Code (đường vòng khi gateway từ chối Claude Code); tắt bằng -neutral-headers=false")
	launch := flags.Bool("run", false, "mở Claude Code ngay sau khi xong")
	dry := flags.Bool("dry-run", false, "chạy thử: không ghi file, không gọi mạng, không cài, không đăng nhập")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("có tham số không hiểu; xem: wj help claude")
	}
	neutralSet := false
	flags.Visit(func(f *flag.Flag) { neutralSet = neutralSet || f.Name == "neutral-headers" })
	var err error
	if *list {
		return listProfiles(home, out)
	}
	if !validName(*provider) {
		return errors.New("provider-id chỉ gồm chữ, số, _ hoặc -")
	}
	if *profile != "" && !validName(*profile) {
		return errors.New("tên profile chỉ gồm chữ, số, _ hoặc - (vd adnx, work)")
	}
	reader := bufio.NewReader(&oneByteReader{input})
	wizard := len(args) == 0 || len(args) == 2 && args[0] == "-only" && args[1] == "claude"
	var p connectionProfile
	if *use != "" {
		if *auth != "" || *base != "" || *key != "" || *keyStdin || *profile != "" || *models != "" {
			return errors.New("-use-profile không đi kèm key, domain hay -models mới")
		}
		p, err = loadProfile(home, *use)
		if err != nil {
			return err
		}
		if neutralSet {
			p.NeutralHeaders = *neutral
		}
		if *only == "" {
			*only = "claude"
		}
	} else {
		if *auth == "" {
			if *base != "" || *key != "" || *keyStdin || os.Getenv("SETUP_GATEWAY_KEY") != "" {
				*auth = "gateway"
			} else {
				fmt.Fprintln(out, "Kết nối Claude Code - bạn đang có gì?\n  1. Domain công ty (vd ai.adnx.vn) + key của domain đó   -> Gateway\n  2. Token bắt đầu bằng sk-ant-oat...                      -> Token OAuth\n  3. API key từ console.anthropic.com                      -> API key\n  4. Tài khoản Claude Pro/Max, đăng nhập trên trình duyệt  -> Đăng nhập web\n  0. Quay lại")
				choice := prompt(reader, out, "Chọn [0-4]")
				if choice == "" || choice == "0" {
					return errCancelled
				}
				*auth = map[string]string{"1": "gateway", "2": "oauth", "3": "api-key", "4": "login"}[choice]
				if *auth == "" {
					return errors.New("hãy chọn số từ 1 đến 4")
				}
			}
		}
		*auth = canonicalAuth(*auth)
		if *only == "" {
			*only = "claude"
			if wizard && *auth == "gateway" {
				clients := []string{"claude", "codex", "opencode", "cursor", "agy"}
				printOptions(out, "Dùng gateway này cho app nào?", clients, []string{"Claude Code", "Codex CLI", "OpenCode", "Cursor (hướng dẫn)", "Antigravity (bỏ qua, không hỗ trợ gateway)"}, 1)
				choice := prompt(reader, out, "Gõ số (vd 1 2 3), all = tất cả, Enter = chỉ Claude, 0 = quay lại")
				if choice == "0" {
					return errCancelled
				}
				if choice != "" {
					picked, err := parseChoices(choice, clients)
					if err != nil {
						return err
					}
					*only = strings.Join(picked, ",")
				}
			} else if *auth == "gateway" && !wizard && *profile == "" && *header != "x-api-key" {
				// Keep the original flag-based gateway invocation compatible.
				*only = "claude,codex,agy,cursor,opencode"
			}
		}
		if *auth == "gateway" && *base == "" {
			if *keyStdin {
				return errors.New("-key-stdin cần kèm -base-url khi dùng gateway")
			}
			*base = prompt(reader, out, "Domain gateway (vd ai.adnx.vn; Enter hoặc 0 = quay lại)")
			if *base == "" || *base == "0" {
				return errCancelled
			}
		}
		if *keyStdin {
			if *key != "" {
				return errors.New("chỉ dùng -key hoặc -key-stdin, không dùng cả hai")
			}
			*key = prompt(reader, io.Discard, "")
		} else if *key == "" && *auth != "login" {
			*key = strings.TrimSpace(os.Getenv("SETUP_GATEWAY_KEY"))
		}
		if *key == "" && *auth != "login" {
			label := "Dán key"
			if *auth == "oauth" {
				label = "Dán token sk-ant-oat..."
			}
			*key, err = askKey(input, reader, out, label)
			if err != nil {
				return err
			}
		}
		p = connectionProfile{Auth: *auth, BaseURL: *base, Credential: strings.TrimSpace(*key), Header: *header, Models: splitList(*models), NeutralHeaders: *neutral}
	}
	targets := splitList(*only)
	if err := validateTargets(targets, p, *profile != "" || *use != ""); err != nil {
		return err
	}
	if (*launch || *install) && !contains(targets, "claude") {
		return errors.New("-install-claude và -run chỉ dùng khi có claude trong -only")
	}
	if err := p.validate(); err != nil {
		return err
	}
	if p.NeutralHeaders && p.Auth != "gateway" {
		return errors.New("-neutral-headers chỉ dùng với kết nối Gateway")
	}
	if *use != "" && neutralSet && *skip && !*dry {
		// The normal path saves the profile after fetching models; with -skip-check it does not.
		if err := saveProfile(home, *use, p, false); err != nil {
			return err
		}
	}
	if wizard && len(targets) == 1 && targets[0] == "claude" {
		*profile = prompt(reader, out, "Lưu kết nối này với tên gì để đổi lại sau? (vd adnx, work; Enter = không lưu, 0 = quay lại)")
		if *profile == "0" {
			return errCancelled
		}
		if *profile != "" && !validName(*profile) {
			return errors.New("tên profile chỉ gồm chữ, số, _ hoặc - (vd adnx, work)")
		}
	}
	// Refuse invalid user settings before installing software or making requests.
	if contains(targets, "claude") {
		if _, err := readSettings(claudeSettingsPath(home)); err != nil {
			return err
		}
	}
	if !*dry && !*skip && (p.Auth == "gateway" || p.Auth == "api-key") {
		fmt.Fprintln(out, "Đang kiểm tra key và lấy danh sách model (không tốn token)...")
		ids, err := discoverModels(p)
		if err != nil {
			return err
		}
		if len(p.Models) == 0 || *use != "" && *models == "" {
			// A saved profile follows the gateway: new models appear without re-entry.
			p.Models = ids
			if *use != "" {
				if err := saveProfile(home, *use, p, false); err != nil {
					return err
				}
			}
		}
		fmt.Fprintf(out, "Key hợp lệ, gateway có %d model.\n", len(ids))
	}
	if wizard && contains(targets, "claude") && findClaude(home) == "" {
		answer := prompt(reader, out, "Máy chưa có Claude Code. Cài luôn? [y/N, Enter = không cài, 0 = quay lại]")
		if answer == "0" {
			return errCancelled
		}
		*install = yes(answer)
	}
	if *install {
		if err := ensureClaude(home, *dry, out); err != nil {
			return err
		}
	}
	if !*dry && contains(targets, "claude") && (p.Auth == "login" && !*noLogin || *launch) && findClaude(home) == "" {
		return errors.New("máy chưa có Claude Code; thêm -install-claude, hoặc -no-login nếu chỉ muốn ghi cấu hình")
	}
	if *profile != "" {
		if err := saveProfile(home, *profile, p, *dry); err != nil {
			return err
		}
		fmt.Fprintf(out, "Đã lưu profile: %s (%s)\n", *profile, p.Auth)
	}
	// Gateways name models their own way: a GPT-style name is only a preference.
	gpt := pick(p.Models, []string{"gpt"}, []string{"review"})
	if gpt == "" && len(p.Models) > 0 {
		gpt = p.Models[0]
	}
	for _, name := range targets {
		var line string
		var err error
		switch name {
		case "claude":
			line, err = applyClaude(home, p, *dry)
		case "codex":
			line, err = applyCodexModels(home, "gateway", p.BaseURL, p.Credential, gpt, *provider, p.Models, *dry)
		case "cursor":
			line = setupCursor()
		case "opencode":
			line, err = setupOpenCode(home, p.BaseURL, p.Credential, p.Models, *provider, *dry)
		case "agy":
			line = "SKIP - Antigravity không hỗ trợ gateway riêng, dùng tài khoản đã đăng nhập"
		}
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		fmt.Fprintf(out, "%s: %s\n", name, line)
	}
	if *dry {
		fmt.Fprintln(out, "Dry run: chỉ xem trước, không ghi file, không gọi mạng, không cài, không đăng nhập.")
		return nil
	}
	if contains(targets, "claude") {
		warnEnvironment(out)
		if p.Auth == "login" && !*noLogin {
			if err := runClaude(home, "auth", "login", "--claudeai"); err != nil {
				return fmt.Errorf("đã lưu cấu hình nhưng đăng nhập web lỗi: %w", err)
			}
		}
		if p.Auth == "oauth" {
			fmt.Fprintln(out, "Đã lưu token OAuth. Không cần đăng nhập web; Claude sẽ kiểm tra token ở lần chat đầu.")
		}
		if findClaude(home) == "" {
			fmt.Fprintln(out, "Máy chưa có Claude Code. Cài bằng: wj apps install claude")
		}
		fmt.Fprintln(out, "Xong. Mở phiên Claude MỚI rồi gõ /status để kiểm tra (phiên đang mở vẫn dùng kết nối cũ).")
		printOpenSessionsHint(out, home)
		if *launch {
			return runClaude(home)
		}
	}
	return nil
}

func prompt(in *bufio.Reader, out io.Writer, label string) string {
	fmt.Fprint(out, label+": ")
	line, _ := in.ReadString('\n')
	return strings.TrimSpace(line)
}

func promptSecret(input io.Reader, reader *bufio.Reader, out io.Writer, label string) (string, error) {
	if file, ok := input.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		fmt.Fprint(out, label+": ")
		value, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(out)
		return strings.TrimSpace(string(value)), err
	}
	return prompt(reader, out, label), nil
}

// askKey is the one way back from a key prompt: without a key there is nothing to do.
func askKey(input io.Reader, reader *bufio.Reader, out io.Writer, label string) (string, error) {
	value, err := promptSecret(input, reader, out, label+" (gõ sẽ không hiện ra; Enter hoặc 0 = quay lại)")
	if err == nil && (value == "" || value == "0") {
		return "", errCancelled
	}
	return value, err
}

func splitList(value string) []string {
	var result []string
	for _, v := range strings.Split(value, ",") {
		if v = strings.TrimSpace(v); v != "" && !contains(result, v) {
			result = append(result, v)
		}
	}
	return result
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func validateTargets(targets []string, p connectionProfile, profile bool) error {
	if len(targets) == 0 {
		return errors.New("chưa chọn app nào")
	}
	for _, target := range targets {
		if !contains([]string{"claude", "codex", "cursor", "opencode", "agy"}, target) {
			return fmt.Errorf("không có app %q (chọn trong: claude, codex, cursor, opencode, agy)", target)
		}
		if (p.Auth != "gateway" || profile || p.Header == "x-api-key") && target != "claude" {
			return errors.New("token OAuth, API key, đăng nhập web, profile và header x-api-key chỉ áp dụng cho Claude (-only claude). App khác: wj configure codex")
		}
	}
	return nil
}
