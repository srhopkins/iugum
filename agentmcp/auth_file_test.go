package agentmcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAuthFileHTTPAndNoEnvironmentExport(t *testing.T) {
	const secret = "synthetic-private-bearer-token"
	const envKey = "IUGUM_TEST_AUTH_FILE_MUST_NOT_EXPORT"
	t.Setenv(envKey, "")
	file := filepath.Join(t.TempDir(), "control-token")
	if err := os.WriteFile(file, []byte("  "+secret+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	var authorized atomic.Int64
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authorized.Add(1)
		handler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tools, closeClient, err := Connect(ctx, []Config{{Name: "editor", URL: httpServer.URL, AuthFile: file}}, "agent:test", &testGate{})
	if err != nil {
		t.Fatal(err)
	}
	defer closeClient()
	if len(tools) != 1 {
		t.Fatal("missing authenticated tool")
	}
	if _, err = tools[0].Execute(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if authorized.Load() < 2 {
		t.Fatal("discovery/call not authenticated")
	}
	for _, entry := range os.Environ() {
		if strings.Contains(entry, secret) {
			t.Fatal("credential was exported")
		}
	}
	output, err := exec.Command("/bin/sh", "-c", "env").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(output), secret) {
		t.Fatal("credential inherited by child process")
	}
}

func TestAuthFileValidationAndRedaction(t *testing.T) {
	const secret = "synthetic-secret-do-not-print"
	dir := t.TempDir()
	valid := filepath.Join(dir, "private-token")
	if err := os.WriteFile(valid, []byte(secret), 0600); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string, mode os.FileMode) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		return p
	}
	link := filepath.Join(dir, "symlink-token")
	if err := os.Symlink(valid, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IUGUM_TEST_AUTH_ENV", secret)
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{"conflict", Config{AuthEnv: "IUGUM_TEST_AUTH_ENV", AuthFile: valid}, "mutually exclusive"},
		{"relative", Config{AuthFile: "relative-token"}, "absolute private regular"},
		{"missing", Config{AuthFile: filepath.Join(dir, "missing-"+secret)}, "cannot read auth_file"},
		{"directory", Config{AuthFile: dir}, "absolute private regular"},
		{"symlink", Config{AuthFile: link}, "absolute private regular"},
		{"public", Config{AuthFile: write("public", secret, 0644)}, "absolute private regular"},
		{"blank", Config{AuthFile: write("blank", " \n\t", 0600)}, "is blank"},
		{"oversize", Config{AuthFile: write("large", strings.Repeat("x", maxAuthFileBytes+1), 0600)}, "exceeds size limit"},
		{"invalid", Config{AuthFile: write("invalid", secret+"\nsecond-line", 0600)}, "invalid token"},
	}
	if os.Geteuid() != 0 {
		tests = append(tests, struct {
			name string
			cfg  Config
			want string
		}{"unreadable", Config{AuthFile: write("unreadable", secret, 0000)}, "cannot read auth_file"})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.cfg.Name = "editor"
			token, err := authToken(tt.cfg)
			if token != "" || err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatal("expected sanitized auth_file validation error")
			}
			if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), dir) {
				t.Fatal("credential or path exposed")
			}
		})
	}
}

func TestAuthEnvCompatibility(t *testing.T) {
	t.Setenv("IUGUM_TEST_EXISTING_AUTH_ENV", " existing-env-token ")
	token, err := authToken(Config{Name: "test", AuthEnv: "IUGUM_TEST_EXISTING_AUTH_ENV"})
	if err != nil || token != " existing-env-token " {
		t.Fatal("auth_env behavior changed")
	}
	t.Setenv("IUGUM_TEST_EXISTING_AUTH_ENV", "")
	if _, err = authToken(Config{Name: "test", AuthEnv: "IUGUM_TEST_EXISTING_AUTH_ENV"}); err == nil {
		t.Fatal("missing auth_env accepted")
	}
	if token, err = authToken(Config{Name: "test"}); err != nil || token != "" {
		t.Fatal("unauthenticated source behavior changed")
	}
}

func TestInvalidAuthFileDoesNotContactServer(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	for _, cfg := range []Config{
		{Name: "editor", URL: server.URL, AuthFile: filepath.Join(t.TempDir(), "missing")},
		{Name: "editor", URL: server.URL, AuthFile: "/unused", AuthEnv: "CONFLICT"},
	} {
		_, closeClient, err := Connect(context.Background(), []Config{cfg}, "agent:test", &testGate{})
		closeClient()
		if err == nil || calls.Load() != 0 {
			t.Fatal("invalid credentials contacted server")
		}
	}
}
