package app

import (
	"context"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/scheduler"
	"github.com/jrand0m/corsarr/internal/tv"
	"github.com/jrand0m/corsarr/internal/tvauto"
)

// The TV media kind's wiring: the tv module, its automation, their jobs,
// subscriptions and routes.

func (a *App) newTV() *tv.Service {
	return tv.New(tv.Deps{Repo: a.d.Store.TV(), Files: a.d.Store.Library(), Pub: a.d.Bus, Metadata: a.seriesMetadata,
		RootFolders: a.tvRoots, Modes: a.mode, Log: a.d.Log})
}

func (a *App) newTVAuto() *tvauto.Service {
	st := a.d.Store
	return tvauto.New(tvauto.Deps{TV: a.TV, Files: st.Library(), Queue: a.Queue, Blocklist: st.Blocklist(),
		Cursors: st.FeedCursors(), Recent: st.RecentReleases(), Indexers: a.indexers, Pub: a.d.Bus, Payload: decide.PayloadPolicy{}, Modes: a.mode, Log: a.d.Log})
}

func (a *App) seriesMetadata() (plugin.SeriesMetadata, bool) {
	list := usable[plugin.SeriesMetadata](a, plugin.KindMetadata)
	if len(list) == 0 {
		return nil, false
	}
	return list[0].Impl, true
}

func (a *App) tvRoots() []string {
	var out []string
	for _, r := range a.roots() {
		if r.Kind == domain.KindTV {
			out = append(out, r.Path)
		}
	}
	return out
}

func (a *App) tvJobs() []scheduler.Job {
	return []scheduler.Job{
		{Name: tv.RefreshJob, Interval: tv.RefreshInterval, Heavy: true, Run: a.TV.RefreshAll},
		{Name: tvauto.RSSJob, Interval: tvauto.RSSInterval, Heavy: true, Run: a.TVAuto.SyncRSS},
	}
}

// startTV loads the TV wanted index, subscribes TV automation and arms the
// episode availability timer.
func (a *App) startTV(ctx context.Context) error {
	if err := a.TVAuto.Rebuild(ctx); err != nil {
		return err
	}
	a.listen(ctx, typeIn(domain.EvItemAdded, domain.EvItemUpdated, domain.EvItemDeleted, domain.EvItemAvailable,
		domain.EvImportCompleted, domain.EvFileDeleted, domain.EvGrabSent, domain.EvDownloadFailed, domain.EvGrabFailed,
		domain.EvPayloadRejected, domain.EvDownloadRemoved), a.tvEvent)
	a.TV.Start(ctx)
	return nil
}

func (a *App) tvEvent(ctx context.Context, ev domain.Event) {
	a.TVAuto.OnFailure(ctx, ev)
	a.TVAuto.OnEvent(ctx, ev)
}

// tvItem names a TV item reference for queue events.
func (a *App) tvItem(ctx context.Context, r domain.ItemRef) domain.EventItem {
	if r.Variant == domain.VariantSeries {
		if s, err := a.TV.Series(ctx, r.ID); err == nil {
			return s.EventItem()
		}
		return domain.EventItem{Ref: r}
	}
	return a.TV.EpisodeItem(ctx, r.ID)
}
