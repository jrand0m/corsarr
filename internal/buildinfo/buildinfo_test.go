package buildinfo

import (
	"runtime"
	"runtime/debug"
	"testing"
)

func stamp(t *testing.T, c, tg, e, d string) {
	t.Helper()
	oc, ot, oe, od := commit, tag, epoch, dirty
	commit, tag, epoch, dirty = c, tg, e, d
	t.Cleanup(func() { commit, tag, epoch, dirty = oc, ot, oe, od })
}

func TestResolveStamped(t *testing.T) {
	stamp(t, "abc123", "v1.2.3", "1700000000", "false")
	vcs := []debug.BuildSetting{{Key: "vcs.revision", Value: "other"}, {Key: "vcs.modified", Value: "true"}}
	got := resolve("v1.2.3", "go1.99", vcs)
	want := Info{Version: "v1.2.3", Commit: "abc123", Tag: "v1.2.3", GoVersion: "go1.99", BuildDate: "2023-11-14T22:13:20Z"}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestResolveFallsBackToVCS(t *testing.T) {
	stamp(t, "", "", "", "")
	vcs := []debug.BuildSetting{{Key: "vcs.revision", Value: "def456"}, {Key: "vcs.time", Value: "2026-01-02T03:04:05Z"}, {Key: "vcs.modified", Value: "true"}}
	got := resolve("dev", "go1.99", vcs)
	if got.Commit != "def456" || got.BuildDate != "2026-01-02T03:04:05Z" || !got.Dirty || got.Tag != "" {
		t.Fatalf("%+v", got)
	}
}

func TestEpochDateInvalid(t *testing.T) {
	if epochDate("") != "" || epochDate("soon") != "" {
		t.Fatal("invalid epoch should render empty")
	}
}

func TestGetReportsToolchain(t *testing.T) {
	if got := Get("dev"); got.Version != "dev" || got.GoVersion != runtime.Version() {
		t.Fatalf("%+v", got)
	}
}
