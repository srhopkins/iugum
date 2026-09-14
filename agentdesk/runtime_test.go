package agentdesk

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestRuntimeStopRequiresCapability(t *testing.T) {
	stopped := make(chan bool, 1)
	s, e := New(Config{DataDir: t.TempDir(), ControlToken: "secret", Stop: func() { stopped <- true }})
	if e != nil {
		t.Fatal(e)
	}
	for _, token := range []string{"", "wrong", "secret"} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:3850/api/runtime/stop", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if token != "secret" && w.Code != 403 {
			t.Fatal(w.Code)
		}
		if token == "secret" && w.Code != 200 {
			t.Fatal(w.Code)
		}
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stop not called")
	}
}
