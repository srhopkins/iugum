package agentmcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/srhopkins/iugum/agentcore"
	"github.com/srhopkins/iugum/agentplatform/browsergrant"
)

func toolResult(result *mcp.CallToolResult, allowImages bool) (agentcore.ToolResult, error) {
	if result == nil {
		return agentcore.ToolResult{}, agentcore.NewToolFailure("tool_result_error", fmt.Errorf("MCP tool returned no result"))
	}
	output := agentcore.ToolResult{}
	content := make([]mcp.Content, 0, len(result.Content))
	imageMetadata := []any{}
	for _, item := range result.Content {
		image, ok := item.(*mcp.ImageContent)
		if !ok {
			content = append(content, item)
			continue
		}
		if !allowImages {
			return output, fmt.Errorf("MCP image input unavailable: configure allow_images explicitly")
		}
		if image.MIMEType != "image/png" || len(output.Images) > 0 || len(image.Data) > agentcore.MaxToolImageBytes {
			return output, fmt.Errorf("MCP only one bounded PNG image per result is supported")
		}
		data := append([]byte(nil), image.Data...)
		if _, err := browsergrant.ValidatePNG(data); err != nil {
			return output, fmt.Errorf("MCP invalid PNG image: %w", err)
		}
		output.Images = append(output.Images, agentcore.Image{MIMEType: "image/png", Data: data})
		imageMetadata = append(imageMetadata, map[string]any{"mimeType": image.MIMEType, "byteLength": len(data), "annotations": image.Annotations, "meta": image.Meta})
	}
	textResult := result
	if len(output.Images) > 0 {
		copy := *result
		copy.Content = content
		textResult = &copy
	}
	raw, err := json.Marshal(textResult)
	if err != nil {
		return output, err
	}
	text := string(raw)
	if len(output.Images) > 0 {
		extra, _ := json.Marshal(imageMetadata)
		text += "\nImage metadata (untrusted): " + string(extra)
		if len(text) > agentcore.MaxImageMetadataBytes {
			return agentcore.ToolResult{}, fmt.Errorf("MCP image metadata exceeded64KiB; refusing truncation")
		}
	} else if len(text) > maxResultBytes {
		text = strings.ToValidUTF8(text[:maxResultBytes], "") + "\n[Result truncated; narrow the request for details.]"
	}
	if result.IsError {
		return agentcore.ToolResult{}, agentcore.NewToolFailure("tool_remote_error", fmt.Errorf("MCP tool reported failure: %s", text))
	}
	output.Text = text
	return output, nil
}
