// Package release writes release archives that are byte-for-byte reproducible:
// entries in sorted order, one fixed modification time, owner 0:0 with no
// names, normalised permissions, and no host details in the gzip or zip headers.
package release

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// entry is one file or directory inside the archive, slash-separated.
type entry struct {
	name string
	dir  bool
	exec bool
	src  string
}

// Pack archives the directory root as a single top-level folder named after
// it. The format follows dst: .tar.gz or .zip. Every entry gets mtime.
func Pack(root, dst string, mtime time.Time) error {
	entries, err := collect(root)
	if err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	switch {
	case strings.HasSuffix(dst, ".tar.gz"):
		err = writeTarGz(f, entries, mtime)
	case strings.HasSuffix(dst, ".zip"):
		err = writeZip(f, entries, mtime)
	default:
		err = fmt.Errorf("unknown archive type %q (want .tar.gz or .zip)", dst)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// collect lists root and everything under it, sorted by archive path.
func collect(root string) ([]entry, error) {
	base := filepath.Base(filepath.Clean(root))
	var out []entry
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		name := path.Join(base, filepath.ToSlash(rel))
		if !d.IsDir() && !d.Type().IsRegular() {
			return fmt.Errorf("%s: only regular files and directories can be archived", p)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, entry{name: name, dir: d.IsDir(), exec: info.Mode()&0o111 != 0, src: p})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, err
}

// mode is 0755 for directories and executables, 0644 otherwise.
func (e entry) mode() int64 {
	if e.dir || e.exec {
		return 0o755
	}
	return 0o644
}

func writeTarGz(w io.Writer, entries []entry, mtime time.Time) error {
	gz, _ := gzip.NewWriterLevel(w, gzip.BestCompression) // the header keeps no name and a zero time
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: e.mode(), ModTime: mtime, Typeflag: tar.TypeReg, Format: tar.FormatPAX}
		if e.dir {
			h.Name, h.Typeflag = e.name+"/", tar.TypeDir
		}
		if err := addTar(tw, h, e); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func addTar(tw *tar.Writer, h *tar.Header, e entry) error {
	if e.dir {
		return tw.WriteHeader(h)
	}
	data, err := os.ReadFile(e.src)
	if err != nil {
		return err
	}
	h.Size = int64(len(data))
	if err := tw.WriteHeader(h); err != nil {
		return err
	}
	_, err = tw.Write(data)
	return err
}

func writeZip(w io.Writer, entries []entry, mtime time.Time) error {
	zw := zip.NewWriter(w)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate, Modified: mtime.UTC()}
		h.SetMode(fs.FileMode(e.mode()))
		if e.dir {
			h.Name, h.Method = e.name+"/", zip.Store
			h.SetMode(fs.ModeDir | 0o755)
		}
		if err := addZip(zw, h, e); err != nil {
			return err
		}
	}
	return zw.Close()
}

func addZip(zw *zip.Writer, h *zip.FileHeader, e entry) error {
	out, err := zw.CreateHeader(h)
	if err != nil || e.dir {
		return err
	}
	data, err := os.ReadFile(e.src)
	if err != nil {
		return err
	}
	_, err = out.Write(data)
	return err
}
