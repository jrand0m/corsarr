package plugintest

import (
	"encoding/json"
	"testing"

	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// The SDK only calls host functions it knows with well-formed requests, so
// these answers are checked at the host-function boundary itself.
func TestHostFunctionBoundary(t *testing.T) {
	hs := New(Host{})
	for name, tc := range map[string]struct {
		fn    string
		req   string
		class string
	}{
		"unknown function":     {abi.HostTake, `{}`, abi.ClassDenied},
		"malformed http_do":    {abi.HostHTTPDo, `not json`, abi.ClassServerError},
		"settings need no req": {abi.HostSettingsGet, ``, ""},
	} {
		var env abi.Envelope
		if err := json.Unmarshal(hs.serve(tc.fn, []byte(tc.req)), &env); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := ""
		if env.Error != nil {
			got = env.Error.Class
		}
		if got != tc.class {
			t.Errorf("%s: class %q, want %q (%+v)", name, got, tc.class, env.Error)
		}
	}
}
