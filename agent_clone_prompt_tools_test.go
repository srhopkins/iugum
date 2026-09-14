package main

import (
	"context"
	"encoding/json"
	"github.com/srhopkins/iugum/agentdesk"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClonePromptApprovalAndDrift(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	root := filepath.Join(dir, "clones")
	clone := filepath.Join(root, "candidate")
	_ = os.MkdirAll(clone, 0700)
	parent := filepath.Join(dir, "instructions.md")
	_ = os.WriteFile(parent, []byte("Original instructions"), 0600)
	_ = os.WriteFile(filepath.Join(clone, "instructions.md"), []byte("Original instructions"), 0600)
	var approval agentdesk.Approval
	tools, execute := buildClonePromptTools(root, parent, func(context.Context, string, string) error { return nil }, func(_ context.Context, r agentdesk.ApprovalRequest) (agentdesk.Approval, error) {
		approval = agentdesk.Approval{ApprovalRequest: r, Status: "executing"}
		return approval, nil
	})
	call := func(name string, v any) error {
		t.Helper()
		raw, _ := json.Marshal(v)
		for _, tool := range tools {
			if tool.Name == name {
				_, err := tool.Execute(context.Background(), raw)
				return err
			}
		}
		t.Fatal(name)
		return nil
	}
	if err := call("clone_prompt_write", map[string]string{"name": "candidate", "content": "Improved concise instructions"}); err != nil {
		t.Fatal(err)
	}
	if err := call("clone_prompt_request_promotion", map[string]string{"name": "candidate"}); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(parent, []byte("Independent parent change"), 0600)
	if _, err := execute(context.Background(), approval); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatal(err)
	}
	_ = os.WriteFile(parent, []byte("Original instructions"), 0600)
	if _, err := execute(context.Background(), approval); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(parent)
	if string(content) != "Improved concise instructions" {
		t.Fatal(string(content))
	}
	backup, _ := os.ReadFile(filepath.Join(clone, "parent-instructions-before-promotion.md"))
	if string(backup) != "Original instructions" {
		t.Fatal(string(backup))
	}
}
func TestClonePromptRejectsSymlink(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	root := filepath.Join(dir, "clones")
	_ = os.MkdirAll(root, 0700)
	outside := filepath.Join(dir, "outside")
	_ = os.MkdirAll(outside, 0700)
	_ = os.Symlink(outside, filepath.Join(root, "candidate"))
	tools, _ := buildClonePromptTools(root, "", func(context.Context, string, string) error { return nil }, nil)
	for _, tool := range tools {
		if tool.Name == "clone_prompt_read" {
			_, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"candidate"}`))
			if err == nil {
				t.Fatal("symlink accepted")
			}
		}
	}
}
