package app

import (
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/mcp"
)

// newMCP wires the assistant endpoint onto the same services the native API
// uses.
func (a *App) newMCP() *mcp.Service {
	st := a.d.Store
	return mcp.New(mcp.Deps{Config: a.d.Config, Modes: domain.Modes(a.mode), Movies: a.Movies, MovieSearch: a.Automation,
		TV: a.TV, TVSearch: a.TVAuto, Music: a.Music, MusicSearch: a.MusicAuto, Queue: a.Queue, Downloaders: a.Queue, Library: st.Library(),
		Searches: st.Searches(), History: st.History(), Health: a.d.Health, RootFolders: a.Roots, Bus: a.d.Bus,
		Go: a.background, Log: a.d.Log, Plugins: mcpPlugins{a}})
}
