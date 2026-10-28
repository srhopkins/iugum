package beadsadapt

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/srhopkins/iugum/beadactivity"
)

type activityProject struct{ Name, Dir string }
type activityRun func(context.Context, string, ...string) ([]byte, error)

func activityHandler(next http.Handler, projects []activityProject, run activityRun, now func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/iugum/activity" {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "GET" {
			w.Header().Set("Allow", "GET")
			http.Error(w, "read-only endpoint", 405)
			return
		}
		name := r.URL.Query().Get("project")
		var selected *activityProject
		for i := range projects {
			if projects[i].Name == name {
				selected = &projects[i]
				break
			}
		}
		if selected == nil {
			http.Error(w, "configured project is required", 400)
			return
		}
		raw, err := run(r.Context(), selected.Dir, "list", "--all", "--limit", "0", "--json")
		if err != nil {
			http.Error(w, "activity source unavailable", 503)
			return
		}
		var issues []struct {
			ID, Assignee, Status string
			Metadata             map[string]json.RawMessage
		}
		if json.Unmarshal(raw, &issues) != nil {
			http.Error(w, "invalid activity source", 502)
			return
		}
		type item struct {
			ID string `json:"id"`
			beadactivity.View
			Executions []beadactivity.Execution `json:"executions"`
		}
		views := make([]item, 0, len(issues))
		at := now()
		for _, i := range issues {
			views = append(views, item{i.ID, beadactivity.Project(i.Assignee, i.Status, beadactivity.Decode(i.Metadata), at), beadactivity.Executions(i.Metadata)})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Project string    `json:"project"`
			AsOf    time.Time `json:"as_of"`
			Issues  []item    `json:"issues"`
		}{name, at, views})
	})
}
