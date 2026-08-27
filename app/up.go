package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/embedbin"
)

// ServeWikiSupervised runs the wiki and stops it when ctx ends.
// The built-in SilverBullet adapter ignores ctx. This helper runs the same
// embedded binary as a child that dies with ctx. Other wiki adapters use Serve.
func (a *App) ServeWikiSupervised(ctx context.Context, opts contract.WikiOpts) error {
	if err := a.Check(ctx, "wiki", "serve"); err != nil {
		return err
	}
	if a.Wiki == nil || a.Wiki.Name() != "silverbullet" {
		return a.Wiki.Serve(ctx, opts)
	}
	if opts.Port == 0 {
		opts.Port = 3000
	}
	if opts.Host == "" {
		opts.Host = "127.0.0.1"
	}
	if opts.Space == "" {
		opts.Space = "./wiki"
	}
	if err := os.MkdirAll(opts.Space, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp("", "iugum-silverbullet-*")
	if err != nil {
		return err
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err := f.Write(embedbin.Silverbullet); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(0o700); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	args := []string{"-p", strconv.Itoa(opts.Port), "-L", opts.Host, opts.Space}
	return runChild(ctx, "wiki", path, args...)
}

// runChild runs one child process until it exits or ctx ends.
// On ctx end it sends SIGTERM, then SIGKILL after a short delay.
// A child that ends because ctx ended returns nil.
func runChild(ctx context.Context, label, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 3 * time.Second
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return fmt.Errorf("%s: exited", label)
}

// RunCodeServer starts code-server as a child process until ctx ends.
// The caller checks the gate and confirms the binary exists.
func (a *App) RunCodeServer(ctx context.Context, bin, host string, port int, dir string) error {
	addr := host + ":" + strconv.Itoa(port)
	return runChild(ctx, "code-server", bin, "--bind-addr", addr, "--auth", "none", dir)
}
