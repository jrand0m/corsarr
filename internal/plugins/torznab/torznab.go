// Package torznab is the built-in Torznab indexer plugin, implemented from the
// public Torznab/Newznab API description.
package torznab

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// ID is the plugin id used as `type:` in config.
const ID = "torznab"

const (
	maxXML     = 8 << 20
	maxTorrent = 5 << 20
)

var defaultCategories = map[string]any{
	"movie": []int{2000}, "tv": []int{5000}, "music": []int{3000}, "book": []int{7000},
}

var manifest = plugin.Manifest{
	ID: ID, Kind: plugin.KindIndexer, Version: "1.0.0", APIVersion: plugin.APIVersion,
	Name: "Torznab", Delivery: "builtin",
	Capabilities: []string{"search", "feed", "resolve", "movie-id-search"},
	Settings: []plugin.Field{
		{Key: "url", Type: plugin.FieldURL, Label: "Indexer URL", Required: true,
			Help: "Base address of the Torznab indexer, for example http://jackett:9117/api/v2.0/indexers/all/results/torznab."},
		{Key: "apiPath", Type: plugin.FieldText, Label: "API path", Default: "/api",
			Help: "Path appended to the address. Leave as /api unless the indexer documents another path; use an empty value when the address already ends in the API endpoint."},
		{Key: "apiKey", Type: plugin.FieldSecret, Label: "API key",
			Help: "API key issued by the indexer. Public indexers may not need one."},
		{Key: "categories", Type: plugin.FieldMap, Label: "Categories per media kind", Default: defaultCategories,
			Help: "Torznab category ids to search for each media kind (movie, tv, music, book). Defaults use the standard parents 2000, 5000, 3000 and 7000."},
		{Key: "priority", Type: plugin.FieldNumber, Label: "Priority", Default: 25,
			Help: "Lower numbers win when the same release is found on several indexers (1 to 50)."},
	},
}

func init() {
	plugin.Default.MustRegister(plugin.Registration{Manifest: manifest, New: New})
}

// Indexer is one configured Torznab instance.
type Indexer struct {
	inst     plugin.Instance
	endpoint *url.URL
	apiKey   string
	cats     map[domain.MediaKind][]int
	ref      domain.IndexerRef
	links    *linkCache

	mu   sync.Mutex
	caps *parsedCaps
}

// New is the plugin factory: it validates settings once, here.
func New(inst plugin.Instance) (any, error) {
	endpoint, err := parseEndpoint(inst.Settings)
	if err != nil {
		return nil, err
	}
	cats, err := parseCategories(inst.Settings.Map("categories"))
	if err != nil {
		return nil, err
	}
	return &Indexer{
		inst: inst, endpoint: endpoint, apiKey: inst.Settings.String("apiKey"), cats: cats,
		ref:   domain.IndexerRef{Key: inst.Key, Name: inst.Name, Priority: inst.Settings.Int("priority", 25)},
		links: newLinkCache(2048),
	}, nil
}

func parseEndpoint(s plugin.Settings) (*url.URL, error) {
	raw := s.String("url")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, plugin.Errorf(plugin.ClassConfig, "torznab url %q must be an http(s) address", raw)
	}
	path := "/api"
	if v, ok := s["apiPath"].(string); ok {
		path = v
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	return u, nil
}

// get calls the API with the given parameters.
func (ix *Indexer) get(ctx context.Context, vals url.Values, max int64) (*plugin.Response, error) {
	u := *ix.endpoint
	q := u.Query()
	for k, v := range vals {
		q[k] = v
	}
	if ix.apiKey != "" {
		q.Set("apikey", ix.apiKey)
	}
	u.RawQuery = q.Encode()
	return ix.fetch(ctx, u.String(), max)
}

func (ix *Indexer) fetch(ctx context.Context, rawURL string, max int64) (*plugin.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, plugin.Wrap(plugin.ClassConfig, "building request", err)
	}
	return plugin.Fetch(ix.inst.Env.HTTP(), req, max)
}

// Test fetches and parses the capabilities document.
func (ix *Indexer) Test(ctx context.Context) error {
	_, err := ix.loadCaps(ctx)
	return err
}

// Caps returns the (cached) capabilities.
func (ix *Indexer) Caps(ctx context.Context) (plugin.Caps, error) {
	ix.mu.Lock()
	cached := ix.caps
	ix.mu.Unlock()
	if cached != nil {
		return cached.Caps, nil
	}
	pc, err := ix.loadCaps(ctx)
	return pc.Caps, err
}

func (ix *Indexer) loadCaps(ctx context.Context) (parsedCaps, error) {
	resp, err := ix.get(ctx, url.Values{"t": {"caps"}}, maxXML)
	if err != nil {
		return parsedCaps{}, err
	}
	pc, err := parseCaps(resp.Body)
	if err != nil {
		return parsedCaps{}, err
	}
	ix.mu.Lock()
	ix.caps = &pc
	ix.mu.Unlock()
	return pc, nil
}

func (ix *Indexer) capsOrLoad(ctx context.Context) (parsedCaps, error) {
	ix.mu.Lock()
	cached := ix.caps
	ix.mu.Unlock()
	if cached != nil {
		return *cached, nil
	}
	return ix.loadCaps(ctx)
}
