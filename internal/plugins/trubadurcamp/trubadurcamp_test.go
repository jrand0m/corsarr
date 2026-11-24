package trubadurcamp

import (
	"net/http"
	"testing"

	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/conformance"
	"github.com/jrand0m/corsarr/internal/plugins/sourcekit"
)

func TestManifestAndSettings(t *testing.T) {
	reg, ok := plugin.Default.Get(ID)
	if !ok {
		t.Fatal("not registered")
	}
	conformance.Manifest(t, reg.Manifest, plugin.KindSource)
	env := conformance.Env(http.DefaultClient)
	if _, err := New(plugin.Instance{Settings: plugin.Settings{"baseUrl": "nope"}, Env: env}); err == nil {
		t.Fatal("bad address accepted")
	}
	if _, err := New(plugin.Instance{Settings: plugin.Settings{"baseUrl": "http://x.invalid"}, Env: env}); err == nil {
		t.Fatal("download folder is required")
	}
	p, err := New(plugin.Instance{Key: "camp", Settings: plugin.Settings{"baseUrl": "http://x.invalid", "downloadDir": t.TempDir(),
		"identity": "abc"}, Env: env})
	if err != nil {
		t.Fatal(err)
	}
	src := p.(*sourcekit.Source)
	_ = src.Close()
	if cookie("") != "" || cookie("abc") != "identity=abc" {
		t.Fatal("cookie")
	}
}
