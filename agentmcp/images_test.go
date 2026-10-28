package agentmcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/srhopkins/iugum/agentcore"
)

func TestSDKPNGActualModelImageWire(t *testing.T) {
	var buf bytes.Buffer
	png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	original := append([]byte(nil), buf.Bytes()...)
	server := mcp.NewServer(&mcp.Implementation{Name: "image-test", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "screen", Description: "Synthetic PNG"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: `{"observation":"synthetic-original-observation","fresh":true}`}, &mcp.ImageContent{MIMEType: "image/png", Data: original}}}, nil, nil
	})
	sdk := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	defer sdk.Close()
	ctx := context.Background()
	tools, close, e := Connect(ctx, []Config{{Name: "screen", URL: sdk.URL, AllowImages: true}}, "agent:test", &testGate{})
	if e != nil {
		t.Fatal(e)
	}
	defer close()
	if _, e = tools[0].Execute(ctx, json.RawMessage(`{}`)); e == nil {
		t.Fatal("legacy string consumer silently stripped image")
	}
	calls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"original-call","type":"function","function":{"name":"mcp_screen__screen","arguments":"{}"}}]}}]}`))
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		raw, _ := json.Marshal(body)
		if !bytes.Contains(raw, []byte("data:image/png;base64,"+base64.StdEncoding.EncodeToString(original))) || !bytes.Contains(raw, []byte("synthetic-original-observation")) || !bytes.Contains(raw, []byte("original-call")) {
			t.Error("actual SDK PNG or provenance missing")
		}
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"saw synthetic raster"}}]}`))
	}))
	defer model.Close()
	cfg := agentcore.Config{DefaultProfile: "vision", Pools: map[string]agentcore.Pool{"synthetic": {Provider: "openai", BaseURL: model.URL}}, Profiles: map[string]agentcore.Profile{"vision": {Pool: "synthetic", Model: "synthetic", Vision: true}}}
	runtime, e := agentcore.New(cfg, "", &testGate{})
	if e != nil {
		t.Fatal(e)
	}
	_, e = runtime.Run(ctx, agentcore.RunRequest{Actor: "agent:test", Messages: []agentcore.Message{{Role: "user", Content: "original human image question"}}}, tools)
	if e != nil || calls != 2 {
		t.Fatal(e, calls)
	}

	// Join the actual SDK image tool to the actual subscription decision loop,
	// substituting only the external CLI process with a local fixed script.
	temp := t.TempDir()
	binary := filepath.Join(temp, "fake-codex")
	attached := filepath.Join(temp, "image.png")
	promptFile := filepath.Join(temp, "prompt")
	script := fmt.Sprintf(`#!/bin/sh
while [ "$#" -gt 0 ]; do
 case "$1" in --image) shift; image="$1";; -o) shift; output="$1";; esac
 shift
done
prompt=$(cat)
if [ -n "$image" ]; then
 cp "$image" '%s'
 printf '%%s' "$prompt" > '%s'
 printf '%%s' '{"reply":"saw original PNG","tool_calls":[]}' > "$output"
else
 printf '%%s' '{"reply":"","tool_calls":[{"name":"mcp_screen__screen","arguments":{}}]}' > "$output"
fi
printf '%%s\n' '{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}'
`, attached, promptFile)
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	cfg.SubscriptionVisionModels = []string{"synthetic-vision"}
	sub, e := agentcore.New(cfg, "", &testGate{})
	if e != nil {
		t.Fatal(e)
	}
	answer, e := sub.RunSubscriptionTools(ctx, agentcore.RunRequest{Actor: "agent:test", Messages: []agentcore.Message{{Role: "user", Content: "original human image question"}}}, binary, "synthetic-vision", tools)
	if e != nil || answer.Text != "saw original PNG" {
		t.Fatal(answer.Text, e)
	}
	copied, _ := os.ReadFile(attached)
	if !bytes.Equal(copied, original) {
		t.Fatal("SDK-to-CLI PNG changed")
	}
	prompt, _ := os.ReadFile(promptFile)
	if !bytes.Contains(prompt, []byte(`"current_user_request":"original human image question"`)) || !bytes.Contains(prompt, []byte("synthetic-original-observation")) {
		t.Fatal("subscription anchor/provenance missing")
	}
	denied, stop, e := Connect(ctx, []Config{{Name: "screen", URL: sdk.URL}}, "agent:test", &testGate{})
	if e != nil {
		t.Fatal(e)
	}
	defer stop()
	if _, e = denied[0].ExecuteResult(ctx, json.RawMessage(`{}`)); e == nil || !strings.Contains(e.Error(), "allow_images") {
		t.Fatal(e)
	}
}

func TestMCPImageFailuresNeverBecomeTruncatedText(t *testing.T) {
	var buf bytes.Buffer
	png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	for name, result := range map[string]*mcp.CallToolResult{
		"svg":               {Content: []mcp.Content{&mcp.ImageContent{MIMEType: "image/svg+xml", Data: []byte("<svg/>")}}},
		"invalid-png":       {Content: []mcp.Content{&mcp.ImageContent{MIMEType: "image/png", Data: []byte("<html/>")}}},
		"metadata-overflow": {Content: []mcp.Content{&mcp.ImageContent{MIMEType: "image/png", Data: buf.Bytes()}, &mcp.TextContent{Text: strings.Repeat("x", agentcore.MaxImageMetadataBytes)}}},
		"two-images":        {Content: []mcp.Content{&mcp.ImageContent{MIMEType: "image/png", Data: buf.Bytes()}, &mcp.ImageContent{MIMEType: "image/png", Data: buf.Bytes()}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := toolResult(result, true); err == nil {
				t.Fatal("invalid image accepted")
			}
		})
	}
	reader := &boundedReader{reader: strings.NewReader(strings.Repeat("x", 101)), remaining: 100}
	raw, err := io.ReadAll(reader)
	if err == nil || len(raw) > 100 {
		t.Fatal("oversized stream accepted", len(raw), err)
	}
	plain, err := toolResult(&mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: strings.Repeat("x", maxResultBytes+100)}}}, false)
	if err != nil || !strings.Contains(plain.Text, "Result truncated") {
		t.Fatal("text compatibility changed", err)
	}
}
