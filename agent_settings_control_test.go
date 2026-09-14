package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsOnlyWritesConfiguredDocuments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mission.md")
	os.WriteFile(path, []byte("original"), 0600)
	c := nativeAgentConfig{MissionFile: path}
	deny := false
	check := func(_ context.Context, _ string, action string) error {
		if deny && action == "write" {
			return errors.New("revoked")
		}
		return nil
	}
	value := "next mission"
	state, err := nativeSettingsControl(context.Background(), c, check, "mission", &value)
	if err != nil || len(state.Fields) != 1 || state.Fields[0].Value != value {
		t.Fatalf("%+v %v", state, err)
	}
	if _, err = nativeSettingsControl(context.Background(), c, check, "policy", &value); err == nil {
		t.Fatal("policy was writable")
	}
	deny = true
	value = "forbidden"
	if _, err = nativeSettingsControl(context.Background(), c, check, "mission", &value); err == nil {
		t.Fatal("revoked write accepted")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "next mission" {
		t.Fatal("denied write changed file")
	}
	state, err = nativeSettingsControl(context.Background(), c, check, "", nil)
	if err != nil || state.Fields[0].Editable {
		t.Fatal("revoked field not read-only")
	}
}
