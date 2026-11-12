package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

const verifyHelp = `KIỂM TRA MỘT TASK TRONG WORKTREE (cho worker Orca)

  wj verify T-01       kiểm tra task T-01 trong PLAN.md
  wj verify            không có task: chỉ chạy kiểm tra của project

Lệnh làm gì:
  1. Liệt kê file đã đổi (sửa, staged, mới), cảnh báo file ngoài Scope của task.
  2. Chạy lệnh Verify của task. Verify là - thì tự đoán:
     Unity EditMode test > script test trong package.json (hoặc tsc --noEmit) > pytest > go test.
  3. In phần cuối của output, rồi một dòng kết quả.

Dòng cuối và mã thoát:
  VERIFY: PASS                    0
  VERIFY: FAIL (...)              1   tính là một lần hỏng
  VERIFY: NOT_VERIFIED (lý do)    2   chưa kiểm tra được gì: không phải pass, không tính hỏng

Biến môi trường tuỳ chọn:
  UNITY_EXE            đường dẫn Unity editor (bỏ qua bước tự tìm)
  UNITY_TEST_PLATFORM  EditMode (mặc định) hoặc PlayMode
  VERIFY_TAIL          số dòng output in ra khi hỏng (mặc định 60)`

// exitError ends the process with this code and prints nothing more, so the
// last line of output stays the one the caller reads.
type exitError int

func (e exitError) Error() string { return "exit " + strconv.Itoa(int(e)) }

type verdict struct {
	status string
	code   int
}

func verifyFail(format string, a ...any) verdict {
	return verdict{"FAIL (" + fmt.Sprintf(format, a...) + ")", 1}
}

// verifySkip means nothing was checked: not a pass and not a strike.
func verifySkip(reason string) verdict { return verdict{"NOT_VERIFIED (" + reason + ")", 2} }

type verifier struct {
	out  io.Writer
	root string
	tail int
}

var (
	verifyIDPattern    = regexp.MustCompile(`^[A-Za-z]+-\d+$`)
	nodeCommandPattern = regexp.MustCompile(`^(npm|npx|pnpm|yarn|bun)(\s|$)`)
	ansiPattern        = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	// Child processes: run once (no watch mode), no colors, no pagers, UTF-8 output.
	verifyEnv = []string{"CI=true", "NO_COLOR=1", "FORCE_COLOR=0", "GIT_PAGER=cat", "PAGER=cat",
		"PYTHONUTF8=1", "PYTHONIOENCODING=utf-8", "PYTHONDONTWRITEBYTECODE=1"}
	// sharedFiles belong to the whole project: a worker changes them only when
	// its task's Scope names them.
	sharedFiles = []string{"AGENTS.md", "CLAUDE.md", "PLAN-ARCHIVE.md", ".claude/"}
)

func runVerify(args []string, out io.Writer) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	return runVerifyIn(args, out, dir)
}

func runVerifyIn(args []string, out io.Writer, dir string) error {
	if len(args) > 0 && contains([]string{"help", "-h", "-help", "--help"}, args[0]) {
		fmt.Fprintln(out, verifyHelp)
		return nil
	}
	if len(args) > 1 {
		return errors.New("cách dùng: wj verify [T-01]")
	}
	id := ""
	if len(args) == 1 {
		id = args[0]
	}
	v := verifier{out: out, tail: 60}
	if n, err := strconv.Atoi(os.Getenv("VERIFY_TAIL")); err == nil && n > 0 {
		v.tail = n
	}
	result := v.run(dir, id)
	fmt.Fprintln(out, "VERIFY:", result.status)
	if result.code != 0 {
		return exitError(result.code)
	}
	return nil
}

func (v *verifier) run(dir, id string) verdict {
	top, err := gitOutput(dir, "rev-parse", "--show-toplevel")
	if err != nil || strings.TrimSpace(top) == "" {
		return verifySkip("not inside a git repository")
	}
	v.root = filepath.FromSlash(strings.TrimSpace(top))

	var scope []string
	verify := ""
	if id != "" {
		if !verifyIDPattern.MatchString(id) {
			return verifySkip(fmt.Sprintf("bad task ID %q (expected e.g. T-01)", id))
		}
		data, err := os.ReadFile(filepath.Join(v.root, "PLAN.md"))
		if err != nil {
			return verifySkip("PLAN.md not found - commit it before launching workers")
		}
		row := findPlanRow(string(data), id)
		if row == nil {
			return verifySkip("task " + id + " not found in this worktree's PLAN.md - was PLAN.md committed before the worker started," +
				` and is worktree.baseRef "head" in .claude/settings.json?`)
		}
		scope = backticked(row[7])
		if command := backticked(row[8]); len(command) > 0 {
			verify = command[0]
		}
	}

	label := ""
	if id != "" {
		label = " (" + id + ")"
	}
	fmt.Fprintf(v.out, "--- changes%s ---\n", label)
	status, err := gitOutput(v.root, "-c", "core.quotepath=false", "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return verifySkip("git status failed")
	}
	var lines []string
	for _, line := range strings.Split(status, "\n") {
		if line = strings.TrimRight(line, "\r"); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return verifySkip("no changes in this worktree")
	}
	for i, line := range lines {
		if i == 30 {
			fmt.Fprintf(v.out, "... and %d more (check .gitignore?)\n", len(lines)-30)
			break
		}
		fmt.Fprintln(v.out, line)
	}
	for _, file := range changedPaths(lines) {
		switch {
		case file == "PLAN.md":
			fmt.Fprintln(v.out, "WARN PLAN.md was edited - revert it (only the orchestrator edits PLAN.md)")
		case id != "" && len(scope) > 0:
			if !inScope(file, scope) {
				fmt.Fprintln(v.out, "WARN out-of-scope:", file)
			}
		case id != "" && inScope(file, sharedFiles):
			fmt.Fprintln(v.out, "WARN shared file:", file)
		}
	}
	if id != "" && len(scope) == 0 {
		fmt.Fprintln(v.out, "NOTE", id, "has no Scope - scope check skipped")
	}

	has := func(rel string) bool {
		_, err := os.Stat(filepath.Join(v.root, filepath.FromSlash(rel)))
		return err == nil
	}
	isUnity := has("ProjectSettings/ProjectVersion.txt")
	isNode := has("package.json")
	pm, install := "npm", "npm install"
	switch {
	case has("pnpm-lock.yaml"):
		pm, install = "pnpm", "pnpm install --frozen-lockfile"
	case has("yarn.lock"):
		pm, install = "yarn", "yarn install --frozen-lockfile"
	case has("bun.lock") || has("bun.lockb"):
		pm, install = "bun", "bun install --frozen-lockfile"
	case has("package-lock.json"):
		install = "npm ci"
	}
	needsModules := isNode && (nodeCommandPattern.MatchString(verify) || (verify == "" && !isUnity))
	if needsModules && !has("node_modules") {
		return verifySkip(fmt.Sprintf("node_modules missing in this worktree - run '%s' once, then rerun", install))
	}

	if verify != "" {
		return v.check(verify)
	}
	if isUnity {
		return v.unityCheck()
	}
	if isNode {
		if script := packageTestScript(v.root); script != "" && !strings.Contains(script, "no test specified") {
			return v.check(map[string]string{"npm": "npm test --silent", "bun": "bun run test", "pnpm": "pnpm test", "yarn": "yarn test"}[pm])
		}
		if has("tsconfig.json") {
			return v.check(map[string]string{"npm": "npx --no-install tsc --noEmit -p .", "bun": "bunx tsc --noEmit -p .",
				"pnpm": "pnpm exec tsc --noEmit -p .", "yarn": "yarn exec tsc --noEmit -p ."}[pm])
		}
		return verifySkip("package.json has no test script and no tsconfig.json - set Verify in PLAN.md")
	}
	if has("pyproject.toml") || has("pytest.ini") || has("setup.cfg") || has("setup.py") {
		const pytest = "-m pytest -q --tb=short --maxfail=1"
		for _, python := range []string{".venv/Scripts/python.exe", ".venv/bin/python"} {
			if has(python) {
				return v.check(`"` + filepath.FromSlash(python) + `" ` + pytest)
			}
		}
		if _, err := exec.LookPath("pytest"); err == nil {
			return v.check("pytest -q --tb=short --maxfail=1")
		}
		return v.check("python " + pytest)
	}
	if has("go.mod") {
		return v.check("go test ./...")
	}
	return verifySkip("no Verify command in PLAN.md and no known stack (Unity / package.json / pytest / go)")
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.Output()
	return string(output), err
}

// findPlanRow returns the cells of one task without checking the rest of the
// table, so a format problem in another row never blocks a worker.
func findPlanRow(text, id string) []string {
	text = strings.TrimPrefix(text, utf8BOM)
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if cells, ok := splitPlanRow(line); ok && len(cells) == len(planColumns) && strings.EqualFold(cells[0], id) {
			return cells
		}
	}
	return nil
}

// changedPaths reads `git status --porcelain` lines. Files in .tmp/, the
// scratch folder from AGENTS.md, are not part of the change.
func changedPaths(status []string) []string {
	var paths []string
	for _, line := range status {
		if len(line) < 4 {
			continue
		}
		path := line[3:]
		if i := strings.LastIndex(path, " -> "); i >= 0 {
			path = path[i+4:]
		}
		path = strings.Trim(path, `"`)
		if !strings.HasPrefix(path, ".tmp/") {
			paths = append(paths, path)
		}
	}
	return paths
}

// inScope matches one changed file against Scope entries: an exact path, a
// folder (with or without the trailing slash) or a wildcard pattern.
func inScope(file string, scope []string) bool {
	file = strings.ReplaceAll(file, `\`, "/")
	candidates := []string{file}
	if strings.HasSuffix(file, ".meta") { // Unity: X.meta follows X
		candidates = append(candidates, strings.TrimSuffix(file, ".meta"))
	}
	for _, raw := range scope {
		entry := strings.ToLower(strings.TrimPrefix(strings.ReplaceAll(raw, `\`, "/"), "./"))
		for _, candidate := range candidates {
			candidate = strings.ToLower(candidate)
			switch {
			case strings.ContainsAny(entry, "*?["):
				if wildcardMatch(entry, candidate) {
					return true
				}
			case strings.HasSuffix(entry, "/"):
				if strings.HasPrefix(candidate, entry) {
					return true
				}
			case candidate == entry || strings.HasPrefix(candidate, entry+"/"):
				return true
			}
		}
	}
	return false
}

// wildcardMatch: * is any text (it crosses folders), ? one character, [..] a set.
func wildcardMatch(pattern, text string) bool {
	var expr strings.Builder
	expr.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			expr.WriteString(".*")
		case '?':
			expr.WriteString(".")
		case '[':
			if end := strings.IndexByte(pattern[i:], ']'); end > 1 {
				expr.WriteString(pattern[i : i+end+1])
				i += end
				continue
			}
			expr.WriteString(`\[`)
		default:
			expr.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	expr.WriteString("$")
	matcher, err := regexp.Compile(expr.String())
	return err == nil && matcher.MatchString(text)
}

func (v *verifier) check(command string) verdict {
	fmt.Fprintf(v.out, "--- check: %s ---\n", command)
	cmd := shellCommand(command)
	cmd.Dir = v.root
	cmd.Env = append(os.Environ(), verifyEnv...)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	lines := strings.Split(output.String(), "\n")
	if err == nil {
		v.writeTail(lines, 3)
		return verdict{"PASS", 0}
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return verifySkip("cannot run the check: " + err.Error())
	}
	v.writeTail(lines, v.tail)
	return verifyFail("exit %d", exit.ExitCode())
}

// cleanLines keeps the last state of \r progress lines and drops ANSI escapes
// and blank lines.
func cleanLines(lines []string) []string {
	var clean []string
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if i := strings.LastIndex(line, "\r"); i >= 0 {
			line = line[i+1:]
		}
		if line = strings.TrimRight(ansiPattern.ReplaceAllString(line, ""), " \t"); line != "" {
			clean = append(clean, line)
		}
	}
	return clean
}

func (v *verifier) writeTail(lines []string, count int) {
	clean := cleanLines(lines)
	if len(clean) > count {
		fmt.Fprintf(v.out, "... (%d earlier lines hidden)\n", len(clean)-count)
		clean = clean[len(clean)-count:]
	}
	for _, line := range clean {
		fmt.Fprintln(v.out, line)
	}
}

func packageTestScript(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return ""
	}
	var pkg struct {
		Scripts map[string]any `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return ""
	}
	script, _ := pkg.Scripts["test"].(string)
	return script
}

func unityVersion(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "ProjectSettings", "ProjectVersion.txt"))
	if err != nil {
		return ""
	}
	m := regexp.MustCompile(`(?m)^m_EditorVersion:\s*(\S+)`).FindSubmatch(data)
	if m == nil {
		return ""
	}
	return string(m[1])
}

// findUnityEditor looks in the default Unity Hub install folder, and in the
// custom one if Unity Hub has it set.
func findUnityEditor(version string) string {
	home, _ := os.UserHomeDir()
	var bases []string
	var hubConfig, editor string
	switch runtime.GOOS {
	case "windows":
		bases = []string{filepath.Join(os.Getenv("ProgramFiles"), "Unity", "Hub", "Editor")}
		hubConfig = filepath.Join(os.Getenv("APPDATA"), "UnityHub", "secondaryInstallPath.json")
		editor = filepath.Join("Editor", "Unity.exe")
	case "darwin":
		bases = []string{"/Applications/Unity/Hub/Editor"}
		hubConfig = filepath.Join(home, "Library", "Application Support", "UnityHub", "secondaryInstallPath.json")
		editor = "Unity.app/Contents/MacOS/Unity"
	default:
		bases = []string{filepath.Join(home, "Unity", "Hub", "Editor")}
		hubConfig = filepath.Join(home, ".config", "UnityHub", "secondaryInstallPath.json")
		editor = filepath.Join("Editor", "Unity")
	}
	if data, err := os.ReadFile(hubConfig); err == nil {
		var custom string
		if json.Unmarshal(data, &custom) == nil && custom != "" {
			bases = append([]string{custom}, bases...)
		}
	}
	for _, base := range bases {
		path := filepath.Join(base, version, editor)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func (v *verifier) unityCheck() verdict {
	unity := os.Getenv("UNITY_EXE")
	if unity == "" {
		version := unityVersion(v.root)
		if version == "" {
			return verifySkip("cannot read m_EditorVersion from ProjectSettings/ProjectVersion.txt")
		}
		if unity = findUnityEditor(version); unity == "" {
			return verifySkip("Unity " + version + " not found - set UNITY_EXE to the editor path")
		}
	} else if _, err := os.Stat(unity); err != nil {
		return verifySkip("UNITY_EXE not found: " + unity)
	}
	platform := os.Getenv("UNITY_TEST_PLATFORM")
	if platform == "" {
		platform = "EditMode"
	}
	tmp, err := os.MkdirTemp("", "orca-verify-")
	if err != nil {
		return verifySkip("cannot create a temp folder: " + err.Error())
	}
	results, logPath := filepath.Join(tmp, "results.xml"), filepath.Join(tmp, "unity.log")

	fmt.Fprintf(v.out, "--- check: Unity %s tests (batchmode) ---\n", platform)
	code := 0
	err = exec.Command(unity, "-batchmode", "-nographics", "-projectPath", v.root, "-runTests",
		"-testPlatform", platform, "-testResults", results, "-logFile", logPath).Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		os.RemoveAll(tmp)
		return verifySkip("cannot start Unity: " + err.Error())
	}

	if data, err := os.ReadFile(results); err == nil {
		run := parseUnityResults(data)
		fmt.Fprintf(v.out, "tests: %s total, %s passed, %s failed\n", run.total, run.passed, run.failed)
		if total, err := strconv.Atoi(run.total); err == nil && total == 0 {
			fmt.Fprintln(v.out, "NOTE no tests found - only compilation was checked")
		}
		if len(run.failures) == 0 && code == 0 {
			os.RemoveAll(tmp)
			return verdict{"PASS", 0}
		}
		for i, failure := range run.failures {
			if i == 10 {
				fmt.Fprintf(v.out, "... and %d more\n", len(run.failures)-10)
				break
			}
			fmt.Fprintf(v.out, "FAILED %s: %s\n", failure.name, failure.message)
		}
		fmt.Fprintln(v.out, "full results:", results)
		return verifyFail("%d Unity tests failed, exit %d", len(run.failures), code)
	}

	// No results file: Unity stopped before running tests.
	logData, _ := os.ReadFile(logPath)
	logText := string(logData)
	if regexp.MustCompile(`(?i)Multiple Unity instances cannot open the same project|another Unity instance is running`).MatchString(logText) {
		return verifySkip("this project folder is already open in a Unity Editor - close it and rerun")
	}
	if regexp.MustCompile(`(?i)No valid Unity Editor license|License is not active|Failed to activate`).MatchString(logText) {
		return verifySkip("Unity license problem - open Unity Hub and sign in")
	}
	var compileErrors []string
	for _, line := range regexp.MustCompile(`(?m)^.*error CS\d{4}:.*$`).FindAllString(logText, -1) {
		compileErrors = appendUnique(compileErrors, strings.TrimSpace(line))
	}
	if len(compileErrors) > 0 {
		for i, line := range compileErrors {
			if i == 15 {
				fmt.Fprintf(v.out, "... and %d more\n", len(compileErrors)-15)
				break
			}
			fmt.Fprintln(v.out, line)
		}
		fmt.Fprintln(v.out, "full log:", logPath)
		return verifyFail("%d C# compile errors", len(compileErrors))
	}
	if code == 0 {
		return verifySkip("Unity exited 0 but wrote no test results - see " + logPath)
	}
	v.writeTail(strings.Split(logText, "\n"), 40)
	fmt.Fprintln(v.out, "full log:", logPath)
	return verifyFail("Unity exit %d, no test results", code)
}

type unityFailure struct{ name, message string }

type unityRun struct {
	total, passed, failed string
	failures              []unityFailure
}

// parseUnityResults reads the NUnit XML Unity writes: the counts on <test-run>
// and, for every failed <test-case>, the first line of its failure message.
func parseUnityResults(data []byte) unityRun {
	var run unityRun
	decoder := xml.NewDecoder(bytes.NewReader(data))
	inCase, inFailure, inMessage := false, false, false
	var message strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			return run
		}
		switch element := token.(type) {
		case xml.StartElement:
			attr := func(name string) string {
				for _, a := range element.Attr {
					if a.Name.Local == name {
						return a.Value
					}
				}
				return ""
			}
			switch element.Name.Local {
			case "test-run":
				run.total, run.passed, run.failed = attr("total"), attr("passed"), attr("failed")
			case "test-case":
				if inCase = attr("result") == "Failed"; inCase {
					run.failures = append(run.failures, unityFailure{name: attr("fullname")})
				}
			case "failure":
				inFailure = inCase
			case "message":
				inMessage = inFailure
				message.Reset()
			}
		case xml.CharData:
			if inMessage {
				message.Write(element)
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "message":
				if inMessage {
					first, _, _ := strings.Cut(strings.TrimSpace(message.String()), "\n")
					run.failures[len(run.failures)-1].message = strings.TrimSpace(first)
				}
				inMessage = false
			case "failure":
				inFailure = false
			case "test-case":
				inCase = false
			}
		}
	}
}
