// Package agentplatform owns coding-agent sessions behind the public Go contract.
package agentplatform

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

type frame struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type rpc struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[string]chan frame
	next    atomic.Uint64
	done    chan struct{}
	once    sync.Once
	notify  func(frame)
	request func(frame) (json.RawMessage, error)
}

func startRPC(ctx context.Context, binary string, args []string, cwd string, notify func(frame), request func(frame) (json.RawMessage, error)) (*rpc, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = cwd
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return nil, err
	}
	// CLI logs can contain prompts and credentials. Only protocol data is consumed.
	cmd.Stderr = io.Discard
	r := &rpc{cmd: cmd, in: in, pending: map[string]chan frame{}, done: make(chan struct{}), notify: notify, request: request}
	return r, r.start(out)
}
func (r *rpc) start(out io.ReadCloser) error {
	if err := r.cmd.Start(); err != nil {
		r.in.Close()
		out.Close()
		return err
	}
	go func() {
		scanner := bufio.NewScanner(out)
		scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
		for scanner.Scan() {
			var f frame
			if json.Unmarshal(scanner.Bytes(), &f) != nil {
				continue
			}
			if len(f.ID) > 0 && f.Method != "" {
				go r.handle(f)
				continue
			}
			if len(f.ID) > 0 {
				r.mu.Lock()
				ch := r.pending[string(f.ID)]
				r.mu.Unlock()
				if ch != nil {
					select {
					case ch <- f:
					default:
					}
				}
				continue
			}
			if r.notify != nil {
				r.notify(f)
			}
		}
		r.close()
		_ = r.cmd.Wait()
	}()
	return nil
}
func (r *rpc) handle(f frame) {
	result, err := json.RawMessage(nil), errors.New("unsupported client capability; no approval granted")
	if r.request != nil {
		result, err = r.request(f)
	}
	if err == nil && len(result) == 0 {
		result = json.RawMessage("null")
	}
	response := frame{JSONRPC: "2.0", ID: f.ID, Result: result}
	if err != nil {
		response.Error = &rpcError{Code: -32601, Message: err.Error()}
		response.Result = nil
	}
	_ = r.write(response)
}
func (r *rpc) write(value any) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	select {
	case <-r.done:
		return errors.New("agent connection closed; inspect history before resending")
	default:
	}
	return json.NewEncoder(r.in).Encode(value)
}
func (r *rpc) call(ctx context.Context, method string, params any, result any) error {
	// All operations are bounded, but a prompt's caller can use a longer deadline.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
	}
	id := fmt.Sprint(r.next.Add(1))
	ch := make(chan frame, 1)
	r.mu.Lock()
	r.pending[id] = ch
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.pending, id); r.mu.Unlock() }()
	if err := r.write(map[string]any{"jsonrpc": "2.0", "id": json.Number(id), "method": method, "params": params}); err != nil {
		return err
	}
	select {
	case f := <-ch:
		if f.Error != nil {
			return fmt.Errorf("%s: %s", method, f.Error.Message)
		}
		if result != nil {
			return json.Unmarshal(f.Result, result)
		}
		return nil
	case <-r.done:
		return errors.New("agent connection closed; inspect history before resending")
	case <-ctx.Done():
		r.close()
		return fmt.Errorf("%s: %w; delivery may be uncertain; not retried", method, ctx.Err())
	}
}
func (r *rpc) notification(method string, params any) error {
	return r.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}
func (r *rpc) close() {
	r.once.Do(func() {
		close(r.done)
		r.in.Close()
		if r.cmd.Process != nil {
			_ = r.cmd.Process.Kill()
		}
	})
}
