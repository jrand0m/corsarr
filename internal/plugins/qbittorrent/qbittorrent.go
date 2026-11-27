// Package qbittorrent is the built-in qBittorrent downloader plugin, implemented
// from the public WebAPI v2 documentation. It supports 4.x and 5.x.
package qbittorrent

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// ID is the plugin id used as `type:` in config.
const ID = "qbittorrent"

const (
	maxSmall = 1 << 20
	maxList  = 16 << 20
	maxFile  = 5 << 20
)

var defaultCategories = map[string]any{"tv": "tv", "movie": "movies", "music": "music", "book": "books"}

var manifest = plugin.Manifest{
	ID: ID, Kind: plugin.KindDownloader, Version: "1.0.0", APIVersion: plugin.APIVersion,
	Name: "qBittorrent", Delivery: "builtin",
	Capabilities: []string{"torrent", "magnet", "categories", "priority", "priority-levels", "pause", "pause-all", "speed-limits", "alt-speed", "recheck", "reannounce", "set-category", "stats", "trackers"},
	Settings: []plugin.Field{
		{Key: "url", Type: plugin.FieldURL, Label: "Web UI address", Required: true,
			Help: "Address of the qBittorrent Web UI, for example http://qbittorrent:8080. A reverse-proxy sub-path is allowed."},
		{Key: "username", Type: plugin.FieldText, Label: "Username",
			Help: "Web UI user name. Leave empty when authentication is bypassed for this host."},
		{Key: "password", Type: plugin.FieldSecret, Label: "Password",
			Help: "Web UI password."},
		{Key: "categories", Type: plugin.FieldMap, Label: "Category per media kind", Default: defaultCategories,
			Help: "qBittorrent category used for each media kind (movie, tv, music, book). Imports only look at these categories."},
		{Key: "pathMap", Type: plugin.FieldList, Label: "Path mapping", Check: plugin.CheckPathRule,
			Help: "Needed when qBittorrent sees its download folders under other paths than corsarr (another host, or containers with different mounts). One remote=local entry per folder, for example /downloads=/mnt/media/downloads. The longest matching prefix wins."},
	},
}

func init() {
	plugin.Default.MustRegister(plugin.Registration{Manifest: manifest, New: New})
}

var versionRe = regexp.MustCompile(`^v?(\d+)\.`)

// Client is one configured qBittorrent instance.
type Client struct {
	inst       plugin.Instance
	base       *url.URL
	categories map[domain.MediaKind]string
	paths      plugin.PathMap
	dirExists  func(path string) bool

	mu       sync.Mutex
	loggedIn bool
	cookie   string
	major    int // 0 = unknown
}

// New is the plugin factory: settings are validated once, here.
func New(inst plugin.Instance) (any, error) {
	base, err := url.Parse(strings.TrimRight(inst.Settings.String("url"), "/"))
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, plugin.Errorf(plugin.ClassConfig, "qbittorrent url %q must be an http(s) address", inst.Settings.String("url"))
	}
	cats, err := parseCategories(inst.Settings.Map("categories"))
	if err != nil {
		return nil, err
	}
	paths, err := plugin.ParsePathMap(inst.Settings.Strings("pathMap"))
	if err != nil {
		return nil, plugin.Wrap(plugin.ClassConfig, "pathMap", err)
	}
	return &Client{inst: inst, base: base, categories: cats, paths: paths, dirExists: dirExists}, nil
}

func parseCategories(raw map[string]any) (map[domain.MediaKind]string, error) {
	out := map[domain.MediaKind]string{}
	for k, v := range defaultCategories {
		kind, _ := domain.ParseMediaKind(k)
		out[kind] = v.(string)
	}
	for k, v := range raw {
		kind, err := domain.ParseMediaKind(k)
		if err != nil {
			return nil, plugin.Wrap(plugin.ClassConfig, "categories", err)
		}
		name, _ := v.(string)
		if strings.TrimSpace(name) == "" {
			return nil, plugin.Errorf(plugin.ClassConfig, "categories.%s must be a category name", k)
		}
		out[kind] = name
	}
	return out, nil
}

// CategoryFor returns the configured qBittorrent category for a media kind.
func (c *Client) CategoryFor(kind domain.MediaKind) string { return c.categories[kind] }

// Protocols reports torrent only.
func (c *Client) Protocols() []domain.Protocol { return []domain.Protocol{domain.ProtoTorrent} }

// Test logs in, reads the application version and checks that the folders
// qBittorrent saves into exist locally.
func (c *Client) Test(ctx context.Context) error {
	if _, err := c.version(ctx); err != nil {
		return err
	}
	return c.checkPaths(ctx)
}

func (c *Client) version(ctx context.Context) (int, error) {
	resp, err := c.get(ctx, "app/version", nil, maxSmall)
	if err != nil {
		return 0, err
	}
	m := versionRe.FindStringSubmatch(strings.TrimSpace(string(resp.Body)))
	if m == nil {
		return 0, plugin.Errorf(plugin.ClassParse, "unexpected app/version answer")
	}
	major, _ := strconv.Atoi(m[1])
	c.mu.Lock()
	c.major = major
	c.mu.Unlock()
	return major, nil
}

// majorVersion returns the cached major version, asking the client once if needed.
func (c *Client) majorVersion(ctx context.Context) (int, error) {
	c.mu.Lock()
	m := c.major
	c.mu.Unlock()
	if m != 0 {
		return m, nil
	}
	return c.version(ctx)
}

func (c *Client) endpoint(path string, q url.Values) string {
	u := *c.base
	u.Path += "/api/v2/" + path
	if q != nil {
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func (c *Client) get(ctx context.Context, path string, q url.Values, max int64) (*plugin.Response, error) {
	return c.call(ctx, max, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint(path, q), nil)
	})
}
