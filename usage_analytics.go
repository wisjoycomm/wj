package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// `wj usage analytics [day|week|...]`: where the tokens of a period went, and why.
// Every call sends the whole conversation again, so most tokens are the context read
// back from the cache; the sections below show what made that context big or made it
// be written again.

// compaction is one time a session's context was summarised to make room.
type compaction struct {
	When         time.Time
	Project, App string
	Session      string
	Pre, Post    int64 // context size before and after; 0 = not known
	Automatic    bool
}

func (s *sessionScanner) readClaudeCompaction(path string, raw []byte) bool {
	var line struct {
		Type      string `json:"type"`
		Subtype   string `json:"subtype"`
		UUID      string `json:"uuid"`
		Cwd       string `json:"cwd"`
		Timestamp string `json:"timestamp"`
		Meta      struct {
			Trigger string `json:"trigger"`
			Pre     int64  `json:"preTokens"`
			Post    int64  `json:"postTokens"`
		} `json:"compactMetadata"`
	}
	if json.Unmarshal(raw, &line) != nil || line.Type != "system" || line.Subtype != "compact_boundary" {
		return false
	}
	when, err := time.Parse(time.RFC3339Nano, line.Timestamp)
	if err != nil {
		return true
	}
	id, _ := sessionOf(path)
	key := line.UUID
	if key == "" {
		key = path + "\x00" + line.Timestamp
	}
	s.compactions["claude\x00"+key] = compaction{When: when, Project: s.projectOf(line.Cwd), App: "claude", Session: id,
		Pre: line.Meta.Pre, Post: line.Meta.Post, Automatic: line.Meta.Trigger != "manual"}
	return true
}

// readCodexCompaction counts a "compacted" line of a rollout. A new rollout that starts
// from the summary of an older one also begins with such a line; it is no compaction of
// its own, so only a line after a call of the same file counts.
func (s *sessionScanner) readCodexCompaction(path string, state *sessionFile, raw []byte) bool {
	var line struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
	}
	if json.Unmarshal(raw, &line) != nil || line.Type != "compacted" {
		return false
	}
	when, err := time.Parse(time.RFC3339Nano, line.Timestamp)
	if err != nil || state.lastCtx == 0 {
		return true
	}
	id, _ := sessionOf(path)
	s.compactions["codex\x00"+id+"\x00"+line.Timestamp] = compaction{When: when, Project: s.projectOf(state.cwd), App: "codex",
		Session: id, Pre: state.lastCtx, Automatic: true}
	return true
}

// Token kinds, in the order of costParts.
const (
	partInput = iota
	partOutput
	partCacheRead
	partCacheWrite
)

type contextBucket struct {
	usageRow
	Label    string
	Below    int64 // the bucket holds calls with a context under this size; 0 = no upper end
	contexts int64
}

type toolRow struct {
	Name   string
	Calls  int
	Tokens int64
	Cost   float64
}

type analyticsSession struct {
	usageRow
	ID, App, Project, Title string
	MaxCtx, contexts        int64
	Compactions, Rebuilds   int
	tools                   map[string]int
}

type usageAnalytics struct {
	Period       usagePeriod
	Total        usageRow
	Tokens       [4]int64
	Costs        [4]float64
	Thinking     int64
	ThinkingCost float64
	Buckets      []contextBucket
	Tools        []toolRow
	Rebuilds     int
	RebuildTok   int64
	RebuildCost  float64
	Compactions  []compaction
	Main, Sub    usageRow
	Hours        [24]int64
	Sessions     []analyticsSession
	Models       []modelNode
	contexts     int64
}

// cacheLife is how long a prompt cache lives after it was last used: Claude keeps it
// 5 minutes, or an hour when it was written for an hour; others about as long.
func cacheLife(call sessionCall) time.Duration {
	if call.CacheWrite1h > 0 {
		return time.Hour
	}
	return 5 * time.Minute
}

const noTool = "(chỉ trả lời, không gọi tool)"

func (s *sessionScanner) analytics(p usagePeriod, project, client string) usageAnalytics {
	a := usageAnalytics{Period: p}
	for _, b := range []struct {
		label string
		below int64
	}{{"dưới 20k", 20_000}, {"20k-50k", 50_000}, {"50k-100k", 100_000}, {"100k-200k", 200_000}, {"200k-500k", 500_000}, {"trên 500k", 0}} {
		a.Buckets = append(a.Buckets, contextBucket{Label: b.label, Below: b.below})
	}
	tools := map[string]*toolRow{}
	sessions := map[string]*analyticsSession{}
	threads := map[string][]sessionCall{} // one conversation: a session's main thread or one worker
	loc := p.Start.Location()
	for _, call := range s.calls {
		if !callInPeriod(call, p, project, client) {
			continue
		}
		a.Total.add(call)
		parts, _ := costParts(call)
		for i, n := range [4]int64{call.Input, call.Output, call.CacheRead, call.CacheWrite} {
			a.Tokens[i] += n
			a.Costs[i] += parts[i]
		}
		a.Thinking += call.Thinking
		if call.Output > 0 {
			a.ThinkingCost += parts[partOutput] * float64(min(call.Thinking, call.Output)) / float64(call.Output)
		}
		a.contexts += call.Context
		for i := range a.Buckets {
			if b := &a.Buckets[i]; b.Below == 0 || call.Context < b.Below {
				b.add(call)
				b.contexts += call.Context
				break
			}
		}
		total := call.Input + call.Output + call.CacheRead + call.CacheWrite
		cost := parts[0] + parts[1] + parts[2] + parts[3]
		names := call.Tools
		if len(names) == 0 {
			names = []string{noTool}
		}
		for _, name := range names {
			t := tools[name]
			if t == nil {
				t = &toolRow{Name: name}
				tools[name] = t
			}
			// A reply that called several tools is shared out between them.
			t.Calls++
			t.Tokens += total / int64(len(names))
			t.Cost += cost / float64(len(names))
		}
		if call.Sub {
			a.Sub.add(call)
		} else {
			a.Main.add(call)
		}
		a.Hours[call.When.In(loc).Hour()] += total
		key := call.App + "\x00" + call.Session
		se := sessions[key]
		if se == nil {
			se = &analyticsSession{ID: call.Session, App: call.App, Project: call.Project, Title: s.titles[call.Session], tools: map[string]int{}}
			sessions[key] = se
		}
		se.add(call)
		se.MaxCtx = max(se.MaxCtx, call.Context)
		se.contexts += call.Context
		for _, name := range call.Tools {
			se.tools[name]++
		}
		thread := key + "\x00" + call.Agent
		threads[thread] = append(threads[thread], call)
	}
	// A call that comes after the cache has expired writes the whole context again.
	for key, calls := range threads {
		sort.Slice(calls, func(i, j int) bool { return calls[i].When.Before(calls[j].When) })
		for i := 1; i < len(calls); i++ {
			prev, call := calls[i-1], calls[i]
			rewritten := call.CacheWrite + call.Input
			if call.When.Sub(prev.When) <= cacheLife(prev) || call.Context < 10_000 || rewritten*2 < call.Context {
				continue
			}
			parts, _ := costParts(call)
			a.Rebuilds++
			a.RebuildTok += rewritten
			a.RebuildCost += parts[partInput] + parts[partCacheWrite]
			if se := sessions[key[:strings.LastIndexByte(key, 0)]]; se != nil {
				se.Rebuilds++
			}
		}
	}
	for _, c := range s.compactions {
		if !c.When.Before(p.Start) && c.When.Before(p.End) && (project == "" || strings.EqualFold(c.Project, project)) &&
			(client == "" || strings.EqualFold(c.App, client)) {
			a.Compactions = append(a.Compactions, c)
			if se := sessions[c.App+"\x00"+c.Session]; se != nil {
				se.Compactions++
			}
		}
	}
	sort.Slice(a.Compactions, func(i, j int) bool { return a.Compactions[i].When.Before(a.Compactions[j].When) })
	for _, t := range tools {
		a.Tools = append(a.Tools, *t)
	}
	sort.Slice(a.Tools, func(i, j int) bool {
		if a.Tools[i].Tokens != a.Tools[j].Tokens {
			return a.Tools[i].Tokens > a.Tools[j].Tokens
		}
		return a.Tools[i].Name < a.Tools[j].Name
	})
	for _, se := range sessions {
		a.Sessions = append(a.Sessions, *se)
	}
	sort.Slice(a.Sessions, func(i, j int) bool {
		if a.Sessions[i].Total != a.Sessions[j].Total {
			return a.Sessions[i].Total > a.Sessions[j].Total
		}
		return a.Sessions[i].ID < a.Sessions[j].ID
	})
	a.Models = s.periodTree(p, project, client)
	return a
}

func (se analyticsSession) topTool() string {
	best, n := "", 0
	for name, c := range se.tools {
		if c > n || c == n && name < best {
			best, n = name, c
		}
	}
	return best
}

func averageOf(sum int64, n int) int64 {
	if n == 0 {
		return 0
	}
	return sum / int64(n)
}

func costShare(part, whole float64) string {
	if whole <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", part*100/whole)
}

func (s *sessionScanner) printAnalytics(out io.Writer, p usagePeriod, v usageView) {
	a := s.analytics(p, v.Project, v.Client)
	fmt.Fprintf(out, "Phân tích token, %s\n\n", p.Label)
	if a.Total.Calls == 0 {
		fmt.Fprintln(out, "Không có lượt gọi nào trong khoảng này.")
		return
	}
	cost := a.Total.Cost
	fmt.Fprintf(out, "Tổng: %d lượt, %s token, %s. Mỗi lượt gửi lại trung bình %s token ngữ cảnh.\n",
		a.Total.Calls, groupDigits(a.Total.Total), costText(a.Total), compactTokens(averageOf(a.contexts, a.Total.Calls)))

	fmt.Fprintln(out, "\n1. Token là loại gì")
	meaning := [4]string{"phần mới của prompt (chưa có trong cache)", "model viết ra: trả lời, code, lệnh tool",
		"đọc lại cuộc trò chuyện đã cache, mỗi lượt", "đưa ngữ cảnh mới vào cache (đắt hơn đọc)"}
	names := [4]string{"Vào", "Ra", "Đọc cache", "Ghi cache"}
	var rows [][]string
	for _, i := range []int{partCacheRead, partCacheWrite, partInput, partOutput} {
		rows = append(rows, []string{names[i], compactTokens(a.Tokens[i]), percentText(a.Tokens[i], a.Total.Total),
			costText(usageRow{Cost: a.Costs[i]}), costShare(a.Costs[i], cost), meaning[i]})
	}
	if a.Thinking > 0 {
		rows = append(rows, []string{"  trong đó suy nghĩ", compactTokens(a.Thinking), percentText(a.Thinking, a.Total.Total),
			costText(usageRow{Cost: a.ThinkingCost}), costShare(a.ThinkingCost, cost), "thinking / reasoning, nằm trong Ra"})
	}
	renderTable(out, []string{"LOẠI", "TOKEN", "% TOKEN", "≈USD", "% USD", "Ý NGHĨA"}, []bool{true, false, false, false, false, true}, rows)

	fmt.Fprintln(out, "\n2. Theo cỡ ngữ cảnh mỗi lượt (ngữ cảnh càng to, mỗi lượt càng đắt)")
	rows = nil
	for _, b := range a.Buckets {
		if b.Calls == 0 {
			continue
		}
		rows = append(rows, []string{b.Label, fmt.Sprint(b.Calls), compactTokens(averageOf(b.contexts, b.Calls)), compactTokens(b.Total),
			percentText(b.Total, a.Total.Total), costText(b.usageRow), costShare(b.Cost, cost)})
	}
	renderTable(out, []string{"NGỮ CẢNH", "LƯỢT", "TB/LƯỢT", "TỔNG", "% TOKEN", "≈USD", "% USD"}, []bool{true, false, false, false, false, false, false}, rows)

	fmt.Fprintln(out, "\n3. Theo tool model gọi trong lượt (lượt gọi nhiều tool thì chia đều)")
	rows = nil
	for i, t := range a.Tools {
		if i == 12 {
			rows = append(rows, []string{fmt.Sprintf("… %d tool khác", len(a.Tools)-i), "", "", "", ""})
			break
		}
		rows = append(rows, []string{t.Name, fmt.Sprint(t.Calls), compactTokens(t.Tokens), percentText(t.Tokens, a.Total.Total),
			costText(usageRow{Cost: t.Cost})})
	}
	renderTable(out, []string{"TOOL", "LƯỢT", "TOKEN", "% TOKEN", "≈USD"}, []bool{true, false, false, false, false}, rows)

	fmt.Fprintln(out, "\n4. Ngữ cảnh bị làm lại")
	fmt.Fprintf(out, "  Cache hết hạn (nghỉ quá 5 phút, hoặc 1 giờ với cache 1 giờ) rồi gửi lại: %d lần, ghi lại %s token, %s (%s chi phí)\n",
		a.Rebuilds, compactTokens(a.RebuildTok), costText(usageRow{Cost: a.RebuildCost}), costShare(a.RebuildCost, cost))
	var pre int64
	auto := 0
	for _, c := range a.Compactions {
		pre += c.Pre
		if c.Automatic {
			auto++
		}
	}
	if len(a.Compactions) > 0 {
		fmt.Fprintf(out, "  Nén ngữ cảnh (compact): %d lần (%d tự động), ngữ cảnh lúc nén trung bình %s token\n",
			len(a.Compactions), auto, compactTokens(averageOf(pre, len(a.Compactions))))
	} else {
		fmt.Fprintln(out, "  Nén ngữ cảnh (compact): không lần nào")
	}

	fmt.Fprintln(out, "\n5. Ai tiêu: phiên chính, worker (subagent), model")
	rows = [][]string{
		{"phiên chính", fmt.Sprint(a.Main.Calls), compactTokens(a.Main.Total), percentText(a.Main.Total, a.Total.Total), costText(a.Main)},
		{"worker (subagent)", fmt.Sprint(a.Sub.Calls), compactTokens(a.Sub.Total), percentText(a.Sub.Total, a.Total.Total), costText(a.Sub)},
	}
	for _, m := range a.Models {
		rows = append(rows, []string{"model " + m.Name, fmt.Sprint(m.Calls), compactTokens(m.Total), percentText(m.Total, a.Total.Total), costText(m.usageRow)})
	}
	renderTable(out, []string{"", "LƯỢT", "TOKEN", "% TOKEN", "≈USD"}, []bool{true, false, false, false, false}, rows)

	fmt.Fprintln(out, "\n6. Theo giờ trong ngày")
	printHours(out, a.Hours)

	fmt.Fprintln(out, "\n7. Session tốn nhất")
	rows = nil
	for i, se := range a.Sessions {
		if i == 5 {
			break
		}
		title := se.Title
		if title == "" {
			title = se.Project
		}
		rows = append(rows, []string{shortID(se.ID), se.App, compactTokens(se.Total), percentText(se.Total, a.Total.Total), fmt.Sprint(se.Calls),
			compactTokens(averageOf(se.contexts, se.Calls)), compactTokens(se.MaxCtx), fmt.Sprint(se.Compactions), fmt.Sprint(se.Rebuilds),
			se.topTool(), cutRunes(title, 32)})
	}
	renderTable(out, []string{"SESSION", "APP", "TỔNG", "%", "LƯỢT", "NGỮ CẢNH TB", "LỚN NHẤT", "NÉN", "CACHE LẠI", "TOOL NHIỀU", "TÊN"},
		[]bool{true, true, false, false, false, false, false, false, false, true, true}, rows)

	fmt.Fprintln(out, "\nVì sao tốn:")
	for _, line := range a.insights() {
		fmt.Fprintln(out, "  • "+line)
	}
}

// printHours draws one bar per hour, from 00 to 23.
func printHours(out io.Writer, hours [24]int64) {
	var peak int64
	for _, n := range hours {
		peak = max(peak, n)
	}
	levels := []rune(" ▁▂▃▄▅▆▇█")
	var bars, labels strings.Builder
	for h, n := range hours {
		level := 0
		if peak > 0 && n > 0 {
			level = max(1, int(n*8/peak))
		}
		bars.WriteString(" " + string(levels[level]) + string(levels[level]))
		labels.WriteString(fmt.Sprintf(" %02d", h))
	}
	busiest := 0
	for h, n := range hours {
		if n > hours[busiest] {
			busiest = h
		}
	}
	fmt.Fprintln(out, bars.String())
	fmt.Fprintln(out, labels.String())
	fmt.Fprintf(out, "  Giờ nhiều nhất: %02d:00, %s token\n", busiest, compactTokens(hours[busiest]))
}

// insights are the few plain sentences that explain the numbers above, the biggest
// cause first.
func (a usageAnalytics) insights() []string {
	var out []string
	total, cost := a.Total.Total, a.Total.Cost
	share := func(n int64) int64 { return (n*100 + total/2) / max(total, 1) }
	if read := share(a.Tokens[partCacheRead]); read >= 60 {
		out = append(out, fmt.Sprintf("%d%% token là đọc lại cache: mỗi lượt model đọc lại cả cuộc trò chuyện (trung bình %s token/lượt). "+
			"Cuộc trò chuyện càng dài, mỗi lượt càng đắt. Việc mới thì mở session mới (/clear), việc dài thì /compact sớm.",
			read, compactTokens(averageOf(a.contexts, a.Total.Calls))))
	}
	if cost > 0 && a.Costs[partCacheWrite] >= cost*0.25 {
		out = append(out, fmt.Sprintf("Ghi cache chỉ %d%% token nhưng %s chi phí: mọi thứ mới vào ngữ cảnh (kết quả tool, file đọc, worker mới mở) "+
			"bị ghi vào cache với giá cao hơn đọc khoảng 12 lần. Output tool càng ngắn, càng ít worker, phần này càng nhỏ.",
			share(a.Tokens[partCacheWrite]), costShare(a.Costs[partCacheWrite], cost)))
	}
	var big usageRow
	for _, b := range a.Buckets {
		if b.Below == 0 || b.Below > 100_000 {
			big.Calls += b.Calls
			big.Total += b.Total
			big.Cost += b.Cost
		}
	}
	if big.Calls > 0 && share(big.Total) >= 30 {
		out = append(out, fmt.Sprintf("%d lượt có ngữ cảnh trên 100k token mà chiếm %d%% token (%s chi phí). "+
			"Thường do đọc file lớn, log dài, output lệnh dài hoặc session để chạy quá lâu.", big.Calls, share(big.Total), costShare(big.Cost, cost)))
	}
	if a.Rebuilds > 0 && a.RebuildCost >= cost*0.05 {
		out = append(out, fmt.Sprintf("%d lần cache hết hạn vì nghỉ giữa chừng, phải ghi lại %s token (%s chi phí). "+
			"Quay lại một session đã nghỉ lâu thì mở session mới với tóm tắt ngắn sẽ rẻ hơn.", a.Rebuilds, compactTokens(a.RebuildTok), costShare(a.RebuildCost, cost)))
	}
	if n := len(a.Compactions); n > 0 {
		out = append(out, fmt.Sprintf("%d lần nén ngữ cảnh: các session đã chạm trần ngữ cảnh. Trước mỗi lần nén, mỗi lượt đọc lại gần đầy ngữ cảnh, đó là lúc đắt nhất.", n))
	}
	if a.Sub.Calls > 0 && share(a.Sub.Total) >= 25 {
		out = append(out, fmt.Sprintf("Worker (subagent) chiếm %d%% token: mỗi worker mở ngữ cảnh riêng và đọc lại file từ đầu. "+
			"Giao việc nhỏ, rõ cho worker; xem từng worker bằng agents.", share(a.Sub.Total)))
	}
	for _, t := range a.Tools {
		if t.Name == noTool {
			continue
		}
		if share(t.Tokens) >= 15 {
			out = append(out, fmt.Sprintf("Lượt gọi %s chiếm %d%% token (%d lượt). Kết quả tool vào thẳng ngữ cảnh và bị đọc lại mọi lượt sau: "+
				"đọc file theo đoạn, lọc output lệnh (rg -l, head, --quiet) để ngữ cảnh nhỏ lại.", t.Name, share(t.Tokens), t.Calls))
		}
		break
	}
	if a.Thinking > 0 && a.ThinkingCost >= cost*0.15 {
		out = append(out, fmt.Sprintf("Suy nghĩ (thinking) chiếm %s chi phí. Việc đơn giản thì giảm mức suy nghĩ (effort) là bớt được.", costShare(a.ThinkingCost, cost)))
	}
	if len(a.Models) > 1 && cost > 0 && a.Models[0].Cost >= cost*0.7 {
		out = append(out, fmt.Sprintf("Model %s chiếm %s chi phí. Việc nhỏ (tìm file, sửa vặt, worker) giao cho model rẻ hơn sẽ đỡ nhiều.",
			a.Models[0].Name, costShare(a.Models[0].Cost, cost)))
	}
	if len(out) == 0 {
		out = append(out, "Không có gì nổi bật: token chia khá đều giữa các phần.")
	}
	return out
}

func analyticsJSON(sc *sessionScanner, v usageView, p usagePeriod) any {
	a := sc.analytics(p, v.Project, v.Client)
	type part struct {
		Kind    string  `json:"kind"`
		Tokens  int64   `json:"tokens"`
		CostUSD float64 `json:"costUSD"`
	}
	type bucket struct {
		Label      string `json:"contextSize"`
		AvgContext int64  `json:"avgContextTokens"`
		jsonTotals
	}
	type tool struct {
		Name    string  `json:"tool"`
		Calls   int     `json:"calls"`
		Tokens  int64   `json:"tokens"`
		CostUSD float64 `json:"costUSD"`
	}
	type session struct {
		ID          string `json:"id"`
		App         string `json:"app"`
		Project     string `json:"project"`
		Title       string `json:"title,omitempty"`
		AvgContext  int64  `json:"avgContextTokens"`
		MaxContext  int64  `json:"maxContextTokens"`
		Compactions int    `json:"compactions"`
		Rebuilds    int    `json:"cacheRebuilds"`
		TopTool     string `json:"topTool,omitempty"`
		jsonTotals
	}
	type compactionJSON struct {
		When      time.Time `json:"time"`
		App       string    `json:"app"`
		Session   string    `json:"session"`
		Pre       int64     `json:"preTokens,omitempty"`
		Post      int64     `json:"postTokens,omitempty"`
		Automatic bool      `json:"automatic"`
	}
	result := struct {
		Period      jsonPeriod       `json:"period"`
		Totals      jsonTotals       `json:"totals"`
		Parts       []part           `json:"byKind"`
		Context     []bucket         `json:"byContextSize"`
		Tools       []tool           `json:"byTool"`
		Rebuilds    map[string]any   `json:"cacheRebuilds"`
		Compactions []compactionJSON `json:"compactions"`
		Main        jsonTotals       `json:"mainThreads"`
		Sub         jsonTotals       `json:"subagents"`
		Hours       [24]int64        `json:"tokensByHour"`
		Sessions    []session        `json:"sessions"`
		Insights    []string         `json:"insights"`
	}{Period: jsonPeriod{p.Label, p.Start, p.End}, Totals: totalsOf(a.Total), Main: totalsOf(a.Main), Sub: totalsOf(a.Sub), Hours: a.Hours,
		Rebuilds: map[string]any{"count": a.Rebuilds, "tokens": a.RebuildTok, "costUSD": a.RebuildCost},
		Parts:    []part{}, Context: []bucket{}, Tools: []tool{}, Compactions: []compactionJSON{}, Sessions: []session{}, Insights: a.insights()}
	for i, kind := range []string{"input", "output", "cacheRead", "cacheWrite"} {
		result.Parts = append(result.Parts, part{kind, a.Tokens[i], a.Costs[i]})
	}
	result.Parts = append(result.Parts, part{"thinking (part of output)", a.Thinking, a.ThinkingCost})
	for _, b := range a.Buckets {
		if b.Calls > 0 {
			result.Context = append(result.Context, bucket{b.Label, averageOf(b.contexts, b.Calls), totalsOf(b.usageRow)})
		}
	}
	for _, t := range a.Tools {
		result.Tools = append(result.Tools, tool{t.Name, t.Calls, t.Tokens, t.Cost})
	}
	for _, c := range a.Compactions {
		result.Compactions = append(result.Compactions, compactionJSON{c.When, c.App, c.Session, c.Pre, c.Post, c.Automatic})
	}
	for i, se := range a.Sessions {
		if v.Top > 0 && i == v.Top {
			break
		}
		result.Sessions = append(result.Sessions, session{se.ID, se.App, se.Project, se.Title, averageOf(se.contexts, se.Calls), se.MaxCtx,
			se.Compactions, se.Rebuilds, se.topTool(), totalsOf(se.usageRow)})
	}
	return result
}
