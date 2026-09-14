package agentdesk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApprovalChangedEvidenceAndOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review.txt")
	os.WriteFile(path, []byte("before"), 0600)
	calls := 0
	s, e := New(Config{DataDir: dir, ApprovalExecute: func(ctx context.Context, a Approval) (string, error) { calls++; return "Resumed once", nil }})
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.RequestApproval(context.Background(), ApprovalRequest{Title: "Promote clone", Action: "promote", Evidence: []Evidence{{Path: path}}})
	if e != nil {
		t.Fatal(e)
	}
	os.WriteFile(path, []byte("after"), 0600)
	body := `{"decision":"approve","digest":"` + a.Digest + `"}`
	w := req(s, "POST", "/api/approvals/"+a.ID, body)
	if w.Code != 409 || calls != 0 {
		t.Fatal(w.Code, calls)
	}
	os.WriteFile(path, []byte("before"), 0600)
	w = req(s, "POST", "/api/approvals/"+a.ID, body)
	if w.Code != 200 || calls != 1 {
		t.Fatal(w.Code, calls, w.Body.String())
	}
	w = req(s, "POST", "/api/approvals/"+a.ID, body)
	if w.Code != 409 || calls != 1 {
		t.Fatal(w.Code, calls)
	}
	w = req(s, "GET", "/api/approvals", "")
	if !strings.Contains(w.Body.String(), "Resumed once") {
		t.Fatal(w.Body.String())
	}
}
func TestLegacyMigrationAndExternalDocument(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "workspace.json"), []byte(`{"commitments":[{"id":"c7","title":"Existing work"}],"next_id":7}`), 0600)
	s, e := New(Config{DataDir: dir})
	if e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "workspace.json"))
	if strings.Contains(string(b), "Existing work") {
		t.Fatal("duplicate commitment truth")
	}
	p := filepath.Join(dir, "Commitments.md")
	b, _ = os.ReadFile(p)
	os.WriteFile(p, []byte(strings.Replace(string(b), "Existing work", "Human edited", 1)), 0600)
	if !strings.Contains(s.Context(), "Human edited") {
		t.Fatal(s.Context())
	}
	if _, e = s.UpdateCommitment(context.Background(), "c7", "due", "2026-09-12"); e != nil {
		t.Fatal(e)
	}
	items, e := s.documents.List()
	if e != nil || items[0].Due != "2026-09-12" {
		t.Fatal(items, e)
	}
}
