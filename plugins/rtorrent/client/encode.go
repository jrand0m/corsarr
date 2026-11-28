package client

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"math"
	"sort"
	"strconv"
)

// XML-RPC values map onto Go values one way:
//
//	string            <string>
//	int, int32, int64 <i4> when it fits 32 bits, <i8> otherwise
//	bool              <boolean>
//	float64           <double>
//	[]byte            <base64>
//	[]any, []string   <array>
//	map[string]any    <struct> (members in key order)
//	nil               <nil/>
//
// Decoding yields string, int64, bool, float64, []byte, []any,
// map[string]any or nil; dateTime.iso8601 values stay strings.

// EncodeCall renders a methodCall document.
func EncodeCall(method string, params ...any) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(xml.Header)
	b.WriteString("<methodCall><methodName>")
	escape(&b, method)
	b.WriteString("</methodName><params>")
	for _, p := range params {
		b.WriteString("<param>")
		if err := writeValue(&b, p, 0); err != nil {
			return nil, err
		}
		b.WriteString("</param>")
	}
	b.WriteString("</params></methodCall>")
	return b.Bytes(), nil
}

// EncodeResponse renders a methodResponse document carrying one value.
func EncodeResponse(v any) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(xml.Header)
	b.WriteString("<methodResponse><params><param>")
	if err := writeValue(&b, v, 0); err != nil {
		return nil, err
	}
	b.WriteString("</param></params></methodResponse>")
	return b.Bytes(), nil
}

// EncodeFault renders a methodResponse document carrying a fault.
func EncodeFault(code int, message string) []byte {
	var b bytes.Buffer
	b.WriteString(xml.Header)
	b.WriteString("<methodResponse><fault>")
	_ = writeValue(&b, map[string]any{"faultCode": int64(code), "faultString": message}, 0)
	b.WriteString("</fault></methodResponse>")
	return b.Bytes()
}

func escape(b *bytes.Buffer, s string) { _ = xml.EscapeText(b, []byte(s)) }

func writeValue(b *bytes.Buffer, v any, depth int) error {
	if depth > maxValueDepth {
		return errorf(KindConfig, "value nests deeper than %d levels", maxValueDepth)
	}
	b.WriteString("<value>")
	if ok, err := writeScalar(b, v); ok || err != nil {
		if err != nil {
			return err
		}
		b.WriteString("</value>")
		return nil
	}
	if err := writeContainer(b, v, depth); err != nil {
		return err
	}
	b.WriteString("</value>")
	return nil
}

// writeScalar writes v when it is a scalar and reports whether it was one.
func writeScalar(b *bytes.Buffer, v any) (bool, error) {
	switch x := v.(type) {
	case nil:
		b.WriteString("<nil/>")
	case string:
		b.WriteString("<string>")
		escape(b, x)
		b.WriteString("</string>")
	case bool:
		b.WriteString("<boolean>" + map[bool]string{false: "0", true: "1"}[x] + "</boolean>")
	case float64:
		return true, writeDouble(b, x)
	case []byte:
		b.WriteString("<base64>" + base64.StdEncoding.EncodeToString(x) + "</base64>")
	default:
		return writeInteger(b, v), nil
	}
	return true, nil
}

// writeInteger writes v when it is an integer and reports whether it was one.
func writeInteger(b *bytes.Buffer, v any) bool {
	switch x := v.(type) {
	case int:
		writeInt(b, int64(x))
	case int32:
		writeInt(b, int64(x))
	case int64:
		writeInt(b, x)
	default:
		return false
	}
	return true
}

func writeInt(b *bytes.Buffer, n int64) {
	tag := "i8"
	if n >= math.MinInt32 && n <= math.MaxInt32 {
		tag = "i4"
	}
	b.WriteString("<" + tag + ">" + strconv.FormatInt(n, 10) + "</" + tag + ">")
}

func writeDouble(b *bytes.Buffer, f float64) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return errorf(KindConfig, "XML-RPC cannot carry %v", f)
	}
	b.WriteString("<double>" + strconv.FormatFloat(f, 'f', -1, 64) + "</double>")
	return nil
}

func writeContainer(b *bytes.Buffer, v any, depth int) error {
	switch x := v.(type) {
	case []string:
		items := make([]any, len(x))
		for i, s := range x {
			items[i] = s
		}
		return writeArray(b, items, depth)
	case []any:
		return writeArray(b, x, depth)
	case map[string]any:
		return writeStruct(b, x, depth)
	}
	return errorf(KindConfig, "XML-RPC cannot carry a %T", v)
}

func writeArray(b *bytes.Buffer, items []any, depth int) error {
	b.WriteString("<array><data>")
	for _, it := range items {
		if err := writeValue(b, it, depth+1); err != nil {
			return err
		}
	}
	b.WriteString("</data></array>")
	return nil
}

func writeStruct(b *bytes.Buffer, m map[string]any, depth int) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b.WriteString("<struct>")
	for _, k := range keys {
		b.WriteString("<member><name>")
		escape(b, k)
		b.WriteString("</name>")
		if err := writeValue(b, m[k], depth+1); err != nil {
			return err
		}
		b.WriteString("</member>")
	}
	b.WriteString("</struct>")
	return nil
}
