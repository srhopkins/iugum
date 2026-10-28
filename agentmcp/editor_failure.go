package agentmcp

import "github.com/modelcontextprotocol/go-sdk/mcp"

// The installed editor server has no structured rejection codes. Its SDK maps
// fixed prequeue errors to one text entry. Match only exact whole static values
// on the named editor tools. This observation never permits recovery or replay.
func editorFailureCategory(server, tool string, result *mcp.CallToolResult) string {
	if server != "editor" || result == nil || !result.IsError || len(result.Content) != 1 || result.StructuredContent != nil {
		return ""
	}
	switch tool {
	case "editor_call", "editor_open_beads", "editor_status":
	default:
		return ""
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return ""
	}
	switch text.Text {
	case "editor instance disconnected":
		return "editor_instance_disconnected"
	case "editor commandId already reserved":
		return "editor_command_id_reserved"
	case "editor capability unavailable":
		return "editor_capability_unavailable"
	case "editor queue full":
		return "editor_queue_full"
	case "editor command not found for this instance and commandId; use the exact ID from a queued receipt, not a rejected or invented ID; a missing receipt does not mean an editor capability is unsupported":
		return "editor_receipt_not_found"
	}
	return ""
}
