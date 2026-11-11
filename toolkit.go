package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed toolkit/windows/*.ps1 toolkit/unix/*.sh
var bundledToolkit embed.FS

func runApp(args []string, input io.Reader, out io.Writer) error {
	if len(args) == 0 {
		return mainMenu(input, out)
	}
	switch args[0] {
	case "apps", "status":
		return newAppManager().run(args[1:], input, out)
	case "configure":
		return configureClient(args[1:], input, out)
	case "update":
		return updateSelf(args[1:], out)
	case "version", "--version", "-version":
		fmt.Fprintln(out, "wj", strings.TrimSpace(buildVersion))
		return nil
	case "auth":
		return runCLI(args[1:], input, out)
	case "tools":
		return runToolkit(args[1:], input, out)
	case "proxy":
		return runProxy(args[1:], input, out)
	case "direct":
		return runDirectCommand(args[1:], out)
	case "check":
		return runCheck(args[1:], out)
	case "doctor":
		return runDoctor(args[1:], input, out)
	case "plan":
		return runPlan(args[1:], out)
	case "verify":
		return runVerify(args[1:], out)
	case "digest":
		return runDigest(args[1:], out)
	case "init":
		return runInit(args[1:], input, out)
	case "usage":
		return runUsage(args[1:], input, out)
	case "top":
		return runTop(args[1:], out)
	case "overlay":
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		return runOverlay(args[1:], out, home)
	case "models":
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		return runModels(args[1:], input, out, home)
	case "onboard":
		if len(args) != 1 {
			return errors.New("onboard chỉ chạy dạng hỏi-đáp, không nhận tham số; dùng tools hoặc auth nếu muốn chạy bằng lệnh")
		}
		return onboard(input, out)
	case "help", "-h", "--help", "-help", "/?":
		return printHelp(args[1:], input, out)
	default:
		return runCLI(args, input, out)
	}
}

// Do not buffer ahead across menu/child-process boundaries: the child owns stdin.
func menuAnswer(input io.Reader, out io.Writer, label string) string {
	fmt.Fprint(out, label+": ")
	var line []byte
	var b [1]byte
	for {
		n, err := input.Read(b[:])
		if n > 0 {
			if b[0] == '\n' {
				break
			}
			line = append(line, b[0])
		}
		if err != nil {
			break
		}
	}
	return strings.TrimSpace(string(line))
}

const mainMenuText = `
Wisjoy wj %s

   1. Ứng dụng & môi trường (máy mới, ứng dụng, công cụ dòng lệnh)
   2. Setup project (rule, skill Orca)
   3. AI Agent (kết nối, model, xem token, kiểm tra cấu hình)
   4. Cập nhật wj

   h. Hướng dẫn nhanh      0. Thoát
`

const menuPrompt = "Chọn [0-4, h]"

type menuItem struct {
	label string
	run   func() error
}

// errCancelled means the user backed out of a question with 0. Nothing has changed.
// A menu shows itself again; a command run directly ends with it and a failing exit
// code, so a script never mistakes a cancelled step for a finished one.
var errCancelled = errors.New("đã huỷ")

func reportMenuResult(out io.Writer, err error) {
	switch {
	case errors.Is(err, errCancelled):
		fmt.Fprintln(out, "Đã huỷ.")
	case err != nil:
		fmt.Fprintln(out, "Lỗi:", err)
	}
}

// subMenu stays open after each choice; 0 or Enter goes back exactly one level.
func subMenu(title string, items []menuItem, input io.Reader, out io.Writer) error {
	for {
		fmt.Fprintln(out, "\n"+title)
		for i, item := range items {
			fmt.Fprintf(out, "  %d. %s\n", i+1, item.label)
		}
		fmt.Fprintln(out, "  0. Quay lại")
		answer := menuAnswer(input, out, fmt.Sprintf("Chọn [0-%d]", len(items)))
		if answer == "" || answer == "0" {
			return nil
		}
		found := false
		for i, item := range items {
			if answer == fmt.Sprint(i+1) {
				found = true
				reportMenuResult(out, item.run())
			}
		}
		if !found {
			fmt.Fprintln(out, "Không có mục này.")
		}
	}
}

func agentMenu(input io.Reader, out io.Writer) error {
	return subMenu("AI Agent", []menuItem{
		{"Xem token mọi project (wj top, wj overlay, wj usage)", func() error { return tokenMenu(input, out) }},
		{"Kết nối và cấu hình agent (gateway, model, kiểm tra, đo qua mitmproxy)", func() error { return agentConfigMenu(input, out) }},
	}, input, out)
}

func tokenMenu(input io.Reader, out io.Writer) error {
	return subMenu("Xem token", []menuItem{
		{"Bảng trực tiếp mọi project, theo model, cache và ≈USD (wj top)", func() error { return runTop(nil, out) }},
		{"Overlay luôn nổi trên màn hình (wj overlay)", func() error { return runApp([]string{"overlay"}, input, out) }},
		{"Đóng overlay (wj overlay stop)", func() error { return runApp([]string{"overlay", "stop"}, input, out) }},
		{"Theo ngày/tuần/tháng, từng session và worker tốn bao nhiêu, gõ lệnh để đổi kỳ (wj usage)", func() error { return runUsage(nil, input, out) }},
		{"Bảng chi tiết 30 ngày, từng model trong mỗi project", func() error {
			return runUsage([]string{"-since", "720h", "-by", "project-model"}, input, out)
		}},
	}, input, out)
}

func agentConfigMenu(input io.Reader, out io.Writer) error {
	return subMenu("Kết nối và cấu hình agent", []menuItem{
		{"Kết nối một agent (gateway, API key, đăng nhập)", func() error { return configureClient(nil, input, out) }},
		{"Đổi sang kết nối Claude đã lưu", func() error { return pickProfile(input, out) }},
		{"Lấy lại danh sách model từ gateway (wj models)", func() error { return runApp([]string{"models"}, input, out) }},
		{"Thử model nào dùng được, tốn vài token (wj models test)", func() error { return runApp([]string{"models", "test"}, input, out) }},
		{"Kiểm tra cấu hình các agent có khớp không (wj check)", func() error { return runCheck(nil, out) }},
		{"Tự kiểm tra máy, báo và sửa vấn đề (wj doctor)", func() error { return runDoctor(nil, input, out) }},
		{"Đo token qua mitmproxy, cho app khác Claude/Codex", func() error { return proxyMenu(input, out) }},
	}, input, out)
}

func appsMenu(input io.Reader, out io.Writer) error {
	tool := func(args ...string) func() error {
		return func() error { return runToolkit(args, input, out) }
	}
	return subMenu("Ứng dụng & môi trường", []menuItem{
		{"Cài máy mới (hỏi từng bước)", func() error { return onboard(input, out) }},
		{"Cài thêm ứng dụng (hiện danh sách đã cài trước khi chọn)", func() error { return appMenu("install", input, out) }},
		{"Cập nhật ứng dụng", func() error { return appMenu("update", input, out) }},
		{"Cài công cụ dòng lệnh (terminal, git, node, python...)", tool("install", "cli")},
		{"Cài quản lý cửa sổ (không bắt buộc)", tool("install", "wm")},
		{"Gỡ công cụ dòng lệnh", tool("uninstall", "cli")},
		{"Gỡ quản lý cửa sổ", tool("uninstall", "wm")},
	}, input, out)
}

func mainMenu(input io.Reader, out io.Writer) error {
	for {
		fmt.Fprintf(out, mainMenuText, strings.TrimSpace(buildVersion))
		var err error
		switch strings.ToLower(menuAnswer(input, out, menuPrompt)) {
		case "", "0", "q":
			return nil
		case "1":
			err = appsMenu(input, out)
		case "2":
			err = initMenu(input, out)
		case "3":
			err = agentMenu(input, out)
		case "4":
			if yes(menuAnswer(input, out, "Tải bản wj mới nhất? [y/N, Enter = huỷ]")) {
				err = updateSelf(nil, out)
			}
		case "h", "?", "help":
			err = printHelp(nil, input, out)
		default:
			fmt.Fprintln(out, "Không có mục này. Gõ số từ 0 đến 4, hoặc h để xem hướng dẫn.")
		}
		reportMenuResult(out, err)
	}
}

// yes accepts y/yes and the Vietnamese c/có.
func yes(answer string) bool {
	return contains([]string{"y", "yes", "c", "có", "co"}, strings.ToLower(strings.TrimSpace(answer)))
}

func appMenu(action string, input io.Reader, out io.Writer) error {
	m := newAppManager()
	if err := m.run(nil, input, out); err != nil {
		return err
	}
	answer := menuAnswer(input, out, "Gõ số ứng dụng, nhiều số cách nhau dấu cách (vd 1 2 8, 1-3), all = tất cả, Enter = bỏ qua, 0 = quay lại")
	if answer == "" {
		return nil
	}
	if answer == "0" {
		return errCancelled
	}
	ids, err := parseChoices(answer, appIDs())
	if err != nil {
		return err
	}
	return m.run([]string{action, strings.Join(ids, ",")}, input, out)
}

func appIDs() []string {
	var ids []string
	for _, app := range appCatalog {
		ids = append(ids, app.ID)
	}
	return ids
}

func onboard(input io.Reader, out io.Writer) error {
	fmt.Fprintln(out, "Cài máy mới: tool hỏi từng phần. Enter = bỏ qua phần đó, 0 = dừng và quay lại.")
	// wants is one step's question: 0 stops the whole walk-through, not just the step.
	wants := func(label string) (bool, error) {
		answer := menuAnswer(input, out, label+" [y/N, Enter = bỏ qua, 0 = dừng]")
		if answer == "0" {
			return false, errCancelled
		}
		return yes(answer), nil
	}
	fmt.Fprintln(out, "\nBước 1/4 - Bộ công cụ dòng lệnh (terminal đẹp, git, node, python, ripgrep...)")
	install, err := wants("Cài không?")
	if err != nil {
		return err
	}
	if install {
		if err := runToolkit([]string{"install", "cli"}, input, out); err != nil {
			return err
		}
	}
	fmt.Fprintln(out, "\nBước 2/4 - Ứng dụng lập trình")
	if err := appMenu("install", input, out); errors.Is(err, errCancelled) {
		return err
	} else if err != nil {
		fmt.Fprintln(out, "Có ứng dụng cài chưa xong:", err)
	}
	fmt.Fprintln(out, "\nBước 3/4 - Quản lý cửa sổ kiểu i3 (không bắt buộc, đổi phím tắt và cách xếp cửa sổ)")
	install, err = wants("Cài không?")
	if err != nil {
		return err
	}
	if install {
		if err := runToolkit([]string{"install", "wm"}, input, out); err != nil {
			return err
		}
	}
	fmt.Fprintln(out, "\nBước 4/4 - Kết nối / đăng nhập ứng dụng")
	for {
		connect, err := wants("Kết nối một ứng dụng?")
		if err != nil {
			return err
		}
		if !connect {
			fmt.Fprintln(out, "Xong. Mở terminal mới để nhận PATH mới. Gõ wj help khi cần.")
			return nil
		}
		reportMenuResult(out, configureClient(nil, input, out))
	}
}

func toolkitScript(platform, action, module string) (string, error) {
	if action != "install" && action != "uninstall" {
		return "", errors.New("chỉ có install hoặc uninstall")
	}
	if module != "cli" && module != "wm" {
		return "", errors.New("chỉ có cli (công cụ dòng lệnh) hoặc wm (quản lý cửa sổ)")
	}
	switch platform {
	case "windows":
		return "toolkit/windows/" + action + "-" + module + ".ps1", nil
	case "darwin", "linux":
		return "toolkit/unix/" + action + "-" + module + ".sh", nil
	default:
		return "", fmt.Errorf("toolkit is not supported on %s", platform)
	}
}

func toolkitList(out io.Writer) {
	fmt.Fprintln(out, "Bundled tools from Bo_cong_cu:\n  cli group 1: terminal, Starship, Nerd Font, fzf, zoxide; PowerShell/Clink on Windows\n  cli group 2: ripgrep, fd, bat, eza, jq, lazygit, delta, btop, GitHub CLI\n  cli group 3: Git, fnm/Node, uv/Python, Neovim, Antigravity IDE; optional Docker\n  cli group 4: Claude Code, Codex CLI, Antigravity CLI\n  wm Windows: Komorebi + whkd, i3-style keys, monitor workspaces and bars\n  wm macOS: AeroSpace and i3-style keys\n  wm Linux: configure an existing i3/Hyprland installation\n\nBoth modules include uninstall. Package availability depends on your OS.\nUse -dry-run to preview an action without extracting or executing scripts.")
}

func runToolkit(args []string, input io.Reader, out io.Writer) error {
	if len(args) == 0 || args[0] == "list" || args[0] == "-help" {
		toolkitList(out)
		return nil
	}
	if len(args) < 2 {
		return errors.New("cách dùng: wj tools install|uninstall cli|wm [-groups 1,2,4] [-dry-run]; xem: wj help tools")
	}
	action, module := args[0], args[1]
	script, err := toolkitScript(runtime.GOOS, action, module)
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("tools", flag.ContinueOnError)
	flags.SetOutput(out)
	groups := flags.String("groups", "", "CLI package groups, comma-separated: 1 terminal, 2 utilities, 3 development, 4 agents")
	dry := flags.Bool("dry-run", false, "show the bundled action without running it")
	if err := flags.Parse(args[2:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("có tham số không hiểu; xem: wj help tools")
	}
	selected := splitList(*groups)
	if len(selected) > 0 && module != "cli" {
		return errors.New("-groups chỉ dùng với cli")
	}
	for _, group := range selected {
		if !contains([]string{"1", "2", "3", "4"}, group) {
			return errors.New("-groups chỉ nhận các số 1,2,3,4 cách dấu phẩy")
		}
	}
	fmt.Fprintf(out, "%s %s on %s using bundled %s\n", action, module, runtime.GOOS, script)
	if module == "cli" {
		fmt.Fprintln(out, "Updates the toolkit's marked shell-profile block, command aliases, terminal styling and Git defaults.")
	} else {
		fmt.Fprintln(out, "Changes window-manager configuration, shortcuts, workspace layout and desktop/taskbar integration.")
	}
	if len(selected) > 0 {
		fmt.Fprintln(out, "Selected CLI groups:", strings.Join(selected, ", "))
	}
	if *dry {
		fmt.Fprintln(out, "Dry run: no extraction, package installs, configuration changes or processes.")
		return nil
	}
	if !yes(menuAnswer(input, out, "Tiếp tục? [y/N, Enter = huỷ]")) {
		fmt.Fprintln(out, "Đã huỷ.")
		return nil
	}
	data, err := bundledToolkit.ReadFile(script)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "wisjoy-toolkit-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, filepath.Base(script))
	if runtime.GOOS == "windows" {
		data = append([]byte{0xef, 0xbb, 0xbf}, data...)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", path)
	} else {
		cmd = exec.Command("bash", path)
	}
	cmd.Env = cleanClaudeEnvironment(os.Environ())
	// The CLI flags own group selection; do not silently inherit an unattended selection.
	var env []string
	for _, value := range cmd.Env {
		if !strings.HasPrefix(strings.ToUpper(value), "WJ_") {
			env = append(env, value)
		}
	}
	cmd.Env = append(env, "WJ_GROUPS="+strings.Join(selected, " "))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = input, out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s did not complete; review package results above: %w", action, module, err)
	}
	refreshProcessPath()
	fmt.Fprintln(out, "Xong. Mở terminal mới để nhận PATH/profile mới.")
	return nil
}
