package main

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestResolveVersionInfo(t *testing.T) {
	cases := []struct {
		name       string
		commit     string
		date       string
		cgo        string
		info       *debug.BuildInfo
		goVersion  string
		wantCommit string
		wantDate   string
		wantGo     string
		wantCGO    string
	}{
		{
			name:       "ldflags set, clean tree",
			commit:     "abc123def456",
			date:       "2026-09-25 04:07",
			cgo:        "on",
			goVersion:  "go1.26.5",
			wantCommit: "abc123def456",
			wantDate:   "2026-09-25 04:07",
			wantGo:     "go1.26.5",
			wantCGO:    "on",
		},
		{
			name:      "ldflags empty, falls back to vcs.* settings",
			commit:    "",
			date:      "",
			cgo:       "",
			goVersion: "",
			info: &debug.BuildInfo{
				GoVersion: "go1.26.5",
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"},
					{Key: "vcs.modified", Value: "false"},
				},
			},
			wantCommit: "0123456789ab",
			wantDate:   "unknown",
			wantGo:     "go1.26.5",
			wantCGO:    "unknown",
		},
		{
			name:      "ldflags empty, dirty vcs tree",
			commit:    "",
			date:      "",
			cgo:       "",
			goVersion: "go1.26.5",
			info: &debug.BuildInfo{
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "fedcba987654"},
					{Key: "vcs.modified", Value: "true"},
				},
			},
			wantCommit: "fedcba987654-dirty",
			wantDate:   "unknown",
			wantGo:     "go1.26.5",
			wantCGO:    "unknown",
		},
		{
			name:       "nothing at all: unknown, never a blank field",
			commit:     "",
			date:       "",
			cgo:        "",
			info:       nil,
			goVersion:  "",
			wantCommit: "unknown",
			wantDate:   "unknown",
			wantGo:     "unknown",
			wantCGO:    "unknown",
		},
		{
			name:      "ldflags commit set, no dirty suffix added twice",
			commit:    "aaaaaaaaaaaa-dirty",
			date:      "2026-09-25 04:07",
			cgo:       "off",
			goVersion: "go1.26.5",
			info: &debug.BuildInfo{
				Settings: []debug.BuildSetting{
					{Key: "vcs.modified", Value: "true"},
				},
			},
			wantCommit: "aaaaaaaaaaaa-dirty",
			wantDate:   "2026-09-25 04:07",
			wantGo:     "go1.26.5",
			wantCGO:    "off",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveVersionInfo(tc.commit, tc.date, tc.cgo, tc.info, tc.goVersion)
			if got.Commit != tc.wantCommit {
				t.Errorf("Commit = %q, want %q", got.Commit, tc.wantCommit)
			}
			if got.Date != tc.wantDate {
				t.Errorf("Date = %q, want %q", got.Date, tc.wantDate)
			}
			if got.Go != tc.wantGo {
				t.Errorf("Go = %q, want %q", got.Go, tc.wantGo)
			}
			if got.CGO != tc.wantCGO {
				t.Errorf("CGO = %q, want %q", got.CGO, tc.wantCGO)
			}
		})
	}
}

func TestVersionLinesFormatsDateWithoutT(t *testing.T) {
	v := versionInfo{Commit: "abc123", Date: "2026-09-25 04:07", Go: "go1.26.5", CGO: "off"}
	out := versionLines(v)
	for _, want := range []string{"commit: abc123", "build date: 2026-09-25 04:07", "go version: go1.26.5", "cgo: off"} {
		if !strings.Contains(out, want) {
			t.Errorf("versionLines() = %q, missing %q", out, want)
		}
	}
}
