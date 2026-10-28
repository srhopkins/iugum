package agentplatform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/srhopkins/iugum/agentcore"
	"github.com/srhopkins/iugum/agentmcp"
)

func TestFlatEditorOpenBeadsUsesOriginalGateQueueAndReceipt(t *testing.T) {
	denied := false
	checked := []string{}
	s, e := New(context.Background(), Config{DataDir: t.TempDir(), HostTools: true, Check: func(_ context.Context, obj, act string) error {
		if obj == "agent/host/open_beads" {
			checked = append(checked, obj+":"+act)
			if denied {
				return errors.New("denied")
			}
		}
		return nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	identity, e := s.hostRegister(HostRegistration{Instance: testID, Capabilities: []string{"open_beads"}})
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "flat-fixture", Version: "1"}, nil)
	session, e := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: server.URL + "/api/host/mcp", HTTPClient: &http.Client{Transport: coordinationAuth{s.token}}}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	listed, e := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	if e != nil {
		t.Fatal(e)
	}
	foundGeneric, foundFlat := false, false
	for _, tool := range listed.Tools {
		foundGeneric = foundGeneric || tool.Name == "editor_call"
		if tool.Name == "editor_open_beads" {
			foundFlat = true
			b, _ := json.Marshal(tool.InputSchema)
			var schema map[string]any
			json.Unmarshal(b, &schema)
			props := schema["properties"].(map[string]any)
			if props["args"] != nil || props["operation"] != nil || props["instance"] == nil || props["commandId"] == nil {
				t.Fatal("dedicated schema retained wrapper")
			}
		}
	}
	if !foundGeneric || !foundFlat {
		t.Fatal("backward compatibility or dedicated discovery lost")
	}
	for _, input := range []map[string]any{{"instance": testID, "commandId": "word-label"}, {"instance": testID, "commandId": otherID, "args": map[string]any{}}, {"instance": testID, "commandId": otherID, "operation": "open_beads"}, {"instance": testID, "commandId": otherID, "lanes": "true"}, {"instance": testID, "commandId": otherID, "ticketProject": "iugum"}} {
		result, e := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "editor_open_beads", Arguments: input})
		if e != nil || !result.IsError || len(s.host.commands) != 0 {
			t.Fatal("invalid flat input dispatched", e)
		}
	}
	input := map[string]any{"instance": testID, "commandId": otherID, "view": "tree", "projects": []string{"iugum"}, "search": "schema", "sort": "priority", "ticket": "iugum-9din", "ticketProject": "iugum", "lanes": true}
	result, e := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "editor_open_beads", Arguments: input})
	if e != nil || result.IsError || len(s.host.commands) != 1 {
		t.Fatal("valid flat call rejected", e)
	}
	if len(checked) != 1 || checked[0] != "agent/host/open_beads:write" {
		t.Fatalf("unexpected authority check: %+v", checked)
	}
	command, e := s.hostPoll(context.Background(), identity)
	if e != nil || command == nil || command.Operation != "open_beads" || command.CommandID != otherID || !strings.Contains(string(command.Args), `"view":"tree"`) {
		t.Fatal("flat call did not delegate exact operation/navigation", e)
	}
	if e = s.hostResult(HostResult{HostIdentity: identity, CommandID: otherID, Result: json.RawMessage(`{"opened":true}`)}); e != nil {
		t.Fatal(e)
	}
	receipt, e := s.hostStatus(context.Background(), testID, otherID)
	if e != nil || receipt.State != "complete" {
		t.Fatal("receipt lifecycle lost", e)
	}
	result, e = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "editor_open_beads", Arguments: input})
	if e != nil || !result.IsError || len(s.host.commands) != 1 {
		t.Fatal("duplicate replayed", e)
	}
	denied = true
	input["commandId"] = testID
	result, e = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "editor_open_beads", Arguments: input})
	if e != nil || !result.IsError || len(s.host.commands) != 1 {
		t.Fatal("denial bypassed", e)
	}
}
func TestFlatEditorOpenBeadsNativeValidationAndHardDenial(t *testing.T) {
	denied := false
	s, e := New(context.Background(), Config{DataDir: t.TempDir(), HostTools: true, Check: func(_ context.Context, obj, act string) error {
		if denied && obj == "agent/host/open_beads" {
			return errors.New("denied")
		}
		return nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, e = s.hostRegister(HostRegistration{Instance: testID, Capabilities: []string{"open_beads"}}); e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	t.Setenv("EDITOR_FLAT_FIXTURE_TOKEN", s.token)
	tools, closeTools, e := agentmcp.Connect(context.Background(), []agentmcp.Config{{Name: "editor", URL: server.URL + "/api/host/mcp", AuthEnv: "EDITOR_FLAT_FIXTURE_TOKEN"}}, "fixture", hostFixtureGate{})
	if e != nil {
		t.Fatal(e)
	}
	defer closeTools()
	var flat agentcore.Tool
	for _, tool := range tools {
		if tool.Name == "mcp_editor__editor_open_beads" {
			flat = tool
		}
	}
	if flat.ExecuteResult == nil {
		t.Fatal("native adapter omitted dedicated tool")
	}
	raw, _ := json.Marshal(map[string]any{"instance": testID, "commandId": otherID, "args": map[string]any{}})
	_, e = flat.ExecuteResult(context.Background(), raw)
	var invalid *agentcore.InvalidToolArguments
	if !errors.As(e, &invalid) || len(s.host.commands) != 0 {
		t.Fatal("nested flat input not rejected locally", e)
	}
	denied = true
	raw, _ = json.Marshal(map[string]any{"instance": testID, "commandId": otherID})
	_, e = flat.ExecuteResult(context.Background(), raw)
	if e == nil || errors.As(e, &invalid) || len(s.host.commands) != 0 {
		t.Fatal("remote policy denial became recoverable", e)
	}
}

func TestFlatEditorOpenBeadsCannotUseWindowsOnlyAuthority(t *testing.T) {
	readonly := false
	s, e := New(context.Background(), Config{DataDir: t.TempDir(), HostTools: true, Check: func(_ context.Context, obj, act string) error {
		if readonly && strings.HasPrefix(obj, "agent/host/") && obj != "agent/host/transport" && obj != "agent/host/windows" {
			return errors.New("windows-only authority")
		}
		return nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, e = s.hostRegister(HostRegistration{Instance: testID, Capabilities: []string{"open_beads"}}); e != nil {
		t.Fatal(e)
	}
	readonly = true
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "readonly-fixture", Version: "1"}, nil)
	session, e := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: server.URL + "/api/host/mcp", HTTPClient: &http.Client{Transport: coordinationAuth{s.token}}}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	result, e := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "editor_windows", Arguments: map[string]any{}})
	if e != nil || result.IsError {
		t.Fatal("windows read authority lost")
	}
	result, e = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "editor_open_beads", Arguments: map[string]any{"instance": testID, "commandId": otherID}})
	if e != nil || !result.IsError || len(s.host.commands) != 0 {
		t.Fatal("windows authority invoked write action")
	}
}
