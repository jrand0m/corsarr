package config

import (
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/plugin"
)

func TestNotifierInstancesCarryEventToggles(t *testing.T) {
	reg := testRegistry(t)
	reg.MustRegister(plugin.Registration{New: func(plugin.Instance) (any, error) { return nil, nil }, Manifest: plugin.Manifest{
		ID: "native", Kind: plugin.KindNotifier, APIVersion: plugin.APIVersion,
		Settings: []plugin.Field{{Key: "urls", Type: plugin.FieldList, Secret: true}},
	}})
	files := map[string]string{"/c.yml": "notifications:\n  phone:\n    type: native\n    urls: [\"ntfy://h/t\"]\n    on: [import, grab]\n"}
	c, err := Load(inputs([]string{"--configFile=/c.yml"}, nil, files), reg)
	if err != nil {
		t.Fatal(err)
	}
	on := c.Instances["notifications"]["phone"].Settings.Strings("on")
	if len(on) != 2 || on[0] != "import" || on[1] != "grab" {
		t.Fatalf("toggles: %v", on)
	}
	files["/c.yml"] = strings.Replace(files["/c.yml"], "grab", "everything", 1)
	if _, err := Load(inputs([]string{"--configFile=/c.yml"}, nil, files), reg); err == nil {
		t.Fatal("unknown event group accepted")
	}
}
