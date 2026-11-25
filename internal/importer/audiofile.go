package importer

import (
	"github.com/jrand0m/corsarr/internal/audiotag"
	"github.com/jrand0m/corsarr/internal/parse"
)

// codecFormats maps header codecs to the formats of the audio ladder. MPEG
// layer 1 and 2 are absent on purpose: such files stay unknown quality so
// they never satisfy a profile, whatever their name says.
var codecFormats = map[audiotag.Codec]parse.AudioFormat{
	audiotag.CodecMP3: parse.FormatMP3, audiotag.CodecAAC: parse.FormatAAC, audiotag.CodecALAC: parse.FormatALAC,
	audiotag.CodecFLAC: parse.FormatFLAC, audiotag.CodecVorbis: parse.FormatVorbis, audiotag.CodecOpus: parse.FormatOpus,
	audiotag.CodecAPE: parse.FormatAPE, audiotag.CodecWavPack: parse.FormatWavPack, audiotag.CodecPCM: parse.FormatWAV,
}

// VBR MP3 averages at or above these rates are taken as the V0 and V2
// presets when the release title names no preset.
const (
	vbrV0Floor = 220
	vbrV2Floor = 170
)

// fileAudio reads the quality a track file really has from its stream
// headers; ok=false when the headers cannot be read.
func fileAudio(path string, title parse.MusicRelease) (parse.MusicRelease, bool) {
	t, err := audiotag.Read(path)
	if err != nil || t.Audio.Codec == "" {
		return parse.MusicRelease{}, false
	}
	return headerRelease(t.Audio, title), true
}

// headerRelease describes header audio properties as a music release so the
// ladder can rank them. The header decides codec, bitrate and bit depth;
// the title only names the VBR preset of a VBR MP3.
func headerRelease(a audiotag.AudioInfo, title parse.MusicRelease) parse.MusicRelease {
	m := parse.MusicRelease{Group: title.Group}
	f, ok := codecFormats[a.Codec]
	if !ok {
		return m
	}
	m.Format = parse.Field[parse.AudioFormat]{Value: f, Known: true}
	m.Bitrate = parse.Field[int]{Value: a.Bitrate, Known: a.Bitrate > 0}
	m.BitDepth = parse.Field[int]{Value: a.BitDepth, Known: a.BitDepth > 0}
	if f == parse.FormatMP3 && a.VBR {
		m.VBR = vbrPreset(a.Bitrate, title.VBR)
	}
	return m
}

func vbrPreset(kbps int, named parse.Field[string]) parse.Field[string] {
	switch {
	case named.Known:
		return named
	case kbps >= vbrV0Floor:
		return parse.Field[string]{Value: "v0", Known: true}
	case kbps >= vbrV2Floor:
		return parse.Field[string]{Value: "v2", Known: true}
	}
	return parse.Field[string]{}
}
