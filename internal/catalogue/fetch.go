package catalogue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Doer sends HTTP requests (the shared outbound client).
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// MaxAge is how long a cached index is used before it is fetched again.
const MaxAge = time.Hour

// Fetcher reads a catalogue through HTTP and keeps the last verified copy
// of each catalogue address under Dir.
type Fetcher struct {
	Dir       string
	HTTP      Doer
	Keys      []Key
	MaxModule int64
	Now       func() time.Time

	mu sync.Mutex // one fetch at a time; the cache has one writer
}

// Snapshot is a verified index and where it came from.
type Snapshot struct {
	URL       string
	Index     Index
	FetchedAt time.Time
	Stale     bool  // the copy is the cached one because the fetch failed
	Err       error // why the fetch failed (Stale) or nil
}

type meta struct {
	URL       string    `json:"url"`
	ETag      string    `json:"etag,omitempty"`
	FetchedAt time.Time `json:"fetchedAt"`
}

func (f *Fetcher) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

func (f *Fetcher) dir(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(f.Dir, hex.EncodeToString(sum[:8]))
}

// Get returns the catalogue at url: the cached copy while it is younger than
// MaxAge (unless refresh), otherwise a fresh one. When fetching fails the
// last verified copy is returned with Stale and the error; with no copy the
// error is returned.
func (f *Fetcher) Get(ctx context.Context, url string, refresh bool) (Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cached, m, cacheErr := f.load(url)
	if cacheErr == nil && !refresh && f.now().Sub(m.FetchedAt) < MaxAge {
		return cached, nil
	}
	snap, err := f.fetch(ctx, url, m, cacheErr == nil)
	if err == nil {
		return snap, nil
	}
	if cacheErr == nil {
		cached.Stale, cached.Err = true, err
		return cached, nil
	}
	return Snapshot{URL: url}, err
}

// Cached returns the last verified copy without any network access.
func (f *Fetcher) Cached(url string) (Snapshot, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, _, err := f.load(url)
	return s, err == nil
}

func (f *Fetcher) load(url string) (Snapshot, meta, error) {
	d := f.dir(url)
	var m meta
	b, err := os.ReadFile(filepath.Join(d, "meta.json"))
	if err != nil {
		return Snapshot{}, m, err
	}
	if err := json.Unmarshal(b, &m); err != nil || m.URL != url {
		return Snapshot{}, m, errors.New("catalogue cache does not match")
	}
	index, err1 := os.ReadFile(filepath.Join(d, "index.json"))
	sig, err2 := os.ReadFile(filepath.Join(d, "index.json.sig"))
	if err := errors.Join(err1, err2); err != nil {
		return Snapshot{}, m, err
	}
	ix, err := f.verify(index, sig)
	if err != nil {
		return Snapshot{}, m, err
	}
	return Snapshot{URL: url, Index: ix, FetchedAt: m.FetchedAt}, m, nil
}

func (f *Fetcher) verify(index, sig []byte) (Index, error) {
	if err := Verify(index, sig, f.Keys); err != nil {
		return Index{}, err
	}
	return Decode(index, f.MaxModule)
}

func (f *Fetcher) fetch(ctx context.Context, url string, m meta, haveCache bool) (Snapshot, error) {
	etag := ""
	if haveCache {
		etag = m.ETag
	}
	index, newTag, notModified, err := f.getBytes(ctx, url, etag, MaxIndexBytes)
	if err != nil {
		return Snapshot{}, err
	}
	now := f.now()
	if notModified {
		m.FetchedAt = now
		if err := f.writeMeta(url, m); err != nil {
			return Snapshot{}, err
		}
		s, _, err := f.load(url)
		return s, err
	}
	sig, _, _, err := f.getBytes(ctx, url+".sig", "", 64<<10)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return Snapshot{}, ErrUnsigned
		}
		return Snapshot{}, err
	}
	ix, err := f.verify(index, sig)
	if err != nil {
		return Snapshot{}, err
	}
	if err := f.store(url, index, sig, meta{URL: url, ETag: newTag, FetchedAt: now}); err != nil {
		return Snapshot{}, err
	}
	return Snapshot{URL: url, Index: ix, FetchedAt: now}, nil
}

var errNotFound = errors.New("not found")

// getBytes fetches url (conditionally when etag is set) and reads at most
// limit bytes.
func (f *Fetcher) getBytes(ctx context.Context, url, etag string, limit int64) (body []byte, newTag string, notModified bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", false, fmt.Errorf("catalogue address: %w", err)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, "", false, fmt.Errorf("catalogue unreachable: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotModified && etag != "":
		return nil, etag, true, nil
	case resp.StatusCode == http.StatusNotFound:
		return nil, "", false, fmt.Errorf("%w: %s answered 404", errNotFound, req.URL.Redacted())
	case resp.StatusCode != http.StatusOK:
		return nil, "", false, fmt.Errorf("catalogue answered HTTP %d", resp.StatusCode)
	}
	body, err = io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", false, fmt.Errorf("catalogue read: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, "", false, fmt.Errorf("catalogue file is larger than %d bytes", limit)
	}
	return body, resp.Header.Get("ETag"), false, nil
}

func (f *Fetcher) store(url string, index, sig []byte, m meta) error {
	d := f.dir(url)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(d, "index.json"), index); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(d, "index.json.sig"), sig); err != nil {
		return err
	}
	return f.writeMeta(url, m)
}

func (f *Fetcher) writeMeta(url string, m meta) error {
	b, _ := json.Marshal(m)
	return writeFile(filepath.Join(f.dir(url), "meta.json"), b)
}

func writeFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
