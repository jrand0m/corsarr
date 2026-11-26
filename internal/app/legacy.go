package app

import (
	"context"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/legacy"
)

// newLegacy wires the compatibility API onto the same services the native
// API uses.
func (a *App) newLegacy() *legacy.Service {
	st := a.d.Store
	return legacy.New(legacy.Deps{Config: a.d.Config, Modes: domain.Modes(a.mode), TV: a.TV, TVSearch: a.TVAuto,
		Movies: a.Movies, MovieSearch: a.Automation, Music: a.Music, MusicWanted: a.MusicAuto, Queue: a.Queue,
		RootFolders: a.Roots, Indexers: a.IndexerAdmin, Queries: st.Searches(), Grabs: st.Queue(), IDs: st.LegacyIDs(),
		Go: a.background, Log: a.d.Log})
}

// background runs fn on the pipeline's context; Close waits for it.
func (a *App) background(fn func(context.Context)) {
	a.modesMu.Lock()
	ctx := a.ctx
	a.modesMu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	a.goRun(func() { fn(ctx) })
}
