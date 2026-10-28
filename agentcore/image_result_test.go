package agentcore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func imagePNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestTypedImageBatchActualModelWire(t *testing.T) {
	original := imagePNG(t)
	mutable := append([]byte(nil), original...)
	calls := 0
	const metadata = `{"observation":"33333333-3333-4333-8333-333333333333","viewport":{"width":2,"height":2}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"image-call","type":"function","function":{"name":"screen","arguments":"{}"}},{"id":"text-call","type":"function","function":{"name":"text","arguments":"{}"}}]}}]}`))
			return
		}
		var q struct {
			Messages []struct {
				Role       string          `json:"role"`
				Content    json.RawMessage `json:"content"`
				ToolCallID string          `json:"tool_call_id"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
			return
		}
		if len(q.Messages) != 6 || q.Messages[3].Role != "tool" || q.Messages[4].Role != "tool" || q.Messages[5].Role != "user" {
			t.Errorf("tool batch ordering: %+v", q.Messages)
			return
		}
		if q.Messages[3].ToolCallID != "image-call" || q.Messages[4].ToolCallID != "text-call" {
			t.Error("original call IDs lost")
		}
		var text string
		if err := json.Unmarshal(q.Messages[3].Content, &text); err != nil || text != metadata {
			t.Errorf("tool role must remain original text: %s", q.Messages[3].Content)
		}
		var parts []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			ImageURL struct {
				URL string `json:"url"`
			} `json:"image_url"`
		}
		if err := json.Unmarshal(q.Messages[5].Content, &parts); err != nil {
			t.Error(err)
			return
		}
		if len(parts) != 2 || !strings.Contains(parts[0].Text, "UNTRUSTED TOOL IMAGE") || !strings.Contains(parts[0].Text, "image-call") || !strings.Contains(parts[0].Text, "original human instruction") || !strings.Contains(parts[0].Text, "33333333-") {
			t.Error("image provenance/current human request lost")
		}
		const prefix = "data:image/png;base64,"
		if !strings.HasPrefix(parts[1].ImageURL.URL, prefix) {
			t.Error("not fixed inline PNG")
		}
		decoded, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(parts[1].ImageURL.URL, prefix))
		if e != nil || !bytes.Equal(decoded, original) || sha256.Sum256(decoded) != sha256.Sum256(original) {
			t.Error("original PNG bytes changed")
		}
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"saw exact image"}}]}`))
	}))
	defer server.Close()
	cfg := config(server.URL)
	p := cfg.Profiles["mini"]
	p.Vision = true
	cfg.Profiles["mini"] = p
	r, _ := New(cfg, "", gate{})
	out, err := r.Run(context.Background(), RunRequest{Actor: "test", Messages: []Message{{Role: "user", Content: "original human instruction"}}}, []Tool{{Name: "screen", ExecuteResult: func(context.Context, json.RawMessage) (ToolResult, error) {
		return ToolResult{Text: metadata, Images: []Image{{MIMEType: "image/png", Data: mutable}}}, nil
	}}, {Name: "text", Execute: func(context.Context, json.RawMessage) (string, error) {
		mutable[0] = 0
		return "unchanged text result", nil
	}}})
	if err != nil || out.Text != "saw exact image" || calls != 2 {
		t.Fatal(out.Text, err, calls)
	}
	stored, _ := json.Marshal(out.Messages)
	if bytes.Contains(stored, []byte(base64.StdEncoding.EncodeToString(original))) || bytes.Contains(stored, []byte("data:image")) {
		t.Fatal("PNG leaked into history JSON")
	}
}
func TestImageVisionAndFallbackDenyBeforeNextModelCall(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(map[bool]string{false: "unsupported", true: "fallback"}[fallback], func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"one","type":"function","function":{"name":"image","arguments":"{}"}}]}}],"usage":{"prompt_tokens":1000,"completion_tokens":100}}`))
			}))
			defer s.Close()
			cfg := config(s.URL)
			if fallback {
				p := cfg.Profiles["mini"]
				p.Vision = true
				cfg.Profiles["mini"] = p
				pool := cfg.Pools["openai"]
				pool.MonthlyTargetUSD = 0.00000001
				pool.FallbackProfile = "text-only"
				cfg.Pools["openai"] = pool
				cfg.Pools["other"] = Pool{BaseURL: s.URL}
				cfg.Profiles["text-only"] = Profile{Pool: "other", Model: "text"}
			}
			r, _ := New(cfg, "", gate{})
			_, err := r.Run(context.Background(), RunRequest{Actor: "test"}, []Tool{{Name: "image", ExecuteResult: func(context.Context, json.RawMessage) (ToolResult, error) {
				return ToolResult{Text: "observation metadata", Images: []Image{{MIMEType: "image/png", Data: imagePNG(t)}}}, nil
			}}})
			if err == nil || !strings.Contains(err.Error(), "vision opt-in") || calls != 1 {
				t.Fatal(err, calls)
			}
		})
	}
}
func TestImageAggregateBoundsBeforeModelIO(t *testing.T) {
	for _, images := range [][]Image{make([]Image, 17), {{MIMEType: "image/png", Data: make([]byte, 8<<20)}, {MIMEType: "image/png", Data: make([]byte, 8<<20)}, {MIMEType: "image/png", Data: make([]byte, 1)}}} {
		if err := imageAggregate([]Message{{Images: images}}); err == nil {
			t.Fatal("aggregate overflow accepted")
		}
	}
}
func TestStrictPNGToolBoundaryRejectsActiveMalformedAndOversize(t *testing.T) {
	valid := imagePNG(t)
	chunk := func(kind string, data []byte) []byte {
		b := make([]byte, len(data)+12)
		binary.BigEndian.PutUint32(b, uint32(len(data)))
		copy(b[4:], kind)
		copy(b[8:], data)
		binary.BigEndian.PutUint32(b[len(b)-4:], crc32.ChecksumIEEE(b[4:len(b)-4]))
		return b
	}
	active := append([]byte(nil), valid[:len(valid)-12]...)
	active = append(active, chunk("tEXt", []byte("payload"))...)
	active = append(active, valid[len(valid)-12:]...)
	for _, im := range []Image{{MIMEType: "image/svg+xml", Data: valid}, {MIMEType: "image/png", Data: []byte("<svg/>")}, {MIMEType: "image/png", Data: []byte("<html/>")}, {MIMEType: "image/png", Data: valid[:len(valid)-1]}, {MIMEType: "image/png", Data: append(append([]byte(nil), valid...), 0)}, {MIMEType: "image/png", Data: active}, {MIMEType: "image/png", Data: make([]byte, MaxToolImageBytes+1)}} {
		if _, err := validatedImages([]Image{im}); err == nil {
			t.Fatal("invalid/active image accepted")
		}
	}
}
