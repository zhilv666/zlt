package buildinfo

import (
	"fmt"
	"runtime"
	"strings"
	"time"
)

var (
	Version      = "dev"
	Commit       = "unknown"
	BuildTime    = "unknown"
	BuildProfile = ""
	TargetOS     = ""
	TargetArch   = ""
)

type Info struct {
	Version      string `json:"version"`
	Commit       string `json:"commit"`
	BuildTime    string `json:"build_time"`
	BuildProfile string `json:"build_profile"`
	GoVersion    string `json:"go_version"`
	Platform     string `json:"platform"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
}

func Current() Info {
	osName := TargetOS
	if osName == "" {
		osName = runtime.GOOS
	}

	arch := TargetArch
	if arch == "" {
		arch = runtime.GOARCH
	}

	return Info{
		Version:      normalize(Version, "dev"),
		Commit:       normalize(Commit, "unknown"),
		BuildTime:    normalize(BuildTime, "unknown"),
		BuildProfile: normalize(BuildProfile, defaultBuildProfile()),
		GoVersion:    runtime.Version(),
		Platform:     fmt.Sprintf("%s/%s", osName, arch),
		OS:           osName,
		Arch:         arch,
	}
}

func Summary() string {
	info := Current()
	return fmt.Sprintf(
		"version=%s commit=%s build_time=%s profile=%s platform=%s go=%s",
		info.Version,
		info.Commit,
		info.BuildTime,
		info.BuildProfile,
		info.Platform,
		info.GoVersion,
	)
}

func DisplayVersion(version string) string {
	version = normalize(version, "dev")
	return strings.TrimPrefix(version, "v")
}

func HumanBuildTime(value string) string {
	value = normalize(value, "unknown")
	if value == "unknown" {
		return value
	}

	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05-0700"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			_, offset := parsed.Zone()
			if offset == 0 {
				return parsed.UTC().Format("2006-01-02 15:04:05 UTC")
			}
			return parsed.Format("2006-01-02 15:04:05 -0700")
		}
	}

	return value
}

func defaultBuildProfile() string {
	if normalize(Version, "dev") == "dev" {
		return "debug"
	}
	return "release"
}

func normalize(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
