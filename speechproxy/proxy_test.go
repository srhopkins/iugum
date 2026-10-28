package speechproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFixedUpstreamValidation(t *testing.T) {
	for _, u := range []string{"", "ftp://127.0.0.1", "http://user:pass@localhost", "http://localhost?x=1", "http://localhost/#fragment", "http://localhost/base", "http://localhost//", "http://localhost/%2f"} {
		if _, err := New(Config{Upstream: u, Check: func(context.Context, string, string) error { return nil }}); err == nil {
			t.Fatalf("unsafe upstream accepted %q", u)
		}
	}
}
func TestOnlyExactRoutesAndLocalHealth(t *testing.T) {
	calls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.Write([]byte(`{}`)) }))
	defer up.Close()
	p, err := New(Config{Upstream: up.URL, Check: func(context.Context, string, string) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ method, path string }{{"GET", "/v1/audio/speech"}, {"POST", "/arbitrary"}, {"POST", "/v1/audio/speech?x=1"}, {"POST", "/v1/audio/../audio/speech"}, {"POST", "/v1/audio/speech/"}} {
		w := httptest.NewRecorder()
		p.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code < 400 {
			t.Fatalf("accepted %s %s", tc.method, tc.path)
		}
	}
	w := httptest.NewRecorder()
	p.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 200 || calls != 0 {
		t.Fatal("health or bad routes reached upstream")
	}
}

func speechRequest(t *testing.T, p *Proxy, path string, body []byte, kind string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", path, bytes.NewReader(body))
	r.Header.Set("Content-Type", kind)
	r.Header.Set("Authorization", "Bearer must-not-forward")
	r.Header.Set("Cookie", "session=private")
	r.Header.Set("Proxy-Authorization", "hidden")
	r.Header.Set("X-Forwarded-For", "evil")
	r.Header.Set("Idempotency-Key", "never-replay")
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)
	return w
}
func localProxy(t *testing.T, handler http.HandlerFunc, check func(context.Context, string, string) error, timeout time.Duration) *Proxy {
	t.Helper()
	up := httptest.NewServer(handler)
	t.Cleanup(up.Close)
	if check == nil {
		check = func(context.Context, string, string) error { return nil }
	}
	p, err := New(Config{Upstream: up.URL, Check: check, Timeout: timeout})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}
func ttsJSON(t *testing.T, input string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]string{"model": "tts-1", "voice": "af_nova", "input": input, "response_format": "wav"})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func multipartAudio(t *testing.T, size int) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="dictation.webm"`)
	header.Set("Content-Type", "audio/webm;codecs=opus")
	header.Set("Authorization", "part-private")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(bytes.Repeat([]byte{'x'}, size))
	_ = writer.WriteField("model", "whisper-1")
	_ = writer.Close()
	return body.Bytes(), writer.FormDataContentType()
}
func TestSpeechPreservesContractWithoutForwardedCredentials(t *testing.T) {
	p := localProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != speechPath || r.URL.RawQuery != "" {
			t.Error("wrong upstream request")
		}
		for _, h := range []string{"Authorization", "Cookie", "Proxy-Authorization", "X-Forwarded-For", "Idempotency-Key", "Origin"} {
			if r.Header.Get(h) != "" {
				t.Errorf("forwarded %s", h)
			}
		}
		var q map[string]string
		_ = json.NewDecoder(r.Body).Decode(&q)
		if q["input"] != "synthetic reply" || q["model"] != "tts-1" || q["voice"] != "af_nova" || q["response_format"] != "wav" {
			t.Error(q)
		}
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("Set-Cookie", "private=upstream")
		w.Header().Set("Authorization", "private")
		_, _ = w.Write([]byte("fake-wav"))
	}, nil, 0)
	w := speechRequest(t, p, speechPath, ttsJSON(t, "synthetic reply"), "application/json")
	if w.Code != 200 || w.Body.String() != "fake-wav" || w.Header().Get("Set-Cookie") != "" || w.Header().Get("Authorization") != "" {
		t.Fatalf("response %d %v %s", w.Code, w.Header(), w.Body.String())
	}
	transport := p.client.Transport.(*http.Transport)
	if transport.Proxy != nil || !transport.DisableKeepAlives {
		t.Fatal("ambient proxy or stale-connection replay allowed")
	}
}
func TestSpeechUTF16AndJSONBounds(t *testing.T) {
	var calls atomic.Int32
	p := localProxy(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("fake"))
	}, nil, 0)
	for _, input := range []string{strings.Repeat("a", MaxTextUnits), strings.Repeat("🦉", MaxTextUnits/2)} {
		if w := speechRequest(t, p, speechPath, ttsJSON(t, input), "application/json"); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	for _, raw := range [][]byte{ttsJSON(t, strings.Repeat("a", MaxTextUnits+1)), ttsJSON(t, strings.Repeat("🦉", MaxTextUnits/2+1)), []byte(`{"input":"ok","api_key":"never"}`), []byte(`{"input":"ok"} {"input":"extra"}`), []byte(`{"input":" "}`), bytes.Repeat([]byte{' '}, maxJSONBytes+1)} {
		if w := speechRequest(t, p, speechPath, raw, "application/json"); w.Code < 400 {
			t.Fatal("unsafe JSON accepted")
		}
	}
	if calls.Load() != 2 {
		t.Fatal("invalid text reached upstream")
	}
}
func TestTranscriptionFileBoundaryAndSafeReconstruction(t *testing.T) {
	var calls atomic.Int32
	p := localProxy(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		reader, err := r.MultipartReader()
		if err != nil {
			t.Error(err)
			return
		}
		file, err := reader.NextPart()
		if err != nil {
			t.Error(err)
			return
		}
		if file.FormName() != "file" || file.FileName() != "dictation.webm" || file.Header.Get("Authorization") != "" || file.Header.Get("Content-Type") != "audio/webm" {
			t.Error("unsafe file metadata")
		}
		n, _ := io.Copy(io.Discard, file)
		if n != MaxAudioBytes {
			t.Error(n)
		}
		model, err := reader.NextPart()
		if err != nil {
			t.Error(err)
			return
		}
		b, _ := io.ReadAll(model)
		if model.FormName() != "model" || string(b) != "whisper-1" {
			t.Error("wrong model")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"synthetic transcript","private":"drop me"}`))
	}, nil, 0)
	raw, kind := multipartAudio(t, MaxAudioBytes)
	w := speechRequest(t, p, transcriptionPath, raw, kind)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "synthetic transcript") || strings.Contains(w.Body.String(), "private") {
		t.Fatal(w.Body.String())
	}
	raw, kind = multipartAudio(t, MaxAudioBytes+1)
	if w = speechRequest(t, p, transcriptionPath, raw, kind); w.Code < 400 {
		t.Fatal("oversized audio accepted")
	}
	if calls.Load() != 1 {
		t.Fatal("oversized audio reached upstream")
	}
}
func TestRedirectAndUpstreamErrorPayloadRefused(t *testing.T) {
	var redirected atomic.Int32
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer trap.Close()
	for _, status := range []int{302, 307, 401, 500} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			p := localProxy(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", trap.URL)
				w.Header().Set("Set-Cookie", "private")
				w.WriteHeader(status)
				_, _ = w.Write([]byte("private upstream payload"))
			}, nil, 0)
			w := speechRequest(t, p, speechPath, ttsJSON(t, "fixture"), "application/json")
			if w.Code < 400 || strings.Contains(w.Body.String(), "private") || w.Header().Get("Location") != "" || w.Header().Get("Set-Cookie") != "" {
				t.Fatal(w.Body.String())
			}
		})
	}
	if redirected.Load() != 0 {
		t.Fatal("redirect followed")
	}
}
func TestResponseBoundsAreEnforcedBeforeWriting(t *testing.T) {
	for _, path := range []string{speechPath, transcriptionPath} {
		t.Run(path, func(t *testing.T) {
			p := localProxy(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "audio/wav")
				limit := MaxAudioBytes
				if path == transcriptionPath {
					limit = maxTranscriptBytes
				}
				_, _ = w.Write(bytes.Repeat([]byte{'x'}, limit+1))
			}, nil, 0)
			body, kind := ttsJSON(t, "fixture"), "application/json"
			if path == transcriptionPath {
				body, kind = multipartAudio(t, 1)
			}
			w := speechRequest(t, p, path, body, kind)
			if w.Code != 502 || w.Body.Len() > 200 {
				t.Fatal("unbounded/partial upstream response escaped")
			}
		})
	}
}
func TestStartupAndPerDispatchPolicyDenial(t *testing.T) {
	if _, err := New(Config{Upstream: "http://127.0.0.1:1", Check: func(context.Context, string, string) error { return errors.New("private policy detail") }}); err == nil {
		t.Fatal("startup denied policy ignored")
	}
	var calls atomic.Int32
	allowed := true
	p := localProxy(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }, func(_ context.Context, obj, action string) error {
		if action == "serve" && obj == "agent/speech-relay" {
			return nil
		}
		if obj != "agent/speech-relay"+speechPath || action != "forward" || !allowed {
			return errors.New("denied")
		}
		return nil
	}, 0)
	allowed = false
	w := speechRequest(t, p, speechPath, ttsJSON(t, "fixture"), "application/json")
	if w.Code != 403 || calls.Load() != 0 {
		t.Fatal("revoked policy reached upstream")
	}
}
func TestCancellationTimeoutAndNoRetry(t *testing.T) {
	for _, mode := range []string{"cancel", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			started := make(chan struct{})
			cancelled := make(chan struct{})
			var calls atomic.Int32
			p := localProxy(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				close(started)
				<-r.Context().Done()
				close(cancelled)
			}, nil, 150*time.Millisecond)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := httptest.NewRequest("POST", speechPath, bytes.NewReader(ttsJSON(t, "fixture"))).WithContext(ctx)
			r.Header.Set("Content-Type", "application/json")
			done := make(chan struct{})
			go func() { p.ServeHTTP(httptest.NewRecorder(), r); close(done) }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("synthetic upstream never started")
			}
			if mode == "cancel" {
				cancel()
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("relay did not cancel")
			}
			select {
			case <-cancelled:
			case <-time.After(time.Second):
				t.Fatal("upstream request not cancelled")
			}
			if calls.Load() != 1 {
				t.Fatal("request retried")
			}
		})
	}
}
func TestLiteralLoopbackListenerOnly(t *testing.T) {
	for _, s := range []string{"127.0.0.1:8766", "[::1]:8766", "127.0.0.2:0"} {
		if err := ValidateListen(s); err != nil {
			t.Fatal(s, err)
		}
	}
	for _, s := range []string{"localhost:8766", "0.0.0.0:8766", "[::]:8766", "192.168.1.42:8766", "127.0.0.1:http", "127.0.0.1:65536", "127.0.0.1:"} {
		if ValidateListen(s) == nil {
			t.Fatal("unsafe listen", s)
		}
	}
}

func TestUncertainConnectionResetIsNotRetried(t *testing.T) {
	var calls atomic.Int32
	p := localProxy(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = connection.Close()
	}, nil, 0)
	w := speechRequest(t, p, speechPath, ttsJSON(t, "synthetic"), "application/json")
	if w.Code != 502 || calls.Load() != 1 {
		t.Fatalf("uncertain request retried: code=%d attempts=%d", w.Code, calls.Load())
	}
}

func TestUnknownMultipartMetadataAndTrailingBodyBounds(t *testing.T) {
	var calls atomic.Int32
	p := localProxy(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }, nil, 0)
	for _, field := range []string{"token", "url", "authorization"} {
		var raw bytes.Buffer
		writer := multipart.NewWriter(&raw)
		_ = writer.WriteField(field, "private")
		_ = writer.Close()
		if w := speechRequest(t, p, transcriptionPath, raw.Bytes(), writer.FormDataContentType()); w.Code < 400 {
			t.Fatal("unknown multipart metadata accepted")
		}
	}
	raw, kind := multipartAudio(t, 1)
	raw = append(raw, bytes.Repeat([]byte{'x'}, maxMultipartBytes)...)
	r := httptest.NewRequest("POST", transcriptionPath, bytes.NewReader(raw))
	r.ContentLength = -1
	r.Header.Set("Content-Type", kind)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code < 400 || calls.Load() != 0 {
		t.Fatal("chunked oversized multipart epilogue accepted")
	}
}
