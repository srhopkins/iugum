package agentplatform

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/policy"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestBrowserBootstrapHTTPDefaultOffPrivateLeaseAndPolicy(t *testing.T) {
	g, e := policy.New("", "")
	if e != nil {
		t.Fatal(e)
	}
	deny := false
	check := func(ctx context.Context, obj, act string) error {
		if deny && obj == "agent/host/browser_bootstrap" {
			return context.Canceled
		}
		return g.Enforce(ctx, contract.Request{Sub: "actor", Obj: obj, Act: act})
	}
	s, e := New(context.Background(), Config{DataDir: t.TempDir(), HostTools: true, Check: check})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	host, e := s.hostRegister(HostRegistration{Instance: testID, Capabilities: []string{"inspect"}})
	if e != nil {
		t.Fatal(e)
	}
	post := func(body string, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/host/browser/bootstrap/offer", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		return w
	}
	body, _ := json.Marshal(map[string]any{"instance": host.Instance, "lease": host.Lease, "request": map[string]any{}})
	if w := post(string(body), s.token); w.Code != 404 {
		t.Fatalf("default enabled:%d", w.Code)
	}
	a, e := s.NewBrowserGrantAdapter(g, "actor", filepath.Join(t.TempDir(), "receipts.json"))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.EnableBrowserGrantRoutes(a); e != nil {
		t.Fatal(e)
	}
	if w := post(string(body), "wrong"); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := post(`{"instance":"`+testID+`","lease":"wrong","request":{}}`, s.token); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := post(`{"instance":"`+testID+`","instance":"`+testID+`","lease":"wrong","request":{}}`, s.token); w.Code != 400 {
		t.Fatal(w.Code)
	}
	w := post(string(body), s.token)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte(host.Lease)) {
		t.Fatal("raw private editor lease leaked")
	}
	var reply map[string]any
	if e = json.Unmarshal(w.Body.Bytes(), &reply); e != nil {
		t.Fatal(e)
	}
	if reply["serverCommitment"] == nil || reply["offerDigest"] == nil {
		t.Fatal("wire response mismatch")
	}
	deny = true
	if w := post(string(body), s.token); w.Code != 403 {
		t.Fatal("actual outer operation deny", w.Code)
	}
}
