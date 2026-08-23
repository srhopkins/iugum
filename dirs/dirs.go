// Package dirs resolves iugum config and data folders for macOS, Linux, and Windows.
package dirs

import (
	"os"
	"path/filepath"
	"runtime"
)

// ConfigSearch returns candidate config files, first existing wins at load time.
// Order: IUGUM_CONFIG, ./iugum.yaml, ~/.config/iugum/config.yaml,
// then os.UserConfigDir()/iugum/config.yaml (Library on macOS).
func ConfigSearch() []string {
	var out []string
	if p := os.Getenv("IUGUM_CONFIG"); p != "" {
		out = append(out, p)
	}
	out = append(out, "iugum.yaml")
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, ".config", "iugum", "config.yaml"))
	}
	if cfg, err := os.UserConfigDir(); err == nil {
		out = append(out, filepath.Join(cfg, "iugum", "config.yaml"))
	}
	return out
}

// DataDir is the host default for SQLite and other user data.
// Override with IUGUM_DATA or config data_dir.
// macOS: ~/Library/Application Support/iugum
// Linux: $XDG_DATA_HOME/iugum or ~/.local/share/iugum
// Windows: %LocalAppData%\iugum
func DataDir() (string, error) {
	if p := os.Getenv("IUGUM_DATA"); p != "" {
		return p, nil
	}
	return defaultDataDir()
}

func defaultDataDir() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "iugum"), nil
	case "windows":
		if p := os.Getenv("LOCALAPPDATA"); p != "" {
			return filepath.Join(p, "iugum"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "AppData", "Local", "iugum"), nil
	default:
		if p := os.Getenv("XDG_DATA_HOME"); p != "" {
			return filepath.Join(p, "iugum"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "share", "iugum"), nil
	}
}

// ResolveData picks IUGUM_DATA, then override, then DataDir.
func ResolveData(override string) (string, error) {
	if p := os.Getenv("IUGUM_DATA"); p != "" {
		return p, nil
	}
	if override != "" {
		return override, nil
	}
	return defaultDataDir()
}
