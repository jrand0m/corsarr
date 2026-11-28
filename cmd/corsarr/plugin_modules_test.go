package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/wasm/wasmtest"
	"github.com/jrand0m/corsarr/plugins/rtorrent/fakexmlrpc"
	"github.com/jrand0m/corsarr/plugins/transmission/fakerpc"
)

// fixture is one recorded server a module's conformance run talks to.
type fixture struct {
	name     string
	handler  http.Handler
	settings func(url string) map[string]any
}

// The official modules pass `corsarr plugin test` against a recorded fake of
// every server version they support.
func TestOfficialModulesConformance(t *testing.T) {
	modules := map[string][]fixture{"./plugins/transmission": nil, "./plugins/rtorrent": nil}
	for _, v := range fakerpc.Versions {
		modules["./plugins/transmission"] = append(modules["./plugins/transmission"], fixture{"transmission " + v.Name,
			fakerpc.New(v, fakerpc.Options{Username: "u", Password: "p"}).Handler(),
			func(url string) map[string]any { return map[string]any{"url": url, "username": "u", "password": "p"} }})
	}
	for _, p := range fakexmlrpc.Profiles {
		modules["./plugins/rtorrent"] = append(modules["./plugins/rtorrent"], fixture{"rtorrent " + p.Name,
			fakexmlrpc.New(p, fakexmlrpc.Options{Username: "rt", Password: "pw"}).Handler(),
			func(url string) map[string]any {
				return map[string]any{"url": url + "/RPC2", "username": "rt", "password": "pw"}
			}})
	}
	for pkg, fixtures := range modules {
		mod := writeModule(t, "m.wasm", wasmtest.Module(t, pkg))
		for _, f := range fixtures {
			t.Run(f.name, func(t *testing.T) {
				srv := httptest.NewServer(f.handler)
				defer srv.Close()
				b, _ := json.Marshal(f.settings(srv.URL))
				settings := writeModule(t, "s.json", b)
				// A generous start budget: the race detector and parallel tests slow instantiation.
				code, out, errs := cliRun(t, nil, "plugin", "test", mod, "--settings="+settings, "--start-budget=5s")
				if !strings.Contains(out, "PASS  downloader conformance") || !(code == 0 || raceEnabled) {
					t.Fatalf("%d\n%s\n%s", code, out, errs)
				}
			})
		}
	}
}
