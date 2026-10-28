package agentdesk

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAgentModelSetupHTTPPolicyAndValidation(t *testing.T) {
	denied := false
	calls := 0
	s, e := New(Config{DataDir: t.TempDir(), Check: func(context.Context, string, string) error {
		if denied {
			return errors.New("denied")
		}
		return nil
	}, ModelSetup: func(_ context.Context, q *ModelSetup) (ModelSetupState, error) {
		calls++
		return ModelSetupState{RestartRequired: q != nil}, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	for _, method := range []string{"GET", "POST"} {
		denied = true
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(method, "http://localhost/api/model-setup", strings.NewReader(`{}`)))
		if w.Code != 403 || calls != 0 {
			t.Fatalf("policy %s: %d calls %d", method, w.Code, calls)
		}
	}
	denied = false
	for _, body := range []string{`{"api_key":"secret"}`, `{} {}`, `{"pools":{"x":{"api_key_file":"/etc/passwd"}}}`} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("POST", "http://localhost/api/model-setup", strings.NewReader(body)))
		if w.Code != 400 || calls != 0 {
			t.Fatalf("invalid body accepted: %d", w.Code)
		}
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("POST", "http://localhost/api/model-setup", strings.NewReader(`{"default_profile":"mini","pools":{},"profiles":{}}`)))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"restart_required":true`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
