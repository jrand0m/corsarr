package plugin_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugin"
	"github.com/jrand0m/corsarr/sdk/go/plugintest"
)

func TestDoer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-session", r.Header.Get("X-Token"))
		if r.URL.Path == "/big" {
			_, _ = w.Write([]byte(strings.Repeat("a", 11)))
			return
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte("body"))
	}))
	defer srv.Close()
	plugintest.New(plugintest.Host{Hosts: []string{"127.0.0.1"}})
	d := plugin.Doer{MaxBytes: 10}
	status, h, body, err := d.Do(context.Background(), "POST", srv.URL+"/x", map[string]string{"x-token": "t"}, []byte("{}"))
	if err != nil || status != http.StatusConflict || h["X-Session"] != "t" || string(body) != "body" {
		t.Fatalf("%d %v %q %v", status, h, body, err)
	}
	if _, _, _, err := d.Do(context.Background(), "GET", srv.URL+"/big", nil, nil); plugin.ClassOf(err) != abi.ClassParse {
		t.Errorf("oversize body: %v", err)
	}
	if _, _, _, err := (plugin.Doer{}).Do(context.Background(), "GET", srv.URL+"/big", nil, nil); err != nil {
		t.Errorf("default cap: %v", err)
	}
	if _, _, _, err := d.Do(context.Background(), "GET", "http://elsewhere.test/", nil, nil); plugin.ClassOf(err) != abi.ClassDenied {
		t.Errorf("undeclared host: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := d.Do(ctx, "GET", srv.URL, nil, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v", err)
	}
}

type fixedTransport struct{ resp *http.Response }

func (f fixedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp := *f.resp
	resp.Request = r
	resp.Body = io.NopCloser(strings.NewReader("ok"))
	return &resp, nil
}

func TestDoerHeadersWithoutValuesAreLeftOut(t *testing.T) {
	plugintest.New(plugintest.Host{Client: &http.Client{Transport: fixedTransport{&http.Response{StatusCode: http.StatusOK,
		Header: http.Header{"X-Empty": {}, "x-one": {"1", "2"}}}}}})
	_, h, body, err := plugin.Doer{}.Do(context.Background(), "GET", "http://module.test/", nil, nil)
	if err != nil || string(body) != "ok" || len(h) != 1 || h["X-One"] != "1" {
		t.Fatalf("%v %q %v", h, body, err)
	}
}

func TestDoerCapIsInclusiveAndFailuresCarryNoStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ab"))
	}))
	defer srv.Close()
	plugintest.New(plugintest.Host{Hosts: []string{"127.0.0.1"}})
	if status, _, body, err := (plugin.Doer{MaxBytes: 2}).Do(context.Background(), "GET", srv.URL, nil, nil); err != nil || status != http.StatusOK || string(body) != "ab" {
		t.Fatalf("a body of exactly the cap: %d %q %v", status, body, err)
	}
	if status, h, body, err := (plugin.Doer{MaxBytes: 1}).Do(context.Background(), "GET", srv.URL, nil, nil); err == nil || status != 0 || h != nil || body != nil {
		t.Fatalf("a cap of one byte: %d %v %q %v", status, h, body, err)
	}
	if status, _, _, err := (plugin.Doer{}).Do(context.Background(), "GET", "http://elsewhere.test/", nil, nil); err == nil || status != 0 {
		t.Fatalf("refused host: %d %v", status, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if status, _, _, err := (plugin.Doer{}).Do(ctx, "GET", srv.URL, nil, nil); err == nil || status != 0 {
		t.Fatalf("cancelled: %d %v", status, err)
	}
}
