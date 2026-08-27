package sqladapt

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/srhopkins/iugum/adapter/observe/ql"
	"github.com/srhopkins/iugum/contract"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "observe.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestIngestJSONAndQueryName(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	samples, err := ql.ParseMetricsJSON([]byte(`[
		{"name":"junction_c","value":78.5,"labels":{"gpu":"mi50"}},
		{"name":"cpu_c","value":45}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.IngestMetrics(ctx, samples); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryMetrics(ctx, contract.MetricQuery{Name: "junction_c"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "junction_c" || len(got[0].Points) != 1 {
		t.Fatalf("series %+v", got)
	}
	if got[0].Points[0][1] != 78.5 {
		t.Fatalf("value %v — want °C not millidegrees", got[0].Points[0][1])
	}
}

func TestIngestPromTextMillidegrees(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	samples, err := ql.ParsePromText(`junction_temp_millidegree_celsius{gpu="mi50"} 102000`)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.IngestMetrics(ctx, samples); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryMetrics(ctx, contract.MetricQuery{Expr: `junction_c{gpu="mi50"}`})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("series %+v", got)
	}
	v := got[0].Points[0][1]
	if v < 101.9 || v > 102.1 {
		t.Fatalf("value %v want 102 °C", v)
	}
}

func TestQueryNameFilter(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_ = s.IngestMetrics(ctx, []contract.Sample{
		{Name: "junction_c", Value: 80},
		{Name: "edge_c", Value: 70},
	})
	got, err := s.QueryMetrics(ctx, contract.MetricQuery{Name: "edge_c"})
	if err != nil || len(got) != 1 || got[0].Name != "edge_c" {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestLogFTS(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if err := s.IngestLogs(ctx, []contract.Log{
		{Stream: "homelab", Message: "SYS_FAN1 duty 40 percent"},
		{Stream: "homelab", Message: "wiki started"},
		{Stream: "other", Message: "fan ignored"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.SearchLogs(ctx, contract.LogQuery{Expr: `{stream="homelab"} |= "fan"`})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Message != "SYS_FAN1 duty 40 percent" {
		t.Fatalf("got %+v", got)
	}
}

func TestCelsiusNotMillidegreesOnIngest(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if err := s.IngestMetrics(ctx, []contract.Sample{{Name: "junction_c", Value: 78500}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryMetrics(ctx, contract.MetricQuery{Name: "junction_c"})
	if err != nil {
		t.Fatal(err)
	}
	v := got[0].Points[0][1]
	if v > 200 {
		t.Fatalf("stored millidegrees %v", v)
	}
}

func TestQueryMetricRangeStep(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_ = s.IngestMetrics(ctx, []contract.Sample{
		{Name: "junction_c", Value: 70, TimeUS: 1_000_000},
		{Name: "junction_c", Value: 80, TimeUS: 2_000_000},
		{Name: "junction_c", Value: 90, TimeUS: 3_000_000},
		{Name: "junction_c", Value: 100, TimeUS: 4_000_000},
	})
	got, err := s.QueryMetricRange(ctx, contract.MetricRangeQuery{
		Expr:    "junction_c",
		StartUS: 1_000_000,
		EndUS:   4_000_000,
		StepUS:  2_000_000,
	})
	if err != nil || len(got) != 1 || len(got[0].Points) != 2 {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if got[0].Points[0][1] != 70 || got[0].Points[1][1] != 90 {
		t.Fatalf("values %+v", got[0].Points)
	}
}

func TestQueryMetricRangeEmpty(t *testing.T) {
	s := openTest(t)
	got, err := s.QueryMetricRange(context.Background(), contract.MetricRangeQuery{
		Expr:    "junction_c",
		StartUS: 1,
		EndUS:   2,
		StepUS:  1,
	})
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestQueryMetricRangeLabelSelector(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_ = s.IngestMetrics(ctx, []contract.Sample{
		{Name: "junction_c", Labels: map[string]string{"gpu": "mi50"}, Value: 78, TimeUS: 1_000_000},
		{Name: "junction_c", Labels: map[string]string{"gpu": "vii"}, Value: 60, TimeUS: 1_000_000},
		{Name: "junction_c", Labels: map[string]string{"gpu": "mi50"}, Value: 82, TimeUS: 3_000_000},
	})
	got, err := s.QueryMetricRange(ctx, contract.MetricRangeQuery{
		Expr:    `junction_c{gpu="mi50"}`,
		StartUS: 1_000_000,
		EndUS:   3_000_000,
		StepUS:  2_000_000,
	})
	if err != nil || len(got) != 1 || len(got[0].Points) != 2 {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if got[0].Labels["gpu"] != "mi50" || got[0].Points[0][1] != 78 || got[0].Points[1][1] != 82 {
		t.Fatalf("values %+v", got)
	}
}

func TestQueryMetricRangeLookback(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_ = s.IngestMetrics(ctx, []contract.Sample{
		{Name: "junction_c", Value: 70, TimeUS: 500_000},
		{Name: "junction_c", Value: 80, TimeUS: 2_000_000},
	})
	got, err := s.QueryMetricRange(ctx, contract.MetricRangeQuery{
		Expr:    "junction_c",
		StartUS: 1_000_000,
		EndUS:   3_000_000,
		StepUS:  1_000_000,
	})
	if err != nil || len(got) != 1 || len(got[0].Points) != 3 {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if got[0].Points[0][0] != 1 || got[0].Points[0][1] != 70 {
		t.Fatalf("lookback first %+v", got[0].Points[0])
	}
}

func TestQueryMetricRangeNoStep(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_ = s.IngestMetrics(ctx, []contract.Sample{
		{Name: "junction_c", Value: 70, TimeUS: 500_000},
		{Name: "junction_c", Value: 80, TimeUS: 2_000_000},
	})
	got, err := s.QueryMetricRange(ctx, contract.MetricRangeQuery{
		Expr:    "junction_c",
		StartUS: 1_000_000,
		EndUS:   3_000_000,
	})
	if err != nil || len(got) != 1 || len(got[0].Points) != 1 || got[0].Points[0][1] != 80 {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestQueryMetricRangeRate(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_ = s.IngestMetrics(ctx, []contract.Sample{
		{Name: "junction_c", Value: 10, TimeUS: 1_000_000},
		{Name: "junction_c", Value: 20, TimeUS: 11_000_000},
		{Name: "junction_c", Value: 30, TimeUS: 21_000_000},
	})
	got, err := s.QueryMetricRange(ctx, contract.MetricRangeQuery{
		Expr:    `rate(junction_c[20s])`,
		StartUS: 21_000_000,
		EndUS:   21_000_000,
		StepUS:  21_000_000,
	})
	if err != nil || len(got) != 1 || len(got[0].Points) != 1 {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if got[0].Points[0][1] != 1 {
		t.Fatalf("rate %v want 1 /s", got[0].Points[0][1])
	}
}

func TestQueryMetricRangeAvgBy(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_ = s.IngestMetrics(ctx, []contract.Sample{
		{Name: "junction_c", Labels: map[string]string{"gpu": "mi50", "rack": "a"}, Value: 80, TimeUS: 1_000_000},
		{Name: "junction_c", Labels: map[string]string{"gpu": "vii", "rack": "a"}, Value: 60, TimeUS: 1_000_000},
	})
	got, err := s.QueryMetricRange(ctx, contract.MetricRangeQuery{
		Expr:    `avg by (rack) (junction_c)`,
		StartUS: 1_000_000,
		EndUS:   1_000_000,
		StepUS:  1_000_000,
	})
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if got[0].Labels["rack"] != "a" || len(got[0].Labels) != 1 || got[0].Points[0][1] != 70 {
		t.Fatalf("avg %+v", got[0])
	}
}

func TestQueryMetricRangeBadExpr(t *testing.T) {
	s := openTest(t)
	_, err := s.QueryMetricRange(context.Background(), contract.MetricRangeQuery{
		Expr:    "rate(junction_c)",
		StartUS: 1,
		EndUS:   2,
		StepUS:  1,
	})
	if err == nil {
		t.Fatal("want parse error")
	}
}

func TestQueryMetricsInstantAfterRange(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_ = s.IngestMetrics(ctx, []contract.Sample{
		{Name: "junction_c", Value: 78.5, TimeUS: 1_000_000},
		{Name: "junction_c", Value: 80, TimeUS: 2_000_000},
	})
	_, err := s.QueryMetricRange(ctx, contract.MetricRangeQuery{
		Expr:    "junction_c",
		StartUS: 1_000_000,
		EndUS:   2_000_000,
		StepUS:  1_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryMetrics(ctx, contract.MetricQuery{Expr: "junction_c"})
	if err != nil || len(got) != 1 || len(got[0].Points) != 2 {
		t.Fatalf("instant %+v err=%v", got, err)
	}
	if got[0].Points[0][1] != 78.5 {
		t.Fatalf("value %v want °C", got[0].Points[0][1])
	}
}

func TestSearchLogRange(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_ = s.IngestLogs(ctx, []contract.Log{
		{Stream: "homelab", Message: "SYS_FAN1 duty 40", TimeUS: 2_000_000},
		{Stream: "homelab", Message: "later", TimeUS: 9_000_000},
	})
	var fts int
	if err := s.logs.QueryRow(`SELECT count(*) FROM logs_fts`).Scan(&fts); err != nil || fts != 2 {
		t.Fatalf("logs_fts rows=%d err=%v", fts, err)
	}
	got, err := s.SearchLogRange(ctx, contract.LogRangeQuery{
		Expr:    `{stream="homelab"} |= "fan"`,
		StartUS: 1_000_000,
		EndUS:   3_000_000,
	})
	if err != nil || len(got) != 1 || got[0].Message != "SYS_FAN1 duty 40" {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestSearchLogRangeLimit(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_ = s.IngestLogs(ctx, []contract.Log{
		{Stream: "homelab", Message: "one", TimeUS: 1_000_000},
		{Stream: "homelab", Message: "two", TimeUS: 2_000_000},
		{Stream: "homelab", Message: "three", TimeUS: 3_000_000},
	})
	got, err := s.SearchLogRange(ctx, contract.LogRangeQuery{
		Expr:    `{stream="homelab"}`,
		StartUS: 1,
		EndUS:   4_000_000,
		Limit:   2,
	})
	if err != nil || len(got) != 2 {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if got[0].Message != "three" || got[1].Message != "two" {
		t.Fatalf("want newest first, got %+v", got)
	}
}

func TestSearchLogRangeStreamLabels(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_ = s.IngestLogs(ctx, []contract.Log{
		{Stream: "homelab", Level: "error", Message: "fan stall", Attrs: map[string]string{"host": "tower"}, TimeUS: 2_000_000},
		{Stream: "homelab", Level: "info", Message: "fan ok", Attrs: map[string]string{"host": "tower"}, TimeUS: 2_100_000},
		{Stream: "homelab", Level: "error", Message: "fan stall", Attrs: map[string]string{"host": "mini"}, TimeUS: 2_200_000},
	})
	got, err := s.SearchLogRange(ctx, contract.LogRangeQuery{
		Expr:    `{stream="homelab", level="error", host="tower"} |= "fan"`,
		StartUS: 1_000_000,
		EndUS:   3_000_000,
	})
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if got[0].Level != "error" || got[0].Attrs["host"] != "tower" {
		t.Fatalf("labels %+v", got[0])
	}
}

func TestOpenDirCreatesSplitFiles(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if !fileExists(filepath.Join(dir, metricsFile)) || !fileExists(filepath.Join(dir, logsFile)) {
		t.Fatal("missing split sqlite files")
	}
}

func TestMigrateLegacyObserveDB(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, legacyFile)
	src, err := sql.Open("sqlite", old)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.Exec(metricsSchema + logsSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Exec(`INSERT INTO samples(name, labels, value, time_us) VALUES ('junction_c', '{}', 71, 1000)`); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Exec(`INSERT INTO logs(time_us, stream, level, message, attrs) VALUES (2000, 'homelab', 'info', 'ok', '{}')`); err != nil {
		t.Fatal(err)
	}
	_ = src.Close()

	s, err := OpenDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if fileExists(old) {
		t.Fatal("legacy observe.db should become observe.db.bak")
	}
	if !fileExists(old + ".bak") {
		t.Fatal("missing observe.db.bak")
	}
	ctx := context.Background()
	series, err := s.QueryMetrics(ctx, contract.MetricQuery{Name: "junction_c"})
	if err != nil || len(series) != 1 || series[0].Points[0][1] != 71 {
		t.Fatalf("metrics %+v err=%v", series, err)
	}
	logs, err := s.SearchLogs(ctx, contract.LogQuery{Stream: "homelab"})
	if err != nil || len(logs) != 1 || logs[0].Message != "ok" {
		t.Fatalf("logs %+v err=%v", logs, err)
	}
}
