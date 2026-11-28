package client

import (
	"bytes"
	"crypto/sha1" //nolint:gosec // BitTorrent v1 info hashes are SHA-1 by definition.
	"encoding/base32"
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"
)

const maxBencodeDepth = 32

// MagnetHash returns the upper-case hex info hash of a magnet link (40 hex or
// 32 base32 characters), the form rTorrent reports.
func MagnetHash(magnet string) (string, error) {
	u, err := url.Parse(magnet)
	if err != nil || u.Scheme != "magnet" {
		return "", malformed("not a magnet link")
	}
	for _, xt := range u.Query()["xt"] {
		if h, ok := strings.CutPrefix(strings.ToLower(xt), "urn:btih:"); ok {
			return normalizeHash(h)
		}
	}
	return "", malformed("magnet link has no btih info hash")
}

func normalizeHash(h string) (string, error) {
	switch len(h) {
	case 40:
		if _, err := hex.DecodeString(h); err == nil {
			return strings.ToUpper(h), nil
		}
	case 32:
		if raw, err := base32.StdEncoding.DecodeString(strings.ToUpper(h)); err == nil {
			return strings.ToUpper(hex.EncodeToString(raw)), nil
		}
	}
	return "", malformed("invalid info hash %q", truncate(h))
}

// TorrentHash returns the upper-case hex v1 info hash of a .torrent file:
// SHA-1 of the bencoded info dictionary.
func TorrentHash(torrent []byte) (string, error) {
	if len(torrent) == 0 || torrent[0] != 'd' {
		return "", malformed("not a torrent file")
	}
	i := 1
	for i < len(torrent) && torrent[i] != 'e' {
		key, next, err := bencodeString(torrent, i)
		if err != nil {
			return "", err
		}
		end, err := bencodeSkip(torrent, next, 0)
		if err != nil {
			return "", err
		}
		if key == "info" {
			sum := sha1.Sum(torrent[next:end]) //nolint:gosec // see import
			return strings.ToUpper(hex.EncodeToString(sum[:])), nil
		}
		i = end
	}
	return "", malformed("torrent file has no info dictionary")
}

func badTorrent() error { return malformed("malformed torrent file") }

func digitsEnd(b []byte, i int) int {
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		i++
	}
	return i
}

func bencodeString(b []byte, i int) (string, int, error) {
	colon := digitsEnd(b, i)
	if colon == i || colon >= len(b) || b[colon] != ':' {
		return "", 0, badTorrent()
	}
	n, err := strconv.Atoi(string(b[i:colon]))
	if err != nil || n > len(b)-colon-1 {
		return "", 0, badTorrent()
	}
	return string(b[colon+1 : colon+1+n]), colon + 1 + n, nil
}

// bencodeSkip returns the offset just after the value starting at i.
func bencodeSkip(b []byte, i, depth int) (int, error) {
	if i >= len(b) || depth > maxBencodeDepth {
		return 0, badTorrent()
	}
	switch c := b[i]; {
	case c == 'i':
		end := bytes.IndexByte(b[i:], 'e')
		if end < 0 {
			return 0, badTorrent()
		}
		return i + end + 1, nil
	case c == 'l' || c == 'd':
		return bencodeContainer(b, i+1, depth+1)
	case c >= '0' && c <= '9':
		_, next, err := bencodeString(b, i)
		return next, err
	}
	return 0, badTorrent()
}

func bencodeContainer(b []byte, i, depth int) (int, error) {
	for i < len(b) && b[i] != 'e' {
		next, err := bencodeSkip(b, i, depth)
		if err != nil {
			return 0, err
		}
		i = next
	}
	if i >= len(b) {
		return 0, badTorrent()
	}
	return i + 1, nil
}
