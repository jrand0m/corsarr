package audiotag

// mpegScan is how far past the tags the first MPEG frame is searched for.
const mpegScan = 64 << 10

// mpegFrame is one decoded MPEG audio frame header.
type mpegFrame struct {
	version int // 1, 2, or 25 for MPEG 2.5
	layer   int // 1, 2 or 3
	kbps    int
	rate    int
	padding int
	mono    bool
}

var mpegRates = [3]int{44100, 48000, 32000}

// mpegBitrates holds the kbps per bitrate index for MPEG-1 layers 1-3 and
// MPEG-2/2.5 layer 1 and layers 2-3.
var mpegBitrates = [5][15]int{
	{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448},
	{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384},
	{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320},
	{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256},
	{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},
}

var mpegVersions = map[byte]int{0: 25, 2: 2, 3: 1}

// parseMPEG decodes the four header bytes at b; ok=false when they are not
// a valid frame header (bad sync, reserved fields or free-format bitrate).
func parseMPEG(b []byte) (mpegFrame, bool) {
	h := slice(b, 0, 4)
	if !synced(h) {
		return mpegFrame{}, false
	}
	v, okV := mpegVersions[(h[1]>>3)&3]
	f := mpegFrame{version: v, layer: 4 - int((h[1]>>1)&3), padding: int(h[2]>>1) & 1, mono: h[3]>>6 == 3}
	bi, ri := int(h[2]>>4), int((h[2]>>2)&3)
	if !okV || !f.valid(bi, ri) {
		return mpegFrame{}, false
	}
	f.rate = mpegRates[ri] / map[int]int{1: 1, 2: 2, 25: 4}[v]
	f.kbps = mpegBitrates[f.table()][bi]
	return f, true
}

func synced(h []byte) bool { return len(h) == 4 && h[0] == 0xff && h[1]&0xe0 == 0xe0 }

// valid rejects the reserved layer, free-format and bad bitrate indexes and
// the reserved sample rate index.
func (f mpegFrame) valid(bi, ri int) bool { return f.layer != 4 && bi > 0 && bi < 15 && ri < 3 }

func (f mpegFrame) table() int {
	if f.version == 1 {
		return f.layer - 1
	}
	return 3 + min(f.layer-1, 1)
}

// samples is the number of samples one frame holds.
func (f mpegFrame) samples() int {
	switch {
	case f.layer == 1:
		return 384
	case f.layer == 3 && f.version != 1:
		return 576
	}
	return 1152
}

// length is the frame's size in bytes.
func (f mpegFrame) length() int {
	if f.layer == 1 {
		return (12*f.kbps*1000/f.rate + f.padding) * 4
	}
	return f.samples()/8*f.kbps*1000/f.rate + f.padding
}

// xingOffset is where a Xing/Info header sits: after the layer 3 side info.
func (f mpegFrame) xingOffset() int {
	side := map[bool]int{true: 17, false: 32}[f.mono]
	if f.version != 1 {
		side = map[bool]int{true: 9, false: 17}[f.mono]
	}
	return 4 + side
}

func (f mpegFrame) codec() Codec {
	return [4]Codec{"", CodecMP1, CodecMP2, CodecMP3}[f.layer]
}

// findFrame returns the offset of the first frame header in b that is
// followed by another valid header (or by the end of b), to skip stray
// sync patterns inside tags or junk.
func findFrame(b []byte) (int, mpegFrame, bool) {
	for i := 0; i+4 <= len(b); i++ {
		f, ok := parseMPEG(b[i:])
		if ok && confirmed(b, i+f.length()) {
			return i, f, true
		}
	}
	return 0, mpegFrame{}, false
}

func confirmed(b []byte, next int) bool {
	if next+4 > len(b) {
		return true
	}
	_, ok := parseMPEG(b[next:])
	return ok
}

// readMPEG reads the first frame and any Xing, Info or VBRI header in it.
// Without such a header the stream is taken as constant bitrate and its
// duration follows from the payload size.
func readMPEG(s source, off int64, t *Tags) error {
	buf := s.at(off, mpegScan)
	i, f, ok := findFrame(buf)
	if !ok {
		return ErrUnsupported
	}
	a := AudioInfo{Codec: f.codec(), Bitrate: f.kbps, SampleRate: f.rate, Channels: map[bool]int{true: 1, false: 2}[f.mono]}
	frame := buf[i:]
	payload := s.size - off - int64(i)
	if frames, bytes, vbr, ok := vbrHeader(frame, f); ok {
		a.VBR = vbr
		a.Duration = seconds(frames*int64(f.samples()), f.rate)
		if vbr {
			a.Bitrate = 0
			a.estimate(first64(bytes, payload))
		}
	} else {
		a.Duration = seconds(payload*8, f.kbps*1000)
	}
	t.Audio = a
	return nil
}

func first64(a, b int64) int64 {
	if a > 0 {
		return a
	}
	return b
}

// vbrHeader reads a Xing ("Xing" marks VBR, "Info" CBR) or VBRI header:
// the frame count and, when stated, the stream size in bytes.
func vbrHeader(frame []byte, f mpegFrame) (frames, bytes int64, vbr, ok bool) {
	if f.layer != 3 {
		return 0, 0, false, false
	}
	x := f.xingOffset()
	switch string(slice(frame, x, 4)) {
	case "Xing", "Info":
		return xing(frame[x:])
	}
	if string(slice(frame, 36, 4)) == "VBRI" {
		return be32(frame, 36+14), be32(frame, 36+10), true, true
	}
	return 0, 0, false, false
}

func xing(b []byte) (frames, bytes int64, vbr, ok bool) {
	flags, p := be32(b, 4), 8
	if flags&1 == 0 {
		return 0, 0, false, false
	}
	frames, p = be32(b, p), p+4
	if flags&2 != 0 {
		bytes = be32(b, p)
	}
	return frames, bytes, string(b[:4]) == "Xing", frames > 0
}
