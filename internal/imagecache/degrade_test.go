package imagecache

import (
	"context"
	"fmt"
	"image"
	"math/rand/v2"
	"os"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/store"
)

type fakeMeta struct{ bytes, evictable int64 }

func (m *fakeMeta) Bytes(context.Context) (int64, error) { return m.bytes, nil }
func (m *fakeMeta) EvictUnused(_ context.Context, want int64) (int, int64, error) {
	freed := min(want, m.evictable)
	m.evictable -= freed
	m.bytes -= freed
	if freed == 0 {
		return 0, 0, nil
	}
	return 1, freed, nil
}

func stored(t testing.TB, e *env) []store.ImageEntry {
	t.Helper()
	list, err := e.st.Images().Stored(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func fill(t testing.TB, e *env, n int, prefix string) []string {
	t.Helper()
	var urls []string
	for i := 0; i < n; i++ {
		u := fmt.Sprintf("https://img.example/%s%d.jpg", prefix, i)
		if _, err := e.cache.Image(ctx, e.poster(u, noisy(200, 300, uint64(i+1))), 0); err != nil {
			t.Fatal(err)
		}
		e.clock.add(time.Second)
		urls = append(urls, u)
	}
	return urls
}

func smallSettings(d *Deps) {
	d.Settings.MaxWidth, d.Settings.Degrade.WidthFloor = 200, 100
}

func TestDegradeLowersQualityBeforeWidthBeforeEviction(t *testing.T) {
	var kicks int
	e := newEnv(t, func(d *Deps) { smallSettings(d); d.Kick = func() { kicks++ } })
	fill(t, e, 10, "p")
	full := stored(t, e)
	var total int64
	for _, x := range full {
		total += x.Bytes
	}
	e.cache.d.Settings.MaxSize = total * 10 / 9 // at high water; low water a hair below
	e.cache.d.Settings.Degrade.LowWater = 0.89
	if err := e.cache.Degrade(ctx); err != nil {
		t.Fatal(err)
	}
	after := stored(t, e)
	st, _ := e.cache.Status(ctx)
	if st.Usage > e.cache.d.Settings.low() || st.LastRun == nil || st.LastRun.Reencoded == 0 || st.LastRun.Evicted != 0 || len(after) != 10 {
		t.Fatalf("%+v %+v", st, st.LastRun)
	}
	if after[0].Quality >= 82 || after[9].Quality != 82 {
		t.Fatal("least recently used must shrink first", after[0].Quality, after[9].Quality)
	}
	e.cache.d.Settings.MaxSize, e.cache.d.Settings.Degrade.LowWater = total/4, 0.8
	if err := e.cache.Degrade(ctx); err != nil {
		t.Fatal(err)
	}
	st, _ = e.cache.Status(ctx)
	if st.Usage > e.cache.d.Settings.low() || st.LastRun.Resized == 0 && st.LastRun.Evicted == 0 {
		t.Fatalf("%+v", st.LastRun)
	}
	for _, x := range stored(t, e) {
		if st.LastRun.Evicted > 0 && (x.Quality != 60 || x.Width != 100) {
			t.Fatalf("evicted before shrinking %+v", x)
		}
	}
	before := st.LastRun
	if err := e.cache.Degrade(ctx); err != nil || e.cache.last.Load() != before {
		t.Fatal("a second run below the mark must do nothing")
	}
}

func TestEvictionTakesRecordsAndOtherArtworkBeforeLibraryArtwork(t *testing.T) {
	meta := &fakeMeta{bytes: 1000, evictable: 600}
	lib := map[string]bool{}
	e := newEnv(t, func(d *Deps) {
		smallSettings(d)
		d.Settings.Degrade.QualityFloor, d.Settings.Degrade.WidthFloor = 82, 200 // nothing can shrink
		d.Metadata = meta
		d.Library = func(context.Context) (map[string]bool, error) { return lib, nil }
	})
	libURLs := fill(t, e, 3, "lib")
	for _, u := range libURLs {
		lib[u] = true
	}
	fill(t, e, 3, "other")
	if _, err := e.cache.Image(ctx, e.token(RolePoster, libURLs[0]), 96); err != nil {
		t.Fatal(err)
	}
	usage, _ := e.cache.usage(ctx)
	e.cache.d.Settings.MaxSize = usage * 10 / 9 // just at high water; low water needs ~11 % freed
	if err := e.cache.Degrade(ctx); err != nil {
		t.Fatal(err)
	}
	left := map[string]bool{}
	variants := 0
	for _, x := range stored(t, e) {
		left[x.Origin] = true
		if x.Variant > 0 {
			variants++
		}
	}
	st := e.cache.last.Load()
	if meta.evictable != 0 || st.MetadataEvicted != 1 || variants != 0 {
		t.Fatalf("records and copies go first: %+v meta %+v", st, meta)
	}
	for _, u := range libURLs {
		if !left[u] {
			t.Fatalf("library artwork %s evicted while other artwork remained: %v", u, left)
		}
	}
	if left["https://img.example/other0.jpg"] {
		t.Fatal("least recently used other artwork kept")
	}
}

// TestCapHoldsUnderLoad: with a cap far below the artwork stored (2 000
// posters, scaled down so the run stays short), usage never exceeds the cap,
// shrinking runs before any eviction, and library artwork outlives the rest.
func TestCapHoldsUnderLoad(t *testing.T) {
	n, capBytes, libN := 2000, int64(1<<20), 100
	if testing.Short() || raceOn {
		n, capBytes, libN = 400, 200<<10, 20
	}
	lib := map[string]bool{}
	e := newEnv(t, func(d *Deps) {
		d.Settings.MaxSize = capBytes
		d.Settings.MaxWidth, d.Settings.Degrade.WidthFloor = 64, 52
		d.Library = func(context.Context) (map[string]bool, error) { return lib, nil }
	})
	src := jpegBytes(t, static(96, 144, 7), 92)
	firstEviction := false
	for i := 0; i < n; i++ {
		u := fmt.Sprintf("https://img.example/p%d.jpg", i)
		if i < libN {
			lib[u] = true
		}
		e.doer.set(u, reply{status: 200, body: src})
		if _, err := e.cache.Image(ctx, e.token(RolePoster, u), 0); err != nil {
			t.Fatal(err)
		}
		e.clock.add(time.Second)
		usage, _ := e.cache.usage(ctx)
		if usage > capBytes {
			t.Fatalf("usage %d over the cap after %d images", usage, i+1)
		}
		if last := e.cache.last.Load(); !firstEviction && last != nil && last.Evicted > 0 {
			firstEviction = true
			list := stored(t, e)
			for _, x := range list[:len(list)-1] { // the newest arrived after the run
				if shrinkable(t, e, x) {
					t.Fatalf("evicted while %+v could still shrink", x)
				}
			}
		}
	}
	if !firstEviction {
		t.Fatal("the load never reached eviction; raise n")
	}
	left := map[string]bool{}
	for _, x := range stored(t, e) {
		left[x.Origin] = true
	}
	for u := range lib {
		if !left[u] {
			t.Fatalf("library artwork %s evicted", u)
		}
	}
}

// static is per-pixel noise: it barely compresses, so even floor-sized
// artwork fills a small cap.
func static(w, h int, seed uint64) *image.RGBA {
	r := rand.New(rand.NewPCG(seed, seed+1))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = uint8(r.IntN(256)), uint8(r.IntN(256)), uint8(r.IntN(256)), 255
	}
	return img
}

// shrinkable reports whether one more quality or width step would make a
// stored file smaller (a step that would not is skipped by design).
func shrinkable(t testing.TB, e *env, x store.ImageEntry) bool {
	t.Helper()
	data, err := os.ReadFile(e.cache.path(x))
	if err != nil {
		t.Fatal(err)
	}
	d := e.cache.d.Settings.Degrade
	if q := d.nextQuality(x.Quality); q != x.Quality {
		if enc, _ := process(data, x.Width, q); int64(len(enc.Data)) < x.Bytes {
			return true
		}
	}
	if w := nextWidth(x.Width, d.WidthFloor, d.WidthStep); w != x.Width {
		if enc, _ := process(data, w, max(x.Quality, d.QualityFloor)); int64(len(enc.Data)) < x.Bytes {
			return true
		}
	}
	return false
}
