// Package ql parses the PromQL and LogQL subsets used by iugum observe.
package ql

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/srhopkins/iugum/contract"
)

// PromQL is a metric selector: name or name{label="v"}.
type PromQL struct {
	Name   string
	Labels map[string]string
}

// LogQL is a log selector: {stream="x"} or {stream="x"} |= "word".
type LogQL struct {
	Stream string
	Labels map[string]string
	Text   string
}

// ParsePromQL accepts name or name{k="v",k2="v2"}.
func ParsePromQL(expr string) (PromQL, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return PromQL{}, nil
	}
	name, rest, ok := strings.Cut(expr, "{")
	name = strings.TrimSpace(name)
	if !ok {
		if !isIdent(name) {
			return PromQL{}, fmt.Errorf("promql: bad metric name %q", expr)
		}
		return PromQL{Name: name}, nil
	}
	if name != "" && !isIdent(name) {
		return PromQL{}, fmt.Errorf("promql: bad metric name %q", name)
	}
	if !strings.HasSuffix(rest, "}") {
		return PromQL{}, fmt.Errorf("promql: missing closing brace")
	}
	labs, err := parseLabels(strings.TrimSuffix(rest, "}"))
	if err != nil {
		return PromQL{}, err
	}
	return PromQL{Name: name, Labels: labs}, nil
}

// ParseLogQL accepts {stream="x"} and optional |= "word".
func ParseLogQL(expr string) (LogQL, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return LogQL{}, nil
	}
	if !strings.HasPrefix(expr, "{") {
		return LogQL{}, fmt.Errorf("logql: want {stream=\"x\"}")
	}
	end := strings.Index(expr, "}")
	if end < 0 {
		return LogQL{}, fmt.Errorf("logql: missing closing brace")
	}
	labs, err := parseLabels(expr[1:end])
	if err != nil {
		return LogQL{}, err
	}
	out := LogQL{Labels: labs, Stream: labs["stream"]}
	tail := strings.TrimSpace(expr[end+1:])
	if tail == "" {
		return out, nil
	}
	if !strings.HasPrefix(tail, "|=") {
		return LogQL{}, fmt.Errorf("logql: only |= line match is supported")
	}
	word, err := parseQuoted(strings.TrimSpace(tail[2:]))
	if err != nil {
		return LogQL{}, err
	}
	out.Text = word
	return out, nil
}

func parseLabels(s string) (map[string]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	out := map[string]string{}
	for _, part := range splitComma(s) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("label: want key=\"value\"")
		}
		k = strings.TrimSpace(k)
		if !isIdent(k) {
			return nil, fmt.Errorf("label: bad key %q", k)
		}
		val, err := parseQuoted(strings.TrimSpace(v))
		if err != nil {
			return nil, err
		}
		out[k] = val
	}
	return out, nil
}

func splitComma(s string) []string {
	var parts []string
	var b strings.Builder
	inQ := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQ != 0 {
			b.WriteByte(c)
			if c == inQ && (i == 0 || s[i-1] != '\\') {
				inQ = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			inQ = c
			b.WriteByte(c)
			continue
		}
		if c == ',' {
			parts = append(parts, b.String())
			b.Reset()
			continue
		}
		b.WriteByte(c)
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	return parts
}

func parseQuoted(s string) (string, error) {
	if len(s) < 2 {
		return "", fmt.Errorf("want quoted string")
	}
	q := s[0]
	if (q != '"' && q != '\'') || s[len(s)-1] != q {
		return "", fmt.Errorf("want quoted string")
	}
	inner := s[1 : len(s)-1]
	inner = strings.ReplaceAll(inner, `\"`, `"`)
	inner = strings.ReplaceAll(inner, `\'`, `'`)
	return inner, nil
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' && r != ':' {
				return false
			}
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != ':' {
			return false
		}
	}
	return true
}

// ApplyPromQL copies a parsed selector onto q.
func ApplyPromQL(q *contract.MetricQuery, p PromQL) {
	if q.Name == "" {
		q.Name = p.Name
	}
	if len(q.Labels) == 0 && len(p.Labels) > 0 {
		q.Labels = p.Labels
	}
}

// ApplyLogQL copies a parsed selector onto q.
func ApplyLogQL(q *contract.LogQuery, p LogQL) {
	if q.Stream == "" {
		q.Stream = p.Stream
	}
	if q.Text == "" {
		q.Text = p.Text
	}
}

// ApplyLogQLRange copies a parsed selector onto a range query.
func ApplyLogQLRange(q *contract.LogRangeQuery, p LogQL) {
	if q.Stream == "" {
		q.Stream = p.Stream
	}
	if q.Text == "" {
		q.Text = p.Text
	}
}

// LogStreamAttrs splits LogQL labels into level (logs.level) and the rest (logs.attrs).
// stream is already on LogQL.Stream / LogRangeQuery.Stream.
func LogStreamAttrs(labs map[string]string) (level string, attrs map[string]string) {
	if len(labs) == 0 {
		return "", nil
	}
	for k, v := range labs {
		switch k {
		case "stream":
			continue
		case "level":
			level = v
		default:
			if attrs == nil {
				attrs = map[string]string{}
			}
			attrs[k] = v
		}
	}
	return level, attrs
}

var nameAlias = map[string]string{
	"memory_c":      "mem_c",
	"sys_fan1":      "fan_pct",
	"sys_fan1_pct":  "fan_pct",
	"sys_fan1_duty": "fan_pct",
}

var tempNames = map[string]bool{
	"junction_c": true,
	"edge_c":     true,
	"mem_c":      true,
	"memory_c":   true,
	"cpu_c":      true,
}

// NormalizeSample stores °C and short metric names. Millidegree inputs become °C.
func NormalizeSample(s contract.Sample) contract.Sample {
	s.Name = strings.TrimSpace(s.Name)
	lower := strings.ToLower(s.Name)
	if alias, ok := nameAlias[lower]; ok {
		s.Name = alias
		lower = alias
	}
	if strings.Contains(lower, "millidegree") || strings.Contains(lower, "millicelsius") {
		s.Value = s.Value / 1000
		s.Name = rewriteMilliName(lower)
	} else if tempNames[lower] && s.Value > 200 {
		s.Value = s.Value / 1000
		if alias, ok := nameAlias[lower]; ok {
			s.Name = alias
		}
	} else if alias, ok := nameAlias[lower]; ok {
		s.Name = alias
	}
	return s
}

func rewriteMilliName(lower string) string {
	for _, key := range []string{"junction", "edge", "memory", "mem", "cpu"} {
		if strings.Contains(lower, key) {
			if key == "memory" {
				return "mem_c"
			}
			return key + "_c"
		}
	}
	n := strings.ReplaceAll(lower, "_millidegree_celsius", "_c")
	n = strings.ReplaceAll(n, "millidegree_celsius", "c")
	n = strings.ReplaceAll(n, "_millidegrees", "_c")
	return n
}

// ParsePromText parses Prometheus text exposition (HELP/TYPE lines ignored).
func ParsePromText(body string) ([]contract.Sample, error) {
	var out []contract.Sample
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		s, err := parsePromLine(line)
		if err != nil {
			return nil, err
		}
		out = append(out, NormalizeSample(s))
	}
	return out, nil
}

func parsePromLine(line string) (contract.Sample, error) {
	// name{labels} value [timestamp]
	// name value
	sel, rest, ok := cutLastNumber(line)
	if !ok {
		return contract.Sample{}, fmt.Errorf("prom text: no value on %q", line)
	}
	val, err := strconv.ParseFloat(rest, 64)
	if err != nil {
		return contract.Sample{}, fmt.Errorf("prom text: bad value on %q", line)
	}
	// rest may be "value" or we already split; timestamp ignored (optional third field already cut)
	sel = strings.TrimSpace(sel)
	p, err := ParsePromQL(sel)
	if err != nil {
		return contract.Sample{}, err
	}
	return contract.Sample{Name: p.Name, Labels: p.Labels, Value: val}, nil
}

func cutLastNumber(line string) (prefix, num string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", "", false
	}
	// last field may be timestamp (integer ms); value is last float that is not only digits after a float
	if len(fields) >= 3 {
		if _, err := strconv.ParseInt(fields[len(fields)-1], 10, 64); err == nil {
			if _, err := strconv.ParseFloat(fields[len(fields)-2], 64); err == nil {
				num = fields[len(fields)-2]
				prefix = strings.Join(fields[:len(fields)-2], " ")
				return prefix, num, true
			}
		}
	}
	num = fields[len(fields)-1]
	prefix = strings.Join(fields[:len(fields)-1], " ")
	return prefix, num, true
}

// ParseMetricsJSON accepts []Sample, one Sample, or a flat homelab object.
func ParseMetricsJSON(body []byte) ([]contract.Sample, error) {
	body = []byte(strings.TrimSpace(string(body)))
	if len(body) == 0 {
		return nil, fmt.Errorf("empty metrics body")
	}
	if body[0] == '[' {
		var samples []contract.Sample
		if err := json.Unmarshal(body, &samples); err != nil {
			return nil, err
		}
		for i := range samples {
			samples[i] = NormalizeSample(samples[i])
		}
		return samples, nil
	}
	var one contract.Sample
	if err := json.Unmarshal(body, &one); err == nil && one.Name != "" {
		return []contract.Sample{NormalizeSample(one)}, nil
	}
	var flat map[string]any
	if err := json.Unmarshal(body, &flat); err != nil {
		return nil, err
	}
	return samplesFromFlat(flat)
}

func samplesFromFlat(flat map[string]any) ([]contract.Sample, error) {
	labels := map[string]string{}
	var timeUS int64
	metricKeys := []string{"junction_c", "edge_c", "mem_c", "memory_c", "cpu_c", "fan_pct", "sys_fan1", "sys_fan1_pct", "fan_rpm"}
	isMetric := map[string]bool{}
	for _, k := range metricKeys {
		isMetric[k] = true
	}
	if ts, ok := asInt(flat["_timestamp"]); ok {
		timeUS = ts
	}
	for k, v := range flat {
		if isMetric[k] || isMetric[strings.ToLower(k)] {
			continue
		}
		if k == "_timestamp" || k == "unit" {
			continue
		}
		if s, ok := v.(string); ok {
			labels[k] = s
		}
	}
	var out []contract.Sample
	for k, v := range flat {
		lk := strings.ToLower(k)
		if !isMetric[k] && !isMetric[lk] {
			if f, ok := asFloat(v); ok && strings.HasSuffix(lk, "_c") {
				out = append(out, NormalizeSample(contract.Sample{Name: k, Labels: labels, Value: f, TimeUS: timeUS}))
			}
			continue
		}
		f, ok := asFloat(v)
		if !ok {
			continue
		}
		labs := map[string]string{}
		for lk, lv := range labels {
			labs[lk] = lv
		}
		out = append(out, NormalizeSample(contract.Sample{Name: k, Labels: labs, Value: f, TimeUS: timeUS}))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no metric fields in JSON")
	}
	return out, nil
}

func asFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		f, err := strconv.ParseFloat(t, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func asInt(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case json.Number:
		i, err := t.Int64()
		return i, err == nil
	case int:
		return int64(t), true
	case int64:
		return t, true
	default:
		return 0, false
	}
}

// ParseLogsJSON accepts []Log or one Log.
func ParseLogsJSON(body []byte) ([]contract.Log, error) {
	body = []byte(strings.TrimSpace(string(body)))
	if len(body) == 0 {
		return nil, fmt.Errorf("empty logs body")
	}
	if body[0] == '[' {
		var recs []contract.Log
		if err := json.Unmarshal(body, &recs); err != nil {
			return nil, err
		}
		return recs, nil
	}
	var one contract.Log
	if err := json.Unmarshal(body, &one); err != nil {
		return nil, err
	}
	return []contract.Log{one}, nil
}

// LooksLikeProm is true when the body is not JSON.
func LooksLikeProm(body []byte, contentType string) bool {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "text/plain") || strings.Contains(ct, "openmetrics") || strings.Contains(ct, "text/plain; version=") {
		return true
	}
	trim := strings.TrimSpace(string(body))
	if trim == "" {
		return false
	}
	return trim[0] != '[' && trim[0] != '{'
}
