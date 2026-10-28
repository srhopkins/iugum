// A disposable voice lab. This does not replace the installed Iugum Chat.
package main

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

//go:embed web/*
var assets embed.FS

const maxAudio = 1024 * 1024 // More than30seconds of16kHz monoPCM16.

type lab struct {
	speech, modelBase, model, token, origin string
	native, conversation                    string
	client                                  *http.Client
	stt, tts, llm                           chan struct{}
}

func fixedBase(raw string) (string, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("explicit HTTP(S) service URL without credentials required")
	}
	return strings.TrimRight(raw, "/"), nil
}

func main() {
	listen := flag.String("listen", "127.0.0.1:3862", "loopback listener")
	speech := flag.String("speech", "", "approved fixed speech server")
	modelBase := flag.String("model-base", "", "fixed OpenAI-compatible model API base including /v1")
	model := flag.String("model", "", "local model name")
	tokenFile := flag.String("model-token-file", "", "private token file, never returned to browser")
	origin := flag.String("origin", "https://iugum-agent-code.local", "allowed browser origin")
	nativeBase := flag.String("native", "", "fixed native agent URL; draft-enabled isolated instance")
	conversation := flag.String("conversation", "main", "fixed native conversation")
	flag.Parse()
	host, _, e := net.SplitHostPort(*listen)
	if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		log.Fatal("loopback listener required")
	}
	s, e := fixedBase(*speech)
	if e != nil {
		log.Fatal(e)
	}
	m, e := fixedBase(*modelBase)
	if e != nil {
		log.Fatal(e)
	}
	n := ""
	if *nativeBase != "" {
		n, e = fixedBase(*nativeBase)
		if e != nil {
			log.Fatal(e)
		}
	}
	if *conversation != "main" {
		if len(*conversation) != 32 {
			log.Fatal("invalid conversation")
		}
		for _, c := range *conversation {
			if !strings.ContainsRune("0123456789abcdef", c) {
				log.Fatal("invalid conversation")
			}
		}
	}
	token := ""
	if *tokenFile != "" {
		b, e := os.ReadFile(*tokenFile)
		if e != nil {
			log.Fatal("model token unavailable")
		}
		token = strings.TrimSpace(string(b))
	}
	l := &lab{speech: s, modelBase: m, model: *model, token: token, origin: *origin, client: &http.Client{Transport: &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 4, ResponseHeaderTimeout: 45 * time.Second}, Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, stt: make(chan struct{}, 1), tts: make(chan struct{}, 1), llm: make(chan struct{}, 1)}
	l.native = n
	l.conversation = *conversation
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := &http.Server{Addr: *listen, Handler: l, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 40 * time.Second, WriteTimeout: 100 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384, BaseContext: func(net.Listener) context.Context { return ctx }}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()
	log.Printf("Voice lab ready on %s; payloads are not logged", *listen)
	if e = srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Fatal(e)
	}
}

func (l *lab) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; media-src 'self' blob:; frame-ancestors 'self'")
	if r.URL.RawQuery != "" {
		http.Error(w, "query parameters are not supported", 400)
		return
	}
	if r.Method == "GET" {
		switch r.URL.Path {
		case "/api/health":
			jsonReply(w, map[string]any{"ready": true, "model": l.model, "partialMode": "bounded snapshots", "maxUtteranceSeconds": 30, "nativeConversation": l.conversation, "nativeConfigured": l.native != ""})
			return
		case "/", "/index.html", "/app.mjs", "/core.mjs", "/draft.mjs", "/style.css":
			path := r.URL.Path
			if path == "/" {
				path = "/index.html"
			}
			b, e := assets.ReadFile("web" + path)
			if e != nil {
				http.NotFound(w, r)
				return
			}
			switch {
			case strings.HasSuffix(path, ".mjs"):
				w.Header().Set("Content-Type", "text/javascript")
			case strings.HasSuffix(path, ".css"):
				w.Header().Set("Content-Type", "text/css")
			default:
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
			}
			_, _ = w.Write(b)
			return
		}
	}
	if r.Method != "POST" {
		http.NotFound(w, r)
		return
	}
	// A same-origin custom header prevents cross-site form submissions. No CORS.
	if r.Header.Get("X-Voice-Lab") != "1" || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != l.origin) {
		http.Error(w, "same-origin voice lab request required", 403)
		return
	}
	switch r.URL.Path {
	case "/api/transcribe":
		l.transcribe(w, r)
	case "/api/speak":
		l.speak(w, r)
	case "/api/respond":
		l.respond(w, r)
	case "/api/native/draft", "/api/native/chat":
		l.nativeRequest(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (l *lab) nativeRequest(w http.ResponseWriter, r *http.Request) {
	if l.native == "" {
		http.Error(w, "native draft instance is not configured", 503)
		return
	}
	b, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 20<<10))
	if e != nil || !json.Valid(b) {
		http.Error(w, "bounded JSON required", 400)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/native/")
	target := l.native + "/api/" + path + "?conversation=" + url.QueryEscape(l.conversation)
	req, e := http.NewRequestWithContext(r.Context(), "POST", target, bytes.NewReader(b))
	if e != nil {
		http.Error(w, "invalid native request", 400)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	res, e := l.client.Do(req)
	if e != nil {
		http.Error(w, "native agent unavailable; uncertain submissions are not retried", 502)
		return
	}
	defer res.Body.Close()
	body, e := io.ReadAll(io.LimitReader(res.Body, (128<<10)+1))
	if e != nil || len(body) > 128<<10 {
		http.Error(w, "native response unavailable", 502)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(res.StatusCode)
	_, _ = w.Write(body)
}
func jsonReply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func take(w http.ResponseWriter, r *http.Request, c chan struct{}) bool {
	select {
	case c <- struct{}{}:
		return true
	default:
		http.Error(w, "voice lane busy; do not queue stale requests", 429)
		return false
	}
}
func readJSON(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "JSON required", 415)
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		http.Error(w, "invalid or oversized JSON", 400)
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		http.Error(w, "one JSON value required", 400)
		return false
	}
	return true
}
func (l *lab) send(ctx context.Context, path, kind string, body io.Reader) (*http.Response, error) {
	req, e := http.NewRequestWithContext(ctx, "POST", path, body)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", kind)
	if strings.HasPrefix(path, l.modelBase+"/") && l.token != "" {
		req.Header.Set("Authorization", "Bearer "+l.token)
	}
	res, e := l.client.Do(req)
	if e != nil {
		return nil, e
	}
	if res.StatusCode != 200 {
		res.Body.Close()
		return nil, fmt.Errorf("upstream status %d", res.StatusCode)
	}
	return res, nil
}
func validWAV(b []byte) bool {
	return len(b) > 44 && len(b) <= maxAudio && string(b[:4]) == "RIFF" && string(b[8:16]) == "WAVEfmt " && binary.LittleEndian.Uint16(b[20:22]) == 1 && binary.LittleEndian.Uint16(b[22:24]) == 1 && binary.LittleEndian.Uint32(b[24:28]) == 16000 && binary.LittleEndian.Uint16(b[34:36]) == 16 && string(b[36:40]) == "data" && int(binary.LittleEndian.Uint32(b[40:44])) == len(b)-44
}
func (l *lab) transcribe(w http.ResponseWriter, r *http.Request) {
	if !take(w, r, l.stt) {
		return
	}
	defer func() { <-l.stt }()
	start := time.Now()
	b, e := io.ReadAll(http.MaxBytesReader(w, r.Body, maxAudio))
	if e != nil || !validWAV(b) {
		http.Error(w, "bounded16kHz monoPCM WAV required", 400)
		return
	}
	var form bytes.Buffer
	mp := multipart.NewWriter(&form)
	f, _ := mp.CreateFormFile("file", "voice.wav")
	_, _ = f.Write(b)
	_ = mp.WriteField("model", "whisper-1")
	_ = mp.Close()
	res, e := l.send(r.Context(), l.speech+"/v1/audio/transcriptions", mp.FormDataContentType(), &form)
	if e != nil {
		http.Error(w, "transcription service unavailable", 502)
		return
	}
	defer res.Body.Close()
	var result struct {
		Text string `json:"text"`
	}
	d := json.NewDecoder(io.LimitReader(res.Body, 65536))
	if d.Decode(&result) != nil {
		http.Error(w, "invalid transcript response", 502)
		return
	}
	jsonReply(w, map[string]any{"text": result.Text, "milliseconds": time.Since(start).Milliseconds()})
}
func (l *lab) speak(w http.ResponseWriter, r *http.Request) {
	if !take(w, r, l.tts) {
		return
	}
	defer func() { <-l.tts }()
	var input struct {
		Text string `json:"text"`
	}
	if !readJSON(w, r, &input, 8192) {
		return
	}
	if strings.TrimSpace(input.Text) == "" || len(input.Text) > 4096 {
		http.Error(w, "text must be1to4096bytes", 400)
		return
	}
	raw, _ := json.Marshal(map[string]string{"input": input.Text, "model": "tts-1", "voice": "af_nova", "response_format": "pcm"})
	res, e := l.send(r.Context(), l.speech+"/v1/audio/speech", "application/json", bytes.NewReader(raw))
	if e != nil {
		http.Error(w, "speech synthesis unavailable", 502)
		return
	}
	defer res.Body.Close()
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "audio/") {
		http.Error(w, "invalid speech response", 502)
		return
	}
	w.Header().Set("Content-Type", "audio/pcm")
	w.Header().Set("X-Audio-Sample-Rate", "24000")
	w.Header().Set("X-Accel-Buffering", "no")
	// Flush each chunk: unlike the installed bounded blob relay, this lab starts playback early.
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 8192)
	left := 8 * 1024 * 1024
	for left > 0 {
		n, e := res.Body.Read(buf[:min(len(buf), left)])
		if n > 0 {
			if _, err := w.Write(buf[:n]); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
			left -= n
		}
		if e != nil {
			return
		}
	}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (l *lab) respond(w http.ResponseWriter, r *http.Request) {
	if !take(w, r, l.llm) {
		return
	}
	defer func() { <-l.llm }()
	var input struct {
		Text    string    `json:"text"`
		History []message `json:"history"`
	}
	if !readJSON(w, r, &input, 32768) {
		return
	}
	if strings.TrimSpace(input.Text) == "" || len(input.Text) > 4096 || len(input.History) > 12 {
		http.Error(w, "bounded prompt and history required", 400)
		return
	}
	messages := []message{{"system", "You are a concise voice assistant in a local prototype. Answer in plain spoken English, usually one or two short sentences. Do not use markdown. You have no tools and cannot perform actions. Do not claim you executed anything."}}
	for _, m := range input.History {
		if (m.Role != "user" && m.Role != "assistant") || len(m.Content) > 4096 {
			http.Error(w, "invalid history", 400)
			return
		}
		messages = append(messages, m)
	}
	messages = append(messages, message{"user", input.Text})
	raw, _ := json.Marshal(map[string]any{"model": l.model, "messages": messages, "stream": true, "max_tokens": 160, "temperature": 0.3})
	start := time.Now()
	res, e := l.send(r.Context(), l.modelBase+"/chat/completions", "application/json", bytes.NewReader(raw))
	if e != nil {
		http.Error(w, "local model unavailable", 502)
		return
	}
	defer res.Body.Close()
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Accel-Buffering", "no")
	enc := json.NewEncoder(w)
	flush, _ := w.(http.Flusher)
	scan := bufio.NewScanner(io.LimitReader(res.Body, 1024*1024))
	scan.Buffer(make([]byte, 4096), 65536)
	first := true
	finished := false
	for scan.Scan() {
		line := scan.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			finished = true
			break
		}
		var event struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Error any `json:"error"`
		}
		if json.Unmarshal([]byte(data), &event) != nil || event.Error != nil {
			_ = enc.Encode(map[string]any{"error": "model stream failed"})
			if flush != nil {
				flush.Flush()
			}
			return
		}
		for _, choice := range event.Choices {
			if choice.Delta.Content != "" {
				v := map[string]any{"text": choice.Delta.Content}
				if first {
					v["firstTokenMilliseconds"] = time.Since(start).Milliseconds()
					first = false
				}
				if enc.Encode(v) != nil {
					return
				}
				if flush != nil {
					flush.Flush()
				}
			}
		}
	}
	if scan.Err() != nil || !finished {
		_ = enc.Encode(map[string]any{"error": "model stream interrupted"})
	} else if first {
		_ = enc.Encode(map[string]any{"error": "model returned no spoken reply"})
	} else {
		_ = enc.Encode(map[string]any{"done": true, "milliseconds": time.Since(start).Milliseconds()})
	}
	if flush != nil {
		flush.Flush()
	}
}
