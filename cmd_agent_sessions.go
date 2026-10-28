package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/srhopkins/iugum/agentcontrol"
	"github.com/srhopkins/iugum/agentplatform"
	"github.com/srhopkins/iugum/app"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/policy"
)

func runAgentSessions(ctx context.Context, a *app.App, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "setup" || args[0] == "start" || args[0] == "wait-ready") {
		return runAgentSessionLifecycle(ctx, a, args, stdout, stderr)
	}
	if len(args) == 0 || args[0] != "serve" {
		fmt.Fprintln(stderr, "Usage: iugum agent sessions serve [--listen 127.0.0.1:3853] [--data-dir DIR] [--codex-bin PATH] [--claude-bin PATH --claude-actor ACTOR --claude-account EMAIL --claude-config-dir DIR --claude-model MODEL --claude-policy FILE] [--opencode-url URL --opencode-password-file FILE] [--cursor-bin PATH] [--native-url URL --native-cwd DIR] [--coordination] [--host-tools] [--browser-grants]")
		return 2
	}
	home, _ := os.UserHomeDir()
	fs := flag.NewFlagSet("agent sessions serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", "127.0.0.1:3853", "loopback listener")
	dir := fs.String("data-dir", filepath.Join(home, ".local", "share", "iugum", "provider-sessions"), "private session service state")
	codex := fs.String("codex-bin", "codex", "Codex executable")
	claude := fs.String("claude-bin", "claude", "Claude executable (enabled only with explicit account configuration)")
	actor := fs.String("claude-actor", "", "Claude policy actor")
	account := fs.String("claude-account", "", "expected Claude subscription login email")
	configDir := fs.String("claude-config-dir", "", "absolute Claude account configuration directory")
	model := fs.String("claude-model", "", "explicit Claude model")
	policyFile := fs.String("claude-policy", "", "Claude session policy CSV, reloaded for every operation")
	endpoint := fs.String("opencode-url", "", "existing authenticated loopback OpenCode server")
	username := fs.String("opencode-username", "opencode", "OpenCode server username")
	passwordFile := fs.String("opencode-password-file", "", "private OpenCode password file (otherwise OPENCODE_SERVER_PASSWORD)")
	coordination := fs.Bool("coordination", false, "expose optional policy-gated coordination MCP tools (owner enable still required)")
	hostTools := fs.Bool("host-tools", false, "expose optional exact editor-window MCP tools; requires extension adapter, no browser mouse connector")
	browserGrants := fs.Bool("browser-grants", false, "opt in to exact paired browser grant routes; requires host-tools, static application Casbin and trusted client ceremony")
	nativeURL := fs.String("native-url", "", "existing loopback native Iugum listener (optional)")
	nativeCwd := fs.String("native-cwd", "", "absolute native runtime working directory")
	cursor := fs.String("cursor-bin", "cursor-agent", "Cursor CLI executable")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *browserGrants && !*hostTools {
		fmt.Fprintln(stderr, "browser-grants requires host-tools")
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected session arguments")
		return 2
	}
	host, _, err := net.SplitHostPort(*listen)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() {
		fmt.Fprintln(stderr, "session listener must use a loopback IP address")
		return 2
	}
	if *endpoint == "" {
		*endpoint = os.Getenv("IUGUM_OPENCODE_URL")
	}
	if a.Gate == nil {
		fmt.Fprintln(stderr, "session application policy required")
		return 2
	}
	if err = a.Check(ctx, "agent/sessions", "serve"); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *browserGrants {
		static, ok := a.Gate.(*policy.Gate)
		if !ok || static == nil || a.Actor == "" {
			fmt.Fprintln(stderr, "browser-grants requires an explicit actor and immutable actual application Casbin")
			return 2
		}
		if err = a.Check(ctx, "agent/browser", "serve"); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	providers, err := sessionProviders(sessionProviderConfig{NativeURL: *nativeURL, NativeCwd: *nativeCwd, DataDir: *dir, CodexBinary: *codex, CursorBinary: *cursor, ClaudeBinary: *claude, ClaudeActor: *actor, ClaudeAccount: *account, ClaudeConfigDir: *configDir, ClaudeModel: *model, ClaudePolicy: *policyFile, OpenCodeURL: *endpoint, OpenCodeUsername: *username, OpenCodePassword: os.Getenv("OPENCODE_SERVER_PASSWORD"), OpenCodePasswordFile: *passwordFile}, a.Gate)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if *account == "" {
		fmt.Fprintln(stderr, "Claude unavailable: explicit actor/account/config/model/policy required")
	}
	if *endpoint == "" {
		fmt.Fprintln(stderr, "OpenCode unavailable: configure an authenticated existing loopback server")
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer listener.Close()
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	service, err := agentplatform.New(ctx, agentplatform.Config{BrowserGrants: *browserGrants, BrowserActor: a.Actor, BrowserPolicy: a.Gate, HostTools: *hostTools, Coordination: *coordination, DataDir: *dir, Home: home, Check: a.Check, Providers: providers})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer service.Close()
	server := &http.Server{Handler: service.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	fmt.Fprintf(stdout, "Agent sessions: http://%s\nControl token file: %s\n", listener.Addr(), service.TokenFile())
	if err = server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// sessionProviderConfig never crosses the HTTP boundary or enters log output.
type sessionProviderConfig struct {
	NativeURL, NativeCwd                                                                 string
	DataDir, CodexBinary, CursorBinary                                                   string
	ClaudeBinary, ClaudeActor, ClaudeAccount, ClaudeConfigDir, ClaudeModel, ClaudePolicy string
	OpenCodeURL, OpenCodeUsername, OpenCodePassword, OpenCodePasswordFile                string
}
type sessionPolicy struct {
	host   contract.Policy
	claude contract.Policy
}

func (p sessionPolicy) Enforce(ctx context.Context, r contract.Request) error {
	if p.host == nil {
		return errors.New("session host policy required")
	}
	if err := p.host.Enforce(ctx, r); err != nil {
		return err
	}
	return p.claude.Enforce(ctx, r)
}
func sessionProviders(c sessionProviderConfig, host contract.Policy) ([]contract.AgentSessionProvider, error) {
	out := []contract.AgentSessionProvider{agentplatform.Provider{Provider: "Codex", Binary: c.CodexBinary}, agentplatform.Provider{Provider: "Cursor", Binary: c.CursorBinary}}
	if c.NativeURL != "" || c.NativeCwd != "" {
		p := agentplatform.NativeHTTPProvider{BaseURL: c.NativeURL, Cwd: c.NativeCwd}
		if err := p.Validate(); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	enabled := c.ClaudeActor != "" || c.ClaudeAccount != "" || c.ClaudeConfigDir != "" || c.ClaudeModel != "" || c.ClaudePolicy != ""
	if enabled {
		if host == nil {
			return nil, errors.New("Claude requires application policy")
		}
		address, e := mail.ParseAddress(c.ClaudeAccount)
		if c.ClaudeActor == "" || e != nil || address.Address != c.ClaudeAccount || !filepath.IsAbs(c.ClaudeConfigDir) || c.ClaudeModel == "" || c.ClaudePolicy == "" {
			return nil, errors.New("Claude requires actor, login email, absolute config directory, model and policy file")
		}
		if _, e := os.Stat(c.ClaudePolicy); e != nil {
			return nil, errors.New("Claude policy file unavailable")
		}
		if _, e := policy.New("", c.ClaudePolicy); e != nil {
			return nil, errors.New("Claude policy file invalid")
		}
		st, e := os.Stat(c.ClaudeConfigDir)
		if e != nil || !st.IsDir() {
			return nil, errors.New("Claude config directory unavailable")
		}
		m, e := agentcontrol.New(filepath.Join(c.DataDir, "claude-control"), sessionPolicy{host, policy.Live{Path: c.ClaudePolicy}})
		if e != nil {
			return nil, e
		}
		m.Executable = c.ClaudeBinary
		out = append(out, agentplatform.ClaudeProvider{Manager: m, Actor: c.ClaudeActor, Account: c.ClaudeAccount, ConfigDir: c.ClaudeConfigDir, Model: c.ClaudeModel})
	}
	if c.OpenCodePasswordFile != "" {
		st, e := os.Stat(c.OpenCodePasswordFile)
		if e != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
			return nil, errors.New("OpenCode password file must be private and regular")
		}
		data, e := os.ReadFile(c.OpenCodePasswordFile)
		if e != nil {
			return nil, errors.New("OpenCode password file unreadable")
		}
		c.OpenCodePassword = strings.TrimSpace(string(data))
	}
	if c.OpenCodeURL != "" || c.OpenCodePassword != "" || c.OpenCodePasswordFile != "" {
		u, e := url.Parse(c.OpenCodeURL)
		if e != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || c.OpenCodePassword == "" {
			return nil, errors.New("OpenCode requires loopback HTTP URL and private server password")
		}
		ip := net.ParseIP(u.Hostname())
		if ip == nil || !ip.IsLoopback() {
			return nil, errors.New("OpenCode URL must use a loopback IP")
		}
		out = append(out, agentplatform.OpenCodeHTTPProvider{BaseURL: c.OpenCodeURL, Username: c.OpenCodeUsername, Password: c.OpenCodePassword})
	}
	return out, nil
}
