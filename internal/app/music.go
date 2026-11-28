package app

import (
	"context"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/instances"
	"github.com/jrand0m/corsarr/internal/music"
	"github.com/jrand0m/corsarr/internal/musicauto"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/scheduler"
)

// The music media kind's wiring: the music module, its automation, their
// jobs, subscriptions and routes.

func (a *App) newMusic() *music.Service {
	return music.New(music.Deps{Repo: a.d.Store.Music(), Files: a.d.Store.Library(), Pub: a.d.Bus, Metadata: a.musicMetadata,
		RootFolders: a.musicRoots, Naming: a.namingOf(domain.KindMusic), Modes: a.mode, Art: a.art, Log: a.d.Log})
}

func (a *App) newMusicAuto() *musicauto.Service {
	st := a.d.Store
	return musicauto.New(musicauto.Deps{Music: a.Music, Queue: a.Queue, Blocklist: st.Blocklist(), Indexers: a.indexers,
		Pub: a.d.Bus, Payload: decide.PayloadPolicy{}, Modes: a.mode, Recent: st.RecentReleases(), Searches: st.Searches(), Log: a.d.Log})
}

func (a *App) musicMetadata() (plugin.MusicMetadata, bool) {
	list := usable[plugin.MusicMetadata](a, plugin.KindMetadata)
	if len(list) == 0 {
		return nil, false
	}
	return list[0].Impl, true
}

func (a *App) musicRoots() []string {
	var out []string
	for _, r := range a.roots() {
		if r.Kind == domain.KindMusic {
			out = append(out, r.Path)
		}
	}
	return out
}

func (a *App) musicJobs() []scheduler.Job {
	return []scheduler.Job{
		{Name: music.RefreshJob, Interval: music.RefreshInterval, Heavy: true, Run: a.Music.RefreshAll},
		{Name: music.ReleaseJob, Interval: music.ReleaseInterval, Run: a.Music.CheckReleased},
	}
}

// startMusic loads the album wanted index and subscribes music automation.
func (a *App) startMusic(ctx context.Context) error {
	a.Music.Start(ctx)
	if err := a.MusicAuto.Rebuild(ctx); err != nil {
		return err
	}
	a.listen(ctx, typeIn(domain.EvItemAdded, domain.EvItemUpdated, domain.EvItemDeleted, domain.EvItemAvailable,
		domain.EvImportCompleted, domain.EvFileDeleted, domain.EvGrabSent, domain.EvDownloadFailed, domain.EvGrabFailed,
		domain.EvPayloadRejected, domain.EvDownloadRemoved), a.musicEvent)
	return nil
}

func (a *App) musicEvent(ctx context.Context, ev domain.Event) {
	a.MusicAuto.OnFailure(ctx, ev)
	a.MusicAuto.OnEvent(ctx, ev)
}

// musicItem names a music item reference for queue events.
func (a *App) musicItem(ctx context.Context, r domain.ItemRef) domain.EventItem {
	if r.Variant == domain.VariantArtist {
		if ar, err := a.Music.Artist(ctx, r.ID); err == nil {
			return ar.EventItem()
		}
		return domain.EventItem{Ref: r}
	}
	return a.Music.AlbumItem(ctx, r.ID)
}

// sources are the running direct sources seen through one extension point
// (a source is an indexer and a downloader at once).
func sources[T any](a *App) []instances.Named[T] {
	return usable[T](a, plugin.KindSource)
}
