package main

import (
	"reflect"
	"testing"

	"github.com/jrand0m/corsarr/plugins/transmission/client"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

func TestSettingsFormFields(t *testing.T) {
	help := map[string]string{}
	for _, f := range client.Settings() {
		help[f.Key] = f.Help
	}
	path := &abi.TextRule{Pattern: abi.PatternPathAbsolute}
	name := &abi.TextRule{MinLength: ptr(1), MaxLength: ptr(64)}
	want := []abi.Field{
		{Key: client.KeyURL, Type: abi.TypeString, Label: "Address", Description: help[client.KeyURL], Required: true, Pattern: abi.PatternURLHTTP},
		{Key: client.KeyUsername, Type: abi.TypeString, Label: "Username", Description: help[client.KeyUsername]},
		{Key: client.KeyPassword, Type: abi.TypeSecret, Label: "Password", Description: help[client.KeyPassword]},
		{Key: client.KeyCategoryDirs, Type: abi.TypeMap, Label: "Download directory per category", Description: help[client.KeyCategoryDirs],
			MaxEntries: maxMapEntries, KeyRule: name, ValueRule: path},
		{Key: client.KeyPathMap, Type: abi.TypeMap, Label: "Path mapping", Description: help[client.KeyPathMap],
			MaxEntries: maxMapEntries, KeyRule: path, ValueRule: path},
		{Key: client.KeyAddPaused, Type: abi.TypeBool, Label: "Add paused", Description: help[client.KeyAddPaused], Default: false},
		{Key: client.KeyTimeout, Type: abi.TypeDuration, Label: "Request timeout", Description: help[client.KeyTimeout],
			Default: client.DefaultTimeout.String(), MinDuration: "1s", MaxDuration: "60s"},
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
