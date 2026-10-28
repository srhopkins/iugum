package agentplatform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/srhopkins/iugum/agentcore"
	"github.com/srhopkins/iugum/agentmcp"
)

func resolvedEditorSchema(t *testing.T, raw map[string]any) *jsonschema.Resolved {
	t.Helper()
	b, _ := json.Marshal(raw)
	var s jsonschema.Schema
	if e := json.Unmarshal(b, &s); e != nil {
		t.Fatal(e)
	}
	r, e := s.Resolve(nil)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func editorInput(op string, args any) map[string]any {
	return map[string]any{"instance": testID, "commandId": otherID, "operation": op, "args": args}
}
func TestEditorSchemaAndServerOperationBoundaries(t *testing.T) {
	schema := editorCallSchema()
	for _, key := range []string{"oneOf", "anyOf", "allOf", "enum", "const", "not"} {
		if _, exists := schema[key]; exists {
			t.Fatalf("provider-forbidden root keyword %s", key)
		}
	}
	r := resolvedEditorSchema(t, schema)
	cases := []struct {
		name, op, args string
		valid          bool
	}{
		{"inspect", "inspect", `{}`, true}, {"beads landing", "open_beads", `{}`, true}, {"beads navigation", "open_beads", `{"view":"tree","projects":["iugum"],"search":"tools","statuses":["open"],"priorities":["1"],"types":["bug"],"assignees":["worker"],"labels":["test"],"sort":"priority","lanes":true,"ticket":"iugum-9din","ticketProject":"iugum"}`, true},
		{"file", "open_file", `{"path":"README.md","line":1,"group":1}`, true}, {"file upper group", "open_file", `{"path":"docs/readme","line":1,"column":0,"endLine":0,"endColumn":0,"group":9}`, true}, {"focus", "focus", `{"pane":"chat"}`, true},
		{"unknown op", "shell", `{}`, false}, {"inspect fields", "inspect", `{"pane":"chat"}`, false}, {"focus missing", "focus", `{}`, false}, {"focus unknown", "focus", `{"pane":"not-a-pane"}`, false}, {"file missing", "open_file", `{"path":"README.md"}`, false}, {"file zero line", "open_file", `{"path":"README.md","line":0,"group":1}`, false}, {"file group10", "open_file", `{"path":"README.md","line":1,"group":10}`, false}, {"file fractional", "open_file", `{"path":"README.md","line":1.5,"group":1}`, false}, {"file null line", "open_file", `{"path":"README.md","line":null,"group":1}`, false}, {"file null column", "open_file", `{"path":"README.md","line":1,"column":null,"group":1}`, false},
		{"absolute file", "open_file", `{"path":"/README.md","line":1,"group":1}`, false}, {"backslash file", "open_file", `{"path":"a\\b","line":1,"group":1}`, false}, {"parent file", "open_file", `{"path":"a/../README.md","line":1,"group":1}`, false},
		{"beads unknown field", "open_beads", `{"url":"https://other.invalid"}`, false}, {"beads bad view", "open_beads", `{"view":"terminal"}`, false}, {"beads bad sort", "open_beads", `{"sort":"name"}`, false}, {"beads null", "open_beads", `{"search":null}`, false}, {"beads bool type", "open_beads", `{"lanes":"true"}`, false}, {"beads dependent ticket", "open_beads", `{"ticketProject":"iugum"}`, false}, {"beads ticket pattern", "open_beads", `{"ticket":"../x"}`, false}, {"beads array type", "open_beads", `{"labels":"bug"}`, false}, {"beads item type", "open_beads", `{"labels":[1]}`, false}, {"beads control", "open_beads", `{"search":"a\nb"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var args any
			json.Unmarshal([]byte(tc.args), &args)
			schemaOK := r.Validate(editorInput(tc.op, args)) == nil
			serverOK := validateHostCall(HostCall{Instance: testID, CommandID: otherID, Operation: tc.op, Args: json.RawMessage(tc.args)}) == nil
			if schemaOK != tc.valid || serverOK != tc.valid {
				t.Fatalf("schema=%v server=%v want=%v", schemaOK, serverOK, tc.valid)
			}
		})
	}
	for _, key := range []string{"instance", "commandId"} {
		for _, bad := range []string{"open_beads_command", "", strings.Repeat("x", 36)} {
			q := editorInput("open_beads", map[string]any{})
			q[key] = bad
			if r.Validate(q) == nil {
				t.Fatalf("accepted invalid %s", key)
			}
		}
	}
	for _, field := range []string{"instance", "commandId", "operation", "args"} {
		q := editorInput("inspect", map[string]any{})
		delete(q, field)
		if r.Validate(q) == nil {
			t.Fatalf("missing required field accepted: %s", field)
		}
	}
	for _, pane := range []string{"editor", "sidebar", "panel", "terminal", "chat"} {
		if r.Validate(editorInput("focus", map[string]any{"pane": pane})) != nil {
			t.Fatalf("valid pane rejected: %s", pane)
		}
	}
	labels := make([]string, 21)
	for i := range labels {
		labels[i] = "fixture"
	}
	if r.Validate(editorInput("open_beads", map[string]any{"labels": labels})) == nil {
		t.Fatal("21 filters accepted")
	}
	if r.Validate(editorInput("open_beads", map[string]any{"labels": labels[:20]})) != nil {
		t.Fatal("20 filters rejected")
	}
	if r.Validate(editorInput("open_beads", map[string]any{"search": strings.Repeat("a", 513)})) == nil {
		t.Fatal("oversized search accepted")
	}
	status := resolvedEditorSchema(t, editorStatusSchema())
	if status.Validate(map[string]any{"instance": testID, "commandId": otherID}) != nil || status.Validate(map[string]any{"instance": testID, "commandId": "word-label"}) == nil {
		t.Fatal("status identity schema mismatch")
	}
	// Cross-field selection ordering cannot be expressed by these JSON Schema
	// keywords. The authoritative server rejects it before queuing, matching the extension.
	if validateHostCall(HostCall{Instance: testID, CommandID: otherID, Operation: "open_file", Args: json.RawMessage(`{"path":"README.md","line":3,"endLine":2,"group":1}`)}) == nil {
		t.Fatal("reversed selection accepted")
	}
}
func TestEditorActualSchemaSameTurnUUIDCorrectionAndReceipt(t *testing.T) {
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
	t.Setenv("EDITOR_CONTRACT_FIXTURE_TOKEN", s.token)
	tools, closeTools, e := agentmcp.Connect(context.Background(), []agentmcp.Config{{Name: "editor", URL: server.URL + "/api/host/mcp", AuthEnv: "EDITOR_CONTRACT_FIXTURE_TOKEN"}}, "fixture", hostFixtureGate{})
	if e != nil {
		t.Fatal(e)
	}
	defer closeTools()
	var call agentcore.Tool
	for _, tool := range tools {
		if tool.Name == "mcp_editor__editor_call" {
			call = tool
		}
	}
	bad := fmt.Sprintf(`{"instance":%q,"commandId":"open_beads_command","operation":"open_beads","args":{}}`, testID)
	_, e = call.Execute(context.Background(), json.RawMessage(bad))
	var invalid *agentcore.InvalidToolArguments
	if !errors.As(e, &invalid) || !strings.Contains(e.Error(), "arguments.commandId must be a UUID") || strings.Contains(e.Error(), "open_beads_command") || len(s.host.commands) != 0 {
		t.Fatalf("unsafe invalid attempt: %v", e)
	}
	// External clients receive the same constraints, and SDK rejection queues no action.
	client := mcp.NewClient(&mcp.Implementation{Name: "external-fixture", Version: "1"}, nil)
	session, e := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: server.URL + "/api/host/mcp", HTTPClient: &http.Client{Transport: coordinationAuth{s.token}}}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	result, e := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "editor_call", Arguments: json.RawMessage(bad)})
	if e != nil || !result.IsError || len(s.host.commands) != 0 {
		t.Fatalf("external invalid dispatch: %v %+v", e, result)
	}
	steps := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		steps++
		var body struct {
			Messages []agentcore.Message `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		arguments := bad
		id := "invalid"
		if steps == 2 {
			if len(s.host.commands) != 0 {
				t.Error("invalid call queued")
			}
			last := body.Messages[len(body.Messages)-1]
			if !strings.Contains(last.Content, "UUID") {
				t.Error("missing UUID feedback")
			}
			arguments = fmt.Sprintf(`{"instance":%q,"commandId":%q,"operation":"open_beads","args":{}}`, testID, otherID)
			id = "corrected"
		}
		if steps > 2 {
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"queued; receipt required"}}]}`)
			return
		}
		response := map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": id, "type": "function", "function": map[string]any{"name": call.Name, "arguments": arguments}}}}}}}
		json.NewEncoder(w).Encode(response)
	}))
	defer model.Close()
	cfg := agentcore.Config{DefaultProfile: "fixture", Pools: map[string]agentcore.Pool{"fixture": {Provider: "openai", BaseURL: model.URL}}, Profiles: map[string]agentcore.Profile{"fixture": {Pool: "fixture", Model: "synthetic"}}}
	runtime, e := agentcore.New(cfg, "", hostFixtureGate{})
	if e != nil {
		t.Fatal(e)
	}
	_, e = runtime.Run(context.Background(), agentcore.RunRequest{Actor: "fixture"}, tools)
	if e != nil || steps != 3 || len(s.host.commands) != 1 {
		t.Fatalf("same-turn correction: %v steps=%d commands=%d", e, steps, len(s.host.commands))
	}
	receipt, e := s.hostStatus(context.Background(), testID, otherID)
	if e != nil || receipt.CommandID != otherID || receipt.State != "queued" {
		t.Fatalf("receipt mismatch: %+v %v", receipt, e)
	}
	if _, e = s.hostStatus(context.Background(), testID, testID); e == nil || !strings.Contains(e.Error(), "does not mean an editor capability is unsupported") {
		t.Fatal("missing receipt misdiagnosed", e)
	}
	denied = true
	_, e = call.Execute(context.Background(), json.RawMessage(fmt.Sprintf(`{"instance":%q,"commandId":%q,"operation":"open_beads","args":{}}`, testID, testID)))
	if e == nil || errors.As(e, &invalid) || len(s.host.commands) != 1 {
		t.Fatalf("denial reclassified: %v", e)
	}
}
