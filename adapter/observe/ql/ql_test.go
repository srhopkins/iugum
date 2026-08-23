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
