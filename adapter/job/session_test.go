package job

import (
	"bufio"
	"context"
	"io"
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

// runStallTest wires a *stallWatchdog around script the same way promptSession
// wires one around opencode: cmd.Cancel kills the whole process group,
// stdout is scanned line-by-line (each line counts as ACP/stdout activity),
// and stderr is wrapped so its bytes count too. It returns the wait error
// and whichever limit (if any) the watchdog fired.
func runStallTest(t *testing.T, script string, timeout, idleTimeout time.Duration) (waitErr error, reason string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("process groups are unix-only")
	}
	dir := t.TempDir()

	hardCtx, hardCancel := context.WithTimeout(context.Background(), timeout)
	defer hardCancel()
	runCtx, runCancel := context.WithCancel(hardCtx)
	defer runCancel()

	sw := newStallWatchdog(runCtx, hardCtx, runCancel, idleTimeout, dir)
	defer sw.close()

	cmd := exec.CommandContext(runCtx, "sh", "-c", script)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		killProcessGroup(cmd)
		return nil
	}
	cmd.WaitDelay = 5 * time.Second
	cmd.Stderr = sw.wrapWriter(io.Discard)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		sc := bufio.NewScanner(sw.wrapReader(stdout))
		for sc.Scan() {
			// touch() already happened inside wrapReader's Read; nothing
			// else to do per line.
			_ = sc.Text()
		}
	}()

	waitErr = cmd.Wait()
	<-scanDone
	return waitErr, sw.reason()
}

// TestStallWatchdogChattyJobSurvivesShortIdleTimeout proves stdout activity
// resets the idle clock: a job that would die almost immediately under a
// naive short fixed timeout survives as long as it keeps printing, because
// each line pushes idleFor() back to zero.
func TestStallWatchdogChattyJobSurvivesShortIdleTimeout(t *testing.T) {
	script := "for i in $(seq 1 8); do echo tick; sleep 0.05; done"
	waitErr, reason := runStallTest(t, script, 5*time.Second, 150*time.Millisecond)
	if waitErr != nil {
		t.Fatalf("expected clean exit, got %v (reason=%q)", waitErr, reason)
	}
	if reason != "" {
		t.Fatalf("expected no stall reason for a chatty job, got %q", reason)
	}
}

// TestStallWatchdogSilentJobDiesOnIdleTimeout proves a job with no stdout,
// stderr, or file activity gets killed once idle_timeout elapses, and that
// the watchdog reports "idle" as the cause.
func TestStallWatchdogSilentJobDiesOnIdleTimeout(t *testing.T) {
	start := time.Now()
	waitErr, reason := runStallTest(t, "sleep 5", 5*time.Second, 150*time.Millisecond)
	elapsed := time.Since(start)

	if reason != reasonIdle {
		t.Fatalf("want reason %q, got %q (waitErr=%v)", reasonIdle, reason, waitErr)
	}
	if waitErr == nil {
		t.Fatalf("expected the idle kill to surface as a wait error")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("idle watchdog did not fire promptly: took %v", elapsed)
	}
}

// TestStallWatchdogChattyJobDiesOnHardTimeout proves stdout activity does
// not extend the hard timeout: a job that never goes idle still dies once
// the overall ceiling elapses, and the watchdog reports "timeout".
func TestStallWatchdogChattyJobDiesOnHardTimeout(t *testing.T) {
	script := "while true; do echo tick; sleep 0.02; done"
	start := time.Now()
	waitErr, reason := runStallTest(t, script, 150*time.Millisecond, 5*time.Second)
	elapsed := time.Since(start)

	if reason != reasonTimeout {
		t.Fatalf("want reason %q, got %q (waitErr=%v)", reasonTimeout, reason, waitErr)
	}
	if waitErr == nil {
		t.Fatalf("expected the timeout kill to surface as a wait error")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("hard timeout watchdog did not fire promptly: took %v", elapsed)
	}
}

// TestResolveJobDurationsDefaults proves jobs.yaml entries without timeout /
// idle_timeout — the pre-existing shape — resolve to the documented
// defaults, so old jobs.yaml files keep working unchanged.
func TestResolveJobDurationsDefaults(t *testing.T) {
	timeout, idleTimeout := resolveJobDurations("", "")
	if timeout != defaultTimeout {
		t.Errorf("timeout: want %v, got %v", defaultTimeout, timeout)
	}
	if idleTimeout != defaultIdleTimeout {
		t.Errorf("idleTimeout: want %v, got %v", defaultIdleTimeout, idleTimeout)
	}
}

// TestResolveJobDurationsParsesOverrides proves explicit jobs.yaml values win.
func TestResolveJobDurationsParsesOverrides(t *testing.T) {
	timeout, idleTimeout := resolveJobDurations("30s", "5s")
	if timeout != 30*time.Second {
		t.Errorf("timeout: want 30s, got %v", timeout)
	}
	if idleTimeout != 5*time.Second {
		t.Errorf("idleTimeout: want 5s, got %v", idleTimeout)
	}
}

// TestResolveJobDurationsFallsBackOnBadValue proves a malformed or
// non-positive value falls back to the default instead of erroring or
// producing a zero/negative duration that would fire immediately.
func TestResolveJobDurationsFallsBackOnBadValue(t *testing.T) {
	timeout, idleTimeout := resolveJobDurations("not-a-duration", "-5s")
	if timeout != defaultTimeout {
		t.Errorf("timeout: want default %v, got %v", defaultTimeout, timeout)
	}
	if idleTimeout != defaultIdleTimeout {
		t.Errorf("idleTimeout: want default %v, got %v", defaultIdleTimeout, idleTimeout)
	}
}
