package declarative

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jrand0m/corsarr/internal/definition"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// csrfSite only accepts a login that carries the token from its login page
// together with the pre-login cookie that page set.
type csrfSite struct {
	srv     *httptest.Server
	pages   atomic.Int32
	logins  atomic.Int32
	noForm  atomic.Bool
	formTok atomic.Value
}

func newCSRFSite(t *testing.T) *csrfSite {
	s := &csrfSite{}
	s.formTok.Store("tok-1")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /account/login", s.landing)
	mux.HandleFunc("POST /account/session", s.login)
	mux.HandleFunc("GET /browse.php", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("sid"); err != nil || c.Value != "ok" {
			http.Redirect(w, r, "/login.php", http.StatusFound)
			return
		}
		_, _ = w.Write(fixture(t, "browse.html"))
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *csrfSite) landing(w http.ResponseWriter, _ *http.Request) {
	s.pages.Add(1)
	http.SetCookie(w, &http.Cookie{Name: "pre", Value: "p1"})
	if s.noForm.Load() {
		_, _ = w.Write([]byte(`<html><body>maintenance</body></html>`))
		return
	}
	tok := s.formTok.Load().(string)
	_, _ = w.Write([]byte(`<html><head><meta name="xsrf" content="m-` + tok + `"></head><body>
<form class="search" action="/search"><input name="q"></form>
<form class="signin" action="/account/session" method="post">
<input type="hidden" name="authenticity" value="` + tok + `">
<input type="hidden" name="return" value="/index.php">
<input name="user"><input type="password" name="pass">
<input type="submit" value="Go">
</form></body></html>`))
}

func (s *csrfSite) login(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	f := r.PostForm
	pre, err := r.Cookie("pre")
	tok := s.formTok.Load().(string)
	if err != nil || pre.Value != "p1" || f.Get("authenticity") != tok || f.Get("hdr") != "m-"+tok || f.Get("return") != "/index.php" ||
		f.Get("user") != "owner" || f.Get("pass") != "pw" {
		_, _ = w.Write([]byte(`<div class="login-error">Session expired, reload the page</div>`))
		return
	}
	s.logins.Add(1)
	http.SetCookie(w, &http.Cookie{Name: "sid", Value: "ok"})
	_, _ = w.Write([]byte(`<a href="/logout.php">Log out</a>`))
}

func landingDef(t *testing.T) *definition.Definition {
	d := exampleDef(t)
	d.Login.Path = ""
	d.Login.Page = "/account/login"
	d.Login.Form = "form.signin"
	d.Login.Tokens = map[string]definition.Pick{"hdr": {Selector: `meta[name="xsrf"]`, Attr: "content"}}
	if probs := d.Validate(); len(probs) > 0 {
		t.Fatal(probs)
	}
	return d
}

func TestFormLoginCarriesTheLandingPageTokens(t *testing.T) {
	s := newCSRFSite(t)
	ix := instance(t, landingDef(t), s.srv.URL, plugin.Settings{"username": "owner", "password": "pw"})
	rels, err := ix.Search(context.Background(), domain.SearchRequest{Mode: domain.SearchText, Text: "Quiet"})
	if err != nil || len(rels) != 2 {
		t.Fatalf("%v %v", rels, err)
	}
	if s.pages.Load() != 1 || s.logins.Load() != 1 {
		t.Errorf("pages %d logins %d", s.pages.Load(), s.logins.Load())
	}
}

func TestFormLoginRefreshesTheTokenOnRelogin(t *testing.T) {
	s := newCSRFSite(t)
	ix := instance(t, landingDef(t), s.srv.URL, plugin.Settings{"username": "owner", "password": "pw"})
	if err := ix.ensureSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.formTok.Store("tok-2")
	ix.sess.drop()
	if err := ix.ensureSession(context.Background()); err != nil {
		t.Fatalf("second login with a new token: %v", err)
	}
	if s.pages.Load() != 2 || s.logins.Load() != 2 {
		t.Errorf("pages %d logins %d", s.pages.Load(), s.logins.Load())
	}
}

func TestFormLoginWithAnExplicitPathPostsThere(t *testing.T) {
	s := newCSRFSite(t)
	d := landingDef(t)
	d.Login.Path = "/account/session"
	d.Login.Form = ""
	ix := instance(t, d, s.srv.URL, plugin.Settings{"username": "owner", "password": "pw"})
	// The first form on the page is the search form, so no token is carried.
	if err := ix.ensureSession(context.Background()); plugin.ClassOf(err) != plugin.ClassAuth || !strings.Contains(err.Error(), "reload the page") {
		t.Errorf("%v", err)
	}
}

func TestFormLoginWithoutTheFormOnTheLandingPage(t *testing.T) {
	s := newCSRFSite(t)
	s.noForm.Store(true)
	ix := instance(t, landingDef(t), s.srv.URL, plugin.Settings{"username": "owner", "password": "pw"})
	err := ix.ensureSession(context.Background())
	if plugin.ClassOf(err) != plugin.ClassParse || !strings.Contains(err.Error(), "form.signin") {
		t.Errorf("%v", err)
	}
}

func TestFormLoginNeverPostsCredentialsOffSite(t *testing.T) {
	var posted atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posted.Store(posted.Load() || r.Method == http.MethodPost)
		_, _ = w.Write([]byte(`<form class="signin" action="https://elsewhere.test/collect"><input name="user"></form>`))
	}))
	t.Cleanup(srv.Close)
	ix := instance(t, landingDef(t), srv.URL, plugin.Settings{"username": "owner", "password": "pw"})
	err := ix.ensureSession(context.Background())
	if plugin.ClassOf(err) != plugin.ClassConfig || !strings.Contains(err.Error(), "outside the site") || posted.Load() {
		t.Errorf("%v (posted %v)", err, posted.Load())
	}
}

func TestFormLoginLandingPageBlocked(t *testing.T) {
	srv := challengeServer(t)
	ix := instance(t, landingDef(t), srv.URL, plugin.Settings{"username": "owner", "password": "pw"})
	if err := ix.ensureSession(context.Background()); plugin.ClassOf(err) != plugin.ClassChallenge {
		t.Errorf("%v", err)
	}
}
