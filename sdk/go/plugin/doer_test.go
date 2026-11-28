package plugin_test

import (
	"context"
	"errors"
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
