package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The embedded prices.json only knows the models of its release day. The token
// commands also keep a copy of OpenRouter's public list in ~/.wj/prices.json and
// renew it once a day, so a model released after the build gets a price too.
// WJ_PRICES_UPDATE=off keeps wj offline and uses the embedded list only.

var pricesUpdateURL = "https://openrouter.ai/api/v1/models"

// pricesCacheOff keeps tests on the embedded prices only.
var pricesCacheOff bool

const pricesMaxAge = 24 * time.Hour

func pricesCachePath(home string) string { return filepath.Join(home, ".wj", "prices.json") }

// refreshPrices loads the saved list, downloads a new one (when download is set) if it
// is a day old or missing, and adds its models on top of the embedded ones. It never fails: without
// a network the last saved or the embedded prices stay. status says what happened.
func refreshPrices(home string, status io.Writer, download bool) {
	if pricesCacheOff {
		return
	}
	path := pricesCachePath(home)
	saved, _ := os.ReadFile(path)
	info, statErr := os.Stat(path)
	fresh := statErr == nil && time.Since(info.ModTime()) < pricesMaxAge
	if download && !fresh && pricesUpdateURL != "" && !strings.EqualFold(os.Getenv("WJ_PRICES_UPDATE"), "off") {
		if status != nil {
			fmt.Fprint(status, "Đang cập nhật bảng giá model (OpenRouter)...")
		}
		data, err := downloadPrices(pricesUpdateURL, time.Now())
		switch {
		case err == nil:
			if os.MkdirAll(filepath.Dir(path), 0o755) == nil && os.WriteFile(path, data, 0o644) == nil {
				saved = data
			}
		case len(saved) > 0:
			// Try again tomorrow, not on every start while offline.
			_ = os.Chtimes(path, time.Now(), time.Now())
		}
		if status != nil {
			if err != nil {
				fmt.Fprintf(status, " không được (%v), dùng giá đã lưu.\r\n", err)
			} else {
				fmt.Fprint(status, " xong.\r\n")
			}
		}
	}
	if len(saved) == 0 {
		return
	}
	asOf, prices, err := parsePrices(saved)
	if err != nil {
		return
	}
	for key, price := range prices {
		modelPrices[key] = price
	}
	if asOf > pricesAsOf {
		pricesAsOf = asOf
	}
}

// downloadPrices turns OpenRouter's model list into the prices.json format, with the
// same filter as .tools/release-new-wj-version/refresh-model-prices-from-openrouter.sh.
func downloadPrices(url string, now time.Time) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "wj-prices/1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tải bảng giá: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tải bảng giá: HTTP %d", resp.StatusCode)
	}
	var list struct {
		Data []struct {
			ID      string `json:"id"`
			Pricing struct {
				openRouterPrice
				Overrides []openRouterPrice `json:"overrides"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&list); err != nil {
		return nil, fmt.Errorf("đọc bảng giá: %w", err)
	}
	type entry struct {
		openRouterPrice
		Overrides []openRouterPrice `json:"overrides,omitempty"`
	}
	models := map[string]entry{}
	positive := func(text string) bool {
		v, err := strconv.ParseFloat(text, 64)
		return err == nil && v > 0
	}
	for _, m := range list.Data {
		if strings.Contains(m.ID, ":") || strings.HasPrefix(m.ID, "~") || strings.HasPrefix(m.ID, "openrouter/") {
			continue
		}
		if !positive(m.Pricing.Prompt) && !positive(m.Pricing.Completion) {
			continue
		}
		models[m.ID] = entry{m.Pricing.openRouterPrice, m.Pricing.Overrides}
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("bảng giá trống")
	}
	book := struct {
		AsOf   string           `json:"asOf"`
		Source string           `json:"source"`
		Models map[string]entry `json:"models"`
	}{now.UTC().Format("2006-01-02"), url, models}
	return json.MarshalIndent(book, "", "  ")
}

// parsePrices is loadPrices for a file that may be broken: it reports instead of panicking.
func parsePrices(data []byte) (asOf string, prices map[string]modelPrice, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	asOf, prices = loadPrices(data)
	return asOf, prices, nil
}
