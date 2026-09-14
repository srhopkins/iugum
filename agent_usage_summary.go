package main

import (
	"fmt"
	"github.com/srhopkins/iugum/agentcore"
	"sort"
	"strings"
	"time"
)

func agentUsageSummary(rows []agentcore.UsageRecord, now time.Time) string {
	type total struct {
		calls, tokens, unknown int
		cost                   float64
		priced                 bool
	}
	pools := map[string]*total{}
	for _, r := range rows {
		if r.Time.UTC().Format("2006-01") != now.UTC().Format("2006-01") {
			continue
		}
		p := pools[r.Pool]
		if p == nil {
			p = &total{}
			pools[r.Pool] = p
		}
		p.calls++
		if r.UsageKnown {
			p.tokens += r.InputTokens + r.OutputTokens
		} else {
			p.unknown++
		}
		if r.PriceKnown {
			p.cost += r.EstimatedUSD
			p.priced = true
		}
	}
	if len(pools) == 0 {
		return "No model usage recorded this month."
	}
	keys := []string{}
	for k := range pools {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := []string{"Usage this month (UTC billing window):"}
	for _, k := range keys {
		p := pools[k]
		cost := "billed cost unavailable"
		if p.priced {
			cost = fmt.Sprintf("$%.4f estimated", p.cost)
		}
		line := fmt.Sprintf("%s: %d model calls · %d reported tokens · %s", k, p.calls, p.tokens, cost)
		if p.unknown > 0 {
			line += fmt.Sprintf(" · usage unavailable for %d calls", p.unknown)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
