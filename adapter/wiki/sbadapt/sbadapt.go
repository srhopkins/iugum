package sbadapt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/embedbin"
	"github.com/srhopkins/iugum/plugin"
)

func init() {
	plugin.RegisterWiki("silverbullet", func(map[string]string) (contract.Wiki, error) {
		return Wiki{}, nil
	})
}

// Wiki extracts the embedded SilverBullet binary and runs it.
type Wiki struct{}

func (Wiki) Name() string { return "silverbullet" }

func (Wiki) Serve(_ context.Context, opts contract.WikiOpts) error {
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
	cmd := exec.Command(path, "--single", "-p", strconv.Itoa(opts.Port), "-L", opts.Host, opts.Space)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wiki: %w", err)
	}
	return nil
}
