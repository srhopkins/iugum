package agentplatform

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/srhopkins/iugum/agentmcp"
	"github.com/srhopkins/iugum/contract"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type hostFixtureGate struct{}

func (hostFixtureGate) Enforce(context.Context, contract.Request) error { return nil }

func TestHostToolsNativeMCPClient(t *testing.T) {
	s, e := New(context.Background(), Config{DataDir: t.TempDir(), HostTools: true, Check: func(context.Context, string, string) error { return nil }})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	t.Setenv("IUGUM_HOST_FIXTURE_TOKEN", s.token)
	tools, closeClient, e := agentmcp.Connect(context.Background(), []agentmcp.Config{{Name: "editor", URL: server.URL + "/api/host/mcp", AuthEnv: "IUGUM_HOST_FIXTURE_TOKEN"}}, "fixture-native", hostFixtureGate{})
	if e != nil {
		t.Fatal(e)
	}
	defer closeClient()
	found := false
	for _, tool := range tools {
		if tool.Name == "mcp_editor__editor_windows" {
			found = true
			value, e := tool.Execute(context.Background(), json.RawMessage(`{}`))
			if e != nil || !strings.Contains(value, "extension-host-window") || !strings.Contains(value, "mouse") {
				t.Fatal("native MCP capability missing", value, e)
			}
		}
	}
	if !found {
		t.Fatal("native agent did not discover editor tools")
	}
}

func TestHostToolsExactWindowDeliveryAndNoReplay(t *testing.T) {
	s, e := New(context.Background(), Config{DataDir: t.TempDir(), HostTools: true, Check: func(context.Context, string, string) error { return nil }})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	a, e := s.hostRegister(HostRegistration{Instance: testID, Label: "Mac editor", Capabilities: []string{"inspect", "open_file"}})
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.hostRegister(HostRegistration{Instance: otherID, Label: "Phone editor", Capabilities: []string{"inspect"}})
	if e != nil {
		t.Fatal(e)
	}
	q := HostCall{Instance: testID, CommandID: "33333333-3333-4333-8333-333333333333", Operation: "open_file", Args: json.RawMessage(`{"path":"README.md","line":2,"group":1}`)}
	c, e := s.hostCall(context.Background(), q)
	if e != nil || c.State != "queued" {
		t.Fatalf("%+v %v", c, e)
	}
	if _, e = s.hostCall(context.Background(), q); e == nil {
		t.Fatal("duplicate accepted")
	}
	if got, e := s.hostPoll(context.Background(), HostIdentity{Instance: otherID, Lease: b.Lease}); e != nil || got != nil {
		t.Fatal("wrong editor received command", e)
	}
	got, e := s.hostPoll(context.Background(), HostIdentity{Instance: testID, Lease: a.Lease})
	if e != nil || got == nil || got.CommandID != q.CommandID {
		t.Fatal("target missing", e)
	}
	if got, e = s.hostPoll(context.Background(), HostIdentity{Instance: testID, Lease: a.Lease}); e != nil || got != nil {
		t.Fatal("dispatched command replayed")
	}
	if e = s.hostResult(HostResult{HostIdentity: HostIdentity{Instance: otherID, Lease: b.Lease}, CommandID: q.CommandID, Result: json.RawMessage(`{}`)}); e == nil {
		t.Fatal("wrong editor result accepted")
	}
	if e = s.hostResult(HostResult{HostIdentity: HostIdentity{Instance: testID, Lease: a.Lease}, CommandID: q.CommandID, Result: json.RawMessage(`{"opened":true}`)}); e != nil {
		t.Fatal(e)
	}
	if e = s.hostResult(HostResult{HostIdentity: HostIdentity{Instance: testID, Lease: a.Lease}, CommandID: q.CommandID}); e == nil {
		t.Fatal("duplicate result accepted")
	}
}

// Opt-in cross-repository contract fixture. The extension is a separate package;
// callers supply its current trusted module rather than installing it globally.
func TestHostToolsAdapterIntegration(t *testing.T) {
	module := os.Getenv("IUGUM_EDITOR_HOST_MODULE")
	if module == "" {
		t.Skip("set IUGUM_EDITOR_HOST_MODULE to current extension src/editor-host.js")
	}
	if !filepath.IsAbs(module) {
		t.Fatal("adapter module must be an absolute path")
	}
	s, e := New(context.Background(), Config{DataDir: t.TempDir(), HostTools: true, Check: func(context.Context, string, string) error { return nil }})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	script := `const {EditorHost}=require(process.env.IUGUM_EDITOR_HOST_MODULE); const readline=require('node:readline');
 const logs=[[],[]];const service={post:async(route,body,signal)=>{const r=await fetch(process.env.FIXTURE_URL+'/api'+route,{method:'POST',headers:{Authorization:'Bearer '+process.env.FIXTURE_TOKEN,'Content-Type':'application/json'},body:JSON.stringify(body),signal});if(!r.ok)throw Error(await r.text());return r.json()}};
 const hosts=logs.map((log,n)=>new EditorHost({vscode:{workspace:{isTrusted:true},window:{},commands:{executeCommand:async(...args)=>{log.push(args)}}},service,label:'Fixture window '+n}));
 (async()=>{for(const h of hosts){await h.start();clearInterval(h.timer);if(!h.identity)throw Error('registration missing')};console.log(JSON.stringify(hosts.map(h=>h.instance)));for await(const line of readline.createInterface({input:process.stdin})){for(const h of hosts)await h.tick();console.log(JSON.stringify(logs));}for(const h of hosts)h.dispose()})().catch(e=>{console.error(e.message);process.exitCode=1});`
	command := exec.CommandContext(ctx, "node", "-e", script)
	command.Env = append(os.Environ(), "FIXTURE_URL="+server.URL, "FIXTURE_TOKEN="+s.token)
	stdin, e := command.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	stdout, e := command.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	var stderr strings.Builder
	command.Stderr = &stderr
	if e = command.Start(); e != nil {
		t.Fatal(e)
	}
	defer command.Process.Kill()
	scan := bufio.NewScanner(stdout)
	if !scan.Scan() {
		t.Fatal("adapter registration missing", stderr.String())
	}
	var ids []string
	if e = json.Unmarshal(scan.Bytes(), &ids); e != nil || len(ids) != 2 || ids[0] == ids[1] {
		t.Fatal("window identities not distinct", e)
	}
	for n, id := range ids {
		commandID := testID
		if n == 1 {
			commandID = otherID
		}
		pane := "chat"
		if n == 1 {
			pane = "terminal"
		}
		if _, e = s.hostCall(ctx, HostCall{Instance: id, CommandID: commandID, Operation: "focus", Args: json.RawMessage(`{"pane":"` + pane + `"}`)}); e != nil {
			t.Fatal(e)
		}
		if _, e = stdin.Write([]byte("tick\n")); e != nil {
			t.Fatal(e)
		}
		if !scan.Scan() {
			t.Fatal("adapter result missing", stderr.String())
		}
		var logs [][][]string
		if e = json.Unmarshal(scan.Bytes(), &logs); e != nil || len(logs) != 2 || len(logs[0]) != 1 || len(logs[1]) != n {
			t.Fatal("wrong window focus delivery", e, string(scan.Bytes()))
		}
		receipt, e := s.hostStatus(ctx, id, commandID)
		if e != nil || receipt.State != "complete" {
			t.Fatal("adapter result wire mismatch", receipt, e)
		}
	}
	stdin.Close()
	if e = command.Wait(); e != nil {
		t.Fatal(e, stderr.String())
	}
}

func TestHostToolsReviewFailClosedAndReceiptFailure(t *testing.T) {
	if s, e := New(context.Background(), Config{DataDir: t.TempDir(), HostTools: true}); e == nil {
		s.Close()
		t.Error("host tools enabled without actual policy")
	}
	q := HostCall{Instance: testID, CommandID: otherID, Operation: "inspect", Args: json.RawMessage("{" + strings.Repeat(" ", 20000) + "}")}
	if e := validateHostCall(q); e == nil {
		t.Error("oversized tool args accepted")
	}
	s, e := New(context.Background(), Config{DataDir: t.TempDir(), HostTools: true, Check: func(context.Context, string, string) error { return nil }})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	reg, e := s.hostRegister(HostRegistration{Instance: testID, Capabilities: []string{"inspect"}})
	if e != nil {
		t.Fatal(e)
	}
	q.Args = json.RawMessage(`{}`)
	if _, e = s.hostCall(context.Background(), q); e != nil {
		t.Fatal(e)
	}
	identity := HostIdentity{Instance: testID, Lease: reg.Lease}
	if _, e = s.hostPoll(context.Background(), identity); e != nil {
		t.Fatal(e)
	}
	receipt := filepath.Join(s.cfg.DataDir, "host-commands.json")
	if e = os.Remove(receipt); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(receipt, 0700); e != nil {
		t.Fatal(e)
	}
	if e = s.hostResult(HostResult{HostIdentity: identity, CommandID: otherID, Result: json.RawMessage(`{}`)}); e == nil {
		t.Fatal("receipt save failure hidden")
	}
	c, _ := s.hostStatus(context.Background(), testID, otherID)
	if c.State != "uncertain" {
		t.Fatalf("false completion after save failure: %+v", c)
	}
}

func TestHostToolsPolicyValidationAndUncertainRestart(t *testing.T) {
	deny := false
	cfg := Config{DataDir: t.TempDir(), HostTools: true, Check: func(_ context.Context, obj, act string) error {
		if deny {
			return errors.New("denied")
		}
		return nil
	}}
	s, e := New(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	reg, e := s.hostRegister(HostRegistration{Instance: testID, Capabilities: []string{"inspect"}})
	if e != nil {
		t.Fatal(e)
	}
	q := HostCall{Instance: testID, CommandID: otherID, Operation: "inspect", Args: json.RawMessage(`{}`)}
	deny = true
	if _, e = s.hostCall(context.Background(), q); e == nil {
		t.Fatal("operation bypassed policy")
	}
	deny = false
	for _, op := range []string{"executeCommand", "mouse"} {
		bad := q
		bad.Operation = op
		if _, e = s.hostCall(context.Background(), bad); e == nil {
			t.Fatal("unsupported action accepted", op)
		}
	}
	if _, e = s.hostCall(context.Background(), q); e != nil {
		t.Fatal(e)
	}
	deny = true
	if _, e = s.hostPoll(context.Background(), HostIdentity{Instance: testID, Lease: reg.Lease}); e == nil {
		t.Fatal("poll bypassed revocation")
	}
	deny = false
	s.Close()
	s, e = New(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	c, e := s.hostStatus(context.Background(), testID, otherID)
	if e != nil || c.State != "uncertain" {
		t.Fatalf("restart replay risk %+v %v", c, e)
	}
	if _, e = s.hostPoll(context.Background(), HostIdentity{Instance: testID, Lease: reg.Lease}); e == nil {
		t.Fatal("old editor lease survived restart")
	}
	if _, e = s.hostCall(context.Background(), q); e == nil {
		t.Fatal("old command ID reused")
	}
	reg, e = s.hostRegister(HostRegistration{Instance: testID, Capabilities: []string{"inspect"}})
	if e != nil {
		t.Fatal(e)
	}
	q.CommandID = "44444444-4444-4444-8444-444444444444"
	if _, e = s.hostCall(context.Background(), q); e != nil {
		t.Fatal(e)
	}
	s.host.mu.Lock()
	s.host.commands[q.CommandID].Expires = time.Now().Add(-time.Second)
	s.host.mu.Unlock()
	c, e = s.hostStatus(context.Background(), testID, q.CommandID)
	if e != nil || c.State != "uncertain" {
		t.Fatal("timeout not explicit", c, e)
	}
}

func TestHostToolsOptionalAndRoutePolicy(t *testing.T) {
	s, e := New(context.Background(), Config{DataDir: t.TempDir()})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if w := callService(s, "POST", "/api/host/register", HostRegistration{Instance: testID}); w.Code != 404 {
		t.Fatal("optional host route enabled", w.Code)
	}
}

func TestHostToolsMCPAndValidation(t *testing.T) {
	denied := false
	s, e := New(context.Background(), Config{DataDir: t.TempDir(), HostTools: true, Check: func(_ context.Context, obj, act string) error {
		if denied && obj == "agent/host/transport" {
			return errors.New("deny")
		}
		return nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	httpServer := httptest.NewServer(s.Handler())
	defer httpServer.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "host-fixture", Version: "1"}, nil)
	session, e := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL + "/api/host/mcp", HTTPClient: &http.Client{Transport: coordinationAuth{s.token}}}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	tools, e := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	if e != nil {
		t.Fatal("tools missing", e)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	for _, name := range []string{"ffai_workers", "editor_windows", "editor_call", "editor_open_beads", "editor_status"} {
		if !names[name] {
			t.Fatalf("tool %s missing", name)
		}
	}
	value, e := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "editor_windows", Arguments: map[string]any{}})
	if e != nil || value.IsError {
		t.Fatal("window capability tool failed", e, value)
	}
	registered := callService(s, "POST", "/api/host/register", HostRegistration{Instance: testID, Capabilities: []string{"open_file"}})
	if registered.Code != 200 {
		t.Fatal(registered.Body.String())
	}
	var identity HostIdentity
	if e = json.Unmarshal(registered.Body.Bytes(), &identity); e != nil {
		t.Fatal(e)
	}
	value, e = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "editor_call", Arguments: map[string]any{"instance": testID, "commandId": otherID, "operation": "open_file", "args": map[string]any{"path": "README.md", "line": 3, "group": 2}}})
	if e != nil || value.IsError {
		b, _ := json.Marshal(value)
		t.Fatal("MCP call contract failed", e, string(b))
	}
	poll := callService(s, "POST", "/api/host/poll", identity)
	var delivered HostCommand
	if e = json.Unmarshal(poll.Body.Bytes(), &delivered); e != nil || delivered.CommandID != otherID || !strings.Contains(string(delivered.Args), "README.md") {
		t.Fatal("MCP args/delivery lost", e, poll.Body.String())
	}
	completed := callService(s, "POST", "/api/host/result", HostResult{HostIdentity: identity, CommandID: otherID, Result: json.RawMessage(`{"opened":true}`)})
	if completed.Code != 200 {
		t.Fatal(completed.Body.String())
	}
	value, e = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "editor_status", Arguments: map[string]any{"instance": testID, "commandId": otherID}})
	if e != nil || value.IsError {
		t.Fatal("MCP status failed", e, value)
	}
	encoded, _ := json.Marshal(value)
	if !strings.Contains(string(encoded), "complete") || !strings.Contains(string(encoded), "opened") {
		t.Fatal("MCP result missing", string(encoded))
	}
	for _, args := range []string{`{"path":"../secret","line":1,"group":1}`, `{"path":"/secret","line":1,"group":1}`, `{"path":"ok","line":0,"group":1}`, `{"path":"ok","line":1,"group":1,"command":"reload"}`} {
		if e = validateHostCall(HostCall{Instance: testID, CommandID: otherID, Operation: "open_file", Args: json.RawMessage(args)}); e == nil {
			t.Fatal("invalid file action accepted", args)
		}
	}
	denied = true
	if _, e = session.ListTools(context.Background(), &mcp.ListToolsParams{}); e == nil {
		t.Fatal("MCP route bypassed host policy")
	}
}
