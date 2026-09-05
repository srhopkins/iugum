package sbadapt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/embedbin"
	"github.com/srhopkins/iugum/plugin"
	"github.com/srhopkins/iugum/spaceseed"
)

// Environment switches. The embedded release binary stays the default, so a
// host with no Rust or Node toolchain keeps working.
//
//	IUGUM_WIKI_SB_BIN      run this SilverBullet binary
//	IUGUM_WIKI_SB_SRC      build the SilverBullet source in this directory, then run it
//	IUGUM_WIKI_SB_REBUILD  build again even when a release artifact is present
//
// IUGUM_WIKI_SB_BIN wins over IUGUM_WIKI_SB_SRC. Neither one changes the
// embedded binary, so a source build cannot break the default path.
const (
	envBin     = "IUGUM_WIKI_SB_BIN"
	envSrc     = "IUGUM_WIKI_SB_SRC"
	envRebuild = "IUGUM_WIKI_SB_REBUILD"
	// envSeed controls the atomdown space assets that the program carries.
	//
	//	off    do not touch the space
	//	force  take the built-in copy, even over a changed file
	//
	// The default is additive: write a missing file, keep a changed one.
	envSeed = "IUGUM_WIKI_SEED"
)

func init() {
	plugin.RegisterWiki("silverbullet", func(map[string]string) (contract.Wiki, error) {
		return Wiki{}, nil
	})
}

// Wiki runs a SilverBullet server binary. The binary is the embedded release
// blob, a path from IUGUM_WIKI_SB_BIN, or a build of the source tree named by
// IUGUM_WIKI_SB_SRC.
type Wiki struct{}

func (Wiki) Name() string { return "silverbullet" }

func (Wiki) Serve(_ context.Context, opts contract.WikiOpts) error {
	if opts.Port == 0 {
		opts.Port = 3000
	}
	if opts.Host == "" {
		opts.Host = "127.0.0.1"
	}
	if opts.Space == "" {
		opts.Space = "./wiki"
	}
	if err := os.MkdirAll(opts.Space, 0o755); err != nil {
		return err
	}
	if err := seedSpace(opts.Space); err != nil {
		return err
	}
	path, cleanup, err := resolveBinary()
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return err
	}
	args := []string{"-p", strconv.Itoa(opts.Port), "-L", opts.Host, opts.Space}
	// SilverBullet 2.10.0 and later need --single for one-space mode. Version
	// 2.9.0 and earlier reject the flag, so ask the binary what it accepts.
	if supportsSingle(path) {
		args = append([]string{"--single"}, args...)
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wiki: %w", err)
	}
	return nil
}

// seedSpace writes the compiled-in atomdown assets into the space. It runs
// once per space per process, so the supervised restart in `iugum up` does
// not repeat the log. A seed failure stops the server: a half-seeded space
// draws the cards with no CSS, which looks like a broken feature.
func seedSpace(space string) error {
	switch os.Getenv(envSeed) {
	case "off", "0", "no":
		return nil
	case "force":
		_, err := spaceseed.Seed(space, spaceseed.Options{Force: true, Once: true, Log: os.Stdout})
		return err
	default:
		_, err := spaceseed.Seed(space, spaceseed.Options{Once: true, Log: os.Stdout})
		return err
	}
}

// resolveBinary finds a SilverBullet binary to run. The second result removes
// the temporary file that holds the embedded binary. It is nil for the other
// two paths, which run a file that is already on disk.
func resolveBinary() (string, func(), error) {
	if p := os.Getenv(envBin); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", nil, fmt.Errorf("wiki: %s=%s: %w", envBin, p, err)
		}
		return p, nil, nil
	}
	if src := os.Getenv(envSrc); src != "" {
		p, err := buildFromSource(src)
		return p, nil, err
	}
	return writeEmbedded()
}

// writeEmbedded copies the embedded release binary to a temporary file.
func writeEmbedded() (string, func(), error) {
	f, err := os.CreateTemp("", "iugum-silverbullet-*")
	if err != nil {
		return "", nil, err
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := f.Write(embedbin.Silverbullet); err != nil {
		_ = f.Close()
		return "", cleanup, err
	}
	if err := f.Chmod(0o700); err != nil {
		_ = f.Close()
		return "", cleanup, err
	}
	if err := f.Close(); err != nil {
		return "", cleanup, err
	}
	return path, cleanup, nil
}

// buildFromSource builds the SilverBullet server from the tree at src and
// returns the release artifact. It needs make, npm, and cargo on PATH.
func buildFromSource(src string) (string, error) {
	dir, err := filepath.Abs(src)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(dir, "Makefile")); err != nil {
		return "", fmt.Errorf("wiki: %s=%s is not a SilverBullet source tree: %w", envSrc, src, err)
	}
	out := filepath.Join(dir, "target", "release", "silverbullet")
	if _, err := os.Stat(out); err == nil && os.Getenv(envRebuild) == "" {
		return out, nil
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); err != nil {
		if err := run(dir, "npm", "install"); err != nil {
			return "", err
		}
	}
	// build-rs makes the client bundle, then the release server binary.
	if err := run(dir, "make", "build-rs"); err != nil {
		return "", err
	}
	if _, err := os.Stat(out); err != nil {
		return "", fmt.Errorf("wiki: source build made no %s: %w", out, err)
	}
	return out, nil
}

func run(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	// Build output goes to stderr so it cannot corrupt piped stdout.
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wiki: %s %v in %s: %w", name, args, dir, err)
	}
	return nil
}

// supportsSingle reports whether the binary accepts --single.
func supportsSingle(path string) bool {
	out, err := exec.Command(path, "--help").CombinedOutput()
	if err != nil && len(out) == 0 {
		return false
	}
	return bytes.Contains(out, []byte("--single"))
}
