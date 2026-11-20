package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Tests never read ~/.wj/prices.json or go online for prices.
func init() { pricesCacheOff = true }

func TestDownloadPricesKeepsSellableModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"id":"anthropic/claude-new-9","pricing":{"prompt":"0.000002","completion":"0.00001","input_cache_read":"0.0000002",
			 "overrides":[{"min_prompt_tokens":200000,"prompt":"0.000004","completion":"0.00002"}]}},
			{"id":"anthropic/claude-new-9:batch","pricing":{"prompt":"0.000001","completion":"0.000005"}},
			{"id":"~anthropic/claude-latest","pricing":{"prompt":"0.000002","completion":"0.00001"}},
			{"id":"free/model","pricing":{"prompt":"0","completion":"0"}}]}`))
	}))
	defer server.Close()
	data, err := downloadPrices(server.URL, time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	asOf, prices, err := parsePrices(data)
	if err != nil || asOf != "2026-11-20" {
		t.Fatalf("asOf %q, %d prices, err %v:\n%s", asOf, len(prices), err, data)
	}
	for _, skipped := range []string{"claude-new-9:batch", "claude-latest", "model"} {
		if _, ok := prices[priceKey(skipped)]; ok {
			t.Errorf("%s should be left out", skipped)
		}
	}
	price := prices[priceKey("claude-new-9")]
	if price.Input != 0.000002 || price.CacheRead != 0.0000002 || price.LongFrom != 200000 || price.Long.Input != 0.000004 {
		t.Errorf("price = %+v", price)
	}
}

func TestRefreshPricesAddsSavedModelsAndRenewsADayOldFile(t *testing.T) {
	pricesCacheOff = false
	defer func() { pricesCacheOff = true }()
	oldURL, oldAsOf := pricesUpdateURL, pricesAsOf
	defer func() { pricesUpdateURL, pricesAsOf = oldURL, oldAsOf; delete(modelPrices, priceKey("claude-new-9")) }()
	t.Setenv("WJ_PRICES_UPDATE", "")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"data":[{"id":"anthropic/claude-new-9","pricing":{"prompt":"0.000002","completion":"0.00001"}}]}`))
	}))
	defer server.Close()
	pricesUpdateURL = server.URL
	home := t.TempDir()

	var status strings.Builder
	refreshPrices(home, &status, false) // no file, no download allowed: nothing happens
	if calls != 0 {
		t.Fatal("downloaded without permission")
	}
	refreshPrices(home, &status, true)
	if calls != 1 || !strings.Contains(status.String(), "xong") {
		t.Fatalf("calls %d, status %q", calls, status.String())
	}
	if _, ok := priceOf("cc/claude-new-9"); !ok {
		t.Error("the downloaded model has no price")
	}
	refreshPrices(home, nil, true) // fresh file: no new download
	if calls != 1 {
		t.Errorf("a fresh file was downloaded again (%d calls)", calls)
	}
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(filepath.Join(home, ".wj", "prices.json"), old, old); err != nil {
		t.Fatal(err)
	}
	refreshPrices(home, nil, true)
	if calls != 2 {
		t.Errorf("a day-old file was not renewed (%d calls)", calls)
	}
	t.Setenv("WJ_PRICES_UPDATE", "off")
	_ = os.Chtimes(filepath.Join(home, ".wj", "prices.json"), old, old)
	refreshPrices(home, nil, true)
	if calls != 2 {
		t.Error("WJ_PRICES_UPDATE=off still went online")
	}
}

func TestScanWithProgressCountsToTheEnd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	dir := filepath.Join(home, ".claude", "projects", "D--p")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"assistant","timestamp":"2026-11-20T01:00:00Z","message":{"model":"claude-x","usage":{"input_tokens":1,"output_tokens":2}}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "a.jsonl"), []byte(strings.Repeat(line, 10)), 0o644); err != nil {
		t.Fatal(err)
	}
	sc := newSessionScanner(home)
	if got := sc.pendingBytes(time.Time{}); got != int64(10*len(line)) {
		t.Errorf("pendingBytes = %d, want %d", got, 10*len(line))
	}
	var out strings.Builder
	scanWithProgress(sc, time.Time{}, &out)
	if !strings.Contains(out.String(), "100%") || !strings.HasSuffix(out.String(), "xong.\r\n") {
		t.Errorf("out = %q", out.String())
	}
	if sc.pendingBytes(time.Time{}) != 0 {
		t.Error("a second scan would read the file again")
	}
}
