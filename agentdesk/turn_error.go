package agentdesk

import (
	"context"
	"errors"
	"fmt"

	"github.com/srhopkins/iugum/agentcore"
	"regexp"
	"strings"
)

// TurnFailure is application feedback, never model-authored conversation text.
// Details contain only fixed categories and bounded tool outcome evidence.
type TurnFailure struct {
	Code    string `json:"code"`
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"`
}

type runtimeTurnError struct {
	cause    error
	evidence string
}

func (e *runtimeTurnError) Error() string { return "native runtime turn failed" }
func (e *runtimeTurnError) Unwrap() error { return e.cause }

// NewRuntimeTurnError retains the cause for server-side classification without
// sending its free-form text, provider payloads, paths or credentials to clients.
func NewRuntimeTurnError(cause error, evidence string) error {
	return &runtimeTurnError{cause: cause, evidence: evidence}
}

var safeLoopEvidence = regexp.MustCompile(`^Tool loop: result=failed steps=[0-9]+(?:; step=[0-9]+ [A-Za-z0-9_-]{1,64}=(?:completed|error|recoverable_invalid_arguments|recoverable_unknown_skill|recoverable_invalid_path|exit_[0-9]{1,3}|timeout|canceled|launch_error))*(?:; additional_completed_calls_omitted)?$`)

func turnFailure(err error) TurnFailure {
	f := TurnFailure{Code: "turn_failed", Summary: "The request could not finish. Try again.", Details: "Stage: request execution. Raw diagnostics are withheld."}
	if errors.Is(err, context.Canceled) {
		f.Code = "turn_canceled"
		f.Summary = "The request was stopped."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		f.Code = "turn_timeout"
		f.Summary = "The request timed out. Try again."
	}
	var runtimeErr *runtimeTurnError
	if errors.As(err, &runtimeErr) {
		f.Details = "Stage: native runtime. Raw diagnostics are withheld."
		if diagnostic := agentcore.SafeTurnFailure(runtimeErr.cause); diagnostic != nil {
			f.Details += "\nFailure category: " + diagnostic.Category
			if diagnostic.Tool != "" {
				f.Details += fmt.Sprintf("; tool=%s; step=%d", diagnostic.Tool, diagnostic.Step)
			}
		}
		if len(runtimeErr.evidence) <= 4096 && strings.Count(runtimeErr.evidence, "; step=") <= 16 && safeLoopEvidence.MatchString(runtimeErr.evidence) {
			f.Details += "\n" + runtimeErr.evidence
		}
	}
	return f
}
