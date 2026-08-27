package sqladapt

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	observeui "github.com/srhopkins/iugum/web/observe"
)

const (
	defaultDashboardName = "homelab-dashboard.json"
	embeddedDashboard    = "dist/homelab-dashboard.json"
	dashboardsSubdir     = "dashboards"
	maxDashboardBytes    = 1 << 20
)

// DashboardDir is {dataDir}/dashboards. Empty dataDir means no on-disk store.
func DashboardDir(dataDir string) string {
	if strings.TrimSpace(dataDir) == "" {
		return ""
	}
	return filepath.Join(dataDir, dashboardsSubdir)
}

func dashboardPath(dataDir, name string) string {
	safe, err := safeDashboardName(name)
	if err != nil {
		safe = defaultDashboardName
	}
	return filepath.Join(DashboardDir(dataDir), safe)
}

func safeDashboardName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return defaultDashboardName, nil
	}
	base := filepath.Base(name)
	if base != name || strings.Contains(base, "..") {
		return "", fmt.Errorf("bad dashboard name")
	}
	if !strings.HasSuffix(strings.ToLower(base), ".json") {
		return "", fmt.Errorf("dashboard name must end in .json")
	}
	return base, nil
}

func listDashboards(dataDir string) []map[string]string {
	_ = SeedDashboards(dataDir)
	dir := DashboardDir(dataDir)
	out := []map[string]string{}
	if dir == "" {
		return out
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		title := e.Name()
		if b, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil {
			var raw map[string]any
			if json.Unmarshal(b, &raw) == nil {
				if spec, ok := raw["spec"].(map[string]any); ok {
					if display, ok := spec["display"].(map[string]any); ok {
						if n, ok := display["name"].(string); ok && n != "" {
							title = n
						}
					}
				}
				if title == e.Name() {
					if meta, ok := raw["metadata"].(map[string]any); ok {
						if n, ok := meta["name"].(string); ok && n != "" {
							title = n
						}
					}
				}
			}
		}
		out = append(out, map[string]string{"file": e.Name(), "title": title})
	}
	return out
}

// SeedDashboards copies the embedded default board into dataDir/dashboards
// when that file is missing. It does not overwrite a user file.
func SeedDashboards(dataDir string) error {
	dir := DashboardDir(dataDir)
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	dst := dashboardPath(dataDir, defaultDashboardName)
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	body, err := observeui.Dist.ReadFile(embeddedDashboard)
	if err != nil {
		return err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func readDashboard(dataDir, name string) ([]byte, error) {
	if dataDir != "" {
		p := dashboardPath(dataDir, name)
		b, err := os.ReadFile(p)
		if err == nil {
			return b, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return observeui.Dist.ReadFile(embeddedDashboard)
}

func writeDashboard(dataDir, name string, body []byte) error {
	if dataDir == "" {
		return fmt.Errorf("no data dir")
	}
	if err := validateDashboardJSON(body); err != nil {
		return err
	}
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	pretty, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	pretty = append(pretty, '\n')
	if err := os.MkdirAll(DashboardDir(dataDir), 0o755); err != nil {
		return err
	}
	dst := dashboardPath(dataDir, name)
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, pretty, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func validateDashboardJSON(body []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return fmt.Errorf("invalid json")
	}
	kind, _ := raw["kind"].(string)
	if kind != "Dashboard" {
		return fmt.Errorf(`kind must be "Dashboard"`)
	}
	if _, ok := raw["spec"].(map[string]any); !ok {
		return fmt.Errorf("missing spec")
	}
	return nil
}

func serveOneDashboard(dataDir, name string, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		b, err := readDashboard(dataDir, name)
		if err != nil {
			http.Error(w, "dashboard not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(b)
	case http.MethodPut:
		if dataDir == "" {
			http.Error(w, "no data dir", http.StatusNotImplemented)
			return
		}
		if _, err := safeDashboardName(name); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxDashboardBytes+1))
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if len(body) > maxDashboardBytes {
			http.Error(w, "dashboard too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err := writeDashboard(dataDir, name, body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"saved": dashboardPath(dataDir, name),
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func registerDashboardHTTP(mux *http.ServeMux, dataDir string) {
	_ = SeedDashboards(dataDir)
	mux.HandleFunc("GET /dashboards", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"dashboards": listDashboards(dataDir)})
	})
	mux.HandleFunc("/dashboards/{name}", func(w http.ResponseWriter, r *http.Request) {
		name, err := safeDashboardName(r.PathValue("name"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		serveOneDashboard(dataDir, name, w, r)
	})
	mux.HandleFunc("/homelab-dashboard.json", func(w http.ResponseWriter, r *http.Request) {
		serveOneDashboard(dataDir, defaultDashboardName, w, r)
	})
}
