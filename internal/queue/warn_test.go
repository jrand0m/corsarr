package queue

import (
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

func TestClientWarningsStallWithAReasonAndNeverFail(t *testing.T) {
	e := newEnv(t)
	d := e.grab(t, "aaa")
	e.dl.set("aaa", func(s *plugin.Snapshot) { s.State, s.Error = domain.StateStalled, "disk full" })
	_ = e.svc.Poll(ctx)
	got, _ := e.svc.Get(ctx, d.ID)
	if got.State != domain.StateStalled || got.StatusReason != "disk full" {
		t.Fatalf("queued download with a client warning: %s %q", got.State, got.StatusReason)
	}
	e.dl.set("aaa", func(s *plugin.Snapshot) { s.Error = "files missing" })
	_ = e.svc.Poll(ctx)
	if got, _ = e.svc.Get(ctx, d.ID); got.StatusReason != "files missing" {
		t.Fatalf("reason follows the client: %q", got.StatusReason)
	}
	e.dl.set("aaa", func(s *plugin.Snapshot) { s.State, s.Error = domain.StateDownloading, "" })
	_ = e.svc.Poll(ctx)
	if got, _ = e.svc.Get(ctx, d.ID); got.State != domain.StateDownloading || got.StatusReason != "" {
		t.Fatalf("recovered: %s %q", got.State, got.StatusReason)
	}
	if len(e.rec.Of(domain.EvDownloadFailed)) != 0 {
		t.Fatal("a client warning must not fail the download")
	}
}
