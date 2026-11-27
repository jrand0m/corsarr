// Package plugins blank-imports every built-in plugin so that importing this
// one package registers them all into plugin.Default. Lean builds
// drop the shoutrrr notifier with `-tags nonotify_shoutrrr`.
package plugins

import (
	_ "github.com/jrand0m/corsarr/internal/plugins/declarative" // bundled tracker definitions
	_ "github.com/jrand0m/corsarr/internal/plugins/mechanicalwavescloud"
	_ "github.com/jrand0m/corsarr/internal/plugins/musicbrainz"
	_ "github.com/jrand0m/corsarr/internal/plugins/notify"
	_ "github.com/jrand0m/corsarr/internal/plugins/plex"
	_ "github.com/jrand0m/corsarr/internal/plugins/qbittorrent"
	_ "github.com/jrand0m/corsarr/internal/plugins/sabnzbd"
	_ "github.com/jrand0m/corsarr/internal/plugins/tmdb"
	_ "github.com/jrand0m/corsarr/internal/plugins/torznab"
	_ "github.com/jrand0m/corsarr/internal/plugins/trubadurcamp"
)
