// Package iptables renders contract.NetRules as iptables commands.
// Plan gives one shell-ready line per command. Apply runs each line with exec.
package iptables

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/plugin"
)

func init() {
	plugin.RegisterNet("iptables", func(cfg map[string]string) (contract.Net, error) {
		bin := cfg["bin"]
		if bin == "" {
			bin = "iptables"
		}
		return &Net{Bin: bin}, nil
	})
}

// Net is the iptables backend. Bin is the iptables program name.
type Net struct {
	Bin string
}

func (n *Net) Name() string { return "iptables" }

// Plan renders the rule set. Order: flush INPUT and OUTPUT, set chain policy,
// accept loopback, then one -A line per rule.
func (n *Net) Plan(_ context.Context, rules contract.NetRules) ([]string, error) {
	if err := validate(rules); err != nil {
		return nil, err
	}
	bin := n.Bin
	if bin == "" {
		bin = "iptables"
	}
	out := []string{
		bin + " -F INPUT",
		bin + " -F OUTPUT",
		bin + " -P INPUT " + target(rules.Default.In, "allow"),
		bin + " -P OUTPUT " + target(rules.Default.Out, "allow"),
	}
	if rules.Default.In == "deny" {
		out = append(out, bin+" -A INPUT -i lo -j ACCEPT")
	}
	if rules.Default.Out == "deny" {
		out = append(out, bin+" -A OUTPUT -o lo -j ACCEPT")
	}
	for _, r := range rules.Rules {
		out = append(out, bin+" "+strings.Join(renderRule(r), " "))
	}
	return out, nil
}

// Apply runs each planned line as its own process.
func (n *Net) Apply(ctx context.Context, rules contract.NetRules) error {
	lines, err := n.Plan(ctx, rules)
	if err != nil {
		return err
	}
	for _, line := range lines {
		argv := strings.Fields(line)
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("iugum net: %s: %v: %s", line, err, strings.TrimSpace(stderr.String()))
		}
	}
	return nil
}

// Show returns iptables -S.
func (n *Net) Show(ctx context.Context) (string, error) {
	bin := n.Bin
	if bin == "" {
		bin = "iptables"
	}
	out, err := exec.CommandContext(ctx, bin, "-S").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("iugum net: %s -S: %v: %s", bin, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// renderRule gives the argv after the program name for one rule.
func renderRule(r contract.NetRule) []string {
	chain := "INPUT"
	if r.Dir == "out" {
		chain = "OUTPUT"
	}
	args := []string{"-A", chain}
	if r.Proto != "" && r.Proto != "all" {
		args = append(args, "-p", r.Proto)
	}
	if r.Port > 0 && (r.Proto == "tcp" || r.Proto == "udp") {
		args = append(args, "--dport", fmt.Sprint(r.Port))
	}
	if r.Src != "" {
		args = append(args, "-s", r.Src)
	}
	if r.Dst != "" {
		args = append(args, "-d", r.Dst)
	}
	return append(args, "-j", target(r.Action, "allow"))
}

func target(action, def string) string {
	if action == "" {
		action = def
	}
	if action == "deny" {
		return "DROP"
	}
	return "ACCEPT"
}

// validate rejects values the renderer cannot express.
func validate(rules contract.NetRules) error {
	for _, v := range []string{rules.Default.In, rules.Default.Out} {
		if v != "" && v != "allow" && v != "deny" {
			return fmt.Errorf("iugum net: default must be allow or deny, got %q", v)
		}
	}
	for _, r := range rules.Rules {
		if r.Dir != "in" && r.Dir != "out" {
			return fmt.Errorf("iugum net: rule %q: dir must be in or out", r.Name)
		}
		switch r.Proto {
		case "", "tcp", "udp", "icmp", "all":
		default:
			return fmt.Errorf("iugum net: rule %q: proto must be tcp, udp, icmp, or all", r.Name)
		}
		if r.Action != "" && r.Action != "allow" && r.Action != "deny" {
			return fmt.Errorf("iugum net: rule %q: action must be allow or deny", r.Name)
		}
		if r.Port < 0 || r.Port > 65535 {
			return fmt.Errorf("iugum net: rule %q: port out of range", r.Name)
		}
		for _, s := range []string{r.Name, r.Src, r.Dst} {
			if strings.ContainsAny(s, " \t\n\"'`$;|&") {
				return fmt.Errorf("iugum net: rule %q: unsafe character in value %q", r.Name, s)
			}
		}
	}
	return nil
}

// Validate is the shared check for any backend.
func Validate(rules contract.NetRules) error { return validate(rules) }
