package decide

import (
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

func TestLostToNamesTheFirstDifferingStep(t *testing.T) {
	base := input(t, "Film.2020.1080p.WEB-DL-GRP", 5*gb)
	winner := Decide(base)
	cases := []struct {
		name   string
		change func(in *Input)
		want   string
	}{
		{"quality", func(in *Input) { *in = input(t, "Film.2020.720p.WEB-DL-GRP", 5*gb) }, "quality"},
		{"seeders", func(in *Input) { in.Release.Seeders = 2 }, "seeders"},
		{"indexer", func(in *Input) { in.Release.Indexers = []domain.IndexerRef{{Key: "b", Priority: 50}} }, "indexer_priority"},
		{"size", func(in *Input) { in.Release.Size = 2 * gb }, "size"},
		{"age", func(in *Input) { in.Release.PublishedAt = base.Release.PublishedAt.Add(-time.Hour) }, "age"},
		{"tie", func(*Input) {}, "tie"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := base
			c.change(&in)
			if got := LostTo(Decide(in), winner); got.Criterion != c.want || got.Message == "" {
				t.Fatalf("got %+v, want %s", got, c.want)
			}
		})
	}
}
