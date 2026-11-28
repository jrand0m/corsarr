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

func TestReleaseFlagsFromVolumeFactors(t *testing.T) {
	cases := map[string][]string{
		`<torznab:attr name="downloadvolumefactor" value="0"/>`:                                                      {"freeleech"},
		`<torznab:attr name="downloadvolumefactor" value="0.5"/><torznab:attr name="uploadvolumefactor" value="2"/>`: {"halfleech", "doubleupload"},
		`<torznab:attr name="downloadvolumefactor" value="0.25"/>`:                                                   {"freeleech75"},
		`<torznab:attr name="downloadvolumefactor" value="1"/><torznab:attr name="uploadvolumefactor" value="1"/>`:   nil,
	}
	for attrs, want := range cases {
		items, err := parseRSS([]byte(feedOf(item("1", "Flags", attrs))), "application/rss+xml")
		if err != nil {
			t.Fatal(err)
		}
		rel, _, _ := (&Indexer{}).release(items[0])
		if len(rel.Flags) != len(want) || (len(want) > 0 && (rel.Flags[0] != want[0] || rel.Flags[len(want)-1] != want[len(want)-1])) {
			t.Errorf("%s: flags %v", attrs, rel.Flags)
		}
	}
}
