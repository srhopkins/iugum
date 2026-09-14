package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/srhopkins/iugum/app"
	"gopkg.in/yaml.v3"
)

func runAgentClone(ctx context.Context, a *app.App, args []string, out, errout io.Writer) int {
	fs := flag.NewFlagSet("agent clone", flag.ContinueOnError)
	fs.SetOutput(errout)
	config := fs.String("config", "", "parent agent configuration")
	name := fs.String("name", "", "new clone name")
	output := fs.String("output", "", "new private clone directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *config == "" || !validNativeCloneName(*name) || *output == "" || fs.NArg() != 0 {
		fmt.Fprintln(errout, "Usage: iugum agent clone --config FILE --name NAME --output DIR")
		return 2
	}
	if err := a.Check(ctx, "agent", "clone"); err != nil {
		fmt.Fprintln(errout, app.DenyMessage(err))
		return 1
	}
	path, err := createNativeAgentClone(ctx, *config, *name, *output)
	if err != nil {
		fmt.Fprintln(errout, "agent clone:", err)
		return 1
	}
	fmt.Fprintf(out, "Created clone configuration: %s\nSeparate empty data directory; promotion is manual.\nRun: iugum agent run --config %q\n", path, path)
	return 0
}
func createNativeAgentClone(ctx context.Context, config, name, output string) (result string, err error) {
	if !validNativeCloneName(name) {
		return "", fmt.Errorf("invalid clone name")
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	config, err = filepath.Abs(config)
	if err != nil {
		return "", err
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(config)
	if err != nil {
		return "", err
	}
	var c map[string]any
	if err = yaml.Unmarshal(b, &c); err != nil {
		return "", err
	}
	if c == nil {
		return "", fmt.Errorf("configuration must be a mapping")
	}
	parent, _ := c["name"].(string)
	if !validNativeCloneName(parent) || parent == name {
		return "", fmt.Errorf("clone requires a different name from the named parent")
	}
	if runtime, _ := c["runtime"].(string); runtime != "" && runtime != "native" {
		return "", fmt.Errorf("clone supports native configurations only")
	}
	base := filepath.Dir(config)
	resolve := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(base, p)
	}
	files := map[string][]byte{}
	for _, field := range []string{"instructions_file", "policy_file"} {
		p, _ := c[field].(string)
		if p == "" {
			if field == "policy_file" {
				return "", fmt.Errorf("an explicit policy_file is required for cloning")
			}
			continue
		}
		data, e := os.ReadFile(resolve(p))
		if e != nil {
			return "", e
		}
		dest := "instructions.md"
		if field == "policy_file" {
			dest = "policy.csv"
			data = []byte(clonePolicyActor(string(data), "agent:"+parent, "agent:"+name))
		}
		files[dest] = data
		c[field] = dest
	}
	for _, field := range []string{"code_repo", "usage_path"} {
		if p, ok := c[field].(string); ok {
			c[field] = resolve(p)
		}
	}
	if models, ok := c["models"].(map[string]any); ok {
		if pools, ok := models["pools"].(map[string]any); ok {
			for _, raw := range pools {
				if pool, ok := raw.(map[string]any); ok {
					if p, ok := pool["api_key_file"].(string); ok {
						pool["api_key_file"] = resolve(p)
					}
				}
			}
		}
	}
	if sources, ok := c["sources"].([]any); ok {
		for _, raw := range sources {
			if s, ok := raw.(map[string]any); ok {
				if p, ok := s["root"].(string); ok {
					s["root"] = resolve(p)
				}
			}
		}
	}
	if port, ok := c["wiki_port"].(int); ok && port > 0 {
		c["wiki_url"] = fmt.Sprintf("http://127.0.0.1:%d", port)
	}
	delete(c, "wiki_port")
	c["wiki_dir"] = "wiki"
	if p, _ := c["usage_path"].(string); p == "" {
		data, _ := c["data_dir"].(string)
		if data == "" {
			data = "data"
		}
		c["usage_path"] = filepath.Join(resolve(data), "usage.json")
	}
	c["name"] = name
	c["data_dir"] = "data"
	c["listen"] = "127.0.0.1:3851"
	c["clone"] = map[string]any{"parent_config": config, "parent_name": parent, "created_at": time.Now().UTC().Format(time.RFC3339), "promotion": "manual_only", "memory_mode": "fresh", "memory_snapshot": "not_implemented", "code_worktree": "not_implemented"}
	files["agent.yaml"], err = yaml.Marshal(c)
	if err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if err = os.Mkdir(output, 0700); err != nil {
		return "", fmt.Errorf("output must be a new directory with an existing parent: %w", err)
	}
	defer func() {
		if err != nil {
			os.RemoveAll(output)
		}
	}()
	if err = os.Mkdir(filepath.Join(output, "wiki"), 0700); err != nil {
		return "", err
	}
	if err = os.Mkdir(filepath.Join(output, "data"), 0700); err != nil {
		return "", err
	}
	for file, data := range files {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		if err = os.WriteFile(filepath.Join(output, file), data, 0600); err != nil {
			return "", err
		}
	}
	return filepath.Join(output, "agent.yaml"), nil
}

// Change exact actor tokens only. Prefix matches would corrupt similarly named
// agents, resource paths, or comments. Wildcard subjects retain their semantics.
func clonePolicyActor(policy, oldActor, newActor string) string {
	lines := strings.Split(policy, "\n")
	for n, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 2 {
			continue
		}
		kind := strings.TrimSpace(fields[0])
		if kind != "p" && kind != "g" {
			continue
		}
		last := 1
		if kind == "g" && len(fields) > 2 {
			last = 2
		}
		for idx := 1; idx <= last; idx++ {
			if strings.TrimSpace(fields[idx]) == oldActor {
				fields[idx] = strings.Replace(fields[idx], oldActor, newActor, 1)
			}
		}
		lines[n] = strings.Join(fields, ",")
	}
	return strings.Join(lines, "\n")
}

func validNativeCloneName(name string) bool {
	return regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`).MatchString(name)
}
