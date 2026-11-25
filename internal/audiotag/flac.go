package audiotag

// FLAC metadata block types.
const (
	flacStreamInfo = 0
	flacComments   = 4
)

// readFLAC walks the metadata blocks after the "fLaC" marker, reading the
// stream info and the Vorbis comments and skipping everything else
// (pictures, seek tables, padding) without loading it.
func readFLAC(s source, off int64, t *Tags) error {
	pos := off + 4
	for {
		h := s.at(pos, 4)
		if len(h) < 4 {
			return ErrMalformed
		}
		n := be24(h, 1)
		flacBlock(s, h[0]&0x7f, pos+4, n, t)
		pos += 4 + int64(n)
		if h[0]&0x80 != 0 {
			break
		}
	}
	if t.Audio.Codec == "" {
		return ErrMalformed
	}
	t.Audio.estimate(s.size - pos)
	return nil
}

func flacBlock(s source, typ byte, pos int64, n int, t *Tags) {
	switch typ {
	case flacStreamInfo:
		t.Audio = streamInfo(s.at(pos, n))
	case flacComments:
		readComments(s.at(pos, n), t)
	}
}

// streamInfo decodes a FLAC STREAMINFO block: 20 bits of sample rate,
// 3 of channels-1, 5 of bits-per-sample-1 and 36 of total samples, starting
// at byte 10.
func streamInfo(b []byte) AudioInfo {
	p := slice(b, 10, 8)
	if p == nil {
		return AudioInfo{}
	}
	rate := int(p[0])<<12 | int(p[1])<<4 | int(p[2])>>4
	total := int64(p[3]&0x0f)<<32 | be32(p, 4)
	return AudioInfo{Codec: CodecFLAC, SampleRate: rate, Channels: int(p[2]>>1&7) + 1,
		BitDepth: (int(p[2]&1)<<4 | int(p[3]>>4)) + 1, Duration: seconds(total, rate)}
}
