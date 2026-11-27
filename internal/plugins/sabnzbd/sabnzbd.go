// Package sabnzbd is the built-in downloader plugin for services speaking the
// SABnzbd JSON API (SABnzbd itself and compatible bridges), implemented from
// the public API documentation.
package sabnzbd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// ID is the plugin id used as `type:` in config.
const ID = "sabnzbd"

const (
	maxSmall = 1 << 20
	maxList  = 16 << 20
)

var defaultCategories = map[string]any{"tv": "tv", "movie": "movies", "music": "music", "book": "books"}

var manifest = plugin.Manifest{
	ID: ID, Kind: plugin.KindDownloader, Version: "1.0.0", APIVersion: plugin.APIVersion,
	Name: "SABnzbd-compatible", Delivery: "builtin",
	Capabilities: []string{"nzb", "categories", "priority", "priority-levels", "pause", "pause-all", "speed-limits", "stats"},
	Settings: []plugin.Field{
		{Key: "url", Type: plugin.FieldURL, Label: "Address", Required: true,
			Help: "Address of the SABnzbd web interface, for example http://sabnzbd:8080. A reverse-proxy sub-path is allowed."},
		{Key: "apiKey", Type: plugin.FieldSecret, Label: "API key", Required: true,
			Help: "The full API key from the SABnzbd general settings (not the NZB key)."},
		{Key: "categories", Type: plugin.FieldMap, Label: "Category per media kind", Default: defaultCategories,
			Help: "SABnzbd category used for each media kind (movie, tv, music, book). A missing category is created in SABnzbd."},
		{Key: "pathMap", Type: plugin.FieldList, Label: "Path mapping", Check: plugin.CheckPathRule,
			Help: "Needed when SABnzbd sees its folders under other paths than corsarr. One remote=local entry per folder, for example /downloads=/mnt/media/downloads. The longest matching prefix wins."},
	},
}

func init() {
	plugin.Default.MustRegister(plugin.Registration{Manifest: manifest, New: New})
}

// Client is one configured SABnzbd-compatible service.
type Client struct {
	inst       plugin.Instance
	base       *url.URL
	key        string
	categories map[domain.MediaKind]string
	paths      plugin.PathMap
}

// New is the plugin factory: settings are validated once, here.
func New(inst plugin.Instance) (any, error) {
	base, err := url.Parse(strings.TrimRight(inst.Settings.String("url"), "/"))
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, plugin.Errorf(plugin.ClassConfig, "sabnzbd url %q must be an http(s) address", inst.Settings.String("url"))
	}
	key := strings.TrimSpace(inst.Settings.String("apiKey"))
	if key == "" {
		return nil, plugin.Errorf(plugin.ClassConfig, "sabnzbd apiKey is required")
	}
	cats, err := parseCategories(inst.Settings.Map("categories"))
	if err != nil {
		return nil, err
	}
	paths, err := plugin.ParsePathMap(inst.Settings.Strings("pathMap"))
	if err != nil {
		return nil, plugin.Wrap(plugin.ClassConfig, "pathMap", err)
	}
	return &Client{inst: inst, base: base, key: key, categories: cats, paths: paths}, nil
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

// CategoryFor returns the configured category for a media kind.
func (c *Client) CategoryFor(kind domain.MediaKind) string { return c.categories[kind] }

// Protocols reports usenet only.
func (c *Client) Protocols() []domain.Protocol { return []domain.Protocol{domain.ProtoUsenet} }

// Test checks the API key with a small queue read.
func (c *Client) Test(ctx context.Context) error {
	_, err := c.queue(ctx, url.Values{"limit": {"1"}})
	return err
}

func (c *Client) endpoint(mode string, q url.Values) string {
	u := *c.base
	u.Path += "/api"
	v := url.Values{}
	for k, vs := range q {
		v[k] = vs
	}
	v.Set("mode", mode)
	v.Set("apikey", c.key)
	v.Set("output", "json")
	u.RawQuery = v.Encode()
	return u.String()
}

// apiStatus is the error envelope every call may answer with.
type apiStatus struct {
	Status *bool  `json:"status"`
	Error  string `json:"error"`
}

// call sends one API request and decodes the answer into out (nil = only
// check the status envelope).
func (c *Client) call(ctx context.Context, mode string, q url.Values, max int64, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint(mode, q), nil)
	if err != nil {
		return plugin.Wrap(plugin.ClassConfig, "building request", err)
	}
	return c.do(req, mode, max, out)
}

// post sends a multipart body (an NZB file upload).
func (c *Client) post(ctx context.Context, mode string, q url.Values, ctype string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(mode, q), bytes.NewReader(body))
	if err != nil {
		return plugin.Wrap(plugin.ClassConfig, "building request", err)
	}
	req.Header.Set("Content-Type", ctype)
	return c.do(req, mode, maxSmall, out)
}

func (c *Client) do(req *http.Request, mode string, max int64, out any) error {
	resp, err := plugin.Do(c.inst.Env.HTTP(), req, max)
	if err != nil {
		return err
	}
	if class := plugin.ClassifyStatus(resp.Status); class != "" {
		return plugin.Errorf(class, "SABnzbd answered HTTP %d", resp.Status)
	}
	if resp.Status >= 400 {
		return plugin.Errorf(plugin.ClassServerError, "SABnzbd answered HTTP %d", resp.Status)
	}
	return decode(resp.Body, mode, out)
}

func decode(body []byte, mode string, out any) error {
	var st apiStatus
	if err := json.Unmarshal(body, &st); err != nil {
		return plugin.Wrap(plugin.ClassParse, mode, err)
	}
	if st.Error != "" || (st.Status != nil && !*st.Status) {
		return apiError(st.Error)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return plugin.Wrap(plugin.ClassParse, mode, err)
	}
	return nil
}

func apiError(msg string) error {
	if strings.Contains(strings.ToLower(msg), "api key") {
		return plugin.Errorf(plugin.ClassAuth, "SABnzbd rejected the API key")
	}
	if msg == "" {
		msg = "the request failed"
	}
	return plugin.Errorf(plugin.ClassServerError, "SABnzbd: %s", msg)
}

// num reads the numbers SABnzbd sends either as JSON numbers or as strings.
type num float64

func (n *num) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*n = 0
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	*n = num(f)
	return err
}

// local turns a path SABnzbd reported into corsarr's view of it.
func (c *Client) local(p string) string {
	out, _ := c.paths.ToLocal(p)
	return out
}
