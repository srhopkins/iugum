package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	defaultAgentImage  = "iugum:latest"
	defaultNetworkMode = "open"
	defaultRestart     = "unless-stopped"
)

// AgentFile is the sparse, per-agent source of truth in agent.yaml.
type AgentFile struct {
	Name       string          `yaml:"name"`
	Image      string          `yaml:"image"`
	Mounts     []AgentMount    `yaml:"mounts,omitempty"`
	Ports      []string        `yaml:"ports,omitempty"`
	Network    AgentNetwork    `yaml:"network"`
	Privileges *AgentPrivilege `yaml:"privileges,omitempty"`
	Startup    AgentStartup    `yaml:"startup,omitempty"`
	Jobs       string          `yaml:"jobs,omitempty"`
	ShmSize    string          `yaml:"shm_size,omitempty"` // docker --shm-size, e.g. 1g for Chromium
	ExtraHosts []string        `yaml:"extra_hosts,omitempty"`
}

// AgentMount describes a bind mount or a tmpfs mask.
// A tmpfs mount omits Source and sets Tmpfs.
type AgentMount struct {
	Source string `yaml:"source,omitempty"`
	Target string `yaml:"target"`
	RO     bool   `yaml:"ro,omitempty"`
	Tmpfs  bool   `yaml:"tmpfs,omitempty"`
}

type AgentNetwork struct {
	Name string `yaml:"name"`
	Mode string `yaml:"mode,omitempty"`
}

type AgentPrivilege struct {
	CapAdd []string `yaml:"cap_add,omitempty"`
}

type AgentStartup struct {
	Restart string   `yaml:"restart,omitempty"`
	Env     []string `yaml:"env,omitempty"`
	Command []string `yaml:"command,omitempty"` // extra argv after the image; empty = image default (up)
}

// ParseAgentFile parses agent.yaml and applies defaults that may be omitted.
func ParseAgentFile(data []byte) (AgentFile, error) {
	var cfg AgentFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return AgentFile{}, fmt.Errorf("agent.yaml: %w", err)
	}
	cfg.applyDefaults()
	return cfg, nil
}

// LoadAgentFile reads and parses an agent.yaml file.
func LoadAgentFile(path string) (AgentFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentFile{}, fmt.Errorf("agent.yaml: %w", err)
	}
	return ParseAgentFile(data)
}

func (a *AgentFile) applyDefaults() {
	if a.Network.Mode == "" {
		a.Network.Mode = defaultNetworkMode
	}
	if a.Startup.Restart == "" {
		a.Startup.Restart = defaultRestart
	}
	if len(a.ExtraHosts) == 0 {
		a.ExtraHosts = []string{"host.docker.internal:host-gateway"}
	}
}
