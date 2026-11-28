package app

import (
	"context"
	"path/filepath"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/imagecache"
	"github.com/jrand0m/corsarr/internal/music"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// linkPurpose names the key that signs artwork links.
const linkPurpose = "artwork-links"

// imageSettings maps the cache section onto the cache's settings; an empty
// folder means the cache folder inside the data folder.
func imageSettings(c config.Cache, dataDir string) imagecache.Settings {
	dir := c.Dir
	if dir == "" {
		dir = filepath.Join(dataDir, "cache")
	}
	d := c.Images.Degrade
	return imagecache.Settings{Enabled: c.Enabled, Dir: dir, MaxSize: c.MaxSize, Quality: c.Images.Quality,
		MaxWidth: c.Images.MaxWidth, BackdropMaxWidth: c.Images.BackdropMaxWidth,
		Degrade: imagecache.Degrade{QualityFloor: d.QualityFloor, QualityStep: d.QualityStep, WidthFloor: d.WidthFloor,
			WidthStep: d.WidthStep, HighWater: d.HighWater, LowWater: d.LowWater}}
}

// artworkRef attributes artwork fetches when the client has no plain Do.
var artworkRef = domain.IntegrationRef{Type: "cache", Key: "artwork"}

func (a *App) newImages() *imagecache.Cache {
	st := a.d.Store
	doer, ok := a.d.HTTP.(plugin.HTTPDoer)
	if !ok {
		doer = a.d.HTTP.For(artworkRef)
	}
	return imagecache.New(imagecache.Deps{Settings: imageSettings(a.d.Config().Cache, st.Dir()), Index: st.Images(),
		Metadata: movieRecords{a}, HTTP: doer, Signer: imagecache.NewSigner(st.SubKey(linkPurpose)),
		Library: a.libraryArtwork, Kick: func() { _ = a.d.Jobs.RunNow(imagecache.DegradeJob) }, Log: a.d.Log})
}

// art is the link function handed to the media modules.
func (a *App) art(role domain.ArtRole, origin string) string { return a.Images.Link(role, origin) }

// libraryArtwork lists the provider addresses of artwork used by library
// items: movie and series posters and fanart, and album covers.
func (a *App) libraryArtwork(ctx context.Context) (map[string]bool, error) {
	urls, err := a.d.Store.ArtworkOrigins(ctx)
	if err != nil {
		return nil, err
	}
	mbids, err := a.d.Store.Music().AlbumMBIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(urls)+len(mbids))
	for _, u := range urls {
		out[u] = true
	}
	for _, id := range mbids {
		out[music.CoverURL(id)] = true
	}
	return out, nil
}

// movieRecords lets the cache count and evict provider records through the
// movies module, their only writer.
type movieRecords struct{ a *App }

func (m movieRecords) Bytes(ctx context.Context) (int64, error) { return m.a.Movies.MetadataBytes(ctx) }
func (m movieRecords) EvictUnused(ctx context.Context, want int64) (int, int64, error) {
	return m.a.Movies.EvictMetadata(ctx, want)
}
