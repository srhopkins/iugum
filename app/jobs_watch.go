package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/srhopkins/iugum/config"
)

// WatchJobsFile reloads jobs.yaml when it changes and registers new jobs live.
func WatchJobsFile(ctx context.Context, a *App, path string) error {
	if path == "" {
		return nil
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		dir, err = os.Getwd()
		if err != nil {
			_ = w.Close()
			return err
		}
	}
	if err := w.Add(dir); err != nil {
		_ = w.Close()
		return err
	}
	base := filepath.Base(path)
	go func() {
		defer w.Close()
		var delay *time.Timer
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if filepath.Base(ev.Name) != base {
					continue
				}
				if delay != nil {
					delay.Stop()
				}
				delay = time.AfterFunc(200*time.Millisecond, func() {
					jobs, err := config.LoadJobsFile(path)
					if err != nil {
						fmt.Fprintf(os.Stderr, "iugum: jobs file: %v\n", err)
						return
					}
					if err := a.SyncJobs(jobs); err != nil {
						fmt.Fprintf(os.Stderr, "iugum: sync jobs: %v\n", err)
					}
				})
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				fmt.Fprintf(os.Stderr, "iugum: jobs watch: %v\n", err)
			}
		}
	}()
	return nil
}
