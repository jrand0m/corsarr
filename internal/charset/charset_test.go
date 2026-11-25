package charset

import (
	"net/url"
	"testing"
)

// "Привет" in windows-1251 and KOI8-R.
var (
	cp1251 = []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2}
	koi8r  = []byte{0xF0, 0xD2, 0xC9, 0xD7, 0xC5, 0xD4}
)

func TestKnownLabels(t *testing.T) {
	for _, l := range []string{"windows-1251", "Windows-1251", "cp1251", "WIN1251", "iso-8859-1", "latin1", "ISO_8859-5",
		"koi8-r", "cp866", "ibm866", "utf-8", "UTF8", "us-ascii", ""} {
		if !Known(l) {
			t.Errorf("%q should be known", l)
		}
	}
	for _, l := range []string{"shift_jis", "klingon", "utf-16"} {
		if Known(l) {
			t.Errorf("%q should be unknown", l)
		}
	}
}

func TestToUTF8(t *testing.T) {
	cases := []struct {
		name  string
		body  []byte
		label string
		want  string
	}{
		{"windows-1251", cp1251, "windows-1251", "Привет"},
		{"koi8-r", koi8r, "KOI8-R", "Привет"},
		{"latin1", []byte{'c', 'a', 'f', 0xE9}, "iso-8859-1", "café"},
		{"utf-8 unchanged", []byte("Привет"), "utf-8", "Привет"},
		{"empty label unchanged", []byte("plain"), "", "plain"},
		{"unknown label unchanged", []byte("plain"), "klingon", "plain"},
		{"utf-8 BOM dropped", append([]byte{0xEF, 0xBB, 0xBF}, "x"...), "", "x"},
		{"BOM wins over a wrong label", append([]byte{0xEF, 0xBB, 0xBF}, "é"...), "windows-1251", "é"},
	}
	for _, c := range cases {
		if got := string(ToUTF8(c.body, c.label)); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestFromUTF8(t *testing.T) {
	if got := FromUTF8("Привет", "windows-1251"); got != string(cp1251) {
		t.Errorf("windows-1251: % x", got)
	}
	if got := FromUTF8("Привет", "utf-8"); got != "Привет" {
		t.Errorf("utf-8: %q", got)
	}
	if got := FromUTF8("a€b", "iso-8859-5"); got != "a&#8364;b" {
		t.Errorf("unencodable: %q", got)
	}
	if got := url.QueryEscape(FromUTF8("Привет мир", "cp1251")); got != "%CF%F0%E8%E2%E5%F2+%EC%E8%F0" {
		t.Errorf("escaped: %s", got)
	}
}

func TestFromContentType(t *testing.T) {
	for ct, want := range map[string]string{
		"text/html; charset=windows-1251":   "windows-1251",
		`text/xml; Charset="KOI8-R"`:        "KOI8-R",
		"application/json":                  "",
		"":                                  "",
		"text/html; charset":                "",
		"text/html;;; charset=utf-8; q=1.0": "utf-8",
	} {
		if got := FromContentType(ct); got != want {
			t.Errorf("%q: %q, want %q", ct, got, want)
		}
	}
}

func TestDeclared(t *testing.T) {
	for body, want := range map[string]string{
		`<?xml version="1.0" encoding="windows-1251"?><rss/>`:                          "windows-1251",
		`<?xml version='1.0' encoding='ISO-8859-1' ?>`:                                 "ISO-8859-1",
		`<html><head><meta charset="koi8-r"><title>x</title>`:                          "koi8-r",
		`<meta http-equiv="Content-Type" content="text/html; charset=windows-1251" />`: "windows-1251",
		`<html><body>no declaration</body></html>`:                                     "",
		`<?xml version="1.0"?><rss/>`:                                                  "",
	} {
		if got := Declared([]byte(body)); got != want {
			t.Errorf("%s: %q, want %q", body, got, want)
		}
	}
}

func TestDetect(t *testing.T) {
	page := []byte(`<meta charset="koi8-r">`)
	if got := Detect("windows-1251", "text/html; charset=utf-8", page); got != "windows-1251" {
		t.Errorf("configured label must win: %q", got)
	}
	if got := Detect("", "text/html; charset=utf-8", page); got != "utf-8" {
		t.Errorf("header must win over the page: %q", got)
	}
	if got := Detect("", "text/html", page); got != "koi8-r" {
		t.Errorf("page declaration: %q", got)
	}
}
