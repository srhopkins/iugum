package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/srhopkins/iugum/config"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/plugin"
)

// wireNet builds the Net slot from the network: block.
// backend off or absent leaves a.Net nil. auto picks nftables when nft is on PATH.
func (a *App) wireNet(nc config.Network) error {
	backend := nc.Backend
	switch backend {
	case "", "off":
		return nil
	case "auto":
		backend = "iptables"
		if _, err := exec.LookPath("nft"); err == nil {
			backend = "nftables"
		}
	}
	n, err := plugin.NewNet(backend, nil)
	if err != nil {
		return err
	}
	a.Net = n
	a.NetRules = toContract(nc)
	return nil
}

func toContract(nc config.Network) contract.NetRules {
	out := contract.NetRules{Default: contract.NetDefault{In: nc.Default.In, Out: nc.Default.Out}}
	for _, r := range nc.Rules {
		out.Rules = append(out.Rules, contract.NetRule{
			Name: r.Name, Dir: r.Dir, Proto: r.Proto, Port: r.Port,
			Src: r.Src, Dst: r.Dst, Action: r.Action,
		})
	}
	return out
}

// PlanNet renders the rule set. Backend off returns nil, nil.
func (a *App) PlanNet(ctx context.Context) ([]string, error) {
	if err := a.Check(ctx, "net", "plan"); err != nil {
		return nil, err
	}
	if a.Net == nil {
		return nil, nil
	}
	return a.Net.Plan(ctx, a.NetRules)
}

// DryRunNet passes the apply gate and renders the rule set with no side effect.
func (a *App) DryRunNet(ctx context.Context) ([]string, error) {
	if err := a.Check(ctx, "net", "apply"); err != nil {
		return nil, err
	}
	if a.Net == nil {
		return nil, nil
	}
	return a.Net.Plan(ctx, a.NetRules)
}

// ApplyNet writes the rule set to the host firewall. Backend off is a no-op.
// Linux only. The process needs CAP_NET_ADMIN.
func (a *App) ApplyNet(ctx context.Context) error {
	if err := a.Check(ctx, "net", "apply"); err != nil {
		return err
	}
	if a.Net == nil {
		return nil
	}
	if err := netAdminOK(); err != nil {
		return err
	}
	return a.Net.Apply(ctx, a.NetRules)
}

// ShowNet reads the live ruleset from the backend.
func (a *App) ShowNet(ctx context.Context) (string, error) {
	if err := a.Check(ctx, "net", "show"); err != nil {
		return "", err
	}
	if a.Net == nil {
		return "", nil
	}
	return a.Net.Show(ctx)
}

// ErrNetLinuxOnly is the apply error on a non-linux OS.
var ErrNetLinuxOnly = errors.New("iugum net apply: linux only (this OS has no iptables or nftables); use plan or apply --dry-run")

// netAdminOK returns nil when the process may change the firewall.
func netAdminOK() error {
	if runtime.GOOS != "linux" {
		return ErrNetLinuxOnly
	}
	if hasCapNetAdmin() {
		return nil
	}
	return errors.New("iugum net apply: this process has no CAP_NET_ADMIN; run as root or with --cap-add NET_ADMIN")
}

// hasCapNetAdmin reads CapEff from /proc/self/status and tests bit 12 (CAP_NET_ADMIN).
// When the file is unreadable it falls back to uid 0.
func hasCapNetAdmin() bool {
	raw, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return os.Geteuid() == 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(line, "CapEff:") {
			continue
		}
		v, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "CapEff:")), 16, 64)
		if err != nil {
			return os.Geteuid() == 0
		}
		return v&(1<<12) != 0
	}
	return os.Geteuid() == 0
}

// NetBackendName is the active backend name or "off".
func (a *App) NetBackendName() string {
	if a.Net == nil {
		return "off"
	}
	return fmt.Sprint(a.Net.Name())
}
