package agentplatform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/srhopkins/iugum/agentcore"
	"github.com/srhopkins/iugum/agentmcp"
)

func TestFlatEditorNavigationCorrectionPreservesAllRequestedFields(t *testing.T) {
	s, e := New(context.Background(), Config{DataDir: t.TempDir(), HostTools: true, Check: func(context.Context, string, string) error { return nil }})
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
	t.Setenv("EDITOR_NAV_FEEDBACK_TOKEN", s.token)
	tools, closeTools, e := agentmcp.Connect(context.Background(), []agentmcp.Config{{Name: "editor", URL: server.URL + "/api/host/mcp", AuthEnv: "EDITOR_NAV_FEEDBACK_TOKEN"}}, "fixture", hostFixtureGate{})
	if e != nil {
		t.Fatal(e)
	}
	defer closeTools()
	var tool agentcore.Tool
	for _, candidate := range tools {
		if candidate.Name == "mcp_editor__editor_open_beads" {
			tool = candidate
		}
	}
	nav := map[string]any{"view": "board", "projects": []any{"iugum"}, "statuses": []any{"closed"}, "labels": []any{"side-request"}, "ticket": "iugum-wiz3.1"}
	bad, _ := json.Marshal(map[string]any{"instance": testID, "commandId": otherID, "view": "board", "args": nav, "PRIVATE_UNKNOWN": "PRIVATE_VALUE"})
	_, e = tool.ExecuteResult(context.Background(), bad)
	var invalid *agentcore.InvalidToolArguments
	if !errors.As(e, &invalid) || len(s.host.commands) != 0 {
		t.Fatal("malformed call crossed dispatch boundary")
	}
	for key := range editorOpenBeadsSchema()["properties"].(map[string]any) {
		if !strings.Contains(invalid.Message, key) {
			t.Fatalf("feedback omitted schema field %s", key)
		}
	}
	if strings.Contains(invalid.Message, "PRIVATE") || strings.Contains(invalid.Message, "iugum-wiz3.1") || strings.Contains(invalid.Message, testID) {
		t.Fatal("feedback exposed input values")
	}
	corrected := map[string]any{"instance": testID, "commandId": otherID}
	for k, v := range nav {
		corrected[k] = v
	}
	raw, _ := json.Marshal(corrected)
	if _, e = tool.ExecuteResult(context.Background(), raw); e != nil {
		t.Fatal(e)
	}
	command, e := s.hostPoll(context.Background(), identity)
	if e != nil || command == nil {
		t.Fatal("corrected call not queued")
	}
	var got map[string]any
	json.Unmarshal(command.Args, &got)
	if !reflect.DeepEqual(got, nav) || len(s.host.commands) != 1 {
		t.Fatal("requested navigation changed")
	}
}
