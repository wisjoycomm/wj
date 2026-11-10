package main

import (
	"encoding/json"
	"io"
	"sort"
	"time"
)

// `wj usage ... -json`: the same numbers as the tables, for scripts and status lines.
// Times are RFC 3339; costs are USD at list price, like the QUY ĐỔI column.

type jsonTotals struct {
	Calls          int     `json:"calls"`
	Input          int64   `json:"inputTokens"`
	Output         int64   `json:"outputTokens"`
	CacheRead      int64   `json:"cacheReadTokens"`
	CacheWrite     int64   `json:"cacheWriteTokens"`
	Total          int64   `json:"totalTokens"`
	CostUSD        float64 `json:"costUSD"`
	UnpricedTokens int64   `json:"unpricedTokens,omitempty"`
}

func totalsOf(r usageRow) jsonTotals {
	return jsonTotals{r.Calls, r.Input, r.Output, r.CacheRead, r.CacheWrite, r.Total, r.Cost, r.Unpriced}
}

type jsonPeriod struct {
	Label string    `json:"label"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

func writeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// periodJSON is the report of one period: by day, by session and by model.
func periodJSON(sc *sessionScanner, v usageView, now time.Time) (any, error) {
	p, err := resolvePeriod(v.Spec, now)
	if err != nil {
		return nil, err
	}
	if v.Blocks {
		return blocksJSON(sc, v, p, now)
	}
	if v.Analytics {
		sc.scan(p.Start)
		return analyticsJSON(sc, v, p), nil
	}
	sc.scan(p.Start)
	days, sessions, total := sc.periodReport(p, v.Project, v.Client)
	type day struct {
		Date     string `json:"date"`
		Sessions int    `json:"sessions"`
		jsonTotals
	}
	type worker struct {
		ID          string `json:"id"`
		Type        string `json:"type,omitempty"`
		Description string `json:"description,omitempty"`
		Model       string `json:"model"`
		MaxContext  int64  `json:"maxContextTokens"`
		jsonTotals
	}
	type session struct {
		ID          string   `json:"id"`
		App         string   `json:"app"`
		Project     string   `json:"project"`
		Title       string   `json:"title,omitempty"`
		Model       string   `json:"model"`
		MaxContext  int64    `json:"maxContextTokens"`
		AgentTokens int64    `json:"subagentTokens"`
		LastCall    string   `json:"lastCall"`
		Workers     []worker `json:"workers,omitempty"`
		jsonTotals
	}
	type model struct {
		Model    string         `json:"model"`
		Projects map[string]any `json:"projects"`
		jsonTotals
	}
	result := struct {
		Period   jsonPeriod `json:"period"`
		Project  string     `json:"project,omitempty"`
		App      string     `json:"app,omitempty"`
		Totals   jsonTotals `json:"totals"`
		Days     []day      `json:"days"`
		Sessions []session  `json:"sessions"`
		Models   []model    `json:"models"`
	}{Period: jsonPeriod{p.Label, p.Start, p.End}, Project: v.Project, App: v.Client, Totals: totalsOf(total),
		Days: []day{}, Sessions: []session{}, Models: []model{}}
	for _, d := range days {
		result.Days = append(result.Days, day{d.Day.Format("2006-01-02"), d.Sessions, totalsOf(d.usageRow)})
	}
	if v.Top > 0 && len(sessions) > v.Top {
		sessions = sessions[:v.Top]
	}
	for _, se := range sessions {
		row := session{ID: se.ID, App: se.Client, Project: se.Project, Title: se.Title, Model: topModel(se.models),
			MaxContext: se.MaxCtx, AgentTokens: se.Agent, LastCall: se.Last.Format(time.RFC3339), jsonTotals: totalsOf(se.usageRow)}
		for _, w := range se.sortedWorkers() {
			row.Workers = append(row.Workers, worker{w.ID, w.Type, w.Description, topModel(w.models), w.MaxCtx, totalsOf(w.usageRow)})
		}
		result.Sessions = append(result.Sessions, row)
	}
	for _, m := range sc.periodTree(p, v.Project, v.Client) {
		projects := map[string]any{}
		for _, pr := range m.Projects {
			projects[pr.Project] = totalsOf(pr)
		}
		result.Models = append(result.Models, model{m.Name, projects, totalsOf(m.usageRow)})
	}
	return result, nil
}

func blocksJSON(sc *sessionScanner, v usageView, p usagePeriod, now time.Time) (any, error) {
	since := p.Start.Add(-blockLength)
	sc.scan(since)
	all := sc.blocks(since, v.Project, v.Client, now)
	limit, err := blockLimit(v.Limit, all)
	if err != nil {
		return nil, err
	}
	type rate struct {
		TokensPerMinute      float64 `json:"tokensPerMinute"`
		PlainTokensPerMinute float64 `json:"inputOutputTokensPerMinute"`
		CostPerHour          float64 `json:"costPerHourUSD"`
	}
	type projection struct {
		Tokens       int64   `json:"totalTokens"`
		CostUSD      float64 `json:"costUSD"`
		MinutesLeft  int     `json:"remainingMinutes"`
		LimitPercent float64 `json:"limitPercent,omitempty"`
		LimitStatus  string  `json:"limitStatus,omitempty"`
	}
	type block struct {
		Start      time.Time   `json:"start"`
		End        time.Time   `json:"end"`
		FirstCall  *time.Time  `json:"firstCall,omitempty"`
		LastCall   *time.Time  `json:"lastCall,omitempty"`
		Gap        bool        `json:"gap,omitempty"`
		Active     bool        `json:"active,omitempty"`
		Models     []string    `json:"models,omitempty"`
		BurnRate   *rate       `json:"burnRate,omitempty"`
		Projection *projection `json:"projection,omitempty"`
		jsonTotals
	}
	statusWord := func(tokens int64) string {
		return [...]string{"ok", "warning", "exceeds"}[warnLevel(tokens, limit)]
	}
	result := struct {
		Period jsonPeriod `json:"period"`
		Limit  int64      `json:"tokenLimit,omitempty"`
		Blocks []block    `json:"blocks"`
	}{Period: jsonPeriod{p.Label, p.Start, p.End}, Limit: limit, Blocks: []block{}}
	for _, b := range all {
		if !b.End.After(p.Start) || !b.Start.Before(p.End) {
			continue
		}
		row := block{Start: b.Start, End: b.End, Gap: b.Gap, Active: b.Active, jsonTotals: totalsOf(b.usageRow)}
		if !b.Gap {
			first, last := b.First, b.Last
			row.FirstCall, row.LastCall = &first, &last
			for m := range b.models {
				row.Models = append(row.Models, m)
			}
			sort.Strings(row.Models)
		}
		if perMin, plain, costPerHour, ok := b.burnRate(); ok && b.Active {
			row.BurnRate = &rate{perMin, plain, costPerHour}
		}
		if tokens, cost, ok := b.projection(now); ok {
			pr := &projection{Tokens: tokens, CostUSD: cost, MinutesLeft: int(max(0, b.End.Sub(now).Minutes()))}
			if limit > 0 {
				pr.LimitPercent, pr.LimitStatus = float64(tokens)*100/float64(limit), statusWord(tokens)
			}
			row.Projection = pr
		}
		result.Blocks = append(result.Blocks, row)
	}
	return result, nil
}
