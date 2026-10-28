package beadactivity

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Execution identifies actual work. AgentID and SessionID are distinct:
// an agent identifier is never presented as a resumable session identifier.
type Execution struct {
	Version             int        `json:"version"`
	RunID               string     `json:"run_id"`
	Actor               string     `json:"actor"`
	Label               string     `json:"label"`
	Platform            string     `json:"platform"`
	Host                string     `json:"host"`
	Container           string     `json:"container,omitempty"`
	SessionID           string     `json:"session_id,omitempty"`
	ContextSessionID    string     `json:"context_session_id,omitempty"`
	AgentID             string     `json:"agent_id,omitempty"`
	ParentSessionID     string     `json:"parent_session_id,omitempty"`
	ParentAgentID       string     `json:"parent_agent_id,omitempty"`
	Transcript          string     `json:"transcript,omitempty"`
	ThreadURL           string     `json:"thread_url,omitempty"`
	IdentityUnavailable string     `json:"identity_unavailable,omitempty"`
	CWD                 string     `json:"cwd"`
	StartedAt           time.Time  `json:"started_at"`
	LastSeen            time.Time  `json:"last_seen"`
	EndedAt             *time.Time `json:"ended_at,omitempty"`
	Outcome             string     `json:"outcome,omitempty"`
}

var runIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

func (e Execution) Validate(actor string) error {
	if e.Actor != actor || actor == "" {
		return errors.New("execution actor must match current assignee")
	}
	if !runIDPattern.MatchString(e.RunID) || e.Platform == "" || e.Host == "" || e.Label == "" || e.CWD == "" {
		return errors.New("explicit run id, platform, host, label and cwd are required")
	}
	if e.SessionID == "" && e.AgentID == "" && e.IdentityUnavailable == "" {
		return errors.New("actual session/agent identity or explicit unavailable reason is required")
	}
	if e.Version != 0 && e.Version != 1 {
		return errors.New("unsupported execution reference version")
	}
	return nil
}

func ReportExecution(run Runner, e Execution, id, state, reason string, now time.Time, ttl time.Duration) error {
	if err := e.Validate(e.Actor); err != nil {
		return err
	}
	// Report supplies the canonical owner check. Collect both writes so
	// activity and its reference change in the same bd update.
	wrapper := func(args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "show" {
			raw, err := run(args...)
			if err != nil {
				return nil, err
			}
			var rows []struct{ Metadata map[string]json.RawMessage }
			if json.Unmarshal(raw, &rows) == nil && len(rows) == 1 {
				var prior Execution
				if decodeMetadataObject(rows[0].Metadata[executionKey(e.RunID)], &prior) == nil && prior.RunID != "" {
					if prior.Actor != e.Actor || prior.Platform != e.Platform || prior.SessionID != e.SessionID || prior.AgentID != e.AgentID || prior.Host != e.Host || prior.Container != e.Container {
						return nil, errors.New("run id already belongs to a different execution")
					}
					if e.ContextSessionID != "" && prior.ContextSessionID != "" && e.ContextSessionID != prior.ContextSessionID {
						return nil, errors.New("run context session changed")
					}
					if e.ParentSessionID != "" && prior.ParentSessionID != "" && e.ParentSessionID != prior.ParentSessionID {
						return nil, errors.New("run parent session changed")
					}
					if e.ParentAgentID != "" && prior.ParentAgentID != "" && e.ParentAgentID != prior.ParentAgentID {
						return nil, errors.New("run parent agent changed")
					}
					if e.ContextSessionID == "" {
						e.ContextSessionID = prior.ContextSessionID
					}
					if e.ParentSessionID == "" {
						e.ParentSessionID = prior.ParentSessionID
					}
					if e.ParentAgentID == "" {
						e.ParentAgentID = prior.ParentAgentID
					}
					if e.Transcript == "" {
						e.Transcript = prior.Transcript
					}
					if e.ThreadURL == "" {
						e.ThreadURL = prior.ThreadURL
					}
					e.StartedAt = prior.StartedAt
					e.EndedAt = prior.EndedAt
					if e.Outcome == "" {
						e.Outcome = prior.Outcome
					}
					if prior.EndedAt != nil && state != "completed" {
						return nil, errors.New("completed execution requires a new run id")
					}
				}
			}
			return raw, nil
		}
		if len(args) > 2 && args[2] == "update" {
			e.Version = 1
			e.LastSeen = now.UTC()
			if state == "completed" {
				at := now.UTC()
				if e.EndedAt == nil {
					e.EndedAt = &at
				}
				if reason != "" {
					e.Outcome = reason
				}
			}
			if e.StartedAt.IsZero() {
				e.StartedAt = now.UTC()
			}
			data, err := json.Marshal(e)
			if err != nil {
				return nil, err
			}
			args = append(args, "--set-metadata", executionKey(e.RunID)+"="+string(data))
		}
		return run(args...)
	}
	return report(wrapper, e.Actor, id, state, reason, e.RunID, now, ttl)
}

func Executions(metadata map[string]json.RawMessage) []Execution {
	out := []Execution{}
	for k, v := range metadata {
		if len(k) < len("iugum.execution.") || k[:len("iugum.execution.")] != "iugum.execution." {
			continue
		}
		var e Execution
		if decodeMetadataObject(v, &e) == nil && e.Version == 1 && e.Validate(e.Actor) == nil {
			out = append(out, e)
		}
	}
	return out
}

func executionKey(run string) string {
	return fmt.Sprintf("iugum.execution.%x", sha256.Sum256([]byte(run)))
}
