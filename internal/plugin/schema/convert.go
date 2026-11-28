package schema

import (
	"errors"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

var fieldTypes = map[string]plugin.FieldType{
	abi.TypeString: plugin.FieldText, abi.TypeSecret: plugin.FieldSecret, abi.TypeBool: plugin.FieldBool,
	abi.TypeInt: plugin.FieldNumber, abi.TypeNumber: plugin.FieldNumber, abi.TypeEnum: plugin.FieldEnum,
	abi.TypeDuration: plugin.FieldDuration, abi.TypeListString: plugin.FieldList, abi.TypeListInt: plugin.FieldList,
	abi.TypeMap: plugin.FieldList, // config files write a map as "key=value" entries
}

// PluginFields converts a validated schema into the registry's field model,
// so module settings become config keys and are checked by the same rules
// when a config file is loaded.
func PluginFields(s Schema) []plugin.Field {
	out := make([]plugin.Field, 0, len(s.Fields))
	for _, f := range s.Fields {
		pf := plugin.Field{Key: f.Key, Type: fieldTypes[f.Type], Label: f.Label, Help: f.Description,
			Required: f.Required && f.Default == nil, Min: f.Min, Max: f.Max, Check: entryCheck(f)}
		if f.Type != abi.TypeSecret && !isList(f.Type) {
			pf.Default = f.Default
		}
		for _, o := range f.Options {
			pf.Options = append(pf.Options, o.Value)
		}
		out = append(out, pf)
	}
	return out
}

func isList(t string) bool {
	return t == abi.TypeListString || t == abi.TypeListInt || t == abi.TypeMap
}

// entryCheck validates one config text value (a scalar or one list entry).
func entryCheck(f Field) func(string) error {
	var check func(string) string
	switch f.Type {
	case abi.TypeString, abi.TypeSecret, abi.TypeListString:
		check = func(s string) string {
			return stringProblem(abi.TextRule{MinLength: f.MinLength, MaxLength: f.MaxLength, Pattern: f.Pattern}, s)
		}
	case abi.TypeInt, abi.TypeNumber, abi.TypeListInt:
		check = func(s string) string { return numberProblem(f, s) }
	case abi.TypeDuration:
		check = func(s string) string { return durationProblem(f, s) }
	case abi.TypeMap:
		check = func(s string) string { return mapProblem(f, []any{s}) }
	default:
		return nil
	}
	return func(s string) error {
		if msg := check(s); msg != "" {
			return errors.New(msg)
		}
		return nil
	}
}

// ModuleValues turns an instance's settings into what a module reads:
// secrets left out, durations as text, maps as objects, defaults filled in.
func ModuleValues(s Schema, settings map[string]any) map[string]any {
	out := map[string]any{}
	for _, f := range s.Fields {
		if f.Type == abi.TypeSecret {
			continue
		}
		v, ok := settings[f.Key]
		if !ok || v == nil {
			if f.Default != nil {
				out[f.Key] = f.Default
			}
			continue
		}
		out[f.Key] = wireValue(f, v)
	}
	return out
}

func wireValue(f Field, v any) any {
	switch f.Type {
	case abi.TypeDuration:
		if d, ok := v.(time.Duration); ok {
			return d.String()
		}
	case abi.TypeMap:
		if m, ok := AsMap(v); ok {
			return m
		}
	}
	return v
}

// SecretFields lists the keys of secret fields.
func SecretFields(s Schema) map[string]bool {
	out := map[string]bool{}
	for _, f := range s.Fields {
		if f.Type == abi.TypeSecret {
			out[f.Key] = true
		}
	}
	return out
}
