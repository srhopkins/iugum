package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAgentProcessLockAndCleanup(t *testing.T) {
	home := t.TempDir()
	p, release, e := acquireAgentProcess(home, "127.0.0.1:3999")
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	if len(p.Token) != 64 {
		t.Fatal("missing control capability")
	}
	if _, r, e := acquireAgentProcess(home, "127.0.0.1:4000"); e == nil {
		r()
		t.Fatal("duplicate process accepted")
	}
	release()
	if _, e = os.Stat(filepath.Join(home, "data", "runtime.json")); !os.IsNotExist(e) {
		t.Fatal("stale runtime record")
	}
}
