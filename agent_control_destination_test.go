package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/srhopkins/iugum/agentcontrol"
	"github.com/srhopkins/iugum/agentcore"
	"github.com/srhopkins/iugum/agentdesk"
	"github.com/srhopkins/iugum/contract"
)

type destinationPolicy struct {
	deny  string
	cause error
}

func (g *destinationPolicy) Enforce(_ context.Context, r contract.Request) error {
	if r.Act == g.deny {
		if g.cause != nil {
			return g.cause
		}
		return errors.New("PRIVATE_POLICY")
	}
	return nil
}
func destinationTool(t *testing.T, b *agentControlBridge, name string) agentcore.Tool {
	t.Helper()
	for _, tool := range b.Tools() {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatal("missing fixture tool")
	return agentcore.Tool{}
}
func destinationBridge(t *testing.T, root string, gate contract.Policy, accounts map[string]string, check func(context.Context, string, string) error) (*agentcontrol.Manager, *agentControlBridge) {
	t.Helper()
	m, err := agentcontrol.New(root, gate)
	if err != nil {
		t.Fatal(err)
	}
	b := newAgentControlBridge(context.Background(), m, "fixture", accounts, check, func(context.Context, agentdesk.ApprovalRequest) (agentdesk.Approval, error) {
		t.Error("unexpected start approval")
		return agentdesk.Approval{}, errors.New("not allowed")
	})
	return m, b
}

func TestControlMissingDestinationContinuesWithoutWorker(t *testing.T) {
	root := t.TempDir()
	manager, bridge := destinationBridge(t, root, &destinationPolicy{}, nil, func(context.Context, string, string) error { return nil })
	sessions := destinationTool(t, bridge, "claude_sessions")
	if output, err := sessions.Execute(context.Background(), json.RawMessage("{}")); err != nil || output != "[]" {
		t.Fatal("fixture is not empty")
	}
	modelCalls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelCalls++
		if modelCalls == 1 {
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"one","type":"function","function":{"name":"claude_send","arguments":"{\"id\":\"PRIVATE_ID\",\"prompt\":\"PRIVATE_PROMPT\"}"}}]}}]}`)
			return
		}
		var request struct {
			Messages []agentcore.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		last := request.Messages[len(request.Messages)-1]
		var feedback struct {
			IsError bool   `json:"is_error"`
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal([]byte(last.Content), &feedback) != nil || !feedback.IsError || feedback.Code != "invalid_arguments" || last.ToolCallID != "one" {
			t.Error("missing same-turn correction")
		}
		for _, required := range []string{"claude_sessions", "tmux", "already-authorized", "explicit approval", "Do not automatically"} {
			if !strings.Contains(feedback.Message, required) {
				t.Error("missing fixed guidance")
			}
		}
		for _, private := range []string{"PRIVATE_ID", "PRIVATE_PROMPT", root} {
			if strings.Contains(last.Content, private) {
				t.Error("private destination data leaked")
			}
		}
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"No managed destination is available. I will use the already authorized route."}}]}`)
	}))
	defer model.Close()
	cfg := agentcore.Config{DefaultProfile: "fixture", MaxSteps: 2, Pools: map[string]agentcore.Pool{"fixture": {BaseURL: model.URL}}, Profiles: map[string]agentcore.Profile{"fixture": {Pool: "fixture", Model: "fixture"}}}
	runtime, err := agentcore.New(cfg, "", controlTestGate{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Run(context.Background(), agentcore.RunRequest{Actor: "fixture"}, bridge.Tools())
	if err != nil || modelCalls != 2 || !strings.Contains(result.Text, "No managed destination") {
		t.Fatalf("correction failed: %v", err)
	}
	states, err := manager.List(context.Background(), "fixture")
	entries, readErr := os.ReadDir(root)
	if err != nil || readErr != nil || len(states) != 0 || len(entries) != 0 {
		t.Fatal("missing destination wrote state or started worker")
	}
}

func TestControlMissingDestinationDenialStaysHard(t *testing.T) {
	for _, denied := range []string{"send", "read"} {
		t.Run(denied, func(t *testing.T) {
			root := t.TempDir()
			_, bridge := destinationBridge(t, root, &destinationPolicy{deny: denied}, nil, func(context.Context, string, string) error { return nil })
			_, err := destinationTool(t, bridge, "claude_send").Execute(context.Background(), json.RawMessage(`{"id":"PRIVATE_ID","prompt":"PRIVATE_PROMPT"}`))
			var invalid *agentcore.InvalidToolArguments
			if err == nil || errors.As(err, &invalid) || errors.Is(err, agentcontrol.ErrSessionNotFound) {
				t.Fatal("policy denial became correction")
			}
		})
	}
}

func TestControlSavedUnmanagedDestinationIsPrequeueCorrection(t *testing.T) {
	root := t.TempDir()
	state := agentcontrol.State{ID: "saved", Status: "disconnected", Request: agentcontrol.StartRequest{Account: "fixture@example.test", Project: root, ConfigDir: root}}
	raw, _ := json.Marshal(state)
	if err := os.WriteFile(filepath.Join(root, "saved.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	_, bridge := destinationBridge(t, root, &destinationPolicy{}, map[string]string{"fixture@example.test": root}, func(context.Context, string, string) error { return nil })
	_, err := destinationTool(t, bridge, "claude_send").Execute(context.Background(), json.RawMessage(`{"id":"saved","prompt":"PRIVATE_PROMPT"}`))
	var invalid *agentcore.InvalidToolArguments
	if !errors.As(err, &invalid) {
		t.Fatal("prequeue unmanaged result was not correction")
	}
	after, err := os.ReadFile(filepath.Join(root, "saved.json"))
	if err != nil || string(after) != string(raw) {
		t.Fatal("unmanaged correction mutated state")
	}
	if _, err = os.Stat(filepath.Join(root, "saved.events.jsonl")); !os.IsNotExist(err) {
		t.Fatal("unmanaged correction journaled a message")
	}
}

// This child only speaks the test protocol; it never invokes a coding agent.
func TestControlDestinationProcessFixture(t *testing.T) {
	helper := false
	for _, arg := range os.Args {
		if arg == "--print" {
			helper = true
		}
	}
	if !helper {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request map[string]json.RawMessage
		_ = json.Unmarshal(scanner.Bytes(), &request)
		var kind string
		_ = json.Unmarshal(request["type"], &kind)
		if kind == "control_request" {
			var id string
			_ = json.Unmarshal(request["request_id"], &id)
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"type": "control_response", "response": map[string]any{"request_id": id, "subtype": "success", "response": map[string]any{}}})
		}
	}
	os.Exit(0)
}

func TestControlManagedDestinationAndHardFailures(t *testing.T) {
	for _, mode := range []string{"managed", "revoked_account", "account_denied", "project_denied", "journal_failure", "write_uncertainty"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			stateRoot := filepath.Join(root, "state")
			account := "fixture@example.test"
			accounts := map[string]string{account: root}
			_, bridge := destinationBridge(t, stateRoot, &destinationPolicy{}, accounts, func(_ context.Context, obj, act string) error {
				if mode == "account_denied" && strings.HasPrefix(obj, "agent/account/") {
					return errors.New("PRIVATE_ACCOUNT_DENIAL")
				}
				if mode == "project_denied" && strings.HasPrefix(obj, "project:") {
					return errors.New("PRIVATE_PROJECT_DENIAL")
				}
				return nil
			})
			manager := bridge.manager
			binary, _ := os.Executable()
			script := filepath.Join(root, "fixture")
			if err := os.WriteFile(script, []byte(fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestControlDestinationProcessFixture -- \"$@\"\n", binary)), 0700); err != nil {
				t.Fatal(err)
			}
			manager.Executable = script
			manager.VerifyAccount = func(context.Context, string, agentcontrol.StartRequest) error { return nil }
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := manager.Start(ctx, agentcontrol.StartRequest{Actor: "fixture", ID: "managed", Account: account, Project: root, ConfigDir: root, MaxDuration: 4 * time.Second}); err != nil {
				t.Fatal(err)
			}
			defer manager.Stop(context.Background(), "fixture", "managed")
			if mode == "revoked_account" {
				delete(accounts, account)
			}
			if mode == "journal_failure" {
				if err := os.Remove(filepath.Join(stateRoot, "managed.events.jsonl")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(stateRoot, "managed.events.jsonl"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "write_uncertainty" {
				if err := os.Remove(filepath.Join(stateRoot, "managed.json")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(stateRoot, "managed.json"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			output, err := destinationTool(t, bridge, "claude_send").Execute(context.Background(), json.RawMessage(`{"id":"managed","prompt":"PRIVATE_PROMPT"}`))
			var invalid *agentcore.InvalidToolArguments
			if mode == "managed" {
				if err != nil || !strings.Contains(output, "managed") {
					t.Fatal("existing managed send changed")
				}
				events, eventErr := manager.Events(context.Background(), "fixture", "managed", 50)
				if eventErr != nil {
					t.Fatal(eventErr)
				}
				queued := 0
				for _, event := range events {
					if event.Direction == "control" {
						queued++
					}
				}
				if queued != 1 {
					t.Fatal("managed message not queued exactly once")
				}
			} else {
				if err == nil || errors.As(err, &invalid) {
					t.Fatal("hard or uncertain failure became correction")
				}
				state, snapshotErr := manager.Snapshot(context.Background(), "fixture", "managed")
				if snapshotErr != nil {
					t.Fatal(snapshotErr)
				}
				if mode == "write_uncertainty" {
					if len(state.Pending) != 1 {
						t.Fatal("uncertain write was retried or forgotten")
					}
				} else if len(state.Pending) != 0 {
					t.Fatal("scope denial queued work")
				}
			}
		})
	}
}

func TestControlNonMissingStateErrorsRemainHard(t *testing.T) {
	for _, mode := range []string{"corrupt", "unreadable", "invalid_id"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			_, bridge := destinationBridge(t, root, &destinationPolicy{}, nil, func(context.Context, string, string) error { return nil })
			id := "PRIVATE_ID"
			if mode == "corrupt" {
				if err := os.WriteFile(filepath.Join(root, id+".json"), []byte("PRIVATE_CORRUPT"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if mode == "unreadable" {
				if err := os.Mkdir(filepath.Join(root, id+".json"), 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				id = "../PRIVATE_ID"
			}
			raw, _ := json.Marshal(map[string]string{"id": id, "prompt": "PRIVATE_PROMPT"})
			_, err := destinationTool(t, bridge, "claude_send").Execute(context.Background(), raw)
			var invalid *agentcore.InvalidToolArguments
			if err == nil || errors.As(err, &invalid) {
				t.Fatal("non-missing state error became correction")
			}
		})
	}
}

func TestControlPolicyWrappedDestinationSentinelsRemainHard(t *testing.T) {
	for _, action := range []string{"send", "read"} {
		for _, reason := range []error{agentcontrol.ErrSessionNotFound, agentcontrol.ErrUnmanaged} {
			root := t.TempDir()
			cause := fmt.Errorf("PRIVATE_DENIAL: %w", reason)
			_, bridge := destinationBridge(t, root, &destinationPolicy{deny: action, cause: cause}, nil, func(context.Context, string, string) error { return nil })
			_, err := destinationTool(t, bridge, "claude_send").Execute(context.Background(), json.RawMessage(`{"id":"PRIVATE_ID","prompt":"PRIVATE_PROMPT"}`))
			var invalid *agentcore.InvalidToolArguments
			if err == nil || errors.As(err, &invalid) || !errors.Is(err, cause) {
				t.Fatal("sentinel-wrapped policy denial became correction or chain lost")
			}
			entries, readErr := os.ReadDir(root)
			if readErr != nil || len(entries) != 0 {
				t.Fatal("denial wrote destination state")
			}
		}
	}
}
