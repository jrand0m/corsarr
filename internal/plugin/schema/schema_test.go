package schema

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

func f64(v float64) *float64 { return &v }

func good() Schema {
	return Schema{
		Fields: []Field{
			{Key: "url", Type: abi.TypeString, Label: "Address", Pattern: abi.PatternURLHTTP, Required: true},
			{Key: "apiKey", Type: abi.TypeSecret, Label: "API key 🔑"},
			{Key: "useSsl", Type: abi.TypeBool, Label: "TLS", Default: false},
			{Key: "port", Type: abi.TypeInt, Label: "Port", Min: f64(1), Max: f64(65535), Default: 8080.0},
			{Key: "ratio", Type: abi.TypeNumber, Label: "Ratio", Min: f64(0), Step: f64(0.5), Decimals: ptr(1)},
			{Key: "mode", Type: abi.TypeEnum, Label: "Mode", Options: []Option{{Value: "fast", Label: "Fast ⚡"}, {Value: "slow", Label: "Slow 🐢"}}},
			{Key: "every", Type: abi.TypeDuration, Label: "Poll", MinDuration: "5s", MaxDuration: "1h"},
			{Key: "tags", Type: abi.TypeListString, Label: "Tags", MaxItems: 3, Pattern: abi.PatternIdentifier},
			{Key: "ports", Type: abi.TypeListInt, Label: "Ports", Max: f64(10)},
			{Key: "paths", Type: abi.TypeMap, Label: "Path map", MaxEntries: 2,
				KeyRule: &abi.TextRule{Pattern: abi.PatternPathAbsolute}, ValueRule: &abi.TextRule{Pattern: abi.PatternPathAbsolute}},
		},
		Groups:  []Group{{Title: "Connection", Fields: []string{"url", "apiKey"}}},
		Actions: []Action{{ID: "test", Label: "Test ✅"}, {ID: "discover", Label: "Find"}},
	}
}

func TestGoodSchema(t *testing.T) {
	if ps := Check(good()); len(ps) != 0 {
		t.Fatalf("%v", ps)
	}
	b, _ := json.Marshal(good())
	if _, ps := Decode(b); len(ps) != 0 {
		t.Fatalf("decode: %v", ps)
	}
}

func TestHostileSchema(t *testing.T) {
	s := Schema{
		Fields: []Field{
			{Key: "Bad_key", Type: abi.TypeString, Label: "<b>bold</b> is fine as text"},
			{Key: "x", Type: "html", Label: "x"},
			{Key: "y", Type: abi.TypeString, Label: "evil\u202Etxt.exe"},
			{Key: "z", Type: abi.TypeString, Label: "z\u200Bz", Pattern: "regex"},
			{Key: "z", Type: abi.TypeBool, Label: ""},
			{Key: "n", Type: abi.TypeInt, Label: "n", Min: f64(5), Max: f64(1), Step: f64(0.5), Options: []Option{{Value: "a", Label: "a"}}},
			{Key: "e", Type: abi.TypeEnum, Label: "e", Options: []Option{{Value: "A", Label: "🤖"}}},
			{Key: "s", Type: abi.TypeSecret, Label: "s", Default: "hunter2"},
			{Key: "d", Type: abi.TypeDuration, Label: "d", MinDuration: "soon", Default: "x"},
			{Key: "m", Type: abi.TypeMap, Label: "m", KeyRule: &abi.TextRule{Pattern: "nope"}},
		},
		Groups:  []Group{{Title: "g", Fields: []string{"x", "x", "ghost"}}},
		Actions: []Action{{ID: "Bad", Label: "a"}, {ID: "test", Label: "b"}, {ID: "test", Label: "c"}, {ID: "four", Label: "d"}},
	}
	got := Check(s).Error()
	for _, want := range []string{
		"fields[0].key: must start", "fields[1].type: unknown type \"html\"", "fields[2].label: contains a text-direction",
		"fields[3].label: contains a zero-width", "fields[3].pattern: unknown pattern", "fields[4].key: duplicate key",
		"fields[4].label: is required", "fields[5].options: does not apply", "fields[5].min: must not be greater",
		"fields[5].step: must be a whole number", "fields[6].options[0].value", "fields[6].options[0].label: contains a symbol",
		"fields[7].default: a secret cannot have a default", "fields[8].minDuration", "fields[8].default",
		"fields[9].keyRule.pattern", "groups[0].fields[1]: field \"x\" is already", "groups[0].fields[2]: names no field",
		"actions: has 4 actions",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, strings.ReplaceAll(got, "; ", "\n"))
		}
	}
	if !strings.Contains(Check(Schema{Actions: s.Actions[:3]}).Error(), "actions[2].id: duplicate action") {
		t.Error("duplicate action not reported")
	}
}

func TestSizeLimits(t *testing.T) {
	many := Schema{}
	for i := range 10000 {
		many.Fields = append(many.Fields, Field{Key: fmt.Sprintf("f%d", i), Type: abi.TypeBool, Label: "f"})
	}
	if ps := Check(many); len(ps) != 1 || !strings.Contains(ps[0].Msg, "limit is 50") {
		t.Fatalf("%v", ps)
	}
	groups := Schema{Groups: make([]Group, 11)}
	if ps := Check(groups); len(ps) != 1 || ps[0].Path != "groups" {
		t.Fatalf("%v", ps)
	}
}

func TestDecodeRejectsUnknown(t *testing.T) {
	_, ps := Decode([]byte(`{"fields":[{"key":"a","type":"bool","label":"a","html":"<b>"}]}`))
	if len(ps) != 1 || !strings.Contains(ps[0].Msg, "unknown field") {
		t.Fatalf("%v", ps)
	}
}

func TestValidateValues(t *testing.T) {
	s := good()
	ok := map[string]any{"url": "http://box:9091", "apiKey": "k", "useSsl": true, "port": 9091.0, "ratio": 1.5,
		"mode": "fast", "every": "30s", "tags": []any{"a", "b"}, "ports": []any{1.0, 2.0},
		"paths": map[string]any{"/remote": "/local"}}
	if ps := Validate(s, ok); len(ps) != 0 {
		t.Fatalf("%v", ps)
	}
	bad := map[string]any{"url": "ftp://x", "useSsl": "yes", "port": 0.5, "ratio": 1.25, "mode": "turbo",
		"every": "1s", "tags": []any{"a", "B"}, "ports": []any{11.0}, "paths": []any{"relative=/x"}, "extra": 1}
	got := Validate(s, bad).Error()
	for _, want := range []string{"url: must be an http", "useSsl: must be true or false", "port: must be a whole number",
		"ratio: must be a multiple of 0.5", "mode: must be one of", "every: must be at least 5s",
		"tags: entry 2 must be an identifier", "ports: entry 1 must be at most 10", "paths: key \"relative\" must be an absolute",
		"extra: is not a setting"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if ps := Validate(s, map[string]any{}); len(ps) != 1 || ps[0].Path != "url" {
		t.Errorf("required: %v", ps)
	}
}

func TestValueEdges(t *testing.T) {
	f := Field{Key: "a", Type: abi.TypeString, Label: "a", MinLength: ptr(2), MaxLength: ptr(3), Hint: "two or three"}
	for v, want := range map[any]string{"a": "at least 2", "abcd": "at most 3", 5.0: "must be text", "a\x00": "NUL"} {
		if got := valueProblem(f, v); !strings.Contains(got, want) {
			t.Errorf("%v: %q", v, got)
		}
	}
	if ps := Validate(Schema{Fields: []Field{f}}, map[string]any{"a": "x"}); !strings.Contains(ps.Error(), "(two or three)") {
		t.Errorf("hint missing: %v", ps)
	}
	n := Field{Type: abi.TypeNumber, Decimals: ptr(1), Max: f64(5)}
	for v, want := range map[any]string{"x": "a number", 1.25: "decimals", 6.0: "at most 5", int64(3): "", 2: ""} {
		if got := valueProblem(n, v); (want == "") != (got == "") || !strings.Contains(got, want) {
			t.Errorf("%v: %q", v, got)
		}
	}
	d := Field{Type: abi.TypeDuration, MaxDuration: "1m"}
	for v, want := range map[any]string{time.Hour: "at most 1m", "bad": "duration like", time.Second: "", 4: "duration like"} {
		if got := valueProblem(d, v); (want == "") != (got == "") || !strings.Contains(got, want) {
			t.Errorf("%v: %q", v, got)
		}
	}
	m := Field{Type: abi.TypeMap, MaxEntries: 1, ValueRule: &abi.TextRule{MaxLength: ptr(1)}}
	for _, c := range []struct {
		v    any
		want string
	}{{"x", "key=value"}, {map[string]any{"a": 1.0}, "key=value"}, {map[string]string{"a": "b", "c": "d"}, "at most 1"},
		{[]string{"a=bc"}, "value of \"a\""}, {[]any{"noequals"}, "key=value"}, {map[string]string{"": "x"}, "at least 1"}} {
		if got := valueProblem(m, c.v); !strings.Contains(got, c.want) {
			t.Errorf("%v: %q", c.v, got)
		}
	}
	l := Field{Type: abi.TypeListString, MaxItems: 1}
	if got := valueProblem(l, "x"); got != "must be a list" {
		t.Error(got)
	}
	if got := valueProblem(l, []string{"a", "b"}); !strings.Contains(got, "at most 1") {
		t.Error(got)
	}
	if valueProblem(Field{Type: abi.TypeEnum}, 3) == "" || valueProblem(Field{Type: "other"}, 1) != "" {
		t.Error("enum/unknown")
	}
}

func TestPatterns(t *testing.T) {
	for p, cases := range map[string]map[string]bool{
		abi.PatternHostname:     {"example.org": true, "10.0.0.1": true, "-bad": false, "a b": false},
		abi.PatternURLHTTP:      {"https://a.b/x": true, "http://u:p@a": false, "javascript:alert(1)": false, "http://a#f": false},
		abi.PatternPathAbsolute: {"/data": true, `C:\dl`: true, "rel/x": false, "/a/../b": false, "/a\nb": false},
		abi.PatternIdentifier:   {"abc1": true, "1abc": false},
	} {
		for s, want := range cases {
			if got := CheckPattern(p, s) == nil; got != want {
				t.Errorf("%s %q: %v", p, s, got)
			}
		}
	}
	if CheckPattern("", "anything") != nil {
		t.Error("empty pattern")
	}
}

func TestPluginFieldsAndModuleValues(t *testing.T) {
	s := good()
	fields := PluginFields(s)
	if len(fields) != len(s.Fields) {
		t.Fatal(len(fields))
	}
	byKey := map[string]plugin.Field{}
	for _, f := range fields {
		byKey[f.Key] = f
	}
	if byKey["apiKey"].Type != plugin.FieldSecret || !byKey["url"].Required || byKey["mode"].Options[1] != "slow" {
		t.Errorf("%+v", byKey)
	}
	if byKey["paths"].Type != plugin.FieldList || byKey["paths"].Check("/a=/b") != nil || byKey["paths"].Check("a=b") == nil {
		t.Error("map entries")
	}
	if byKey["port"].Check("0.5") == nil || byKey["every"].Check("1s") == nil || byKey["url"].Check("ftp://x") == nil {
		t.Error("scalar checks")
	}
	if byKey["useSsl"].Check != nil || byKey["tags"].Default != nil {
		t.Error("bool check / list default")
	}
	vals := ModuleValues(s, map[string]any{"url": "http://a", "apiKey": "secret", "every": 30 * time.Second, "paths": []any{"/r=/l"}})
	if _, leaked := vals["apiKey"]; leaked || vals["every"] != "30s" || vals["port"] != 8080.0 || vals["paths"].(map[string]string)["/r"] != "/l" {
		t.Errorf("%v", vals)
	}
	if !SecretFields(s)["apiKey"] || !HasAction(s, "test") || HasAction(s, "x") {
		t.Error("helpers")
	}
	if _, ok := FieldOf(s, "nope"); ok {
		t.Error("FieldOf")
	}
}

// FuzzDecode: whatever a schema says, accepted text never contains markup
// that could escape (all text passed plaintext) and decoding never panics.
func FuzzDecode(f *testing.F) {
	b, _ := json.Marshal(good())
	f.Add(b)
	f.Add([]byte(`{"fields":[{"key":"a","type":"string","label":"\u202e"}]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		s, ps := Decode(data)
		if len(ps) != 0 {
			return
		}
		for _, fl := range s.Fields {
			if _, ok := typeRules[fl.Type]; !ok {
				t.Fatalf("unknown type accepted: %q", fl.Type)
			}
			if !IsIdentifier(fl.Key) {
				t.Fatalf("bad key accepted: %q", fl.Key)
			}
		}
		_ = Validate(s, map[string]any{"a": "x"})
	})
}
