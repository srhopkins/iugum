package agentplatform

// These schemas expose the same UUID and operation rules enforced by hostCall.
// Both native and external MCP clients receive the constraints before dispatch.
const editorUUIDPattern = `^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`

func editorIdentityProperties() map[string]any {
	return map[string]any{
		"instance":  map[string]any{"type": "string", "format": "uuid", "pattern": editorUUIDPattern, "description": "Exact live instance UUID from editor_windows; preserve its value."},
		"commandId": map[string]any{"type": "string", "format": "uuid", "pattern": editorUUIDPattern, "description": "Fresh command UUID in 8-4-4-4-12 hexadecimal form, for example 12345678-1234-4234-8234-123456789abc. Generate a new UUID; do not use this example or a word label. For status use the exact ID from the queued receipt."},
	}
}
func editorCallSchema() map[string]any {
	p := editorIdentityProperties()
	p["operation"] = map[string]any{"type": "string", "enum": []string{"inspect", "open_file", "focus", "open_beads"}, "description": "Choose an advertised operation from this instance's capabilities."}
	p["args"] = map[string]any{"type": "object", "additionalProperties": true, "description": "Operation-specific arguments. inspect uses {}; open_beads accepts optional typed view/filter/search/sort/lanes/ticket fields and {} opens its configured landing view."}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": p, "required": []string{"instance", "commandId", "operation", "args"}, "dependentSchemas": map[string]any{"operation": map[string]any{"allOf": editorOperationSchemas()}}}
}
func editorStatusSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": editorIdentityProperties(), "required": []string{"instance", "commandId"}}
}

func editorOperationSchemas() []any {
	empty := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}
	focus := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"pane": map[string]any{"type": "string", "enum": []string{"editor", "sidebar", "panel", "terminal", "chat"}}}, "required": []string{"pane"}}
	position := func(min int) map[string]any {
		return map[string]any{"type": "integer", "minimum": min, "maximum": 9007199254740991}
	}
	group := position(1)
	group["maximum"] = 9
	file := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
		"path": map[string]any{"type": "string", "minLength": 1, "maxLength": 4096, "not": map[string]any{"anyOf": []any{map[string]any{"pattern": `^/`}, map[string]any{"pattern": `\\`}, map[string]any{"pattern": "\x00"}, map[string]any{"pattern": `(^|/)\.\.(/|$)`}}}, "description": "Workspace-relative file path. No absolute path, backslash, NUL, or path escaping the workspace. Server also enforces the 4096-byte limit and workspace containment."},
		"line": position(1), "column": position(0), "endLine": position(0), "endColumn": position(0), "group": group}, "required": []string{"path", "line", "group"}}
	branches := []any{}
	for _, item := range []struct {
		op   string
		args map[string]any
	}{{"inspect", empty}, {"open_beads", editorBeadsArgsSchema()}, {"focus", focus}, {"open_file", file}} {
		branches = append(branches, map[string]any{"if": map[string]any{"properties": map[string]any{"operation": map[string]any{"const": item.op}}, "required": []string{"operation"}}, "then": map[string]any{"properties": map[string]any{"args": item.args}}})
	}
	return branches
}

func editorBeadsArgsSchema() map[string]any {
	text := func(limit int) map[string]any {
		return map[string]any{"type": "string", "minLength": 1, "maxLength": limit, "pattern": "^[^\x00-\x1f\x7f]+$", "description": "UTF-8 text without control characters. Server also enforces this length limit in bytes."}
	}
	properties := map[string]any{
		"view":   map[string]any{"type": "string", "enum": []string{"board", "tree", "activity", "graph", "list"}},
		"sort":   map[string]any{"type": "string", "enum": []string{"updated", "created", "priority", "status"}},
		"search": text(512), "ticketProject": text(128), "lanes": map[string]any{"type": "boolean"},
		"ticket": map[string]any{"type": "string", "pattern": `^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`},
	}
	for _, key := range []string{"projects", "statuses", "priorities", "types", "assignees", "labels"} {
		properties[key] = map[string]any{"type": "array", "maxItems": 20, "items": text(128)}
	}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "dependentRequired": map[string]any{"ticketProject": []string{"ticket"}}}
}
