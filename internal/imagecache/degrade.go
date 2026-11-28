package imagecache

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/jrand0m/corsarr/internal/store"
)

// DegradeJob is the scheduled job that keeps the cache under its cap.
const (
	DegradeJob      = "cache.degrade"
	DegradeInterval = time.Hour
)

// RunStats is what one degradation run did.
type RunStats struct {
	At              time.Time `json:"at"`
	Reencoded       int       `json:"reencoded"`
	Resized         int       `json:"resized"`
	Evicted         int       `json:"evicted"`
	MetadataEvicted int       `json:"metadataEvicted"`
	BytesFreed      int64     `json:"bytesFreed"`
	Before          int64     `json:"-"`
	After           int64     `json:"-"`
}

func (s RunStats) acted() bool { return s.Reencoded+s.Resized+s.Evicted+s.MetadataEvicted > 0 }

// Degrade runs when usage is at or above the high-water mark and shrinks the
// cache to the low-water mark. Below the mark it does nothing, so running it
// twice ends in the same state.
func (c *Cache) Degrade(ctx context.Context) error {
	if !c.d.Settings.Enabled {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.d.Index.DeleteMisses(ctx, c.d.Now()); err != nil {
		return err
	}
	usage, err := c.usage(ctx)
	if err != nil || usage < c.d.Settings.high() {
		return err
	}
	st, err := c.degrade(ctx, usage, c.d.Settings.low())
	c.record(st)
	return err
}

func (c *Cache) record(st RunStats) {
	if !st.acted() {
		return
	}
	c.last.Store(&st)
	c.d.Log.Info("image cache shrunk", "reencoded", st.Reencoded, "resized", st.Resized, "evicted", st.Evicted,
		"metadataEvicted", st.MetadataEvicted, "freed", st.BytesFreed, "usage", st.After)
}

// degrader carries one run's state; the caller holds mu.
type degrader struct {
	c      *Cache
	ctx    context.Context
	target int64
	usage  int64
	st     RunStats
	gone   map[string]bool // key-variant of files removed during the run
}

func goneKey(e store.ImageEntry) string { return e.Key + "-" + strconv.Itoa(e.Variant) }

// degrade shrinks until usage ≤ target: lower quality, then smaller width,
// then eviction by tier (records of items outside the library, smaller
// copies, other artwork, library artwork), least recently used first.
func (c *Cache) degrade(ctx context.Context, usage, target int64) (RunStats, error) {
	g := &degrader{c: c, ctx: ctx, target: target, usage: usage, st: RunStats{At: c.d.Now(), Before: usage}, gone: map[string]bool{}}
	err := g.run()
	g.st.After, g.st.BytesFreed = g.usage, g.st.Before-g.usage
	return g.st, err
}

func (g *degrader) done() bool { return g.usage <= g.target }

func (g *degrader) run() error {
	entries, err := g.c.d.Index.Stored(g.ctx)
	if err != nil {
		return err
	}
	// Quality first, then width; a smaller width can make one more quality
	// step pay off, so both repeat until neither shrinks anything.
	for more := true; more && !g.done(); {
		q, err := g.passes(entries, g.lowerQuality)
		if err != nil || g.done() {
			return err
		}
		w, err := g.passes(entries, g.lowerWidth)
		if err != nil {
			return err
		}
		more = q || w
	}
	if g.done() {
		return nil
	}
	return g.evict(entries)
}

// passes applies one shrink step to each original, least recently used
// first, and repeats until the target is met or nothing can shrink further;
// it reports whether anything shrank.
func (g *degrader) passes(entries []store.ImageEntry, step func(*store.ImageEntry) (bool, error)) (bool, error) {
	any := false
	for progressed := true; progressed && !g.done(); {
		progressed = false
		for i := range entries {
			if g.done() {
				return true, nil
			}
			if entries[i].Variant > 0 || entries[i].Format == "" {
				continue
			}
			changed, err := step(&entries[i])
			if err != nil {
				return any, err
			}
			progressed = progressed || changed
			any = any || changed
		}
	}
	return any, nil
}

func (g *degrader) lowerQuality(e *store.ImageEntry) (bool, error) {
	q := g.c.d.Settings.Degrade.nextQuality(e.Quality)
	if e.Format != FormatJPEG || q == e.Quality {
		return false, nil
	}
	ok, err := g.rewrite(e, e.Width, q)
	if ok {
		g.st.Reencoded++
	}
	return ok, err
}

func (g *degrader) lowerWidth(e *store.ImageEntry) (bool, error) {
	s := g.c.d.Settings
	role := RolePoster
	if e.Role == RoleBackdrop.String() {
		role = RoleBackdrop
	}
	w := nextWidth(e.Width, s.widthFloor(role), s.Degrade.WidthStep)
	if w == e.Width {
		return false, nil
	}
	ok, err := g.rewrite(e, w, max(e.Quality, s.Degrade.QualityFloor))
	if ok {
		g.st.Resized++
		err = g.dropVariants(e.Key)
	}
	return ok, err
}

// rewrite re-encodes a stored original at a width and quality; it keeps the
// old file when the new one would not be smaller.
func (g *degrader) rewrite(e *store.ImageEntry, width, quality int) (bool, error) {
	path := g.c.path(*e)
	data, err := os.ReadFile(path)
	if err != nil {
		return false, g.forget(e)
	}
	enc, err := process(data, width, quality)
	if err != nil || int64(len(enc.Data)) >= e.Bytes {
		return false, nil
	}
	next := *e
	next.Format, next.Bytes, next.Width, next.Height, next.Quality = enc.Format, int64(len(enc.Data)), enc.Width, enc.Height, enc.Quality
	if next.Format != e.Format {
		return false, nil // a transparent image stays as it is
	}
	if err := g.c.write(path, enc.Data); err != nil {
		return false, err
	}
	if err := g.c.d.Index.Put(g.ctx, next); err != nil {
		return false, err
	}
	g.usage -= e.Bytes - next.Bytes
	*e = next
	return true, nil
}

// forget drops the row of a file that disappeared from disk.
func (g *degrader) forget(e *store.ImageEntry) error {
	g.usage -= e.Bytes
	err := g.c.d.Index.Delete(g.ctx, e.Key, e.Variant)
	e.Format, e.Bytes = "", 0
	return err
}

func (g *degrader) dropVariants(key string) error {
	vs, err := g.c.d.Index.Variants(g.ctx, key)
	for _, v := range vs {
		if err != nil {
			break
		}
		if err = g.c.remove(g.ctx, v); err == nil {
			g.usage -= v.Bytes
			g.gone[goneKey(v)] = true
		}
	}
	return err
}

// evict removes records of items outside the library first, then files by
// tier, least recently used first.
func (g *degrader) evict(entries []store.ImageEntry) error {
	if m := g.c.d.Metadata; m != nil {
		n, freed, err := m.EvictUnused(g.ctx, g.usage-g.target)
		if err != nil {
			return err
		}
		g.st.MetadataEvicted, g.usage = n, g.usage-freed
	}
	library, err := g.library()
	if err != nil {
		return err
	}
	for _, e := range evictionOrder(entries, library) {
		if g.done() {
			return nil
		}
		if e.Format == "" || g.gone[goneKey(e)] {
			continue
		}
		if err := g.c.remove(g.ctx, e); err != nil {
			return err
		}
		g.usage -= e.Bytes
		g.st.Evicted++
	}
	return nil
}

func (g *degrader) library() (map[string]bool, error) {
	if g.c.d.Library == nil {
		return map[string]bool{}, nil
	}
	return g.c.d.Library(g.ctx)
}
