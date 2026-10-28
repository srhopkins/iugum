package main

import (
	"bytes"
	"context"
	"github.com/srhopkins/iugum/app"
	"github.com/srhopkins/iugum/policy"
	"os"
	"path/filepath"
	"testing"
)

func TestProviderLifecycleRetainsApplicationGate(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "policy.csv")
	os.WriteFile(p, []byte("p, tester, agent/sessions, serve, deny\n"), 0600)
	gate, e := policy.New("", p)
	if e != nil {
		t.Fatal(e)
	}
	a := &app.App{Actor: "tester", Gate: gate}
	var out, err bytes.Buffer
	code := runAgentSessions(context.Background(), a, []string{"setup", "--config", filepath.Join(root, "missing")}, &out, &err)
	if code != 1 || !bytes.Contains(err.Bytes(), []byte("denied")) {
		t.Fatal("setup bypassed service gate", code, err.String())
	}
	if _, e = os.Stat(filepath.Join(root, "missing")); !os.IsNotExist(e) {
		t.Fatal("denied setup wrote settings")
	}
}
