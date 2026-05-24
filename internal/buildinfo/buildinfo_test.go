package buildinfo

import (
	"runtime"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	if got := normalize("  value  ", "fallback"); got != "value" {
		t.Fatalf("normalize returned %q", got)
	}
	if got := normalize("   ", "fallback"); got != "fallback" {
		t.Fatalf("normalize fallback returned %q", got)
	}
}

func TestCurrentUsesFallbackAndOverrides(t *testing.T) {
	oldVersion, oldCommit, oldBuildTime, oldProfile := Version, Commit, BuildTime, BuildProfile
	oldOS, oldArch := TargetOS, TargetArch
	t.Cleanup(func() {
		Version, Commit, BuildTime, BuildProfile = oldVersion, oldCommit, oldBuildTime, oldProfile
		TargetOS, TargetArch = oldOS, oldArch
	})

	Version = ""
	Commit = ""
	BuildTime = ""
	BuildProfile = ""
	TargetOS = ""
	TargetArch = ""

	info := Current()
	if info.Version != "dev" {
		t.Fatalf("expected default version, got %q", info.Version)
	}
	if info.Commit != "unknown" {
		t.Fatalf("expected default commit, got %q", info.Commit)
	}
	if info.BuildTime != "unknown" {
		t.Fatalf("expected default build time, got %q", info.BuildTime)
	}
	if info.BuildProfile != "debug" {
		t.Fatalf("expected default build profile, got %q", info.BuildProfile)
	}
	if info.OS != runtime.GOOS || info.Arch != runtime.GOARCH {
		t.Fatalf("expected runtime platform, got %s/%s", info.OS, info.Arch)
	}

	Version = "v1.2.3"
	Commit = "abc123"
	BuildTime = "2026-05-20T00:00:00+0800"
	BuildProfile = "release"
	TargetOS = "windows"
	TargetArch = "amd64"

	info = Current()
	if info.Version != "v1.2.3" || info.Commit != "abc123" || info.BuildTime != "2026-05-20T00:00:00+0800" || info.BuildProfile != "release" {
		t.Fatalf("unexpected build info: %+v", info)
	}
	if info.Platform != "windows/amd64" {
		t.Fatalf("unexpected platform %q", info.Platform)
	}
}

func TestSummaryContainsCoreFields(t *testing.T) {
	oldVersion, oldCommit, oldBuildTime, oldProfile := Version, Commit, BuildTime, BuildProfile
	oldOS, oldArch := TargetOS, TargetArch
	t.Cleanup(func() {
		Version, Commit, BuildTime, BuildProfile = oldVersion, oldCommit, oldBuildTime, oldProfile
		TargetOS, TargetArch = oldOS, oldArch
	})

	Version = "v9.9.9"
	Commit = "deadbee"
	BuildTime = "2026-05-20T00:00:00+0800"
	BuildProfile = "release"
	TargetOS = "linux"
	TargetArch = "amd64"

	summary := Summary()
	for _, part := range []string{"version=v9.9.9", "commit=deadbee", "build_time=2026-05-20T00:00:00+0800", "profile=release", "platform=linux/amd64", "go="} {
		if !strings.Contains(summary, part) {
			t.Fatalf("summary %q does not contain %q", summary, part)
		}
	}
}

func TestDisplayVersion(t *testing.T) {
	if got := DisplayVersion("v0.2.0"); got != "0.2.0" {
		t.Fatalf("unexpected display version %q", got)
	}
	if got := DisplayVersion("dev"); got != "dev" {
		t.Fatalf("unexpected display version %q", got)
	}
}

func TestHumanBuildTime(t *testing.T) {
	if got := HumanBuildTime("2026-05-20T00:00:00Z"); got != "2026-05-20 00:00:00 UTC" {
		t.Fatalf("unexpected human build time %q", got)
	}
	if got := HumanBuildTime("2026-05-20T08:00:00+0800"); got != "2026-05-20 08:00:00 +0800" {
		t.Fatalf("unexpected human build time %q", got)
	}
}
