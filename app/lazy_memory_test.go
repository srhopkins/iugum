package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewDoesNotOpenMemoryFile(t *testing.T) {
	cfg := e2eConfig(t)
	dir := cfg.DataDir
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, "memory.db")
	if _, err := os.Stat(db); err == nil {
		t.Fatalf("%s exists after New; memory must open lazily", db)
	}
	if _, _, err := a.Memory.Recall(t.Context(), "default", "k"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(db); err != nil {
		t.Fatalf("memory.db not created on first use: %v", err)
	}
}
