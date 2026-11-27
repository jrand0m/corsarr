package store

import (
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

func TestQueueFilterAndTransitions(t *testing.T) {
	s := open(t, t.TempDir())
	r := s.Queue()
	g, d := newGrab(1, "aaa")
	_, a, err := r.CreateGrab(ctx, g, d)
	must(t, err)
	g, d = newGrab(2, "bbb")
	d.Downloader, d.Kind = "sab", domain.KindTV
	_, b, err := r.CreateGrab(ctx, g, d)
	must(t, err)
	b.State = domain.StateFailed
	must(t, r.Save(ctx, b))
	for name, c := range map[string]struct {
		f    QueueFilter
		want []int64
	}{
		"default hides nothing live": {QueueFilter{}, []int64{b.ID, a.ID}},
		"by client":                  {QueueFilter{Downloader: "sab"}, []int64{b.ID}},
		"by kind":                    {QueueFilter{Kind: a.Kind}, []int64{a.ID}},
		"by state":                   {QueueFilter{States: []domain.DownloadState{domain.StateFailed, domain.StateRemoved}}, []int64{b.ID}},
		"no match":                   {QueueFilter{Kind: domain.KindBook}, nil},
	} {
		got, _, err := r.ListFiltered(ctx, c.f, 0, 10)
		must(t, err)
		ids := []int64{}
		for _, x := range got {
			ids = append(ids, x.ID)
		}
		if len(ids) != len(c.want) || (len(ids) > 0 && ids[0] != c.want[0]) {
			t.Errorf("%s: %v want %v", name, ids, c.want)
		}
	}
	if page, next, _ := r.ListFiltered(ctx, QueueFilter{}, 0, 1); len(page) != 1 || next != b.ID {
		t.Fatalf("paging: %d %d", len(page), next)
	}
	at := time.UnixMilli(1_700_000_000_000).UTC()
	must(t, r.AddTransition(ctx, a.ID, Transition{From: domain.StateQueued, To: domain.StateDownloading, Actor: domain.ActorDownloader, At: at}))
	must(t, r.AddTransition(ctx, a.ID, Transition{From: domain.StateDownloading, To: domain.StateFailed, Actor: domain.ActorUser, Reason: "r", At: at}))
	hist, err := r.Transitions(ctx, a.ID)
	if err != nil || len(hist) != 2 || hist[1].Reason != "r" || !hist[0].At.Equal(at) || hist[0].To != domain.StateDownloading {
		t.Fatalf("%+v %v", hist, err)
	}
	if none, err := r.Transitions(ctx, b.ID); err != nil || len(none) != 0 {
		t.Fatalf("%v %v", none, err)
	}
}
