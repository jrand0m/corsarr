package release

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var epoch = time.Unix(1700000000, 0)

// tree builds pkg/{bin, README.md, sub/x} with the given mtime and modes.
func tree(t *testing.T, mtime time.Time, binMode os.FileMode) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "pkg")
	must(t, os.MkdirAll(filepath.Join(root, "sub"), 0o700))
	must(t, os.WriteFile(filepath.Join(root, "bin"), []byte("binary"), binMode))
	must(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("readme"), 0o600))
	must(t, os.WriteFile(filepath.Join(root, "sub", "x"), []byte("x"), 0o666))
	for _, p := range []string{"bin", "README.md", "sub/x", "sub", "."} {
		must(t, os.Chtimes(filepath.Join(root, p), mtime, mtime))
	}
	return root
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func packBytes(t *testing.T, root, ext string) []byte {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "out"+ext)
	must(t, Pack(root, dst, epoch))
	b, err := os.ReadFile(dst)
	must(t, err)
	return b
}

func TestPackIsReproducible(t *testing.T) {
	for _, ext := range []string{".tar.gz", ".zip"} {
		a := packBytes(t, tree(t, time.Now(), 0o755), ext)
		b := packBytes(t, tree(t, time.Now().Add(-time.Hour), 0o700), ext)
		if !bytes.Equal(a, b) {
			t.Fatalf("%s differs between trees that differ only in mtime and permissions", ext)
		}
	}
}

func TestPackTarEntries(t *testing.T) {
	gz, err := gzip.NewReader(bytes.NewReader(packBytes(t, tree(t, time.Now(), 0o755), ".tar.gz")))
	must(t, err)
	if gz.Name != "" || !gz.ModTime.IsZero() {
		t.Fatalf("gzip header leaks %q %v", gz.Name, gz.ModTime)
	}
	tr := tar.NewReader(gz)
	var got []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		must(t, err)
		if !h.ModTime.Equal(epoch) || h.Uid != 0 || h.Gid != 0 || h.Uname != "" {
			t.Fatalf("%s: %v %d:%d %q", h.Name, h.ModTime, h.Uid, h.Gid, h.Uname)
		}
		got = append(got, h.Name+" "+os.FileMode(h.Mode).String())
	}
	want := []string{"pkg/ -rwxr-xr-x", "pkg/README.md -rw-r--r--", "pkg/bin -rwxr-xr-x", "pkg/sub/ -rwxr-xr-x", "pkg/sub/x -rw-r--r--"}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entry %d: %q want %q", i, got[i], want[i])
		}
	}
}

func TestPackZipEntries(t *testing.T) {
	b := packBytes(t, tree(t, time.Now(), 0o755), ".zip")
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	must(t, err)
	var names []string
	for _, f := range zr.File {
		if !f.Modified.Equal(epoch) {
			t.Fatalf("%s: %v", f.Name, f.Modified)
		}
		names = append(names, f.Name)
	}
	if len(names) != 5 || names[0] != "pkg/" || names[2] != "pkg/bin" || !zr.File[2].Mode().IsRegular() || zr.File[2].Mode().Perm() != 0o755 {
		t.Fatalf("%v", names)
	}
}

func TestPackRejects(t *testing.T) {
	root := tree(t, time.Now(), 0o755)
	if err := Pack(root, filepath.Join(t.TempDir(), "out.rar"), epoch); err == nil {
		t.Fatal("unknown extension accepted")
	}
	must(t, os.Symlink("bin", filepath.Join(root, "link")))
	if err := Pack(root, filepath.Join(t.TempDir(), "out.zip"), epoch); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := Pack(filepath.Join(root, "missing"), filepath.Join(t.TempDir(), "out.zip"), epoch); err == nil {
		t.Fatal("missing root accepted")
	}
	if err := Pack(root, filepath.Join(t.TempDir(), "no", "out.zip"), epoch); err == nil {
		t.Fatal("unwritable destination accepted")
	}
}
