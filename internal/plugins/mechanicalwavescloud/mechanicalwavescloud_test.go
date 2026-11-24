package mechanicalwavescloud

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
	p, err := New(plugin.Instance{Key: "waves", Settings: plugin.Settings{"baseUrl": "https://x.invalid", "downloadDir": t.TempDir()}, Env: env})
	if err != nil {
		t.Fatal(err)
	}
	_ = p.(*sourcekit.Source).Close()
}
