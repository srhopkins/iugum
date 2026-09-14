package agenthome

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
)

type Skill struct {
	Plugin      string
	Name        string
	Description string
	Root        string
}

// Skills reads portable Agent Skills in a root-manifest Agent Plugin. It does
// not execute scripts or expand runtime-specific extensions.
func Skills(root string) ([]Skill, error) {
	b, e := os.ReadFile(filepath.Join(root, "plugin.json"))
	if e != nil {
		return nil, e
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if e = json.Unmarshal(b, &manifest); e != nil {
		return nil, e
	}
	if manifest.Name == "" {
		return nil, fmt.Errorf("plugin name is required")
	}
	if _, e = os.Stat(filepath.Join(root, "mcp.json")); e == nil {
		return nil, fmt.Errorf("plugin %s: automatic mcp.json import is not supported yet; configure MCP explicitly", manifest.Name)
	}
	entries, e := os.ReadDir(filepath.Join(root, "skills"))
	if os.IsNotExist(e) {
		return []Skill{}, nil
	}
	if e != nil {
		return nil, e
	}
	var result []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, "skills", entry.Name())
		b, e := ReadAsset(dir, "SKILL.md")
		if e != nil {
			return nil, e
		}
		parts := strings.SplitN(string(b), "---", 3)
		if len(parts) != 3 || strings.TrimSpace(parts[0]) != "" {
			return nil, fmt.Errorf("skill %s requires YAML frontmatter", entry.Name())
		}
		var meta struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
		}
		if e = yaml.Unmarshal([]byte(parts[1]), &meta); e != nil {
			return nil, e
		}
		if meta.Name == "" || meta.Description == "" {
			return nil, fmt.Errorf("skill name and description required")
		}
		result = append(result, Skill{manifest.Name, meta.Name, meta.Description, dir})
	}
	return result, nil
}

// ReadAsset confines reads to one skill's directory, including symlink targets.
func ReadAsset(root, relative string) ([]byte, error) {
	if filepath.IsAbs(relative) {
		return nil, fmt.Errorf("relative skill path required")
	}
	base, e := filepath.EvalSymlinks(root)
	if e != nil {
		return nil, e
	}
	target, e := filepath.EvalSymlinks(filepath.Join(base, relative))
	if e != nil {
		return nil, e
	}
	rel, e := filepath.Rel(base, target)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("skill path leaves its directory")
	}
	info, e := os.Stat(target)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() || info.Size() > 256<<10 {
		return nil, fmt.Errorf("skill asset must be a regular file under 256 KiB")
	}
	return os.ReadFile(target)
}
