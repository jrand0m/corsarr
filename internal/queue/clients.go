package queue

import (
	"context"
	"fmt"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
)

// ClientInfo is one configured downloader as the host knows it.
type ClientInfo struct {
	Key    domain.ObjectKey
	Name   string
	Type   string // plugin id
	Health *domain.IntegrationHealth
	Impl   plugin.Downloader
}

// ClientView is a downloader with its live state, as listed to the user.
type ClientView struct {
	Key          domain.ObjectKey          `json:"key"`
	Name         string                    `json:"name"`
	Type         string                    `json:"type"`
	Health       *domain.IntegrationHealth `json:"health,omitempty"`
	Protocols    []domain.Protocol         `json:"protocols"`
	Capabilities []string                  `json:"capabilities"`
	Stats        *plugin.ClientStatus      `json:"stats,omitempty"`
	StatsError   string                    `json:"statsError,omitempty"`
}

// capabilityChecks name the optional capabilities a client may have.
var capabilityChecks = []struct {
	name string
	has  func(plugin.Downloader) bool
}{
	{"files", is[plugin.FileLister]}, {"priority-levels", is[plugin.PriorityLeveler]}, {"trackers", is[plugin.TrackerLister]}, {"pause-all", is[plugin.GlobalPauser]},
	{"speed-limits", is[plugin.SpeedLimiter]}, {"recheck", is[plugin.Rechecker]}, {"reannounce", is[plugin.Reannouncer]},
	{"set-category", is[plugin.Categoriser]}, {"stats", is[plugin.ClientStats]},
}

// Capabilities lists the optional capabilities a downloader implements.
func Capabilities(dl plugin.Downloader) []string {
	out := []string{}
	for _, c := range capabilityChecks {
		if c.has(dl) {
			out = append(out, c.name)
		}
	}
	return out
}

func (s *Service) clients() []ClientInfo {
	if s.d.Clients != nil {
		return s.d.Clients()
	}
	var out []ClientInfo
	for _, d := range s.d.Plugins.Downloaders() {
		out = append(out, ClientInfo{Key: d.Key, Name: string(d.Key), Impl: d.Impl})
	}
	return out
}

func (s *Service) clientInfo(key domain.ObjectKey) (ClientInfo, error) {
	for _, c := range s.clients() {
		if c.Key == key {
			return c, nil
		}
	}
	return ClientInfo{}, fmt.Errorf("%w: %s", ErrUnknownClient, key)
}

// Clients lists every configured downloader with health, protocols,
// capabilities and, where the client reports them, live stats.
func (s *Service) Clients(ctx context.Context) []ClientView {
	out := []ClientView{}
	for _, c := range s.clients() {
		out = append(out, s.view(ctx, c))
	}
	return out
}

// Client returns one downloader's view.
func (s *Service) Client(ctx context.Context, key domain.ObjectKey) (ClientView, error) {
	c, err := s.clientInfo(key)
	if err != nil {
		return ClientView{}, err
	}
	return s.view(ctx, c), nil
}

func (s *Service) view(ctx context.Context, c ClientInfo) ClientView {
	v := ClientView{Key: c.Key, Name: c.Name, Type: c.Type, Health: c.Health, Protocols: c.Impl.Protocols(), Capabilities: Capabilities(c.Impl)}
	if st, ok := c.Impl.(plugin.ClientStats); ok {
		stats, err := st.Stats(ctx)
		if err != nil {
			v.StatsError = err.Error()
		} else {
			v.Stats = &stats
		}
	}
	return v
}

// clientWrite runs one client-wide write. A client serves every media kind,
// so while any kind is in shadow mode the write is recorded as
// downloader.shadowed instead of sent.
func (s *Service) clientWrite(key domain.ObjectKey, action string, args map[string]any, run func() error) error {
	for _, k := range mediaKinds {
		if s.d.Modes.Of(k) == domain.ModeShadow {
			_, _ = s.d.Pub.Publish(domain.Event{Type: domain.EvClientShadowed, Actor: "user", Kind: k,
				Data: domain.ClientActionData{Downloader: key, Action: action, Args: args}})
			return ErrShadowed
		}
	}
	if err := run(); err != nil {
		return err
	}
	s.Wake()
	return nil
}

var mediaKinds = []domain.MediaKind{domain.KindMovie, domain.KindTV, domain.KindMusic, domain.KindBook}

// capable returns the client as T, or the unsupported error when probing.
func capable[T any](key domain.ObjectKey, s *Service, what string) (T, error) {
	var zero T
	c, err := s.clientInfo(key)
	if err != nil {
		return zero, err
	}
	t, ok := c.Impl.(T)
	if !ok {
		return zero, unsupported(what)
	}
	return t, nil
}

// clientCap is clientWrite for an optional capability T.
func clientCap[T any](s *Service, key domain.ObjectKey, action string, args map[string]any, fn func(T) error) error {
	t, err := capable[T](key, s, action)
	if err != nil {
		return err
	}
	return s.clientWrite(key, action, args, func() error { return fn(t) })
}

// PauseAll stops every download of one client.
func (s *Service) PauseAll(ctx context.Context, key domain.ObjectKey) error {
	return clientCap(s, key, "pause-all", nil, func(p plugin.GlobalPauser) error { return p.PauseAll(ctx) })
}

// ResumeAll restarts every download of one client.
func (s *Service) ResumeAll(ctx context.Context, key domain.ObjectKey) error {
	return clientCap(s, key, "resume-all", nil, func(p plugin.GlobalPauser) error { return p.ResumeAll(ctx) })
}

// SetSpeedLimits sets a client's global limits in KiB/s (0 = unlimited).
func (s *Service) SetSpeedLimits(ctx context.Context, key domain.ObjectKey, l plugin.SpeedLimits) error {
	if l.DownKiB < 0 || l.UpKiB < 0 {
		return plugin.Errorf(plugin.ClassConfig, "speed limits must be 0 (unlimited) or more")
	}
	return clientCap(s, key, "speed-limits", map[string]any{"downKiB": l.DownKiB, "upKiB": l.UpKiB},
		func(p plugin.SpeedLimiter) error { return p.SetSpeedLimits(ctx, l) })
}

// SetAltSpeed switches a client's alternative speed mode; nil flips it.
// It returns the mode asked for.
func (s *Service) SetAltSpeed(ctx context.Context, key domain.ObjectKey, on *bool) (bool, error) {
	p, err := capable[plugin.SpeedLimiter](key, s, "alt-speed")
	if err != nil {
		return false, err
	}
	want := false
	if on != nil {
		want = *on
	} else {
		cur, err := p.AltSpeed(ctx)
		if err != nil {
			return false, err
		}
		want = !cur
	}
	return want, clientCap(s, key, "alt-speed", map[string]any{"on": want},
		func(p plugin.SpeedLimiter) error { return p.SetAltSpeed(ctx, want) })
}

// TestClient self-tests one downloader (a read: shadow mode does not apply).
func (s *Service) TestClient(ctx context.Context, key domain.ObjectKey) error {
	c, err := s.clientInfo(key)
	if err != nil {
		return err
	}
	if s.d.TestClient != nil {
		return s.d.TestClient(ctx, key)
	}
	return c.Impl.Test(ctx)
}

// Filter narrows the queue listing.
type Filter = store.QueueFilter

// ListFiltered pages the queue newest first under a filter.
func (s *Service) ListFiltered(ctx context.Context, f Filter, cursor int64, limit int) ([]store.StoredDownload, int64, error) {
	return s.d.Repo.ListFiltered(ctx, f, cursor, limit)
}

// Detail is one download with what its client reports about it and its
// state history.
type Detail struct {
	store.StoredDownload
	Files         []plugin.FileEntry `json:"files,omitempty"`
	FilesError    string             `json:"filesError,omitempty"`
	Trackers      []plugin.Tracker   `json:"trackers,omitempty"`
	TrackersError string             `json:"trackersError,omitempty"`
	Capabilities  []string           `json:"capabilities"`
	History       []store.Transition `json:"history"`
}

// Detail returns a download with its files and trackers (where the client
// lists them) and every recorded state change.
func (s *Service) Detail(ctx context.Context, id int64) (Detail, error) {
	d, err := s.load(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	out := Detail{StoredDownload: d, Capabilities: []string{}}
	if out.History, err = s.d.Repo.Transitions(ctx, id); err != nil {
		return Detail{}, err
	}
	dl, ok := s.downloader(d.Downloader)
	if !ok {
		return out, nil
	}
	out.Capabilities = Capabilities(dl)
	if fl, ok := dl.(plugin.FileLister); ok {
		out.Files, err = fl.Files(ctx, d.DownloadID)
		out.FilesError = errText(err)
	}
	if tl, ok := dl.(plugin.TrackerLister); ok {
		out.Trackers, err = tl.Trackers(ctx, d.DownloadID)
		out.TrackersError = errText(err)
	}
	return out, nil
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
