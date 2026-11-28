package imagecache

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/jrand0m/corsarr/internal/api"
)

// Mount registers the image route and the cache status and clear endpoints.
func (c *Cache) Mount(m api.Mounter) {
	m.Handle("GET /api/v1/images/{token}", c.serve)
	m.Handle("GET /api/v1/system/cache", c.status)
	m.Handle("DELETE /api/v1/system/cache", c.clear)
}

func (c *Cache) serve(w http.ResponseWriter, r *http.Request) {
	width := 0
	if v := r.URL.Query().Get("w"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			api.WriteError(w, http.StatusBadRequest, "invalid", "w must be a positive number of pixels", "w")
			return
		}
		width = n
	}
	img, err := c.Image(r.Context(), r.PathValue("token"), width)
	if err != nil {
		writeImageErr(w, err)
		return
	}
	body, err := img.open()
	if err != nil {
		api.WriteError(w, http.StatusNotFound, "not_found", "the image is no longer stored; reload the page", "")
		return
	}
	defer body.Close()
	h := w.Header()
	h.Set("Content-Type", "image/"+img.Format)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, max-age=3600")
	if img.Stored {
		h.Set("ETag", img.ETag)
		h.Set("Cache-Control", "private, max-age=604800")
	}
	http.ServeContent(w, r, "", img.ModTime, body)
}

type readSeekCloser interface {
	io.ReadSeeker
	io.Closer
}

type bytesBody struct{ *bytes.Reader }

func (bytesBody) Close() error { return nil }

func (img Image) open() (readSeekCloser, error) {
	if !img.Stored {
		return bytesBody{bytes.NewReader(img.Data)}, nil
	}
	return os.Open(img.Path)
}

func writeImageErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		api.WriteError(w, http.StatusNotFound, "not_found", "no such image", "")
	case errors.Is(err, context.Canceled):
		api.WriteError(w, http.StatusServiceUnavailable, "cancelled", "the request was cancelled", "")
	default:
		api.WriteError(w, http.StatusBadGateway, "upstream", "the artwork could not be fetched: "+err.Error(), "")
	}
}

// Status is the cache's usage for System › Health.
type Status struct {
	Enabled       bool      `json:"enabled"`
	Dir           string    `json:"dir"`
	MaxSize       int64     `json:"maxSize"`
	Usage         int64     `json:"usage"`
	ImageBytes    int64     `json:"imageBytes"`
	MetadataBytes int64     `json:"metadataBytes"`
	Images        int       `json:"images"`
	Variants      int       `json:"variants"`
	Misses        int       `json:"misses"`
	Requests      int64     `json:"requests"`
	Hits          int64     `json:"hits"`
	HitRate       float64   `json:"hitRate"`
	HighWater     float64   `json:"highWater"`
	LowWater      float64   `json:"lowWater"`
	LastRun       *RunStats `json:"lastRun,omitempty"`
}

// Status reports usage, counters since start and the last degradation run.
func (c *Cache) Status(ctx context.Context) (Status, error) {
	s := c.d.Settings
	u, err := c.d.Index.Usage(ctx)
	if err != nil {
		return Status{}, err
	}
	var meta int64
	if c.d.Metadata != nil {
		if meta, err = c.d.Metadata.Bytes(ctx); err != nil {
			return Status{}, err
		}
	}
	out := Status{Enabled: s.Enabled, Dir: s.Dir, MaxSize: s.MaxSize, Usage: u.Bytes + meta, ImageBytes: u.Bytes, MetadataBytes: meta,
		Images: u.Originals, Variants: u.Variants, Misses: u.Misses, Requests: c.requests.Load(), Hits: c.hits.Load(),
		HighWater: s.Degrade.HighWater, LowWater: s.Degrade.LowWater, LastRun: c.last.Load()}
	if out.Requests > 0 {
		out.HitRate = float64(out.Hits) / float64(out.Requests)
	}
	return out, nil
}

func (c *Cache) status(w http.ResponseWriter, r *http.Request) {
	st, err := c.Status(r.Context())
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "internal", err.Error(), "")
		return
	}
	api.WriteJSON(w, http.StatusOK, st)
}

func (c *Cache) clear(w http.ResponseWriter, r *http.Request) {
	if err := c.Clear(r.Context()); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "internal", err.Error(), "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
