package torznab

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/jrand0m/corsarr/internal/charset"
)

// cleanXML turns an indexer's answer into text the XML decoder accepts: it
// decodes a legacy encoding (Content-Type charset, else the XML declaration)
// to UTF-8, drops a byte-order mark, replaces invalid UTF-8 and removes the
// characters XML 1.0 forbids (most C0 controls).
func cleanXML(body []byte, contentType string) []byte {
	body = charset.ToUTF8(body, charset.Detect("", contentType, body))
	body = bytes.ToValidUTF8(body, []byte("\uFFFD"))
	body = bytes.Map(func(r rune) rune {
		if xmlChar(r) {
			return r
		}
		return -1
	}, body)
	return fixReferences(body)
}

// reference matches what may follow an "&": a named or numeric character reference.
var reference = regexp.MustCompile(`^&(#[0-9]{1,7}|#[xX][0-9a-fA-F]{1,6}|[A-Za-z][A-Za-z0-9]{0,31});`)

// fixReferences escapes every "&" that does not start a reference the
// decoder resolves (a bare "&" in "Tom & Jerry", an unknown entity name),
// and drops numeric references to characters XML forbids.
func fixReferences(body []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(body))
	for {
		i := bytes.IndexByte(body, '&')
		if i < 0 {
			out.Write(body)
			return out.Bytes()
		}
		out.Write(body[:i])
		body = body[i:]
		n, keep := referenceAt(body)
		switch {
		case n == 0:
			out.WriteString("&amp;")
			n = 1
		case keep:
			out.Write(body[:n])
		}
		body = body[n:]
	}
}

// referenceAt returns the length of a reference at the start of b (0 when
// there is none the decoder knows) and whether to keep it (false for a
// numeric reference to a forbidden character).
func referenceAt(b []byte) (int, bool) {
	m := reference.FindSubmatch(b)
	if m == nil {
		return 0, false
	}
	name := string(m[1])
	if name[0] != '#' {
		_, known := xml.HTMLEntity[name]
		if !known && !predefined[name] {
			return 0, false
		}
		return len(m[0]), true
	}
	r, err := numericRef(name[1:])
	return len(m[0]), err == nil && r <= unicode.MaxRune && xmlChar(r)
}

// predefined are the five entities every XML decoder knows.
var predefined = map[string]bool{"amp": true, "lt": true, "gt": true, "quot": true, "apos": true}

// numericRef parses the digits of "&#…;" (decimal) or "&#x…;" (hex).
func numericRef(digits string) (rune, error) {
	base := 10
	if hex, ok := strings.CutPrefix(strings.ToLower(digits), "x"); ok {
		digits, base = hex, 16
	}
	n, err := strconv.ParseInt(digits, base, 32)
	return rune(n), err
}

// xmlChar reports whether XML 1.0 allows r in a document.
func xmlChar(r rune) bool {
	if r == '\t' || r == '\n' || r == '\r' {
		return true
	}
	return r >= 0x20 && (r < 0xD800 || r > 0xDFFF) && r != 0xFFFE && r != 0xFFFF
}

// decoder reads cleaned text: HTML named entities such as &nbsp; resolve,
// and the declared encoding is ignored because cleanXML already produced
// UTF-8. Structure stays strict so a broken item is noticed, not guessed at.
func decoder(body []byte) *xml.Decoder {
	d := xml.NewDecoder(bytes.NewReader(body))
	d.Entity = xml.HTMLEntity
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	return d
}

// unmarshal decodes cleaned text into v with the lenient decoder.
func unmarshal(body []byte, v any) error { return decoder(body).Decode(v) }

// itemChunks splits a feed into its <item>…</item> elements, so a single
// malformed item can be skipped while the rest of the page is read.
func itemChunks(body []byte) [][]byte {
	var out [][]byte
	for {
		start := itemStart(body)
		if start < 0 {
			return out
		}
		end := bytes.Index(body[start:], []byte("</item>"))
		if end < 0 {
			return out
		}
		end += start + len("</item>")
		out = append(out, body[start:end])
		body = body[end:]
	}
}

// itemStart finds the next "<item>" or "<item …>" opening tag.
func itemStart(body []byte) int {
	off := 0
	for {
		i := bytes.Index(body[off:], []byte("<item"))
		if i < 0 {
			return -1
		}
		i += off
		if next := i + len("<item"); next < len(body) && strings.ContainsRune("> \t\r\n", rune(body[next])) {
			return i
		}
		off = i + 1
	}
}

// itemsOneByOne decodes each item on its own and keeps those that parse.
func itemsOneByOne(body []byte) []rssItem {
	var out []rssItem
	for _, chunk := range itemChunks(body) {
		var it rssItem
		if unmarshal(chunk, &it) == nil {
			out = append(out, it)
		}
	}
	return out
}
