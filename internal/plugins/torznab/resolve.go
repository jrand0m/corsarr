package torznab

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// target is where a release's payload comes from; it stays inside the plugin
// because download links carry the indexer's credentials.
type target struct{ link, magnet string }

// linkCache is a bounded FIFO map from release id to download target.
type linkCache struct {
	mu    sync.Mutex
	max   int
	order []string
	m     map[string]target
}

func newLinkCache(max int) *linkCache { return &linkCache{max: max, m: map[string]target{}} }

func (c *linkCache) put(id string, t target) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.m[id]; !ok {
		c.order = append(c.order, id)
		if len(c.order) > c.max {
			delete(c.m, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.m[id] = t
}

func (c *linkCache) get(id string) (target, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.m[id]
	return t, ok
}

// Resolve passes magnets through and fetches .torrent files with the indexer's
// own credentials. A release unknown to this process (restart) falls back to
// the persisted DownloadURL, then to the API's t=get by guid.
func (ix *Indexer) Resolve(ctx context.Context, rel domain.Release) (plugin.Payload, error) {
	tg, ok := ix.links.get(rel.ID)
	switch {
	case ok:
	case rel.DownloadURL != "":
		tg = target{link: rel.DownloadURL}
	case rel.GUID != "":
		tg = target{link: ix.getURL(rel.GUID)}
	default:
		return plugin.Payload{}, plugin.Errorf(plugin.ClassParse, "release %q has no download link", rel.ID)
	}
	if m := magnetOf(tg); m != "" {
		return plugin.Payload{Kind: "magnet", URL: m, Name: rel.Title}, nil
	}
	return ix.fetchTorrent(ctx, tg.link, rel.Title)
}

func magnetOf(tg target) string {
	if strings.HasPrefix(tg.link, "magnet:") {
		return tg.link
	}
	if tg.link == "" {
		return tg.magnet
	}
	return ""
}

func (ix *Indexer) getURL(guid string) string {
	u := *ix.endpoint
	q := u.Query()
	q.Set("t", "get")
	q.Set("id", guid)
	u.RawQuery = q.Encode()
	return u.String()
}

func (ix *Indexer) fetchTorrent(ctx context.Context, link, name string) (plugin.Payload, error) {
	resp, err := ix.fetch(ctx, ix.authorize(link), maxTorrent)
	if err != nil {
		if m := redirectedMagnet(err); m != "" {
			return plugin.Payload{Kind: "magnet", URL: m, Name: name}, nil
		}
		return plugin.Payload{}, err
	}
	if len(resp.Body) == 0 || resp.Body[0] != 'd' {
		if e := serverError(resp.Body); e != nil {
			return plugin.Payload{}, e
		}
		return plugin.Payload{}, plugin.Errorf(plugin.ClassParse, "download did not return a torrent file")
	}
	return plugin.Payload{Kind: "torrent", Content: resp.Body, Name: name}, nil
}

// redirectedMagnet recognises a redirect to a magnet: URI, which HTTP clients cannot follow.
func redirectedMagnet(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) && strings.HasPrefix(ue.URL, "magnet:") {
		return ue.URL
	}
	return ""
}

// authorize resolves relative links and adds the API key for links on the indexer's own host only.
func (ix *Indexer) authorize(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return link
	}
	u = ix.endpoint.ResolveReference(u)
	q := u.Query()
	if ix.apiKey != "" && u.Host == ix.endpoint.Host && q.Get("apikey") == "" {
		q.Set("apikey", ix.apiKey)
		u.RawQuery = q.Encode()
	}
	return u.String()
}
