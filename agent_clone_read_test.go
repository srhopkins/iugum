package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/srhopkins/iugum/agentcore"
	"os"
	"path/filepath"
	"testing"
)

func TestCloneReadInvalidNameDoesNotAccessCandidate(t *testing.T) {
	calls := 0
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "source"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source", "probe"), []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	execute := cloneReadExecution(func(name string) (string, error) { calls++; return root, nil })
	for _, name := range []string{"", "../escape", "plugin/skill", "/absolute"} {
		raw, _ := json.Marshal(map[string]string{"Name": name, "Path": "probe"})
		_, err := execute(context.Background(), raw)
		var invalid *agentcore.InvalidToolArguments
		if !errors.As(err, &invalid) || calls != 0 {
			t.Fatalf("invalid name accessed candidate: calls=%d err=%v", calls, err)
		}
	}
	out, err := execute(context.Background(), json.RawMessage(`{"name":"existing","path":"probe"}`))
	if err != nil || out != "synthetic" || calls != 1 {
		t.Fatalf("corrected call: %q %v calls=%d", out, err, calls)
	}
	_, err = execute(context.Background(), json.RawMessage(`{"name":"existing","path":"missing"}`))
	var invalid *agentcore.InvalidToolArguments
	if !os.IsNotExist(err) || errors.As(err, &invalid) {
		t.Fatalf("OS error changed: %v", err)
	}
	_, err = execute(context.Background(), json.RawMessage(`{"name":"existing","path":"../escape"}`))
	if err == nil || errors.As(err, &invalid) {
		t.Fatalf("confinement softened: %v", err)
	}
}
