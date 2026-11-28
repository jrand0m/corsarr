package mcp

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/netip"
	"os"
	"slices"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
)

// validate checks a decoded JSON value against the subset of JSON Schema
// the output schemas use: type (with null), enum, properties, required,
// additionalProperties, items and anyOf.
func validate(s map[string]any, v any, path string) []string {
	var errs []string
	if t, ok := s["type"]; ok && !typeMatches(t, v) {
		return []string{fmt.Sprintf("%s: %T is not %v", path, v, t)}
	}
	if alts, ok := s["anyOf"].([]any); ok && !anyMatches(alts, v, path) {
		errs = append(errs, path+": matches none of anyOf")
	}
	if enum, ok := s["enum"].([]any); ok && !slices.Contains(enum, v) {
		errs = append(errs, fmt.Sprintf("%s: %v not in %v", path, v, enum))
	}
	switch val := v.(type) {
	case map[string]any:
		errs = append(errs, validateObject(s, val, path)...)
	case []any:
		if items, ok := s["items"].(map[string]any); ok {
			for i, it := range val {
				errs = append(errs, validate(items, it, fmt.Sprintf("%s[%d]", path, i))...)
			}
		}
	}
	return errs
}

func anyMatches(alts []any, v any, path string) bool {
	for _, a := range alts {
		if len(validate(a.(map[string]any), v, path)) == 0 {
			return true
		}
	}
	return false
}

func validateObject(s map[string]any, val map[string]any, path string) []string {
	var errs []string
	props, _ := s["properties"].(map[string]any)
	for _, r := range asStrings(s["required"]) {
		if _, ok := val[r]; !ok {
			errs = append(errs, path+": missing "+r)
		}
	}
	for k, fv := range val {
		if ps, ok := props[k].(map[string]any); ok {
			errs = append(errs, validate(ps, fv, path+"."+k)...)
			continue
		}
		switch ap := s["additionalProperties"].(type) {
		case bool:
			if !ap && props != nil {
				errs = append(errs, path+": unexpected "+k)
			}
		case map[string]any:
			errs = append(errs, validate(ap, fv, path+"."+k)...)
		}
	}
	return errs
}

func asStrings(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, x := range list {
		out = append(out, x.(string))
	}
	return out
}

func typeMatches(t, v any) bool {
	if one, ok := t.(string); ok {
		return isType(one, v)
	}
	for _, x := range t.([]any) {
		if isType(x.(string), v) {
			return true
		}
	}
	return false
}

func isType(t string, v any) bool {
	switch t {
	case "null":
		return v == nil
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "integer":
		f, ok := v.(float64)
		return ok && f == math.Trunc(f)
	case "number":
		_, ok := v.(float64)
		return ok
	}
	return false
}

// decoded turns a Go value into its decoded JSON form.
func decoded(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// validated are the tools whose structuredContent a test has checked.
var validated sync.Map

// TestMain runs the tests and, for a full run, requires every tool's
// output to have been checked against its schema at least once.
func TestMain(m *testing.M) {
	flag.Parse()
	code := m.Run()
	if f := flag.Lookup("test.run"); code == 0 && f != nil && f.Value.String() == "" {
		code = missingOutputs()
	}
	os.Exit(code)
}

func missingOutputs() int {
	h := &harness{modes: map[domain.MediaKind]domain.KindMode{}}
	h.setMCP(config.MCP{Enabled: true})
	s := New(Deps{Config: h.cfg.Load, Downloaders: &fakeDownloaders{}})
	code := 0
	for _, tool := range s.Tools() {
		if _, ok := validated.Load(tool.Name); !ok && tool.Name != "request_book" { // books always refuse in this build
			fmt.Fprintf(os.Stderr, "no test checks the structuredContent of %s against its outputSchema\n", tool.Name)
			code = 1
		}
	}
	return code
}

// checkOutput validates a tool's structuredContent against its schema.
func checkOutput(t *testing.T, tool *Tool, data any) {
	t.Helper()
	if tool == nil || data == nil {
		return
	}
	validated.Store(tool.Name, true)
	s := decoded(t, tool.Output).(map[string]any)
	if errs := validate(s, decoded(t, data), tool.Name); len(errs) > 0 {
		sort.Strings(errs)
		t.Errorf("%s: structuredContent does not match outputSchema:\n%v", tool.Name, errs)
	}
}

func TestEveryToolDeclaresAnObjectOutputSchema(t *testing.T) {
	h := newHarness(t)
	for _, tool := range h.s.Tools() {
		if tool.Output == nil || tool.Output["type"] != "object" {
			t.Errorf("%s: outputSchema must be an object schema, got %v", tool.Name, tool.Output)
		}
		if _, err := json.Marshal(tool.info()); err != nil {
			t.Errorf("%s: %v", tool.Name, err)
		}
	}
	out := h.rpc(t, "tools/list", nil)
	var list struct {
		Tools []struct {
			Name   string         `json:"name"`
			Output map[string]any `json:"outputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(out.Result, &list); err != nil || len(list.Tools) == 0 {
		t.Fatal(err)
	}
	for _, tl := range list.Tools {
		if tl.Output["type"] != "object" {
			t.Errorf("%s is listed without its outputSchema", tl.Name)
		}
	}
}

type schemaSample struct {
	Name     string            `json:"name"`
	Count    int               `json:"count,omitempty"`
	Ratio    float64           `json:"ratio"`
	At       time.Time         `json:"at"`
	Tags     []string          `json:"tags"`
	Opt      *int              `json:"opt,omitempty"`
	Labels   map[string]string `json:"labels"`
	Any      any               `json:"any"`
	Kind     domain.MediaKind  `json:"kind"`
	Raw      json.RawMessage   `json:"raw"`
	Self     *schemaSample     `json:"self,omitempty"`
	Hidden   string            `json:"-"`
	Untagged bool
	Addr     netip.Addr `json:"addr"`
	Bytes    []byte     `json:"bytes,omitempty"`
	unexp    int
	schemaEmbedded
	*schemaPtrEmbedded
}

type schemaEmbedded struct {
	Inner string `json:"inner"`
}

type schemaPtrEmbedded struct {
	Maybe string `json:"maybe"`
}

func TestOutputSchemaFollowsTheJSONEncoding(t *testing.T) {
	s := outputOf[schemaSample]()
	props := s["properties"].(map[string]schema)
	for _, want := range []string{"name", "ratio", "at", "tags", "labels", "any", "raw", "Untagged", "inner"} {
		if !slices.Contains(s["required"].([]string), want) {
			t.Errorf("%s should be required: %v", want, s["required"])
		}
	}
	for _, opt := range []string{"count", "opt", "self", "maybe"} {
		if slices.Contains(s["required"].([]string), opt) {
			t.Errorf("%s is optional", opt)
		}
	}
	if _, ok := props["Hidden"]; ok || props["unexp"] != nil {
		t.Fatal("skipped fields stay out")
	}
	if props["addr"]["type"] != "string" || props["bytes"]["type"] != "string" {
		t.Fatalf("text and bytes encode as strings: %v %v", props["addr"], props["bytes"])
	}
	if props["at"]["format"] != "date-time" || props["self"]["type"] != "object" || len(props["raw"]) != 0 {
		t.Fatalf("%v %v %v", props["at"], props["self"], props["raw"])
	}
	if fmt.Sprint(props["tags"]["type"]) != "[array null]" || props["count"]["type"] != "integer" || props["ratio"]["type"] != "number" {
		t.Fatalf("%v %v %v", props["tags"], props["count"], props["ratio"])
	}
	n := 2
	ok := schemaSample{Name: "x", Tags: []string{"a"}, Opt: &n, Raw: json.RawMessage(`{"a":1}`), schemaEmbedded: schemaEmbedded{Inner: "i"}}
	if errs := validate(decoded(t, s).(map[string]any), decoded(t, ok), "sample"); len(errs) > 0 {
		t.Fatal(errs)
	}
	bad := map[string]any{"name": 1, "extra": true}
	if errs := validate(decoded(t, s).(map[string]any), bad, "sample"); len(errs) == 0 {
		t.Fatal("a wrong value is caught")
	}
	alt := decoded(t, oneOfOutputs(outputOf[TestOut](), outputOf[StartedOut]())).(map[string]any)
	if len(validate(alt, map[string]any{"downloader": "q", "ok": true}, "alt")) > 0 || len(validate(alt, map[string]any{"x": 1}, "alt")) == 0 {
		t.Fatal("anyOf")
	}
}
