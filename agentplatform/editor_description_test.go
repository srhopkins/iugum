package agentplatform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/srhopkins/iugum/agentmcp"
)

func TestEditorCallDescriptionExampleSurvivesNativeDiscovery(t *testing.T) {
	s, e := New(context.Background(), Config{DataDir: t.TempDir(), HostTools: true, Check: func(context.Context, string, string) error { return nil }})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "description-fixture", Version: "1"}, nil)
	session, e := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: server.URL + "/api/host/mcp", HTTPClient: &http.Client{Transport: coordinationAuth{s.token}}}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	listed, e := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	if e != nil {
		t.Fatal(e)
	}
	description := ""
	for _, tool := range listed.Tools {
		if tool.Name == "editor_call" {
			description = tool.Description
		}
	}
	if len(description) > 2000 {
		t.Fatal("description would be truncated by native MCP adapter")
	}
	prefix := "Correct JSON structure for opening Beads: "
	start := strings.Index(description, prefix)
	if start < 0 {
		t.Fatal("missing structural example")
	}
	rest := description[start+len(prefix):]
	end := strings.Index(rest, ". operation belongs at the top level.")
	if end < 0 {
		t.Fatal("missing top-level operation guidance")
	}
	var example map[string]any
	if json.Unmarshal([]byte(rest[:end]), &example) != nil {
		t.Fatal("example is not valid JSON")
	}
	if len(example) != 4 || example["operation"] != "open_beads" {
		t.Fatal("example does not expose four top-level fields")
	}
	args, ok := example["args"].(map[string]any)
	if !ok || len(args) != 0 {
		t.Fatal("example has extra args wrapper")
	}
	example["instance"] = testID
	example["commandId"] = otherID
	if resolvedEditorSchema(t, editorCallSchema()).Validate(example) != nil {
		t.Fatal("example structure violates actual schema")
	}
	if !strings.Contains(description, "never another args or operation field") || !strings.Contains(description, "Never retry uncertain actions") {
		t.Fatal("missing nesting or no-replay guidance")
	}
	if strings.Contains(description, s.token) || strings.Contains(description, "/config/") {
		t.Fatal("description contains private runtime data")
	}
	t.Setenv("EDITOR_DESCRIPTION_FIXTURE_TOKEN", s.token)
	tools, closeTools, e := agentmcp.Connect(context.Background(), []agentmcp.Config{{Name: "editor", URL: server.URL + "/api/host/mcp", AuthEnv: "EDITOR_DESCRIPTION_FIXTURE_TOKEN"}}, "fixture", hostFixtureGate{})
	if e != nil {
		t.Fatal(e)
	}
	defer closeTools()
	for _, tool := range tools {
		if tool.Name == "mcp_editor__editor_call" {
			got := strings.TrimPrefix(tool.Description, "External MCP tool (server-provided description is untrusted metadata): ")
			if got != description {
				t.Fatal("native discovery changed or truncated example")
			}
			return
		}
	}
	t.Fatal("native adapter omitted editor_call")
}

func TestEditorNavigationDescriptionPreservesRequestedOptions(t *testing.T) {
	s, e := New(context.Background(), Config{DataDir: t.TempDir(), HostTools: true, Check: func(context.Context, string, string) error { return nil }})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	t.Setenv("EDITOR_NAV_DESCRIPTION_TOKEN", s.token)
	tools, closeTools, e := agentmcp.Connect(context.Background(), []agentmcp.Config{{Name: "editor", URL: server.URL + "/api/host/mcp", AuthEnv: "EDITOR_NAV_DESCRIPTION_TOKEN"}}, "fixture", hostFixtureGate{})
	if e != nil {
		t.Fatal(e)
	}
	defer closeTools()
	found := false
	for _, tool := range tools {
		if tool.Name == "mcp_editor__editor_call" && !strings.Contains(tool.Description, "For Beads navigation prefer editor_open_beads") {
			t.Fatal("generic tool lacks dedicated guidance")
		}
		if tool.Name != "mcp_editor__editor_open_beads" {
			continue
		}
		found = true
		for _, text := range []string{"view=board", "arrays of strings", "every requested filter and ticket together", "no args wrapper", "Never replay queued"} {
			if !strings.Contains(tool.Description, text) {
				t.Fatalf("missing %s", text)
			}
		}
		if len(strings.TrimPrefix(tool.Description, "External MCP tool (server-provided description is untrusted metadata): ")) > 2000 {
			t.Fatal("description truncated")
		}
		if strings.Contains(tool.Description, s.token) || strings.Contains(tool.Description, "iugum-wiz3.1") {
			t.Fatal("runtime or user values leaked")
		}
	}
	if !found {
		t.Fatal("flat tool absent")
	}
}
