package agentdesk

import (
	"strings"
	"testing"
)

func TestSearchUsesSafeMarkdownForEverySource(t *testing.T) {
	input := map[string]any{"transcripts": []any{map[string]any{"Text": "**Transcript**\n\n<script>bad()</script>"}}, "wiki": []any{map[string]any{"text": "**Wiki**"}}, "memory": []any{map[string]any{"Value": "**Memory**"}}, "tasks": []any{map[string]any{"description": "**Task**"}}}
	v, err := renderSearchResults(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range []string{"transcripts", "wiki", "memory", "tasks"} {
		item := v.(map[string]any)[group].([]any)[0].(map[string]any)
		h := item["html"].(string)
		if !strings.Contains(h, "<strong>") || strings.Contains(h, "<script") {
			t.Fatal(group, h)
		}
	}
	if _, ok := input["transcripts"].([]any)[0].(map[string]any)["html"]; ok {
		t.Fatal("provider result mutated")
	}
}
