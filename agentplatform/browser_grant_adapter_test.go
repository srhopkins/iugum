package agentplatform

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"github.com/srhopkins/iugum/agentplatform/browsergrant"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/policy"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBrowserGrantAdapterActualRegistryAndCasbin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app-policy.csv")
	if e := os.WriteFile(path, []byte("p, actor, agent/host/*, *, allow\np, actor, agent/browser/pair/observe, write, allow\n"), 0600); e != nil {
		t.Fatal(e)
	}
	gate, e := policy.New("", path)
	if e != nil {
		t.Fatal(e)
	}
	check := func(ctx context.Context, obj, act string) error {
		return gate.Enforce(ctx, contract.Request{Sub: "actor", Obj: obj, Act: act})
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
	a, e := s.NewBrowserGrantAdapter(gate, "actor", filepath.Join(t.TempDir(), "receipts.json"))
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	p := browsergrant.Pair{ID: "pair", Authority: browserAuthorityID(s.controls.incarnation), ServerKeyID: "server", ClientKeyID: "clientkey", Instance: host.Instance, WindowLease: windowGeneration(host), Client: testID, Document: otherID, Origin: "https://iugum-agent-code.local", TabID: 1, Expires: time.Now().Add(30 * time.Second).UnixMilli(), ClientKey: pub}
	if p.WindowLease == host.Lease {
		t.Fatal("raw private credential leaked")
	}
	if e = a.RegisterTrustedPair(context.Background(), host, p); e != nil {
		t.Fatal(e)
	}
	q := browsergrant.Request{Pair: p.ID, CommandID: testID, Operation: "observe", Args: map[string]any{}}
	wrong := host
	wrong.Lease = "wrong"
	if _, e = a.Grant(context.Background(), wrong, q); e == nil {
		t.Fatal("wrong private lease granted")
	}
	if _, e = a.dispatcher.QueueForWindow(context.Background(), host.Instance, windowGeneration(host), q); e != nil {
		t.Fatal(e)
	}
	if _, e = a.dispatcher.PollForWindow(context.Background(), p.ID, host.Instance, windowGeneration(host)); e != nil {
		t.Fatal(e)
	}
	if _, e = a.Grant(context.Background(), host, q); e != nil {
		t.Fatal(e)
	}
	// Existing host registry expiration must fence even previously paired authority.
	w := s.host.windows[host.Instance]
	w.Expires = time.Now().Add(-time.Second)
	s.host.windows[host.Instance] = w
	q.CommandID = otherID
	if _, e = a.Grant(context.Background(), host, q); e == nil {
		t.Fatal("expired host grant")
	}
	if _, e = s.NewBrowserGrantAdapter(policy.Live{Path: path}, "actor", filepath.Join(t.TempDir(), "live.json")); e == nil {
		t.Fatal("unsupported Live policy wiring accepted")
	}
}

func TestBrowserGrantConfigFailClosedBeforeStateWrite(t *testing.T) {
	for _, kind := range []string{"host", "check", "actor", "gate", "live", "deny"} {
		t.Run(kind, func(t *testing.T) {
			g, e := policy.New("", "")
			if e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(t.TempDir(), "must-not-exist")
			c := Config{DataDir: path, HostTools: true, BrowserGrants: true, BrowserActor: "actor", BrowserPolicy: g, Check: func(context.Context, string, string) error { return nil }}
			switch kind {
			case "host":
				c.HostTools = false
			case "check":
				c.Check = nil
			case "actor":
				c.BrowserActor = ""
			case "gate":
				c.BrowserPolicy = nil
			case "live":
				c.BrowserPolicy = policy.Live{}
			case "deny":
				c.Check = func(context.Context, string, string) error { return context.Canceled }
			}
			if s, e := New(context.Background(), c); e == nil {
				s.Close()
				t.Fatal("unsafe prerequisites accepted")
			}
			if _, e = os.Stat(path); !os.IsNotExist(e) {
				t.Fatal("rejected startup wrote private state")
			}
		})
	}
}
func TestBrowserGrantConfigExplicitStartup(t *testing.T) {
	g, e := policy.New("", "")
	if e != nil {
		t.Fatal(e)
	}
	s, e := New(context.Background(), Config{DataDir: t.TempDir(), HostTools: true, BrowserGrants: true, BrowserActor: "actor", BrowserPolicy: g, Check: func(ctx context.Context, o, a string) error {
		return g.Enforce(ctx, contract.Request{Sub: "actor", Obj: o, Act: a})
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if s.host.browser == nil {
		t.Fatal("explicit flag did not source-wire optional adapter")
	}
}
func TestBrowserServerDataDirOwnerBeforeExistingStateMutation(t *testing.T) {
	g, e := policy.New("", "")
	if e != nil {
		t.Fatal(e)
	}
	c := Config{DataDir: t.TempDir(), HostTools: true, BrowserGrants: true, BrowserActor: "actor", BrowserPolicy: g, Check: func(context.Context, string, string) error { return nil }}
	a, e := New(context.Background(), c)
	if e != nil {
		t.Fatal(e)
	}
	token, e := os.ReadFile(filepath.Join(c.DataDir, "control-token"))
	if e != nil {
		t.Fatal(e)
	}
	commands := filepath.Join(c.DataDir, "host-commands.json")
	before, e := os.ReadFile(commands)
	if e != nil {
		t.Fatal(e)
	}
	if b, e := New(context.Background(), c); e == nil {
		b.Close()
		t.Fatal("second server owned live browser DataDir")
	}
	after, _ := os.ReadFile(commands)
	if string(before) != string(after) {
		t.Fatal("rejected server rewrote host receipts")
	}
	after, _ = os.ReadFile(filepath.Join(c.DataDir, "control-token"))
	if string(token) != string(after) {
		t.Fatal("rejected server changed live auth")
	}
	a.Close()
	b, e := New(context.Background(), c)
	if e != nil {
		t.Fatal(e)
	}
	b.Close()
}
