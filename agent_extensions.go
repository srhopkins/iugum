package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/srhopkins/iugum/agentclones"
	"github.com/srhopkins/iugum/agentcontrol"
	"github.com/srhopkins/iugum/agentcore"
	"github.com/srhopkins/iugum/agentdesk"
	"github.com/srhopkins/iugum/agentmcp"
	"github.com/srhopkins/iugum/contract"
	"gopkg.in/yaml.v3"
)

func buildAgentExtensions(lifetime context.Context, c nativeAgentConfig, gate contract.Policy, desk *agentdesk.Server) ([]agentcore.Tool, func(context.Context, agentdesk.Approval) (string, error), error) {
	actor := "agent:" + c.Name
	check := func(ctx context.Context, obj, act string) error {
		return gate.Enforce(ctx, contract.Request{Sub: actor, Obj: obj, Act: act})
	}
	manager, err := agentcontrol.New(filepath.Join(c.DataDir, "claude-control"), gate)
	if err != nil {
		return nil, nil, err
	}
	bridge := newAgentControlBridge(lifetime, manager, actor, c.ClaudeAccounts, check, desk.RequestApproval)
	tools := bridge.Tools()
	skillTools, skillErr := pluginSkillTools(lifetime, c, check)
	if skillErr != nil {
		return nil, nil, skillErr
	}
	tools = append(tools, skillTools...)
	mcpTools, closeMCP, e := agentmcp.Connect(lifetime, c.MCP, actor, gate)
	if e != nil {
		return nil, nil, e
	}
	go func() { <-lifetime.Done(); closeMCP() }()
	tools = append(tools, mcpTools...)
	root := filepath.Join(c.DataDir, "clones")
	promptTools, promptExecute := buildClonePromptTools(root, c.InstructionsFile, check, desk.RequestApproval)
	tools = append(tools, promptTools...)
	candidate := func(name string) (string, error) {
		if !validNativeCloneName(name) {
			return "", fmt.Errorf("invalid clone name")
		}
		return filepath.Join(root, name), nil
	}
	schema := func(props map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": required}
	}
	str := map[string]string{"type": "string"}
	add := func(name, desc string, params map[string]any, fn func(context.Context, json.RawMessage) (string, error)) {
		tools = append(tools, agentcore.Tool{Name: name, Description: desc, Parameters: params, Execute: func(ctx context.Context, b json.RawMessage) (string, error) {
			if e := check(ctx, "clone:"+c.Name, name); e != nil {
				return "", e
			}
			return fn(ctx, b)
		}})
	}
	add("clone_create", "Create an isolated named candidate configuration and code worktree including current uncommitted source. Parent remains unchanged. No automatic promotion.", schema(map[string]any{"name": str}, "name"), func(ctx context.Context, b json.RawMessage) (string, error) {
		var q struct{ Name string }
		if e := json.Unmarshal(b, &q); e != nil {
			return "", e
		}
		dest, e := candidate(q.Name)
		if e != nil {
			return "", e
		}
		if c.CodeRepo == "" {
			return "", fmt.Errorf("code_repo must be configured for candidate development")
		}
		if e = check(ctx, "project:"+c.CodeRepo, "read"); e != nil {
			return "", e
		}
		if e = os.MkdirAll(root, 0700); e != nil {
			return "", e
		}
		p, e := createNativeAgentClone(ctx, c.ConfigPath, q.Name, dest)
		if e != nil {
			return "", e
		}
		_, e = agentclones.SnapshotWorktree(ctx, c.CodeRepo, filepath.Join(dest, "source"))
		if e != nil {
			return "", e
		}
		_, e = agentclones.SnapshotMemory(ctx, filepath.Join(c.DataDir, "memory.db"), filepath.Join(dest, "data", "memory.db"), c.Name, q.Name, check)
		if e != nil {
			return "", e
		}
		raw, e := os.ReadFile(p)
		if e != nil {
			return "", e
		}
		var cfg map[string]any
		if e = yaml.Unmarshal(raw, &cfg); e != nil {
			return "", e
		}
		cfg["code_repo"] = filepath.Join(dest, "source")
		cfg["clone"] = map[string]any{"parent_config": c.ConfigPath, "parent_name": c.Name, "promotion": "manual_only", "memory_mode": "snapshot", "memory_snapshot": filepath.Join(dest, "data", "memory-snapshot.json"), "code_worktree": filepath.Join(dest, "source")}
		raw, e = yaml.Marshal(cfg)
		if e != nil {
			return "", e
		}
		if e = writeAgentState(p, raw); e != nil {
			return "", e
		}
		return p, nil
	})
	add("clone_read", "Read a relative file from the named candidate source. Symlinks and parent paths are refused.", schema(map[string]any{"name": str, "path": str}, "name", "path"), func(ctx context.Context, b json.RawMessage) (string, error) {
		var q struct{ Name, Path string }
		if e := json.Unmarshal(b, &q); e != nil {
			return "", e
		}
		dest, e := candidate(q.Name)
		if e != nil {
			return "", e
		}
		v, e := agentclones.Read(filepath.Join(dest, "source"), q.Path)
		return string(v), e
	})
	add("clone_write", "Write a complete source file in a named candidate only. This invalidates any previous evaluation.", schema(map[string]any{"name": str, "path": str, "content": str}, "name", "path", "content"), func(ctx context.Context, b json.RawMessage) (string, error) {
		var q struct{ Name, Path, Content string }
		if e := json.Unmarshal(b, &q); e != nil {
			return "", e
		}
		dest, e := candidate(q.Name)
		if e != nil {
			return "", e
		}
		e = agentclones.Write(filepath.Join(dest, "source"), q.Path, []byte(q.Content))
		return "Candidate file saved; evaluate before requesting promotion.", e
	})
	add("clone_evaluate", "Run bounded Go tests in the candidate; records a content-bound result. Candidate test code executes in this policy-authorized development environment.", schema(map[string]any{"name": str, "packages": map[string]any{"type": "array", "items": str}}, "name"), func(ctx context.Context, b json.RawMessage) (string, error) {
		var q struct {
			Name     string
			Packages []string
		}
		if e := json.Unmarshal(b, &q); e != nil {
			return "", e
		}
		dest, e := candidate(q.Name)
		if e != nil {
			return "", e
		}
		if len(q.Packages) == 0 {
			q.Packages = []string{"./..."}
		}
		for _, p := range q.Packages {
			if !strings.HasPrefix(p, "./") || strings.Contains(p, "..") && p != "./..." {
				return "", fmt.Errorf("packages cannot contain flags")
			}
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		v, e := agentclones.Evaluate(ctx, filepath.Join(dest, "source"), append([]string{"test"}, q.Packages...))
		r, _ := json.Marshal(v)
		return string(r), e
	})
	add("clone_request_promotion", "Request human review of a passing candidate. Never applies changes automatically.", schema(map[string]any{"name": str}, "name"), func(ctx context.Context, b json.RawMessage) (string, error) {
		var q struct{ Name string }
		if e := json.Unmarshal(b, &q); e != nil {
			return "", e
		}
		dest, e := candidate(q.Name)
		if e != nil {
			return "", e
		}
		source := filepath.Join(dest, "source")
		m, paths, e := agentclones.PromotionPlan(ctx, source)
		if e != nil {
			return "", e
		}
		if len(paths) == 0 {
			return "No candidate changes to promote.", nil
		}
		evidence := []agentdesk.Evidence{{Label: "Passing evaluation", Path: filepath.Join(dest, "evaluation.json")}, {Label: "Parent baseline", Path: filepath.Join(dest, "source-manifest.json")}}
		for _, p := range paths {
			v, e := agentclones.Read(source, p)
			if e != nil && !os.IsNotExist(e) {
				return "", e
			}
			evidence = append(evidence, agentdesk.Evidence{Label: p, Text: string(v)})
		}
		payload, _ := json.Marshal(map[string]string{"name": q.Name, "parent": m.Parent})
		a, e := desk.RequestApproval(ctx, agentdesk.ApprovalRequest{Title: "Promote candidate " + q.Name, Action: "clone_promote", Payload: payload, Evidence: evidence})
		v, _ := json.Marshal(a)
		return string(v), e
	})
	execute := func(ctx context.Context, a agentdesk.Approval) (string, error) {
		if a.Status != "executing" {
			return "", fmt.Errorf("approval must be executing")
		}
		if a.Action == "clone_prompt_promote" {
			return promptExecute(ctx, a)
		}
		if a.Action != "clone_promote" {
			return bridge.ExecuteApproval(ctx, a)
		}
		var q struct{ Name, Parent string }
		if e := json.Unmarshal(a.Payload, &q); e != nil {
			return "", e
		}
		if e := check(ctx, "project:"+q.Parent, "write"); e != nil {
			return "", e
		}
		dest, e := candidate(q.Name)
		if e != nil {
			return "", e
		}
		paths, e := agentclones.Promote(ctx, filepath.Join(dest, "source"), q.Parent)
		if e != nil {
			return "", e
		}
		return fmt.Sprintf("Promoted %d files. Backup saved in the candidate directory. Running service remains on its tested binary until rebuilt.", len(paths)), nil
	}
	return tools, execute, nil
}
