package agentdesk

import "encoding/json"

// Decorate provider results at the presentation boundary; adapters retain their
// public data contracts. HTML always passes through the shared safe renderer.
func renderSearchResults(value any) (any, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err = json.Unmarshal(b, &decoded); err != nil {
		return nil, err
	}
	result, ok := decoded.(map[string]any)
	if !ok {
		return value, nil
	}
	for _, group := range []string{"transcripts", "wiki", "memory", "tasks"} {
		rows, _ := result[group].([]any)
		for _, row := range rows {
			item, ok := row.(map[string]any)
			if !ok {
				continue
			}
			var text string
			for _, key := range []string{"Text", "text", "Value", "value", "description"} {
				if v, ok := item[key].(string); ok {
					text = v
					break
				}
			}
			item["html"] = renderMarkdown(text)
		}
	}
	return result, nil
}
