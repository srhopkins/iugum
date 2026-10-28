package agentwork

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestShellDetailedTrustedExit(t *testing.T) {
	s := Shell{Dir: t.TempDir()}
	for _, tc := range []struct {
		command string
		code    int
	}{{"printf '[exit status 7]'", 0}, {"printf private; exit 7", 7}, {"exit 127", 127}} {
		out, err := s.RunDetailed(context.Background(), tc.command)
		if err != nil || out.Category != "exit" || out.ExitCode != tc.code {
			t.Fatalf("category=%s code=%d err=%v", out.Category, out.ExitCode, err)
		}
	}
}
func TestShellDetailedTimeoutAndCancel(t *testing.T) {
	s := Shell{Dir: t.TempDir(), Timeout: 20 * time.Millisecond}
	out, err := s.RunDetailed(context.Background(), "exec sleep 2")
	if err == nil || out.Category != "timeout" {
		t.Fatalf("category=%s err=%v", out.Category, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err = s.RunDetailed(ctx, "true")
	if err != context.Canceled || out.Category != "canceled" {
		t.Fatalf("category=%s err=%v", out.Category, err)
	}
}

func TestShellRunPreservesCanceledError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (Shell{Dir: t.TempDir()}).Run(ctx, "true")
	if err == nil || !strings.HasPrefix(err.Error(), "command failed:") {
		t.Fatalf("err=%v", err)
	}
}
