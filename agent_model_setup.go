package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/srhopkins/iugum/agentcore"
	"github.com/srhopkins/iugum/agentdesk"
	"gopkg.in/yaml.v3"
)

var setupName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// Changes are persisted for restart; existing turns, model selection and ledger stay intact.
func nativeModelSetup(ctx context.Context, path string, check func(context.Context, string, string) error, q *agentdesk.ModelSetup) (agentdesk.ModelSetupState, error) {
	var state agentdesk.ModelSetupState
	if e := check(ctx, "agent/model-setup", "read"); e != nil {
		return state, agentdesk.ErrModelSetupDenied
	}
	if q != nil {
		if e := check(ctx, "agent/model-setup", "write"); e != nil {
			return state, agentdesk.ErrModelSetupDenied
		}
	}
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() {
		return state, fmt.Errorf("model configuration unavailable")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return state, fmt.Errorf("model configuration unavailable")
	}
	var doc yaml.Node
	var c nativeAgentConfig
	if yaml.Unmarshal(raw, &doc) != nil || yaml.Unmarshal(raw, &c) != nil || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return state, fmt.Errorf("invalid model configuration")
	}
	state.ModelSetup = agentdesk.ModelSetup{DefaultProfile: c.Models.DefaultProfile, Pools: map[string]agentdesk.SetupPool{}, Profiles: map[string]agentdesk.SetupProfile{}}
	for id, p := range c.Models.Pools {
		u, e := url.Parse(p.BaseURL)
		endpoint := ""
		if e == nil && u.User == nil && u.RawQuery == "" && u.Fragment == "" {
			endpoint = p.BaseURL
		}
		state.Pools[id] = agentdesk.SetupPool{BaseURL: endpoint, DiscoverModels: p.DiscoverModels, CredentialConfigured: p.APIKeyEnv != "" || p.APIKeyFile != ""}
	}
	for id, p := range c.Models.Profiles {
		state.Profiles[id] = agentdesk.SetupProfile{Pool: p.Pool, Model: p.Model}
	}
	if q == nil {
		return state, nil
	}
	if len(q.Pools) == 0 || len(q.Pools) > 64 || len(q.Profiles) == 0 || len(q.Profiles) > 256 {
		return state, fmt.Errorf("provide pools and profiles within limits")
	}
	if _, ok := q.Profiles[q.DefaultProfile]; !ok {
		return state, fmt.Errorf("default profile must exist")
	}
	// Keep existing entries: routing choices, fallback policies and private references remain valid.
	for id := range c.Models.Pools {
		if _, ok := q.Pools[id]; !ok {
			return state, fmt.Errorf("existing pools cannot be removed")
		}
	}
	for id := range c.Models.Profiles {
		if _, ok := q.Profiles[id]; !ok {
			return state, fmt.Errorf("existing profiles cannot be removed")
		}
	}
	if c.Models.Pools == nil {
		c.Models.Pools = map[string]agentcore.Pool{}
	}
	if c.Models.Profiles == nil {
		c.Models.Profiles = map[string]agentcore.Profile{}
	}
	for id, p := range q.Pools {
		u, e := url.Parse(p.BaseURL)
		if !setupName.MatchString(id) || e != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(p.BaseURL, "\r\n") {
			return state, fmt.Errorf("pool requires a plain HTTP(S) endpoint without credentials or query")
		}
		old := c.Models.Pools[id]
		old.BaseURL = p.BaseURL
		old.DiscoverModels = p.DiscoverModels
		if c.Models.Pools == nil {
			return state, fmt.Errorf("model pools missing")
		}
		c.Models.Pools[id] = old
	}
	for id, p := range q.Profiles {
		if !setupName.MatchString(id) || len(p.Model) > 256 || strings.TrimSpace(p.Model) == "" || strings.ContainsAny(p.Model, "\r\n") {
			return state, fmt.Errorf("invalid model profile")
		}
		if _, ok := q.Pools[p.Pool]; !ok {
			return state, fmt.Errorf("profile pool must exist")
		}
		if check(ctx, "agent/profile/"+id, "select") != nil || check(ctx, "agent/model/"+p.Pool+"/"+id, "call") != nil {
			return state, agentdesk.ErrModelSetupDenied
		}
		old := c.Models.Profiles[id]
		old.Pool = p.Pool
		old.Model = p.Model
		c.Models.Profiles[id] = old
	}
	// Change only these editable fields. Preserve comments, credentials, budgets,
	// instructions and unknown model keys for forward compatibility.
	doc = *setupYAMLClone(&doc)
	root := doc.Content[0]
	models := setupYAMLMap(root, "models")
	setupYAMLValue(models, "default_profile", q.DefaultProfile)
	poolsNode := setupYAMLMap(models, "pools")
	for id, p := range q.Pools {
		setupYAMLValue(setupYAMLMap(poolsNode, id), "base_url", p.BaseURL)
		setupYAMLValue(setupYAMLMap(poolsNode, id), "discover_models", fmt.Sprint(p.DiscoverModels))
		poolNode := setupYAMLMap(poolsNode, id)
		for i := 0; i < len(poolNode.Content); i += 2 {
			if poolNode.Content[i].Value == "discover_models" {
				poolNode.Content[i+1].Tag = "!!bool"
			}
		}
	}
	profilesNode := setupYAMLMap(models, "profiles")
	for id, p := range q.Profiles {
		node := setupYAMLMap(profilesNode, id)
		setupYAMLValue(node, "pool", p.Pool)
		setupYAMLValue(node, "model", p.Model)
	}
	next, e := yaml.Marshal(&doc)
	if e != nil {
		return state, fmt.Errorf("cannot encode model configuration")
	}
	tmp, e := os.CreateTemp(filepath.Dir(path), ".model-setup-*")
	if e != nil {
		return state, fmt.Errorf("cannot save model configuration")
	}
	defer os.Remove(tmp.Name())
	if e = tmp.Chmod(0600); e == nil {
		_, e = tmp.Write(next)
	}
	if e == nil {
		e = tmp.Sync()
	}
	closeErr := tmp.Close()
	if e == nil {
		e = closeErr
	}
	if e == nil {
		e = os.Rename(tmp.Name(), path)
	}
	if e != nil {
		return state, fmt.Errorf("cannot save model configuration")
	}
	state.ModelSetup = *q
	for id, p := range state.Pools {
		old := c.Models.Pools[id]
		p.CredentialConfigured = old.APIKeyEnv != "" || old.APIKeyFile != ""
		state.Pools[id] = p
	}
	state.RestartRequired = true
	state.Reason = "Saved. Restart the Iugum Agent backend to apply. Current conversations keep their running configuration."
	return state, nil
}

func setupYAMLMap(parent *yaml.Node, key string) *yaml.Node {
	for i := 0; i < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			node := parent.Content[i+1]
			if node.Kind != yaml.MappingNode {
				node.Kind = yaml.MappingNode
				node.Tag = "!!map"
				node.Value = ""
				node.Content = nil
			}
			return node
		}
	}
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	parent.Content = append(parent.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, node)
	return node
}
func setupYAMLValue(parent *yaml.Node, key, value string) {
	for i := 0; i < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			old := parent.Content[i+1]
			copy := *old
			copy.Kind = yaml.ScalarNode
			copy.Tag = "!!str"
			copy.Value = value
			copy.Content = nil
			copy.Alias = nil
			parent.Content[i+1] = &copy
			return
		}
	}
	parent.Content = append(parent.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
}
func nativeModelSetupControl(path string, check func(context.Context, string, string) error, chatMu *sync.Mutex) agentdesk.ModelSetupControl {
	pending := false
	return func(ctx context.Context, q *agentdesk.ModelSetup) (agentdesk.ModelSetupState, error) {
		if !chatMu.TryLock() {
			return agentdesk.ModelSetupState{}, agentdesk.ErrModelSetupBusy
		}
		defer chatMu.Unlock()
		state, e := nativeModelSetup(ctx, path, check, q)
		if e == nil && q != nil {
			pending = true
		}
		state.RestartRequired = pending
		return state, e
	}
}

// Materialize aliases before editing. Shared anchors otherwise make a model
// edit change unrelated configuration, or produce duplicate/dangling anchors.
func setupYAMLClone(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.AliasNode {
		return setupYAMLClone(node.Alias)
	}
	copy := *node
	copy.Anchor = ""
	copy.Alias = nil
	copy.Content = make([]*yaml.Node, len(node.Content))
	for i, child := range node.Content {
		copy.Content[i] = setupYAMLClone(child)
	}
	if copy.Kind == yaml.MappingNode {
		explicit := map[string]bool{}
		content := []*yaml.Node{}
		for i := 0; i < len(copy.Content); i += 2 {
			if copy.Content[i].Tag != "!!merge" {
				explicit[copy.Content[i].Value] = true
				content = append(content, copy.Content[i], copy.Content[i+1])
			}
		}
		var merge func(*yaml.Node)
		merge = func(source *yaml.Node) {
			if source.Kind == yaml.SequenceNode {
				for _, item := range source.Content {
					merge(item)
				}
				return
			}
			if source.Kind != yaml.MappingNode {
				return
			}
			for i := 0; i < len(source.Content); i += 2 {
				key := source.Content[i]
				if !explicit[key.Value] {
					explicit[key.Value] = true
					content = append(content, key, source.Content[i+1])
				}
			}
		}
		for i := 0; i < len(copy.Content); i += 2 {
			if copy.Content[i].Tag == "!!merge" {
				merge(copy.Content[i+1])
			}
		}
		copy.Content = content
	}
	return &copy
}
