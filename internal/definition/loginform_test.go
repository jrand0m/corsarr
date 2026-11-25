package definition

import (
	"strings"
	"testing"
)

const landing = `<html><head><meta name="csrf-token" content="meta-tok"></head><body>
<form id="search" action="/search"><input name="q"></form>
<form id="login" action="/account/login?next=%2F" method="post">
  <input type="hidden" name="_token" value="form-tok">
  <input type="text" name="user" value="">
  <input type="password" name="pass">
  <input type="checkbox" name="remember" checked>
  <input type="checkbox" name="newsletter" value="yes">
  <input type="radio" name="scope" value="all">
  <input type="radio" name="scope" value="mine" checked>
  <input type="submit" name="go" value="Sign in">
  <input type="image" name="pic" src="x.png">
  <input value="no name">
  <select name="lang"><option value="en">English</option><option value="uk" selected>Ukrainian</option></select>
  <select name="theme"><option>dark</option><option>light</option></select>
  <select name="empty"></select>
  <textarea name="note">  hello  </textarea>
</form></body></html>`

func TestLoginFormOnAPage(t *testing.T) {
	l := Login{Form: "form#login", Tokens: map[string]Pick{"csrf": {Selector: `meta[name="csrf-token"]`, Attr: "content"}, "missing": {Selector: "div.none"}}}
	f, err := l.FormOn([]byte(landing))
	if err != nil {
		t.Fatal(err)
	}
	if f.Action != "/account/login?next=%2F" {
		t.Errorf("action %q", f.Action)
	}
	want := "_token=form-tok&csrf=meta-tok&lang=uk&note=++hello++&pass=&remember=on&scope=mine&theme=dark&user="
	if got := f.Fields.Encode(); got != want {
		t.Errorf("fields\n got %s\nwant %s", got, want)
	}
}

func TestLoginFormDefaultsToTheFirstForm(t *testing.T) {
	f, err := (Login{}).FormOn([]byte(landing))
	if err != nil || f.Action != "/search" || f.Fields.Get("q") != "" || len(f.Fields) != 1 {
		t.Fatalf("%+v %v", f, err)
	}
	if _, err := (Login{Form: "form#nope"}).FormOn([]byte(landing)); err == nil || !strings.Contains(err.Error(), "form#nope") {
		t.Errorf("missing form: %v", err)
	}
}

func TestLoginLandingPageValidation(t *testing.T) {
	ok := Login{Method: LoginForm, Page: "/login.php", Fields: Strings{"u": "x"}, Success: Check{Contains: "out"}}
	var v validator
	v.login(ok)
	if len(v.out) != 0 {
		t.Errorf("page without path rejected: %v", v.out)
	}
	bad := ok
	bad.Page, bad.Form, bad.Tokens = "", "[[", map[string]Pick{"t": {Selector: ">"}, "": {Selector: "meta"}}
	v = validator{}
	v.login(bad)
	if len(v.out) != 4 {
		t.Errorf("want path, form, token selector and token name problems: %v", v.out)
	}
}
