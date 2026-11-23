package importer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ErrNoSpace is returned when a copy would not fit.
var ErrNoSpace = errors.New("not enough free space")

// Placement says how a file reached the library.
type Placement string

const (
	PlacedHardlink Placement = "hardlink"
	PlacedCopy     Placement = "copy"
	PlacedExisting Placement = "existing"
)

// link is os.Link; tests swap it to force the cross-filesystem path.
var link = os.Link

// Place puts src at dest: a hardlink when both share a filesystem, else a
// copy written under a temporary name, size-verified and renamed atomically.
// A dest that already is src is left alone.
func Place(src, dest string, freeSpace func(string) int64) (Placement, error) {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if same(src, dest) {
		return PlacedExisting, nil
	}
	if err := link(src, dest); err == nil {
		return PlacedHardlink, nil
	}
	return PlacedCopy, copyFile(src, dest, freeSpace)
}

func same(a, b string) bool {
	sa, err1 := os.Stat(a)
	sb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(sa, sb)
}

func copyFile(src, dest string, freeSpace func(string) int64) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	if freeSpace != nil {
		if err := EnoughSpace(filepath.Dir(dest), freeSpace(filepath.Dir(dest)), st.Size()); err != nil {
			return err
		}
	}
	tmp := dest + ".corsarr-partial"
	if err := writeVerified(in, tmp, st.Size()); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("copy of %s: %w", src, err)
	}
	return os.Rename(tmp, dest)
}

// writeVerified writes the temp copy and checks its size.
func writeVerified(in io.Reader, tmp string, size int64) error {
	if err := writeAll(in, tmp); err != nil {
		return err
	}
	got, err := os.Stat(tmp)
	if err == nil && got.Size() != size {
		err = errors.New("incomplete copy")
	}
	return err
}

func writeAll(in io.Reader, path string) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// move renames src to dest, copying across filesystems.
func move(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dest); err == nil {
		return nil
	}
	if err := copyFile(src, dest, nil); err != nil {
		return err
	}
	return os.Remove(src)
}
