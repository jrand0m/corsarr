package audiotag

import (
	"bytes"
	"encoding/binary"
	"unicode/utf16"
)

// Builders for tiny synthetic audio files. None of them hold real audio:
// they carry just enough structure for the header and tag readers.

func cat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

func u16be(n int) []byte { return binary.BigEndian.AppendUint16(nil, uint16(n)) }
func u32be(n int) []byte { return binary.BigEndian.AppendUint32(nil, uint32(n)) }
func u64be(n int) []byte { return binary.BigEndian.AppendUint64(nil, uint64(n)) }
func u16le(n int) []byte { return binary.LittleEndian.AppendUint16(nil, uint16(n)) }
func u32le(n int) []byte { return binary.LittleEndian.AppendUint32(nil, uint32(n)) }
func u64le(n int) []byte { return binary.LittleEndian.AppendUint64(nil, uint64(n)) }

func syncsafeBytes(n int) []byte {
	return []byte{byte(n >> 21 & 0x7f), byte(n >> 14 & 0x7f), byte(n >> 7 & 0x7f), byte(n & 0x7f)}
}

// --- ID3v2 ---

// txt is a text frame body: encoding byte then the values joined by NUL.
func txt(enc byte, values ...string) []byte {
	out := []byte{enc}
	for i, v := range values {
		if i > 0 {
			out = append(out, sep(enc)...)
		}
		out = append(out, encode(enc, v)...)
	}
	return out
}

func sep(enc byte) []byte {
	if enc == 1 || enc == 2 {
		return []byte{0, 0}
	}
	return []byte{0}
}

func encode(enc byte, s string) []byte {
	switch enc {
	case 1: // UTF-16 little-endian with byte order mark
		out := []byte{0xff, 0xfe}
		for _, u := range utf16.Encode([]rune(s)) {
			out = append(out, u16le(int(u))...)
		}
		return out
	case 2:
		var out []byte
		for _, u := range utf16.Encode([]rune(s)) {
			out = append(out, u16be(int(u))...)
		}
		return out
	case 0:
		out := make([]byte, 0, len(s))
		for _, r := range s {
			out = append(out, byte(r))
		}
		return out
	}
	return []byte(s)
}

func frame(ver byte, fid string, body []byte, flags ...byte) []byte {
	if ver == 2 {
		return cat([]byte(fid), []byte{byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}, body)
	}
	size := u32be(len(body))
	if ver == 4 {
		size = syncsafeBytes(len(body))
	}
	fl := []byte{0, 0}
	if len(flags) > 0 {
		fl[1] = flags[0]
	}
	return cat([]byte(fid), size, fl, body)
}

func id3(ver, flags byte, frames ...[]byte) []byte {
	body := cat(frames...)
	body = append(body, make([]byte, 16)...) // padding
	return cat([]byte("ID3"), []byte{ver, 0, flags}, syncsafeBytes(len(body)), body)
}

// --- MPEG audio ---

// mpeg1L3 is an MPEG-1 layer 3, 128 kbps, 44.1 kHz joint-stereo header;
// each frame is 417 bytes.
var mpeg1L3 = []byte{0xff, 0xfb, 0x90, 0x64}

func mpegFrames(header []byte, length, n int, first []byte) []byte {
	var out []byte
	for i := 0; i < n; i++ {
		f := make([]byte, length)
		copy(f, header)
		if i == 0 && first != nil {
			copy(f[4:], first)
		}
		out = append(out, f...)
	}
	return out
}

// xingAt is a Xing/Info header placed after the side info of a stereo
// MPEG-1 layer 3 frame.
func xingAt(tag string, flags, frames, size int) []byte {
	return cat(make([]byte, 32), []byte(tag), u32be(flags), u32be(frames), u32be(size))
}

// --- FLAC and Vorbis comments ---

func comments(entries ...string) []byte {
	out := cat(u32le(6), []byte("tester"), u32le(len(entries)))
	for _, e := range entries {
		out = cat(out, u32le(len(e)), []byte(e))
	}
	return out
}

func streamInfoBody(rate, channels, depth int, total int64) []byte {
	b := make([]byte, 34)
	b[10] = byte(rate >> 12)
	b[11] = byte(rate >> 4)
	b[12] = byte(rate<<4) | byte(channels-1)<<1 | byte((depth-1)>>4)
	b[13] = byte((depth-1)&0xf)<<4 | byte(total>>32&0xf)
	binary.BigEndian.PutUint32(b[14:], uint32(total))
	return b
}

func flacBlockBytes(typ byte, last bool, body []byte) []byte {
	if last {
		typ |= 0x80
	}
	return cat([]byte{typ, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}, body)
}

// --- Ogg ---

func lacing(p []byte) []byte {
	segs := bytes.Repeat([]byte{255}, len(p)/255)
	return append(segs, byte(len(p)%255))
}

func oggPage(granule int64, segs, data []byte) []byte {
	return cat([]byte("OggS"), []byte{0, 0}, u64le(int(granule)), u32le(1), u32le(0), u32le(0),
		[]byte{byte(len(segs))}, segs, data)
}

func oggPacketsPage(granule int64, packets ...[]byte) []byte {
	var segs, data []byte
	for _, p := range packets {
		segs, data = append(segs, lacing(p)...), append(data, p...)
	}
	return oggPage(granule, segs, data)
}

// --- MP4 ---

func box(typ string, parts ...[]byte) []byte {
	body := cat(parts...)
	return cat(u32be(8+len(body)), []byte(typ), body)
}

func fullBox(typ string, parts ...[]byte) []byte {
	return box(typ, append([]byte{0, 0, 0, 0}, cat(parts...)...))
}

func dataBox(value []byte) []byte { return box("data", u32be(1), u32be(0), value) }

func soundEntry(typ string, channels, size, rate int, children ...[]byte) []byte {
	return box(typ, make([]byte, 6), u16be(1), make([]byte, 8), u16be(channels), u16be(size),
		u16be(0), u16be(0), u32be(rate<<16), cat(children...))
}

func trak(entry []byte) []byte {
	stsd := fullBox("stsd", u32be(1), entry)
	return box("trak", box("mdia", box("minf", box("stbl", stsd))))
}

func mvhd(scale, duration int) []byte {
	return fullBox("mvhd", u32be(0), u32be(0), u32be(scale), u32be(duration), make([]byte, 80))
}

func esds(avg int) []byte {
	return fullBox("esds", []byte{3, 0x80, 0x80, 0x80, 0x19, 0, 1, 0}, []byte{4, 0x11, 0x40, 0x15, 0, 0, 0},
		u32be(avg+1000), u32be(avg))
}

// --- APE ---

func apeItem(flags int, key, value string) []byte {
	return cat(u32le(len(value)), u32le(flags), []byte(key), []byte{0}, []byte(value))
}

func apeTag(items ...[]byte) []byte {
	body := cat(items...)
	return cat(body, []byte("APETAGEX"), u32le(2000), u32le(len(body)+32), u32le(len(items)), u32le(0), make([]byte, 8))
}
