package sqladapt

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"github.com/srhopkins/iugum/adapter/observe/ql"
	"github.com/srhopkins/iugum/contract"
	observeui "github.com/srhopkins/iugum/web/observe"
)

// TempMarksC are horizontal graph lines: fan floor, VBIOS crit, emergency.
var TempMarksC = []float64{50, 100, 105}

// Handler serves the embedded UI plus ingest and query.
func Handler(ob contract.Observer) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /ingest/metrics", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var samples []contract.Sample
		if ql.LooksLikeProm(body, r.Header.Get("Content-Type")) {
			samples, err = ql.ParsePromText(string(body))
		} else {
			samples, err = ql.ParseMetricsJSON(body)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := ob.IngestMetrics(r.Context(), samples); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{"ingested": len(samples)})
	})
	mux.HandleFunc("POST /ingest/logs", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		recs, err := ql.ParseLogsJSON(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := ob.IngestLogs(r.Context(), recs); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{"ingested": len(recs)})
	})
	mux.HandleFunc("GET /query/metrics", func(w http.ResponseWriter, r *http.Request) {
		q := contract.MetricQuery{
			Expr:      r.URL.Query().Get("q"),
			MaxPoints: atoi(r.URL.Query().Get("max"), 2000),
		}
		if v := r.URL.Query().Get("start"); v != "" {
			if sec, err := strconv.ParseFloat(v, 64); err == nil {
				q.StartUS = int64(sec * 1e6)
			}
		}
		if v := r.URL.Query().Get("end"); v != "" {
			if sec, err := strconv.ParseFloat(v, 64); err == nil {
				q.EndUS = int64(sec * 1e6)
			}
		}
		series, err := ob.QueryMetrics(r.Context(), q)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metricsResponse{
			Series: series,
			Marks:  TempMarksC,
			Unit:   "C",
		})
	})
	mux.HandleFunc("GET /query/logs", func(w http.ResponseWriter, r *http.Request) {
		q := contract.LogQuery{
			Expr:  r.URL.Query().Get("q"),
			Limit: atoi(r.URL.Query().Get("limit"), 200),
		}
		logs, err := ob.SearchLogs(r.Context(), q)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"logs": logs})
	})
	mux.HandleFunc("GET /meta.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Meta())
	})
	ui, err := fs.Sub(observeui.Dist, "dist")
	if err != nil {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "observe UI missing", http.StatusInternalServerError)
		})
		return mux
	}
	file := http.FileServer(http.FS(ui))
	mux.Handle("/", file)
	return mux
}

type metricsResponse struct {
	Series []contract.Series `json:"series"`
	Marks  []float64         `json:"marks"`
	Unit   string            `json:"unit"`
}

// Meta is chart config the UI and tests share.
func Meta() map[string]any {
	return map[string]any{
		"marks_c": TempMarksC,
		"series":  []string{"junction_c", "edge_c", "mem_c", "cpu_c", "fan_pct"},
		"unit":    "C",
		"fan":     "SYS_FAN1",
	}
}

func atoi(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return def
	}
	return n
}
