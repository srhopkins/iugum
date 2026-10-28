package agentplatform

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/srhopkins/iugum/agentplatform/browsergrant"
	"net/http"
	"strings"
)

// EnableBrowserGrantRoutes wires only the explicit Config.BrowserGrants opt-in.
// The CLI flag defaults false; reviewed source is not runtime deployment.
func (s *Server) EnableBrowserGrantRoutes(a *BrowserGrantAdapter) error {
	if a == nil || a.server != s || s.host == nil {
		return errors.New("exact browser adapter required")
	}
	s.host.mu.Lock()
	defer s.host.mu.Unlock()
	if s.host.browser != nil {
		return errors.New("browser adapter already registered")
	}
	s.host.browser = a
	return nil
}
func fields(m map[string]any, keys ...string) bool {
	if len(m) != len(keys) {
		return false
	}
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}
func field(m map[string]any, k string) string { v, _ := m[k].(string); return v }
func (a *BrowserGrantAdapter) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/host/browser/mcp" {
		r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
		a.handler.ServeHTTP(w, r)
		return
	}
	if r.Method != "POST" || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	limit := int64(1024 * 1024)
	if r.URL.Path == "/api/host/browser/observation" {
		limit = 12 * 1024 * 1024
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	m, e := browsergrant.Decode(r.Body)
	if e != nil || !fields(m, "instance", "lease", "request") {
		http.Error(w, "invalid private browser request", 400)
		return
	}
	q, ok := m["request"].(map[string]any)
	if !ok {
		http.Error(w, "request object required", 400)
		return
	}
	host := HostIdentity{Instance: field(m, "instance"), Lease: field(m, "lease")}
	s := a.server
	s.host.mu.Lock()
	defer s.host.mu.Unlock()
	operation := "browser_bootstrap"
	if !strings.HasPrefix(r.URL.Path, "/api/host/browser/bootstrap/") {
		operation = "browser_transport"
	}
	if e = s.hostCheck(r.Context(), operation, "write"); e != nil {
		http.Error(w, "browser bootstrap policy denied", 403)
		return
	}
	if !s.pairingWindow(host) {
		http.Error(w, "exact current private window required", 409)
		return
	}
	generation := windowGeneration(host)
	pair := field(q, "pair")
	var value any
	action := strings.TrimPrefix(r.URL.Path, "/api/host/browser/bootstrap/")
	switch action {
	case "/api/host/browser/poll":
		if !fields(q, "pair") {
			e = errors.New("unexpected poll fields")
		} else {
			var command *browsergrant.DispatchCommand
			command, e = a.dispatcher.PollForWindow(r.Context(), pair, host.Instance, generation)
			var wire any
			if command != nil {
				wire = map[string]any{"pair": command.Pair, "commandId": command.CommandID, "operation": command.Operation, "args": command.Args, "observation": command.Observation}
			}
			value = map[string]any{"command": wire}
		}
	case "/api/host/browser/clock-probe":
		if !fields(q, "pair", "nonce") {
			e = errors.New("unexpected clock fields")
		} else {
			value, e = a.dispatcher.ClockProbeForWindow(r.Context(), pair, host.Instance, generation, field(q, "nonce"))
		}
	case "/api/host/browser/status":
		if !fields(q, "pair", "commandId") {
			e = errors.New("unexpected status fields")
		} else {
			value, e = a.dispatcher.StatusForWindow(r.Context(), pair, host.Instance, generation, field(q, "commandId"))
		}
	case "/api/host/browser/cancel":
		if !fields(q, "pair", "commandId") {
			e = errors.New("unexpected cancel fields")
		} else {
			e = a.dispatcher.CancelForWindow(r.Context(), pair, host.Instance, generation, field(q, "commandId"))
			value = map[string]any{"accepted": e == nil}
		}
	case "/api/host/browser/grant":
		args, ok := q["args"].(map[string]any)
		if !fields(q, "pair", "commandId", "operation", "args", "observation") || !ok {
			e = errors.New("unexpected grant request")
		} else {
			value, e = a.dispatcher.GrantForWindow(r.Context(), host.Instance, generation, browsergrant.Request{Pair: field(q, "pair"), CommandID: field(q, "commandId"), Operation: field(q, "operation"), Observation: field(q, "observation"), Args: args})
		}
	case "/api/host/browser/observation":
		envelope, ok := decodeEnvelope(q["envelope"])
		viewport, vok := q["viewport"].(map[string]any)
		targets, tok := q["targets"].([]any)
		if !fields(q, "envelope", "viewport", "targets", "raster") || !ok || !vok || !tok {
			e = errors.New("unexpected observation request")
		} else {
			var raster []byte
			raster, e = base64.StdEncoding.Strict().DecodeString(field(q, "raster"))
			if e == nil {
				value, e = a.dispatcher.AcceptObservationACKForWindow(r.Context(), host.Instance, generation, envelope, viewport, targets, raster)
			}
		}
	case "/api/host/browser/result":
		envelope, ok := decodeEnvelope(q["envelope"])
		data, dok := q["data"].(map[string]any)
		if q["data"] == nil {
			dok = true
		}
		if !fields(q, "envelope", "data") || !ok || !dok {
			e = errors.New("unexpected result request")
		} else {
			value, e = a.dispatcher.AcceptResultACKForWindow(r.Context(), host.Instance, generation, envelope, data)
		}

	case "offer":
		if !fields(q) {
			e = errors.New("unexpected offer fields")
		} else {
			value, e = a.authority.Offer(r.Context(), host.Instance, generation)
		}
	case "commit":
		if !fields(q, "pair", "role", "offerDigest", "commitment") || q["role"] != "companion" {
			e = errors.New("unexpected commitment fields")
		} else {
			e = a.authority.Commit(pair, host.Instance, generation, field(q, "offerDigest"), field(q, "commitment"))
			value = map[string]any{"state": "commitments-fixed"}
		}
	case "reveal":
		c, ok := q["contribution"].(map[string]any)
		if !fields(q, "pair", "role", "offerDigest", "contribution") || q["role"] != "companion" || !ok {
			e = errors.New("unexpected reveal fields")
		} else {
			value, e = a.authority.Reveal(pair, host.Instance, generation, field(q, "offerDigest"), c)
		}
	case "confirm":
		confirmed, ok := q["confirmed"].(bool)
		if !fields(q, "pair", "bootstrapDigest", "confirmed") || !ok {
			e = errors.New("unexpected confirmation fields")
		} else {
			var receipt browsergrant.Envelope
			receipt, e = a.authority.ConfirmEditor(pair, host.Instance, generation, field(q, "bootstrapDigest"), confirmed)
			value = map[string]any{"editorConfirmation": receipt, "state": "compared"}
		}
	case "companion-confirm":
		rec, ok := q["confirmation"].(map[string]any)
		if !fields(q, "pair", "confirmation", "signature") || !ok {
			e = errors.New("unexpected companion confirmation")
		} else {
			e = a.authority.ConfirmCompanion(pair, host.Instance, generation, browsergrant.Envelope{Record: rec, Signature: field(q, "signature")})
			value = map[string]any{"accepted": e == nil}
		}
	case "status":
		if !fields(q, "pair") {
			e = errors.New("unexpected status fields")
		} else {
			value, e = a.authority.Status(pair, host.Instance, generation)
		}
	case "cancel":
		if !fields(q, "pair") {
			e = errors.New("unexpected cancel fields")
		} else {
			e = a.authority.Cancel(pair, host.Instance, generation)
			value = map[string]any{"state": "aborted"}
		}
	default:
		http.NotFound(w, r)
		return
	}
	if e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func decodeEnvelope(v any) (browsergrant.Envelope, bool) {
	m, ok := v.(map[string]any)
	if !ok || !fields(m, "record", "signature") {
		return browsergrant.Envelope{}, false
	}
	r, ok := m["record"].(map[string]any)
	sig, sok := m["signature"].(string)
	return browsergrant.Envelope{Record: r, Signature: sig}, ok && sok
}
