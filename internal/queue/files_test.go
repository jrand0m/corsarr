package queue

import (
	"slices"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

func TestPayloadFilesAsksTheDownloadersFileList(t *testing.T) {
	e := newEnv(t)
	d := e.grab(t, "aaa")
	e.dl.files["aaa"] = []plugin.FileEntry{{Path: "a.mkv", Size: 1}, {Path: "Subs/a.srt", Size: 1}}
	got, err := e.svc.PayloadFiles(ctx, d)
	if err != nil || !slices.Equal(got, []string{"a.mkv", "Subs/a.srt"}) {
		t.Fatal(got, err)
	}
	d.Downloader = domain.ObjectKey("gone")
	if _, err := e.svc.PayloadFiles(ctx, d); err == nil {
		t.Fatal("an unknown downloader cannot list files")
	}
}
