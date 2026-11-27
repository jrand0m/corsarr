package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/health"
)

func TestEgressAllows(t *testing.T) {
	e := newEgress([]string{"api.example.org", "*.example.net", "10.0.0.0/8", " LOCALHOST ", "[::1]"})
	cases := map[string]bool{
		"api.example.org": true, "API.Example.org.": true, "other.example.org": false,
		"a.example.net": true, "a.b.example.net": true, "example.net": false,
		"10.1.2.3": true, "11.1.2.3": false, "::ffff:10.0.0.1": true,
		"localhost": true, "::1": true, "evil.com": false,
	}
	for host, want := range cases {
		if got := e.allows(host); got != want {
			t.Errorf("allows(%q) = %v, want %v", host, got, want)
		}
	}
	var none *egress
	if !none.allows("anything") || newEgress(nil) != nil {
		t.Fatal("an empty list must allow every host")
	}
}

type recordReporter struct{ last health.Observation }

func (r *recordReporter) Report(_ domain.IntegrationRef, o health.Observation) domain.IntegrationHealth {
	r.last = o
	return domain.IntegrationHealth{}
}

func TestClientBlocksHostsOutsideAllowList(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits.Add(1) }))
	defer srv.Close()
	rep := &recordReporter{}
	c := New(Options{Allow: []string{"127.0.0.1"}, Reporter: rep})
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	blocked := New(Options{Allow: []string{"api.example.org"}, Reporter: rep})
	req, _ = http.NewRequest(http.MethodGet, srv.URL, nil)
	_, err = blocked.For(domain.IntegrationRef{Type: "indexer", Key: "x"}).Do(req)
	var be ErrEgressBlocked
	if !errors.As(err, &be) || be.Host != "127.0.0.1" {
		t.Fatalf("err = %v, want blocked", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("server saw %d requests, want 1", hits.Load())
	}
	if rep.last.OK || rep.last.Class != health.ClassUnreachable || rep.last.Message != be.Error() {
		t.Fatalf("health = %+v", rep.last)
	}
}
