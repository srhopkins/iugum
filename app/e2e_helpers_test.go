package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/srhopkins/iugum/config"
	"github.com/srhopkins/iugum/contract"
	_ "github.com/srhopkins/iugum/defaults"
)

func e2eConfig(t *testing.T) config.File {
	t.Helper()
	cfg := config.Defaults()
	cfg.DataDir = t.TempDir()
	t.Setenv("IUGUM_DATA", cfg.DataDir)
	cfg.Embeddings.Enabled = false
	cfg.Embeddings.Kind = "off"
	cfg.Graph.Glossary = filepath.Join("..", "glossaries", "memory-graph.yaml")
	cfg.Graph.Extractor = "rules"
	return cfg
}

func newE2EApp(t *testing.T, cfg config.File) *App {
	t.Helper()
	if cfg.DataDir == "" {
		cfg.DataDir = t.TempDir()
		t.Setenv("IUGUM_DATA", cfg.DataDir)
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// forgetMem mirrors App gate checks until ForgetMem lands on App.
func forgetMem(ctx context.Context, a *App, ns, key string) error {
	if err := a.Check(ctx, contract.MemoryObj("*", ns), "write"); err != nil {
		return err
	}
	return a.Memory.Forget(ctx, ns, key)
}

// linkMem mirrors App gate checks until LinkMem lands on App.
func linkMem(ctx context.Context, a *App, ns string, e contract.MemoryEdge) error {
	if err := a.Check(ctx, contract.MemoryObj("graph", ns), "write"); err != nil {
		return err
	}
	return a.Memory.Link(ctx, ns, e)
}
