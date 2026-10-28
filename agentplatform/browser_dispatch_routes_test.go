package agentplatform

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"github.com/srhopkins/iugum/agentplatform/browsergrant"
	"github.com/srhopkins/iugum/policy"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestBrowserDispatchPollExactWireAndArbitraryGrantDenied(t *testing.T) {
	g, e := policy.New("", "")
	if e != nil {
		t.Fatal(e)
	}
	s, e := New(context.Background(), Config{DataDir: t.TempDir(), HostTools: true, Check: func(context.Context, string, string) error { return nil }})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	host, e := s.hostRegister(HostRegistration{Instance: testID, Capabilities: []string{"inspect"}})
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.NewBrowserGrantAdapter(g, "actor", filepath.Join(t.TempDir(), "receipts.json"))
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	if e = s.EnableBrowserGrantRoutes(a); e != nil {
		t.Fatal(e)
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	p := browsergrant.Pair{ID: "pair", Authority: browserAuthorityID(s.controls.incarnation), ServerKeyID: "server", ClientKeyID: "clientkey", Instance: host.Instance, WindowLease: windowGeneration(host), Client: testID, Document: otherID, Origin: "https://iugum-agent-code.local", TabID: 1, Expires: time.Now().Add(30 * time.Second).UnixMilli(), ClientKey: pub}
	if e = a.RegisterTrustedPair(context.Background(), host, p); e != nil {
		t.Fatal(e)
	}
	post := func(path string, q map[string]any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(map[string]any{"instance": host.Instance, "lease": host.Lease, "request": q})
		r := httptest.NewRequest("POST", "http://127.0.0.1/api/host/browser/"+path, bytes.NewReader(b))
		r.Header.Set("Authorization", "Bearer "+s.token)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	command := map[string]any{"pair": "pair", "commandId": testID, "operation": "observe", "args": map[string]any{}, "observation": ""}
	if w := post("grant", command); w.Code != 409 {
		t.Fatal("unqueued arbitrary grant", w.Code)
	}
	if _, e = a.dispatcher.QueueForWindow(context.Background(), host.Instance, windowGeneration(host), browsergrant.Request{Pair: "pair", CommandID: testID, Operation: "observe", Args: map[string]any{}}); e != nil {
		t.Fatal(e)
	}
	w := post("poll", map[string]any{"pair": "pair"})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var v map[string]any
	if e = json.Unmarshal(w.Body.Bytes(), &v); e != nil {
		t.Fatal(e)
	}
	wire, ok := v["command"].(map[string]any)
	if !ok || !fields(wire, "pair", "commandId", "operation", "args", "observation") {
		t.Fatal("poll leaked internal durable fields", v)
	}
	w = post("grant", command)
	if w.Code != 200 {
		t.Fatal("real dispatched grant rejected", w.Code, w.Body.String())
	}
	if w = post("grant", command); w.Code != 409 {
		t.Fatal("repeated grant", w.Code)
	}
}
