package agentcore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"time"
)

const MaxEditorDiagnosticRecords = 16
const MaxEditorDiagnosticBytes = 16 << 10

var diagnosticUUID = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
var diagnosticIdentity = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
var editorDiagnosticFields = []string{"instance", "commandId", "operation", "args", "view", "projects", "search", "statuses", "priorities", "types", "assignees", "labels", "sort", "lanes", "ticket", "ticketProject", "path", "line", "column", "endLine", "endColumn", "group", "pane"}

type EditorFieldShape struct {
	Field         string             `json:"field"`
	Present       bool               `json:"present"`
	Type          string             `json:"type,omitempty"`
	UUIDValid     *bool              `json:"uuid_valid,omitempty"`
	Fields        []EditorFieldShape `json:"fields,omitempty"`
	UnknownFields int                `json:"unknown_fields,omitempty"`
}
type EditorValidationIssue struct {
	Code  string `json:"code"`
	Path  string `json:"path"`
	Field string `json:"field,omitempty"`
}
type EditorCallDiagnostic struct {
	Step          int                     `json:"step"`
	Tool          string                  `json:"tool"`
	Shape         string                  `json:"shape"`
	Fields        []EditorFieldShape      `json:"fields,omitempty"`
	UnknownFields int                     `json:"unknown_fields,omitempty"`
	Result        string                  `json:"result"`
	Issues        []EditorValidationIssue `json:"schema_shape_issues,omitempty"`
}
type EditorModelIdentity struct {
	Profile string `json:"profile"`
	Model   string `json:"model"`
}
type EditorDefinitionIdentity struct {
	Tool              string `json:"tool"`
	DescriptionSHA256 string `json:"description_sha256"`
	SchemaSHA256      string `json:"schema_sha256"`
}
type EditorTurnDiagnostic struct {
	Failure               *TurnFailureDiagnostic     `json:"failure,omitempty"`
	Version               int                        `json:"version"`
	At                    string                     `json:"at"`
	RuntimeOutcome        string                     `json:"runtime_outcome"`
	RequestedProfile      string                     `json:"requested_profile"`
	Models                []EditorModelIdentity      `json:"models"`
	MaxSteps              int                        `json:"max_steps"`
	RequestSystemSHA256   string                     `json:"request_system_sha256"`
	RequestMessageCount   int                        `json:"request_message_count"`
	InstructionsSHA256    string                     `json:"instructions_sha256"`
	ToolDefinitionsSHA256 string                     `json:"tool_definitions_sha256"`
	EditorDefinitions     []EditorDefinitionIdentity `json:"editor_definitions"`
	Calls                 []EditorCallDiagnostic     `json:"calls"`
	Truncated             bool                       `json:"truncated"`
}

func knownEditorDiagnosticTool(name string) bool {
	return name == "mcp_editor__editor_windows" || name == "mcp_editor__editor_call" || name == "mcp_editor__editor_status" || name == "mcp_editor__editor_open_beads"
}
func diagnosticHash(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func diagnosticJSONHash(value any) string {
	b, e := json.Marshal(value)
	if e != nil {
		return "unavailable"
	}
	return diagnosticHash(b)
}
func safeDiagnosticIdentity(s string) string {
	if !diagnosticIdentity.MatchString(s) {
		return "omitted"
	}
	return s
}
func diagnosticType(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "other"
	}
}
func knownDiagnosticField(name string) bool {
	for _, field := range editorDiagnosticFields {
		if field == name {
			return true
		}
	}
	return false
}
func diagnosticShape(object map[string]any, keys []string, depth int) ([]EditorFieldShape, int) {
	fields := []EditorFieldShape{}
	unknown := 0
	for key := range object {
		found := false
		for _, known := range keys {
			found = found || key == known
		}
		if !found {
			unknown++
		}
	}
	for _, key := range keys {
		value, present := object[key]
		field := EditorFieldShape{Field: key, Present: present}
		if present {
			field.Type = diagnosticType(value)
			if key == "instance" || key == "commandId" {
				text, _ := value.(string)
				valid := diagnosticUUID.MatchString(text)
				field.UUIDValid = &valid
			}
			if nested, ok := value.(map[string]any); ok && key == "args" && depth < 2 {
				field.Fields, field.UnknownFields = diagnosticShape(nested, editorDiagnosticFields, depth+1)
			}
		}
		if depth == 0 || present {
			fields = append(fields, field)
		}
	}
	return fields, unknown
}
func schemaRequired(s map[string]any) []string {
	out := []string{}
	switch required := s["required"].(type) {
	case []any:
		for _, v := range required {
			if field, ok := v.(string); ok && knownDiagnosticField(field) {
				out = append(out, field)
			}
		}
	case []string:
		for _, field := range required {
			if knownDiagnosticField(field) {
				out = append(out, field)
			}
		}
	}
	return out
}
func editorArgsDiagnosticSchema(schema map[string]any, operation any) map[string]any {
	dependencies, _ := schema["dependentSchemas"].(map[string]any)
	dependent, _ := dependencies["operation"].(map[string]any)
	branches, _ := dependent["allOf"].([]any)
	for _, raw := range branches {
		branch, _ := raw.(map[string]any)
		condition, _ := branch["if"].(map[string]any)
		props, _ := condition["properties"].(map[string]any)
		op, _ := props["operation"].(map[string]any)
		expected, ok := op["const"].(string)
		actual, _ := operation.(string)
		if ok && expected == actual {
			then, _ := branch["then"].(map[string]any)
			props, _ = then["properties"].(map[string]any)
			args, _ := props["args"].(map[string]any)
			return args
		}
	}
	return nil
}
func diagnosticIssues(schema map[string]any, input map[string]any, path string, depth int) []EditorValidationIssue {
	issues := []EditorValidationIssue{}
	if depth > 1 {
		return issues
	}
	for _, field := range schemaRequired(schema) {
		if _, present := input[field]; !present {
			issues = append(issues, EditorValidationIssue{Code: "required", Path: path, Field: field})
		}
	}
	properties, _ := schema["properties"].(map[string]any)
	for _, field := range editorDiagnosticFields {
		value, present := input[field]
		if !present {
			continue
		}
		raw, exists := properties[field]
		if !exists {
			if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
				issues = append(issues, EditorValidationIssue{Code: "additional_field", Path: path, Field: field})
			}
			continue
		}
		constraint, _ := raw.(map[string]any)
		expected, _ := constraint["type"].(string)
		actual := diagnosticType(value)
		if expected != "" && expected != actual && !(expected == "integer" && actual == "number") {
			issues = append(issues, EditorValidationIssue{Code: "type", Path: path, Field: field})
		}
		if values, ok := constraint["enum"].([]any); ok {
			actualText, isString := value.(string)
			matched := false
			for _, allowed := range values {
				if text, ok := allowed.(string); ok && isString && text == actualText {
					matched = true
				}
			}
			if !matched {
				issues = append(issues, EditorValidationIssue{Code: "enum", Path: path, Field: field})
			}
		}
		if constraint["format"] == "uuid" {
			text, _ := value.(string)
			if !diagnosticUUID.MatchString(text) {
				issues = append(issues, EditorValidationIssue{Code: "uuid", Path: path, Field: field})
			}
		}
	}
	if args, ok := input["args"].(map[string]any); ok && depth == 0 {
		if selected := editorArgsDiagnosticSchema(schema, input["operation"]); selected != nil {
			issues = append(issues, diagnosticIssues(selected, args, "arguments.args", depth+1)...)
		}
	}
	if len(issues) > 12 {
		issues = issues[:12]
	}
	return issues
}

// EditorTurnDiagnostics is a read-only projection. It never copies text, values,
// call IDs, unknown keys, tool output, error messages, or model reasoning.
func (r *Runtime) EditorTurnDiagnostics(q RunRequest, result RunResult, tools []Tool, failed bool, causes ...error) EditorTurnDiagnostic {
	d := EditorTurnDiagnostic{Version: 1, At: r.Now().UTC().Format(time.RFC3339), RuntimeOutcome: "returned", RequestedProfile: safeDiagnosticIdentity(q.Profile), MaxSteps: r.config.MaxSteps, InstructionsSHA256: diagnosticHash([]byte(r.config.Instructions)), Models: []EditorModelIdentity{}, EditorDefinitions: []EditorDefinitionIdentity{}, Calls: []EditorCallDiagnostic{}}
	if failed {
		d.RuntimeOutcome = "error"
	}
	if failed && len(causes) > 0 {
		d.Failure = SafeTurnFailure(causes[0])
	}
	systemMessages := []string{}
	for _, message := range q.Messages {
		if message.Role == "system" {
			systemMessages = append(systemMessages, message.Content)
		}
	}
	d.RequestSystemSHA256 = diagnosticJSONHash(systemMessages)
	d.RequestMessageCount = len(q.Messages)
	definitions := []any{}
	known := map[string]Tool{}
	for _, tool := range tools {
		definitions = append(definitions, map[string]any{"type": "function", "function": map[string]any{"name": tool.Name, "description": tool.Description, "parameters": tool.Parameters}})
		if knownEditorDiagnosticTool(tool.Name) {
			known[tool.Name] = tool
			d.EditorDefinitions = append(d.EditorDefinitions, EditorDefinitionIdentity{Tool: tool.Name, DescriptionSHA256: diagnosticHash([]byte(tool.Description)), SchemaSHA256: diagnosticJSONHash(tool.Parameters)})
		}
	}
	d.ToolDefinitionsSHA256 = diagnosticJSONHash(definitions)
	r.mu.Lock()
	for _, record := range r.records {
		if q.RunID != "" && record.RunID == q.RunID {
			identity := EditorModelIdentity{safeDiagnosticIdentity(record.Profile), safeDiagnosticIdentity(record.Model)}
			if len(d.Models) == 0 || d.Models[len(d.Models)-1] != identity {
				if len(d.Models) < 8 {
					d.Models = append(d.Models, identity)
				}
			}
		}
	}
	r.mu.Unlock()
	messages := result.Messages
	if len(messages) >= len(q.Messages) {
		messages = messages[len(q.Messages):]
	} else {
		messages = nil
	}
	pending := map[string]int{}
	step := 0
	for _, message := range messages {
		if message.Role == "assistant" {
			step++
			for _, call := range message.ToolCalls {
				tool, ok := known[call.Function.Name]
				if !ok {
					continue
				}
				if len(d.Calls) >= MaxEditorDiagnosticRecords {
					d.Truncated = true
					continue
				}
				event := EditorCallDiagnostic{Step: step, Tool: tool.Name, Shape: "object", Result: "no_tool_result"}
				var input map[string]any
				if len(call.Function.Arguments) > 64<<10 {
					event.Shape = "size_limit"
				} else if json.Unmarshal([]byte(call.Function.Arguments), &input) != nil || input == nil {
					event.Shape = "not_object"
				} else {
					keys := []string{"instance", "commandId", "operation", "args"}
					if tool.Name == "mcp_editor__editor_open_beads" {
						keys = []string{"instance", "commandId", "view", "projects", "search", "statuses", "priorities", "types", "assignees", "labels", "sort", "lanes", "ticket", "ticketProject", "args", "operation"}
					}
					event.Fields, event.UnknownFields = diagnosticShape(input, keys, 0)
					event.Issues = diagnosticIssues(tool.Parameters, input, "arguments", 0)
				}
				pending[call.ID] = len(d.Calls)
				d.Calls = append(d.Calls, event)
			}
		}
		if message.Role == "tool" {
			index, ok := pending[message.ToolCallID]
			if !ok {
				continue
			}
			delete(pending, message.ToolCallID)
			d.Calls[index].Result = "tool_result"
			if message.ToolValidationCode == "invalid_arguments" {
				d.Calls[index].Result = "recoverable_invalid_arguments"
			}
		}
	}
	sort.Slice(d.EditorDefinitions, func(i, j int) bool { return d.EditorDefinitions[i].Tool < d.EditorDefinitions[j].Tool })
	for {
		encoded, e := json.Marshal(d)
		if e == nil && len(encoded) <= MaxEditorDiagnosticBytes {
			break
		}
		d.Truncated = true
		if len(d.Calls) == 0 {
			break
		}
		d.Calls = d.Calls[:len(d.Calls)-1]
	}
	return d
}
