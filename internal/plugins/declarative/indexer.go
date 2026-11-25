package declarative

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/definition"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

const (
	maxTorrent    = 5 << 20
	maxRedirects  = 3
	htmlInterval  = 2 * time.Second // spacing for scraped sites without their own rate limit
	linkCacheSize = 2048
)

// Indexer is one configured instance of a definition.
type Indexer struct {
	def      *definition.Definition
	inst     plugin.Instance
	base     *url.URL
	ref      domain.IndexerRef
	settings map[string]string
	links    *linkCache
	pace     *pacer
	sess     session
}

// New validates the instance settings once and builds the indexer.
func New(d *definition.Definition, inst plugin.Instance) (*Indexer, error) {
	base, err := siteBase(firstNonEmpty(inst.Settings.String("baseUrl"), d.Links[0]))
	if err != nil {
		return nil, err
	}
	settings := map[string]string{}
	for _, s := range d.Settings {
		settings[s.Key] = settingText(inst.Settings[s.Key], s.Default)
	}
	if d.Login.Method == definition.LoginCookie && inst.Settings.String("cookie") == "" {
		return nil, plugin.Errorf(plugin.ClassConfig, "%s needs a session cookie", d.Name)
	}
	every := spacing(d)
	return &Indexer{
		def: d, inst: inst, base: base, settings: settings, links: newLinkCache(linkCacheSize), pace: &pacer{every: every},
		ref:  domain.IndexerRef{Key: inst.Key, Name: inst.Name, Priority: inst.Settings.Int("priority", 25)},
		sess: session{cookie: inst.Settings.String("cookie")},
	}, nil
}

// siteBase parses the site address; it always ends in "/" so definition
// paths resolve under it.
func siteBase(raw string) (*url.URL, error) {
	base, err := url.Parse(raw)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, plugin.Errorf(plugin.ClassConfig, "site address %q must be an http(s) address", raw)
	}
	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	return base, nil
}

// spacing is the definition's request interval, else a polite default for scraped pages.
func spacing(d *definition.Definition) time.Duration {
	every := time.Duration(d.RateLimit.Interval)
	if every == 0 && d.Search.Response.Type == definition.ResponseHTML {
		every = htmlInterval
	}
	return every
}

func settingText(v, def any) string {
	if v == nil {
		v = def
	}
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// Definition returns the definition the instance runs.
func (ix *Indexer) Definition() *definition.Definition { return ix.def }

// MinInterval is the site's request spacing, for the shared per-host limiter.
func (ix *Indexer) MinInterval() (string, time.Duration) { return ix.base.Hostname(), ix.pace.every }

// Test logs in when the definition needs it and runs the self-test query.
func (ix *Indexer) Test(ctx context.Context) error {
	if err := ix.ensureSession(ctx); err != nil {
		return err
	}
	rels, err := ix.Search(ctx, domain.SearchRequest{Mode: domain.SearchText, Text: ix.def.Test.Query})
	if err != nil {
		return err
	}
	if len(rels) < ix.def.Test.MinRows {
		return plugin.Errorf(plugin.ClassParse, "self-test found %d results, expected at least %d", len(rels), ix.def.Test.MinRows)
	}
	return nil
}

// Caps reports the definition's capabilities.
func (ix *Indexer) Caps(context.Context) (plugin.Caps, error) { return CapsOf(ix.def), nil }

// Search runs the richest request the definition supports for req.
func (ix *Indexer) Search(ctx context.Context, req domain.SearchRequest) ([]domain.Release, error) {
	cats := ix.def.SiteCategories(req.Kind)
	return ix.query(ctx, ix.def.Translate(req, cats, ix.settings))
}

// SearchCategories is Search restricted to standard categories (the outbound
// Torznab endpoint passes the caller's cat parameter this way).
func (ix *Indexer) SearchCategories(ctx context.Context, req domain.SearchRequest, standard []int) ([]domain.Release, error) {
	cats := ix.def.SiteCategoriesFor(standard)
	if len(standard) == 0 {
		cats = ix.def.SiteCategories(req.Kind)
	}
	return ix.query(ctx, ix.def.Translate(req, cats, ix.settings))
}

func (ix *Indexer) query(ctx context.Context, q definition.Query) ([]domain.Release, error) {
	r, ok := ix.def.Pick(q)
	if !ok {
		return nil, plugin.Errorf(plugin.ClassConfig, "%s has no request for %s searches", ix.def.Name, q.Mode)
	}
	hr, err := ix.def.Render(ix.base, r, q.Vars)
	if err != nil {
		return nil, plugin.Wrap(plugin.ClassConfig, "rendering the search request", err)
	}
	body, err := ix.page(ctx, hr)
	if err != nil {
		return nil, err
	}
	rows, err := ix.def.Rows(body)
	if err != nil {
		return nil, plugin.Wrap(plugin.ClassParse, "results page could not be parsed", err)
	}
	return ix.releases(rows), nil
}

func (ix *Indexer) releases(rows []definition.Row) []domain.Release {
	now := time.Now()
	if ix.inst.Env != nil {
		now = ix.inst.Env.Now()
	}
	var out []domain.Release
	seen := map[string]bool{}
	for _, row := range rows {
		rel, link, ok := ix.def.Release(row, ix.base, now)
		if !ok || seen[rel.ID] {
			continue
		}
		seen[rel.ID] = true
		rel.Indexers = []domain.IndexerRef{ix.ref}
		ix.links.put(rel.ID, link)
		out = append(out, rel)
	}
	return out
}

// Feed returns the latest releases of a kind newer than the cursor.
func (ix *Indexer) Feed(ctx context.Context, kind domain.MediaKind, cursor string) ([]domain.Release, string, error) {
	rels, err := ix.Search(ctx, domain.SearchRequest{Kind: kind, Mode: domain.SearchText})
	if err != nil {
		return nil, cursor, err
	}
	fresh, next := plugin.NewerThan(rels, cursor)
	return fresh, next, nil
}

// page sends a rendered request, logging in first and re-logging in at most
// once when the site serves its logged-out page.
func (ix *Indexer) page(ctx context.Context, hr definition.HTTPRequest) ([]byte, error) {
	if err := ix.ensureSession(ctx); err != nil {
		return nil, err
	}
	resp, err := ix.send(ctx, hr)
	if err != nil {
		return nil, err
	}
	if !ix.expired(resp) {
		return ix.ok(hr, resp)
	}
	if ix.def.Login.Method != definition.LoginForm {
		return nil, plugin.Errorf(plugin.ClassSessionExpired, "%s no longer accepts the session cookie", ix.def.Name)
	}
	ix.sess.drop()
	if err := ix.ensureSession(ctx); err != nil {
		return nil, err
	}
	if resp, err = ix.send(ctx, hr); err != nil {
		return nil, err
	}
	if ix.expired(resp) {
		return nil, plugin.Errorf(plugin.ClassSessionExpired, "%s logged the session out again right after login", ix.def.Name)
	}
	return ix.ok(hr, resp)
}

func (ix *Indexer) expired(r *reply) bool {
	return ix.def.Login.Method != definition.LoginNone && ix.def.Login.Expired.Holds(r.status, r.location, r.body)
}

func (ix *Indexer) ok(hr definition.HTTPRequest, r *reply) ([]byte, error) {
	u, _ := url.Parse(hr.URL)
	if err := plugin.StatusError(hr.Method+" "+plugin.Redact(u), r.status, r.header, r.body); err != nil {
		return nil, err
	}
	if r.status >= 300 {
		return nil, plugin.Errorf(plugin.ClassServerError, "unexpected HTTP %d", r.status)
	}
	return r.body, nil
}

// reply is a bounded response after redirects.
type reply struct {
	status   int
	location string
	header   http.Header
	body     []byte
}

// send paces, adds the session, and follows same-site GET redirects (bounded).
func (ix *Indexer) send(ctx context.Context, hr definition.HTTPRequest) (*reply, error) {
	method, target, body := hr.Method, hr.URL, hr.Body
	for hop := 0; ; hop++ {
		r, err := ix.once(ctx, method, target, body, hr.ContentType, hr.Headers, definition.MaxResponseSize)
		if err != nil || hop == maxRedirects {
			return r, err
		}
		next, ok := ix.follow(target, r)
		if !ok {
			return r, nil
		}
		method, target, body = http.MethodGet, next, ""
	}
}

// follow returns the redirect target to fetch next: same site only, and
// never the logged-out page (that is for the session check to see).
func (ix *Indexer) follow(from string, r *reply) (string, bool) {
	if r.status < 300 || r.status >= 400 || r.location == "" || ix.def.Login.Expired.Holds(r.status, r.location, nil) {
		return "", false
	}
	u, err := url.Parse(from)
	if err != nil {
		return "", false
	}
	loc, err := u.Parse(r.location)
	if err != nil || loc.Host != ix.base.Host {
		return "", false
	}
	return loc.String(), true
}

func (ix *Indexer) once(ctx context.Context, method, target, body, ctype string, headers map[string]string, max int64) (*reply, error) {
	if err := ix.pace.wait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
	if err != nil {
		return nil, plugin.Wrap(plugin.ClassConfig, "building request", err)
	}
	if body == "" {
		req.Body, req.GetBody = http.NoBody, nil
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	ix.sess.apply(req, ix.base.Host)
	resp, err := plugin.Do(ix.inst.Env.HTTP(), req, max)
	if err != nil {
		return nil, err
	}
	ix.sess.keep(resp.Header, req.URL.Host == ix.base.Host)
	return &reply{status: resp.Status, location: resp.Header.Get("Location"), header: resp.Header, body: resp.Body}, nil
}

// Resolve turns a release into a magnet or .torrent bytes, fetching protected
// links with the instance's own session and following the details page when
// the definition says so.
func (ix *Indexer) Resolve(ctx context.Context, rel domain.Release) (plugin.Payload, error) {
	link := ix.linkOf(rel)
	if link.Magnet == "" && link.Download == "" && link.Details != "" && ix.def.Download != nil {
		var err error
		if link, err = ix.followDetails(ctx, link.Details); err != nil {
			return plugin.Payload{}, err
		}
	}
	switch {
	case link.Magnet != "":
		return plugin.Payload{Kind: "magnet", URL: link.Magnet, Name: rel.Title}, nil
	case link.Download != "":
		return ix.fetchTorrent(ctx, link.Download, rel.Title)
	}
	return plugin.Payload{}, plugin.Errorf(plugin.ClassParse, "release %q has no download link", rel.ID)
}

// linkOf is the link remembered from the search, else the one persisted on
// the release (after a restart).
func (ix *Indexer) linkOf(rel domain.Release) definition.Link {
	if link, ok := ix.links.get(rel.ID); ok {
		return link
	}
	if strings.HasPrefix(rel.DownloadURL, "magnet:") {
		return definition.Link{Magnet: rel.DownloadURL}
	}
	return definition.Link{Download: rel.DownloadURL, Details: rel.DetailsURL}
}

func (ix *Indexer) followDetails(ctx context.Context, details string) (definition.Link, error) {
	body, err := ix.page(ctx, definition.HTTPRequest{Method: http.MethodGet, URL: details})
	if err != nil {
		return definition.Link{}, err
	}
	href := definition.Check{Selector: ix.def.Download.Selector}.Attr(body, firstNonEmpty(ix.def.Download.Attr, "href"))
	if href == "" {
		return definition.Link{}, plugin.Errorf(plugin.ClassParse, "details page has no download link")
	}
	u, err := url.Parse(details)
	if err != nil {
		return definition.Link{}, plugin.Wrap(plugin.ClassParse, "details link", err)
	}
	if strings.HasPrefix(href, "magnet:") {
		return definition.Link{Magnet: href}, nil
	}
	ref, err := u.Parse(href)
	if err != nil {
		return definition.Link{}, plugin.Wrap(plugin.ClassParse, "download link", err)
	}
	return definition.Link{Download: ref.String()}, nil
}

func (ix *Indexer) fetchTorrent(ctx context.Context, link, name string) (plugin.Payload, error) {
	r, err := ix.once(ctx, http.MethodGet, link, "", "", nil, maxTorrent)
	if err != nil {
		return plugin.Payload{}, err
	}
	if strings.HasPrefix(r.location, "magnet:") {
		return plugin.Payload{Kind: "magnet", URL: r.location, Name: name}, nil
	}
	if err := plugin.StatusError("download", r.status, r.header, r.body); err != nil {
		return plugin.Payload{}, err
	}
	if len(r.body) == 0 || r.body[0] != 'd' {
		return plugin.Payload{}, plugin.Errorf(plugin.ClassParse, "download did not return a torrent file")
	}
	return plugin.Payload{Kind: "torrent", Content: r.body, Name: name}, nil
}

// pacer spaces requests of one instance by a minimum interval.
type pacer struct {
	mu    sync.Mutex
	every time.Duration
	next  time.Time
}

func (p *pacer) wait(ctx context.Context) error {
	if p.every <= 0 {
		return nil
	}
	p.mu.Lock()
	now := time.Now()
	at := p.next
	if at.Before(now) {
		at = now
	}
	p.next = at.Add(p.every)
	p.mu.Unlock()
	d := time.Until(at)
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// linkCache is a bounded FIFO map from release id to its link.
type linkCache struct {
	mu    sync.Mutex
	max   int
	order []string
	m     map[string]definition.Link
}

func newLinkCache(max int) *linkCache { return &linkCache{max: max, m: map[string]definition.Link{}} }

func (c *linkCache) put(id string, l definition.Link) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.m[id]; !ok {
		c.order = append(c.order, id)
		if len(c.order) > c.max {
			delete(c.m, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.m[id] = l
}

func (c *linkCache) get(id string) (definition.Link, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	l, ok := c.m[id]
	return l, ok
}

// errLogin marks a failed form login for callers that want the message.
var errLogin = errors.New("login failed")
