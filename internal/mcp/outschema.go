package mcp

import (
	"encoding"
	"encoding/json"
	"reflect"
	"strings"
	"time"
)

// Output schemas are generated from the Go types a tool answers with, so the
// declared outputSchema follows the structuredContent it sends: a struct is
// an object of its JSON fields (fields without omitempty are required), a
// nil-able field that is always sent may be null.

var (
	timeType      = reflect.TypeFor[time.Time]()
	marshalerType = reflect.TypeFor[json.Marshaler]()
	textType      = reflect.TypeFor[encoding.TextMarshaler]()
)

// outputOf is the output schema of a result type T.
func outputOf[T any]() schema {
	return typeSchema(reflect.TypeFor[T](), map[reflect.Type]bool{})
}

// oneOfOutputs is an object output that is one of several shapes.
func oneOfOutputs(shapes ...schema) schema {
	list := make([]any, len(shapes))
	for i, s := range shapes {
		list[i] = s
	}
	return schema{"type": "object", "anyOf": list}
}

func typeSchema(t reflect.Type, seen map[reflect.Type]bool) schema {
	if s, ok := specialSchema(t); ok {
		return s
	}
	switch t.Kind() {
	case reflect.Pointer:
		return typeSchema(t.Elem(), seen)
	case reflect.Struct:
		return structSchema(t, seen)
	case reflect.Slice, reflect.Array:
		return schema{"type": "array", "items": typeSchema(t.Elem(), seen)}
	case reflect.Map:
		return schema{"type": "object", "additionalProperties": typeSchema(t.Elem(), seen)}
	}
	return scalarSchema(t.Kind())
}

// specialSchema covers types with their own JSON encoding.
func specialSchema(t reflect.Type) (schema, bool) {
	switch {
	case t == timeType:
		return schema{"type": "string", "format": "date-time"}, true
	case t.Implements(marshalerType) || reflect.PointerTo(t).Implements(marshalerType):
		return schema{}, true
	case t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8:
		return schema{"type": "string"}, true
	case t.Implements(textType) || reflect.PointerTo(t).Implements(textType):
		return schema{"type": "string"}, true
	}
	return nil, false
}

var scalarTypes = map[reflect.Kind]string{
	reflect.Bool: "boolean", reflect.String: "string",
	reflect.Int: "integer", reflect.Int8: "integer", reflect.Int16: "integer", reflect.Int32: "integer", reflect.Int64: "integer",
	reflect.Uint: "integer", reflect.Uint8: "integer", reflect.Uint16: "integer", reflect.Uint32: "integer", reflect.Uint64: "integer",
	reflect.Float32: "number", reflect.Float64: "number",
}

func scalarSchema(k reflect.Kind) schema {
	if name, ok := scalarTypes[k]; ok {
		return schema{"type": name}
	}
	return schema{} // interfaces: anything
}

// structSchema is an object of the struct's JSON fields; a type met again
// inside itself is left open.
func structSchema(t reflect.Type, seen map[reflect.Type]bool) schema {
	if seen[t] {
		return schema{"type": "object"}
	}
	seen[t] = true
	defer delete(seen, t)
	props, required := map[string]schema{}, []string{}
	addFields(t, seen, props, &required)
	s := schema{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func addFields(t reflect.Type, seen map[reflect.Type]bool, props map[string]schema, required *[]string) {
	for i := range t.NumField() {
		f := t.Field(i)
		name, omit, ok := jsonName(f)
		switch {
		case !ok:
		case f.Anonymous && name == "" && derefKind(f.Type) == reflect.Struct:
			embedded := []string{}
			addFields(deref(f.Type), seen, props, &embedded)
			if f.Type.Kind() != reflect.Pointer { // a nil embedded pointer sends none of its fields
				*required = append(*required, embedded...)
			}
		case f.IsExported():
			props[fieldName(name, f)] = fieldSchema(f.Type, omit, seen)
			if !omit {
				*required = append(*required, fieldName(name, f))
			}
		}
	}
}

// jsonName reads the json tag: the name ("" when untagged), omitempty, and
// whether the field is encoded at all.
func jsonName(f reflect.StructField) (string, bool, bool) {
	tag, has := f.Tag.Lookup("json")
	if tag == "-" {
		return "", false, false
	}
	name, opts, _ := strings.Cut(tag, ",")
	omit := strings.Contains(","+opts+",", ",omitempty,") || strings.Contains(","+opts+",", ",omitzero,")
	if !has {
		return "", false, true
	}
	return name, omit, true
}

func fieldName(name string, f reflect.StructField) string {
	if name == "" {
		return f.Name
	}
	return name
}

// fieldSchema lets an always-sent pointer, slice or map be null.
func fieldSchema(t reflect.Type, omit bool, seen map[reflect.Type]bool) schema {
	s := typeSchema(t, seen)
	if omit || !nilable(t) {
		return s
	}
	if typ, ok := s["type"].(string); ok {
		out := schema{}
		for k, v := range s {
			out[k] = v
		}
		out["type"] = []string{typ, "null"}
		return out
	}
	return s
}

func nilable(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface:
		return true
	}
	return false
}

func deref(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Pointer {
		return t.Elem()
	}
	return t
}

func derefKind(t reflect.Type) reflect.Kind { return deref(t).Kind() }
