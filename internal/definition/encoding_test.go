package definition

import (
	"net/url"
	"strings"
	"testing"
)

const hello1251 = "%CF%F0%E8%E2%E5%F2" // "Привет" in windows-1251, percent-encoded

func TestEncodingIsValidated(t *testing.T) {
	d := mustParse(t, jsonDef+"encoding: windows-1251\n")
	if d.Encoding != "windows-1251" {
		t.Fatal(d.Encoding)
	}
	_, err := Parse([]byte(jsonDef + "encoding: klingon\n"))
	if err == nil || !strings.Contains(err.Error(), "encoding") {
		t.Errorf("unknown encoding accepted: %v", err)
	}
}

func TestRenderEncodesRequestsInTheSiteEncoding(t *testing.T) {
	d := mustParse(t, jsonDef+"encoding: cp1251\n")
	base, _ := url.Parse("https://site.test/")
	v := Vars{"keywords": "Привет"}
	hr, err := d.Render(base, Request{Method: "GET", Path: "/s/{{keywords|pathescape}}", Query: Strings{"q": "{{keywords}}", "u": "{{keywords|urlencode}}"}}, v)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://site.test/s/" + hello1251 + "?q=" + hello1251 + "&u=%25CF%25F0%25E8%25E2%25E5%25F2"
	if hr.URL != want {
		t.Errorf("url\n got %s\nwant %s", hr.URL, want)
	}
	form, err := d.Render(base, Request{Method: "POST", Path: "/s", Body: "q={{keywords|urlencode}}&raw={{keywords}}"}, v)
	if err != nil || form.Body != "q="+hello1251+"&raw=\xCF\xF0\xE8\xE2\xE5\xF2" {
		t.Errorf("form body %q %v", form.Body, err)
	}
	js, _ := d.Render(base, Request{Method: "POST", Path: "/s", Body: `{"q":{{keywords|json}}}`, Type: "application/json"}, v)
	if js.Body != `{"q":"Привет"}` {
		t.Errorf("a JSON body stays UTF-8: %q", js.Body)
	}
}

func TestRenderWithoutEncodingStaysUTF8(t *testing.T) {
	d := mustParse(t, jsonDef)
	base, _ := url.Parse("https://site.test/")
	hr, _ := d.Render(base, Request{Method: "GET", Path: "/s", Query: Strings{"q": "{{keywords}}"}}, Vars{"keywords": "Привет"})
	if hr.URL != "https://site.test/s?q=%D0%9F%D1%80%D0%B8%D0%B2%D0%B5%D1%82" {
		t.Error(hr.URL)
	}
}

func TestEncodeForSite(t *testing.T) {
	d := &Definition{Encoding: "windows-1251"}
	if got := url.QueryEscape(d.EncodeText("Привет")); got != hello1251 {
		t.Error(got)
	}
	if got := (&Definition{}).EncodeText("Привет"); got != "Привет" {
		t.Error(got)
	}
}
