package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// `wj usage prune` deletes old Claude Code and Codex session files whose calls are
// already in the usage store, so the report keeps them after the files are gone.

func runUsagePrune(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("usage prune", flag.ContinueOnError)
	flags.SetOutput(out)
	older := flags.String("older", "30d", "chỉ xoá file phiên cũ hơn mức này, ví dụ 30d, 90d, 720h")
	yes := flags.Bool("yes", false, "xoá thật; không có cờ này chỉ in những gì sẽ xoá")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("wj usage prune không nhận %q; dùng -older 30d, -yes", flags.Arg(0))
	}
	age, err := parseAge(*older)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return pruneSessions(newSessionScanner(home), time.Now().Add(-age), *yes, out)
}

// parseAge reads 30d as 30 days, anything else as a Go duration (720h).
func parseAge(text string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(text, "d"); ok {
		if n, err := strconv.Atoi(days); err == nil && n >= 0 {
			return time.Duration(n) * 24 * time.Hour, nil
		}
	} else if d, err := time.ParseDuration(text); err == nil && d >= 0 {
		return d, nil
	}
	return 0, fmt.Errorf("-older %q: cần dạng 30d hoặc 720h", text)
}

// pruneSessions deletes, or with yes=false only lists, the session files that are older
// than cutoff, fully read into the store and not part of an open session.
func pruneSessions(s *sessionScanner, cutoff time.Time, yes bool, out io.Writer) error {
	// Read what was appended first, and save it: a file is deleted only once the store
	// on disk holds all of it.
	s.scanFiles(time.Time{})
	if s.storeDirty {
		if err := s.writeStore(); err != nil {
			return fmt.Errorf("không xoá gì vì chưa lưu được store: %w", err)
		}
		s.storeDirty = false
	}
	s.refreshLive()
	var roots []string
	for _, root := range s.fileRoots() {
		if real, err := filepath.EvalSymlinks(root); err == nil {
			roots = append(roots, real)
		}
	}
	type victim struct {
		path string
		size int64
		mod  time.Time
	}
	var list []victim
	for path, state := range s.files {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(path), ".jsonl") ||
			!info.ModTime().Before(cutoff) || state.offset != info.Size() || !state.modTime.Equal(info.ModTime()) {
			continue
		}
		if id, _ := sessionOf(path); s.liveByID[id].ID != "" {
			continue
		}
		if !insideRoots(path, roots) {
			continue
		}
		list = append(list, victim{path, info.Size(), info.ModTime()})
	}
	if len(list) == 0 {
		fmt.Fprintf(out, "Không có file phiên nào cũ hơn %s đã lưu đủ trong store.\n", cutoff.Format("2006-01-02"))
		return nil
	}
	sort.Slice(list, func(i, j int) bool { return list[i].mod.Before(list[j].mod) })
	var total int64
	for _, v := range list {
		total += v.size
	}
	span := fmt.Sprintf("%s → %s", list[0].mod.Format("2006-01-02"), list[len(list)-1].mod.Format("2006-01-02"))
	if !yes {
		fmt.Fprintf(out, "Sẽ xoá %d file phiên, %.1f MB, sửa lần cuối %s. Chạy lại với -yes để xoá.\n"+
			"Số token vẫn giữ trong store; phiên đã xoá không resume được nữa.\n", len(list), mb(total), span)
		return nil
	}
	var freed int64
	deleted, failed := 0, 0
	for _, v := range list {
		if err := os.Remove(v.path); err != nil {
			fmt.Fprintf(out, "Không xoá được %s: %v\n", v.path, err)
			failed++
			continue
		}
		deleted++
		freed += v.size
		dir := filepath.Dir(v.path)
		if filepath.Base(dir) == "subagents" {
			// The description beside it is already in the store; without it the folder can empty.
			_ = os.Remove(strings.TrimSuffix(v.path, filepath.Ext(v.path)) + ".meta.json")
			_ = os.Remove(dir) // fails, as wanted, while other files are left
		}
	}
	fmt.Fprintf(out, "Đã xoá %d file phiên, giải phóng %.1f MB.\n", deleted, mb(freed))
	if failed > 0 {
		return fmt.Errorf("%d file không xoá được", failed)
	}
	return nil
}

func mb(n int64) float64 { return float64(n) / (1 << 20) }

// insideRoots is true when path, with links resolved, is inside one of the roots.
func insideRoots(path string, roots []string) bool {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	for _, root := range roots {
		if rel, err := filepath.Rel(root, real); err == nil && rel != "." && rel != ".." &&
			!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}
