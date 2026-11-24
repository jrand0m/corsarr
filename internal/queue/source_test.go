package queue

import (
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

type protoDL struct {
	plugin.Downloader
	protos []domain.Protocol
}

func (d protoDL) Protocols() []domain.Protocol { return d.protos }

type ownerDL struct {
	protoDL
	key domain.ObjectKey
}

func (d ownerDL) Owns(rel domain.Release) bool { return rel.Indexers[0].Key == d.key }

func TestSourceReleasesGoToTheSourceThatFoundThem(t *testing.T) {
	src := []domain.Protocol{domain.ProtoSource}
	rel := domain.Release{Protocol: domain.ProtoSource, Indexers: []domain.IndexerRef{{Key: "camp"}}}
	if handles(ownerDL{protoDL{protos: src}, "waves"}, rel) || !handles(ownerDL{protoDL{protos: src}, "camp"}, rel) {
		t.Fatal("a source takes only its own releases")
	}
	if handles(protoDL{protos: []domain.Protocol{domain.ProtoTorrent}}, rel) || !handles(protoDL{protos: src}, rel) {
		t.Fatal("protocol match")
	}
}
