package audiotag

import (
	"bytes"
	"strings"
	"unicode/utf16"
)

// id3Keys maps ID3v2 text frames (four-character 2.3/2.4 ids and their
// three-character 2.2 equivalents) to the shared field keys.
var id3Keys = map[string]string{
	"TIT2": "TITLE", "TT2": "TITLE",
	"TPE1": "ARTIST", "TP1": "ARTIST",
	"TPE2": "ALBUMARTIST", "TP2": "ALBUMARTIST",
	"TALB": "ALBUM", "TAL": "ALBUM",
	"TRCK": "TRACKNUMBER", "TRK": "TRACKNUMBER",
	"TPOS": "DISCNUMBER", "TPA": "DISCNUMBER",
	"TDRC": "DATE", "TDRL": "DATE",
	"TDOR": "ORIGINALDATE",
	"TORY": "ORIGINALYEAR", "TOR": "ORIGINALYEAR",
	"TPUB": "LABEL", "TPB": "LABEL",
}

// musicBrainzOwner is the owner of the unique file identifier frame that
// holds the recording id.
const musicBrainzOwner = "http://musicbrainz.org"

// readID3v2 reads an ID3v2 tag at the start of the file and returns the
// offset just past it (0 when there is none).
func readID3v2(s source, t *Tags) int64 {
	h := s.at(0, 10)
	if string(slice(h, 0, 3)) != "ID3" || len(h) < 10 {
		return 0
	}
	ver, flags, size := h[3], h[5], syncsafe(h[6:10])
	body := s.at(10, size)
	if flags&0x80 != 0 && ver < 4 {
		body = unsync(body)
	}
	if flags&0x40 != 0 {
		body = skipExtended(body, ver)
	}
	readFrames(body, ver, t)
	end := int64(10 + size)
	if flags&0x10 != 0 {
		end += 10
	}
	return end
}

// unsync reverses ID3 unsynchronisation (0xFF 0x00 back to 0xFF).
func unsync(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte{0xff, 0x00}, []byte{0xff})
}

// skipExtended drops the extended header: 2.4 counts it in its size, 2.3
// does not.
func skipExtended(b []byte, ver byte) []byte {
	n := int(be32(b, 0)) + 4
	if ver >= 4 {
		n = syncsafe(b)
	}
	if n > len(b) {
		return nil
	}
	return b[n:]
}

// id3State collects the 2.3 split date frames until all frames are read.
type id3State struct {
	year, dayMonth string
}

func readFrames(b []byte, ver byte, t *Tags) {
	var st id3State
	for len(b) > 0 {
		fid, body, rest, ok := nextFrame(b, ver)
		if !ok {
			break
		}
		st.frame(t, fid, body)
		b = rest
	}
	st.finish(t)
}

// nextFrame splits off one frame, undoing the 2.4 per-frame flags.
// Compressed or encrypted frames come back with an empty body.
func nextFrame(b []byte, ver byte) (fid string, body, rest []byte, ok bool) {
	idLen, hdr := 4, 10
	size := int(be32(b, 4))
	switch ver {
	case 2:
		idLen, hdr, size = 3, 6, be24(b, 3)
	case 4:
		size = syncsafe(b[min(4, len(b)):])
	}
	fid = string(slice(b, 0, idLen))
	if !validFrameID(fid, idLen) || hdr+size > len(b) {
		return "", nil, nil, false
	}
	body = frameBody(b[hdr:hdr+size], slice(b, 8, 2), ver)
	return fid, body, b[hdr+size:], true
}

func validFrameID(fid string, n int) bool {
	if len(fid) != n {
		return false
	}
	for _, c := range fid {
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// frameBody applies the frame flags: 2.3 and 2.4 compression/encryption
// make the frame unreadable; 2.4 may add a data length prefix and per-frame
// unsynchronisation.
func frameBody(body, flags []byte, ver byte) []byte {
	switch {
	case ver < 3 || len(flags) < 2:
		return body
	case ver == 3:
		return v3Body(body, flags[1])
	}
	return v4Body(body, flags[1])
}

func v3Body(body []byte, format byte) []byte {
	if format&0xc0 != 0 {
		return nil
	}
	return body
}

func v4Body(body []byte, format byte) []byte {
	if format&0x0c != 0 {
		return nil
	}
	if format&0x01 != 0 {
		body = rest(body, 4)
	}
	if format&0x02 != 0 {
		body = unsync(body)
	}
	return body
}

func (st *id3State) frame(t *Tags, fid string, body []byte) {
	switch fid {
	case "TXXX", "TXX":
		userText(t, body)
	case "UFID", "UFI":
		fileID(t, body)
	case "TYER", "TYE":
		st.year = firstText(body)
	case "TDAT", "TDA":
		st.dayMonth = firstText(body)
	case "TLEN", "TLE":
		t.setLength(firstText(body))
	default:
		if key, ok := id3Keys[fid]; ok {
			t.set(key, firstText(body))
		}
	}
}

// userText stores a user text frame under its description.
func userText(t *Tags, body []byte) {
	if v := id3Text(body); len(v) > 1 {
		t.set(v[0], v[1])
	}
}

// fileID stores the recording id from the MusicBrainz unique file
// identifier; identifiers of other owners are ignored.
func fileID(t *Tags, body []byte) {
	if owner, val, ok := bytes.Cut(body, []byte{0}); ok && string(owner) == musicBrainzOwner {
		t.set("MUSICBRAINZTRACKID", string(val))
	}
}

// finish combines the 2.3 year frame with its DDMM day-month frame into one
// date; a full date frame read earlier takes precedence.
func (st *id3State) finish(t *Tags) {
	d := st.year
	if len(st.dayMonth) == 4 && atoi(st.dayMonth) > 0 {
		d += "-" + st.dayMonth[2:] + "-" + st.dayMonth[:2]
	}
	t.set("DATE", d)
}

func firstText(body []byte) string {
	if v := id3Text(body); len(v) > 0 {
		return v[0]
	}
	return ""
}

// id3Text decodes a text frame (encoding byte, then NUL-separated values)
// into its values.
func id3Text(body []byte) []string {
	if len(body) == 0 {
		return nil
	}
	s := decodeText(body[0], body[1:])
	s = strings.ReplaceAll(strings.TrimRight(s, "\x00"), string(rune(0xfeff)), "")
	return strings.Split(s, "\x00")
}

func decodeText(enc byte, b []byte) string {
	switch enc {
	case 1:
		return decodeUTF16(b, bomOrder(b))
	case 2:
		return decodeUTF16(b, true)
	case 3:
		return string(b)
	}
	return latin1(b)
}

// bomOrder reports big-endian for a FE FF byte order mark; anything else is
// read as little-endian, the common case.
func bomOrder(b []byte) bool { return len(b) >= 2 && b[0] == 0xfe && b[1] == 0xff }

func decodeUTF16(b []byte, bigEndian bool) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		if bigEndian {
			u[i] = uint16(be16(b, 2*i))
		} else {
			u[i] = uint16(le16(b, 2*i))
		}
	}
	return string(utf16.Decode(u))
}

func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}
