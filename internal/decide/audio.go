package decide

import (
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/parse"
)

// AudioTier groups the audio ladder for size bounds: lossy, lossless 16-bit
// and hi-res 24-bit files differ by an order of magnitude per minute.
type AudioTier string

const (
	AudioUnknownTier AudioTier = "unknown"
	AudioLossy       AudioTier = "lossy"
	AudioLossless    AudioTier = "lossless"
	AudioHiRes       AudioTier = "hires"
)

// AudioUnknown is the bottom rung of the audio ladder.
const AudioUnknown QualityID = "audio-unknown"

// audioLadder lists every audio quality from worst to best; the tier of each
// rung decides its default size bounds.
var audioLadder = func() []Quality {
	rows := []struct {
		id, name string
		tier     AudioTier
	}{
		{string(AudioUnknown), "Unknown audio", AudioUnknownTier},
		{"mp3-low", "MP3-low", AudioLossy},
		{"mp3-96", "MP3-96", AudioLossy},
		{"mp3-128", "MP3-128", AudioLossy},
		{"mp3-160", "MP3-160", AudioLossy},
		{"wma", "WMA", AudioLossy},
		{"mp3-192", "MP3-192", AudioLossy},
		{"aac-192", "AAC-192", AudioLossy},
		{"vorbis", "OGG Vorbis", AudioLossy},
		{"mp3-vbr-v2", "MP3-VBR-V2", AudioLossy},
		{"mp3-224", "MP3-224", AudioLossy},
		{"mp3-256", "MP3-256", AudioLossy},
		{"aac-256", "AAC-256", AudioLossy},
		{"aac-vbr", "AAC-VBR", AudioLossy},
		{"opus", "Opus", AudioLossy},
		{"mp3-vbr-v0", "MP3-VBR-V0", AudioLossy},
		{"mp3-320", "MP3-320", AudioLossy},
		{"aac-320", "AAC-320", AudioLossy},
		{"wav", "WAV", AudioLossless},
		{"ape", "APE", AudioLossless},
		{"wavpack", "WavPack", AudioLossless},
		{"alac", "ALAC", AudioLossless},
		{"flac", "FLAC", AudioLossless},
		{"alac-24", "ALAC 24bit", AudioHiRes},
		{"flac-24", "FLAC 24bit", AudioHiRes},
	}
	out := make([]Quality, len(rows))
	for i, r := range rows {
		out[i] = Quality{ID: QualityID(r.id), Name: r.name, Rank: i}
		audioTiers[out[i].ID] = r.tier
	}
	return out
}()

var audioTiers = map[QualityID]AudioTier{}

var audioTierDefaults = map[AudioTier]QualityDefinition{
	AudioUnknownTier: {min: 0, preferred: 6, max: 60},
	AudioLossy:       {min: 0, preferred: 2, max: 3},
	AudioLossless:    {min: 1, preferred: 6, max: 12},
	AudioHiRes:       {min: 3, preferred: 25, max: 60},
}

func init() {
	for _, q := range audioLadder {
		byID[q.ID] = q
	}
}

// AudioLadder returns the audio ladder, worst first.
func AudioLadder() []Quality { return append([]Quality(nil), audioLadder...) }

// AudioTierOf is the size tier of an audio quality (ok=false for video ids).
func AudioTierOf(id QualityID) (AudioTier, bool) { t, ok := audioTiers[id]; return t, ok }

// shippedDefinition is the default size bound of any quality on either ladder.
func shippedDefinition(id QualityID) QualityDefinition {
	if t, ok := audioTiers[id]; ok {
		return audioTierDefaults[t]
	}
	return tierDefaults[byID[id].Tier]
}

var mp3Steps = []struct {
	kbps int
	id   QualityID
}{{320, "mp3-320"}, {256, "mp3-256"}, {224, "mp3-224"}, {192, "mp3-192"}, {160, "mp3-160"}, {128, "mp3-128"}, {96, "mp3-96"}}

// AudioQualityOf maps what a music title states to a rung of the audio
// ladder. A bitrate falls to the step at or below it; a format without a
// bitrate takes its most conservative rung and reports confident=false.
func AudioQualityOf(m parse.MusicRelease) (Quality, bool) {
	id, confident := audioID(m)
	return byID[id], confident
}

func audioID(m parse.MusicRelease) (QualityID, bool) {
	switch {
	case !m.Format.Known && m.BitDepth.Value == 24:
		return "flac-24", false
	case !m.Format.Known && m.Bitrate.Known:
		return mp3ID(m)
	case !m.Format.Known:
		return AudioUnknown, false
	}
	return formatID(m)
}

func formatID(m parse.MusicRelease) (QualityID, bool) {
	switch f := m.Format.Value; f {
	case parse.FormatFLAC, parse.FormatALAC:
		if m.BitDepth.Value == 24 {
			return QualityID(string(f) + "-24"), true
		}
		return QualityID(f), true
	case parse.FormatMP3:
		return mp3ID(m)
	case parse.FormatAAC:
		return aacID(m)
	case parse.FormatWavPack, parse.FormatAPE, parse.FormatWAV, parse.FormatWMA, parse.FormatVorbis, parse.FormatOpus:
		return QualityID(f), true
	}
	return AudioUnknown, false
}

func mp3ID(m parse.MusicRelease) (QualityID, bool) {
	if m.VBR.Known {
		return QualityID("mp3-vbr-" + m.VBR.Value), true
	}
	if !m.Bitrate.Known {
		return "mp3-128", false
	}
	for _, s := range mp3Steps {
		if m.Bitrate.Value >= s.kbps {
			return s.id, true
		}
	}
	return "mp3-low", true
}

func aacID(m parse.MusicRelease) (QualityID, bool) {
	switch {
	case !m.Bitrate.Known:
		return "aac-192", false
	case m.Bitrate.Value >= 320:
		return "aac-320", true
	case m.Bitrate.Value >= 256:
		return "aac-256", true
	}
	return "aac-192", true
}

// qualityFor reads the release's quality on its kind's ladder.
func qualityFor(in Input) (Quality, bool) {
	if in.Item.Ref.Kind == domain.KindMusic {
		return AudioQualityOf(parse.ParseMusic(in.Release.Title))
	}
	return QualityOf(in.Parsed)
}

// musicFloor is the lowest audio quality any shipped music profile allows.
const musicFloor QualityID = "mp3-128"

func musicSpecs() []ProfileSpec {
	var above []QualityID
	floor := byID[musicFloor].Rank
	for _, q := range audioLadder[floor:] {
		above = append(above, q.ID)
	}
	lossy := group("Lossy high", "mp3-192", "aac-192", "vorbis", "mp3-vbr-v2", "mp3-224", "mp3-256", "aac-256", "aac-vbr", "opus")
	lossless := group("Lossless", "wav", "ape", "wavpack", "alac", "flac")
	return []ProfileSpec{
		{Name: "Any", Kind: domain.KindMusic, Groups: single(above...), Cutoff: "flac", Upgrades: true},
		{Name: "Standard", Kind: domain.KindMusic, Groups: []QualityGroup{lossy, group("MP3 high", "mp3-vbr-v0", "mp3-320", "aac-320")},
			Cutoff: "mp3-320", Upgrades: true},
		{Name: "Lossless", Kind: domain.KindMusic, Groups: []QualityGroup{lossless, group("Hi-res", "alac-24", "flac-24")},
			Cutoff: "flac", Upgrades: true},
	}
}
