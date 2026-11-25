// Package charset converts site text between UTF-8 and the legacy
// single-byte encodings some trackers still use (windows-125x, ISO-8859-x,
// KOI8, IBM code pages). Only single-byte tables are compiled in, which keeps
// the binary small. Unknown labels leave text untouched.
package charset

import (
	"bytes"
	"mime"
	"regexp"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

var bom = []byte{0xEF, 0xBB, 0xBF}

// utf8Names are labels whose text needs no conversion.
var utf8Names = map[string]bool{"": true, "utf8": true, "usascii": true, "ascii": true}

// aliases map common short labels onto the normalised table names.
var aliases = map[string]string{
	"latin1": "iso88591", "latin2": "iso88592", "latin3": "iso88593", "latin4": "iso88594",
	"latin5": "iso88599", "latin6": "iso885910", "latin9": "iso885915", "cyrillic": "iso88595",
	"koi8": "koi8r", "macroman": "macintosh", "xmaccyrillic": "macintoshcyrillic",
}

// tables are the single-byte encodings by normalised name.
var tables = func() map[string]encoding.Encoding {
	m := map[string]encoding.Encoding{}
	for _, e := range charmap.All {
		if cm, ok := e.(*charmap.Charmap); ok {
			m[normalise(cm.String())] = cm
		}
	}
	return m
}()

// normalise lowercases a label and keeps only letters and digits.
func normalise(label string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(label) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// canonical rewrites the common spellings of a label onto a table name.
func canonical(n string) string {
	if a, ok := aliases[n]; ok {
		return a
	}
	for _, p := range []string{"windows", "win", "cp"} {
		if rest, ok := strings.CutPrefix(n, p); ok && strings.HasPrefix(rest, "125") {
			return "windows" + rest
		}
	}
	for _, p := range []string{"ibmcodepage", "ibm", "cp"} {
		if rest, ok := strings.CutPrefix(n, p); ok && rest != "" {
			return "ibmcodepage" + rest
		}
	}
	return n
}

// lookup returns the table for a label; utf8 is true when no conversion is needed.
func lookup(label string) (enc encoding.Encoding, utf8 bool) {
	n := normalise(label)
	if utf8Names[n] {
		return nil, true
	}
	return tables[canonical(n)], false
}

// Known reports whether text in this label can be converted (UTF-8 included).
func Known(label string) bool {
	enc, utf8 := lookup(label)
	return utf8 || enc != nil
}

// ToUTF8 decodes body from label. A UTF-8 byte-order mark wins over the
// label and is dropped; UTF-8, empty and unknown labels return body as is.
func ToUTF8(body []byte, label string) []byte {
	if rest, ok := bytes.CutPrefix(body, bom); ok {
		return rest
	}
	enc, _ := lookup(label)
	if enc == nil {
		return body
	}
	out, err := enc.NewDecoder().Bytes(body)
	if err != nil {
		return body
	}
	return out
}

// FromUTF8 encodes s in label, writing characters the encoding lacks as
// HTML numeric references (what browsers submit in that case). UTF-8, empty
// and unknown labels return s as is.
func FromUTF8(s, label string) string {
	enc, _ := lookup(label)
	if enc == nil {
		return s
	}
	out, err := encoding.HTMLEscapeUnsupported(enc.NewEncoder()).String(s)
	if err != nil {
		return s
	}
	return out
}

// FromContentType returns the charset parameter of a Content-Type header ("" when absent).
func FromContentType(ct string) string {
	_, params, err := mime.ParseMediaType(ct)
	if err != nil {
		return lenientCharset(ct)
	}
	return params["charset"]
}

// lenientCharset reads charset= from a header the strict parser rejects.
func lenientCharset(ct string) string {
	for _, p := range strings.Split(ct, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
		if ok && strings.EqualFold(k, "charset") {
			return strings.Trim(v, `"' `)
		}
	}
	return ""
}

// declScan is how far into a body an encoding declaration is looked for.
const declScan = 1024

var declared = regexp.MustCompile(`(?i)<(?:\?xml|meta)\b[^>]*?\b(?:encoding|charset)\s*=\s*["']?([a-z0-9_.:-]+)`)

// Declared returns the encoding an XML declaration or HTML meta tag near the
// start of body names ("" when none).
func Declared(body []byte) string {
	head := body[:min(len(body), declScan)]
	if m := declared.FindSubmatch(head); m != nil {
		return string(m[1])
	}
	return ""
}

// Detect picks the label for a response: the configured one, else the
// Content-Type charset, else the document's own declaration.
func Detect(configured, contentType string, body []byte) string {
	if configured != "" {
		return configured
	}
	if l := FromContentType(contentType); l != "" {
		return l
	}
	return Declared(body)
}
