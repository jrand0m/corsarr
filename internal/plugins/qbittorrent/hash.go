package qbittorrent

import (
	"crypto/sha1" //nolint:gosec // BitTorrent v1 info-hashes are SHA-1 by definition.
	"encoding/base32"
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/plugin"
)

const maxBencodeDepth = 32

// magnetHash extracts the lower-case hex info hash from a magnet URI
// (40 hex characters or 32 base32 characters).
func magnetHash(magnet string) (string, error) {
	u, err := url.Parse(magnet)
	if err != nil || u.Scheme != "magnet" {
		return "", plugin.Errorf(plugin.ClassParse, "not a magnet link")
	}
	for _, xt := range u.Query()["xt"] {
		if h, ok := strings.CutPrefix(strings.ToLower(xt), "urn:btih:"); ok {
			return normalizeHash(h)
		}
	}
	return "", plugin.Errorf(plugin.ClassParse, "magnet link has no btih info hash")
}

func normalizeHash(h string) (string, error) {
	switch len(h) {
	case 40:
		if _, err := hex.DecodeString(h); err == nil {
			return h, nil
		}
	case 32:
		if raw, err := base32.StdEncoding.DecodeString(strings.ToUpper(h)); err == nil {
			return hex.EncodeToString(raw), nil
		}
	}
	return "", plugin.Errorf(plugin.ClassParse, "invalid info hash %q", h)
}

// torrentHash computes the v1 info hash: SHA-1 of the bencoded "info" value.
func torrentHash(torrent []byte) (string, error) {
	if len(torrent) == 0 || torrent[0] != 'd' {
		return "", plugin.Errorf(plugin.ClassParse, "not a torrent file")
	}
	i := 1
	for i < len(torrent) && torrent[i] != 'e' {
		key, next, err := readString(torrent, i)
		if err != nil {
			return "", err
		}
		end, err := skip(torrent, next, 0)
		if err != nil {
			return "", err
		}
		if key == "info" {
			sum := sha1.Sum(torrent[next:end]) //nolint:gosec // see import
			return hex.EncodeToString(sum[:]), nil
		}
		i = end
	}
	return "", plugin.Errorf(plugin.ClassParse, "torrent file has no info dictionary")
}

func malformed() error { return plugin.Errorf(plugin.ClassParse, "malformed torrent file") }

// digitsEnd returns the offset after the run of ASCII digits starting at i.
func digitsEnd(b []byte, i int) int {
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		i++
	}
	return i
}

// readString reads a bencoded byte string at i and returns it with the next offset.
func readString(b []byte, i int) (string, int, error) {
	colon := digitsEnd(b, i)
	if colon == i || colon >= len(b) || b[colon] != ':' {
		return "", 0, malformed()
	}
	n, err := strconv.Atoi(string(b[i:colon]))
	if err != nil || n > len(b)-colon-1 {
		return "", 0, malformed()
	}
	return string(b[colon+1 : colon+1+n]), colon + 1 + n, nil
}

// skip returns the offset just after the bencoded value starting at i.
func skip(b []byte, i, depth int) (int, error) {
	if i >= len(b) || depth > maxBencodeDepth {
		return 0, malformed()
	}
	switch c := b[i]; {
	case c == 'i':
		end := indexByte(b, i, 'e')
		if end < 0 {
			return 0, malformed()
		}
		return end + 1, nil
	case c == 'l' || c == 'd':
		return skipContainer(b, i+1, depth+1)
	case c >= '0' && c <= '9':
		_, next, err := readString(b, i)
		return next, err
	}
	return 0, malformed()
}

func skipContainer(b []byte, i, depth int) (int, error) {
	for i < len(b) && b[i] != 'e' {
		next, err := skip(b, i, depth)
		if err != nil {
			return 0, err
		}
		i = next
	}
	if i >= len(b) {
		return 0, malformed()
	}
	return i + 1, nil
}

func indexByte(b []byte, from int, c byte) int {
	for i := from; i < len(b); i++ {
		if b[i] == c {
			return i
		}
	}
	return -1
}
