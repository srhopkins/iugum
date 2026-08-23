package execadapt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/plugin"
)

func init() {
	plugin.RegisterWiki("exec", func(cfg map[string]string) (contract.Wiki, error) {
		cmd := fields(cfg["command"])
		if len(cmd) == 0 {
			return nil, fmt.Errorf("iugum: exec wiki needs exec.wiki command")
		}
		return Wiki{cmd: cmd}, nil
	})
}

// Wiki is an external server that accepts -p PORT -L HOST SPACE like SilverBullet.
type Wiki struct{ cmd []string }

func (Wiki) Name() string { return "exec" }

func (w Wiki) Serve(_ context.Context, opts contract.WikiOpts) error {
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
	args := append(append([]string{}, w.cmd[1:]...), "-p", strconv.Itoa(opts.Port), "-L", opts.Host, opts.Space)
	c := exec.Command(w.cmd[0], args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

func fields(s string) []string {
	var out []string
	cur := ""
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(s[i])
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
