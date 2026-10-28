package agentplatform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/policy"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHostPairingHTTPTransportAndOperationPolicy(t *testing.T) {
	denyRoute, denyOperation := false, false
	s, err := New(context.Background(), Config{DataDir: t.TempDir(), HostTools: true, Check: func(_ context.Context, obj, act string) error {
		if denyRoute && obj == "agent/sessions/api/host/pairing/challenge" || denyOperation && obj == "agent/host/pairing_attest" {
			return errors.New("denied")
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	identity, err := s.hostRegister(HostRegistration{Instance: testID, Label: "HTTP", Capabilities: []string{"inspect"}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	post := func(path string, value any, token string) (int, []byte) {
		t.Helper()
		body, _ := json.Marshal(value)
		r, _ := http.NewRequest(http.MethodPost, server.URL+path, bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		response, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		var result bytes.Buffer
		_, _ = result.ReadFrom(response.Body)
		return response.StatusCode, result.Bytes()
	}
	q := HostPairChallenge{HostIdentity: identity, Client: testID, Document: otherID}
	if status, _ := post("/api/host/pairing/challenge", q, "wrong"); status != 401 {
		t.Fatal(status)
	}
	denyRoute = true
	if status, _ := post("/api/host/pairing/challenge", q, s.token); status != 403 {
		t.Fatal(status)
	}
	denyRoute = false
	status, body := post("/api/host/pairing/challenge", q, s.token)
	if status != 200 {
		t.Fatal(status, string(body))
	}
	var binding HostPairBinding
	if err = json.Unmarshal(body, &binding); err != nil {
		t.Fatal(err)
	}
	attest := HostPairAttest{HostIdentity: identity, Binding: binding}
	denyOperation = true
	if status, _ = post("/api/host/pairing/attest", attest, s.token); status != 409 {
		t.Fatal(status)
	}
	denyOperation = false
	if status, body = post("/api/host/pairing/attest", attest, s.token); status != 200 {
		t.Fatal(status, string(body))
	}
	if status, _ = post("/api/host/pairing/attest", attest, s.token); status != 409 {
		t.Fatal("replayed", status)
	}
}

func TestHostPairingExactLeaseNoncePolicyAndExpiry(t *testing.T) {
	deny := false
	s, err := New(context.Background(), Config{DataDir: t.TempDir(), HostTools: true, Check: func(_ context.Context, obj, act string) error {
		if deny && obj == "agent/host/pairing_attest" {
			return errors.New("revoked")
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, err := s.hostRegister(HostRegistration{Instance: testID, Label: "A", Capabilities: []string{"inspect"}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.hostRegister(HostRegistration{Instance: otherID, Label: "B", Capabilities: []string{"inspect"}})
	if err != nil {
		t.Fatal(err)
	}
	q := HostPairChallenge{HostIdentity: a, Client: otherID, Document: testID}
	first, err := s.hostPairChallenge(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	attest := HostPairAttest{HostIdentity: a, Binding: first}
	wrong := attest
	wrong.HostIdentity = b
	if _, err = s.hostPairAttest(context.Background(), wrong); err == nil {
		t.Fatal("other host accepted")
	}
	wrong = attest
	wrong.Binding.Document = otherID
	if _, err = s.hostPairAttest(context.Background(), wrong); err == nil {
		t.Fatal("wrong document accepted")
	}
	deny = true
	if _, err = s.hostPairAttest(context.Background(), attest); err == nil {
		t.Fatal("revoked operation accepted")
	}
	deny = false
	receipt, err := s.hostPairAttest(context.Background(), attest)
	if err != nil || receipt.Mouse || receipt.Scope != "own-webview-private-channel" {
		t.Fatal(receipt, err)
	}
	if _, err = s.hostPairAttest(context.Background(), attest); err == nil {
		t.Fatal("nonce replay accepted")
	}
	second, err := s.hostPairChallenge(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	s.host.mu.Lock()
	p := s.host.pairings[second.Challenge]
	p.Expires = time.Now().Add(-time.Second).UnixMilli()
	s.host.pairings[second.Challenge] = p
	s.host.mu.Unlock()
	if _, err = s.hostPairAttest(context.Background(), HostPairAttest{HostIdentity: a, Binding: second}); err == nil {
		t.Fatal("expired challenge accepted")
	}
	q.Client = "invalid"
	if _, err = s.hostPairChallenge(context.Background(), q); err == nil {
		t.Fatal("invalid client accepted")
	}
}

func TestHostPairingRealCasbinAndReloadedLease(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "policy.csv")
	rules := "p, probe, agent/host/register, write, allow\np, probe, agent/host/pairing_challenge, create, allow\n"
	if err := os.WriteFile(p, []byte(rules), 0600); err != nil {
		t.Fatal(err)
	}
	gate, err := policy.New("", p)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(ctx, Config{DataDir: t.TempDir(), HostTools: true, Check: func(ctx context.Context, obj, act string) error {
		return gate.Enforce(ctx, contract.Request{Sub: "probe", Obj: obj, Act: act})
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, err := s.hostRegister(HostRegistration{Instance: testID, Label: "first", Capabilities: []string{"inspect"}})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := s.hostPairChallenge(ctx, HostPairChallenge{HostIdentity: a, Client: testID, Document: otherID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.hostPairAttest(ctx, HostPairAttest{HostIdentity: a, Binding: binding}); err == nil {
		t.Fatal("real policy denied operation was accepted")
	}
	if err = os.WriteFile(p, []byte(rules+"p, probe, agent/host/pairing_attest, write, allow\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gate, err = policy.New("", p)
	if err != nil {
		t.Fatal(err)
	}
	s.host.mu.Lock()
	w := s.host.windows[testID]
	w.Expires = time.Now().Add(-time.Second)
	s.host.windows[testID] = w
	s.host.mu.Unlock()
	next, err := s.hostRegister(HostRegistration{Instance: testID, Label: "reloaded", Capabilities: []string{"inspect"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.hostPairAttest(ctx, HostPairAttest{HostIdentity: next, Binding: binding}); err == nil {
		t.Fatal("old nonce accepted after host lease changed")
	}
	fresh, err := s.hostPairChallenge(ctx, HostPairChallenge{HostIdentity: next, Client: testID, Document: otherID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.hostPairAttest(ctx, HostPairAttest{HostIdentity: next, Binding: fresh}); err != nil {
		t.Fatal(err)
	}
}
