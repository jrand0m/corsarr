package queue

import (
	"errors"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
)

func TestGrabToChosenDownloader(t *testing.T) {
	e := newEnv(t)
	other := newDL()
	e.svc.d.Plugins.(*plugins).dls = append(e.svc.d.Plugins.(*plugins).dls, Downloader{Key: "qb2", Impl: other})
	req := GrabRequest{Kind: domain.KindMovie, Items: []domain.EventItem{movie}, Release: release("aa"),
		Decision: domain.Decision{Accepted: true}, Origin: domain.OriginInteractive, Downloader: "qb2"}
	g, err := e.svc.Grab(ctx, req)
	if err != nil || g.Downloader != "qb2" || len(other.snaps) != 1 || len(e.dl.snaps) != 0 {
		t.Fatalf("grab %+v err %v", g, err)
	}
	req.Downloader, req.Release = "nope", release("bb")
	if _, err := e.svc.Grab(ctx, req); !errors.Is(err, ErrNoDownloader) {
		t.Fatalf("an unknown client is refused: %v", err)
	}
}

func TestAddURL(t *testing.T) {
	e := newEnv(t)
	g, err := e.svc.AddURL(ctx, URLRequest{Kind: domain.KindMovie, Items: []domain.EventItem{movie}, URL: "magnet:?xt=urn:btih:cc&dn=Example.2021"})
	if err != nil || g.Downloader != "qb" || g.Release.Title != "Example.2021" || g.Release.Protocol != domain.ProtoTorrent {
		t.Fatalf("grab %+v err %v", g, err)
	}
	list, _ := e.st.Queue().Downloads(ctx, domain.StateQueued)
	if len(list) != 1 || list[0].Links[0].Item != movie.Ref || list[0].Title != "Example.2021" {
		t.Fatalf("the download is tracked for the item: %+v", list)
	}
	if len(e.rec.Of(domain.EvGrabSent)) != 1 {
		t.Fatal("grab.sent is published")
	}
}

func TestAddURLModesAndErrors(t *testing.T) {
	e := newEnv(t)
	req := URLRequest{Kind: domain.KindMovie, Items: []domain.EventItem{movie}, URL: "magnet:?xt=urn:btih:dd"}
	e.svc.d.Modes = func(domain.MediaKind) domain.KindMode { return domain.ModeShadow }
	if _, err := e.svc.AddURL(ctx, req); !errors.Is(err, ErrShadowed) || len(e.dl.snaps) != 0 || len(e.rec.Of(domain.EvClientShadowed)) != 1 {
		t.Fatalf("shadow records instead of sending: %v", err)
	}
	e.svc.d.Modes = func(domain.MediaKind) domain.KindMode { return domain.ModeDisabled }
	var me domain.ModeError
	if _, err := e.svc.AddURL(ctx, req); !errors.As(err, &me) {
		t.Fatalf("disabled refuses: %v", err)
	}
	for _, bad := range []string{"ftp://x/a.torrent", "https://example.org/file", "not a url"} {
		if _, err := e.svc.AddURL(ctx, URLRequest{Kind: domain.KindMovie, URL: bad}); !errors.Is(err, ErrBadURL) {
			t.Errorf("%s: %v", bad, err)
		}
	}
}

func TestURLKinds(t *testing.T) {
	cases := []struct {
		url   string
		proto domain.Protocol
		kind  string
		name  string
		want  domain.Protocol
	}{
		{"https://example.org/x/Show.S01.torrent?key=1", "", "torrent", "Show.S01", domain.ProtoTorrent},
		{"https://example.org/get/Film.NZB", "", "nzb", "Film", domain.ProtoUsenet},
		{"https://example.org/get?id=5", domain.ProtoUsenet, "nzb", "get", domain.ProtoUsenet},
		{"magnet:?xt=urn:btih:ee", "", "magnet", "ee", domain.ProtoTorrent},
		{"magnet:?%zz", "", "magnet", "magnet", domain.ProtoTorrent},
	}
	for _, c := range cases {
		kind, proto, name, err := urlKind(c.url, c.proto)
		if err != nil || kind != c.kind || proto != c.want || name != c.name {
			t.Errorf("%s: %s %s %s %v", c.url, kind, proto, name, err)
		}
	}
}
