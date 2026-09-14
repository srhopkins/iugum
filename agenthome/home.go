// Package agenthome describes a private instance, independent of wiki or model provider.
package agenthome

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Definition adds runtime state to reusable plugin packages. Plugin code is never
// executed merely by loading this definition. Namespace is the policy subject.
type Definition struct {
	Sources          []map[string]string `yaml:"sources"`
	Version          int                 `yaml:"version"`
	Name             string              `yaml:"name"`
	Namespace        string              `yaml:"namespace"`
	Runtime          string              `yaml:"runtime"`
	DataDir          string              `yaml:"data_dir"`
	InstructionsFile string              `yaml:"instructions_file"`
	MissionFile      string              `yaml:"mission_file"`
	PolicyFile       string              `yaml:"policy_file"`
	Plugins          []string            `yaml:"plugins"`
}

var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func Init(root, name string) error {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	root = absolute
	if !validName.MatchString(name) {
		return fmt.Errorf("invalid agent name")
	}
	if _, err := os.Lstat(root); err == nil {
		return fmt.Errorf("agent home already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	d := Definition{Sources: []map[string]string{{"platform": "claude", "root": filepath.Join(root, "data", "transcripts")}}, Version: 1, Name: name, Namespace: "agent:" + name, Runtime: "native", DataDir: "data", InstructionsFile: "instructions.md", MissionFile: "mission.md", PolicyFile: "policy.csv", Plugins: []string{}}
	b, err := yaml.Marshal(d)
	if err != nil {
		return err
	}
	files := map[string][]byte{"agent.yaml": b, "instructions.md": []byte("Describe this agent's standing role and communication preferences here.\n"), "mission.md": []byte("No mission assigned. Wait for the user to set one.\n"), "policy.csv": []byte("# No grants by default. Configure approved roles and permissions before use.\n"), ".gitignore": []byte("data/\n*.log\n")}
	for p, b := range files {
		if err := os.WriteFile(filepath.Join(root, p), b, 0600); err != nil {
			return err
		}
	}
	return os.Mkdir(filepath.Join(root, "data"), 0700)
}

// ValidateStateRoot prevents a managed definition from accidentally sharing an
// external writable state directory. Explicit resource sharing belongs in policy.
func ValidateStateRoot(home, data string) error {
	root, e := filepath.Abs(home)
	if e != nil {
		return e
	}
	target, e := filepath.Abs(data)
	if e != nil {
		return e
	}
	rel, e := filepath.Rel(root, target)
	if e != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("managed agent data_dir must be inside its own home")
	}
	if actual, e := filepath.EvalSymlinks(target); e == nil {
		canonicalRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			return err
		}
		rel, e = filepath.Rel(canonicalRoot, actual)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("managed agent state symlink leaves its home")
		}
	}
	return nil
}
