package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Each machine gets its own ID prefix (SON-01, VIE-01...), so two people who plan
// on two branches never hand out the same ID. The prefix of this clone lives in
// git config wj.planPrefix; the list of who owns which prefix is committed in
// .claude/plan-prefixes, so a clash shows up as a merge conflict in that file.
const planPrefixFile = ".claude/plan-prefixes"

// planLegacyPrefix is the prefix of every ID from before per-machine prefixes.
// Nobody can claim it: its numbers are shared by everyone.
const planLegacyPrefix = "T"

var planPrefixPattern = regexp.MustCompile(`^[A-Z]{2,4}$`)

type planPrefixEntry struct {
	Line           int
	Prefix, Holder string
}

// readPlanPrefixes parses .claude/plan-prefixes: one "PREFIX holder" per line,
// # starts a comment. A missing file means the project has no prefixes yet.
func readPlanPrefixes(root string) ([]planPrefixEntry, bool) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(planPrefixFile)))
	if err != nil {
		return nil, false
	}
	var entries []planPrefixEntry
	for i, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		entries = append(entries, planPrefixEntry{Line: i + 1, Prefix: fields[0], Holder: strings.Join(fields[1:], " ")})
	}
	return entries, true
}

// planPrefixIssues lists bad or duplicate prefixes in .claude/plan-prefixes.
func planPrefixIssues(entries []planPrefixEntry) []string {
	var issues []string
	seen := map[string]int{}
	for _, e := range entries {
		where := fmt.Sprintf("%s dòng %d", planPrefixFile, e.Line)
		switch {
		case e.Prefix == planLegacyPrefix:
			issues = append(issues, fmt.Sprintf("%s: prefix %s dành cho ID cũ, chọn prefix khác", where, e.Prefix))
		case !planPrefixPattern.MatchString(e.Prefix):
			issues = append(issues, fmt.Sprintf("%s: prefix %q phải là 2-4 chữ in hoa", where, e.Prefix))
		}
		if prev, dup := seen[e.Prefix]; dup {
			issues = append(issues, fmt.Sprintf("%s: prefix %s trùng với dòng %d; một người đổi prefix khác", where, e.Prefix, prev))
		} else {
			seen[e.Prefix] = e.Line
		}
	}
	return issues
}

func gitConfig(root string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", root, "config"}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

// machinePlanPrefix is the prefix this clone hands out, or "" when none is set.
func machinePlanPrefix(root string) string {
	p, err := gitConfig(root, "--get", "wj.planPrefix")
	if err != nil || !planPrefixPattern.MatchString(p) || p == planLegacyPrefix {
		return ""
	}
	return p
}

// nextPlanIDFor is the next free ID with the given prefix, counting PLAN.md and
// the archive.
func nextPlanIDFor(prefix string, tasks []planTask, archive string) string {
	highest := 0
	ids := append(taskIDs(tasks), regexp.MustCompile(`\b`+regexp.QuoteMeta(prefix)+`-\d+\b`).FindAllString(archive, -1)...)
	for _, id := range ids {
		rest, ok := strings.CutPrefix(id, prefix+"-")
		if !ok {
			continue
		}
		if n, err := strconv.Atoi(rest); err == nil && n > highest {
			highest = n
		}
	}
	return fmt.Sprintf("%s-%02d", prefix, highest+1)
}

// planPrefix shows (no argument) or claims (one argument) this machine's prefix.
func planPrefix(out io.Writer, root string, args []string) error {
	entries, _ := readPlanPrefixes(root)
	if len(args) == 0 {
		mine := machinePlanPrefix(root)
		if mine == "" {
			fmt.Fprintln(out, "Máy này chưa có prefix. Chọn một: wj plan prefix ABC")
		} else {
			fmt.Fprintln(out, "Prefix của máy này:", mine)
		}
		for _, e := range entries {
			fmt.Fprintf(out, "  %-4s %s\n", e.Prefix, e.Holder)
		}
		for _, issue := range planPrefixIssues(entries) {
			fmt.Fprintln(out, "  LỖI:", issue)
		}
		return nil
	}
	if len(args) != 1 {
		return errors.New("cách dùng: wj plan prefix [ABC]")
	}
	prefix := strings.ToUpper(strings.TrimSpace(args[0]))
	if prefix == planLegacyPrefix {
		return fmt.Errorf("prefix %s dành cho ID cũ, chọn prefix khác", prefix)
	}
	if !planPrefixPattern.MatchString(prefix) {
		return fmt.Errorf("prefix %q phải là 2-4 chữ in hoa, vd SON", prefix)
	}
	holder, _ := gitConfig(root, "--get", "user.name")
	if holder == "" {
		holder = "?"
	}
	registered := false
	for _, e := range entries {
		if e.Prefix != prefix {
			continue
		}
		if e.Holder != holder {
			return fmt.Errorf("prefix %s đã thuộc về %s (%s dòng %d); chọn prefix khác", prefix, e.Holder, planPrefixFile, e.Line)
		}
		registered = true
	}
	if !registered {
		path := filepath.Join(root, filepath.FromSlash(planPrefixFile))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		old, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		text := string(old)
		if text == "" {
			text = "# Prefix ID của PLAN.md cho từng máy: PREFIX người giữ. Thêm bằng: wj plan prefix ABC\n"
		} else if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		if err := os.WriteFile(path, []byte(text+prefix+" "+holder+"\n"), 0o644); err != nil {
			return fmt.Errorf("không ghi được %s: %w", planPrefixFile, err)
		}
	}
	if _, err := gitConfig(root, "wj.planPrefix", prefix); err != nil {
		return fmt.Errorf("không đặt được git config wj.planPrefix: %w", err)
	}
	fmt.Fprintf(out, "Máy này dùng prefix %s: task mới là %s-01, %s-02... Commit %s để người khác thấy.\n", prefix, prefix, prefix, planPrefixFile)
	return nil
}

// printNextPlanID prints the NEXT ID line of wj plan, in this machine's prefix
// when it has one, and says what is wrong with the prefix setup.
func printNextPlanID(out io.Writer, root string, tasks []planTask, archive string) {
	entries, hasFile := readPlanPrefixes(root)
	if mine := machinePlanPrefix(root); mine != "" {
		fmt.Fprintf(out, "NEXT ID: %s (prefix %s của máy này)\n", nextPlanIDFor(mine, tasks, archive), mine)
	} else {
		fmt.Fprintln(out, "NEXT ID:", nextPlanID(tasks, archive))
		if hasFile {
			fmt.Fprintf(out, "PREFIX: máy này chưa có prefix riêng, ID T- dễ trùng với máy khác; chọn một chưa có trong %s: wj plan prefix ABC\n", planPrefixFile)
		}
	}
	for _, issue := range planPrefixIssues(entries) {
		fmt.Fprintln(out, "PREFIX:", issue)
	}
}

// planPrefixCheck reports prefix problems for wj plan check.
func planPrefixCheck(out io.Writer, root string) error {
	entries, _ := readPlanPrefixes(root)
	issues := planPrefixIssues(entries)
	for _, issue := range issues {
		fmt.Fprintln(out, "  ", issue)
	}
	if len(issues) > 0 {
		return fmt.Errorf("%s có %d lỗi", planPrefixFile, len(issues))
	}
	return nil
}
