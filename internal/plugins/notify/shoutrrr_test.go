//go:build !nonotify_shoutrrr

package notify

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/conformance"
)

type countingDoer struct {
	n atomic.Int64
}

func (c *countingDoer) Do(r *http.Request) (*http.Response, error) {
	c.n.Add(1)
	return http.DefaultClient.Do(r)
}

func shoutrrrFor(t *testing.T, d plugin.HTTPDoer, urls ...string) *Shoutrrr {
	t.Helper()
	asAny := make([]any, len(urls))
	for i, u := range urls {
		asAny[i] = u
	}
	p, err := NewShoutrrr(plugin.Instance{Key: "s", Settings: plugin.Settings{"urls": asAny}, Env: conformance.Env(d)})
	if err != nil {
		t.Fatal(err)
	}
	return p.(*Shoutrrr)
}

func TestShoutrrrConformance(t *testing.T) {
	reg, ok := plugin.Default.Get(ShoutrrrID)
	if !ok {
		t.Fatal("shoutrrr must be registered by default")
	}
	conformance.Manifest(t, reg.Manifest, plugin.KindNotifier)
	s := newSink(t)
	s.reply = ntfyReply
	conformance.Notifier(t, shoutrrrFor(t, http.DefaultClient, "ntfy://"+s.host()+"/topic?scheme=http"), sampleEvent(), sampleMsg)
}

func TestShoutrrrUsesTheSharedHTTPClient(t *testing.T) {
	s := newSink(t)
	s.reply = ntfyReply
	d := &countingDoer{}
	n := shoutrrrFor(t, d, "ntfy://"+s.host()+"/topic?scheme=http")
	if err := n.Send(t.Context(), sampleEvent(), sampleMsg); err != nil {
		t.Fatal(err)
	}
	c := s.last()
	if d.n.Load() != 1 || string(c.body) != "Some Movie (2021)" || c.header.Get("Title") != "Imported" {
		t.Errorf("requests via env client = %d, delivered %+v", d.n.Load(), c)
	}
}

func TestShoutrrrFanOut(t *testing.T) {
	good, bad := newSink(t), newSink(t)
	good.reply = ntfyReply
	bad.status = 500
	n := shoutrrrFor(t, http.DefaultClient, "ntfy://"+bad.host()+"/a?scheme=http", "ntfy://"+good.host()+"/b?scheme=http")
	err := n.Send(t.Context(), sampleEvent(), sampleMsg)
	if err == nil || good.count() != 1 {
		t.Fatalf("one URL failing must not block the other: %v good=%d", err, good.count())
	}
	if strings.Contains(err.Error(), bad.host()) || strings.Contains(err.Error(), "/a") {
		t.Errorf("error leaks the service URL: %v", err)
	}
}

func TestShoutrrrSchemesAndValidation(t *testing.T) {
	n := shoutrrrFor(t, http.DefaultClient, "ntfy://h/t")
	has := map[string]bool{}
	for _, s := range n.Schemes() {
		has[s] = true
	}
	for _, want := range []string{"ntfy", "pushover", "telegram", "discord", "smtp", "generic"} {
		if !has[want] {
			t.Errorf("scheme %s missing from %v", want, n.Schemes())
		}
	}
	for _, s := range []plugin.Settings{{}, {"urls": []any{"nosuchservice://x"}}} {
		if _, err := NewShoutrrr(plugin.Instance{Settings: s, Env: conformance.Env(http.DefaultClient)}); plugin.ClassOf(err) != plugin.ClassConfig {
			t.Errorf("%v: %v", s, err)
		}
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestClassifyShoutrrr(t *testing.T) {
	cases := []struct {
		err  error
		want plugin.FailureClass
	}{
		{context.DeadlineExceeded, plugin.ClassTimeout},
		{timeoutErr{}, plugin.ClassTimeout},
		{&net.OpError{Op: "dial", Err: errors.New("connection refused")}, plugin.ClassUnreachable},
		{errors.New("server returned 401 Unauthorized"), plugin.ClassAuth},
		{errors.New("invalid token for https://x/secret"), plugin.ClassAuth},
		{errors.New("got 429 Too Many Requests"), plugin.ClassRateLimited},
		{errors.New("got 502 bad gateway"), plugin.ClassServerError},
		{errors.New("something else"), plugin.ClassServerError},
	}
	for _, c := range cases {
		got := classifyShoutrrr(c.err)
		if plugin.ClassOf(got) != c.want || strings.Contains(got.Error(), "https://x/secret") {
			t.Errorf("%v → %v", c.err, got)
		}
	}
}

func TestShoutrrrTestSendsTestMessage(t *testing.T) {
	s := newSink(t)
	s.reply = ntfyReply
	n := shoutrrrFor(t, http.DefaultClient, "ntfy://"+s.host()+"/t?scheme=http")
	if err := n.Test(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(s.last().body), "Test notification") {
		t.Errorf("%s", s.last().body)
	}
}

const ntfyReply = `{"id":"abc123","time":1700000000,"event":"message","topic":"t"}`
