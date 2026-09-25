package main

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"strings"
)

// buildCommit and buildDate are set by scripts/build.sh via:
//
//	-ldflags "-X main.buildCommit=... -X main.buildDate=..."
//
// buildCGO records whether the build was CGO_ENABLED=1 (cgo, embedded Dolt
// works) or CGO_ENABLED=0 (static, beads needs server mode). build.sh sets it
// the same way. All three are empty in a bare `go build .` (no script), which
// is why versionInfo() falls back to runtime/debug's vcs.* settings.
var (
	buildCommit = ""
	buildDate   = ""
	buildCGO    = ""
)

// versionInfo is the resolved, printable state of the four fields
// `iugum version` reports. It exists apart from the package vars so the
// fallback logic (ldflags -X first, runtime/debug.ReadBuildInfo second,
// "unknown" last) is a pure function a table test can drive directly.
type versionInfo struct {
	Commit string
	Date   string
	Go     string
	CGO    string
}

// resolveVersionInfo picks each field from ldflags values first, then from a
// debug.BuildInfo (as runtime/debug.ReadBuildInfo would return for the running
// binary), then "unknown". goVersion is passed in rather than read from
// runtime.Version() so the table test can drive it too.
func resolveVersionInfo(commit, date, cgo string, info *debug.BuildInfo, goVersion string) versionInfo {
	v := versionInfo{
		Commit: commit,
		Date:   date,
		CGO:    cgo,
	}

	var revision, modified string
	if info != nil {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				revision = s.Value
			case "vcs.modified":
				modified = s.Value
			}
		}
	}

	if v.Commit == "" && revision != "" {
		v.Commit = revision
		if len(v.Commit) > 12 {
			v.Commit = v.Commit[:12]
		}
	}
	if v.Commit != "" && modified == "true" && !strings.HasSuffix(v.Commit, "-dirty") {
		v.Commit += "-dirty"
	}
	if v.Commit == "" {
		v.Commit = "unknown"
	}

	if v.Date == "" {
		v.Date = "unknown"
	} else if idx := strings.IndexByte(v.Date, 'T'); idx >= 0 {
		// build.sh stamps YYYY-MM-DDTHH:MM (no space) because ldflags -X
		// splits its value on whitespace; render it back as "YYYY-MM-DD HH:MM".
		v.Date = v.Date[:idx] + " " + v.Date[idx+1:]
	}

	if v.CGO == "" {
		v.CGO = "unknown"
	}

	v.Go = goVersion
	if v.Go == "" {
		if info != nil && info.GoVersion != "" {
			v.Go = info.GoVersion
		} else {
			v.Go = "unknown"
		}
	}

	return v
}

// currentVersionInfo resolves versionInfo for the running binary.
func currentVersionInfo() versionInfo {
	info, _ := debug.ReadBuildInfo()
	return resolveVersionInfo(buildCommit, buildDate, buildCGO, info, runtime.Version())
}

// versionLines formats versionInfo as the four lines `iugum version` prints.
func versionLines(v versionInfo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "commit: %s\n", v.Commit)
	fmt.Fprintf(&b, "build date: %s\n", v.Date)
	fmt.Fprintf(&b, "go version: %s\n", v.Go)
	fmt.Fprintf(&b, "cgo: %s\n", v.CGO)
	return b.String()
}

func runVersion(stdout io.Writer) int {
	fmt.Fprint(stdout, versionLines(currentVersionInfo()))
	return 0
}
