package decide

import (
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/parse"
)

func musicProfile(t *testing.T, name string) QualityProfile {
	t.Helper()
	for _, p := range DefaultProfiles(domain.KindMusic) {
		if p.Spec().Name == name {
			return p
		}
	}
	t.Fatalf("no music profile %s", name)
	return QualityProfile{}
}

func TestAudioQualityOfTitles(t *testing.T) {
	cases := map[string]struct {
		id        QualityID
		confident bool
	}{
		"A - B (2020) [FLAC]":            {"flac", true},
		"A - B (2020) [FLAC 24bit]":      {"flac-24", true},
		"A - B (2020) [ALAC]":            {"alac", true},
		"A - B (2020) MP3 320kbps":       {"mp3-320", true},
		"A - B (2020) MP3 200kbps":       {"mp3-192", true},
		"A - B (2020) [MP3 V0]":          {"mp3-vbr-v0", true},
		"A - B (2020) [MP3]":             {"mp3-128", false},
		"A - B (2020) [MP3 64kbps]":      {"mp3-low", true},
		"A - B (2020) [AAC 256]":         {"aac-256", true},
		"A - B (2020) [AAC]":             {"aac-192", false},
		"A - B (2020) [Opus]":            {"opus", true},
		"A - B (2020)":                   {AudioUnknown, false},
		"A - B (2020) [24-96]":           {"flac-24", false},
		"A - B (2020) 256kbps":           {"mp3-256", true},
		"A - B (2020) [WavPack]":         {"wavpack", true},
		"A - B (2020) [AAC 320kbps]":     {"aac-320", true},
		"A - B (2020) [ALAC 24bit 96k]":  {"alac-24", true},
		"A - B (2020) [vorbis]":          {"vorbis", true},
		"A - B (2020) [MP3 112kbps]":     {"mp3-96", true},
		"A - B (2020) [MP3 V2]":          {"mp3-vbr-v2", true},
		"A - B (2020) [WMA]":             {"wma", true},
		"A - B (2020) [APE]":             {"ape", true},
		"A - B (2020) [WAV]":             {"wav", true},
		"A - B (2020) [MP3 160 kbps]":    {"mp3-160", true},
		"A - B (2020) [MP3 224kbps]":     {"mp3-224", true},
		"A - B (2020) [MP3 128kbps CBR]": {"mp3-128", true},
	}
	for title, want := range cases {
		q, ok := AudioQualityOf(parse.ParseMusic(title))
		if q.ID != want.id || ok != want.confident {
			t.Errorf("%q: %s %v, want %s %v", title, q.ID, ok, want.id, want.confident)
		}
	}
}

func TestMusicProfilesRespectTheFloorAndRankHiResAboveFLAC(t *testing.T) {
	for _, p := range DefaultProfiles(domain.KindMusic) {
		for _, low := range []QualityID{AudioUnknown, "mp3-low", "mp3-96"} {
			if _, ok := p.Rank(low); ok {
				t.Errorf("%s allows %s below the floor", p.Spec().Name, low)
			}
		}
	}
	lossless := musicProfile(t, "Lossless")
	if _, ok := lossless.Rank("mp3-320"); ok {
		t.Fatal("Lossless allows MP3-320")
	}
	hi, _ := lossless.Rank("flac-24")
	flac, _ := lossless.Rank("flac")
	if hi <= flac {
		t.Fatal("FLAC 24bit must rank above FLAC")
	}
}

func TestMusicDecisionUsesTheAudioLadderAndAlbumSizes(t *testing.T) {
	p := musicProfile(t, "Lossless")
	item := Item{Ref: domain.ItemRef{Kind: domain.KindMusic, ID: 1}, Monitored: true, Available: true, RuntimeMinutes: 40, Profile: p}
	decideTitle := func(title string, size int64) domain.Decision {
		rel := domain.Release{ID: title, Title: title, Protocol: domain.ProtoTorrent, Size: size, Seeders: 5}
		return Decide(Input{Release: rel, Parsed: parse.ParseRelease(title), Item: item})
	}
	if d := decideTitle("A - B (2020) [FLAC]", 250e6); !d.Accepted || d.Quality != "FLAC" {
		t.Fatalf("flac: %+v", d)
	}
	if d := decideTitle("A - B (2020) [MP3 320kbps]", 90e6); d.Accepted {
		t.Fatalf("mp3 accepted by Lossless: %+v", d)
	}
	// 40 min at 12 MB/min lossless max is 480 MB
	if d := decideTitle("A - B (2020) [FLAC]", 900e6); d.Accepted {
		t.Fatalf("oversized flac accepted: %+v", d)
	}
	hi, lo := decideTitle("A - B (2020) [FLAC 24bit]", 900e6), decideTitle("A - B (2020) [FLAC]", 250e6)
	if !hi.Accepted || hi.RankKey <= lo.RankKey {
		t.Fatalf("hi-res should rank above flac: %s vs %s", hi.RankKey, lo.RankKey)
	}
	if tier, ok := AudioTierOf("flac-24"); !ok || tier != AudioHiRes {
		t.Fatal("tier")
	}
	if len(AudioLadder()) == 0 || DefaultDefinitions().For("mp3-320").Max() != 3 {
		t.Fatal("audio definitions")
	}
}

func TestAACVBRRungSitsAfterAAC256(t *testing.T) {
	l := AudioLadder()
	var at int
	for i, q := range l {
		if q.ID == "aac-vbr" {
			at = i
		}
	}
	if at == 0 || l[at].Name != "AAC-VBR" || l[at-1].ID != "aac-256" {
		t.Fatalf("aac-vbr at %d", at)
	}
	if tier, _ := AudioTierOf("aac-vbr"); tier != AudioLossy || DefaultDefinitions().For("aac-vbr").Max() != 3 {
		t.Fatal("aac-vbr must be lossy with lossy bounds")
	}
	for _, p := range DefaultProfiles(domain.KindMusic)[:2] {
		if _, ok := p.Rank("aac-vbr"); !ok {
			t.Errorf("%s must allow aac-vbr", p.Spec().Name)
		}
	}
}
