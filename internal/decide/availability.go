package decide

import "github.com/jrand0m/corsarr/internal/domain"

// Health names how well a torrent is seeded.
type Health string

const (
	HealthUnknown Health = "unknown" // the indexer did not report the swarm
	HealthNone    Health = "none"    // no seeders
	HealthFew     Health = "few"     // 1-4 seeders
	HealthSome    Health = "some"    // 5-19 seeders
	HealthGood    Health = "good"    // 20 or more seeders
)

// Swarm is a torrent's seeders and leechers as the indexer reported them.
// Leechers is nil when it cannot be told (peers missing or below seeders).
type Swarm struct {
	Seeders  int    `json:"seeders"`
	Leechers *int   `json:"leechers,omitempty"`
	Reported bool   `json:"reported"`
	Health   Health `json:"health"`
}

// AvailabilityStatus says whether the whole payload can be fetched.
type AvailabilityStatus string

const (
	AvailComplete AvailabilityStatus = "complete"          // a full copy is seeded, or a source serves whole files
	AvailPartial  AvailabilityStatus = "may_be_incomplete" // peers but no seeder
	AvailNoPeers  AvailabilityStatus = "no_peers"          // reported empty swarm
	AvailUnknown  AvailabilityStatus = "unknown"           // nothing reported
)

// Availability is the status plus the evidence the indexer gave.
type Availability struct {
	Status       AvailabilityStatus `json:"status"`
	Grabs        int                `json:"grabs,omitempty"`
	Files        int                `json:"files,omitempty"`
	MinimumRatio float64            `json:"minimumRatio,omitempty"`
}

// swarmReported also trusts non-zero counts from releases stored before the
// indexer said whether it reported the swarm.
func swarmReported(r domain.Release) bool { return r.SwarmReported || r.Seeders > 0 || r.Peers > 0 }

// SwarmOf reads a torrent's swarm; nil for usenet and direct sources.
func SwarmOf(r domain.Release) *Swarm {
	if r.Protocol != domain.ProtoTorrent {
		return nil
	}
	s := &Swarm{Seeders: r.Seeders, Reported: swarmReported(r), Health: HealthUnknown}
	if !s.Reported {
		return s
	}
	if r.Peers >= r.Seeders {
		l := r.Peers - r.Seeders
		s.Leechers = &l
	}
	s.Health = healthOf(r.Seeders)
	return s
}

// healthOf uses the same steps ranking uses to bucket seeders.
func healthOf(seeders int) Health {
	return []Health{HealthNone, HealthFew, HealthSome, HealthGood, HealthGood}[seedersBucket(domain.Release{Protocol: domain.ProtoTorrent, Seeders: seeders})]
}

// AvailabilityOf judges whether the whole payload can be fetched.
func AvailabilityOf(r domain.Release) Availability {
	return Availability{Status: availabilityStatus(r), Grabs: r.Grabs, Files: r.Files, MinimumRatio: r.MinimumRatio}
}

func availabilityStatus(r domain.Release) AvailabilityStatus {
	switch {
	case r.Protocol == domain.ProtoSource:
		return AvailComplete
	case r.Protocol != domain.ProtoTorrent || !swarmReported(r):
		return AvailUnknown
	case r.Seeders > 0:
		return AvailComplete
	case r.Peers > 0:
		return AvailPartial
	}
	return AvailNoPeers
}
