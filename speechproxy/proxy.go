// Package speechproxy relays only the two bounded Chat speech requests to an
// explicitly configured upstream. It does not capture audio or start itself.
package speechproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

const MaxAudioBytes = 8 * 1024 * 1024
const MaxTextUnits = 4096
const maxMultipartBytes = MaxAudioBytes + 64*1024
const maxJSONBytes = 64 * 1024
const maxTranscriptBytes = 1024 * 1024
const transcriptionPath = "/v1/audio/transcriptions"
const speechPath = "/v1/audio/speech"

type Config struct {
	Upstream string
	Timeout  time.Duration
	Check    func(context.Context, string, string) error
}
type Proxy struct {
	upstream *url.URL
	client   *http.Client
	check    func(context.Context, string, string) error
}

// ValidateListen accepts only a literal loopback address, including IPv6.
func ValidateListen(address string) error {
	host, port, err := net.SplitHostPort(address)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() || port == "" {
		return errors.New("speech relay listener must use a loopback IP literal and port")
	}
	value, parseErr := strconv.Atoi(port)
	if parseErr != nil || value < 0 || value > 65535 {
		return errors.New("speech relay listener requires a numeric port")
	}
	for _, c := range port {
		if c < '0' || c > '9' {
			return errors.New("speech relay listener requires a numeric port")
		}
	}
	return nil
}
func upstreamURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Host == "" || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.Opaque != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") || strings.Contains(u.Host, "%") {
		return nil, errors.New("explicit HTTP(S) upstream root URL without credentials, query or fragment required")
	}
	// An explicit empty fragment is still a fragment in the supplied configuration.
	if strings.Contains(raw, "#") {
		return nil, errors.New("upstream fragments are not allowed")
	}
	u.Path = ""
	return u, nil
}
func New(config Config) (*Proxy, error) {
	upstream, err := upstreamURL(config.Upstream)
	if err != nil {
		return nil, err
	}
	if config.Check == nil {
		return nil, errors.New("speech relay application policy required")
	}
	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}
	if config.Timeout <= 0 || config.Timeout > 60*time.Second {
		return nil, errors.New("speech relay timeout must be positive and at most60seconds")
	}
	if err = config.Check(context.Background(), "agent/speech-relay", "serve"); err != nil {
		return nil, errors.New("speech relay startup policy denied")
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext, DisableKeepAlives: true, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: config.Timeout, MaxResponseHeaderBytes: 32 * 1024}
	client := &http.Client{Transport: transport, Timeout: config.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Proxy{upstream: upstream, client: client, check: config.Check}, nil
}
func (p *Proxy) Close() { p.client.CloseIdleConnections() }
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.Fragment != "" || r.URL.RawPath != "" || r.URL.Scheme != "" || r.URL.Host != "" || r.URL.Opaque != "" {
		http.Error(w, "exact local speech path required", http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/healthz" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
		return
	}
	if r.URL.Path != transcriptionPath && r.URL.Path != speechPath {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var payload []byte
	var contentType string
	var err error
	if r.URL.Path == speechPath {
		payload, contentType, err = readSpeech(r)
	} else {
		payload, contentType, err = readTranscription(r)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	target := *p.upstream
	target.Path = r.URL.Path
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target.String(), bytes.NewReader(payload))
	if err != nil {
		http.Error(w, "speech request unavailable", http.StatusBadGateway)
		return
	}
	// The request is rebuilt rather than reverse-proxied: no inbound credentials,
	// cookies, forwarding headers, origin, host override or idempotency key escape.
	request.Header.Set("Content-Type", contentType)
	request.GetBody = nil
	if err = p.check(r.Context(), "agent/speech-relay"+r.URL.Path, "forward"); err != nil {
		http.Error(w, "speech route policy denied", http.StatusForbidden)
		return
	}
	response, err := p.client.Do(request)
	if err != nil {
		http.Error(w, "speech upstream unavailable or cancelled; request was not retried", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		http.Error(w, "speech upstream redirect refused", http.StatusBadGateway)
		return
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		status := response.StatusCode
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		http.Error(w, "speech upstream rejected request", status)
		return
	}
	limit := MaxAudioBytes
	if r.URL.Path == transcriptionPath {
		limit = maxTranscriptBytes
	}
	body, err := readBounded(response.Body, int64(limit))
	if err != nil {
		http.Error(w, "speech upstream response exceeded limit or failed", http.StatusBadGateway)
		return
	}
	if r.URL.Path == transcriptionPath {
		var result struct {
			Text *string `json:"text"`
		}
		if json.Unmarshal(body, &result) != nil || result.Text == nil {
			http.Error(w, "speech upstream returned no transcript", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
		return
	}
	kind, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(kind, "audio/") || len(body) == 0 {
		http.Error(w, "speech upstream returned no audio", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", kind)
	_, _ = w.Write(body)
}
func readBounded(reader io.Reader, max int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, max+1))
	if err != nil {
		return nil, errors.New("speech payload read failed")
	}
	if int64(len(body)) > max {
		return nil, errors.New("speech payload exceeds size limit")
	}
	return body, nil
}
func readSpeech(r *http.Request) ([]byte, string, error) {
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || kind != "application/json" {
		return nil, "", errors.New("speech JSON content type required")
	}
	raw, err := readBounded(r.Body, maxJSONBytes)
	if err != nil || !utf8.Valid(raw) {
		return nil, "", errors.New("speech JSON exceeds size limit or is invalid")
	}
	var value struct {
		Model  string `json:"model"`
		Voice  string `json:"voice"`
		Input  string `json:"input"`
		Format string `json:"response_format"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, "", errors.New("exact speech JSON fields required")
	}
	if strings.TrimSpace(value.Input) == "" || len(utf16.Encode([]rune(value.Input))) > MaxTextUnits || len(value.Model) > 256 || len(value.Voice) > 256 || (value.Format != "" && value.Format != "wav") {
		return nil, "", errors.New("speech input requires1to4096text units and bounded WAV settings")
	}
	if value.Format == "" {
		value.Format = "wav"
	}
	body, _ := json.Marshal(value)
	return body, "application/json", nil
}
func readTranscription(r *http.Request) ([]byte, string, error) {
	if r.ContentLength > maxMultipartBytes {
		return nil, "", errors.New("audio request exceeds size limit")
	}
	raw, err := readBounded(r.Body, maxMultipartBytes)
	if err != nil {
		return nil, "", errors.New("audio request exceeds size limit or failed")
	}
	kind, parameters, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || kind != "multipart/form-data" || parameters["boundary"] == "" {
		return nil, "", errors.New("transcription multipart form required")
	}
	reader := multipart.NewReader(bytes.NewReader(raw), parameters["boundary"])
	var audio []byte
	model := ""
	seenModel := false
	audioType := ""
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", errors.New("invalid or oversized transcription multipart")
		}
		switch part.FormName() {
		case "file":
			if audio != nil {
				return nil, "", errors.New("one audio file required")
			}
			audioType, _, err = mime.ParseMediaType(part.Header.Get("Content-Type"))
			if err != nil || !strings.HasPrefix(audioType, "audio/") {
				return nil, "", errors.New("audio file content type required")
			}
			audio, err = readBounded(part, MaxAudioBytes)
			if err != nil || len(audio) == 0 {
				return nil, "", errors.New("audio file requires1to8MiB")
			}
		case "model":
			if seenModel {
				return nil, "", errors.New("one transcription model allowed")
			}
			seenModel = true
			raw, err := readBounded(part, 256)
			if err != nil || !utf8.Valid(raw) {
				return nil, "", errors.New("transcription model exceeds size limit")
			}
			model = string(raw)
		default:
			return nil, "", errors.New("only audio file and model fields allowed")
		}
		_ = part.Close()
	}
	if len(audio) == 0 {
		return nil, "", errors.New("one audio file required")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	extension := map[string]string{"audio/wav": "wav", "audio/x-wav": "wav", "audio/webm": "webm", "audio/mp4": "m4a", "audio/mpeg": "mp3", "audio/ogg": "ogg", "audio/flac": "flac"}[audioType]
	if extension == "" {
		extension = "audio"
	}
	header.Set("Content-Disposition", `form-data; name="file"; filename="dictation.`+extension+`"`)
	header.Set("Content-Type", audioType)
	file, _ := writer.CreatePart(header)
	_, _ = file.Write(audio)
	if seenModel {
		_ = writer.WriteField("model", model)
	}
	_ = writer.Close()
	return body.Bytes(), writer.FormDataContentType(), nil
}
