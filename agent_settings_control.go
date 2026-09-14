package main

import (
	"context"
	"fmt"
	"github.com/srhopkins/iugum/agentdesk"
	"os"
)

// Only named instruction documents are exposed. Runtime commands, credentials,
// namespaces and policy files are not writable through the settings drawer.
func nativeSettingsControl(ctx context.Context, c nativeAgentConfig, check func(context.Context, string, string) error, id string, value *string) (agentdesk.SettingsState, error) {
	state := agentdesk.SettingsState{Fields: []agentdesk.SettingField{}}
	fields := []struct{ id, label, path string }{{"mission", "Mission", c.MissionFile}, {"instructions", "Instructions", c.InstructionsFile}}
	if value != nil {
		found := false
		for _, field := range fields {
			if field.id == id && field.path != "" {
				found = true
				if err := check(ctx, "agent/settings/"+id, "write"); err != nil {
					return state, err
				}
				if len(*value) > 65536 {
					return state, fmt.Errorf("setting exceeds 64 KiB")
				}
				if err := writeAgentState(field.path, []byte(*value)); err != nil {
					return state, err
				}
			}
		}
		if !found {
			return state, fmt.Errorf("setting is not editable")
		}
	}
	for _, field := range fields {
		if field.path == "" || check(ctx, "agent/settings/"+field.id, "read") != nil {
			continue
		}
		data, err := os.ReadFile(field.path)
		if err != nil {
			return state, err
		}
		state.Fields = append(state.Fields, agentdesk.SettingField{ID: field.id, Label: field.label, Value: string(data), Editable: check(ctx, "agent/settings/"+field.id, "write") == nil})
	}
	if len(state.Fields) == 0 {
		state.Reason = "No instruction documents are configured or permitted."
	}
	return state, nil
}
