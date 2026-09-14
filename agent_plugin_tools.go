package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/srhopkins/iugum/agentcore"
	"github.com/srhopkins/iugum/agenthome"
	"path/filepath"
)

func pluginSkillTools(ctx context.Context, c nativeAgentConfig, check func(context.Context, string, string) error) ([]agentcore.Tool, error) {
	var skills []agenthome.Skill
	for _, root := range c.Plugins {
		if !filepath.IsAbs(root) {
			root = filepath.Join(filepath.Dir(c.ConfigPath), root)
		}
		if e := check(ctx, "plugin:"+root, "read"); e != nil {
			return nil, e
		}
		found, e := agenthome.Skills(root)
		if e != nil {
			return nil, e
		}
		skills = append(skills, found...)
	}
	if len(skills) == 0 {
		return nil, nil
	}
	catalog := "Read an installed skill or its supporting asset. Available skills: "
	for _, s := range skills {
		catalog += s.Plugin + "/" + s.Name + ": " + s.Description + "; "
	}
	return []agentcore.Tool{{Name: "skill_read", Description: catalog, Parameters: map[string]any{"type": "object", "properties": map[string]any{"skill": map[string]string{"type": "string"}, "path": map[string]string{"type": "string"}}, "required": []string{"skill"}}, Execute: func(ctx context.Context, b json.RawMessage) (string, error) {
		var q struct {
			Skill string
			Path  string
		}
		if e := json.Unmarshal(b, &q); e != nil {
			return "", e
		}
		if q.Path == "" {
			q.Path = "SKILL.md"
		}
		for _, s := range skills {
			if q.Skill == s.Plugin+"/"+s.Name {
				if e := check(ctx, "skill:"+q.Skill, "read"); e != nil {
					return "", e
				}
				data, e := agenthome.ReadAsset(s.Root, q.Path)
				return string(data), e
			}
		}
		return "", fmt.Errorf("unknown installed skill")
	}}}, nil
}
