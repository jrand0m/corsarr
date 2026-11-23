package plugin

import "testing"

func reg(id string, kind Kind, api int) Registration {
	return Registration{
		Manifest: Manifest{ID: id, Kind: kind, APIVersion: api, Version: "1", Name: id, Delivery: "builtin"},
		New:      func(Instance) (any, error) { return nil, nil },
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(reg("torznab", KindIndexer, APIVersion)); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(reg("qbittorrent", KindDownloader, APIVersion)); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(reg("torznab", KindIndexer, APIVersion)); err == nil {
		t.Error("duplicate id must fail")
	}
	if err := r.Register(reg("future", KindIndexer, APIVersion+1)); err == nil {
		t.Error("api mismatch must fail")
	}
	if err := r.Register(Registration{Manifest: Manifest{ID: "x", Kind: KindIndexer, APIVersion: APIVersion}}); err == nil {
		t.Error("missing factory must fail")
	}
	if got := r.List(KindIndexer); len(got) != 1 || got[0].ID != "torznab" {
		t.Errorf("List(indexer) = %+v", got)
	}
	if got := r.List(""); len(got) != 2 || got[0].ID != "qbittorrent" {
		t.Errorf("List(all) sorted = %+v", got)
	}
	if _, ok := r.Get("qbittorrent"); !ok {
		t.Error("Get failed")
	}
}

func TestSection(t *testing.T) {
	want := map[Kind]string{KindIndexer: "indexers", KindDownloader: "downloadClients", KindNotifier: "notifications", KindMediaServer: "mediaServers", KindMetadata: "metadata", KindArtwork: "metadata"}
	for k, s := range want {
		if k.Section() != s {
			t.Errorf("%s → %s, want %s", k, k.Section(), s)
		}
	}
}

func TestSettingsString(t *testing.T) {
	s := Settings{"url": "http://x", "n": 3}
	if s.String("url") != "http://x" || s.String("n") != "" || s.String("missing") != "" {
		t.Fatal("Settings.String")
	}
}
