package tvauto

import (
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/tv"
)

func TestWantedEntriesLinkPostersThroughTheCache(t *testing.T) {
	s := &Service{d: Deps{Art: func(r domain.ArtRole, o string) string { return "cache/" + o }}}
	in := []Entry{{Series: tv.SeriesRef{ID: 1, PosterURL: "https://img.example/p.jpg"}}, {Series: tv.SeriesRef{ID: 2}}}
	out := s.withArt(in)
	if out[0].Series.PosterURL != "cache/https://img.example/p.jpg" || out[1].Series.PosterURL != "" || in[0].Series.PosterURL != "https://img.example/p.jpg" {
		t.Fatalf("%+v", out)
	}
}
