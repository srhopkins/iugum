package main

import (
	"context"
	"encoding/json"
	"github.com/srhopkins/iugum/agentclones"
	"github.com/srhopkins/iugum/agentcore"
	"path/filepath"
)

const nativeCloneNamePattern = `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`

func cloneReadExecution(candidate func(string) (string, error)) func(context.Context, json.RawMessage) (string, error) {
	return func(_ context.Context, raw json.RawMessage) (string, error) {
		var q struct{ Name, Path string }
		if err := json.Unmarshal(raw, &q); err != nil {
			return "", err
		}
		if !validNativeCloneName(q.Name) {
			return "", &agentcore.InvalidToolArguments{Message: "name must be an existing candidate identifier of 1–64 ASCII letters, digits, underscores or hyphens, starting with a letter or digit. Do not pass a filesystem path or plugin/skill identifier. Use shell for workspace files and skill_read for installed skill assets."}
		}
		dest, err := candidate(q.Name)
		if err != nil {
			return "", err
		}
		contents, err := agentclones.Read(filepath.Join(dest, "source"), q.Path)
		return string(contents), err
	}
}
