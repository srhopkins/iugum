package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srhopkins/iugum/agentcore"
)

func TestNativeEditorDiagnosticPrivateAtomicLatestForBothOutcomes(t *testing.T) {
	root := t.TempDir()
	for _, outcome := range []string{"returned", "error"} {
		d := agentcore.EditorTurnDiagnostic{Version: 1, RuntimeOutcome: outcome, Calls: []agentcore.EditorCallDiagnostic{{Tool: "mcp_editor__editor_call", Shape: "object", Result: "no_tool_result"}}}
		if err := persistNativeEditorDiagnostic(root, d); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "editor-diagnostics", "latest.json")
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("diagnostic is not private", err)
		}
		raw, _ := os.ReadFile(path)
		var got agentcore.EditorTurnDiagnostic
		if json.Unmarshal(raw, &got) != nil || got.RuntimeOutcome != outcome {
			t.Fatal("latest outcome not preserved")
		}
	}
	entries, _ := os.ReadDir(filepath.Join(root, "editor-diagnostics"))
	if len(entries) != 1 || entries[0].Name() != "latest.json" {
		t.Fatal("unbounded diagnostic files")
	}
	before, _ := os.ReadFile(filepath.Join(root, "editor-diagnostics", "latest.json"))
	oversized := agentcore.EditorTurnDiagnostic{At: strings.Repeat("X", agentcore.MaxEditorDiagnosticBytes+1)}
	if persistNativeEditorDiagnostic(root, oversized) == nil {
		t.Fatal("oversized snapshot accepted")
	}
	after, _ := os.ReadFile(filepath.Join(root, "editor-diagnostics", "latest.json"))
	if string(before) != string(after) {
		t.Fatal("failed write replaced prior snapshot")
	}
}
func TestNativeEditorDiagnosticSymlinksAndNonprivateDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "private-target")
	if err := os.WriteFile(target, []byte("UNCHANGED"), 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "editor-diagnostics")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	latest := filepath.Join(dir, "latest.json")
	if err := os.Symlink(target, latest); err != nil {
		t.Fatal(err)
	}
	if err := persistNativeEditorDiagnostic(root, agentcore.EditorTurnDiagnostic{Version: 1}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(target)
	if string(raw) != "UNCHANGED" {
		t.Fatal("followed snapshot symlink")
	}
	info, _ := os.Lstat(latest)
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		t.Fatal("snapshot was not atomically replaced with private regular file")
	}
	other := t.TempDir()
	if err := os.Symlink(dir, filepath.Join(other, "editor-diagnostics")); err != nil {
		t.Fatal(err)
	}
	if persistNativeEditorDiagnostic(other, agentcore.EditorTurnDiagnostic{}) == nil {
		t.Fatal("followed directory symlink")
	}
	public := t.TempDir()
	if err := os.Mkdir(filepath.Join(public, "editor-diagnostics"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(public, "editor-diagnostics"), 0755); err != nil {
		t.Fatal(err)
	}
	if persistNativeEditorDiagnostic(public, agentcore.EditorTurnDiagnostic{}) == nil {
		t.Fatal("accepted nonprivate directory")
	}
}
