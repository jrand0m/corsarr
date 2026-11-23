package movies

import (
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/parse"
)

func day(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

func TestAvailability(t *testing.T) {
	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		m    domain.Movie
		want bool
		at   *time.Time
	}{
		{"announced is always available", domain.Movie{MinAvailability: domain.AvailAnnounced}, true, &time.Time{}},
		{"in cinemas uses the cinema date", domain.Movie{MinAvailability: domain.AvailInCinemas, InCinemas: day(2024, 5, 1)}, true, day(2024, 5, 1)},
		{"in cinemas without date falls back to released", domain.Movie{MinAvailability: domain.AvailInCinemas, Digital: day(2024, 7, 1)}, false, day(2024, 7, 1)},
		{"released takes the earliest digital or physical", domain.Movie{MinAvailability: domain.AvailReleased, Digital: day(2024, 7, 1), Physical: day(2024, 5, 20)}, true, day(2024, 5, 20)},
		{"released with only physical", domain.Movie{MinAvailability: domain.AvailReleased, Physical: day(2024, 8, 1)}, false, day(2024, 8, 1)},
		{"released with only digital", domain.Movie{MinAvailability: domain.AvailReleased, Digital: day(2024, 1, 1)}, true, day(2024, 1, 1)},
		{"released falls back to cinema + 90 days", domain.Movie{MinAvailability: domain.AvailReleased, InCinemas: day(2024, 4, 1)}, false, day(2024, 6, 30)},
		{"no dates: the year after release", domain.Movie{MinAvailability: domain.AvailReleased, Year: 1999}, true, day(2000, 1, 1)},
		{"no dates, this year: waits for the year to end", domain.Movie{MinAvailability: domain.AvailReleased, Year: 2024}, false, day(2025, 1, 1)},
		{"in cinemas, no dates: the same year rule", domain.Movie{MinAvailability: domain.AvailInCinemas, Year: 1999}, true, day(2000, 1, 1)},
		{"a cinema date wins over the year rule", domain.Movie{MinAvailability: domain.AvailReleased, Year: 1999, InCinemas: day(2024, 4, 1)}, false, day(2024, 6, 30)},
		{"nothing known", domain.Movie{MinAvailability: domain.AvailReleased}, false, nil},
	}
	for _, c := range cases {
		at, ok := AvailableAt(c.m)
		if got := IsAvailable(c.m, now); got != c.want || ok != (c.at != nil) || (c.at != nil && !at.Equal(*c.at)) {
			t.Errorf("%s: available=%v at=%v ok=%v", c.name, got, at, ok)
		}
	}
}

var example = domain.Movie{ID: 3, TMDBID: 603, IMDBID: "tt0133093", Title: "The Matrix", OriginalTitle: "Matrix", Year: 1999,
	AltTitles: []string{"Matrix", "Ma & Trix"}, Monitored: true, MinAvailability: domain.AvailReleased, RuntimeMin: 136}

func TestSearchRequestAndIndexKeys(t *testing.T) {
	req := SearchRequest(example, domain.OriginAutomatic)
	if req.Mode != domain.SearchMovie || req.IDs["tmdb"] != "603" || req.IDs["imdb"] != "tt0133093" || req.Year != 1999 ||
		len(req.Titles) != 4 || req.ItemRefs[0] != example.Ref() || req.Origin != domain.OriginAutomatic {
		t.Fatalf("%+v", req)
	}
	if r := SearchRequest(domain.Movie{TMDBID: 1, Title: "X"}, domain.OriginRSS); r.IDs["imdb"] != "" || len(r.Titles) != 1 {
		t.Fatalf("%+v", r)
	}
	keys := IndexKeys(example)
	if len(keys) != 3 || keys[0] != "thematrix" || keys[1] != "matrix" || keys[2] != "maandtrix" {
		t.Fatalf("keys %v", keys)
	}
}

func TestMatchesAndReject(t *testing.T) {
	for title, want := range map[string]bool{
		"The.Matrix.1999.1080p.WEB-DL.x264-GRP": true,
		"The.Matrix.2000.1080p.BluRay.x264-GRP": true,
		"Matrix.1999.720p.HDTV-X":               true,
		"The.Matrix.Reloaded.2003.1080p-GRP":    false,
		"The.Matrix.1080p.WEB-DL-GRP":           false,
		"The.Matrix.2005.1080p.WEB-DL-GRP":      false,
	} {
		p := parse.ParseRelease(title)
		if Matches(example, p) != want || (Reject(example, p) == nil) != want {
			t.Errorf("%s: want %v", title, want)
		}
	}
}

func TestDecideAddsMismatchReason(t *testing.T) {
	prof, _ := ProfileByKey("any")
	item := DecisionItem(example, prof, nil, time.Now())
	if !item.Available || item.RuntimeMinutes != 136 || !item.Monitored {
		t.Fatalf("%+v", item)
	}
	good := domain.Release{ID: "1", Title: "The.Matrix.1999.1080p.WEB-DL.x264-GRP", Protocol: domain.ProtoTorrent, Size: 6_000_000_000, Seeders: 9}
	if d := Decide(example, item, good, decide.Context{Now: time.Now()}, nil); !d.Accepted {
		t.Fatalf("good release rejected: %+v", d.Reasons)
	}
	bad := good
	bad.Title = "Other.Film.1999.1080p.WEB-DL.x264-GRP"
	d := Decide(example, item, bad, decide.Context{Now: time.Now()}, nil)
	if d.Accepted || d.Override != domain.OverrideConfirm || d.Reasons[len(d.Reasons)-1].RuleID != "movie.mismatch" {
		t.Fatalf("%+v", d)
	}
}

func TestProfiles(t *testing.T) {
	if ProfileKey("HD-1080p") != "hd1080p" || ProfileKey("Ultra-HD") != "ultrahd" {
		t.Fatal("keys")
	}
	if len(Profiles()) != 3 {
		t.Fatal("three shipped profiles")
	}
	if _, ok := ProfileByKey(DefaultProfileKey); !ok {
		t.Fatal("default exists")
	}
	if _, ok := ProfileByKey("nope"); ok {
		t.Fatal("unknown")
	}
	if FolderName(example) != "The Matrix (1999)" {
		t.Fatal(FolderName(example))
	}
}
