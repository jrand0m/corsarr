package main

import (
	"github.com/jrand0m/corsarr/plugins/transmission/client"
	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugin"
)

// maxMapEntries bounds the category and path maps of the settings form.
const maxMapEntries = 50

var (
	anyKey   = abi.TextRule{MinLength: ptr(1), MaxLength: ptr(64)}
	pathRule = abi.TextRule{Pattern: abi.PatternPathAbsolute}
)

func ptr(n int) *int { return &n }

// fieldOf turns one of the client's setting descriptions into a schema field.
func fieldOf(f client.Field) *plugin.Field {
	var out *plugin.Field
	switch f.Type {
	case client.FieldURL:
		out = plugin.String(f.Key, f.Label).Pattern(abi.PatternURLHTTP)
	case client.FieldSecret:
		out = plugin.SecretField(f.Key, f.Label)
	case client.FieldBool:
		out = plugin.Bool(f.Key, f.Label)
	case client.FieldDuration:
		out = plugin.Duration(f.Key, f.Label).Between("1s", "60s")
	case client.FieldMap:
		key := anyKey
		if f.Key == client.KeyPathMap {
			key = pathRule
		}
		out = plugin.Map(f.Key, f.Label).Entries(maxMapEntries, key, pathRule)
	default:
		out = plugin.String(f.Key, f.Label)
	}
	out.Describe(f.Help)
	if f.Required {
		out.Required()
	}
	if f.Default != nil && f.Type != client.FieldMap {
		out.Default(f.Default)
	}
	return out
}

// settingsSchema is the settings form, built from the client's descriptions.
func settingsSchema() abi.Schema {
	fields := client.Settings()
	built := make([]*plugin.Field, len(fields))
	for i, f := range fields {
		built[i] = fieldOf(f)
	}
	s := plugin.Schema(built...)
	s.Groups = []abi.Group{
		plugin.Group("Connection", client.KeyURL, client.KeyUsername, client.KeyPassword, client.KeyTimeout),
		plugin.Group("Downloads", client.KeyCategoryDirs, client.KeyAddPaused),
		plugin.Group("Paths", client.KeyPathMap),
	}
	s.Actions = []abi.Action{plugin.Action("test", "Test connection")}
	return s
}
