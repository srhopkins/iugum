package graphgloss

import "testing"

func TestExtractOwns(t *testing.T) {
	f := Default()
	got := f.Extract("Steve owns the MI50. The MI50 is located-in the tower.")
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	if got[0].From != "steve" || got[0].Rel != "owns" || got[0].To != "mi50" {
		t.Fatalf("first %+v", got[0])
	}
	if got[1].From != "mi50" || got[1].Rel != "located-in" || got[1].To != "tower" {
		t.Fatalf("second %+v", got[1])
	}
}

func TestExtractInflectionIn(t *testing.T) {
	f := Default()
	got := f.Extract("The MI50 is in the tower")
	if len(got) != 1 || got[0].Rel != "located-in" || got[0].To != "tower" {
		t.Fatalf("%+v", got)
	}
}

func TestUnknownRelDropped(t *testing.T) {
	f := Default()
	got := f.Extract("Steve invented the MI50")
	if len(got) != 0 {
		t.Fatalf("want none, got %+v", got)
	}
}

func TestAddRelInYAML(t *testing.T) {
	f, err := Parse([]byte(`
rels:
  - word: invented
`))
	if err != nil {
		t.Fatal(err)
	}
	got := f.Extract("Steve invented the MI50")
	if len(got) != 1 || got[0].Rel != "invented" || got[0].To != "mi50" {
		t.Fatalf("%+v", got)
	}
}

func TestAllowedRel(t *testing.T) {
	f := Default()
	if !f.AllowedRel("owns") || !f.AllowedRel("in") || f.AllowedRel("invented") {
		t.Fatal("rel list")
	}
}
