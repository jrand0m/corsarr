package torznab

import (
	"testing"
)

func TestReleaseSwarmAndPayloadAttributes(t *testing.T) {
	items, err := parseRSS([]byte(feedOf(
		item("1", "Reported", `<torznab:attr name="seeders" value="0"/><torznab:attr name="peers" value="4"/>`,
			`<torznab:attr name="files" value="3"/><torznab:attr name="minimumratio" value="1.5"/>`),
		item("2", "Silent"),
	)), "application/rss+xml")
	if err != nil {
		t.Fatal(err)
	}
	ix := &Indexer{}
	rel, _, ok := ix.release(items[0])
	if !ok || !rel.SwarmReported || rel.Peers != 4 || rel.Files != 3 || rel.MinimumRatio != 1.5 {
		t.Fatalf("reported item: %+v", rel)
	}
	rel, _, ok = ix.release(items[1])
	if !ok || rel.SwarmReported || rel.Files != 0 || rel.MinimumRatio != 0 {
		t.Fatalf("silent item: %+v", rel)
	}
}
