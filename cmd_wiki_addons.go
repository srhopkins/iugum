package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/srhopkins/iugum/agentacp"
	"github.com/srhopkins/iugum/agentdesk"
	"github.com/srhopkins/iugum/app"
	"github.com/srhopkins/iugum/contract"
	chiefui "github.com/srhopkins/iugum/plugs/chief"
	"gopkg.in/yaml.v3"
)

type wikiAddonsConfig struct {
	Homes      map[string]string          `yaml:"homes"`
	Search     bool                       `yaml:"search"`
	Chat       bool                       `yaml:"chat"`
	PolicyFile string                     `yaml:"policy_file"`
	DataDir    string                     `yaml:"data_dir"`
	Agents     map[string]agentacp.Config `yaml:"agents"`
}

// extractWikiAddons keeps the existing wiki argument parser and plain wiki path.
func extractWikiAddons(args []string) ([]string, string, error) {
	var rest []string
	config := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--addons" {
			if i+1 == len(args) {
				return nil, "", fmt.Errorf("wiki: --addons requires a configuration file")
			}
			i++
			config = args[i]
		} else if strings.HasPrefix(args[i], "--addons=") {
			config = strings.TrimPrefix(args[i], "--addons=")
			if config == "" {
				return nil, "", fmt.Errorf("wiki: --addons requires a configuration file")
			}
		} else {
			rest = append(rest, args[i])
		}
	}
	return rest, config, nil
}

func runWikiAddons(ctx context.Context, a *app.App, opts contract.WikiOpts, path string, errout io.Writer) int {
	fail := func(err error) int { fmt.Fprintln(errout, err); return 1 }
	if err := a.Check(ctx, "wiki", "serve"); err != nil {
		return fail(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return fail(err)
	}
	var c wikiAddonsConfig
	decoder := yaml.NewDecoder(strings.NewReader(string(b)))
	decoder.KnownFields(true)
	if err = decoder.Decode(&c); err != nil {
		return fail(err)
	}
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return fail(err)
	}
	resolve := func(p string) string {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(base, p)
	}
	if c.DataDir == "" {
		c.DataDir = ".iugum-wiki"
	}
	c.DataDir = resolve(c.DataDir)
	spaceRoot, err := filepath.Abs(opts.Space)
	if err != nil {
		return fail(err)
	}
	privatePath := func(p string) error {
		abs, e := filepath.Abs(p)
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(spaceRoot, abs)
		if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("wiki add-on configuration and agent data must be outside the served wiki space")
		}
		return nil
	}
	if err = privatePath(path); err != nil {
		return fail(err)
	}
	if err = privatePath(c.DataDir); err != nil {
		return fail(err)
	}
	for _, home := range c.Homes {
		if err = privatePath(resolve(home)); err != nil {
			return fail(err)
		}
	}
	var gate contract.Policy = a.Gate
	if c.PolicyFile != "" {
		gate = agentFilePolicy{path: resolve(c.PolicyFile), fallback: a.Gate}
	}
	check := func(ctx context.Context, obj, act string) error {
		return gate.Enforce(ctx, contract.Request{Sub: a.Actor, Obj: obj, Act: act})
	}
	if err = check(ctx, "wiki/addons", "configure"); err != nil {
		return fail(err)
	}
	if !c.Chat && (len(c.Agents) > 0 || len(c.Homes) > 0) {
		return fail(fmt.Errorf("wiki: agents configured but chat is disabled"))
	}
	if err = chiefui.InstallFeatures(opts.Space, c.Chat, c.Search); err != nil {
		return fail(err)
	}
	if !c.Chat && !c.Search {
		if err = a.ServeWiki(ctx, opts); err != nil {
			return fail(err)
		}
		return 0
	}
	// Loopback proxy keeps chat and wiki on one origin. SilverBullet stays an independent service.
	if opts.Host != "localhost" {
		ip := net.ParseIP(opts.Host)
		if ip == nil || !ip.IsLoopback() {
			return fail(fmt.Errorf("wiki add-ons currently require a loopback hostname"))
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fail(err)
	}
	inner := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	listen := net.JoinHostPort(opts.Host, fmt.Sprint(opts.Port))
	agents := map[string]*agentdesk.Server{}
	for id, cfg := range c.Agents {
		if id == "default" || !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(id) {
			return fail(fmt.Errorf("invalid agent id %q", id))
		}
		if len(cfg.Command) == 0 {
			return fail(fmt.Errorf("agent %s needs an ACP command", id))
		}
		if cfg.Name == "" {
			cfg.Name = id
		}
		if cfg.Cwd == "" {
			cfg.Cwd = base
		} else {
			cfg.Cwd = resolve(cfg.Cwd)
		}
		dir := filepath.Join(c.DataDir, "agents", id)
		actor := "agent:" + id
		agentCheck := func(ctx context.Context, obj, act string) error {
			return gate.Enforce(ctx, contract.Request{Sub: actor, Obj: obj, Act: act})
		}
		client := &agentacp.Client{Config: cfg, StateDir: dir, Check: agentCheck}
		child, e := agentdesk.New(agentdesk.Config{Name: cfg.Name, DataDir: dir, Listen: listen, PassthroughChat: true, Chat: client.Chat, Check: agentCheck, Metadata: map[string]string{"transport": "acp", "model": cfg.Model}})
		if e != nil {
			return fail(e)
		}
		agents[id] = child
	}
	connections := map[string]func(context.Context) (string, error){}
	for id, home := range c.Homes {
		if id == "default" || !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(id) || agents[id] != nil {
			return fail(fmt.Errorf("invalid or duplicate agent id %q", id))
		}
		root := resolve(home)
		connections[id] = func(ctx context.Context) (string, error) {
			file, _ := processPaths(root)
			b, e := os.ReadFile(file)
			if e != nil {
				return "", fmt.Errorf("agent is stopped; start its home first")
			}
			var p agentProcess
			if e = json.Unmarshal(b, &p); e != nil {
				return "", e
			}
			host, _, e := net.SplitHostPort(p.Listen)
			ip := net.ParseIP(host)
			if e != nil || ip == nil || !ip.IsLoopback() {
				return "", fmt.Errorf("agent endpoint must be loopback")
			}
			return "http://" + p.Listen, nil
		}
	}
	desk, err := agentdesk.New(agentdesk.Config{ConnectionControl: func(ctx context.Context, id, verb string) error {
		root, ok := c.Homes[id]
		if !ok {
			return fmt.Errorf("unknown agent home")
		}
		var output bytes.Buffer
		if code := runAgentProcess(ctx, a, verb, []string{"--home", resolve(root)}, &output, &output); code != 0 {
			return fmt.Errorf("%s", output.String())
		}
		return nil
	}, Connections: connections, Name: "Wiki", DataDir: filepath.Join(c.DataDir, "wiki"), Listen: listen, WikiIntegrated: true, WikiURL: fmt.Sprintf("http://127.0.0.1:%d", inner), Agents: agents, HideDefaultAgent: true, DisableChat: !c.Chat, DisableSearch: !c.Search, Check: check, Search: func(ctx context.Context, q, scope string) (any, error) {
		if scope != "" && scope != "all" && scope != "wiki" {
			return nil, fmt.Errorf("this wiki supports the wiki search source only")
		}
		var terms, exclude []string
		for _, word := range strings.Fields(q) {
			if strings.HasPrefix(word, "-") && len(word) > 1 {
				exclude = append(exclude, word[1:])
			} else {
				terms = append(terms, word)
			}
		}
		hits, e := searchAgentWiki(ctx, opts.Space, strings.Join(terms, " "), exclude, check)
		return map[string]any{"wiki": hits}, e
	}})
	if err != nil {
		return fail(err)
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	wikiErr := make(chan error, 1)
	go func() {
		err := a.ServeWiki(ctx, contract.WikiOpts{Space: opts.Space, Host: "127.0.0.1", Port: inner})
		wikiErr <- err
		cancel()
	}()
	fmt.Fprintf(errout, "Wiki add-ons: chat=%t search=%t at http://%s\n", c.Chat, c.Search, listen)
	err = desk.Run(ctx)
	select {
	case e := <-wikiErr:
		if e != nil && ctx.Err() == nil {
			return fail(e)
		}
	default:
	}
	if err != nil {
		return fail(err)
	}
	return 0
}
