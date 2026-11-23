package config

import (
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/plugin"
)

func TestNotificationTemplateIsValidatedAtLoad(t *testing.T) {
	reg := testRegistry(t)
	reg.MustRegister(plugin.Registration{New: func(plugin.Instance) (any, error) { return nil, nil }, Manifest: plugin.Manifest{
		ID: "native", Kind: plugin.KindNotifier, APIVersion: plugin.APIVersion, Name: "Native"}})
	good := "notifications:\n  phone:\n    type: native\n    template:\n      title: \"[corsarr] {{.Title}}\"\n      body: \"{{join .Payload.Files}}\"\n"
	c, err := Load(inputs([]string{"--configFile=/c.yml"}, nil, map[string]string{"/c.yml": good}), reg)
	if err != nil {
		t.Fatal(err)
	}
	tpl := c.Instances["notifications"]["phone"].Settings.Map("template")
	if tpl["title"] != "[corsarr] {{.Title}}" || tpl["body"] != "{{join .Payload.Files}}" {
		t.Fatalf("%v", tpl)
	}
	_, err = Load(inputs([]string{"--notifications.phone.type=native", "--notifications.phone.template.title={{.Title"}, nil, nil), reg)
	if err == nil || !strings.Contains(err.Error(), "notifications.phone.template.title (--notifications.phone.template.title): not a valid message template") {
		t.Fatalf("bad template: %v", err)
	}
}
