package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/srhopkins/iugum/agentcore"
)

func skillToolFixture(t *testing.T, check func(context.Context, string, string) error) (agentcore.Tool, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "skills", "find-transcripts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(root, "plugin.json"):   `{"name":"steve-operations"}`,
		filepath.Join(dir, "SKILL.md"):       "---\nname: find-transcripts\ndescription: Find earlier conversations\n---\nRead references.txt for details.",
		filepath.Join(dir, "references.txt"): "supporting instructions",
	}
	for name, content := range files {
		if err := os.WriteFile(name, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if check == nil {
		check = func(context.Context, string, string) error { return nil }
	}
	tools, err := pluginSkillTools(context.Background(), nativeAgentConfig{Plugins: []string{root}}, check)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("got %d tools", len(tools))
	}
	return tools[0], dir
}

func TestSkillReadSchemaDistinguishesIdentifierAndPath(t *testing.T) {
	tool, _ := skillToolFixture(t, nil)
	props := tool.Parameters["properties"].(map[string]any)
	skill := props["skill"].(map[string]any)
	if !reflect.DeepEqual(skill["enum"], []string{"steve-operations/find-transcripts"}) {
		t.Fatalf("skill enum: %#v", skill)
	}
	path := props["path"].(map[string]any)
	if path["default"] != "SKILL.md" || !strings.Contains(path["description"].(string), "relative") {
		t.Fatalf("path schema: %#v", path)
	}
}

func TestSkillReadRecoversWithinSameTool(t *testing.T) {
	tool, dir := skillToolFixture(t, nil)
	if err := os.Symlink(filepath.Join(dir, "..", "..", "plugin.json"), filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "large"), make([]byte, (256<<10)+1), 0600); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, arguments, code string }{
		{"plugin used as skill", `{"skill":"steve-operations"}`, "unknown_skill"},
		{"unknown skill", `{"skill":"steve-operations/missing"}`, "unknown_skill"},
		{"plugin used as path", `{"skill":"steve-operations/find-transcripts","path":"steve-operations"}`, "invalid_path"},
		{"missing asset", `{"skill":"steve-operations/find-transcripts","path":"missing.txt"}`, "invalid_path"},
		{"absolute", `{"skill":"steve-operations/find-transcripts","path":"/etc/passwd"}`, "invalid_path"},
		{"traversal", `{"skill":"steve-operations/find-transcripts","path":"../../plugin.json"}`, "invalid_path"},
		{"symlink", `{"skill":"steve-operations/find-transcripts","path":"escape"}`, "invalid_path"},
		{"oversized", `{"skill":"steve-operations/find-transcripts","path":"large"}`, "invalid_path"},
		{"directory", `{"skill":"steve-operations/find-transcripts","path":"."}`, "invalid_path"},
		{"bad argument type", `{"skill":12}`, "invalid_arguments"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), json.RawMessage(tc.arguments))
			if err != nil {
				t.Fatalf("recoverable lookup aborted: %v", err)
			}
			var problem struct {
				IsError   bool     `json:"is_error"`
				Code      string   `json:"code"`
				Available []string `json:"available_skills"`
				Default   string   `json:"default_path"`
			}
			if err := json.Unmarshal([]byte(result), &problem); err != nil {
				t.Fatal(err)
			}
			if !problem.IsError || problem.Code != tc.code || len(problem.Available) != 1 || problem.Default != "SKILL.md" {
				t.Fatalf("unhelpful error: %s", result)
			}
			if strings.Contains(result, dir) {
				t.Fatalf("exposed filesystem root: %s", result)
			}
			corrected, err := tool.Execute(context.Background(), json.RawMessage(`{"skill":"steve-operations/find-transcripts"}`))
			if err != nil || !strings.Contains(corrected, "Read references.txt") {
				t.Fatalf("corrected read failed: %q %v", corrected, err)
			}
		})
	}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"skill":"steve-operations/find-transcripts","path":"references.txt"}`))
	var asset skillReadSuccess
	decodeErr := json.Unmarshal([]byte(result), &asset)
	if err != nil || decodeErr != nil || asset.Contents != "supporting instructions" || asset.Path != "references.txt" {
		t.Fatalf("asset read: %q %v", result, err)
	}
}

func TestSkillReadAuthorizationRemainsHardError(t *testing.T) {
	denied := errors.New("policy denied")
	tool, _ := skillToolFixture(t, func(_ context.Context, resource, action string) error {
		if strings.HasPrefix(resource, "skill:") {
			return denied
		}
		return nil
	})
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"skill":"steve-operations/find-transcripts"}`))
	if !errors.Is(err, denied) || result != "" {
		t.Fatalf("authorization softened: %q %v", result, err)
	}
	_, err = pluginSkillTools(context.Background(), nativeAgentConfig{Plugins: []string{t.TempDir()}}, func(context.Context, string, string) error { return denied })
	if !errors.Is(err, denied) {
		t.Fatalf("plugin authorization softened: %v", err)
	}
}

func TestSkillReadFilesystemPermissionRemainsHardError(t *testing.T) {
	tool, dir := skillToolFixture(t, nil)
	path := filepath.Join(dir, "references.txt")
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("process can bypass file permissions")
	}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"skill":"steve-operations/find-transcripts","path":"references.txt"}`))
	if !os.IsPermission(err) || result != "" {
		t.Fatalf("filesystem permission softened: %q %v", result, err)
	}
}

func TestSkillReadRejectsAutomaticMCPImport(t *testing.T) {
	_, dir := skillToolFixture(t, nil)
	root := filepath.Dir(filepath.Dir(dir))
	if err := os.WriteFile(filepath.Join(root, "mcp.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := pluginSkillTools(context.Background(), nativeAgentConfig{Plugins: []string{root}}, func(context.Context, string, string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "automatic mcp.json import") {
		t.Fatalf("MCP import accepted: %v", err)
	}
}

func TestSkillReadProvidesLocationForRelativeScript(t *testing.T) {
	tool, dir := skillToolFixture(t, nil)
	scripts := filepath.Join(dir, "scripts")
	if err := os.Mkdir(scripts, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scripts, "probe.sh"), []byte("printf synthetic-probe"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"skill":"steve-operations/find-transcripts"}`))
	if err != nil {
		t.Fatal(err)
	}
	var read skillReadSuccess
	if err := json.Unmarshal([]byte(result), &read); err != nil {
		t.Fatal(err)
	}
	if read.Skill != "steve-operations/find-transcripts" || read.BaseDirectory != dir || read.Path != "SKILL.md" || !strings.Contains(read.Contents, "Read references.txt") || !strings.Contains(read.PathInstructions, "base_directory") {
		t.Fatalf("missing location context: %s", result)
	}
	// The shell runs outside the skill. Only returned location context resolves it.
	cmd := exec.Command("/bin/sh", filepath.Join(read.BaseDirectory, "scripts", "probe.sh"))
	cmd.Dir = t.TempDir()
	output, err := cmd.CombinedOutput()
	if err != nil || string(output) != "synthetic-probe" {
		t.Fatalf("script resolution: %q %v", output, err)
	}
	asset, err := tool.Execute(context.Background(), json.RawMessage(`{"skill":"steve-operations/find-transcripts","path":"scripts/probe.sh"}`))
	if err != nil {
		t.Fatal(err)
	}
	var supporting skillReadSuccess
	if err := json.Unmarshal([]byte(asset), &supporting); err != nil {
		t.Fatal(err)
	}
	if supporting.BaseDirectory != dir || supporting.Path != "scripts/probe.sh" || supporting.Contents != "printf synthetic-probe" {
		t.Fatalf("supporting asset context: %s", asset)
	}
}
