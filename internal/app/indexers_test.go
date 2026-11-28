package app

import (
	"context"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/httpx"
	"github.com/jrand0m/corsarr/internal/instances"
	"github.com/jrand0m/corsarr/internal/plugin"
)

type slowIndexer struct{ plugin.Indexer }

func (slowIndexer) Search(context.Context, domain.SearchRequest) ([]domain.Release, error) {
	return nil, plugin.Errorf(plugin.ClassRateLimited, "request limit reached")
}

func TestReportingPassesRateLimitErrorsToTheClient(t *testing.T) {
	ref := domain.IntegrationRef{Type: "indexer", Key: "api"}
	var got []domain.IntegrationRef
	r := reporting{Indexer: slowIndexer{}, ref: ref, health: health.New(nil, nil, nil),
		throttle: func(ref domain.IntegrationRef, _ time.Duration) { got = append(got, ref) }}
	_, _ = r.Search(context.Background(), domain.SearchRequest{})
	if len(got) != 1 || got[0] != ref {
		t.Fatalf("throttle reports %v", got)
	}
}

type shaperLog struct {
	limits  map[string]httpx.Limits
	probes  map[domain.IntegrationRef]httpx.ProbeMode
	caps    map[domain.IntegrationRef]float64
	declare map[domain.IntegrationRef]float64
}

func (s *shaperLog) SetLimits(h string, l httpx.Limits) { s.limits[h] = l }
func (s *shaperLog) SetProbe(r domain.IntegrationRef, m httpx.ProbeMode) {
	s.probes[r] = m
}
func (s *shaperLog) Cap(r domain.IntegrationRef, per time.Duration, n float64) {
	s.caps[r] = n / per.Hours()
}
func (s *shaperLog) Declare(r domain.IntegrationRef, per time.Duration, n float64) {
	s.declare[r] = n / per.Seconds()
}

type scrapedIndexer struct{ plugin.Indexer }

func (scrapedIndexer) MinInterval() (string, time.Duration) {
	return "tracker.example.org", 2 * time.Second
}
func (scrapedIndexer) DeclaredLimit() (time.Duration, int) { return time.Minute, 30 }

func TestShapeIndexerSetsProbeModeCapsAndDeclaredLimits(t *testing.T) {
	s := &shaperLog{limits: map[string]httpx.Limits{}, probes: map[domain.IntegrationRef]httpx.ProbeMode{},
		caps: map[domain.IntegrationRef]float64{}, declare: map[domain.IntegrationRef]float64{}}
	api := instances.Running{Key: "api", Kind: plugin.KindIndexer, Impl: slowIndexer{},
		Settings: plugin.Settings{"queryLimit": 100, "queryLimitPer": "hour"}}
	scraped := instances.Running{Key: "site", Kind: plugin.KindIndexer, Impl: scrapedIndexer{},
		Settings: plugin.Settings{"queryLimit": 48}}
	shapeIndexer(s, api)
	shapeIndexer(s, scraped)
	if s.probes[api.Ref()] != httpx.ProbeGentle || s.probes[scraped.Ref()] != httpx.ProbeOff {
		t.Fatalf("probe modes %v", s.probes)
	}
	if s.caps[api.Ref()] != 100 || s.caps[scraped.Ref()] != 2 {
		t.Fatalf("caps per hour %v", s.caps)
	}
	if s.limits["tracker.example.org"].Rate != 0.5 || s.declare[scraped.Ref()] != 0.5 {
		t.Fatalf("limits %v declared %v", s.limits, s.declare)
	}
}

type brokenIndexer struct{ plugin.Indexer }

func (brokenIndexer) Search(context.Context, domain.SearchRequest) ([]domain.Release, error) {
	return nil, plugin.Errorf(plugin.ClassParse, "results page could not be parsed")
}

func (brokenIndexer) Feed(context.Context, domain.MediaKind, string) ([]domain.Release, string, error) {
	return nil, "c", plugin.Errorf(plugin.ClassSessionExpired, "logged out")
}

func TestReportingRecordsFailuresTheClientCannotSee(t *testing.T) {
	reg := health.New(nil, nil, nil)
	ref := domain.IntegrationRef{Type: "indexer", Key: "site"}
	r := reporting{Indexer: brokenIndexer{}, ref: ref, health: reg}
	if _, err := r.Search(context.Background(), domain.SearchRequest{}); err == nil {
		t.Fatal("error swallowed")
	}
	if h, _ := reg.Get(ref); h.LastErrorClass != "parse" {
		t.Errorf("parse failure not in health: %+v", h)
	}
	if _, cur, _ := r.Feed(context.Background(), domain.KindTV, ""); cur != "c" {
		t.Error("cursor lost")
	}
	if h, _ := reg.Get(ref); h.Status != domain.HealthActionNeeded {
		t.Errorf("expired session not action needed: %+v", h)
	}
}
