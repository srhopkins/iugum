package beadview

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProjectConfig is one entry in the --projects file. Today a project is a
// repo root with a .beads directory. Server-backed trackers will add
// host, port and database fields to the same entry later.
type ProjectConfig struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

// projectNamePattern keeps a project name safe in a URL path segment and
// in a query string.
var projectNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// LoadProjects reads the --projects YAML file. The file is a list of
// {name, path} entries. A top-level "projects:" key holding that list is
// accepted too. A relative path is resolved against the file's directory.
// Each path must hold a .beads directory, and names must be unique.
func LoadProjects(file string) ([]ProjectConfig, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("beadview: reading projects file: %w", err)
	}
	return ParseProjects(raw, filepath.Dir(file))
}

// ParseProjects is LoadProjects for bytes already read. base resolves
// relative paths.
func ParseProjects(raw []byte, base string) ([]ProjectConfig, error) {
	var list []ProjectConfig
	if err := yaml.Unmarshal(raw, &list); err != nil {
		var wrapped struct {
			Projects []ProjectConfig `yaml:"projects"`
		}
		if err2 := yaml.Unmarshal(raw, &wrapped); err2 != nil {
			return nil, fmt.Errorf("beadview: projects file must be a list of {name, path}: %w", err)
		}
		list = wrapped.Projects
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("beadview: projects file lists no projects")
	}
	seen := map[string]bool{}
	for i := range list {
		p := &list[i]
		p.Name = strings.TrimSpace(p.Name)
		p.Path = strings.TrimSpace(p.Path)
		if !projectNamePattern.MatchString(p.Name) {
			return nil, fmt.Errorf("beadview: project %d: name %q must match %s", i+1, p.Name, projectNamePattern)
		}
		if seen[p.Name] {
			return nil, fmt.Errorf("beadview: project name %q is listed twice", p.Name)
		}
		seen[p.Name] = true
		if p.Path == "" {
			return nil, fmt.Errorf("beadview: project %q has no path", p.Name)
		}
		if strings.HasPrefix(p.Path, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				p.Path = filepath.Join(home, p.Path[2:])
			}
		}
		if !filepath.IsAbs(p.Path) {
			p.Path = filepath.Join(base, p.Path)
		}
		abs, err := filepath.Abs(p.Path)
		if err != nil {
			return nil, err
		}
		p.Path = abs
		if fi, err := os.Stat(filepath.Join(abs, ".beads")); err != nil || !fi.IsDir() {
			return nil, fmt.Errorf("beadview: project %q: no .beads/ directory in %s", p.Name, abs)
		}
	}
	return list, nil
}
