// Package mechanicalwavescloud is the MechanicalWavesCloud music source:
// sets, mixes and tracks published on MechanicalWavesCloud are searched next
// to torrent results and fetched as audio files. It reaches the site through
// the source protocol (see sourcekit.HTTPSite) with the owner's access token.
package mechanicalwavescloud

import (
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugins/sourcekit"
)

// ID is the plugin id used as `type:` in config.
const ID = "mechanicalwavescloud"

var manifest = plugin.Manifest{
	ID: ID, Kind: plugin.KindSource, Version: "1.0.0", APIVersion: plugin.APIVersion,
	Name: "MechanicalWavesCloud", Delivery: "builtin",
	Capabilities: []string{"music", "search", "fetch"},
	Settings: []plugin.Field{
		{Key: "baseUrl", Type: plugin.FieldURL, Label: "Source address", Required: true,
			Help: "Address of the MechanicalWavesCloud source service corsarr searches and downloads through."},
		{Key: "token", Type: plugin.FieldSecret, Label: "Access token",
			Help: "An access token of your MechanicalWavesCloud account; it unlocks liked and private uploads."},
		{Key: "format", Type: plugin.FieldEnum, Label: "Download format", Default: "original", Options: []string{"original", "mp3"},
			Help: "original keeps the uploaded file where the uploader allows it; mp3 takes the streaming copy."},
		{Key: "downloadDir", Type: plugin.FieldPath, Label: "Download folder", Required: true,
			Help: "Where fetched audio is written before import; on the same file system as the music library it is hardlinked."},
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
		format = "original"
	}
	inst.PluginID = ID
	site := sourcekit.HTTPSite{Base: base, Token: s.String("token"), Format: format, HTTP: inst.Env.HTTP(), Name: "MechanicalWavesCloud"}
	return sourcekit.New(inst, site, s.String("downloadDir"))
}
