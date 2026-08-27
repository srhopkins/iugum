package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/srhopkins/iugum/adapter/observe/sqladapt"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/dirs"
	"github.com/srhopkins/iugum/plugin"
)

func (a *App) IngestLogs(ctx context.Context, recs []contract.Log) error {
	if err := a.Check(ctx, "observe", "ingest"); err != nil {
		return err
	}
	return a.Observer.IngestLogs(ctx, recs)
}

func (a *App) SearchLogs(ctx context.Context, q contract.LogQuery) ([]contract.Log, error) {
	if err := a.Check(ctx, "observe", "query"); err != nil {
		return nil, err
	}
	return a.Observer.SearchLogs(ctx, q)
}

func (a *App) QueryMetricRange(ctx context.Context, q contract.MetricRangeQuery) ([]contract.Series, error) {
	if err := a.Check(ctx, "observe", "query"); err != nil {
		return nil, err
	}
	return a.Observer.QueryMetricRange(ctx, q)
}

func (a *App) SearchLogRange(ctx context.Context, q contract.LogRangeQuery) ([]contract.Log, error) {
	if err := a.Check(ctx, "observe", "query"); err != nil {
		return nil, err
	}
	return a.Observer.SearchLogRange(ctx, q)
}

// ServeObserve starts the sqlite metrics/logs UI. Port 0 means 3848.
func (a *App) ServeObserve(ctx context.Context, port int, host string) error {
	if err := a.Check(ctx, "observe", "serve"); err != nil {
		return err
	}
	if err := a.ensureSQLiteObserve(); err != nil {
		return err
	}
	if port == 0 {
		port = 3848
	}
	if host == "" {
		host = "127.0.0.1"
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	srv := &http.Server{Addr: addr, Handler: a.ObserveHandler()}
	if data, err := dirs.ResolveData(""); err == nil {
		fmt.Fprintf(os.Stdout, "iugum observe: http://%s\n", addr)
		fmt.Fprintf(os.Stdout, "iugum observe dashboards: %s\n", sqladapt.DashboardDir(data))
	} else {
		fmt.Fprintf(os.Stdout, "iugum observe: http://%s\n", addr)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		_ = srv.Shutdown(context.Background())
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil
		}
		return ctx.Err()
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

// ObserveHandler is UI + ingest + query. Casbin gates ingest and query per request.
func (a *App) ObserveHandler() http.Handler {
	data, err := dirs.ResolveData("")
	if err != nil {
		data = ""
	}
	inner := sqladapt.NewHandler(a.Observer, sqladapt.HandlerOptions{DataDir: data})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if act, ok := observeAct(r); ok {
			if err := a.Check(r.Context(), "observe", act); err != nil {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
		}
		inner.ServeHTTP(w, r)
	})
}

func observeAct(r *http.Request) (string, bool) {
	p := r.URL.Path
	if strings.HasPrefix(p, "/ingest/") {
		return "ingest", true
	}
	if strings.HasPrefix(p, "/query/") || strings.HasPrefix(p, "/api/v1/") || strings.HasPrefix(p, "/loki/") {
		return "query", true
	}
	if r.Method == http.MethodPut && (p == "/homelab-dashboard.json" || strings.HasPrefix(p, "/dashboards/")) {
		return "serve", true
	}
	return "", false
}

func (a *App) ensureSQLiteObserve() error {
	if a.Observer != nil && a.Observer.Name() == "sqlite" {
		return nil
	}
	data, err := dirs.ResolveData("")
	if err != nil {
		return err
	}
	ob, err := plugin.NewObserver("sqlite", map[string]string{"data_dir": data})
	if err != nil {
		return err
	}
	a.Observer = ob
	return nil
}
