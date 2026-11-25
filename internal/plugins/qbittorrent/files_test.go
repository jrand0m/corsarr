package qbittorrent

import (
	"context"
	"testing"

	"github.com/jrand0m/corsarr/internal/plugin"
)

func TestFilesListsPayloadOnceMetadataIsKnown(t *testing.T) {
	f := newFake(t, "v5.0.1")
	c := newClient(t, f, nil)
	ctx := context.Background()
	f.put(magnetHex, map[string]any{"name": "m", "state": "metaDL"})
	if got, err := c.Files(ctx, magnetHex); err != nil || len(got) != 0 {
		t.Fatalf("before metadata: %v %v", got, err)
	}
	f.put(magnetHex, map[string]any{"name": "m", "state": "downloading", "files": []map[string]any{
		{"name": "Movie/movie.mkv", "size": 900}, {"name": "Movie/movie.mkv.scr", "size": 10}}})
	got, err := c.Files(ctx, "0123456789ABCDEF0123456789ABCDEF01234567")
	if err != nil || len(got) != 2 || got[1] != (plugin.FileEntry{Path: "Movie/movie.mkv.scr", Size: 10}) {
		t.Fatalf("files: %v %v", got, err)
	}
}

func TestFilesErrors(t *testing.T) {
	f := newFake(t, "v5.0.1")
	c := newClient(t, f, nil)
	if _, err := c.Files(context.Background(), "ffff"); err == nil {
		t.Fatal("unknown hash must fail")
	}
	f.put(magnetHex, map[string]any{"files": "garbage"})
	if _, err := c.Files(context.Background(), magnetHex); plugin.ClassOf(err) != plugin.ClassParse {
		t.Fatalf("garbage: %v", err)
	}
}

func TestSnapshotCarriesContentPath(t *testing.T) {
	ti := torrentInfo{Hash: "h", State: "downloading", ContentPath: "/dl/Movie"}
	if s := (&Client{}).snapshot(ti); s.ContentPath != "/dl/Movie" || len(s.Files) != 0 {
		t.Fatalf("%+v", s)
	}
}
