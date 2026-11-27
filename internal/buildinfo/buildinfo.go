// Package buildinfo reports where a binary came from: the version, the source
// commit and tag it was built from, the Go toolchain, the build date and
// whether the source tree had uncommitted changes.
//
// Release builds stamp the values with -ldflags -X (see the Dockerfile); the
// build date is SOURCE_DATE_EPOCH, the commit time of the tag, so two builds
// of one tag produce identical bytes. Plain `go build` in a checkout falls
// back to the VCS stamp the Go toolchain embeds.
package buildinfo

import (
	"runtime"
	"runtime/debug"
	"strconv"
	"time"
)

// Stamped at link time: -X github.com/jrand0m/corsarr/internal/buildinfo.commit=…
var (
	commit string
	tag    string
	epoch  string // SOURCE_DATE_EPOCH, seconds since 1970 UTC
	dirty  string // "true" or "false"
)

// Info is the provenance of the running binary.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Tag       string `json:"tag"`
	GoVersion string `json:"goVersion"`
	BuildDate string `json:"buildDate"`
	Dirty     bool   `json:"dirty"`
}

// Get returns the provenance; version is the main package's stamped version.
func Get(version string) Info {
	var settings []debug.BuildSetting
	if bi, ok := debug.ReadBuildInfo(); ok {
		settings = bi.Settings
	}
	return resolve(version, runtime.Version(), settings)
}

// resolve prefers link-time values and fills gaps from the toolchain's VCS stamp.
func resolve(version, goVersion string, settings []debug.BuildSetting) Info {
	vcs := map[string]string{}
	for _, s := range settings {
		vcs[s.Key] = s.Value
	}
	in := Info{Version: version, Commit: commit, Tag: tag, GoVersion: goVersion, BuildDate: epochDate(epoch), Dirty: dirty == "true"}
	if in.Commit == "" {
		in.Commit = vcs["vcs.revision"]
		in.Dirty = vcs["vcs.modified"] == "true"
	}
	if in.BuildDate == "" {
		in.BuildDate = vcs["vcs.time"]
	}
	return in
}

// epochDate renders a Unix-seconds string as RFC 3339 UTC; empty when unset or invalid.
func epochDate(s string) string {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return ""
	}
	return time.Unix(n, 0).UTC().Format(time.RFC3339)
}
