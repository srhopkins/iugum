package main

import (
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
)

// Resolve the native data directory before composition opens any default stores.
func nativeAgentDataDir(args []string) (string, error) {
	path := ""
	for n, a := range args {
		if a == "--home" && n+1 < len(args) {
			path = filepath.Join(args[n+1], "agent.yaml")
			break
		}
		if strings.HasPrefix(a, "--home=") {
			path = filepath.Join(strings.TrimPrefix(a, "--home="), "agent.yaml")
			break
		}
		if a == "--config" && n+1 < len(args) {
			path = args[n+1]
			break
		}
		if strings.HasPrefix(a, "--config=") {
			path = strings.TrimPrefix(a, "--config=")
			break
		}
	}
	if path == "" {
		return "", nil
	}
	absolute, e := filepath.Abs(path)
	if e != nil {
		return "", e
	}
	b, e := os.ReadFile(absolute)
	if e != nil {
		return "", e
	}
	var c struct {
		DataDir string `yaml:"data_dir"`
	}
	if e = yaml.Unmarshal(b, &c); e != nil {
		return "", e
	}
	if c.DataDir == "" {
		c.DataDir = "data"
	}
	if !filepath.IsAbs(c.DataDir) {
		c.DataDir = filepath.Join(filepath.Dir(absolute), c.DataDir)
	}
	return c.DataDir, nil
}
