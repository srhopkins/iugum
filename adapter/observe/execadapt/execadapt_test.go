package execadapt

import (
	"context"
	"reflect"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

func TestFields(t *testing.T) {
	got := fields("sh  -c\tscript")
	want := []string{"sh", "-c", "script"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fields = %v, want %v", got, want)
	}
}

func TestIngestMetricsRunsArgv(t *testing.T) {
	o := Observer{cmd: []string{"true"}}
	err := o.IngestMetrics(context.Background(), []contract.Sample{{Name: "cpu_c", Value: 1}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryMetricsReadsJSONStdout(t *testing.T) {
	o := Observer{cmd: []string{"sh", "-c", `echo '[{"name":"n","points":[[1,2]]}]'`}}
	got, err := o.QueryMetrics(context.Background(), contract.MetricQuery{Name: "n"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "n" || got[0].Points[0][1] != 2 {
		t.Fatalf("got %+v", got)
	}
}

func TestSearchLogsReadsJSONStdout(t *testing.T) {
	o := Observer{cmd: []string{"sh", "-c", `echo '[{"stream":"app","message":"hi"}]'`}}
	got, err := o.SearchLogs(context.Background(), contract.LogQuery{Stream: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Stream != "app" || got[0].Message != "hi" {
		t.Fatalf("got %+v", got)
	}
}

func TestQueryMetricRangeReadsJSONStdout(t *testing.T) {
	o := Observer{cmd: []string{"sh", "-c", `echo '[{"name":"n","points":[[1,2]]}]'`}}
	got, err := o.QueryMetricRange(context.Background(), contract.MetricRangeQuery{
		Expr: "n", StartUS: 1, EndUS: 2, StepUS: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "n" || got[0].Points[0][1] != 2 {
		t.Fatalf("got %+v", got)
	}
}

func TestSearchLogRangeReadsJSONStdout(t *testing.T) {
	o := Observer{cmd: []string{"sh", "-c", `echo '[{"stream":"app","message":"hi"}]'`}}
	got, err := o.SearchLogRange(context.Background(), contract.LogRangeQuery{Expr: `{stream="app"}`})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Stream != "app" {
		t.Fatalf("got %+v", got)
	}
}

func TestCallReportsStderr(t *testing.T) {
	o := Observer{cmd: []string{"sh", "-c", `echo fail >&2; exit 1`}}
	err := o.IngestLogs(context.Background(), []contract.Log{{Message: "x"}})
	if err == nil {
		t.Fatal("want exec error")
	}
}
