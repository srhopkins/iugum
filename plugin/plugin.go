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
	MemoryFactory     func(cfg map[string]string) (contract.Memory, error)
	SchedulerFactory  func(cfg map[string]string) (contract.Scheduler, error)
	HooksFactory      func(cfg map[string]string) (contract.Hooks, error)
	WatcherFactory    func(cfg map[string]string) (contract.Watcher, error)
)

var (
	mu        sync.RWMutex
	trackers  = map[string]TrackerFactory{}
	wikis     = map[string]WikiFactory{}
	observers = map[string]ObserverFactory{}
	memories   = map[string]MemoryFactory{}
	schedulers = map[string]SchedulerFactory{}
	hooks      = map[string]HooksFactory{}
	watchers   = map[string]WatcherFactory{}
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

func RegisterMemory(name string, f MemoryFactory) {
	mu.Lock()
	defer mu.Unlock()
	memories[name] = f
}

func NewMemory(name string, cfg map[string]string) (contract.Memory, error) {
	mu.RLock()
	f, ok := memories[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("iugum: unknown memory %q (compile it in, or use memory: exec)", name)
	}
	return f(cfg)
}

func RegisterScheduler(name string, f SchedulerFactory) {
	mu.Lock()
	defer mu.Unlock()
	schedulers[name] = f
}

func NewScheduler(name string, cfg map[string]string) (contract.Scheduler, error) {
	mu.RLock()
	f, ok := schedulers[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("iugum: unknown scheduler %q", name)
	}
	return f(cfg)
}

func RegisterHooks(name string, f HooksFactory) {
	mu.Lock()
	defer mu.Unlock()
	hooks[name] = f
}

func NewHooks(name string, cfg map[string]string) (contract.Hooks, error) {
	mu.RLock()
	f, ok := hooks[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("iugum: unknown hooks %q", name)
	}
	return f(cfg)
}

func RegisterWatcher(name string, f WatcherFactory) {
	mu.Lock()
	defer mu.Unlock()
	watchers[name] = f
}

func NewWatcher(name string, cfg map[string]string) (contract.Watcher, error) {
	mu.RLock()
	f, ok := watchers[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("iugum: unknown watcher %q", name)
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

// NetFactory builds a contract.Net adapter from a flat config map.
type NetFactory func(cfg map[string]string) (contract.Net, error)

var nets = map[string]NetFactory{}

func RegisterNet(name string, f NetFactory) {
	mu.Lock()
	defer mu.Unlock()
	nets[name] = f
}

func NewNet(name string, cfg map[string]string) (contract.Net, error) {
	mu.RLock()
	f, ok := nets[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("iugum: unknown net backend %q (use iptables, nftables, auto, or off)", name)
	}
	return f(cfg)
}
