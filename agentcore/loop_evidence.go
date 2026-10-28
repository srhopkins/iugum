package agentcore

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Pass only messages added by the current run, excluding its request history.
// FailedToolLoopSummary reports only completed calls to installed tool names.
// Unanswered calls are omitted: message history cannot prove they were attempted.
// Arguments, output text, IDs, paths and free-form errors never enter the summary.
func FailedToolLoopSummary(messages []Message, tools []Tool) string {
	known := map[string]bool{}
	safeName := regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	for _, tool := range tools {
		if safeName.MatchString(tool.Name) {
			known[tool.Name] = true
		}
	}
	type called struct {
		name string
		step int
	}
	calls := map[string]called{}
	steps := 0
	completed := []string{}
	omitted := false
	for _, m := range messages {
		if m.Role == "assistant" {
			steps++
			for _, call := range m.ToolCalls {
				if known[call.Function.Name] {
					calls[call.ID] = called{call.Function.Name, steps}
				}
			}
		}
		if m.Role != "tool" {
			continue
		}
		call, ok := calls[m.ToolCallID]
		if !ok {
			continue
		}
		delete(calls, m.ToolCallID)
		if len(completed) >= 16 {
			omitted = true
			continue
		}
		category := "completed"
		// Inspect only fixed error fields; completion does not prove semantic success.
		var result struct {
			IsError bool   `json:"is_error"`
			Code    string `json:"code"`
		}
		if json.Unmarshal([]byte(m.Content), &result) == nil && result.IsError {
			category = "error"
			switch result.Code {
			case "invalid_arguments", "unknown_skill", "invalid_path":
				category = "recoverable_" + result.Code
			}
		}
		if call.name == "shell" && m.ToolOutcome != nil {
			switch m.ToolOutcome.Category {
			case "exit":
				if m.ToolOutcome.ExitCode >= 0 && m.ToolOutcome.ExitCode <= 255 {
					category = fmt.Sprintf("exit_%d", m.ToolOutcome.ExitCode)
				}
			case "timeout", "canceled", "launch_error":
				category = m.ToolOutcome.Category
			}
		}
		completed = append(completed, fmt.Sprintf("step=%d %s=%s", call.step, call.name, category))
	}
	summary := fmt.Sprintf("Tool loop: result=failed steps=%d", steps)
	if len(completed) > 0 {
		summary += "; " + strings.Join(completed, "; ")
	}
	if omitted {
		summary += "; additional_completed_calls_omitted"
	}
	return summary
}
