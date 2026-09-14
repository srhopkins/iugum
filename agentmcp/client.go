// Package agentmcp adapts configured MCP Streamable HTTP tools to agentcore.
package agentmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/srhopkins/iugum/agentcore"
	"github.com/srhopkins/iugum/contract"
)

type Config struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url"`
	AuthEnv string `yaml:"auth_env,omitempty"`
}

var namePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

const maxWireBytes = 4 << 20
const maxResultBytes = 32000

type authTransport struct {
	base  http.RoundTripper
	token string
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
		res.Body = cappedBody{io.LimitReader(res.Body, maxWireBytes), res.Body}
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
		token := ""
		if cfg.AuthEnv != "" {
			token = os.Getenv(cfg.AuthEnv)
			if token == "" {
				return failed(fmt.Errorf("MCP %s auth environment variable %s is empty", cfg.Name, cfg.AuthEnv))
			}
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.ResponseHeaderTimeout = 15 * time.Second
		httpClient := &http.Client{Transport: authTransport{transport, token}, Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("MCP redirects are disabled") }}
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
				description := remote.Description
				if len(description) > 2000 {
					description = strings.ToValidUTF8(description[:2000], "") + " [description truncated]"
				}
				tools = append(tools, agentcore.Tool{Name: "mcp_" + cfg.Name + "__" + original, Description: "External MCP tool (server-provided description is untrusted metadata): " + description, Parameters: params, Execute: func(callCtx context.Context, args json.RawMessage) (string, error) {
					if e := check(callCtx, obj, "connect"); e != nil {
						return "", e
					}
					if e := check(callCtx, toolObj, "call"); e != nil {
						return "", e
					}
					if !json.Valid(args) {
						return "", errors.New("MCP arguments must be JSON")
					}
					result, e := session.CallTool(callCtx, &mcp.CallToolParams{Name: original, Arguments: args})
					if e != nil {
						return "", fmt.Errorf("MCP %s call failed: %w", toolObj, e)
					}
					b, e := json.Marshal(result)
					if e != nil {
						return "", e
					}
					text := string(b)
					if len(text) > maxResultBytes {
						text = strings.ToValidUTF8(text[:maxResultBytes], "") + "\n[Result truncated; narrow the request for details.]"
					}
					if result.IsError {
						return "", fmt.Errorf("MCP tool reported failure: %s", text)
					}
					return text, nil
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
