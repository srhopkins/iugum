package agentdocuments

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExternalEditAndStableIdentity(t *testing.T) {
	p := filepath.Join(t.TempDir(), "Work.md")
	s, e := Open(p)
	if e != nil {
		t.Fatal(e)
	}
	c, e := s.Put(Commitment{ID: "c1", Title: "Ship clear tickets", Due: "2026-09-11"})
	if e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(p)
	b = []byte(strings.Replace(string(b), "Ship clear tickets", "Review contract scope", 1))
	b = append(b, []byte("\n\n## Notes\n\nPreserve this paragraph.\n")...)
	if e = os.WriteFile(p, b, 0600); e != nil {
		t.Fatal(e)
	}
	items, e := s.List()
	if e != nil {
		t.Fatal(e)
	}
	if items[0].Title != "Review contract scope" || items[0].AtomID != c.AtomID {
		t.Fatal(items)
	}
	updated := items[0]
	updated.Done = true
	if _, e = s.Put(updated); e != nil {
		t.Fatal(e)
	}
	items, e = s.List()
	if e != nil || !items[0].Done || items[0].Due != "2026-09-11" {
		t.Fatal(items, e)
	}
	after, _ := os.ReadFile(p)
	if !strings.Contains(string(after), "Preserve this paragraph.") {
		t.Fatal(string(after))
	}
	if strings.Count(string(after), c.AtomID) != 1 {
		t.Fatal("identity changed")
	}
}
func TestRejectInvalidDate(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "Work.md"))
	if _, e := s.Put(Commitment{Title: "Work", Due: "tomorrow"}); e == nil {
		t.Fatal("relative date stored")
	}
}
