package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Claude Code writes a small file for every running session, ~/.claude/sessions/<pid>.json,
// with its pid, session id, working folder, and whether it is answering ("busy") or
// waiting for the user ("idle"). Reading these is how wj knows which sessions are open.
// Codex and OpenCode write nothing like it, so their open sessions are not seen.

// liveSession is a Claude Code session whose process is running.
type liveSession struct {
	ID, Project, Cwd string
	Status, Kind     string // status: busy or idle; kind: interactive or bg
	PID              int
	Started          time.Time
}

type sessionMarker struct {
	PID       int     `json:"pid"`
	SessionID string  `json:"sessionId"`
	Cwd       string  `json:"cwd"`
	Status    string  `json:"status"`
	Kind      string  `json:"kind"`
	StartedAt flexInt `json:"startedAt"` // ms since the epoch
	ProcStart flexInt `json:"procStart"` // a string of digits in the files Claude Code writes
	PIDDomain string  `json:"pidDomain"`
}

// filetimeTime turns a Windows FILETIME (100 ns steps since 1601) into a time.
func filetimeTime(ft int64) time.Time {
	const epochDiff = 116444736000000000
	return time.Unix(0, (ft-epochDiff)*100)
}

// sameProcess is false when the pid now belongs to a different process than the one
// that wrote the marker (Windows reuses pids). Where the marker or the system does not
// give a start time, the pid is trusted.
func sameProcess(m sessionMarker) bool {
	if runtime.GOOS != "windows" || int64(m.ProcStart) <= 0 || !strings.HasPrefix(m.PIDDomain, "win32:") {
		return true
	}
	started, ok := processStartTime(m.PID)
	if !ok {
		return true
	}
	diff := started.Sub(filetimeTime(int64(m.ProcStart)))
	return diff > -2*time.Second && diff < 2*time.Second
}

// refreshLive reads the markers again. It is cheap: a few small files and a process
// check each.
func (s *sessionScanner) refreshLive() {
	paths := s.claudeSessionMarkers()
	var live []liveSession
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var m sessionMarker
		if json.Unmarshal(data, &m) != nil || m.PID <= 0 || m.Cwd == "" || !processAlive(m.PID) || !sameProcess(m) {
			continue
		}
		l := liveSession{ID: m.SessionID, Project: s.projectOf(m.Cwd), Cwd: m.Cwd, Status: m.Status, Kind: m.Kind, PID: m.PID}
		if int64(m.StartedAt) > 0 {
			l.Started = time.UnixMilli(int64(m.StartedAt))
		}
		live = append(live, l)
	}
	sort.Slice(live, func(i, j int) bool {
		if live[i].Project != live[j].Project {
			return live[i].Project < live[j].Project
		}
		return live[i].PID < live[j].PID
	})
	s.live, s.liveByID = live, map[string]liveSession{}
	stamp := fnv.New32a()
	for _, l := range live {
		if l.ID != "" {
			s.liveByID[l.ID] = l
		}
		fmt.Fprintf(stamp, "%d|%s|%s;", l.PID, l.ID, l.Status)
	}
	s.liveStamp = stamp.Sum32()
}

// keepOpen keeps the sessions that are open now, in their order. Sessions of other
// apps are dropped: only Claude Code writes the markers.
func keepOpen(sessions []sessionRow, live map[string]liveSession) []sessionRow {
	kept := make([]sessionRow, 0, len(sessions))
	for _, se := range sessions {
		if _, open := live[se.ID]; open && se.Client == "claude" {
			kept = append(kept, se)
		}
	}
	return kept
}

// totalOf adds up the sessions' own totals.
func totalOf(sessions []sessionRow) usageRow {
	var total usageRow
	for _, se := range sessions {
		total.Calls += se.Calls
		total.Input += se.Input
		total.Output += se.Output
		total.CacheRead += se.CacheRead
		total.CacheWrite += se.CacheWrite
		total.Total += se.Total
		total.Cost += se.Cost
		total.Unpriced += se.Unpriced
	}
	return total
}

// liveMark is the sign put after a session id: ● answering, ○ open and waiting.
func liveMark(status string) string {
	switch status {
	case "":
		return ""
	case "busy":
		return " ●"
	}
	return " ○"
}

func statusText(status string) string {
	switch status {
	case "busy":
		return "đang trả lời"
	case "idle":
		return "rảnh"
	case "":
		return "?"
	}
	return status
}

func kindText(kind string) string {
	switch kind {
	case "interactive":
		return "cửa sổ"
	case "bg":
		return "nền"
	case "":
		return "-"
	}
	return kind
}

// printLiveSessions writes the block of open sessions. tokens is what each used in the
// period shown, so an open session that did nothing yet shows 0.
func printLiveSessions(out io.Writer, live []liveSession, tokens map[string]int64, now time.Time) {
	if len(live) == 0 {
		return
	}
	fmt.Fprintf(out, "Phiên Claude Code đang mở (%d):  ● đang trả lời, ○ rảnh\n", len(live))
	rows := make([][]string, 0, len(live))
	for _, l := range live {
		since := "-"
		if !l.Started.IsZero() {
			since = l.Started.Local().Format("01-02 15:04")
			if l.Started.Local().Format("2006-01-02") == now.Format("2006-01-02") {
				since = l.Started.Local().Format("15:04")
			}
		}
		rows = append(rows, []string{strings.TrimSpace(liveMark(l.Status)), shortID(l.ID), l.Project, statusText(l.Status), kindText(l.Kind), since, compactTokens(tokens[l.ID])})
	}
	renderTable(out, []string{"", "SESSION", "PROJECT", "TRẠNG THÁI", "LOẠI", "MỞ TỪ", "TOKEN KỲ NÀY"},
		[]bool{true, true, true, true, true, true, false}, rows)
	fmt.Fprintln(out, "(Codex và OpenCode không ghi lại phiên đang mở nên chưa nhận biết được.)")
	fmt.Fprintln(out)
}

// flexInt reads a whole number written either as a JSON number or as a string of
// digits: Claude Code writes procStart as a string. Anything else reads as 0.
type flexInt int64

func (f *flexInt) UnmarshalJSON(data []byte) error {
	text := strings.Trim(strings.TrimSpace(string(data)), `"`)
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexInt(n)
	return nil
}

// printOpenSessionsHint is shown after the provider was changed. Sessions that were open
// keep the old connection until they are restarted, so it lists them with the command
// that reopens each one on the new connection. It closes and opens nothing itself.
func printOpenSessionsHint(out io.Writer, home string) {
	s := newSessionScanner(home)
	s.refreshLive()
	var mine []liveSession
	for _, l := range s.live {
		if l.ID != "" {
			mine = append(mine, l)
		}
	}
	if len(mine) == 0 {
		return
	}
	fmt.Fprintf(out, "\nCó %d phiên Claude đang mở, vẫn dùng kết nối cũ cho tới khi mở lại (wj không tự đóng phiên nào):\n", len(mine))
	for _, l := range mine {
		note := "rảnh, mở lại được ngay"
		if l.Status == "busy" {
			note = "đang trả lời, đợi nó xong rồi hẵng đóng"
		}
		fmt.Fprintf(out, "  %-9s %-18s %s\n", shortID(l.ID)+liveMark(l.Status), l.Project, note)
		fmt.Fprintf(out, "    claude --resume %s\n", l.ID)
	}
	fmt.Fprintln(out, "Xem trạng thái hiện tại: wj usage rồi gõ open")
}
