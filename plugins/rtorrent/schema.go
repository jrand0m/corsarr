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
	build, ok := fieldKinds[f.Type]
	if !ok {
		build = textField
	}
	out := build(f)
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

// fieldKinds builds the schema field for each description type; any other
// type is plain text.
var fieldKinds = map[string]func(client.Field) *plugin.Field{
	"url":      urlField,
	"secret":   secretField,
	"enum":     enumField,
	"duration": durationField,
	"map":      pathMapField,
}

func textField(f client.Field) *plugin.Field { return plugin.String(f.Key, f.Label) }

func urlField(f client.Field) *plugin.Field {
	return plugin.String(f.Key, f.Label).Pattern(abi.PatternURLHTTP)
}

func secretField(f client.Field) *plugin.Field { return plugin.SecretField(f.Key, f.Label) }

func enumField(f client.Field) *plugin.Field {
	opts := make([]abi.Option, len(f.Options))
	for i, o := range f.Options {
		opts[i] = plugin.Opt(o, o)
	}
	return plugin.Enum(f.Key, f.Label, opts...)
}

func durationField(f client.Field) *plugin.Field {
	return plugin.Duration(f.Key, f.Label).Between("1s", "60s")
}

func pathMapField(f client.Field) *plugin.Field {
	return plugin.Map(f.Key, f.Label).Entries(maxPathMaps, pathRule, pathRule)
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
