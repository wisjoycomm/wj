package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

// `wj usage` with no arguments: the report on top and a prompt below it, like Claude
// Code. The line typed at the prompt changes the view (period, filters, workers) and
// the report is drawn again.

const usageShellHelp = `Lệnh gõ ở dòng dưới (không cần gõ "wj usage"):
  1 2 3  hoặc  day  week  month                       xem theo ngày / tuần / tháng (tuần bắt đầu thứ Hai)
  prev  next  (hoặc <  >)                              kỳ trước / kỳ sau cùng loại
  today  yesterday  lastweek  lastmonth  2026-W45      tới thẳng một kỳ
  7d  30d                                              N ngày gần nhất
  2026-11-09   2026-11   2026-11-01..2026-11-09        một ngày, một tháng, một khoảng ngày
  agents  /  agents off                                bật/tắt liệt kê từng worker dưới session
  open  /  open off                                    chỉ các phiên đang mở lúc này (Claude Code): ● bận, ○ rảnh
  analytics  /  analytics off                          phân tích token đi đâu, vì sao tốn (theo kỳ đang xem)
  blocks  /  blocks off                                khối 5 giờ (như gói Claude tính): tốc độ đốt, dự kiến hết khối
  limit 5M  /  limit max  /  limit off                 giới hạn token một khối, cảnh báo từ 80%
  top 30  /  top 0  /  top auto                       số session hiện ra (0 = tất cả)
  project TÊN  /  project all                          lọc theo project
  app claude / codex / opencode / antigravity / all    lọc theo app
  reset                                                về mặc định (hôm nay, không lọc)
  help                                                 xem lại bảng này
  q  hoặc  0  hoặc Enter                               thoát`

// applyUsageCommand changes the view by one typed line. It returns a short message
// to show above the prompt (empty when the view just changed) and whether to quit.
func applyUsageCommand(v *usageView, line string, now time.Time) (message string, quit bool) {
	fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "/"))
	if len(fields) == 0 {
		return "", true
	}
	cmd := strings.ToLower(fields[0])
	arg := ""
	if len(fields) > 1 {
		arg = strings.Join(fields[1:], " ")
	}
	switch cmd {
	case "q", "quit", "exit", "0":
		return "", true
	case "help", "h", "?":
		return usageShellHelp, false
	case "reset":
		*v = usageView{Spec: "today", Top: -1, Overview: v.Overview}
		return "", false
	case "1", "2", "3", "day", "week", "month": // a kind of period, around the day in view
		u := map[string]periodUnit{"1": unitDay, "2": unitWeek, "3": unitMonth, "day": unitDay, "week": unitWeek, "month": unitMonth}[cmd]
		spec, err := specOfUnit(v.Spec, u, now)
		if err != nil {
			spec = u.command()
		}
		v.Spec = spec
		return "", false
	case "prev", "next", "<", ">":
		dir := 1
		if cmd == "prev" || cmd == "<" {
			dir = -1
		}
		spec, ok := shiftSpec(v.Spec, dir, now)
		if !ok {
			return "đang xem kỳ mới nhất, không có kỳ sau", false
		}
		v.Spec = spec
		return "", false
	case "open", "live", "running":
		switch strings.ToLower(arg) {
		case "", "on", "yes", "y":
			v.Open = true
		case "off", "no", "n":
			v.Open = false
		default:
			return "open on hoặc open off", false
		}
		return "", false
	case "analytics", "analyze", "why":
		switch strings.ToLower(arg) {
		case "", "on", "yes", "y":
			v.Analytics, v.Blocks = true, false
		case "off", "no", "n":
			v.Analytics = false
		default:
			return "analytics on hoặc analytics off", false
		}
		return "", false
	case "blocks", "block":
		switch strings.ToLower(arg) {
		case "", "on", "yes", "y":
			v.Blocks, v.Analytics = true, false
		case "off", "no", "n":
			v.Blocks = false
		default:
			return "blocks on hoặc blocks off", false
		}
		return "", false
	case "limit":
		if _, err := blockLimit(arg, nil); err != nil {
			return "limit 5M, limit max hoặc limit off", false
		}
		v.Limit = strings.ToLower(strings.TrimSpace(arg))
		if v.Limit == "off" || v.Limit == "0" {
			v.Limit = ""
		}
		return "", false
	case "agents", "agent", "workers":
		switch strings.ToLower(arg) {
		case "", "on", "yes", "y":
			v.Agents = true
		case "off", "no", "n":
			v.Agents = false
		default:
			return "agents on hoặc agents off", false
		}
		return "", false
	case "top":
		switch strings.ToLower(arg) {
		case "auto", "":
			v.Top = -1
		default:
			n, err := strconv.Atoi(arg)
			if err != nil || n < 0 {
				return "top N: N là số từ 0 trở lên (0 = tất cả), hoặc top auto", false
			}
			v.Top = n
		}
		return "", false
	case "project", "p":
		if strings.EqualFold(arg, "all") || arg == "" {
			v.Project = ""
		} else {
			v.Project = arg
		}
		return "", false
	case "app", "a":
		switch strings.ToLower(arg) {
		case "", "all":
			v.Client = ""
		case "claude", "codex", "opencode", "antigravity":
			v.Client = strings.ToLower(arg)
		default:
			return "app claude, codex, opencode, antigravity hoặc all", false
		}
		return "", false
	}
	// Anything else is a period.
	if _, err := resolvePeriod(cmd, now); err != nil {
		return fmt.Sprintf("không hiểu %q. Gõ help để xem các lệnh; kỳ hợp lệ: %s", strings.TrimSpace(line), periodHelp), false
	}
	v.Spec = cmd
	return "", false
}

func usageFooter(v usageView) string {
	state := []string{"kỳ: " + v.Spec}
	if v.Project != "" {
		state = append(state, "project: "+v.Project)
	}
	if v.Client != "" {
		state = append(state, "app: "+v.Client)
	}
	if v.Agents {
		state = append(state, "agents: bật")
	}
	if v.Blocks {
		state = append(state, "khối 5 giờ")
	}
	if v.Analytics {
		state = append(state, "phân tích")
	}
	if v.Limit != "" {
		state = append(state, "limit: "+v.Limit)
	}
	switch {
	case v.Top == 0:
		state = append(state, "top: tất cả")
	case v.Top > 0:
		state = append(state, fmt.Sprintf("top: %d", v.Top))
	}
	return strings.Join(state, " · ") + "   —   gõ: day/week/month, prev/next, 2026-11-05, agents, top 30, project TÊN, help, q"
}

var errPeriodUsage = errors.New("cách dùng: wj usage [kỳ] [-top N] [-agents] [-project TÊN] [-app claude|codex|opencode|antigravity] [-json]; wj usage blocks [kỳ] [-limit 5M]; wj usage analytics [kỳ]; kỳ: " + periodHelp)

// runUsageShell shows the report with a prompt under it. Without a terminal (a pipe,
// a script) it prints the report once and returns.
func runUsageShell(input io.Reader, out io.Writer, home string, v usageView) error {
	sc := newSessionScanner(home)
	oldest := sc.oldestSession()
	stdout, outIsFile := out.(*os.File)
	if !interactive(input) || !outIsFile || !term.IsTerminal(int(stdout.Fd())) {
		text, err := reportText(sc, v, time.Now(), 0, oldest)
		if err != nil {
			return err
		}
		_, err = io.WriteString(out, text)
		return err
	}
	if file, isFile := input.(*os.File); isFile {
		if ok, err := runUsageScreen(file, out, sc, v, oldest); ok {
			return err
		}
	}
	defer enableTerminalEscapes()()
	height := func() int {
		if _, h, err := term.GetSize(int(stdout.Fd())); err == nil && h >= 12 {
			return h
		}
		return 0
	}
	return usageLoop(input, out, sc, v, oldest, height, true)
}

// usageLoop draws the report, reads one command line, changes the view, and draws
// again until the user quits. height gives the terminal height (0 = unknown) so the
// report leaves room for the prompt; clear redraws the screen instead of scrolling.
func usageLoop(input io.Reader, out io.Writer, sc *sessionScanner, v usageView, oldest time.Time, height func() int, clear bool) error {
	reader := bufio.NewReader(input)
	message := ""
	for {
		now := time.Now()
		// Keep room under the report for the footer, a message and the prompt.
		reserve := 4
		if message != "" {
			reserve += strings.Count(message, "\n") + 1
		}
		maxLines := 0
		if h := height(); h > 0 {
			maxLines = h - reserve
		}
		text, err := reportText(sc, v, now, maxLines, oldest)
		if err != nil {
			message, text = err.Error(), ""
		}
		if clear {
			fmt.Fprint(out, "\x1b[2J\x1b[H")
		}
		fmt.Fprint(out, text)
		fmt.Fprintf(out, "\n%s\n", usageFooter(v))
		if message != "" {
			fmt.Fprintln(out, message)
			message = ""
		}
		fmt.Fprint(out, "> ")
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			fmt.Fprintln(out)
			return nil
		}
		next := v
		msg, quit := applyUsageCommand(&next, line, now)
		if quit {
			fmt.Fprintln(out)
			return nil
		}
		v, message = next, msg
	}
}
