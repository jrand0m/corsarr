package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/conformance"
)

type captured struct {
	method, path, query string
	header              http.Header
	body                []byte
}

type sink struct {
	srv    *httptest.Server
	mu     sync.Mutex
	got    []captured
	status int
	reply  string
}

func newSink(t *testing.T) *sink {
	t.Helper()
	s := &sink{status: 200}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.got = append(s.got, captured{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), b})
		status, reply := s.status, s.reply
		s.mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *sink) host() string { return strings.TrimPrefix(s.srv.URL, "http://") }

func (s *sink) last() captured {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.got[len(s.got)-1]
}

func (s *sink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.got)
}

func nativeFor(t *testing.T, urls ...string) *Native {
	t.Helper()
	asAny := make([]any, len(urls))
	for i, u := range urls {
		asAny[i] = u
	}
	p, err := NewNative(plugin.Instance{Key: "n", Settings: plugin.Settings{"urls": asAny}, Env: conformance.Env(http.DefaultClient)})
	if err != nil {
		t.Fatal(err)
	}
	return p.(*Native)
}

func sampleEvent() domain.Event {
	return domain.Event{SchemaVersion: 1, EventID: "e1", Type: domain.EvImportCompleted,
		OccurredAt: time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC), Actor: "system", Kind: domain.KindMovie}
}

var sampleMsg = plugin.Message{Title: "Imported", Body: "Some Movie (2021)"}

func TestNativeConformance(t *testing.T) {
	reg, ok := plugin.Default.Get(NativeID)
	if !ok {
		t.Fatal("native not registered")
	}
	conformance.Manifest(t, reg.Manifest, plugin.KindNotifier)
	s := newSink(t)
	conformance.Notifier(t, nativeFor(t, "ntfy://"+s.host()+"/topic?scheme=http"), sampleEvent(), sampleMsg)
	conformance.Notifier(t, nativeFor(t, "generic+http://"+s.host()+"/hook"), sampleEvent(), sampleMsg)
}

func TestNativeErrorMapping(t *testing.T) {
	for _, mk := range []func(host string) string{
		func(h string) string { return "ntfy://" + h + "/t?scheme=http" },
		func(h string) string { return "generic+http://" + h + "/hook" },
	} {
		conformance.ErrorMapping(t, func(u string) plugin.Tester {
			return nativeFor(t, mk(strings.TrimPrefix(u, "http://")))
		})
	}
}

func TestNtfyRequest(t *testing.T) {
	s := newSink(t)
	n := nativeFor(t, "ntfy://alice:pw@"+s.host()+"/my%20topic?scheme=http&priority=2&tags=movie,new")
	if err := n.Send(t.Context(), sampleEvent(), sampleMsg); err != nil {
		t.Fatal(err)
	}
	c := s.last()
	user, pass, _ := (&http.Request{Header: c.header}).BasicAuth()
	if c.method != "POST" || c.path != "/my topic" || string(c.body) != "Some Movie (2021)" || c.header.Get("Title") != "Imported" ||
		c.header.Get("Priority") != "2" || c.header.Get("Tags") != "movie,new" || user != "alice" || pass != "pw" {
		t.Errorf("%+v", c)
	}
	if err := n.Send(t.Context(), sampleEvent(), plugin.Message{Body: "x", Priority: 5}); err != nil {
		t.Fatal(err)
	}
	if c := s.last(); c.header.Get("Priority") != "5" || c.header.Get("Title") != "" {
		t.Errorf("message priority wins, no title: %+v", c.header)
	}
}

func TestNtfyTokenAuth(t *testing.T) {
	s := newSink(t)
	n := nativeFor(t, "ntfy://"+s.host()+"/t?scheme=http&token=tk_abc")
	if err := n.Send(t.Context(), sampleEvent(), sampleMsg); err != nil {
		t.Fatal(err)
	}
	if c := s.last(); c.header.Get("Authorization") != "Bearer tk_abc" || c.header.Get("Priority") != "" {
		t.Errorf("%+v", c.header)
	}
}

func TestPushoverRequest(t *testing.T) {
	s := newSink(t)
	s.reply = `{"status":1,"request":"abc"}`
	n := nativeFor(t, "pushover://shoutrrr:APPTOKEN@USERKEY/?devices=phone,tablet&priority=1")
	n.targets[0].(*pushoverTarget).endpoint = s.srv.URL + "/1/messages.json"
	if err := n.Send(t.Context(), sampleEvent(), sampleMsg); err != nil {
		t.Fatal(err)
	}
	c := s.last()
	form, _ := url.ParseQuery(string(c.body))
	if c.path != "/1/messages.json" || form.Get("token") != "APPTOKEN" || form.Get("user") != "USERKEY" || form.Get("device") != "phone,tablet" ||
		form.Get("message") != "Some Movie (2021)" || form.Get("title") != "Imported" || form.Get("priority") != "1" {
		t.Errorf("%+v %v", c, form)
	}
	if err := n.Send(t.Context(), sampleEvent(), plugin.Message{Body: "b", Priority: 1}); err != nil {
		t.Fatal(err)
	}
	if form, _ := url.ParseQuery(string(s.last().body)); form.Get("priority") != "-2" || form.Get("title") != "" {
		t.Errorf("priority scale %v", form)
	}
}

func TestPushoverErrors(t *testing.T) {
	cases := []struct {
		status int
		reply  string
		want   plugin.FailureClass
	}{
		{400, `{"status":0,"errors":["application token is invalid"]}`, plugin.ClassAuth},
		{400, `{"status":0,"errors":["user identifier is invalid"]}`, plugin.ClassAuth},
		{400, `{"status":0,"errors":["message cannot be blank"]}`, plugin.ClassServerError},
		{429, ``, plugin.ClassRateLimited},
		{500, ``, plugin.ClassServerError},
		{200, `{"status":0}`, plugin.ClassServerError},
		{200, `not json`, plugin.ClassServerError},
	}
	for _, c := range cases {
		s := newSink(t)
		s.status, s.reply = c.status, c.reply
		n := nativeFor(t, "pushover://x:T@U/")
		n.targets[0].(*pushoverTarget).endpoint = s.srv.URL
		if err := n.Send(t.Context(), sampleEvent(), sampleMsg); plugin.ClassOf(err) != c.want {
			t.Errorf("%d %s: %v", c.status, c.reply, err)
		}
	}
}

func TestWebhookEnvelopeAndSignature(t *testing.T) {
	s := newSink(t)
	n := nativeFor(t, "generic+http://"+s.host()+"/hook/path?secret=s3cret&keep=1")
	if err := n.Send(t.Context(), sampleEvent(), plugin.Message{Title: "T", Body: "B", Priority: 4}); err != nil {
		t.Fatal(err)
	}
	c := s.last()
	if c.path != "/hook/path" || c.query != "keep=1" || c.header.Get("Content-Type") != "application/json" {
		t.Errorf("secret options must be removed from the called URL: %+v", c)
	}
	var env Envelope
	if err := json.Unmarshal(c.body, &env); err != nil || env.SchemaVersion != 1 || env.Title != "T" || env.Body != "B" ||
		env.Priority != 4 || env.Event.EventID != "e1" || env.Event.Type != domain.EvImportCompleted {
		t.Fatalf("%v %+v", err, env)
	}
	if got := c.header.Get(DefaultSignatureHeader); got != Sign("s3cret", c.body) || !strings.HasPrefix(got, "sha256=") || len(got) != 7+64 {
		t.Errorf("signature %q", got)
	}
}

func TestWebhookWithoutSecretAndCustomHeader(t *testing.T) {
	s := newSink(t)
	n := nativeFor(t, "generic+http://"+s.host()+"/h")
	if err := n.Send(t.Context(), sampleEvent(), sampleMsg); err != nil {
		t.Fatal(err)
	}
	if s.last().header.Get(DefaultSignatureHeader) != "" {
		t.Error("no secret, no signature")
	}
	n = nativeFor(t, "generic+http://"+s.host()+"/h?secret=k&signatureHeader=X-Sig")
	if err := n.Send(t.Context(), sampleEvent(), sampleMsg); err != nil {
		t.Fatal(err)
	}
	if c := s.last(); c.header.Get("X-Sig") != Sign("k", c.body) {
		t.Error("custom header")
	}
	// Known test vector: HMAC-SHA256("key", "The quick brown fox jumps over the lazy dog").
	if got := Sign("key", []byte("The quick brown fox jumps over the lazy dog")); got != "sha256=f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8" {
		t.Errorf("hmac %s", got)
	}
}

func TestWebhookDefaultsToHTTPS(t *testing.T) {
	tg, err := parseTarget("generic://example.com/hook")
	if err != nil || tg.(*webhookTarget).endpoint != "https://example.com/hook" {
		t.Fatalf("%+v %v", tg, err)
	}
}

func TestFanOutDeliversToOthersWhenOneFails(t *testing.T) {
	good, bad := newSink(t), newSink(t)
	bad.status = 503
	n := nativeFor(t, "generic+http://"+bad.host()+"/a", "generic+http://"+good.host()+"/b")
	err := n.Send(t.Context(), sampleEvent(), sampleMsg)
	if plugin.ClassOf(err) != plugin.ClassServerError || good.count() != 1 || bad.count() != 1 {
		t.Fatalf("%v good=%d bad=%d", err, good.count(), bad.count())
	}
}

func TestErrorsNeverLeakURLs(t *testing.T) {
	s := newSink(t)
	s.status = 302
	n := nativeFor(t, "generic+http://"+s.host()+"/hook/TOPSECRET?secret=KEY")
	err := n.Send(t.Context(), sampleEvent(), sampleMsg)
	if err == nil || strings.Contains(err.Error(), "TOPSECRET") || strings.Contains(err.Error(), "KEY") {
		t.Fatalf("redirect status error: %v", err)
	}
	dead := newSink(t)
	host := dead.host()
	dead.srv.Close()
	n = nativeFor(t, "generic+http://"+host+"/hook/TOPSECRET")
	err = n.Send(t.Context(), sampleEvent(), sampleMsg)
	if plugin.ClassOf(err) != plugin.ClassUnreachable || strings.Contains(err.Error(), "TOPSECRET") {
		t.Fatalf("transport error: %v", err)
	}
}

func TestSettingsValidation(t *testing.T) {
	bad := []plugin.Settings{
		{}, {"urls": []any{}}, {"urls": []any{"telegram://x@y"}}, {"urls": []any{"ntfy://host"}}, {"urls": []any{"ntfy:///topic"}},
		{"urls": []any{"ntfy://h/a/b"}}, {"urls": []any{"pushover://U"}}, {"urls": []any{"generic://"}}, {"urls": []any{"generic+ftp://h/x"}},
		{"urls": []any{"://bad"}},
	}
	for i, s := range bad {
		if _, err := NewNative(plugin.Instance{Settings: s}); plugin.ClassOf(err) != plugin.ClassConfig {
			t.Errorf("case %d: %v", i, err)
		}
	}
	if _, err := NewNative(plugin.Instance{Settings: plugin.Settings{"urls": []any{"telegram://x@y"}}}); err == nil || !strings.Contains(err.Error(), "telegram") {
		t.Errorf("unsupported scheme must be named: %v", err)
	}
	if got := nativeFor(t, "ntfy://h/t").Schemes(); strings.Join(got, ",") != "ntfy,pushover,generic" {
		t.Errorf("schemes %v", got)
	}
}

func TestPriorityMaps(t *testing.T) {
	if _, ok := ntfyPriority(0); ok {
		t.Error("0 unset")
	}
	if p, ok := ntfyPriority(3); !ok || p != "3" {
		t.Error("3")
	}
	if _, ok := pushoverPriority(6); ok {
		t.Error("6 out of range")
	}
	if p, ok := pushoverPriority(5); !ok || p != 2 {
		t.Error("5 → 2")
	}
}

func TestScrubText(t *testing.T) {
	if got := scrubText(`Post "https://h/hook/SECRET?x=1": boom, ntfy://u:p@h/t`); strings.Contains(got, "SECRET") || strings.Contains(got, "u:p") {
		t.Error(got)
	}
	if sanitize("x", http.ErrAbortHandler) == nil {
		t.Error("untyped errors still become errors")
	}
}
