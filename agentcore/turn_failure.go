package agentcore

import (
	"context"
	"errors"
	"regexp"
)

// TurnFailureDiagnostic contains only runtime-established, bounded metadata.
// It never enters model context or changes retry or recovery decisions.
type TurnFailureDiagnostic struct {
	Category string `json:"category"`
	Tool     string `json:"tool,omitempty"`
	Step     int    `json:"step,omitempty"`
}

type categorizedToolError struct {
	category string
	cause    error
}

func (e *categorizedToolError) Error() string { return e.cause.Error() }
func (e *categorizedToolError) Unwrap() error { return e.cause }

// NewToolFailure marks a known adapter boundary without inspecting error text.
// Unknown categories become tool_execution_error. The original error is retained.
func NewToolFailure(category string, cause error) error {
	if cause == nil {
		return nil
	}
	return &categorizedToolError{category: safeFailureCategory(category), cause: cause}
}

func safeFailureCategory(category string) string {
	switch category {
	case "tool_unknown", "tool_invalid_json", "tool_policy_error",
		"tool_authorization_error", "tool_execution_error", "tool_transport_error",
		"mcp_call_error", "tool_remote_error", "tool_result_error",
		"editor_instance_disconnected", "editor_command_id_reserved",
		"editor_capability_unavailable", "editor_queue_full", "editor_receipt_not_found",
		"canceled", "timeout":
		return category
	}
	return "tool_execution_error"
}

type toolTurnError struct {
	diagnostic TurnFailureDiagnostic
	cause      error
}

func (e *toolTurnError) Error() string { return e.cause.Error() }
func (e *toolTurnError) Unwrap() error { return e.cause }

var safeFailureTool = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func newToolTurnError(step int, name, category string, cause error) error {
	if category == "tool_execution_error" {
		var known *categorizedToolError
		if errors.As(cause, &known) {
			category = known.category
		}
	}
	if !safeFailureTool.MatchString(name) {
		name = ""
	}
	return &toolTurnError{diagnostic: TurnFailureDiagnostic{Category: safeFailureCategory(category), Tool: name, Step: step}, cause: cause}
}

// SafeTurnFailure projects typed errors only. Free-form causes remain private.
func SafeTurnFailure(err error) *TurnFailureDiagnostic {
	if err == nil {
		return nil
	}
	d := TurnFailureDiagnostic{Category: "runtime_error"}
	var tool *toolTurnError
	if errors.As(err, &tool) {
		d = tool.diagnostic
	} else {
		var known *categorizedToolError
		if errors.As(err, &known) {
			d.Category = safeFailureCategory(known.category)
		}
	}
	if errors.Is(err, context.Canceled) {
		d.Category = "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		d.Category = "timeout"
	}
	return &d
}
