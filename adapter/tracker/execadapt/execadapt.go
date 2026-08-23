package execadapt

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/plugin"
)

func init() {
	plugin.RegisterTracker("exec", func(cfg map[string]string) (contract.Tracker, error) {
		cmd := splitCmd(cfg["command"])
		if len(cmd) == 0 {
			return nil, fmt.Errorf("iugum: exec tracker needs exec.tracker command")
		}
		return Tracker{cmd: cmd}, nil
	})
}

// Tracker is an external binary that speaks the same CLI as bd.
type Tracker struct{ cmd []string }

func (Tracker) Name() string { return "exec" }

func (t Tracker) Run(_ context.Context, args []string) error {
	c := exec.Command(t.cmd[0], append(t.cmd[1:], args...)...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

func splitCmd(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range splitWS(s) {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitWS(s string) []string {
	return fields(s)
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
