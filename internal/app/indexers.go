package app

import (
	"context"
	"path/filepath"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/httpx"
	"github.com/jrand0m/corsarr/internal/indexers"
	"github.com/jrand0m/corsarr/internal/instances"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugins/declarative"
)

// newIndexerAdmin builds the indexer settings service over the store and the
// definition catalogue, and loads the owner definitions and UI instances.
func (a *App) newIndexerAdmin(ctx context.Context) error {
	d, st := a.d, a.d.Store
	plain, _ := d.HTTP.(plugin.HTTPDoer)
	a.IndexerAdmin = indexers.New(indexers.Deps{
		Repo: st.Indexers(), Catalog: declarative.Default, Plugins: d.Plugins,
		UserDir: filepath.Join(st.Dir(), declarative.UserDir), Config: d.Config,
		Running: func() []instances.Running { return a.Instances.All(plugin.KindIndexer) },
		HTTP:    d.HTTP, Plain: plain, Health: d.Health, Queries: st.Searches(), KV: st.Settings(),
		Apply: func() { a.applyInstances(a.d.Config()) }, Log: d.Log,
		BasePath: func() string { return a.d.Config().Server.BasePath },
	})
	return a.IndexerAdmin.Load(ctx)
}

// applyInstances runs the config's instances plus the UI-managed indexers,
// then hands each definition's request spacing to the shared client.
func (a *App) applyInstances(cfg *config.Config) {
	if a.IndexerAdmin != nil {
		cfg = a.IndexerAdmin.Merged(cfg)
	}
	a.Instances.Apply(cfg)
	limiter, ok := a.d.HTTP.(interface{ SetLimits(string, httpx.Limits) })
	if !ok {
		return
	}
	for _, r := range a.Instances.All(plugin.KindIndexer) {
		if p, ok := r.Impl.(interface {
			MinInterval() (string, time.Duration)
		}); ok {
			if host, every := p.MinInterval(); every > 0 {
				limiter.SetLimits(host, httpx.Limits{Rate: float64(time.Second) / float64(every), Burst: 1})
			}
		}
	}
}

// reporting records failures the shared client cannot see (a results page
// that could not be parsed, an expired session) in the indexer's health.
type reporting struct {
	plugin.Indexer
	ref    domain.IntegrationRef
	health *health.Registry
}

func (r reporting) Search(ctx context.Context, req domain.SearchRequest) ([]domain.Release, error) {
	rels, err := r.Indexer.Search(ctx, req)
	r.note(err)
	return rels, err
}

func (r reporting) Feed(ctx context.Context, kind domain.MediaKind, cursor string) ([]domain.Release, string, error) {
	rels, next, err := r.Indexer.Feed(ctx, kind, cursor)
	r.note(err)
	return rels, next, err
}

func (r reporting) note(err error) { indexers.ReportUnseen(r.health, r.ref, err) }
