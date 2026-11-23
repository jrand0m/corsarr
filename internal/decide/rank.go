package decide

import (
	"fmt"

	"github.com/jrand0m/corsarr/internal/domain"
)

// rankKey renders the release ordering as a fixed-width string
// where a larger string is a better release:
//
//	quality rank . format score . revision . protocol . indexer priority .
//	seeders bucket . closeness to preferred size . publish time
func rankKey(f facts) string {
	r := f.in.Release
	return fmt.Sprintf("%03d.%06d.%02d.%d.%02d.%d.%05d.%010d",
		qualityPart(f), clamp(f.score+500000, 0, 999999), clamp(f.in.Parsed.Revision(), 0, 99),
		protocolPart(r.Protocol, f.in.Context.PreferredProtocol), priorityPart(r.Indexers),
		seedersBucket(r), closeness(r.Size, f.bounds.Preferred), clamp64(r.PublishedAt.Unix(), 0, 9999999999))
}

func clamp(v, lo, hi int) int { return max(lo, min(v, hi)) }

func clamp64(v, lo, hi int64) int64 { return max(lo, min(v, hi)) }

func qualityPart(f facts) int {
	if !f.allowed {
		return 0
	}
	return clamp(f.rank+1, 0, 999)
}

func protocolPart(p, preferred domain.Protocol) int {
	if preferred != "" && p == preferred {
		return 1
	}
	return 0
}

// priorityPart favours the best (lowest-numbered) indexer that has the release.
func priorityPart(ix []domain.IndexerRef) int {
	if len(ix) == 0 {
		return 0
	}
	best := ix[0].Priority
	for _, i := range ix[1:] {
		best = min(best, i.Priority)
	}
	return 99 - clamp(best, 0, 99)
}

var seederSteps = []int{1, 5, 20, 100}

// seedersBucket groups seeders so small differences do not reorder releases;
// usenet and source releases rank as well seeded.
func seedersBucket(r domain.Release) int {
	if r.Protocol != domain.ProtoTorrent {
		return len(seederSteps)
	}
	b := 0
	for _, step := range seederSteps {
		if r.Seeders >= step {
			b++
		}
	}
	return b
}

// closeness is higher the nearer size is to preferred (in MB, capped).
func closeness(size, preferred int64) int {
	d := size - preferred
	if d < 0 {
		d = -d
	}
	return 99999 - int(min(d/1_000_000, 99999))
}
