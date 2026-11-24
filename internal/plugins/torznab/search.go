package torznab

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
)

// idParam maps a corsarr external id name to the Torznab parameter.
type idParam struct{ ext, param string }

var modeT = map[domain.SearchMode]string{
	domain.SearchMovie: "movie", domain.SearchTV: "tvsearch", domain.SearchMusic: "music", domain.SearchBook: "book",
}

var modeIDs = map[domain.SearchMode][]idParam{
	domain.SearchMovie: {{"imdb", "imdbid"}, {"tmdb", "tmdbid"}},
	domain.SearchTV:    {{"tvdb", "tvdbid"}, {"imdb", "imdbid"}, {"tmdb", "tmdbid"}, {"tvmaze", "tvmazeid"}},
	domain.SearchMusic: {{"artist", "artist"}, {"album", "album"}},
}

// Search prefers an id search the indexer advertises, then falls back to a
// plain text search when the id search is impossible or finds nothing.
func (ix *Indexer) Search(ctx context.Context, req domain.SearchRequest) ([]domain.Release, error) {
	caps, err := ix.capsOrLoad(ctx)
	if err != nil {
		return nil, err
	}
	var rels []domain.Release
	if q, ok := ix.idQuery(req, caps); ok {
		if rels, err = ix.run(ctx, q); err != nil || len(rels) > 0 {
			return rels, err
		}
	}
	if q, ok := ix.textQuery(req, caps); ok {
		return ix.run(ctx, q)
	}
	return rels, nil
}

func (ix *Indexer) base(kind domain.MediaKind, t string) url.Values {
	v := url.Values{"t": {t}, "extended": {"1"}}
	if c := ix.cats[kind]; len(c) > 0 {
		v.Set("cat", joinInts(c))
	}
	return v
}

func (ix *Indexer) idQuery(req domain.SearchRequest, caps parsedCaps) (url.Values, bool) {
	t, ok := modeT[req.Mode]
	if !ok || !caps.hasMode(req.Mode) {
		return nil, false
	}
	v := ix.base(req.Kind, t)
	found := false
	for _, p := range modeIDs[req.Mode] {
		if id := req.IDs[p.ext]; id != "" && caps.supports(req.Mode, p.param) {
			v.Set(p.param, normalizeID(p.ext, id))
			found = true
		}
	}
	if found {
		narrow(v, req, caps)
	}
	return v, found
}

// normalizeID strips the "tt" prefix Newznab-style imdbid parameters omit.
func normalizeID(ext, id string) string {
	if ext == "imdb" {
		return strings.TrimPrefix(id, "tt")
	}
	return id
}

// narrow adds the mode's extra parameters to an id search.
func narrow(v url.Values, req domain.SearchRequest, caps parsedCaps) {
	switch {
	case req.Mode == domain.SearchTV:
		addEpisode(v, req)
	case req.Mode == domain.SearchMusic && req.Year > 0 && caps.supports(req.Mode, "year"):
		v.Set("year", strconv.Itoa(req.Year))
	}
}

func addEpisode(v url.Values, req domain.SearchRequest) {
	if req.Season > 0 {
		v.Set("season", strconv.Itoa(req.Season))
	}
	if len(req.Episodes) == 1 {
		v.Set("ep", strconv.Itoa(req.Episodes[0]))
	}
}

func (ix *Indexer) textQuery(req domain.SearchRequest, caps parsedCaps) (url.Values, bool) {
	text := req.Text
	if text == "" && len(req.Titles) > 0 {
		text = req.Titles[0]
	}
	if text == "" || !caps.hasMode(domain.SearchText) {
		return nil, false
	}
	year := strconv.Itoa(req.Year)
	if req.Kind == domain.KindMovie && req.Year > 0 && !strings.Contains(text, year) {
		text += " " + year
	}
	v := ix.base(req.Kind, "search")
	v.Set("q", text)
	return v, true
}

func (ix *Indexer) run(ctx context.Context, q url.Values) ([]domain.Release, error) {
	items, err := ix.items(ctx, q)
	if err != nil {
		return nil, err
	}
	rels := ix.releases(items)
	return rels, nil
}
