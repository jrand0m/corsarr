// Package sourcekit is the shared shape of direct music source plugins
// (sites that sell or stream albums rather than index torrents). A source
// plugin implements Site — search the site's catalogue and list the files
// of one album — and sourcekit turns it into both an indexer (its albums
// rank next to torrent results in every music search) and a downloader
// (a grab fetches the album's files into the source's download folder,
// where the importer picks them up like any completed download).
package sourcekit

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// Query is one album search on a source.
type Query struct {
	Artist string
	Album  string
	Text   string // free text when artist and album are unknown
}

// Item is one album a source offers.
type Item struct {
	ID       string   `json:"id"`
	Artist   string   `json:"artist"`
	Album    string   `json:"album"`
	Year     int      `json:"year,omitempty"`
	Format   string   `json:"format,omitempty"`  // flac | mp3 | aac | ogg | opus | wav
	Bitrate  int      `json:"bitrate,omitempty"` // kbps for lossy formats
	BitDepth int      `json:"bitDepth,omitempty"`
	Tracks   []string `json:"tracks,omitempty"` // the source's own track list
	Size     int64    `json:"size,omitempty"`
	URL      string   `json:"url,omitempty"` // the album's page on the site
}

// FileRef is one file of an album, fetched with the source's credentials.
type FileRef struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size,omitempty"`
}

// Site is what a source plugin implements.
type Site interface {
	plugin.Tester
	Search(ctx context.Context, q Query) ([]Item, error)
	Files(ctx context.Context, itemID string) ([]FileRef, error)
	// Fetch opens one file for download (the site adds its credentials).
	Fetch(ctx context.Context, f FileRef) (Body, error)
}

// Body is an open file download.
type Body interface {
	Read(p []byte) (int, error)
	Close() error
}

// Source is a Site seen as an indexer and a downloader at once.
type Source struct {
	key  domain.ObjectKey
	name string
	site Site
	dir  string // download folder

	mu     sync.Mutex
	jobs   map[string]*job
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New wraps a site; dir is where grabbed albums are written.
func New(inst plugin.Instance, site Site, dir string) (*Source, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, plugin.Errorf(plugin.ClassConfig, "%s needs a downloadDir", inst.PluginID)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Source{key: inst.Key, name: inst.Name, site: site, dir: dir, jobs: map[string]*job{}, ctx: ctx, cancel: cancel}, nil
}

// Test checks the site.
func (s *Source) Test(ctx context.Context) error { return s.site.Test(ctx) }

// Close stops running downloads.
func (s *Source) Close() error {
	s.cancel()
	s.wg.Wait()
	return nil
}

// Caps: a source answers music searches only.
func (s *Source) Caps(context.Context) (plugin.Caps, error) {
	return plugin.Caps{Modes: []domain.SearchMode{domain.SearchMusic, domain.SearchText}, Categories: map[int]string{3000: "Audio"}}, nil
}

// Search asks the site for albums; other kinds get nothing.
func (s *Source) Search(ctx context.Context, req domain.SearchRequest) ([]domain.Release, error) {
	if req.Kind != domain.KindMusic {
		return nil, nil
	}
	items, err := s.site.Search(ctx, Query{Artist: req.IDs["artist"], Album: req.IDs["album"], Text: req.Text})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Release, len(items))
	for i, it := range items {
		out[i] = s.release(it)
	}
	return out, nil
}

// release renders an item as a release whose title the music parser reads:
// "Artist - Album (Year) [FLAC 24bit]".
func (s *Source) release(it Item) domain.Release {
	return domain.Release{ID: string(s.key) + ":" + it.ID, Title: Title(it), Protocol: domain.ProtoSource, Size: it.Size,
		Indexers: []domain.IndexerRef{{Key: s.key, Name: s.name}}, GUID: it.ID, DetailsURL: it.URL, FileList: it.Tracks,
		Categories: []int{3000}, ExternalIDs: map[string]string{"source": it.ID}}
}

// Title names an item the way music releases are named.
func Title(it Item) string {
	t := it.Artist + " - " + it.Album
	if it.Year > 0 {
		t += fmt.Sprintf(" (%d)", it.Year)
	}
	var tags []string
	if it.Format != "" {
		tags = append(tags, strings.ToUpper(it.Format))
	}
	if it.BitDepth == 24 {
		tags = append(tags, "24bit")
	}
	if it.Bitrate > 0 {
		tags = append(tags, fmt.Sprintf("%dkbps", it.Bitrate))
	}
	if len(tags) > 0 {
		t += " [" + strings.Join(tags, " ") + "]"
	}
	return t
}

// Feed: sources have no RSS; new albums arrive through searches.
func (s *Source) Feed(_ context.Context, _ domain.MediaKind, cursor string) ([]domain.Release, string, error) {
	return nil, cursor, nil
}

// Resolve hands the grab the item id; the downloader side fetches it.
func (s *Source) Resolve(_ context.Context, rel domain.Release) (plugin.Payload, error) {
	if rel.GUID == "" {
		return plugin.Payload{}, plugin.Errorf(plugin.ClassConfig, "release %q has no source item", rel.Title)
	}
	return plugin.Payload{Kind: "source", URL: rel.GUID, Name: rel.Title}, nil
}

// Protocols: a source downloads only its own releases.
func (s *Source) Protocols() []domain.Protocol { return []domain.Protocol{domain.ProtoSource} }

// Owns reports whether a release came from this source; the queue sends a
// source release to the source that found it.
func (s *Source) Owns(rel domain.Release) bool {
	return len(rel.Indexers) > 0 && rel.Indexers[0].Key == s.key
}

// CategoryFor is the source's own category, named after its key.
func (s *Source) CategoryFor(domain.MediaKind) string { return string(s.key) }

// EnsureCategory: categories are only labels on a source.
func (s *Source) EnsureCategory(context.Context, string, string) error { return nil }

// Pause, Resume and SetPriority do nothing: an album fetch is short.
func (s *Source) Pause(context.Context, string) error                        { return nil }
func (s *Source) Resume(context.Context, string) error                       { return nil }
func (s *Source) SetPriority(context.Context, string, plugin.Priority) error { return nil }
