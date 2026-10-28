package browsergrant

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

func TestSharedCanonicalVectors(t *testing.T) {
	b, e := os.ReadFile("testdata/canonical-vectors.json")
	if e != nil {
		t.Fatal(e)
	}
	var rows []struct {
		Value     any    `json:"value"`
		Canonical string `json:"canonical"`
		Domain    string `json:"domain"`
		Digest    string `json:"digest"`
	}
	if e = json.Unmarshal(b, &rows); e != nil {
		t.Fatal(e)
	}
	for _, r := range rows {
		b, e := Canonical(r.Value)
		if e != nil || string(b) != r.Canonical {
			t.Fatalf("%v => %s %v want%s", r.Value, b, e, r.Canonical)
		}
		d, e := Digest(r.Domain, r.Value)
		if e != nil || d != r.Digest {
			t.Fatalf("digest %s %v", d, e)
		}
	}
}
func TestCanonicalRejectsUnsafeValues(t *testing.T) {
	for _, v := range []any{math.Copysign(0, -1), math.NaN(), math.Inf(1), float64(9007199254740992), "é", map[string]any{"é": 1}} {
		if _, e := Canonical(v); e == nil {
			t.Fatalf("accepted%v", v)
		}
	}
}
