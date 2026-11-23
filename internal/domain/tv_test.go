package domain

import (
	"testing"
	"time"
)

func TestTVParsers(t *testing.T) {
	if st, err := ParseSeriesType(""); err != nil || st != SeriesStandard {
		t.Fatalf("default series type: %v %v", st, err)
	}
	if st, err := ParseSeriesType("daily"); err != nil || st != SeriesDaily {
		t.Fatalf("daily: %v %v", st, err)
	}
	if _, err := ParseSeriesType("cartoon"); err == nil {
		t.Fatal("unknown series type accepted")
	}
	if m, err := ParseMonitorNew(""); err != nil || m != MonitorNewAll {
		t.Fatalf("default monitor new: %v %v", m, err)
	}
	if m, err := ParseMonitorNew("none"); err != nil || m != MonitorNewNone {
		t.Fatalf("none: %v %v", m, err)
	}
	if _, err := ParseMonitorNew("some"); err == nil {
		t.Fatal("unknown monitor-new accepted")
	}
	if m, err := ParseMonitorMode("latestSeason"); err != nil || m != MonitorLatestSeason {
		t.Fatalf("latestSeason: %v %v", m, err)
	}
	if m, _ := ParseMonitorMode(""); m != MonitorAll {
		t.Fatalf("default mode %v", m)
	}
	if _, err := ParseMonitorMode("most"); err == nil {
		t.Fatal("unknown mode accepted")
	}
}

func TestSeriesAndEpisodeRefs(t *testing.T) {
	s := Series{ID: 3, Title: "Show", Tags: []string{"a"}}
	if r := s.Ref(); r.Kind != KindTV || r.Variant != VariantSeries || s.EventItem().Title != "Show" {
		t.Fatalf("series ref %v", r)
	}
	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	e := Episode{ID: 9, Season: 1, Number: 2, AirAt: &at}
	it := e.EventItem("Show")
	if it.Ref != (ItemRef{Kind: KindTV, ID: 9}) || it.Title != "Show S01E02" || it.Series != "Show" {
		t.Fatalf("episode item %+v", it)
	}
}

func TestItemsTitleCollapsesEpisodesOfOneSeries(t *testing.T) {
	var items []EventItem
	for n := 1; n <= 10; n++ {
		items = append(items, Episode{ID: int64(n), Season: 2, Number: n}.EventItem("Show"))
	}
	if got := ItemsTitle(items); got != "Show · S02 · 10 episodes" {
		t.Fatalf("season pack title %q", got)
	}
	two := []EventItem{Episode{Season: 1, Number: 1}.EventItem("Show"), Episode{Season: 2, Number: 1}.EventItem("Show")}
	if got := ItemsTitle(two); got != "Show · 2 seasons · 2 episodes" {
		t.Fatalf("multi-season title %q", got)
	}
	mixed := []EventItem{{Title: "Movie"}, Episode{Season: 1, Number: 3}.EventItem("Show")}
	if got := ItemsTitle(mixed); got != "Movie, Show S01E03" {
		t.Fatalf("mixed title %q", got)
	}
}

func TestKindOf(t *testing.T) {
	if KindOf(nil, KindMovie) != KindMovie {
		t.Fatal("default kind")
	}
	if KindOf([]EventItem{{Ref: ItemRef{Kind: KindTV}}}, KindMovie) != KindTV {
		t.Fatal("tv item kind")
	}
}
