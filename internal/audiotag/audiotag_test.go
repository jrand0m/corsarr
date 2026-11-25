package audiotag

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func readBytes(t *testing.T, b []byte) Tags {
	t.Helper()
	tags, err := ReadFrom(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return tags
}

func near(got, want time.Duration) bool {
	d := got - want
	return d > -5*time.Millisecond && d < 5*time.Millisecond
}

func TestID3v24WithVBRStream(t *testing.T) {
	tag := id3(4, 0,
		frame(4, "TIT2", txt(3, "Opening")),
		frame(4, "TPE1", txt(3, "Example Artist", "Guest")),
		frame(4, "TPE2", txt(1, "Example Artist")),
		frame(4, "TALB", txt(0, "First Album")),
		frame(4, "TRCK", txt(3, "3/12")),
		frame(4, "TPOS", txt(3, "1/2")),
		frame(4, "TDRC", txt(3, "2020-05-06T10:00")),
		frame(4, "TDOR", txt(3, "1999")),
		frame(4, "TPUB", txt(3, "Example Label")),
		frame(4, "TXXX", txt(3, "MusicBrainz Album Id", "rel-1")),
		frame(4, "TXXX", txt(1, "MusicBrainz Release Group Id", "rg-1")),
		frame(4, "TXXX", txt(3, "MusicBrainz Artist Id", "ar-1/ar-2")),
		frame(4, "TXXX", txt(3, "MusicBrainz Album Artist Id", "aa-1")),
		frame(4, "TXXX", txt(3, "MusicBrainz Release Track Id", "trk-1")),
		frame(4, "TXXX", txt(3, "MusicBrainz Album Type", "album")),
		frame(4, "TXXX", txt(3, "MusicBrainz Album Status", "official")),
		frame(4, "UFID", cat([]byte(musicBrainzOwner), []byte{0}, []byte("rec-1"))),
		frame(4, "UFID", cat([]byte("other"), []byte{0}, []byte("x"))),
		frame(4, "TLEN", txt(3, "999000")),
	)
	file := cat(tag, mpegFrames(mpeg1L3, 417, 100, xingAt("Xing", 3, 100, 100*417)))
	got := readBytes(t, file)
	want := Tags{Title: "Opening", Artist: "Example Artist", AlbumArtist: "Example Artist", Album: "First Album",
		Track: 3, TrackTotal: 12, Disc: 1, DiscTotal: 2, Date: "2020-05-06", Year: 2020, OriginalDate: "1999",
		OriginalYear: 1999, Label: "Example Label", ReleaseStatus: "official", ReleaseType: "album",
		MusicBrainz: MBIDs{Album: "rel-1", ReleaseGroup: "rg-1", Artist: "ar-1", AlbumArtist: "aa-1", Recording: "rec-1", Track: "trk-1"}}
	want.Audio, want.Duration = got.Audio, got.Duration
	if got != want {
		t.Fatalf("tags\n got %+v\nwant %+v", got, want)
	}
	a := got.Audio
	dur := seconds(100*1152, 44100)
	if a.Codec != CodecMP3 || !a.VBR || a.SampleRate != 44100 || a.Channels != 2 || !near(a.Duration, dur) || !near(got.Duration, dur) {
		t.Fatalf("audio %+v", a)
	}
	if a.Bitrate != 127 {
		t.Fatalf("vbr average bitrate %d", a.Bitrate)
	}
}

func TestID3v23SplitDateFrames(t *testing.T) {
	cases := []struct {
		name   string
		frames [][]byte
		date   string
	}{
		{"year and day-month", [][]byte{frame(3, "TYER", txt(0, "2004")), frame(3, "TDAT", txt(0, "0605"))}, "2004-05-06"},
		{"year only", [][]byte{frame(3, "TYER", txt(0, "2004"))}, "2004"},
		{"day-month not numeric", [][]byte{frame(3, "TYER", txt(0, "2004")), frame(3, "TDAT", txt(0, "xxxx"))}, "2004"},
		{"full date frame wins", [][]byte{frame(3, "TDRC", txt(0, "2001-02-03")), frame(3, "TYER", txt(0, "2004"))}, "2001-02-03"},
		{"no date", [][]byte{frame(3, "TIT2", txt(0, "x"))}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := readBytes(t, cat(id3(3, 0, c.frames...), mpegFrames(mpeg1L3, 417, 3, nil)))
			if got.Date != c.date {
				t.Fatalf("date %q, want %q", got.Date, c.date)
			}
		})
	}
}

func TestID3v23OriginalYearAndEncodings(t *testing.T) {
	tag := id3(3, 0,
		frame(3, "TORY", txt(0, "1975")),
		frame(3, "TIT2", txt(1, "Café")),
		frame(3, "TPE1", txt(2, "Big Endian")),
		frame(3, "TALB", txt(0, "Café Album")),
		frame(3, "TPE2", txt(3, "Skipped"), 0x80), // compressed
		frame(3, "TXXX", txt(0, "LABEL")),         // description without value
	)
	got := readBytes(t, cat(tag, mpegFrames(mpeg1L3, 417, 3, nil)))
	if got.OriginalYear != 1975 || got.Title != "Café" || got.Artist != "Big Endian" || got.Album != "Café Album" || got.AlbumArtist != "" {
		t.Fatalf("%+v", got)
	}
}

func TestID3UnsyncAndExtendedHeader(t *testing.T) {
	// 2.3 whole-tag unsynchronisation and an extended header.
	body := cat(u32be(6), make([]byte, 6), frame(3, "TIT2", txt(0, "ÿTitle")))
	body = bytes.ReplaceAll(body, []byte{0xff}, []byte{0xff, 0x00})
	tag := cat([]byte("ID3"), []byte{3, 0, 0xc0}, syncsafeBytes(len(body)), body)
	got := readBytes(t, cat(tag, mpegFrames(mpeg1L3, 417, 3, nil)))
	if got.Title != "ÿTitle" {
		t.Fatalf("v2.3 unsync title %q", got.Title)
	}

	// 2.4 extended header (counted in its own size), per-frame
	// unsynchronisation with a data length indicator, an encrypted frame and
	// a footer.
	fbody := cat(u32be(0), bytes.ReplaceAll(txt(0, "ÿT"), []byte{0xff}, []byte{0xff, 0x00}))
	frames := cat(syncsafeBytes(6), []byte{1, 0},
		frame(4, "TIT2", fbody, 0x03), frame(4, "TPE1", txt(3, "hidden"), 0x04), frame(4, "TALB", txt(3, "Album")))
	tag = cat([]byte("ID3"), []byte{4, 0, 0x50}, syncsafeBytes(len(frames)), frames, []byte("3DI"), make([]byte, 7))
	got = readBytes(t, cat(tag, mpegFrames(mpeg1L3, 417, 3, nil)))
	if got.Title != "ÿT" || got.Artist != "" || got.Album != "Album" {
		t.Fatalf("v2.4 flags %+v", got)
	}
}

func TestID3v22Frames(t *testing.T) {
	tag := id3(2, 0,
		frame(2, "TT2", txt(0, "Old Title")),
		frame(2, "TP1", txt(0, "Old Artist")),
		frame(2, "TRK", txt(0, "7")),
		frame(2, "TYE", txt(0, "1988")),
		frame(2, "TDA", txt(0, "3112")),
		frame(2, "TXX", txt(0, "MusicBrainz Album Id", "rel-22")),
		frame(2, "UFI", cat([]byte(musicBrainzOwner), []byte{0}, []byte("rec-22"))),
		frame(2, "TLE", txt(0, "61000")),
	)
	got, err := ReadFrom(bytes.NewReader(tag), int64(len(tag)))
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("tag without audio: %v", err)
	}
	if got.Title != "Old Title" || got.Track != 7 || got.Date != "1988-12-31" || got.MusicBrainz.Album != "rel-22" ||
		got.MusicBrainz.Recording != "rec-22" || got.Duration != 61*time.Second {
		t.Fatalf("%+v", got)
	}
}

func TestMPEGConstantBitrate(t *testing.T) {
	stream := mpegFrames(mpeg1L3, 417, 50, nil)
	got := readBytes(t, cat([]byte("junk"), stream))
	want := time.Duration(float64(len(stream)*8) / 128000 * float64(time.Second))
	if a := got.Audio; a.Codec != CodecMP3 || a.VBR || a.Bitrate != 128 || !near(a.Duration, want) {
		t.Fatalf("%+v want %v", a, want)
	}

	// An Info header marks a CBR stream and gives the exact frame count.
	got = readBytes(t, mpegFrames(mpeg1L3, 417, 20, xingAt("Info", 1, 20, 0)))
	if a := got.Audio; a.VBR || a.Bitrate != 128 || !near(a.Duration, seconds(20*1152, 44100)) {
		t.Fatalf("info %+v", a)
	}

	// A Xing header without a frame count is ignored.
	got = readBytes(t, mpegFrames(mpeg1L3, 417, 4, xingAt("Xing", 2, 0, 9)))
	if got.Audio.VBR || got.Audio.Bitrate != 128 {
		t.Fatalf("frameless xing %+v", got.Audio)
	}
}

func TestMPEGVBRIHeader(t *testing.T) {
	vbri := cat(make([]byte, 32), []byte("VBRI"), u16be(1), u16be(0), u16be(75), u32be(10*417), u32be(10))
	got := readBytes(t, mpegFrames(mpeg1L3, 417, 10, vbri))
	if a := got.Audio; !a.VBR || a.Bitrate != 127 || !near(a.Duration, seconds(10*1152, 44100)) {
		t.Fatalf("%+v", a)
	}
}

func TestMPEGVBRWithoutByteCount(t *testing.T) {
	got := readBytes(t, mpegFrames(mpeg1L3, 417, 10, xingAt("Xing", 1, 10, 0)))
	if a := got.Audio; !a.VBR || a.Bitrate < 125 || a.Bitrate > 130 {
		t.Fatalf("%+v", a)
	}
}

func TestMPEGLayersAndVersions(t *testing.T) {
	cases := []struct {
		name    string
		header  []byte
		codec   Codec
		kbps    int
		rate    int
		samples int
		mono    bool
	}{
		{"mpeg1 layer2", []byte{0xff, 0xfd, 0xa0, 0x00}, CodecMP2, 192, 44100, 1152, false},
		{"mpeg1 layer1", []byte{0xff, 0xff, 0x40, 0x00}, CodecMP1, 128, 44100, 384, false},
		{"mpeg2 layer3 mono", []byte{0xff, 0xf3, 0x84, 0xc0}, CodecMP3, 64, 24000, 576, true},
		{"mpeg2.5 layer3", []byte{0xff, 0xe3, 0x40, 0x00}, CodecMP3, 32, 11025, 576, false},
		{"mpeg2 layer1", []byte{0xff, 0xf7, 0x20, 0x00}, CodecMP1, 48, 22050, 384, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, ok := parseMPEG(c.header)
			if !ok || f.codec() != c.codec || f.kbps != c.kbps || f.rate != c.rate || f.samples() != c.samples || f.mono != c.mono {
				t.Fatalf("%+v ok=%v", f, ok)
			}
			got := readBytes(t, mpegFrames(c.header, f.length(), 4, nil))
			if got.Audio.Codec != c.codec || got.Audio.Bitrate != c.kbps {
				t.Fatalf("%+v", got.Audio)
			}
		})
	}
	// A mono MPEG-2 layer 3 frame carries its Xing header after 9 bytes of
	// side info.
	if (mpegFrame{version: 2, layer: 3, mono: true}).xingOffset() != 13 || (mpegFrame{version: 1, mono: true}).xingOffset() != 21 {
		t.Fatal("xing offsets")
	}
}

func TestMPEGRejectsInvalidHeaders(t *testing.T) {
	for _, h := range [][]byte{
		{0xff, 0xfb, 0x00, 0x00}, // free-format bitrate
		{0xff, 0xfb, 0xf0, 0x00}, // bad bitrate index
		{0xff, 0xfb, 0x9c, 0x00}, // reserved sample rate
		{0xff, 0xf9, 0x90, 0x00}, // reserved layer
		{0xff, 0xeb, 0x90, 0x00}, // reserved version
		{0xfe, 0xfb, 0x90, 0x00}, // no sync
		{0xff, 0xfb},
	} {
		if _, ok := parseMPEG(h); ok {
			t.Fatalf("% x accepted", h)
		}
	}
	// A stray sync pattern not followed by a second frame is skipped.
	b := cat([]byte{0xff, 0xfb, 0x90, 0x64}, make([]byte, 500), mpegFrames(mpeg1L3, 417, 3, nil))
	if i, _, ok := findFrame(b); !ok || i != 504 {
		t.Fatalf("frame at %d ok=%v", i, ok)
	}
}

func TestFLAC(t *testing.T) {
	file := cat(
		id3(4, 0, frame(4, "TIT2", txt(3, "From ID3"))),
		[]byte("fLaC"),
		flacBlockBytes(0, false, streamInfoBody(96000, 2, 24, 96000*30)),
		flacBlockBytes(6, false, make([]byte, 4000)), // picture, never loaded
		flacBlockBytes(4, false, comments(
			"TITLE=Ignored, ID3 came first", "ARTIST=Example Artist", "ALBUMARTIST=Example Artist", "ALBUM=Hi-Res",
			"TRACKNUMBER=2", "TRACKTOTAL=9", "DISCNUMBER=1", "TOTALDISCS=1", "DATE=2021", "ORIGINALDATE=2019-03",
			"ORGANIZATION=Org Label", "LABEL=Second Label", "RELEASETYPE=album", "MUSICBRAINZ_ALBUMSTATUS=official",
			"MUSICBRAINZ_ALBUMID=rel-2", "MUSICBRAINZ_RELEASEGROUPID=rg-2", "MUSICBRAINZ_ARTISTID=ar-2",
			"MUSICBRAINZ_ALBUMARTISTID=aa-2", "MUSICBRAINZ_TRACKID=rec-2", "MUSICBRAINZ_RELEASETRACKID=trk-2",
			"NOEQUALSIGN", "UNKNOWN=x")),
		flacBlockBytes(1, true, make([]byte, 10)),
		make([]byte, 200000),
	)
	got := readBytes(t, file)
	want := Tags{Title: "From ID3", Artist: "Example Artist", AlbumArtist: "Example Artist", Album: "Hi-Res",
		Track: 2, TrackTotal: 9, Disc: 1, DiscTotal: 1, Date: "2021", Year: 2021, OriginalDate: "2019-03", OriginalYear: 2019,
		Label: "Org Label", ReleaseStatus: "official", ReleaseType: "album",
		MusicBrainz: MBIDs{Album: "rel-2", ReleaseGroup: "rg-2", Artist: "ar-2", AlbumArtist: "aa-2", Recording: "rec-2", Track: "trk-2"},
		Duration:    30 * time.Second,
		Audio:       AudioInfo{Codec: CodecFLAC, SampleRate: 96000, Channels: 2, BitDepth: 24, Duration: 30 * time.Second},
	}
	if got.Audio.Bitrate == 0 {
		t.Fatal("no lossless average bitrate")
	}
	want.Audio.Bitrate = got.Audio.Bitrate
	if got != want {
		t.Fatalf("\n got %+v\nwant %+v", got, want)
	}
}

func TestFLACMalformed(t *testing.T) {
	for name, b := range map[string][]byte{
		"truncated blocks": cat([]byte("fLaC"), flacBlockBytes(4, false, comments())),
		"no stream info":   cat([]byte("fLaC"), flacBlockBytes(4, true, comments())),
		"short info":       cat([]byte("fLaC"), flacBlockBytes(0, true, make([]byte, 8))),
	} {
		if _, err := ReadFrom(bytes.NewReader(b), int64(len(b))); !errors.Is(err, ErrMalformed) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestOggVorbis(t *testing.T) {
	ident := cat([]byte("\x01vorbis"), u32le(0), []byte{2}, u32le(44100), u32le(0), u32le(192000), u32le(0), []byte{0xb8, 1})
	comment := cat([]byte("\x03vorbis"), comments("TITLE=Ogg Title", longComment(), "ARTIST=Ogg Artist"), []byte{1})
	// The comment packet is split over two pages.
	segs := lacing(comment)
	file := cat(oggPacketsPage(0, ident),
		oggPage(0, segs[:1], comment[:255]),
		oggPage(0, segs[1:], comment[255:]),
		oggPage(44100*5, []byte{10}, make([]byte, 10)))
	file = append(file, bytes.Repeat([]byte{'x'}, 300)...) // no OggS after the last page
	got := readBytes(t, file)
	if got.Title != "Ogg Title" || got.Artist != "Ogg Artist" {
		t.Fatalf("%+v", got)
	}
	if a := got.Audio; a.Codec != CodecVorbis || !a.VBR || a.Bitrate != 192 || a.SampleRate != 44100 || a.Channels != 2 || a.Duration != 5*time.Second {
		t.Fatalf("%+v", a)
	}
}

func longComment() string {
	return "COMMENT=" + string(bytes.Repeat([]byte{'z'}, 300))
}

func TestOggOpusEstimatesBitrate(t *testing.T) {
	ident := cat([]byte("OpusHead"), []byte{1, 2}, u16le(312), u32le(44100), u16le(0), []byte{0})
	tags := cat([]byte("OpusTags"), comments("TITLE=Opus Title", longComment(), "MUSICBRAINZ_TRACKID=rec-o"))
	payload := make([]byte, 20000)
	file := cat(oggPacketsPage(0, ident), oggPacketsPage(0, tags), oggPage(48000*2+312, lacing(payload), payload))
	got := readBytes(t, file)
	if got.Title != "Opus Title" || got.MusicBrainz.Recording != "rec-o" {
		t.Fatalf("%+v", got)
	}
	if a := got.Audio; a.Codec != CodecOpus || !a.VBR || a.Duration != 2*time.Second || a.SampleRate != 44100 || a.Bitrate != len(file)*8/2/1000 {
		t.Fatalf("%+v", a)
	}
}

func TestOggFLAC(t *testing.T) {
	ident := cat([]byte("\x7fFLAC"), []byte{1, 0}, u16be(1), []byte("fLaC"), flacBlockBytes(0, false, streamInfoBody(48000, 2, 16, 0)))
	vc := flacBlockBytes(4, true, comments("ALBUM=Ogg FLAC"))
	file := cat(oggPacketsPage(0, ident, vc), oggPage(48000*3, []byte{1}, []byte{0}))
	got := readBytes(t, file)
	if got.Album != "Ogg FLAC" || got.Audio.Codec != CodecFLAC || got.Audio.BitDepth != 16 || got.Audio.Duration != 3*time.Second {
		t.Fatalf("%+v", got)
	}
}

func TestOggUnknownCodecAndEmpty(t *testing.T) {
	b := oggPacketsPage(0, []byte("\x80theora"))
	if _, err := ReadFrom(bytes.NewReader(b), int64(len(b))); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("theora: %v", err)
	}
	b = []byte("OggS but not a page")
	if _, err := ReadFrom(bytes.NewReader(b), int64(len(b))); !errors.Is(err, ErrMalformed) {
		t.Fatalf("broken: %v", err)
	}
}

func mp4File(entry []byte, meta []byte, moovFirst bool) []byte {
	moov := box("moov", mvhd(1000, 4500), box("trak", box("tkhd")), trak(entry), box("udta", meta))
	ftyp := box("ftyp", []byte("M4A "), u32be(0))
	mdat := cat(u32be(1), []byte("mdat"), u64be(16+2000), make([]byte, 2000))
	if moovFirst {
		return cat(ftyp, moov, mdat)
	}
	return cat(ftyp, mdat, moov)
}

func ilst(items ...[]byte) []byte { return box("ilst", items...) }

func freeform(name, value string) []byte {
	return box("----", fullBox("mean", []byte("com.apple.iTunes")), fullBox("name", []byte(name)), dataBox([]byte(value)))
}

func TestMP4AAC(t *testing.T) {
	meta := fullBox("meta", fullBox("hdlr", make([]byte, 20)), ilst(
		box("\xa9nam", dataBox([]byte("M4A Title"))),
		box("\xa9ART", dataBox([]byte("M4A Artist"))),
		box("aART", dataBox([]byte("M4A Album Artist"))),
		box("\xa9alb", dataBox([]byte("M4A Album"))),
		box("\xa9day", dataBox([]byte("2018-07-01T00:00:00Z"))),
		box("trkn", dataBox(cat(u16be(0), u16be(4), u16be(11), u16be(0)))),
		box("disk", dataBox(cat(u16be(0), u16be(2), u16be(2)))),
		box("covr", dataBox([]byte("img"))),
		freeform("MusicBrainz Album Id", "rel-4"),
		freeform("MusicBrainz Track Id", "rec-4"),
		freeform("MusicBrainz Release Track Id", "trk-4"),
		freeform("LABEL", "M4A Label"),
	))
	for _, first := range []bool{true, false} {
		got := readBytes(t, mp4File(soundEntry("mp4a", 2, 16, 44100, esds(256000)), meta, first))
		want := Tags{Title: "M4A Title", Artist: "M4A Artist", AlbumArtist: "M4A Album Artist", Album: "M4A Album",
			Date: "2018-07-01", Year: 2018, Track: 4, TrackTotal: 11, Disc: 2, DiscTotal: 2, Label: "M4A Label",
			MusicBrainz: MBIDs{Album: "rel-4", Recording: "rec-4", Track: "trk-4"}, Duration: 4500 * time.Millisecond,
			Audio: AudioInfo{Codec: CodecAAC, Bitrate: 256, SampleRate: 44100, Channels: 2, Duration: 4500 * time.Millisecond}}
		if got != want {
			t.Fatalf("moov first=%v\n got %+v\nwant %+v", first, got, want)
		}
	}
}

func TestMP4ALACAndQuickTimeMeta(t *testing.T) {
	cookie := fullBox("alac", u32be(4096), []byte{0, 24, 40, 10, 14, 2}, u16be(255), u32be(0), u32be(0), u32be(96000))
	entry := soundEntry("alac", 2, 16, 0, cookie)
	meta := box("meta", fullBox("hdlr", make([]byte, 20)), ilst(box("\xa9nam", dataBox([]byte("Lossless")))))
	got := readBytes(t, mp4File(entry, meta, true))
	if a := got.Audio; a.Codec != CodecALAC || a.BitDepth != 24 || a.SampleRate != 96000 || a.Channels != 2 || a.Bitrate == 0 {
		t.Fatalf("%+v", a)
	}
	if got.Title != "Lossless" {
		t.Fatalf("quicktime meta: %+v", got)
	}
}

func TestMP4VersionedHeaders(t *testing.T) {
	// A version 1 movie header and a QuickTime version 1 sound description
	// (16 extra bytes before the children).
	mv := fullBox("mvhd", u64be(0), u64be(0), u32be(10), u64be(25), make([]byte, 80))
	mv[8] = 1
	entry := box("mp4a", make([]byte, 6), u16be(1), u16be(1), make([]byte, 6), u16be(1), u16be(16), u32be(0),
		u32be(22050<<16), make([]byte, 16), esds(64000))
	file := cat(box("ftyp", []byte("M4A ")), box("moov", mv, trak(entry)))
	got := readBytes(t, file)
	if a := got.Audio; a.Codec != CodecAAC || a.Duration != 2500*time.Millisecond || a.Bitrate != 64 || a.SampleRate != 22050 || a.Channels != 1 {
		t.Fatalf("%+v", a)
	}
}

func TestMP4Errors(t *testing.T) {
	video := box("trak", box("mdia", box("minf", box("stbl", fullBox("stsd", u32be(1), box("avc1", make([]byte, 40)))))))
	for name, c := range map[string]struct {
		b   []byte
		err error
	}{
		"no moov":    {cat(box("ftyp", []byte("M4A ")), box("free")), ErrMalformed},
		"bad size":   {cat(box("ftyp", []byte("M4A ")), u32be(3), []byte("moov")), ErrMalformed},
		"video only": {cat(box("ftyp", []byte("M4A ")), box("moov", video)), ErrUnsupported},
		"open box":   {cat(box("ftyp", []byte("M4A ")), u32be(0), []byte("free")), ErrMalformed},
	} {
		if _, err := ReadFrom(bytes.NewReader(c.b), int64(len(c.b))); !errors.Is(err, c.err) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// An ES descriptor with optional fields, and one of the wrong kind.
	es := cat(u32be(0), []byte{3, 20, 0, 1, 0xe0, 0, 0, 2, 'a', 'b', 0, 0, 4, 13, 0x40, 0x15, 0, 0, 0}, u32be(0), u32be(96000))
	if esdsBitrate(es) != 96000 || esdsBitrate(cat(u32be(0), []byte{5, 1})) != 0 || esdsBitrate(cat(u32be(0), []byte{3, 3, 0, 0, 0, 5})) != 0 {
		t.Fatal("esds")
	}
}

func TestAPETagFillsGaps(t *testing.T) {
	ape := apeTag(
		apeItem(0, "Title", "APE Title"),
		apeItem(0, "Album Artist", "APE Album Artist"),
		apeItem(0, "Track", "5/10"),
		apeItem(0, "Year", "1999"),
		apeItem(2, "Cover Art (Front)", "binary"),
		apeItem(0, "MUSICBRAINZ_ALBUMID", "rel-a"),
	)
	stream := mpegFrames(mpeg1L3, 417, 3, nil)
	file := cat(id3(4, 0, frame(4, "TIT2", txt(3, "ID3 Title"))), stream, ape, []byte("TAG"), make([]byte, 125))
	got := readBytes(t, file)
	if got.Title != "ID3 Title" || got.AlbumArtist != "APE Album Artist" || got.Track != 5 || got.TrackTotal != 10 ||
		got.Year != 1999 || got.MusicBrainz.Album != "rel-a" {
		t.Fatalf("%+v", got)
	}
	// Items running past the tag are dropped.
	broken := apeTag(apeItem(0, "Title", "x"))
	broken[0] = 200
	tags := Tags{}
	readAPEItems(broken[:len(broken)-32], 1, &tags)
	readAPEItems([]byte{1, 0, 0, 0, 0, 0, 0, 0, 'k'}, 1, &tags)
	if tags.Title != "" {
		t.Fatalf("%+v", tags)
	}
}

func TestMonkeysAudio(t *testing.T) {
	desc := cat([]byte("MAC "), u16le(3990), u16le(0), u32le(52), make([]byte, 40))
	hdr := cat(u16le(2000), u16le(0), u32le(73728), u32le(44100), u32le(11), u16le(16), u16le(2), u32le(44100))
	got := readBytes(t, cat(desc, hdr, make([]byte, 5000)))
	want := 10*73728 + 44100
	if a := got.Audio; a.Codec != CodecAPE || a.BitDepth != 16 || a.Channels != 2 || a.SampleRate != 44100 ||
		!near(a.Duration, time.Duration(float64(want)/44100*float64(time.Second))) || a.Bitrate == 0 {
		t.Fatalf("%+v", a)
	}
	old := cat([]byte("MAC "), u16le(3970), u16le(2000), u16le(0), u16le(1), u32le(22050), make([]byte, 30))
	if a := readBytes(t, old).Audio; a.Codec != CodecAPE || a.Channels != 1 || a.SampleRate != 22050 {
		t.Fatalf("old %+v", a)
	}
}

func TestWavPack(t *testing.T) {
	flags := 1 | 9<<23 // 16-bit, stereo, 44.1 kHz
	h := cat([]byte("wvpk"), u32le(1000), u16le(0x410), []byte{0, 0}, u32le(44100*3), u32le(0), u32le(0), u32le(flags), u32le(0))
	got := readBytes(t, cat(h, make([]byte, 2000)))
	if a := got.Audio; a.Codec != CodecWavPack || a.BitDepth != 16 || a.Channels != 2 || a.SampleRate != 44100 || a.Duration != 3*time.Second {
		t.Fatalf("%+v", a)
	}
	h = cat([]byte("wvpk"), make([]byte, 20), u32le(15<<23|4), u32le(0))
	if a := readBytes(t, h).Audio; a.SampleRate != 0 || a.Channels != 1 || a.Duration != 0 {
		t.Fatalf("custom rate %+v", a)
	}
}

func TestWAV(t *testing.T) {
	fmtChunk := cat([]byte("fmt "), u32le(16), u16le(1), u16le(2), u32le(44100), u32le(176400), u16le(4), u16le(16))
	list := cat([]byte("LIST"), u32le(3), []byte("abc"), []byte{0})
	data := cat([]byte("data"), u32le(176400*2), make([]byte, 100))
	body := cat([]byte("WAVE"), fmtChunk, list, data)
	got := readBytes(t, cat([]byte("RIFF"), u32le(len(body)), body))
	if a := got.Audio; a.Codec != CodecPCM || a.BitDepth != 16 || a.Bitrate != 1411 || a.Channels != 2 || a.Duration == 0 {
		t.Fatalf("%+v", a)
	}
	noFmt := cat([]byte("RIFF"), u32le(4), []byte("WAVE"), data)
	if _, err := ReadFrom(bytes.NewReader(noFmt), int64(len(noFmt))); !errors.Is(err, ErrMalformed) {
		t.Fatalf("no fmt: %v", err)
	}
}

func TestUnsupportedInput(t *testing.T) {
	for _, b := range [][]byte{nil, []byte("ab"), bytes.Repeat([]byte("not audio "), 50)} {
		if _, err := ReadFrom(bytes.NewReader(b), int64(len(b))); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%q: %v", b, err)
		}
	}
}

// Every truncation of every fixture must be survivable: corrupt or partial
// downloads are common and must never crash an import.
func TestTruncatedFilesNeverPanic(t *testing.T) {
	fixtures := [][]byte{
		cat(id3(3, 0xc0, frame(3, "TIT2", txt(1, "abc"))), mpegFrames(mpeg1L3, 417, 3, xingAt("Xing", 3, 3, 1251))),
		cat([]byte("fLaC"), flacBlockBytes(0, false, streamInfoBody(44100, 2, 16, 100)), flacBlockBytes(4, true, comments("A=b"))),
		cat(oggPacketsPage(0, cat([]byte("OpusHead"), make([]byte, 11))), oggPacketsPage(0, cat([]byte("OpusTags"), comments("A=b")))),
		mp4File(soundEntry("mp4a", 2, 16, 44100, esds(128000)), fullBox("meta", ilst(freeform("LABEL", "x"))), true),
		cat(mpegFrames(mpeg1L3, 417, 2, nil), apeTag(apeItem(0, "Title", "x"))),
		cat([]byte("MAC "), u16le(3990), make([]byte, 80)),
	}
	for _, f := range fixtures {
		for n := 0; n <= len(f); n += 1 + n/64 {
			_, _ = ReadFrom(bytes.NewReader(f[:n]), int64(n))
		}
	}
}

func TestReadPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.flac")
	b := cat([]byte("fLaC"), flacBlockBytes(0, false, streamInfoBody(44100, 2, 16, 44100)), flacBlockBytes(4, true, comments("TITLE=On Disk")))
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Read(p)
	if err != nil || got.Title != "On Disk" || got.Duration != time.Second {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := Read(filepath.Join(t.TempDir(), "missing.flac")); err == nil {
		t.Fatal("missing file read")
	}
	if _, err := Read(t.TempDir()); err == nil {
		t.Fatal("directory read")
	}
}

func TestDateNormalisation(t *testing.T) {
	next := strconv.Itoa(time.Now().Year() + 1)
	later := strconv.Itoa(time.Now().Year() + 2)
	for in, want := range map[string]string{
		"2004": "2004", "2004-05": "2004-05", "2004.05.06": "2004-05-06", " 1860 ": "1860", next: next,
		"1859": "", later: "", "abcd": "", "": "", "04": "",
	} {
		if got := normaliseDate(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
	if yearOf("") != 0 || yearOf("1999-01") != 1999 {
		t.Fatal("yearOf")
	}
}

func TestFieldKeysAndPrecedence(t *testing.T) {
	var tg Tags
	tg.set("Album Artist", "A")
	tg.set("album_artist", "B")
	tg.set("TrackNumber", "x/7")
	tg.set("TRACKNUMBER", "4")
	tg.set("disctotal", "-1")
	tg.set("musicbrainz_artistid", " ; ")
	tg.set("musicbrainz_artistid", "id-1; id-2")
	tg.set("NOT A KEY", "v")
	tg.set("TITLE", "   ")
	tg.setLength("abc")
	if tg.AlbumArtist != "A" || tg.Track != 4 || tg.TrackTotal != 7 || tg.DiscTotal != 0 || tg.MusicBrainz.Artist != "id-1" || tg.Title != "" || tg.Duration != 0 {
		t.Fatalf("%+v", tg)
	}
}

func TestCodecLossless(t *testing.T) {
	for c, want := range map[Codec]bool{CodecFLAC: true, CodecALAC: true, CodecAPE: true, CodecWavPack: true, CodecPCM: true,
		CodecMP3: false, CodecAAC: false, CodecOpus: false, CodecVorbis: false, CodecMP2: false} {
		if c.Lossless() != want {
			t.Errorf("%s lossless=%v", c, !want)
		}
	}
}

func TestTextDecoding(t *testing.T) {
	be := cat([]byte{1, 0xfe, 0xff}, encode(2, "BE with BOM"))
	if firstText(be) != "BE with BOM" || firstText(nil) != "" || len(id3Text(nil)) != 0 {
		t.Fatal("utf16 BE BOM")
	}
	if skipExtended([]byte{0, 0, 0, 50}, 3) != nil {
		t.Fatal("oversized extended header")
	}
	if validFrameID("TI", 4) || validFrameID("ti t", 4) {
		t.Fatal("frame id")
	}
}
