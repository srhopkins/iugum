package agentplatform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/srhopkins/iugum/agentmcp"
)

func TestFFAIInventoryParse(t *testing.T) {
	v := parseFFAIInventory("host\nabc\nz\\t%2\\tbash\\t/config/workspace\na\\t%1\\tclaude\\t/config/projects/FutureFit-ai\n", "", nil)
	if v.Status != "ok" || v.Hostname != "host" || v.User != "abc" || len(v.Workers) != 2 || v.Workers[0].Session != "a" || !v.Workers[0].ClaudeProcess || v.Workers[0].Managed {
		t.Fatalf("unexpected inventory: %+v", v)
	}
	for _, tt := range []struct {
		out, stderr, status string
		err                 error
	}{
		{"host\nroot\n", "private diagnostic", "unavailable", nil},
		{"", "private diagnostic", "unavailable", errors.New("docker failed")},
		{"host\nabc\n", "no server running on /tmp/tmux-1000/default", "empty", errors.New("exit 1")},
		{"host\nabc\n", "error connecting to /tmp/tmux-1000/default (No such file or directory)", "empty", errors.New("exit 1")},
		{"host\nabc\n", "private diagnostic", "tmux_unavailable", errors.New("exit 1")},
		{"host\nabc\nmalformed\n", "", "invalid_inventory", nil},
		{"host\nabc\n", "", "empty", nil},
	} {
		v := parseFFAIInventory(tt.out, tt.stderr, tt.err)
		if v.Status != tt.status || strings.Contains(v.Message, "private diagnostic") {
			t.Fatalf("%s: %+v", tt.status, v)
		}
	}
	v = parseFFAIInventory("host\nabc\n"+strings.Repeat("a\\t%1\\tclaude\\t/config\n", 65), "", nil)
	if v.Status != "invalid_inventory" || len(v.Workers) != 0 {
		t.Fatal(v)
	}
}
func TestFFAIFixedExecution(t *testing.T) {
	expected := []string{"exec", "-u", "abc", "--env", "BASH_ENV=", "--env", "ENV=", "ffai-agent", "/bin/bash", "--noprofile", "--norc", "-c", ffaiInventoryScript}
	if !reflect.DeepEqual(ffaiDockerArgs(), expected) {
		t.Fatal(ffaiDockerArgs())
	}
	var output ffaiOutput
	if _, err := output.Write(make([]byte, 16*1024+1)); err == nil || !output.overflow || output.Len() != 0 {
		t.Fatal("output cap failed")
	}
}
func TestFFAIWorkersSharedMCP(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\nprintf 'fixture-host\\nabc\\nmanager\\\\t%%1\\\\tclaude\\\\t/config/workspace\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	s, err := New(context.Background(), Config{DataDir: t.TempDir(), HostTools: true, Check: func(context.Context, string, string) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	t.Setenv("IUGUM_FFAI_TEST_TOKEN", s.token)
	tools, closeClient, err := agentmcp.Connect(context.Background(), []agentmcp.Config{{Name: "editor", URL: server.URL + "/api/host/mcp", AuthEnv: "IUGUM_FFAI_TEST_TOKEN"}}, "fixture", hostFixtureGate{})
	if err != nil {
		t.Fatal(err)
	}
	defer closeClient()
	for _, tool := range tools {
		if tool.Name == "mcp_editor__ffai_workers" {
			result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(result, "fixture-host") || !strings.Contains(result, "claude_process") {
				t.Fatal(result)
			}
			return
		}
	}
	t.Fatal("FFAI tool missing from shared MCP")
}
