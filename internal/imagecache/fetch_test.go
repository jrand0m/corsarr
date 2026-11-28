package imagecache

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestFetchFollowsBoundedRedirects(t *testing.T) {
	d := newDoer()
	d.set("https://a.example/1", reply{status: http.StatusTemporaryRedirect, location: "/2"})
	d.set("https://a.example/2", reply{status: http.StatusFound, location: "https://b.example/3"})
	d.set("https://b.example/3", reply{status: http.StatusMovedPermanently, location: "https://c.example/4"})
	d.set("https://c.example/4", reply{status: http.StatusOK, body: []byte("img")})
	if body, err := fetch(ctx, d, "https://a.example/1"); err != nil || string(body) != "img" {
		t.Fatal(string(body), err)
	}
	d.set("https://c.example/4", reply{status: http.StatusSeeOther, location: "https://d.example/5"})
	d.set("https://d.example/5", reply{status: http.StatusOK, body: []byte("img")})
	if _, err := fetch(ctx, d, "https://a.example/1"); err == nil || d.count("https://d.example/5") != 0 {
		t.Fatal("fourth redirect followed", err)
	}
	d.set("https://e.example/x", reply{status: http.StatusFound, location: "file:///etc/passwd"})
	d.set("https://e.example/y", reply{status: http.StatusFound})
	for _, u := range []string{"https://e.example/x", "https://e.example/y"} {
		if _, err := fetch(ctx, d, u); err == nil {
			t.Errorf("%s: bad redirect followed", u)
		}
	}
}

func TestFetchClassifiesAnswers(t *testing.T) {
	d := newDoer()
	d.set("https://a.example/gone", reply{status: http.StatusGone})
	d.set("https://a.example/err", reply{status: http.StatusInternalServerError})
	d.set("https://a.example/big", reply{status: http.StatusOK, body: []byte(strings.Repeat("x", maxSource+1))})
	if _, err := fetch(ctx, d, "https://a.example/missing"); !errors.Is(err, ErrMissing) {
		t.Fatal(err)
	}
	if _, err := fetch(ctx, d, "https://a.example/gone"); !errors.Is(err, ErrMissing) {
		t.Fatal(err)
	}
	if _, err := fetch(ctx, d, "https://a.example/err"); err == nil || errors.Is(err, ErrMissing) {
		t.Fatal(err)
	}
	if _, err := fetch(ctx, d, "https://a.example/big"); !errors.Is(err, ErrTooLarge) {
		t.Fatal(err)
	}
	if _, err := fetch(ctx, d, "ftp://a.example/x"); err == nil {
		t.Fatal("ftp fetched")
	}
	d.down = true
	if _, err := fetch(ctx, d, "https://a.example/err"); err == nil {
		t.Fatal("transport error lost")
	}
}
