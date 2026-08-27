package ql

import (
	"testing"

	"github.com/srhopkins/iugum/contract"
)

func TestParsePromQLName(t *testing.T) {
	p, err := ParsePromQL("junction_c")
	if err != nil || p.Name != "junction_c" || len(p.Labels) != 0 {
		t.Fatalf("got %+v err=%v", p, err)
	}
}

func TestParsePromQLLabels(t *testing.T) {
	p, err := ParsePromQL(`junction_c{gpu="mi50"}`)
	if err != nil || p.Name != "junction_c" || p.Labels["gpu"] != "mi50" {
		t.Fatalf("got %+v err=%v", p, err)
	}
}

func TestParseLogQL(t *testing.T) {
	p, err := ParseLogQL(`{stream="homelab"} |= "fan"`)
	if err != nil || p.Stream != "homelab" || p.Text != "fan" {
		t.Fatalf("got %+v err=%v", p, err)
	}
}

func TestParseLogQLStreamLabels(t *testing.T) {
	p, err := ParseLogQL(`{stream="homelab", level="error", host="tower"} |= "fan"`)
	if err != nil || p.Stream != "homelab" || p.Text != "fan" {
		t.Fatalf("got %+v err=%v", p, err)
	}
	level, attrs := LogStreamAttrs(p.Labels)
	if level != "error" || attrs["host"] != "tower" {
		t.Fatalf("level=%q attrs=%v", level, attrs)
	}
}

func TestNormalizeMillidegrees(t *testing.T) {
	s := NormalizeSample(contract.Sample{
		Name:  "amd_gpu_junction_temp_millidegree_celsius",
		Value: 78500,
	})
	if s.Name != "junction_c" {
		t.Fatalf("name %q", s.Name)
	}
	if s.Value < 78.4 || s.Value > 78.6 {
		t.Fatalf("value %v want 78.5 °C", s.Value)
	}
}

func TestNormalizeTempOver200(t *testing.T) {
	s := NormalizeSample(contract.Sample{Name: "junction_c", Value: 102000})
	if s.Value < 101.9 || s.Value > 102.1 {
		t.Fatalf("value %v want 102 °C", s.Value)
	}
}

func TestParsePromText(t *testing.T) {
	body := `# TYPE junction_c gauge
junction_c{gpu="mi50"} 78.5
edge_c 72
`
	samples, err := ParsePromText(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 {
		t.Fatalf("len %d", len(samples))
	}
	if samples[0].Name != "junction_c" || samples[0].Value != 78.5 || samples[0].Labels["gpu"] != "mi50" {
		t.Fatalf("first %+v", samples[0])
	}
}

func TestParseMetricsJSONArray(t *testing.T) {
	body := `[{"name":"cpu_c","value":45.2,"labels":{"host":"homelab"}}]`
	samples, err := ParseMetricsJSON([]byte(body))
	if err != nil || len(samples) != 1 || samples[0].Name != "cpu_c" {
		t.Fatalf("got %+v err=%v", samples, err)
	}
}

func TestParseMetricsJSONFlat(t *testing.T) {
	body := `{"junction_c":78.5,"fan_pct":40,"host":"homelab","unit":"C"}`
	samples, err := ParseMetricsJSON([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]contract.Sample{}
	for _, s := range samples {
		by[s.Name] = s
	}
	if by["junction_c"].Value != 78.5 || by["fan_pct"].Value != 40 {
		t.Fatalf("got %+v", samples)
	}
	if by["junction_c"].Labels["host"] != "homelab" {
		t.Fatalf("labels %+v", by["junction_c"].Labels)
	}
}

func TestMemoryAlias(t *testing.T) {
	s := NormalizeSample(contract.Sample{Name: "memory_c", Value: 70})
	if s.Name != "mem_c" {
		t.Fatalf("name %q", s.Name)
	}
}

func TestAlignSeriesStep(t *testing.T) {
	in := []contract.Series{{
		Name: "cpu_c",
		Points: [][2]float64{
			{1.0, 10},
			{2.0, 20},
			{3.0, 30},
			{4.0, 40},
		},
	}}
	got := AlignSeries(in, 1_000_000, 4_000_000, 2_000_000)
	if len(got) != 1 || len(got[0].Points) != 2 {
		t.Fatalf("got %+v", got)
	}
	if got[0].Points[0][0] != 1 || got[0].Points[0][1] != 10 {
		t.Fatalf("first %+v", got[0].Points[0])
	}
	if got[0].Points[1][0] != 3 || got[0].Points[1][1] != 30 {
		t.Fatalf("second %+v", got[0].Points[1])
	}
}

func TestAlignSeriesNoStep(t *testing.T) {
	in := []contract.Series{{Name: "n", Points: [][2]float64{{1, 2}}}}
	got := AlignSeries(in, 0, 0, 0)
	if len(got) != 1 || len(got[0].Points) != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestParseRangePromQLSelector(t *testing.T) {
	rx, err := ParseRangePromQL(`junction_c{gpu="mi50"}`)
	if err != nil || rx.Name != "junction_c" || rx.Labels["gpu"] != "mi50" || rx.RateUS != 0 {
		t.Fatalf("got %+v err=%v", rx, err)
	}
}

func TestParseRangePromQLRate(t *testing.T) {
	rx, err := ParseRangePromQL(`rate(junction_c{gpu="mi50"}[5m])`)
	if err != nil || rx.Name != "junction_c" || rx.Labels["gpu"] != "mi50" || rx.RateUS != 5*60*1_000_000 {
		t.Fatalf("got %+v err=%v", rx, err)
	}
}

func TestParseRangePromQLAvgByRate(t *testing.T) {
	rx, err := ParseRangePromQL(`avg by (gpu) (rate(junction_c[1m]))`)
	if err != nil || rx.Name != "junction_c" || rx.RateUS != 60_000_000 || len(rx.GroupBy) != 1 || rx.GroupBy[0] != "gpu" {
		t.Fatalf("got %+v err=%v", rx, err)
	}
}

func TestParseDurationUS(t *testing.T) {
	us, err := ParseDurationUS("30s")
	if err != nil || us != 30_000_000 {
		t.Fatalf("got %d err=%v", us, err)
	}
}

func TestEvalRangeLookback(t *testing.T) {
	in := []contract.Series{{
		Name:   "junction_c",
		Points: [][2]float64{{0.5, 70}, {2, 80}},
	}}
	got := EvalRange(in, RangeExpr{Name: "junction_c"}, 1_000_000, 3_000_000, 1_000_000)
	if len(got) != 1 || len(got[0].Points) != 3 {
		t.Fatalf("got %+v", got)
	}
	if got[0].Points[0][0] != 1 || got[0].Points[0][1] != 70 {
		t.Fatalf("first %+v", got[0].Points[0])
	}
	if got[0].Points[1][1] != 80 || got[0].Points[2][1] != 80 {
		t.Fatalf("later %+v", got[0].Points)
	}
}

func TestRateSeries(t *testing.T) {
	in := []contract.Series{{
		Name: "junction_c",
		Points: [][2]float64{
			{0, 10},
			{10, 20},
			{20, 30},
		},
	}}
	got := RateSeries(in, 20_000_000, 20_000_000, 20_000_000, 20_000_000)
	if len(got) != 1 || len(got[0].Points) != 1 {
		t.Fatalf("got %+v", got)
	}
	if got[0].Points[0][1] != 1 {
		t.Fatalf("rate %v want 1 /s", got[0].Points[0][1])
	}
}

func TestAvgBySeries(t *testing.T) {
	in := []contract.Series{
		{Name: "junction_c", Labels: map[string]string{"gpu": "mi50", "rack": "a"}, Points: [][2]float64{{1, 80}}},
		{Name: "junction_c", Labels: map[string]string{"gpu": "vii", "rack": "a"}, Points: [][2]float64{{1, 60}}},
	}
	got := AvgBySeries(in, []string{"rack"})
	if len(got) != 1 || got[0].Labels["rack"] != "a" || len(got[0].Labels) != 1 {
		t.Fatalf("got %+v", got)
	}
	if got[0].Points[0][1] != 70 {
		t.Fatalf("avg %v", got[0].Points[0][1])
	}
}
