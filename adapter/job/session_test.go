package job

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestKillProcessGroupKillsGrandchild proves the fix for the zombie bug: on
// a session timeout, killProcessGroup must take down every descendant the
// child spawned, not just the child itself. A plain cmd.Process.Kill() (the
// old behavior) only signals the direct child; a background grandchild
// survives, gets reparented to PID 1, and — if PID 1 never reaps it — stays
// forever as a zombie that fools a name-based busy check on the next tick.
func TestKillProcessGroupKillsGrandchild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process groups are unix-only")
	}

	dir := t.TempDir()
	pidFile := filepath.Join(dir, "grandchild.pid")

	// The shell backgrounds a long sleep (the "grandchild") and then blocks
	// forever, standing in for opencode wedging with a spawned subprocess.
	script := "sleep 30 & echo $! > " + pidFile + "; wait"

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		killProcessGroup(cmd)
		return nil
	}
	cmd.WaitDelay = 5 * time.Second

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	_ = cmd.Wait() // expected to report the context-cancel kill, not a clean exit

	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read grandchild pid file: %v", err)
	}
	grandchildPID, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("parse grandchild pid: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(grandchildPID, 0); err != nil {
			return // ESRCH: grandchild is gone, the group kill worked
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("grandchild pid %d still alive after process-group kill on timeout", grandchildPID)
}

// TestKillProcessGroupNilProcess guards the nil-Process no-op path so a
// double-cancel or a kill called before Start can't panic.
func TestKillProcessGroupNilProcess(t *testing.T) {
	cmd := exec.Command("true")
	killProcessGroup(cmd) // must not panic on a never-started cmd
}
