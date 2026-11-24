package indexers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/instances"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// AllID is the endpoint id that searches every usable indexer.
const AllID = "all"

// Fan-out bounds of the outbound endpoint.
const (
	perIndexerTimeout = 20 * time.Second
	fanOutLimit       = 4
	torznabKV         = "torznab.enabled"
)

// categorySearcher is the optional indexer capability to search by standard categories.
type categorySearcher interface {
	SearchCategories(ctx context.Context, req domain.SearchRequest, standard []int) ([]domain.Release, error)
}

// TorznabEnabled reports whether the outbound endpoint is on (off by default).
func (s *Service) TorznabEnabled(ctx context.Context) bool {
	v, ok, err := s.d.KV.Get(ctx, torznabKV)
	return err == nil && ok && v == "true"
}

// SetTorznab turns the outbound endpoint on or off.
func (s *Service) SetTorznab(ctx context.Context, on bool) error {
	v := "false"
	if on {
		v = "true"
	}
	return s.d.KV.Set(ctx, torznabKV, v)
}

// target is one running indexer behind an endpoint.
type target struct {
	instances.Running
	ix plugin.Indexer
}

// targets returns the usable indexers behind an endpoint id.
func (s *Service) targets(id string) []target {
	var out []target
	for _, r := range s.d.Running() {
		ix, ok := r.Impl.(plugin.Indexer)
		if !ok || (id != AllID && string(r.Key) != id) {
			continue
		}
		out = append(out, target{Running: r, ix: ix})
	}
	return out
}

func (s *Service) torznab(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	if !s.TorznabEnabled(r.Context()) {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, codeUnsupported, "the Torznab endpoint is turned off in corsarr's indexer settings")
		return
	}
	id := r.PathValue("id")
	ts := s.targets(id)
	if len(ts) == 0 && id != AllID {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, codeNotFound, "no running indexer "+id)
		return
	}
	req, err := ParseRequest(r.URL.Query())
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, codeBadParam, err.Error())
		return
	}
	title := "corsarr"
	if id != AllID && len(ts) == 1 {
		title = "corsarr: " + firstNonEmpty(ts[0].Name, id)
	}
	if req.Op == "caps" {
		_, _ = w.Write(CapsDoc(title, s.caps(r.Context(), ts)))
		return
	}
	rels := s.fanOut(r.Context(), ts, req)
	page := rels[min(req.Offset, len(rels)):min(req.Offset+req.Limit, len(rels))]
	base := s.publicBase(r, id)
	_, _ = w.Write(ResultsDoc(title, page, func(rel domain.Release) string { return s.grabLink(base, r, rel) }))
}

func (s *Service) caps(ctx context.Context, ts []target) []plugin.Caps {
	var out []plugin.Caps
	for _, t := range ts {
		if c, err := t.ix.Caps(ctx); err == nil {
			out = append(out, c)
		}
	}
	return out
}

// fanOut searches every target in parallel (bounded), skipping blocked or
// over-limit indexers, and merges the results by identity, best priority first.
func (s *Service) fanOut(ctx context.Context, ts []target, req Request) []domain.Release {
	var mu sync.Mutex
	var all [][]domain.Release
	sem := make(chan struct{}, fanOutLimit)
	var wg sync.WaitGroup
	for _, t := range ts {
		if s.d.Health.Blocked(t.Ref()) || !s.allowed(ctx, t) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			rels := s.searchOne(ctx, t, req)
			mu.Lock()
			all = append(all, rels)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return MergeReleases(all)
}

func (s *Service) searchOne(ctx context.Context, t target, req Request) []domain.Release {
	ctx, cancel := context.WithTimeout(ctx, perIndexerTimeout)
	defer cancel()
	_ = s.d.Queries.RecordQuery(ctx, t.Key, s.d.Now())
	var rels []domain.Release
	var err error
	if cs, ok := t.ix.(categorySearcher); ok && len(req.Cats) > 0 {
		rels, err = cs.SearchCategories(ctx, req.Search, req.Cats)
	} else {
		rels, err = t.ix.Search(ctx, req.Search)
	}
	if err != nil {
		s.reportFailure(t.Ref(), err)
		return nil
	}
	return rels
}

// reportFailure records failures the shared client cannot see (a page it
// fetched fine but that could not be parsed, an expired session).
func (s *Service) reportFailure(ref domain.IntegrationRef, err error) {
	ReportUnseen(s.d.Health, ref, err)
}

// unseen are the failure classes a page fetched over HTTP 200 can still carry.
var unseen = map[plugin.FailureClass]bool{plugin.ClassParse: true, plugin.ClassSessionExpired: true, plugin.ClassChallenge: true}

// ReportUnseen records in health the failures the shared client cannot see
// (a results page that could not be parsed, an expired session).
func ReportUnseen(h interface {
	Report(domain.IntegrationRef, health.Observation) domain.IntegrationHealth
}, ref domain.IntegrationRef, err error) {
	if c := plugin.ClassOf(err); unseen[c] {
		h.Report(ref, health.Observation{Class: string(c), Message: err.Error()})
	}
}

// allowed applies the indexer's query limit over the persisted query log.
func (s *Service) allowed(ctx context.Context, t target) bool {
	limit := t.Settings.Int("queryLimit", 0)
	if limit <= 0 {
		return true
	}
	per := 24 * time.Hour
	if first(t.Settings.Strings("queryLimitPer")) == "hour" {
		per = time.Hour
	}
	n, _, err := s.d.Queries.Queries(ctx, t.Key, s.d.Now().Add(-per))
	return err == nil && n < limit
}

func first(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	return xs[0]
}

// MergeReleases merges per-indexer results: one release per identity (info hash,
// else id), keeping every providing indexer, ordered by publish date (pure).
func MergeReleases(lists [][]domain.Release) []domain.Release {
	byKey := map[string]int{}
	var out []domain.Release
	for _, rels := range lists {
		for _, r := range rels {
			key := firstNonEmpty(strings.ToLower(r.InfoHash), r.ID)
			if i, ok := byKey[key]; ok {
				out[i].Indexers = append(out[i].Indexers, r.Indexers...)
				out[i].Seeders = max(out[i].Seeders, r.Seeders)
				continue
			}
			byKey[key] = len(out)
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].PublishedAt.After(out[j].PublishedAt) })
	return out
}

// publicBase is the address clients reach this endpoint at.
func (s *Service) publicBase(r *http.Request, id string) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := firstNonEmpty(r.Header.Get("X-Forwarded-Host"), r.Host)
	return scheme + "://" + host + strings.TrimSuffix(s.d.BasePath(), "/") + "/torznab/" + url.PathEscape(id)
}

// grabLink remembers where a release came from and returns its proxied
// download address; the indexer's own link never leaves corsarr.
func (s *Service) grabLink(base string, r *http.Request, rel domain.Release) string {
	src := domain.ObjectKey("")
	if len(rel.Indexers) > 0 {
		src = rel.Indexers[0].Key
	}
	sum := sha256.Sum256([]byte(string(src) + "\x00" + rel.ID))
	token := hex.EncodeToString(sum[:12])
	s.grabs.put(token, grab{indexer: src, rel: rel})
	q := url.Values{"id": {token}, "apikey": {r.URL.Query().Get("apikey")}}
	return base + "/dl?" + q.Encode()
}

// download resolves a proxied link: magnets redirect, torrent files stream.
func (s *Service) download(w http.ResponseWriter, r *http.Request) {
	if !s.TorznabEnabled(r.Context()) {
		http.NotFound(w, r)
		return
	}
	g, ok := s.grabs.get(r.URL.Query().Get("id"))
	var t []target
	if ok {
		t = s.targets(string(g.indexer))
	}
	if !ok || len(t) == 0 {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		writeError(w, codeNotFound, "this download link has expired; search again")
		return
	}
	p, err := t[0].ix.Resolve(r.Context(), g.rel)
	if err != nil {
		s.reportFailure(t[0].Ref(), err)
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		writeError(w, codeServer, "the indexer did not hand over the download: "+err.Error())
		return
	}
	if p.Kind == "magnet" {
		http.Redirect(w, r, p.URL, http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "application/x-bittorrent")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeName(p.Name)+`.torrent"`)
	_, _ = w.Write(p.Content)
}

func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`"\/:*?<>|`, r) {
			return '_'
		}
		return r
	}, s)
	return firstNonEmpty(s, "download")
}

func denyTorznab(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	writeError(w, codeAuth, "missing or wrong apikey")
}

// grab is a release offered through the endpoint, waiting to be downloaded.
type grab struct {
	indexer domain.ObjectKey
	rel     domain.Release
}

// grabCache is a bounded FIFO map of offered releases.
type grabCache struct {
	mu    sync.Mutex
	max   int
	order []string
	m     map[string]grab
}

func newGrabCache(max int) *grabCache { return &grabCache{max: max, m: map[string]grab{}} }

func (c *grabCache) put(token string, g grab) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.m[token]; !ok {
		c.order = append(c.order, token)
		if len(c.order) > c.max {
			delete(c.m, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.m[token] = g
}

func (c *grabCache) get(token string) (grab, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	g, ok := c.m[token]
	return g, ok
}
