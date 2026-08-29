package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// JobsFile is the extra jobs list. Order: IUGUM_JOBS, then ./jobs.yaml.
// Empty string means no extra file exists yet. Writes create ./jobs.yaml
// unless IUGUM_JOBS is set.
func JobsFile() string {
	if p := os.Getenv("IUGUM_JOBS"); p != "" {
		return p
	}
	if _, err := os.Stat("jobs.yaml"); err == nil {
		return "jobs.yaml"
	}
	return ""
}

// JobsFileForWrite is where `iugum job add|rm` persist.
func JobsFileForWrite() string {
	if p := os.Getenv("IUGUM_JOBS"); p != "" {
		return p
	}
	return "jobs.yaml"
}

type jobsFile struct {
	Jobs []JobSpec `yaml:"jobs"`
}

func mergeJobsFile(cfg File) (File, error) {
	path := JobsFile()
	if path == "" {
		return cfg, nil
	}
	extra, err := LoadJobsFile(path)
	if err != nil {
		return cfg, err
	}
	cfg.Jobs = append(cfg.Jobs, extra...)
	return cfg, nil
}

// LoadJobsFile reads a YAML file with a top-level jobs: list.
func LoadJobsFile(path string) ([]JobSpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var file jobsFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("jobs file %s: %w", path, err)
	}
	return file.Jobs, nil
}

// WriteJobsFile replaces the jobs list in path.
func WriteJobsFile(path string, jobs []JobSpec) error {
	if jobs == nil {
		jobs = []JobSpec{}
	}
	raw, err := yaml.Marshal(jobsFile{Jobs: jobs})
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
