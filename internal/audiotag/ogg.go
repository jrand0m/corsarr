package audiotag

import (
	"bytes"
	"strings"
)

// oggTail is how much of the file end is searched for the last page, which
// carries the final granule position (the stream length in samples).
const oggTail = 64 << 10

// oggCodec describes one codec carried in Ogg: how its identification
// packet starts, how to decode it, and how long the prefix of its comment
// packet is before the Vorbis comment block.
type oggCodec struct {
	prefix   string
	ident    func(p []byte) (a AudioInfo, clock int, skip int64)
	comments int
}

var oggCodecs = []oggCodec{
	{"\x01vorbis", vorbisIdent, 7},
	{"OpusHead", opusIdent, 8},
	{"\x7fFLAC", oggFLACIdent, 4},
}

// readOgg reads the identification and comment packets of the first
// logical stream and takes the duration from the last page's granule.
func readOgg(s source, off int64, t *Tags) error {
	pk := oggPackets(s, off, 2)
	if len(pk) == 0 {
		return ErrMalformed
	}
	c, ok := oggCodecOf(pk[0])
	if !ok {
		return ErrUnsupported
	}
	a, clock, skip := c.ident(pk[0])
	if len(pk) > 1 {
		readComments(rest(pk[1], c.comments), t)
	}
	a.Duration = seconds(lastGranule(s)-skip, clock)
	a.estimate(s.size - off)
	t.Audio = a
	return nil
}

func oggCodecOf(p []byte) (oggCodec, bool) {
	for _, c := range oggCodecs {
		if strings.HasPrefix(string(p), c.prefix) {
			return c, true
		}
	}
	return oggCodec{}, false
}

// vorbisIdent decodes the Vorbis identification header; its nominal
// bitrate is used when the encoder set one.
func vorbisIdent(p []byte) (AudioInfo, int, int64) {
	rate := int(le32(p, 12))
	a := AudioInfo{Codec: CodecVorbis, VBR: true, Channels: int(at(p, 11)), SampleRate: rate}
	if nominal := int(int32(le32(p, 20))); nominal > 0 {
		a.Bitrate = nominal / 1000
	}
	return a, rate, 0
}

// opusIdent decodes OpusHead. Opus always runs at 48 kHz and its granule
// includes the pre-skip; the stated rate is the original input rate.
func opusIdent(p []byte) (AudioInfo, int, int64) {
	return AudioInfo{Codec: CodecOpus, VBR: true, Channels: int(at(p, 9)), SampleRate: int(le32(p, 12))},
		48000, int64(le16(p, 10))
}

// oggFLACIdent decodes the FLAC mapping header, which embeds "fLaC" and a
// STREAMINFO block after a 9-byte prefix.
func oggFLACIdent(p []byte) (AudioInfo, int, int64) {
	a := streamInfo(rest(p, 17))
	return a, a.SampleRate, 0
}

func at(b []byte, i int) byte {
	if i < len(b) {
		return b[i]
	}
	return 0
}

// oggPackets reassembles the first want packets from consecutive pages.
func oggPackets(s source, off int64, want int) [][]byte {
	var out [][]byte
	var cur []byte
	for len(out) < want && len(cur) <= maxBlock {
		h := s.at(off, 27)
		if string(slice(h, 0, 4)) != "OggS" || len(h) < 27 {
			break
		}
		segs := s.at(off+27, int(h[26]))
		data := s.at(off+27+int64(len(segs)), lacedSize(segs))
		out, cur = lace(out, cur, segs, data)
		off += 27 + int64(len(segs)) + int64(lacedSize(segs))
	}
	return out
}

func lacedSize(segs []byte) int {
	n := 0
	for _, l := range segs {
		n += int(l)
	}
	return n
}

// lace splits page data by its segment table: a segment shorter than 255
// bytes ends a packet, a full one continues it.
func lace(out [][]byte, cur, segs, data []byte) ([][]byte, []byte) {
	p := 0
	for _, l := range segs {
		end := min(p+int(l), len(data))
		cur = append(cur, data[p:end]...)
		p = end
		if l < 255 {
			out, cur = append(out, cur), nil
		}
	}
	return out, cur
}

func lastGranule(s source) int64 {
	from := max(0, s.size-oggTail)
	tail := s.at(from, oggTail)
	i := bytes.LastIndex(tail, []byte("OggS"))
	if i < 0 {
		return 0
	}
	return le64(tail, i+6)
}
