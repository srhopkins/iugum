package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestModelControlPersistsAndHonorsRevocation(t *testing.T) {
	c := nativeAgentConfig{ChatTransport: "codex-subscription", SubscriptionProfiles: map[string]string{"fast": "model-fast", "deep": "model-deep"}}
	native, sub := "", ""
	path := filepath.Join(t.TempDir(), "routing.json")
	denied := false
	check := func(context.Context, string, string) error {
		if denied {
			return errors.New("revoked")
		}
		return nil
	}
	state, err := nativeModelControl(context.Background(), nil, c, &native, &sub, path, check, "fast")
	if err != nil || state.Selected != "fast" || sub != "fast" {
		t.Fatalf("%+v %v", state, err)
	}
	before, _ := os.ReadFile(path)
	denied = true
	if _, err = nativeModelControl(context.Background(), nil, c, &native, &sub, path, check, "deep"); err == nil {
		t.Fatal("revoked selection accepted")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) || sub != "fast" {
		t.Fatal("denied write changed selection")
	}
	denied = false
	if _, err = nativeModelControl(context.Background(), nil, c, &native, &sub, path, check, "arbitrary"); err == nil {
		t.Fatal("unconfigured model accepted")
	}
}
