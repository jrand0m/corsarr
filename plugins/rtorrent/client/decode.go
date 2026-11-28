package client

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
)

// Decoding limits. The decoder never expands entities beyond the five XML
// built-ins and character references, refuses any DOCTYPE (so no internal or
// external entity can be declared), caps element nesting and refuses
// documents larger than the size cap before parsing.
const (
	// maxElementDepth caps element nesting in a decoded document.
	maxElementDepth = 32
	// maxValueDepth caps array/struct nesting in an encoded value.
	maxValueDepth = 16
	// DefaultMaxResponse is the default cap on one XML-RPC response body.
	DefaultMaxResponse = 64 << 20
)

func malformed(format string, args ...any) *Error { return errorf(KindParse, format, args...) }

// DecodeResponse parses a methodResponse document of at most limit bytes
// (DefaultMaxResponse when limit <= 0). A fault answer comes back as a *Fault
// error.
func DecodeResponse(doc []byte, limit int) (any, error) {
	r, err := newReader(doc, limit)
	if err != nil {
		return nil, err
	}
	if err := r.open("methodResponse"); err != nil {
		return nil, err
	}
	start, err := r.start()
	if err != nil {
		return nil, err
	}
	var v any
	switch start {
	case "params":
		v, err = r.params()
	case "fault":
		v, err = r.fault()
	default:
		err = malformed("unexpected <%s> in methodResponse", start)
	}
	if err != nil {
		return nil, err
	}
	return v, r.finish()
}

// DecodeCall parses a methodCall document of at most limit bytes.
func DecodeCall(doc []byte, limit int) (string, []any, error) {
	r, err := newReader(doc, limit)
	if err != nil {
		return "", nil, err
	}
	if err := r.open("methodCall"); err != nil {
		return "", nil, err
	}
	if err := r.open("methodName"); err != nil {
		return "", nil, err
	}
	method, err := r.text()
	if err != nil {
		return "", nil, err
	}
	params, err := r.callParams()
	if err != nil {
		return "", nil, err
	}
	return strings.TrimSpace(method), params, r.finish()
}

type reader struct {
	d     *xml.Decoder
	depth int
}

func newReader(doc []byte, limit int) (*reader, error) {
	if limit <= 0 {
		limit = DefaultMaxResponse
	}
	if len(doc) > limit {
		return nil, malformed("document of %d bytes exceeds the %d byte limit", len(doc), limit)
	}
	d := xml.NewDecoder(bytes.NewReader(doc))
	d.Strict = true
	d.Entity = nil        // only the five XML built-ins
	d.CharsetReader = nil // UTF-8 only
	return &reader{d: d}, nil
}

// token returns the next element or text token; comments and processing
// instructions are skipped, a DOCTYPE or other directive is refused.
func (r *reader) token() (xml.Token, error) {
	for {
		t, err := r.d.Token()
		if err != nil {
			return nil, malformed("bad XML: %v", err)
		}
		switch v := t.(type) {
		case xml.StartElement:
			r.depth++
			if r.depth > maxElementDepth {
				return nil, malformed("document nests deeper than %d elements", maxElementDepth)
			}
			return v, nil
		case xml.EndElement:
			r.depth--
			return v, nil
		case xml.CharData:
			return v.Copy(), nil
		case xml.Directive:
			return nil, malformed("document declarations are not accepted")
		}
	}
}

// element skips blank text and returns the next start or end tag.
func (r *reader) element() (xml.Token, error) {
	for {
		t, err := r.token()
		if err != nil {
			return nil, err
		}
		cd, isText := t.(xml.CharData)
		if !isText {
			return t, nil
		}
		if len(bytes.TrimSpace(cd)) > 0 {
			return nil, malformed("unexpected text %q", truncate(string(cd)))
		}
	}
}

// start returns the name of the next element, which must be an opening tag.
func (r *reader) start() (string, error) {
	t, err := r.element()
	if err != nil {
		return "", err
	}
	se, ok := t.(xml.StartElement)
	if !ok {
		return "", malformed("unexpected closing tag")
	}
	return se.Name.Local, nil
}

// next returns the name of the next opening tag, or "" at a closing tag.
func (r *reader) next() (string, error) {
	t, err := r.element()
	if err != nil {
		return "", err
	}
	if se, ok := t.(xml.StartElement); ok {
		return se.Name.Local, nil
	}
	return "", nil
}

func (r *reader) open(name string) error {
	got, err := r.start()
	if err == nil && got != name {
		err = malformed("expected <%s>, found <%s>", name, got)
	}
	return err
}

// close consumes the closing tag of the current element (the decoder
// already checks that it matches).
func (r *reader) close() error {
	t, err := r.element()
	if err != nil {
		return err
	}
	if _, ok := t.(xml.EndElement); !ok {
		return malformed("unexpected <%s>", t.(xml.StartElement).Name.Local)
	}
	return nil
}

// text reads character data up to the closing tag of the current element.
func (r *reader) text() (string, error) {
	var sb strings.Builder
	for {
		t, err := r.token()
		if err != nil {
			return "", err
		}
		switch v := t.(type) {
		case xml.CharData:
			sb.Write(v)
		case xml.StartElement:
			return "", malformed("unexpected <%s> in text", v.Name.Local)
		default:
			return sb.String(), nil
		}
	}
}

// finish checks that nothing but blanks, comments or processing
// instructions follow the root element.
func (r *reader) finish() error {
	for {
		t, err := r.d.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return malformed("bad XML after the document: %v", err)
		}
		if !trailingOK(t) {
			return malformed("content after the document")
		}
	}
}

func trailingOK(t xml.Token) bool {
	switch v := t.(type) {
	case xml.CharData:
		return len(bytes.TrimSpace(v)) == 0
	case xml.Comment, xml.ProcInst:
		return true
	}
	return false
}

// params reads the inside of <params> in a response: one value, or none.
func (r *reader) params() (any, error) {
	name, err := r.next()
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, r.close()
	}
	if name != "param" {
		return nil, malformed("unexpected <%s> in params", name)
	}
	v, err := r.param()
	if err != nil {
		return nil, err
	}
	if err := r.close(); err != nil {
		return nil, err
	}
	return v, r.close()
}

// param reads <value>…</value></param> after <param>.
func (r *reader) param() (any, error) {
	if err := r.open("value"); err != nil {
		return nil, err
	}
	v, err := r.value()
	if err != nil {
		return nil, err
	}
	return v, r.close()
}

// callParams reads the optional <params> of a call up to </methodCall>.
func (r *reader) callParams() ([]any, error) {
	out := []any{}
	name, err := r.next()
	if err != nil || name == "" {
		return out, err
	}
	if name != "params" {
		return nil, malformed("unexpected <%s> in methodCall", name)
	}
	for {
		name, err := r.next()
		switch {
		case err != nil:
			return nil, err
		case name == "":
			return out, r.close()
		case name != "param":
			return nil, malformed("unexpected <%s> in params", name)
		}
		v, err := r.param()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
}

// fault reads <value>…</value></fault></methodResponse> and returns the fault.
func (r *reader) fault() (any, error) {
	if err := r.open("value"); err != nil {
		return nil, err
	}
	v, err := r.value()
	if err != nil {
		return nil, err
	}
	if err := r.close(); err != nil {
		return nil, err
	}
	if err := r.close(); err != nil {
		return nil, err
	}
	m, _ := v.(map[string]any)
	code, _ := m["faultCode"].(int64)
	msg, _ := m["faultString"].(string)
	return nil, &Fault{Code: int(code), Message: msg}
}

// value reads the inside of <value> up to and including </value>. Text
// without a type element is a string.
func (r *reader) value() (any, error) {
	var text strings.Builder
	for {
		t, err := r.token()
		if err != nil {
			return nil, err
		}
		switch v := t.(type) {
		case xml.CharData:
			text.Write(v)
		case xml.EndElement:
			return text.String(), nil
		case xml.StartElement:
			return r.typedValue(v.Name.Local, text.String())
		}
	}
}

func (r *reader) typedValue(name, before string) (any, error) {
	if strings.TrimSpace(before) != "" {
		return nil, malformed("text before <%s> in a value", name)
	}
	v, err := r.typed(name)
	if err != nil {
		return nil, err
	}
	return v, r.close()
}

var scalars = map[string]func(string) (any, error){
	"string":           func(s string) (any, error) { return s, nil },
	"int":              parseInt,
	"i4":               parseInt,
	"i8":               parseInt,
	"boolean":          parseBool,
	"double":           parseDouble,
	"base64":           parseBase64,
	"dateTime.iso8601": func(s string) (any, error) { return strings.TrimSpace(s), nil },
	"nil":              parseNil,
}

// typed reads one typed element after its opening tag, through its closing tag.
func (r *reader) typed(name string) (any, error) {
	switch name {
	case "array":
		return r.array()
	case "struct":
		return r.structure()
	}
	conv, ok := scalars[name]
	if !ok {
		return nil, malformed("unknown value type <%s>", name)
	}
	s, err := r.text()
	if err != nil {
		return nil, err
	}
	return conv(s)
}

func (r *reader) array() (any, error) {
	if err := r.open("data"); err != nil {
		return nil, err
	}
	out := []any{}
	for {
		name, err := r.next()
		switch {
		case err != nil:
			return nil, err
		case name == "":
			return out, r.close()
		case name != "value":
			return nil, malformed("unexpected <%s> in array", name)
		}
		v, err := r.value()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
}

func (r *reader) structure() (any, error) {
	out := map[string]any{}
	for {
		name, err := r.next()
		switch {
		case err != nil:
			return nil, err
		case name == "":
			return out, nil
		case name != "member":
			return nil, malformed("unexpected <%s> in struct", name)
		}
		key, v, err := r.member()
		if err != nil {
			return nil, err
		}
		out[key] = v
	}
}

func (r *reader) member() (string, any, error) {
	if err := r.open("name"); err != nil {
		return "", nil, err
	}
	key, err := r.text()
	if err != nil {
		return "", nil, err
	}
	v, err := r.param() // <value>…</value></member>
	return key, v, err
}

func parseInt(s string) (any, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return nil, malformed("bad integer %q", truncate(s))
	}
	return n, nil
}

func parseBool(s string) (any, error) {
	switch strings.TrimSpace(s) {
	case "1":
		return true, nil
	case "0":
		return false, nil
	}
	return nil, malformed("bad boolean %q", truncate(s))
}

func parseDouble(s string) (any, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return nil, malformed("bad double %q", truncate(s))
	}
	return f, nil
}

func parseBase64(s string) (any, error) {
	clean := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, s)
	b, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		return nil, malformed("bad base64 value")
	}
	return b, nil
}

func parseNil(s string) (any, error) {
	if strings.TrimSpace(s) != "" {
		return nil, malformed("nil with content")
	}
	return nil, nil
}

func truncate(s string) string {
	if len(s) > 40 {
		return s[:40] + "…"
	}
	return s
}
