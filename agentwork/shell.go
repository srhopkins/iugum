package agentwork

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Shell runs a single command in a configured workspace. Policy authorization
// belongs to its caller; this type limits each individual invocation.
type Shell struct {
	Dir       string
	Timeout   time.Duration
	MaxOutput int
}

func (s Shell) Run(ctx context.Context, command string) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("command is required")
	}
	if s.Dir == "" {
		return "", fmt.Errorf("shell working directory is not configured")
	}
	if s.Timeout <= 0 {
		s.Timeout = 30 * time.Second
	}
	if s.MaxOutput <= 0 {
		s.MaxOutput = 32 << 10
	}
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	output := &limitedOutput{limit: s.MaxOutput}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	cmd.Dir = s.Dir
	cmd.Stdout = output
	cmd.Stderr = output
	err := cmd.Run()
	result := output.String()
	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("command timed out after %s", s.Timeout)
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if result != "" {
				result += "\n"
			}
			return fmt.Sprintf("%s[exit status %d]", result, exitErr.ExitCode()), nil
		}
		return result, fmt.Errorf("command failed: %w", err)
	}
	return result, nil
}

type limitedOutput struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (o *limitedOutput) Write(p []byte) (int, error) {
	remaining := o.limit - o.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			o.buf.Write(p[:remaining])
			o.truncated = true
		} else {
			o.buf.Write(p)
		}
	} else {
		o.truncated = true
	}
	return len(p), nil
}

func (o *limitedOutput) String() string {
	if o.truncated {
		return o.buf.String() + "\n[output truncated]"
	}
	return o.buf.String()
}
