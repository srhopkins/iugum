package agenthome

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndependentHomesAndNoImplicitGrant(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"first", "second"} {
		if err := Init(filepath.Join(root, name), name); err != nil {
			t.Fatal(err)
		}
	}
	if err := Init(filepath.Join(root, "first"), "first"); err == nil {
		t.Fatal("overwrote home")
	}
	p := filepath.Join(root, "first", "mission.md")
	os.WriteFile(p, []byte("New mission"), 0600)
	b, _ := os.ReadFile(filepath.Join(root, "second", "mission.md"))
	if strings.Contains(string(b), "New mission") {
		t.Fatal("shared mission")
	}
	b, _ = os.ReadFile(filepath.Join(root, "first", "policy.csv"))
	if strings.Contains(string(b), "allow") {
		t.Fatal("implicit grant")
	}
}
func TestStateCannotLeaveHome(t *testing.T) {
	root := t.TempDir()
	if err := ValidateStateRoot(root, filepath.Join(root, "data")); err != nil {
		t.Fatal(err)
	}
	if err := ValidateStateRoot(root, filepath.Join(root, "..", "other")); err == nil {
		t.Fatal("external state accepted")
	}
	outside := t.TempDir()
	os.Symlink(outside, filepath.Join(root, "data"))
	if err := ValidateStateRoot(root, filepath.Join(root, "data")); err == nil {
		t.Fatal("external symlink accepted")
	}
}
