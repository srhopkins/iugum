package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitAgentScaffoldAndParse(t *testing.T) {
	cwd := t.TempDir()
	var warnings bytes.Buffer
	if err := initAgent(cwd, "scout", &warnings); err != nil {
		t.Fatal(err)
	}
	if warnings.Len() != 0 {
		t.Fatalf("outside git must not warn: %s", warnings.String())
	}

	root := filepath.Join(cwd, "scout")
	cfg, err := LoadAgentFile(filepath.Join(root, "agent.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "scout" || cfg.Image != defaultAgentImage {
		t.Fatalf("identity = %+v", cfg)
	}
	if cfg.Network.Name != "scout" || cfg.Network.Mode != "open" {
		t.Fatalf("network = %+v", cfg.Network)
	}
	if cfg.Startup.Restart != "unless-stopped" {
		t.Fatalf("startup = %+v", cfg.Startup)
	}
	if len(cfg.Mounts) != 2 {
		t.Fatalf("mounts = %+v", cfg.Mounts)
	}
	if cfg.Jobs != "jobs.yaml" {
		t.Fatalf("jobs = %q", cfg.Jobs)
	}
	for _, path := range []string{
		filepath.Join(root, "home", "policy.csv"),
		filepath.Join(root, "home", ".iugum-probe"),
		filepath.Join(root, "data", ".iugum-probe"),
		filepath.Join(root, "data", "iugum.yaml"),
		filepath.Join(root, "jobs.yaml"),
	} {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("%s missing or empty: %v", path, err)
		}
	}
}

func TestInitAgentRefusesExistingConfig(t *testing.T) {
	cwd := t.TempDir()
	if err := initAgent(cwd, "worker", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cwd, "worker", "agent.yaml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := initAgent(cwd, "worker", &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("second init must refuse overwrite: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("second init changed agent.yaml")
	}
}

func TestParseAgentFileOptionalSchema(t *testing.T) {
	cfg, err := ParseAgentFile([]byte(`name: locked
image: example:v1
mounts:
  - target: /run/secrets
    tmpfs: true
    ro: true
ports:
  - "127.0.0.1:8080:8080"
network:
  name: locked
  mode: locked
privileges:
  cap_add: [NET_ADMIN]
startup:
  restart: "no"
  env: [SSH_AUTH_SOCK]
jobs: jobs.yaml
`))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Mounts[0].Tmpfs || !cfg.Mounts[0].RO {
		t.Fatalf("tmpfs mount = %+v", cfg.Mounts[0])
	}
	if cfg.Network.Mode != "locked" || cfg.Privileges.CapAdd[0] != "NET_ADMIN" {
		t.Fatalf("optional schema = %+v", cfg)
	}
	if cfg.Startup.Restart != "no" || cfg.Jobs != "jobs.yaml" {
		t.Fatalf("startup/jobs = %+v", cfg)
	}
}

func TestAgentGitIgnoreWarning(t *testing.T) {
	cwd := t.TempDir()
	if err := exec.Command("git", "init", "-q", cwd).Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	var warnings bytes.Buffer
	if err := initAgent(cwd, "visible", &warnings); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warnings.String(), "visible/home/") || !strings.Contains(warnings.String(), "visible/data/") {
		t.Fatalf("warning = %q", warnings.String())
	}

	ignored := t.TempDir()
	if err := exec.Command("git", "init", "-q", ignored).Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ignored, ".gitignore"), []byte("*/home/\n*/data/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	warnings.Reset()
	if err := initAgent(ignored, "private", &warnings); err != nil {
		t.Fatal(err)
	}
	if warnings.Len() != 0 {
		t.Fatalf("ignored paths must not warn: %s", warnings.String())
	}
}

func TestValidAgentName(t *testing.T) {
	for _, good := range []string{"worker", "worker-1", "worker_one"} {
		if !validAgentName(good) {
			t.Errorf("%q must be valid", good)
		}
	}
	for _, bad := range []string{"", ".", "..", "../escape", "nested/name", "/absolute"} {
		if validAgentName(bad) {
			t.Errorf("%q must be invalid", bad)
		}
	}
}

func TestAgentRunArgvLifecycleAndCapabilities(t *testing.T) {
	root := filepath.Join(t.TempDir(), "scout")
	cfg := AgentFile{
		Name:  "scout",
		Image: "example:v1",
		Mounts: []AgentMount{
			{Source: "./home", Target: "/home/iugum", RO: true},
		},
		Network:    AgentNetwork{Name: "scout", Mode: "open"},
		Privileges: &AgentPrivilege{CapAdd: []string{"NET_ADMIN", "SYS_PTRACE"}},
		Startup:    AgentStartup{Restart: "unless-stopped", Env: []string{"SSH_AUTH_SOCK"}},
	}
	argv := strings.Join(agentRunArgv("docker", root, cfg), " ")
	for _, want := range []string{
		"docker run -d",
		"--network iugum-agent-scout",
		"--restart unless-stopped",
		"--user 1000:1000",
		"/home/iugum:ro",
		"-e SSH_AUTH_SOCK",
		"--cap-add NET_ADMIN",
		"--cap-add SYS_PTRACE",
		"example:v1",
	} {
		if !strings.Contains(argv, want) {
			t.Errorf("argv %q lacks %q", argv, want)
		}
	}
	if strings.Contains(argv, " --rm") {
		t.Fatalf("long-lived agent argv contains --rm: %s", argv)
	}
	if strings.Contains(argv, "--env-file") {
		t.Fatalf("argv leaked --env-file without home/.env: %s", argv)
	}

	if err := os.MkdirAll(filepath.Join(root, "home"), 0o755); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(root, "home", ".env")
	if err := os.WriteFile(envPath, []byte("HASS_TOKEN=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	withEnv := strings.Join(agentRunArgv("docker", root, cfg), " ")
	if !strings.Contains(withEnv, "--env-file "+envPath) {
		t.Fatalf("argv %q lacks --env-file %s", withEnv, envPath)
	}
}

func TestAgentNetworkNameIncludesAgentName(t *testing.T) {
	cfg := AgentFile{Name: "worker", Network: AgentNetwork{Name: "worker"}}
	if got, want := agentNetworkName(cfg), "iugum-agent-worker"; got != want {
		t.Fatalf("agentNetworkName() = %q, want %q", got, want)
	}
	cfg.Network.Name = "private"
	if got, want := agentNetworkName(cfg), "iugum-agent-worker-private"; got != want {
		t.Fatalf("custom agentNetworkName() = %q, want %q", got, want)
	}
}

func TestAgentAttachArgvTTYModes(t *testing.T) {
	tui := agentAttachArgv("docker", "scout", "tui")
	if got, want := strings.Join(tui, " "), "docker exec -it scout opencode"; got != want {
		t.Fatalf("tui argv = %q, want %q", got, want)
	}

	acp := agentAttachArgv("podman", "scout", "acp")
	if got, want := strings.Join(acp, " "), "podman exec -i scout opencode acp"; got != want {
		t.Fatalf("acp argv = %q, want %q", got, want)
	}
	if strings.Contains(strings.Join(acp, " "), "-it") || strings.Contains(strings.Join(acp, " "), "--tty") {
		t.Fatalf("ACP argv allocates a tty: %v", acp)
	}
}

func TestCheckpointAgentMemoryStagesOnlyDatabase(t *testing.T) {
	repo := t.TempDir()
	runGitTestCommand(t, repo, "init", "-q")
	runGitTestCommand(t, repo, "config", "user.email", "agent-test@example.invalid")
	runGitTestCommand(t, repo, "config", "user.name", "Agent Test")
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("*-wal\n*-shm\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, repo, "add", ".gitignore")
	runGitTestCommand(t, repo, "commit", "-qm", "init")

	agentRoot := filepath.Join(repo, "scout")
	home := filepath.Join(agentRoot, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		filepath.Join(home, "memory.db"):     "database",
		filepath.Join(home, "memory.db-wal"): "wal",
		filepath.Join(home, "memory.db-shm"): "shm",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sqlite := filepath.Join(t.TempDir(), "sqlite3")
	if err := os.WriteFile(sqlite, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := checkpointAgentMemory(agentRoot, "scout", sqlite, &stdout, &stderr); err != nil {
		t.Fatalf("checkpointAgentMemory() error = %v; stderr = %s", err, stderr.String())
	}
	tracked := strings.Fields(runGitTestCommand(t, repo, "ls-files"))
	if got, want := strings.Join(tracked, " "), ".gitignore scout/home/memory.db"; got != want {
		t.Fatalf("tracked files = %q, want %q", got, want)
	}
	if subject := strings.TrimSpace(runGitTestCommand(t, repo, "log", "-1", "--format=%s")); subject != "checkpoint scout agent memory" {
		t.Fatalf("checkpoint commit subject = %q", subject)
	}
}

func TestCheckpointAgentMemorySkipsMissingDatabase(t *testing.T) {
	var stdout bytes.Buffer
	if err := checkpointAgentMemory(t.TempDir(), "empty", "sqlite3", &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "skipped") {
		t.Fatalf("missing database output = %q", stdout.String())
	}
}

func runGitTestCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
