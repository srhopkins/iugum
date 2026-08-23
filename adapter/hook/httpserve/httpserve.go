package httpserve

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/srhopkins/iugum/contract"
)

const signatureHeader = "X-Hub-Signature-256"

// Handler serves POST /hooks/{name}. Other methods get 405. Unknown paths get 404.
// When secret is non-empty, requests must carry a valid GitHub-style HMAC-SHA256
// signature over the raw body (X-Hub-Signature-256: sha256=<hex>).
func Handler(secret string, fire func(context.Context, contract.Event) error) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/{name}", func(w http.ResponseWriter, r *http.Request) {
		handleHook(w, r, secret, fire)
	})
	mux.HandleFunc("/hooks/{name}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})
	return mux
}

// Start binds addr and serves hooks in a background goroutine. It returns after
// the listener is ready so callers can continue (e.g. the watch loop).
func Start(addr, secret string, fire func(context.Context, contract.Event) error) error {
	h := Handler(secret, fire)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: h}
	go func() {
		_ = srv.Serve(ln)
	}()
	return nil
}

func handleHook(w http.ResponseWriter, r *http.Request, secret string, fire func(context.Context, contract.Event) error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if secret != "" && !verifySignature([]byte(secret), body, r.Header.Get(signatureHeader)) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	pathName := r.PathValue("name")
	ev, err := parseEvent(body, pathName)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := fire(r.Context(), ev); err != nil {
		http.Error(w, "hook failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func verifySignature(secret, body []byte, header string) bool {
	if !strings.HasPrefix(header, "sha256=") {
		return false
	}
	want, err := hex.DecodeString(header[7:])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	got := mac.Sum(nil)
	return hmac.Equal(got, want)
}

type hookBody struct {
	Event      string         `json:"event"`
	Name       string         `json:"name"`
	DeliveryID string         `json:"delivery_id"`
	TS         string         `json:"ts"`
	Data       map[string]any `json:"data"`
}

func parseEvent(body []byte, pathName string) (contract.Event, error) {
	var raw hookBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &raw); err != nil {
			return contract.Event{}, err
		}
	}
	name := raw.Event
	if name == "" {
		name = raw.Name
	}
	if name == "" {
		name = pathName
	}
	attrs := map[string]string{
		"source": "http",
	}
	if raw.DeliveryID != "" {
		attrs["delivery_id"] = raw.DeliveryID
	}
	if raw.TS != "" {
		attrs["ts"] = raw.TS
	}
	for k, v := range raw.Data {
		if s, ok := v.(string); ok {
			attrs[k] = s
			continue
		}
		b, err := json.Marshal(v)
		if err != nil {
			return contract.Event{}, err
		}
		attrs[k] = string(b)
	}
	return contract.Event{
		Name:   name,
		Source: "http",
		Attrs:  attrs,
	}, nil
}

func SignBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
