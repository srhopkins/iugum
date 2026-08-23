package fswatch

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/plugin"
)

const debounce = 200 * time.Millisecond

func init() {
	plugin.RegisterWatcher("fsnotify", func(map[string]string) (contract.Watcher, error) {
		return New()
	})
}

// Watcher uses fsnotify (inotify / kqueue / ReadDirectoryChangesW). No CGO.
// Recursion is ours: each subdirectory is added.
type Watcher struct {
	w    *fsnotify.Watcher
	out  chan contract.Event
	mu   sync.Mutex
	wait map[string]*time.Timer
}

func New() (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	wt := &Watcher{w: w, out: make(chan contract.Event, 32), wait: map[string]*time.Timer{}}
	go wt.loop()
	return wt, nil
}

func (w *Watcher) Name() string { return "fsnotify" }

func (w *Watcher) Add(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return w.w.Add(path)
		}
		return nil
	})
}

func (w *Watcher) Events() <-chan contract.Event { return w.out }

func (w *Watcher) Close() error {
	return w.w.Close()
}

func (w *Watcher) loop() {
	for {
		select {
		case ev, ok := <-w.w.Events:
			if !ok {
				return
			}
			if ev.Has(fsnotify.Create) {
				if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
					_ = w.w.Add(ev.Name)
				}
			}
			kind := "write"
			switch {
			case ev.Has(fsnotify.Create):
				kind = "create"
			case ev.Has(fsnotify.Remove), ev.Has(fsnotify.Rename):
				kind = "remove"
			case ev.Has(fsnotify.Write):
				kind = "write"
			default:
				// Ignore Chmod. Spotlight on macOS floods it (kqueue).
				continue
			}
			w.queue(contract.Event{Name: "watch.changed", Source: "watch", Path: ev.Name, Attrs: map[string]string{"op": kind}})
		case _, ok := <-w.w.Errors:
			if !ok {
				return
			}
		}
	}
}

func (w *Watcher) queue(ev contract.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if t, ok := w.wait[ev.Path]; ok {
		t.Stop()
	}
	e := ev
	w.wait[ev.Path] = time.AfterFunc(debounce, func() {
		w.mu.Lock()
		delete(w.wait, e.Path)
		w.mu.Unlock()
		select {
		case w.out <- e:
		default:
		}
	})
}
