// Package trubadurcamp is the TrubadurCamp music source: albums bought on
// (or free to download from) TrubadurCamp are searched next to torrent
// results and fetched in the chosen format. It reaches the site through
// the source protocol (see sourcekit.HTTPSite) with the owner's identity
// cookie.
package trubadurcamp

import (
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugins/sourcekit"
)

// ID is the plugin id used as `type:` in config.
const ID = "trubadurcamp"

var formats = []string{"flac", "mp3-320", "mp3-v0", "alac", "aac-hi", "vorbis", "wav"}

var manifest = plugin.Manifest{
	ID: ID, Kind: plugin.KindSource, Version: "1.0.0", APIVersion: plugin.APIVersion,
	Name: "TrubadurCamp", Delivery: "builtin",
	Capabilities: []string{"music", "search", "fetch"},
	Settings: []plugin.Field{
		{Key: "baseUrl", Type: plugin.FieldURL, Label: "Source address", Required: true,
			Help: "Address of the TrubadurCamp source service corsarr searches and downloads through."},
		{Key: "identity", Type: plugin.FieldSecret, Label: "Identity cookie",
			Help: "The session cookie of your TrubadurCamp account; it unlocks the albums in your collection."},
		{Key: "format", Type: plugin.FieldEnum, Label: "Download format", Default: "flac", Options: formats,
			Help: "The format albums are downloaded in; FLAC keeps the original quality."},
		{Key: "downloadDir", Type: plugin.FieldPath, Label: "Download folder", Required: true,
			Help: "Where fetched albums are written before import; on the same file system as the music library they are hardlinked."},
	},
}

func init() {
	plugin.Default.MustRegister(plugin.Registration{Manifest: manifest, New: New})
}

// New is the plugin factory: settings are validated once, here.
func New(inst plugin.Instance) (any, error) {
	s := inst.Settings
	base, err := sourcekit.ParseBase(ID, s.String("baseUrl"))
	if err != nil {
		return nil, err
	}
	format := s.String("format")
	if format == "" {
		format = "flac"
	}
	inst.PluginID = ID
	site := sourcekit.HTTPSite{Base: base, Cookie: cookie(s.String("identity")), Format: format, HTTP: inst.Env.HTTP(), Name: "TrubadurCamp"}
	return sourcekit.New(inst, site, s.String("downloadDir"))
}

func cookie(identity string) string {
	if identity == "" {
		return ""
	}
	return "identity=" + identity
}
