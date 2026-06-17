// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Version is set at release time via ldflags
// (-X github.com/llbbl/lsm/internal/cmd.Version=<tag>). When empty/"dev" it
// falls through to runtime/debug.BuildInfo so that `go install ...@vX.Y.Z`
// users and local `go build` users still get a meaningful version string.
var Version = "dev"

// versionInfo is the fully-resolved set of build facts surfaced by
// `lsm version` and `lsm --version`.
type versionInfo struct {
	Version   string
	Commit    string
	BuildTime string
	GoVersion string
	OS        string
	Arch      string
}

// resolveVersion picks the binary's version using the live build environment.
// It delegates the priority logic to versionFromBuildInfo so the branches are
// unit-testable without a real build.
func resolveVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		info = nil
	}
	return versionFromBuildInfo(Version, info)
}

// versionFromBuildInfo resolves a version string from an ldflags override and
// an optional *debug.BuildInfo, in priority order:
//
//  1. override (the release ldflags value), if non-empty and not "dev".
//  2. info.Main.Version, if present and not the placeholder "(devel)" — this
//     is the value the toolchain stamps on the `go install pkg@vX.Y.Z` path.
//  3. a "devel-<sha>[+dirty]" string derived from the vcs.* build settings,
//     when a VCS revision is available (the local `go build` path).
//  4. "dev" as a last resort.
func versionFromBuildInfo(override string, info *debug.BuildInfo) string {
	if override != "" && override != "dev" {
		return override
	}
	if info == nil {
		return "dev"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	if dev := develVersionFromSettings(info.Settings); dev != "" {
		return dev
	}
	return "dev"
}

// develVersionFromSettings builds a "devel-<sha>[+dirty]" version from the
// vcs.* build settings, or "" when no VCS revision was recorded.
func develVersionFromSettings(settings []debug.BuildSetting) string {
	var revision string
	var modified bool
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if revision == "" {
		return ""
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	v := "devel-" + revision
	if modified {
		v += "+dirty"
	}
	return v
}

// resolveVersionInfo gathers the full set of build facts for display.
func resolveVersionInfo() versionInfo {
	vi := versionInfo{
		Version:   resolveVersion(),
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		vi.GoVersion = info.GoVersion
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				vi.Commit = s.Value
			case "vcs.time":
				vi.BuildTime = s.Value
			}
		}
	}
	return vi
}

// String renders the detailed multi-line form used by `lsm version` and the
// `lsm --version` template.
func (vi versionInfo) String() string {
	out := fmt.Sprintf("lsm %s\n", vi.Version)
	if vi.Commit != "" {
		out += fmt.Sprintf("  commit:  %s\n", vi.Commit)
	}
	if vi.BuildTime != "" {
		out += fmt.Sprintf("  built:   %s\n", vi.BuildTime)
	}
	out += fmt.Sprintf("  go:      %s\n", vi.GoVersion)
	out += fmt.Sprintf("  os/arch: %s/%s\n", vi.OS, vi.Arch)
	return out
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit, and build information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(cmd.OutOrStdout(), resolveVersionInfo().String())
			return err
		},
	}
}
