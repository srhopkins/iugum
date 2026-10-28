package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testLab(upstream string) *lab {
	return &lab{speech: upstream, modelBase: upstream, model: "fixture", token: "fixture-secret", origin: "https://iugum-agent-code.local", client: &http.Client{Timeout: time.Second * 3}, stt: make(chan struct{}, 1), tts: make(chan struct{}, 1), llm: make(chan struct{}, 1)}
}

func TestNativeProxyUsesFixedConversationAndNoModelCredential(t *testing.T) {
	calls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/draft" || r.URL.Query().Get("conversation") != "main" || r.Header.Get("Authorization") != "" {
			t.Error("native destination or credential isolation changed")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ready":true,"draft_id":"opaque"}`)
	}))
	defer up.Close()
	l := testLab("")
	l.native, l.conversation = up.URL, "main"
	w := httptest.NewRecorder()
	l.ServeHTTP(w, request("/api/native/draft", `{"operation":"prepare"}`))
	if w.Code != 200 || calls != 1 || !strings.Contains(w.Body.String(), "opaque") {
		t.Fatal(w.Code, calls, w.Body.String())
	}
	w = httptest.NewRecorder()
	l.ServeHTTP(w, request("/api/native/draft?conversation=elsewhere", `{}`))
	if w.Code != 400 || calls != 1 {
		t.Fatal("client-selected conversation reached native agent")
	}
}
func request(path, body string) *http.Request {
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	r.Header.Set("X-Voice-Lab", "1")
	r.Header.Set("Content-Type", "application/json")
	return r
}
func wavFixture() []byte {
	b := make([]byte, 364)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 16000)
	binary.LittleEndian.PutUint32(b[28:], 32000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(len(b)-44))
	return b
}
func TestOriginAndRoutes(t *testing.T) {
	l := testLab("")
	for _, tc := range []struct {
		path, origin, header string
		want                 int
	}{{"/api/speak", "https://evil.example", "1", 403}, {"/api/speak", "", "", 403}, {"/api/unknown", "", "1", 404}, {"/api/speak?upstream=evil", "", "1", 400}} {
		r := request(tc.path, `{"text":"hi"}`)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-Voice-Lab", tc.header)
		w := httptest.NewRecorder()
		l.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s: got%d want%d", tc.path, w.Code, tc.want)
		}
	}
}
func TestTranscriptionAndAudioBounds(t *testing.T) {
	calls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/audio/transcriptions" {
			t.Error("wrong route")
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("speech received model credential")
		}
		f, _, e := r.FormFile("file")
		if e != nil {
			t.Error(e)
			return
		}
		defer f.Close()
		b, _ := io.ReadAll(f)
		if !validWAV(b) {
			t.Error("invalid uploaded audio")
		}
		_, _ = io.WriteString(w, `{"text":"fixture words"}`)
	}))
	defer up.Close()
	l := testLab(up.URL)
	l.modelBase = up.URL + "/model"
	for _, b := range [][]byte{[]byte("not audio"), make([]byte, maxAudio+1), wavFixture()} {
		r := httptest.NewRequest("POST", "/api/transcribe", bytes.NewReader(b))
		r.Header.Set("X-Voice-Lab", "1")
		w := httptest.NewRecorder()
		l.ServeHTTP(w, r)
		if validWAV(b) {
			if w.Code != 200 || !strings.Contains(w.Body.String(), "fixture words") {
				t.Fatal(w.Code, w.Body.String())
			}
		} else if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	if calls != 1 {
		t.Fatal("invalid audio reached upstream", calls)
	}
}
func TestTTSFirstChunkAndCancellation(t *testing.T) {
	sent := make(chan struct{})
	cancelled := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("model token leaked to speech")
		}
		w.Header().Set("Content-Type", "audio/pcm")
		_, _ = w.Write([]byte{1, 2})
		w.(http.Flusher).Flush()
		close(sent)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer up.Close()
	l := testLab(up.URL)
	l.modelBase = up.URL + "/model"
	srv := httptest.NewServer(l)
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	r := request("/api/speak", `{"text":"synthetic"}`)
	r.URL.Scheme = "http"
	r.URL.Host = strings.TrimPrefix(srv.URL, "http://")
	r.RequestURI = ""
	r = r.WithContext(ctx)
	res, e := http.DefaultClient.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	<-sent
	b := make([]byte, 2)
	if _, e = io.ReadFull(res.Body, b); e != nil {
		t.Fatal("first audio was buffered", e)
	}
	cancel()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("upstream did not cancel")
	}
}
func TestModelStream(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-secret" {
			t.Error("missing private model credential")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello.\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer up.Close()
	l := testLab(up.URL)
	w := httptest.NewRecorder()
	l.ServeHTTP(w, request("/api/respond", `{"text":"hello","history":[]}`))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Hello.") || !strings.Contains(w.Body.String(), "firstTokenMilliseconds") {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestBusyDoesNotQueue(t *testing.T) {
	l := testLab("")
	l.stt <- struct{}{}
	w := httptest.NewRecorder()
	r := request("/api/transcribe", "")
	l.ServeHTTP(w, r)
	if w.Code != 429 {
		t.Fatal(w.Code)
	}
}
func TestJSONRejectsExtraFields(t *testing.T) {
	l := testLab("")
	for _, body := range []string{`{"text":"hi","upstream":"evil"}`, `{"text":"hi"} {"text":"again"}`} {
		w := httptest.NewRecorder()
		l.ServeHTTP(w, request("/api/speak", body))
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
}
