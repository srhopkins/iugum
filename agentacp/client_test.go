package agentacp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	acp "github.com/coder/acp-go-sdk"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestACPHelper(t *testing.T) {
	if os.Getenv("IUGUM_ACP_TEST_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var r struct {
			ID     any            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal(scanner.Bytes(), &r)
		result := map[string]any{}
		switch r.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{"loadSession": true}}
		case "session/new":
			result = map[string]any{"sessionId": "fixture-session"}
		case "session/load":
			if r.Params["sessionId"] != "fixture-session" {
				os.Exit(2)
			}
		case "session/set_config_option":
			if r.Params["configId"] != "model" || r.Params["value"] != "ollama/test" {
				os.Exit(3)
			}
			result = map[string]any{"configOptions": []any{}}
		case "session/prompt":
			b, _ := json.Marshal(r.Params)
			if !strings.Contains(string(b), "Current time:") {
				os.Exit(4)
			}
			n := map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "fixture-session", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "ACP answer"}}}}
			_ = json.NewEncoder(os.Stdout).Encode(n)
			result = map[string]any{"stopReason": "end_turn"}
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"jsonrpc": "2.0", "id": r.ID, "result": result})
	}
	os.Exit(0)
}
func TestSDKRoundTripAndReload(t *testing.T) {
	c := &Client{Config: Config{Command: []string{os.Args[0], "-test.run=TestACPHelper"}, Cwd: t.TempDir(), Model: "ollama/test", Env: map[string]string{"IUGUM_ACP_TEST_HELPER": "1"}}, StateDir: t.TempDir(), Check: func(context.Context, string, string) error { return nil }}
	for i := 0; i < 2; i++ {
		answer, err := c.Chat(context.Background(), "hello")
		if err != nil || answer != "ACP answer" {
			t.Fatalf("turn %d: %q %v", i, answer, err)
		}
	}
	usage, err := os.ReadFile(filepath.Join(c.StateDir, "usage.jsonl"))
	if err != nil || !strings.Contains(string(usage), `"usage":null`) {
		t.Fatal("missing usage must remain unknown", string(usage), err)
	}
	c.Check = func(context.Context, string, string) error { return fmt.Errorf("denied") }
	if _, err := c.Chat(context.Background(), "hello"); err == nil {
		t.Fatal("policy bypass")
	}
}
func TestPermissionDefaultsToDenied(t *testing.T) {
	c := &Client{}
	r, err := c.RequestPermission(context.Background(), acp.RequestPermissionRequest{})
	if err != nil || r.Outcome.Cancelled == nil {
		t.Fatal(r, err)
	}
}
