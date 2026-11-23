package notify

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
	"github.com/jrand0m/corsarr/internal/testkit"
)

var ctx = context.Background()

type fakeNotifier struct {
	mu   sync.Mutex
	msgs []plugin.Message
	err  error
}

func (f *fakeNotifier) Test(context.Context) error { return nil }
func (f *fakeNotifier) Schemes() []string          { return []string{"fake"} }
func (f *fakeNotifier) Send(_ context.Context, _ domain.Event, m plugin.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.msgs = append(f.msgs, m)
	return nil
}
func (f *fakeNotifier) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.msgs) }

type fakeServer struct {
	mu    sync.Mutex
	calls [][]string
	err   error
}

func (f *fakeServer) Test(context.Context) error { return nil }
func (f *fakeServer) RefreshPaths(_ context.Context, p []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.calls = append(f.calls, append([]string(nil), p...))
	return nil
}

type env struct {
	svc   *Service
	st    *store.Store
	phone *fakeNotifier
	plex  *fakeServer
	conns []Connection
	now   time.Time
}

func newEnv(t *testing.T) *env {
	e := &env{st: testkit.Store(t), phone: &fakeNotifier{}, plex: &fakeServer{}, now: time.Now()}
	e.conns = []Connection{{Key: "phone", Impl: e.phone, Toggles: []string{"import"}}}
	e.svc = New(Deps{Outbox: e.st.Outbox(), Refreshes: e.st.Refreshes(), Connections: func() []Connection { return e.conns },
		MediaServers: func() []MediaServer { return []MediaServer{{Key: "plex", Impl: e.plex}} },
		Now:          func() time.Time { return e.now }, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	return e
}

var item = domain.EventItem{Ref: domain.ItemRef{Kind: domain.KindMovie, ID: 1}, Title: "Example Movie"}

func importEvent(id, outcome string) domain.Event {
	return domain.Event{EventID: id, Type: domain.EvImportCompleted, Items: []domain.EventItem{item}, Data: domain.ImportEventData{
		Outcome: domain.ImportOutcome{Status: domain.ImportImported, Items: []domain.ImportItem{{Item: item.Ref, Outcome: outcome,
			NewPaths: []string{"/m/Example Movie (2021)/Example Movie (2021).mkv"}}}},
		Folders: []string{"/m/Example Movie (2021)"}}}
}

func TestGroupsAndToggles(t *testing.T) {
	if g := Groups(importEvent("e", "upgraded")); len(g) != 2 || g[1] != "upgrade" {
		t.Fatal(g)
	}
	grab := domain.Event{Type: domain.EvGrabSent}
	if !Wants(nil, grab) || Wants([]string{"import"}, grab) || Wants(nil, domain.Event{Type: domain.EvItemAdded}) {
		t.Fatal("toggles")
	}
	if !Wants([]string{"upgrade"}, importEvent("e", "upgraded")) || Wants([]string{"upgrade"}, importEvent("e", "imported")) {
		t.Fatal("upgrade group")
	}
}

func TestRender(t *testing.T) {
	cases := []struct {
		ev          domain.Event
		title, body string
	}{
		{importEvent("e", "imported"), "Imported: Example Movie", "Example Movie (2021).mkv"},
		{importEvent("e", "upgraded"), "Upgraded: Example Movie", ""},
		{domain.Event{Type: domain.EvGrabSent, Items: []domain.EventItem{item}, Data: domain.GrabEventData{Release: domain.Release{Title: "R"},
			Decision: domain.Decision{Quality: "WEBDL-1080p"}, Downloader: "qb"}}, "Grabbed: Example Movie", "R (WEBDL-1080p) sent to qb."},
		{domain.Event{Type: domain.EvPayloadRejected, Items: []domain.EventItem{item}, Data: domain.PayloadEventData{Files: []string{"a.scr", "b.exe"}}},
			"Unsafe download removed", "a.scr, b.exe"},
		{domain.Event{Type: domain.EvDownloadFailed, Data: domain.DownloadEventData{Title: "T", Reason: "missingFiles"}}, "Download failed", "T: missingFiles"},
		{domain.Event{Type: domain.EvItemAdded, Items: []domain.EventItem{item}}, "item.added", "Example Movie"},
	}
	for _, c := range cases {
		title, body := Render(c.ev)
		if !strings.Contains(title, c.title) || !strings.Contains(body, c.body) {
			t.Errorf("%s: %q / %q", c.ev.Type, title, body)
		}
	}
}

func TestOneMessagePerEventAndRefreshMerged(t *testing.T) {
	e := newEnv(t)
	e.svc.OnEvent(ctx, importEvent("ev1", "imported"))
	e.svc.OnEvent(ctx, importEvent("ev1", "imported")) // duplicate delivery of the same event
	e.svc.OnEvent(ctx, domain.Event{EventID: "ev2", Type: domain.EvGrabSent, Items: []domain.EventItem{item}, Data: domain.GrabEventData{}})
	e.svc.OnEvent(ctx, domain.Event{EventID: "ev3", Type: domain.EvFileDeleted, Data: domain.FileEventData{Path: "/m/Other (2020)/o.mkv"}})
	next := e.svc.Flush(ctx)
	if e.phone.count() != 1 || e.phone.msgs[0].Title != "Imported: Example Movie" {
		t.Fatalf("messages %+v", e.phone.msgs)
	}
	if len(e.plex.calls) != 0 || next.IsZero() {
		t.Fatal("refresh waits for the merge window")
	}
	e.now = e.now.Add(MergeWindow + time.Second)
	if next := e.svc.Flush(ctx); !next.IsZero() {
		t.Fatalf("nothing left: %v", next)
	}
	if len(e.plex.calls) != 1 {
		t.Fatalf("one merged refresh call: %v", e.plex.calls)
	}
	got := e.plex.calls[0]
	sort.Strings(got)
	if len(got) != 2 || got[0] != "/m/Example Movie (2021)" || got[1] != "/m/Other (2020)" {
		t.Fatalf("%v", got)
	}
}

func TestRetriesWithBackoffThenGivesUp(t *testing.T) {
	e := newEnv(t)
	e.phone.err = plugin.Errorf(plugin.ClassUnreachable, "down")
	e.svc.OnEvent(ctx, importEvent("ev1", "imported"))
	for i := 0; i < MaxAttempts; i++ {
		e.svc.Flush(ctx)
		e.now = e.now.Add(MaxBackoff + time.Minute)
	}
	failed, _ := e.st.Outbox().ByStatus(ctx, "failed")
	if len(failed) != 1 || failed[0].Attempts != MaxAttempts || failed[0].LastError == "" {
		t.Fatalf("%+v", failed)
	}
	e.phone.err = nil
	e.svc.OnEvent(ctx, importEvent("ev2", "imported"))
	e.conns = nil // connection removed before delivery
	e.svc.Flush(ctx)
	if failed, _ := e.st.Outbox().ByStatus(ctx, "failed"); len(failed) != 2 {
		t.Fatal("removed connection fails the delivery")
	}
}

func TestBackoff(t *testing.T) {
	half := func(n int64) int64 { return n / 2 }
	if d := Backoff(time.Second, 1, half); d < 500*time.Millisecond || d > time.Second {
		t.Fatal(d)
	}
	if d := Backoff(time.Second, 40, half); d > MaxBackoff {
		t.Fatal(d)
	}
}

func TestRefreshFailureRetriesAndUnknownServerDrops(t *testing.T) {
	e := newEnv(t)
	e.plex.err = errors.New("plex down")
	e.svc.OnEvent(ctx, importEvent("ev1", "imported"))
	e.now = e.now.Add(MergeWindow + time.Second)
	e.svc.Flush(ctx)
	if next, ok, _ := e.st.Refreshes().NextDue(ctx); !ok || !next.After(e.now) {
		t.Fatal("retry postponed")
	}
	_ = e.st.Refreshes().Add(ctx, "gone", "/m/x", e.now)
	e.svc.Flush(ctx)
	if due, _ := e.st.Refreshes().Due(ctx, e.now.Add(time.Hour*3)); len(due["gone"]) != 0 {
		t.Fatal("unknown server's refresh dropped")
	}
	if RefreshFolders(domain.Event{Type: domain.EvImportBlocked, Data: domain.ImportEventData{Folders: []string{"x"}}}) != nil {
		t.Fatal("blocked imports refresh nothing")
	}
	moved := RefreshFolders(domain.Event{Type: domain.EvFileRenamed, Data: domain.FileEventData{Path: "/b/m/x.mkv", OldPath: "/a/m/x.mkv"}})
	if len(moved) != 2 || moved[0] != "/b/m" || moved[1] != "/a/m" {
		t.Fatalf("a moved folder refreshes old and new: %v", moved)
	}
	if renamed := RefreshFolders(domain.Event{Data: domain.FileEventData{Path: "/b/m/y.mkv", OldPath: "/b/m/x.mkv"}}); len(renamed) != 1 {
		t.Fatalf("a rename in place refreshes one folder: %v", renamed)
	}
}

func TestRunDeliversOnWake(t *testing.T) {
	e := newEnv(t)
	e.svc.d.Now = time.Now
	c, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { e.svc.Run(c); close(done) }()
	e.svc.OnEvent(ctx, importEvent("ev1", "imported"))
	testkit.Eventually(t, 2*time.Second, "delivery", func() bool { return e.phone.count() == 1 })
	cancel()
	<-done
	if !earliest(time.Time{}, false, time.Time{}, false).IsZero() {
		t.Fatal("no times")
	}
	a, b := time.Now(), time.Now().Add(time.Second)
	if !earliest(a, true, b, true).Equal(a) || !earliest(b, true, a, true).Equal(a) || !earliest(a, false, b, true).Equal(b) {
		t.Fatal("earliest")
	}
}

func TestEventsOfOneDownloadBecomeOneMessage(t *testing.T) {
	e := newEnv(t)
	e.conns = []Connection{{Key: "phone", Impl: e.phone, Toggles: []string{"grab", "import"}}}
	grab := domain.Event{EventID: "g", Type: domain.EvGrabSent, Items: []domain.EventItem{item},
		Data: domain.GrabEventData{DownloadID: 7, Release: domain.Release{Title: "R"}, Downloader: "qb"}}
	imp := importEvent("i", "imported")
	d := imp.Data.(domain.ImportEventData)
	d.Outcome.DownloadID = 7
	imp.Data = d
	e.svc.OnEvent(ctx, grab)
	e.svc.Flush(ctx)
	if e.phone.count() != 0 {
		t.Fatal("a download's first event waits for the merge window")
	}
	e.now = e.now.Add(MergeWindow / 2)
	e.svc.OnEvent(ctx, imp) // inside the window
	e.now = e.now.Add(MergeWindow/2 + time.Millisecond)
	e.svc.Flush(ctx)
	if e.phone.count() != 1 || e.phone.msgs[0].Title != "Imported: Example Movie" || !strings.Contains(e.phone.msgs[0].Body, "R (") ||
		!strings.Contains(e.phone.msgs[0].Body, "Example Movie (2021).mkv") {
		t.Fatalf("one merged message: %+v", e.phone.msgs)
	}
	e.now = e.now.Add(2 * MergeWindow)
	if e.svc.Flush(ctx); e.phone.count() != 1 {
		t.Fatal("the import went out with the grab")
	}
	sent, _ := e.st.Outbox().ByStatus(ctx, "sent")
	if len(sent) != 2 {
		t.Fatalf("both deliveries are marked sent: %d", len(sent))
	}
	// a later event of the same download is its own message
	late := grab
	late.EventID, late.Type = "f", domain.EvDownloadFailed
	late.Data = domain.DownloadEventData{DownloadID: 7, Title: "R", Reason: "gone"}
	e.conns[0].Toggles = append(e.conns[0].Toggles, "downloadFailed")
	e.svc.OnEvent(ctx, late)
	e.now = e.now.Add(MergeWindow + time.Millisecond)
	if e.svc.Flush(ctx); e.phone.count() != 2 {
		t.Fatalf("after the window: %d", e.phone.count())
	}
	for _, c := range []struct {
		ev   domain.Event
		want string
	}{
		{domain.Event{Data: domain.PayloadEventData{DownloadID: 3}}, "download:3"},
		{domain.Event{Data: domain.GrabEventData{}}, ""},
		{domain.Event{Type: domain.EvHealthChanged}, ""},
	} {
		if got := BatchKey(c.ev); got != c.want {
			t.Errorf("%+v: %q", c.ev, got)
		}
	}
}

func TestConnectionTemplateOverrideAndTest(t *testing.T) {
	e := newEnv(t)
	tpl, err := ParseTemplate("[corsarr] {{.Title}}", "{{.Event}} of {{.Items}}: {{.Body}}")
	if err != nil {
		t.Fatal(err)
	}
	e.conns = []Connection{{Key: "phone", Impl: e.phone, Toggles: []string{"import"}, Template: tpl},
		{Key: "plain", Impl: &fakeNotifier{}, Toggles: []string{"import"}}}
	e.svc.OnEvent(ctx, importEvent("ev1", "imported"))
	e.svc.Flush(ctx)
	m := e.phone.msgs[0]
	if m.Title != "[corsarr] Imported: Example Movie" || !strings.HasPrefix(m.Body, "import.completed of Example Movie: /m/") {
		t.Fatalf("%+v", m)
	}
	if title, _ := RenderWith(Template{}, importEvent("x", "imported")); title != "Imported: Example Movie" {
		t.Fatal("no override keeps the built-in text")
	}
	if _, err := ParseTemplate("{{.Title", ""); err == nil {
		t.Fatal("bad title")
	}
	if _, err := ParseTemplate("", "{{end}}"); err == nil {
		t.Fatal("bad body")
	}
	srv := testkit.Serve(t, func(mx testkit.Mux) { e.svc.Mount(mx) })
	code, body := testkit.Do(t, srv, "POST", "/api/v1/notifications/phone/test?send=false", "")
	if code != 200 || !strings.Contains(body, `"title":"[corsarr] Imported: Example Movie"`) || !strings.Contains(body, `"sent":false`) || e.phone.count() != 1 {
		t.Fatalf("preview: %d %s", code, body)
	}
	if code, body := testkit.Do(t, srv, "POST", "/api/v1/notifications/phone/test", ""); code != 200 || !strings.Contains(body, `"sent":true`) || e.phone.count() != 2 {
		t.Fatalf("test send: %d %s", code, body)
	}
	e.phone.err = errors.New("unreachable")
	if _, body := testkit.Do(t, srv, "POST", "/api/v1/notifications/phone/test", ""); !strings.Contains(body, `"error":"unreachable"`) {
		t.Fatal(body)
	}
	if code, _ := testkit.Do(t, srv, "POST", "/api/v1/notifications/nope/test", ""); code != 404 {
		t.Fatal(code)
	}
}

func TestRefreshRetriesStopAfterTheCap(t *testing.T) {
	e := newEnv(t)
	reg, rec := health.New(nil, nil, nil), &testkit.Recorder{}
	e.svc.d.Health, e.svc.d.Pub = reg, rec
	e.plex.err = plugin.Errorf(plugin.ClassUnreachable, "plex offline")
	e.svc.OnEvent(ctx, importEvent("ev1", "imported"))
	var waits []time.Duration
	for i := 0; i < MaxRefreshAttempts; i++ {
		e.now = e.now.Add(MaxBackoff + time.Minute)
		e.svc.Flush(ctx)
		if next, ok, _ := e.st.Refreshes().NextDue(ctx); ok {
			waits = append(waits, next.Sub(e.now))
		}
	}
	if len(waits) != MaxRefreshAttempts-1 || waits[len(waits)-1] <= waits[0]/2 {
		t.Fatalf("retries back off, then stop: %v", waits)
	}
	if _, ok, _ := e.st.Refreshes().NextDue(ctx); ok {
		t.Fatal("refresh dropped after the cap")
	}
	h, _ := reg.Get(domain.IntegrationRef{Type: "mediaserver", Key: "plex"})
	if h.Status == domain.HealthOK || h.LastErrorClass != "unreachable" || !strings.Contains(h.LastErrorMessage, "gave up after 5 attempts") {
		t.Fatalf("server marked unhealthy: %+v", h)
	}
	ab := rec.Of(domain.EvRefreshAbandoned)
	if len(ab) != 1 || ab[0].Data.(domain.RefreshEventData).Attempts != MaxRefreshAttempts || ab[0].Data.(domain.RefreshEventData).Paths[0] != "/m/Example Movie (2021)" {
		t.Fatalf("history event: %+v", ab)
	}
	if !domain.EvRefreshAbandoned.Persisted() || len(Groups(ab[0])) != 1 {
		t.Fatal("kept in History and notifiable as health")
	}
	if failureClass(errors.New("plain")) != health.ClassUnreachable {
		t.Fatal("default class")
	}
}
