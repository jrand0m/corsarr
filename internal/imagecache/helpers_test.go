package imagecache

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/store"
)

var ctx = context.Background()

// noisy is an opaque image that compresses poorly, so sizes behave like photos.
func noisy(w, h int, seed uint64) *image.RGBA {
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(x*255/w) ^ uint8(r.IntN(64))
			img.Set(x, y, color.RGBA{v, uint8(y * 255 / h), uint8(r.IntN(256)), 255})
		}
	}
	return img
}

func jpegBytes(t testing.TB, img image.Image, q int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: q}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func pngBytes(t testing.TB, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// reply is one canned provider answer.
type reply struct {
	status   int
	body     []byte
	location string
}

// fakeDoer answers from a URL table and counts calls; down makes every call fail.
type fakeDoer struct {
	mu      sync.Mutex
	replies map[string]reply
	calls   map[string]int
	down    bool
	gate    chan struct{} // when set, each call waits for it
}

func newDoer() *fakeDoer { return &fakeDoer{replies: map[string]reply{}, calls: map[string]int{}} }

func (d *fakeDoer) set(url string, r reply) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.replies[url] = r
}

func (d *fakeDoer) count(url string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls[url]
}

func (d *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	if d.gate != nil {
		<-d.gate
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	u := req.URL.String()
	d.calls[u]++
	if d.down {
		return nil, io.ErrUnexpectedEOF
	}
	r, ok := d.replies[u]
	if !ok {
		r = reply{status: http.StatusNotFound}
	}
	h := http.Header{}
	if r.location != "" {
		h.Set("Location", r.location)
	}
	return &http.Response{StatusCode: r.status, Header: h, Body: io.NopCloser(bytes.NewReader(r.body)), Request: req}, nil
}

func testSettings(dir string) Settings {
	return Settings{Enabled: true, Dir: dir, MaxSize: 1 << 30, Quality: 82, MaxWidth: 600, BackdropMaxWidth: 1280,
		Degrade: Degrade{QualityFloor: 60, QualityStep: 10, WidthFloor: 300, WidthStep: 0.75, HighWater: 0.9, LowWater: 0.8}}
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type env struct {
	t     testing.TB
	st    *store.Store
	doer  *fakeDoer
	clock *clock
	cache *Cache
	sign  Signer
}

func newEnv(t testing.TB, tune func(*Deps)) *env {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	e := &env{t: t, st: st, doer: newDoer(), clock: &clock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}, sign: NewSigner([]byte("k"))}
	d := Deps{Settings: testSettings(dir + "/cache"), Index: st.Images(), HTTP: e.doer, Signer: e.sign, Now: e.clock.now}
	if tune != nil {
		tune(&d)
	}
	e.cache = New(d)
	return e
}

func (e *env) token(role Role, url string) string { return e.sign.Sign(role, url) }

func (e *env) poster(url string, img image.Image) string {
	e.doer.set(url, reply{status: http.StatusOK, body: jpegBytes(e.t, img, 92)})
	return e.token(RolePoster, url)
}

func decodeServed(t testing.TB, img Image) image.Config {
	t.Helper()
	body, err := img.open()
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	cfg, _, err := image.DecodeConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func contains(b []byte, s string) bool { return strings.Contains(string(b), s) }
