package store

import (
	"context"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

var ctx = context.Background()

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestMoviesRoundTripAndIdempotentAdd(t *testing.T) {
	r := open(t, t.TempDir()).Movies()
	cin := time.Date(2021, 5, 1, 0, 0, 0, 0, time.UTC)
	m := domain.Movie{TMDBID: 42, IMDBID: "tt42", Title: "Example", Year: 2021, AltTitles: []string{"Ejemplo"}, RuntimeMin: 100,
		InCinemas: &cin, Monitored: true, MinAvailability: domain.AvailReleased, ProfileKey: "any", RootFolder: "/m",
		Path: "/m/Example (2021)", AddedAt: cin}
	got, created, err := r.Add(ctx, m)
	must(t, err)
	if !created || got.ID == 0 || got.AltTitles[0] != "Ejemplo" || !got.InCinemas.Equal(cin) || got.Digital != nil {
		t.Fatalf("%+v", got)
	}
	again, created, err := r.Add(ctx, m)
	if err != nil || created || again.ID != got.ID {
		t.Fatalf("second add: %+v %v %v", again, created, err)
	}
	got.Monitored, got.ProfileKey = false, "hd1080p"
	must(t, r.Update(ctx, got))
	must(t, r.SetFile(ctx, got.ID, 9))
	back, ok, err := r.Get(ctx, got.ID)
	if err != nil || !ok || back.Monitored || back.ProfileKey != "hd1080p" || back.FileID != 9 {
		t.Fatalf("%+v %v %v", back, ok, err)
	}
	if _, ok, _ := r.Get(ctx, 999); ok {
		t.Fatal("missing movie found")
	}
}

func TestMoviesListEachPending(t *testing.T) {
	r := open(t, t.TempDir()).Movies()
	for i := int64(1); i <= 3; i++ {
		_, _, err := r.Add(ctx, domain.Movie{TMDBID: i, Title: "M", Monitored: i != 2, MinAvailability: domain.AvailReleased, ProfileKey: "any", RootFolder: "/m", Path: "/m/x"})
		must(t, err)
	}
	page, next, err := r.List(ctx, 0, 2)
	if err != nil || len(page) != 2 || next != page[1].ID {
		t.Fatalf("%v %d %v", page, next, err)
	}
	page, next, _ = r.List(ctx, next, 2)
	if len(page) != 1 || next != 0 {
		t.Fatalf("last page %v %d", page, next)
	}
	n := 0
	must(t, r.Each(ctx, func(domain.Movie) error { n++; return nil }))
	must(t, r.SetAvailablePublished(ctx, 1, true))
	var pending []int64
	must(t, r.PendingAvailability(ctx, func(m domain.Movie) error { pending = append(pending, m.ID); return nil }))
	if n != 3 || len(pending) != 1 || pending[0] != 3 {
		t.Fatalf("each %d pending %v", n, pending)
	}
	if err := r.Each(ctx, func(domain.Movie) error { return context.Canceled }); err != context.Canceled {
		t.Fatalf("callback error must stop: %v", err)
	}
	must(t, r.Delete(ctx, 1))
	if _, ok, _ := r.Get(ctx, 1); ok {
		t.Fatal("deleted")
	}
}

func newGrab(item int64, hash string) (domain.Grab, domain.Download) {
	ref := domain.ItemRef{Kind: domain.KindMovie, ID: item}
	g := domain.Grab{Kind: domain.KindMovie, ItemRefs: []domain.ItemRef{ref}, Origin: domain.OriginAutomatic, Downloader: "qb",
		DownloadID: hash, SentAt: time.Now(),
		Release:  domain.Release{ID: "r1", Title: "Example.2021.1080p", GUID: "g1", InfoHash: hash, DownloadURL: "magnet:?xt=urn:btih:" + hash},
		Decision: domain.Decision{Accepted: true, Quality: "WEBDL-1080p"}}
	d := domain.Download{Downloader: "qb", DownloadID: hash, Protocol: domain.ProtoTorrent, Kind: domain.KindMovie,
		Title: g.Release.Title, State: domain.StateQueued, Category: "movies", AddedAt: time.Now(),
		Links: []domain.DownloadLink{{Item: ref, Status: domain.LinkPending}}}
	return g, d
}

func TestQueueGrabDownloadLifecycle(t *testing.T) {
	s := open(t, t.TempDir())
	r := s.Queue()
	g, d := newGrab(1, "abc")
	g, sd, err := r.CreateGrab(ctx, g, d)
	must(t, err)
	if g.ID == 0 || sd.ID == 0 || sd.GrabID != g.ID {
		t.Fatalf("%+v %+v", g, sd)
	}
	g2, sd2, err := r.CreateGrab(ctx, g, d)
	if err != nil || g2.ID != g.ID || sd2.ID != sd.ID {
		t.Fatalf("retry must return the live download: %+v %+v %v", g2, sd2, err)
	}
	back, ok, err := r.Grab(ctx, g.ID)
	if err != nil || !ok || back.Release.DownloadURL != "magnet:?xt=urn:btih:abc" || back.ItemRefs[0].ID != 1 || !back.Decision.Accepted {
		t.Fatalf("%+v %v %v", back, ok, err)
	}
	if _, ok, _ := r.Grab(ctx, 99); ok {
		t.Fatal("missing grab")
	}
	sd.State, sd.Progress, sd.ContentPath = domain.StateCompleted, 1, "/dl/x"
	sd.Links[0].Status = domain.LinkImported
	must(t, r.Save(ctx, sd))
	got, ok, err := r.Download(ctx, sd.ID)
	if err != nil || !ok || got.State != domain.StateCompleted || got.ContentPath != "/dl/x" || got.Links[0].Status != domain.LinkImported {
		t.Fatalf("%+v %v", got, err)
	}
	if _, ok, _ := r.Download(ctx, 99); ok {
		t.Fatal("missing download")
	}
	list, err := r.Downloads(ctx, domain.StateCompleted)
	if err != nil || len(list) != 1 {
		t.Fatalf("%v %v", list, err)
	}
	if none, _ := r.Downloads(ctx); len(none) != 0 {
		t.Fatal("no states, no rows")
	}
	if c, _ := r.PayloadChecked(ctx, sd.ID); c {
		t.Fatal("not checked yet")
	}
	must(t, r.SetPayloadChecked(ctx, sd.ID))
	if c, _ := r.PayloadChecked(ctx, sd.ID); !c {
		t.Fatal("checked")
	}
}

func TestQueueLookups(t *testing.T) {
	r := open(t, t.TempDir()).Queue()
	item := domain.ItemRef{Kind: domain.KindMovie, ID: 1}
	g, d := newGrab(1, "abc")
	_, sd, err := r.CreateGrab(ctx, g, d)
	must(t, err)
	qs, err := r.QueuedQualities(ctx, item)
	if err != nil || len(qs) != 1 || qs[0] != "WEBDL-1080p" {
		t.Fatalf("%v %v", qs, err)
	}
	for _, id := range []domain.ReleaseIdentity{{GUID: "g1"}, {InfoHash: "abc"}, {Title: "Example.2021.1080p"}} {
		if ok, err := r.GrabbedBefore(ctx, item, id); err != nil || !ok {
			t.Errorf("%+v not found: %v", id, err)
		}
	}
	if ok, _ := r.GrabbedBefore(ctx, item, domain.ReleaseIdentity{Title: "other"}); ok {
		t.Error("other release")
	}
	sd.State = domain.StateRemoved
	must(t, r.Save(ctx, sd))
	if qs, _ := r.QueuedQualities(ctx, item); len(qs) != 0 {
		t.Fatal("removed downloads are not queued")
	}
	// A removed download frees its client id for a new grab.
	_, sd2, err := r.CreateGrab(ctx, g, d)
	if err != nil || sd2.ID == sd.ID {
		t.Fatalf("regrab %+v %v", sd2, err)
	}
	page, next, err := r.List(ctx, 0, 1)
	if err != nil || len(page) != 1 || page[0].ID != sd2.ID || next != sd2.ID {
		t.Fatalf("%v %d %v", page, next, err)
	}
	if page, _, _ := r.List(ctx, next, 1); len(page) != 0 {
		t.Fatal("removed download hidden from the queue")
	}
}

func TestLibraryFilesAndImportRecords(t *testing.T) {
	r := open(t, t.TempDir()).Library()
	f, err := r.SaveFile(ctx, domain.LibraryFile{Item: domain.ItemRef{Kind: domain.KindMovie, ID: 1}, Path: "/m/a.mkv", Size: 5, Quality: "webdl-1080p", Revision: 1})
	must(t, err)
	f2, err := r.SaveFile(ctx, domain.LibraryFile{Item: f.Item, Path: "/m/a.mkv", Size: 6, Quality: "bluray-1080p"})
	if err != nil || f2.ID != f.ID {
		t.Fatalf("upsert by path: %v %v", f2, err)
	}
	got, ok, err := r.File(ctx, f.ID)
	if err != nil || !ok || got.Size != 6 || got.Quality != "bluray-1080p" || got.Item.Kind != domain.KindMovie {
		t.Fatalf("%+v", got)
	}
	if list, err := r.FilesOf(ctx, f.Item); err != nil || len(list) != 1 || list[0].ID != f.ID {
		t.Fatalf("files of item: %v %v", list, err)
	}
	must(t, r.DeleteFile(ctx, f.ID))
	if _, ok, _ := r.File(ctx, f.ID); ok {
		t.Fatal("deleted")
	}
	if _, _, ok, _ := r.ImportRecord(ctx, 1, "/dl/a.mkv"); ok {
		t.Fatal("no record yet")
	}
	must(t, r.RecordImport(ctx, 1, "/dl/a.mkv", "/m/a.mkv", 3))
	must(t, r.RecordImport(ctx, 1, "/dl/a.mkv", "/m/other.mkv", 4))
	dest, id, ok, err := r.ImportRecord(ctx, 1, "/dl/a.mkv")
	if err != nil || !ok || dest != "/m/a.mkv" || id != 3 {
		t.Fatalf("first record wins: %s %d %v", dest, id, err)
	}
}

func TestBlocklistAndCursors(t *testing.T) {
	s := open(t, t.TempDir())
	r := s.Blocklist()
	item := domain.ItemRef{Kind: domain.KindMovie, ID: 1}
	e := domain.BlocklistEntry{Item: item, Release: domain.ReleaseIdentity{Indexer: "ix", GUID: "g", InfoHash: "h", Title: "T"}, Source: "payload", AddedAt: time.Now()}
	got, created, err := r.Add(ctx, e)
	if err != nil || !created || got.ID == 0 || got.Release.Indexer != "ix" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, created, _ := r.Add(ctx, e); created {
		t.Fatal("duplicate created")
	}
	for _, id := range []domain.ReleaseIdentity{{GUID: "g"}, {InfoHash: "h"}, {Title: "T"}} {
		if ok, _ := r.Has(ctx, item, id); !ok {
			t.Errorf("%+v", id)
		}
	}
	if ok, _ := r.Has(ctx, domain.ItemRef{Kind: domain.KindMovie, ID: 2}, domain.ReleaseIdentity{Title: "T"}); ok {
		t.Error("other item")
	}
	list, next, err := r.List(ctx, 0, 1)
	if err != nil || len(list) != 1 || next != list[0].ID {
		t.Fatalf("%v %d %v", list, next, err)
	}
	removed, ok, err := r.Remove(ctx, got.ID)
	if err != nil || !ok || removed.Release.Title != "T" {
		t.Fatalf("%+v %v", removed, err)
	}
	if _, ok, _ := r.Remove(ctx, got.ID); ok {
		t.Fatal("removed twice")
	}
	c := s.FeedCursors()
	if cur, err := c.Get(ctx, "ix", domain.KindMovie); err != nil || cur != "" {
		t.Fatalf("%q %v", cur, err)
	}
	must(t, c.Set(ctx, "ix", domain.KindMovie, "a"))
	must(t, c.Set(ctx, "ix", domain.KindMovie, "b"))
	if cur, _ := c.Get(ctx, "ix", domain.KindMovie); cur != "b" {
		t.Fatalf("%q", cur)
	}
}

func TestRulesRunsAndActions(t *testing.T) {
	r := open(t, t.TempDir()).Rules()
	at := time.Now().Add(-time.Hour)
	run, err := r.StartRun(ctx, "recyclepurge", "live", at)
	must(t, err)
	id, err := r.AddAction(ctx, RuleAction{RunID: run, RuleID: "recyclepurge", Mode: "live", Candidate: "/r/a", Classification: "would-act",
		Evidence: map[string]any{"age": "8d"}, Ops: []string{"delete"}, Status: "planned", CreatedAt: at})
	must(t, err)
	a, ok, err := r.Action(ctx, id)
	if err != nil || !ok || a.Evidence["age"] != "8d" || a.Ops[0] != "delete" || a.UndoAvailable {
		t.Fatalf("%+v %v", a, err)
	}
	acted := time.Now()
	a.Status, a.Classification, a.Undo, a.ActedAt = "acted", "acted", `{"from":"x"}`, &acted
	must(t, r.FinishAction(ctx, a))
	a, _, _ = r.Action(ctx, id)
	if !a.UndoAvailable || a.ActedAt == nil {
		t.Fatalf("%+v", a)
	}
	must(t, r.FinishRun(ctx, run, map[string]int{"acted": 1}, time.Now()))
	list, next, err := r.Actions(ctx, 0, 5)
	if err != nil || len(list) != 1 || next != 0 {
		t.Fatalf("%v %d %v", list, next, err)
	}
	var old []int64
	must(t, r.ActedBefore(ctx, "recyclepurge", time.Now(), func(a RuleAction) error { old = append(old, a.ID); return nil }))
	if len(old) != 1 {
		t.Fatalf("%v", old)
	}
	if err := r.ActedBefore(ctx, "recyclepurge", time.Now(), func(RuleAction) error { return context.Canceled }); err != context.Canceled {
		t.Fatal(err)
	}
	if _, ok, _ := r.Action(ctx, 99); ok {
		t.Fatal("missing")
	}
}

func TestOutboxAndRefreshes(t *testing.T) {
	s := open(t, t.TempDir())
	o := s.Outbox()
	t0 := time.Now().Add(-time.Second)
	if _, ok, err := o.NextDue(ctx); ok || err != nil {
		t.Fatal("empty outbox has nothing due")
	}
	e := OutboxEntry{EventID: "e1", Connection: "phone", EventType: "grab.sent", Title: "t", Body: "b", Envelope: "{}", NextAt: t0}
	if ok, err := o.Enqueue(ctx, e); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if ok, _ := o.Enqueue(ctx, e); ok {
		t.Fatal("duplicate delivery")
	}
	due, err := o.Due(ctx, time.Now(), 10)
	if err != nil || len(due) != 1 || due[0].Status != "pending" {
		t.Fatalf("%v %v", due, err)
	}
	if next, ok, _ := o.NextDue(ctx); !ok || next.After(time.Now()) {
		t.Fatal("next due")
	}
	due[0].Status, due[0].Attempts = "sent", 1
	must(t, o.Finish(ctx, due[0]))
	if sent, _ := o.ByStatus(ctx, "sent"); len(sent) != 1 {
		t.Fatal("sent")
	}
	r := s.Refreshes()
	must(t, r.Add(ctx, "plex", "/m/A", t0))
	must(t, r.Add(ctx, "plex", "/m/A", t0))
	must(t, r.Add(ctx, "plex", "/m/B", t0))
	got, err := r.Due(ctx, time.Now())
	if err != nil || len(got["plex"]) != 2 {
		t.Fatalf("%v %v", got, err)
	}
	must(t, r.Retry(ctx, "plex", []string{"/m/B"}, time.Now().Add(time.Hour)))
	must(t, r.Retry(ctx, "plex", []string{"/m/B"}, time.Now().Add(time.Hour)))
	if n, err := r.Attempts(ctx, "plex", []string{"/m/A", "/m/B", "/m/gone"}); err != nil || n != 2 {
		t.Fatalf("attempts %d %v", n, err)
	}
	must(t, r.Done(ctx, "plex", []string{"/m/A"}))
	if got, _ := r.Due(ctx, time.Now()); len(got) != 0 {
		t.Fatalf("%v", got)
	}
	if next, ok, _ := r.NextDue(ctx); !ok || next.Before(time.Now()) {
		t.Fatal("retry postponed")
	}
}
