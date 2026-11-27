package queue

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

func TestGrabCreatesOneDownloadAndPollsToCompletion(t *testing.T) {
	e := newEnv(t)
	d := e.grab(t, "aaa")
	if d.DownloadID != "aaa" || d.Category != "movies" || len(d.Links) != 1 || d.Links[0].Status != domain.LinkPending {
		t.Fatalf("%+v", d)
	}
	sent := e.rec.Of(domain.EvGrabSent)
	if len(sent) != 1 || sent[0].Data.(domain.GrabEventData).DownloadID != d.ID || e.jobs.runs != 1 || e.dl.ensured != 1 {
		t.Fatalf("grab.sent %+v runs %d", sent, e.jobs.runs)
	}
	e.grab(t, "bbb") // category ensured once per downloader
	if e.dl.ensured != 1 {
		t.Fatal("category ensured twice")
	}
	e.dl.set("aaa", func(s *plugin.Snapshot) { s.State, s.Progress, s.Downloaded = domain.StateDownloading, 0.5, 2e9 })
	if err := e.svc.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	e.dl.set("aaa", func(s *plugin.Snapshot) { s.Progress, s.UpSpeed, s.Ratio, s.Seeders, s.Peers = 0.7, 300, 0.4, 5, 9 })
	_ = e.svc.Poll(ctx)
	if got, _ := e.svc.Get(ctx, d.ID); got.UpSpeed != 300 || got.Ratio != 0.4 || got.Seeders != 5 || got.Peers != 9 {
		t.Fatalf("upload, ratio and swarm not recorded: %+v", got)
	}
	e.dl.set("aaa", func(s *plugin.Snapshot) {
		s.State, s.Progress, s.ContentPath, s.SavePath = domain.StateCompleted, 1, "/dl/Example", "/dl"
	})
	_ = e.svc.Poll(ctx)
	if got := states(e.rec); len(got) != 2 || got[0] != "queued>downloading" || got[1] != "downloading>completed" {
		t.Fatalf("transitions %v", got)
	}
	done := e.rec.Of(domain.EvDownloadCompleted)
	if len(done) != 1 || done[0].Data.(domain.DownloadEventData).ContentPath != "/dl/Example" || done[0].Items[0].Ref != movie.Ref {
		t.Fatalf("%+v", done)
	}
	if len(e.rec.Of(domain.EvDownloadProgress)) != 1 {
		t.Fatal("progress published when it moved without a state change")
	}
	got, _ := e.svc.Get(ctx, d.ID)
	if got.CompletedAt == nil || got.Progress != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestImportReportsDriveTheAggregate(t *testing.T) {
	e := newEnv(t)
	d := e.grab(t, "aaa")
	e.dl.set("aaa", func(s *plugin.Snapshot) { s.State = domain.StateCompleted })
	_ = e.svc.Poll(ctx)
	if err := e.svc.Transition(ctx, d.ID, domain.StateImportPending, domain.ActorImport, ""); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Transition(ctx, d.ID, domain.StateImported, domain.ActorImport, ""); err == nil {
		t.Fatal("illegal move accepted")
	}
	_ = e.svc.Transition(ctx, d.ID, domain.StateImporting, domain.ActorImport, "")
	err := e.svc.ReportImport(ctx, domain.ImportOutcome{DownloadID: d.ID, Status: domain.ImportImported,
		Items: []domain.ImportItem{{Item: movie.Ref, Outcome: "imported"}}})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(ctx, d.ID)
	if got.State != domain.StateImported || got.Links[0].Status != domain.LinkImported {
		t.Fatalf("%+v", got)
	}
	if err := e.svc.Transition(ctx, 99, domain.StateFailed, domain.ActorUser, ""); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err := e.svc.ReportImport(ctx, domain.ImportOutcome{DownloadID: 99}); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestPayloadCheckAndSafetyAction(t *testing.T) {
	e := newEnv(t)
	d := e.grab(t, "aaa")
	e.dl.set("aaa", func(s *plugin.Snapshot) { s.State = domain.StateDownloading })
	_ = e.svc.Poll(ctx) // no file list yet: unknown, checked later
	e.dl.files["aaa"] = []plugin.FileEntry{{Path: "Example/movie.mkv", Size: 100}, {Path: "Example/movie.mkv.scr", Size: 1}}
	_ = e.svc.Poll(ctx)
	rej := e.rec.Of(domain.EvPayloadRejected)
	if len(rej) != 1 {
		t.Fatalf("payload.rejected: %d", len(rej))
	}
	data := rej[0].Data.(domain.PayloadEventData)
	if data.Stage != domain.StagePreDownload || data.Files[0] != "Example/movie.mkv.scr" || data.Release.GUID != "g-aaa" {
		t.Fatalf("%+v", data)
	}
	_ = e.svc.Poll(ctx) // checked once only
	if len(e.rec.Of(domain.EvPayloadRejected)) != 1 {
		t.Fatal("verdict taken twice")
	}
	e.svc.OnPayloadRejected(ctx, rej[0])
	if del, ok := e.dl.removed["aaa"]; !ok || !del {
		t.Fatalf("safety removes with files: %v", e.dl.removed)
	}
	rm := e.rec.Of(domain.EvDownloadRemoved)
	if len(rm) != 1 || rm[0].Data.(domain.DownloadEventData).Cause != "unsafe_payload" || !rm[0].Data.(domain.DownloadEventData).FilesDeleted {
		t.Fatalf("%+v", rm)
	}
	e.svc.OnPayloadRejected(ctx, rej[0]) // idempotent
	e.svc.OnPayloadRejected(ctx, domain.Event{Data: "junk"})
	if len(e.rec.Of(domain.EvDownloadRemoved)) != 1 {
		t.Fatal("removed twice")
	}
	if got, _ := e.svc.Get(ctx, d.ID); got.State != domain.StateRemoved {
		t.Fatal(got.State)
	}
}

func TestSafePayloadContinues(t *testing.T) {
	e := newEnv(t)
	e.grab(t, "aaa")
	e.dl.files["aaa"] = []plugin.FileEntry{{Path: "Example/movie.mkv", Size: 100}}
	e.dl.set("aaa", func(s *plugin.Snapshot) { s.State = domain.StateDownloading })
	_ = e.svc.Poll(ctx)
	if len(e.rec.Of(domain.EvPayloadRejected)) != 0 || len(states(e.rec)) != 1 {
		t.Fatal("safe payload keeps going")
	}
}

func TestFailuresAndExternalRemoval(t *testing.T) {
	e := newEnv(t)
	d := e.grab(t, "aaa")
	e.dl.set("aaa", func(s *plugin.Snapshot) { s.State, s.Error = domain.StateFailed, "the client gave up" })
	_ = e.svc.Poll(ctx)
	failed := e.rec.Of(domain.EvDownloadFailed)
	if len(failed) != 1 || failed[0].Data.(domain.DownloadEventData).Cause != "downloader" {
		t.Fatalf("%+v", failed)
	}
	d2 := e.grab(t, "bbb")
	delete(e.dl.snaps, "bbb")
	_ = e.svc.Poll(ctx) // within the grace period: still there
	e.now = e.now.Add(MissingGrace + time.Second)
	_ = e.svc.Sweep(ctx)
	got, _ := e.svc.Get(ctx, d2.ID)
	got1, _ := e.svc.Get(ctx, d.ID)
	if got.State != domain.StateRemoved || got1.State != domain.StateFailed {
		t.Fatalf("%s %s", got.State, got1.State)
	}
	rm := e.rec.Of(domain.EvDownloadRemoved)
	if len(rm) != 1 || rm[0].Data.(domain.DownloadEventData).Cause != "external" {
		t.Fatalf("%+v", rm)
	}
	e.dl.listErr = errors.New("down")
	if err := e.svc.Poll(ctx); err != nil {
		t.Fatal("a failing downloader degrades only itself")
	}
}

func TestGrabFailures(t *testing.T) {
	e := newEnv(t)
	req := GrabRequest{Kind: domain.KindMovie, Items: []domain.EventItem{movie}, Release: release("aaa"), Origin: domain.OriginInteractive}
	e.dl.addErr = errors.New("refused")
	if _, err := e.svc.Grab(ctx, req); err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatal(err)
	}
	e.dl.addErr, e.ix.err = nil, errors.New("expired")
	if _, err := e.svc.Grab(ctx, req); err == nil || !strings.Contains(err.Error(), "payload") {
		t.Fatal(err)
	}
	e.ix.err = nil
	noIx := req
	noIx.Release.Indexers = []domain.IndexerRef{{Key: "gone"}}
	if _, err := e.svc.Grab(ctx, noIx); err == nil {
		t.Fatal("unknown indexer")
	}
	noIx.Release.Indexers = nil
	if _, err := e.svc.Grab(ctx, noIx); err == nil {
		t.Fatal("no indexer")
	}
	usenet := req
	usenet.Release.Protocol = domain.ProtoUsenet
	if _, err := e.svc.Grab(ctx, usenet); !errors.Is(err, ErrNoDownloader) {
		t.Fatal(err)
	}
	failed := e.rec.Of(domain.EvGrabFailed)
	if len(failed) != 5 || failed[0].Actor != "user" || failed[0].Data.(domain.GrabEventData).Reason == "" {
		t.Fatalf("%d %+v", len(failed), failed)
	}
}

func TestLookups(t *testing.T) {
	e := newEnv(t)
	e.dl.category = "films"
	d := e.grab(t, "aaa")
	if d.Category != "films" {
		t.Fatal("downloader category wins")
	}
	qs, err := e.svc.QueuedQualities(ctx, movie.Ref)
	if err != nil || len(qs) != 1 {
		t.Fatal(qs, err)
	}
	if ok, _ := e.svc.GrabbedBefore(ctx, movie.Ref, release("aaa")); !ok {
		t.Fatal("grabbed before")
	}
	g, ok, err := e.svc.GrabOf(ctx, d)
	if err != nil || !ok || g.Release.DownloadURL != "magnet:?xt=urn:btih:aaa" {
		t.Fatal(g, err)
	}
}

func TestAdaptivePolling(t *testing.T) {
	for _, c := range []struct {
		load Load
		want time.Duration
		on   bool
	}{
		{Load{UI: 1}, 0, false},
		{Load{Active: 1, UI: 1}, FastPoll, true},
		{Load{Active: 2}, SlowPoll, true},
		{Load{Waiting: 1}, SlowPoll, true}, // finished downloads waiting for import keep polling on
		{Load{Active: 1, UI: 1, IdleRuns: IdleGrace}, FastPoll, true},
		{Load{Active: 1, UI: 1, IdleRuns: IdleGrace + 1}, 2 * FastPoll, true},
		{Load{Active: 1, IdleRuns: IdleGrace + 2}, 4 * SlowPoll, true},
		{Load{Active: 1, IdleRuns: 500}, MaxIdlePoll, true},
		{Load{Active: 1, Waiting: 1, IdleRuns: 500}, SlowPoll, true},
	} {
		if d, on := Interval(c.load, FastPoll, SlowPoll); d != c.want || on != c.on {
			t.Errorf("%+v: %v %v", c.load, d, on)
		}
	}
	e := newEnv(t)
	e.svc.Adjust(ctx)
	if e.jobs.enabled == nil || *e.jobs.enabled {
		t.Fatal("empty queue disables polling")
	}
	e.grab(t, "aaa")
	if !*e.jobs.enabled || e.jobs.runs != 1 {
		t.Fatal("grab wakes polling")
	}
	e.svc.Adjust(ctx)
	if e.jobs.interval != SlowPoll {
		t.Fatalf("no UI: %v", e.jobs.interval)
	}
	e.ui = 1
	e.svc.Adjust(ctx)
	if e.jobs.interval != FastPoll {
		t.Fatalf("UI connected: %v", e.jobs.interval)
	}
}

func TestIdleDownloadsBackOffAndActivityResets(t *testing.T) {
	e := newEnv(t)
	e.ui = 1
	e.grab(t, "aaa")
	e.dl.set("aaa", func(s *plugin.Snapshot) { s.State = domain.StateStalled })
	for range IdleGrace + 3 {
		_ = e.svc.Poll(ctx)
	}
	if e.jobs.interval <= FastPoll {
		t.Fatalf("idle download keeps the fast cadence: %v", e.jobs.interval)
	}
	e.dl.set("aaa", func(s *plugin.Snapshot) { s.State, s.Progress = domain.StateDownloading, 0.4 })
	_ = e.svc.Poll(ctx)
	if e.jobs.interval != FastPoll {
		t.Fatalf("activity drops back to the fast cadence: %v", e.jobs.interval)
	}
	for range IdleGrace + 2 {
		_ = e.svc.Poll(ctx)
	}
	e.svc.Wake()
	e.svc.Adjust(ctx)
	if e.jobs.interval != FastPoll {
		t.Fatalf("a wake resets the back-off: %v", e.jobs.interval)
	}
}

func TestFinishedDownloadsMissingTheirImportAreReannounced(t *testing.T) {
	e := newEnv(t)
	d := e.grab(t, "aaa")
	e.dl.set("aaa", func(s *plugin.Snapshot) { s.State = domain.StateCompleted })
	_ = e.svc.Poll(ctx)
	if n := len(e.rec.Of(domain.EvDownloadCompleted)); n != 1 {
		t.Fatalf("completed once: %d", n)
	}
	if e.jobs.enabled != nil && !*e.jobs.enabled {
		t.Fatal("a finished download waiting for import keeps polling on")
	}
	_ = e.svc.Poll(ctx) // within the grace: the original event is still in flight
	if n := len(e.rec.Of(domain.EvDownloadCompleted)); n != 1 {
		t.Fatalf("re-announced too early: %d", n)
	}
	e.now = e.now.Add(ReannounceAfter + time.Second)
	_ = e.svc.Poll(ctx)
	done := e.rec.Of(domain.EvDownloadCompleted)
	if len(done) != 2 || done[1].Data.(domain.DownloadEventData).DownloadID != d.ID || done[1].Actor != "system" {
		t.Fatalf("missed import handed back: %+v", done)
	}
	// held or inactive-kind downloads are left alone and stop the polling
	_ = e.svc.Hold(ctx, d.ID, true)
	_ = e.svc.Poll(ctx)
	if n := len(e.rec.Of(domain.EvDownloadCompleted)); n != 2 || *e.jobs.enabled {
		t.Fatalf("held download re-announced (%d) or polling kept on", n)
	}
}

func TestShadowKindGetsNoClientWritesExceptSafety(t *testing.T) {
	e := newEnv(t)
	a, b := e.grab(t, "aaa"), e.grab(t, "bbb") // grabbed while active, then the kind goes to shadow
	mode := domain.ModeShadow
	e.svc.d.Modes = func(domain.MediaKind) domain.KindMode { return mode }
	ops := map[string]func() error{
		"pause":    func() error { return e.svc.Pause(ctx, a.ID) },
		"resume":   func() error { return e.svc.Resume(ctx, a.ID) },
		"priority": func() error { return e.svc.SetPriority(ctx, a.ID, plugin.PrioTop) },
		"remove":   func() error { return e.svc.Remove(ctx, a.ID, RemoveOptions{DeleteFiles: true}) },
	}
	for name, op := range ops { // a user's write in a shadow kind is recorded, not sent
		if err := op(); !errors.Is(err, ErrShadowed) {
			t.Errorf("%s in shadow: %v", name, err)
		}
	}
	if sh := e.rec.Of(domain.EvClientShadowed); len(sh) != len(ops) || sh[0].Data.(domain.ClientActionData).DownloadID != a.ID {
		t.Fatalf("would-be actions: %+v", sh)
	}
	mode = domain.ModeDisabled
	var me domain.ModeError
	for name, op := range ops { // a disabled kind refuses
		if err := op(); !errors.As(err, &me) || me.Mode != domain.ModeDisabled {
			t.Errorf("%s when disabled: %v", name, err)
		}
	}
	if len(e.dl.removed) != 0 || len(e.dl.ops) != 0 {
		t.Fatalf("client written in shadow: %v %v", e.dl.removed, e.dl.ops)
	}
	e.svc.OnPayloadRejected(ctx, domain.Event{Type: domain.EvPayloadRejected, Data: domain.PayloadEventData{DownloadID: b.ID, Files: []string{"x.scr"}}})
	if del, ok := e.dl.removed["bbb"]; !ok || !del {
		t.Fatal("the safety action runs whatever the mode")
	}
}

func TestHoldBlocksAutomationButNotSafety(t *testing.T) {
	e := newEnv(t)
	a, b := e.grab(t, "aaa"), e.grab(t, "bbb")
	if err := e.svc.Hold(ctx, a.ID, true); err != nil {
		t.Fatal(err)
	}
	// a rule (automation actor) can neither remove nor move a held download
	if err := e.svc.removeAs(ctx, a.ID, domain.ActorAutomation, "rule", "rule", RemoveOptions{}); !errors.Is(err, ErrHeld) {
		t.Fatalf("automation removed a held download: %v", err)
	}
	if err := e.svc.Transition(ctx, a.ID, domain.StateFailed, domain.ActorAutomation, "rule"); !errors.Is(err, ErrHeld) {
		t.Fatalf("automation failed a held download: %v", err)
	}
	if len(e.dl.removed) != 0 {
		t.Fatal("client written for a held download")
	}
	// moving a download into the hold category in the client holds it too
	e.dl.set("bbb", func(s *plugin.Snapshot) { s.Category = domain.HoldCategory })
	_ = e.svc.Poll(ctx)
	if got, _ := e.svc.Get(ctx, b.ID); !got.OnHold() || got.State == domain.StateRemoved {
		t.Fatalf("hold category: %+v", got)
	}
	// the safety action removes an unsafe payload even on hold
	e.svc.OnPayloadRejected(ctx, domain.Event{Type: domain.EvPayloadRejected, Data: domain.PayloadEventData{DownloadID: a.ID, Files: []string{"x.scr"}}})
	if del, ok := e.dl.removed["aaa"]; !ok || !del {
		t.Fatal("safety removes a held download with its files")
	}
	if got, _ := e.svc.Get(ctx, a.ID); got.State != domain.StateRemoved {
		t.Fatal(got.State)
	}
}

func TestReleasingACompletedDownloadHandsItToImport(t *testing.T) {
	e := newEnv(t)
	d := e.grab(t, "aaa")
	_ = e.svc.Hold(ctx, d.ID, true)
	e.dl.set("aaa", func(s *plugin.Snapshot) { s.State = domain.StateCompleted })
	_ = e.svc.Poll(ctx)
	done := e.rec.Of(domain.EvDownloadCompleted)
	if len(done) != 1 || !done[0].Data.(domain.DownloadEventData).Held {
		t.Fatalf("completed while held carries held: %+v", done)
	}
	if err := e.svc.Hold(ctx, d.ID, true); err != nil || len(e.rec.Of(domain.EvDownloadCompleted)) != 1 {
		t.Fatal("holding twice is a no-op", err)
	}
	if err := e.svc.Hold(ctx, d.ID, false); err != nil {
		t.Fatal(err)
	}
	done = e.rec.Of(domain.EvDownloadCompleted)
	if len(done) != 2 || done[1].Data.(domain.DownloadEventData).Held || done[1].Actor != "user" {
		t.Fatalf("release re-announces completion: %+v", done)
	}
	if err := e.svc.Hold(ctx, 99, true); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}
