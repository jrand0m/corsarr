package naming

import "testing"

func TestPreview_DefaultEpisodeTemplates(t *testing.T) {
	v := Values{SeriesTitle: "Harbor Lights: Night", ReleaseYear: 2019, Season: "02", Episode: "01-E03",
		EpisodeTitle: "Fog", QualityFull: "WEBDL-1080p"}
	got, warns := Options{Extension: "mkv"}.Preview(DefaultSeriesFolder+"/"+DefaultSeasonFolder+"/"+DefaultEpisodeFile, v)
	want := "Harbor Lights - Night (2019)/Season 02/Harbor Lights - Night - S02E01-E03 - Fog [WEBDL-1080p].mkv"
	if got != want || len(warns) != 0 {
		t.Fatalf("got %q %v", got, warns)
	}
	if got, _ := Preview("{Air Date}", Values{AirDate: "2024-01-02"}); got != "2024-01-02" {
		t.Fatalf("air date %q", got)
	}
}
