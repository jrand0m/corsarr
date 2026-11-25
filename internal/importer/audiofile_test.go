package importer

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrand0m/corsarr/internal/audiotag"
	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/parse"
)

func TestHeaderAudioQuality(t *testing.T) {
	v0 := parse.ParseMusic("Artist - Album [MP3 V0]")
	cases := []struct {
		name  string
		info  audiotag.AudioInfo
		title parse.MusicRelease
		want  decide.QualityID
	}{
		{"cbr 320", audiotag.AudioInfo{Codec: audiotag.CodecMP3, Bitrate: 320}, v0, "mp3-320"},
		{"cbr 192", audiotag.AudioInfo{Codec: audiotag.CodecMP3, Bitrate: 192}, parse.MusicRelease{}, "mp3-192"},
		{"vbr high average", audiotag.AudioInfo{Codec: audiotag.CodecMP3, Bitrate: 241, VBR: true}, parse.MusicRelease{}, "mp3-vbr-v0"},
		{"vbr mid average", audiotag.AudioInfo{Codec: audiotag.CodecMP3, Bitrate: 188, VBR: true}, parse.MusicRelease{}, "mp3-vbr-v2"},
		{"vbr preset from title", audiotag.AudioInfo{Codec: audiotag.CodecMP3, Bitrate: 200, VBR: true}, v0, "mp3-vbr-v0"},
		{"vbr low average", audiotag.AudioInfo{Codec: audiotag.CodecMP3, Bitrate: 131, VBR: true}, parse.MusicRelease{}, "mp3-128"},
		{"flac 24 despite title", audiotag.AudioInfo{Codec: audiotag.CodecFLAC, BitDepth: 24}, parse.ParseMusic("A - B [FLAC 16bit]"), "flac-24"},
		{"flac 16 despite title", audiotag.AudioInfo{Codec: audiotag.CodecFLAC, BitDepth: 16}, parse.ParseMusic("A - B [FLAC 24bit]"), "flac"},
		{"mp3 named flac", audiotag.AudioInfo{Codec: audiotag.CodecMP3, Bitrate: 128}, parse.ParseMusic("A - B [FLAC]"), "mp3-128"},
		{"alac 24", audiotag.AudioInfo{Codec: audiotag.CodecALAC, BitDepth: 24}, parse.MusicRelease{}, "alac-24"},
		{"aac 256", audiotag.AudioInfo{Codec: audiotag.CodecAAC, Bitrate: 256}, parse.MusicRelease{}, "aac-256"},
		{"opus", audiotag.AudioInfo{Codec: audiotag.CodecOpus, Bitrate: 160, VBR: true}, parse.MusicRelease{}, "opus"},
		{"mpeg layer 2 is unknown", audiotag.AudioInfo{Codec: audiotag.CodecMP2, Bitrate: 256}, parse.ParseMusic("A - B [MP3 320]"), decide.AudioUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q, _ := decide.AudioQualityOf(headerRelease(c.info, c.title))
			if q.ID != c.want {
				t.Fatalf("got %s, want %s", q.ID, c.want)
			}
		})
	}
}

// flacHeader is a minimal FLAC stream: marker and a STREAMINFO block.
func flacHeader(depth int) []byte {
	info := make([]byte, 34)
	rate := 44100
	info[10], info[11] = byte(rate>>12), byte(rate>>4)
	info[12] = byte(rate<<4) | 1<<1 | byte((depth-1)>>4)
	info[13] = byte((depth-1)&0xf) << 4
	binary.BigEndian.PutUint32(info[14:], 44100*60)
	return append([]byte("fLaC\x80\x00\x00\x22"), info...)
}

func TestAudioCandidatePrefersFileHeaders(t *testing.T) {
	dir := t.TempDir()
	hi := filepath.Join(dir, "01.flac")
	if err := os.WriteFile(hi, flacHeader(24), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(dir, "02.flac")
	if err := os.WriteFile(fake, []byte("not audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := decide.QualityProfile{}
	if c := audioCandidate("Artist - Album [FLAC]", hi, p); c.quality.ID != "flac-24" {
		t.Fatalf("header ignored: %s", c.quality.ID)
	}
	// Unreadable files fall back to the title, then to the extension.
	if c := audioCandidate("Artist - Album [MP3 320]", fake, p); c.quality.ID != "mp3-320" {
		t.Fatalf("title fallback: %s", c.quality.ID)
	}
	if c := audioCandidate("Artist - Album", fake, p); c.quality.ID != "flac" {
		t.Fatalf("extension fallback: %s", c.quality.ID)
	}
}
