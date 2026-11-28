package main

import (
	"reflect"
	"testing"

	"github.com/jrand0m/corsarr/plugins/rtorrent/client"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

func TestSettingsFormFields(t *testing.T) {
	help := map[string]string{}
	for _, f := range client.Fields() {
		help[f.Key] = f.Help
	}
	path := &abi.TextRule{Pattern: abi.PatternPathAbsolute}
	opt := func(v string) abi.Option { return abi.Option{Value: v, Label: v} }
	want := []abi.Field{
		{Key: "url", Type: abi.TypeString, Label: "XML-RPC URL", Description: help["url"], Required: true, Pattern: abi.PatternURLHTTP},
		{Key: "username", Type: abi.TypeString, Label: "Username", Description: help["username"]},
		{Key: "password", Type: abi.TypeSecret, Label: "Password"},
		{Key: "pathMap", Type: abi.TypeMap, Label: "Path mapping", Description: help["pathMap"],
			MaxEntries: maxPathMaps, KeyRule: path, ValueRule: path},
		{Key: "labelField", Type: abi.TypeEnum, Label: "Label field", Description: help["labelField"], Default: "custom1",
			Options: []abi.Option{opt("custom1"), opt("custom2"), opt("custom3"), opt("custom4")}},
		{Key: "timeout", Type: abi.TypeDuration, Label: "Request timeout", Default: client.DefaultTimeout.String(),
			MinDuration: "1s", MaxDuration: "60s"},
	}
	got := settingsSchema().Fields
	if len(got) != len(want) {
		t.Fatalf("%d fields, want %d", len(got), len(want))
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("field %s:\n got %+v\nwant %+v", want[i].Key, got[i], want[i])
		}
	}
}
