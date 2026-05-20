package buildinfo

import (
	"fmt"
	"runtime"
	"strings"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
	BuiltBy   = "unknown"
	TargetOS  = ""
	TargetArch = ""
)

type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	BuiltBy   string `json:"built_by"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
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
		Version:   normalize(Version, "dev"),
		Commit:    normalize(Commit, "unknown"),
		BuildTime: normalize(BuildTime, "unknown"),
		BuiltBy:   normalize(BuiltBy, "unknown"),
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", osName, arch),
		OS:        osName,
		Arch:      arch,
	}
}

func Summary() string {
	info := Current()
	return fmt.Sprintf(
		"version=%s commit=%s build_time=%s platform=%s go=%s built_by=%s",
		info.Version,
		info.Commit,
		info.BuildTime,
		info.Platform,
		info.GoVersion,
		info.BuiltBy,
	)
}

func normalize(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
