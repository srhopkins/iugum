package agentplatform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/srhopkins/iugum/agentcore"
	"github.com/srhopkins/iugum/agentmcp"
	"gopkg.in/yaml.v3"
)

// Opt in only: uses the existing configured GPT-4o-mini profile against a wholly
// disposable editor host and simulated extension. Never contacts a live editor.
func TestEditorUnseededModelProbe(t *testing.T) {
	configPath := os.Getenv("IUGUM_EDITOR_MODEL_FIXTURE_CONFIG")
	if configPath == "" {
		t.Skip("explicit model fixture opt-in required")
	}
	raw, e := os.ReadFile(configPath)
	if e != nil {
		t.Fatal("fixture configuration unavailable")
	}
	var cfg struct {
		Models           agentcore.Config `yaml:"models"`
		InstructionsFile string           `yaml:"instructions_file"`
	}
	if yaml.Unmarshal(raw, &cfg) != nil {
		t.Fatal("invalid fixture configuration")
	}
	resolve := func(path string) string {
		if path != "" && !filepath.IsAbs(path) {
			return filepath.Join(filepath.Dir(configPath), path)
		}
		return path
	}
	for name, pool := range cfg.Models.Pools {
		pool.APIKeyFile = resolve(pool.APIKeyFile)
		cfg.Models.Pools[name] = pool
	}
	if cfg.InstructionsFile != "" {
		if text, e := os.ReadFile(resolve(cfg.InstructionsFile)); e == nil {
			cfg.Models.Instructions = string(text)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if agentcore.DiscoverProfiles(ctx, &cfg.Models) != nil {
		t.Fatal("configured model discovery failed")
	}
	count := 0
	for name, p := range cfg.Models.Profiles {
		if p.Model == "gpt-4o-mini" {
			count++
			cfg.Models.DefaultProfile = name
		}
	}
	if count != 1 {
		t.Fatal("fixture requires unique existing GPT-4o-mini profile")
	}
	cfg.Models.UsagePath = ""
	host, e := New(ctx, Config{DataDir: t.TempDir(), HostTools: true, Check: func(context.Context, string, string) error { return nil }})
	if e != nil {
		t.Fatal(e)
	}
	defer host.Close()
	identity, e := host.hostRegister(HostRegistration{Instance: testID, Capabilities: []string{"open_beads"}})
	if e != nil {
		t.Fatal(e)
	}
	sdk := httptest.NewServer(host.Handler())
	defer sdk.Close()
	t.Setenv("EDITOR_MODEL_FIXTURE_TOKEN", host.token)
	tools, closeTools, e := agentmcp.Connect(ctx, []agentmcp.Config{{Name: "editor", URL: sdk.URL + "/api/host/mcp", AuthEnv: "EDITOR_MODEL_FIXTURE_TOKEN"}}, "fixture", hostFixtureGate{})
	if e != nil {
		t.Fatal(e)
	}
	defer closeTools()
	bad, _ := json.Marshal(editorInput("open_beads", map[string]any{}))
	var values map[string]any
	json.Unmarshal(bad, &values)
	values["commandId"] = "open_beads_command"
	bad, _ = json.Marshal(values)
	var call agentcore.Tool
	for _, tool := range tools {
		if strings.HasSuffix(tool.Name, "__editor_call") {
			call = tool
		}
	}
	_, e = call.Execute(ctx, bad)
	var invalid *agentcore.InvalidToolArguments
	if !errors.As(e, &invalid) || len(host.host.commands) != 0 {
		t.Fatal("fixture preexecution validation failed")
	}
	feedback, _ := json.Marshal(map[string]any{"is_error": true, "code": "invalid_arguments", "message": invalid.Message})
	windows, _ := host.hostWindows(ctx)
	windowJSON, _ := json.Marshal(windows)
	var simulatedExecutions atomic.Int64
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				command, e := host.hostPoll(ctx, identity)
				if e != nil || command == nil {
					continue
				}
				simulatedExecutions.Add(1)
				_ = host.hostResult(HostResult{HostIdentity: identity, CommandID: command.CommandID, Result: json.RawMessage(`{"fixture":true,"opened":true}`)})
			}
		}
	}()
	runtime, e := agentcore.New(cfg.Models, "", hostFixtureGate{})
	if e != nil {
		t.Fatal("fixture runtime setup failed")
	}
	runtime.Client = &http.Client{Transport: editorFixtureDiagnostics{t: t}, Timeout: 60 * time.Second}
	variant := os.Getenv("IUGUM_EDITOR_PROBE_VARIANT")
	for i := range tools {
		if tools[i].Name == "mcp_editor__editor_call" {
			switch variant {
			case "A":
				b, _ := json.Marshal(tools[i].Parameters)
				var simpler map[string]any
				json.Unmarshal(b, &simpler)
				delete(simpler, "dependentSchemas")
				props := simpler["properties"].(map[string]any)
				argProps := editorBeadsArgsSchema()["properties"].(map[string]any)
				for key, value := range map[string]any{"path": map[string]any{"type": "string"}, "line": map[string]any{"type": "integer"}, "column": map[string]any{"type": "integer"}, "endLine": map[string]any{"type": "integer"}, "endColumn": map[string]any{"type": "integer"}, "group": map[string]any{"type": "integer"}, "pane": map[string]any{"type": "string"}} {
					argProps[key] = value
				}
				props["args"] = map[string]any{"type": "object", "additionalProperties": false, "properties": argProps, "description": "Operation arguments only. operation is a separate top-level field. open_beads: {} for landing view, or optional navigation fields; inspect: {}; focus: pane; open_file: path, line, group and optional selection."}
				tools[i].Parameters = simpler
			case "C":
				tools[i].Description += ` Correct JSON structure for opening Beads: {"instance":"UUID from editor_windows","commandId":"fresh UUID","operation":"open_beads","args":{}}. operation belongs at the top level. args contains only navigation options, never another args or operation field. Keep args empty to open the configured landing view.`
			}
		}
	}
	if variant == "B" {
		props := editorBeadsArgsSchema()["properties"].(map[string]any)
		for key, value := range editorIdentityProperties() {
			props[key] = value
		}
		original := call
		tools = append(tools, agentcore.Tool{Name: "mcp_editor__editor_open_beads", Description: "Open the configured Beads viewer in this exact editor instance. Prefer this dedicated tool for Beads. All fields are top level: instance, fresh commandId UUID, and optional navigation options. Omit all optional options for the configured landing view. It accepts no args wrapper and no operation field. Use editor_status with the returned exact receipt commandId to verify completion; never replay uncertain actions.", Parameters: map[string]any{"type": "object", "additionalProperties": false, "properties": props, "required": []string{"instance", "commandId"}, "dependentRequired": map[string]any{"ticketProject": []string{"ticket"}}}, ExecuteResult: func(ctx context.Context, raw json.RawMessage) (agentcore.ToolResult, error) {
			var flat map[string]any
			if json.Unmarshal(raw, &flat) != nil {
				return agentcore.ToolResult{}, &agentcore.InvalidToolArguments{Message: "Use a flat JSON object matching the dedicated tool schema."}
			}
			args := map[string]any{}
			for key, value := range flat {
				if key != "instance" && key != "commandId" {
					args[key] = value
				}
			}
			q, _ := json.Marshal(map[string]any{"instance": flat["instance"], "commandId": flat["commandId"], "operation": "open_beads", "args": args})
			return original.ExecuteResult(ctx, q)
		}})
	}
	if catalogPath := os.Getenv("IUGUM_EDITOR_PROBE_FULL_CATALOG"); catalogPath != "" {
		raw, e := os.ReadFile(catalogPath)
		if e != nil {
			t.Fatal("catalog unavailable")
		}
		var definitions []struct {
			Function struct {
				Name        string         `json:"name"`
				Description string         `json:"description"`
				Parameters  map[string]any `json:"parameters"`
			} `json:"function"`
		}
		if json.Unmarshal(raw, &definitions) != nil || len(definitions) > 100 {
			t.Fatal("invalid bounded catalog")
		}
		editors := map[string]agentcore.Tool{}
		for _, tool := range tools {
			editors[tool.Name] = tool
		}
		full := []agentcore.Tool{}
		for _, definition := range definitions {
			f := definition.Function
			if tool, exists := editors[f.Name]; exists {
				full = append(full, tool)
			} else {
				full = append(full, agentcore.Tool{Name: f.Name, Description: f.Description, Parameters: f.Parameters, ExecuteResult: func(context.Context, json.RawMessage) (agentcore.ToolResult, error) {
					return agentcore.ToolResult{}, errors.New("Disposable diagnostic forbids noneditor action")
				}})
			}
		}
		tools = full
	}
	t.Logf("probe_variant=%s prompt_bytes=383 cap=%d catalog_tools=%d", variant, cfg.Models.MaxSteps, len(tools))
	var diagnosticCount atomic.Int64
	for i := range tools {
		original := tools[i]
		tools[i].ExecuteResult = func(ctx context.Context, raw json.RawMessage) (agentcore.ToolResult, error) {
			record := editorProbeShape(original.Name, raw)
			result, err := original.ExecuteResult(ctx, raw)
			if err != nil {
				var invalid *agentcore.InvalidToolArguments
				if errors.As(err, &invalid) {
					record["validation"] = invalid.Message
				} else {
					record["validation"] = "hard_error"
				}
			} else {
				record["validation"] = "accepted"
			}
			if diagnosticCount.Add(1) <= 16 {
				t.Logf("EDITOR_PROBE %s", editorProbeJSON(record))
			}
			return result, err
		}
	}
	_ = feedback
	_ = windowJSON
	result, e := runtime.Run(ctx, agentcore.RunRequest{Actor: "fixture", Messages: []agentcore.Message{{Role: "user", Content: "Open the configured Beads viewer inside this code-server window using the available typed editor-control tools and its Simple Browser panel. Discover the live editor instance, issue the supported open_beads action, and check its execution receipt. Do not use shell commands, launch another code-server process, or change settings or permissions. Report what the actual tools confirm."}}}, tools)
	if e != nil || simulatedExecutions.Load() != 1 {
		t.Fatalf("unseeded result success=%t simulated_executions=%d error=%v", e == nil, simulatedExecutions.Load(), e)
	}
	host.host.mu.Lock()
	defer host.host.mu.Unlock()
	if len(host.host.commands) != 1 {
		t.Fatal("fixture command count changed")
	}
	for _, receipt := range host.host.commands {
		if receipt.State != "complete" {
			t.Fatal("fixture receipt incomplete")
		}
	}
	sawStatus := false
	for _, m := range result.Messages {
		for _, c := range m.ToolCalls {
			if strings.HasSuffix(c.Function.Name, "__editor_status") {
				sawStatus = true
				var q struct {
					CommandID string `json:"commandId"`
				}
				json.Unmarshal([]byte(c.Function.Arguments), &q)
				if host.host.commands[q.CommandID] == nil {
					t.Fatal("model looked up invented receipt")
				}
			}
		}
	}
	if !sawStatus {
		t.Fatal("model did not check actual receipt")
	}
	t.Logf("model=gpt-4o-mini variant=%s unseeded_success=true simulated_executions=1 receipt=complete status_linkage=true", variant)
}

type editorFixtureDiagnostics struct{ t *testing.T }

func (d editorFixtureDiagnostics) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		raw, _ := io.ReadAll(io.LimitReader(req.Body, 1<<20))
		req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(raw))
		var body map[string]json.RawMessage
		if json.Unmarshal(raw, &body) == nil && len(body["tools"]) < 65536 {
			_ = os.WriteFile("/tmp/iugum-editor-unseeded-native-definitions-"+os.Getenv("IUGUM_EDITOR_PROBE_VARIANT")+os.Getenv("IUGUM_EDITOR_PROBE_CATALOG_LABEL")+".json", body["tools"], 0600)
		}
	}
	resp, e := http.DefaultTransport.RoundTrip(req)
	if e != nil {
		return resp, e
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(raw))
		var payload struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Param   string `json:"param"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &payload) == nil {
			message := payload.Error.Message
			if len(message) > 2000 {
				message = message[:2000]
			}
			d.t.Logf("fixture provider error type=%s param=%s message=%s", payload.Error.Type, payload.Error.Param, message)
		}
	}
	return resp, nil
}

func editorProbeJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func editorProbeShape(name string, raw json.RawMessage) map[string]any {
	out := map[string]any{"tool": name}
	var input map[string]any
	if json.Unmarshal(raw, &input) != nil || input == nil {
		out["shape"] = "not_object"
		return out
	}
	known := map[string]bool{"instance": true, "commandId": true, "operation": true, "args": true}
	fields := map[string]any{}
	unknown := 0
	for key, value := range input {
		if !known[key] {
			unknown++
			continue
		}
		kind := "other"
		switch value.(type) {
		case string:
			kind = "string"
		case map[string]any:
			kind = "object"
		case []any:
			kind = "array"
		case float64:
			kind = "number"
		case bool:
			kind = "boolean"
		case nil:
			kind = "null"
		}
		field := map[string]any{"type": kind}
		if key == "instance" || key == "commandId" {
			v, _ := value.(string)
			field["uuid_valid"] = uuid.MatchString(v)
		}
		if key == "operation" {
			v, _ := value.(string)
			field["supported"] = hostOperation(v)
		}
		fields[key] = field
	}
	out["fields"] = fields
	out["unknown_field_count"] = unknown
	if args, ok := input["args"].(map[string]any); ok {
		names := []string{}
		other := 0
		for key := range args {
			if known[key] || key == "view" || key == "search" || key == "projects" || key == "statuses" || key == "priorities" || key == "types" || key == "assignees" || key == "labels" || key == "sort" || key == "lanes" || key == "ticket" || key == "ticketProject" {
				names = append(names, key)
			} else {
				other++
			}
		}
		out["args_known_fields"] = names
		out["args_unknown_field_count"] = other
	}
	return out
}
