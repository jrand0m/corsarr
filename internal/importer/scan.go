package importer

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jrand0m/corsarr/internal/decide"
)

// Class is a file's role in a payload.
type Class string

const (
	ClassVideo      Class = "video"
	ClassSample     Class = "sample"
	ClassExtra      Class = "extra"
	ClassExecutable Class = "executable"
	ClassOther      Class = "other"
)

// File is one payload file.
type File struct {
	Path  string // absolute
	Rel   string // relative to the payload root
	Size  int64
	Class Class
}

var videoExts = map[string]bool{".mkv": true, ".mp4": true, ".m4v": true, ".avi": true, ".mov": true, ".wmv": true,
	".ts": true, ".m2ts": true, ".webm": true, ".mpg": true, ".mpeg": true}

var extraExts = map[string]bool{".srt": true, ".ass": true, ".ssa": true, ".sub": true, ".idx": true, ".vtt": true}

// Classify gives a file its class; largest is the biggest video size in the
// payload (samples are small videos named or filed as samples).
func Classify(rel string, size, largest int64, policy decide.PayloadPolicy) Class {
	ext := strings.ToLower(filepath.Ext(rel))
	switch {
	case policy.Verdict([]decide.PayloadFile{{Path: rel}}, false).Status == decide.PayloadDangerous:
		return ClassExecutable
	case videoExts[ext] && isSample(rel, size, largest):
		return ClassSample
	case videoExts[ext]:
		return ClassVideo
	case extraExts[ext]:
		return ClassExtra
	}
	return ClassOther
}

func isSample(rel string, size, largest int64) bool {
	name := strings.ToLower(rel)
	named := strings.Contains(name, "sample")
	return named && (largest == 0 || size < largest/2)
}

// junkNames are system files that never belong to a payload.
var junkNames = map[string]bool{"thumbs.db": true, "desktop.ini": true, "@eadir": true, "#recycle": true, "#snapshot": true}

// partialSuffixes mark files a client is still writing.
var partialSuffixes = []string{".part", ".partial~", ".!qb", ".!ut", ".crdownload"}

// junk reports whether a file or folder name is system clutter or a partial
// download: hidden entries (macOS AppleDouble "._" files and dot-folders
// included), NAS thumbnail and index folders, Windows shell files and the
// temporary names clients write to.
func junk(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasPrefix(name, ".") || junkNames[lower] {
		return true
	}
	for _, suf := range partialSuffixes {
		if strings.HasSuffix(lower, suf) {
			return true
		}
	}
	return false
}

// junkDir also skips sample folders inside a payload.
func junkDir(name string) bool {
	lower := strings.ToLower(name)
	return junk(name) || lower == "sample" || lower == "samples"
}

// Scan walks the payload (a file or a folder) streaming, then classifies
// every regular file; junk files and folders below the root are skipped.
func Scan(root string, policy decide.PayloadPolicy) ([]File, error) {
	var files []File
	err := filepath.WalkDir(root, func(p string, de fs.DirEntry, err error) error {
		if err != nil || p == root && de.IsDir() {
			return err
		}
		if skip, ret := skipEntry(de); skip {
			return ret
		}
		info, err := de.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(filepath.Dir(root), p) // p is under root
		files = append(files, File{Path: p, Rel: rel, Size: info.Size()})
		return nil
	})
	return classifyAll(files, policy), err
}

// skipEntry decides whether the walk passes over an entry, and what to return
// for it (SkipDir for junk folders).
func skipEntry(de fs.DirEntry) (bool, error) {
	switch {
	case de.IsDir() && junkDir(de.Name()):
		return true, filepath.SkipDir
	case de.IsDir() || !de.Type().IsRegular() || junk(de.Name()):
		return true, nil
	}
	return false, nil
}

// ScanList classifies the listed files below base (paths relative to it, with
// forward slashes), for a download whose files share a folder with other
// downloads. Junk entries are skipped; a listed file that is missing, or a
// path leaving base, is an error.
func ScanList(base string, paths []string, policy decide.PayloadPolicy) ([]File, error) {
	var files []File
	for _, p := range paths {
		rel := filepath.FromSlash(p)
		if !filepath.IsLocal(rel) {
			return nil, fmt.Errorf("listed file %s lies outside %s", p, base)
		}
		if junkPath(rel) {
			continue
		}
		full := filepath.Join(base, rel)
		info, err := os.Lstat(full)
		if err != nil {
			return nil, err
		}
		if info.Mode().IsRegular() {
			files = append(files, File{Path: full, Rel: rel, Size: info.Size()})
		}
	}
	return classifyAll(files, policy), nil
}

// junkPath reports whether any part of a relative path is junk.
func junkPath(rel string) bool {
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		if junk(part) || i < len(parts)-1 && junkDir(part) {
			return true
		}
	}
	return false
}

// classifyAll gives every file its class against the largest video.
func classifyAll(files []File, policy decide.PayloadPolicy) []File {
	var largest int64
	for _, f := range files {
		if videoExts[strings.ToLower(filepath.Ext(f.Path))] && f.Size > largest {
			largest = f.Size
		}
	}
	for i := range files {
		files[i].Class = Classify(files[i].Rel, files[i].Size, largest, policy)
	}
	return files
}

// MainVideo is the largest non-sample video, if any.
func MainVideo(files []File) (File, bool) {
	var best File
	for _, f := range files {
		if f.Class == ClassVideo && f.Size >= best.Size {
			best = f
		}
	}
	return best, best.Path != ""
}

// Executables lists the dangerous files.
func Executables(files []File) []string {
	var out []string
	for _, f := range files {
		if f.Class == ClassExecutable {
			out = append(out, f.Rel)
		}
	}
	return out
}
