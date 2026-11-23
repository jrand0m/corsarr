package importer

import (
	"io/fs"
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

// Scan walks the payload (a file or a folder) streaming, then classifies
// every regular file.
func Scan(root string, policy decide.PayloadPolicy) ([]File, error) {
	var files []File
	var largest int64
	err := filepath.WalkDir(root, func(p string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() || !de.Type().IsRegular() {
			return err
		}
		info, err := de.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(filepath.Dir(root), p) // p is under root
		files = append(files, File{Path: p, Rel: rel, Size: info.Size()})
		if videoExts[strings.ToLower(filepath.Ext(p))] && info.Size() > largest {
			largest = info.Size()
		}
		return nil
	})
	for i := range files {
		files[i].Class = Classify(files[i].Rel, files[i].Size, largest, policy)
	}
	return files, err
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
