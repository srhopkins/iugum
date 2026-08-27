package memadapt

import (
	"context"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

func TestIngestQueryMetrics(t *testing.T) {
	o := New()
	ctx := context.Background()
	if err := o.IngestMetrics(ctx, []contract.Sample{
		{Name: "cpu_c", Value: 46, Labels: map[string]string{"host": "a"}, TimeUS: 100},
		{Name: "fan_pct", Value: 40, Labels: map[string]string{"host": "a"}, TimeUS: 100},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := o.QueryMetrics(ctx, contract.MetricQuery{Name: "cpu_c"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "cpu_c" || len(got[0].Points) != 1 {
		t.Fatalf("got %+v", got)
	}
	if got[0].Points[0][1] != 46 {
		t.Fatalf("value %v", got[0].Points[0][1])
	}
}

func TestQueryMetricsLabelFilter(t *testing.T) {
	o := New()
	ctx := context.Background()
	_ = o.IngestMetrics(ctx, []contract.Sample{
		{Name: "cpu_c", Value: 1, Labels: map[string]string{"host": "a"}},
		{Name: "cpu_c", Value: 2, Labels: map[string]string{"host": "b"}},
	})
	got, err := o.QueryMetrics(ctx, contract.MetricQuery{
		Name:   "cpu_c",
		Labels: map[string]string{"host": "b"},
	})
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if got[0].Points[0][1] != 2 {
		t.Fatalf("value %v", got[0].Points[0][1])
	}
}

func TestIngestSearchLogs(t *testing.T) {
	o := New()
	ctx := context.Background()
	if err := o.IngestLogs(ctx, []contract.Log{
		{Stream: "homelab", Message: "MI50 junction 81"},
		{Stream: "other", Message: "skip"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := o.SearchLogs(ctx, contract.LogQuery{Stream: "homelab", Text: "junction"})
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if got[0].Message != "MI50 junction 81" {
		t.Fatalf("message %q", got[0].Message)
	}
}

func TestQueryMetricRangeStep(t *testing.T) {
	o := New()
	ctx := context.Background()
	_ = o.IngestMetrics(ctx, []contract.Sample{
		{Name: "cpu_c", Value: 10, TimeUS: 1_000_000},
		{Name: "cpu_c", Value: 20, TimeUS: 2_000_000},
		{Name: "cpu_c", Value: 30, TimeUS: 3_000_000},
		{Name: "cpu_c", Value: 40, TimeUS: 4_000_000},
	})
	got, err := o.QueryMetricRange(ctx, contract.MetricRangeQuery{
		Expr:    "cpu_c",
		StartUS: 1_000_000,
		EndUS:   4_000_000,
		StepUS:  2_000_000,
	})
	if err != nil || len(got) != 1 || len(got[0].Points) != 2 {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestSearchLogRange(t *testing.T) {
	o := New()
	ctx := context.Background()
	_ = o.IngestLogs(ctx, []contract.Log{
		{Stream: "homelab", Message: "fan 40", TimeUS: 2_000_000},
		{Stream: "homelab", Message: "skip", TimeUS: 9_000_000},
	})
	got, err := o.SearchLogRange(ctx, contract.LogRangeQuery{
		Expr:    `{stream="homelab"} |= "fan"`,
		StartUS: 1_000_000,
		EndUS:   3_000_000,
	})
	if err != nil || len(got) != 1 || got[0].Message != "fan 40" {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestQueryMetricRangeEmpty(t *testing.T) {
	o := New()
	got, err := o.QueryMetricRange(context.Background(), contract.MetricRangeQuery{
		Expr:    "missing",
		StartUS: 1,
		EndUS:   2,
		StepUS:  1,
	})
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v err=%v", got, err)
	}
}
