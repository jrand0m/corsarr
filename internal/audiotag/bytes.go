package audiotag

import (
	"encoding/binary"
	"io"
)

// maxBlock caps any single tag or header block loaded into memory, so a
// corrupt length field cannot make the reader allocate gigabytes.
const maxBlock = 32 << 20

// source is a random-access file of a known size.
type source struct {
	r    io.ReaderAt
	size int64
}

// at returns up to n bytes at off: fewer near the end of the file, nil when
// off is outside the file or n is not a sane block length.
func (s source) at(off int64, n int) []byte {
	if off < 0 || off >= s.size || n <= 0 || n > maxBlock {
		return nil
	}
	n = int(min(int64(n), s.size-off))
	b := make([]byte, n)
	got, err := s.r.ReadAt(b, off)
	if got < n && err != nil {
		return b[:got]
	}
	return b
}

// slice returns b[off:off+n], or nil when that range is not inside b.
func slice(b []byte, off, n int) []byte {
	if off < 0 || n < 0 || off+n > len(b) {
		return nil
	}
	return b[off : off+n]
}

// rest returns b from off on, or nil when off is past the end.
func rest(b []byte, off int) []byte {
	if off > len(b) {
		return nil
	}
	return b[off:]
}

func be16(b []byte, off int) int {
	if p := slice(b, off, 2); p != nil {
		return int(binary.BigEndian.Uint16(p))
	}
	return 0
}

func be24(b []byte, off int) int {
	if p := slice(b, off, 3); p != nil {
		return int(p[0])<<16 | int(p[1])<<8 | int(p[2])
	}
	return 0
}

func be32(b []byte, off int) int64 {
	if p := slice(b, off, 4); p != nil {
		return int64(binary.BigEndian.Uint32(p))
	}
	return 0
}

func be64(b []byte, off int) int64 {
	if p := slice(b, off, 8); p != nil {
		return int64(binary.BigEndian.Uint64(p))
	}
	return 0
}

func le16(b []byte, off int) int {
	if p := slice(b, off, 2); p != nil {
		return int(binary.LittleEndian.Uint16(p))
	}
	return 0
}

func le32(b []byte, off int) int64 {
	if p := slice(b, off, 4); p != nil {
		return int64(binary.LittleEndian.Uint32(p))
	}
	return 0
}

func le64(b []byte, off int) int64 {
	if p := slice(b, off, 8); p != nil {
		return int64(binary.LittleEndian.Uint64(p))
	}
	return 0
}

// syncsafe decodes a 28-bit integer stored 7 bits per byte.
func syncsafe(b []byte) int {
	n := 0
	for _, c := range slice(b, 0, 4) {
		n = n<<7 | int(c&0x7f)
	}
	return n
}
