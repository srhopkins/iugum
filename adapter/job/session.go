package job

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/srhopkins/iugum/contract"
)

const (
	defaultSessionFile = "/home/iugum/session.id"
	defaultSessionCWD  = "/workspace"

	// defaultTimeout and defaultIdleTimeout are the jobs.yaml fallbacks:
	// timeout (hard ceiling) and idle_timeout (max silence). A jobs.yaml
	// without these fields, or with a value that fails to parse, gets these.
	defaultTimeout     = 4 * time.Hour
	defaultIdleTimeout = 10 * time.Minute
)

// SessionPrompt returns a JobFunc that injects text into the standing
// OpenCode session via ACP. timeoutStr and idleTimeoutStr are jobs.yaml's
// optional per-job timeout / idle_timeout fields (Go duration strings, e.g.
// "4h", "10m"); empty or unparsable falls back to defaultTimeout /
// defaultIdleTimeout.
func SessionPrompt(prompt, timeoutStr, idleTimeoutStr string) contract.JobFunc {
	text := prompt
	return func(ctx context.Context, _ contract.Event) error {
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("iugum: session job: empty prompt")
		}
		return promptSession(ctx, text, timeoutStr, idleTimeoutStr)
	}
}

// resolveJobDurations parses jobs.yaml's timeout / idle_timeout strings.
// Empty, non-positive, or unparsable input falls back to the default rather
// than erroring, so a malformed jobs.yaml can't wedge a job with a zero
// timeout, and a jobs.yaml written before these fields existed loads
// unchanged.
func resolveJobDurations(timeoutStr, idleTimeoutStr string) (timeout, idleTimeout time.Duration) {
	timeout = defaultTimeout
	if d, err := time.ParseDuration(timeoutStr); err == nil && d > 0 {
		timeout = d
	}
	idleTimeout = defaultIdleTimeout
	if d, err := time.ParseDuration(idleTimeoutStr); err == nil && d > 0 {
		idleTimeout = d
	}
	return timeout, idleTimeout
}

func promptSession(ctx context.Context, text, timeoutStr, idleTimeoutStr string) error {
	if _, err := exec.LookPath("opencode"); err != nil {
		return fmt.Errorf("iugum: session job: opencode not on PATH")
	}
	cwd := sessionCWD()
	file := sessionFile()
	timeout, idleTimeout := resolveJobDurations(timeoutStr, idleTimeoutStr)

	// hardCtx is the overall ceiling. runCtx is a child of it that the idle
	// watchdog can also cancel on its own, so either limit tears the process
	// down the same way (cmd.Cancel below, via exec.CommandContext).
	hardCtx, hardCancel := context.WithTimeout(ctx, timeout)
	defer hardCancel()
	runCtx, runCancel := context.WithCancel(hardCtx)
	defer runCancel()

	cmd := exec.CommandContext(runCtx, "opencode", "acp")
	cmd.Dir = cwd
	cmd.Env = os.Environ()
	// Put opencode in its own process group so a timeout kill can take the
	// whole group with it. Without this, exec.CommandContext's default
	// context-cancel behavior only signals the direct child: any grandchild
	// opencode spawns survives the kill, gets reparented to PID 1, and — if
	// the container's PID 1 never reaps it — sits forever as a zombie that
	// fools a name-based busy check on the next tick.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		killProcessGroup(cmd)
		return nil
	}
	// Bound how long Wait() waits after Cancel fires before giving up on the
	// pipes, so a wedged process can't also wedge this call forever.
	cmd.WaitDelay = 5 * time.Second

	// The stall watchdog treats three things as activity: a byte on stdout
	// or stderr, an ACP event (these ride the stdout JSON-RPC stream, so the
	// stdout wrapper below already covers them), and a file write under cwd
	// (the job's data dir: opencode's own working directory). idleTimeout
	// with none of those resets the process group same as a hard timeout.
	sw := newStallWatchdog(runCtx, hardCtx, runCancel, idleTimeout, cwd)
	defer sw.close()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = sw.wrapWriter(os.Stderr)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("iugum: session job: %w", err)
	}

	c := &acpClient{
		enc:     json.NewEncoder(stdin),
		pending: map[int]chan acpReply{},
	}
	errCh := make(chan error, 1)
	go func() { errCh <- c.read(sw.wrapReader(stdout)) }()

	if _, err := c.call(runCtx, "initialize", map[string]any{
		"protocolVersion": 1,
		"clientCapabilities": map[string]any{
			"fs": map[string]any{"readTextFile": true, "writeTextFile": false},
		},
	}); err != nil {
		killProcessGroup(cmd)
		return fmt.Errorf("iugum: session job: initialize: %w", err)
	}

	sessionID := readSessionID(file)
	if sessionID != "" {
		_, err := c.call(runCtx, "session/load", map[string]any{
			"cwd": cwd, "sessionId": sessionID, "mcpServers": []any{},
		})
		if err != nil {
			sessionID = ""
		}
	}
	if sessionID == "" {
		res, err := c.call(runCtx, "session/new", map[string]any{
			"cwd": cwd, "mcpServers": []any{},
		})
		if err != nil {
			killProcessGroup(cmd)
			return fmt.Errorf("iugum: session job: session/new: %w", err)
		}
		sessionID, _ = res["sessionId"].(string)
		if sessionID == "" {
			killProcessGroup(cmd)
			return fmt.Errorf("iugum: session job: session/new missing sessionId")
		}
		if err := writeSessionID(file, sessionID); err != nil {
			fmt.Fprintf(os.Stderr, "iugum: session job: write session id: %v\n", err)
		}
	}

	if model := sessionModel(); model != "" {
		if _, err := c.call(runCtx, "session/set_model", map[string]any{
			"sessionId": sessionID, "modelId": model,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "iugum: session job: set_model %s: %v\n", model, err)
		}
	}

	if prefix := wakePreamble(); prefix != "" {
		text = prefix + text
	}

	if _, err := c.call(runCtx, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]string{{"type": "text", "text": text}},
	}); err != nil {
		killProcessGroup(cmd)
		return fmt.Errorf("iugum: session job: prompt: %w", err)
	}

	_ = stdin.Close()
	waitErr := cmd.Wait()
	if reason := sw.reason(); reason != "" {
		fmt.Fprintf(os.Stderr, "iugum: session job: killed on %s limit\n", reason)
	}
	select {
	case readErr := <-errCh:
		if readErr != nil && waitErr == nil {
			return readErr
		}
	default:
	}
	return nil
}

// killProcessGroup kills cmd's whole process group, not just cmd itself, so
// a hung opencode session can't leave descendants behind for the container's
// init to miss reaping. Falls back to killing just the process if the group
// lookup fails (e.g. it already exited).
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		return
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}

// stallReason names which limit, if any, ended the job.
const (
	reasonTimeout = "timeout"
	reasonIdle    = "idle"
)

// activityTracker records the last time something happened, as a unix-nano
// timestamp so it's safe to touch and read from multiple goroutines without
// a mutex.
type activityTracker struct {
	lastNano atomic.Int64
}

func newActivityTracker() *activityTracker {
	a := &activityTracker{}
	a.touch()
	return a
}

func (a *activityTracker) touch() { a.lastNano.Store(time.Now().UnixNano()) }

func (a *activityTracker) idleFor() time.Duration {
	return time.Since(time.Unix(0, a.lastNano.Load()))
}

// stallWatchdog kills a session job's process group when either the hard
// timeout or the idle_timeout elapses first, and remembers which one fired
// so the caller can log it.
type stallWatchdog struct {
	act       *activityTracker
	hardCtx   context.Context
	idleFired atomic.Bool
	stop      func()
}

// newStallWatchdog starts watching. cancel is called the moment idleTimeout
// elapses with no activity; the caller is expected to wire cancel to the
// same context that cmd.Cancel tears the process group down on (so a hard
// timeout and an idle timeout both end the job the same way). watchDir is
// the job's data dir: a file write anywhere under it counts as activity.
func newStallWatchdog(runCtx, hardCtx context.Context, cancel context.CancelFunc, idleTimeout time.Duration, watchDir string) *stallWatchdog {
	sw := &stallWatchdog{act: newActivityTracker(), hardCtx: hardCtx}
	sw.stop = startStallTicker(runCtx, sw.act, idleTimeout, watchDir, func() {
		sw.idleFired.Store(true)
		cancel()
	})
	return sw
}

// wrapWriter returns a writer that forwards to w and counts any write as activity.
func (sw *stallWatchdog) wrapWriter(w io.Writer) io.Writer {
	return &activityWriter{w: w, touch: sw.act.touch}
}

// wrapReader returns a reader that forwards from r and counts any read as
// activity. Used on the ACP stdout pipe: every ACP event (session update,
// tool call, …) arrives as bytes on this stream, so counting bytes here
// covers "stdout" and "ACP event" with one signal.
func (sw *stallWatchdog) wrapReader(r io.Reader) io.Reader {
	return &activityReader{r: r, touch: sw.act.touch}
}

// reason reports which limit ended the job, if either did. Checked after
// cmd.Wait() returns. Priority goes to the hard timeout on a tie, since
// hardCtx is the ancestor context and reads as authoritative.
func (sw *stallWatchdog) reason() string {
	if sw.hardCtx.Err() == context.DeadlineExceeded {
		return reasonTimeout
	}
	if sw.idleFired.Load() {
		return reasonIdle
	}
	return ""
}

func (sw *stallWatchdog) close() { sw.stop() }

type activityWriter struct {
	w     io.Writer
	touch func()
}

func (a *activityWriter) Write(p []byte) (int, error) {
	n, err := a.w.Write(p)
	if n > 0 {
		a.touch()
	}
	return n, err
}

type activityReader struct {
	r     io.Reader
	touch func()
}

func (a *activityReader) Read(p []byte) (int, error) {
	n, err := a.r.Read(p)
	if n > 0 {
		a.touch()
	}
	return n, err
}

// startStallTicker polls act and calls onIdle once idleTimeout has elapsed
// with no touch. It also watches watchDir with fsnotify (the directory tree
// as it exists at call time — a best-effort signal, not a guarantee against
// a brand-new subdirectory created after startup) so a file write there
// counts as activity even when the process stays quiet on stdout/stderr.
// Returns a stop func; safe to call exactly once.
func startStallTicker(ctx context.Context, act *activityTracker, idleTimeout time.Duration, watchDir string, onIdle func()) func() {
	done := make(chan struct{})
	var once sync.Once
	stop := func() { once.Do(func() { close(done) }) }

	if watcher, err := fsnotify.NewWatcher(); err == nil {
		addWatchTree(watcher, watchDir)
		go func() {
			defer watcher.Close()
			for {
				select {
				case <-done:
					return
				case ev, ok := <-watcher.Events:
					if !ok {
						return
					}
					if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 {
						act.touch()
					}
				case _, ok := <-watcher.Errors:
					if !ok {
						return
					}
				}
			}
		}()
	}

	interval := idleTimeout / 5
	if interval < 20*time.Millisecond {
		interval = 20 * time.Millisecond
	}
	if interval > 2*time.Second {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if act.idleFor() >= idleTimeout {
					onIdle()
					return
				}
			}
		}
	}()
	return stop
}

// addWatchTree adds watchDir and every subdirectory under it to w. Errors
// (missing dir, permission) are ignored: the fsnotify signal is a bonus on
// top of the stdout/stderr activity signal, never the only one.
func addWatchTree(w *fsnotify.Watcher, watchDir string) {
	_ = filepath.WalkDir(watchDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // best-effort watch, see comment above
		}
		if d.IsDir() {
			_ = w.Add(path)
		}
		return nil
	})
}

func sessionFile() string {
	if p := os.Getenv("IUGUM_SESSION_FILE"); p != "" {
		return p
	}
	return defaultSessionFile
}

func sessionCWD() string {
	if p := os.Getenv("IUGUM_SESSION_CWD"); p != "" {
		return p
	}
	return defaultSessionCWD
}

func sessionModel() string {
	file := os.Getenv("IUGUM_SESSION_MODEL_FILE")
	if file == "" {
		file = "/data/MODEL"
	}
	if m := readTrimmed(file); m != "" {
		return m
	}
	if m := os.Getenv("OPENCODE_MODEL"); m != "" {
		return m
	}
	if m := os.Getenv("IUGUM_SESSION_MODEL"); m != "" {
		return m
	}
	return ""
}

func wakePreamble() string {
	p := os.Getenv("IUGUM_WAKE_PREAMBLE_FILE")
	if p == "" {
		p = "/data/WAKE-USAGE.txt"
	}
	s := readTrimmed(p)
	if s == "" {
		return ""
	}
	return "WAKE USAGE: " + s + "\n\n"
}

func readTrimmed(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func readSessionID(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func writeSessionID(path, id string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(id+"\n"), 0o644)
}

type acpReply struct {
	result map[string]any
	err    error
}

type acpClient struct {
	mu      sync.Mutex
	next    int
	enc     *json.Encoder
	pending map[int]chan acpReply
}

func (c *acpClient) call(ctx context.Context, method string, params any) (map[string]any, error) {
	c.mu.Lock()
	c.next++
	id := c.next
	ch := make(chan acpReply, 1)
	c.pending[id] = ch
	err := c.enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case reply := <-ch:
		return reply.result, reply.err
	}
}

func (c *acpClient) reply(id any, result any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (c *acpClient) read(r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if method, _ := msg["method"].(string); method != "" && msg["id"] != nil {
			c.handleRequest(msg)
			continue
		}
		if msg["id"] == nil {
			continue
		}
		id := jsonNumber(msg["id"])
		c.mu.Lock()
		ch := c.pending[id]
		delete(c.pending, id)
		c.mu.Unlock()
		if ch == nil {
			continue
		}
		if errObj, ok := msg["error"]; ok {
			ch <- acpReply{err: fmt.Errorf("%v", errObj)}
			continue
		}
		res, _ := msg["result"].(map[string]any)
		ch <- acpReply{result: res}
	}
	return sc.Err()
}

func (c *acpClient) handleRequest(msg map[string]any) {
	method, _ := msg["method"].(string)
	id := msg["id"]
	params, _ := msg["params"].(map[string]any)
	switch method {
	case "session/request_permission":
		c.reply(id, map[string]any{
			"outcome": map[string]any{"outcome": "selected", "optionId": "always"},
		})
	case "fs/read_text_file":
		path, _ := params["path"].(string)
		raw, err := os.ReadFile(path)
		if err != nil {
			c.reply(id, map[string]any{"content": ""})
			return
		}
		c.reply(id, map[string]any{"content": string(raw)})
	default:
		c.reply(id, map[string]any{})
	}
}

func jsonNumber(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}
