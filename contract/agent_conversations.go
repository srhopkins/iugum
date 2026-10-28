package contract

// AgentConversation owns visible history independently of execution sessions.
// It transfers neither hidden reasoning nor provider permissions/runtime state.
type AgentConversation struct {
	ID       string                     `json:"id"`
	Title    string                     `json:"title"`
	Revision uint64                     `json:"revision"`
	Selected AgentSessionRef            `json:"selected"`
	Links    []AgentSessionRef          `json:"links"`
	Messages []AgentConversationMessage `json:"messages"`
	Commands []AgentConversationCommand `json:"commands"`
}
type AgentConversationMessage struct {
	Role       string `json:"role"`
	Text       string `json:"text"`
	Provider   string `json:"provider"`
	SessionID  string `json:"sessionId"`
	CommandID  string `json:"commandId,omitempty"`
	Provenance string `json:"provenance"`
}
type AgentConversationCommand struct {
	ID      string          `json:"id"`
	Session AgentSessionRef `json:"session"`
	Digest  string          `json:"digest"`
	Status  string          `json:"status"`
	Error   string          `json:"error,omitempty"`
}
