package plugins

import (
	"testing"

	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/conformance"
)

func TestEveryBuiltInIsRegisteredAndValid(t *testing.T) {
	want := map[string]plugin.Kind{
		"torznab": plugin.KindIndexer, "qbittorrent": plugin.KindDownloader, "tmdb": plugin.KindMetadata,
		"plex": plugin.KindMediaServer, "native": plugin.KindNotifier, "shoutrrr": plugin.KindNotifier,
		"knaben": plugin.KindIndexer, "thepiratebay": plugin.KindIndexer,
		"musicbrainz": plugin.KindMetadata, "trubadurcamp": plugin.KindSource, "mechanicalwavescloud": plugin.KindSource,
	}
	for id, kind := range want {
		reg, ok := plugin.Default.Get(id)
		if !ok {
			t.Errorf("%s not registered", id)
			continue
		}
		conformance.Manifest(t, reg.Manifest, kind)
	}
	if got := len(plugin.Default.List("")); got != len(want) {
		t.Errorf("%d plugins registered, want %d", got, len(want))
	}
}
