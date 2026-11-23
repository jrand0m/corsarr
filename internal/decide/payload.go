package decide

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

// shippedDangerous is the product's one dangerous-extension list. Users may
// add to it; nothing here can be removed.
var shippedDangerous = setOf(
	"exe", "scr", "bat", "cmd", "com", "msi", "msp", "pif", "vbs", "vbe", "js", "jse", "wsf", "wsh",
	"ps1", "psm1", "jar", "lnk", "url", "apk", "dll", "hta", "cpl", "msc", "reg", "scf", "inf",
	"dmg", "app", "pkg", "appx", "msix", "deb", "rpm", "sh", "run", "gadget", "application",
)

var videoExts = setOf("mkv", "mp4", "m4v", "avi", "mov", "wmv", "ts", "m2ts", "mts", "webm",
	"mpg", "mpeg", "vob", "iso", "img", "ogm", "divx", "flv")

func setOf(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// DangerousExtensions lists the shipped minimum, sorted.
func DangerousExtensions() []string {
	out := make([]string, 0, len(shippedDangerous))
	for e := range shippedDangerous {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}

// PayloadPolicy is the shipped list plus user additions. The zero value is
// the shipped list alone.
type PayloadPolicy struct {
	extra map[string]bool
}

var extRe = regexp.MustCompile(`^[a-z0-9]{1,16}$`)

// NewPayloadPolicy accepts extensions with or without the leading dot.
func NewPayloadPolicy(additions ...string) (PayloadPolicy, error) {
	p := PayloadPolicy{extra: map[string]bool{}}
	for _, a := range additions {
		e := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(a), "."))
		if !extRe.MatchString(e) {
			return PayloadPolicy{}, fmt.Errorf("extension %q must be 1-16 letters or digits", a)
		}
		p.extra[e] = true
	}
	return p, nil
}

func (p PayloadPolicy) dangerousExt(e string) bool { return shippedDangerous[e] || p.extra[e] }

// PayloadFile is one file of a release; Size 0 means unknown.
type PayloadFile struct {
	Path string
	Size int64
}

// PayloadStatus is the payload verdict outcome.
type PayloadStatus string

const (
	PayloadUnknown   PayloadStatus = "unknown"   // no file list yet: check again after download
	PayloadSafe      PayloadStatus = "safe"      // nothing suspicious
	PayloadDangerous PayloadStatus = "dangerous" // executable or script inside
	PayloadNotVideo  PayloadStatus = "not_video" // largest file is not video for a video release
)

// PayloadVerdict names the offending files, if any.
type PayloadVerdict struct {
	Status PayloadStatus `json:"status"`
	Files  []string      `json:"files,omitempty"`
}

// normalizeName returns the lower-case base name without trailing dots or
// spaces (Windows ignores them, so "x.exe." runs as x.exe).
func normalizeName(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	return strings.ToLower(strings.TrimRight(path.Base(p), ". "))
}

func extOf(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return ""
}

func (p PayloadPolicy) isDangerous(file string) bool {
	name := normalizeName(file)
	// U+202E right-to-left override disguises "xxx‮vkm.exe" as "xxxexe.mkv".
	return strings.ContainsRune(name, '‮') || p.dangerousExt(extOf(name))
}

// Verdict judges a file list. wantVideo applies the "largest file must be
// video" rule when sizes are known.
func (p PayloadPolicy) Verdict(files []PayloadFile, wantVideo bool) PayloadVerdict {
	if len(files) == 0 {
		return PayloadVerdict{Status: PayloadUnknown}
	}
	var bad []string
	for _, f := range files {
		if p.isDangerous(f.Path) {
			bad = append(bad, f.Path)
		}
	}
	if len(bad) > 0 {
		return PayloadVerdict{Status: PayloadDangerous, Files: bad}
	}
	if big, ok := largest(files); ok && wantVideo && !videoExts[extOf(normalizeName(big.Path))] {
		return PayloadVerdict{Status: PayloadNotVideo, Files: []string{big.Path}}
	}
	return PayloadVerdict{Status: PayloadSafe}
}

// largest returns the biggest file; ok is false when no size is known.
func largest(files []PayloadFile) (PayloadFile, bool) {
	var big PayloadFile
	for _, f := range files {
		if f.Size > big.Size {
			big = f
		}
	}
	return big, big.Size > 0
}

// FilesFromNames wraps a size-less file list (indexer or torrent metadata).
func FilesFromNames(names []string) []PayloadFile {
	out := make([]PayloadFile, len(names))
	for i, n := range names {
		out[i] = PayloadFile{Path: n}
	}
	return out
}
