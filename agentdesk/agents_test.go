package agentdesk

import (
	"context"
	"strings"
	"testing"
)

func TestAgentRoutingIsolation(t *testing.T) {
	child, _ := New(Config{Name: "Local", DataDir: t.TempDir(), Chat: func(context.Context, string) (string, error) { return "Local response", nil }, PassthroughChat: true})
	root, _ := New(Config{Name: "Coordinator", DataDir: t.TempDir(), Agents: map[string]*Server{"local": child}})
	options := req(root, "GET", "/api/agents", "")
	if !strings.Contains(options.Body.String(), "Coordinator") || !strings.Contains(options.Body.String(), "Local") {
		t.Fatal(options.Body.String())
	}
	r := req(root, "POST", "/api/agents/local/chat", `{"text":"status"}`)
	if !strings.Contains(r.Body.String(), "Local response") {
		t.Fatal(r.Body.String())
	}
	if strings.Contains(req(root, "GET", "/api/messages", "").Body.String(), "Local response") {
		t.Fatal("history leaked to default")
	}
	if !strings.Contains(req(root, "GET", "/api/agents/local/messages", "").Body.String(), "Local response") {
		t.Fatal("history missing")
	}
	if req(root, "GET", "/api/agents/unknown/messages", "").Code != 404 {
		t.Fatal("unknown agent accepted")
	}
}
