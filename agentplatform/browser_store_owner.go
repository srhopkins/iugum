package agentplatform

import (
	"errors"
	"github.com/gofrs/flock"
	"os"
	"path/filepath"
)

// Optional browser servers claim their shared DataDir before loading/writing any
// existing session/host state, not merely when the receipt authority starts.
func lockBrowserDataDir(c Config) (func(), error) {
	if !c.BrowserGrants {
		return nil, nil
	}
	if e := os.MkdirAll(c.DataDir, 0700); e != nil {
		return nil, e
	}
	lock := flock.New(filepath.Join(c.DataDir, "browser-server.lock"))
	ok, e := lock.TryLock()
	if e != nil || !ok {
		_ = lock.Close()
		return nil, errors.New("browser-enabled session data directory already owned")
	}
	return func() { _ = lock.Close() }, nil
}
