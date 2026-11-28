package store

import (
	"bytes"
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

func TestImageIndexRoundTrip(t *testing.T) {
	ctx := context.Background()
	r := open(t, t.TempDir()).Images()
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	orig := ImageEntry{Key: "k1", Origin: "https://img.example/a.jpg", Role: "poster", Format: "jpeg", Bytes: 1000, Width: 600,
		Height: 900, Quality: 82, LastAccess: at, CreatedAt: at}
	must(t, r.Put(ctx, orig))
	must(t, r.Put(ctx, ImageEntry{Key: "k1", Variant: 160, Origin: orig.Origin, Role: "poster", Format: "jpeg", Bytes: 100,
		Width: 160, Height: 240, Quality: 82, LastAccess: at.Add(time.Hour), CreatedAt: at}))
	must(t, r.Put(ctx, ImageEntry{Key: "k2", Origin: "https://img.example/b.png", Role: "backdrop", Format: "png", Bytes: 500,
		LastAccess: at.Add(-time.Hour), CreatedAt: at}))
	must(t, r.Put(ctx, ImageEntry{Key: "gone", Origin: "https://img.example/x", Role: "poster", LastAccess: at,
		MissUntil: at.Add(time.Hour), CreatedAt: at}))
	got, ok, err := r.Get(ctx, "k1", 0)
	if err != nil || !ok || got != orig {
		t.Fatalf("%+v %v %v", got, ok, err)
	}
	if _, ok, _ := r.Get(ctx, "k1", 96); ok {
		t.Fatal("unknown variant found")
	}
	u, err := r.Usage(ctx)
	if err != nil || u != (ImageUsage{Bytes: 1600, Originals: 2, Variants: 1, Misses: 1}) {
		t.Fatalf("%+v %v", u, err)
	}
	stored, _ := r.Stored(ctx)
	if len(stored) != 3 || stored[0].Key != "k2" || stored[2].Variant != 160 {
		t.Fatalf("lru order %+v", stored)
	}
	must(t, r.Touch(ctx, "k2", 0, at.Add(2*time.Hour)))
	if stored, _ = r.Stored(ctx); stored[2].Key != "k2" {
		t.Fatalf("touch %+v", stored)
	}
	vs, _ := r.Variants(ctx, "k1")
	if len(vs) != 1 || vs[0].Variant != 160 {
		t.Fatalf("%+v", vs)
	}
	must(t, r.DeleteMisses(ctx, at))
	if u, _ := r.Usage(ctx); u.Misses != 1 {
		t.Fatal("unexpired miss dropped")
	}
	must(t, r.DeleteMisses(ctx, at.Add(time.Hour)))
	must(t, r.Delete(ctx, "k1", 160))
	must(t, r.Delete(ctx, "k1", 160))
	if u, _ := r.Usage(ctx); u != (ImageUsage{Bytes: 1500, Originals: 2}) {
		t.Fatalf("%+v", u)
	}
	must(t, r.DeleteAll(ctx))
	if u, _ := r.Usage(ctx); u != (ImageUsage{}) {
		t.Fatalf("%+v", u)
	}
}

func TestArtworkOriginsAndCoverAlbums(t *testing.T) {
	ctx := context.Background()
	st := open(t, t.TempDir())
	m, _, err := st.Movies().Add(ctx, domain.Movie{TMDBID: 1, Title: "M", MinAvailability: domain.AvailReleased, ProfileKey: "any",
		RootFolder: "/m", Path: "/m/x", PosterURL: "https://img.example/m.jpg", FanartURL: "https://img.example/old.jpg"})
	must(t, err)
	m.FanartURL = "https://img.example/mb.jpg"
	must(t, st.Movies().Update(ctx, m))
	if got, _, _ := st.Movies().Get(ctx, m.ID); got.PosterURL != m.PosterURL || got.FanartURL != m.FanartURL {
		t.Fatalf("%+v", got)
	}
	s, seasons, eps := tvFixture()
	s.PosterURL, s.FanartURL = "https://img.example/s.jpg", ""
	_, _, err = st.TV().AddSeries(ctx, s, seasons, eps)
	must(t, err)
	got, err := st.ArtworkOrigins(ctx)
	slices.Sort(got)
	if err != nil || !slices.Equal(got, []string{"https://img.example/m.jpg", "https://img.example/mb.jpg", "https://img.example/s.jpg"}) {
		t.Fatal(got, err)
	}
	old, newer := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)
	art, _, err := st.Music().AddArtist(ctx, domain.Artist{MBID: "a1", Name: "A", SortName: "A", RootFolder: "/m", Path: "/m/A",
		ProfileKey: "lossless", MetadataProfile: "standard", MonitorNew: domain.MonitorNewItemsNew}, []domain.Album{
		{MBID: "old", Title: "Old", PrimaryType: domain.AlbumTypeAlbum, ReleaseDate: &old},
		{MBID: "undated", Title: "U", PrimaryType: domain.AlbumTypeAlbum},
		{MBID: "single", Title: "S", PrimaryType: "single", ReleaseDate: &newer},
		{MBID: "new", Title: "New", PrimaryType: domain.AlbumTypeAlbum, ReleaseDate: &newer}})
	must(t, err)
	covers, err := st.Music().CoverAlbums(ctx)
	if err != nil || covers[art.ID] != "new" || len(covers) != 1 {
		t.Fatal(covers, err)
	}
	mbids, _ := st.Music().AlbumMBIDs(ctx)
	if len(mbids) != 4 {
		t.Fatal(mbids)
	}
}

func TestMetadataEvictsOnlyItemsOutsideTheLibrary(t *testing.T) {
	ctx := context.Background()
	st := open(t, t.TempDir())
	_, _, err := st.Movies().Add(ctx, domain.Movie{TMDBID: 1, Title: "Kept", MinAvailability: domain.AvailReleased, ProfileKey: "any", RootFolder: "/m", Path: "/m/x"})
	must(t, err)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, id := range []string{"1", "2", "3"} {
		must(t, st.Metadata().Put(ctx, MetadataEntry{Source: "tmdb", Kind: domain.KindMovie, ExternalID: id, Record: "0123456789",
			FetchedAt: at.Add(time.Duration(i) * time.Hour), NextRefresh: at}))
	}
	if n, err := st.Metadata().Bytes(ctx); err != nil || n != 30 {
		t.Fatal(n, err)
	}
	n, freed, err := st.Metadata().EvictUnused(ctx, 5)
	if err != nil || n != 1 || freed != 10 {
		t.Fatal(n, freed, err)
	}
	if _, ok, _ := st.Metadata().Get(ctx, "tmdb", domain.KindMovie, "2"); ok {
		t.Fatal("oldest unused record kept")
	}
	n, _, _ = st.Metadata().EvictUnused(ctx, 1000)
	if _, ok, _ := st.Metadata().Get(ctx, "tmdb", domain.KindMovie, "1"); n != 1 || !ok {
		t.Fatal("library record evicted", n)
	}
}

func TestSubKeysDifferByPurposeAndAreStable(t *testing.T) {
	dir := t.TempDir()
	st := open(t, dir)
	a, b := st.SubKey("links"), st.SubKey("other")
	if len(a) != 32 || bytes.Equal(a, b) || !bytes.Equal(a, st.SubKey("links")) {
		t.Fatal("sub keys")
	}
}

func TestMoviePosterBackfillFromCachedRecord(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	db, err := openDB(filepath.Join(dir, DBFile), false, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	all, err := loadMigrations(migrationFS)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(all, func(m migration) bool { return m.version == 260 })
	must(t, applyMigrations(ctx, db, all[:i], "", filepath.Join(dir, "b")))
	_, err = db.ExecContext(ctx, `INSERT INTO movies (tmdb_id, title, profile_key, root_folder, path, added_at) VALUES
		(603, 'With record', 'any', '/m', '/m/a', 0), (604, 'Without', 'any', '/m', '/m/b', 0)`)
	must(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO metadata_cache (source, kind, external_id, record, fetched_at, next_refresh)
		VALUES ('tmdb', 'movie', '603', '{"posterUrl":"https://img.example/603.jpg"}', 0, 0)`)
	must(t, err)
	must(t, applyMigrations(ctx, db, all, "", filepath.Join(dir, "b")))
	var a, b string
	must(t, db.QueryRowContext(ctx, "SELECT poster_url FROM movies WHERE tmdb_id = 603").Scan(&a))
	must(t, db.QueryRowContext(ctx, "SELECT poster_url FROM movies WHERE tmdb_id = 604").Scan(&b))
	if a != "https://img.example/603.jpg" || b != "" {
		t.Fatalf("%q %q", a, b)
	}
}
