package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Per-project metering for Claude Code: the project's personal settings file
// (.claude/settings.local.json, ignored by git) routes that project's Claude
// sessions through the local proxy, tagged with the project name. Nothing
// outside the project changes and every Claude session started there counts,
// including --continue / --resume.

// Keys this feature owns in settings.local.json env. A marker keeps "off" from
// removing proxy settings the user wrote by hand.
var projectProxyKeys = []string{"HTTPS_PROXY", "NODE_EXTRA_CA_CERTS", "NO_PROXY"}

const projectMarkerKey = "SETUP_GATEWAY_PROJECT"

func projectRoot(dir string) string {
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return dir
		}
		d = parent
	}
}

func projectSettingsPath(root string) string {
	return filepath.Join(root, ".claude", "settings.local.json")
}

func projectsIndexPath(home string) string {
	return filepath.Join(proxyDir(home), "projects.json")
}

func readJSONObject(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if json.Unmarshal(data, &doc) != nil || doc == nil {
		return nil, fmt.Errorf("file %s bị lỗi cú pháp JSON; tool không sửa nó", path)
	}
	return doc, nil
}

func writeJSONObject(path string, doc map[string]any) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, string(data)+"\n", false)
}

func loadProjectIndex(home string) map[string]string {
	index := map[string]string{}
	if data, err := os.ReadFile(projectsIndexPath(home)); err == nil {
		_ = json.Unmarshal(data, &index)
	}
	return index
}

func saveProjectIndex(home string, index map[string]string) error {
	if err := os.MkdirAll(proxyDir(home), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(projectsIndexPath(home), string(data)+"\n", false)
}

func proxyProject(args []string, input io.Reader, out io.Writer, home string) error {
	if len(args) == 0 || contains([]string{"help", "-h", "-help", "--help"}, args[0]) {
		fmt.Fprintln(out, projectHelp)
		return nil
	}
	switch args[0] {
	case "on":
		return projectOn(args[1:], out, home)
	case "off":
		return projectOff(args[1:], out, home)
	case "list", "ls":
		return projectList(out, home)
	}
	return fmt.Errorf("không hiểu lệnh %q; dùng: proxy project on | off | list", args[0])
}

const projectHelp = `Đo token theo từng project (Claude Code)

  cd vào thư mục project rồi:
    wj proxy project on        bật đo cho project này
    wj proxy project off       tắt
    wj proxy project list      các project đang bật

  Sau khi bật, cứ mở claude như bình thường trong project đó (kể cả
  claude --continue / --resume): mọi phiên đều được đếm, không cần proxy run.
  Cần proxy đang chạy: wj proxy start  (để cửa sổ đó mở).

  Tool ghi vào <project>/.claude/settings.local.json (file cá nhân, git bỏ qua),
  không đụng project khác. Codex chưa hỗ trợ cách này: dùng proxy run codex.

  Tuỳ chọn on:  -name TÊN   tên project trong bảng usage (mặc định = tên thư mục)
                -port 8877  cổng proxy start`

func projectOn(args []string, out io.Writer, home string) error {
	flags := flag.NewFlagSet("proxy project on", flag.ContinueOnError)
	flags.SetOutput(out)
	name := flags.String("name", "", "tên project trong bảng usage; mặc định = tên thư mục git")
	port := flags.Int("port", defaultProxyPort, "cổng của proxy start")
	dir := flags.String("dir", "", "thư mục project; mặc định = thư mục đang đứng")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || *port < 1 || *port > 65535 {
		return errors.New("cách dùng: wj proxy project on [-name TÊN] [-port 8877]")
	}
	start := *dir
	if start == "" {
		var err error
		if start, err = os.Getwd(); err != nil {
			return err
		}
	}
	start, err := filepath.Abs(start)
	if err != nil {
		return err
	}
	root := projectRoot(start)
	if *name == "" {
		*name = filepath.Base(root)
	}
	if !validName(*name) {
		return errors.New("tên project chỉ gồm chữ, số, _ hoặc - (dùng -name để đặt)")
	}
	if _, err := os.Stat(mitmCAPath(home)); err != nil {
		return errors.New("chưa có chứng chỉ mitmproxy; chạy trước: wj proxy setup")
	}
	path := projectSettingsPath(root)
	doc, err := readJSONObject(path)
	if err != nil {
		return err
	}
	env, _ := doc["env"].(map[string]any)
	if env == nil {
		if _, exists := doc["env"]; exists {
			return fmt.Errorf("mục env trong %s không đúng dạng; tool không sửa file", path)
		}
		env = map[string]any{}
	}
	if existing, ok := env["HTTPS_PROXY"].(string); ok && env[projectMarkerKey] == nil && existing != "" {
		return fmt.Errorf("project đã có HTTPS_PROXY=%s do bạn tự đặt; tool không ghi đè", existing)
	}
	proxy := url.URL{Scheme: "http", User: url.UserPassword(*name, "x"), Host: "127.0.0.1:" + strconv.Itoa(*port)}
	env["HTTPS_PROXY"] = proxy.String()
	env["NODE_EXTRA_CA_CERTS"] = filepath.ToSlash(mitmCAPath(home))
	env["NO_PROXY"] = "localhost,127.0.0.1,::1"
	env[projectMarkerKey] = *name
	doc["env"] = env
	if err := writeJSONObject(path, doc); err != nil {
		return err
	}
	ignored, err := ensureGitIgnored(root)
	if err != nil {
		return err
	}
	index := loadProjectIndex(home)
	index[root] = *name
	if err := saveProjectIndex(home, index); err != nil {
		return err
	}
	fmt.Fprintf(out, "Đã bật đo cho project %q\n  file: %s\n", *name, path)
	if ignored != "" {
		fmt.Fprintln(out, "  đã thêm .claude/settings.local.json vào", ignored)
	}
	if !portOpen(*port) {
		fmt.Fprintf(out, "Chú ý: proxy chưa chạy ở cổng %d. Mở cửa sổ khác: wj proxy start\n", *port)
		fmt.Fprintln(out, "Khi proxy tắt, Claude trong project này sẽ không kết nối được; tắt đo bằng: wj proxy project off")
	}
	fmt.Fprintln(out, "Mở phiên Claude MỚI trong project (claude, claude --continue...). Xem: wj proxy usage -project", *name)
	return nil
}

func projectOff(args []string, out io.Writer, home string) error {
	flags := flag.NewFlagSet("proxy project off", flag.ContinueOnError)
	flags.SetOutput(out)
	dir := flags.String("dir", "", "thư mục project; mặc định = thư mục đang đứng")
	all := flags.Bool("all", false, "tắt cho mọi project đã bật")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("cách dùng: wj proxy project off [-all]")
	}
	index := loadProjectIndex(home)
	var roots []string
	if *all {
		for root := range index {
			roots = append(roots, root)
		}
		sort.Strings(roots)
		if len(roots) == 0 {
			fmt.Fprintln(out, "Không có project nào đang bật.")
			return nil
		}
	} else {
		start := *dir
		if start == "" {
			var err error
			if start, err = os.Getwd(); err != nil {
				return err
			}
		}
		start, err := filepath.Abs(start)
		if err != nil {
			return err
		}
		roots = []string{projectRoot(start)}
	}
	var failures []error
	for _, root := range roots {
		removed, err := removeProjectProxy(root)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		delete(index, root)
		if removed {
			fmt.Fprintln(out, "Đã tắt đo:", root)
		} else {
			fmt.Fprintln(out, "Project này chưa bật đo:", root)
		}
	}
	if err := saveProjectIndex(home, index); err != nil {
		return err
	}
	return errors.Join(failures...)
}

// removeProjectProxy deletes only the keys this tool wrote, and the file when
// nothing else remains in it.
func removeProjectProxy(root string) (bool, error) {
	path := projectSettingsPath(root)
	doc, err := readJSONObject(path)
	if err != nil {
		return false, err
	}
	env, _ := doc["env"].(map[string]any)
	if env == nil || env[projectMarkerKey] == nil {
		return false, nil
	}
	for _, key := range append(projectProxyKeys, projectMarkerKey) {
		delete(env, key)
	}
	if len(env) == 0 {
		delete(doc, "env")
	}
	if len(doc) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		return true, nil
	}
	return true, writeJSONObject(path, doc)
}

func projectList(out io.Writer, home string) error {
	index := loadProjectIndex(home)
	if len(index) == 0 {
		fmt.Fprintln(out, "Chưa bật đo cho project nào. cd vào project rồi: wj proxy project on")
		return nil
	}
	var roots []string
	for root := range index {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	fmt.Fprintln(out, "Project đang bật đo (Claude Code):")
	for i, root := range roots {
		state := "đang bật"
		if doc, err := readJSONObject(projectSettingsPath(root)); err != nil {
			state = "file lỗi"
		} else if env, _ := doc["env"].(map[string]any); env == nil || env[projectMarkerKey] == nil {
			state = "đã bị gỡ tay"
		}
		fmt.Fprintf(out, "  %d. %-20s %s  (%s)\n", i+1, index[root], root, state)
	}
	if !portOpen(defaultProxyPort) {
		fmt.Fprintf(out, "Proxy chưa chạy ở cổng %d -> wj proxy start\n", defaultProxyPort)
	}
	return nil
}

// ensureGitIgnored keeps the personal settings file out of commits. It returns
// the .gitignore it changed, or "" when nothing was needed.
func ensureGitIgnored(root string) (string, error) {
	return ensureGitIgnoredEntry(root, ".claude/settings.local.json")
}

// ensureGitIgnoredEntry adds entry (a path from the repo root) to .gitignore
// unless it, or a folder that holds it, is already listed.
func ensureGitIgnoredEntry(root, entry string) (string, error) {
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return "", nil
	}
	path := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		line = strings.TrimSuffix(strings.TrimPrefix(line, "/"), "/")
		if line == entry || strings.HasPrefix(entry, line+"/") {
			return "", nil
		}
	}
	text := string(data)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += entry + "\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
