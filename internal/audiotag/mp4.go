package audiotag

import (
	"fmt"
	"time"
)

// mp4Keys maps iTunes-style metadata atoms to the shared field keys. Atom
// names starting with the copyright sign are matched by their raw 0xA9 byte.
var mp4Keys = map[string]string{
	"\xa9nam": "TITLE",
	"\xa9ART": "ARTIST",
	"aART":    "ALBUMARTIST",
	"\xa9alb": "ALBUM",
	"\xa9day": "DATE",
}

// readMP4 finds the top-level movie box (which may follow the media data)
// and reads its duration, first audio sample entry and metadata list.
func readMP4(s source, off int64, t *Tags) error {
	moov := topBox(s, off, "moov")
	if moov == nil {
		return ErrMalformed
	}
	a := sampleEntry(audioEntry(moov))
	if a.Codec == "" {
		return ErrUnsupported
	}
	a.Duration = movieDuration(child(moov, "mvhd"))
	a.estimate(s.size)
	t.Audio = a
	each(child(moov, "udta", "meta", "ilst"), func(typ string, body []byte) { mp4Item(t, typ, body) })
	return nil
}

// box is one parsed box header: its type and where its body lies.
type boxHeader struct {
	typ        string
	start, end int64 // body range
}

// parseBox reads the header at h (the first 16 bytes of a box at pos);
// ok=false when the size is not sane.
func parseBox(h []byte, pos, limit int64) (boxHeader, bool) {
	size, hdr := be32(h, 0), int64(8)
	switch size {
	case 0:
		size = limit - pos
	case 1:
		size, hdr = be64(h, 8), 16
	}
	if len(h) < 8 || size < hdr || pos+size > limit {
		return boxHeader{}, false
	}
	return boxHeader{typ: string(h[4:8]), start: pos + hdr, end: pos + size}, true
}

// topBox loads the body of the first top-level box of the given type.
func topBox(s source, off int64, typ string) []byte {
	for pos := off; pos < s.size; {
		b, ok := parseBox(s.at(pos, 16), pos, s.size)
		if !ok {
			return nil
		}
		if b.typ == typ {
			return s.at(b.start, int(b.end-b.start))
		}
		pos = b.end
	}
	return nil
}

// each calls fn for every box in b.
func each(b []byte, fn func(typ string, body []byte)) {
	for pos := int64(0); pos < int64(len(b)); {
		bx, ok := parseBox(slice(b, int(pos), min(16, len(b)-int(pos))), pos, int64(len(b)))
		if !ok {
			return
		}
		fn(bx.typ, boxBody(bx.typ, b[bx.start:bx.end]))
		pos = bx.end
	}
}

// boxBody drops the version/flags word of full boxes that hold children.
// A "meta" box is a full box in MP4 files but a plain one in QuickTime
// files; the QuickTime form starts directly with its handler box.
func boxBody(typ string, body []byte) []byte {
	if typ == "meta" && string(slice(body, 4, 4)) != "hdlr" {
		return rest(body, 4)
	}
	return body
}

// child descends through nested boxes by type, returning the body of the
// first match at each level.
func child(b []byte, path ...string) []byte {
	for _, typ := range path {
		var found []byte
		each(b, func(t string, body []byte) {
			if t == typ && found == nil {
				found = body
			}
		})
		if found == nil {
			return nil
		}
		b = found
	}
	return b
}

// audioEntry returns the type and body of the first AAC or ALAC sample
// entry of the first track that has one.
func audioEntry(moov []byte) (typ string, entry []byte) {
	each(moov, func(t string, trak []byte) {
		if t == "trak" && entry == nil {
			typ, entry = trackAudio(trak)
		}
	})
	return typ, entry
}

func trackAudio(trak []byte) (typ string, entry []byte) {
	stsd := child(trak, "mdia", "minf", "stbl", "stsd")
	each(rest(stsd, 8), func(et string, body []byte) {
		if entry == nil && (et == "mp4a" || et == "alac") {
			typ, entry = et, body
		}
	})
	return typ, entry
}

// sampleEntry decodes an audio sample entry: channel count, sample size and
// rate, then the codec-specific child (esds for AAC, alac for ALAC).
func sampleEntry(typ string, b []byte) AudioInfo {
	a := AudioInfo{Channels: be16(b, 16), BitDepth: be16(b, 18), SampleRate: int(be32(b, 24) >> 16)}
	children := rest(b, 28+soundVersionExtra(be16(b, 8)))
	switch typ {
	case "mp4a":
		a.Codec, a.BitDepth = CodecAAC, 0
		a.Bitrate = int(esdsBitrate(child(children, "esds")) / 1000)
	case "alac":
		a.Codec = CodecALAC
		alacConfig(child(children, "alac"), &a)
	}
	return a
}

// soundVersionExtra is the extra length of QuickTime version 1 and 2 sound
// sample descriptions.
func soundVersionExtra(v int) int {
	return map[int]int{1: 16, 2: 36}[v]
}

// alacConfig reads bit depth and sample rate from the ALAC magic cookie,
// which is more accurate than the generic sample entry fields.
func alacConfig(b []byte, a *AudioInfo) {
	if len(b) < 28 {
		return
	}
	a.BitDepth, a.Channels, a.SampleRate = int(b[9]), int(b[13]), int(be32(b, 24))
}

// esdsBitrate walks the elementary stream descriptor to the decoder
// config descriptor and returns its average bitrate (bits per second).
func esdsBitrate(b []byte) int64 {
	tag, p := descriptor(b, 4)
	if tag != 3 {
		return 0
	}
	flags := at(b, p+2)
	p += 3 + optionalESFields(b, p+3, flags)
	if tag, p = descriptor(b, p); tag != 4 {
		return 0
	}
	return be32(b, p+9)
}

func optionalESFields(b []byte, p int, flags byte) int {
	n := 0
	if flags&0x80 != 0 {
		n += 2
	}
	if flags&0x40 != 0 {
		n += 1 + int(at(b, p+n))
	}
	if flags&0x20 != 0 {
		n += 2
	}
	return n
}

// descriptor reads a descriptor tag and its variable-length size, returning
// the tag and the offset of its payload.
func descriptor(b []byte, p int) (byte, int) {
	tag := at(b, p)
	p++
	for i := 0; i < 4 && at(b, p)&0x80 != 0; i++ {
		p++
	}
	return tag, p + 1
}

// movieDuration reads the movie header's time scale and duration.
func movieDuration(b []byte) time.Duration {
	if at(b, 0) == 1 {
		return seconds(be64(b, 24), int(be32(b, 20)))
	}
	return seconds(be32(b, 16), int(be32(b, 12)))
}

// mp4Item stores one metadata list item; its value sits in a "data" child
// after an 8-byte type and locale header.
func mp4Item(t *Tags, typ string, body []byte) {
	v := rest(child(body, "data"), 8)
	switch typ {
	case "trkn":
		t.set("TRACKNUMBER", pairText(be16(v, 2), be16(v, 4)))
	case "disk":
		t.set("DISCNUMBER", pairText(be16(v, 2), be16(v, 4)))
	case "----":
		t.set(string(rest(child(body, "name"), 4)), string(v))
	default:
		t.set(mp4Keys[typ], string(v))
	}
}

func pairText(n, total int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", n, total)
}
