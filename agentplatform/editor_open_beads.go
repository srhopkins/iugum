package agentplatform

import (
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func editorOpenBeadsSchema() map[string]any {
	schema := editorBeadsArgsSchema()
	properties := schema["properties"].(map[string]any)
	for key, value := range editorIdentityProperties() {
		properties[key] = value
	}
	schema["required"] = []string{"instance", "commandId"}
	return schema
}

// The flat MCP entry point delegates to the original operation's policy gate,
// prequeue validator, exact-instance dispatch, uniqueness and receipt lifecycle.
func (s *Server) editorOpenBeads(ctx context.Context, q map[string]any) (*mcp.CallToolResult, any, error) {
	args := map[string]any{}
	for key, value := range q {
		if key != "instance" && key != "commandId" {
			args[key] = value
		}
	}
	raw, e := json.Marshal(args)
	if e != nil {
		return nil, nil, e
	}
	instance, _ := q["instance"].(string)
	commandID, _ := q["commandId"].(string)
	receipt, e := s.hostCall(ctx, HostCall{Instance: instance, CommandID: commandID, Operation: "open_beads", Args: raw})
	return nil, receipt, e
}
