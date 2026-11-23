package domain

import "testing"

func TestParseMinAvailability(t *testing.T) {
	for in, want := range map[string]MinAvailability{"": AvailReleased, "announced": AvailAnnounced, "inCinemas": AvailInCinemas, "released": AvailReleased} {
		if got, err := ParseMinAvailability(in); err != nil || got != want {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	if _, err := ParseMinAvailability("soon"); err == nil {
		t.Error("unknown value accepted")
	}
}

func TestIdentityAndRefs(t *testing.T) {
	r := Release{Title: "T", GUID: "g", InfoHash: "h", Indexers: []IndexerRef{{Key: "ix"}}}
	if id := IdentityOf(r); id != (ReleaseIdentity{Indexer: "ix", GUID: "g", InfoHash: "h", Title: "T"}) {
		t.Fatalf("%+v", id)
	}
	if id := IdentityOf(Release{Title: "T"}); id.Indexer != "" {
		t.Fatalf("%+v", id)
	}
	m := Movie{ID: 7, Title: "M"}
	if m.Ref() != (ItemRef{Kind: KindMovie, ID: 7}) || m.EventItem().Title != "M" {
		t.Fatal("movie refs")
	}
}
