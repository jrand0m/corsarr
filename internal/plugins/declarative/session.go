package declarative

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/jrand0m/corsarr/internal/definition"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// session holds the cookies an instance sends to its own site: the pasted
// cookie for cookie login, or what a form login and later responses set.
type session struct {
	mu       sync.Mutex
	cookie   string            // pasted Cookie header
	jar      map[string]string // name → value set by the site
	loggedIn bool
}

func (s *session) apply(req *http.Request, siteHost string) {
	if req.URL.Host != siteHost {
		return // the session never leaves the site
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	parts := []string{}
	if s.cookie != "" {
		parts = append(parts, s.cookie)
	}
	names := make([]string, 0, len(s.jar))
	for n := range s.jar {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		parts = append(parts, n+"="+s.jar[n])
	}
	if len(parts) > 0 {
		req.Header.Set("Cookie", strings.Join(parts, "; "))
	}
}

func (s *session) keep(h http.Header, ownSite bool) {
	if !ownSite {
		return
	}
	cookies := (&http.Response{Header: h}).Cookies()
	if len(cookies) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jar == nil {
		s.jar = map[string]string{}
	}
	for _, c := range cookies {
		if c.MaxAge < 0 || c.Value == "" {
			delete(s.jar, c.Name)
			continue
		}
		s.jar[c.Name] = c.Value
	}
}

func (s *session) drop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jar, s.loggedIn = nil, false
}

func (s *session) ready() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loggedIn
}

func (s *session) markReady() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loggedIn = true
}

// ensureSession logs in with the form once per session; other methods need nothing.
func (ix *Indexer) ensureSession(ctx context.Context) error {
	if ix.def.Login.Method != definition.LoginForm || ix.sess.ready() {
		return nil
	}
	return ix.login(ctx)
}

// login posts the credentials (on top of the landing page's form when the
// definition names a login page) and checks the page the site answers with.
func (ix *Indexer) login(ctx context.Context) error {
	form, target, err := ix.loginForm(ctx)
	if err != nil {
		return err
	}
	vars := definition.Vars{}
	for k, v := range ix.settings {
		vars["settings."+k] = v
	}
	for field, tpl := range ix.def.Login.Fields {
		v, err := definition.Expand(tpl, vars)
		if err != nil {
			return plugin.Wrap(plugin.ClassConfig, "login field "+field, err)
		}
		form.Set(field, v)
	}
	r, err := ix.send(ctx, definition.HTTPRequest{Method: http.MethodPost, URL: target, Body: ix.encodeForm(form),
		ContentType: definition.FormType})
	if err != nil {
		return err
	}
	return ix.loginResult(r)
}

// encodeForm writes form values in the site's encoding.
func (ix *Indexer) encodeForm(form url.Values) string {
	out := url.Values{}
	for k, vs := range form {
		for _, v := range vs {
			out.Add(ix.def.EncodeText(k), ix.def.EncodeText(v))
		}
	}
	return out.Encode()
}

// sitePath resolves a definition path under the site address.
func (ix *Indexer) sitePath(p string) (*url.URL, error) {
	u, err := ix.base.Parse(strings.TrimPrefix(p, "/"))
	if err != nil {
		return nil, plugin.Wrap(plugin.ClassConfig, "login path", err)
	}
	return u, nil
}

// loginForm returns the values to start the login from and where to post
// them: nothing and the login path, or, with a login page, that page's form
// (its cookies kept by the session) and its action unless a path is set.
func (ix *Indexer) loginForm(ctx context.Context) (url.Values, string, error) {
	l := ix.def.Login
	if l.Page == "" {
		target, err := ix.sitePath(l.Path)
		if err != nil {
			return nil, "", err
		}
		return url.Values{}, target.String(), nil
	}
	page, err := ix.sitePath(l.Page)
	if err != nil {
		return nil, "", err
	}
	f, err := ix.landingForm(ctx, page)
	if err != nil {
		return nil, "", err
	}
	target, err := ix.formTarget(page, f.Action)
	return f.Fields, target, err
}

// landingForm fetches the login page and reads its form.
func (ix *Indexer) landingForm(ctx context.Context, page *url.URL) (definition.LandingForm, error) {
	hr := definition.HTTPRequest{Method: http.MethodGet, URL: page.String()}
	r, err := ix.send(ctx, hr)
	if err != nil {
		return definition.LandingForm{}, err
	}
	body, err := ix.ok(hr, r)
	if err != nil {
		return definition.LandingForm{}, err
	}
	f, err := ix.def.Login.FormOn(body)
	if err != nil {
		return definition.LandingForm{}, plugin.Wrap(plugin.ClassParse, "reading the login page", err)
	}
	return f, nil
}

// formTarget is where the landing page's form posts: the configured path,
// else the form's action resolved against the page. Credentials never
// leave the site.
func (ix *Indexer) formTarget(page *url.URL, action string) (string, error) {
	if ix.def.Login.Path != "" {
		u, err := ix.sitePath(ix.def.Login.Path)
		if err != nil {
			return "", err
		}
		return u.String(), nil
	}
	u, err := page.Parse(action)
	if err != nil || u.Host != ix.base.Host {
		return "", plugin.Errorf(plugin.ClassConfig, "the login form posts to %q, outside the site", action)
	}
	return u.String(), nil
}

func (ix *Indexer) loginResult(r *reply) error {
	l := ix.def.Login
	if plugin.Challenged(r.status, r.header, r.body) {
		return plugin.ChallengeError(r.status, r.header) // the credentials were never seen
	}
	if l.Failure.Holds(r.status, r.location, r.body) {
		msg := firstNonEmpty(l.Failure.Message(r.body), "the site refused the credentials")
		return plugin.Wrap(plugin.ClassAuth, msg, errLogin)
	}
	if err := plugin.StatusError("login", r.status, r.header, r.body); err != nil {
		return err
	}
	if !l.Success.Holds(r.status, r.location, r.body) {
		return plugin.Wrap(plugin.ClassAuth, "login did not reach the signed-in page", errLogin)
	}
	ix.sess.markReady()
	return nil
}
