package app

import (
	"context"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/plugin"
)

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
