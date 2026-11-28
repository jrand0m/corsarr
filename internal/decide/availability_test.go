package decide

import (
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
)

func torrent(seeders, peers int, reported bool) domain.Release {
	return domain.Release{Protocol: domain.ProtoTorrent, Seeders: seeders, Peers: peers, SwarmReported: reported}
}

func TestSwarmOf(t *testing.T) {
	cases := []struct {
		name     string
		rel      domain.Release
		leechers int // -1 = not reported
		health   Health
		reported bool
	}{
		{"healthy", torrent(120, 130, true), 10, HealthGood, true},
		{"starved", torrent(2, 92, true), 90, HealthFew, true},
		{"some", torrent(5, 5, true), 0, HealthSome, true},
		{"no seeders", torrent(0, 15, true), 15, HealthNone, true},
		{"empty", torrent(0, 0, true), 0, HealthNone, true},
		{"peers below seeders", torrent(30, 4, true), -1, HealthGood, true},
		{"not reported", torrent(0, 0, false), -1, HealthUnknown, false},
		{"old stored release", torrent(3, 9, false), 6, HealthFew, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := SwarmOf(c.rel)
			got := -1
			if s.Leechers != nil {
				got = *s.Leechers
			}
			if got != c.leechers || s.Health != c.health || s.Reported != c.reported || s.Seeders != c.rel.Seeders {
				t.Fatalf("got %+v leechers %d", s, got)
			}
		})
	}
	if SwarmOf(domain.Release{Protocol: domain.ProtoUsenet}) != nil || SwarmOf(domain.Release{Protocol: domain.ProtoSource}) != nil {
		t.Fatal("no swarm outside torrents")
	}
}

func TestAvailabilityOf(t *testing.T) {
	cases := []struct {
		name string
		rel  domain.Release
		want AvailabilityStatus
	}{
		{"seeded", torrent(1, 1, true), AvailComplete},
		{"peers only", torrent(0, 14, true), AvailPartial},
		{"dead", torrent(0, 0, true), AvailNoPeers},
		{"silent", torrent(0, 0, false), AvailUnknown},
		{"usenet", domain.Release{Protocol: domain.ProtoUsenet, Grabs: 5}, AvailUnknown},
		{"source", domain.Release{Protocol: domain.ProtoSource}, AvailComplete},
	}
	for _, c := range cases {
		if got := AvailabilityOf(c.rel).Status; got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
	r := torrent(3, 4, true)
	r.Grabs, r.Files, r.MinimumRatio = 210, 3, 1
	if a := AvailabilityOf(r); a.Grabs != 210 || a.Files != 3 || a.MinimumRatio != 1 {
		t.Fatalf("evidence %+v", a)
	}
}
