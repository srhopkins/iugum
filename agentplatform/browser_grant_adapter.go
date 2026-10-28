package agentplatform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"

	"github.com/srhopkins/iugum/agentplatform/browsergrant"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/policy"
)

// BrowserGrantAdapter is default-off source integration for the explicit
// --host-tools --browser-grants opt-in. It uses the real existing registry and
// outer application gate, not the unrelated Claude tool policy.Live.
type BrowserGrantAdapter struct {
	server     *Server
	authority  *browsergrant.Authority
	dispatcher *browsergrant.Dispatcher
	handler    http.Handler
}

func (s *Server) NewBrowserGrantAdapter(g contract.Policy, actor, store string) (*BrowserGrantAdapter, error) {
	static, ok := g.(*policy.Gate)
	if !ok || static == nil || s.host == nil || s.cfg.Check == nil {
		return nil, errors.New("browser grants require explicit static application Casbin and live host registry")
	}
	a, e := browsergrant.New(store, actor, browserAuthorityID(s.controls.incarnation), static)
	if e != nil {
		return nil, e
	}
	d, e := browsergrant.NewDispatcher(a)
	if e != nil {
		_ = a.Close()
		return nil, e
	}
	adapter := &BrowserGrantAdapter{server: s, authority: a, dispatcher: d}
	adapter.initMCP()
	return adapter, nil
}
func windowGeneration(q HostIdentity) string {
	b := sha256.Sum256([]byte("iugum/browser/window-generation/2\n" + q.Instance + "\n" + q.Lease))
	return hex.EncodeToString(b[:])
}

// RegisterTrustedPair is available to the future trusted ceremony adapter only;
// never bind this directly to untrusted JSON or expose the host lease to a page.
func (a *BrowserGrantAdapter) RegisterTrustedPair(ctx context.Context, host HostIdentity, p browsergrant.Pair) error {
	s := a.server
	s.host.mu.Lock()
	defer s.host.mu.Unlock()
	if e := s.hostCheck(ctx, "browser_pair", "create"); e != nil {
		return e
	}
	if !s.pairingWindow(host) || p.Instance != host.Instance || p.WindowLease != windowGeneration(host) {
		return errors.New("browser pair does not match exact current private editor lease")
	}
	return a.authority.RegisterTrustedPair(p)
}
func (a *BrowserGrantAdapter) Grant(ctx context.Context, host HostIdentity, q browsergrant.Request) (browsergrant.Envelope, error) {
	s := a.server
	s.host.mu.Lock()
	defer s.host.mu.Unlock()
	if e := s.hostCheck(ctx, "browser_transport", "write"); e != nil {
		return browsergrant.Envelope{}, e
	}
	if !s.pairingWindow(host) {
		return browsergrant.Envelope{}, errors.New("editor window lease expired")
	}
	return a.dispatcher.GrantForWindow(ctx, host.Instance, windowGeneration(host), q)
}

func browserAuthorityID(inc string) string {
	b := sha256.Sum256([]byte("iugum/browser/authority/2\n" + inc))
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func validateBrowserConfig(ctx context.Context, c Config) error {
	if !c.BrowserGrants {
		return nil
	}
	gate, ok := c.BrowserPolicy.(*policy.Gate)
	if !c.HostTools || c.Check == nil || c.BrowserActor == "" || !ok || gate == nil {
		return errors.New("browser grants require host tools, explicit actor and immutable actual application Casbin")
	}
	if e := c.Check(ctx, "agent/browser", "serve"); e != nil {
		return e
	}
	return gate.Enforce(ctx, contract.Request{Sub: c.BrowserActor, Obj: "agent/browser", Act: "serve"})
}

// ReplacePolicy serializes actual browser operation revocation against durable
// grant acceptance. Outer application transport policy stays independently
// checked; this cannot relax an outer denial or mutate the application gate.
func (a *BrowserGrantAdapter) ReplacePolicy(g contract.Policy) error {
	static, ok := g.(*policy.Gate)
	if !ok || static == nil {
		return errors.New("immutable actual Casbin required")
	}
	return a.authority.ReplacePolicy(static)
}

func (a *BrowserGrantAdapter) Close() error {
	return errors.Join(a.dispatcher.Close(), a.authority.Close())
}
