package sqladapt

import (
	"context"
	"path/filepath"
	"testing"

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
