package audiotag

import "bytes"

const (
	apeFooterLen = 32
	id3v1Len     = 128
)

// readAPE reads an APEv2 tag at the end of the file (before a trailing
// ID3v1 tag, if any). Values only fill fields that earlier tags left empty.
func readAPE(s source, t *Tags) {
	end := s.size
	if string(s.at(end-id3v1Len, 3)) == "TAG" {
		end -= id3v1Len
	}
	f := s.at(end-apeFooterLen, apeFooterLen)
	if string(slice(f, 0, 8)) != "APETAGEX" {
		return
	}
	size := le32(f, 12)
	readAPEItems(s.at(end-size, int(size)-apeFooterLen), int(le32(f, 16)), t)
}

// readAPEItems parses n items: value size, flags, NUL-terminated key, then
// the value. Only UTF-8 text items (type bits zero) are read.
func readAPEItems(b []byte, n int, t *Tags) {
	p := 0
	for i := 0; i < n; i++ {
		vlen, flags := int(le32(b, p)), le32(b, p+4)
		k := bytes.IndexByte(b[min(p+8, len(b)):], 0)
		if k < 0 {
			return
		}
		key, value := b[p+8:p+8+k], slice(b, p+9+k, vlen)
		if value == nil {
			return
		}
		if flags&0x6 == 0 {
			t.set(string(key), string(value))
		}
		p += 9 + k + vlen
	}
}

// readMonkey reads a Monkey's Audio header. Files from encoder version
// 3.98 on carry a descriptor followed by the header with the frame layout;
// older files only yield the codec, channels and rate.
func readMonkey(s source, off int64, t *Tags) error {
	h := s.at(off, 52)
	a := AudioInfo{Codec: CodecAPE}
	if le16(h, 4) < 3980 {
		a.Channels, a.SampleRate = le16(h, 10), int(le32(h, 12))
		t.Audio = a
		return nil
	}
	hdr := s.at(off+le32(h, 8), 24)
	frames, perFrame, final := le32(hdr, 12), le32(hdr, 4), le32(hdr, 8)
	a.BitDepth, a.Channels, a.SampleRate = le16(hdr, 16), le16(hdr, 18), int(le32(hdr, 20))
	if frames > 0 {
		a.Duration = seconds((frames-1)*perFrame+final, a.SampleRate)
	}
	a.estimate(s.size - off)
	t.Audio = a
	return nil
}

var wavPackRates = []int{6000, 8000, 9600, 11025, 12000, 16000, 22050, 24000, 32000, 44100, 48000, 64000, 88200, 96000, 192000}

// readWavPack reads the first WavPack block header: total samples and the
// flags word holding bytes per sample, mono and the sample rate index.
func readWavPack(s source, off int64, t *Tags) error {
	h := s.at(off, 32)
	flags := le32(h, 24)
	a := AudioInfo{Codec: CodecWavPack, BitDepth: int(flags&3+1) * 8, Channels: 2 - int(flags>>2&1)}
	if ri := int(flags >> 23 & 0xf); ri < len(wavPackRates) {
		a.SampleRate = wavPackRates[ri]
	}
	a.Duration = seconds(le32(h, 12), a.SampleRate)
	a.estimate(s.size - off)
	t.Audio = a
	return nil
}

// readWAV walks the RIFF chunks for the format and data chunks.
func readWAV(s source, off int64, t *Tags) error {
	a := AudioInfo{Codec: CodecPCM}
	var data, byteRate int64
	for pos := off + 12; pos+8 <= s.size; {
		h := s.at(pos, 8)
		n := le32(h, 4)
		switch string(slice(h, 0, 4)) {
		case "fmt ":
			byteRate = wavFormat(s.at(pos+8, int(n)), &a)
		case "data":
			data = min(n, s.size-pos-8)
		}
		pos += 8 + n + n&1
	}
	if a.SampleRate == 0 {
		return ErrMalformed
	}
	a.Duration = seconds(data, int(byteRate))
	t.Audio = a
	return nil
}

// wavFormat decodes the format chunk and returns the byte rate.
func wavFormat(b []byte, a *AudioInfo) int64 {
	a.Channels, a.SampleRate, a.BitDepth = le16(b, 2), int(le32(b, 4)), le16(b, 14)
	a.Bitrate = int(le32(b, 8) * 8 / 1000)
	return le32(b, 8)
}
