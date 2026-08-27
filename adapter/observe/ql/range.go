package ql

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/srhopkins/iugum/contract"
)

// DefaultLookbackUS is Prometheus query_range lookback: 5 minutes.
const DefaultLookbackUS int64 = 5 * 60 * 1_000_000

// RangeExpr is the PromQL subset QueryMetricRange evaluates.
// Selector: junction_c or junction_c{gpu="mi50"}.
// Optional: rate(selector[duration]) and avg by (labels) (inner).
type RangeExpr struct {
	Name    string
	Labels  map[string]string
	RateUS  int64
	GroupBy []string
}

// ParseRangePromQL parses a range PromQL subset.
func ParseRangePromQL(expr string) (RangeExpr, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return RangeExpr{}, nil
	}
	if strings.HasPrefix(expr, "avg") {
		tail := strings.TrimSpace(expr[3:])
		if strings.HasPrefix(tail, "by") {
			return parseAvgBy(expr)
		}
	}
	if strings.HasPrefix(expr, "rate(") {
		return parseRate(expr)
	}
	p, err := ParsePromQL(expr)
	if err != nil {
		return RangeExpr{}, err
	}
	return RangeExpr{Name: p.Name, Labels: p.Labels}, nil
}

func parseRate(expr string) (RangeExpr, error) {
	inner := strings.TrimSpace(strings.TrimPrefix(expr, "rate"))
	selDur, extra, err := matchParen(inner)
	if err != nil {
		return RangeExpr{}, fmt.Errorf("promql: rate: %w", err)
	}
	if extra != "" {
		return RangeExpr{}, fmt.Errorf("promql: rate: trailing %q", extra)
	}
	selDur = strings.TrimSpace(selDur)
	open := strings.LastIndex(selDur, "[")
	if open < 0 || !strings.HasSuffix(selDur, "]") {
		return RangeExpr{}, fmt.Errorf("promql: rate: want metric[duration]")
	}
	sel := strings.TrimSpace(selDur[:open])
	dur := strings.TrimSpace(selDur[open+1 : len(selDur)-1])
	us, err := ParseDurationUS(dur)
	if err != nil {
		return RangeExpr{}, err
	}
	if us <= 0 {
		return RangeExpr{}, fmt.Errorf("promql: rate: duration must be > 0")
	}
	p, err := ParsePromQL(sel)
	if err != nil {
		return RangeExpr{}, err
	}
	return RangeExpr{Name: p.Name, Labels: p.Labels, RateUS: us}, nil
}

func parseAvgBy(expr string) (RangeExpr, error) {
	rest := strings.TrimSpace(expr[3:])
	if !strings.HasPrefix(rest, "by") {
		return RangeExpr{}, fmt.Errorf("promql: only avg by (...) is supported")
	}
	rest = strings.TrimSpace(rest[2:])
	list, rest, err := matchParen(rest)
	if err != nil {
		return RangeExpr{}, fmt.Errorf("promql: avg by: %w", err)
	}
	var keys []string
	for _, part := range strings.Split(list, ",") {
		k := strings.TrimSpace(part)
		if k == "" {
			continue
		}
		if !isIdent(k) {
			return RangeExpr{}, fmt.Errorf("promql: avg by: bad label %q", k)
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return RangeExpr{}, fmt.Errorf("promql: avg by: empty label list")
	}
	inner, extra, err := matchParen(strings.TrimSpace(rest))
	if err != nil {
		return RangeExpr{}, fmt.Errorf("promql: avg by: %w", err)
	}
	if extra != "" {
		return RangeExpr{}, fmt.Errorf("promql: avg by: trailing %q", extra)
	}
	rx, err := ParseRangePromQL(inner)
	if err != nil {
		return RangeExpr{}, err
	}
	if len(rx.GroupBy) > 0 {
		return RangeExpr{}, fmt.Errorf("promql: nested avg by is not supported")
	}
	rx.GroupBy = keys
	return rx, nil
}

func matchParen(s string) (inner, rest string, err error) {
	s = strings.TrimSpace(s)
	if s == "" || s[0] != '(' {
		return "", "", fmt.Errorf("want (")
	}
	depth := 0
	inQ := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQ != 0 {
			if c == inQ && (i == 0 || s[i-1] != '\\') {
				inQ = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			inQ = c
			continue
		}
		if c == '(' {
			depth++
			continue
		}
		if c == ')' {
			depth--
			if depth == 0 {
				return s[1:i], strings.TrimSpace(s[i+1:]), nil
			}
		}
	}
	return "", "", fmt.Errorf("missing )")
}

// ParseDurationUS parses a Prometheus duration into unix microseconds.
func ParseDurationUS(s string) (int64, error) {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	if i == 0 {
		return 0, fmt.Errorf("promql: bad duration %q", s)
	}
	n, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0, fmt.Errorf("promql: bad duration %q", s)
	}
	var us float64
	switch s[i:] {
	case "ms":
		us = n * 1e3
	case "s":
		us = n * 1e6
	case "m":
		us = n * 60 * 1e6
	case "h":
		us = n * 3600 * 1e6
	case "d":
		us = n * 86400 * 1e6
	default:
		return 0, fmt.Errorf("promql: bad duration unit %q", s[i:])
	}
	return int64(us), nil
}

// LookbackStartUS is the fetch start so the first step can use a prior sample.
// startUS 0 means no lower bound.
func LookbackStartUS(startUS, lookbackUS int64) int64 {
	if startUS == 0 {
		return 0
	}
	if lookbackUS <= 0 {
		lookbackUS = DefaultLookbackUS
	}
	fs := startUS - lookbackUS
	if fs < 1 {
		return 0
	}
	return fs
}

// RangeFromQuery parses Expr (or Name/Labels) into a RangeExpr.
func RangeFromQuery(q contract.MetricRangeQuery) (RangeExpr, error) {
	var rx RangeExpr
	if strings.TrimSpace(q.Expr) != "" {
		var err error
		rx, err = ParseRangePromQL(q.Expr)
		if err != nil {
			return RangeExpr{}, err
		}
	}
	if rx.Name == "" {
		rx.Name = q.Name
	}
	if len(rx.Labels) == 0 && len(q.Labels) > 0 {
		rx.Labels = q.Labels
	}
	return rx, nil
}

// SampleQuery returns a MetricQuery that loads samples for EvalRange.
// Expr is left empty so QueryMetrics does not re-parse rate()/avg by().
func SampleQuery(rx RangeExpr, startUS, endUS int64) contract.MetricQuery {
	lookback := DefaultLookbackUS
	if rx.RateUS > lookback {
		lookback = rx.RateUS
	}
	return contract.MetricQuery{
		Name:    rx.Name,
		Labels:  rx.Labels,
		StartUS: LookbackStartUS(startUS, lookback),
		EndUS:   endUS,
	}
}

// EvalRange turns raw samples into a Prometheus matrix (step timestamps).
func EvalRange(series []contract.Series, rx RangeExpr, startUS, endUS, stepUS int64) []contract.Series {
	if rx.RateUS > 0 {
		series = RateSeries(series, startUS, endUS, stepUS, rx.RateUS)
	} else if stepUS > 0 {
		series = AlignSeries(series, startUS, endUS, stepUS)
	} else {
		series = clipSeries(series, startUS, endUS)
	}
	if len(rx.GroupBy) > 0 {
		series = AvgBySeries(series, rx.GroupBy)
	}
	return series
}

func clipSeries(series []contract.Series, startUS, endUS int64) []contract.Series {
	if startUS == 0 && endUS == 0 {
		return series
	}
	out := make([]contract.Series, 0, len(series))
	for _, s := range series {
		var pts [][2]float64
		for _, p := range s.Points {
			t := int64(p[0] * 1e6)
			if startUS != 0 && t < startUS {
				continue
			}
			if endUS != 0 && t > endUS {
				continue
			}
			pts = append(pts, p)
		}
		if len(pts) == 0 {
			continue
		}
		s.Points = pts
		out = append(out, s)
	}
	return out
}

// RateSeries is (last-first)/seconds over (t-range, t] at each step.
func RateSeries(series []contract.Series, startUS, endUS, stepUS, rangeUS int64) []contract.Series {
	out := make([]contract.Series, 0, len(series))
	for _, s := range series {
		times := evalTimes(s.Points, startUS, endUS, stepUS)
		var pts [][2]float64
		for _, t := range times {
			v, ok := rateAt(s.Points, t, rangeUS)
			if !ok {
				continue
			}
			pts = append(pts, [2]float64{float64(t) / 1e6, v})
		}
		if len(pts) == 0 {
			continue
		}
		s.Points = pts
		out = append(out, s)
	}
	return out
}

func evalTimes(pts [][2]float64, startUS, endUS, stepUS int64) []int64 {
	if len(pts) == 0 {
		return nil
	}
	if stepUS <= 0 {
		var times []int64
		for _, p := range pts {
			t := int64(p[0] * 1e6)
			if startUS != 0 && t < startUS {
				continue
			}
			if endUS != 0 && t > endUS {
				continue
			}
			times = append(times, t)
		}
		return times
	}
	st, en := startUS, endUS
	if st == 0 {
		st = int64(pts[0][0] * 1e6)
	}
	if en == 0 {
		en = int64(pts[len(pts)-1][0] * 1e6)
	}
	if en < st {
		return nil
	}
	var times []int64
	for t := st; t <= en; {
		times = append(times, t)
		next := t + stepUS
		if next <= t {
			break
		}
		t = next
	}
	return times
}

func rateAt(pts [][2]float64, tUS, rangeUS int64) (float64, bool) {
	if rangeUS <= 0 || len(pts) == 0 {
		return 0, false
	}
	windowStart := tUS - rangeUS
	var firstT, lastT int64
	var firstV, lastV float64
	n := 0
	for _, p := range pts {
		tus := int64(p[0] * 1e6)
		if tus <= windowStart || tus > tUS {
			continue
		}
		if n == 0 {
			firstT, firstV = tus, p[1]
		}
		lastT, lastV = tus, p[1]
		n++
	}
	if n < 2 || lastT <= firstT {
		return 0, false
	}
	return (lastV - firstV) / (float64(lastT-firstT) / 1e6), true
}

// AvgBySeries averages series that share the grouping labels, at matching times.
func AvgBySeries(series []contract.Series, keys []string) []contract.Series {
	if len(keys) == 0 {
		return series
	}
	type acc struct {
		name   string
		labels map[string]string
		sum    map[int64]float64
		n      map[int64]int
	}
	groups := map[string]*acc{}
	order := []string{}
	for _, s := range series {
		labs := map[string]string{}
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			v := ""
			if s.Labels != nil {
				v = s.Labels[k]
			}
			labs[k] = v
			parts = append(parts, k+"="+v)
		}
		id := strings.Join(parts, ",")
		g, ok := groups[id]
		if !ok {
			g = &acc{name: s.Name, labels: labs, sum: map[int64]float64{}, n: map[int64]int{}}
			groups[id] = g
			order = append(order, id)
		}
		for _, p := range s.Points {
			t := int64(p[0] * 1e6)
			g.sum[t] += p[1]
			g.n[t]++
		}
	}
	out := make([]contract.Series, 0, len(order))
	for _, id := range order {
		g := groups[id]
		ts := make([]int64, 0, len(g.sum))
		for t := range g.sum {
			ts = append(ts, t)
		}
		sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })
		pts := make([][2]float64, 0, len(ts))
		for _, t := range ts {
			pts = append(pts, [2]float64{float64(t) / 1e6, g.sum[t] / float64(g.n[t])})
		}
		if len(pts) == 0 {
			continue
		}
		out = append(out, contract.Series{Name: g.name, Labels: g.labels, Points: pts})
	}
	return out
}

// AlignSeries downsamples each series to one point per StepUS.
// Each point timestamp is the step instant in unix seconds (Prometheus query_range).
// The value is the last sample at or before that instant.
// StepUS <= 0 leaves points unchanged.
// StartUS or EndUS of 0 uses the first or last sample time in that series.
func AlignSeries(series []contract.Series, startUS, endUS, stepUS int64) []contract.Series {
	if stepUS <= 0 {
		return series
	}
	out := make([]contract.Series, 0, len(series))
	for _, s := range series {
		pts := alignPoints(s.Points, startUS, endUS, stepUS)
		if len(pts) == 0 {
			continue
		}
		s.Points = pts
		out = append(out, s)
	}
	return out
}

func alignPoints(pts [][2]float64, startUS, endUS, stepUS int64) [][2]float64 {
	if len(pts) == 0 || stepUS <= 0 {
		return pts
	}
	first := int64(pts[0][0] * 1e6)
	last := int64(pts[len(pts)-1][0] * 1e6)
	if startUS == 0 {
		startUS = first
	}
	if endUS == 0 {
		endUS = last
	}
	if endUS < startUS {
		return nil
	}
	var out [][2]float64
	i := 0
	var have bool
	var val float64
	for t := startUS; t <= endUS; {
		for i < len(pts) && int64(pts[i][0]*1e6) <= t {
			val = pts[i][1]
			have = true
			i++
		}
		if have {
			out = append(out, [2]float64{float64(t) / 1e6, val})
		}
		next := t + stepUS
		if next <= t {
			break
		}
		t = next
	}
	return out
}

// MetricQueryFromRange copies range fields onto MetricQuery (no MaxPoints).
func MetricQueryFromRange(q contract.MetricRangeQuery) contract.MetricQuery {
	return contract.MetricQuery{
		Name:    q.Name,
		Labels:  q.Labels,
		Expr:    q.Expr,
		StartUS: q.StartUS,
		EndUS:   q.EndUS,
	}
}

// LogQueryFromRange copies range fields onto LogQuery. StepUS is dropped.
func LogQueryFromRange(q contract.LogRangeQuery) contract.LogQuery {
	return contract.LogQuery{
		Stream:  q.Stream,
		Text:    q.Text,
		Expr:    q.Expr,
		StartUS: q.StartUS,
		EndUS:   q.EndUS,
		Limit:   q.Limit,
	}
}
