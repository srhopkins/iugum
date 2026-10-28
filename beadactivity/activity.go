// Package beadactivity projects explicit agent activity from Beads metadata.
// A claim or an in-progress issue alone never establishes a live worker.
package beadactivity

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const MetadataKey = "iugum.activity"
const MaxTTL = 10 * time.Minute

type Signal struct {
	RunID     string    `json:"run_id,omitempty"`
	Actor     string    `json:"actor"`
	State     string    `json:"state"`
	SeenAt    time.Time `json:"seen_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Reason    string    `json:"reason,omitempty"`
}

type View struct {
	RunID     string    `json:"run_id,omitempty"`
	State     string    `json:"state"`
	Actor     string    `json:"actor,omitempty"`
	SeenAt    time.Time `json:"seen_at,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

func NewSignal(actor, assignee, state, reason string, now time.Time, ttl time.Duration) (Signal, error) {
	if strings.TrimSpace(actor) == "" || actor != assignee {
		return Signal{}, errors.New("activity requires the current assignee as actor")
	}
	if state != "working" && state != "paused" && state != "completed" {
		return Signal{}, errors.New("activity state must be working, paused or completed")
	}
	if ttl < time.Second || ttl > MaxTTL {
		return Signal{}, errors.New("activity expiry must be between one second and ten minutes")
	}
	if len(reason) > 500 {
		return Signal{}, errors.New("activity reason exceeds 500 bytes")
	}
	return Signal{Actor: actor, State: state, SeenAt: now.UTC(), ExpiresAt: now.Add(ttl).UTC(), Reason: reason}, nil
}

func Decode(metadata map[string]json.RawMessage) *Signal {
	var s Signal
	if err := decodeMetadataObject(metadata[MetadataKey], &s); err != nil {
		return nil
	}
	return &s
}

func Project(assignee, status string, s *Signal, now time.Time) View {
	if status == "closed" {
		return View{State: "completed", Actor: assignee}
	}
	if strings.TrimSpace(assignee) == "" {
		return View{State: "unassigned"}
	}
	if s == nil {
		return View{State: "unknown", Actor: assignee}
	}
	v := View{RunID: s.RunID, State: s.State, Actor: assignee, SeenAt: s.SeenAt, ExpiresAt: s.ExpiresAt, Reason: s.Reason}
	if s.Actor != assignee || s.SeenAt.IsZero() || s.SeenAt.After(now) || !s.ExpiresAt.After(s.SeenAt) || s.ExpiresAt.Sub(s.SeenAt) > MaxTTL {
		v.State = "stale"
		v.Reason = "Activity does not match the current owner or has invalid timestamps"
		return v
	}
	switch s.State {
	case "paused", "completed":
		return v
	case "working":
		if !s.ExpiresAt.After(now) {
			v.State = "stale"
			v.Reason = "Worker heartbeat expired"
		}
		return v
	default:
		v.State = "unknown"
		return v
	}
}
