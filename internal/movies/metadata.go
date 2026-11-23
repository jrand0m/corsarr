package movies

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
)

// Metadata cache: provider records are stored per movie and served from the
// store while fresh; a refresh job renews them, and lookups fall back to the
// stored records while the provider is down.
const (
	RefreshJob      = "movies.refresh"
	RefreshInterval = 24 * time.Hour
	// RefreshEvery is how long a record stays fresh; each movie is staggered
	// by up to a day so refreshes spread over the week.
	RefreshEvery = 7 * 24 * time.Hour
	// RefreshBatch caps one run (the shared client still rate-limits each call).
	RefreshBatch = 200
	cacheSource  = "tmdb"
	searchLimit  = 20
)

// MetaCache is the metadata cache persistence (store.Metadata).
type MetaCache interface {
	Put(ctx context.Context, e store.MetadataEntry) error
	Get(ctx context.Context, source string, kind domain.MediaKind, id string) (store.MetadataEntry, bool, error)
	Search(ctx context.Context, kind domain.MediaKind, term string, limit int) ([]store.MetadataEntry, error)
}

// NextRefresh staggers a record's next refresh by its id (pure).
func NextRefresh(fetched time.Time, tmdbID int64) time.Time {
	return fetched.Add(RefreshEvery + time.Duration(tmdbID%24)*time.Hour)
}

// Merge copies provider metadata onto a library movie and names the fields
// that changed (pure). Monitoring, profile, folder and file stay as they are.
func Merge(m domain.Movie, rec plugin.MovieRecord) (domain.Movie, []string) {
	next := m
	applyRecord(&next, rec)
	var changed []string
	add := func(name string, differs bool) {
		if differs {
			changed = append(changed, name)
		}
	}
	add("title", next.Title != m.Title)
	add("originalTitle", next.OriginalTitle != m.OriginalTitle)
	add("year", next.Year != m.Year)
	add("altTitles", !slices.Equal(next.AltTitles, m.AltTitles))
	add("runtime", next.RuntimeMin != m.RuntimeMin)
	add("imdbId", next.IMDBID != m.IMDBID)
	add("releaseDates", !sameTime(next.InCinemas, m.InCinemas) || !sameTime(next.Digital, m.Digital) || !sameTime(next.Physical, m.Physical))
	return next, changed
}

func sameTime(a, b *time.Time) bool {
	return (a == nil) == (b == nil) && (a == nil || a.Equal(*b))
}

// movie returns the record for a TMDB id: from the cache while fresh, else
// from the provider (and cached); a provider failure falls back to the cache.
func (s *Service) movie(ctx context.Context, ids map[string]string) (plugin.MovieRecord, error) {
	id := ids["tmdb"]
	cached, ok := s.cached(ctx, id)
	if ok && s.d.Now().Before(cached.next) {
		return cached.rec, nil
	}
	rec, err := s.fetch(ctx, ids)
	if err != nil && ok && !errors.Is(err, plugin.ErrNotFound) {
		s.d.Log.Info("metadata provider failed; serving the cached record", "tmdb", id, "err", err)
		return cached.rec, nil
	}
	return rec, err
}

type cachedRecord struct {
	rec  plugin.MovieRecord
	next time.Time
}

func (s *Service) cached(ctx context.Context, id string) (cachedRecord, bool) {
	if id == "" || s.d.Cache == nil {
		return cachedRecord{}, false
	}
	e, ok, err := s.d.Cache.Get(ctx, cacheSource, domain.KindMovie, id)
	var rec plugin.MovieRecord
	if err != nil || !ok || json.Unmarshal([]byte(e.Record), &rec) != nil {
		return cachedRecord{}, false
	}
	return cachedRecord{rec: rec, next: e.NextRefresh}, true
}

// fetch asks the provider and stores the record.
func (s *Service) fetch(ctx context.Context, ids map[string]string) (plugin.MovieRecord, error) {
	md, ok := s.d.Metadata()
	if !ok {
		return plugin.MovieRecord{}, ErrNoMetadata
	}
	rec, err := md.Movie(ctx, ids)
	if err == nil {
		s.store(ctx, rec)
	}
	return rec, err
}

func (s *Service) store(ctx context.Context, rec plugin.MovieRecord) {
	id, err := strconv.ParseInt(rec.IDs["tmdb"], 10, 64)
	if s.d.Cache == nil || err != nil {
		return
	}
	body, _ := json.Marshal(rec) // plain data
	now := s.d.Now().UTC()
	s.logErr(s.d.Cache.Put(ctx, store.MetadataEntry{Source: cacheSource, Kind: domain.KindMovie, ExternalID: rec.IDs["tmdb"],
		Title: rec.Title, Record: string(body), FetchedAt: now, NextRefresh: NextRefresh(now, id)}))
}

// search asks the provider; when it fails or none is usable, the cached
// records whose title matches answer instead.
func (s *Service) search(ctx context.Context, term string) ([]plugin.MovieRecord, error) {
	md, ok := s.d.Metadata()
	var err error = ErrNoMetadata
	if ok {
		var recs []plugin.MovieRecord
		if recs, err = md.SearchMovies(ctx, term, 0); err == nil {
			return recs, nil
		}
	}
	if s.d.Cache == nil {
		return nil, err
	}
	entries, cerr := s.d.Cache.Search(ctx, domain.KindMovie, term, searchLimit)
	if cerr != nil || len(entries) == 0 {
		return nil, err
	}
	out := make([]plugin.MovieRecord, 0, len(entries))
	for _, e := range entries {
		var rec plugin.MovieRecord
		if json.Unmarshal([]byte(e.Record), &rec) == nil {
			out = append(out, rec)
		}
	}
	return out, nil
}

// Refresh renews the metadata of library movies whose record is missing or
// due, up to RefreshBatch per run, writes changed fields to the movie and
// publishes item.updated. A provider failure ends the run (the scheduler
// backs off); a refresh never deletes a movie or its file.
func (s *Service) Refresh(ctx context.Context) error {
	if _, ok := s.d.Metadata(); !ok || s.d.Modes.Of(domain.KindMovie) == domain.ModeDisabled {
		return nil
	}
	due, err := s.dueForRefresh(ctx)
	if err != nil {
		return err
	}
	for _, m := range due {
		rec, err := s.fetch(ctx, map[string]string{"tmdb": strconv.FormatInt(m.TMDBID, 10)})
		if errors.Is(err, plugin.ErrNotFound) {
			continue // gone at the provider: the movie stays as it is
		}
		if err != nil {
			return err
		}
		if err := s.applyRefresh(ctx, m.ID, rec); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) dueForRefresh(ctx context.Context) ([]domain.Movie, error) {
	var due []domain.Movie
	now := s.d.Now()
	err := s.d.Repo.Each(ctx, func(m domain.Movie) error {
		if c, ok := s.cached(ctx, strconv.FormatInt(m.TMDBID, 10)); (!ok || !now.Before(c.next)) && len(due) < RefreshBatch {
			due = append(due, m)
		}
		return nil
	})
	return due, err
}

// applyRefresh merges a fresh record into the stored movie (read again so a
// concurrent edit is not lost).
func (s *Service) applyRefresh(ctx context.Context, id int64, rec plugin.MovieRecord) error {
	m, err := s.Get(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	next, changed := Merge(m, rec)
	if len(changed) == 0 {
		return nil
	}
	if err := s.d.Repo.Update(ctx, next); err != nil {
		return err
	}
	if !IsAvailable(next, s.d.Now()) {
		s.logErr(s.d.Repo.SetAvailablePublished(ctx, next.ID, false))
	}
	_, _ = s.d.Pub.Publish(domain.Event{Type: domain.EvItemUpdated, Actor: "system", Kind: domain.KindMovie,
		Items: []domain.EventItem{next.EventItem()}, Data: domain.ItemEventData{Origin: "metadata", Monitored: next.Monitored, Changed: changed}})
	s.Kick() // release dates may have moved
	return nil
}
