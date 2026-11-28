package app

import (
	"context"
	"errors"
	"time"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/importer"
	"github.com/jrand0m/corsarr/internal/instances"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/notify"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/queue"
	"github.com/jrand0m/corsarr/internal/rootfolders"
)

// usable drops instances whose health needs user action (no blind calls).
func usable[T any](a *App, kind plugin.Kind) []instances.Named[T] {
	var out []instances.Named[T]
	for _, n := range instances.Of[T](a.Instances, kind) {
		if !a.d.Health.Blocked(n.Ref()) {
			out = append(out, n)
		}
	}
	return out
}

// plugins adapts the instance manager to the queue's view.
type plugins struct{ a *App }

func (p plugins) Indexer(key domain.ObjectKey) (plugin.Indexer, bool) {
	n, ok := instances.Get[plugin.Indexer](p.a.Instances, plugin.KindIndexer, key)
	if !ok {
		n, ok = instances.Get[plugin.Indexer](p.a.Instances, plugin.KindSource, key)
	}
	return n.Impl, ok
}

func (p plugins) Downloaders() []queue.Downloader {
	var out []queue.Downloader
	for _, n := range append(usable[plugin.Downloader](p.a, plugin.KindDownloader), sources[plugin.Downloader](p.a)...) {
		out = append(out, queue.Downloader{Key: n.Key, Impl: n.Impl})
	}
	return out
}

// downloadClients lists every configured downloader with its health, usable
// or not, for the downloader overview.
func (a *App) downloadClients() []queue.ClientInfo {
	var out []queue.ClientInfo
	for _, n := range instances.Of[plugin.Downloader](a.Instances, plugin.KindDownloader) {
		c := queue.ClientInfo{Key: n.Key, Name: n.Name, Type: n.PluginID, Impl: n.Impl}
		if h, ok := a.d.Health.Get(n.Ref()); ok {
			c.Health = &h
		}
		out = append(out, c)
	}
	return out
}

func (a *App) testDownloader(ctx context.Context, key domain.ObjectKey) error {
	return a.Instances.Test(ctx, plugin.KindDownloader, key)
}

func (a *App) indexers() []automation.Indexer {
	var out []automation.Indexer
	var throttle func(domain.IntegrationRef, time.Duration)
	if t, ok := a.d.HTTP.(interface {
		Throttled(domain.IntegrationRef, time.Duration)
	}); ok {
		throttle = t.Throttled
	}
	for _, n := range append(usable[plugin.Indexer](a, plugin.KindIndexer), sources[plugin.Indexer](a)...) {
		out = append(out, automation.Indexer{Key: n.Key, Impl: reporting{Indexer: n.Impl, ref: n.Ref(), health: a.d.Health, throttle: throttle},
			Limit: automation.ParseLimit(n.Settings.Int("queryLimit", 0), first(n.Settings.Strings("queryLimitPer")))})
	}
	return out
}

func (a *App) metadata() (plugin.MovieMetadata, bool) {
	list := usable[plugin.MovieMetadata](a, plugin.KindMetadata)
	if len(list) == 0 {
		return nil, false
	}
	return list[0].Impl, true
}

func (a *App) connections() []notify.Connection {
	var out []notify.Connection
	for _, n := range instances.Of[plugin.Notifier](a.Instances, plugin.KindNotifier) {
		tpl := n.Settings.Map("template")
		title, _ := tpl["title"].(string)
		body, _ := tpl["body"].(string)
		t, err := notify.ParseTemplate(title, body) // validated at config load
		if err != nil {
			a.d.Log.Warn("notification template", "connection", n.Key, "err", err)
		}
		out = append(out, notify.Connection{Key: n.Key, Impl: n.Impl, Toggles: n.Settings.Strings("on"), Template: t})
	}
	return out
}

func (a *App) mediaServers() []notify.MediaServer {
	var out []notify.MediaServer
	for _, n := range instances.Of[plugin.MediaServer](a.Instances, plugin.KindMediaServer) {
		out = append(out, notify.MediaServer{Key: n.Key, Impl: n.Impl})
	}
	return out
}

func (a *App) roots() []rootfolders.Configured {
	var out []rootfolders.Configured
	for _, r := range a.d.Config().RootFolders {
		out = append(out, rootfolders.Configured{Path: r.Path, Kind: r.Kind})
	}
	return out
}

func (a *App) movieRoots() []string {
	var out []string
	for _, r := range a.roots() {
		if r.Kind == domain.KindMovie {
			out = append(out, r.Path)
		}
	}
	return out
}

func first(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	return xs[0]
}

// folders hands item-folder moves to the importer (which owns library files).
type folders struct{ a *App }

func (f folders) Move(ctx context.Context, item domain.ItemRef, from, to string) (any, error) {
	plan, err := f.a.Importer.MoveItem(ctx, item, from, to)
	if errors.Is(err, importer.ErrMoveConflict) {
		err = movies.FieldError{Field: "rootFolder", Msg: err.Error()}
	}
	return plan, err
}

func (f folders) Plan(ctx context.Context, item domain.ItemRef, from, to string) (any, error) {
	plan, _, err := f.a.Importer.PlanMove(ctx, item, from, to)
	return plan, err
}

// itemTitles snapshots item titles for queue events.
func (a *App) itemTitles(ctx context.Context, refs []domain.ItemRef) []domain.EventItem {
	out := make([]domain.EventItem, len(refs))
	for i, r := range refs {
		out[i] = domain.EventItem{Ref: r}
		switch r.Kind {
		case domain.KindTV:
			out[i] = a.tvItem(ctx, r)
			continue
		case domain.KindMusic:
			out[i] = a.musicItem(ctx, r)
			continue
		}
		if m, err := a.Movies.Get(ctx, r.ID); err == nil && r.Kind == domain.KindMovie {
			out[i].Title = m.Title
		}
	}
	return out
}
