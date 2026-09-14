package agentdesk

import (
	"strings"
	"testing"
)

func TestChatMarkdownAndSafety(t *testing.T) {
	got := renderChat("**Done**\n\n- first\n- second\n\n```go\nfmt.Println(1)\n```\n\n| A | B |\n|---|---|\n| 1 | 2 |\n\n<script>alert(1)</script>\n\n[bad](javascript:alert(1))")
	for _, want := range []string{"<strong>Done</strong>", "<ul>", "<pre><code", "fmt.Println(1)", "<table>"} {
		if !strings.Contains(got, want) {
			t.Fatal(want, got)
		}
	}
	for _, bad := range []string{"<script", "javascript:"} {
		if strings.Contains(got, bad) {
			t.Fatal(got)
		}
	}
}
func TestStatusEvidenceIsCollapsedAndCodePreserved(t *testing.T) {
	got := renderChat("Latest session: claude — ffai\nLast recorded message: **Fixed**\n\n```js\ncall();\n```\nSession: abc\nSource: /private/path\nEarlier context: Investigating.\nIndex checked today.")
	for _, want := range []string{"<details><summary>Last recorded message</summary>", "<strong>Fixed</strong>", "<pre><code", "<summary>Source details</summary>", "<summary>Index freshness</summary>"} {
		if !strings.Contains(got, want) {
			t.Fatal(want, got)
		}
	}
	if strings.Contains(got, "<details open") {
		t.Fatal(got)
	}
}
