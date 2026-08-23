package config

import (
	"os"
	"os/user"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// File is iugum.yaml. Missing file = built-in defaults.
type File struct {
	Actor   string `yaml:"actor"`
	Tracker string `yaml:"tracker"`
	Wiki    string `yaml:"wiki"`
	Observe string `yaml:"observe"`
	Policy  Policy `yaml:"policy"`
	Exec    Exec   `yaml:"exec"`
}

// Policy points at Casbin files. Empty paths use the embedded allow-all model.
type Policy struct {
	Model  string `yaml:"model"`
	Policy string `yaml:"policy"`
}

// Exec is the external-linkage slot. Used when tracker/wiki/observe is "exec".
// The command must uphold the same contract as the in-process adapter.
type Exec struct {
	Tracker  []string `yaml:"tracker"`
	Wiki     []string `yaml:"wiki"`
	Observe  []string `yaml:"observe"`
}

func Defaults() File {
	return File{
		Tracker: "beads",
		Wiki:    "silverbullet",
		Observe: "memory",
	}
}

// Load reads IUGUM_CONFIG, then ./iugum.yaml, then ~/.config/iugum/config.yaml.
func Load() (File, error) {
	cfg := Defaults()
	path := os.Getenv("IUGUM_CONFIG")
	if path == "" {
		if _, err := os.Stat("iugum.yaml"); err == nil {
			path = "iugum.yaml"
		} else if home, err := os.UserHomeDir(); err == nil {
			p := filepath.Join(home, ".config", "iugum", "config.yaml")
			if _, err := os.Stat(p); err == nil {
				path = p
			}
		}
	}
	if path == "" {
		return cfg, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Tracker == "" {
		cfg.Tracker = "beads"
	}
	if cfg.Wiki == "" {
		cfg.Wiki = "silverbullet"
	}
	if cfg.Observe == "" {
		cfg.Observe = "memory"
	}
	return cfg, nil
}

func Actor(cfg File) string {
	if a := os.Getenv("IUGUM_ACTOR"); a != "" {
		return a
	}
	if cfg.Actor != "" {
		return cfg.Actor
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "local"
}
