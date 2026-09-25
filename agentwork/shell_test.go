package agentwork

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestShellRunsInConfiguredDirectory(t *testing.T) {
	s := Shell{Dir: t.TempDir()}
	out, err := s.Run(context.Background(), "pwd")
	if err != nil || strings.TrimSpace(out) != s.Dir {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestShellBoundsOutputAndTimeout(t *testing.T) {
	s := Shell{Dir: t.TempDir(), MaxOutput: 5, Timeout: 20 * time.Millisecond}
	out, err := s.Run(context.Background(), "printf 123456")
	if err != nil || out != "12345\n[output truncated]" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	failed, err := (Shell{Dir: t.TempDir()}).Run(context.Background(), "printf failed >&2; exit 7")
	if err != nil || failed != "failed\n[exit status 7]" {
		t.Fatalf("failed=%q err=%v", failed, err)
	}
	_, err = s.Run(context.Background(), "sleep 1")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatal(err)
	}
}
