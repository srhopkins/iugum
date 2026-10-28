// Package agentcore provides a bounded, policy-gated model loop independent of UI.
package agentcore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/srhopkins/iugum/contract"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Compaction               CompactionConfig   `json:"compaction,omitempty" yaml:"compaction,omitempty"`
	SubscriptionVisionModels []string           `json:"subscription_vision_models,omitempty" yaml:"subscription_vision_models,omitempty"`
	UsagePath                string             `json:"usage_path,omitempty" yaml:"usage_path,omitempty"`
	Instructions             string             `json:"instructions" yaml:"instructions"`
	DefaultProfile           string             `json:"default_profile" yaml:"default_profile"`
	Timezone                 string             `json:"timezone" yaml:"timezone"`
	MaxSteps                 int                `json:"max_steps" yaml:"max_steps"`
	Pools                    map[string]Pool    `json:"pools" yaml:"pools"`
	Profiles                 map[string]Profile `json:"profiles" yaml:"profiles"`
}
type Pool struct {
	DiscoverModels   bool      `json:"discover_models,omitempty" yaml:"discover_models,omitempty"`
	AlertFractions   []float64 `json:"alert_fractions,omitempty" yaml:"alert_fractions,omitempty"`
	APIKeyFile       string    `json:"api_key_file,omitempty" yaml:"api_key_file,omitempty"`
	Provider         string    `json:"provider" yaml:"provider"`
	BaseURL          string    `json:"base_url" yaml:"base_url"`
	APIKeyEnv        string    `json:"api_key_env" yaml:"api_key_env"`
	MonthlyTargetUSD float64   `json:"monthly_target_usd" yaml:"monthly_target_usd"`
	FallbackProfile  string    `json:"fallback_profile,omitempty" yaml:"fallback_profile,omitempty"`
}
type Profile struct {
	ContextWindowTokens int     `json:"context_window_tokens,omitempty" yaml:"context_window_tokens,omitempty"`
	Vision              bool    `json:"vision,omitempty" yaml:"vision,omitempty"`
	ZeroCost            bool    `json:"zero_cost,omitempty" yaml:"zero_cost,omitempty"`
	Pool                string  `json:"pool" yaml:"pool"`
	Model               string  `json:"model" yaml:"model"`
	InputUSDPerMillion  float64 `json:"input_usd_per_million" yaml:"input_usd_per_million"`
	OutputUSDPerMillion float64 `json:"output_usd_per_million" yaml:"output_usd_per_million"`
	MaxCompletionTokens int     `json:"max_completion_tokens,omitempty" yaml:"max_completion_tokens,omitempty"`
}
type Message struct {
	ToolValidationCode string       `json:"-"`
	ToolOutcome        *ToolOutcome `json:"-"`
	Role               string       `json:"role"`
	Images             []Image      `json:"-"`
	Content            string       `json:"content"`
	ToolCalls          []ToolCall   `json:"tool_calls,omitempty"`
	ToolCallID         string       `json:"tool_call_id,omitempty"`
}
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
type Tool struct {
	Name, Description string
	Parameters        map[string]any
	// Authorize resolves argument-dependent objects and rechecks current permissions.
	Authorize     func(context.Context, json.RawMessage) error
	Execute       func(context.Context, json.RawMessage) (string, error)
	ExecuteResult func(context.Context, json.RawMessage) (ToolResult, error)
}
type RunRequest struct {
	Actor, Profile, RunID string
	maxCompletionTokens   int
	requireComplete       bool
	Messages              []Message
}
type RunResult struct {
	Text     string    `json:"text"`
	Profile  string    `json:"profile"`
	Messages []Message `json:"messages"`
	Warnings []string  `json:"warnings,omitempty"`
}
type UsageRecord struct {
	Time         time.Time `json:"time"`
	RunID        string    `json:"run_id"`
	Pool         string    `json:"pool"`
	Profile      string    `json:"profile"`
	Model        string    `json:"model"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	EstimatedUSD float64   `json:"estimated_usd"`
	UsageKnown   bool      `json:"usage_known"`
	PriceKnown   bool      `json:"price_known"`
}
type Runtime struct {
	config  Config
	path    string
	gate    contract.Policy
	Client  *http.Client
	Now     func() time.Time
	mu      sync.Mutex
	records []UsageRecord
}

func LoadConfig(path string) (Config, error) {
	var c Config
	b, e := os.ReadFile(path)
	if e != nil {
		return c, e
	}
	e = yaml.Unmarshal(b, &c)
	return c, e
}
func New(c Config, usagePath string, gate contract.Policy) (*Runtime, error) {
	if err := c.Compaction.validate(); err != nil {
		return nil, err
	}
	if gate == nil {
		return nil, fmt.Errorf("agentcore: policy is required")
	}
	if c.MaxSteps <= 0 {
		c.MaxSteps = 8
	}
	if c.Timezone == "" {
		c.Timezone = "UTC"
	}
	if _, e := time.LoadLocation(c.Timezone); e != nil {
		return nil, e
	}
	if _, ok := c.Profiles[c.DefaultProfile]; !ok {
		return nil, fmt.Errorf("agentcore: default profile %q missing", c.DefaultProfile)
	}
	for name, p := range c.Profiles {
		if p.ContextWindowTokens < 0 || p.MaxCompletionTokens < 0 {
			return nil, fmt.Errorf("agentcore: profile %q has negative token limits", name)
		}
		if p.InputUSDPerMillion < 0 || p.OutputUSDPerMillion < 0 {
			return nil, fmt.Errorf("agentcore: profile %q has negative prices", name)
		}
		pool, ok := c.Pools[p.Pool]
		if !ok || p.Model == "" {
			return nil, fmt.Errorf("agentcore: invalid profile %q", name)
		}
		u, e := url.Parse(pool.BaseURL)
		if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return nil, fmt.Errorf("agentcore: pool %q needs an HTTP base_url", p.Pool)
		}
	}
	if usagePath == "" {
		usagePath = c.UsagePath
	}
	for name, pool := range c.Pools {
		if pool.MonthlyTargetUSD < 0 {
			return nil, fmt.Errorf("agentcore: pool %q has negative target", name)
		}
		for _, f := range pool.AlertFractions {
			if f <= 0 || f > 1 {
				return nil, fmt.Errorf("agentcore: pool %q alert fractions must be in (0,1]", name)
			}
		}
	}
	r := &Runtime{config: c, path: usagePath, gate: gate, Client: &http.Client{Timeout: 120 * time.Second}, Now: time.Now}
	if usagePath != "" {
		b, e := os.ReadFile(usagePath)
		if e == nil {
			if e = json.Unmarshal(b, &r.records); e != nil {
				return nil, fmt.Errorf("agentcore: usage ledger: %w", e)
			}
		} else if !os.IsNotExist(e) {
			return nil, e
		}
	}
	return r, nil
}
func (r *Runtime) Run(ctx context.Context, q RunRequest, tools []Tool) (RunResult, error) {
	return r.run(ctx, q, tools, nil, false)
}

func (r *Runtime) run(ctx context.Context, q RunRequest, tools []Tool, prepared *PreparedTurn, preparing bool) (RunResult, error) {
	var result RunResult
	if q.Actor == "" {
		return result, fmt.Errorf("agentcore: actor is required")
	}
	messages, imageErr := cloneMessages(q.Messages)
	if imageErr != nil {
		return result, imageErr
	}
	originalRequest := currentUserRequest(q.Messages)
	defs := []any{}
	byName := map[string]Tool{}
	for _, t := range tools {
		if (t.Execute == nil && t.ExecuteResult == nil) || t.Name == "" {
			return result, fmt.Errorf("agentcore: invalid tool")
		}
		if _, ok := byName[t.Name]; ok {
			return result, fmt.Errorf("agentcore: duplicate tool %s", t.Name)
		}
		byName[t.Name] = t
		defs = append(defs, map[string]any{"type": "function", "function": map[string]any{"name": t.Name, "description": t.Description, "parameters": t.Parameters}})
	}
	loc, _ := time.LoadLocation(r.config.Timezone)
	for step := 0; step < r.config.MaxSteps; step++ {
		if e := ctx.Err(); e != nil {
			return result, e
		}
		decision, routeErr := r.RoutingStatus(q.Profile)
		if routeErr != nil {
			return result, routeErr
		}
		name, warnings := decision.SelectedProfile, decision.Warnings
		p, ok := r.config.Profiles[name]
		if !ok {
			return result, fmt.Errorf("agentcore: unknown profile %q", name)
		}
		pool := r.config.Pools[p.Pool]
		result.Profile = name
		result.Warnings = warnings
		if e := r.gate.Enforce(ctx, contract.Request{Sub: q.Actor, Obj: "agent/model/" + p.Pool + "/" + name, Act: "call"}); e != nil {
			return result, e
		}
		var m Message
		var e error
		if step == 0 && prepared != nil {
			if !prepared.valid(r, q, tools, name) || !prepared.used.CompareAndSwap(false, true) {
				return result, ErrStalePrepared
			}
			m = prepared.message
		} else {
			key := ""
			if pool.APIKeyFile != "" {
				rawKey, err := os.ReadFile(pool.APIKeyFile)
				if err != nil {
					return result, fmt.Errorf("agentcore: cannot read configured credential file: %w", err)
				}
				key = strings.TrimSpace(string(rawKey))
				if key == "" {
					return result, fmt.Errorf("agentcore: configured credential file is empty")
				}
			}
			if pool.APIKeyEnv != "" {
				key = os.Getenv(pool.APIKeyEnv)
				if key == "" {
					return result, fmt.Errorf("agentcore: credential environment variable %s is not set", pool.APIKeyEnv)
				}
			}
			system := r.config.Instructions + "\nCurrent timestamp: " + r.Now().In(loc).Format(time.RFC3339Nano) + " (" + r.config.Timezone + "). This timestamp is refreshed for every model call. Date historical events using their recorded timestamps."
			wireMessages, wireErr := modelMessages(append([]Message{{Role: "system", Content: system}}, messages...), p.Vision)
			if wireErr != nil {
				return result, wireErr
			}
			limit := p.MaxCompletionTokens
			if q.maxCompletionTokens > 0 {
				limit = q.maxCompletionTokens
			}
			if !r.config.Compaction.Disabled && p.ContextWindowTokens > 0 {
				budget, budgetErr := r.contextBudget(name, p)
				if budgetErr != nil {
					return result, budgetErr
				}
				if limit <= 0 {
					limit = budget.OutputTokens
				}
				if hasImages(messages) {
					result.Warnings = append(result.Warnings, "Context guard counts text only; provider-specific image token use is unknown.")
				}
				if estimateRequest(system, messages, tools)+limit > budget.WindowTokens {
					return result, fmt.Errorf("agentcore: estimated request plus output exceeds profile %q context window; shorten current request/tool output or increase verified context_window_tokens", name)
				}
			}
			body := map[string]any{"model": p.Model, "messages": wireMessages}
			if len(defs) > 0 {
				body["tools"] = defs
			}
			if limit > 0 {
				body["max_completion_tokens"] = limit
			}
			b, e := json.Marshal(body)
			if e != nil {
				return result, e
			}
			if hasImages(messages) && len(b) > MaxImageModelWireBytes {
				return result, fmt.Errorf("agentcore: image model request exceeds32MiB")
			}
			req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(pool.BaseURL, "/")+"/chat/completions", bytes.NewReader(b))
			if e != nil {
				return result, e
			}
			req.Header.Set("Content-Type", "application/json")
			if key != "" {
				req.Header.Set("Authorization", "Bearer "+key)
			}
			resp, e := r.Client.Do(req)
			if e != nil {
				return result, fmt.Errorf("agentcore: model request failed: %w", e)
			}
			raw, e := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
			resp.Body.Close()
			if e != nil {
				return result, e
			}
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return result, fmt.Errorf("agentcore: provider returned HTTP %d", resp.StatusCode)
			}
			var out struct {
				Choices []struct {
					Message      Message `json:"message"`
					FinishReason string  `json:"finish_reason"`
				} `json:"choices"`
				Usage *struct {
					Prompt     int `json:"prompt_tokens"`
					Completion int `json:"completion_tokens"`
				} `json:"usage"`
			}
			if e = json.Unmarshal(raw, &out); e != nil {
				return result, fmt.Errorf("agentcore: invalid model response: %w", e)
			}
			usage := UsageRecord{Time: r.Now().UTC(), RunID: q.RunID, Pool: p.Pool, Profile: name, Model: p.Model, UsageKnown: out.Usage != nil, PriceKnown: p.ZeroCost || p.InputUSDPerMillion > 0 || p.OutputUSDPerMillion > 0}
			if out.Usage != nil {
				usage.InputTokens = out.Usage.Prompt
				usage.OutputTokens = out.Usage.Completion
				usage.EstimatedUSD = (float64(usage.InputTokens)*p.InputUSDPerMillion + float64(usage.OutputTokens)*p.OutputUSDPerMillion) / 1e6
			}
			if e = r.record(usage); e != nil {
				return result, fmt.Errorf("agentcore: persist usage: %w", e)
			}
			if len(out.Choices) == 0 {
				return result, fmt.Errorf("agentcore: model returned no choices")
			}
			m = out.Choices[0].Message
			if q.requireComplete && (out.Choices[0].FinishReason != "stop" || m.Role != "assistant" || len(m.ToolCalls) > 0) {
				return result, fmt.Errorf("agentcore: summary completion is incomplete (finish_reason %q)", out.Choices[0].FinishReason)
			}
		}
		if preparing {
			if err := validatePreparedMessage(m, byName); err != nil {
				return result, err
			}
			result.Messages = append(messages, m)
			result.Text = m.Content
			return result, nil
		}
		messages = append(messages, m)
		result.Messages = messages
		if len(m.ToolCalls) == 0 {
			result.Text = m.Content
			return result, nil
		}
		imageContexts := []Message{}
		for _, call := range m.ToolCalls {
			t, ok := byName[call.Function.Name]
			if !ok {
				return result, newToolTurnError(step+1, "", "tool_unknown", fmt.Errorf("agentcore: unknown tool %q", call.Function.Name))
			}
			args := json.RawMessage(call.Function.Arguments)
			if !json.Valid(args) {
				return result, newToolTurnError(step+1, t.Name, "tool_invalid_json", fmt.Errorf("agentcore: invalid arguments for %s", t.Name))
			}
			if e = r.gate.Enforce(ctx, contract.Request{Sub: q.Actor, Obj: "agent/tool/" + t.Name, Act: "execute"}); e != nil {
				return result, newToolTurnError(step+1, t.Name, "tool_policy_error", e)
			}
			if t.Authorize != nil {
				if e = t.Authorize(ctx, args); e != nil {
					return result, newToolTurnError(step+1, t.Name, "tool_authorization_error", e)
				}
			}
			if e = ctx.Err(); e != nil {
				return result, newToolTurnError(step+1, t.Name, "tool_execution_error", e)
			}
			answer, e := executeTool(ctx, t, args)
			validationCode := ""
			if e != nil {
				var invalid *InvalidToolArguments
				if !errors.As(e, &invalid) {
					if answer.Outcome != nil {
						result.Messages = append(messages, Message{Role: "tool", ToolCallID: call.ID, ToolOutcome: answer.Outcome})
					}
					return result, newToolTurnError(step+1, t.Name, "tool_execution_error", fmt.Errorf("agentcore: tool %s: %w", t.Name, e))
				}
				body, marshalErr := json.Marshal(map[string]any{"is_error": true, "code": "invalid_arguments", "message": invalid.Message})
				if marshalErr != nil {
					return result, marshalErr
				}
				answer = ToolResult{Text: string(body)}
				validationCode = "invalid_arguments"
			}
			messages = append(messages, Message{Role: "tool", Content: answer.Text, ToolCallID: call.ID, ToolOutcome: answer.Outcome, ToolValidationCode: validationCode})
			result.Messages = messages
			if len(answer.Images) > 0 {
				imageContexts = append(imageContexts, imageContext(t.Name, call, answer, originalRequest))
			}
		}
		messages = append(messages, imageContexts...)
		result.Messages = messages
		if imageErr := imageAggregate(messages); imageErr != nil {
			return result, imageErr
		}
	}
	result.Messages = messages
	return result, fmt.Errorf("agentcore: maximum model steps reached (%d)", r.config.MaxSteps)
}
