package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Cache is the cache section: the artwork cache and how it shrinks near its size cap.
type Cache struct {
	Enabled bool
	Dir     string // "" = <data.dir>/cache
	MaxSize int64  // bytes
	Images  CacheImages
}

// CacheImages are the stored-image settings.
type CacheImages struct {
	Quality          int
	MaxWidth         int
	BackdropMaxWidth int
	Degrade          CacheDegrade
}

// CacheDegrade are the rules for shrinking images as the cache nears its cap.
type CacheDegrade struct {
	QualityFloor int
	QualityStep  int
	WidthFloor   int
	WidthStep    float64
	HighWater    float64
	LowWater     float64
}

func cacheNode() *Node {
	num := func(def, doc string, lo, hi float64) *Node {
		return &Node{Kind: KindNumber, Default: def, Doc: doc, Check: between(lo, hi)}
	}
	integer := func(def, doc string, lo, hi float64) *Node {
		return &Node{Kind: KindInt, Default: def, Doc: doc, Check: between(lo, hi)}
	}
	return obj("Artwork cache: posters and covers are fetched once, stored and served by corsarr (applies at start).",
		Field{Name: "enabled", Node: &Node{Kind: KindBool, Default: "true",
			Doc: "Keep fetched artwork on disk; turn off to fetch artwork from the provider on every view."}},
		Field{Name: "dir", Node: &Node{Kind: KindString, Check: checkOptionalAbsPath,
			Doc: "Folder for cached artwork; leave empty to use the cache folder inside the data folder."}},
		Field{Name: "maxSize", Node: &Node{Kind: KindString, Default: "1GiB", Check: checkCacheSize,
			Doc: "Most disk space the cache may use, with a unit such as 500MiB or 2GiB."}},
		Field{Name: "images", Node: obj("How artwork is stored.",
			Field{Name: "quality", Node: integer("82", "JPEG quality of stored artwork, from 30 to 95; lower saves space, higher looks sharper.", 30, 95)},
			Field{Name: "maxWidth", Node: integer("600", "Widest a stored poster or cover may be, in pixels, from 100 to 2000.", 100, 2000)},
			Field{Name: "backdropMaxWidth", Node: integer("1280", "Widest a stored backdrop may be, in pixels, from 100 to 4000.", 100, 4000)},
			Field{Name: "degrade", Node: obj("How artwork shrinks when the cache nears its size cap: lower quality first, then smaller sizes, then the least recently viewed is removed.",
				Field{Name: "qualityFloor", Node: integer("60", "Lowest JPEG quality artwork is reduced to before it is made smaller.", 30, 95)},
				Field{Name: "qualityStep", Node: integer("10", "How much the JPEG quality drops in each reduction step.", 1, 50)},
				Field{Name: "widthFloor", Node: integer("300", "Narrowest a poster or cover is made, in pixels, before it is removed instead.", 50, 2000)},
				Field{Name: "widthStep", Node: num("0.75", "Share of its width artwork keeps in each size reduction step, between 0.5 and 0.95.", 0.5, 0.95)},
				Field{Name: "highWater", Node: num("0.9", "Share of the size cap at which shrinking starts, between 0.5 and 1.", 0.5, 1)},
				Field{Name: "lowWater", Node: num("0.8", "Share of the size cap at which shrinking stops, between 0.3 and 0.95; keep it below highWater.", 0.3, 0.95)},
			)},
		)},
	)
}

func between(lo, hi float64) func(string) error {
	return func(s string) error {
		v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil || v < lo || v > hi {
			return fmt.Errorf("must be a number from %g to %g", lo, hi)
		}
		return nil
	}
}

func checkOptionalAbsPath(s string) error {
	if s == "" {
		return nil
	}
	return checkAbsPath(s)
}

func checkCacheSize(s string) error {
	n, err := ParseSize(s)
	if err != nil {
		return err
	}
	if n < 1<<20 {
		return fmt.Errorf("must be at least 1MiB")
	}
	return nil
}

var sizeUnits = map[string]int64{
	"": 1, "b": 1,
	"k": 1 << 10, "kb": 1 << 10, "kib": 1 << 10,
	"m": 1 << 20, "mb": 1 << 20, "mib": 1 << 20,
	"g": 1 << 30, "gb": 1 << 30, "gib": 1 << 30,
	"t": 1 << 40, "tb": 1 << 40, "tib": 1 << 40,
}

// ParseSize reads a byte size such as 1GiB, 500MB, 1.5G or a bare byte count;
// every prefix is a power of 1024.
func ParseSize(s string) (int64, error) {
	t := strings.ToLower(strings.TrimSpace(s))
	i := strings.IndexFunc(t, func(r rune) bool { return (r < '0' || r > '9') && r != '.' })
	if i < 0 {
		i = len(t)
	}
	num, unit := t[:i], strings.TrimSpace(t[i:])
	mult, ok := sizeUnits[unit]
	f, err := strconv.ParseFloat(num, 64)
	if !ok || err != nil || f < 0 || f*float64(mult) > math.MaxInt64/2 {
		return 0, fmt.Errorf("must be a size such as 500MiB or 2GiB")
	}
	return int64(f * float64(mult)), nil
}

func cacheOf(t tree) Cache {
	c, _ := t["cache"].(tree)
	img, _ := c["images"].(tree)
	deg, _ := img["degrade"].(tree)
	out := Cache{}
	out.Enabled, _ = c["enabled"].(bool)
	out.Dir, _ = c["dir"].(string)
	size, _ := c["maxSize"].(string)
	out.MaxSize, _ = ParseSize(size) // validated by the schema
	out.Images = CacheImages{Quality: intOf(img["quality"]), MaxWidth: intOf(img["maxWidth"]),
		BackdropMaxWidth: intOf(img["backdropMaxWidth"])}
	out.Images.Degrade = CacheDegrade{QualityFloor: intOf(deg["qualityFloor"]), QualityStep: intOf(deg["qualityStep"]),
		WidthFloor: intOf(deg["widthFloor"]), WidthStep: floatOf(deg["widthStep"]),
		HighWater: floatOf(deg["highWater"]), LowWater: floatOf(deg["lowWater"])}
	return out
}

func floatOf(v any) float64 {
	f, _ := v.(float64)
	return f
}

// cacheErrors reports the cache settings that contradict each other.
func cacheErrors(c Cache, locs map[string]Location) Errors {
	var out Errors
	add := func(bad bool, path, msg string) {
		if bad {
			out = append(out, FieldError{Path: path, Loc: locs[path], Msg: msg})
		}
	}
	d := c.Images.Degrade
	add(d.LowWater >= d.HighWater, "cache.images.degrade.lowWater", "must be below cache.images.degrade.highWater")
	add(d.QualityFloor > c.Images.Quality, "cache.images.degrade.qualityFloor", "must not be above cache.images.quality")
	add(d.WidthFloor > c.Images.MaxWidth, "cache.images.degrade.widthFloor", "must not be above cache.images.maxWidth")
	return out
}
