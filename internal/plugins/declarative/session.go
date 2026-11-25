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

// login posts the credentials and checks the page the site answers with.
func (ix *Indexer) login(ctx context.Context) error {
	l := ix.def.Login
	vars := definition.Vars{}
	for k, v := range ix.settings {
		vars["settings."+k] = v
	}
	form := url.Values{}
	for field, tpl := range l.Fields {
		v, err := definition.Expand(tpl, vars)
		if err != nil {
			return plugin.Wrap(plugin.ClassConfig, "login field "+field, err)
		}
		form.Set(field, v)
	}
	target, err := ix.base.Parse(strings.TrimPrefix(l.Path, "/"))
	if err != nil {
		return plugin.Wrap(plugin.ClassConfig, "login path", err)
	}
	r, err := ix.send(ctx, definition.HTTPRequest{Method: http.MethodPost, URL: target.String(), Body: form.Encode(),
		ContentType: "application/x-www-form-urlencoded"})
	if err != nil {
		return err
	}
	return ix.loginResult(r)
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
