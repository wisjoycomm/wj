package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// List prices, for the "what would this cost on the API" column. prices.json is a
// snapshot of OpenRouter's public list (scripts/update-prices.sh), embedded so the
// token commands stay offline. It ships with each release.
//
//go:embed prices.json
var pricesJSON []byte

type openRouterPrice struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
	Read       string `json:"input_cache_read"`
	Write      string `json:"input_cache_write"`
	Write1h    string `json:"input_cache_write_1h"`
	MinPrompt  int64  `json:"min_prompt_tokens"`
}

type tokenRates struct {
	Input, Output, CacheRead, Write5m, Write1h float64 // USD per token
}

type modelPrice struct {
	tokenRates
	// A prompt over LongFrom tokens is billed at Long instead. 0 means no such tier.
	LongFrom int64
	Long     tokenRates
}

// OpenRouter leaves out models it does not sell. These come from
// platform.claude.com/docs/en/about-claude/pricing, per million tokens.
var officialOnlyPrices = map[string]tokenRates{
	"claude-mythos-5-1": perMillion(10, 50, 0.25, 12.50, 20),
	"claude-mythos-5":   perMillion(10, 50, 1, 12.50, 20),
	"claude-opus-4":     perMillion(15, 75, 1.50, 18.75, 30),
	"claude-sonnet-4":   perMillion(3, 15, 0.30, 3.75, 6),
	"claude-haiku-3-5":  perMillion(0.80, 4, 0.08, 1, 1.60),
}

func perMillion(input, output, read, write5m, write1h float64) tokenRates {
	const m = 1_000_000
	return tokenRates{input / m, output / m, read / m, write5m / m, write1h / m}
}

var pricesAsOf, modelPrices = loadPrices(pricesJSON)

func loadPrices(data []byte) (string, map[string]modelPrice) {
	var book struct {
		AsOf   string `json:"asOf"`
		Models map[string]struct {
			openRouterPrice
			Overrides []openRouterPrice `json:"overrides"`
		} `json:"models"`
	}
	// The file is checked at build time by the tests, so this cannot fail at run time.
	if err := json.Unmarshal(data, &book); err != nil {
		panic(fmt.Errorf("prices.json: %w", err))
	}
	prices := make(map[string]modelPrice, len(book.Models)+len(officialOnlyPrices))
	for id, entry := range book.Models {
		price := modelPrice{tokenRates: entry.rates(tokenRates{})}
		if len(entry.Overrides) > 0 {
			price.LongFrom = entry.Overrides[0].MinPrompt
			price.Long = entry.Overrides[0].rates(price.tokenRates)
		}
		prices[priceKey(id)] = price
	}
	for name, rates := range officialOnlyPrices {
		prices[priceKey(name)] = modelPrice{tokenRates: rates}
	}
	return book.AsOf, prices
}

// rates fills what the list leaves out: a model with no cache price bills those tokens
// as plain input, and one with no 1-hour write price bills it as a normal write.
func (p openRouterPrice) rates(base tokenRates) tokenRates {
	number := func(text string, fallback float64) float64 {
		if value, err := strconv.ParseFloat(text, 64); err == nil && value >= 0 {
			return value
		}
		return fallback
	}
	input := number(p.Prompt, base.Input)
	write := number(p.Write, input)
	return tokenRates{
		Input:     input,
		Output:    number(p.Completion, base.Output),
		CacheRead: number(p.Read, input),
		Write5m:   write,
		Write1h:   number(p.Write1h, write),
	}
}

var modelDateSuffix = regexp.MustCompile(`-(\d{8}|\d{4}-\d{2}-\d{2})$`)

// priceKey ignores only a provider or gateway prefix, a date snapshot and a context
// tag, and treats claude-opus-5-5 and claude-opus-5.5 as the same name.
func priceKey(model string) string {
	name := strings.ToLower(strings.TrimSpace(model))
	name = name[strings.LastIndex(name, "/")+1:]
	if i := strings.Index(name, "["); i >= 0 {
		name = name[:i]
	}
	name = modelDateSuffix.ReplaceAllString(name, "")
	return strings.ReplaceAll(name, ".", "-")
}

// priceOf finds a model by name. A name that is not listed has no price: guessing from
// a similar name would print a confident wrong number.
func priceOf(model string) (modelPrice, bool) {
	key := priceKey(model)
	if price, ok := modelPrices[key]; ok {
		return price, ok
	}
	// Antigravity names a thinking level (gemini-3.8-flash-high) or the family first
	// (claude-4.5-sonnet); these bill as the plain model.
	if m := effortSuffix.FindStringSubmatch(key); m != nil {
		key = m[1]
	} else if m := familyFirst.FindStringSubmatch(key); m != nil {
		key = "claude-" + m[2] + "-" + m[1]
	}
	if price, ok := modelPrices[key]; ok {
		return price, ok
	}
	// gemini-3.1-pro is sold as gemini-3.1-pro-preview.
	price, ok := modelPrices[key+"-preview"]
	return price, ok && strings.HasPrefix(key, "gemini-")
}

var (
	effortSuffix = regexp.MustCompile(`^((?:gemini-[\d-]+-flash)|gpt-oss-\d+b)-(?:high|medium|low)$`)
	familyFirst  = regexp.MustCompile(`^claude-([\d-]+)-(opus|sonnet|haiku)$`)
)

// callCost is what one call would cost at list price. The tier comes from the call's
// own prompt size, which counts cached tokens too.
func callCost(call sessionCall) (float64, bool) {
	parts, ok := costParts(call)
	return parts[0] + parts[1] + parts[2] + parts[3], ok
}

// costParts is callCost split by kind of token: input, output, cache read, cache write.
func costParts(call sessionCall) ([4]float64, bool) {
	price, ok := priceOf(call.Model)
	if !ok {
		return [4]float64{}, false
	}
	rates := price.tokenRates
	if price.LongFrom > 0 && call.Input+call.CacheRead+call.CacheWrite > price.LongFrom {
		rates = price.Long
	}
	write1h := min(call.CacheWrite1h, call.CacheWrite)
	return [4]float64{float64(call.Input) * rates.Input, float64(call.Output) * rates.Output,
		float64(call.CacheRead) * rates.CacheRead, float64(call.CacheWrite-write1h)*rates.Write5m + float64(write1h)*rates.Write1h}, true
}

// costText marks what the number leaves out: "$12+" has tokens without a price on top
// and "?" has no priced tokens at all.
func costText(row usageRow) string {
	suffix := ""
	if row.Unpriced > 0 {
		if row.Cost == 0 {
			return "?"
		}
		suffix = "+"
	}
	switch {
	case row.Cost == 0:
		return "$0"
	case row.Cost < 0.01:
		return "<$0.01" + suffix
	case row.Cost < 100:
		return fmt.Sprintf("$%.2f", row.Cost) + suffix
	}
	return "$" + groupDigits(int64(row.Cost+0.5)) + suffix
}
