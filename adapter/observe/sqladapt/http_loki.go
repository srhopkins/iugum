package sqladapt

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/srhopkins/iugum/adapter/observe/ql"
	"github.com/srhopkins/iugum/contract"
)

const (
	lokiDefaultLimit = 100
	lokiScanLimit    = 10000
)

// RegisterLokiHTTP adds the Loki HTTP the Perses plugin calls.
func RegisterLokiHTTP(mux *http.ServeMux, ob contract.Observer) {
	mux.HandleFunc("GET /loki/api/v1/query", lokiQuery(ob))
	mux.HandleFunc("GET /loki/api/v1/query_range", lokiQueryRange(ob))
	mux.HandleFunc("GET /loki/api/v1/labels", lokiLabels(ob))
	mux.HandleFunc("GET /loki/api/v1/label/{labelName}/values", lokiLabelValues(ob))
	mux.HandleFunc("GET /loki/api/v1/series", lokiSeries(ob))
	mux.HandleFunc("GET /loki/api/v1/index/volume", lokiVolume(ob, false))
	mux.HandleFunc("GET /loki/api/v1/index/volume_range", lokiVolume(ob, true))
	mux.HandleFunc("GET /loki/api/v1/index/stats", lokiIndexStats(ob))
}

func lokiQuery(ob contract.Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit := atoi(q.Get("limit"), lokiDefaultLimit)
		logs, err := ob.SearchLogs(r.Context(), contract.LogQuery{
			Expr:  q.Get("query"),
			EndUS: unixSecToUS(q.Get("time")),
			Limit: limit,
		})
		if err != nil {
			writeLokiErr(w, err)
			return
		}
		logs = applyDirection(logs, q.Get("direction"))
		writeLokiOK(w, lokiResult{ResultType: "streams", Result: logsToStreams(logs)})
	}
}

func lokiQueryRange(ob contract.Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit := atoi(q.Get("limit"), lokiDefaultLimit)
		logs, err := ob.SearchLogRange(r.Context(), contract.LogRangeQuery{
			Expr:    q.Get("query"),
			StartUS: unixSecToUS(q.Get("start")),
			EndUS:   unixSecToUS(q.Get("end")),
			StepUS:  parseLokiStepUS(q.Get("step")),
			Limit:   limit,
		})
		if err != nil {
			writeLokiErr(w, err)
			return
		}
		logs = applyDirection(logs, q.Get("direction"))
		writeLokiOK(w, lokiResult{ResultType: "streams", Result: logsToStreams(logs)})
	}
}

func lokiLabels(ob contract.Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		logs, err := lokiScan(r.Context(), ob, q.Get("query"), unixSecToUS(q.Get("start")), unixSecToUS(q.Get("end")))
		if err != nil {
			writeLokiErr(w, err)
			return
		}
		seen := map[string]struct{}{}
		for _, rec := range logs {
			for k := range logLabels(rec) {
				seen[k] = struct{}{}
			}
		}
		writeLokiOK(w, lokiSortedKeys(seen))
	}
}

func lokiLabelValues(ob contract.Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		name := r.PathValue("labelName")
		logs, err := lokiScan(r.Context(), ob, q.Get("query"), unixSecToUS(q.Get("start")), unixSecToUS(q.Get("end")))
		if err != nil {
			writeLokiErr(w, err)
			return
		}
		seen := map[string]struct{}{}
		for _, rec := range logs {
			if v := logLabels(rec)[name]; v != "" {
				seen[v] = struct{}{}
			}
		}
		writeLokiOK(w, lokiSortedKeys(seen))
	}
}

func lokiSeries(ob contract.Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		startUS := unixSecToUS(q.Get("start"))
		endUS := unixSecToUS(q.Get("end"))
		matches := q["match[]"]
		if len(matches) == 0 {
			matches = []string{""}
		}
		seen := map[string]map[string]string{}
		order := []string{}
		for _, expr := range matches {
			logs, err := lokiScan(r.Context(), ob, expr, startUS, endUS)
			if err != nil {
				writeLokiErr(w, err)
				return
			}
			for _, rec := range logs {
				labs := logLabels(rec)
				key := labelsKey(labs)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = labs
				order = append(order, key)
			}
		}
		out := make([]map[string]string, 0, len(order))
		for _, key := range order {
			out = append(out, seen[key])
		}
		writeLokiOK(w, out)
	}
}

func lokiVolume(ob contract.Observer, rng bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		startUS := unixSecToUS(q.Get("start"))
		endUS := unixSecToUS(q.Get("end"))
		stepUS := parseLokiStepUS(q.Get("step"))
		limit := atoi(q.Get("limit"), lokiDefaultLimit)
		logs, err := lokiScan(r.Context(), ob, q.Get("query"), startUS, endUS)
		if err != nil {
			writeLokiErr(w, err)
			return
		}
		if rng {
			writeLokiOK(w, lokiResult{ResultType: "matrix", Result: volumeMatrix(logs, startUS, endUS, stepUS, limit)})
			return
		}
		writeLokiOK(w, lokiResult{ResultType: "vector", Result: volumeVector(logs, endUS, limit)})
	}
}

func lokiIndexStats(ob contract.Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		logs, err := lokiScan(r.Context(), ob, q.Get("query"), unixSecToUS(q.Get("start")), unixSecToUS(q.Get("end")))
		if err != nil {
			writeLokiErr(w, err)
			return
		}
		streams := map[string]struct{}{}
		var bytes int
		for _, rec := range logs {
			streams[labelsKey(logLabels(rec))] = struct{}{}
			bytes += len(rec.Message)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{
			"streams": len(streams),
			"chunks":  len(streams),
			"entries": len(logs),
			"bytes":   bytes,
		})
	}
}

func lokiScan(ctx context.Context, ob contract.Observer, expr string, startUS, endUS int64) ([]contract.Log, error) {
	if startUS != 0 || endUS != 0 {
		return ob.SearchLogRange(ctx, contract.LogRangeQuery{
			Expr: expr, StartUS: startUS, EndUS: endUS, Limit: lokiScanLimit,
		})
	}
	return ob.SearchLogs(ctx, contract.LogQuery{Expr: expr, Limit: lokiScanLimit})
}

type lokiEnvelope struct {
	Status string `json:"status"`
	Data   any    `json:"data"`
}

type lokiResult struct {
	ResultType string `json:"resultType"`
	Result     any    `json:"result"`
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

type lokiVector struct {
	Metric map[string]string `json:"metric"`
	Value  [2]any            `json:"value"`
}

type lokiMatrix struct {
	Metric map[string]string `json:"metric"`
	Values [][2]any          `json:"values"`
}

func writeLokiOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(lokiEnvelope{Status: "success", Data: data})
}

func writeLokiErr(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "error",
		"errorType": "bad_data",
		"error":     err.Error(),
	})
}

func unixSecToUS(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	sec, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(sec * 1e6)
}

func parseLokiStepUS(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if sec, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(sec * 1e6)
	}
	us, err := ql.ParseDurationUS(s)
	if err != nil {
		return 0
	}
	return us
}

func applyDirection(logs []contract.Log, dir string) []contract.Log {
	if !strings.EqualFold(dir, "forward") {
		return logs
	}
	out := append([]contract.Log(nil), logs...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func logsToStreams(logs []contract.Log) []lokiStream {
	type bucket struct {
		labels map[string]string
		values [][2]string
	}
	by := map[string]*bucket{}
	order := []string{}
	for _, rec := range logs {
		labs := logLabels(rec)
		key := labelsKey(labs)
		b, ok := by[key]
		if !ok {
			b = &bucket{labels: labs}
			by[key] = b
			order = append(order, key)
		}
		b.values = append(b.values, [2]string{strconv.FormatInt(rec.TimeUS*1000, 10), rec.Message})
	}
	out := make([]lokiStream, 0, len(order))
	for _, key := range order {
		b := by[key]
		out = append(out, lokiStream{Stream: b.labels, Values: b.values})
	}
	return out
}

func logLabels(r contract.Log) map[string]string {
	m := map[string]string{}
	if r.Stream != "" {
		m["stream"] = r.Stream
	}
	if r.Level != "" {
		m["level"] = r.Level
	}
	for k, v := range r.Attrs {
		if v != "" {
			m[k] = v
		}
	}
	return m
}

func labelsKey(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
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

func lokiSortedKeys(seen map[string]struct{}) []string {
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func volumeVector(logs []contract.Log, endUS int64, limit int) []lokiVector {
	type acc struct {
		metric map[string]string
		bytes  int
	}
	by := map[string]*acc{}
	order := []string{}
	for _, rec := range logs {
		labs := logLabels(rec)
		key := labelsKey(labs)
		a, ok := by[key]
		if !ok {
			a = &acc{metric: labs}
			by[key] = a
			order = append(order, key)
		}
		a.bytes += len(rec.Message)
	}
	if limit <= 0 || limit > len(order) {
		limit = len(order)
	}
	ts := float64(endUS) / 1e6
	out := make([]lokiVector, 0, limit)
	for _, key := range order[:limit] {
		a := by[key]
		out = append(out, lokiVector{
			Metric: a.metric,
			Value:  [2]any{ts, strconv.Itoa(a.bytes)},
		})
	}
	return out
}

func volumeMatrix(logs []contract.Log, startUS, endUS, stepUS int64, limit int) []lokiMatrix {
	if stepUS <= 0 {
		vec := volumeVector(logs, endUS, limit)
		out := make([]lokiMatrix, 0, len(vec))
		for _, v := range vec {
			out = append(out, lokiMatrix{Metric: v.Metric, Values: [][2]any{v.Value}})
		}
		return out
	}
	type acc struct {
		metric map[string]string
		bytes  map[int64]int
	}
	by := map[string]*acc{}
	order := []string{}
	for _, rec := range logs {
		labs := logLabels(rec)
		key := labelsKey(labs)
		a, ok := by[key]
		if !ok {
			a = &acc{metric: labs, bytes: map[int64]int{}}
			by[key] = a
			order = append(order, key)
		}
		t := rec.TimeUS
		if startUS != 0 && t < startUS {
			continue
		}
		bucket := t
		if startUS != 0 {
			bucket = startUS + ((t-startUS)/stepUS)*stepUS
		} else {
			bucket = (t / stepUS) * stepUS
		}
		a.bytes[bucket] += len(rec.Message)
	}
	if limit <= 0 || limit > len(order) {
		limit = len(order)
	}
	out := make([]lokiMatrix, 0, limit)
	for _, key := range order[:limit] {
		a := by[key]
		ts := make([]int64, 0, len(a.bytes))
		for t := range a.bytes {
			ts = append(ts, t)
		}
		sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })
		vals := make([][2]any, 0, len(ts))
		for _, t := range ts {
			vals = append(vals, [2]any{float64(t) / 1e6, strconv.Itoa(a.bytes[t])})
		}
		out = append(out, lokiMatrix{Metric: a.metric, Values: vals})
	}
	return out
}
