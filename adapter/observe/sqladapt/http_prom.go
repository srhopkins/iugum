package sqladapt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/srhopkins/iugum/adapter/observe/ql"
	"github.com/srhopkins/iugum/contract"
)

// RegisterPromHTTP mounts the Prometheus HTTP API Perses calls (ENDPOINTS.md).
func RegisterPromHTTP(mux *http.ServeMux, ob contract.Observer) {
	mux.HandleFunc("GET /-/healthy", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /api/v1/query", func(w http.ResponseWriter, r *http.Request) {
		promInstant(ob, w, r)
	})
	mux.HandleFunc("POST /api/v1/query_range", func(w http.ResponseWriter, r *http.Request) {
		promRange(ob, w, r)
	})
	mux.HandleFunc("POST /api/v1/labels", func(w http.ResponseWriter, r *http.Request) {
		promLabels(ob, w, r)
	})
	mux.HandleFunc("GET /api/v1/label/{labelName}/values", func(w http.ResponseWriter, r *http.Request) {
		promLabelValues(ob, w, r)
	})
	mux.HandleFunc("GET /api/v1/metadata", func(w http.ResponseWriter, r *http.Request) {
		promMetadata(ob, w, r)
	})
	mux.HandleFunc("POST /api/v1/series", func(w http.ResponseWriter, r *http.Request) {
		promSeries(ob, w, r)
	})
	mux.HandleFunc("POST /api/v1/parse_query", func(w http.ResponseWriter, r *http.Request) {
		promParseQuery(w, r)
	})
}

func promInstant(ob contract.Observer, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writePromErr(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	expr := r.FormValue("query")
	if strings.TrimSpace(expr) == "" {
		writePromErr(w, http.StatusBadRequest, "bad_data", "missing query")
		return
	}
	at := time.Now().UnixMicro()
	if v := r.FormValue("time"); v != "" {
		us, err := parseUnixSecUS(v)
		if err != nil {
			writePromErr(w, http.StatusBadRequest, "bad_data", "bad time")
			return
		}
		at = us
	}
	ctx, cancel, err := withPromTimeout(r)
	if err != nil {
		writePromErr(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	defer cancel()
	series, err := ob.QueryMetricRange(ctx, contract.MetricRangeQuery{
		Expr:    expr,
		StartUS: at,
		EndUS:   at,
		StepUS:  1_000_000,
	})
	if err != nil {
		writePromErr(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	writePromOK(w, map[string]any{
		"resultType": "vector",
		"result":     seriesToVector(series),
	})
}

func promRange(ob contract.Observer, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writePromErr(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	expr := r.FormValue("query")
	if strings.TrimSpace(expr) == "" {
		writePromErr(w, http.StatusBadRequest, "bad_data", "missing query")
		return
	}
	startUS, err := parseUnixSecUS(r.FormValue("start"))
	if err != nil {
		writePromErr(w, http.StatusBadRequest, "bad_data", "bad start")
		return
	}
	endUS, err := parseUnixSecUS(r.FormValue("end"))
	if err != nil {
		writePromErr(w, http.StatusBadRequest, "bad_data", "bad end")
		return
	}
	stepUS, err := parseStepUS(r.FormValue("step"))
	if err != nil {
		writePromErr(w, http.StatusBadRequest, "bad_data", "bad step")
		return
	}
	ctx, cancel, err := withPromTimeout(r)
	if err != nil {
		writePromErr(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	defer cancel()
	series, err := ob.QueryMetricRange(ctx, contract.MetricRangeQuery{
		Expr:    expr,
		StartUS: startUS,
		EndUS:   endUS,
		StepUS:  stepUS,
	})
	if err != nil {
		writePromErr(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	writePromOK(w, map[string]any{
		"resultType": "matrix",
		"result":     seriesToMatrix(series),
	})
}

func promLabels(ob contract.Observer, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writePromErr(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	series, err := loadPromSeries(r.Context(), ob, r.Form["match[]"], optionalUnixUS(r.FormValue("start")), optionalUnixUS(r.FormValue("end")))
	if err != nil {
		writePromErr(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	names := map[string]struct{}{"__name__": {}}
	for _, s := range series {
		for k := range s.Labels {
			names[k] = struct{}{}
		}
	}
	writePromOK(w, limitStrings(sortedKeys(names), atoi(r.FormValue("limit"), 0)))
}

func promLabelValues(ob contract.Observer, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writePromErr(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	label := r.PathValue("labelName")
	series, err := loadPromSeries(r.Context(), ob, r.Form["match[]"], optionalUnixUS(r.FormValue("start")), optionalUnixUS(r.FormValue("end")))
	if err != nil {
		writePromErr(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	vals := map[string]struct{}{}
	for _, s := range series {
		if label == "__name__" {
			if s.Name != "" {
				vals[s.Name] = struct{}{}
			}
			continue
		}
		if s.Labels != nil {
			if v, ok := s.Labels[label]; ok {
				vals[v] = struct{}{}
			}
		}
	}
	writePromOK(w, limitStrings(sortedKeys(vals), atoi(r.FormValue("limit"), 0)))
}

func promMetadata(ob contract.Observer, w http.ResponseWriter, r *http.Request) {
	metricFilter := r.URL.Query().Get("metric")
	limit := atoi(r.URL.Query().Get("limit"), 0)
	series, err := ob.QueryMetrics(r.Context(), contract.MetricQuery{})
	if err != nil {
		writePromErr(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	out := map[string][]map[string]string{}
	seen := map[string]struct{}{}
	for _, s := range series {
		if s.Name == "" {
			continue
		}
		if metricFilter != "" && s.Name != metricFilter {
			continue
		}
		if _, ok := seen[s.Name]; ok {
			continue
		}
		seen[s.Name] = struct{}{}
		out[s.Name] = []map[string]string{{"type": "gauge", "help": ""}}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	writePromOK(w, out)
}

func promSeries(ob contract.Observer, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writePromErr(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	matches := r.Form["match[]"]
	if len(matches) == 0 {
		writePromErr(w, http.StatusBadRequest, "bad_data", "no match[] parameter provided")
		return
	}
	series, err := loadPromSeries(r.Context(), ob, matches, optionalUnixUS(r.FormValue("start")), optionalUnixUS(r.FormValue("end")))
	if err != nil {
		writePromErr(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	seen := map[string]struct{}{}
	var out []map[string]string
	for _, s := range series {
		m := seriesMetric(s)
		key := metricKey(m)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, m)
	}
	if out == nil {
		out = []map[string]string{}
	}
	writePromOK(w, limitMetrics(out, atoi(r.FormValue("limit"), 0)))
}

func promParseQuery(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writePromErr(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	expr := r.FormValue("query")
	if strings.TrimSpace(expr) == "" {
		writePromErr(w, http.StatusBadRequest, "bad_data", "missing query")
		return
	}
	ast, err := parsePromAST(expr)
	if err != nil {
		writePromErr(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	writePromOK(w, ast)
}

func loadPromSeries(ctx context.Context, ob contract.Observer, matches []string, startUS, endUS int64) ([]contract.Series, error) {
	if len(matches) == 0 {
		return ob.QueryMetrics(ctx, contract.MetricQuery{StartUS: startUS, EndUS: endUS})
	}
	var out []contract.Series
	for _, m := range matches {
		ser, err := ob.QueryMetrics(ctx, contract.MetricQuery{Expr: m, StartUS: startUS, EndUS: endUS})
		if err != nil {
			return nil, err
		}
		out = append(out, ser...)
	}
	return out, nil
}

func seriesToVector(series []contract.Series) []map[string]any {
	out := make([]map[string]any, 0, len(series))
	for _, s := range series {
		if len(s.Points) == 0 {
			continue
		}
		p := s.Points[len(s.Points)-1]
		out = append(out, map[string]any{
			"metric": seriesMetric(s),
			"value":  valueTuple(p[0], p[1]),
		})
	}
	return out
}

func seriesToMatrix(series []contract.Series) []map[string]any {
	out := make([]map[string]any, 0, len(series))
	for _, s := range series {
		vals := make([][]any, 0, len(s.Points))
		for _, p := range s.Points {
			vals = append(vals, valueTuple(p[0], p[1]))
		}
		out = append(out, map[string]any{
			"metric": seriesMetric(s),
			"values": vals,
		})
	}
	return out
}

func seriesMetric(s contract.Series) map[string]string {
	m := map[string]string{}
	for k, v := range s.Labels {
		m[k] = v
	}
	if s.Name != "" {
		m["__name__"] = s.Name
	}
	return m
}

func valueTuple(ts, val float64) []any {
	return []any{ts, strconv.FormatFloat(val, 'f', -1, 64)}
}

func parseUnixSecUS(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty time")
	}
	sec, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return int64(sec * 1e6), nil
}

func optionalUnixUS(s string) int64 {
	us, err := parseUnixSecUS(s)
	if err != nil {
		return 0
	}
	return us
}

func parseStepUS(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty step")
	}
	if sec, err := strconv.ParseFloat(s, 64); err == nil {
		if sec <= 0 {
			return 0, fmt.Errorf("step must be > 0")
		}
		return int64(sec * 1e6), nil
	}
	us, err := ql.ParseDurationUS(s)
	if err != nil {
		return 0, err
	}
	if us <= 0 {
		return 0, fmt.Errorf("step must be > 0")
	}
	return us, nil
}

func withPromTimeout(r *http.Request) (context.Context, context.CancelFunc, error) {
	t := strings.TrimSpace(r.FormValue("timeout"))
	if t == "" {
		return r.Context(), func() {}, nil
	}
	var d time.Duration
	if sec, err := strconv.ParseFloat(t, 64); err == nil {
		d = time.Duration(sec * float64(time.Second))
	} else {
		us, err := ql.ParseDurationUS(t)
		if err != nil {
			return nil, nil, fmt.Errorf("bad timeout")
		}
		d = time.Duration(us) * time.Microsecond
	}
	if d <= 0 {
		return nil, nil, fmt.Errorf("bad timeout")
	}
	ctx, cancel := context.WithTimeout(r.Context(), d)
	return ctx, cancel, nil
}

func writePromOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": data})
}

func writePromErr(w http.ResponseWriter, code int, errType, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "error",
		"errorType": errType,
		"error":     msg,
	})
}

func parsePromAST(expr string) (any, error) {
	rx, err := ql.ParseRangePromQL(expr)
	if err != nil {
		return nil, err
	}
	node := any(vectorSelectorAST(rx.Name, rx.Labels))
	if rx.RateUS > 0 {
		node = promCall{
			Type: "call",
			Func: promFunc{
				Name:       "rate",
				ArgTypes:   []string{"matrix"},
				Variadic:   0,
				ReturnType: "vector",
			},
			Args: []any{matrixSelectorAST(rx.Name, rx.Labels, rx.RateUS)},
		}
	}
	if len(rx.GroupBy) > 0 {
		node = promAgg{
			Type:     "aggregation",
			Expr:     node,
			Op:       "avg",
			Param:    nil,
			Grouping: rx.GroupBy,
			Without:  false,
		}
	}
	return node, nil
}

func selectorMatchers(name string, labels map[string]string) []promMatcher {
	out := []promMatcher{}
	if name != "" {
		out = append(out, promMatcher{Type: "=", Name: "__name__", Value: name})
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, promMatcher{Type: "=", Name: k, Value: labels[k]})
	}
	return out
}

func vectorSelectorAST(name string, labels map[string]string) promVectorSel {
	return promVectorSel{
		Type:     "vectorSelector",
		Name:     name,
		Matchers: selectorMatchers(name, labels),
	}
}

func matrixSelectorAST(name string, labels map[string]string, rangeUS int64) promMatrixSel {
	return promMatrixSel{
		Type:     "matrixSelector",
		Name:     name,
		Matchers: selectorMatchers(name, labels),
		Range:    rangeUS / 1000,
	}
}

type promMatcher struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type promVectorSel struct {
	Type       string        `json:"type"`
	Name       string        `json:"name"`
	Matchers   []promMatcher `json:"matchers"`
	Offset     int           `json:"offset"`
	Timestamp  *int64        `json:"timestamp"`
	StartOrEnd *string       `json:"startOrEnd"`
}

type promMatrixSel struct {
	Type       string        `json:"type"`
	Name       string        `json:"name"`
	Matchers   []promMatcher `json:"matchers"`
	Range      int64         `json:"range"`
	Offset     int           `json:"offset"`
	Timestamp  *int64        `json:"timestamp"`
	StartOrEnd *string       `json:"startOrEnd"`
}

type promFunc struct {
	Name       string   `json:"name"`
	ArgTypes   []string `json:"argTypes"`
	Variadic   int      `json:"variadic"`
	ReturnType string   `json:"returnType"`
}

type promCall struct {
	Type string   `json:"type"`
	Func promFunc `json:"func"`
	Args []any    `json:"args"`
}

type promAgg struct {
	Type     string   `json:"type"`
	Expr     any      `json:"expr"`
	Op       string   `json:"op"`
	Param    any      `json:"param"`
	Grouping []string `json:"grouping"`
	Without  bool     `json:"without"`
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func limitStrings(in []string, limit int) []string {
	if limit > 0 && len(in) > limit {
		return in[:limit]
	}
	return in
}

func limitMetrics(in []map[string]string, limit int) []map[string]string {
	if limit > 0 && len(in) > limit {
		return in[:limit]
	}
	return in
}

func metricKey(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(m[k])
		b.WriteByte(',')
	}
	return b.String()
}
