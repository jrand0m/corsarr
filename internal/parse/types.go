// Package parse turns a release title into a ParsedRelease.
//
// It is pure and deterministic: no I/O, no clock, no globals that change. Every
// attribute is either known (detected in the title) or unknown; nothing is
// guessed. The parser is written from general knowledge of scene and P2P naming
// conventions (clean room).
package parse

// Field is a value the title either states (Known) or does not.
// The zero value is "unknown", so a forgotten assignment can never claim knowledge.
type Field[T comparable] struct {
	Value T    `json:"value"`
	Known bool `json:"known"`
}

func known[T comparable](v T) Field[T] { return Field[T]{Value: v, Known: true} }

// Resolution is the vertical pixel count of the video.
type Resolution int

const (
	R480  Resolution = 480
	R576  Resolution = 576
	R720  Resolution = 720
	R1080 Resolution = 1080
	R2160 Resolution = 2160
)

// Source is where the video was captured or encoded from.
type Source string

const (
	SourceCam      Source = "cam"
	SourceTelesync Source = "telesync"
	SourceTelecine Source = "telecine"
	SourceScreener Source = "screener"
	SourceDVD      Source = "dvd"
	SourceHDTV     Source = "hdtv"
	SourceWEBRip   Source = "webrip"
	SourceWEBDL    Source = "webdl"
	SourceBluray   Source = "bluray"
	SourceRemux    Source = "remux"
)

// Codec is the video codec.
type Codec string

const (
	CodecH264  Codec = "h264"
	CodecH265  Codec = "h265"
	CodecAV1   Codec = "av1"
	CodecVP9   Codec = "vp9"
	CodecXviD  Codec = "xvid"
	CodecVC1   Codec = "vc1"
	CodecMPEG2 Codec = "mpeg2"
)

// HDR is one high-dynamic-range format; a release may carry several (DV + HDR10).
type HDR string

const (
	HDR10     HDR = "hdr10"
	HDR10Plus HDR = "hdr10+"
	HDRDV     HDR = "dv"
	HDRHLG    HDR = "hlg"
)

// DiscFormat names a full-disc image layout.
type DiscFormat string

const (
	DiscISO  DiscFormat = "iso"
	DiscBDMV DiscFormat = "bdmv"
	DiscIMG  DiscFormat = "img"
)

// Audio is the primary audio codec.
type Audio string

const (
	AudioAAC    Audio = "aac"
	AudioAC3    Audio = "ac3"
	AudioEAC3   Audio = "eac3"
	AudioDTS    Audio = "dts"
	AudioDTSHD  Audio = "dts-hd"
	AudioDTSX   Audio = "dts-x"
	AudioTrueHD Audio = "truehd"
	AudioFLAC   Audio = "flac"
	AudioOpus   Audio = "opus"
	AudioMP3    Audio = "mp3"
	AudioLPCM   Audio = "lpcm"
)

// EpisodeKind says how a title addresses TV content.
type EpisodeKind string

const (
	EpisodeNone     EpisodeKind = ""        // not TV-shaped (movie)
	EpisodeStandard EpisodeKind = "episode" // S01E02, 1x02, multi-episode
	EpisodeSeason   EpisodeKind = "season"  // whole-season pack (S01, Season 1, S01-S03)
	EpisodeDaily    EpisodeKind = "daily"   // 2024.03.15
)

// Episode is the TV address of a release. Only the fields meaningful for Kind are set.
type Episode struct {
	Kind     EpisodeKind `json:"kind"`
	Seasons  []int       `json:"seasons,omitempty"`  // one for episodes, 1..n for packs
	Episodes []int       `json:"episodes,omitempty"` // ordered, de-duplicated
	AirDate  string      `json:"airDate,omitempty"`  // YYYY-MM-DD for daily shows
}

// ParsedRelease is everything a title says about a release.
type ParsedRelease struct {
	Raw        string            `json:"raw"`
	Title      string            `json:"title"`            // primary clean title ("" when none)
	Titles     []string          `json:"titles,omitempty"` // all clean titles (AKA)
	Year       Field[int]        `json:"year"`
	Resolution Field[Resolution] `json:"resolution"`
	Source     Field[Source]     `json:"source"`
	Codec      Field[Codec]      `json:"codec"`
	HDR        []HDR             `json:"hdr,omitempty"` // empty = unknown/SDR
	DVProfile  Field[int]        `json:"dvProfile"`
	ThreeD     bool              `json:"threeD"`
	Disc       Field[DiscFormat] `json:"disc"`
	Audio      Field[Audio]      `json:"audio"`
	Atmos      bool              `json:"atmos"`
	Channels   Field[string]     `json:"channels"` // "5.1", "7.1", "2.0"
	Group      Field[string]     `json:"group"`
	Proper     bool              `json:"proper"`
	Repack     bool              `json:"repack"`
	Version    Field[int]        `json:"version"` // v2, v3 …
	Edition    Field[string]     `json:"edition"`
	Languages  []string          `json:"languages,omitempty"` // lower-case ISO 639-1, or "multi"
	Episode    Episode           `json:"episode"`
}

// Revision orders re-releases of the same quality: 1 for an original, +1 for
// a PROPER/REPACK, and the explicit version number when one is given.
func (p ParsedRelease) Revision() int {
	r := 1
	if p.Version.Known {
		r = p.Version.Value
	}
	if p.Proper || p.Repack {
		r++
	}
	return r
}
