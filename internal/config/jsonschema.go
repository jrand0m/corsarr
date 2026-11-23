package config

import "encoding/json"

// JSONSchema renders the assembled schema as JSON Schema (draft 2020-12),
// deterministic (encoding/json sorts map keys). Plugin instances become a
// oneOf over the registered plugins of their kind.
func (s *Schema) JSONSchema() []byte {
	root := s.jsonNode(s.Root)
	root["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	root["title"] = "corsarr configuration"
	b, _ := json.MarshalIndent(root, "", "  ") // only maps, strings and numbers
	return append(b, '\n')
}

type js = map[string]any

var leafTypes = map[NodeKind]string{
	KindString: "string", KindEnum: "string", KindSecret: "string", KindDuration: "string",
	KindInt: "integer", KindNumber: "number", KindBool: "boolean",
}

func (s *Schema) jsonNode(n *Node) js {
	var out js
	switch n.Kind {
	case KindObject:
		out = jsObject(s, n.Fields)
	case KindMap:
		out = js{"type": "object", "propertyNames": js{"pattern": "^[a-z][a-z0-9]*$"}, "additionalProperties": s.jsonNode(n.Elem)}
	case KindList:
		out = js{"type": "array", "items": s.jsonNode(n.Elem)}
	case KindInstance:
		out = s.jsonInstance(n)
	default:
		out = jsLeaf(n)
	}
	if n.Doc != "" {
		out["description"] = n.Doc
	}
	return out
}

func jsObject(s *Schema, fields []Field) js {
	props := js{}
	var req []string
	for _, f := range fields {
		props[f.Name] = s.jsonNode(f.Node)
		if f.Node.Required {
			req = append(req, f.Name)
		}
	}
	out := js{"type": "object", "properties": props, "additionalProperties": false}
	if len(req) > 0 {
		out["required"] = req
	}
	return out
}

func jsLeaf(n *Node) js {
	out := js{"type": leafTypes[n.Kind]}
	if n.Kind == KindEnum && len(n.Enum) > 0 {
		out["enum"] = n.Enum
	}
	if n.Kind == KindSecret {
		out["writeOnly"] = true
		out["description"] = "Secret: a value or ${env:NAME} / ${file:/path}."
	}
	if n.Default != "" {
		out["default"] = jsDefault(n)
	}
	return out
}

func jsDefault(n *Node) any {
	v, err := parseScalar(n, n.Default)
	if err != nil || n.Kind == KindDuration {
		return n.Default
	}
	return v
}

func (s *Schema) jsonInstance(n *Node) js {
	var variants []any
	for _, m := range s.Plugins.List(n.Plugin) {
		fields := append(commonInstanceFields(n.Plugin, s.Plugins), pluginFields(m)...)
		v := jsObject(s, fields)
		v["properties"].(js)["type"] = js{"const": m.ID}
		v["title"] = m.Name
		variants = append(variants, v)
	}
	if len(variants) == 0 {
		return js{"type": "object", "required": []string{"type"}}
	}
	return js{"oneOf": variants}
}
