// Package nftables renders contract.NetRules as one nft ruleset in table inet iugum.
// Plan gives the ruleset text as a single string. Apply pipes it to nft -f -.
package nftables

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/srhopkins/iugum/adapter/net/iptables"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/plugin"
)

func init() {
	plugin.RegisterNet("nftables", func(cfg map[string]string) (contract.Net, error) {
		bin := cfg["bin"]
		if bin == "" {
			bin = "nft"
		}
		return &Net{Bin: bin}, nil
	})
}

// Net is the nftables backend. Bin is the nft program name.
type Net struct {
	Bin string
}

func (n *Net) Name() string { return "nftables" }

// Plan renders one ruleset. The first two lines make the table idempotent:
// create it when absent, then flush it, then declare the chains.
func (n *Net) Plan(_ context.Context, rules contract.NetRules) ([]string, error) {
	if err := iptables.Validate(rules); err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("table inet iugum {}\n")
	b.WriteString("flush table inet iugum\n")
	b.WriteString("table inet iugum {\n")
	writeChain(&b, "input", rules.Default.In, "iif lo accept", rules.Rules, "in")
	writeChain(&b, "output", rules.Default.Out, "oif lo accept", rules.Rules, "out")
	b.WriteString("}\n")
	return []string{b.String()}, nil
}

func writeChain(b *strings.Builder, chain, def, lo string, rules []contract.NetRule, dir string) {
	fmt.Fprintf(b, "\tchain %s {\n", chain)
	fmt.Fprintf(b, "\t\ttype filter hook %s priority 0; policy %s;\n", chain, verdict(def))
	if def == "deny" {
		fmt.Fprintf(b, "\t\t%s\n", lo)
	}
	for _, r := range rules {
		if r.Dir != dir {
			continue
		}
		fmt.Fprintf(b, "\t\t%s\n", renderRule(r))
	}
	b.WriteString("\t}\n")
}

// renderRule gives one nft statement for a rule.
func renderRule(r contract.NetRule) string {
	var parts []string
	if r.Src != "" {
		parts = append(parts, ipFamily(r.Src)+" saddr "+r.Src)
	}
	if r.Dst != "" {
		parts = append(parts, ipFamily(r.Dst)+" daddr "+r.Dst)
	}
	switch r.Proto {
	case "tcp", "udp":
		if r.Port > 0 {
			parts = append(parts, fmt.Sprintf("%s dport %d", r.Proto, r.Port))
		} else {
			parts = append(parts, "meta l4proto "+r.Proto)
		}
	case "icmp":
		parts = append(parts, "meta l4proto { icmp, ipv6-icmp }")
	}
	parts = append(parts, verdict(r.Action))
	if r.Name != "" {
		parts = append(parts, fmt.Sprintf("comment %q", r.Name))
	}
	return strings.Join(parts, " ")
}

func ipFamily(addr string) string {
	if strings.Contains(addr, ":") {
		return "ip6"
	}
	return "ip"
}

func verdict(action string) string {
	if action == "deny" {
		return "drop"
	}
	return "accept"
}

// Apply pipes the planned ruleset to nft -f -.
func (n *Net) Apply(ctx context.Context, rules contract.NetRules) error {
	lines, err := n.Plan(ctx, rules)
	if err != nil {
		return err
	}
	bin := n.bin()
	cmd := exec.CommandContext(ctx, bin, "-f", "-")
	cmd.Stdin = strings.NewReader(strings.Join(lines, ""))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("iugum net: %s -f -: %v: %s", bin, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// Show returns nft list ruleset.
func (n *Net) Show(ctx context.Context) (string, error) {
	bin := n.bin()
	out, err := exec.CommandContext(ctx, bin, "list", "ruleset").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("iugum net: %s list ruleset: %v: %s", bin, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (n *Net) bin() string {
	if n.Bin == "" {
		return "nft"
	}
	return n.Bin
}
