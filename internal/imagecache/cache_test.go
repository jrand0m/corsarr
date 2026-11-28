package imagecache

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestFirstViewFetchesThenServesFromDisk(t *testing.T) {
	e := newEnv(t, nil)
	tok := e.poster("https://img.example/p.jpg", noisy(1000, 1500, 1))
	img, err := e.cache.Image(ctx, tok, 0)
	if err != nil || !img.Stored || img.Format != FormatJPEG || img.ETag == "" {
		t.Fatalf("%+v %v", img, err)
	}
	if cfg := decodeServed(t, img); cfg.Width != 600 || cfg.Height != 900 {
		t.Fatalf("%+v", cfg)
	}
	e.doer.down = true
	again, err := e.cache.Image(ctx, tok, 0)
	if err != nil || again.Path != img.Path || e.doer.count("https://img.example/p.jpg") != 1 {
		t.Fatal("provider asked again or cache not used", err)
	}
	st, _ := e.cache.Status(ctx)
	if st.Images != 1 || st.Requests != 2 || st.Hits != 1 || st.HitRate != 0.5 || st.ImageBytes == 0 {
		t.Fatalf("%+v", st)
	}
}

func TestBackdropsUseTheirOwnWidth(t *testing.T) {
	e := newEnv(t, nil)
	e.doer.set("https://img.example/b.jpg", reply{status: http.StatusOK, body: jpegBytes(t, noisy(1920, 1080, 2), 90)})
	img, err := e.cache.Image(ctx, e.token(RoleBackdrop, "https://img.example/b.jpg"), 0)
	if err != nil || decodeServed(t, img).Width != 1280 {
		t.Fatal(err)
	}
}

func TestSmallerCopiesAreMadeOnceFromTheOriginal(t *testing.T) {
	e := newEnv(t, nil)
	tok := e.poster("https://img.example/p.jpg", noisy(1000, 1500, 1))
	img, err := e.cache.Image(ctx, tok, 150)
	if err != nil || decodeServed(t, img).Width != 160 {
		t.Fatal(err)
	}
	e.doer.down = true
	again, err := e.cache.Image(ctx, tok, 160)
	if err != nil || again.Path != img.Path {
		t.Fatal("variant not reused", err)
	}
	if big, _ := e.cache.Image(ctx, tok, 2000); decodeServed(t, big).Width != 600 {
		t.Fatal("wide request must serve the original")
	}
	st, _ := e.cache.Status(ctx)
	if st.Images != 1 || st.Variants != 1 {
		t.Fatalf("%+v", st)
	}
}

func TestMissesAreRememberedForADay(t *testing.T) {
	e := newEnv(t, nil)
	tok := e.token(RolePoster, "https://img.example/none.jpg")
	for i := 0; i < 2; i++ {
		if _, err := e.cache.Image(ctx, tok, 0); !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
	}
	if n := e.doer.count("https://img.example/none.jpg"); n != 1 {
		t.Fatalf("provider asked %d times", n)
	}
	e.clock.add(25 * time.Hour)
	e.poster("https://img.example/none.jpg", noisy(100, 150, 3))
	if _, err := e.cache.Image(ctx, tok, 0); err != nil {
		t.Fatal("miss not retried after a day", err)
	}
}

func TestBadTokensAndUpstreamFailures(t *testing.T) {
	e := newEnv(t, nil)
	if _, err := e.cache.Image(ctx, "forged", 0); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := e.cache.Image(ctx, NewSigner([]byte("other")).Sign(RolePoster, "https://img.example/p.jpg"), 0); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	e.doer.set("https://img.example/html", reply{status: http.StatusOK, body: []byte("<html>")})
	if _, err := e.cache.Image(ctx, e.token(RolePoster, "https://img.example/html"), 0); !errors.Is(err, ErrNotImage) {
		t.Fatal(err)
	}
	e.doer.down = true
	_, err := e.cache.Image(ctx, e.token(RolePoster, "https://img.example/x"), 0)
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if st, _ := e.cache.Status(ctx); st.Misses != 0 {
		t.Fatal("a transport failure must not be remembered")
	}
}

func TestDisabledCacheStoresNothing(t *testing.T) {
	e := newEnv(t, func(d *Deps) { d.Settings.Enabled = false })
	tok := e.poster("https://img.example/p.jpg", noisy(1000, 1500, 1))
	img, err := e.cache.Image(ctx, tok, 200)
	if err != nil || img.Stored || len(img.Data) == 0 || decodeServed(t, img).Width != 240 {
		t.Fatal(err)
	}
	if _, err := os.Stat(e.cache.imagesDir()); !os.IsNotExist(err) {
		t.Fatal("files written while disabled")
	}
	if err := e.cache.Degrade(ctx); err != nil {
		t.Fatal(err)
	}
	e.doer.set("https://img.example/none", reply{status: http.StatusNotFound})
	if _, err := e.cache.Image(ctx, e.token(RolePoster, "https://img.example/none"), 0); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestConcurrentFirstViewsFetchOnce(t *testing.T) {
	e := newEnv(t, nil)
	tok := e.poster("https://img.example/p.jpg", noisy(300, 450, 1))
	e.doer.gate = make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := e.cache.Image(ctx, tok, 0); errs <- err }()
	}
	time.Sleep(50 * time.Millisecond)
	close(e.doer.gate)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n := e.doer.count("https://img.example/p.jpg"); n != 1 {
		t.Fatalf("fetched %d times", n)
	}
}

func TestAccessIsRecordedAtMostHourly(t *testing.T) {
	e := newEnv(t, nil)
	tok := e.poster("https://img.example/p.jpg", noisy(100, 150, 1))
	_, _ = e.cache.Image(ctx, tok, 0)
	key := Key("https://img.example/p.jpg")
	e.clock.add(30 * time.Minute)
	_, _ = e.cache.Image(ctx, tok, 0)
	row, _, _ := e.st.Images().Get(ctx, key, 0)
	if !row.LastAccess.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("touched within the hour", row.LastAccess)
	}
	e.clock.add(time.Hour)
	_, _ = e.cache.Image(ctx, tok, 0)
	if row, _, _ = e.st.Images().Get(ctx, key, 0); !row.LastAccess.Equal(e.clock.now()) {
		t.Fatal("not touched after an hour", row.LastAccess)
	}
}

func TestMissingFileIsFetchedAgainAndClearEmptiesTheCache(t *testing.T) {
	e := newEnv(t, nil)
	tok := e.poster("https://img.example/p.jpg", noisy(100, 150, 1))
	img, _ := e.cache.Image(ctx, tok, 0)
	if err := os.Remove(img.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := e.cache.Image(ctx, tok, 0); err != nil || e.doer.count("https://img.example/p.jpg") != 2 {
		t.Fatal("lost file not refetched", err)
	}
	if err := e.cache.Clear(ctx); err != nil {
		t.Fatal(err)
	}
	st, _ := e.cache.Status(ctx)
	matches, _ := filepath.Glob(filepath.Join(e.cache.imagesDir(), "*", "*"))
	if st.Images != 0 || st.ImageBytes != 0 || len(matches) != 0 {
		t.Fatalf("%+v %v", st, matches)
	}
	if err := e.cache.Clear(ctx); err != nil {
		t.Fatal("clear twice", err)
	}
}

func TestLinkUsesTheSigner(t *testing.T) {
	e := newEnv(t, nil)
	link := e.cache.Link(RolePoster, "https://img.example/p.jpg")
	if _, origin, ok := e.sign.Open(link[len(LinkPrefix):]); !ok || origin != "https://img.example/p.jpg" {
		t.Fatal(link)
	}
}
