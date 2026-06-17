// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"runtime/debug"
	"testing"
)

func TestVersionFromBuildInfo(t *testing.T) {
	tests := []struct {
		name     string
		override string
		info     *debug.BuildInfo
		want     string
	}{
		{
			name:     "override wins over everything",
			override: "v1.2.3",
			info: &debug.BuildInfo{
				Main: debug.Module{Version: "v9.9.9"},
			},
			want: "v1.2.3",
		},
		{
			name:     "override of dev is ignored",
			override: "dev",
			info: &debug.BuildInfo{
				Main: debug.Module{Version: "v2.0.0"},
			},
			want: "v2.0.0",
		},
		{
			name:     "main version is a real tag (go install path)",
			override: "",
			info: &debug.BuildInfo{
				Main: debug.Module{Version: "v3.1.0"},
			},
			want: "v3.1.0",
		},
		{
			name:     "devel main with clean vcs revision",
			override: "",
			info: &debug.BuildInfo{
				Main: debug.Module{Version: "(devel)"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef"},
					{Key: "vcs.modified", Value: "false"},
				},
			},
			want: "devel-0123456789ab",
		},
		{
			name:     "devel main with dirty vcs revision",
			override: "",
			info: &debug.BuildInfo{
				Main: debug.Module{Version: "(devel)"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "abcdef0123456789"},
					{Key: "vcs.modified", Value: "true"},
				},
			},
			want: "devel-abcdef012345+dirty",
		},
		{
			name:     "short revision is not truncated",
			override: "",
			info: &debug.BuildInfo{
				Main:     debug.Module{Version: "(devel)"},
				Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"}},
			},
			want: "devel-abc123",
		},
		{
			name:     "devel main with no vcs info falls back to dev",
			override: "",
			info: &debug.BuildInfo{
				Main: debug.Module{Version: "(devel)"},
			},
			want: "dev",
		},
		{
			name:     "empty main version with no vcs info falls back to dev",
			override: "",
			info:     &debug.BuildInfo{},
			want:     "dev",
		},
		{
			name:     "nil build info with no override falls back to dev",
			override: "",
			info:     nil,
			want:     "dev",
		},
		{
			name:     "override still wins when build info is nil",
			override: "v4.5.6",
			info:     nil,
			want:     "v4.5.6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := versionFromBuildInfo(tt.override, tt.info)
			if got != tt.want {
				t.Errorf("versionFromBuildInfo(%q, ...) = %q, want %q", tt.override, got, tt.want)
			}
		})
	}
}

func TestVersionCmd_PrintsBuildDetails(t *testing.T) {
	out, err := runCmd(t, "version")
	if err != nil {
		t.Fatalf("version command error: %v", err)
	}
	for _, want := range []string{"lsm ", "go:", "os/arch:"} {
		if !contains(out, want) {
			t.Errorf("version output %q missing %q", out, want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
