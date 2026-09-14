// Package agentacp connects configured stdio agents to the reusable wiki chat.
package agentacp

import (
	"context"
	"encoding/json"
	"fmt"
	acp "github.com/coder/acp-go-sdk"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Name       string            `yaml:"name"`
	Command    []string          `yaml:"command"`
	Cwd        string            `yaml:"cwd"`
	Model      string            `yaml:"model"`
	Env        map[string]string `yaml:"env"`
	AllowTools bool              `yaml:"allow_tools"`
}
type Client struct {
	Config   Config
	StateDir string
	Check    func(context.Context, string, string) error
	mu       sync.Mutex
	outputMu sync.Mutex
	output   strings.Builder
	loading  bool
}

func (c *Client) Chat(ctx context.Context, text string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.Config.Command) == 0 {
		return "", fmt.Errorf("ACP command is not configured")
	}
	if c.Check == nil {
		return "", fmt.Errorf("ACP requires a policy checker")
	}
	if err := c.Check(ctx, "run", "call"); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Config.Command[0], c.Config.Command[1:]...)
	cmd.Dir = c.Config.Cwd
	cmd.Env = os.Environ()
	for k, v := range c.Config.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err = cmd.Start(); err != nil {
		return "", err
	}
	defer func() { in.Close(); cmd.Process.Kill(); cmd.Wait() }()
	conn := acp.NewClientSideConnection(c, in, out)
	init, err := conn.Initialize(ctx, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber, ClientCapabilities: acp.ClientCapabilities{}})
	if err != nil {
		return "", err
	}
	path := filepath.Join(c.StateDir, "session.json")
	var session acp.SessionId
	b, readErr := os.ReadFile(path)
	if readErr == nil {
		if err = json.Unmarshal(b, &session); err != nil {
			return "", err
		}
	} else if !os.IsNotExist(readErr) {
		return "", readErr
	}
	c.outputMu.Lock()
	c.output.Reset()
	c.loading = true
	c.outputMu.Unlock()
	if session != "" {
		if !init.AgentCapabilities.LoadSession {
			return "", fmt.Errorf("ACP agent cannot reload its saved session; use a load-session capable agent")
		}
		_, err = conn.LoadSession(ctx, acp.LoadSessionRequest{SessionId: session, Cwd: c.Config.Cwd, McpServers: []acp.McpServer{}})
	} else {
		var s acp.NewSessionResponse
		s, err = conn.NewSession(ctx, acp.NewSessionRequest{Cwd: c.Config.Cwd, McpServers: []acp.McpServer{}})
		session = s.SessionId
	}
	if err != nil {
		return "", err
	}
	if c.Config.Model != "" {
		if err = c.Check(ctx, "model/"+c.Config.Model, "call"); err != nil {
			return "", err
		}
		b, _ := json.Marshal(map[string]any{"sessionId": session, "configId": "model", "value": c.Config.Model})
		var req acp.SetSessionConfigOptionRequest
		_ = json.Unmarshal(b, &req)
		if _, err = conn.SetSessionConfigOption(ctx, req); err != nil {
			return "", fmt.Errorf("select ACP model: %w", err)
		}
	}
	if err = os.MkdirAll(c.StateDir, 0700); err != nil {
		return "", err
	}
	b, _ = json.Marshal(session)
	if err = writeState(path, b); err != nil {
		return "", err
	}
	c.outputMu.Lock()
	c.loading = false
	c.outputMu.Unlock()
	result, err := conn.Prompt(ctx, acp.PromptRequest{SessionId: session, Prompt: []acp.ContentBlock{acp.TextBlock("Current time: " + time.Now().Format(time.RFC3339) + "\n\n" + text)}})
	if err != nil {
		return "", err
	}
	usage, usageErr := os.OpenFile(filepath.Join(c.StateDir, "usage.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if usageErr != nil {
		return "", usageErr
	}
	usageErr = json.NewEncoder(usage).Encode(map[string]any{"at": time.Now(), "model": c.Config.Model, "usage": result.Usage, "stop_reason": result.StopReason})
	closeErr := usage.Close()
	if usageErr != nil {
		return "", usageErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	c.outputMu.Lock()
	defer c.outputMu.Unlock()
	answer := c.output.String()
	if answer == "" {
		return "", fmt.Errorf("ACP turn ended with %s but returned no answer", result.StopReason)
	}
	return answer, nil
}
func (c *Client) SessionUpdate(_ context.Context, n acp.SessionNotification) error {
	c.outputMu.Lock()
	defer c.outputMu.Unlock()
	if !c.loading && n.Update.AgentMessageChunk != nil && n.Update.AgentMessageChunk.Content.Text != nil {
		if c.output.Len() > 2<<20 {
			return fmt.Errorf("ACP response exceeded limit")
		}
		c.output.WriteString(n.Update.AgentMessageChunk.Content.Text.Text)
	}
	return nil
}
func (c *Client) RequestPermission(ctx context.Context, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	title := "unknown"
	if p.ToolCall.Title != nil {
		title = *p.ToolCall.Title
	}
	if c.Config.AllowTools && c.Check != nil && c.Check(ctx, "tool/"+title, "call") == nil {
		for _, o := range p.Options {
			if string(o.Kind) == "allow_once" {
				return acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{Selected: &acp.RequestPermissionOutcomeSelected{OptionId: o.OptionId}}}, nil
			}
		}
	}
	return acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{Cancelled: &acp.RequestPermissionOutcomeCancelled{}}}, nil
}

func (c *Client) ReadTextFile(context.Context, acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, fmt.Errorf("client capability not enabled")
}

func (c *Client) WriteTextFile(context.Context, acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, fmt.Errorf("client capability not enabled")
}

func (c *Client) CreateTerminal(context.Context, acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, fmt.Errorf("client capability not enabled")
}

func (c *Client) KillTerminal(context.Context, acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, fmt.Errorf("client capability not enabled")
}

func (c *Client) TerminalOutput(context.Context, acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, fmt.Errorf("client capability not enabled")
}

func (c *Client) ReleaseTerminal(context.Context, acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, fmt.Errorf("client capability not enabled")
}

func (c *Client) WaitForTerminalExit(context.Context, acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, fmt.Errorf("client capability not enabled")
}

func writeState(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".session-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
