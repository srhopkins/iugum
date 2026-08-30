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
	"syscall"
	"time"

	"github.com/srhopkins/iugum/contract"
)

const (
	defaultSessionFile = "/home/iugum/session.id"
	defaultSessionCWD  = "/workspace"
	sessionTimeout     = 10 * time.Minute
)

// SessionPrompt returns a JobFunc that injects text into the standing OpenCode session via ACP.
func SessionPrompt(prompt string) contract.JobFunc {
	text := prompt
	return func(ctx context.Context, _ contract.Event) error {
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("iugum: session job: empty prompt")
		}
		return promptSession(ctx, text)
	}
}

func promptSession(ctx context.Context, text string) error {
	if _, err := exec.LookPath("opencode"); err != nil {
		return fmt.Errorf("iugum: session job: opencode not on PATH")
	}
	cwd := sessionCWD()
	file := sessionFile()

	runCtx, cancel := context.WithTimeout(ctx, sessionTimeout)
	defer cancel()

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
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("iugum: session job: %w", err)
	}

	c := &acpClient{
		enc:     json.NewEncoder(stdin),
		pending: map[int]chan acpReply{},
	}
	errCh := make(chan error, 1)
	go func() { errCh <- c.read(stdout) }()

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
