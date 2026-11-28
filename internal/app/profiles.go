package app

import (
	"context"

	"github.com/jrand0m/corsarr/internal/profiles"
)

// newProfiles builds the custom format and profile service and puts the
// catalog in force before any module decides.
func (a *App) newProfiles(ctx context.Context) error {
	a.Profiles = profiles.New(profiles.Deps{Repo: a.d.Store.Quality(), Config: a.d.Config, Pub: a.d.Bus, Log: a.d.Log})
	return a.Profiles.Load(ctx)
}
