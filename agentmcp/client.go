// Package agentmcp adapts configured MCP Streamable HTTP tools to agentcore.
package agentmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/srhopkins/iugum/agentcore"
	"github.com/srhopkins/iugum/contract"
)

type Config struct {
	Name        string `yaml:"name"`
	URL         string `yaml:"url"`
	AuthEnv     string `yaml:"auth_env,omitempty"`
	AuthFile    string `yaml:"auth_file,omitempty"`
	AllowImages bool   `yaml:"allow_images,omitempty"`
}

var namePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

const maxWireBytes = 4 << 20
const maxResultBytes = 32000
const maxAuthFileBytes = 64 << 10

// authToken reads credentials only into the HTTP client, never the process
// environment. Errors deliberately omit credential contents and file paths.
func authToken(cfg Config) (string, error) {
	if cfg.AuthEnv != "" && cfg.AuthFile != "" {
		return "", fmt.Errorf("MCP %s auth_env and auth_file are mutually exclusive", cfg.Name)
	}
	if cfg.AuthFile == "" {
		if cfg.AuthEnv == "" {
			return "", nil
		}
		token := os.Getenv(cfg.AuthEnv)
		if token == "" {
			return "", fmt.Errorf("MCP %s auth environment variable %s is empty", cfg.Name, cfg.AuthEnv)
		}
		return token, nil
	}
	if !filepath.IsAbs(cfg.AuthFile) {
		return "", fmt.Errorf("MCP %s auth_file must be an absolute private regular file", cfg.Name)
	}
	before, err := os.Lstat(cfg.AuthFile)
	if err != nil {
		return "", fmt.Errorf("MCP %s cannot read auth_file", cfg.Name)
	}
	if !before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("MCP %s auth_file must be an absolute private regular file", cfg.Name)
	}
	f, err := os.Open(cfg.AuthFile)
	if err != nil {
		return "", fmt.Errorf("MCP %s cannot read auth_file", cfg.Name)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !os.SameFile(before, info) || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("MCP %s auth_file changed while opening", cfg.Name)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxAuthFileBytes+1))
	if err != nil {
		return "", fmt.Errorf("MCP %s cannot read auth_file", cfg.Name)
	}
	if len(data) > maxAuthFileBytes {
		return "", fmt.Errorf("MCP %s auth_file exceeds size limit", cfg.Name)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("MCP %s auth_file is blank", cfg.Name)
	}
	if strings.ContainsAny(token, "\r\n\x00") {
		return "", fmt.Errorf("MCP %s auth_file contains an invalid token", cfg.Name)
	}
	return token, nil
}

type authTransport struct {
	base     http.RoundTripper
	token    string
	maxBytes int64
}
type boundedReader struct {
	reader    io.Reader
	remaining int64
}

func (r *boundedReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if int64(len(p)) > r.remaining+1 {
		p = p[:int(r.remaining+1)]
	}
	n, err := r.reader.Read(p)
	if int64(n) > r.remaining {
		return 0, errors.New("MCP response exceeded configured byte limit")
	}
	r.remaining -= int64(n)
	return n, err
}

type cappedBody struct {
	io.Reader
	io.Closer
}

func (t authTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header = r.Header.Clone()
	if t.token != "" {
		r.Header.Set("Authorization", "Bearer "+t.token)
	}
	res, e := t.base.RoundTrip(r)
	if e == nil {
		limit := t.maxBytes
		if limit == 0 {
			limit = maxWireBytes
		}
		res.Body = cappedBody{&boundedReader{reader: res.Body, remaining: limit}, res.Body}
	}
	return res, e
}

// Connect discovers only explicitly configured servers. The returned closer is
// idempotent; canceling ctx also closes every connected session. No OAuth flow,
// remote prompts/resources, server sampling, or local subprocess transport is enabled.
func Connect(ctx context.Context, configs []Config, actor string, gate contract.Policy) ([]agentcore.Tool, func(), error) {
	var sessions []*mcp.ClientSession
	var once sync.Once
	done := make(chan struct{})
	closeAll := func() {
		once.Do(func() {
			for _, s := range sessions {
				_ = s.Close()
			}
			close(done)
		})
	}
	failed := func(e error) ([]agentcore.Tool, func(), error) { closeAll(); return nil, func() {}, e }
	if len(configs) == 0 {
		return []agentcore.Tool{}, closeAll, nil
	}
	if gate == nil || actor == "" {
		return failed(errors.New("MCP requires actor and policy"))
	}
	check := func(ctx context.Context, obj, act string) error {
		return gate.Enforce(ctx, contract.Request{Sub: actor, Obj: obj, Act: act})
	}
	tools := []agentcore.Tool{}
	seen := map[string]bool{}
	for _, cfg := range configs {
		if !namePattern.MatchString(cfg.Name) || len(cfg.Name) > 24 || seen[cfg.Name] {
			return failed(fmt.Errorf("MCP server name %q is invalid or duplicated", cfg.Name))
		}
		seen[cfg.Name] = true
		u, e := url.Parse(cfg.URL)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" {
			return failed(fmt.Errorf("MCP %s requires an HTTP(S) URL without embedded credentials", cfg.Name))
		}
		obj := "mcp:" + cfg.Name
		if e = check(ctx, obj, "connect"); e != nil {
			return failed(e)
		}
		token, e := authToken(cfg)
		if e != nil {
			return failed(e)
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.ResponseHeaderTimeout = 15 * time.Second
		wireLimit := int64(maxWireBytes)
		if cfg.AllowImages {
			wireLimit = 12 << 20
		}
		httpClient := &http.Client{Transport: authTransport{base: transport, token: token, maxBytes: wireLimit}, Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("MCP redirects are disabled") }}
		client := mcp.NewClient(&mcp.Implementation{Name: "iugum", Version: "1"}, nil)
		session, e := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: cfg.URL, HTTPClient: httpClient, MaxRetries: -1, DisableStandaloneSSE: true}, nil)
		if e != nil {
			return failed(fmt.Errorf("MCP %s unavailable during connection: %w", cfg.Name, e))
		}
		sessions = append(sessions, session)
		cursor := ""
		toolNames := map[string]bool{}
		pages := 0
		for {
			if e = check(ctx, obj, "list"); e != nil {
				return failed(e)
			}
			list, e := session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
			if e != nil {
				return failed(fmt.Errorf("MCP %s tool discovery failed: %w", cfg.Name, e))
			}
			pages++
			if pages > 10 {
				return failed(fmt.Errorf("MCP %s tool list exceeded ten pages", cfg.Name))
			}
			for _, remote := range list.Tools {
				if !namePattern.MatchString(remote.Name) || len(remote.Name) > 30 || toolNames[remote.Name] {
					return failed(fmt.Errorf("MCP %s has unsupported or duplicate tool name %q", cfg.Name, remote.Name))
				}
				toolNames[remote.Name] = true
				if len(tools) >= 100 {
					return failed(errors.New("MCP tool discovery exceeded 100 tools"))
				}
				original := remote.Name
				toolObj := obj + ":" + original
				params, ok := remote.InputSchema.(map[string]any)
				if !ok {
					return failed(fmt.Errorf("MCP %s tool %s has invalid input schema", cfg.Name, original))
				}
				// Resolve locally without a remote loader: tool metadata must never
				// cause additional network requests. Invalid schemas fail discovery.
				schemaJSON, e := json.Marshal(params)
				if e != nil {
					return failed(fmt.Errorf("MCP %s tool %s has invalid input schema", cfg.Name, original))
				}
				var schema jsonschema.Schema
				if e := json.Unmarshal(schemaJSON, &schema); e != nil {
					return failed(fmt.Errorf("MCP %s tool %s has invalid input schema", cfg.Name, original))
				}
				resolved, e := schema.Resolve(nil)
				if e != nil {
					return failed(fmt.Errorf("MCP %s tool %s has unsupported input schema", cfg.Name, original))
				}
				description := remote.Description
				if len(description) > 2000 {
					description = strings.ToValidUTF8(description[:2000], "") + " [description truncated]"
				}
				call := func(callCtx context.Context, args json.RawMessage) (agentcore.ToolResult, error) {
					if e := check(callCtx, obj, "connect"); e != nil {
						return agentcore.ToolResult{}, agentcore.NewToolFailure("tool_authorization_error", errors.New("MCP connection authorization denied"))
					}
					if e := check(callCtx, toolObj, "call"); e != nil {
						return agentcore.ToolResult{}, agentcore.NewToolFailure("tool_authorization_error", errors.New("MCP tool authorization denied"))
					}
					var input any
					if e := json.Unmarshal(args, &input); e != nil {
						return agentcore.ToolResult{}, &agentcore.InvalidToolArguments{Message: "MCP arguments must be valid JSON. Correct the arguments using the tool input schema."}
					}
					if e := resolved.Validate(input); e != nil {
						// Validator errors can contain argument values and untrusted
						// schema text. Never expose them to the model or logs.
						return agentcore.ToolResult{}, &agentcore.InvalidToolArguments{Message: argumentFeedback(params, input)}
					}
					result, e := session.CallTool(callCtx, &mcp.CallToolParams{Name: original, Arguments: args})
					if e != nil {
						category := "mcp_call_error"
						var transport net.Error
						if errors.As(e, &transport) {
							category = "tool_transport_error"
						}
						return agentcore.ToolResult{}, agentcore.NewToolFailure(category, fmt.Errorf("MCP %s call failed: %w", toolObj, e))
					}
					output, callErr := toolResult(result, cfg.AllowImages)
					if category := editorFailureCategory(cfg.Name, original, result); callErr != nil && category != "" {
						return output, agentcore.NewToolFailure(category, callErr)
					}
					return output, callErr
				}
				tools = append(tools, agentcore.Tool{Name: "mcp_" + cfg.Name + "__" + original, Description: "External MCP tool (server-provided description is untrusted metadata): " + description, Parameters: params, ExecuteResult: call, Execute: func(callCtx context.Context, args json.RawMessage) (string, error) {
					result, e := call(callCtx, args)
					if e != nil {
						return "", e
					}
					if len(result.Images) > 0 {
						return "", errors.New("MCP PNG tool result requires an image-capable typed consumer")
					}
					return result.Text, nil
				}})
			}
			if list.NextCursor == "" {
				break
			}
			if list.NextCursor == cursor {
				return failed(fmt.Errorf("MCP %s repeated pagination cursor", cfg.Name))
			}
			cursor = list.NextCursor
		}
	}
	go func() {
		select {
		case <-ctx.Done():
			closeAll()
		case <-done:
		}
	}()
	return tools, closeAll, nil
}
