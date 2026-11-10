package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// claudeBlock is one line of a streamed Claude reply holding one content block.
func claudeBlock(id, ts, block string, read, write, output, thinking int) string {
	return fmt.Sprintf(`{"type":"assistant","cwd":"/w/game","timestamp":%q,"message":{"id":%q,"model":"claude-sonnet-4-5","content":[%s],"usage":{"input_tokens":10,"output_tokens":%d,"cache_read_input_tokens":%d,"cache_creation_input_tokens":%d,"output_tokens_details":{"thinking_tokens":%d}}}}`+"\n",
		ts, id, block, output, read, write, thinking)
}

func TestAnalyticsExplainsWhereTheTokensWent(t *testing.T) {
	s, home := testScanner(t)
	read := `{"type":"tool_use","name":"Read"}`
	writeTestFile(t, filepath.Join(home, ".claude", "projects", "p", "a.jsonl"),
		// m1 is split over three lines: thinking, text, then a Read call.
		claudeBlock("m1", "2026-11-09T01:00:00Z", `{"type":"thinking"}`, 0, 30000, 500, 200)+
			claudeBlock("m1", "2026-11-09T01:00:00Z", `{"type":"text"}`, 0, 30000, 500, 200)+
			claudeBlock("m1", "2026-11-09T01:00:00Z", read, 0, 30000, 500, 200)+
			claudeBlock("m2", "2026-11-09T01:01:00Z", `{"type":"tool_use","name":"Bash"}`, 30000, 120000, 100, 0)+
			// An hour of pause: the cache expired and the whole context is written again.
			claudeBlock("m3", "2026-11-09T02:10:00Z", `{"type":"text"}`, 0, 150000, 100, 0)+
			`{"type":"system","subtype":"compact_boundary","uuid":"c1","cwd":"/w/game","timestamp":"2026-11-09T02:20:00Z","compactMetadata":{"trigger":"auto","preTokens":160000,"postTokens":9000}}`+"\n")
	writeTestFile(t, filepath.Join(home, ".codex", "sessions", "2026", "11", "09", "rollout-2026-11-09T01-00-00-11111111-2222-3333-4444-555555555555.jsonl"),
		`{"timestamp":"2026-11-09T03:00:00Z","type":"turn_context","payload":{"cwd":"/w/game","model":"gpt-5"}}`+"\n"+
			`{"timestamp":"2026-11-09T03:00:01Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{}","call_id":"c1"}}`+"\n"+
			`{"timestamp":"2026-11-09T03:00:02Z","type":"response_item","payload":{"type":"function_call_output","call_id":"c1","output":"{\"type\":\"function_call\"} big output"}}`+"\n"+
			`{"timestamp":"2026-11-09T03:00:03Z","type":"token_usage_record","payload":{"turn_id":"t1","response_id":"r1","usage":{"input_tokens":5000,"cached_input_tokens":4000,"output_tokens":300,"reasoning_output_tokens":120}}}`+"\n"+
			`{"timestamp":"2026-11-09T03:00:04Z","type":"compacted","payload":{"message":"summary"}}`+"\n")
	s.scan(time.Time{})
	p, _ := resolvePeriod("2026-11-09", uiNow)
	a := s.analytics(p, "", "")
	if a.Total.Calls != 4 || a.Thinking != 320 {
		t.Fatalf("calls = %d, thinking = %d", a.Total.Calls, a.Thinking)
	}
	tools := map[string]toolRow{}
	for _, tr := range a.Tools {
		tools[tr.Name] = tr
	}
	if tools["Read"].Calls != 1 || tools["Bash"].Calls != 1 || tools["exec_command"].Calls != 1 || tools[noTool].Calls != 1 {
		t.Errorf("tools = %+v", a.Tools)
	}
	if a.Rebuilds != 1 || a.RebuildTok != 150010 {
		t.Errorf("rebuilds = %d, %d", a.Rebuilds, a.RebuildTok)
	}
	if len(a.Compactions) != 2 || a.Compactions[0].Pre != 160000 || a.Compactions[1].Pre != 5000 {
		t.Errorf("compactions = %+v", a.Compactions)
	}
	if b := a.Buckets[4]; b.Label != "200k-500k" && b.Calls != 0 {
		t.Errorf("buckets = %+v", a.Buckets)
	}
	text, err := reportText(s, usageView{Spec: "2026-11-09", Analytics: true}, uiNow, 0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Phân tích token", "Đọc cache", "trong đó suy nghĩ", "100k-200k", "Read", "exec_command",
		"Cache hết hạn", "Nén ngữ cảnh (compact): 2 lần", "Giờ nhiều nhất", "Vì sao tốn:"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	var b strings.Builder
	result, err := periodJSON(s, usageView{Spec: "2026-11-09", Analytics: true}, uiNow)
	if err != nil || writeJSON(&b, result) != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"byTool"`, `"tool": "Read"`, `"insights"`, `"preTokens": 160000`} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("missing %q in JSON", want)
		}
	}
}

func TestAnalyticsCommandAtThePrompt(t *testing.T) {
	v := usageView{Spec: "today", Blocks: true}
	if msg, _ := applyUsageCommand(&v, "analytics", uiNow); msg != "" || !v.Analytics || v.Blocks {
		t.Errorf("analytics: %q %+v", msg, v)
	}
	if applyUsageCommand(&v, "week", uiNow); !v.Analytics || v.Spec != "week" {
		t.Errorf("the period changes, analytics stays: %+v", v)
	}
	applyUsageCommand(&v, "analytics off", uiNow)
	if v.Analytics {
		t.Errorf("off: %+v", v)
	}
}
