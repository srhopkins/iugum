package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/srhopkins/iugum/agentcore"
	"github.com/srhopkins/iugum/agenthome"
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
	catalog := "Read an installed skill or its supporting asset. Set skill to a full plugin/skill identifier. Omit path to read SKILL.md; path is a file within that skill, never a plugin or skill identifier. Available skills: "
	ids := make([]string, 0, len(skills))
	for _, s := range skills {
		ids = append(ids, s.Plugin+"/"+s.Name)
		catalog += s.Plugin + "/" + s.Name + ": " + s.Description + "; "
	}
	parameters := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"skill": map[string]any{
				"type": "string", "enum": ids,
				"description": "Full installed skill identifier from this enum, in plugin/skill form.",
			},
			"path": map[string]any{
				"type": "string", "default": "SKILL.md",
				"description": "Optional file path relative to the selected skill directory, such as SKILL.md or references/setup.md. Omit to read SKILL.md. Do not put the plugin name or skill identifier here.",
			},
		},
		"required": []string{"skill"},
	}
	return []agentcore.Tool{{Name: "skill_read", Description: catalog, Parameters: parameters, Execute: func(ctx context.Context, b json.RawMessage) (string, error) {
		var q struct {
			Skill string
			Path  string
		}
		if e := json.Unmarshal(b, &q); e != nil {
			return skillReadProblem("invalid_arguments", "Arguments must be a JSON object with a skill identifier and an optional relative file path.", ids), nil
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
				if e != nil {
					// Argument and asset validation failures can be corrected by the
					// model in this turn. Permission and unexpected I/O failures
					// remain hard errors, as do the authorization checks above.
					if os.IsNotExist(e) || isSkillAssetValidationError(e) {
						return skillReadProblem("invalid_path", "The asset is missing or the path is invalid. Omit path to read SKILL.md, then use a relative supporting file path from that skill. Paths must stay inside the skill; files must be regular and at most 256 KiB.", ids), nil
					}
					return "", e
				}
				base, e := filepath.Abs(s.Root)
				if e != nil {
					return "", e
				}
				result, e := json.Marshal(skillReadSuccess{
					Skill: q.Skill, BaseDirectory: base, Path: q.Path,
					Contents:         string(data),
					PathInstructions: "Resolve relative script and asset paths against base_directory, not the shell working directory. Use skill_read with this skill and a relative path to read supporting assets.",
				})
				return string(result), e
			}
		}
		return skillReadProblem("unknown_skill", "Choose a full plugin/skill identifier from available_skills and omit path to read its SKILL.md.", ids), nil
	}}}, nil
}

// Keep the recoverable set narrow: other failures (especially filesystem
// permissions) must not be converted into a successful tool execution.
func isSkillAssetValidationError(err error) bool {
	switch err.Error() {
	case "relative skill path required", "skill path leaves its directory", "skill asset must be a regular file under 256 KiB":
		return true
	}
	return false
}

func skillReadProblem(code, message string, skills []string) string {
	b, _ := json.Marshal(struct {
		IsError         bool     `json:"is_error"`
		Code            string   `json:"code"`
		Message         string   `json:"message"`
		AvailableSkills []string `json:"available_skills"`
		DefaultPath     string   `json:"default_path"`
	}{true, code, message, skills, "SKILL.md"})
	return string(b)
}

// skillReadSuccess supplies location context for skill-relative scripts without
// listing unrelated files or changing the shell working directory.
type skillReadSuccess struct {
	Skill            string `json:"skill"`
	BaseDirectory    string `json:"base_directory"`
	Path             string `json:"path"`
	Contents         string `json:"contents"`
	PathInstructions string `json:"path_instructions"`
}
