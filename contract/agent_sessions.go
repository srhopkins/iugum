package contract

import (
	"context"
	"encoding/json"
	"time"
)

// AgentSessionRef identifies an existing conversation, never a replacement.
type AgentSessionRef struct {
	Provider string `json:"provider"`
	ID       string `json:"sessionId"`
	Cwd      string `json:"cwd"`
	Title    string `json:"title"`
}

// AgentSessionCapabilities describe what the controlling adapter implements.
// Protocol support alone does not imply the client implements a capability.
type AgentSessionCapabilities struct {
	Create      bool `json:"create"`
	Models      bool `json:"models"`
	SelectModel bool `json:"selectModel"`
	Resume      bool `json:"resume"`
	Stream      bool `json:"stream"`
	Interrupt   bool `json:"interrupt"`
	Approvals   bool `json:"approvals"`
	Steer       bool `json:"steer"`
}

type AgentSessionMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
	At   string `json:"at,omitempty"`
}

type AgentSessionEvent struct {
	Sequence uint64    `json:"sequence"`
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"`
	Text     string    `json:"text,omitempty"`
}

// AgentSessionRequest retains the provider's request shape and identity.
// Reply must receive a decision from the client, never an inferred approval.
type AgentSessionRequest struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type AgentSessionObserver interface {
	Event(kind, text string)
	Request(context.Context, AgentSessionRequest) (json.RawMessage, error)
}

// AgentSession owns one provider conversation independently of a UI window.
// Prompt does not create/fork a conversation or retry uncertain delivery.
type AgentSession interface {
	History(context.Context) ([]AgentSessionMessage, error)
	Prompt(context.Context, string) error
	Interrupt(context.Context) error
	Close() error
}

type AgentSessionProvider interface {
	Name() string
	Capabilities() AgentSessionCapabilities
	Open(context.Context, AgentSessionRef, AgentSessionObserver) (AgentSession, error)
}

// AgentSessionCreator is optional: creating is always an explicit operation.
type AgentSessionCreate struct {
	Provider string `json:"provider"`
	Cwd      string `json:"cwd"`
	Title    string `json:"title,omitempty"`
	Model    string `json:"model,omitempty"`
}
type AgentSessionCreator interface {
	Create(context.Context, AgentSessionCreate, AgentSessionObserver) (AgentSessionRef, AgentSession, error)
}
type AgentSessionModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type AgentSessionModels struct {
	Options   []AgentSessionModel `json:"options"`
	Current   string              `json:"current,omitempty"`
	CanSelect bool                `json:"can_select"`
	Reason    string              `json:"reason,omitempty"`
}
type AgentSessionModelController interface {
	Models(context.Context) (AgentSessionModels, error)
	SelectModel(context.Context, string) error
}
