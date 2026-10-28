package agentmcp

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Diagnostics contain schema-owned field paths and expected types only. Never
// include instance values or validator errors (which may contain secrets).
var feedbackField = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

func argumentFeedback(schema map[string]any, input any) string {
	details := []string{}
	var visit func(map[string]any, any, string, int)
	visit = func(s map[string]any, v any, path string, depth int) {
		if depth > 4 || len(details) >= 12 {
			return
		}
		obj, ok := v.(map[string]any)
		if expected, ok := s["type"].(string); ok {
			matches := true
			switch expected {
			case "object":
				_, matches = v.(map[string]any)
			case "array":
				_, matches = v.([]any)
			case "string":
				_, matches = v.(string)
			case "boolean":
				_, matches = v.(bool)
			case "number":
				_, matches = v.(float64)
			case "integer":
				n, ok := v.(float64)
				matches = ok && n == float64(int64(n))
			case "null":
				matches = v == nil
			default:
				return
			}
			if !matches {
				details = append(details, fmt.Sprintf("%s must be %s", path, expected))
				return
			}
		}
		if values, hasEnum := s["enum"].([]any); hasEnum {
			found := false
			for _, allowed := range values {
				found = found || reflect.DeepEqual(v, allowed)
			}
			if !found && len(details) < 12 {
				details = append(details, path+" must match one of the allowed tool schema values")
			}
		}
		if number, isNumber := v.(float64); isNumber {
			if min, ok := s["minimum"].(float64); ok && number < min && len(details) < 12 {
				details = append(details, fmt.Sprintf("%s must be at least %g", path, min))
			}
			if max, ok := s["maximum"].(float64); ok && number > max && len(details) < 12 {
				details = append(details, fmt.Sprintf("%s must be at most %g", path, max))
			}
		}
		if text, isString := v.(string); isString {
			if min, ok := s["minLength"].(float64); ok && float64(utf8.RuneCountInString(text)) < min && len(details) < 12 {
				details = append(details, path+" is shorter than the tool schema permits")
			}
			if max, ok := s["maxLength"].(float64); ok && float64(utf8.RuneCountInString(text)) > max && len(details) < 12 {
				details = append(details, path+" is longer than the tool schema permits")
			}
			if pattern, hasPattern := s["pattern"].(string); hasPattern {
				if re, e := regexp.Compile(pattern); e == nil && !re.MatchString(text) && len(details) < 12 {
					if s["format"] == "uuid" {
						details = append(details, path+" must be a UUID string in 8-4-4-4-12 hexadecimal form; word labels are invalid")
					} else {
						details = append(details, path+" does not satisfy the tool schema string constraint")
					}
				}
			}
		}
		if !ok {
			return
		}
		if dependencies, exists := s["dependentSchemas"].(map[string]any); exists {
			for key, raw := range dependencies {
				if _, present := obj[key]; present {
					if child, ok := raw.(map[string]any); ok {
						visit(child, v, path, depth+1)
					}
				}
			}
		}
		if conditions, exists := s["allOf"].([]any); exists {
			for _, raw := range conditions {
				branch, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				condition, ok := branch["if"].(map[string]any)
				if !ok {
					continue
				}
				props, ok := condition["properties"].(map[string]any)
				if !ok || len(props) != 1 {
					continue
				}
				for key, rawConstraint := range props {
					constraint, ok := rawConstraint.(map[string]any)
					if !ok {
						continue
					}
					constant, hasConst := constraint["const"]
					actual, present := obj[key]
					if hasConst && present && reflect.DeepEqual(constant, actual) {
						if then, ok := branch["then"].(map[string]any); ok {
							visit(then, v, path, depth+1)
						}
					}
				}
			}
		}
		if dependencies, exists := s["dependentRequired"].(map[string]any); exists {
			for key, raw := range dependencies {
				if _, present := obj[key]; !present {
					continue
				}
				if required, ok := raw.([]any); ok {
					for _, item := range required {
						if field, ok := item.(string); ok && feedbackField.MatchString(field) {
							if _, exists := obj[field]; !exists && len(details) < 12 {
								details = append(details, "missing required field "+path+"."+field)
							}
						}
					}
				}
			}
		}
		properties, _ := s["properties"].(map[string]any)
		required, _ := s["required"].([]any)
		for _, item := range required {
			key, ok := item.(string)
			if !ok || !feedbackField.MatchString(key) {
				continue
			}
			if _, exists := obj[key]; !exists && len(details) < 12 {
				details = append(details, "missing required field "+path+"."+key)
			}
		}
		keys := []string{}
		for key := range properties {
			if feedbackField.MatchString(key) {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		if additional, exists := s["additionalProperties"].(bool); exists && !additional && len(details) < 12 {
			for key := range obj {
				if _, known := properties[key]; !known {
					if len(keys) > 0 && len(keys) <= 16 {
						details = append(details, path+" permits only fields "+strings.Join(keys, ", "))
					}
					break
				}
			}
		}
		for _, key := range keys {
			if value, exists := obj[key]; exists {
				if child, ok := properties[key].(map[string]any); ok {
					visit(child, value, path+"."+key, depth+1)
				}
			}
		}
	}
	visit(schema, input, "arguments", 0)
	message := "MCP arguments do not match the tool input schema."
	if len(details) > 0 {
		message += " " + strings.Join(details, "; ") + "."
	}
	return message + " This call was not sent and queued no action. Correct these arguments using the tool schema and continue the original request in this turn. This local validation error does not require user approval. Do not replay any earlier queued, dispatched, or uncertain action."
}
