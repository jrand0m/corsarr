// Package audiotag reads descriptive tags and audio stream properties from
// music files without decoding any audio. It understands ID3v2 (2.2, 2.3 and
// 2.4), Vorbis comments (FLAC, Ogg Vorbis, Ogg Opus, Ogg FLAC), APEv2 and
// MP4/M4A metadata atoms, and the stream headers of MPEG audio, FLAC, Ogg,
// MP4 (AAC and ALAC), Monkey's Audio, WavPack and WAV.
//
// Reading is bounded: only headers and tag blocks are loaded, never the
// audio payload, so it is cheap even on large lossless files.
package audiotag

import (
	"errors"
	"io"
	"os"
	"time"
)

// Codec names the audio coding of a stream.
type Codec string

const (
	CodecMP1     Codec = "mp1"
	CodecMP2     Codec = "mp2"
	CodecMP3     Codec = "mp3"
	CodecAAC     Codec = "aac"
	CodecALAC    Codec = "alac"
	CodecFLAC    Codec = "flac"
	CodecVorbis  Codec = "vorbis"
	CodecOpus    Codec = "opus"
	CodecAPE     Codec = "ape"
	CodecWavPack Codec = "wavpack"
	CodecPCM     Codec = "pcm"
)

// Lossless reports whether the codec keeps every sample.
func (c Codec) Lossless() bool {
	switch c {
	case CodecALAC, CodecFLAC, CodecAPE, CodecWavPack, CodecPCM:
		return true
	}
	return false
}

// AudioInfo is what the stream headers say about the audio itself. Zero
// values mean the header does not carry that property.
type AudioInfo struct {
	Codec      Codec
	Bitrate    int  // kbps: the constant rate, or the average for VBR and lossless
	VBR        bool // the encoder varied the bitrate (MP3 Xing/VBRI, Vorbis, Opus)
	SampleRate int  // Hz
	BitDepth   int  // bits per sample, for lossless codecs
	Channels   int
	Duration   time.Duration
}

// MBIDs are the MusicBrainz identifiers a tagger stored in the file.
type MBIDs struct {
	Album        string // the release
	ReleaseGroup string
	Artist       string
	AlbumArtist  string
	Recording    string // the recording; most taggers store it as the "track id"
	Track        string // the track on the release
}

// Tags is everything read from one file.
type Tags struct {
	Title       string
	Artist      string
	AlbumArtist string
	Album       string

	Track      int
	TrackTotal int
	Disc       int
	DiscTotal  int

	Date         string // YYYY, YYYY-MM or YYYY-MM-DD
	Year         int
	OriginalDate string
	OriginalYear int

	Label         string
	ReleaseStatus string
	ReleaseType   string

	MusicBrainz MBIDs

	// Duration is the stream duration from the headers, or the length the
	// tags state when the headers carry none.
	Duration time.Duration
	Audio    AudioInfo
}

var (
	// ErrUnsupported means the file is not a recognised audio format.
	ErrUnsupported = errors.New("audiotag: unrecognised audio format")
	// ErrMalformed means the format was recognised but its headers are broken.
	ErrMalformed = errors.New("audiotag: malformed audio headers")
)

// Read reads the tags and stream properties of the file at path.
func Read(path string) (Tags, error) {
	f, err := os.Open(path)
	if err != nil {
		return Tags{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Tags{}, err
	}
	return ReadFrom(f, st.Size())
}

// ReadFrom reads tags and stream properties from r, which holds size bytes.
// Tags read before an error are still returned.
func ReadFrom(r io.ReaderAt, size int64) (Tags, error) {
	s := source{r: r, size: size}
	var t Tags
	off := readID3v2(s, &t)
	read := detect(s.at(off, 12))
	if read == nil {
		return t, ErrUnsupported
	}
	err := read(s, off, &t)
	readAPE(s, &t)
	t.finish()
	return t, err
}

// finish derives the year fields and settles the duration.
func (t *Tags) finish() {
	t.Year = yearOf(t.Date)
	t.OriginalYear = yearOf(t.OriginalDate)
	if t.Audio.Duration > 0 {
		t.Duration = t.Audio.Duration
	}
}

type reader func(s source, off int64, t *Tags) error

type container struct {
	match func(head []byte) bool
	read  reader
}

var containers = []container{
	{magic(0, "fLaC"), readFLAC},
	{magic(0, "OggS"), readOgg},
	{magic(4, "ftyp"), readMP4},
	{magic(0, "MAC "), readMonkey},
	{magic(0, "wvpk"), readWavPack},
	{isWAV, readWAV},
}

// detect picks the reader for the stream starting with head; anything
// unrecognised is tried as MPEG audio, which has no magic of its own.
func detect(head []byte) reader {
	if len(head) < 4 {
		return nil
	}
	for _, c := range containers {
		if c.match(head) {
			return c.read
		}
	}
	return readMPEG
}

func magic(at int, m string) func([]byte) bool {
	return func(h []byte) bool { return string(slice(h, at, len(m))) == m }
}

func isWAV(h []byte) bool {
	return string(slice(h, 0, 4)) == "RIFF" && string(slice(h, 8, 4)) == "WAVE"
}

// estimate fills a missing bitrate from the stream's byte count and duration.
func (a *AudioInfo) estimate(bytes int64) {
	if a.Bitrate > 0 || a.Duration <= 0 || bytes <= 0 {
		return
	}
	a.Bitrate = int(float64(bytes) * 8 / a.Duration.Seconds() / 1000)
}

// seconds converts a sample count at rate into a duration.
func seconds(samples int64, rate int) time.Duration {
	if rate <= 0 || samples <= 0 {
		return 0
	}
	return time.Duration(float64(samples) / float64(rate) * float64(time.Second))
}
