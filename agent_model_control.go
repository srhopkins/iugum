package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/srhopkins/iugum/agentcore"
	"github.com/srhopkins/iugum/agentdesk"
)

// Called under the same lock as chat so selections affect the next turn.
func nativeModelControl(ctx context.Context, runtime *agentcore.Runtime, c nativeAgentConfig, selected, subscription *string, path string, check func(context.Context, string, string) error, id string) (agentdesk.ModelState, error) {
	state := agentdesk.ModelState{Options: []agentdesk.ModelOption{}}
	if err := check(ctx, "agent/routing", "read"); err != nil {
		return state, err
	}
	choices := map[string]string{}
	if c.ChatTransport == "codex-subscription" {
		state.Selected = *subscription
		for key, model := range c.SubscriptionProfiles {
			if state.Selected == "" && model == c.SubscriptionModel {
				state.Selected = key
			}
			if check(ctx, "agent/profile/"+key, "select") == nil && check(ctx, "agent/model/codex-subscription/"+model, "call") == nil {
				choices[key] = model
			}
		}
	} else if (c.ChatTransport == "" || c.ChatTransport == "native") && runtime != nil {
		state.Selected = *selected
		if state.Selected == "" {
			state.Selected = c.Models.DefaultProfile
		}
		for key, p := range c.Models.Profiles {
			if check(ctx, "agent/profile/"+key, "select") == nil && check(ctx, "agent/model/"+p.Pool+"/"+key, "call") == nil {
				choices[key] = p.Model
			}
		}
	}
	if id != "" {
		if _, ok := choices[id]; !ok {
			return state, fmt.Errorf("model profile is unavailable or not permitted")
		}
		raw, _ := json.Marshal(map[string]string{"profile": id})
		tools := nativeRoutingToolsWithSubscription(runtime, c, selected, subscription, path, check)
		if _, err := tools[1].Execute(ctx, raw); err != nil {
			return state, err
		}
		state.Selected = id
	}
	keys := make([]string, 0, len(choices))
	for key := range choices {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if choices[keys[i]] == choices[keys[j]] {
			return keys[i] < keys[j]
		}
		return choices[keys[i]] < choices[keys[j]]
	})
	for _, key := range keys {
		label := choices[key] + " · " + key
		if c.ChatTransport != "codex-subscription" && strings.HasPrefix(key, "discovered-") {
			label = choices[key] + " · " + c.Models.Profiles[key].Pool
		}
		state.Options = append(state.Options, agentdesk.ModelOption{ID: key, Name: label})
	}
	if len(state.Options) == 0 {
		state.Reason = "No selectable model profiles are configured or permitted for this transport."
	}
	return state, nil
}
