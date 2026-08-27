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
	Observe    string     `yaml:"observe"`
	Memory     string     `yaml:"memory"`
	DataDir    string     `yaml:"data_dir"`
	Embeddings Embeddings `yaml:"embeddings"`
	Graph      Graph      `yaml:"graph"`
	Policy     Policy      `yaml:"policy"`
	Exec       Exec        `yaml:"exec"`
	Jobs       []JobSpec   `yaml:"jobs"`
	HookRoutes []HookRoute `yaml:"hooks"`
	Watch      []WatchSpec `yaml:"watch"`
	HookHTTP   string      `yaml:"hook_http"` // empty = do not bind
	Network    Network     `yaml:"network"`
}

// JobSpec is one cron or @triggered job. workflow is a linear step list.
// after + on wire go-cron AddDependencyByName (OnSuccess, OnFailure, …).
type JobSpec struct {
	Name     string   `yaml:"name"`
	Spec     string   `yaml:"spec"`
	Kind     string   `yaml:"kind"` // empty | func | exec | http
	Command  []string `yaml:"command"`
	URL      string   `yaml:"url"`
	Workflow []string `yaml:"workflow"`
	After    []string `yaml:"after"`
	On       string   `yaml:"on"` // success | failure | skipped | complete
}

// HookRoute sends a hook name to a job.
type HookRoute struct {
	On  string `yaml:"on"`
	Job string `yaml:"job"`
}

// WatchSpec is one folder for fsnotify.
type WatchSpec struct {
	Path string `yaml:"path"`
}

// Embeddings is the optional vector slot. Off = substring + FTS5 only.
type Embeddings struct {
	Enabled bool   `yaml:"enabled"`
	Vec     bool   `yaml:"vec"` // sqlite-vec vec0 KNN; default off
	Kind    string `yaml:"kind"` // off | ollama | openai
	URL     string `yaml:"url"`
	Model   string `yaml:"model"`
}

// Graph points at the relation glossary. extractor is off, rules, or llm.
type Graph struct {
	Glossary  string   `yaml:"glossary"`
	Extractor string   `yaml:"extractor"`
	LLM       GraphLLM `yaml:"llm"`
}

// GraphLLM is the optional chat model for extractor llm.
type GraphLLM struct {
	Kind  string `yaml:"kind"` // ollama | openai
	URL   string `yaml:"url"`
	Model string `yaml:"model"`
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
		Memory:  "sqlite",
		Graph: Graph{
			Glossary:  "glossaries/memory-graph.yaml",
			Extractor: "rules",
		},
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
	if cfg.Memory == "" {
		cfg.Memory = "sqlite"
	}
	if cfg.Graph.Extractor == "" {
		cfg.Graph.Extractor = "rules"
	}
	if cfg.Embeddings.Kind == "" {
		if cfg.Embeddings.Enabled {
			cfg.Embeddings.Kind = "ollama"
		} else {
			cfg.Embeddings.Kind = "off"
		}
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

// Network is the top-level network: block. Absent block = backend off.
// backend: auto | iptables | nftables | off. auto picks nftables when nft is on PATH.
type Network struct {
	Backend string     `yaml:"backend"`
	Default NetDefault `yaml:"default"`
	Rules   []NetRule  `yaml:"rules"`
}

// NetDefault is the chain policy per direction: allow | deny.
type NetDefault struct {
	In  string `yaml:"in"`
	Out string `yaml:"out"`
}

// NetRule is one firewall rule in iugum.yaml.
type NetRule struct {
	Name   string `yaml:"name"`
	Dir    string `yaml:"dir"`    // in | out
	Proto  string `yaml:"proto"`  // tcp | udp | icmp | all
	Port   int    `yaml:"port"`   // tcp/udp only
	Src    string `yaml:"src"`    // IP or CIDR; empty = any
	Dst    string `yaml:"dst"`    // IP or CIDR; empty = any
	Action string `yaml:"action"` // allow | deny
}
