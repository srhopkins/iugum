package agentplatform

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testSessionID = "11111111-1111-1111-1111-111111111111"

// This subprocess speaks through real stdin/stdout pipes and never invokes a model.
func TestRPCProcess(t *testing.T) {
	if os.Getenv("IUGUM_RPC_HELPER") != "1" {
		return
	}
	mode := os.Getenv("IUGUM_RPC_MODE")
	cwd, _ := os.Getwd()
	readCwd := cwd
	if mode == "wrongcwd" {
		readCwd = "/foreign"
	}
	resumeCwd := cwd
	if mode == "resumecwd" {
		resumeCwd = "/foreign"
	}
	send := func(v any) { _ = json.NewEncoder(os.Stdout).Encode(v) }
	scan := bufio.NewScanner(os.Stdin)
	for scan.Scan() {
		var f frame
		if json.Unmarshal(scan.Bytes(), &f) != nil {
			os.Exit(2)
		}
		if f.Method == "" {
			continue
		}
		var p map[string]any
		_ = json.Unmarshal(f.Params, &p)
		result := any(map[string]any{})
		switch f.Method {
		case "probe":
			send(map[string]any{"jsonrpc": "2.0", "id": "native-approval", "method": "client/unsupported", "params": map[string]string{"choice": "deny"}})
			if !scan.Scan() {
				os.Exit(6)
			}
			var reply map[string]any
			_ = json.Unmarshal(scan.Bytes(), &reply)
			if reply["jsonrpc"] != "2.0" || reply["id"] != "native-approval" {
				os.Exit(7)
			}
			if mode == "unsupported" && reply["error"] == nil {
				os.Exit(8)
			}
			if mode != "unsupported" && reply["result"] != nil {
				os.Exit(9)
			}
		case "initialize":
			result = map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{"loadSession": true}}
		case "initialized", "session/cancel":
			continue
		case "thread/start":
			result = map[string]any{"thread": map[string]string{"id": testSessionID, "cwd": cwd}, "model": "fixture-model"}
		case "model/list":
			result = map[string]any{"data": []any{map[string]string{"id": "fixture-model", "model": "fixture-model", "displayName": "Fixture"}}}
		case "session/new":
			result = map[string]any{"sessionId": testSessionID, "configOptions": []any{map[string]any{"id": "chosen-model", "category": "model", "type": "select", "currentValue": "fixture-model", "options": []any{map[string]string{"value": "fixture-model", "name": "Fixture"}}}}}
		case "session/set_config_option":
			if p["sessionId"] != testSessionID || p["configId"] != "chosen-model" || p["value"] != "fixture-model" {
				os.Exit(10)
			}
			result = map[string]any{"configOptions": []any{map[string]any{"id": "chosen-model", "category": "model", "type": "select", "currentValue": "fixture-model", "options": []any{map[string]string{"value": "fixture-model", "name": "Fixture"}}}}}
		case "thread/read":
			result = map[string]any{"thread": map[string]any{"id": testSessionID, "cwd": readCwd, "turns": []any{map[string]any{"items": []any{map[string]any{"type": "agentMessage", "text": "history"}}}}}}
		case "thread/resume", "session/load":
			if p["threadId"] != testSessionID && p["sessionId"] != testSessionID {
				os.Exit(3)
			}
			if mode == "reject" {
				send(frame{ID: f.ID, Error: &rpcError{Code: -1, Message: "missing existing session"}})
				continue
			}
			if f.Method == "session/load" {
				send(map[string]any{"method": "session/update", "params": map[string]any{"sessionId": testSessionID, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "history"}}}})
			} else {
				result = map[string]any{"thread": map[string]string{"id": testSessionID, "cwd": resumeCwd}}
			}
		case "turn/start":
			if mode == "disconnect" {
				os.Exit(0)
			}
			result = map[string]any{"turn": map[string]string{"id": "current"}}
			send(frame{ID: f.ID, Result: mustJSON(result)})
			if mode == "interrupt" {
				continue
			}
			for _, id := range []string{"stale", "current"} {
				send(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": testSessionID, "turn": map[string]string{"id": id, "status": map[bool]string{true: "failed", false: "completed"}[id == "stale"]}}})
			}
			continue
		case "session/prompt":
			if mode == "disconnect" {
				os.Exit(0)
			}
			send(map[string]any{"method": "session/update", "params": map[string]any{"sessionId": testSessionID, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "answer"}}}})
		case "turn/interrupt":
			if mode == "interrupt" {
				send(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": testSessionID, "turn": map[string]string{"id": "current", "status": "interrupted"}}})
			}
			if p["turnId"] != "current" {
				os.Exit(4)
			}
		default:
			os.Exit(5)
		}
		send(frame{ID: f.ID, Result: mustJSON(result)})
	}
	os.Exit(0)
}
func mustJSON(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func helperBinary(t *testing.T, mode string) string {
	t.Helper()
	t.Setenv("IUGUM_RPC_HELPER", "1")
	t.Setenv("IUGUM_RPC_MODE", mode)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "agent")
	if err = os.WriteFile(path, []byte(fmt.Sprintf("#!/bin/sh\nexec '%s' -test.run '^TestRPCProcess$'\n", strings.ReplaceAll(exe, "'", "'\\''"))), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestRPCDisconnectNoReplay(t *testing.T) {
	r, err := startRPC(context.Background(), helperBinary(t, "disconnect"), nil, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = r.call(ctx, "turn/start", nil, nil); err == nil {
		t.Fatal("disconnect accepted")
	}
	if err = r.call(ctx, "initialize", nil, nil); err == nil {
		t.Fatal("closed connection reused")
	}
}

func TestRPCInboundRequestIdentityAndFailClosed(t *testing.T) {
	for _, mode := range []string{"unsupported", "null-result"} {
		t.Run(mode, func(t *testing.T) {
			var request func(frame) (json.RawMessage, error)
			if mode == "null-result" {
				request = func(f frame) (json.RawMessage, error) {
					if string(f.ID) != `"native-approval"` {
						t.Error("request identity changed")
					}
					return nil, nil
				}
			}
			r, e := startRPC(context.Background(), helperBinary(t, mode), nil, t.TempDir(), nil, request)
			if e != nil {
				t.Fatal(e)
			}
			defer r.close()
			ctx, c := context.WithTimeout(context.Background(), time.Second)
			defer c()
			if e = r.call(ctx, "probe", nil, nil); e != nil {
				t.Fatal(e)
			}
		})
	}
}
