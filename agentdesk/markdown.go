package agentdesk

import (
	"bytes"
	"html"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

var chatMarkdown = goldmark.New(goldmark.WithExtensions(extension.GFM))
var chatHTMLPolicy = bluemonday.UGCPolicy()

func renderMarkdown(text string) string {
	var out bytes.Buffer
	if err := chatMarkdown.Convert([]byte(text), &out); err != nil {
		return html.EscapeString(text)
	}
	return chatHTMLPolicy.Sanitize(out.String())
}

type messageView struct {
	Message
	HTML string `json:"html"`
}

// Keep the durable Markdown unchanged. Only the response carries rendered HTML.
func viewMessage(m Message) messageView {
	return messageView{m, renderChat(m.Text)}
}

var statusSection = regexp.MustCompile(`(?m)^(Last recorded message|Session|Source|Earlier context|Index checked|Transcript indexing|Index coverage):? `)

func renderChat(text string) string {
	// Status evidence is presented separately from the answer. This also improves
	// existing saved status replies without migrating or rewriting history.
	if !strings.Contains(text, "Latest session:") {
		return renderMarkdown(text)
	}
	matches := statusSection.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return renderMarkdown(text)
	}
	heading := text[:matches[0][0]]
	heading = regexp.MustCompile(`(Latest session: [^\n]+ — )(/\S+)`).ReplaceAllStringFunc(heading, func(line string) string {
		split := strings.LastIndex(line, " — ")
		return line[:split+len(" — ")] + filepath.Base(line[split+len(" — "):])
	})
	main := renderMarkdown(heading)
	var source, freshness strings.Builder
	for n, match := range matches {
		end := len(text)
		if n+1 < len(matches) {
			end = matches[n+1][0]
		}
		label := strings.TrimSpace(strings.TrimSuffix(text[match[0]:match[1]], " "))
		body := strings.TrimSpace(text[match[1]:end])
		switch strings.TrimSuffix(label, ":") {
		case "Last recorded message", "Earlier context":
			main += "<details><summary>" + html.EscapeString(strings.TrimSuffix(label, ":")) + "</summary>" + renderMarkdown(body) + "</details>"
		case "Session", "Source":
			source.WriteString("<p>" + html.EscapeString(label) + " <code>" + html.EscapeString(body) + "</code></p>")
		default:
			freshness.WriteString("<p>" + html.EscapeString(label+" "+body) + "</p>")
		}
	}
	if source.Len() > 0 {
		main += "<details><summary>Source details</summary>" + source.String() + "</details>"
	}
	if freshness.Len() > 0 {
		main += "<details><summary>Index freshness</summary>" + freshness.String() + "</details>"
	}
	return main
}
