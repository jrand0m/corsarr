package main

import (
	"github.com/jrand0m/corsarr/plugins/rtorrent/client"
	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugin"
)

// maxPathMaps bounds the path mapping of the settings form.
const maxPathMaps = 50

var pathRule = abi.TextRule{Pattern: abi.PatternPathAbsolute}

// fieldOf turns one of the client's setting descriptions into a schema field.
func fieldOf(f client.Field) *plugin.Field {
	var out *plugin.Field
	switch f.Type {
	case "url":
		out = plugin.String(f.Key, f.Label).Pattern(abi.PatternURLHTTP)
	case "secret":
		out = plugin.SecretField(f.Key, f.Label)
	case "enum":
		opts := make([]abi.Option, len(f.Options))
		for i, o := range f.Options {
			opts[i] = plugin.Opt(o, o)
		}
		out = plugin.Enum(f.Key, f.Label, opts...)
	case "duration":
		out = plugin.Duration(f.Key, f.Label).Between("1s", "60s")
	case "map":
		out = plugin.Map(f.Key, f.Label).Entries(maxPathMaps, pathRule, pathRule)
	default:
		out = plugin.String(f.Key, f.Label)
	}
	if f.Help != "" {
		out.Describe(f.Help)
	}
	if f.Required {
		out.Required()
	}
	if f.Default != nil {
		out.Default(f.Default)
	}
	return out
}

// settingsSchema is the settings form, built from the client's descriptions.
func settingsSchema() abi.Schema {
	fields := client.Fields()
	built := make([]*plugin.Field, len(fields))
	for i, f := range fields {
		built[i] = fieldOf(f)
	}
	s := plugin.Schema(built...)
	s.Groups = []abi.Group{
		plugin.Group("Connection", "url", "username", "password", "timeout"),
		plugin.Group("Paths", "pathMap"),
		plugin.Group("Advanced", "labelField"),
	}
	s.Actions = []abi.Action{plugin.Action("test", "Test connection")}
	return s
}
