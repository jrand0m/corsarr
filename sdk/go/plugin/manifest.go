package plugin

import (
	"encoding/binary"

	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// Manifest is plugin.json; build Settings with the schema builder below.
type Manifest abi.Manifest

// EmbedManifest returns a copy of module with plugin.json appended as the
// corsarr.manifest custom section.
func EmbedManifest(module, manifest []byte) []byte {
	var body []byte
	body = binary.AppendUvarint(body, uint64(len(abi.ManifestSection)))
	body = append(body, abi.ManifestSection...)
	body = append(body, manifest...)
	out := append([]byte{}, module...)
	out = append(out, 0)
	out = binary.AppendUvarint(out, uint64(len(body)))
	return append(out, body...)
}

// ── settings schema builder ──────────────────────────────────────────────────

// Field starts a settings field; chain the setters and pass it to Schema.
type Field struct{ f abi.Field }

func newField(key, typ, label string) *Field {
	return &Field{abi.Field{Key: key, Type: typ, Label: label}}
}

// String is a text field.
func String(key, label string) *Field { return newField(key, abi.TypeString, label) }

// SecretField is a write-only field (API keys, passwords).
func SecretField(key, label string) *Field { return newField(key, abi.TypeSecret, label) }

// Bool is an on/off field.
func Bool(key, label string) *Field { return newField(key, abi.TypeBool, label) }

// Int is a whole-number field.
func Int(key, label string) *Field { return newField(key, abi.TypeInt, label) }

// Number is a decimal field (at most 3 decimals).
func Number(key, label string) *Field { return newField(key, abi.TypeNumber, label) }

// Duration is a duration field ("30s", "5m").
func Duration(key, label string) *Field { return newField(key, abi.TypeDuration, label) }

// StringList is a list of text entries.
func StringList(key, label string) *Field { return newField(key, abi.TypeListString, label) }

// IntList is a list of whole numbers.
func IntList(key, label string) *Field { return newField(key, abi.TypeListInt, label) }

// Map is a set of key=value text pairs (path mappings, category folders).
func Map(key, label string) *Field { return newField(key, abi.TypeMap, label) }

// Enum is a choice between options.
func Enum(key, label string, options ...abi.Option) *Field {
	f := newField(key, abi.TypeEnum, label)
	f.f.Options = options
	return f
}

// Opt is one enum option.
func Opt(value, label string) abi.Option { return abi.Option{Value: value, Label: label} }

// Describe sets the help text.
func (f *Field) Describe(text string) *Field { f.f.Description = text; return f }

// Hint sets the text shown with validation errors.
func (f *Field) Hint(text string) *Field { f.f.Hint = text; return f }

// Required marks the field required.
func (f *Field) Required() *Field { f.f.Required = true; return f }

// Default sets the default value.
func (f *Field) Default(v any) *Field { f.f.Default = v; return f }

// Pattern sets a text pattern (abi.PatternURLHTTP, …).
func (f *Field) Pattern(p string) *Field { f.f.Pattern = p; return f }

// Length bounds text length.
func (f *Field) Length(minLen, maxLen int) *Field {
	f.f.MinLength, f.f.MaxLength = &minLen, &maxLen
	return f
}

// Range bounds a number.
func (f *Field) Range(lo, hi float64) *Field { f.f.Min, f.f.Max = &lo, &hi; return f }

// Step sets a number's step.
func (f *Field) Step(s float64) *Field { f.f.Step = &s; return f }

// Decimals limits a number's decimals.
func (f *Field) Decimals(d int) *Field { f.f.Decimals = &d; return f }

// Between bounds a duration.
func (f *Field) Between(lo, hi string) *Field { f.f.MinDuration, f.f.MaxDuration = lo, hi; return f }

// MaxItems bounds a list.
func (f *Field) MaxItems(n int) *Field { f.f.MaxItems = n; return f }

// Entries bounds a map and constrains its keys and values.
func (f *Field) Entries(maxEntries int, key, value abi.TextRule) *Field {
	f.f.MaxEntries, f.f.KeyRule, f.f.ValueRule = maxEntries, &key, &value
	return f
}

// Schema assembles fields into a settings schema.
func Schema(fields ...*Field) abi.Schema {
	s := abi.Schema{Fields: make([]abi.Field, len(fields))}
	for i, f := range fields {
		s.Fields[i] = f.f
	}
	return s
}

// Group is a titled set of field keys.
func Group(title string, keys ...string) abi.Group { return abi.Group{Title: title, Fields: keys} }

// Action is a settings-form button that calls OnAction.
func Action(id, label string) abi.Action { return abi.Action{ID: id, Label: label} }
