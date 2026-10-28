package beadsadapt

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestActivityEndpointCanonicalCLIStringMetadata(t *testing.T) {
	// Match the JSON-string values returned by canonical bd list/show --json.
	const rows = `[{"id":"ticket","assignee":"worker","status":"in_progress","metadata":{"private":"hidden","iugum.activity":"{\"run_id\":\"real-run\",\"actor\":\"worker\",\"state\":\"working\",\"seen_at\":\"2026-10-20T01:35:59Z\",\"expires_at\":\"2026-10-20T01:37:59Z\"}","iugum.execution.example":"{\"version\":1,\"run_id\":\"real-run\",\"actor\":\"worker\",\"label\":\"Recovery worker\",\"platform\":\"codex\",\"host\":\"mac\",\"session_id\":\"actual-session\",\"cwd\":\"/workspace\",\"started_at\":\"2026-10-20T01:35:59Z\",\"last_seen\":\"2026-10-20T01:35:59Z\"}"}}]`
	at := time.Date(2026, 10, 20, 1, 36, 0, 0, time.UTC)
	run := func(_ context.Context, dir string, args ...string) ([]byte, error) {
		if dir != "/canonical/iugum" || strings.Join(args, " ") != "list --all --limit 0 --json" {
			t.Fatalf("unexpected source %s %v", dir, args)
		}
		return []byte(rows), nil
	}
	h := activityHandler(http.NotFoundHandler(), []activityProject{{"iugum", "/canonical/iugum"}}, run, func() time.Time { return at })
	for _, want := range []string{"working", "stale"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/api/iugum/activity?project=iugum", nil))
		var got struct {
			Issues []struct {
				State      string `json:"state"`
				RunID      string `json:"run_id"`
				Executions []struct {
					SessionID string `json:"session_id"`
				}
			}
		}
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Issues) != 1 || got.Issues[0].State != want || got.Issues[0].RunID != "real-run" || len(got.Issues[0].Executions) != 1 || got.Issues[0].Executions[0].SessionID != "actual-session" {
			t.Fatalf("projection lost canonical metadata: %s", w.Body.String())
		}
		if strings.Contains(w.Body.String(), "hidden") {
			t.Fatal("private metadata exposed")
		}
		at = at.Add(3 * time.Minute)
	}
}
