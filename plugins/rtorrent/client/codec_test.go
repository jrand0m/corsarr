package client

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	values := []any{
		"plain", "<&>\"' ünïcode", int64(42), int64(-7), int64(math.MaxInt64), true, false, 1.5, []byte{0, 1, 2, 255},
		nil, []any{}, []any{"a", int64(1), []any{true}},
		map[string]any{"b": int64(2), "a": "x", "nested": map[string]any{"k": []any{nil}}},
	}
	for _, v := range values {
		doc, err := EncodeResponse(v)
		must(t, "encode", err)
		got, err := DecodeResponse(doc, 0)
		must(t, "decode "+string(doc), err)
		if !reflect.DeepEqual(got, v) {
			t.Errorf("round trip %#v → %#v", v, got)
		}
	}
}

func TestEncodeCallTypes(t *testing.T) {
	doc, err := EncodeCall("m.x", 1, int32(2), int64(1<<40), []string{"a"}, "x&y")
	must(t, "encode", err)
	s := string(doc)
	for _, want := range []string{"<methodName>m.x</methodName>", "<i4>1</i4>", "<i4>2</i4>", "<i8>1099511627776</i8>",
		"<array><data><value><string>a</string></value></data></array>", "x&amp;y"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in %s", want, s)
		}
	}
	method, params, err := DecodeCall(doc, 0)
	must(t, "decode call", err)
	if method != "m.x" || len(params) != 5 || params[2] != int64(1<<40) {
		t.Errorf("%s %v", method, params)
	}
}

func TestEncodeRejects(t *testing.T) {
	deep := any("x")
	for range maxValueDepth + 2 {
		deep = []any{deep}
	}
	for _, v := range []any{math.NaN(), math.Inf(1), struct{}{}, deep, map[string]any{"x": make(chan int)}, []any{uint(1)}} {
		if _, err := EncodeCall("m", v); KindOf(err) != KindConfig {
			t.Errorf("%T accepted: %v", v, err)
		}
		if _, err := EncodeResponse(v); err == nil {
			t.Errorf("%T accepted in a response", v)
		}
	}
}

func resp(value string) []byte {
	return []byte(`<?xml version="1.0"?><methodResponse><params><param><value>` + value +
		`</value></param></params></methodResponse>`)
}

func TestDecodeForms(t *testing.T) {
	cases := map[string]any{
		"untyped text":                  "untyped text",
		"<int> 5 </int>":                int64(5),
		"<i4>-3</i4>":                   int64(-3),
		"<ex:i8 xmlns:ex='x'>9</ex:i8>": int64(9),
		"<boolean>1</boolean>":          true,
		"<double>-0.25</double>":        -0.25,
		"<base64>AAEC\n/w==</base64>":   []byte{0, 1, 2, 255},
		"<dateTime.iso8601>20260101T00:00:00</dateTime.iso8601>": "20260101T00:00:00",
		"<nil/>":                             nil,
		"<string>&#x263A; &lt;</string>":     "☺ <",
		"<struct></struct>":                  map[string]any{},
		"<array><data>\n</data></array>":     []any{},
		"<string><![CDATA[<raw>]]></string>": "<raw>",
	}
	for in, want := range cases {
		got, err := DecodeResponse(resp(in), 0)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s → %#v %v", in, got, err)
		}
	}
	// Comments and processing instructions are ignored anywhere.
	doc := []byte(`<?xml version="1.0"?><!-- c --><methodResponse><params><param><value><i4>1</i4></value></param></params></methodResponse><!-- t --><?pi x?>` + "\n")
	if v, err := DecodeResponse(doc, 0); err != nil || v != int64(1) {
		t.Errorf("comments: %v %v", v, err)
	}
	empty := []byte(`<methodResponse><params></params></methodResponse>`)
	if v, err := DecodeResponse(empty, 0); err != nil || v != nil {
		t.Errorf("empty params: %v %v", v, err)
	}
}

func TestDecodeFault(t *testing.T) {
	_, err := DecodeResponse(EncodeFault(-501, "Could not find info-hash."), 0)
	var f *Fault
	if !errors.As(err, &f) || f.Code != -501 || f.Message != "Could not find info-hash." {
		t.Fatalf("fault %v", err)
	}
	if classifyFault("d.x", f).Kind != KindNotFound {
		t.Error("info-hash fault not classified as not found")
	}
}

// TestDecodeHostile feeds documents that must be refused without expanding,
// recursing or allocating without bound.
func TestDecodeHostile(t *testing.T) {
	laughs := `<?xml version="1.0"?><!DOCTYPE lolz [<!ENTITY lol "lol"><!ENTITY lol2 "&lol;&lol;&lol;&lol;&lol;&lol;&lol;&lol;&lol;&lol;">` +
		`<!ENTITY lol3 "&lol2;&lol2;&lol2;&lol2;&lol2;&lol2;&lol2;&lol2;&lol2;&lol2;">]>` +
		`<methodResponse><params><param><value><string>&lol3;</string></value></param></params></methodResponse>`
	external := `<?xml version="1.0"?><!DOCTYPE r [<!ENTITY x SYSTEM "file:///etc/passwd">]>` + string(resp("<string>&x;</string>"))
	deep := strings.Repeat("<array><data><value>", 20) + "x" + strings.Repeat("</value></data></array>", 20)
	cases := map[string][]byte{
		"billion laughs":     []byte(laughs),
		"external entity":    []byte(external),
		"undeclared entity":  resp("<string>&lol;</string>"),
		"deep nesting":       resp(deep),
		"unclosed":           []byte(`<methodResponse><params><param><value><string>x`),
		"mismatched":         []byte(`<methodResponse><params></param></methodResponse>`),
		"wrong root":         []byte(`<methodCall></methodCall>`),
		"unknown type":       resp("<bigint>1</bigint>"),
		"bad int":            resp("<i4>x</i4>"),
		"bad bool":           resp("<boolean>yes</boolean>"),
		"bad double":         resp("<double>1,5</double>"),
		"bad base64":         resp("<base64>!!</base64>"),
		"nil content":        resp("<nil>x</nil>"),
		"mixed value":        resp("x<string>y</string>"),
		"element in text":    resp("<string>a<b/></string>"),
		"two types":          resp("<string>a</string><string>b</string>"),
		"text in array":      resp("<array>x<data></data></array>"),
		"wrong array child":  resp("<array><data><item/></data></array>"),
		"wrong struct child": resp("<struct><value/></struct>"),
		"member order":       resp("<struct><member><value>1</value><name>a</name></member></struct>"),
		"trailing element":   append(resp("1"), []byte("<x/>")...),
		"trailing text":      append(resp("1"), []byte("junk")...),
		"two params":         []byte(`<methodResponse><params><param><value>1</value></param><param><value>2</value></param></params></methodResponse>`),
		"wrong params child": []byte(`<methodResponse><params><x/></params></methodResponse>`),
		"wrong response":     []byte(`<methodResponse><other/></methodResponse>`),
		"closing first":      []byte(`<methodResponse></methodResponse>`),
		"bad fault":          []byte(`<methodResponse><fault><x/></fault></methodResponse>`),
		"fault no value":     []byte(`<methodResponse><fault><value><bad/></value></fault></methodResponse>`),
		"fault trailing":     []byte(`<methodResponse><fault><value>1</value><x/></fault></methodResponse>`),
		"fault unclosed":     []byte(`<methodResponse><fault><value>1</value></fault><x/></methodResponse>`),
		"param unclosed":     []byte(`<methodResponse><params><param><value>1</value><x/></param></params></methodResponse>`),
		"params unclosed":    []byte(`<methodResponse><params><param><value>1</value></param><x/></params></methodResponse>`),
		"param no value":     []byte(`<methodResponse><params><param><x/></param></params></methodResponse>`),
		"array no data":      resp("<array><x/></array>"),
		"array unclosed":     resp("<array><data></data><x/></array>"),
		"bad array value":    resp("<array><data><value><bad/></value></data></array>"),
		"member bad name":    resp("<struct><member><name><x/></name></member></struct>"),
		"bad member value":   resp("<struct><member><name>a</name><value><bad/></value></member></struct>"),
		"non-utf8":           []byte(`<?xml version="1.0" encoding="ISO-8859-1"?><methodResponse/>`),
		"empty":              {},
	}
	for name, doc := range cases {
		if _, err := DecodeResponse(doc, 0); KindOf(err) != KindParse {
			t.Errorf("%s: want parse error, got %v", name, err)
		}
	}
}

func TestDecodeSizeCap(t *testing.T) {
	huge := resp("<string>" + strings.Repeat("A", 2<<20) + "</string>")
	if _, err := DecodeResponse(huge, 1<<20); KindOf(err) != KindParse {
		t.Fatalf("oversized document: %v", err)
	}
	if v, err := DecodeResponse(huge, 4<<20); err != nil || len(v.(string)) != 2<<20 {
		t.Fatalf("within cap: %v", err)
	}
}

func TestDecodeCallErrors(t *testing.T) {
	cases := []string{
		``,
		`<methodResponse/>`,
		`<methodCall><params/></methodCall>`,
		`<methodCall><methodName><x/></methodName></methodCall>`,
		`<methodCall><methodName>m</methodName><x/></methodCall>`,
		`<methodCall><methodName>m</methodName><params><x/></params></methodCall>`,
		`<methodCall><methodName>m</methodName><params><param><bad/></param></params></methodCall>`,
		`<methodCall><methodName>m</methodName><params></params><x/></methodCall>`,
		`<methodCall><methodName>m</methodName><params><param><value>1</value></param>`,
		`<methodCall><methodName>m</methodName></methodCall><x/>`,
		`<methodCall><methodName>m`,
	}
	for _, c := range cases {
		if _, _, err := DecodeCall([]byte(c), 0); err == nil {
			t.Errorf("accepted %s", c)
		}
	}
	if _, _, err := DecodeCall([]byte("<methodCall>"), 3); err == nil {
		t.Error("size cap ignored")
	}
	m, p, err := DecodeCall([]byte(`<methodCall><methodName> m </methodName></methodCall>`), 0)
	if err != nil || m != "m" || len(p) != 0 {
		t.Errorf("no params: %q %v %v", m, p, err)
	}
}

func TestHashes(t *testing.T) {
	if h, err := MagnetHash(magnet); err != nil || h != strings.ToUpper(magnetHex) {
		t.Errorf("hex magnet %s %v", h, err)
	}
	if h, err := MagnetHash("magnet:?xt=urn:btih:AERUKZ4JVPG66AJDIVTYTK6N54ASGRLH"); err != nil || h != "0123456789ABCDEF0123456789ABCDEF01234567" {
		t.Errorf("base32 magnet %s %v", h, err)
	}
	for _, bad := range []string{"http://x", "magnet:?dn=x", "magnet:?xt=urn:btih:zz", "magnet:?xt=urn:btih:" + strings.Repeat("z", 40), "%"} {
		if _, err := MagnetHash(bad); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	if h, err := TorrentHash(torrentFile); err != nil || h != fixtureHash() {
		t.Errorf("torrent hash %s %v", h, err)
	}
	bad := []string{"", "x", "d4:infoi1", "d3:fooi1e", "d3:fool", "d4:info", "d3:foox", "d99:x", "d3:fooi1ee",
		"d3:foo" + strings.Repeat("l", 40) + strings.Repeat("e", 40) + "4:infoi1ee", "dx", "d3:fool"}
	for _, b := range bad {
		if _, err := TorrentHash([]byte(b)); err == nil {
			t.Errorf("accepted %q", b)
		}
	}
	if h, err := TorrentHash([]byte("d3:fooi1e4:infod1:xi1eee")); err != nil || len(h) != 40 {
		t.Errorf("nested %s %v", h, err)
	}
}
