// Package plex is the built-in Plex media server plugin, implemented from the
// public Plex HTTP API: identity and library listing, and partial
// library refreshes by path.
package plex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/jrand0m/corsarr/internal/plugin"
)

// ID is the plugin id used as `type:` in config.
const ID = "plex"

const maxJSON = 4 << 20

var manifest = plugin.Manifest{
	ID: ID, Kind: plugin.KindMediaServer, Version: "1.0.0", APIVersion: plugin.APIVersion,
	Name: "Plex", Delivery: "builtin",
	Capabilities: []string{"refresh-paths", "identity"},
	SetupFlows:   []plugin.SetupFlow{plugin.FlowPinPairing},
	Settings: []plugin.Field{
		{Key: "url", Type: plugin.FieldURL, Label: "Server address", Required: true,
			Help: "Address of your Plex server, for example http://plex:32400. It stays exactly as typed; corsarr never replaces it with an address Plex advertises."},
		{Key: "token", Type: plugin.FieldSecret, Label: "Plex token", Required: true,
			Help: "Plex authentication token. Paste one, or use Sign in with Plex when that flow is available."},
		{Key: "machineId", Type: plugin.FieldText, Label: "Server identity",
			Help: "Machine identifier of the paired server. When set, tests fail if another server answers at this address."},
		{Key: "sectionMap", Type: plugin.FieldMap, Label: "Path mapping",
			Help: "Maps path prefixes as corsarr sees them to the same folders as Plex sees them, for example /data/movies to /media/movies. Needed only when the two differ."},
	},
}

func init() {
	plugin.Default.MustRegister(plugin.Registration{Manifest: manifest, New: New})
}

// Server is one configured Plex instance.
type Server struct {
	inst      plugin.Instance
	base      *url.URL
	token     string
	machineID string
	mappings  []mapping
}

// New is the plugin factory: settings are validated once, here.
func New(inst plugin.Instance) (any, error) {
	s := inst.Settings
	base, err := url.Parse(strings.TrimRight(s.String("url"), "/"))
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, plugin.Errorf(plugin.ClassConfig, "plex url %q must be an http(s) address", s.String("url"))
	}
	if s.String("token") == "" {
		return nil, plugin.Errorf(plugin.ClassConfig, "plex needs a token")
	}
	maps, err := parseMappings(s.Map("sectionMap"))
	if err != nil {
		return nil, err
	}
	return &Server{inst: inst, base: base, token: s.String("token"), machineID: s.String("machineId"), mappings: maps}, nil
}

type container[T any] struct {
	MediaContainer T `json:"MediaContainer"`
}

func (s *Server) get(ctx context.Context, path string, q url.Values, out any) error {
	u := *s.base
	u.Path += path
	if q != nil {
		u.RawQuery = q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return plugin.Wrap(plugin.ClassConfig, "building request", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", s.token)
	req.Header.Set("X-Plex-Product", "corsarr")
	resp, err := plugin.Fetch(s.inst.Env.HTTP(), req, maxJSON)
	if err != nil {
		return err
	}
	if resp.Status >= 400 {
		return plugin.Errorf(plugin.ClassServerError, "plex answered HTTP %d for %s", resp.Status, path)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(resp.Body, out); err != nil {
		return plugin.Wrap(plugin.ClassParse, "plex "+path, err)
	}
	return nil
}

type identity struct {
	MachineIdentifier string `json:"machineIdentifier"`
	Version           string `json:"version"`
}

// Test checks reachability and identity, then the token and library listing.
func (s *Server) Test(ctx context.Context) error {
	var id container[identity]
	if err := s.get(ctx, "/identity", nil, &id); err != nil {
		return err
	}
	if id.MediaContainer.MachineIdentifier == "" {
		return plugin.Errorf(plugin.ClassParse, "plex identity has no machineIdentifier")
	}
	if s.machineID != "" && s.machineID != id.MediaContainer.MachineIdentifier {
		return plugin.Errorf(plugin.ClassIdentityMismatch, "address answers as server %s, paired with %s",
			id.MediaContainer.MachineIdentifier, s.machineID)
	}
	_, err := s.sections(ctx)
	return err
}
