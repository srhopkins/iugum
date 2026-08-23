// Package plugin is the adapter registry.
// Built-in adapters register in init(). A fork compiles its own binary by
// importing extra modules that call Register* in their init.
package plugin

import (
	"fmt"
	"sync"

	"github.com/srhopkins/iugum/contract"
)

type (
	TrackerFactory  func(cfg map[string]string) (contract.Tracker, error)
	WikiFactory     func(cfg map[string]string) (contract.Wiki, error)
	ObserverFactory func(cfg map[string]string) (contract.Observer, error)
)

var (
	mu        sync.RWMutex
	trackers  = map[string]TrackerFactory{}
	wikis     = map[string]WikiFactory{}
	observers = map[string]ObserverFactory{}
)

func RegisterTracker(name string, f TrackerFactory) {
	mu.Lock()
	defer mu.Unlock()
	trackers[name] = f
}

func RegisterWiki(name string, f WikiFactory) {
	mu.Lock()
	defer mu.Unlock()
	wikis[name] = f
}

func RegisterObserver(name string, f ObserverFactory) {
	mu.Lock()
	defer mu.Unlock()
	observers[name] = f
}

func NewTracker(name string, cfg map[string]string) (contract.Tracker, error) {
	mu.RLock()
	f, ok := trackers[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("iugum: unknown tracker %q (compile it in, or use tracker: exec)", name)
	}
	return f(cfg)
}

func NewWiki(name string, cfg map[string]string) (contract.Wiki, error) {
	mu.RLock()
	f, ok := wikis[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("iugum: unknown wiki %q (compile it in, or use wiki: exec)", name)
	}
	return f(cfg)
}

func NewObserver(name string, cfg map[string]string) (contract.Observer, error) {
	mu.RLock()
	f, ok := observers[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("iugum: unknown observe %q (compile it in, or use observe: exec)", name)
	}
	return f(cfg)
}
