package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// 5-hour blocks, as the Claude plans count usage: a block starts at the hour of its
// first call and lasts 5 hours; a call more than 5 hours after the block started, or
// after the call before it, starts a new block. The block that is still running gets a
// burn rate and a projection to its end. Ported from ccusage (rust/src/blocks.rs).

const blockLength = 5 * time.Hour

type usageBlock struct {
	usageRow
	Start, End  time.Time // End = Start + 5h
	First, Last time.Time // the first and the last call
	Gap         bool      // a pause of more than 5 hours, no calls
	Active      bool
	models      map[string]int64
}

// tokens are what counts against a limit: every kind, cache included.
func (b usageBlock) tokens() int64 { return b.Total }

// blocks groups the calls of project and client (empty = all) since since into 5-hour
// blocks, the oldest first, with a gap block for each pause longer than a block.
func (s *sessionScanner) blocks(since time.Time, project, client string, now time.Time) []usageBlock {
	var calls []sessionCall
	for _, call := range s.calls {
		if call.When.Before(since) || call.When.After(now) ||
			project != "" && !strings.EqualFold(call.Project, project) ||
			client != "" && !strings.EqualFold(call.App, client) {
			continue
		}
		calls = append(calls, call)
	}
	sort.SliceStable(calls, func(i, j int) bool { return calls[i].When.Before(calls[j].When) })
	var list []usageBlock
	var cur *usageBlock
	for _, call := range calls {
		if cur != nil && (call.When.Sub(cur.Start) > blockLength || call.When.Sub(cur.Last) > blockLength) {
			list = append(list, *cur)
			if call.When.Sub(cur.Last) > blockLength {
				list = append(list, usageBlock{Start: cur.Last.Add(blockLength), End: call.When, Gap: true})
			}
			cur = nil
		}
		if cur == nil {
			start := call.When.Truncate(time.Hour)
			cur = &usageBlock{Start: start, End: start.Add(blockLength), First: call.When, models: map[string]int64{}}
		}
		cur.add(call)
		cur.Last = call.When
		cur.models[call.Model] += call.Input + call.Output + call.CacheRead + call.CacheWrite
	}
	if cur != nil {
		cur.Active = now.Sub(cur.Last) < blockLength && now.Before(cur.End)
		list = append(list, *cur)
	}
	return list
}

// burnRate is tokens per minute between the first and the last call, the same for the
// tokens without cache, and the cost per hour. ok is false when the calls are less
// than a minute apart.
func (b usageBlock) burnRate() (perMin, plainPerMin, costPerHour float64, ok bool) {
	minutes := b.Last.Sub(b.First).Minutes()
	if minutes < 1 {
		return 0, 0, 0, false
	}
	return float64(b.Total) / minutes, float64(b.Input+b.Output) / minutes, b.Cost / minutes * 60, true
}

// projection is the tokens and cost at the end of a running block, if it keeps going
// at its burn rate.
func (b usageBlock) projection(now time.Time) (tokens int64, cost float64, ok bool) {
	perMin, _, costPerHour, ok := b.burnRate()
	if !b.Active || !ok {
		return 0, 0, false
	}
	left := max(0, b.End.Sub(now).Minutes())
	return b.Total + int64(perMin*left), b.Cost + costPerHour/60*left, true
}

// blockLimit reads the -limit value: a token count, "max" (the largest finished
// block) or "" (none).
func blockLimit(text string, list []usageBlock) (int64, error) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "", "off", "0":
		return 0, nil
	case "max":
		var best int64
		for _, b := range list {
			if !b.Gap && !b.Active {
				best = max(best, b.tokens())
			}
		}
		return best, nil
	}
	return parseTokenCount(text)
}

func limitStatus(tokens, limit int64) string {
	switch warnLevel(tokens, limit) {
	case 1:
		return "SẮP CHẠM"
	case 2:
		return "VƯỢT"
	}
	return "ổn"
}

// printBlocks writes the blocks of period p: a table, then the running block in detail.
func printBlocks(out io.Writer, p usagePeriod, list []usageBlock, limitText string, now time.Time) error {
	limit, err := blockLimit(limitText, list)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Khối 5 giờ, %s (mọi agent đọc được trên máy này)\n\n", p.Label)
	var shown []usageBlock
	for _, b := range list {
		if b.End.After(p.Start) && b.Start.Before(p.End) {
			shown = append(shown, b)
		}
	}
	if len(shown) == 0 {
		fmt.Fprintln(out, "Không có lượt gọi nào trong khoảng này.")
		return nil
	}
	loc := p.Start.Location()
	clock := func(t time.Time) string {
		t = t.In(loc)
		if p.singleDay() {
			return t.Format("15:04")
		}
		return t.Format("01-02 15:04")
	}
	headers := []string{"", "BẮT ĐẦU", "GỌI CUỐI", "LƯỢT", "TỔNG", "VÀO+RA", "QUY ĐỔI", "MODEL"}
	if limit > 0 {
		headers = append(headers, "% GIỚI HẠN")
	}
	rows := [][]string{}
	for _, b := range shown {
		if b.Gap {
			row := []string{"", clock(b.Start), "nghỉ " + agoText(b.End.Sub(b.Start)), "", "", "", "", ""}
			if limit > 0 {
				row = append(row, "")
			}
			rows = append(rows, row)
			continue
		}
		mark := ""
		if b.Active {
			mark = "▶"
		}
		row := []string{mark, clock(b.Start), clock(b.Last), fmt.Sprint(b.Calls), compactTokens(b.Total),
			compactTokens(b.Input + b.Output), costText(b.usageRow), topModel(b.models)}
		if limit > 0 {
			row = append(row, percentText(b.Total, limit))
		}
		rows = append(rows, row)
	}
	left := []bool{true, true, true, false, false, false, false, true, false}
	renderTable(out, headers, left[:len(headers)], rows)
	last := shown[len(shown)-1]
	if last.Active {
		fmt.Fprintf(out, "\n▶ Khối đang chạy: %s → %s, đã %s, còn %s\n", clock(last.Start), clock(last.End),
			agoText(now.Sub(last.Start)), agoText(last.End.Sub(now)))
		if perMin, plain, costPerHour, ok := last.burnRate(); ok {
			fmt.Fprintf(out, "  Tốc độ: %s token/phút (vào+ra %s/phút), %s/giờ\n", compactTokens(int64(perMin)), compactTokens(int64(plain)),
				costText(usageRow{Cost: costPerHour}))
		}
		if tokens, cost, ok := last.projection(now); ok {
			line := fmt.Sprintf("  Dự kiến hết khối: %s token, %s", compactTokens(tokens), costText(usageRow{Cost: cost, Unpriced: last.Unpriced}))
			if limit > 0 {
				line += fmt.Sprintf(" — %s giới hạn %s: %s", percentText(tokens, limit), compactTokens(limit), limitStatus(tokens, limit))
			}
			fmt.Fprintln(out, line)
		}
		if limit > 0 {
			fmt.Fprintf(out, "  Hiện tại: %s giới hạn (%s)\n", percentText(last.Total, limit), limitStatus(last.Total, limit))
		}
	}
	fmt.Fprintln(out, "\nMột khối bắt đầu ở giờ tròn của lượt gọi đầu và dài 5 giờ (như gói Claude tính); nghỉ quá 5 giờ thì sang khối mới.")
	fmt.Fprintln(out, "TỔNG gồm cả cache; VÀO+RA là phần không cache. Giới hạn: -limit 5M, hoặc -limit max (khối lớn nhất trước đó).")
	return nil
}
