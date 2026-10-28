package agentplatform

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// This inventory is deliberately separate from Iugum-managed Claude sessions.
// A tmux pane is evidence of a process, not a grant to steer that process.
type ffaiWorker struct {
	Session       string `json:"session"`
	Pane          string `json:"pane"`
	Command       string `json:"command"`
	Cwd           string `json:"cwd"`
	ClaudeProcess bool   `json:"claude_process"`
	Managed       bool   `json:"managed"`
}
type ffaiInventory struct {
	Status    string       `json:"status"`
	Container string       `json:"container"`
	Hostname  string       `json:"hostname,omitempty"`
	User      string       `json:"user,omitempty"`
	Workers   []ffaiWorker `json:"workers"`
	Message   string       `json:"message"`
}

const ffaiInventoryScript = `hostname; id -un; tmux list-panes -a -F '#{session_name}\t#{pane_id}\t#{pane_current_command}\t#{pane_current_path}'`

// Iugum runs beside ffai-agent on its configured Docker socket. Never choose a
// container by fuzzy name, change Docker contexts, or execute startup files.
func ffaiDockerArgs() []string {
	return []string{"exec", "-u", "abc", "--env", "BASH_ENV=", "--env", "ENV=", "ffai-agent", "/bin/bash", "--noprofile", "--norc", "-c", ffaiInventoryScript}
}

type ffaiOutput struct {
	bytes.Buffer
	overflow bool
}

func (b *ffaiOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 16*1024 {
		b.overflow = true
		return 0, errors.New("FFAI inventory output limit")
	}
	return b.Buffer.Write(p)
}

func listFFAIWorkers(ctx context.Context) ffaiInventory {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", ffaiDockerArgs()...)
	var out, stderr ffaiOutput
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return ffaiProblem("timeout", "FFAI discovery timed out; no session was changed.")
	}
	if out.overflow || stderr.overflow {
		return ffaiProblem("output_limit", "FFAI discovery exceeded its output limit; no partial inventory returned.")
	}
	return parseFFAIInventory(out.String(), stderr.String(), err)
}
func ffaiProblem(status, message string) ffaiInventory {
	return ffaiInventory{Status: status, Container: "ffai-agent", Workers: []ffaiWorker{}, Message: message}
}
func parseFFAIInventory(stdout, stderr string, runErr error) ffaiInventory {
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) < 2 || lines[0] == "" || lines[1] != "abc" {
		return ffaiProblem("unavailable", "Could not verify named ffai-agent container and abc user. Check its Docker socket and running state; no fallback host was used.")
	}
	v := ffaiProblem("ok", "Read-only tmux inventory. Claude processes are not Iugum-managed workers; availability and permission to send must be checked before an explicitly authorized handoff.")
	v.Hostname = lines[0]
	v.User = lines[1]
	if runErr != nil {
		if strings.Contains(stderr, "no server running on") || strings.Contains(stderr, "No such file or directory") && strings.Contains(stderr, "error connecting to") {
			v.Status = "empty"
			v.Message = "No abc tmux server is running in ffai-agent."
			return v
		}
		v.Status = "tmux_unavailable"
		v.Message = "Identity verified but abc tmux inventory failed. Check tmux availability and socket permissions; no root socket fallback was used."
		return v
	}
	for _, line := range lines[2:] {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\\t")
		if len(fields) != 4 || len(v.Workers) >= 64 {
			return ffaiProblem("invalid_inventory", "FFAI metadata was malformed or exceeded 64 panes; no partial inventory returned.")
		}
		for _, field := range fields {
			if len(field) > 1024 || strings.ContainsAny(field, "\r\x00") {
				return ffaiProblem("invalid_inventory", "FFAI metadata failed validation.")
			}
		}
		v.Workers = append(v.Workers, ffaiWorker{Session: fields[0], Pane: fields[1], Command: fields[2], Cwd: fields[3], ClaudeProcess: fields[2] == "claude"})
	}
	sort.Slice(v.Workers, func(i, j int) bool {
		if v.Workers[i].Session == v.Workers[j].Session {
			return v.Workers[i].Pane < v.Workers[j].Pane
		}
		return v.Workers[i].Session < v.Workers[j].Session
	})
	if len(v.Workers) == 0 {
		v.Status = "empty"
		v.Message = "abc tmux server returned no panes."
	}
	return v
}

var _ io.Writer = (*ffaiOutput)(nil)
