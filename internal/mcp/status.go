package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/store"
)

// Request states reported by request_status.
const (
	StateImported    = "imported"
	StatePartial     = "partial"
	StateDownloading = "downloading"
	StateShadowed    = "shadowed"
	StateUnmonitored = "unmonitored"
	StateSearching   = "searching"
	StateNotFound    = "no_releases"
	StateRejected    = "all_rejected"
	StateWanted      = "wanted"
)

// DownloadView is a queued download linked to an item.
type DownloadView struct {
	ID         int64   `json:"id"`
	Title      string  `json:"title"`
	Downloader string  `json:"downloader"`
	State      string  `json:"state"`
	Progress   float64 `json:"progress"`
	ETASeconds int64   `json:"etaSeconds,omitempty"`
	Size       int64   `json:"size,omitempty"`
}

// Status is what happened to a requested item.
type Status struct {
	Kind       domain.MediaKind    `json:"kind"`
	ID         int64               `json:"id"`
	AlbumID    int64               `json:"albumId,omitempty"`
	Title      string              `json:"title"`
	Mode       domain.KindMode     `json:"mode"`
	Monitored  bool                `json:"monitored"`
	State      string              `json:"state"`
	Reason     string              `json:"reason,omitempty"`
	Have       int                 `json:"have"`   // movies 0/1, episodes, albums or tracks with files
	Wanted     int                 `json:"wanted"` // of how many monitored
	Files      []string            `json:"files,omitempty"`
	Downloads  []DownloadView      `json:"downloads"`
	LastSearch *domain.SearchState `json:"lastSearch,omitempty"`
	Created    bool                `json:"created,omitempty"` // a request added it just now
}

// judge sets State and Reason from the facts gathered.
func judge(st *Status) {
	switch {
	case st.Wanted > 0 && st.Have >= st.Wanted:
		st.State, st.Reason = StateImported, "every monitored part has a file"
	case len(st.Downloads) > 0:
		d := st.Downloads[0]
		st.State, st.Reason = StateDownloading, fmt.Sprintf("grabbed %q on %s: %s, %.0f%%", d.Title, d.Downloader, d.State, d.Progress*100)
	case st.Mode == domain.ModeShadow:
		st.State, st.Reason = StateShadowed, "the kind is in shadow mode: decisions are recorded in History (decision.shadowed) and nothing is grabbed"
	case !st.Monitored:
		st.State, st.Reason = StateUnmonitored, "not monitored, so automation does not search for it"
	case st.Have > 0:
		st.State, st.Reason = StatePartial, fmt.Sprintf("%d of %d have files", st.Have, st.Wanted)+lastSearchNote(st.LastSearch)
	default:
		st.State, st.Reason = searchVerdict(st.LastSearch)
	}
}

func searchVerdict(ls *domain.SearchState) (string, string) {
	switch {
	case ls == nil || ls.LastResult == nil:
		return StateSearching, "no search has finished yet"
	case ls.LastResult.Releases == 0:
		return StateNotFound, fmt.Sprintf("the last search found no releases (%d indexers, %d failed)", ls.LastResult.Indexers, ls.LastResult.Failed)
	case ls.LastResult.Accepted == 0:
		return StateRejected, fmt.Sprintf("all %d releases were rejected%s; search_releases shows each one's reasons",
			ls.LastResult.Releases, reasonsText(ls.LastResult.Rejections))
	}
	return StateWanted, fmt.Sprintf("%d acceptable releases found; waiting for the grab or a better one", ls.LastResult.Accepted)
}

// lastSearchNote adds what the last finished search found.
func lastSearchNote(ls *domain.SearchState) string {
	if ls == nil || ls.LastResult == nil {
		return ""
	}
	_, why := searchVerdict(ls)
	return "; last search: " + why
}

// reasonsText lists the top rejection reasons of a search.
func reasonsText(list []domain.ReasonCount) string {
	if len(list) == 0 {
		return ""
	}
	parts := make([]string, len(list))
	for i, r := range list {
		parts[i] = fmt.Sprintf("%s ×%d", r.Message, r.Count)
	}
	return " (top reasons: " + strings.Join(parts, "; ") + ")"
}

// status gathers and judges an item's state.
func (s *Service) status(ctx context.Context, kind domain.MediaKind, t target) (Status, error) {
	gather := map[domain.MediaKind]func(context.Context, target) (Status, []domain.ItemRef, error){
		domain.KindMovie: s.movieStatus, domain.KindTV: s.seriesStatus, domain.KindMusic: s.musicStatus,
	}[kind]
	st, refs, err := gather(ctx, t)
	if err != nil {
		return st, err
	}
	st.Kind, st.Mode = kind, s.d.Modes.Of(kind)
	if st.Downloads, err = s.downloadsOf(ctx, refs); err != nil {
		return st, err
	}
	judge(&st)
	return st, nil
}

func (s *Service) movieStatus(ctx context.Context, t target) (Status, []domain.ItemRef, error) {
	m, err := s.d.Movies.Get(ctx, t.ID)
	if err != nil {
		return Status{}, nil, err
	}
	st := Status{ID: m.ID, Title: m.Title, Monitored: m.Monitored, Wanted: 1}
	if m.FileID != 0 {
		st.Have = 1
	}
	if st.Files, err = s.filePaths(ctx, m.Ref()); err != nil {
		return st, nil, err
	}
	st.LastSearch, err = s.lastSearch(ctx, m.Ref())
	return st, []domain.ItemRef{m.Ref()}, err
}

func (s *Service) seriesStatus(ctx context.Context, t target) (Status, []domain.ItemRef, error) {
	d, err := s.d.TV.Detail(ctx, t.ID)
	if err != nil {
		return Status{}, nil, err
	}
	ser := d.Series
	st := Status{ID: ser.ID, Title: ser.Title, Monitored: ser.Monitored, Have: ser.Stats.EpisodeFileCount, Wanted: ser.Stats.EpisodeCount}
	refs := []domain.ItemRef{ser.Ref()}
	for _, e := range d.Episodes {
		refs = append(refs, e.Ref())
	}
	st.LastSearch, err = s.latestSearch(ctx, domain.KindTV, refs[1:])
	return st, refs, err
}

// latestSearch is the most recent search of any of the parts (episodes of
// a series, albums of an artist), which says why the last one came back
// empty.
func (s *Service) latestSearch(ctx context.Context, kind domain.MediaKind, parts []domain.ItemRef) (*domain.SearchState, error) {
	all, err := s.d.Searches.States(ctx, kind)
	if err != nil {
		return nil, err
	}
	var latest *domain.SearchState
	for _, p := range parts {
		st, ok := all[p.ID]
		if ok && st.LastSearched != nil && (latest == nil || st.LastSearched.After(*latest.LastSearched)) {
			latest = &st
		}
	}
	return latest, nil
}

func (s *Service) musicStatus(ctx context.Context, t target) (Status, []domain.ItemRef, error) {
	if t.AlbumID > 0 {
		return s.albumStatus(ctx, t.AlbumID)
	}
	d, err := s.d.Music.Detail(ctx, t.ID)
	if err != nil {
		return Status{}, nil, err
	}
	a := d.Artist
	st := Status{ID: a.ID, Title: a.Name, Monitored: a.Monitored, Have: a.Stats.AlbumsWithFiles, Wanted: a.Stats.MonitoredAlbums}
	refs := []domain.ItemRef{a.Ref()}
	for _, al := range d.Albums {
		refs = append(refs, al.Ref())
	}
	st.LastSearch, err = s.latestSearch(ctx, domain.KindMusic, refs[1:])
	return st, refs, err
}

func (s *Service) albumStatus(ctx context.Context, id int64) (Status, []domain.ItemRef, error) {
	p, err := s.d.Music.AlbumPage(ctx, id)
	if err != nil {
		return Status{}, nil, err
	}
	al := p.Album
	st := Status{ID: p.Artist.ID, AlbumID: al.ID, Title: p.Artist.Name + " - " + al.Title, Monitored: al.Monitored,
		Have: al.FileCount, Wanted: al.TrackCount}
	st.LastSearch, err = s.lastSearch(ctx, al.Ref())
	return st, []domain.ItemRef{al.Ref()}, err
}

func (s *Service) filePaths(ctx context.Context, ref domain.ItemRef) ([]string, error) {
	files, err := s.d.Library.FilesOf(ctx, ref)
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out, err
}

func (s *Service) lastSearch(ctx context.Context, ref domain.ItemRef) (*domain.SearchState, error) {
	st, err := s.d.Searches.State(ctx, ref)
	if err != nil || st.LastSearched == nil {
		return nil, err
	}
	return &st, nil
}

// maxQueueScan bounds how many queued downloads a status looks through.
const maxQueueScan = 1000

// downloadsOf lists the queued downloads linked to any of refs.
func (s *Service) downloadsOf(ctx context.Context, refs []domain.ItemRef) ([]DownloadView, error) {
	want := map[domain.ItemRef]bool{}
	for _, r := range refs {
		want[r] = true
	}
	out := []DownloadView{}
	err := s.eachDownload(ctx, func(d store.StoredDownload) {
		if linked(d, want) {
			out = append(out, viewOf(d))
		}
	})
	return out, err
}

func (s *Service) eachDownload(ctx context.Context, fn func(store.StoredDownload)) error {
	var cursor int64
	for seen := 0; seen < maxQueueScan; {
		list, next, err := s.d.Queue.List(ctx, cursor, MaxLimit)
		if err != nil {
			return err
		}
		for _, d := range list {
			fn(d)
		}
		seen += len(list)
		if next == 0 {
			return nil
		}
		cursor = next
	}
	return nil
}

func linked(d store.StoredDownload, want map[domain.ItemRef]bool) bool {
	for _, l := range d.Links {
		if want[l.Item] {
			return true
		}
	}
	return false
}

func viewOf(d store.StoredDownload) DownloadView {
	return DownloadView{ID: d.ID, Title: d.Title, Downloader: string(d.Downloader), State: string(d.State),
		Progress: d.Progress, ETASeconds: d.ETASeconds, Size: d.Size}
}

func statusText(st Status) string {
	return fmt.Sprintf("%s (%s %d): %s — %s", st.Title, st.Kind, st.ID, st.State, st.Reason)
}
